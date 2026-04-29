---
phase: 7
title: "Per-User Overlay Watcher"
status: implemented
created_at: 2026-04-29
approved_by: ""
approved_at: ""
master_plan_section:
  - "../OPENCLAW_OVERLAY_PLAN_RU.md#8-per-user-watcher"
  - "../OPENCLAW_OVERLAY_PLAN_RU.md#7-overlay-api-daemon"
prior_retros:
  - "phase-6-retro.md"
external_docs_checked:
  - "https://pkg.go.dev/github.com/fsnotify/fsnotify"
---

# Phase 7: Per-User Overlay Watcher

> **User-boundary phase.** Phase 7 replaces the `openclaw-overlay-watcher` stub
> with a per-user daemon. The watcher runs under the managed user's UID, watches
> that user's OpenClaw config, asks overlay-API to publish plugin callback
> routes, and writes returned callback URL/port values back through OpenClaw CLI
> commands. Owner-run E2E checks live in `docs/e2e-use-cases.md`; Phase 7 does
> not add smoke scripts.

---

## 1. Goals

1. Replace `cmd/openclaw-overlay-watcher` panic with a real long-running daemon.
2. Watch `~/.openclaw/openclaw.json` for plugin entry changes using a
   testable watcher abstraction backed by `fsnotify` in production.
3. Parse plugin entries and detect callback-capable plugins without depending on
   OpenClaw internals beyond documented config shape used in this phase.
4. Call overlay-API plugin route endpoints as the managed user and rely on
   daemon-derived route IDs.
5. Write returned callback URL/port into OpenClaw config through injected
   `openclaw config set` commands.
6. Persist watcher snapshot state in `~/.openclaw-overlay/watcher.state`.
7. Keep Phase 7 validation in owner-run E2E use cases, not scripts.
8. `make ci` passes and every task has acceptance evidence.

## 2. Out of Scope

- Changing Phase 6 route identity rules. Plugin route IDs remain
  daemon-derived from `(username, plugin_id, hostname_hint)`.
- Adding arbitrary client-supplied route IDs.
- Implementing Phase 8 live logs or journal viewer.
- Implementing Phase 9 security audit screen, socket group hardening, or broad
  systemd hardening.
- Implementing OpenClaw plugin marketplace logic. User still installs plugins
  through normal OpenClaw commands.
- Adding automated Phase 7 smoke scripts or `make test-phase-7`.

## 3. Inputs

- Current code state after Phase 6 implementation.
- Phase 6 decisions:
  - overlay-API listens on UNIX socket and authorizes with `SO_PEERCRED`;
  - managed users may connect to the socket;
  - plugin route IDs are daemon-derived;
  - cloudflared reload uses SIGHUP.
- Existing code surfaces:
  - `cmd/openclaw-overlay-watcher/main.go` is still a stub.
  - `templates/openclaw-overlay-watcher.service.tmpl` starts the watcher as a
    user service.
  - `internal/api.Client` exists but currently only covers gateway lifecycle.
  - `POST /users/<username>/routes` accepts `plugin_id`, `hostname_hint`, and
    `local_port`.
  - `state.routes` already supports plugin routes.
- External constraints checked:
  - `fsnotify` supports Linux/inotify and emits `Create`, `Write`, `Remove`,
    `Rename`, and `Chmod` events.
  - fsnotify docs warn that one write can produce multiple events and Linux
    remove behavior can first appear as `Chmod`; Phase 7 must debounce and
    rescan rather than trust one event as a complete write.

## 4. Architecture for This Phase

Add one focused package:

```text
internal/watcher/
  config.go          <- OpenClaw config parsing and callback route discovery
  snapshot.go        <- watcher.state load/save and diff logic
  service.go         <- sync orchestration
  fsnotify.go        <- production file watcher adapter
  openclaw.go        <- injected OpenClaw config set adapter
  service_test.go
  config_test.go
  snapshot_test.go
```

Daemon path:

