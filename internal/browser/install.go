// Package browser installs the pinned chrome-headless-shell and runs it in a bwrap
// sandbox that leaves no process or file behind.
package browser

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ubunatic.com/headbox/spec"
)

// zipRoot is the top-level dir inside the Chrome for Testing zip.
const zipRoot = "chrome-headless-shell-linux64"

// BinName is the browser executable inside the install dir.
const BinName = "chrome-headless-shell"

// Home is where browsers are installed: $HEADBOX_HOME, else $XDG_DATA_HOME/headbox.
func Home() (string, error) {
	if h := os.Getenv("HEADBOX_HOME"); h != "" {
		return h, nil
	}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "headbox"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "headbox"), nil
}

// Dir is the install dir of the given spec's version.
func Dir(b *spec.Browser) (string, error) {
	h, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, BinName+"-"+b.Version), nil
}

// Installed reports whether the browser binary exists in dir.
func Installed(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, BinName))
	return err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0
}

// Install puts the spec's browser into dir: from zipFile when set, else downloaded.
// The zip must match the spec's sha256. It unpacks next to dir and renames into place,
// so dir is either complete or absent. An existing install is left alone.
func Install(b *spec.Browser, dir, zipFile string, log io.Writer) error {
	if Installed(dir) {
		fmt.Fprintf(log, "already installed: %s\n", dir)
		return nil
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	if zipFile == "" {
		zipFile = filepath.Join(tmp, "browser.zip")
		fmt.Fprintf(log, "downloading %s\n", b.URL())
		if err := download(b.URL(), zipFile); err != nil {
			return err
		}
	}
	if err := checkSHA256(zipFile, b.SHA256); err != nil {
		return err
	}
	if err := unzip(zipFile, filepath.Join(tmp, "x")); err != nil {
		return err
	}
	src := filepath.Join(tmp, "x", zipRoot)
	if !Installed(src) {
		return fmt.Errorf("zip has no %s/%s", zipRoot, BinName)
	}
	if err := os.Rename(src, dir); err != nil {
		return err
	}
	fmt.Fprintf(log, "installed: %s\n", dir)
	return nil
}

func download(url, dst string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func checkSHA256(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("sha256 mismatch for %s: got %s, want %s", path, got, want)
	}
	return nil
}

func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		path := filepath.Join(dst, f.Name)
		if !strings.HasPrefix(path, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("zip entry escapes target: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return errors.New("zip entry is a symlink: " + f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := extract(f, path); err != nil {
			return err
		}
	}
	return nil
}

func extract(f *zip.File, path string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
