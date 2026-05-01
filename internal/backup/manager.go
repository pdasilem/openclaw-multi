// Package backup orchestrates encrypted OpenClaw backup and restore operations.
package backup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
	defaultBackupRoot    = "/var/lib/openclaw-multi/backups"
	defaultMasterKeyPath = "/etc/openclaw-multi/master.key"
	defaultTemplateDir   = "templates"
)

// Auditor is the small audit interface used by Manager.
type Auditor interface {
	Emit(e audit.Event) error
}

// Options configures backup paths and deterministic test hooks.
type Options struct {
	BackupRoot    string
	MasterKeyPath string
	TemplateDir   string
	Clock         func() time.Time
}

// Manager coordinates encrypted backup and restore operations.
type Manager struct {
	Store  *state.Store
	Exec   shell.Executor
	FS     shell.FS
	Logger Auditor
	Actor  string
	Opts   Options
}

// CreateRequest describes a backup create operation.
type CreateRequest struct {
	Username string
}

// RestoreRequest describes a backup restore operation.
type RestoreRequest struct {
	Username string
	BackupID string
}

// NewManager creates a backup manager with safe defaults.
func NewManager(store *state.Store, exec shell.Executor, fs shell.FS, logger Auditor, opts Options) *Manager {
	if opts.BackupRoot == "" {
		opts.BackupRoot = defaultBackupRoot
	}
	if opts.MasterKeyPath == "" {
		opts.MasterKeyPath = defaultMasterKeyPath
	}
	if opts.TemplateDir == "" {
		opts.TemplateDir = defaultTemplateDir
	}
	if opts.Clock == nil {
		opts.Clock = func() time.Time { return time.Now().UTC() }
	}
	return &Manager{Store: store, Exec: exec, FS: fs, Logger: logger, Actor: "admin", Opts: opts}
}

// List returns backup metadata for username, newest first.
func (m *Manager) List(ctx context.Context, username string) ([]state.Backup, error) {
	if err := m.ready(); err != nil {
		return nil, err
	}
	return m.Store.ListBackupsByUser(ctx, username)
}

// BeforeRemove creates the mandatory backup before destructive user removal.
func (m *Manager) BeforeRemove(ctx context.Context, username string) error {
	_, err := m.Create(ctx, CreateRequest{Username: username})
	return err
}

// Create creates an encrypted backup for username and records metadata.
func (m *Manager) Create(ctx context.Context, req CreateRequest) (*state.Backup, error) {
	start := time.Now()
	if err := m.ready(); err != nil {
		return nil, err
	}
	username := strings.TrimSpace(req.Username)
	user, err := m.Store.GetUser(ctx, username)
	if err != nil {
		m.emit(audit.ActionBackupCreate, username, audit.ResultError, nil, err, start)
		return nil, err
	}
	if err := m.ensureMasterKey(); err != nil {
		m.emit(audit.ActionBackupCreate, username, audit.ResultError, nil, err, start)
		return nil, err
	}

	res, err := m.runTenantShell(ctx, *user, "/home/"+username+"/.local/bin/openclaw backup create --output ~/.openclaw-backup-tmp --verify")
	if err != nil {
		err = fmt.Errorf("create openclaw backup: %w", err)
		m.emit(audit.ActionBackupCreate, username, audit.ResultError, nil, err, start)
		return nil, err
	}
	source, err := parseBackupPath(res.Stdout)
	if err != nil {
		m.emit(audit.ActionBackupCreate, username, audit.ResultError, nil, err, start)
		return nil, err
	}

	ts := m.Opts.Clock().UTC()
	stamp := ts.Format("20060102T150405Z")
	id := username + "-" + stamp
	destDir := filepath.Join(m.Opts.BackupRoot, username)
	dest := filepath.Join(destDir, stamp+".tar.gz.enc")
	if err := m.FS.MkdirAll(destDir, 0o700); err != nil {
		err = fmt.Errorf("mkdir backup dir: %w", err)
		m.emit(audit.ActionBackupCreate, username, audit.ResultError, nil, err, start)
		return nil, err
	}
	if _, err := m.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{
		"openssl", "enc", "-aes-256-cbc", "-pbkdf2", "-salt",
		"-pass", "file:" + m.Opts.MasterKeyPath,
		"-in", source,
		"-out", dest,
	}, Sudo: true}); err != nil {
		err = fmt.Errorf("encrypt backup: %w", err)
		_ = m.FS.Remove(dest)
		m.emit(audit.ActionBackupCreate, username, audit.ResultError, nil, err, start)
		return nil, err
	}
	data, err := m.FS.ReadFile(dest)
	if err != nil {
		err = fmt.Errorf("read encrypted backup: %w", err)
		_ = m.FS.Remove(dest)
		m.emit(audit.ActionBackupCreate, username, audit.ResultError, nil, err, start)
		return nil, err
	}
	sum := sha256.Sum256(data)
	backup := state.Backup{
		ID:        id,
		Username:  username,
		TS:        ts,
		SizeBytes: int64(len(data)),
		SHA256:    hex.EncodeToString(sum[:]),
		Encrypted: true,
		Path:      dest,
	}
	if err := m.Store.UpsertBackup(ctx, backup); err != nil {
		err = fmt.Errorf("record backup metadata: %w", err)
		_ = m.FS.Remove(dest)
		m.emit(audit.ActionBackupCreate, username, audit.ResultError, nil, err, start)
		return nil, err
	}
	m.emit(audit.ActionBackupCreate, username, audit.ResultOk, &backup, nil, start)
	return &backup, nil
}

