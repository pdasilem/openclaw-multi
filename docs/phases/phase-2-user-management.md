---
phase: 2
title: "User management: add/remove/activate/deactivate"
slug: user-management
estimated_duration: "1.5 weeks (7-8 working days)"
status: implemented
approved_at: null
approved_by: null
master_plan_section: "§9 Phase 2, §6.4 (user management), §2.5 (идемпотентность)"
prior_retros: ["phase-0-retro.md", "phase-1-retro.md"]
---

# Phase 2: User Management

> **Test-only phase.** No code in this phase runs real `useradd`, `userdel`,
> `loginctl`, `systemctl`, `su`, `openclaw onboard`, or cloudflared route
> changes during tests. All system interaction must go through
> `shell.Executor` / `shell.FS` and be covered with mocks.
>
> After this phase: TUI menu item 3 is no longer a placeholder. It shows managed
> users and supports add, remove, deactivate, and activate flows against the
> state database and mocked system operations. Docker is explicitly not used as
> a VPS simulator. Real system verification remains deferred to owner-run checks
> on an actual VPS.

---

## 1. Goals

1. `internal/state` supports first-class overlay users, routes, and port pools:
   list/get/upsert/delete users, status transitions, route CRUD, and port
   allocation state.
2. `internal/users` provides idempotent user lifecycle operations:
   add/bootstrap, remove, deactivate, and activate. It depends only on
   injectable `shell.Executor`, `shell.FS`, `state.Store`, config, and audit
   logging.
3. Phase 2 templates exist for per-user setup:
   `openclaw-overlay-watcher.service.tmpl` and, if needed, a minimal
   `openclaw.json.tmpl` or documented env-only onboarding path.
4. TUI menu item 3 opens a user-management screen with list/detail/actions.
   The screen is model-tested with Bubble Tea v2 messages.
5. All mutating user-visible operations emit audit events with existing action
   types: `bootstrap_user`, `delete_user`, `disable_user`, `enable_user`, and
   route actions where route state changes are simulated.
6. Documentation explains the Phase 2 user lifecycle and its test-only
   limitations.
7. `make ci` passes: lint clean, all tests green, amd64+arm64 build.

---

## 2. Out of scope

- Docker-based VPS simulation or Docker smoke tests.
- Real VPS execution during Phase 2. Final system verification is owner-run on a
  real VPS, not in Docker.
- Real `openclaw onboard` interactive execution.
- Real overlay-API HTTP server and real cloudflared config mutation. Phase 2
  records intended gateway routes in state and uses a mockable route publisher
  interface; HTTP implementation remains Phase 6.
- Backup/restore before remove. Phase 3 owns backup and restore.
- Per-plugin callback route watcher. Phase 7 owns watcher behavior.
- User quota management, token rotation, doctor, and route last-seen views.
  These remain later phases.
- Manual gateway token entry.
- Changing OpenClaw core behavior.

---

## 3. Inputs

- Code state at start: current main branch after Phase 1 and the dependency
  refresh.
- Go baseline: `go 1.26.2`.
- TUI baseline: `charm.land/bubbletea/v2 v2.0.6`; `View()` returns
  `tea.View`, and tests use `tea.KeyPressMsg`.
- Styling baseline: `charm.land/lipgloss/v2 v2.0.3`.
- Storage baseline: `modernc.org/sqlite v1.49.1`; `schemas/state.sql` already
  contains `users`, `routes`, and `port_pool` tables, but `state.Store` only
  exposes admin/meta methods today.
- Route prerequisite: `domain` and `subdomain` must be configured before
  `Add User` can proceed.
- Prior retros: `phase-0-retro.md`, `phase-1-retro.md`.
- Constraints from `OPENCLAW_OVERLAY_DEV_PROCESS.md` §0 and current project
  decision: no live installs, no VPS during Phase 2, no Docker simulation,
  tests only.

---

## 4. Architecture for This Phase

