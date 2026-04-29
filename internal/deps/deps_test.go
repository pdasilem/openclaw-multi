package deps

import (
	"context"
	"testing"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/shell"
)

// --- Node.js tests ---

func TestEnsureNodeAlreadyInstalled(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{shell.OKResponse("v22.16.0\n")},
	}
	status, err := EnsureNode(context.Background(), exec, "22.16.0")
	if err != nil {
		t.Fatalf("EnsureNode: %v", err)
	}
	if !status.Skipped {
		t.Error("expected Skipped=true when already at required version")
	}
	if exec.CallCount() != 1 {
		t.Errorf("expected 1 call (version check only), got %d", exec.CallCount())
	}
}

func TestEnsureNodeOlderVersion(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{shell.OKResponse("v18.0.0\n")},
	}
	status, err := EnsureNode(context.Background(), exec, "22.16.0")
	if err != nil {
		t.Fatalf("EnsureNode old version: %v", err)
	}
	if !status.Skipped {
		t.Error("expected Skipped=true; system Node is never upgraded by overlay")
	}
	if exec.CallCount() != 1 {
		t.Errorf("expected no install commands, got %d calls", exec.CallCount())
	}
}

func TestEnsureNodeNotInstalled(t *testing.T) {
	errRes, nodeErr := shell.ErrResponse("not found", 127)
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{errRes},
		Errors:    []error{nodeErr},
	}
	status, err := EnsureNode(context.Background(), exec, "22.16.0")
	if err != nil {
		t.Fatalf("EnsureNode not installed: %v", err)
	}
	if status.Installed {
		t.Error("expected Installed=false when system Node is absent")
	}
	if !status.Skipped {
		t.Error("expected Skipped=true; tenant Node is installed in user-space")
	}
	if exec.CallCount() != 1 {
		t.Errorf("expected no install commands, got %d calls", exec.CallCount())
	}
}

func TestSemverGTE(t *testing.T) {
	cases := []struct {
		ver, min string
		want     bool
	}{
		{"22.16.0", "22.16.0", true},
		{"22.17.0", "22.16.0", true},
		{"23.0.0", "22.16.0", true},
		{"18.0.0", "22.16.0", false},
		{"22.15.9", "22.16.0", false},
	}
	for _, c := range cases {
		if got := semverGTE(c.ver, c.min); got != c.want {
			t.Errorf("semverGTE(%q, %q) = %v, want %v", c.ver, c.min, got, c.want)
		}
	}
}

// --- Tailscale tests ---

func TestEnsureTailscaleAlreadyRunning(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			shell.OKResponse("1.0.0"), // tailscale version
			shell.OKResponse(`{"BackendState":"Running","TailscaleIPs":["100.1.2.3"]}`), // status
		},
	}
	status, err := EnsureTailscale(context.Background(), exec, false)
	if err != nil {
		t.Fatalf("EnsureTailscale running: %v", err)
	}
	if !status.LoggedIn || !status.Skipped {
		t.Errorf("expected LoggedIn=true Skipped=true, got %+v", status)
	}
	if status.IP != "100.1.2.3" {
		t.Errorf("IP: got %q", status.IP)
	}
}

func TestEnsureTailscaleNotLoggedIn_NonInteractive(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			shell.OKResponse("1.0.0"),
			shell.OKResponse(`{"BackendState":"NeedsLogin","TailscaleIPs":[]}`),
		},
	}
	status, err := EnsureTailscale(context.Background(), exec, false)
	if err != nil {
		t.Fatalf("EnsureTailscale non-interactive: %v", err)
	}
	if status.LoggedIn {
		t.Error("expected LoggedIn=false in non-interactive mode")
	}
	// tailscale up should NOT have been called.
	if exec.Called("tailscale up") {
		t.Error("tailscale up should not be called non-interactively")
	}
}