// Restore restores a backup into an existing managed user.
func (m *Manager) Restore(ctx context.Context, req RestoreRequest) error {
	start := time.Now()
	if err := m.ready(); err != nil {
		return err
	}
	username := strings.TrimSpace(req.Username)
	user, err := m.Store.GetUser(ctx, username)
	if err != nil {
		m.emit(audit.ActionBackupRestore, username, audit.ResultError, nil, err, start)
		return err
	}
	b, err := m.Store.GetBackup(ctx, req.BackupID)
	if err != nil {
		m.emit(audit.ActionBackupRestore, username, audit.ResultError, nil, err, start)
		return err
	}
	if b.Username != username {
		err := fmt.Errorf("backup %q belongs to %q, not %q", b.ID, b.Username, username)
		m.emit(audit.ActionBackupRestore, username, audit.ResultError, b, err, start)
		return err
	}
	if err := m.ensureMasterKey(); err != nil {
		m.emit(audit.ActionBackupRestore, username, audit.ResultError, b, err, start)
		return err
	}
	tmpArchive := filepath.Join(m.Opts.BackupRoot, username, b.ID+".restore.tar.gz")
	if _, err := m.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{
		"openssl", "enc", "-aes-256-cbc", "-pbkdf2", "-d",
		"-pass", "file:" + m.Opts.MasterKeyPath,
		"-in", b.Path,
		"-out", tmpArchive,
	}, Sudo: true}); err != nil {
		err = fmt.Errorf("decrypt backup: %w", err)
		m.emit(audit.ActionBackupRestore, username, audit.ResultError, b, err, start)
		return err
	}
	defer m.FS.Remove(tmpArchive) //nolint:errcheck

	if _, err := m.runTenantShell(ctx, *user, "/home/"+username+"/.local/bin/openclaw backup verify "+tmpArchive); err != nil {
		err = fmt.Errorf("restore verify backup: %w", err)
		m.emit(audit.ActionBackupRestore, username, audit.ResultError, b, err, start)
		return err
	}
	if _, err := m.runUserSystemctl(ctx, *user, "stop", "openclaw-gateway.service"); err != nil {
		err = fmt.Errorf("restore stop gateway: %w", err)
		m.emit(audit.ActionBackupRestore, username, audit.ResultError, b, err, start)
		return err
	}
	commands := [][]string{
		{"mkdir", "-p", "/home/" + username + "/.openclaw-restore-tmp"},
		{"tar", "-xzf", tmpArchive, "-C", "/home/" + username + "/.openclaw-restore-tmp"},
		{"rm", "-rf", "/home/" + username + "/.openclaw"},
		{"mv", "/home/" + username + "/.openclaw-restore-tmp/.openclaw", "/home/" + username + "/.openclaw"},
		{"chown", "-R", username + ":" + username, "/home/" + username + "/.openclaw"},
	}
	for _, cmd := range commands {
		if _, err := m.Exec.Run(ctx, shell.ExecOpts{Cmd: cmd, Sudo: true}); err != nil {
			err = fmt.Errorf("restore step %q: %w", strings.Join(cmd, " "), err)
			m.emit(audit.ActionBackupRestore, username, audit.ResultError, b, err, start)
			return err
		}
	}
	if _, err := m.runUserSystemctl(ctx, *user, "start", "openclaw-gateway.service"); err != nil {
		err = fmt.Errorf("restore start gateway: %w", err)
		m.emit(audit.ActionBackupRestore, username, audit.ResultError, b, err, start)
		return err
	}
	m.emit(audit.ActionBackupRestore, username, audit.ResultOk, b, nil, start)
	return nil
}