```text
start as managed user
  -> resolve username/home/config paths
  -> load watcher.state snapshot
  -> initial scan of ~/.openclaw/openclaw.json
  -> discover callback-capable plugin entries
  -> POST /users/<self>/routes to overlay-API
  -> receive daemon-derived route_id, url, local_port
  -> openclaw config set callbackUrl/callbackPort
  -> save watcher.state
  -> watch config directory for create/write/rename/remove events
  -> debounce and rescan
```

The watcher must watch the parent directory, not only the file, so atomic
replace/rename of `openclaw.json` is observed. Unit tests must drive watcher
logic through fake event sources and fake executors.

## 5. Atomic Tasks

| ID  | Title | Est | Depends on | Parallel | Status | PR |
| --- | ----- | --- | ---------- | -------- | ------ | -- |
| T01 | Define watcher config model | 0.75d | - | yes | done | - |
| T02 | Define callback route discovery contract | 0.75d | T01 | yes | done | - |
| T03 | Add watcher snapshot state | 0.75d | T01 | yes | done | - |
| T04 | Add overlay-API plugin client methods | 0.5d | Phase 6 | yes | done | - |
| T05 | Add OpenClaw config writer adapter | 0.75d | T02 | yes | done | - |
| T06 | Add sync service orchestration | 1d | T02, T03, T04, T05 | no | done | - |
| T07 | Add fsnotify watcher adapter and debounce loop | 1d | T06 | no | done | - |
| T08 | Wire `cmd/openclaw-overlay-watcher` flags/startup | 0.75d | T06, T07 | no | done | - |
| T09 | Update watcher systemd template | 0.5d | T08 | yes | done | - |
| T10 | Add unit tests for parser/snapshot/sync | 1d | T01..T06 | no | done | - |
| T11 | Add owner-run Phase 7 E2E cases | 0.5d | T08, T09 | yes | done | - |
| T12 | Update docs/changelog/retro scaffolding | 0.5d | T11 | no | done | - |

### T01: Define watcher config model

**Description.** Add structs and parser for the subset of
`~/.openclaw/openclaw.json` needed by the watcher.

**Acceptance criteria.**

- [ ] Parser accepts valid OpenClaw JSON with `plugins.entries`.
- [ ] Parser rejects malformed JSON with clear errors.
- [ ] Parser preserves plugin entry IDs exactly as plugin IDs.
- [ ] Parser tolerates unknown fields.
- [ ] Tests cover empty config, malformed config, one plugin, multiple plugins,
      and unknown fields.

**Evidence required.** File paths, parser tests, and sample JSON fixtures.

**Test plan.** `go test ./internal/watcher`.

---

### T02: Define callback route discovery contract

**Description.** Detect plugin entries that require overlay callback routes.

**Acceptance criteria.**

- [ ] Discovery returns `plugin_id`, `hostname_hint`, and `local_port`.
- [ ] Discovery ignores plugins without callback route requirements.
- [ ] Discovery uses stable hostname hints.
- [ ] Discovery validates local port range and rejects zero/invalid ports.
- [ ] If the real OpenClaw config shape does not expose enough data, this task
      records the exact missing field in this phase doc before implementation
      continues.

**Implementation notes.** Prefer an explicit config contract over heuristic JSON
search. Do not introduce client-supplied route IDs.

**Evidence required.** Contract examples and tests mapping JSON input to route
requests.

**Test plan.** Table-driven tests in `internal/watcher`.

---

### T03: Add watcher snapshot state

**Description.** Store the last synced plugin callback state in
`~/.openclaw-overlay/watcher.state`.

**Acceptance criteria.**

- [ ] Snapshot includes plugin ID, hostname hint, local port, route ID, URL, and
      config write status.
- [ ] Snapshot load handles missing file as empty state.
- [ ] Snapshot write creates parent directory with `0700`.
- [ ] Snapshot file mode is `0600`.
- [ ] Diff logic detects added, updated, unchanged, and removed plugin routes.

**Evidence required.** Snapshot file format and unit tests.

**Test plan.** `go test ./internal/watcher`.

---

### T04: Add overlay-API plugin client methods

**Description.** Extend `internal/api.Client` with plugin route operations for
watcher use.

**Acceptance criteria.**

- [ ] Client can upsert plugin route with `plugin_id`, `hostname_hint`, and
      `local_port`.
