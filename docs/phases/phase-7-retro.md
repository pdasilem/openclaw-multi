# Phase 7 Retro: Per-User Overlay Watcher

Date: 2026-04-29

## Delivered

Phase 7 replaced the `openclaw-overlay-watcher` stub with a real per-user
daemon. The watcher resolves user config paths, performs an initial sync,
watches the OpenClaw config directory with fsnotify, debounces events, and
rescans the full config.

Implemented:

- `internal/watcher` parser for callback-capable plugin entries.
- Snapshot persistence at `~/.openclaw-overlay/watcher.state`.
- Sync orchestration from OpenClaw config to overlay-API plugin routes.
- OpenClaw config writes through injected `openclaw config set` commands.
- fsnotify-backed directory watcher and testable event loop.
- overlay-API client methods for plugin route upsert/delete.
- Real `cmd/openclaw-overlay-watcher` flags/startup/shutdown handling.
- Updated watcher systemd template.
- `docs/watcher.md`, Phase 7 E2E use cases, and changelog v0.7.0.

Phase decisions:

- Watch the config directory, not only `openclaw.json`.
- Treat file events as rescan triggers after debounce.
- Watcher does not own route IDs.
- Removed plugin entries delete plugin callback routes.
- Validation remains owner-run E2E, not phase smoke scripts.

## Verification

Passed:

```text
GOCACHE=/tmp/codex-go-cache GOTMPDIR=/tmp/codex-go-tmp go test -coverprofile=/tmp/watcher.cover ./internal/watcher
GOCACHE=/tmp/codex-go-cache GOTMPDIR=/tmp/codex-go-tmp GOLANGCI_LINT_CACHE=/tmp/codex-golangci-cache make ci
```

`internal/watcher` statement coverage: 85.1%.

`make ci` passed with `golangci-lint run ./...` reporting `0 issues`. Go still
prints non-fatal stat-cache warnings in the Codex sandbox because the module
cache under `/home/pdasilem/go/pkg/mod` is read-only.

## Deviations

- Phase 7 uses an explicit supported callback config contract documented in
  `docs/watcher.md`. If real OpenClaw plugin config differs on the VPS, update
  the contract before extending watcher heuristics.
- Removed plugin entries delete plugin callback routes. Disable behavior remains
  reserved for user lifecycle routes.
- No smoke scripts or `make test-phase-7` target were added.

## Follow-Up

- Run `UC-0701`..`UC-0704` on the VPS after deployment.
- Confirm real plugin config shape matches the documented callback contract.
- If plugin metadata exposes a stronger callback declaration later, update
  `internal/watcher` discovery and `docs/watcher.md` together.
- Phase 8 should own live watcher/overlay logs in the TUI.
