---
phase: 6
title: "Overlay-API daemon"
slug: overlay-api-daemon
estimated_duration: "1.5 weeks"
status: draft
approved_at: null
approved_by: null
master_plan_section: "§7 Overlay-API daemon, §9 Phase 6"
prior_retros: ["phase-0-retro.md", "phase-1-retro.md", "phase-2-retro.md", "phase-3-retro.md", "phase-4-retro.md", "phase-5-retro.md"]
external_docs_checked:
  - "https://developers.cloudflare.com/tunnel/advanced/local-management/"
  - "https://developers.cloudflare.com/tunnel/advanced/local-management/configuration-file/"
  - "https://developers.cloudflare.com/tunnel/advanced/local-management/as-a-service/linux/"
  - "https://man7.org/linux/man-pages/man7/socket.7.html"
  - "https://man7.org/linux/man-pages/man7/unix.7.html"
---

# Phase 6: Overlay-API Daemon

> **System-boundary phase.** Phase 6 introduces the first real owner of
> `/etc/cloudflared/config.yml`. Unit tests still use injectable filesystem,
> executor, clock, state, and peer-credential interfaces. Owner-run E2E checks
> are documented in `docs/e2e-use-cases.md`.
>
> The per-user watcher remains out of scope. Phase 6 must expose the API and
> client contract that Phase 7 will consume.

---

## 1. Goals

1. Replace the `openclaw-overlay-api` stub with a real HTTP daemon bound to a
   UNIX socket, with health and route-management endpoints covered by tests.
2. Authorize route operations with peer credentials: root may manage all users,
   and a user process may manage only routes for its own Linux UID.
3. Add deterministic cloudflared config generation from `state.routes`, with
   atomic write, backup, validation, SIGHUP reload, and rollback behavior.
4. Move user lifecycle route publication from state-only intent to the
   overlay-API path where practical, while retaining state-only tests and
   fallback seams.
5. Add last-seen HTTP endpoints backed by the existing Phase 5 cloudflared log
   parser/cache behavior.
6. `make ci` passes and owner-run E2E checks are documented.

## 2. Out of Scope

- Implementing `openclaw-overlay-watcher`, inotify, plugin config inspection,
  or `openclaw config set` mutation. That is Phase 7.
- Live Cloudflare DNS writes, wildcard DNS planning, or UFW mutation. Phase 5
  already owns those surfaces.
- TUI live-tail logs, overlay-API journal viewer, or monitoring dashboard. That
  is Phase 8.
- Switching the product from locally-managed tunnels to remotely-managed
  tunnels. Phase 6 keeps the current locally-managed architecture and records
  the design pressure.
- Full production HA reload with cloudflared replicas. Phase 6 uses SIGHUP for
  the single-host local-config reload path and documents owner-run VPS
  validation.
- Uninstall flows or diagnostic snapshot archive.

## 3. Inputs

- Code state at start: main after Phase 5 commit `5d24e26` (working tree also
  has a pre-existing uncommitted `.gitignore` modification; do not touch it).
- Master plan section:
  [§7 Overlay-API daemon](../OPENCLAW_OVERLAY_PLAN_RU.md) and
  [§9 Phase 6](../OPENCLAW_OVERLAY_PLAN_RU.md).
- Prior retros:
  [phase-0-retro.md](phase-0-retro.md),
  [phase-1-retro.md](phase-1-retro.md),
  [phase-2-retro.md](phase-2-retro.md),
  [phase-3-retro.md](phase-3-retro.md),
  [phase-4-retro.md](phase-4-retro.md),
  [phase-5-retro.md](phase-5-retro.md).
- Existing code surfaces:
  - `cmd/openclaw-overlay-api/main.go` is still a Phase 0 stub.
  - `internal/state` already supports users, routes, route upsert/delete,
    route enable/disable, and last-seen cache updates.
  - `internal/users.RoutePublisher` exists and currently defaults to
    `StateRoutePublisher`.
  - `internal/network` already has structured route visibility and a
    conservative cloudflared log parser.
  - `templates/cloudflared-config.tmpl` currently contains only tunnel,
    credentials file, and catch-all 404.
  - `templates/openclaw-overlay-api.service.tmpl` installs the daemon as root.
