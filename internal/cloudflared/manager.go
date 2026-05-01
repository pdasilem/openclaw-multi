package cloudflared

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

type Auditor interface {
	Emit(audit.Event) error
}

type Manager struct {
	Config              *config.OverlayConfig
	ConfigPath          string
	FS                  shell.FS
	Executor            shell.Executor
	Auditor             Auditor
	Actor               string
	ValidateCmd         []string
	SIGHUPCmd           []string
	SkipCredentialCheck bool
	Now                 func() time.Time
}

type PublishSummary struct {
	ConfigPath string
	BackupPath string
	RolledBack bool
}

func (m *Manager) Publish(ctx context.Context, routes []state.Route) error {
	_, err := m.PublishWithSummary(ctx, routes)
	return err
}

func (m *Manager) PublishWithSummary(ctx context.Context, routes []state.Route) (PublishSummary, error) {
	start := time.Now()
	var summary PublishSummary
	if m.ConfigPath == "" {
		m.ConfigPath = DefaultConfigPath
	}
	summary.ConfigPath = m.ConfigPath
	if m.FS == nil {
		m.FS = shell.RealFS{}
	}
	if m.Executor == nil {
		return summary, errors.New("cloudflared manager requires executor")
	}
	credentials, err := CredentialsFile(m.Config)
	if err != nil {
		m.emit(audit.ActionConfigPublish, audit.ResultError, err, start, summary)
		return summary, err
	}
	if !m.SkipCredentialCheck {
		if _, err := m.FS.Stat(credentials); err != nil {
			err = fmt.Errorf("cloudflared credentials file %q: %w", credentials, err)
			m.emit(audit.ActionConfigPublish, audit.ResultError, err, start, summary)
			return summary, err
		}
	}
	data, err := RenderConfig(m.Config, routes)
	if err != nil {
		m.emit(audit.ActionConfigPublish, audit.ResultError, err, start, summary)
		return summary, err
	}
	original, hasOriginal := m.FS.ReadFile(m.ConfigPath)
	if hasOriginal == nil {
		summary.BackupPath = m.backupPath()
		if err := m.FS.MkdirAll(filepath.Dir(summary.BackupPath), 0o700); err != nil {
			m.emit(audit.ActionConfigPublish, audit.ResultError, err, start, summary)
			return summary, err
		}
		if err := m.FS.WriteFile(summary.BackupPath, original, 0o600); err != nil {
			m.emit(audit.ActionConfigPublish, audit.ResultError, err, start, summary)
			return summary, err
		}
	}
	if err := m.FS.WriteFile(m.ConfigPath, data, 0o600); err != nil {
		m.emit(audit.ActionConfigPublish, audit.ResultError, err, start, summary)
		return summary, err
	}
	if err := m.run(ctx, m.validateCmd(m.ConfigPath)); err != nil {
		if hasOriginal == nil {
			_ = m.FS.WriteFile(m.ConfigPath, original, 0o600)
			summary.RolledBack = true
		}
		m.emit(audit.ActionConfigPublish, audit.ResultRolledBack, err, start, summary)
		return summary, fmt.Errorf("validate cloudflared config: %w", err)
	}
	if err := m.run(ctx, m.sighupCmd()); err != nil {
		if hasOriginal == nil {
			if rerr := m.FS.WriteFile(m.ConfigPath, original, 0o600); rerr != nil {
				summary.RolledBack = false
				m.emit(audit.ActionCloudflaredHUP, audit.ResultError, err, start, summary)
				return summary, fmt.Errorf("sighup failed: %w; rollback failed: %v", err, rerr)
			}
			summary.RolledBack = true
			_ = m.run(ctx, m.sighupCmd())
		}
		m.emit(audit.ActionCloudflaredHUP, audit.ResultRolledBack, err, start, summary)
		return summary, fmt.Errorf("sighup cloudflared: %w", err)
	}
	m.emit(audit.ActionConfigPublish, audit.ResultOk, nil, start, summary)
	m.emit(audit.ActionCloudflaredHUP, audit.ResultOk, nil, start, summary)
	return summary, nil
}

func (m *Manager) backupPath() string {
	now := time.Now().UTC()
	if m.Now != nil {
		now = m.Now().UTC()
	}
	return filepath.Join("/var/lib/openclaw-multi/snapshots", now.Format("20060102T150405Z"), "cloudflared-config.yml")
}

func (m *Manager) validateCmd(path string) []string {
	if len(m.ValidateCmd) > 0 {
		return append([]string{}, m.ValidateCmd...)
	}
	return []string{"cloudflared", "tunnel", "ingress", "validate", "--config", path}
}

func (m *Manager) sighupCmd() []string {
	if len(m.SIGHUPCmd) > 0 {
		return append([]string{}, m.SIGHUPCmd...)
	}
	return []string{"sh", "-c", "kill -HUP $(systemctl show cloudflared -p MainPID --value)"}
}

func (m *Manager) run(ctx context.Context, cmd []string) error {
	if len(cmd) == 0 || strings.TrimSpace(cmd[0]) == "" {
		return errors.New("empty command")
	}
	_, err := m.Executor.Run(ctx, shell.ExecOpts{Cmd: cmd, Sudo: true, Timeout: 10 * time.Second})
	return err
}

func (m *Manager) emit(action audit.ActionType, result audit.Result, err error, start time.Time, summary PublishSummary) {
	if m.Auditor == nil {
		return
	}
	actor := m.Actor
	if actor == "" {
		actor = "system"
	}
	var msg string
	if err != nil {
		msg = err.Error()
	}
	_ = m.Auditor.Emit(audit.Event{
		Actor:        actor,
		Action:       action,
		Target:       m.ConfigPath,
		Result:       result,
		ErrorMessage: msg,
		DurationMs:   time.Since(start).Milliseconds(),
		Details: map[string]any{
			"backup_path": summary.BackupPath,
			"rolled_back": summary.RolledBack,
		},
	})
}
