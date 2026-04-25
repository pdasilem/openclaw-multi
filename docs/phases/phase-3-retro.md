# Phase 3 Retro: Backup and Restore

Date: 2026-04-25

## Implemented

Phase 3 added encrypted per-user backup and restore orchestration without live
VPS execution. `internal/backup.Manager` now creates backups, restores existing
managed users, installs intended systemd timer units, and emits backup audit
events through injected dependencies.

The state layer now has first-class backup metadata CRUD. Backup records are no
longer tied to `users` with `ON DELETE CASCADE`, so metadata survives user
deletion as required.

User removal is backup-first. `users.Manager.Remove` accepts a pre-remove hook
and aborts before route deletion, OpenClaw uninstall, linger disable, `userdel`,
state deletion, or port reuse if backup creation fails.

The TUI user-management screen now exposes:

- `b` to create a backup for the selected user;
- `r` to list and restore backups for the selected user.

The `openclaw-multi backup <username>` entrypoint was added so generated timer
units call a real non-interactive backup command instead of a TUI-only binary.

## Verification

- `go test ./...` passed.
- `make ci` passed.
- `internal/backup` coverage reached 88.8%.
- `golangci-lint run ./...` reported 0 issues.

## Deviations

- The implementation kept `CreateRequest`, `RestoreRequest`, and `Options` in
  `manager.go` instead of splitting into separate `models.go` / `crypto.go`
  files. The package is still small enough that the split would add ceremony
  without improving clarity.
- Timer units are installed as system template units under
  `/etc/systemd/system`, so the installer uses system-level `systemctl
  daemon-reload` and `systemctl enable --now openclaw-backup@<user>.timer`.
- `openclaw_version` is currently stored as an empty string because Phase 3
  does not parse OpenClaw manifest metadata yet.

## Carry Forward

- Real VPS validation must verify actual `openclaw backup create` stdout and
  archive layout.
- Restore-to-missing-user remains deferred.
- Retention policy, remote/offsite backups, and master-key rotation remain out
  of scope.
- Diagnostic snapshot work should include `/etc/openclaw-multi/master.key`
  handling guidance because losing that key makes encrypted backups unusable.
