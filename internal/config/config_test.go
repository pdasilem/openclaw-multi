package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsReturnsSensibleValues(t *testing.T) {
	d := Defaults()
	if d.Subdomain != "openclaw" {
		t.Errorf("Subdomain: got %q", d.Subdomain)
	}
	if d.PortRangeStart != 18789 {
		t.Errorf("PortRangeStart: got %d", d.PortRangeStart)
	}
	if d.TunnelMode != "account" {
		t.Errorf("TunnelMode: got %q", d.TunnelMode)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	cfg, err := Load("/nonexistent/config.yml")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if cfg == nil {
		t.Error("expected defaults returned alongside ErrNotFound")
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := "domain: example.com\nsubdomain: myoverlay\ntunnel_mode: quick\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Domain != "example.com" {
		t.Errorf("Domain: got %q", cfg.Domain)
	}
	if cfg.Subdomain != "myoverlay" {
		t.Errorf("Subdomain: got %q", cfg.Subdomain)
	}
	if cfg.TunnelMode != "quick" {
		t.Errorf("TunnelMode: got %q", cfg.TunnelMode)
	}
	// Unset fields should still have defaults.
	if cfg.PortRangeStart != 18789 {
		t.Errorf("PortRangeStart default not preserved: got %d", cfg.PortRangeStart)
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	cfg := Defaults()
	cfg.Domain = "test.io"
	cfg.TunnelID = "abc-123"

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if got.Domain != "test.io" || got.TunnelID != "abc-123" {
		t.Errorf("roundtrip: got domain=%q tunnelID=%q", got.Domain, got.TunnelID)
	}
}

func TestSaveToNonExistentDir(t *testing.T) {
	err := Save("/nonexistent/dir/config.yml", Defaults())
	if err == nil {
		t.Fatal("expected error saving to non-existent dir")
	}
}

// --- Template tests ---

func TestRenderSubstitutes(t *testing.T) {
	tmpl := "tunnel: ${TUNNEL_ID}\ndomain: ${DOMAIN}"
	result, err := Render(tmpl, map[string]string{
		"TUNNEL_ID": "abc-123",
		"DOMAIN":    "example.com",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "tunnel: abc-123\ndomain: example.com"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestRenderUnknownVarReturnsError(t *testing.T) {
	_, err := Render("hello ${UNKNOWN}", map[string]string{})
	if err == nil {
		t.Fatal("expected ErrUnknownVar")
	}
	var e ErrUnknownVar
	if !errors.As(err, &e) {
		t.Errorf("expected ErrUnknownVar, got %T: %v", err, err)
	}
	if e.Var != "UNKNOWN" {
		t.Errorf("Var: got %q", e.Var)
	}
}

func TestRenderNoVars(t *testing.T) {
	result, err := Render("no vars here", map[string]string{})
	if err != nil || result != "no vars here" {
		t.Errorf("got (%q, %v)", result, err)
	}
}

func TestRenderFileAndWrite(t *testing.T) {
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "test.tmpl")
	outPath := filepath.Join(dir, "out.conf")

	if err := os.WriteFile(tmplPath, []byte("id=${ID}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(tmplPath, outPath, map[string]string{"ID": "42"}, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil || string(got) != "id=42" {
		t.Errorf("output: got (%q, %v)", got, err)
	}
}

func TestErrUnknownVarError(t *testing.T) {
	e := ErrUnknownVar{Var: "FOO"}
	if e.Error() == "" {
		t.Error("expected non-empty error string")
	}
}

func TestRenderFileNotFound(t *testing.T) {
	_, err := RenderFile("/nonexistent/template.tmpl", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing template file")
	}
}

func TestWriteFileTemplateError(t *testing.T) {
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "bad.tmpl")
	if err := os.WriteFile(tmplPath, []byte("${MISSING}"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := WriteFile(tmplPath, filepath.Join(dir, "out"), map[string]string{}, 0o644)
	if err == nil {
		t.Fatal("expected ErrUnknownVar")
	}
}
