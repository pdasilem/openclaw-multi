package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

type auditRecorder struct {
	events []audit.Event
}

func (r *auditRecorder) Emit(e audit.Event) error {
	r.events = append(r.events, e)
	return nil
}

func TestCreateEncryptedBackup(t *testing.T) {
	ctx := context.Background()
	store := openBackupTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	fs := shell.NewMemFS()
	fs.Files["/var/lib/openclaw-multi/backups/alice/20260425T010203Z.tar.gz.enc"] = []byte("encrypted")
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{
		shell.OKResponse("created /home/alice/.openclaw-backup-tmp/backup.tar.gz\n"),
		shell.OKResponse(""),
	}}
	log := &auditRecorder{}
	m := testManager(store, exec, fs, log)

	backup, err := m.Create(ctx, CreateRequest{Username: "alice"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if backup.ID != "alice-20260425T010203Z" {
		t.Fatalf("backup ID: got %q", backup.ID)
	}
	sum := sha256.Sum256([]byte("encrypted"))
	if backup.SHA256 != hex.EncodeToString(sum[:]) || backup.SizeBytes != int64(len("encrypted")) {
		t.Fatalf("unexpected metadata: %+v", backup)
	}
	got, err := store.GetBackup(ctx, backup.ID)
	if err != nil {
		t.Fatalf("GetBackup: %v", err)
	}
	if got.Path != backup.Path || !got.Encrypted {
		t.Fatalf("unexpected stored backup: %+v", got)
	}
	if len(log.events) == 0 || log.events[len(log.events)-1].Action != audit.ActionBackupCreate {
		t.Fatalf("expected backup_create audit event, got %+v", log.events)
	}
}

func TestNewManagerDefaultsAndList(t *testing.T) {
	ctx := context.Background()
	store := openBackupTestStore(t)
	if err := store.UpsertBackup(ctx, state.Backup{ID: "b1", Username: "alice", TS: time.Now().UTC(), Path: "/b1.enc"}); err != nil {
		t.Fatalf("UpsertBackup: %v", err)
	}
	m := NewManager(store, &shell.MockExecutor{}, shell.NewMemFS(), nil, Options{})
	if m.Opts.BackupRoot != defaultBackupRoot || m.Opts.MasterKeyPath != defaultMasterKeyPath || m.Opts.TemplateDir != defaultTemplateDir {
		t.Fatalf("defaults not applied: %+v", m.Opts)
	}
	backups, err := m.List(ctx, "alice")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(backups) != 1 || backups[0].ID != "b1" {
		t.Fatalf("unexpected backups: %+v", backups)
	}
}

func TestManagerReadyRejectsMissingDependencies(t *testing.T) {
	ctx := context.Background()
	if _, err := (&Manager{}).List(ctx, "alice"); err == nil {
		t.Fatal("expected missing store error")
	}
	if _, err := (&Manager{Store: openBackupTestStore(t)}).List(ctx, "alice"); err == nil {
		t.Fatal("expected missing executor error")
	}
	if _, err := (&Manager{Store: openBackupTestStore(t), Exec: &shell.MockExecutor{}}).List(ctx, "alice"); err == nil {
		t.Fatal("expected missing filesystem error")
	}
}

func TestCreateRejectsMissingUserAndDoesNotRunCommands(t *testing.T) {
	exec := &shell.MockExecutor{}
	m := testManager(openBackupTestStore(t), exec, shell.NewMemFS(), nil)
	_, err := m.Create(context.Background(), CreateRequest{Username: "missing"})
	if !errors.Is(err, state.ErrNoUser) {
		t.Fatalf("expected ErrNoUser, got %v", err)
	}
	if exec.CallCount() != 0 {
		t.Fatalf("expected no commands, got %d", exec.CallCount())
	}
}

func TestCreateRejectsEmptyMasterKey(t *testing.T) {
	ctx := context.Background()
	store := openBackupTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	fs := shell.NewMemFS()
	fs.Files["/etc/openclaw-multi/master.key"] = []byte("\n")
	exec := &shell.MockExecutor{}
	m := testManager(store, exec, fs, nil)
	_, err := m.Create(ctx, CreateRequest{Username: "alice"})
	if err == nil {
		t.Fatal("expected empty master key error")
	}
	if exec.CallCount() != 0 {
		t.Fatalf("expected no commands, got %d", exec.CallCount())
	}
}

func TestCreateDoesNotRecordMetadataOnCommandFailure(t *testing.T) {
	ctx := context.Background()
	store := openBackupTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	m := testManager(store, shell.FailAt(0, "backup failed"), shell.NewMemFS(), nil)
	_, err := m.Create(ctx, CreateRequest{Username: "alice"})
	if err == nil {
		t.Fatal("expected backup failure")
	}
	backups, err := store.ListBackupsByUser(ctx, "alice")
	if err != nil {
		t.Fatalf("ListBackupsByUser: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("expected no metadata, got %+v", backups)
	}
}

func TestCreateCleansPartialOutputOnEncryptFailure(t *testing.T) {
	ctx := context.Background()
	store := openBackupTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	fs := shell.NewMemFS()
	dest := "/var/lib/openclaw-multi/backups/alice/20260425T010203Z.tar.gz.enc"
	fs.Files[dest] = []byte("partial")
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{shell.OKResponse("/tmp/backup.tar.gz\n"), {ExitCode: 1, Stderr: "encrypt failed"}},
		Errors:    []error{nil, errors.New("encrypt failed")},
	}
	m := testManager(store, exec, fs, nil)
	_, err := m.Create(ctx, CreateRequest{Username: "alice"})
	if err == nil {
		t.Fatal("expected encrypt error")
	}
	if _, ok := fs.Files[dest]; ok {
		t.Fatal("expected partial encrypted output to be removed")
	}
}

func TestCreateCleansOutputOnReadFailure(t *testing.T) {
	ctx := context.Background()
	store := openBackupTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	m := testManager(store, &shell.MockExecutor{
		Responses: []shell.ExecResult{shell.OKResponse("/tmp/backup.tar.gz\n"), shell.OKResponse("")},
	}, shell.NewMemFS(), nil)
	_, err := m.Create(ctx, CreateRequest{Username: "alice"})
	if err == nil {
		t.Fatal("expected encrypted backup read error")
	}
}

func TestCreateRejectsEmptyStdout(t *testing.T) {
	ctx := context.Background()
	store := openBackupTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	m := testManager(store, &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("\n")}}, shell.NewMemFS(), nil)
	_, err := m.Create(ctx, CreateRequest{Username: "alice"})
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRestoreExistingUser(t *testing.T) {
	ctx := context.Background()
	store := openBackupTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	backup := state.Backup{ID: "b1", Username: "alice", TS: time.Now().UTC(), Path: "/backups/alice/b1.tar.gz.enc", Encrypted: true}
	if err := store.UpsertBackup(ctx, backup); err != nil {
		t.Fatalf("UpsertBackup: %v", err)
	}
	fs := shell.NewMemFS()
	fs.Files["/etc/openclaw-multi/master.key"] = []byte("secret\n")
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("")}}
	m := testManager(store, exec, fs, nil)
	if err := m.Restore(ctx, RestoreRequest{Username: "alice", BackupID: "b1"}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if exec.CallCount() != 9 {
		t.Fatalf("expected 9 restore commands, got %d", exec.CallCount())
	}
	if exec.Calls[0].Cmd[0] != "openssl" {
		t.Fatalf("expected openssl first, got %+v", exec.Calls[0].Cmd)
	}
}