func TestEnsureTailscaleNotInstalled(t *testing.T) {
	versionErr, _ := shell.ErrResponse("not found", 127)
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			versionErr,           // tailscale version fails
			shell.OKResponse(""), // install script
			shell.OKResponse(`{"BackendState":"NeedsLogin","TailscaleIPs":[]}`),
		},
		Errors: []error{shell.ErrNonZeroExit{ExitCode: 127}, nil, nil},
	}
	status, err := EnsureTailscale(context.Background(), exec, false)
	if err != nil {
		t.Fatalf("EnsureTailscale not installed: %v", err)
	}
	if !status.Installed {
		t.Error("expected Installed=true after install")
	}
}

// --- Cloudflared tests ---

func mockRenderer(content string) func(string, map[string]string) (string, error) {
	return func(_ string, vars map[string]string) (string, error) {
		result := content
		for k, v := range vars {
			result += k + "=" + v + "\n"
		}
		return result, nil
	}
}

func TestEnsureCloudflaredVariantA(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			shell.OKResponse("cloudflared 2024.1.0"),                               // --version (installed)
			shell.OKResponse("Created tunnel openclaw-multi with id abc-123-uuid"), // tunnel create
			shell.OKResponse(""), // route dns
			shell.OKResponse(""), // ingress validate
			shell.OKResponse(""), // systemctl enable
		},
	}
	fs := shell.NewMemFS()
	cfg := config.Defaults()
	cfg.Domain = "example.com"
	cfg.Subdomain = "openclaw"

	err := EnsureCloudflared(context.Background(), exec, fs, mockRenderer("tunnel: ok\n"), cfg, TunnelModeAccount, ConflictOverwrite)
	if err != nil {
		t.Fatalf("EnsureCloudflared VariantA: %v", err)
	}
	if cfg.TunnelID != "abc-123-uuid" {
		t.Errorf("TunnelID: got %q", cfg.TunnelID)
	}
	if _, err := fs.Stat(cfConfigPath); err != nil {
		t.Error("expected cloudflared config file to be written")
	}
}

func TestEnsureCloudflaredVariantB(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{shell.OKResponse("cloudflared 2024.1.0")},
	}
	fs := shell.NewMemFS()
	cfg := config.Defaults()

	err := EnsureCloudflared(context.Background(), exec, fs, mockRenderer(""), cfg, TunnelModeQuick, ConflictOverwrite)
	if err != nil {
		t.Fatalf("EnsureCloudflared VariantB: %v", err)
	}
	if cfg.TunnelMode != TunnelModeQuick {
		t.Errorf("TunnelMode: got %q", cfg.TunnelMode)
	}
	data, err := fs.ReadFile(cfConfigPath)
	if err != nil || len(data) == 0 {
		t.Error("expected config file written for quick tunnel")
	}
}

func TestEnsureCloudflaredConflictSkip(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{shell.OKResponse("cloudflared 2024.1.0")},
	}
	fs := shell.NewMemFS()
	if err := fs.WriteFile(cfConfigPath, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()

	err := EnsureCloudflared(context.Background(), exec, fs, mockRenderer(""), cfg, TunnelModeAccount, ConflictSkip)
	if err != nil {
		t.Fatalf("ConflictSkip: %v", err)
	}
	data, _ := fs.ReadFile(cfConfigPath)
	if string(data) != "existing" {
		t.Error("ConflictSkip: existing config should be unchanged")
	}
}

func TestEnsureCloudflaredConflictBackup(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			shell.OKResponse("cloudflared 2024.1.0"),
			shell.OKResponse("Created tunnel openclaw-multi with id xyz-789"),
			shell.OKResponse(""),
			shell.OKResponse(""),
			shell.OKResponse(""),
		},
	}
	fs := shell.NewMemFS()
	if err := fs.WriteFile(cfConfigPath, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Domain = "ex.com"

	err := EnsureCloudflared(context.Background(), exec, fs, mockRenderer("backup-test\n"), cfg, TunnelModeAccount, ConflictBackup)
	if err != nil {
		t.Fatalf("ConflictBackup: %v", err)
	}
	// At least one backup file should exist.
	backupFound := false
	for k := range fs.Files {
		if len(k) > len(cfConfigPath) && k[:len(cfConfigPath)] == cfConfigPath {
			backupFound = true
		}
	}
	if !backupFound {
		t.Error("expected backup file to be created")
	}
}

// --- UFW tests ---

