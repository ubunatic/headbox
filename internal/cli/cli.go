// Package cli wires the headbox commands.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"ubunatic.com/headbox/internal/browser"
	"ubunatic.com/headbox/spec"
)

// ExitError carries the exit code of the command run by exec.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return "exit status " + strconv.Itoa(e.Code) }

// New returns the root command.
func New(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "headbox",
		Short:         "Run a pinned headless Chrome in a bwrap sandbox that leaves nothing behind",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(installCmd(), execCmd(), serveCmd(), shotCmd(), pathCmd(), versionCmd(version))
	return root
}

func load() (*spec.Browser, string, error) {
	b, err := spec.LoadBrowser()
	if err != nil {
		return nil, "", err
	}
	dir, err := browser.Dir(b)
	return b, dir, err
}

func installCmd() *cobra.Command {
	var zipFile string
	c := &cobra.Command{
		Use:   "install",
		Short: "Download and verify the pinned chrome-headless-shell",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, dir, err := load()
			if err != nil {
				return err
			}
			return browser.Install(b, dir, zipFile, cmd.ErrOrStderr())
		},
	}
	c.Flags().StringVar(&zipFile, "zip", "", "install from this zip instead of downloading (sha256 is still checked)")
	return c
}

type runFlags struct {
	port    int
	verbose bool
	args    []string
}

func (f *runFlags) register(c *cobra.Command) {
	c.Flags().IntVarP(&f.port, "port", "p", 0, "CDP port (0 picks a free one)")
	c.Flags().BoolVarP(&f.verbose, "verbose", "v", false, "pass Chrome's stderr through")
	c.Flags().StringArrayVar(&f.args, "chrome-arg", nil, "extra Chrome flag (repeatable)")
}

func (f *runFlags) start(stderr io.Writer, roBinds ...string) (*browser.Running, error) {
	b, dir, err := load()
	if err != nil {
		return nil, err
	}
	opt := browser.Options{Port: f.port, Args: f.args, ID: os.Getenv("HEADBOX_ID"), ROBinds: roBinds}
	if f.verbose {
		opt.Log = stderr
	}
	return browser.Start(b, dir, opt)
}

func execCmd() *cobra.Command {
	var f runFlags
	c := &cobra.Command{
		Use:   "exec [flags] -- CMD [ARGS...]",
		Short: "Start the browser, run CMD with CDP_URL set, stop the browser when CMD exits",
		Long: `Start the browser, run CMD with these variables set, and stop the browser when CMD
exits. headbox exits with CMD's exit code.

  CDP_URL           http://127.0.0.1:<port> (for /json/version, puppeteer browserURL)
  HEADBOX_WS_URL    the browser's DevTools WebSocket URL
  HEADBOX_CDP_PORT  the port`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := f.start(cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			defer r.Stop()
			return runChild(r, args)
		},
	}
	f.register(c)
	return c
}

func runChild(r *browser.Running, args []string) error {
	child := exec.Command(args[0], args[1:]...)
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	child.Env = append(os.Environ(),
		"CDP_URL="+r.HTTPURL(),
		"HEADBOX_WS_URL="+r.WSURL,
		"HEADBOX_CDP_PORT="+strconv.Itoa(r.Port),
	)
	// The command dies with headbox too: without the browser it has nothing to talk to.
	child.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)
	if err := child.Start(); err != nil {
		return err
	}
	go func() {
		for s := range sigs {
			_ = child.Process.Signal(s)
		}
	}()
	err := child.Wait()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code := ee.ExitCode()
		if code < 0 {
			code = 128 + int(ee.Sys().(syscall.WaitStatus).Signal())
		}
		return &ExitError{Code: code}
	}
	return err
}

func serveCmd() *cobra.Command {
	var f runFlags
	var idle time.Duration
	c := &cobra.Command{
		Use:   "serve [flags]",
		Short: "Run the browser until it has had no CDP connection for --idle",
		Long: `Run the browser for agents and tools to attach to, for example
  chrome-devtools-mcp --browserUrl $CDP_URL
  agent-browser --cdp $HEADBOX_CDP_PORT
The endpoint is printed as shell variables on stdout. The browser stops on Ctrl-C,
SIGTERM, or after --idle without any CDP connection (0 disables the idle stop).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("idle") {
				idle = spec.MustBrowser().Idle()
			}
			r, err := f.start(cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			defer r.Stop()
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "CDP_URL=%s\nHEADBOX_WS_URL=%s\nHEADBOX_CDP_PORT=%d\n", r.HTTPURL(), r.WSURL, r.Port)
			return serveLoop(r, idle, cmd.ErrOrStderr())
		},
	}
	f.register(c)
	c.Flags().DurationVar(&idle, "idle", 0, "stop after this long without a CDP connection (default from spec)")
	return c
}

func serveLoop(r *browser.Running, idle time.Duration, log io.Writer) error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)
	tick := 5 * time.Second
	if idle > 0 && idle/10 < tick {
		tick = max(idle/10, 100*time.Millisecond)
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	last := time.Now()
	for {
		select {
		case s := <-sigs:
			fmt.Fprintf(log, "headbox: %v, stopping the browser\n", s)
			return nil
		case <-r.Done():
			return errors.New("browser exited")
		case now := <-t.C:
			if idle <= 0 {
				continue
			}
			n, err := browser.Connections(r.Port)
			if err != nil {
				return err
			}
			if n > 0 {
				last = now
			} else if now.Sub(last) >= idle {
				fmt.Fprintf(log, "headbox: idle for %v, stopping the browser\n", idle)
				return nil
			}
		}
	}
}

func pathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the browser's install dir",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, dir, err := load()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), dir)
			return nil
		},
	}
}

func versionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the headbox and browser versions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, dir, err := load()
			if err != nil {
				return err
			}
			state := "not installed, run: headbox install"
			if browser.Installed(dir) {
				state = "installed"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "headbox %s\nchrome-headless-shell %s (%s)\n", version, b.Version, state)
			return nil
		},
	}
}
