package spec

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// schema is the subset of JSON Schema that browser.schema.json uses.
type schema struct {
	Required   []string `json:"required"`
	Properties map[string]struct {
		Type    string `json:"type"`
		Pattern string `json:"pattern"`
		Minimum *int64 `json:"minimum"`
	} `json:"properties"`
}

func TestBrowserMatchesSchema(t *testing.T) {
	raw, err := fs.ReadFile(FS, "schemas/browser.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var s schema
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	data, err := fs.ReadFile(FS, "browser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	for _, k := range s.Required {
		if _, ok := doc[k]; !ok {
			t.Errorf("browser.yaml: required key %q missing", k)
		}
	}
	for k, v := range doc {
		p, ok := s.Properties[k]
		if !ok {
			t.Errorf("browser.yaml: key %q not in schema", k)
			continue
		}
		switch p.Type {
		case "string":
			str, ok := v.(string)
			if !ok {
				t.Errorf("%s: want string, got %T", k, v)
				continue
			}
			if p.Pattern != "" && !regexp.MustCompile(p.Pattern).MatchString(str) {
				t.Errorf("%s: %q does not match %s", k, str, p.Pattern)
			}
		case "integer":
			n, ok := v.(int)
			if !ok {
				t.Errorf("%s: want integer, got %T", k, v)
				continue
			}
			if p.Minimum != nil && int64(n) < *p.Minimum {
				t.Errorf("%s: %d below minimum %d", k, n, *p.Minimum)
			}
		}
	}
}

func TestLoadBrowser(t *testing.T) {
	b, err := LoadBrowser()
	if err != nil {
		t.Fatal(err)
	}
	if u := b.URL(); !strings.Contains(u, "/"+b.Version+"/") || strings.Contains(u, "{version}") {
		t.Errorf("URL() = %q, want the version %s filled in", u, b.Version)
	}
	if b.Idle() <= 0 {
		t.Errorf("Idle() = %v", b.Idle())
	}
}