// InstallTimer installs intended systemd timer units for automatic backups.
func (m *Manager) InstallTimer(ctx context.Context, username string) error {
	if err := m.ready(); err != nil {
		return err
	}
	vars := map[string]string{"USERNAME": username}
	for _, name := range []string{"openclaw-backup@.service", "openclaw-backup@.timer"} {
		templatePath := filepath.Join(m.Opts.TemplateDir, name+".tmpl")
		data, err := m.FS.ReadFile(templatePath)
		if err != nil {
			return fmt.Errorf("read timer template %q: %w", name, err)
		}
		rendered, err := config.Render(string(data), vars)
		if err != nil {
			return fmt.Errorf("render timer template %q: %w", name, err)
		}
		if err := m.FS.WriteFile(filepath.Join("/etc/systemd/system", name), []byte(rendered), 0o644); err != nil {
			return fmt.Errorf("write timer unit %q: %w", name, err)
		}
	}
	for _, cmd := range [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", "openclaw-backup@" + username + ".timer"},
	} {
		if _, err := m.Exec.Run(ctx, shell.ExecOpts{Cmd: cmd, Sudo: true}); err != nil {
			return fmt.Errorf("install backup timer step %q: %w", strings.Join(cmd, " "), err)
		}
	}
	return nil
}

func (m *Manager) ready() error {
	switch {
	case m.Store == nil:
		return errors.New("backup manager requires state store")
	case m.Exec == nil:
		return errors.New("backup manager requires executor")
	case m.FS == nil:
		return errors.New("backup manager requires filesystem")
	}
	return nil
}

func (m *Manager) runTenantShell(ctx context.Context, user state.User, script string) (shell.ExecResult, error) {
	return m.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"-u", user.Username, "-H", "bash", "-lc", script}, Sudo: true})
}

func (m *Manager) runUserSystemctl(ctx context.Context, user state.User, args ...string) (shell.ExecResult, error) {
	cmd := []string{"-u", user.Username, "env", "XDG_RUNTIME_DIR=/run/user/" + strconv.Itoa(user.UID), "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/" + strconv.Itoa(user.UID) + "/bus", "systemctl", "--user"}
	cmd = append(cmd, args...)
	return m.Exec.Run(ctx, shell.ExecOpts{Cmd: cmd, Sudo: true})
}

func (m *Manager) ensureMasterKey() error {
	data, err := m.FS.ReadFile(m.Opts.MasterKeyPath)
	if err == nil {
		if len(strings.TrimSpace(string(data))) == 0 {
			return errors.New("master key is empty")
		}
		return nil
	}
	var key [32]byte
	if _, readErr := rand.Read(key[:]); readErr != nil {
		return fmt.Errorf("generate master key: %w", readErr)
	}
	encoded := make([]byte, hex.EncodedLen(len(key)))
	hex.Encode(encoded, key[:])
	encoded = append(encoded, '\n')
	if writeErr := m.FS.WriteFile(m.Opts.MasterKeyPath, encoded, 0o600); writeErr != nil {
		return fmt.Errorf("write master key: %w", writeErr)
	}
	return nil
}

func parseBackupPath(stdout string) (string, error) {
	for _, line := range reverseLines(stdout) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		return fields[len(fields)-1], nil
	}
	return "", errors.New("backup command did not report archive path")
}

func reverseLines(s string) []string {
	lines := strings.Split(s, "\n")
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines
}

func (m *Manager) emit(action audit.ActionType, target string, result audit.Result, b *state.Backup, err error, start time.Time) {
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
	if b != nil {
		event.Details = map[string]any{"backup_id": b.ID, "path": b.Path}
	}
	if err != nil {
		event.ErrorMessage = err.Error()
	}
	_ = m.Logger.Emit(event)
}