- Owner decisions for this phase:
  - Cloudflared reload uses SIGHUP.
  - Plugin route IDs are daemon-derived, not client-supplied.
  - Managed users and the configured admin may connect to the UNIX socket;
    authorization is enforced by daemon-side `SO_PEERCRED` checks.
  - `cloudflared_credentials_file` is an explicit config field with default
    `/etc/cloudflared/<tunnel_id>.json`; publication fails before writing if the
    file does not exist.
- External constraints checked:
  - Cloudflare local tunnel config files support multiple ingress rules and
    `cloudflared tunnel ingress validate`.
  - Cloudflare Linux service docs document restart to load changed config, but
    the owner decision for Phase 6 is to use SIGHUP and prove it explicitly.
  - Cloudflare docs recommend remote-managed tunnels for most use cases; this
    project intentionally remains local-managed until the master plan changes.
  - Linux `SO_PEERCRED` returns peer credentials for connected AF_UNIX stream
    sockets.

## 4. Architecture for This Phase

Add two focused packages:

```text
internal/api/
  server.go          <- HTTP routing, JSON requests/responses, error mapping
  auth.go            <- peer credentials, root/user authorization
  client.go          <- UNIX-socket client implementing users.RoutePublisher
  routes.go          <- endpoint handlers and route service orchestration
  lastseen.go        <- last-seen endpoint/cache adapter
  server_test.go
  auth_test.go
  client_test.go

internal/cloudflared/
  config.go          <- config model and deterministic YAML render
  manager.go         <- validate/write/reload/rollback orchestration
  manager_test.go
  config_test.go
```

`internal/api` must not import `internal/tui`. `internal/cloudflared` must not
start processes directly; validation and SIGHUP go through
`shell.Executor`, and file writes go through `shell.FS` or a narrow atomic-file
interface that can be backed by `shell.FS` in tests.

The daemon path is:

```text
HTTP request over /run/openclaw-overlay.sock
  -> peer credential lookup
  -> user/route validation
  -> state mutation in SQLite
  -> render all enabled routes into cloudflared config
  -> write temp config + backup previous config
  -> cloudflared ingress validate
  -> SIGHUP cloudflared
  -> health probe/check
  -> return JSON response or rollback and audit error
```

Cloudflared config must always include a final catch-all 404 rule. Enabled
gateway and plugin routes become ingress entries before that catch-all. Disabled
routes remain in state but are omitted from the generated config.

## 5. Atomic Tasks

| ID  | Title | Est | Depends on | Parallel | Status | PR |
| --- | ----- | --- | ---------- | -------- | ------ | -- |
| T01 | Define API request/response contract | 0.5d | - | yes | done | - |
| T02 | Add cloudflared config renderer | 0.75d | T01 | yes | done | - |
| T03 | Add atomic config write and backup helper | 0.75d | T02 | yes | done | - |
| T04 | Add cloudflared validate adapter | 0.5d | T02 | yes | done | - |
| T05 | Add cloudflared SIGHUP reload and rollback manager | 1d | T03, T04 | no | done | - |
| T06 | Add route service orchestration | 1d | T01, T05 | no | done | - |
| T07 | Add peer credential abstraction and authorization | 0.75d | T01 | yes | done | - |
| T08 | Implement HTTP router and JSON errors | 0.75d | T01, T06, T07 | no | done | - |
| T09 | Implement UNIX-socket listener | 0.75d | T07, T08 | no | done | - |
| T10 | Wire `cmd/openclaw-overlay-api` flags/config | 0.75d | T08, T09 | no | done | - |
| T11 | Implement gateway route endpoints | 0.75d | T06, T08 | yes | done | - |
| T12 | Implement plugin route endpoints | 0.75d | T06, T08 | yes | done | - |
| T13 | Implement enable/disable endpoints | 0.5d | T06, T08 | yes | done | - |
| T14 | Implement list routes and last-seen endpoints | 0.75d | T08 | yes | done | - |
| T15 | Add overlay-API UNIX-socket client | 0.75d | T08, T09 | yes | done | - |
| T16 | Wire users manager to API route publisher | 0.75d | T15 | no | done | - |
| T17 | Add audit events for API/config lifecycle | 0.5d | T06, T08 | yes | done | - |
| T18 | Update systemd template and install defaults | 0.5d | T10 | yes | done | - |
| T19 | Add Phase 6 owner-run E2E cases | 1d | T10, T11, T13 | no | done | - |
| T20 | Keep Phase 6 validation in E2E docs | 0.25d | T19 | yes | done | - |
| T21 | Document overlay-API operations | 0.5d | T11, T12, T13, T14 | yes | done | - |
| T22 | Update changelog and phase status | 0.25d | T20, T21 | no | done | - |

