package browser

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/headbox/spec"
)

// fakeZip builds a zip with the given entries (name -> content, mode 0755).
func fakeZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range entries {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0o755)
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(f, content)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func serveZip(t *testing.T, data []byte) *spec.Browser {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return &spec.Browser{Version: "1.2.3.4", URLTemplate: srv.URL + "/{version}/b.zip", SHA256: sum(data)}
}

func TestInstallDownloads(t *testing.T) {
	data := fakeZip(t, map[string]string{zipRoot + "/" + BinName: "#!/bin/true\n"})
	b := serveZip(t, data)
	dir := filepath.Join(t.TempDir(), "x", BinName+"-"+b.Version)
	var log bytes.Buffer
	if err := Install(b, dir, "", &log); err != nil {
		t.Fatal(err)
	}
	if !Installed(dir) {
		t.Fatalf("not installed in %s", dir)
	}
	if err := Install(b, dir, "", &log); err != nil || !strings.Contains(log.String(), "already installed") {
		t.Errorf("second install: err=%v log=%q", err, log.String())
	}
	assertNoTemp(t, filepath.Dir(dir))
}

func TestInstallRejectsWrongSHA(t *testing.T) {
	data := fakeZip(t, map[string]string{zipRoot + "/" + BinName: "x"})
	b := serveZip(t, data)
	b.SHA256 = strings.Repeat("0", 64)
	dir := filepath.Join(t.TempDir(), "b")
	err := Install(b, dir, "", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("want sha256 mismatch, got %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("dir must not exist after a failed install: %v", err)
	}
	assertNoTemp(t, filepath.Dir(dir))
}

func TestInstallFromZipFile(t *testing.T) {
	data := fakeZip(t, map[string]string{zipRoot + "/" + BinName: "x"})
	zipFile := filepath.Join(t.TempDir(), "b.zip")
	os.WriteFile(zipFile, data, 0o644)
	// An unreachable URL proves --zip does not download.
	b := &spec.Browser{Version: "1", URLTemplate: "http://127.0.0.1:1/{version}.zip", SHA256: sum(data)}
	dir := filepath.Join(t.TempDir(), "b")
	if err := Install(b, dir, zipFile, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !Installed(dir) {
		t.Fatal("not installed")
	}
}

func TestInstallRejectsBadZips(t *testing.T) {
	cases := map[string]map[string]string{
		"no binary":    {zipRoot + "/other": "x"},
		"wrong root":   {"elsewhere/" + BinName: "x"},
		"path escapes": {"../" + BinName: "x"},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			b := serveZip(t, fakeZip(t, entries))
			dir := filepath.Join(t.TempDir(), "b")
			if err := Install(b, dir, "", io.Discard); err == nil {
				t.Fatal("want an error")
			}
			if Installed(dir) {
				t.Error("must not be installed")
			}
		})
	}
}

func assertNoTemp(t *testing.T, parent string) {
	t.Helper()
	entries, _ := os.ReadDir(parent)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".install-") {
			t.Errorf("temp dir left: %s", e.Name())
		}
	}
}
