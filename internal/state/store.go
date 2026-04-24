package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // register sqlite driver
)

const defaultDBPath = "/var/lib/openclaw-multi/state.db"

// ErrNoAdmin is returned by GetAdmin when no admin has been configured yet.
var ErrNoAdmin = fmt.Errorf("no admin configured")

// Store is the SQLite-backed state store for the overlay.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens (or creates) the state database at path and applies migrations.
// If path is empty, defaultDBPath is used.
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		path = defaultDBPath
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// GetAdmin returns the stored admin record, or (nil, nil) if none is set yet.
func (s *Store) GetAdmin(ctx context.Context) (*Admin, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT username, uid, set_at, set_by FROM admin LIMIT 1`,
	)
	var a Admin
	var setAt string
	if err := row.Scan(&a.Username, &a.UID, &setAt, &a.SetBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoAdmin
		}
		return nil, fmt.Errorf("get admin: %w", err)
	}
	t, err := time.Parse(time.RFC3339, setAt)
	if err != nil {
		t = time.Time{}
	}
	a.SetAt = t
	return &a, nil
}

// SetMeta writes a key/value pair to the meta table.
func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO meta(key, value) VALUES (?, ?)`, key, value,
	)
	if err != nil {
		return fmt.Errorf("set meta %q: %w", key, err)
	}
	return nil
}

// GetMeta reads a value from the meta table. Returns ("", nil) if not found.
func (s *Store) GetMeta(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get meta %q: %w", key, err)
	}
	return value, nil
}

// SetAdmin persists an admin record, replacing any existing one.
// Only one admin row is ever stored.
func (s *Store) SetAdmin(ctx context.Context, a Admin) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, `DELETE FROM admin`); err != nil {
		return fmt.Errorf("clear admin: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO admin(username, uid, set_at, set_by) VALUES (?,?,?,?)`,
		a.Username, a.UID, a.SetAt.UTC().Format(time.RFC3339), a.SetBy,
	); err != nil {
		return fmt.Errorf("insert admin: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit admin: %w", err)
	}
	return nil
}
