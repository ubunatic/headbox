package browser

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"ubunatic.com/headbox/spec"
)

// sandboxHome is $HOME inside the sandbox, a size-capped tmpfs.
const sandboxHome = "/home/sandbox"

// readyTimeout bounds the wait for Chrome's "DevTools listening" line.
const readyTimeout = 20 * time.Second

var listening = regexp.MustCompile(`DevTools listening on (ws://127\.0\.0\.1:(\d+)/\S+)`)

// proxyVars are passed into the sandbox when set; everything else is cleared.
var proxyVars = []string{
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY",
	"http_proxy", "https_proxy", "no_proxy", "all_proxy",
}

// Options for Start.
type Options struct {
	Port int       // CDP port; 0 lets Chrome pick a free one
	Args []string  // extra Chrome flags
	Log  io.Writer // Chrome's stderr after the ready line; nil discards it
	ID   string    // HEADBOX_ID set inside the sandbox, lets tests find the tree
	// ROBinds are host paths bound read-only at the same path, for example the
	// directory of a local page; they are mounted after the tmpfs mounts, so paths
	// under /tmp work too.
	ROBinds []string
}

// Running is a started browser.
type Running struct {
	Port  int
	WSURL string
	cmd   *exec.Cmd
	done  chan struct{}
	err   error
}

// HTTPURL is the CDP HTTP endpoint (/json/version, /json/list).
func (r *Running) HTTPURL() string {
	return "http://127.0.0.1:" + strconv.Itoa(r.Port)
}

// BwrapArgs returns the bwrap command line for the browser in dir. The sandbox has its
// own PID, IPC and user namespaces, shares only the network, and keeps every writable
// path in a size-capped tmpfs, so ending bwrap ends the browser and frees its files.
func BwrapArgs(b *spec.Browser, dir string, opt Options) []string {
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = "C.UTF-8"
	}
	args := []string{
		"--unshare-all", "--share-net", "--die-with-parent", "--new-session",
		"--clearenv",
		"--setenv", "HOME", sandboxHome,
		"--setenv", "XDG_CACHE_HOME", sandboxHome + "/.cache",
		"--setenv", "XDG_CONFIG_HOME", sandboxHome + "/.config",
		"--setenv", "PATH", "/usr/bin",
		"--setenv", "LANG", lang,
	}
	if opt.ID != "" {
		args = append(args, "--setenv", "HEADBOX_ID", opt.ID)
	}
	// Chrome reads the proxy from the environment when no desktop config is present.
	for _, k := range proxyVars {
		if v, ok := os.LookupEnv(k); ok {
			args = append(args, "--setenv", k, v)
		}
	}
	args = append(args,
		"--proc", "/proc", "--dev", "/dev",
		"--ro-bind", "/usr", "/usr",
		"--ro-bind-try", "/lib64", "/lib64",
		"--ro-bind-try", "/etc", "/etc",
		// /etc/resolv.conf links here with systemd-resolved (Fedora, Ubuntu).
		"--ro-bind-try", "/run/systemd/resolve", "/run/systemd/resolve",
		"--ro-bind", dir, "/opt/browser",
		"--size", strconv.FormatInt(b.TmpBytes, 10), "--tmpfs", "/tmp",
		"--size", strconv.FormatInt(b.ShmBytes, 10), "--tmpfs", "/dev/shm",
		"--size", strconv.FormatInt(b.HomeBytes, 10), "--tmpfs", sandboxHome,
	)
	// The host's fontconfig cache saves about 300 ms per start (ubunatic.com 095).
	if fc := fontconfigCache(); fc != "" {
		args = append(args, "--ro-bind-try", fc, sandboxHome+"/.cache/fontconfig")
	}
	for _, p := range opt.ROBinds {
		args = append(args, "--ro-bind", p, p)
	}
	args = append(args,
		"--chdir", sandboxHome,
		"--", "/opt/browser/"+BinName, "--headless",
		"--remote-debugging-port="+strconv.Itoa(opt.Port),
	)
	args = append(args, opt.Args...)
	return append(args, "about:blank")
}

func fontconfigCache() string {
	if x := os.Getenv("XDG_CACHE_HOME"); x != "" {
		return filepath.Join(x, "fontconfig")
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".cache", "fontconfig")
	}
	return ""
}

// Start runs the browser in bwrap and waits until its CDP endpoint listens. bwrap is
// a direct child with Pdeathsig, so the browser dies with this process even on kill -9.
func Start(b *spec.Browser, dir string, opt Options) (*Running, error) {
	if !Installed(dir) {
		return nil, fmt.Errorf("browser not installed in %s, run: headbox install", dir)
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return nil, errors.New("bwrap not found; install bubblewrap")
	}
	cmd := exec.Command(bwrap, BwrapArgs(b, dir, opt)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL, Setpgid: true}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	r := &Running{cmd: cmd, done: make(chan struct{})}

	ready := make(chan []string, 1)
	var tail []string
	go func() {
		sc := bufio.NewScanner(stderr)
		found := false
		for sc.Scan() {
			line := sc.Text()
			if !found {
				if m := listening.FindStringSubmatch(line); m != nil {
					found = true
					ready <- m
					continue
				}
				tail = append(tail, line)
				if len(tail) > 20 {
					tail = tail[1:]
				}
				continue
			}
			if opt.Log != nil {
				fmt.Fprintln(opt.Log, line)
			}
		}
		if !found {
			close(ready)
		}
	}()
	go func() {
		r.err = cmd.Wait()
		close(r.done)
	}()

	select {
	case m, ok := <-ready:
		if !ok {
			<-r.done
			return nil, fmt.Errorf("browser exited before it was ready (%v):\n%s", r.err, strings.Join(tail, "\n"))
		}
		r.WSURL = m[1]
		r.Port, _ = strconv.Atoi(m[2])
		return r, nil
	case <-time.After(readyTimeout):
		r.Stop()
		return nil, fmt.Errorf("browser not ready after %v", readyTimeout)
	}
}

// Done is closed when the browser has exited.
func (r *Running) Done() <-chan struct{} { return r.done }

// Stop kills the sandbox's process group and waits for bwrap to exit. With its own PID
// namespace, the whole browser tree goes with it and the tmpfs mounts are freed.
func (r *Running) Stop() {
	select {
	case <-r.done:
		return
	default:
	}
	_ = syscall.Kill(-r.cmd.Process.Pid, syscall.SIGKILL)
	<-r.done
}