### Existing foundations

Phase 1 delivered the core test seams:

```go
type Executor interface {
    Run(ctx context.Context, opts ExecOpts) (ExecResult, error)
}

type FS interface {
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, data []byte, perm fs.FileMode) error
    Stat(path string) (fs.FileInfo, error)
    MkdirAll(path string, perm fs.FileMode) error
    Rename(oldpath, newpath string) error
}
```

Phase 2 must reuse these instead of adding direct `os/exec` or `os.*` calls.

### New package: `internal/users`

Recommended shape:

```text
internal/users/
  models.go          <- BootstrapRequest, RemoveRequest, UserStatus
  validate.go        <- username validation and normalization
  ports.go           <- deterministic port allocation
  manager.go         <- Manager with Add/Remove/Deactivate/Activate
  manager_test.go    <- MockExecutor + MemFS + temp state DB
  routes.go          <- RoutePublisher interface + state-backed mock publisher
```

`Manager` should be a thin orchestration layer. It does not own SQL directly;
it calls `state.Store` methods. It does not know Bubble Tea. It emits audit
events through a small logger interface compatible with `*audit.Logger`.

### Route publishing boundary

Master plan says Phase 2 creates a personal Control UI route through
overlay-API, but the HTTP daemon is Phase 6. Use an interface now:

```go
type RoutePublisher interface {
    EnableUserGateway(ctx context.Context, user state.User) (state.Route, error)
    DisableUserRoutes(ctx context.Context, username string) error
    DeleteUserRoutes(ctx context.Context, username string) error
}
```

For Phase 2, implement a state-backed or fake publisher used by tests and TUI.
The real HTTP client/server can replace it in Phase 6 without changing TUI or
user lifecycle code.

`domain` and route naming are prerequisites for user management because the
user-facing gateway URL is derived from config, for example
`gateway-alice.ui.example.com`. Without a configured `domain` and `subdomain`,
`Add User` must be blocked before any state, route, or command mutation. This is
not an optional partial state: a managed user in Phase 2 implies an intended
public gateway URL.

### State model

Add Go models that match `schemas/state.sql`:

```go
type User struct {
    Username   string
    UID        int
    Port       int
    Status     UserStatus // active | paused
    Linger     bool
    GatewayURL string
    CreatedAt  time.Time
    UpdatedAt  time.Time
}

type Route struct {
    ID        string
    Username  string
    Kind      RouteKind // gateway | plugin
    PluginID  string
    LocalPort int
    Hostname  string
    Enabled   bool
}
```

Keep SQL methods explicit and small. Avoid generic repositories.

### Port allocation

Use `config.OverlayConfig.PortRangeStart` and `PortRangeStep`.
Default behavior:

- first user: `18789`
- next users: `18809`, `18829`, ...
- gap must remain at least 20 to avoid browser/CDP derived port overlap.
- deleting a user immediately frees that gateway port for reuse.

The allocator must be deterministic and testable. It may use existing `users`
rows to find the next free port in Phase 2; `port_pool` can be used for
future plugin callback port allocation but should not block the user flow.

### User add flow

Backend add/bootstrap operation:

1. Validate username: `^[a-z0-9_-]{1,32}$`, no leading `-`, no empty value.
2. Validate that `domain` and `subdomain` are configured.
3. Refuse duplicates from state.
4. Allocate gateway port.
5. Generate gateway token automatically.
6. Run, via `Executor`, the intended commands:
   `useradd -m -s /bin/bash <user>`, `loginctl enable-linger <user>`, and
   `su - <user> -c "/home/<user>/.local/bin/openclaw onboard --non-interactive --mode local --auth-choice skip --gateway-bind loopback --gateway-auth token --gateway-token-ref-env OPENCLAW_GATEWAY_TOKEN --gateway-port $OPENCLAW_GATEWAY_PORT --install-daemon --accept-risk"`.
