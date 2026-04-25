---
phase: 3
title: "Backup and restore"
slug: backup-restore
estimated_duration: "1 week"
status: implemented
approved_at: null
approved_by: null
master_plan_section: "§6.4.4 Backup, §6.4.5 Restore, §9 Phase 3"
prior_retros: ["phase-0-retro.md", "phase-1-retro.md", "phase-2-retro.md"]
---

# Phase 3: Backup and Restore

> **Test-only phase.** No code in this phase runs real `openclaw backup`,
> `openssl`, `tar`, `chown`, `systemctl`, `su`, or timer installation during
> tests. All system interaction must go through `shell.Executor` / `shell.FS`
> and be covered with mocks.
>
> Docker is not used as a VPS simulator. Final system verification remains
> deferred to owner-run checks on an actual VPS.

---

## 1. Goals

1. `internal/state` supports first-class backup metadata CRUD and backup records
   survive user deletion.
2. `internal/backup` provides encrypted backup creation, restore for existing
   users, and auto-backup timer installation through injectable dependencies.
3. Phase 2 user removal becomes backup-first: destructive remove aborts if the
   pre-remove backup fails.
4. TUI menu item 3 exposes backup and restore actions for the selected user.
5. Documentation explains backup storage, encryption, restore limits, and the
   test-only boundary.
6. `make ci` passes.

---

## 2. Out of Scope

- Live VPS execution, Docker VPS simulation, or real Linux-user integration
  tests.
- Restore-to-missing-user. Phase 3 restores existing managed users only.
- Manual backup password prompts. Phase 3 uses the master key from config path.
- Master-key rotation.
- Remote/offsite backups.
- Backup retention policy pruning.
- Real overlay-API/cloudflared route mutation.

---

## 3. Inputs

- Code state at start: current main branch after Phase 2.
- Go baseline: `go 1.26.2`.
- TUI baseline: `charm.land/bubbletea/v2 v2.0.6`; `View()` returns
  `tea.View`, tests use `tea.KeyPressMsg`.
- Storage baseline: `modernc.org/sqlite v1.49.1`; `schemas/state.sql` already
  has a `backups` table, but current FK cascades with `users`.
- Prior retros: `phase-0-retro.md`, `phase-1-retro.md`,
  `phase-2-retro.md`.
- Phase 2 carry-forward: backup before remove is absent and must be added here.
- Current project constraints: no live installs, no VPS during Phase 3, no
  Docker simulation, tests only.
- Required local tools for CI: `jq`, `golangci-lint`.

---

## 4. Architecture for This Phase

### Backup ownership

Add a new package:

```text
internal/backup/
  manager.go       <- Manager with Create/List/Restore/InstallTimer
  manager_test.go  <- MockExecutor + MemFS + temp state DB
```

`backup.Manager` must not import `internal/tui` or call `os/exec` directly.
All subprocess calls go through `shell.Executor`. Filesystem operations go
through `shell.FS`.

### Backup storage

Default paths:

- master key: `/etc/openclaw-multi/master.key`
- backup root: `/var/lib/openclaw-multi/backups`
- per-user backup dir: `/var/lib/openclaw-multi/backups/<username>`
- encrypted archive:
  `/var/lib/openclaw-multi/backups/<username>/<timestamp>.tar.gz.enc`

Phase 3 backups are encrypted automatically:

```bash
openssl enc -aes-256-cbc -pbkdf2 -salt \
  -pass file:/etc/openclaw-multi/master.key \
  -in <source.tar.gz> \
  -out <dest.tar.gz.enc>
```

The master key is created if missing with mode `0600` and read through `FS`.
Tests must not depend on real file permissions.

### Backup creation flow

Backend `Create(ctx, username)`:

1. Verify user exists in state.
2. Ensure master key exists.
3. Run:
   `su - <user> -c "openclaw backup create --output ~/.openclaw-backup-tmp --verify"`.
4. Parse produced archive path from stdout.
5. Encrypt archive to backup root path.
6. Compute SHA-256 and size of the encrypted archive.
7. Record backup metadata in state.
8. Emit `backup_create` audit event.