### T01: Define API request/response contract

**Description.** Add stable request and response structs for the Phase 6 HTTP
API. Keep JSON fields explicit and small. Do not expose internal state structs
directly as wire format.

**Acceptance criteria.**

- [ ] `internal/api` contains request/response structs for health, users,
      gateway route upsert/delete, plugin route upsert/delete, route list,
      last-seen, enable, disable, and cloudflared reload.
- [ ] Response structs include enough fields for Phase 7 watcher:
      `url`, `hostname`, `local_port`, `route_id`, `enabled`.
- [ ] Error response shape is documented and covered by unit tests.
- [ ] Contracts do not import `internal/tui`.

**Implementation notes.** Suggested endpoints match the master plan:
`GET /health`, `GET /users`, `POST /users/{u}/gateway-route`,
`DELETE /users/{u}/gateway-route`, `POST /users/{u}/routes`,
`DELETE /users/{u}/routes/{id}`, `GET /users/{u}/routes`,
`GET /users/{u}/routes/{id}/last-seen`, `POST /users/{u}/disable`,
`POST /users/{u}/enable`, `POST /cloudflared/reload`.

**Test plan.** `go test ./internal/api`.

---

### T02: Add cloudflared config renderer

**Description.** Render a complete locally-managed cloudflared config from
overlay config plus state routes.

**Acceptance criteria.**

- [ ] `internal/cloudflared` renders `tunnel`, `credentials-file`, `ingress`,
      enabled route entries, and final `http_status:404`.
- [ ] `config.OverlayConfig` has `cloudflared_credentials_file`; when empty,
      Phase 6 derives `/etc/cloudflared/<tunnel_id>.json`.
- [ ] Config publication validates that the resolved credentials file exists
      before writing `/etc/cloudflared/config.yml`.
- [ ] Disabled routes are omitted.
- [ ] Routes are sorted deterministically by hostname, kind, plugin ID, and
      route ID.
- [ ] Gateway and plugin routes render to `http://127.0.0.1:<local_port>`.
- [ ] Missing tunnel ID or credentials file is a clear validation error before
      any write.

**Implementation notes.** Add `cloudflared_credentials_file` to overlay config.
If the field is empty, derive `/etc/cloudflared/<tunnel_id>.json`; never
hard-code a credentials path without this config resolution step.

**Test plan.** Golden tests for empty, gateway-only, plugin-only, disabled, and
mixed route sets.

---

### T03: Add atomic config write and backup helper

**Description.** Add a helper that backs up the current config, writes a temp
file, fsyncs where supported, and renames into place.

**Acceptance criteria.**

- [ ] Existing config is copied to
      `/var/lib/openclaw-multi/snapshots/<timestamp>/cloudflared-config.yml`
      before replacement.
- [ ] New config is written with mode `0600` or stricter.
- [ ] Failed temp write leaves the original config untouched.
- [ ] Helper is testable without host filesystem mutation.

**Implementation notes.** Match the existing project preference for injected
filesystem seams. If `shell.FS` is too narrow for atomic rename semantics, add a
small interface in `internal/cloudflared` and provide a production adapter.

**Test plan.** Unit tests with a memory or temp filesystem adapter.

---

### T04: Add cloudflared validate adapter

**Description.** Run cloudflared ingress validation through `shell.Executor`.

**Acceptance criteria.**

- [ ] Validation command is injectable and never called directly by tests.
- [ ] Validation failure returns stderr/stdout context in a bounded error.
- [ ] Validation runs after writing candidate config and before SIGHUP.
- [ ] Unit tests cover success and failure.

**Implementation notes.** Prefer the documented `cloudflared tunnel ingress
validate` path and pass an explicit config path if the installed version
supports it in the smoke image.

