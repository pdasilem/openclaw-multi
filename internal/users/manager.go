package users

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

const (
	gatewayTemplateName = "openclaw-gateway.service.tmpl"
	watcherTemplateName = "openclaw-overlay-watcher.service.tmpl"
)

// Auditor is the small audit interface used by Manager.
type Auditor interface {
	Emit(e audit.Event) error
}

// BeforeRemoveHook runs mandatory pre-remove work such as backup creation.
type BeforeRemoveHook interface {
	BeforeRemove(ctx context.Context, username string) error
}

// Manager coordinates lifecycle operations for managed OpenClaw users.
type Manager struct {
	Store        *state.Store
	Exec         shell.Executor
	FS           shell.FS
	Config       *config.OverlayConfig
	Routes       RoutePublisher
	BeforeRemove BeforeRemoveHook
	Logger       Auditor
	Actor        string
	TemplateDir  string
}

// AddRequest describes a user bootstrap operation.
type AddRequest struct {
	Username string
}

// RemoveRequest describes a destructive user removal.
type RemoveRequest struct {
	Username string
}

// NewManager returns a Manager with defaults filled where safe.
func NewManager(store *state.Store, exec shell.Executor, fs shell.FS, cfg *config.OverlayConfig, routes RoutePublisher, logger Auditor) *Manager {
	if cfg == nil {
		cfg = config.Defaults()
	}
	if routes == nil {
		routes = StateRoutePublisher{Store: store, Config: cfg}
	}
	return &Manager{
		Store:       store,
		Exec:        exec,
		FS:          fs,
		Config:      cfg,
		Routes:      routes,
		Logger:      logger,
		Actor:       "admin",
		TemplateDir: "templates",
	}
}

// List returns managed users sorted by username.
func (m *Manager) List(ctx context.Context) ([]state.User, error) {
	if err := m.ready(); err != nil {
		return nil, err
	}
	return m.Store.ListUsers(ctx)
}

// Add validates, bootstraps, records, and routes a new managed user.
func (m *Manager) Add(ctx context.Context, req AddRequest) (*state.User, error) {
	start := time.Now()
	if err := m.ready(); err != nil {
		return nil, err
	}
	username := strings.TrimSpace(req.Username)
	if err := ValidateUsername(username); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if _, err := GatewayHostname(m.Config, username); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	exists, err := m.Store.UserExists(ctx, username)
	if err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if exists {
		err := fmt.Errorf("user %q already exists", username)
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}

	existing, err := m.Store.ListUsers(ctx)
	if err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	port, err := AllocateGatewayPort(m.Config, existing)
	if err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	token, err := generateToken()
	if err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}

	env := []string{
		fmt.Sprintf("OPENCLAW_GATEWAY_PORT=%d", port),
		"OPENCLAW_GATEWAY_TOKEN=" + token,
		"OPENCLAW_GATEWAY_BIND=loopback",
	}
	if err := m.run(ctx, []string{"useradd", "-m", "-s", "/bin/bash", username}, nil); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.run(ctx, []string{"loginctl", "enable-linger", username}, nil); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	uid, err := m.lookupUID(ctx, username)
	if err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.bootstrapTenantRuntime(ctx, username); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.writeOpenClawWrappers(username); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.run(ctx, []string{"chown", "-R", username + ":" + username, "/home/" + username + "/.local/bin"}, nil); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.run(ctx, []string{"su", "-", username, "-c", "/home/" + username + "/.local/bin/openclaw onboard --install-daemon"}, env); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.writeGatewayUnit(username, port, token); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.writeWatcherUnit(username); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.run(ctx, []string{"chown", "-R", username + ":" + username, "/home/" + username + "/.config/systemd/user", "/home/" + username + "/.local/bin"}, nil); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.run(ctx, []string{"chmod", "700", "/home/" + username + "/.openclaw"}, nil); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.run(ctx, []string{"chmod", "600", "/home/" + username + "/.openclaw/openclaw.json"}, nil); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.run(ctx, []string{"su", "-", username, "-c", "systemctl --user daemon-reload"}, nil); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.run(ctx, []string{"su", "-", username, "-c", "systemctl --user enable --now openclaw-gateway.service"}, nil); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.run(ctx, []string{"su", "-", username, "-c", "systemctl --user enable --now openclaw-overlay-watcher.service"}, nil); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}

	gatewayURL, err := GatewayURL(m.Config, username)
	if err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	user := state.User{
		Username:   username,
		UID:        uid,
		Port:       port,
		Status:     state.UserStatusActive,
		Linger:     true,
		GatewayURL: gatewayURL,
	}
	if err := m.Store.UpsertUser(ctx, user); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if _, err := m.Routes.EnableUserGateway(ctx, user); err != nil {
		_ = m.Store.DeleteUser(ctx, username)
		m.emit(audit.ActionBootstrapUser, username, audit.ResultRolledBack, err, start)
		return nil, err
	}
	m.emit(audit.ActionAddRoute, username, audit.ResultOk, nil, start)
	m.emit(audit.ActionBootstrapUser, username, audit.ResultOk, nil, start)
	return m.Store.GetUser(ctx, username)
}

