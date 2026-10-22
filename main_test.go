package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"ubunatic.com/headbox/internal/browser"
	"ubunatic.com/headbox/spec"
)

// headbox builds the binary and skips unless bwrap and the browser are installed.
func headbox(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("starts a browser")
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	dir, err := browser.Dir(spec.MustBrowser())
	if err != nil || !browser.Installed(dir) {
		t.Skip("browser not installed (headbox install)")
	}
	bin := filepath.Join(t.TempDir(), "headbox")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func newID() string { return fmt.Sprintf("test-%d", time.Now().UnixNano()) }

func withID(cmd *exec.Cmd, id string) *exec.Cmd {
	cmd.Env = append(os.Environ(), "HEADBOX_ID="+id)
	return cmd
}

func procsWithID(id string) []string {
	var pids []string
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		env, err := os.ReadFile("/proc/" + e.Name() + "/environ")
		if err == nil && strings.Contains(string(env), "\x00HEADBOX_ID="+id+"\x00") {
			pids = append(pids, e.Name())
		}
	}
	return pids
}

// waitFor polls cond for up to 10 s.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestExecPassesExitCode(t *testing.T) {
	bin := headbox(t)
	id := newID()
	cmd := withID(exec.Command(bin, "exec", "--", "sh", "-c",
		`curl -sf "$CDP_URL/json/version" >/dev/null && test -n "$HEADBOX_WS_URL" && exit 7`), id)
	err := cmd.Run()
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 7 {
		t.Fatalf("want exit 7 (CDP reachable), got %v", err)
	}
	waitFor(t, "no processes left", func() bool { return len(procsWithID(id)) == 0 })
}

func TestKill9LeavesNothing(t *testing.T) {
	bin := headbox(t)
	id := newID()
	cmd := withID(exec.Command(bin, "exec", "--", "sleep", "60"), id)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// headbox, bwrap, the browser tree and sleep all carry the id.
	waitFor(t, "browser and command running", func() bool { return len(procsWithID(id)) >= 4 })
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	waitFor(t, "no processes left after kill -9", func() bool { return len(procsWithID(id)) == 0 })
}

func TestServeStopsWhenIdle(t *testing.T) {
	bin := headbox(t)
	id := newID()
	cmd := withID(exec.Command(bin, "serve", "--idle", "2s"), id)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill() })
	port := ""
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "HEADBOX_CDP_PORT="); ok {
			port = v
			break
		}
	}
	if port == "" {
		t.Fatal("serve printed no HEADBOX_CDP_PORT")
	}
	// An open connection keeps it alive past the idle time.
	conn, err := net.Dial("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		t.Fatalf("serve stopped while a client was connected: %v", err)
	case <-time.After(3 * time.Second):
	}
	conn.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve exit: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop after the idle time")
	}
	waitFor(t, "no processes left", func() bool { return len(procsWithID(id)) == 0 })
}