7. Pass env vars through `ExecOpts.Env`:
   `OPENCLAW_GATEWAY_PORT`, `OPENCLAW_GATEWAY_TOKEN`,
   `OPENCLAW_GATEWAY_BIND=loopback`.
8. Harden user files through mocked commands or `FS` writes where appropriate:
   `chmod 700 ~/.openclaw`, `chmod 600 ~/.openclaw/openclaw.json`.
9. Render/write per-user watcher unit template.
10. Enable/start user services through mockable `systemctl --user` calls.
11. Record user and gateway route in state.
12. Emit audit entry.

### Remove flow

Backend remove operation:

1. Require exact username confirmation in the TUI before invoking backend.
2. If the user is active, the TUI should guide the admin through deactivate
   first before exposing hard delete.
3. Stop user services.
4. Delete/disable routes through `RoutePublisher`.
5. Run `openclaw uninstall --all --yes --non-interactive` through `su -`.
6. `loginctl disable-linger <user>`.
7. `userdel -r <user>`.
8. Delete user and routes from state. The gateway port becomes reusable
   immediately after deletion.
9. Emit audit entry.

Backup remains Phase 3. Phase 2 may show a warning that remove is destructive.

### Deactivate / activate flows

Deactivate:

- stop `openclaw-gateway` and `openclaw-overlay-watcher`;
- `loginctl disable-linger <user>`;
- disable all user routes through `RoutePublisher`;
- set state status to `paused`;
- emit audit.

Activate:

- enable all user routes through `RoutePublisher`;
- `loginctl enable-linger <user>`;
- start user services;
- set state status to `active`;
- emit audit.

---

## 5. Task Breakdown

| ID  | Task                                                        | Est.  | Depends | Parallel | Status  | PR  |
|-----|-------------------------------------------------------------|-------|---------|----------|---------|-----|
| T01 | Add state models for users/routes/port allocation           | 0.5d  | —       | yes      | done    | —   |
| T02 | Implement `state.Store` user CRUD                           | 0.75d | T01     | no       | done    | —   |
| T03 | Implement route CRUD and route enable/disable methods       | 0.75d | T01     | yes      | done    | —   |
| T04 | Implement deterministic gateway port allocator              | 0.5d  | T02     | yes      | done    | —   |
| T05 | Add username validation helpers                             | 0.25d | —       | yes      | done    | —   |
| T06 | Create `internal/users.Manager` skeleton                    | 0.5d  | T02-T05 | no       | done    | —   |
| T07 | Implement add/bootstrap backend flow                        | 1d    | T06     | no       | done    | —   |
| T08 | Implement deactivate/activate backend flows                 | 0.75d | T06     | yes      | done    | —   |
| T09 | Implement remove backend flow                               | 0.75d | T06     | yes      | done    | —   |
| T10 | Add `RoutePublisher` fake/state-backed implementation       | 0.5d  | T03     | yes      | done    | —   |
| T11 | Add per-user watcher unit template                          | 0.25d | T07     | yes      | done    | —   |
| T12 | Wire user manager dependencies in `internal/tui/app.go`     | 0.5d  | T06     | no       | done    | —   |
| T13 | Implement user list TUI screen                              | 0.75d | T12     | no       | done    | —   |
| T14 | Implement add/deactivate/activate/remove TUI interactions   | 1d    | T13     | no       | done    | —   |
| T15 | Add docs/users.md                                           | 0.5d  | T07-T14 | yes      | done    | —   |
| T16 | Update CHANGELOG and phase retro                            | 0.25d | all     | no       | done    | —   |

---

## 6. Detailed Tasks

### T01: Add State Models

**Description.** Extend `internal/state/models.go` with `User`, `UserStatus`,
`Route`, `RouteKind`, and port allocation DTOs.

**Acceptance criteria.**

- [ ] Models map exactly to `schemas/state.sql` columns.
- [ ] Status/kind constants use the DB values: `active`, `paused`, `gateway`,
      `plugin`.
