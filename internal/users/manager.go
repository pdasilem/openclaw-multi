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

	if err := m.run(ctx, []string{"useradd", "-m", "-s", "/bin/bash", username}); err != nil && !isUserAlreadyExists(err) {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.run(ctx, []string{"loginctl", "enable-linger", username}); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	uid, err := m.lookupUID(ctx, username)
	if err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.ensureUserManager(ctx, uid); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.bootstrapTenantRuntime(ctx, username); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.writeOpenClawWrappers(ctx, username); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.runOnboarding(ctx, username, uid, port, token); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.writeGatewayUnit(ctx, username, port, token); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.writeWatcherUnit(ctx, username); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.run(ctx, []string{"chmod", "700", "/home/" + username + "/.openclaw"}); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.run(ctx, []string{"chmod", "600", "/home/" + username + "/.openclaw/openclaw.json"}); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.runUserSystemctl(ctx, username, uid, "daemon-reload"); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.runUserSystemctl(ctx, username, uid, "enable", "--now", "openclaw-gateway.service"); err != nil {
		m.emit(audit.ActionBootstrapUser, username, audit.ResultError, err, start)
		return nil, err
	}
	if err := m.runUserSystemctl(ctx, username, uid, "enable", "--now", "openclaw-overlay-watcher.service"); err != nil {
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
	if err := m.run(ctx, []string{"su", "-", username, "-c", "systemctl --user stop openclaw-gateway.service openclaw-overlay-watcher.service"}); err != nil {
		m.emit(audit.ActionDisableUser, username, audit.ResultError, err, start)
		return err
	}
	if err := m.run(ctx, []string{"loginctl", "disable-linger", username}); err != nil {
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
	if err := m.run(ctx, []string{"loginctl", "enable-linger", username}); err != nil {
		m.emit(audit.ActionEnableUser, username, audit.ResultError, err, start)
		return err
	}
	if err := m.run(ctx, []string{"su", "-", username, "-c", "systemctl --user start openclaw-gateway.service openclaw-overlay-watcher.service"}); err != nil {
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
	if err := m.run(ctx, []string{"su", "-", username, "-c", "openclaw uninstall --all --yes --non-interactive"}); err != nil {
		m.emit(audit.ActionDeleteUser, username, audit.ResultError, err, start)
		return err
	}
	if err := m.run(ctx, []string{"loginctl", "disable-linger", username}); err != nil {
		m.emit(audit.ActionDeleteUser, username, audit.ResultError, err, start)
		return err
	}
	if err := m.run(ctx, []string{"userdel", "-r", username}); err != nil {
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

func (m *Manager) run(ctx context.Context, cmd []string) error {
	_, err := m.Exec.Run(ctx, shell.ExecOpts{Cmd: cmd, Sudo: true})
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
	openclawInstallCommand := strings.TrimSpace(m.Config.OpenClawUpdateCommand)
	if openclawInstallCommand == "" {
		openclawInstallCommand = config.Defaults().OpenClawUpdateCommand
	}
	script := strings.Join([]string{
		"set -e",
		"export NVM_DIR=\"$HOME/.nvm\"",
		"if [ ! -s \"$NVM_DIR/nvm.sh\" ]; then",
		"  if [ -d \"$NVM_DIR/.git\" ]; then",
		"    git -C \"$NVM_DIR\" fetch --tags origin",
		"    git -C \"$NVM_DIR\" checkout v0.40.3",
		"  else",
		"    rm -rf \"$NVM_DIR\"",
		"    git clone https://github.com/nvm-sh/nvm.git \"$NVM_DIR\"",
		"    git -C \"$NVM_DIR\" checkout v0.40.3",
		"  fi",
		"fi",
		". \"$NVM_DIR/nvm.sh\"",
		"nvm install " + shellQuote(nodeVersion),
		"nvm use " + shellQuote(nodeVersion),
		openclawInstallCommand,
		"mkdir -p \"$HOME/.local/bin\"",
	}, "\n")
	return m.run(ctx, []string{"su", "-", username, "-c", script})
}

func isUserAlreadyExists(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "already exist")
}

func (m *Manager) writeOpenClawWrappers(ctx context.Context, username string) error {
	nodeVersion := m.Config.NodeVersionMin
	if nodeVersion == "" {
		nodeVersion = config.Defaults().NodeVersionMin
	}
	openclawWrapper := strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"export NVM_DIR=\"$HOME/.nvm\"",
		". \"$NVM_DIR/nvm.sh\"",
		"nvm use " + shellQuote(nodeVersion) + " >/dev/null",
		"exec openclaw \"$@\"",
		"",
	}, "\n")
	if err := m.writeTenantFile(ctx, username, filepath.Join("/home", username, ".local", "bin", "openclaw"), []byte(openclawWrapper), "0700"); err != nil {
		return fmt.Errorf("write openclaw wrapper: %w", err)
	}
	gatewayWrapper := strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"exec \"$HOME/.local/bin/openclaw\" gateway start --daemon false",
		"",
	}, "\n")
	if err := m.writeTenantFile(ctx, username, filepath.Join("/home", username, ".local", "bin", "openclaw-gateway-start"), []byte(gatewayWrapper), "0700"); err != nil {
		return fmt.Errorf("write gateway wrapper: %w", err)
	}
	return nil
}