If any step fails before metadata insert, no backup metadata should be written.
Best-effort cleanup may remove partial encrypted output through `FS` if
`shell.FS` gains `Remove`.

### Restore flow

Backend `Restore(ctx, username, backupID)`:

1. Verify target user exists in state.
2. Load backup metadata.
3. Read encrypted archive path from state.
4. Decrypt with openssl to a temp `.tar.gz`.
5. Run `openclaw backup verify` or equivalent documented verification command.
6. Stop `openclaw-gateway` for the user.
7. Extract archive to a restore temp directory.
8. Move restored data into `~/.openclaw/` using mocked commands or `FS`.
9. `chown -R <user>:<user> ~/.openclaw`.
10. Start `openclaw-gateway`.
11. Emit `backup_restore` audit event.

Restore-to-missing-user is explicitly deferred. The TUI should show a clear
message if the user is missing.

### Backup-before-remove

Extend Phase 2 user removal with a pre-remove backup hook:

```go
type BeforeRemove interface {
    BeforeRemove(ctx context.Context, username string) error
}
```

or an equivalent function field on `users.Manager`.

Behavior:

- active user removal still follows deactivate-first;
- backup runs before route deletion, uninstall, linger disable, `userdel`, or
  state delete;
- if backup fails, remove aborts and emits an error result.

### Auto-backup timer

Add templates:

- `templates/openclaw-backup@.service.tmpl`
- `templates/openclaw-backup@.timer.tmpl`

Phase 3 installs intended timer units through `FS` and mockable `systemctl`
commands. It does not run real timers in tests.

---

## 5. Atomic Tasks

| ID  | Title                                                       | Est.  | Depends | Parallel | Status  | PR  |
|-----|-------------------------------------------------------------|-------|---------|----------|---------|-----|
| T01 | Add backup state model                                      | 0.25d | —       | yes      | done    | —   |
| T02 | Implement backup metadata CRUD in `state.Store`             | 0.5d  | T01     | no       | done    | —   |
| T03 | Update schema so backups survive user deletion              | 0.5d  | T01     | no       | done    | —   |
| T04 | Extend `shell.FS` if cleanup requires `Remove`              | 0.25d | —       | yes      | done    | —   |
| T05 | Create `internal/backup.Manager` skeleton                   | 0.5d  | T02     | no       | done    | —   |
| T06 | Implement master-key ensure/read behavior                   | 0.5d  | T05     | yes      | done    | —   |
| T07 | Implement encrypted backup creation flow                    | 1d    | T05-T06 | no       | done    | —   |
| T08 | Record checksum, size, version, and audit metadata          | 0.5d  | T07     | no       | done    | —   |
| T09 | Implement restore flow for existing users                   | 1d    | T05-T08 | no       | done    | —   |
| T10 | Add auto-backup systemd timer templates and installer       | 0.75d | T05     | yes      | done    | —   |
| T11 | Wire backup-before-remove into `users.Manager`              | 0.5d  | T07     | no       | done    | —   |
| T12 | Extend TUI user screen with backup/restore actions          | 1d    | T07-T09 | no       | done    | —   |
| T13 | Add `docs/backups.md`                                      | 0.5d  | T07-T12 | yes      | done    | —   |
| T14 | Update CHANGELOG and write Phase 3 retro                    | 0.25d | all     | no       | done    | —   |

---

### T01: Add Backup State Model

**Description.** Add `state.Backup` matching `schemas/state.sql`.

**Acceptance criteria.**

- [x] Model includes `ID`, `Username`, `TS`, `SizeBytes`, `SHA256`,
      `OpenClawVersion`, `Encrypted`, and `Path`.
- [x] Time fields use `time.Time`.
- [x] Godoc exists for exported type.

**Test plan.** Covered by T02 state tests.

---

### T02: Backup Metadata CRUD in `state.Store`

**Description.** Add `ListBackupsByUser`, `GetBackup`, `UpsertBackup`, and
`DeleteBackup`.

**Acceptance criteria.**