- [ ] Time fields use `time.Time`; SQL parsing is centralized in state code.

**Test plan.** Covered indirectly by T02/T03 tests.

---

### T02: User CRUD in `state.Store`

**Description.** Add `ListUsers`, `GetUser`, `UpsertUser`, `DeleteUser`,
`SetUserStatus`, and `UserExists`.

**Acceptance criteria.**

- [ ] `GetUser` returns a sentinel error such as `ErrNoUser`.
- [ ] `ListUsers` is deterministic, sorted by username.
- [ ] `UpsertUser` updates `updated_at`.
- [ ] Deletes cascade to routes/backups/port pool according to schema.

**Test plan.** Unit tests with temp SQLite DB for create/list/get/update/delete.

---

### T03: Route CRUD in `state.Store`

**Description.** Add route methods needed by gateway routes and future plugin
routes.

**Acceptance criteria.**

- [ ] `ListRoutesByUser(username)` returns deterministic ordering.
- [ ] `UpsertRoute`, `DeleteRoute`, `DeleteRoutesByUser` work.
- [ ] `SetRoutesEnabled(username, enabled)` updates all user routes.
- [ ] Duplicate hostname errors are wrapped with useful context.

**Test plan.** Unit tests for gateway and plugin routes, enable/disable, and
cascade delete.

---

### T04: Gateway Port Allocator

**Description.** Allocate unique gateway ports from
`PortRangeStart + n*PortRangeStep`.

**Acceptance criteria.**

- [ ] Defaults are `18789` and step `20`.
- [ ] Existing active/paused users reserve their ports.
- [ ] Deleted users immediately free their previous gateway port; allocator
      reuses the lowest free valid port.
- [ ] Invalid config (`step < 20`, start <= 0) returns a clear error.

**Test plan.** Unit tests for empty DB, existing users, holes, and invalid config.

---

### T05: Username Validation

**Description.** Validate Linux usernames accepted by overlay user management.

**Acceptance criteria.**

- [ ] Accepts only `[a-z0-9_-]`, length 1-32.
- [ ] Rejects leading `-`.
- [ ] Rejects reserved names used by the overlay admin/system where applicable.
- [ ] Error message is safe to show in TUI.

**Test plan.** Table-driven unit tests.

---

### T06: `internal/users.Manager`

**Description.** Create the orchestration type for user lifecycle operations.

**Acceptance criteria.**

- [ ] Constructor takes `Executor`, `FS`, `Store`, config, route publisher, and
      logger interfaces.
- [ ] No package in `internal/users` imports `internal/tui`.
- [ ] No direct `os/exec` calls.
- [ ] Errors wrap the failed step.

**Test plan.** Compile-time and constructor tests.

---

### T07: Add / Bootstrap User Backend

**Description.** Implement the backend for adding a user.

**Acceptance criteria.**

- [ ] Validates username before any mutation.
- [ ] Validates configured `domain` and `subdomain` before any mutation.
- [ ] Refuses duplicate state users.
- [ ] Allocates and stores a unique port.
- [ ] Generates a gateway token automatically.
- [ ] Records expected command sequence using `MockExecutor`:
      `useradd`, `loginctl enable-linger`, `su - <user> -c openclaw onboard`,
      chmod/setup, systemd user daemon-reload, enable/start watcher.
- [ ] Uses `ExecOpts.Env` for OpenClaw gateway env vars.
- [ ] Creates a gateway route in state through `RoutePublisher`.
- [ ] Emits `ActionBootstrapUser`.
- [ ] Rolls back state if a command fails before final commit, or documents and
      tests the compensating state behavior.

**Test plan.** MockExecutor tests for success, duplicate, invalid username,
command failure, route publisher failure, and audit emitted.

---

### T08: Deactivate / Activate Backend

**Description.** Implement pause/resume without deleting data.

**Acceptance criteria.**

