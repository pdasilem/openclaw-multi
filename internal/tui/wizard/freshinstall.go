package wizard

import (
	"context"
	"fmt"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/deps"
	"github.com/pdasilem/openclaw-multi/internal/hardening"
	"github.com/pdasilem/openclaw-multi/internal/preflight"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

// Deps groups injectable dependencies for fresh-install steps.
type Deps struct {
	Exec        shell.Executor
	FS          shell.FS
	Logger      shell.Logger
	Store       *state.Store
	Cfg         *config.OverlayConfig
	CfgPath     string
	TmplDir     string
	Interactive bool // true in real TUI, false in tests
}

// NewFreshInstallSteps returns the ordered slice of Steps for the fresh-install wizard.
func NewFreshInstallSteps(d Deps) []Step {
	renderer := func(tmpl string, vars map[string]string) (string, error) {
		return config.RenderFile(d.TmplDir+"/"+tmpl, vars)
	}
	h := &hardening.Hardener{Exec: d.Exec, FS: d.FS, Logger: d.Logger}
	return []Step{
		&StepPreFlight{exec: d.Exec},
		&StepTailscale{exec: d.Exec, interactive: d.Interactive},
		&StepCloudflared{exec: d.Exec, fs: d.FS, renderer: renderer, cfg: d.Cfg},
		&StepUFW{exec: d.Exec},
		&StepHardening{h: h},
		&StepOverlayAPI{exec: d.Exec, fs: d.FS, renderer: renderer},
		&StepAddUserInfo{},
		&StepSummary{store: d.Store, logger: d.Logger},
	}
}

// --- Step 1: Pre-flight ---

// StepPreFlight runs system prerequisite checks.
type StepPreFlight struct {
	exec    shell.Executor
	Results []preflight.Result // populated after Run
}

func (s *StepPreFlight) Name() string { return "Pre-flight check" }
func (s *StepPreFlight) Run(ctx context.Context) error {
	checker := preflight.NewChecker(s.exec)
	results, err := checker.CheckAll(ctx)
	if err != nil {
		return fmt.Errorf("preflight: %w", err)
	}
	s.Results = results
	for _, r := range results {
		if r.Level == preflight.LevelFail {
			return fmt.Errorf("preflight failed: %s — %s", r.Name, r.Detail)
		}
	}
	return nil
}

// --- Step 2: Tailscale ---

// StepTailscale installs and verifies Tailscale.
type StepTailscale struct {
	exec        shell.Executor
	interactive bool
	Status      deps.TailscaleStatus
}

func (s *StepTailscale) Name() string { return "Install Tailscale" }
func (s *StepTailscale) Run(ctx context.Context) error {
	status, err := deps.EnsureTailscale(ctx, s.exec, s.interactive)
	if err != nil {
		return fmt.Errorf("tailscale: %w", err)
	}
	s.Status = status
	return nil
}

// --- Step 3: Cloudflared ---

// StepCloudflared installs and configures Cloudflare Tunnel.
type StepCloudflared struct {
	exec           shell.Executor
	fs             shell.FS
	renderer       func(string, map[string]string) (string, error)
	cfg            *config.OverlayConfig
	Mode           string
	ConflictPolicy deps.ConflictPolicy
}

func (s *StepCloudflared) Name() string { return "Configure Cloudflare Tunnel" }
func (s *StepCloudflared) Run(ctx context.Context) error {
	mode := s.Mode
	if mode == "" {
		mode = deps.TunnelModeAccount
	}
	return deps.EnsureCloudflared(ctx, s.exec, s.fs, s.renderer, s.cfg, mode, s.ConflictPolicy)
}

// --- Step 5: UFW ---

// StepUFW configures the firewall.
type StepUFW struct {
	exec       shell.Executor
	ExtraPorts []int
	Status     deps.UFWStatus
}

func (s *StepUFW) Name() string { return "Configure UFW firewall" }
func (s *StepUFW) Run(ctx context.Context) error {
	status, err := deps.EnsureUFW(ctx, s.exec, s.ExtraPorts)
	if err != nil {
		return fmt.Errorf("ufw: %w", err)
	}
	s.Status = status
	return nil
}

// --- Step 6: Host hardening ---

// StepHardening applies sysctl, hidepid, profile.d.
type StepHardening struct {
	h *hardening.Hardener
}

func (s *StepHardening) Name() string { return "Apply host hardening" }
func (s *StepHardening) Run(ctx context.Context) error {
	if err := s.h.ApplySysctl(ctx); err != nil {
		return fmt.Errorf("sysctl: %w", err)
	}
	if err := s.h.ApplyHidepid(ctx); err != nil {
		return fmt.Errorf("hidepid: %w", err)
	}
	if err := s.h.ApplyProfile(ctx); err != nil {
		return fmt.Errorf("profile: %w", err)
	}
	return nil
}

// --- Step 7: Overlay-API stub service ---

// StepOverlayAPI installs the overlay-API systemd unit.
type StepOverlayAPI struct {
	exec     shell.Executor
	fs       shell.FS
	renderer func(string, map[string]string) (string, error)
}

func (s *StepOverlayAPI) Name() string { return "Install overlay-API service" }
func (s *StepOverlayAPI) Run(ctx context.Context) error {
	const unitPath = "/etc/systemd/system/openclaw-overlay-api.service"
	content, err := s.renderer("openclaw-overlay-api.service.tmpl", map[string]string{})
	if err != nil {
		return fmt.Errorf("render overlay-api unit: %w", err)
	}
	if err := s.fs.WriteFile(unitPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write unit %s: %w", unitPath, err)
	}
	if _, err := s.exec.Run(ctx, shell.ExecOpts{
		Cmd:  []string{"systemctl", "daemon-reload"},
		Sudo: true,
	}); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	if _, err := s.exec.Run(ctx, shell.ExecOpts{
		Cmd:  []string{"systemctl", "enable", "--now", "openclaw-overlay-api"},
		Sudo: true,
	}); err != nil {
		return fmt.Errorf("systemctl enable overlay-api: %w", err)
	}
	return nil
}

// --- Step 9: Add user info panel ---

// StepAddUserInfo is an informational step (no action).
type StepAddUserInfo struct{}

func (s *StepAddUserInfo) Name() string { return "Ready — add users via menu item 3" }
func (s *StepAddUserInfo) Run(_ context.Context) error {
	return nil // info-only, always succeeds
}

// --- Step 10: Summary ---

// StepSummary persists the install_completed marker in state.db.
type StepSummary struct {
	store  *state.Store
	logger shell.Logger
}

func (s *StepSummary) Name() string { return "Finalise installation" }
func (s *StepSummary) Run(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	if err := s.store.SetMeta(ctx, "install_completed", ts); err != nil {
		return fmt.Errorf("set install_completed: %w", err)
	}
	if s.logger != nil {
		_ = s.logger.Emit(audit.Event{
			Actor:   "system",
			Action:  audit.ActionStartup,
			Target:  "install",
			Result:  audit.ResultOk,
			Details: map[string]any{"install_completed": ts},
		})
	}
	return nil
}