**Test plan.** `go test ./internal/cloudflared`.

---

### T05: Add cloudflared SIGHUP reload and rollback manager

**Description.** Coordinate render, backup, write, validate, SIGHUP reload,
health check, and rollback.

**Acceptance criteria.**

- [ ] Success path writes config, validates it, sends SIGHUP to cloudflared,
      and returns a structured summary.
- [ ] Validation failure does not send SIGHUP.
- [ ] SIGHUP failure restores the previous config and attempts one
      recovery SIGHUP.
- [ ] Rollback outcome is visible in the returned error and audit metadata.
- [ ] Behavior is fully unit-tested with mock executor and filesystem.
- [ ] `docs/e2e-use-cases.md` includes the owner-run SIGHUP validation use
      case.

**Implementation notes.** Owner decision: use SIGHUP. Current Cloudflare Linux
service docs document restart to load changed config, so Phase 6 must treat
SIGHUP behavior as a product assumption that is explicitly verified by
owner-run VPS checks.

**Test plan.** Unit tests for success, validation fail, reload fail, rollback
success, and rollback fail.

---

### T06: Add route service orchestration

**Description.** Add a service layer that mutates route state and invokes the
cloudflared manager exactly once per committed route change.

**Acceptance criteria.**

- [ ] Gateway upsert creates route ID `gateway:<username>` and deterministic
      hostname `gateway-<username>.<subdomain>.<domain>`.
- [ ] Plugin upsert creates stable route IDs and hostnames from request fields.
- [ ] Delete removes only the addressed route after auth checks.
- [ ] Enable/disable toggles all user routes and republishes config.
- [ ] State mutation and config publication failure behavior is documented and
      tested.

**Implementation notes.** Keep this layer independent from HTTP so unit tests
can cover business behavior directly.

**Test plan.** Service unit tests using temp SQLite and fake cloudflared
publisher.

---

### T07: Add peer credential abstraction and authorization

**Description.** Add a peer credential lookup abstraction for UNIX-socket HTTP
requests and an authorization policy.

**Acceptance criteria.**

- [ ] Root UID `0` may call every endpoint.
- [ ] A non-root caller may call `/users/{u}/...` only when its UID matches
      the UID stored for that user.
- [ ] Missing user, missing peer credentials, and UID mismatch return distinct
      404/401/403 style errors.
- [ ] Unit tests do not require actual Linux credentials.

**Implementation notes.** Production code can use Linux `SO_PEERCRED` on the
accepted UNIX socket. Keep the lookup behind an interface so tests can inject
credentials.

**Test plan.** Authorization table tests for root, owner, wrong user, unknown
user, and missing credentials.

---

### T08: Implement HTTP router and JSON errors

**Description.** Implement the HTTP server surface and route handlers.

**Acceptance criteria.**

- [ ] All Phase 6 endpoints return JSON only.
- [ ] Unknown endpoints return JSON 404.
- [ ] Invalid methods return JSON 405.
- [ ] Invalid request bodies return JSON 400 without state mutation.
- [ ] Handler tests cover happy paths and error paths.

**Implementation notes.** Use Go standard `net/http` unless a strong reason
appears to add a dependency.

**Test plan.** `go test ./internal/api`.

---

### T09: Implement UNIX-socket listener

**Description.** Bind the daemon to a filesystem UNIX socket with strict
permissions.

**Acceptance criteria.**

- [ ] Default socket path is `/run/openclaw-overlay.sock`.
- [ ] Stale socket cleanup is safe and refuses to remove non-socket files.
- [ ] Socket permissions allow managed users to connect; every route operation
      is authorized by daemon-side `SO_PEERCRED`, not by trusting socket
      permissions alone.
- [ ] Shutdown closes the listener and removes the socket.
- [ ] Unit tests cover stale socket and non-socket refusal.

**Implementation notes.** Avoid abstract sockets; filesystem sockets make
permissions and operational debugging clearer.

**Test plan.** Unit tests using temp directories.

---

### T10: Wire `cmd/openclaw-overlay-api` flags/config

**Description.** Replace the Phase 0 panic with daemon startup and CLI flags.

**Acceptance criteria.**