- [x] `ListBackupsByUser` returns newest first.
- [x] `GetBackup` returns sentinel `ErrNoBackup`.
- [x] `UpsertBackup` records all metadata fields.
- [x] `DeleteBackup` returns `ErrNoBackup` for missing IDs.

**Test plan.** Temp SQLite DB tests for insert/list/get/delete and missing rows.

---

### T03: Preserve Backups After User Delete

**Description.** Update canonical schema and inlined migration DDL so backups do
not cascade-delete with `users`.

**Acceptance criteria.**

- [x] `schemas/state.sql` no longer defines `backups.username` with
      `ON DELETE CASCADE`.
- [x] `internal/state/migrations.go` stays in sync.
- [x] Deleting a user leaves backup metadata intact.

**Test plan.** State test: create user, create backup metadata, delete user,
list backups by username still returns the backup.

---

### T04: Optional `shell.FS.Remove`

**Description.** Add `Remove(path string) error` to `shell.FS` only if partial
backup cleanup requires it.

**Acceptance criteria.**

- [x] `RealFS` and `MemFS` implement `Remove`.
- [x] Existing packages compile without direct `os.Remove`.

**Test plan.** Existing shell tests plus backup cleanup tests.

---

### T05: `internal/backup.Manager`

**Description.** Create the backup orchestration type.

**Acceptance criteria.**

- [x] Constructor takes `Executor`, `FS`, `Store`, options, and audit logger.
- [x] No direct `os/exec`.
- [x] Errors wrap the failed step.
- [x] Package does not import `internal/tui`.

**Test plan.** Constructor and ready-state unit tests.

---

### T06: Master Key Handling

**Description.** Ensure/read `/etc/openclaw-multi/master.key`.

**Acceptance criteria.**

- [x] Missing key is generated through crypto random bytes and written `0600`.
- [x] Existing key is reused.
- [x] Empty key file is rejected.
- [x] Tests do not require real permissions.

**Test plan.** MemFS tests for missing, existing, empty, and write failure.

---

### T07: Encrypted Backup Creation

**Description.** Implement `Create(ctx, username)`.

**Acceptance criteria.**

- [x] Refuses unknown user.
- [x] Runs OpenClaw backup command through `Executor`.
- [x] Parses produced archive path from stdout.
- [x] Runs openssl encryption through `Executor`.
- [x] Does not record metadata if command/encryption fails.
- [x] Emits error audit on failure.

**Test plan.** MockExecutor tests for success and each failure point.

---

### T08: Backup Metadata and Audit

**Description.** Compute final encrypted archive metadata and persist it.

**Acceptance criteria.**

- [x] Metadata includes SHA-256 and size of encrypted archive.
- [x] `openclaw_version` is stored as an empty string until manifest parsing is
      added in a later phase.
- [x] Emits `backup_create` with result `ok`.
- [x] Audit details include backup ID and path.

**Test plan.** MemFS archive fixture tests and audit recorder assertions.

---

### T09: Restore Existing User

**Description.** Implement `Restore(ctx, username, backupID)` for existing users.

**Acceptance criteria.**

- [x] Refuses unknown user.
- [x] Refuses unknown backup.
- [x] Decrypts archive with openssl.
- [x] Verifies archive before restore.
- [x] Stops gateway, extracts, fixes ownership, starts gateway.
- [x] Emits `backup_restore` audit event.

**Test plan.** MockExecutor tests for success and each failure point.

---

### T10: Auto-backup Timer

**Description.** Add systemd user timer templates and mocked installer.

**Acceptance criteria.**

- [x] Service/timer templates render deterministically.
- [x] Installer writes unit files through `FS`.
- [x] Installer runs intended `systemctl daemon-reload` and `systemctl enable
      --now openclaw-backup@<user>.timer` through `Executor`.
- [x] No real timers are started in tests.

**Test plan.** Template rendering and MockExecutor order tests.

---

### T11: Backup Before Remove

**Description.** Wire backup creation into `users.Manager.Remove`.

**Acceptance criteria.**

- [x] Remove calls backup before route deletion, uninstall, linger disable,
      `userdel`, or state delete.