// Deactivate pauses services and routes for a managed user.
func (m *Manager) Deactivate(ctx context.Context, username string) error {
	start := time.Now()
	if err := m.ready(); err != nil {
		return err
	}
	user, err := m.Store.GetUser(ctx, username)
	if err != nil {
		m.emit(audit.ActionDisableUser, username, audit.ResultError, err, start)
		return err
	}
	if user.Status == state.UserStatusPaused {
		m.emit(audit.ActionDisableUser, username, audit.ResultOk, nil, start)
		return nil
	}
	if err := m.run(ctx, []string{"su", "-", username, "-c", "systemctl --user stop openclaw-gateway.service openclaw-overlay-watcher.service"}, nil); err != nil {
		m.emit(audit.ActionDisableUser, username, audit.ResultError, err, start)
		return err
	}
	if err := m.run(ctx, []string{"loginctl", "disable-linger", username}, nil); err != nil {
		m.emit(audit.ActionDisableUser, username, audit.ResultError, err, start)
		return err
	}
	if err := m.Routes.DisableUserRoutes(ctx, username); err != nil {
		m.emit(audit.ActionDisableUser, username, audit.ResultError, err, start)
		return err
	}
	user.Status = state.UserStatusPaused
	user.Linger = false
	if err := m.Store.UpsertUser(ctx, *user); err != nil {
		m.emit(audit.ActionDisableUser, username, audit.ResultError, err, start)
		return err
	}
	m.emit(audit.ActionDisableUser, username, audit.ResultOk, nil, start)
	return nil
}

// Activate resumes services and routes for a managed user.
func (m *Manager) Activate(ctx context.Context, username string) error {
	start := time.Now()
	if err := m.ready(); err != nil {
		return err
	}
	user, err := m.Store.GetUser(ctx, username)
	if err != nil {
		m.emit(audit.ActionEnableUser, username, audit.ResultError, err, start)
		return err
	}
	if user.Status == state.UserStatusActive {
		m.emit(audit.ActionEnableUser, username, audit.ResultOk, nil, start)
		return nil
	}
	if _, err := m.Routes.EnableUserGateway(ctx, *user); err != nil {
		m.emit(audit.ActionEnableUser, username, audit.ResultError, err, start)
		return err
	}
	if err := m.run(ctx, []string{"loginctl", "enable-linger", username}, nil); err != nil {
		m.emit(audit.ActionEnableUser, username, audit.ResultError, err, start)
		return err
	}
	if err := m.run(ctx, []string{"su", "-", username, "-c", "systemctl --user start openclaw-gateway.service openclaw-overlay-watcher.service"}, nil); err != nil {
		m.emit(audit.ActionEnableUser, username, audit.ResultError, err, start)
		return err
	}
	user.Status = state.UserStatusActive
	user.Linger = true
	if err := m.Store.UpsertUser(ctx, *user); err != nil {
		m.emit(audit.ActionEnableUser, username, audit.ResultError, err, start)
		return err
	}
	m.emit(audit.ActionEnableUser, username, audit.ResultOk, nil, start)
	return nil
}