- [ ] `openclaw-overlay-api --help` exits successfully.
- [ ] Flags include config path, state DB path, socket path, cloudflared config
      path, audit log path, and a foreground/test mode if needed.
- [ ] Startup opens config, state, audit log, listener, and HTTP server.
- [ ] SIGINT/SIGTERM trigger graceful shutdown.
- [ ] Startup emits `startup`; shutdown emits `shutdown`.

**Test plan.** Build test plus command-level test for `--help` and invalid flag
handling.

---

### T11: Implement gateway route endpoints

**Description.** Implement create/update/delete for the per-user Control UI
route.

**Acceptance criteria.**

- [ ] `POST /users/{u}/gateway-route` upserts the gateway route.
- [ ] `DELETE /users/{u}/gateway-route` deletes only the gateway route.
- [ ] Responses include the public HTTPS URL and local port.
- [ ] Calls republish cloudflared config through the route service.
- [ ] Tests cover root caller and owner caller.

**Test plan.** Handler/service tests plus owner-run E2E route publication.

---

### T12: Implement plugin route endpoints

**Description.** Implement create/update/delete for plugin callback ingress
routes.

**Acceptance criteria.**

- [ ] `POST /users/{u}/routes` accepts plugin ID, hostname hint, and local port.
- [ ] The daemon, not the client/watcher, derives plugin route IDs from
      `(username, plugin_id, hostname_hint)`.
- [ ] Hostname generation is deterministic and constrained to the configured
      subdomain/domain.
- [ ] Duplicate plugin/hint updates the existing route instead of creating
      ambiguous duplicates.
- [ ] `DELETE /users/{u}/routes/{id}` removes only that plugin route.
- [ ] Gateway routes cannot be deleted through the plugin route delete path.

**Implementation notes.** Keep route ID and hostname rules conservative; Phase
7 watcher can adapt to this contract.

**Test plan.** Handler/service tests for create, update, delete, conflict, and
invalid hostname hint.

---

### T13: Implement enable/disable endpoints

**Description.** Implement route toggling used by user activate/deactivate.

**Acceptance criteria.**

- [ ] `POST /users/{u}/disable` marks all user routes disabled and republishes.
- [ ] `POST /users/{u}/enable` marks all user routes enabled and republishes.
- [ ] Endpoints are idempotent.
- [ ] User status mutation remains owned by `internal/users`; API endpoints
      only mutate route enabled state.

**Test plan.** Service and handler tests.

---

### T14: Implement list routes and last-seen endpoints

**Description.** Expose route inventory and cached last-seen data through the
daemon.

**Acceptance criteria.**

- [ ] `GET /users/{u}/routes` returns gateway and plugin routes sorted
      deterministically.
- [ ] `GET /users/{u}/routes/{id}/last-seen` returns timestamp or `null`.
- [ ] Last-seen refresh reuses Phase 5 parser/cache behavior where possible.
- [ ] Cloudflared logs without usable access info return `null`, not an error.

**Test plan.** Handler tests with fake last-seen provider and parser tests if
new parsing behavior is added.

---

### T15: Add overlay-API UNIX-socket client

**Description.** Add a client that can call the daemon over the UNIX socket and
implement `users.RoutePublisher`.

**Acceptance criteria.**

- [ ] Client uses `http.Transport.DialContext` for UNIX sockets.
- [ ] Client implements gateway enable, route disable, and route delete methods
      needed by `users.RoutePublisher`.
- [ ] Client maps JSON API errors to useful Go errors.
- [ ] Tests use `httptest` or a temp UNIX socket server.

**Test plan.** `go test ./internal/api ./internal/users`.

---

### T16: Wire users manager to API route publisher

**Description.** Use the API route publisher in production wiring while keeping
the state-only publisher available for tests.

**Acceptance criteria.**

- [ ] Add-user publishes the gateway route through overlay-API in production
      wiring.
- [ ] Activate/deactivate/remove route operations call overlay-API in
      production wiring.
- [ ] Existing users manager tests remain deterministic with
      `StateRoutePublisher` or a fake publisher.
- [ ] If overlay-API route publication fails during add-user, the existing
      rollback behavior still removes user state created in the operation.

**Implementation notes.** Be surgical: do not redesign user lifecycle in this
task.

**Test plan.** Existing `internal/users` tests plus one production-wiring test.

