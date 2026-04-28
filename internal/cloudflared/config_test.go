package cloudflared

import (
	"strings"
	"testing"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

func TestRenderConfig(t *testing.T) {
	cfg := &config.OverlayConfig{TunnelID: "tun-1", CloudflaredCredentialsFile: "/tmp/tun.json"}
	out, err := RenderConfig(cfg, []state.Route{
		{ID: "b", Hostname: "b.example.com", LocalPort: 18002, Enabled: true},
		{ID: "a", Hostname: "a.example.com", LocalPort: 18001, Enabled: true},
		{ID: "off", Hostname: "off.example.com", LocalPort: 18003, Enabled: false},
	})
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		"tunnel: tun-1",
		"credentials-file: /tmp/tun.json",
		"hostname: a.example.com",
		"service: http://127.0.0.1:18001",
		"service: http_status:404",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "off.example.com") {
		t.Fatalf("disabled route rendered:\n%s", got)
	}
	if strings.Index(got, "a.example.com") > strings.Index(got, "b.example.com") {
		t.Fatalf("routes not sorted:\n%s", got)
	}
}

func TestCredentialsFileDefault(t *testing.T) {
	got, err := CredentialsFile(&config.OverlayConfig{TunnelID: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/etc/cloudflared/abc.json" {
		t.Fatalf("got %q", got)
	}
}