// Remove deletes a managed user and its routes/state.
func (m *Manager) Remove(ctx context.Context, req RemoveRequest) error {
	start := time.Now()
	if err := m.ready(); err != nil {
		return err
	}
	username := strings.TrimSpace(req.Username)
	user, err := m.Store.GetUser(ctx, username)
	if err != nil {
		m.emit(audit.ActionDeleteUser, username, audit.ResultError, err, start)
		return err
	}
	if user.Status == state.UserStatusActive {
		if err := m.Deactivate(ctx, username); err != nil {
			m.emit(audit.ActionDeleteUser, username, audit.ResultError, err, start)
			return err
		}
	}
	if m.BeforeRemove != nil {
		if err := m.BeforeRemove.BeforeRemove(ctx, username); err != nil {
			m.emit(audit.ActionDeleteUser, username, audit.ResultError, err, start)
			return err
		}
	}
	if err := m.Routes.DeleteUserRoutes(ctx, username); err != nil {
		m.emit(audit.ActionDeleteUser, username, audit.ResultError, err, start)
		return err
	}
	m.emit(audit.ActionDeleteRoute, username, audit.ResultOk, nil, start)
	if err := m.run(ctx, []string{"su", "-", username, "-c", "openclaw uninstall --all --yes --non-interactive"}, nil); err != nil {
		m.emit(audit.ActionDeleteUser, username, audit.ResultError, err, start)
		return err
	}
	if err := m.run(ctx, []string{"loginctl", "disable-linger", username}, nil); err != nil {
		m.emit(audit.ActionDeleteUser, username, audit.ResultError, err, start)
		return err
	}
	if err := m.run(ctx, []string{"userdel", "-r", username}, nil); err != nil {
		m.emit(audit.ActionDeleteUser, username, audit.ResultError, err, start)
		return err
	}
	if err := m.Store.DeleteUser(ctx, username); err != nil {
		m.emit(audit.ActionDeleteUser, username, audit.ResultError, err, start)
		return err
	}
	m.emit(audit.ActionDeleteUser, username, audit.ResultOk, nil, start)
	return nil
}

func (m *Manager) ready() error {
	switch {
	case m.Store == nil:
		return errors.New("users manager requires state store")
	case m.Exec == nil:
		return errors.New("users manager requires executor")
	case m.FS == nil:
		return errors.New("users manager requires filesystem")
	case m.Routes == nil:
		return errors.New("users manager requires route publisher")
	}
	if m.Config == nil {
		m.Config = config.Defaults()
	}
	return nil
}

func (m *Manager) run(ctx context.Context, cmd []string, env []string) error {
	_, err := m.Exec.Run(ctx, shell.ExecOpts{Cmd: cmd, Env: env})
	if err != nil {
		return fmt.Errorf("run %q: %w", strings.Join(cmd, " "), err)
	}
	return nil
}

func (m *Manager) lookupUID(ctx context.Context, username string) (int, error) {
	res, err := m.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"id", "-u", username}})
	if err != nil {
		return 0, fmt.Errorf("lookup uid for %q: %w", username, err)
	}
	uid, err := strconv.Atoi(strings.TrimSpace(res.Stdout))
	if err != nil {
		return 0, fmt.Errorf("parse uid for %q: %w", username, err)
	}
	return uid, nil
}

func (m *Manager) bootstrapTenantRuntime(ctx context.Context, username string) error {
	nodeVersion := m.Config.NodeVersionMin
	if nodeVersion == "" {
		nodeVersion = config.Defaults().NodeVersionMin
	}
	script := strings.Join([]string{
		"set -e",
		"export NVM_DIR=\"$HOME/.nvm\"",
		"if [ ! -s \"$NVM_DIR/nvm.sh\" ]; then git clone https://github.com/nvm-sh/nvm.git \"$NVM_DIR\"; cd \"$NVM_DIR\"; git checkout v0.40.3; fi",
		". \"$NVM_DIR/nvm.sh\"",
		"nvm install " + shellQuote(nodeVersion),
		"nvm use " + shellQuote(nodeVersion),
		"npm install --global github:pdasilem/openclaw#latest",
		"mkdir -p \"$HOME/.local/bin\"",
	}, "\n")
	return m.run(ctx, []string{"su", "-", username, "-c", script}, nil)
}

