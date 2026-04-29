# Phase 6 Retro: Overlay-API Daemon

Date: 2026-04-29

## Delivered

Phase 6 replaced the `openclaw-overlay-api` stub with a working UNIX-socket HTTP
daemon. The daemon now loads overlay config and state, exposes health and
route-management endpoints, publishes cloudflared config, validates it, sends
SIGHUP, and records lifecycle/config audit events.

Implemented API surface:

- `GET /health`
- `GET /users`
- `POST /users/<username>/gateway-route`
- `DELETE /users/<username>/gateway-route`
- `POST /users/<username>/routes`
- `DELETE /users/<username>/routes/<route_id>`
- `GET /users/<username>/routes`
- `GET /users/<username>/routes/<route_id>/last-seen`
- `POST /users/<username>/disable`
- `POST /users/<username>/enable`
- `POST /cloudflared/reload`

Phase decisions were implemented without ambiguity:

- Cloudflared reload uses SIGHUP.
- Plugin route IDs are daemon-derived from
  `plugin:<username>:<plugin_id>:<hostname_hint>`.
- Managed users may connect to the world-writable socket, but authorization is
  enforced by `SO_PEERCRED`.
- Root and the configured admin UID may manage every user; a managed user UID
  may manage only that user's routes.
- `cloudflared_credentials_file` is explicit in config and defaults to
  `/etc/cloudflared/<tunnel_id>.json`.
- Config publication fails before writing when the credentials file is missing.

The TUI user lifecycle path now uses the overlay API client for gateway route
publication. The systemd template starts the daemon with explicit config, state,
socket, cloudflared config, and audit-log paths.

## Verification

Passed:

```text
go test ./...
GOCACHE=/tmp/codex-go-cache GOTMPDIR=/tmp/codex-go-tmp GOLANGCI_LINT_CACHE=/tmp/codex-golangci-cache make ci
```

`make ci` now runs shellcheck because shellcheck is installed on the Manjaro KDE
6 development host. The `/tmp` cache environment is required for Codex sandbox
runs; otherwise golangci-lint attempts to write under `/home/pdasilem/.cache`.

## Deviations

- Phase 6 does not include automated smoke scripts. Owner-run validation steps
  are documented only in `docs/e2e-use-cases.md`.
- Last-seen is exposed through the API from persisted state. Live cloudflared
  log tailing and richer runtime refresh remain outside this phase.
- Cloudflared SIGHUP behavior is covered by injected-command tests; it still
  needs confirmation on the real VPS service.
- Route state mutation and config publication are orchestrated in one service
  path, but full transactional rollback of state after a publication failure is
  not implemented.

## Follow-Up

- During VPS validation, run the E2E use cases for sign-up/gateway publication
  and explicitly confirm cloudflared accepts SIGHUP without a systemd restart.
- Add the real VPS outcome to `docs/e2e-use-cases.md`: keep SIGHUP as the
  operational reload path only after VPS evidence confirms it.
- Phase 7 watcher should call plugin route endpoints and rely on daemon-derived
  route IDs instead of sending client-owned route IDs.
- Phase 8 should own live log tailing and any richer last-seen refresh loop.
