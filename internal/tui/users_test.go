package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	backupops "github.com/pdasilem/openclaw-multi/internal/backup"
	"github.com/pdasilem/openclaw-multi/internal/state"
	userops "github.com/pdasilem/openclaw-multi/internal/users"
)

type fakeUserService struct {
	users       []state.User
	added       string
	deactivated string
	activated   string
	removed     string
}

func (f *fakeUserService) List(_ context.Context) ([]state.User, error) {
	cp := make([]state.User, len(f.users))
	copy(cp, f.users)
	return cp, nil
}

func (f *fakeUserService) Add(_ context.Context, req userops.AddRequest) (*state.User, error) {
	f.added = req.Username
	u := state.User{Username: req.Username, Port: 18789, Status: state.UserStatusActive}
	f.users = append(f.users, u)
	return &u, nil
}

func (f *fakeUserService) Deactivate(_ context.Context, username string) error {
	f.deactivated = username
	for i := range f.users {
		if f.users[i].Username == username {
			f.users[i].Status = state.UserStatusPaused
		}
	}
	return nil
}

func (f *fakeUserService) Activate(_ context.Context, username string) error {
	f.activated = username
	for i := range f.users {
		if f.users[i].Username == username {
			f.users[i].Status = state.UserStatusActive
		}
	}
	return nil
}

func (f *fakeUserService) Remove(_ context.Context, req userops.RemoveRequest) error {
	f.removed = req.Username
	next := f.users[:0]
	for _, u := range f.users {
		if u.Username != req.Username {
			next = append(next, u)
		}
	}
	f.users = next
	return nil
}

type fakeBackupService struct {
	backups  []state.Backup
	created  string
	restored string
}

func (f *fakeBackupService) List(_ context.Context, username string) ([]state.Backup, error) {
	var out []state.Backup
	for _, backup := range f.backups {
		if backup.Username == username {
			out = append(out, backup)
		}
	}
	return out, nil
}

func (f *fakeBackupService) Create(_ context.Context, req backupops.CreateRequest) (*state.Backup, error) {
	f.created = req.Username
	backup := state.Backup{ID: req.Username + "-backup", Username: req.Username, SizeBytes: 12, Path: "/backups/" + req.Username + ".tar.gz.enc"}
	f.backups = append(f.backups, backup)
	return &backup, nil
}

func (f *fakeBackupService) Restore(_ context.Context, req backupops.RestoreRequest) error {
	f.restored = req.Username + ":" + req.BackupID
	return nil
}

func TestUserManagementLoadsAndRendersUsers(t *testing.T) {
	svc := &fakeUserService{users: []state.User{{
		Username:   "alice",
		Port:       18789,
		Status:     state.UserStatusActive,
		GatewayURL: "https://gateway-alice.ui.example.com",
	}}}
	m := newUserManagement(svc, &fakeBackupService{})
	msg := m.Init()()
	loaded, ok := msg.(usersLoadedMsg)
	if !ok {
		t.Fatalf("expected usersLoadedMsg, got %T", msg)
	}
	m, _ = m.Update(loaded)
	view := m.View()
	if !contains(view, "alice") || !contains(view, "18789") {
		t.Fatalf("expected alice in view, got %q", view)
	}
}

func TestUserManagementAddFlow(t *testing.T) {
	svc := &fakeUserService{}
	m := newUserManagement(svc, &fakeBackupService{})
	m, _ = m.Update(usersLoadedMsg{})
	m, _ = m.Update(keyText("a"))
	for _, ch := range "alice" {
		m, _ = m.Update(keyText(string(ch)))
	}
	_, cmd := m.Update(keyPress(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("expected add command")
	}
	done := cmd().(userOpDoneMsg)
	_, cmd = m.Update(done)
	if done.err != nil {
		t.Fatalf("add error: %v", done.err)
	}
	if svc.added != "alice" {
		t.Fatalf("expected added alice, got %q", svc.added)
	}
	if cmd == nil {
		t.Fatal("expected reload command")
	}
}

func TestUserManagementRemoveActiveDeactivatesFirst(t *testing.T) {
	svc := &fakeUserService{users: []state.User{{
		Username: "alice",
		Status:   state.UserStatusActive,
		Port:     18789,
	}}}
	m := newUserManagement(svc, &fakeBackupService{})
	m, _ = m.Update(usersLoadedMsg{users: svc.users})
	m, _ = m.Update(keyText("x"))
	for _, ch := range "alice" {
		m, _ = m.Update(keyText(string(ch)))
	}
	_, cmd := m.Update(keyPress(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("expected remove command")
	}
	done := cmd().(userOpDoneMsg)
	if done.err != nil {
		t.Fatalf("remove error: %v", done.err)
	}
	if svc.deactivated != "alice" || svc.removed != "alice" {
		t.Fatalf("expected deactivate then remove, got deactivated=%q removed=%q", svc.deactivated, svc.removed)
	}
}

func TestUserManagementBackupFlow(t *testing.T) {
	svc := &fakeUserService{users: []state.User{{Username: "alice", Status: state.UserStatusActive, Port: 18789}}}
	backups := &fakeBackupService{}
	m := newUserManagement(svc, backups)
	m, _ = m.Update(usersLoadedMsg{users: svc.users})
	_, cmd := m.Update(keyText("b"))
	if cmd == nil {
		t.Fatal("expected backup command")
	}
	done := cmd().(userOpDoneMsg)
	if done.err != nil {
		t.Fatalf("backup error: %v", done.err)
	}
	if backups.created != "alice" {
		t.Fatalf("expected backup for alice, got %q", backups.created)
	}
}

func TestUserManagementRestoreFlow(t *testing.T) {
	svc := &fakeUserService{users: []state.User{{Username: "alice", Status: state.UserStatusActive, Port: 18789}}}
	backups := &fakeBackupService{backups: []state.Backup{{ID: "b1", Username: "alice", SizeBytes: 12, Path: "/b1.enc"}}}
	m := newUserManagement(svc, backups)
	m, _ = m.Update(usersLoadedMsg{users: svc.users})
	_, cmd := m.Update(keyText("r"))
	if cmd == nil {
		t.Fatal("expected load backups command")
	}
	loaded := cmd().(backupsLoadedMsg)
	m, _ = m.Update(loaded)
	if !contains(m.View(), "b1") {
		t.Fatalf("expected backup list in view, got %q", m.View())
	}
	_, cmd = m.Update(keyPress(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("expected restore command")
	}
	done := cmd().(userOpDoneMsg)
	if done.err != nil {
		t.Fatalf("restore error: %v", done.err)
	}
	if backups.restored != "alice:b1" {
		t.Fatalf("expected restore alice:b1, got %q", backups.restored)
	}
}