- [ ] Deactivate stops services, disables linger, disables routes, sets
      `status=paused`.
- [ ] Activate enables routes, enables linger, starts services, sets
      `status=active`.
- [ ] Both operations are idempotent when already in the target state.
- [ ] Emits `ActionDisableUser` / `ActionEnableUser`.

**Test plan.** Unit tests for active->paused, paused->active, no-op paths, and
command failure.

---

### T09: Remove Backend

**Description.** Implement destructive remove for a managed user.

**Acceptance criteria.**

- [ ] Refuses unknown users with `ErrNoUser`.
- [ ] Deletes routes through publisher before deleting state.
- [ ] Runs OpenClaw uninstall, stops services, disables linger, and `userdel -r`.
- [ ] Deletes user state.
- [ ] Frees the user's gateway port immediately by deleting the user row.
- [ ] Emits `ActionDeleteUser`.
- [ ] Backup is not implemented here; caller/TUI warns that Phase 3 will add it.

**Test plan.** Unit tests for success, unknown user, command failure, and route
publisher failure.

---

### T10: Route Publisher Fake / State Implementation

**Description.** Provide a Phase 2 route publisher that records gateway routes
in state without requiring overlay-API HTTP.

**Acceptance criteria.**

- [ ] Gateway hostname format is deterministic:
      `gateway-<user>.<subdomain>.<domain>` when domain is configured.
- [ ] Missing domain returns a clear error before user add commits route state.
- [ ] Disable/delete operations update route state consistently.
- [ ] API shape can later be replaced by a real HTTP client in Phase 6.

**Test plan.** Unit tests for hostname construction, missing config, and route
state transitions.

---

### T11: Per-user Watcher Unit Template

**Description.** Add `templates/openclaw-overlay-watcher.service.tmpl`.

**Acceptance criteria.**

- [ ] Template uses `${USERNAME}` and overlay API endpoint variables.
- [ ] Unit runs `openclaw-overlay-watcher`.
- [ ] Unit is suitable for `systemctl --user`.
- [ ] Rendered output is deterministic.

**Test plan.** Template rendering unit test through existing config renderer.

---

### T12: Wire User Manager into TUI App

**Description.** Extend `internal/tui.Model` dependencies so menu item 3 can
invoke user operations.

**Acceptance criteria.**

- [ ] `Run()` constructs real dependencies.
- [ ] Unit tests can construct a model with fake user manager.
- [ ] Existing main menu tests remain green.
- [ ] No cyclic imports.

**Test plan.** TUI app unit tests for menu item 3 routing.

---

### T13: User List TUI Screen

**Description.** Replace menu item 3 placeholder with a user list screen.

**Acceptance criteria.**

- [ ] Shows username, status, port, and gateway URL.
- [ ] Empty state says there are no managed users and offers Add.
- [ ] Navigation works with arrows/`j`/`k`; back with `Esc`/`q`.
- [ ] Uses existing styles and Bubble Tea v2 `tea.View`.

**Test plan.** Model tests for render, empty state, navigation, and back.

---

### T14: User Action TUI Flows

**Description.** Add TUI interactions for add/remove/deactivate/activate.

**Acceptance criteria.**

- [ ] Add prompts for username; gateway token is generated automatically.
- [ ] Add is blocked with a clear config error when `domain` or `subdomain` is
      missing.
- [ ] Remove requires typing the exact username.
- [ ] If a user is active, Remove first routes through Deactivate before hard
      delete confirmation.
- [ ] Deactivate/activate ask for confirmation.
- [ ] Success and failure states are visible.
- [ ] Backend calls are made through fake manager in tests.

**Test plan.** Bubble Tea model tests for happy paths, validation failures, and
backend errors.

---

### T15: `docs/users.md`

**Description.** Document the Phase 2 user lifecycle for operators.

**Acceptance criteria.**

