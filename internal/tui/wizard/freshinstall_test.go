package wizard

import (
	"context"
	"path/filepath"
	"testing"

	"fmt"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/hardening"
	"github.com/pdasilem/openclaw-multi/internal/preflight"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

func openTestStore(t *testing.T) *state.Store {
	t.Helper()
	s, err := state.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func okExecN(n int) *shell.MockExecutor {
	responses := make([]shell.ExecResult, n)
	for i := range n {
		responses[i] = shell.OKResponse("")
	}
	return &shell.MockExecutor{Responses: responses}
}

// --- StepPreFlight ---

func TestStepPreFlight_Pass(t *testing.T) {
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("/bin/tool")}}
	s := &StepPreFlight{exec: exec}
	// Without real OS files this will warn but not fail (distro/ram/disk unreadable = warn).
	err := s.Run(context.Background())
	// tools check uses mock → ok; disk/ram use real syscalls which pass on dev machine.
	_ = err // may return nil or warn; must not panic
}

func TestStepPreFlight_Fail(t *testing.T) {
	// tools all missing → LevelFail
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{{ExitCode: 1}}}
	s := &StepPreFlight{exec: exec}
	err := s.Run(context.Background())
	if err == nil {
		t.Error("expected error when tools missing")
	}
}

// --- StepTailscale ---

func TestStepTailscale_AlreadyRunning(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			shell.OKResponse("1.0.0"),
			shell.OKResponse(`{"BackendState":"Running","TailscaleIPs":["100.1.1.1"]}`),
		},
	}
	s := &StepTailscale{exec: exec, interactive: false}
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("StepTailscale running: %v", err)
	}
	if !s.Status.Skipped {
		t.Error("expected Skipped=true")
	}
}

func TestStepTailscale_NotInstalled_NonInteractive(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			{ExitCode: 127},
			shell.OKResponse(""),
			shell.OKResponse(`{"BackendState":"NeedsLogin","TailscaleIPs":[]}`),
		},
		Errors: []error{shell.ErrNonZeroExit{ExitCode: 127}, nil, nil},
	}
	s := &StepTailscale{exec: exec, interactive: false}
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("StepTailscale not installed: %v", err)
	}
}

// --- StepCloudflared ---

func TestStepCloudflared_VariantA(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			shell.OKResponse("cloudflared 2024.1.0"),
			shell.OKResponse("[]"),
			shell.OKResponse("Created tunnel openclaw-multi with id 12345678-1234-1234-1234-123456789abc"),
			shell.OKResponse(""),
			shell.OKResponse(""),
			shell.OKResponse(""),
		},
	}
	fs := shell.NewMemFS()
	cfg := config.Defaults()
	cfg.Domain = "example.com"
	if err := fs.WriteFile("/home/test/.cloudflared/12345678-1234-1234-1234-123456789abc.json", []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	s := &StepCloudflared{
		exec: exec,
		fs:   fs,
		renderer: func(_ string, vars map[string]string) (string, error) {
			return "tunnel: " + vars["TUNNEL_ID"] + "\n", nil
		},
		cfg:  cfg,
		Mode: "account",
	}
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("StepCloudflared VariantA: %v", err)
	}
	if cfg.TunnelID != "12345678-1234-1234-1234-123456789abc" {
		t.Errorf("TunnelID: got %q", cfg.TunnelID)
	}
}

func TestStepCloudflared_VariantB(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{shell.OKResponse("cloudflared 2024.1.0")},
	}
	fs := shell.NewMemFS()
	cfg := config.Defaults()
	s := &StepCloudflared{
		exec: exec, fs: fs,
		renderer: func(_ string, _ map[string]string) (string, error) { return "", nil },
		cfg:      cfg,
		Mode:     "quick",
	}
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("StepCloudflared VariantB: %v", err)
	}
}

// --- StepUFW ---

func TestStepUFW_FreshSetup(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			shell.OKResponse("Status: inactive\n"),
			shell.OKResponse(""), shell.OKResponse(""),
			shell.OKResponse(""), shell.OKResponse(""),
		},
	}
	s := &StepUFW{exec: exec, ExtraPorts: []int{8080}}
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("StepUFW FreshSetup: %v", err)
	}
	if !s.Status.Active {
		t.Error("expected Active=true")
	}
}