func TestRestoreRejectsMissingBackup(t *testing.T) {
	ctx := context.Background()
	store := openBackupTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	m := testManager(store, &shell.MockExecutor{}, shell.NewMemFS(), nil)
	if err := m.Restore(ctx, RestoreRequest{Username: "alice", BackupID: "missing"}); !errors.Is(err, state.ErrNoBackup) {
		t.Fatalf("expected ErrNoBackup, got %v", err)
	}
}

func TestRestoreRejectsMissingUserAndWrongOwner(t *testing.T) {
	ctx := context.Background()
	store := openBackupTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := store.UpsertBackup(ctx, state.Backup{ID: "b1", Username: "bob", TS: time.Now().UTC(), Path: "/b1.enc"}); err != nil {
		t.Fatalf("UpsertBackup: %v", err)
	}
	m := testManager(store, &shell.MockExecutor{}, shell.NewMemFS(), nil)
	if err := m.Restore(ctx, RestoreRequest{Username: "missing", BackupID: "b1"}); !errors.Is(err, state.ErrNoUser) {
		t.Fatalf("expected ErrNoUser, got %v", err)
	}
	if err := m.Restore(ctx, RestoreRequest{Username: "alice", BackupID: "b1"}); err == nil {
		t.Fatal("expected wrong-owner error")
	}
}

func TestRestoreFailsOnDecryptAndRestoreStep(t *testing.T) {
	ctx := context.Background()
	store := openBackupTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	backup := state.Backup{ID: "b1", Username: "alice", TS: time.Now().UTC(), Path: "/backups/alice/b1.tar.gz.enc", Encrypted: true}
	if err := store.UpsertBackup(ctx, backup); err != nil {
		t.Fatalf("UpsertBackup: %v", err)
	}
	fs := shell.NewMemFS()
	fs.Files["/etc/openclaw-multi/master.key"] = []byte("secret\n")
	if err := testManager(store, shell.FailAt(0, "decrypt failed"), fs, nil).Restore(ctx, RestoreRequest{Username: "alice", BackupID: "b1"}); err == nil {
		t.Fatal("expected decrypt failure")
	}
	if err := testManager(store, shell.FailAt(3, "tar failed"), fs, nil).Restore(ctx, RestoreRequest{Username: "alice", BackupID: "b1"}); err == nil {
		t.Fatal("expected restore step failure")
	}
}

