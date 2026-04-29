# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## v0.7.0 - 2026-04-29

### Added
- `openclaw-overlay-watcher` per-user daemon with config parsing, fsnotify
  directory watching, debounce/full rescan, snapshot persistence, and OpenClaw
  callback config writes.
- `internal/watcher` with callback discovery, watcher state, sync
  orchestration, fsnotify adapter, and unit tests.
- overlay-API client plugin route upsert/delete methods.
- `docs/watcher.md` and Phase 7 owner-run E2E cases.

## v0.6.0 - 2026-04-28

### Added
- `openclaw-overlay-api` daemon on a UNIX socket with health, route, enable,
  disable, last-seen, and cloudflared reload endpoints.
- `internal/api` with JSON contracts, peer credential authorization, UNIX socket
  listener, route service, and client implementing the route publisher shape.
- `internal/cloudflared` with deterministic local tunnel config rendering,
  credentials-file resolution, validation, SIGHUP, backup, and rollback.
- Config field `cloudflared_credentials_file`.
- Audit actions `overlay_api_request`, `cloudflared_config_publish`, and
  `cloudflared_sighup`.
- `docs/overlay-api.md` and Phase 6 owner-run E2E cases.

## v0.5.0 - 2026-04-25

### Added
- `internal/network`: structured network snapshots for Tailscale, Cloudflare
  Tunnel, UFW, route inventory, open ports, gateway probes, wildcard DNS plans,
  and cloudflared last_seen parsing.
- TUI menu item 5 now opens a real Network and firewall screen.
- Cloudflare DNS client boundary with list/create/update support and explicit
  wildcard DNS review/apply flow.
- UFW review/apply flow for enabled route ports using the existing conservative
  `deps.EnsureUFW` helper.
- Audit actions `network_refresh`, `network_probe`, `dns_update`, and
  `ufw_update`.
- Config fields `cloudflare_zone_id` and `cloudflare_api_token`.
- `state.ListRoutes` and `state.SetRouteLastSeen`.
- `docs/network.md` and `docs/phases/phase-5-retro.md`.

### Changed
- Main TUI menu and user-management headings now use English-only labels.

## v0.4.0 - 2026-04-25

### Added
- `internal/doctor`: structured health checks for system, services, managed
  users, filesystem permissions, gateway port listeners, and OpenClaw doctor
  output.
- TUI menu item 4 now opens a real Health Check / Doctor screen.
- Narrow auto-fix support for approved permission/profile drift only.
- Audit actions `doctor_run` and `doctor_fix`.
- `docs/health.md` and `docs/phases/phase-4-retro.md`.

### Changed
- `shell.MemFS` test double now supports configurable file modes for
  permission-check tests.

## v0.3.0 - 2026-04-25

### Added
- `internal/backup`: encrypted backup creation, restore for existing managed
  users, auto-backup timer installation, and backup audit events.
- Backup metadata CRUD in `internal/state`, including `ErrNoBackup`.
- TUI user-management backup and restore actions (`b` and `r`).
- Non-interactive `openclaw-multi backup <username>` command for timer-driven
  backups.
- `templates/openclaw-backup@.service.tmpl` and
  `templates/openclaw-backup@.timer.tmpl`.
- `docs/backups.md` and `docs/phases/phase-3-retro.md`.

### Changed
- Backup metadata now survives user deletion.
- User removal is backup-first and aborts before destructive steps if backup
  creation fails.
- `shell.FS` now includes `Remove` for deterministic cleanup of partial backup
  artifacts.

## v0.2.0 - 2026-04-25

### Added
- `internal/state`: managed user and route models plus CRUD/status helpers for
  `users` and `routes`.
- `internal/users`: user lifecycle manager for add, activate, deactivate, and
  remove operations using injectable `shell.Executor`, `shell.FS`, state store,
  route publisher, and audit logger.
- Deterministic gateway port allocation from `port_range_start` with
  `port_range_step`; deleted users immediately free their gateway port for
  reuse.
- State-backed `RoutePublisher` for Phase 2 gateway routes without live
  cloudflared mutation.
- TUI menu item 3 now opens a user-management screen with list, add,
  activate/deactivate, and remove flows.
- `templates/openclaw-overlay-watcher.service.tmpl` for per-user watcher
  systemd units.
- `docs/users.md` and `docs/phases/phase-2-retro.md`.

### Changed
- `Add User` now requires configured `domain` and `subdomain` before any state,
  route, or command mutation.
- Gateway tokens are generated automatically in Phase 2; manual token entry is
  out of scope.
- Removing an active user follows a deactivate-first flow before hard delete.

## v0.1.0 - 2026-04-25

### Added
- `internal/shell`: `Executor` interface + `RealExecutor` + `MockExecutor` + `FS` interface + `MemFS`.
  Single abstraction for all subprocess and filesystem calls — makes everything testable without a VPS.
- `internal/preflight`: distro, disk, RAM, tools, port-conflict checks. All via injectable interfaces.
- `internal/config`: `OverlayConfig` YAML load/save + envsubst template renderer.
- `internal/hardening`: sysctl, hidepid=2, profile.d writes — idempotent, FS-injectable.
- `internal/deps`: idempotent `EnsureNode`, `EnsureTailscale`, `EnsureCloudflared` (A+B),
  `EnsureUFW` — all tested with `MockExecutor`.
- `internal/tui/wizard`: generic Bubble Tea step-wizard (retry/skip/abort).
- `internal/tui/wizard/freshinstall`: 10-step fresh-install wizard wired to all `internal/` packages.
- TUI menu item 1 now launches the fresh-install wizard (was placeholder).
- `templates/`: sysctl, fstab, profile.d, cloudflared, systemd unit templates.
- `docs/install.md`: installation guide.
- `state.SetMeta` / `GetMeta` for key-value overlay state.

### Changed
- Refreshed Go dependencies to current stable module paths:
  `charm.land/bubbletea/v2 v2.0.6`, `charm.land/lipgloss/v2 v2.0.3`,
  and `modernc.org/sqlite v1.49.1`.
- Updated TUI code for Bubble Tea v2: `View()` now returns `tea.View`,
  alt-screen is configured on the returned view, and tests use the v2 key
  event API.

## v0.0.0 - 2026-04-25

### Added
- Initial project skeleton (phase 0).
- Three binary stubs: `openclaw-multi` (TUI), `openclaw-overlay-api` (daemon),
  `openclaw-overlay-watcher` (per-user daemon).
- Bubble Tea TUI with main menu (10 items), navigation, status bar, lipgloss
  styles, and "not implemented yet" placeholder screens.
- SQLite state store (`internal/state`) with idempotent migration runner.
  Admin identification and verification (`internal/admin`).
- JSONL audit log writer (`internal/audit`) with structured event types.
- First-run admin setup flow in the TUI.
- CI pipeline (lint + test + build × amd64/arm64 + schema-check + shellcheck).
- Docker-based smoke test (`make test-phase-0`).
- `schemas/state.sql` — full SQLite DDL for all overlay tables.
- Go module layout matching master plan §5 directory structure.
