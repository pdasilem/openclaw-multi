// Package admin handles administrator identification and verification.
package admin

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"time"

	"github.com/SergeiM/openclaw-multi/internal/audit"
	"github.com/SergeiM/openclaw-multi/internal/state"
)

// User represents a Linux user identity.
type User struct {
	Username string
	UID      int
}

// ErrNotAdmin is returned when the current user is not the configured admin.
type ErrNotAdmin struct {
	Got      string
	Expected string
}

func (e ErrNotAdmin) Error() string {
	return fmt.Sprintf("not admin: current user %q does not match configured admin %q", e.Got, e.Expected)
}

// ResolveCurrent returns the identity of the currently running process.
func ResolveCurrent() (*User, error) {
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("user.Current: %w", err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return nil, fmt.Errorf("parse uid %q: %w", u.Uid, err)
	}
	return &User{Username: u.Username, UID: uid}, nil
}

// ResolveCandidate returns the admin candidate: prefers $SUDO_USER if set and
// non-root, otherwise falls back to ResolveCurrent.
func ResolveCandidate() (*User, error) {
	if su := os.Getenv("SUDO_USER"); su != "" {
		if su == "root" {
			return nil, fmt.Errorf("SUDO_USER=root: running as root admin is an antipattern")
		}
		u, err := user.Lookup(su)
		if err != nil {
			return nil, fmt.Errorf("lookup SUDO_USER %q: %w", su, err)
		}
		uid, err := strconv.Atoi(u.Uid)
		if err != nil {
			return nil, fmt.Errorf("parse uid for SUDO_USER %q: %w", su, err)
		}
		return &User{Username: su, UID: uid}, nil
	}
	return ResolveCurrent()
}

// IsRoot reports whether the current process runs as root (UID 0).
func IsRoot() bool {
	return os.Getuid() == 0
}

// WarnIfRoot returns a warning message when running as root, or empty string.
func WarnIfRoot() string {
	if !IsRoot() {
		return ""
	}
	return `WARNING: You are running openclaw-multi as root.
Running as root is an antipattern and is not recommended.
Please create a regular user, add them to the sudo group, and run:
  sudo openclaw-multi`
}

// AdminStore is the minimal interface needed from *state.Store.
type AdminStore interface {
	GetAdmin(ctx context.Context) (*state.Admin, error)
	SetAdmin(ctx context.Context, a state.Admin) error
}

// AuditLogger is the minimal interface needed from *audit.Logger.
type AuditLogger interface {
	Emit(e audit.Event) error
}

// PersistAdmin saves u as the overlay admin in store and emits an audit event.
func PersistAdmin(ctx context.Context, store AdminStore, logger AuditLogger, u *User, setBy string) error {
	a := state.Admin{
		Username: u.Username,
		UID:      u.UID,
		SetAt:    time.Now().UTC(),
		SetBy:    setBy,
	}
	if err := store.SetAdmin(ctx, a); err != nil {
		return fmt.Errorf("persist admin: %w", err)
	}
	_ = logger.Emit(audit.Event{
		Actor:  u.Username,
		Action: audit.ActionAdminSet,
		Target: u.Username,
		Result: audit.ResultOk,
	})
	return nil
}

// VerifyAdmin returns nil if current's Username matches the stored admin.
func VerifyAdmin(ctx context.Context, store AdminStore, current *User) error {
	a, err := store.GetAdmin(ctx)
	if err != nil {
		return fmt.Errorf("read admin: %w", err)
	}
	if current.Username != a.Username {
		return ErrNotAdmin{Got: current.Username, Expected: a.Username}
	}
	return nil
}