- [ ] Explains add, deactivate, activate, and remove.
- [ ] States that backup before remove is Phase 3 and not yet implemented.
- [ ] Documents generated gateway URL/token handling.
- [ ] Documents that `domain` and `subdomain` are required before adding users.
- [ ] Documents that live execution is not yet approved in this test-only phase.

**Test plan.** Manual doc review.

---

### T16: CHANGELOG and Retro

**Description.** Update release notes and write the Phase 2 retrospective after
implementation.

**Acceptance criteria.**

- [ ] `CHANGELOG.md` has Phase 2 additions under `## v0.2.0` or current
      unreleased section.
- [ ] `docs/phases/phase-2-retro.md` is written.
- [ ] Retro records any deviations from this plan.

**Test plan.** Manual doc review.

---

## 7. Definition of Done for Phase 2

- [x] All 16 tasks completed.
- [x] `make ci` passes: lint clean, all tests green, amd64+arm64 build.
- [x] Coverage >= 80% on new `internal/users` package.
- [x] Existing coverage on `internal/state` remains meaningful with user CRUD
      tests.
- [x] Menu item 3 opens the user management screen and no longer routes to the
      generic placeholder.
- [x] All Phase 2 mutating operations emit audit events.
- [x] No direct `os/exec` usage outside `internal/shell`.
- [x] `docs/users.md` merged.
- [x] `CHANGELOG.md` updated.
- [x] `docs/phases/phase-2-retro.md` written.

---

## 8. Phase Smoke Test Suite

There are **no Docker or live Linux-user integration tests** in Phase 2.
Verification is limited to unit tests, TUI model tests, builds, and CI. Final
system verification is owner-run on a real VPS later.

| Test level  | What runs                                                     | Where               |
|-------------|---------------------------------------------------------------|---------------------|
| Unit        | State CRUD, port allocation, user manager with MockExecutor   | `go test ./...`     |
| TUI model   | Bubble Tea v2 Update/View flows with fake user manager        | `go test ./...`     |
| Build check | All three binaries compile for amd64/arm64                    | `make build-amd64 build-arm64` |
| CI          | Lint + tests + builds + schema checks                         | `make ci`           |

Optional manual dev check: `make dev`, open menu item 3, verify the empty user
screen and navigation. This must not create real system users.

---

## 9. Documentation Deliverables

- [x] `docs/users.md`
- [x] CHANGELOG entry for Phase 2
- [x] Godoc on exported `internal/users` and new `internal/state` types
- [x] `docs/phases/phase-2-retro.md`

---

## 10. Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Mocked command order diverges from what systemd/loginctl needs on a real VPS | Keep commands explicit, documented, and tested; final validation is owner-run on a real VPS; do not add Docker as a substitute |
| overlay-API route creation is planned before the HTTP daemon exists | Use `RoutePublisher` interface and state-backed fake in Phase 2; real HTTP client/server in Phase 6 |
| Remove is destructive without backup | TUI warning and exact username confirmation; backup remains Phase 3 and must be called out clearly |
| Username validation differs from distro `useradd` rules | Use conservative allowed set and test table; surface useradd errors cleanly |
| Port allocation collisions with browser/CDP derived ports | Enforce `PortRangeStep >= 20`; test allocation gaps |
| Bubble Tea v2 key/view APIs cause test churn | Use local test helpers for `tea.KeyPressMsg`; assert `View().Content` |

---

## 11. Decisions and Design Notes

Resolved:

- Deleted users immediately free their previous gateway port. The allocator
  should reuse the lowest free valid port.
- Hard delete in the TUI should default to "deactivate first" for active users,
  then require exact username confirmation before remove.
- Docker is not used for VPS simulation; final system verification is owner-run
  on a real VPS.
- Gateway tokens are generated automatically in Phase 2. Manual token entry is
  not part of this phase.
- `domain` and `subdomain` are prerequisites for `Add User`. If either is
  missing, the TUI and backend must fail before any mutation with a clear config
  error.