func (m *Manager) writeOpenClawWrappers(username string) error {
	nodeVersion := m.Config.NodeVersionMin
	if nodeVersion == "" {
		nodeVersion = config.Defaults().NodeVersionMin
	}
	dir := filepath.Join("/home", username, ".local", "bin")
	if err := m.FS.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir tenant bin dir: %w", err)
	}
	openclawWrapper := strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"export NVM_DIR=\"$HOME/.nvm\"",
		". \"$NVM_DIR/nvm.sh\"",
		"nvm use " + shellQuote(nodeVersion) + " >/dev/null",
		"exec \"$NVM_DIR/versions/node/v" + nodeVersion + "/bin/openclaw\" \"$@\"",
		"",
	}, "\n")
	if err := m.FS.WriteFile(filepath.Join(dir, "openclaw"), []byte(openclawWrapper), 0o700); err != nil {
		return fmt.Errorf("write openclaw wrapper: %w", err)
	}
	gatewayWrapper := strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"exec \"$HOME/.local/bin/openclaw\" gateway start --daemon false",
		"",
	}, "\n")
	if err := m.FS.WriteFile(filepath.Join(dir, "openclaw-gateway-start"), []byte(gatewayWrapper), 0o700); err != nil {
		return fmt.Errorf("write gateway wrapper: %w", err)
	}
	return nil
}

func (m *Manager) writeGatewayUnit(username string, port int, token string) error {
	templatePath := filepath.Join(m.TemplateDir, gatewayTemplateName)
	data, err := m.FS.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("read gateway template: %w", err)
	}
	rendered, err := config.Render(string(data), map[string]string{
		"USERNAME":      username,
		"GATEWAY_PORT":  strconv.Itoa(port),
		"GATEWAY_TOKEN": token,
	})
	if err != nil {
		return fmt.Errorf("render gateway template: %w", err)
	}
	dir := filepath.Join("/home", username, ".config", "systemd", "user")
	if err := m.FS.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir gateway unit dir: %w", err)
	}
	if err := m.FS.WriteFile(filepath.Join(dir, "openclaw-gateway.service"), []byte(rendered), 0o600); err != nil {
		return fmt.Errorf("write gateway unit: %w", err)
	}
	return nil
}

func (m *Manager) writeWatcherUnit(username string) error {
	templatePath := filepath.Join(m.TemplateDir, watcherTemplateName)
	data, err := m.FS.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("read watcher template: %w", err)
	}
	rendered, err := config.Render(string(data), map[string]string{
		"USERNAME":             username,
		"OVERLAY_API_ENDPOINT": "http://127.0.0.1:18788",
		"OVERLAY_SOCKET":       "/run/openclaw-overlay.sock",
	})
	if err != nil {
		return fmt.Errorf("render watcher template: %w", err)
	}
	dir := filepath.Join("/home", username, ".config", "systemd", "user")
	if err := m.FS.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir watcher unit dir: %w", err)
	}
	if err := m.FS.WriteFile(filepath.Join(dir, "openclaw-overlay-watcher.service"), []byte(rendered), 0o600); err != nil {
		return fmt.Errorf("write watcher unit: %w", err)
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func (m *Manager) emit(action audit.ActionType, target string, result audit.Result, err error, start time.Time) {
	if m.Logger == nil {
		return
	}
	event := audit.Event{
		Actor:      m.Actor,
		Action:     action,
		Target:     target,
		Result:     result,
		DurationMs: time.Since(start).Milliseconds(),
	}
	if err != nil {
		event.ErrorMessage = err.Error()
	}
	_ = m.Logger.Emit(event)
}

func generateToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
