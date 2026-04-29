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

// ErrNoUser is returned when a managed user does not exist.
var ErrNoUser = fmt.Errorf("no user configured")

// ErrNoRoute is returned when a route does not exist.
var ErrNoRoute = fmt.Errorf("no route configured")

// ErrNoBackup is returned when a backup record does not exist.
var ErrNoBackup = fmt.Errorf("no backup configured")

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

// ListUsers returns all managed users sorted by username.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT username, COALESCE(uid, 0), COALESCE(port, 0), status, linger,
		       COALESCE(gateway_url, ''), created_at, updated_at
		FROM users
		ORDER BY username`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list users rows: %w", err)
	}
	return users, nil
}

// GetUser returns a managed user by username.
func (s *Store) GetUser(ctx context.Context, username string) (*User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT username, COALESCE(uid, 0), COALESCE(port, 0), status, linger,
		       COALESCE(gateway_url, ''), created_at, updated_at
		FROM users
		WHERE username = ?`, username)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoUser
	}
	if err != nil {
		return nil, fmt.Errorf("get user %q: %w", username, err)
	}
	return &u, nil
}

// UserExists returns true when username is present in state.
func (s *Store) UserExists(ctx context.Context, username string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM users WHERE username = ?`, username).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("user exists %q: %w", username, err)
	}
	return true, nil
}

// UpsertUser inserts or updates a managed user.
func (s *Store) UpsertUser(ctx context.Context, u User) error {
	now := time.Now().UTC().Format(time.RFC3339)
	createdAt := formatOrNow(u.CreatedAt, now)
	updatedAt := formatOrNow(u.UpdatedAt, now)
	if u.Status == "" {
		u.Status = UserStatusActive
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users(username, uid, port, status, linger, gateway_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET
			uid = excluded.uid,
			port = excluded.port,
			status = excluded.status,
			linger = excluded.linger,
			gateway_url = excluded.gateway_url,
			updated_at = ?`,
		u.Username, nullableInt(u.UID), nullableInt(u.Port), string(u.Status), boolInt(u.Linger),
		nullableString(u.GatewayURL), createdAt, updatedAt, now)
	if err != nil {
		return fmt.Errorf("upsert user %q: %w", u.Username, err)
	}
	return nil
}

// SetUserStatus updates a managed user's status.
func (s *Store) SetUserStatus(ctx context.Context, username string, status UserStatus) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET status = ?, updated_at = ? WHERE username = ?`,
		string(status), time.Now().UTC().Format(time.RFC3339), username)
	if err != nil {
		return fmt.Errorf("set user status %q: %w", username, err)
	}
	return requireAffected(res, ErrNoUser)
}

// DeleteUser removes a managed user. Routes and ports cascade; backups remain.
func (s *Store) DeleteUser(ctx context.Context, username string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete user %q: %w", username, err)
	}
	return requireAffected(res, ErrNoUser)
}

// ListRoutesByUser returns routes for username sorted by kind, plugin_id, hostname.
func (s *Store) ListRoutesByUser(ctx context.Context, username string) ([]Route, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, kind, COALESCE(plugin_id, ''), local_port, hostname,
		       enabled, created_at, COALESCE(last_seen_cached_at, ''), COALESCE(last_seen_value, '')
		FROM routes
		WHERE username = ?
		ORDER BY kind, plugin_id, hostname`, username)
	if err != nil {
		return nil, fmt.Errorf("list routes for %q: %w", username, err)
	}
	defer rows.Close() //nolint:errcheck

	var routes []Route
	for rows.Next() {
		r, err := scanRoute(rows)
		if err != nil {
			return nil, err
		}
		routes = append(routes, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list routes rows for %q: %w", username, err)
	}
	return routes, nil
}

// ListRoutes returns all routes sorted by username, kind, plugin_id, hostname.
func (s *Store) ListRoutes(ctx context.Context) ([]Route, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, kind, COALESCE(plugin_id, ''), local_port, hostname,
		       enabled, created_at, COALESCE(last_seen_cached_at, ''), COALESCE(last_seen_value, '')
		FROM routes
		ORDER BY username, kind, plugin_id, hostname`)
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var routes []Route
	for rows.Next() {
		r, err := scanRoute(rows)
		if err != nil {
			return nil, err
		}
		routes = append(routes, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list routes rows: %w", err)
	}
	return routes, nil
}

// UpsertRoute inserts or updates a route.
func (s *Store) UpsertRoute(ctx context.Context, r Route) error {
	now := time.Now().UTC().Format(time.RFC3339)
	createdAt := formatOrNow(r.CreatedAt, now)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO routes(id, username, kind, plugin_id, local_port, hostname, enabled, created_at,
		                   last_seen_cached_at, last_seen_value)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			username = excluded.username,
			kind = excluded.kind,
			plugin_id = excluded.plugin_id,
			local_port = excluded.local_port,
			hostname = excluded.hostname,
			enabled = excluded.enabled,
			last_seen_cached_at = excluded.last_seen_cached_at,
			last_seen_value = excluded.last_seen_value`,
		r.ID, r.Username, string(r.Kind), nullableString(r.PluginID), r.LocalPort, r.Hostname,
		boolInt(r.Enabled), createdAt, nullableTime(r.LastSeenCachedAt), nullableTime(r.LastSeenValue))
	if err != nil {
		return fmt.Errorf("upsert route %q for %q: %w", r.ID, r.Username, err)
	}
	return nil
}