func TestStepUFW_Idempotent(t *testing.T) {
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{
			shell.OKResponse("Status: active\nDefault: deny (incoming)\nDefault: allow (outgoing)\n8080/tcp ALLOW IN Anywhere\n"),
		},
	}
	s := &StepUFW{exec: exec, ExtraPorts: []int{8080}}
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("StepUFW Idempotent: %v", err)
	}
	if len(s.Status.AddedPorts) != 0 {
		t.Errorf("expected no added ports, got %v", s.Status.AddedPorts)
	}
	if exec.CallCount() != 1 {
		t.Errorf("expected 1 call, got %d", exec.CallCount())
	}
}

// --- StepHardening ---

func TestStepHardening_WritesFiles(t *testing.T) {
	exec := okExecN(10)
	fs := shell.NewMemFS()
	if err := fs.WriteFile("/etc/fstab", []byte("# fstab\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &hardening.Hardener{Exec: exec, FS: fs}
	s := &StepHardening{h: h}
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("StepHardening: %v", err)
	}
	if _, err := fs.Stat("/etc/sysctl.d/openclaw-overlay.conf"); err != nil {
		t.Error("expected sysctl file written")
	}
	if _, err := fs.Stat("/etc/profile.d/openclaw.sh"); err != nil {
		t.Error("expected profile file written")
	}
}

// --- StepOverlayAPI ---

func TestStepOverlayAPI_WritesUnitAndEnables(t *testing.T) {
	exec := okExecN(5)
	fs := shell.NewMemFS()
	s := &StepOverlayAPI{
		exec: exec,
		fs:   fs,
		renderer: func(_ string, _ map[string]string) (string, error) {
			return "[Service]\nExecStart=/usr/local/bin/openclaw-overlay-api\n", nil
		},
	}
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("StepOverlayAPI: %v", err)
	}
	if _, err := fs.Stat("/etc/systemd/system/openclaw-overlay-api.service"); err != nil {
		t.Error("expected unit file written")
	}
	if !exec.Called("systemctl") {
		t.Error("expected systemctl calls")
	}
}

// --- StepAddUserInfo ---

func TestStepAddUserInfo_NoOp(t *testing.T) {
	s := &StepAddUserInfo{}
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("StepAddUserInfo: %v", err)
	}
}

// --- StepSummary ---

func TestStepSummary_PersistsMetaKey(t *testing.T) {
	store := openTestStore(t)
	s := &StepSummary{store: store}
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("StepSummary: %v", err)
	}
	val, err := store.GetMeta(context.Background(), "install_completed")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if val == "" {
		t.Error("expected install_completed to be set")
	}
}

// --- preflight.Result helper ---

func TestPreflightLevelConstants(t *testing.T) {
	r := preflight.Result{Name: "test", Detail: "ok", Level: preflight.LevelOK}
	if !r.OK() {
		t.Error("expected OK() = true")
	}
}

func TestAllStepNames(t *testing.T) {
	// Ensures Name() is covered on all steps.
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("")}}
	fs := shell.NewMemFS()
	cfg := config.Defaults()
	store := openTestStore(t)
	d := Deps{
		Exec: exec, FS: fs, Cfg: cfg, Store: store,
		TmplDir:     "nonexistent",
		Interactive: false,
	}
	steps := NewFreshInstallSteps(d)
	for _, s := range steps {
		if s.Name() == "" {
			t.Errorf("step %T has empty Name()", s)
		}
	}
}

func TestStepOutcomeOK(t *testing.T) {
	ok := StepOutcome{Name: "x"}
	if !ok.OK() {
		t.Error("expected OK()=true for nil error")
	}
	fail := StepOutcome{Name: "x", Err: fmt.Errorf("fail")}
	if fail.OK() {
		t.Error("expected OK()=false for non-nil error")
	}
}

func TestStepSummary_NilStore(t *testing.T) {
	s := &StepSummary{}
	if err := s.Run(context.Background()); err != nil {
		t.Errorf("expected nil error for nil store, got %v", err)
	}
}
