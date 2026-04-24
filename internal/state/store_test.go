package state

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenReturnsErrorForInvalidPath(t *testing.T) {
	_, err := Open(context.Background(), "/nonexistent-dir-openclaw-test/state.db")
	if err == nil {
		t.Fatal("expected error for invalid directory")
	}
}

func TestOpenCreatesFileAndAppliesMigrations(t *testing.T) {
	s := openTemp(t)
	if s == nil {
		t.Fatal("expected non-nil Store")
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")

	s1, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("s1.Close: %v", err)
	}

	s2, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer func() { _ = s2.Close() }()

	// Verify only one migration row exists.
	var count int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM migrations`).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 migration row, got %d", count)
	}
}

func TestSetAndGetAdmin(t *testing.T) {
	s := openTemp(t)
	want := Admin{
		Username: "opadmin",
		UID:      1000,
		SetAt:    time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		SetBy:    "first-run",
	}
	if err := s.SetAdmin(context.Background(), want); err != nil {
		t.Fatalf("SetAdmin: %v", err)
	}
	got, err := s.GetAdmin(context.Background())
	if err != nil {
		t.Fatalf("GetAdmin: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil admin")
	}
	if got.Username != want.Username {
		t.Errorf("Username: got %q, want %q", got.Username, want.Username)
	}
	if got.UID != want.UID {
		t.Errorf("UID: got %d, want %d", got.UID, want.UID)
	}
	if got.SetBy != want.SetBy {
		t.Errorf("SetBy: got %q, want %q", got.SetBy, want.SetBy)
	}
}

func TestGetAdminReturnsErrNoAdminOnEmpty(t *testing.T) {
	s := openTemp(t)
	_, err := s.GetAdmin(context.Background())
	if !errors.Is(err, ErrNoAdmin) {
		t.Errorf("expected ErrNoAdmin, got %v", err)
	}
}

func TestClose(t *testing.T) {
	s := openTemp(t)
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestGetAdminOnClosedDBReturnsError(t *testing.T) {
	s := openTemp(t)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := s.GetAdmin(context.Background())
	if err == nil {
		t.Fatal("expected error when DB is closed")
	}
}

func TestSetAdminOnClosedDBReturnsError(t *testing.T) {
	s := openTemp(t)
	// Close manually (cleanup will try again, which is fine).
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := s.SetAdmin(context.Background(), Admin{
		Username: "ghost", UID: 999, SetAt: time.Now().UTC(), SetBy: "test",
	})
	if err == nil {
		t.Fatal("expected error when DB is closed")
	}
}

func TestSetAdminOverwritesPrevious(t *testing.T) {
	s := openTemp(t)
	first := Admin{Username: "alice", UID: 1001, SetAt: time.Now().UTC(), SetBy: "test"}
	if err := s.SetAdmin(context.Background(), first); err != nil {
		t.Fatalf("SetAdmin first: %v", err)
	}
	second := Admin{Username: "bob", UID: 1002, SetAt: time.Now().UTC(), SetBy: "test"}
	if err := s.SetAdmin(context.Background(), second); err != nil {
		t.Fatalf("SetAdmin second: %v", err)
	}
	got, err := s.GetAdmin(context.Background())
	if err != nil {
		t.Fatalf("GetAdmin: %v", err)
	}
	if got.Username != "bob" {
		t.Errorf("expected admin bob after overwrite, got %q", got.Username)
	}
}
