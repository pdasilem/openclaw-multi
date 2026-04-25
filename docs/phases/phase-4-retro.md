# Phase 4 Retro: Doctor and Health Check

Date: 2026-04-25

## Implemented

Phase 4 replaced TUI menu item 4 with a real Health Check / Doctor screen.
`internal/doctor.Checker` now produces structured results for system, service,
per-user runtime, filesystem permission, network listener, and OpenClaw doctor
checks.

The checker uses only injected `shell.Executor`, `shell.FS`, and `state.Store`.
Tests do not run real `systemctl`, `loginctl`, `ss`, `df`, `chmod`, `su`, or
`openclaw doctor`.

Auto-fix is intentionally narrow. The implementation only allows approved
permission/profile fixes:

- `chmod-openclaw-dir`
- `chmod-openclaw-config`
- `chmod-overlay-dir`
- `repair-umask-profile`

Audit actions were added for `doctor_run` and `doctor_fix`.

## Verification

- `go test ./...` passed.
- `make ci` passed.
- `internal/doctor` coverage reached 84.9%.
- `golangci-lint run ./...` reported 0 issues.

## Deviations

- The implementation kept parser helpers in `parser.go` and the orchestration
  in `checker.go`; no additional files were needed.
- Service checks are intentionally shallow and read-only. `openclaw-overlay-api`
  can be reported as `skipped` while the daemon is still assigned to Phase 6.
- The `openclaw doctor --json` parser is implemented, but real CLI output still
  needs VPS validation. Text fallback is present for non-JSON output.

## Carry Forward

- Phase 5 should own Tailscale, Cloudflare, UFW, DNS, and connectivity
  management.
- Phase 8 should own live-tail logs and richer log parsing UI.
- Phase 9 should own the dedicated security audit screen.
- Phase 10 should own diagnostic snapshot archives and include master-key
  handling guidance from Phase 3.