// DeleteRoute removes a route by ID.
func (s *Store) DeleteRoute(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM routes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete route %q: %w", id, err)
	}
	return requireAffected(res, ErrNoRoute)
}

// DeleteRoutesByUser removes all routes for username.
func (s *Store) DeleteRoutesByUser(ctx context.Context, username string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM routes WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete routes for %q: %w", username, err)
	}
	return nil
}

// SetRoutesEnabled updates enabled state for all routes owned by username.
func (s *Store) SetRoutesEnabled(ctx context.Context, username string, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE routes SET enabled = ? WHERE username = ?`, boolInt(enabled), username)
	if err != nil {
		return fmt.Errorf("set routes enabled for %q: %w", username, err)
	}
	return nil
}

// SetRouteLastSeen updates cached last-seen metadata for a route.
func (s *Store) SetRouteLastSeen(ctx context.Context, id string, seenAt time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE routes SET last_seen_cached_at = ?, last_seen_value = ? WHERE id = ?`,
		now, seenAt.UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("set route last seen %q: %w", id, err)
	}
	return requireAffected(res, ErrNoRoute)
}

// ListBackupsByUser returns backup records for username, newest first.
func (s *Store) ListBackupsByUser(ctx context.Context, username string) ([]Backup, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, ts, size_bytes, sha256, openclaw_version, encrypted, path
		FROM backups
		WHERE username = ?
		ORDER BY ts DESC, id DESC`, username)
	if err != nil {
		return nil, fmt.Errorf("list backups for %q: %w", username, err)
	}
	defer rows.Close() //nolint:errcheck

	var backups []Backup
	for rows.Next() {
		b, err := scanBackup(rows)
		if err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list backups rows for %q: %w", username, err)
	}
	return backups, nil
}

// GetBackup returns a backup by ID.
func (s *Store) GetBackup(ctx context.Context, id string) (*Backup, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, ts, size_bytes, sha256, openclaw_version, encrypted, path
		FROM backups
		WHERE id = ?`, id)
	b, err := scanBackup(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoBackup
	}
	if err != nil {
		return nil, fmt.Errorf("get backup %q: %w", id, err)
	}
	return &b, nil
}

// UpsertBackup inserts or updates backup metadata.
func (s *Store) UpsertBackup(ctx context.Context, b Backup) error {
	ts := formatOrNow(b.TS, time.Now().UTC().Format(time.RFC3339))
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO backups(id, username, ts, size_bytes, sha256, openclaw_version, encrypted, path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			username = excluded.username,
			ts = excluded.ts,
			size_bytes = excluded.size_bytes,
			sha256 = excluded.sha256,
			openclaw_version = excluded.openclaw_version,
			encrypted = excluded.encrypted,
			path = excluded.path`,
		b.ID, b.Username, ts, b.SizeBytes, b.SHA256, b.OpenClawVersion, boolInt(b.Encrypted), b.Path)
	if err != nil {
		return fmt.Errorf("upsert backup %q for %q: %w", b.ID, b.Username, err)
	}
	return nil
}

// DeleteBackup removes backup metadata by ID.
func (s *Store) DeleteBackup(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete backup %q: %w", id, err)
	}
	return requireAffected(res, ErrNoBackup)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (User, error) {
	var u User
	var createdAt, updatedAt string
	var linger int
	if err := row.Scan(&u.Username, &u.UID, &u.Port, &u.Status, &linger, &u.GatewayURL, &createdAt, &updatedAt); err != nil {
		return User{}, err
	}
	u.Linger = linger == 1
	u.CreatedAt = parseDBTime(createdAt)
	u.UpdatedAt = parseDBTime(updatedAt)
	return u, nil
}

func scanRoute(row scanner) (Route, error) {
	var r Route
	var createdAt, cachedAt, seenAt string
	var enabled int
	if err := row.Scan(&r.ID, &r.Username, &r.Kind, &r.PluginID, &r.LocalPort, &r.Hostname,
		&enabled, &createdAt, &cachedAt, &seenAt); err != nil {
		return Route{}, err
	}
	r.Enabled = enabled == 1
	r.CreatedAt = parseDBTime(createdAt)
	r.LastSeenCachedAt = parseDBTime(cachedAt)
	r.LastSeenValue = parseDBTime(seenAt)
	return r, nil
}

func scanBackup(row scanner) (Backup, error) {
	var b Backup
	var ts string
	var encrypted int
	if err := row.Scan(&b.ID, &b.Username, &ts, &b.SizeBytes, &b.SHA256, &b.OpenClawVersion, &encrypted, &b.Path); err != nil {
		return Backup{}, err
	}
	b.TS = parseDBTime(ts)
	b.Encrypted = encrypted == 1
	return b, nil
}

func requireAffected(res sql.Result, notFound error) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return notFound
	}
	return nil
}

func parseDBTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func formatOrNow(t time.Time, defaultValue string) string {
	if t.IsZero() {
		return defaultValue
	}
	return t.UTC().Format(time.RFC3339)
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