- [ ] Client returns `route_id`, `url`, hostname, local port, and enabled flag.
- [ ] Client can delete a plugin route by returned `route_id`.
- [ ] Client does not accept caller-supplied canonical route IDs.
- [ ] Tests cover success and JSON error response.

**Evidence required.** Client tests and call sites.

**Test plan.** `go test ./internal/api`.

---

### T05: Add OpenClaw config writer adapter

**Description.** Add an injected adapter that writes callback values through
OpenClaw CLI commands.

**Acceptance criteria.**

- [ ] Adapter writes callback URL via `openclaw config set`.
- [ ] Adapter writes callback port via `openclaw config set`.
- [ ] Commands are invoked through `shell.Executor`.
- [ ] Tests assert exact command argv without running OpenClaw.
- [ ] Failures return clear errors and do not update watcher snapshot as synced.

**Evidence required.** Fake executor tests.

**Test plan.** `go test ./internal/watcher`.

---

### T06: Add sync service orchestration

**Description.** Implement one sync pass from config parse to overlay-API route
publication, OpenClaw config write, and snapshot update.

**Acceptance criteria.**

- [ ] Initial sync publishes discovered callback routes.
- [ ] Repeated sync is idempotent.
- [ ] Changed local port updates the same daemon-derived route.
- [ ] Removed plugin deletes or disables the route according to the Phase 7
      implementation decision recorded in this task.
- [ ] Overlay-API errors do not write callback values into OpenClaw config.
- [ ] OpenClaw config write errors do not mark snapshot entry as synced.

**Implementation notes.** Phase 7 deletes plugin routes for removed plugins.
Gateway route enable/disable remains Phase 6 user lifecycle behavior.

**Evidence required.** Service tests for success, idempotency, changed port,
removed plugin, API error, and config write error.

**Test plan.** `go test ./internal/watcher`.

---

### T07: Add fsnotify watcher adapter and debounce loop

**Description.** Add production filesystem event adapter and debounce/rescan
loop.

**Acceptance criteria.**

- [ ] Production adapter uses `github.com/fsnotify/fsnotify`.
- [ ] Watcher watches the config directory and filters events for
      `openclaw.json`.
- [ ] `Write`, `Create`, `Rename`, and `Remove` trigger debounced rescan.
- [ ] Multiple events for one write collapse into one sync attempt.
- [ ] Watcher handles atomic replace by continuing to watch the directory.
- [ ] Watcher exits cleanly on context cancellation and closes fsnotify watcher.

**Evidence required.** Fake event source tests; production adapter reviewed for
fsnotify close/error handling.

**Test plan.** `go test ./internal/watcher`.

---

### T08: Wire `cmd/openclaw-overlay-watcher` flags/startup

**Description.** Replace the Phase 0 panic with daemon startup.

**Acceptance criteria.**

- [ ] `openclaw-overlay-watcher --help` exits successfully.
- [ ] Flags include config path, snapshot path, overlay socket path, username,
      and debounce duration.
- [ ] Defaults resolve to the current user's home:
      `~/.openclaw/openclaw.json` and `~/.openclaw-overlay/watcher.state`.
- [ ] Startup performs one sync before entering watch loop.
- [ ] SIGINT/SIGTERM cancel the watcher cleanly.
- [ ] Startup and sync errors are logged to stderr/journal.

**Evidence required.** Command tests for help/default path behavior and invalid
config behavior.

**Test plan.** `go test ./cmd/openclaw-overlay-watcher ./internal/watcher`.

---

### T09: Update watcher systemd template

**Description.** Update `templates/openclaw-overlay-watcher.service.tmpl` for
the real watcher.

**Acceptance criteria.**

- [ ] Unit passes explicit config, snapshot, and socket paths where needed.
- [ ] Unit runs as user service and does not require root.
- [ ] Unit keeps `Restart=on-failure`.
- [ ] Unit has enough environment for OpenClaw CLI lookup.
- [ ] Template rendering tests are updated if existing tests cover it.

**Evidence required.** Template diff and tests or manual rendering evidence.

**Test plan.** `go test ./internal/users` and final `make ci`.

---