func (m *Manager) writeGatewayUnit(ctx context.Context, username string, port int, token string) error {
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
	if err := m.writeTenantFile(ctx, username, filepath.Join("/home", username, ".config", "systemd", "user", "openclaw-gateway.service"), []byte(rendered), "0600"); err != nil {
		return fmt.Errorf("write gateway unit: %w", err)
	}
	return nil
}

func (m *Manager) writeWatcherUnit(ctx context.Context, username string) error {
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
	if err := m.writeTenantFile(ctx, username, filepath.Join("/home", username, ".config", "systemd", "user", "openclaw-overlay-watcher.service"), []byte(rendered), "0600"); err != nil {
		return fmt.Errorf("write watcher unit: %w", err)
	}
	return nil
}

func (m *Manager) writeTenantFile(ctx context.Context, username string, path string, data []byte, mode string) error {
	dir := filepath.Dir(path)
	script := strings.Join([]string{
		"set -e",
		"install -d -m 0700 " + shellQuote(dir),
		"cat > " + shellQuote(path),
		"chmod " + shellQuote(mode) + " " + shellQuote(path),
	}, "\n")
	_, err := m.Exec.Run(ctx, shell.ExecOpts{
		Cmd:   []string{"su", "-", username, "-c", script},
		Sudo:  true,
		Stdin: string(data),
	})
	if err != nil {
		return fmt.Errorf("run tenant file write %q: %w", path, err)
	}
	return nil
}

func (m *Manager) runOnboarding(ctx context.Context, username string, uid int, port int, token string) error {
	envPath := filepath.Join("/home", username, ".openclaw-overlay", "onboard.env")
	envData := strings.Join([]string{
		"OPENCLAW_GATEWAY_TOKEN=" + shellQuote(token),
		"",
	}, "\n")
	if err := m.writeTenantFile(ctx, username, envPath, []byte(envData), "0600"); err != nil {
		return fmt.Errorf("write onboarding env: %w", err)
	}
	onboardCmd := strings.Join([]string{
		"/home/" + username + "/.local/bin/openclaw",
		"onboard",
		"--non-interactive",
		"--mode", "local",
		"--auth-choice", "skip",
		"--gateway-port", strconv.Itoa(port),
		"--gateway-bind", "loopback",
		"--gateway-auth", "token",
		"--gateway-token-ref-env", "OPENCLAW_GATEWAY_TOKEN",
		"--install-daemon",
		"--skip-skills",
		"--skip-health",
		"--accept-risk",
		"--json",
	}, " ")
	cmd := strings.Join([]string{
		"set -e",
		"trap 'rm -f " + shellQuote(envPath) + "' EXIT",
		"set -a",
		". " + shellQuote(envPath),
		"set +a",
		"export XDG_RUNTIME_DIR=/run/user/" + strconv.Itoa(uid),
		"export DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/" + strconv.Itoa(uid) + "/bus",
		onboardCmd,
	}, "\n")
	return m.run(ctx, []string{"su", "-", username, "-c", cmd})
}

func (m *Manager) ensureUserManager(ctx context.Context, uid int) error {
	return m.run(ctx, []string{"systemctl", "start", "user@" + strconv.Itoa(uid) + ".service"})
}

func (m *Manager) runUserSystemctl(ctx context.Context, username string, uid int, args ...string) error {
	cmd := []string{"-u", username, "env", "XDG_RUNTIME_DIR=/run/user/" + strconv.Itoa(uid), "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/" + strconv.Itoa(uid) + "/bus", "systemctl", "--user"}
	cmd = append(cmd, args...)
	_, err := m.Exec.Run(ctx, shell.ExecOpts{Cmd: cmd, Sudo: true})
	if err != nil {
		return fmt.Errorf("run user systemctl %q for %q: %w", strings.Join(args, " "), username, err)
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
