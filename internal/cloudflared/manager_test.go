package cloudflared

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

func TestManagerPublishWritesValidatesAndSIGHUPs(t *testing.T) {
	memFS := shell.NewMemFS()
	memFS.Files["/etc/cloudflared/tun.json"] = []byte("{}")
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("valid"), shell.OKResponse("")}}
	m := &Manager{
		Config:     &config.OverlayConfig{TunnelID: "tun"},
		ConfigPath: "/etc/cloudflared/config.yml",
		FS:         memFS,
		Executor:   exec,
		Now:        func() time.Time { return time.Date(2026, 4, 28, 1, 2, 3, 0, time.UTC) },
	}
	if err := m.Publish(context.Background(), []state.Route{{ID: "r", Hostname: "a.example.com", LocalPort: 18001, Enabled: true}}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	data := string(memFS.Files["/etc/cloudflared/config.yml"])
	if !strings.Contains(data, "a.example.com") {
		t.Fatalf("route not written:\n%s", data)
	}
	if len(exec.Calls) != 2 {
		t.Fatalf("got %d executor calls", len(exec.Calls))
	}
}

func TestManagerPublishRollsBackOnValidateFailure(t *testing.T) {
	memFS := shell.NewMemFS()
	memFS.Files["/etc/cloudflared/tun.json"] = []byte("{}")
	memFS.Files["/etc/cloudflared/config.yml"] = []byte("old")
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{{Stderr: "bad", ExitCode: 1}}, Errors: []error{shell.ErrNonZeroExit{ExitCode: 1, Stderr: "bad"}}}
	m := &Manager{Config: &config.OverlayConfig{TunnelID: "tun"}, ConfigPath: "/etc/cloudflared/config.yml", FS: memFS, Executor: exec}
	err := m.Publish(context.Background(), []state.Route{{ID: "r", Hostname: "a.example.com", LocalPort: 18001, Enabled: true}})
	if err == nil {
		t.Fatal("expected error")
	}
	if string(memFS.Files["/etc/cloudflared/config.yml"]) != "old" {
		t.Fatalf("config not rolled back: %q", memFS.Files["/etc/cloudflared/config.yml"])
	}
}

func TestManagerRequiresCredentialsFile(t *testing.T) {
	m := &Manager{Config: &config.OverlayConfig{TunnelID: "tun"}, ConfigPath: "/etc/cloudflared/config.yml", FS: shell.NewMemFS(), Executor: &shell.MockExecutor{}}
	if err := m.Publish(context.Background(), nil); err == nil {
		t.Fatal("expected missing credentials error")
	}
}