### T10: Add unit tests for parser/snapshot/sync

**Description.** Ensure watcher behavior is proven without real inotify,
OpenClaw, cloudflared, or systemd.

**Acceptance criteria.**

- [ ] `internal/watcher` coverage is at least 80%.
- [ ] Tests do not call real `openclaw`, `systemctl`, cloudflared, or live
      overlay-API.
- [ ] Tests cover every error path listed in T01..T07.
- [ ] Dead-code pass shows no unused exported constants/functions/helpers.

**Evidence required.** Test output, coverage output, and dead-code search notes.

**Test plan.** `go test -cover ./internal/watcher`.

---

### T11: Add owner-run Phase 7 E2E cases

**Description.** Add Phase 7 manual/VPS validation cases to
`docs/e2e-use-cases.md`.

**Acceptance criteria.**

- [ ] E2E covers plugin install/config change triggering watcher sync.
- [ ] E2E covers idempotent resync.
- [ ] E2E covers plugin removal disabling or deleting callback route.
- [ ] E2E covers watcher restart using persisted snapshot.
- [ ] E2E contains only preconditions, steps, expected results, and
      capture-on-failure.

**Evidence required.** Link to `docs/e2e-use-cases.md` section.

**Test plan.** Manual review.

---

### T12: Update docs/changelog/retro scaffolding

**Description.** Update operator docs and release notes after implementation.

**Acceptance criteria.**

- [ ] `docs/overlay-api.md` updated if client contract changes.
- [ ] New `docs/watcher.md` documents watcher behavior and troubleshooting.
- [ ] `CHANGELOG.md` has a `## v0.7.0` section.
- [ ] This phase doc task statuses are updated only after evidence matrix is
      complete.
- [ ] `docs/phases/phase-7-retro.md` is written after verification.

**Evidence required.** Documentation diffs and final evidence matrix.

**Test plan.** Final phase verification checklist.

## 6. Definition of Done for the Phase

- [ ] All tasks T01..T12 have status `completed`.
- [ ] Every task has an acceptance evidence matrix.
- [ ] `make ci` passes on `main`.
- [ ] `cmd/openclaw-overlay-watcher` no longer panics as a stub.
- [ ] Watcher can publish plugin callback routes through overlay-API.
- [ ] Watcher relies on daemon-derived route IDs.
- [ ] Watcher persists and reloads `watcher.state`.
- [ ] Watcher writes callback URL/port through injected OpenClaw commands.
- [ ] Owner-run Phase 7 E2E cases are documented.
- [ ] New exported symbols/helpers are used, tested, or explicitly documented
      as interface surface.
- [ ] Documentation deliverables (§8) merged.
- [ ] Retrospective `docs/phases/phase-7-retro.md` written.

## 7. Owner-Run E2E Checks

Owner-run checks live in `docs/e2e-use-cases.md`.

| Use case | What it proves |
| -------- | -------------- |
| `UC-0701` | Plugin callback route is published by watcher after config change. |
| `UC-0702` | Repeated watcher sync is idempotent. |
| `UC-0703` | Plugin removal removes or disables callback route. |
| `UC-0704` | Watcher restart uses persisted snapshot and does not duplicate routes. |

## 8. Documentation Deliverables

- [ ] `docs/watcher.md` created.
- [ ] `docs/overlay-api.md` updated if plugin client behavior changes.
- [ ] `docs/e2e-use-cases.md` updated with Phase 7 owner-run checks.
- [ ] `CHANGELOG.md` updated under `## v0.7.0`.
- [ ] Inline godoc on new public APIs in `internal/watcher`.

## 9. Risks and Mitigations

| Risk | Mitigation |
| ---- | ---------- |
| OpenClaw config shape for plugin callbacks differs from the assumed subset | Stop implementation at T02, record exact observed config shape, and update this phase doc before continuing. |
| fsnotify emits multiple events or misses atomic file replacement when watching only the file | Watch the parent directory, filter for `openclaw.json`, debounce, and rescan full config. |
| Watcher writes callback values before overlay-API publication succeeds | Sync service must publish first and write OpenClaw config only after route response succeeds. |
| Duplicate routes after watcher restart | Persist route ID/URL snapshot and rely on daemon-derived route idempotency. |
| Removed plugin leaves stale public callback URL | T06 must explicitly choose delete or disable behavior and test it. |
| User process cannot access overlay socket | Phase 6 decision allows managed users; E2E must verify `SO_PEERCRED` user path on VPS. |

