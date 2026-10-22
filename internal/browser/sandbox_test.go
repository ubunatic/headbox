package browser

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"ubunatic.com/headbox/spec"
)

func TestBwrapArgs(t *testing.T) {
	b := spec.MustBrowser()
	args := BwrapArgs(b, "/x/browser", Options{Port: 0, Args: []string{"--foo"}, ID: "t1"})
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--unshare-all --share-net --die-with-parent --new-session --clearenv",
		fmt.Sprintf("--size %d --tmpfs /tmp", b.TmpBytes),
		fmt.Sprintf("--size %d --tmpfs /dev/shm", b.ShmBytes),
		fmt.Sprintf("--size %d --tmpfs %s", b.HomeBytes, sandboxHome),
		"--ro-bind /x/browser /opt/browser",
		"--setenv HEADBOX_ID t1",
		"-- /opt/browser/" + BinName + " --headless --remote-debugging-port=0 --foo about:blank",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args miss %q:\n%s", want, joined)
		}
	}
	if slices.Contains(args, "--no-sandbox") {
		t.Error("Chrome's own sandbox works inside bwrap; --no-sandbox must not be set")
	}
}

// installedBrowser skips unless bwrap and the pinned browser are present.
func installedBrowser(t *testing.T) (*spec.Browser, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("starts a browser")
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	b := spec.MustBrowser()
	dir, err := Dir(b)
	if err != nil || !Installed(dir) {
		t.Skip("browser not installed (headbox install)")
	}
	return b, dir
}

func TestStartStop(t *testing.T) {
	b, dir := installedBrowser(t)
	id := fmt.Sprintf("test-%d", time.Now().UnixNano())
	r, err := Start(b, dir, Options{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(r.HTTPURL() + "/json/version")
	if err != nil {
		r.Stop()
		t.Fatal(err)
	}
	resp.Body.Close()
	if n := len(ProcsWithID(id)); n == 0 {
		t.Error("no sandboxed process found by HEADBOX_ID")
	}
	r.Stop()
	WaitNoProcs(t, id)
}

// ProcsWithID lists PIDs whose environment has HEADBOX_ID=id.
func ProcsWithID(id string) []string {
	var pids []string
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		env, err := os.ReadFile("/proc/" + e.Name() + "/environ")
		if err != nil {
			continue
		}
		for _, kv := range strings.Split(string(env), "\x00") {
			if kv == "HEADBOX_ID="+id {
				pids = append(pids, e.Name())
			}
		}
	}
	return pids
}

// WaitNoProcs fails unless all processes with id are gone within 5 s.
func WaitNoProcs(t *testing.T, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(ProcsWithID(id)) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("processes left: %v", ProcsWithID(id))
}
