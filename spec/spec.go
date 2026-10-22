// Package spec embeds the YAML spec files and their JSON schemas into the binary and
// loads them into typed structs. The spec is application code: Go code must not
// duplicate or shadow values defined here.
package spec

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed *.yaml schemas
var root embed.FS

// FS is the embedded spec filesystem, paths relative to spec/.
var FS fs.FS = root

// Browser mirrors spec/browser.yaml.
type Browser struct {
	Version     string `yaml:"version"`
	URLTemplate string `yaml:"url_template"`
	SHA256      string `yaml:"sha256"`
	TmpBytes    int64  `yaml:"tmp_bytes"`
	ShmBytes    int64  `yaml:"shm_bytes"`
	HomeBytes   int64  `yaml:"home_bytes"`
	ServeIdle   string `yaml:"serve_idle"`
}

// URL is the download URL of the pinned version.
func (b *Browser) URL() string {
	return strings.ReplaceAll(b.URLTemplate, "{version}", b.Version)
}

// Idle is ServeIdle parsed.
func (b *Browser) Idle() time.Duration {
	d, err := time.ParseDuration(b.ServeIdle)
	if err != nil {
		panic(fmt.Sprintf("spec: serve_idle %q: %v", b.ServeIdle, err))
	}
	return d
}

// LoadBrowser reads the embedded browser.yaml; unknown keys are an error.
func LoadBrowser() (*Browser, error) {
	data, err := fs.ReadFile(FS, "browser.yaml")
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var b Browser
	if err := dec.Decode(&b); err != nil {
		return nil, fmt.Errorf("spec: browser.yaml: %w", err)
	}
	return &b, nil
}

// MustBrowser is LoadBrowser for callers that cannot continue without the spec.
func MustBrowser() *Browser {
	b, err := LoadBrowser()
	if err != nil {
		panic(err)
	}
	return b
}