- [x] Backup failure aborts remove.
- [x] Existing deactivate-first behavior remains.
- [x] Tests prove destructive commands are not called after backup failure.

**Test plan.** `internal/users` tests with fake backup hook.

---

### T12: TUI Backup/Restore Actions

**Description.** Extend user-management screen with backup and restore actions.

**Acceptance criteria.**

- [x] User list shows backup/restore key hints.
- [x] `b` triggers backup for selected user.
- [x] `r` opens backup list for selected user and restores selected backup.
- [x] Missing backups render a clear empty state.
- [x] Backend errors render visibly.

**Test plan.** Bubble Tea model tests with fake backup service.

---

### T13: `docs/backups.md`

**Description.** Document backup and restore behavior.

**Acceptance criteria.**

- [x] Documents storage paths.
- [x] Documents automatic encryption and master-key path.
- [x] Documents restore limitation: existing managed users only.
- [x] Documents test-only/live VPS boundary.

**Test plan.** Manual doc review.

---

### T14: CHANGELOG and Retro

**Description.** Update release notes and write the Phase 3 retrospective after
implementation.

**Acceptance criteria.**

- [x] `CHANGELOG.md` has Phase 3 additions under `## v0.3.0`.
- [x] `docs/phases/phase-3-retro.md` is written.
- [x] Retro records deviations from this plan.

**Test plan.** Manual doc review.

---

## 6. Definition of Done for Phase 3

- [x] All 14 tasks completed.
- [x] `make ci` passes.
- [x] Coverage >= 80% on new `internal/backup` package.
- [x] Backup metadata survives user deletion.
- [x] Remove is backup-first and aborts before destructive commands if backup
      fails.
- [x] Restore works for existing managed users in tests.
- [x] No direct `os/exec` usage outside `internal/shell`.
- [x] No live Linux-user, OpenClaw, systemd, openssl, or tar execution in tests.
- [x] `docs/backups.md` merged.
- [x] `CHANGELOG.md` updated.
- [x] `docs/phases/phase-3-retro.md` written.

---

## 7. Phase Smoke Test Suite

There are **no Docker or live Linux-user integration tests** in Phase 3.
Verification is limited to unit tests, TUI model tests, builds, and CI.

| Test level  | What runs                                                   | Where           |
|-------------|-------------------------------------------------------------|-----------------|
| Unit        | Backup manager, state backup CRUD, backup-before-remove     | `go test ./...` |
| TUI model   | Backup/restore user-management flows with fake services     | `go test ./...` |
| Build check | All three binaries compile                                  | `make ci`       |
| CI          | Lint + tests + builds + schema checks                       | `make ci`       |

Optional manual dev check: `make dev`, open menu item 3, verify backup/restore
actions render and error states are visible. This must not create real backups
or run real system commands.

---

## 8. Documentation Deliverables

- [x] `docs/backups.md`
- [x] CHANGELOG entry for Phase 3
- [x] Godoc on exported `internal/backup` and new `internal/state` types
- [x] `docs/phases/phase-3-retro.md`

---

## 9. Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Backup metadata currently cascades with user deletion | Change schema and add regression test before wiring backup-before-remove |
| Mocked OpenClaw backup stdout may differ from real CLI | Keep parsing small and documented; real VPS validation later verifies exact output |
| Encrypted archive checksum may be computed from wrong file | Test checksum over encrypted output fixture, not source archive |
| Restore can corrupt existing `~/.openclaw` if command order is wrong | Keep restore steps explicit and model-tested through command-order assertions |
| Master key loss makes encrypted backups unusable | Document master-key path and diagnostic-snapshot carry-forward; rotation is out of scope |
| Timer behavior differs across systemd environments | Phase 3 only renders/installs intended units through mocks; real timer behavior validated later on VPS |

---

## 10. Decisions

- Phase 3 backups are encrypted automatically with a single master key.
- Manual token/password entry is not part of backup/restore.
- Restore-to-missing-user is deferred.
- Backup-before-remove is mandatory after Phase 3.
- Docker is not used for VPS simulation.
