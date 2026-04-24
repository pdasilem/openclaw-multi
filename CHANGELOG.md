# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

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
