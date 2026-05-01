---
phase: 4
title: "Doctor and health check"
slug: doctor-health
estimated_duration: "1 week"
status: implemented
approved_at: null
approved_by: null
master_plan_section: "§6.5 Health check / Doctor, §9 Phase 4"
prior_retros: ["phase-0-retro.md", "phase-1-retro.md", "phase-2-retro.md", "phase-3-retro.md"]
---

# Phase 4: Doctor and Health Check

> **Test-only phase.** No code in this phase runs real `systemctl`, `loginctl`,
> `ss`, `df`, `openclaw doctor`, `chmod`, or `install` during tests. All system
> interaction must go through `shell.Executor` / `shell.FS` and be covered with
> mocks.
>
> Docker is not used as a VPS simulator. Final system verification remains
> deferred to owner-run checks on an actual VPS.

---

## 1. Goals

1. Add a first-class `internal/doctor` package that produces structured health
   results for system, services, per-user runtime, filesystem permissions, and
   OpenClaw doctor output.
2. TUI menu item **4. Health check / Doctor** becomes a real screen instead of a
   placeholder.
3. The TUI can run read-only checks, run `openclaw doctor` for each active user,
   and apply safe auto-fixes for known permission problems.
4. Auto-fix is intentionally narrow: chmod managed OpenClaw files and optional
   umask profile repair only. No network, route, package, or account mutation.
5. Health results are deterministic and unit-testable without a VPS.
6. `make ci` passes.

---

## 2. Out of Scope

- Live VPS execution, Docker VPS simulation, or real Linux-user integration
  tests.
- Deep network management UI. Tailscale, Cloudflare, UFW route management and
  connectivity tests belong to Phase 5.
- Overlay-API daemon checks beyond service presence. The daemon itself belongs
  to Phase 6.
- Per-user watcher deep behavior. Watcher implementation belongs to Phase 7.
- Live log tailing and log viewport. That belongs to Phase 8.
- Full security audit screen. Phase 4 may report obvious permission/listener
  warnings, but the dedicated audit workflow belongs to Phase 9.
- Diagnostic snapshot archive generation. That belongs to Phase 10, though this
  phase should define reusable result types that later snapshot code can include.
- Restore-to-missing-user, backup retention, offsite backups, and master-key
  rotation.

---

## 3. Inputs

- Code state at start: main after Phase 3 commit `acaf73b`.
- Go baseline: `go 1.26.2`.
- TUI baseline: `charm.land/bubbletea/v2 v2.0.6`; `View()` returns
  `tea.View`, tests use `tea.KeyPressMsg`.
- Current abstractions:
  - `shell.Executor` and `shell.FS` are the only allowed system boundaries.
  - `state.Store` exposes managed users, routes, backups, and admin metadata.
  - `internal/preflight`, `internal/hardening`, and `internal/deps` already
    contain reusable checks for tools, ports, sysctl/hidepid, and services.
  - `internal/users` owns user lifecycle and service command conventions.
- Phase 3 carry-forward:
  - real VPS validation for OpenClaw backup stdout remains deferred;
  - diagnostic snapshot should eventually include master-key handling guidance;
  - no Docker VPS simulation.

---

## 4. Architecture for This Phase

### Package ownership

Add a new package:

```text
internal/doctor/
  models.go       <- Status, CheckResult, Report, FixPlan
  checker.go      <- Checker with Run/RunOpenClawDoctor/PlanFixes/ApplyFixes
  parser.go       <- small parsers for command outputs
  checker_test.go <- MockExecutor + MemFS + temp state DB
```

`doctor.Checker` must not import `internal/tui` or call `os/exec` directly.
All subprocess calls go through `shell.Executor`. Filesystem reads/writes/stat
operations go through `shell.FS`.

### Result model

Use explicit statuses instead of raw strings:

```go
type Status string

const (
    StatusOK      Status = "ok"
    StatusWarn    Status = "warn"
    StatusFail    Status = "fail"
    StatusSkipped Status = "skipped"
)

type CheckResult struct {
    ID          string
    Category    string
    Target      string
    Status      Status
    Message     string
    Details     map[string]string
    Fixable     bool
    FixID       string
}
```

Categories should be stable:

- `system`
- `services`
- `users`
- `filesystem`
- `network`
- `openclaw`

The TUI should render these categories in the same order.

### System checks

Phase 4 system checks are read-only:

