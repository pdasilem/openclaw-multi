# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

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
