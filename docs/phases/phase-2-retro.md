# Phase 2 Retrospective

## What we built

Phase 2 implemented managed user lifecycle support without live VPS execution.
`internal/state` now exposes user and route models plus CRUD/status helpers.
`internal/users` provides add, activate, deactivate, and remove orchestration
through injectable `shell.Executor`, `shell.FS`, state store, route publisher,
and audit logger.

TUI menu item 3 now opens a real user-management screen instead of a placeholder.
The screen supports list, add, activate/deactivate, and remove flows with Bubble
Tea v2 model tests.

## Decisions Applied

- Deleted users immediately free their previous gateway port. The allocator
  reuses the lowest free valid port.
- Gateway tokens are generated automatically in Phase 2. Manual token entry is
  not part of this phase.
- `domain` and `subdomain` are prerequisites for Add User. Missing route config
  blocks the operation before any state, route, filesystem, or command mutation.
- Removing an active user follows a deactivate-first flow before hard delete.
- Docker is not used as a VPS simulator. Final system validation remains an
  owner-run real VPS check.

## Deviations from Plan

- The route publisher is state-backed only. It records intended gateway routes
  and leaves real overlay-API/cloudflared mutation for Phase 6.
- Backup before remove is still absent by design and remains Phase 3.
- User-management tests are unit/model-level only. No live Linux users are
  created.

## Verification

- `go test ./...` passes.
- `make test` passes with race detector; `internal/users` coverage is 84.8%.
- `make build-amd64` and `make build-arm64` pass.
- `go vet ./...` passes.
- `make ci` passes after installing `jq` and `golangci-lint`. `shellcheck` is
  optional in the current Makefile and is skipped when absent.

## Carry Forward

- Phase 3 should add backup/restore before destructive remove.
- Phase 6 should replace the state-backed route publisher with a real
  overlay-API/cloudflared implementation.
- Real VPS validation should verify command ordering for `useradd`, `loginctl`,
  `systemctl --user`, and OpenClaw onboarding.