- disk free summary via `df`;
- memory summary via `/proc/meminfo` or `free`;
- load average via `/proc/loadavg`;
- `kernel.yama.ptrace_scope`;
- `/proc` hidepid mount option;
- required command presence for `openclaw`, `systemctl`, `loginctl`, `ss`.

Use existing `preflight` / `hardening` code where it reduces duplication, but
do not refactor unrelated Phase 1 code.

### Service checks

Phase 4 service checks are shallow status checks:

- `cloudflared`;
- `tailscaled`;
- `openclaw-overlay-api`;

The checks may return `skipped` if a service belongs to a later phase and is not
expected to exist yet. The message must distinguish "not installed yet" from
"installed but failed".

### Per-user checks

For each managed user in state:

- paused users are `skipped` for runtime checks;
- active users check `loginctl show-user <user>` for linger;
- active users check `systemctl --user is-active openclaw-gateway.service`;
- active users check `systemctl --user is-active openclaw-overlay-watcher.service`;
- active users check expected gateway port ownership/listen state through `ss`
  output parsing.

No Linux users are created or modified in tests.

### Filesystem permission checks

For each managed user:

- `/home/<user>/.openclaw` should be mode `0700`;
- `/home/<user>/.openclaw/openclaw.json` should be mode `0600`;
- `/home/<user>/.openclaw-overlay` should be mode `0700` when present;
- sensitive files must not be group/world-readable.

These checks are fixable.

### Auto-fix boundary

Auto-fix is allowed only for fix IDs generated by this phase:

- `chmod-openclaw-dir`: `chmod 0700 /home/<user>/.openclaw`;
- `chmod-openclaw-config`: `chmod 0600 /home/<user>/.openclaw/openclaw.json`;
- `chmod-overlay-dir`: `chmod 0700 /home/<user>/.openclaw-overlay`;
- `repair-umask-profile`: rewrite the existing overlay-managed umask profile if
  it is missing or drifted.

Auto-fix must:

- require an explicit TUI action;
- show the list of planned fixes before applying;
- run every command through `shell.Executor`;
- emit audit events;
- never run package installs, route changes, user lifecycle mutations, backup
  restore, or credential rotation.

### OpenClaw doctor wrapper

Backend flow:

1. Load managed users from state.
2. Skip paused users by default.
3. Run:
   `sudo -u <user> -H bash -lc "/home/<user>/.local/bin/openclaw doctor --json"`
4. Parse JSON output if available.
5. If JSON parse fails, use line-based parsing for obvious
   `ok`/`warn`/`error` markers.
6. Record one `openclaw` category result per user.
7. Emit `doctor_run` audit event.

If the real OpenClaw CLI does not support `--json`, the text parser becomes the
expected path and the retro must record that deviation.

### TUI screen

Menu item 4 should render:

```text
Health Check / Doctor

System:
  [ok] Disk space: free 12.3 GB / 50 GB
  [warn] ptrace_scope = 1, recommended 2

Services:
  [ok] tailscaled active
  [skipped] openclaw-overlay-api not implemented yet

Per-user:
  alice: [ok] gateway active, [ok] watcher active, [ok] linger enabled
  bob: [skipped] paused

Filesystem:
  alice: [warn] openclaw.json mode 0644, expected 0600 [fixable]

OpenClaw:
  alice: [ok] openclaw doctor passed

r run checks   d run openclaw doctor   f review fixes   q back
```

The TUI model must be unit-tested with fake doctor services. It should not run
real checks in tests.

---

## 5. Atomic Tasks

| ID  | Title                                                     | Est.  | Depends | Parallel | Status  | PR  |
|-----|-----------------------------------------------------------|-------|---------|----------|---------|-----|
| T01 | Add doctor result models                                  | 0.25d | —       | yes      | done    | —   |
| T02 | Create `internal/doctor.Checker` skeleton                 | 0.5d  | T01     | no       | done    | —   |
| T03 | Implement system checks                                   | 0.75d | T02     | no       | done    | —   |
| T04 | Implement service checks                                  | 0.5d  | T02     | yes      | done    | —   |
| T05 | Implement per-user runtime checks                         | 0.75d | T02     | no       | done    | —   |
| T06 | Implement filesystem permission checks                    | 0.5d  | T02     | yes      | done    | —   |
| T07 | Implement auto-fix planning and application               | 0.75d | T06     | no       | done    | —   |
| T08 | Implement OpenClaw doctor wrapper and parser              | 0.75d | T02     | no       | done    | —   |
| T09 | Add audit actions for doctor run and fixes                | 0.25d | T07-T08 | yes      | done    | —   |
| T10 | Wire menu item 4 to a real TUI screen                     | 1d    | T02-T09 | no       | done    | —   |
| T11 | Add docs/health.md                                        | 0.5d  | T10     | yes      | done    | —   |
| T12 | Update CHANGELOG and write Phase 4 retro                  | 0.25d | all     | no       | done    | —   |

