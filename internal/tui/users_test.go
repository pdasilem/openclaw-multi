package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

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

func TestUserManagementLoadsAndRendersUsers(t *testing.T) {
	svc := &fakeUserService{users: []state.User{{
		Username:   "alice",
		Port:       18789,
		Status:     state.UserStatusActive,
		GatewayURL: "https://gateway-alice.ui.example.com",
	}}}
	m := newUserManagement(svc)
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
	m := newUserManagement(svc)
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
	m := newUserManagement(svc)
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
