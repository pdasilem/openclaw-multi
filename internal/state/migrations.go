package state

import (
	"context"
	"database/sql"
	"fmt"
)

// schemaDDL is kept in sync with schemas/state.sql (the canonical file used
// for external tools like sqlite3 validation). Inlined here because Go embed
// cannot traverse above the package directory.
const schemaDDL = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS migrations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    applied_at  DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    description TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS admin (
    username TEXT PRIMARY KEY NOT NULL,
    uid      INTEGER NOT NULL,
    set_at   DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    set_by   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
    username    TEXT PRIMARY KEY NOT NULL,
    uid         INTEGER,
    port        INTEGER UNIQUE,
    status      TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'paused')),
    linger      INTEGER NOT NULL DEFAULT 0 CHECK(linger IN (0, 1)),
    gateway_url TEXT,
    created_at  DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at  DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);

CREATE TABLE IF NOT EXISTS routes (
    id                  TEXT PRIMARY KEY NOT NULL,
    username            TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    kind                TEXT NOT NULL CHECK(kind IN ('gateway', 'plugin')),
    plugin_id           TEXT,
    local_port          INTEGER NOT NULL,
    hostname            TEXT NOT NULL UNIQUE,
    enabled             INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    created_at          DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    last_seen_cached_at DATETIME,
    last_seen_value     DATETIME
);
CREATE INDEX IF NOT EXISTS idx_routes_username ON routes(username);
CREATE INDEX IF NOT EXISTS idx_routes_enabled  ON routes(enabled);
CREATE INDEX IF NOT EXISTS idx_routes_hostname ON routes(hostname);

CREATE TABLE IF NOT EXISTS port_pool (
    username    TEXT PRIMARY KEY NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    range_start INTEGER NOT NULL,
    range_end   INTEGER NOT NULL,
    next_alloc  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS backups (
    id               TEXT PRIMARY KEY NOT NULL,
    username         TEXT NOT NULL,
    ts               DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    size_bytes       INTEGER NOT NULL DEFAULT 0,
    sha256           TEXT NOT NULL DEFAULT '',
    openclaw_version TEXT NOT NULL DEFAULT '',
    encrypted        INTEGER NOT NULL DEFAULT 0 CHECK(encrypted IN (0, 1)),
    path             TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_backups_username ON backups(username);
CREATE INDEX IF NOT EXISTS idx_backups_ts       ON backups(ts);

INSERT OR IGNORE INTO meta(key, value) VALUES ('schema_version', '1');
`

// migrate applies the schema DDL idempotently. The DDL uses
// CREATE TABLE IF NOT EXISTS throughout, so re-running is safe.
// It records a single migration entry if none exists yet.
func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, schemaDDL); err != nil {
		return fmt.Errorf("apply schema DDL: %w", err)
	}

	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM migrations`,
	).Scan(&count); err != nil {
		return fmt.Errorf("count migrations: %w", err)
	}
	if count == 0 {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO migrations(description) VALUES (?)`,
			"initial schema v1",
		); err != nil {
			return fmt.Errorf("record initial migration: %w", err)
		}
	}
	return nil
}
