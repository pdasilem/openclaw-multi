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

func TestSetAndGetMeta(t *testing.T) {
	s := openTemp(t)
	if err := s.SetMeta(context.Background(), "install_completed", "2026-04-25T00:00:00Z"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	val, err := s.GetMeta(context.Background(), "install_completed")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if val != "2026-04-25T00:00:00Z" {
		t.Errorf("GetMeta: got %q", val)
	}
}

func TestGetMetaMissing(t *testing.T) {
	s := openTemp(t)
	val, err := s.GetMeta(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetMeta missing: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string, got %q", val)
	}
}

func TestSetMetaOverwrites(t *testing.T) {
	s := openTemp(t)
	if err := s.SetMeta(context.Background(), "k", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta(context.Background(), "k", "v2"); err != nil {
		t.Fatal(err)
	}
	val, _ := s.GetMeta(context.Background(), "k")
	if val != "v2" {
		t.Errorf("expected v2, got %q", val)
	}
}

func TestUserCRUD(t *testing.T) {
	ctx := context.Background()
	s := openTemp(t)

	u := User{
		Username:   "alice",
		UID:        1001,
		Port:       18789,
		Status:     UserStatusActive,
		Linger:     true,
		GatewayURL: "https://gateway-alice.openclaw.example.com",
	}
	if err := s.UpsertUser(ctx, u); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	got, err := s.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.Username != u.Username || got.Port != u.Port || got.Status != UserStatusActive || !got.Linger {
		t.Fatalf("unexpected user: %+v", got)
	}

	exists, err := s.UserExists(ctx, "alice")
	if err != nil {
		t.Fatalf("UserExists: %v", err)
	}
	if !exists {
		t.Fatal("expected alice to exist")
	}

	if err := s.SetUserStatus(ctx, "alice", UserStatusPaused); err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	got, err = s.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUser after status: %v", err)
	}
	if got.Status != UserStatusPaused {
		t.Fatalf("status: got %q", got.Status)
	}

	if err := s.DeleteUser(ctx, "alice"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	_, err = s.GetUser(ctx, "alice")
	if !errors.Is(err, ErrNoUser) {
		t.Fatalf("expected ErrNoUser after delete, got %v", err)
	}
}

func TestListUsersSorted(t *testing.T) {
	ctx := context.Background()
	s := openTemp(t)
	for _, u := range []User{
		{Username: "carol", Port: 18829, Status: UserStatusActive},
		{Username: "alice", Port: 18789, Status: UserStatusActive},
		{Username: "bob", Port: 18809, Status: UserStatusPaused},
	} {
		if err := s.UpsertUser(ctx, u); err != nil {
			t.Fatalf("UpsertUser %s: %v", u.Username, err)
		}
	}
	users, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if got := []string{users[0].Username, users[1].Username, users[2].Username}; got[0] != "alice" || got[1] != "bob" || got[2] != "carol" {
		t.Fatalf("unexpected order: %v", got)
	}
}

func TestRouteCRUDAndCascade(t *testing.T) {
	ctx := context.Background()
	s := openTemp(t)
	if err := s.UpsertUser(ctx, User{Username: "alice", Port: 18789, Status: UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	route := Route{
		ID:        "gateway:alice",
		Username:  "alice",
		Kind:      RouteKindGateway,
		LocalPort: 18789,
		Hostname:  "gateway-alice.openclaw.example.com",
		Enabled:   true,
	}
	if err := s.UpsertRoute(ctx, route); err != nil {
		t.Fatalf("UpsertRoute: %v", err)
	}

	routes, err := s.ListRoutesByUser(ctx, "alice")
	if err != nil {
		t.Fatalf("ListRoutesByUser: %v", err)
	}
	if len(routes) != 1 || routes[0].Hostname != route.Hostname || !routes[0].Enabled {
		t.Fatalf("unexpected routes: %+v", routes)
	}

	if err := s.SetRoutesEnabled(ctx, "alice", false); err != nil {
		t.Fatalf("SetRoutesEnabled: %v", err)
	}
	routes, err = s.ListRoutesByUser(ctx, "alice")
	if err != nil {
		t.Fatalf("ListRoutesByUser after disable: %v", err)
	}
	if routes[0].Enabled {
		t.Fatal("expected route disabled")
	}

	if err := s.DeleteUser(ctx, "alice"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	routes, err = s.ListRoutesByUser(ctx, "alice")
	if err != nil {
		t.Fatalf("ListRoutesByUser after cascade: %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("expected cascade route delete, got %+v", routes)
	}
}

func TestBackupCRUDAndSurvivesUserDelete(t *testing.T) {
	ctx := context.Background()
	s := openTemp(t)
	if err := s.UpsertUser(ctx, User{Username: "alice", Port: 18789, Status: UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	first := Backup{
		ID:              "alice-1",
		Username:        "alice",
		TS:              time.Date(2026, 4, 25, 1, 0, 0, 0, time.UTC),
		SizeBytes:       10,
		SHA256:          "aaa",
		OpenClawVersion: "1.0.0",
		Encrypted:       true,
		Path:            "/var/lib/openclaw-multi/backups/alice/1.tar.gz.enc",
	}
	second := Backup{
		ID:        "alice-2",
		Username:  "alice",
		TS:        time.Date(2026, 4, 25, 2, 0, 0, 0, time.UTC),
		SizeBytes: 20,
		SHA256:    "bbb",
		Encrypted: true,
		Path:      "/var/lib/openclaw-multi/backups/alice/2.tar.gz.enc",
	}
	if err := s.UpsertBackup(ctx, first); err != nil {
		t.Fatalf("UpsertBackup first: %v", err)
	}
	if err := s.UpsertBackup(ctx, second); err != nil {
		t.Fatalf("UpsertBackup second: %v", err)
	}

	backups, err := s.ListBackupsByUser(ctx, "alice")
	if err != nil {
		t.Fatalf("ListBackupsByUser: %v", err)
	}
	if len(backups) != 2 || backups[0].ID != "alice-2" || backups[1].ID != "alice-1" {
		t.Fatalf("unexpected backup order: %+v", backups)
	}

	got, err := s.GetBackup(ctx, "alice-1")
	if err != nil {
		t.Fatalf("GetBackup: %v", err)
	}
	if got.SHA256 != "aaa" || !got.Encrypted {
		t.Fatalf("unexpected backup: %+v", got)
	}

	if err := s.DeleteUser(ctx, "alice"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	backups, err = s.ListBackupsByUser(ctx, "alice")
	if err != nil {
		t.Fatalf("ListBackupsByUser after user delete: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected backups to survive user delete, got %+v", backups)
	}

	if err := s.DeleteBackup(ctx, "alice-1"); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}
	_, err = s.GetBackup(ctx, "alice-1")
	if !errors.Is(err, ErrNoBackup) {
		t.Fatalf("expected ErrNoBackup, got %v", err)
	}
}
