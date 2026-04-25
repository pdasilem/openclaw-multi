-- SQLite schema for openclaw-multi overlay state.
-- Run idempotently: all tables use CREATE TABLE IF NOT EXISTS.
-- Migration tracking via the migrations table.

PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- Key/value metadata, including schema_version.
CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL
);

-- Track applied schema migrations.
CREATE TABLE IF NOT EXISTS migrations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    applied_at  DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    description TEXT NOT NULL
);

-- The overlay administrator identity (exactly one row after first-run).
CREATE TABLE IF NOT EXISTS admin (
    username TEXT PRIMARY KEY NOT NULL,
    uid      INTEGER NOT NULL,
    set_at   DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    set_by   TEXT NOT NULL
);

-- Registered openclaw users managed by the overlay.
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

-- Cloudflare Tunnel ingress routes (gateway + plugin callbacks).
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
CREATE INDEX IF NOT EXISTS idx_routes_username  ON routes(username);
CREATE INDEX IF NOT EXISTS idx_routes_enabled   ON routes(enabled);
CREATE INDEX IF NOT EXISTS idx_routes_hostname  ON routes(hostname);

-- Per-user port allocation pools.
CREATE TABLE IF NOT EXISTS port_pool (
    username    TEXT PRIMARY KEY NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    range_start INTEGER NOT NULL,
    range_end   INTEGER NOT NULL,
    next_alloc  INTEGER NOT NULL
);

-- Backup records.
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

-- Seed initial schema version.
INSERT OR IGNORE INTO meta(key, value) VALUES ('schema_version', '1');
