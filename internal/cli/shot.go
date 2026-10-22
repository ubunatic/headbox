package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"ubunatic.com/headbox/internal/cdp"
	"ubunatic.com/headbox/spec"
)

type shotFlags struct {
	run     runFlags
	output  string
	width   int
	height  int
	dark    bool
	timeout time.Duration
}

func (f *shotFlags) register(c *cobra.Command) {
	b := spec.MustBrowser()
	f.run.register(c)
	c.Flags().StringVarP(&f.output, "output", "o", "", "PNG file to write on the host (required)")
	c.Flags().IntVar(&f.width, "width", b.ShotWidth, "viewport width in pixels")
	c.Flags().IntVar(&f.height, "height", b.ShotHeight, "viewport height in pixels")
	c.Flags().BoolVar(&f.dark, "dark", false, "emulate prefers-color-scheme: dark (default light)")
	c.Flags().DurationVar(&f.timeout, "timeout", b.ShotLimit(), "time limit for start, load and capture")
}

func (f *shotFlags) validate() error {
	if f.output == "" {
		return errors.New("missing -o FILE.png")
	}
	if f.width <= 0 || f.height <= 0 {
		return fmt.Errorf("bad viewport %dx%d, want positive --width and --height", f.width, f.height)
	}
	if f.timeout <= 0 {
		return fmt.Errorf("bad --timeout %v", f.timeout)
	}
	return nil
}

// resolveTarget turns the shot argument into the URL to load and, for local files,
// the host path the sandbox needs read-only. Local paths and file:// URLs bind the
// file's directory, so relative assets (CSS, images) load too.
func resolveTarget(arg string) (target, bind string, err error) {
	var u *url.URL
	if strings.Contains(arg, "://") || strings.HasPrefix(arg, "about:") || strings.HasPrefix(arg, "data:") {
		if u, err = url.Parse(arg); err != nil {
			return "", "", err
		}
		if u.Scheme != "file" {
			return arg, "", nil
		}
		if u.Host != "" && u.Host != "localhost" {
			return "", "", fmt.Errorf("file URL with host %q not supported", u.Host)
		}
		arg = u.Path
	} else {
		u = &url.URL{}
	}
	path, err := filepath.Abs(arg)
	if err != nil {
		return "", "", err
	}
	if path, err = filepath.EvalSymlinks(path); err != nil {
		return "", "", fmt.Errorf("%w (for a web page use http:// or https://)", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", "", err
	}
	if !fi.IsDir() {
		fh, err := os.Open(path)
		if err != nil {
			return "", "", err
		}
		fh.Close()
	}
	bind = path
	if !fi.IsDir() {
		bind = filepath.Dir(path)
	}
	if bind == "/" {
		bind = path // never bind the host root over the sandbox
	}
	out := url.URL{Scheme: "file", Path: path, RawQuery: u.RawQuery, Fragment: u.Fragment}
	return out.String(), bind, nil
}

func shotCmd() *cobra.Command { return newShotCmd(&shotFlags{}) }

func newShotCmd(f *shotFlags) *cobra.Command {
	c := &cobra.Command{
		Use:   "shot URL -o FILE.png [--width N] [--height N] [--dark]",
		Short: "Take a PNG screenshot of URL or a local file in the sandboxed browser",
		Long: `Start the browser, load URL in a viewport of --width x --height pixels, wait for the
load event, write a PNG to -o on the host, and stop the browser.

URL may be http(s)://, file:// or a local path; for local files the file's directory is
bound read-only into the sandbox, so relative assets load. --dark emulates the media
feature prefers-color-scheme: dark, exactly like a user preference; the default is light.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := f.validate(); err != nil {
				return err
			}
			target, bind, err := resolveTarget(args[0])
			if err != nil {
				return err
			}
			var binds []string
			if bind != "" {
				binds = append(binds, bind)
			}
			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			ctx, cancelT := context.WithTimeout(ctx, f.timeout)
			defer cancelT()
			r, err := f.run.start(cmd.ErrOrStderr(), binds...)
			if err != nil {
				return err
			}
			defer r.Stop()
			png, err := cdp.Screenshot(ctx, r.WSURL, cdp.ShotOptions{
				URL: target, Width: f.width, Height: f.height, Dark: f.dark,
			})
			if err != nil {
				return err
			}
			return os.WriteFile(f.output, png, 0o644)
		},
	}
	f.register(c)
	return c
}