---

### T17: Add audit events for API/config lifecycle

**Description.** Add audit actions for overlay-API route and cloudflared config
publication events.

**Acceptance criteria.**

- [ ] Add actions for `overlay_api_request`, `cloudflared_config_publish`, and
      `cloudflared_sighup` or clearer project-consistent names.
- [ ] Route mutation success/error/rollback paths emit audit events.
- [ ] Audit entries include actor identity as `root` or UID/username where
      known.
- [ ] Tests assert emitted action/result in at least one success and one
      rollback path.

**Test plan.** Unit tests with fake auditor.

---

### T18: Update systemd template and install defaults

**Description.** Make the existing systemd unit suitable for the real daemon.

**Acceptance criteria.**

- [ ] Unit passes explicit config/state/socket paths.
- [ ] Unit has a runtime directory or pre-start behavior that supports
      `/run/openclaw-overlay.sock`.
- [ ] Unit keeps `Restart=always`.
- [ ] Fresh install tests are updated for the changed template.

**Implementation notes.** Avoid broad sandboxing hardening in this phase unless
it is necessary for the daemon to run; full service hardening can be Phase 9.

**Test plan.** Template rendering tests; run `systemd-analyze verify` manually
when validating the VPS service.

---

### T19: Add Phase 6 owner-run E2E cases

**Description.** Add owner-run E2E use cases for the daemon/socket/config
workflow. The owner runs these manually on the target VPS.

**Acceptance criteria.**

- [ ] `docs/e2e-use-cases.md` covers SIGHUP reload validation.
- [ ] `docs/e2e-use-cases.md` covers user/admin/root authorization.
- [ ] `docs/e2e-use-cases.md` covers cloudflared config rollback.
- [ ] `docs/e2e-use-cases.md` covers daemon-derived plugin route IDs.
- [ ] `docs/e2e-use-cases.md` covers TUI sign-up publishing a gateway route
      through overlay-API.

**Implementation notes.** The owner validates Phase 6 manually from the E2E
checklist.

**Test plan.** Review `docs/e2e-use-cases.md` Phase 6 section.

---

### T20: Keep Phase 6 validation in E2E docs

**Description.** Keep Phase 6 validation steps in the owner-run E2E checklist.

**Acceptance criteria.**

- [ ] Phase 6 validation steps live in `docs/e2e-use-cases.md`.
- [ ] The phase doc points to the owner-run E2E checklist.

**Test plan.** Review Phase 6 references to `docs/e2e-use-cases.md`.

---

### T21: Document overlay-API operations

**Description.** Add operator and developer documentation for the daemon.

**Acceptance criteria.**

- [ ] Create `docs/overlay-api.md`.
- [ ] Document socket path, endpoints, authorization model, config publication,
      rollback behavior, and test boundary.
- [ ] Update `docs/network.md` to state that Phase 6 owns cloudflared config
      publication.
- [ ] Inline godoc exists on new public package APIs.

**Test plan.** Manual doc review plus link check by `rg` for stale Phase 5
"Phase 6 owns" wording.

---

### T22: Update changelog and phase status

**Description.** Update release notes and phase metadata after implementation.

**Acceptance criteria.**

- [ ] `CHANGELOG.md` has a `## v0.6.0` section.
- [ ] This phase doc task statuses are updated to `completed`.
- [ ] Retrospective `docs/phases/phase-6-retro.md` is written after
      verification.

**Test plan.** Final phase verification checklist.

## 6. Definition of Done for the Phase

- [ ] All tasks T01..T22 have status `completed`.
- [ ] All acceptance criteria checked.
- [ ] `make ci` passes on `main`.
- [ ] `cmd/openclaw-overlay-api` no longer panics as a stub.
- [ ] The daemon can publish, disable, re-enable, and delete gateway and plugin
      routes through the UNIX-socket API.
- [ ] Unauthorized peer credentials cannot manage another user's routes.
- [ ] Cloudflared config publication has tested validation and rollback paths.
- [ ] Documentation deliverables (§8) merged.
- [ ] Release `v0.6.0` tagged.
- [ ] Retrospective `docs/phases/phase-6-retro.md` written.

## 7. Owner-Run E2E Checks