## 10. Phase Decisions

### D01: Watch Directory, Not Only File

Decision: production watcher watches the directory containing
`~/.openclaw/openclaw.json` and filters relevant file events. This handles
atomic replace/rename patterns better than watching only the file.

### D02: Full Rescan After Debounce

Decision: events only trigger a debounced full rescan. The watcher does not
derive behavior from a single event type because fsnotify can emit multiple
events per write and Linux remove behavior can appear as `Chmod` before
`Remove`.

### D03: Watcher Does Not Own Route IDs

Decision: watcher never sends canonical route IDs. It sends
`plugin_id`, `hostname_hint`, and `local_port`; overlay-API returns the
daemon-derived `route_id`.

### D04: Validation Is Owner-Run E2E, Not Scripts

Decision: Phase 7 does not add smoke scripts or Makefile phase targets. The
owner validates watcher behavior from `docs/e2e-use-cases.md`.

### D05: Removed Plugin Deletes Plugin Route

Decision: when a plugin entry disappears from `openclaw.json`, watcher deletes
the plugin route returned by overlay-API. It also removes the snapshot entry.
This keeps public callback ingress aligned with installed plugin state.

## 11. Acceptance Evidence Matrix

| Task | Evidence |
| ---- | -------- |
| T01 | `internal/watcher/config.go`; `TestParseConfigDiscoversCallbacks`, `TestParseConfigRejectsMalformedJSON`, `TestParseConfigEmptyIsValid`. |
| T02 | `internal/watcher/config.go`; `TestDiscoverCallbacksUsesExplicitOverlayContract`, `TestDiscoverCallbacksParsesStringPort`, `TestDiscoverCallbacksRejectsInvalidPort`. |
| T03 | `internal/watcher/snapshot.go`; `TestLoadSnapshotMissingIsEmpty`, `TestSaveSnapshotWritesMode0600`, `TestDiffSnapshot`, save error tests. |
| T04 | `internal/api/client.go`; `TestClientUpsertPluginRoute`, `TestClientDeletePluginRoute`, `TestClientPluginRouteError`. |
| T05 | `internal/watcher/openclaw.go`; `TestOpenClawWriterSetsURLAndPort`, `TestOpenClawWriterStopsOnURLFailure`. |
| T06 | `internal/watcher/service.go`; `TestServiceSyncPublishesAndWritesSnapshot`, `TestServiceSyncIsIdempotent`, `TestServiceSyncDeletesRemovedPlugin`, API/write error tests. |
| T07 | `internal/watcher/fsnotify.go`; `TestLoopDebouncesEvents`, `TestLoopHandlesSourceErrors`, `TestFSNotifySourceReceivesFileEvent`. |
| T08 | `cmd/openclaw-overlay-watcher/main.go`; `TestRunHelp`, `TestRunInvalidFlag`; build covered by `make ci`. |
| T09 | `templates/openclaw-overlay-watcher.service.tmpl`, `internal/users/manager.go`; `go test ./internal/users`; build covered by `make ci`. |
| T10 | `go test -cover ./internal/watcher` reports 85.1% statement coverage. |
| T11 | `docs/e2e-use-cases.md` contains `UC-0701`..`UC-0704`; no Phase 7 smoke script or Makefile target added. |
| T12 | `docs/watcher.md`, `docs/overlay-api.md`, `CHANGELOG.md`, and `docs/phases/phase-7-retro.md`; final `make ci` passed. |

Verification commands:

```text
GOCACHE=/tmp/codex-go-cache GOTMPDIR=/tmp/codex-go-tmp go test -coverprofile=/tmp/watcher.cover ./internal/watcher
GOCACHE=/tmp/codex-go-cache GOTMPDIR=/tmp/codex-go-tmp GOLANGCI_LINT_CACHE=/tmp/codex-golangci-cache make ci
```
