package admin

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

// fakeStore is an in-memory AdminStore for tests.
type fakeStore struct {
	admin *state.Admin
}

func (f *fakeStore) GetAdmin(_ context.Context) (*state.Admin, error) {
	if f.admin == nil {
		return nil, fmt.Errorf("no admin configured")
	}
	return f.admin, nil
}

func (f *fakeStore) SetAdmin(_ context.Context, a state.Admin) error {
	cp := a
	f.admin = &cp
	return nil
}

// fakeLogger captures emitted events.
type fakeLogger struct {
	events []audit.Event
}

func (f *fakeLogger) Emit(e audit.Event) error {
	f.events = append(f.events, e)
	return nil
}

func TestResolveCurrent(t *testing.T) {
	u, err := ResolveCurrent()
	if err != nil {
		t.Fatalf("ResolveCurrent: %v", err)
	}
	if u == nil {
		t.Fatal("expected non-nil User")
	}
	if u.Username == "" {
		t.Error("expected non-empty Username")
	}
}

func TestResolveCandidatePrefersSudoUser(t *testing.T) {
	t.Setenv("SUDO_USER", "")
	// When SUDO_USER is empty, falls back to current user.
	u, err := ResolveCandidate()
	if err != nil {
		t.Fatalf("ResolveCandidate: %v", err)
	}
	if u == nil {
		t.Fatal("expected non-nil User")
	}
}

func TestResolveCandidateRejectsSudoUserRoot(t *testing.T) {
	t.Setenv("SUDO_USER", "root")
	_, err := ResolveCandidate()
	if err == nil {
		t.Fatal("expected error for SUDO_USER=root")
	}
}

func TestWarnIfRootEmptyForNormalUser(t *testing.T) {
	if IsRoot() {
		t.Skip("test runs as root — skipping WarnIfRoot check")
	}
	if w := WarnIfRoot(); w != "" {
		t.Errorf("expected empty warning for non-root user, got %q", w)
	}
}

func TestPersistAdminEmitsAuditEvent(t *testing.T) {
	store := &fakeStore{}
	logger := &fakeLogger{}
	u := &User{Username: "opadmin", UID: 1000}

	if err := PersistAdmin(context.Background(), store, logger, u, "first-run"); err != nil {
		t.Fatalf("PersistAdmin: %v", err)
	}
	if len(logger.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(logger.events))
	}
	e := logger.events[0]
	if e.Action != audit.ActionAdminSet {
		t.Errorf("action: got %q, want %q", e.Action, audit.ActionAdminSet)
	}
	if e.Target != "opadmin" {
		t.Errorf("target: got %q, want %q", e.Target, "opadmin")
	}
	if store.admin == nil {
		t.Fatal("admin not persisted in store")
	}
	if store.admin.SetBy != "first-run" {
		t.Errorf("SetBy: got %q, want %q", store.admin.SetBy, "first-run")
	}
}

func TestVerifyAdminReturnsErrOnMismatch(t *testing.T) {
	store := &fakeStore{admin: &state.Admin{
		Username: "opadmin",
		UID:      1000,
		SetAt:    time.Now().UTC(),
		SetBy:    "first-run",
	}}
	current := &User{Username: "alice", UID: 1001}
	err := VerifyAdmin(context.Background(), store, current)
	if err == nil {
		t.Fatal("expected ErrNotAdmin, got nil")
	}
	if _, ok := err.(ErrNotAdmin); !ok {
		t.Errorf("expected ErrNotAdmin, got %T: %v", err, err)
	}
}

func TestErrNotAdminError(t *testing.T) {
	e := ErrNotAdmin{Got: "alice", Expected: "opadmin"}
	if e.Error() == "" {
		t.Error("expected non-empty error string")
	}
}

func TestVerifyAdminNoAdminStored(t *testing.T) {
	// fakeStore returns nil admin — VerifyAdmin should propagate the error.
	store := &fakeStore{}
	current := &User{Username: "opadmin", UID: 1000}
	err := VerifyAdmin(context.Background(), store, current)
	if err == nil {
		t.Fatal("expected error when GetAdmin returns error")
	}
}

func TestVerifyAdminMatches(t *testing.T) {
	store := &fakeStore{admin: &state.Admin{
		Username: "opadmin",
		UID:      1000,
		SetAt:    time.Now().UTC(),
		SetBy:    "first-run",
	}}
	current := &User{Username: "opadmin", UID: 1000}
	if err := VerifyAdmin(context.Background(), store, current); err != nil {
		t.Errorf("expected nil error for matching admin, got %v", err)
	}
}

func TestResolveCandidateFallsBackToCurrentUser(t *testing.T) {
	t.Setenv("SUDO_USER", "")
	u, err := ResolveCandidate()
	if err != nil {
		t.Fatalf("ResolveCandidate: %v", err)
	}
	cur, _ := ResolveCurrent()
	if u.Username != cur.Username {
		t.Errorf("expected fallback to current user %q, got %q", cur.Username, u.Username)
	}
}

func TestResolveCandidateUsesValidSudoUser(t *testing.T) {
	cur, err := ResolveCurrent()
	if err != nil {
		t.Skip("cannot determine current user")
	}
	if cur.Username == "root" {
		t.Skip("skipping: cannot set SUDO_USER=root with this test")
	}
	t.Setenv("SUDO_USER", cur.Username)
	u, err := ResolveCandidate()
	if err != nil {
		t.Fatalf("ResolveCandidate with SUDO_USER=%q: %v", cur.Username, err)
	}
	if u.Username != cur.Username {
		t.Errorf("got %q, want %q", u.Username, cur.Username)
	}
}

func TestWarnIfRootWhenRoot(t *testing.T) {
	if !IsRoot() {
		t.Skip("test only runs as root")
	}
	if w := WarnIfRoot(); w == "" {
		t.Error("expected non-empty warning for root user")
	}
}