func TestInstallTimerWritesUnitsAndRunsSystemctl(t *testing.T) {
	fs := shell.NewMemFS()
	fs.Files["templates/openclaw-backup@.service.tmpl"] = []byte("service ${USERNAME}\n")
	fs.Files["templates/openclaw-backup@.timer.tmpl"] = []byte("timer ${USERNAME}\n")
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{shell.OKResponse("")}}
	m := testManager(openBackupTestStore(t), exec, fs, nil)
	if err := m.InstallTimer(context.Background(), "alice"); err != nil {
		t.Fatalf("InstallTimer: %v", err)
	}
	if string(fs.Files["/etc/systemd/system/openclaw-backup@.service"]) != "service alice\n" {
		t.Fatalf("service template not rendered")
	}
	if exec.CallCount() != 2 {
		t.Fatalf("expected 2 systemctl commands, got %d", exec.CallCount())
	}
}

func TestInstallTimerReportsTemplateAndCommandErrors(t *testing.T) {
	missingTemplate := testManager(openBackupTestStore(t), &shell.MockExecutor{}, shell.NewMemFS(), nil)
	if err := missingTemplate.InstallTimer(context.Background(), "alice"); err == nil {
		t.Fatal("expected missing template error")
	}

	fsWithBadTemplate := shell.NewMemFS()
	fsWithBadTemplate.Files["templates/openclaw-backup@.service.tmpl"] = []byte("service ${MISSING}\n")
	fsWithBadTemplate.Files["templates/openclaw-backup@.timer.tmpl"] = []byte("timer ${USERNAME}\n")
	if err := testManager(openBackupTestStore(t), &shell.MockExecutor{}, fsWithBadTemplate, nil).InstallTimer(context.Background(), "alice"); err == nil {
		t.Fatal("expected render error")
	}

	fsWithTemplates := shell.NewMemFS()
	fsWithTemplates.Files["templates/openclaw-backup@.service.tmpl"] = []byte("service ${USERNAME}\n")
	fsWithTemplates.Files["templates/openclaw-backup@.timer.tmpl"] = []byte("timer ${USERNAME}\n")
	if err := testManager(openBackupTestStore(t), shell.FailAt(1, "enable failed"), fsWithTemplates, nil).InstallTimer(context.Background(), "alice"); err == nil {
		t.Fatal("expected systemctl failure")
	}
}

func TestBeforeRemoveCreatesBackup(t *testing.T) {
	ctx := context.Background()
	store := openBackupTestStore(t)
	if err := store.UpsertUser(ctx, state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	fs := shell.NewMemFS()
	fs.Files["/var/lib/openclaw-multi/backups/alice/20260425T010203Z.tar.gz.enc"] = []byte("encrypted")
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{
		shell.OKResponse("/tmp/backup.tar.gz\n"),
		shell.OKResponse(""),
	}}
	m := testManager(store, exec, fs, nil)
	if err := m.BeforeRemove(ctx, "alice"); err != nil {
		t.Fatalf("BeforeRemove: %v", err)
	}
	backups, err := store.ListBackupsByUser(ctx, "alice")
	if err != nil {
		t.Fatalf("ListBackupsByUser: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected one backup, got %+v", backups)
	}
}

type failingWriteFS struct {
	*shell.MemFS
}

func (f failingWriteFS) WriteFile(string, []byte, fs.FileMode) error {
	return os.ErrPermission
}

func TestInstallTimerReportsWriteErrors(t *testing.T) {
	mem := shell.NewMemFS()
	mem.Files["templates/openclaw-backup@.service.tmpl"] = []byte("service ${USERNAME}\n")
	mem.Files["templates/openclaw-backup@.timer.tmpl"] = []byte("timer ${USERNAME}\n")
	m := NewManager(openBackupTestStore(t), &shell.MockExecutor{}, failingWriteFS{MemFS: mem}, nil, Options{TemplateDir: "templates"})
	if err := m.InstallTimer(context.Background(), "alice"); err == nil {
		t.Fatal("expected write error")
	}
}

func openBackupTestStore(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testManager(store *state.Store, exec shell.Executor, fs *shell.MemFS, log *auditRecorder) *Manager {
	var logger Auditor
	if log != nil {
		logger = log
	}
	return NewManager(store, exec, fs, logger, Options{
		BackupRoot:    "/var/lib/openclaw-multi/backups",
		MasterKeyPath: "/etc/openclaw-multi/master.key",
		TemplateDir:   "templates",
		Clock: func() time.Time {
			return time.Date(2026, 4, 25, 1, 2, 3, 0, time.UTC)
		},
	})
}