---

### T01: Doctor Result Models

**Description.** Add stable result structs and status enums.

**Acceptance criteria.**

- [x] `Status` enum includes `ok`, `warn`, `fail`, and `skipped`.
- [x] `CheckResult` includes stable `ID`, `Category`, `Target`, `Status`,
      `Message`, `Details`, `Fixable`, and `FixID`.
- [x] `Report` groups results and exposes summary counts.
- [x] Exported types have Godoc.

**Test plan.** Unit tests for summary counts and deterministic ordering.

---

### T02: `internal/doctor.Checker`

**Description.** Create the orchestration type.

**Acceptance criteria.**

- [x] Constructor takes `Store`, `Executor`, `FS`, options, and audit logger.
- [x] No direct `os/exec`.
- [x] Package does not import `internal/tui`.
- [x] Missing dependencies return clear errors.

**Test plan.** Ready-state tests and compile-time interface checks.

---

### T03: System Checks

**Description.** Implement read-only VPS system checks.

**Acceptance criteria.**

- [x] Disk, memory, load, ptrace_scope, hidepid, and required commands are
      represented as structured results.
- [x] Missing optional later-phase services can be `skipped`, not hard fail.
- [x] Parser functions are deterministic and covered.

**Test plan.** MockExecutor and MemFS fixtures for healthy, warn, and fail
outputs.

---

### T04: Service Checks

**Description.** Check shallow service health.

**Acceptance criteria.**

- [x] Checks `tailscaled`, `cloudflared`, and `openclaw-overlay-api`.
- [x] Distinguishes missing later-phase services from failed installed services.
- [x] Does not start, stop, enable, or disable services.

**Test plan.** MockExecutor command-order tests and status mapping tests.

---

### T05: Per-user Runtime Checks

**Description.** Check each managed user's runtime state.

**Acceptance criteria.**

- [x] Paused users are skipped for runtime checks.
- [x] Active users check linger, gateway service, watcher service, and gateway
      port listen state.
- [x] Results include username and port details.
- [x] No Linux users are created or modified.

**Test plan.** Temp state DB with active/paused users and mocked command
responses.

---

### T06: Filesystem Permission Checks

**Description.** Detect unsafe permissions on managed OpenClaw files.

**Acceptance criteria.**

- [x] `~/.openclaw` expected mode is `0700`.
- [x] `~/.openclaw/openclaw.json` expected mode is `0600`.
- [x] `~/.openclaw-overlay` expected mode is `0700` when present.
- [x] Missing files are `warn` or `skipped` with precise messages, not panics.
- [x] Unsafe modes produce fixable results.

**Test plan.** MemFS/stat fixture tests for safe, unsafe, and missing paths.

---

### T07: Auto-fix Planning and Application

**Description.** Apply only known safe permission fixes.

**Acceptance criteria.**

- [x] `PlanFixes(report)` returns only approved fix IDs.
- [x] `ApplyFixes(ctx, plan)` runs chmod/profile repair through `Executor`/`FS`.
- [x] The TUI shows the planned fixes before applying.
- [x] Fix failure leaves the report visible and emits an error.
- [x] No non-permission mutation is possible through this API.

**Test plan.** Unit tests for allowlist behavior, command order, and failure
propagation.

---

### T08: OpenClaw Doctor Wrapper

**Description.** Run OpenClaw's own doctor per active managed user.

**Acceptance criteria.**

- [x] Runs `sudo -u <user> -H bash -lc "openclaw doctor --json"` through `Executor`.
- [x] Skips paused users.
- [x] Parses JSON doctor output when available.
- [x] Falls back to line parsing for non-JSON output.
- [x] Records one or more structured `openclaw` results per user.

**Test plan.** Parser tests for JSON, text, empty output, and non-zero exit.

---

### T09: Audit Events

**Description.** Add audit actions for doctor and fix operations.

**Acceptance criteria.**