Owner-run checks live in `docs/e2e-use-cases.md`.

| Use case | What it proves |
| -------- | -------------- |
| `UC-0601` | SIGHUP reload applies local cloudflared config on the target VPS. |
| `UC-0602` | `SO_PEERCRED` authorization allows owner/admin/root and rejects wrong users. |
| `UC-0603` | Invalid cloudflared config is not activated and rollback is visible. |
| `UC-0604` | Plugin route IDs are daemon-derived and idempotent. |
| `UC-0605` | TUI sign-up publishes gateway routes through overlay-API. |

## 8. Documentation Deliverables

- [ ] `docs/overlay-api.md` created.
- [ ] `docs/network.md` updated for Phase 6 ownership of cloudflared config.
- [ ] `docs/architecture.md` updated if endpoint/socket details change.
- [ ] `docs/e2e-use-cases.md` updated with Phase 6 owner-run VPS checks.
- [ ] `CHANGELOG.md` updated under `## v0.6.0`.
- [ ] Inline godoc on new public APIs in `internal/api` and
      `internal/cloudflared`.

## 9. Risks and Mitigations

| Risk | Mitigation |
| ---- | ---------- |
| Cloudflare docs do not document SIGHUP as the config reload path | Owner decision is SIGHUP; verify it with `docs/e2e-use-cases.md` owner-run VPS validation. |
| Config write succeeds but SIGHUP reload fails | Always keep a timestamped backup and test rollback plus one recovery SIGHUP. |
| Peer credential extraction is hard to test portably | Hide it behind an interface and unit-test authorization separately from Linux-specific socket plumbing. |
| Route state and cloudflared config diverge | Route service owns all route mutations and invokes one publication path after every committed change. |
| Phase 7 watcher needs a different plugin route contract | Keep Phase 6 plugin request/response minimal and deterministic; document it in `docs/overlay-api.md` before watcher work starts. |

## 10. Phase Decisions

### D01: Cloudflared Reload Uses SIGHUP

Decision: Phase 6 uses SIGHUP to ask cloudflared to reload the local ingress
config. This is an owner decision even though the current Cloudflare Linux
service docs emphasize service restart for config changes. The implementation
must include unit coverage and `docs/e2e-use-cases.md` contains an owner-run VPS
check that proves SIGHUP behavior on the target server.

If the VPS check fails, reopen this decision before continuing Phase 7.

### D02: Plugin Route IDs Are Daemon-Derived

Decision: clients and the Phase 7 watcher do not supply canonical route IDs.
For plugin routes, overlay-API derives the route ID from
`(username, plugin_id, hostname_hint)` after validating and normalizing those
inputs. Repeated calls with the same tuple are idempotent updates to the same
route. This keeps route identity under overlay control and avoids leaking
client-side ID rules into state.

If Phase 7 discovers that one plugin needs multiple callbacks with the same
`plugin_id` and `hostname_hint`, do not switch to arbitrary client-supplied IDs
silently. Add an explicit extra discriminator to the API contract and document
the migration.

### D03: Managed Users Can Connect; `SO_PEERCRED` Authorizes

Decision: the UNIX socket must be reachable by managed user processes and the
configured admin. The daemon must authorize every user-scoped route operation
with Linux peer credentials. Root UID `0` and the stored admin UID may manage
all users. Other non-root callers may manage only the user whose UID matches
the stored managed user UID.

Phase 6 does not introduce an `openclaw-overlay` Linux group as an authorization
requirement. Group restriction may be added later in Phase 9 as defense in
depth, but it must not replace `SO_PEERCRED` checks.

### D04: Explicit `cloudflared_credentials_file`

Decision: Phase 6 adds `cloudflared_credentials_file` to overlay config. If the
field is empty, the default is derived as
`/etc/cloudflared/<tunnel_id>.json`. Before publishing cloudflared config,
overlay-API must verify that the resolved credentials file exists. If it does
not exist, publication fails before writing `/etc/cloudflared/config.yml` or
signaling cloudflared.

This field affects only the local cloudflared config managed by
openclaw-multi. It does not mutate the Cloudflare account or other VPS hosts.
If this VPS already has a custom cloudflared service/config, the explicit field
prevents accidental path guessing and makes the operator's intended credentials
file reviewable.