func TestEnsureUFWInactive(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			shell.OKResponse("Status: inactive\n"),
			shell.OKResponse(""), shell.OKResponse(""),
			shell.OKResponse(""), shell.OKResponse(""),
		},
	}
	status, err := EnsureUFW(context.Background(), exec, []int{8080})
	if err != nil {
		t.Fatalf("EnsureUFW inactive: %v", err)
	}
	if !status.Active {
		t.Error("expected Active=true after setup")
	}
}

func TestEnsureUFWAlreadyActive_DenyIncoming(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			shell.OKResponse("Status: active\nDefault: deny (incoming)\nDefault: allow (outgoing)\n"),
		},
	}
	status, err := EnsureUFW(context.Background(), exec, nil)
	if err != nil {
		t.Fatalf("EnsureUFW active: %v", err)
	}
	if !status.DenyIncoming {
		t.Error("expected DenyIncoming=true")
	}
	// No additional commands should have been called.
	if exec.CallCount() != 1 {
		t.Errorf("expected 1 call (status only), got %d", exec.CallCount())
	}
}

func TestEnsureUFWActiveAddsMissingPort(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			// Status: active, deny incoming, but port 9090 not in rules.
			shell.OKResponse("Status: active\nDefault: deny (incoming)\nDefault: allow (outgoing)\n"),
			shell.OKResponse(""), // ufw allow 9090/tcp
		},
	}
	status, err := EnsureUFW(context.Background(), exec, []int{9090})
	if err != nil {
		t.Fatalf("EnsureUFW add port: %v", err)
	}
	if len(status.AddedPorts) != 1 || status.AddedPorts[0] != 9090 {
		t.Errorf("AddedPorts: %v", status.AddedPorts)
	}
}

func TestParseUFWStatus(t *testing.T) {
	output := `Status: active

     To                         Action      From
     --                         ------      ----
Default: deny (incoming)
Default: allow (outgoing)
8080/tcp                   ALLOW IN    Anywhere
`
	s := parseUFWStatus(output)
	if !s.Active {
		t.Error("expected Active")
	}
	if !s.DenyIncoming {
		t.Error("expected DenyIncoming")
	}
	if len(s.AllowedPorts) != 1 || s.AllowedPorts[0] != 8080 {
		t.Errorf("AllowedPorts: %v", s.AllowedPorts)
	}
}

func TestInstallCloudflaredCalled(t *testing.T) {
	// cloudflared not installed → installCloudflared should be called
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			{ExitCode: 127},      // --version fails
			shell.OKResponse(""), // curl download
			shell.OKResponse(""), // dpkg -i
			shell.OKResponse("Created tunnel openclaw-multi with id new-id"),
			shell.OKResponse(""), // route dns
			shell.OKResponse(""), // ingress validate
			shell.OKResponse(""), // systemctl enable
		},
		Errors: []error{shell.ErrNonZeroExit{ExitCode: 127}, nil, nil, nil, nil, nil, nil},
	}
	fs := shell.NewMemFS()
	cfg := config.Defaults()
	cfg.Domain = "example.com"
	err := EnsureCloudflared(context.Background(), exec, fs, mockRenderer("c"), cfg, TunnelModeAccount, ConflictOverwrite)
	if err != nil {
		t.Fatalf("EnsureCloudflared install: %v", err)
	}
}

func TestParseTunnelIDNoMatch(t *testing.T) {
	id := parseTunnelID("no tunnel info here")
	if id != "" {
		t.Errorf("expected empty, got %q", id)
	}
}

func TestEnsureUFWActiveNoDenyIncoming(t *testing.T) {
	// Active but missing deny incoming — should add it
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			shell.OKResponse("Status: active\nDefault: allow (incoming)\nDefault: allow (outgoing)\n"),
			shell.OKResponse(""), // ufw default deny incoming
			shell.OKResponse(""), // ufw reload
		},
	}
	status, err := EnsureUFW(context.Background(), exec, nil)
	if err != nil {
		t.Fatalf("EnsureUFW no deny: %v", err)
	}
	if !status.DenyIncoming {
		t.Error("expected DenyIncoming=true after fix")
	}
}