- [x] New actions: `doctor_run`, `doctor_fix`.
- [x] Successful checks emit `doctor_run` with summary counts.
- [x] Failed checks emit `doctor_run` with `result=error`.
- [x] Applied fixes emit `doctor_fix` with fix IDs and targets.

**Test plan.** Audit recorder tests in `internal/doctor`.

---

### T10: TUI Health Screen

**Description.** Replace menu item 4 placeholder with a real screen.

**Acceptance criteria.**

- [x] Menu item 4 opens the Health Check / Doctor screen.
- [x] `r` runs all read-only checks.
- [x] `d` runs OpenClaw doctor for active users.
- [x] `f` opens a fix review/apply flow when fixable results exist.
- [x] Empty, loading, success, warning, failure, and service-unavailable states
      are rendered clearly.

**Test plan.** Bubble Tea model tests with fake doctor service.

---

### T11: `docs/health.md`

**Description.** Document doctor behavior and auto-fix boundaries.

**Acceptance criteria.**

- [x] Documents check categories.
- [x] Documents which checks are read-only.
- [x] Documents the narrow auto-fix allowlist.
- [x] Documents test-only/live VPS boundary.
- [x] Documents that deeper network, logs, audit, and snapshot workflows belong
      to later phases.

**Test plan.** Manual doc review.

---

### T12: CHANGELOG and Retro

**Description.** Update release notes and write the Phase 4 retrospective after
implementation.

**Acceptance criteria.**

- [x] `CHANGELOG.md` has Phase 4 additions under `## v0.4.0`.
- [x] `docs/phases/phase-4-retro.md` is written.
- [x] Retro records deviations from this plan and real coverage numbers.

**Test plan.** Manual doc review.

---

## 6. Definition of Done for Phase 4

- [x] All 12 tasks completed.
- [x] `make ci` passes.
- [x] Coverage >= 80% on new `internal/doctor` package.
- [x] TUI menu item 4 is no longer a placeholder.
- [x] Doctor checks use only `shell.Executor` / `shell.FS` for system access.
- [x] Auto-fix is limited to approved permission/profile fixes.
- [x] Paused users are skipped for runtime and OpenClaw doctor checks.
- [x] No live Linux-user, OpenClaw, systemd, loginctl, ss, chmod, or install
      execution in tests.
- [x] `docs/health.md` merged.
- [x] `CHANGELOG.md` updated.
- [x] `docs/phases/phase-4-retro.md` written.

---

## 7. Phase Smoke Test Suite

There are **no Docker or live Linux-user integration tests** in Phase 4.
Verification is limited to unit tests, TUI model tests, builds, and CI.

| Test level  | What runs                                               | Where           |
|-------------|---------------------------------------------------------|-----------------|
| Unit        | Doctor checker, parsers, fix planning, audit            | `go test ./...` |
| TUI model   | Health screen flows with fake doctor service            | `go test ./...` |
| Build check | All three binaries compile                              | `make ci`       |
| CI          | Lint + race tests + builds + schema checks              | `make ci`       |

Optional manual dev check: `make dev`, open menu item 4, verify health groups,
doctor action, fix review, and error states render. This must not create real
users or mutate live services.

---

## 8. Documentation Deliverables

- [x] `docs/health.md`
- [x] CHANGELOG entry for Phase 4
- [x] Godoc on exported `internal/doctor` types
- [x] `docs/phases/phase-4-retro.md`

---

## 9. Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Phase 4 accidentally becomes Phase 5 network management | Keep network checks shallow and read-only; route/DNS/UFW management remains Phase 5 |
| Auto-fix mutates more than intended | Fix IDs must be allowlisted and tested; no package, route, service lifecycle, user lifecycle, or credential mutation |
| Real `openclaw doctor` output format differs | Implement JSON parser plus conservative text parser; record real VPS deviation in retro |
| Missing later-phase services make health look broken | Use `skipped` with clear "not implemented yet" messages where appropriate |
| Permission checks need realistic file modes in tests | Extend test fixtures around `shell.FS.Stat`; avoid direct OS filesystem assumptions |
| TUI becomes noisy with too many checks | Group by stable categories and render summary counts first |

---

## 10. Decisions

- Phase 4 owns Doctor / Health Check only.
- Auto-fix is opt-in and limited to safe permission/profile fixes.
- Paused users are skipped for runtime and OpenClaw doctor checks.
- Later-phase services may be `skipped` instead of `fail` until implemented.
- Docker is not used for VPS simulation.
