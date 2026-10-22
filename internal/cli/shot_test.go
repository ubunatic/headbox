package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ubunatic.com/headbox/spec"
)

func TestShotFlags(t *testing.T) {
	b := spec.MustBrowser()
	tests := []struct {
		name    string
		args    []string
		wantErr string
		check   func(*testing.T, *shotFlags)
	}{
		{name: "defaults", args: []string{"-o", "a.png"}, check: func(t *testing.T, f *shotFlags) {
			if f.width != b.ShotWidth || f.height != b.ShotHeight || f.dark || f.output != "a.png" {
				t.Errorf("got %+v", f)
			}
			if f.width != 1280 || f.height != 800 {
				t.Errorf("want the documented 1280x800 default, got %dx%d", f.width, f.height)
			}
			if f.timeout != b.ShotLimit() {
				t.Errorf("timeout %v", f.timeout)
			}
		}},
		{name: "all", args: []string{"--output", "b.png", "--width", "320", "--height", "200", "--dark", "--timeout", "5s"},
			check: func(t *testing.T, f *shotFlags) {
				if f.width != 320 || f.height != 200 || !f.dark || f.output != "b.png" || f.timeout != 5*time.Second {
					t.Errorf("got %+v", f)
				}
			}},
		{name: "no output", args: nil, wantErr: "missing -o"},
		{name: "zero width", args: []string{"-o", "a.png", "--width", "0"}, wantErr: "bad viewport"},
		{name: "negative height", args: []string{"-o", "a.png", "--height", "-1"}, wantErr: "bad viewport"},
		{name: "bad timeout", args: []string{"-o", "a.png", "--timeout", "0s"}, wantErr: "bad --timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &shotFlags{}
			c := newShotCmd(f)
			if err := c.ParseFlags(tt.args); err != nil {
				t.Fatal(err)
			}
			err := f.validate()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, f)
		})
	}
}

func TestShotArgs(t *testing.T) {
	c := shotCmd()
	if err := c.Args(c, nil); err == nil {
		t.Error("want an error without URL")
	}
	if err := c.Args(c, []string{"a", "b"}); err == nil {
		t.Error("want an error with two URLs")
	}
}

func TestResolveTarget(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(dir, "a b.html")
	if err := os.WriteFile(page, []byte("<p>hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link.html")
	if err := os.Symlink(page, link); err != nil {
		t.Fatal(err)
	}
	fileURL := "file://" + strings.ReplaceAll(page, " ", "%20")
	tests := []struct {
		arg, target, bind, wantErr string
	}{
		{arg: "https://example.com/x?y=1", target: "https://example.com/x?y=1"},
		{arg: "about:blank", target: "about:blank"},
		{arg: page, target: fileURL, bind: dir},
		{arg: link, target: fileURL, bind: dir},
		{arg: fileURL + "#top", target: fileURL + "#top", bind: dir},
		{arg: "file://localhost" + page, target: fileURL, bind: dir},
		{arg: dir, target: "file://" + dir, bind: dir},
		{arg: filepath.Join(dir, "missing.html"), wantErr: "no such file"},
		{arg: "example.com", wantErr: "http://"},
		{arg: "file://otherhost/x.html", wantErr: "host"},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			target, bind, err := resolveTarget(tt.arg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if target != tt.target || bind != tt.bind {
				t.Errorf("got (%q, %q), want (%q, %q)", target, bind, tt.target, tt.bind)
			}
		})
	}
}
