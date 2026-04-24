---
phase: 0
title: "Skeleton"
slug: skeleton
estimated_duration: "1 week (5 working days)"
status: draft
approved_at: null
approved_by: null
master_plan_section: "§9 — Phase 0. Skeleton (1 week)"
prior_retros: []
---

# Phase 0: Skeleton

> Project bootstrap. After this phase: a Go monorepo with three
> binaries (TUI, daemon, watcher) that compile, lint clean, run a
> stub TUI menu, and ship via CI as installable artifacts on
> `linux-amd64` and `linux-arm64`. No real functionality yet — every
> menu action prints "Not implemented yet".
>
> Master architecture: see [`OPENCLAW_OVERLAY_PLAN_RU.md`](./OPENCLAW_OVERLAY_PLAN_RU.md)
> §3-5, §9 (Phase 0).
> Workflow: see [`OPENCLAW_OVERLAY_DEV_PROCESS.md`](./OPENCLAW_OVERLAY_DEV_PROCESS.md).

---

## 1. Goals

After this phase:

1. The `openclaw-multi` repository exists with a clean Go module
   structure (per master plan §5).
2. Three binaries (`openclaw-multi`, `openclaw-overlay-api`,
   `openclaw-overlay-watcher`) compile cleanly and run without
   errors (binaries are stubs — no real functionality).
3. The TUI binary opens the main menu (10 items per master plan §6.1),
   navigation works (↑/↓/Enter/q), every item shows a "not implemented
   yet" placeholder.
4. SQLite `state.db` schema is designed, migration runner works,
   admin record can be persisted.
5. Audit log writer emits valid JSONL entries (per master plan
   §12 OQ-1).
6. CI pipeline (lint + test + build × 2 arches) runs green on `main`.
7. Smoke test in Docker confirms the binary boots, opens the TUI,
   navigates the menu, and exits cleanly.

---

## 2. Out of scope

- Any real functionality behind menu items (Phases 1-10 cover those).
- `cloudflared` integration (Phase 5/6).
- Per-user `useradd` / `loginctl` (Phase 2).
- Backup/restore (Phase 3).
- Production hardening (Phase 9).
- Documentation for end-users (just a `README.md` skeleton + arch link).

---

## 3. Inputs

- Code state at start: empty `openclaw-multi` repo.
- Master plan section: §9 Phase 0, §5 directory structure, §6.1 menu.
- Prior retros: none (first phase).
- Open questions resolved (§12 OQ-1, OQ-2, OQ-3 already locked in
  master plan): JSONL audit format / master-key encryption /
  Telegram notifications.

---

## 4. Architecture for this phase

Skeleton only. Three Go binaries, no IPC between them yet (added in
Phases 6-7). One shared `internal/` package layer.

```
cmd/openclaw-multi/main.go                       ← TUI entrypoint
cmd/openclaw-overlay-api/main.go                 ← daemon entrypoint (stub)
cmd/openclaw-overlay-watcher/main.go             ← watcher entrypoint (stub)

internal/tui/                                    ← Bubble Tea models
  app.go             ← root model
  mainmenu.go        ← main menu model
  styles.go          ← Lip Gloss styles
  messages.go        ← message types

internal/state/                                  ← SQLite layer
  store.go           ← public API
  migrations.go      ← migration runner
  models.go          ← Go structs matching tables

internal/audit/                                  ← JSONL writer
  log.go
  events.go          ← event type enums

internal/admin/                                  ← admin identification
  resolver.go        ← read $SUDO_USER / $USER, persist
```

Key dependencies (pinned in `go.mod`):

| Module                               | Version       | Why                                          |
| ------------------------------------ | ------------- | -------------------------------------------- |
| `github.com/charmbracelet/bubbletea` | latest stable | TUI runtime                                  |
| `github.com/charmbracelet/lipgloss`  | latest stable | TUI styling                                  |
| `github.com/charmbracelet/bubbles`   | latest stable | reusable TUI components (list, viewport)     |
| `modernc.org/sqlite`                 | latest stable | pure-Go SQLite (no CGo, simpler cross-build) |
| `gopkg.in/yaml.v3`                   | latest stable | YAML config parsing                          |

No use of `cgo` anywhere in Phase 0 — keeps cross-compilation trivial.

---

## 5. Atomic tasks

### Task table

| ID  | Title                                                | Est   | Depends on | Parallel  | Status  | PR  |
| --- | ---------------------------------------------------- | ----- | ---------- | --------- | ------- | --- |
| T01 | Initialize repo with LICENSE + README + .gitignore   | 0.25d | —          | no        | pending | —   |
| T02 | Bootstrap Go module + project layout                 | 0.5d  | T01        | no        | pending | —   |
| T03 | Add Makefile (build/test/lint/ci targets)            | 0.5d  | T02        | no        | pending | —   |
| T04 | Set up golangci-lint config                          | 0.25d | T02        | yes (T05) | pending | —   |
| T05 | Add `.editorconfig` + `.gitattributes`               | 0.25d | T01        | yes (T04) | pending | —   |
| T06 | Stub `cmd/openclaw-overlay-api/main.go`              | 0.25d | T02        | yes       | pending | —   |
| T07 | Stub `cmd/openclaw-overlay-watcher/main.go`          | 0.25d | T02        | yes       | pending | —   |
| T08 | Skeleton `cmd/openclaw-multi/main.go` (no TUI yet)   | 0.25d | T02        | yes       | pending | —   |
| T09 | Add Bubble Tea + Lip Gloss + Bubbles deps            | 0.25d | T08        | no        | pending | —   |
| T10 | TUI: root model + program loop                       | 0.5d  | T09        | no        | pending | —   |
| T11 | TUI: main menu model with 10 items                   | 1d    | T10        | no        | pending | —   |
| T12 | TUI: status bar (host, services, users count stubs)  | 0.5d  | T11        | yes (T13) | pending | —   |
| T13 | TUI: lipgloss styles + colour scheme                 | 0.5d  | T10        | yes (T12) | pending | —   |
| T14 | TUI: "Not implemented yet" placeholder screen        | 0.25d | T11        | no        | pending | —   |
| T15 | Design `schemas/state.sql` (DDL)                     | 0.5d  | T02        | yes       | pending | —   |
| T16 | `internal/state`: store + migration runner           | 1d    | T15        | no        | pending | —   |
| T17 | `internal/state`: unit tests (CRUD on `admin` row)   | 0.5d  | T16        | no        | pending | —   |
| T18 | `internal/audit`: JSONL writer + event enums         | 0.5d  | T02        | yes       | pending | —   |
| T19 | `internal/audit`: unit tests (file rotation, format) | 0.5d  | T18        | no        | pending | —   |
| T20 | `internal/admin`: resolver (`$SUDO_USER` / `$USER`)  | 0.5d  | T16        | yes       | pending | —   |
| T21 | `internal/admin`: unit tests (root warning, persist) | 0.5d  | T20        | no        | pending | —   |
| T22 | Wire admin resolver into TUI startup                 | 0.5d  | T11, T21   | no        | pending | —   |
| T23 | CI: `.github/workflows/ci.yml` (lint+test+build)     | 0.5d  | T03, T04   | yes       | pending | —   |
| T24 | CI: `Dockerfile.test` (ubuntu:22.04 + systemd)       | 0.5d  | T03        | yes       | pending | —   |
| T25 | Smoke test: Docker run, navigate menu, exit clean    | 0.5d  | T22, T24   | no        | pending | —   |
| T26 | `docs/architecture.md` skeleton + link to master     | 0.25d | —          | yes       | pending | —   |
| T27 | `docs/contributing.md` (PR/commit conventions)       | 0.25d | —          | yes       | pending | —   |
| T28 | `CHANGELOG.md` skeleton + `## v0.0.0` entry          | 0.25d | —          | yes       | pending | —   |
| T29 | Tag `v0.0.0` release after all tasks complete        | 0.25d | all        | no        | pending | —   |

**Total estimate:** ~13 person-days. With 1 implementer agent + 1
reviewer agent and aggressive parallelization (T04–T08 are pure
independent), realistic calendar duration: **5 working days = 1 week**.

---

### T01: Initialize repo with LICENSE + README + .gitignore

**Description.** Create the public-facing repository skeleton.

**Acceptance criteria.**

- [ ] `LICENSE` file present, MIT (matches OpenClaw upstream).
- [ ] `README.md` present, ≤ 50 lines, contains: project name,
      one-paragraph description (linking to `OPENCLAW_OVERLAY_PLAN_RU.md`
      in the openclaw repo), "Status: pre-alpha", "Install: not yet".
- [ ] `.gitignore` present, covers Go (`*.exe`, `bin/`, `dist/`,
      `coverage.out`, `.idea/`, `.vscode/`).
- [ ] `git init` done, first commit `phase-0/T01: initial repo skeleton`.

**Test plan.** `git log --oneline` shows one commit. `cat LICENSE`
prints the MIT licence with current year and author placeholder.

---

### T02: Bootstrap Go module + project layout

**Description.** Create the Go module and the directory skeleton from
master plan §5.

**Acceptance criteria.**

- [ ] `go.mod` present, module path `github.com/<org>/openclaw-multi`,
      Go directive `go 1.22` or newer.
- [ ] Directories created (each with a `.gitkeep` file if empty):
      `cmd/openclaw-multi/`, `cmd/openclaw-overlay-api/`,
      `cmd/openclaw-overlay-watcher/`, `internal/tui/`,
      `internal/state/`, `internal/audit/`, `internal/admin/`,
      `internal/api/`, `internal/watcher/`, `internal/cloudflared/`,
      `internal/tailscale/`, `internal/ufw/`, `internal/notifications/`,
      `internal/backup/`, `internal/ports/`, `internal/shell/`,
      `pkg/`, `templates/`, `scripts/`, `schemas/`, `test/docker/`,
      `test/fixtures/`, `test/e2e/`, `test/integration/`,
      `docs/phases/`, `man/`.
- [ ] `go build ./...` succeeds (no source files yet beyond what T06-T08
      add — but the module compiles).

**Test plan.** `tree -L 2 -I node_modules` shows the layout. `go vet
./...` returns no errors.

---

### T03: Add Makefile (build/test/lint/ci targets)

**Description.** Single source of truth for common commands.

**Acceptance criteria.**

- [ ] `Makefile` has at minimum these targets (each one-liner or
      multi-step but documented): - `build` — builds all three binaries to `bin/` - `build-amd64` and `build-arm64` — explicit cross-builds via
      `GOOS=linux GOARCH=amd64`/`arm64` - `test` — `go test -race -cover ./...` - `test-unit` — same, but excluding `test/integration/` and `test/e2e/` - `lint` — `golangci-lint run` - `schema-check` — validates `schemas/state.sql` parses (sqlite3),
      validates JSON Schemas with `jq` or `ajv` - `ci` — runs `lint test build schema-check` in order - `clean` — removes `bin/`, `coverage.out` - `dev` — runs `go run ./cmd/openclaw-multi` - `install` — copies binaries to `/usr/local/bin/` (requires sudo;
      Makefile prints a warning if not running as root)
- [ ] `make help` prints a list of available targets with one-line
      descriptions (use `## ` comments after target names).

**Test plan.** `make help` shows all targets. `make build` produces
three binaries in `bin/`. `make ci` exit code is 0 on a clean checkout.

---

### T04: Set up golangci-lint config

**Description.** Code-quality baseline.

**Acceptance criteria.**

- [ ] `.golangci.yml` present, enables at least: `errcheck`, `gosimple`,
      `govet`, `ineffassign`, `staticcheck`, `unused`, `gofmt`,
      `goimports`, `misspell`, `unconvert`, `unparam`, `gocritic`,
      `revive`, `nilnil`, `bodyclose`.
- [ ] `golangci-lint run` exits 0 on the empty/skeleton codebase.
- [ ] `make lint` works.

**Test plan.** `golangci-lint run` zero output, exit 0.

---

### T05: Add `.editorconfig` + `.gitattributes`

**Description.** Cross-editor consistency.

**Acceptance criteria.**

- [ ] `.editorconfig`: tabs for `*.go`, 2-space for `*.{yml,yaml,json}`,
      LF line endings, UTF-8, trim trailing whitespace, insert final
      newline.
- [ ] `.gitattributes`: force LF for text files, mark binary file
      patterns properly.

**Test plan.** Files exist and contain documented rules.

---

### T06: Stub `cmd/openclaw-overlay-api/main.go`

**Description.** Compileable stub for the future daemon binary.

**Acceptance criteria.**

- [ ] `cmd/openclaw-overlay-api/main.go` contains:
      ```go
      package main

      func main() {
          // TODO(phase-6): implement HTTP daemon on UNIX socket.
          panic("openclaw-overlay-api: not implemented in phase 0")
      }
      ```

- [ ] `go build ./cmd/openclaw-overlay-api` succeeds.
- [ ] Binary exists at `bin/openclaw-overlay-api` after `make build`.

**Test plan.** `bin/openclaw-overlay-api` runs (panics, exit 2).
`make build` succeeds.

---

### T07: Stub `cmd/openclaw-overlay-watcher/main.go`

Same shape as T06 but for the watcher binary, panic message
`openclaw-overlay-watcher: not implemented in phase 0`. TODO comment
references `phase-7`.

**Acceptance criteria.** Same as T06.

---

### T08: Skeleton `cmd/openclaw-multi/main.go`

**Description.** Real entrypoint for the TUI binary, but at this point
just calls into a placeholder `tui.Run()` that returns immediately. T10
will fill in the actual TUI.

**Acceptance criteria.**

- [ ] `cmd/openclaw-multi/main.go`:
      ```go
      package main

      import (
          "fmt"
          "os"

          "github.com/<org>/openclaw-multi/internal/tui"
      )

      func main() {
          if err := tui.Run(); err != nil {
              fmt.Fprintln(os.Stderr, "fatal:", err)
              os.Exit(1)
          }
      }
      ```

- [ ] `internal/tui/app.go` has `func Run() error { return nil }` stub.
- [ ] `go build ./cmd/openclaw-multi` succeeds.

**Test plan.** `bin/openclaw-multi` exits 0.

---

### T09: Add Bubble Tea + Lip Gloss + Bubbles deps

**Acceptance criteria.**

- [ ] `go.mod` has the three dependencies pinned to specific versions
      (no `latest`).
- [ ] `go.sum` updated.
- [ ] `go mod tidy` produces no diff.

**Test plan.** `go mod verify` exits 0.

---

### T10: TUI root model + program loop

**Description.** Implement Bubble Tea root model in `internal/tui/app.go`
that opens a Bubble Tea program, handles `q`/`Ctrl+C` to quit cleanly,
and renders a single placeholder string "OpenClaw Multi Overlay v0.0.0".

**Acceptance criteria.**

- [ ] `internal/tui/app.go` defines: - `type Model struct { ... }` - `func (m Model) Init() tea.Cmd` - `func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)` - `func (m Model) View() string` - `func Run() error` that creates the program and runs it.
- [ ] `q` and `Ctrl+C` both quit cleanly with exit 0.
- [ ] Window resize re-renders correctly.

**Test plan.** Manual: `make dev`, the app shows the placeholder, press
`q`, exit 0. Unit test: `TestModelInitReturnsNilCmd`,
`TestModelUpdateQuit`.

---

### T11: TUI main menu model with 10 items

**Description.** Implement the main menu (master plan §6.1). 10 items,
items 1–10 plus "0. Выход". Use `bubbles/list` or a custom list.
Selection state, ↑/↓ navigation, Enter triggers a menu-action message.

**Acceptance criteria.**

- [ ] All 10 items present with the exact wording from master plan §6.1.
- [ ] Cursor moves with ↑/↓, wraps at top/bottom.
- [ ] Enter on any item dispatches a `MenuActionMsg{ItemID: int}`.
- [ ] Item "0. Выход" exits cleanly.
- [ ] Status bar at top reserved (filled by T12).

**Test plan.** Unit test: navigation moves cursor, Enter dispatches
correct ID. Manual: `make dev`, navigate, see all 10 items.

---

### T12: TUI status bar

**Description.** Top status bar showing host/Tailscale/CF/users info.
For Phase 0, all values are placeholders (`Host: <hostname>`,
`Tailscale: ?`, `CF Tunnel: ?`, `Users: 0`). Real values come in
Phases 1+.

**Acceptance criteria.**

- [ ] Status bar visible above the menu.
- [ ] Hostname read from `os.Hostname()`.
- [ ] Other fields show `?` placeholder.
- [ ] Status bar visually distinct from menu (lipgloss style from T13).

**Test plan.** Manual: `make dev` shows the status bar. Unit test:
`TestStatusBarRendersHostname`.

---

### T13: TUI lipgloss styles + colour scheme

**Description.** Centralize all colours/styles in
`internal/tui/styles.go`. Title bar, status bar, menu cursor, selected
item, "not implemented" panel.

**Acceptance criteria.**

- [ ] `styles.go` exports named styles: `TitleStyle`, `StatusStyle`,
      `MenuItemStyle`, `MenuSelectedStyle`, `PanelStyle`,
      `ErrorStyle`, `WarnStyle`, `OkStyle`.
- [ ] Each style is a `lipgloss.Style` with deterministic colours
      (no random).
- [ ] Visible diff in `make dev` between Phase 0 plain output and the
      styled output.

**Test plan.** Manual visual check.

---

### T14: TUI "Not implemented yet" placeholder screen

**Description.** When user picks any of items 1–10, show a panel:

```
┌─ <item title> ────────────────────────────────────────────────────┐
│                                                                   │
│   Not implemented yet (phase <N>).                                │
│                                                                   │
│   See OPENCLAW_OVERLAY_PLAN_RU.md §6.<n> for the planned UI.      │
│                                                                   │
│   Press Esc or q to return to the main menu.                      │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

**Acceptance criteria.**

- [ ] Each menu item routes to the placeholder with its own title.
- [ ] Esc/q returns to main menu (not exits the app).
- [ ] Item ID → phase mapping is correct (e.g. menu item 5 "Сеть и
      фаервол" → "phase 5"), encoded in a `var menuPhases =
    map[int]int{1:1, 2:1, 3:2, 4:4, 5:5, 6:7, 7:8, 8:9, 9:10, 10:11}`.

**Test plan.** Unit: `TestMenuRoutesToPlaceholder`. Manual:
`make dev`, pick each item, see correct title and phase reference.

---

### T15: Design `schemas/state.sql` (DDL)

**Description.** SQLite schema for overlay state. Phase 0 only needs
the `admin` table and the `audit_log_meta` table; Phases 2+ add
`users`, `routes`, `port_pool`, `backups`. But declare the _full_
schema upfront (with `CREATE TABLE IF NOT EXISTS`) — implementation
will fill columns as phases land.

**Acceptance criteria.**

- [ ] `schemas/state.sql` contains: - `meta` table (key/value, includes `schema_version`) - `admin` table (`username TEXT PRIMARY KEY`, `uid INT`,
      `set_at TIMESTAMP`, `set_by TEXT`) - `users` table (per master plan §3, columns: `username`, `uid`,
      `port`, `status`, `linger`, `gateway_url`, `created_at`,
      `updated_at`) - `routes` table (`id`, `username`, `kind` ('gateway'|'plugin'),
      `plugin_id` NULLABLE, `local_port`, `hostname`, `enabled`,
      `created_at`, `last_seen_cached_at` NULLABLE,
      `last_seen_value` NULLABLE) - `port_pool` table (`username`, `range_start`, `range_end`,
      `next_alloc`) - `backups` table (`id`, `username`, `ts`, `size_bytes`,
      `sha256`, `openclaw_version`, `encrypted` BOOL, `path`) - `migrations` table (`id`, `applied_at`, `description`)
- [ ] Each table has indices for expected query patterns.
- [ ] `sqlite3 :memory: < schemas/state.sql` succeeds.
- [ ] Schema documented in `docs/architecture.md` link target.

**Test plan.** `make schema-check` validates.

---

### T16: `internal/state` — store + migration runner

**Description.** Go package wrapping SQLite access. Idempotent
migration runner reads `schemas/state.sql` and applies in order, tracks
applied migrations in `migrations` table.

**Acceptance criteria.**

- [ ] `internal/state/store.go` exports: - `type Store struct { ... }` - `func Open(path string) (*Store, error)` — opens (creates if
      missing) `state.db`, runs migrations. - `func (s *Store) Close() error` - `func (s *Store) GetAdmin(ctx) (*Admin, error)` - `func (s *Store) SetAdmin(ctx, admin Admin) error`
- [ ] Uses `modernc.org/sqlite` (pure Go, no CGo).
- [ ] Path defaults to `/var/lib/openclaw-multi/state.db`; tests use
      tempdir.
- [ ] Migration runner is idempotent: re-running on an up-to-date DB
      is a no-op.

**Test plan.** Unit tests in T17.

---

### T17: `internal/state` unit tests

**Acceptance criteria.**

- [ ] `TestOpenCreatesFileAndAppliesMigrations`
- [ ] `TestOpenIsIdempotent` (open same path twice, no errors,
      migrations not re-applied)
- [ ] `TestSetAndGetAdmin` (write, then read, expect equality)
- [ ] `TestGetAdminReturnsNilOnEmpty` (no admin set yet → returns
      `(nil, nil)`)
- [ ] Coverage for `internal/state` ≥ 80 %.

**Test plan.** `go test -cover ./internal/state` reports ≥ 80 %.

---

### T18: `internal/audit` — JSONL writer + event enums

**Description.** Audit log writer per master plan §12 OQ-1.

**Acceptance criteria.**

- [ ] `internal/audit/log.go` exports: - `type Logger struct { ... }` - `func New(path string) (*Logger, error)` — opens
      `/var/log/openclaw-multi/audit.log` for append. - `func (l *Logger) Emit(event Event) error` — writes one JSONL
      line. - `func (l *Logger) Close() error`.
- [ ] `internal/audit/events.go` exports: - `type Event struct { TS time.Time; Actor string;
      Action ActionType; Target string; Result Result;
      Details map[string]any; ErrorMessage string;
      DurationMs int64 }` - `type ActionType string` with enum constants:
      `ActionBootstrapUser`, `ActionDeleteUser`,
      `ActionUpdateOpenclaw`, `ActionAddRoute`, `ActionDeleteRoute`,
      `ActionEnableUser`, `ActionDisableUser`, `ActionBackupCreate`,
      `ActionBackupRestore`, `ActionUninstall`, `ActionAdminSet`,
      `ActionStartup`, `ActionShutdown` (more added in later phases). - `type Result string` with `ResultOk`, `ResultError`,
      `ResultRolledBack`.
- [ ] Each emitted line is valid JSON (canonical key order).
- [ ] File opened with mode `0o600`, owner = current uid.

**Test plan.** Unit tests in T19.

---

### T19: `internal/audit` unit tests

**Acceptance criteria.**

- [ ] `TestNewCreatesFileWith0600`
- [ ] `TestEmitWritesValidJSONL` — emit 3 events, parse with
      `json.Decoder`, expect 3 valid objects with required fields.
- [ ] `TestEmitAppendsNotOverwrites` — emit 3, close, reopen, emit 2,
      expect 5 lines total.
- [ ] Coverage ≥ 80 %.

**Test plan.** `go test -cover ./internal/audit`.

---

### T20: `internal/admin` — resolver

**Description.** Identifies the admin user per master plan §1.3.

**Acceptance criteria.**

- [ ] `internal/admin/resolver.go` exports: - `func ResolveCurrent() (*User, error)` — returns
      `{Username, UID}` of the running user (`os.Getuid()` +
      `user.Current()`). - `func ResolveCandidate() (*User, error)` — returns admin
      candidate: prefers `$SUDO_USER` if set + non-empty + non-`root`,
      otherwise `ResolveCurrent()`. Returns error if `$SUDO_USER` is
      `root`. - `func IsRoot() bool` — `os.Getuid() == 0`. - `func WarnIfRoot() string` — empty string if not root, else a
      formatted warning explaining root-as-admin is an antipattern
      and recommending `sudo openclaw-multi` from a normal user. - `func PersistAdmin(ctx, store, user) error` — saves to state.db,
      emits audit log entry `ActionAdminSet`. - `func VerifyAdmin(ctx, store, current) error` — returns nil if
      current's `Username` matches stored admin, else
      `ErrNotAdmin{Got, Expected}`.

**Test plan.** Unit tests in T21.

---

### T21: `internal/admin` unit tests

**Acceptance criteria.**

- [ ] `TestResolveCurrent` (via mocked `os/user`)
- [ ] `TestResolveCandidatePrefersSudoUser`
- [ ] `TestResolveCandidateRejectsSudoUserRoot`
- [ ] `TestWarnIfRootEmptyForNormalUser`
- [ ] `TestPersistAdminEmitsAuditEvent` (using a fake audit logger)
- [ ] `TestVerifyAdminReturnsErrOnMismatch`
- [ ] Coverage ≥ 80 %.

**Test plan.** `go test -cover ./internal/admin`.

---

### T22: Wire admin resolver into TUI startup

**Description.** Before showing the main menu, the TUI should:

1. Open `state.db` (default path or `--state-dir <path>` flag).
2. Read stored admin.
3. If no admin stored: show "First-run admin setup" screen, suggest
   candidate from `ResolveCandidate()`, ask Y/n to confirm. If user
   accepts → `PersistAdmin`, then continue. If declines → ask for
   custom username.
4. If admin stored: call `VerifyAdmin`. If mismatch → show error
   "This account is not the configured admin", emit audit
   `ActionStartup{Result:Error}`, exit non-zero.
5. If running as root with no `$SUDO_USER`: show `WarnIfRoot()` warning
   panel (still allow first-run setup, but warn).

**Acceptance criteria.**

- [ ] All five flows above implemented.
- [ ] First-run sets `admin` row with `set_by = "first-run"`.
- [ ] Mismatch flow emits audit and exits with code 3.
- [ ] Root warning visible at top of TUI for the entire session if
      running as root.

**Test plan.** Integration test using a fake `state.db` (different
admin) → expect exit 3. Manual: `make dev`, see first-run setup.

---

### T23: CI — `.github/workflows/ci.yml`

**Description.** GitHub Actions workflow.

**Acceptance criteria.**

- [ ] Triggers: `push` to `main`, `pull_request` to `main`.
- [ ] Jobs (matrix where appropriate): - `lint` (`golangci-lint-action`) - `test` (Go matrix `go-version: [1.22, 1.23]` if both
      applicable; else single) - `build` (matrix `goarch: [amd64, arm64]`) - `schema-check` - `shellcheck` for `scripts/`
- [ ] All jobs use `ubuntu-22.04` runner.
- [ ] Coverage uploaded as artifact (`coverage.out`).
- [ ] All jobs pass on a clean PR with no real code yet (skeleton).

**Test plan.** Open a draft PR with skeleton changes — CI shows all
green checkmarks.

---

### T24: `Dockerfile.test` (ubuntu:22.04 + systemd)

**Description.** Container image used for integration smoke tests
(real systemd, no need for VM).

**Acceptance criteria.**

- [ ] `test/docker/Dockerfile.test`: - Base: `ubuntu:22.04` - Installs: `systemd`, `dbus`, `bash`, `curl`, `ca-certificates`,
      `sudo` - Creates user `opadmin` with sudo (no password — this is a test
      container only). - Copies built `bin/openclaw-multi` into `/usr/local/bin/`. - `ENTRYPOINT ["/sbin/init"]` (so systemd is PID 1).
- [ ] `make test-docker-build` builds the image.
- [ ] `make test-docker-run` runs the image, drops into shell as
      `opadmin`.

**Test plan.** Image builds. Manual: `docker run -it --rm
--privileged openclaw-multi-test`, get a working shell.

---

### T25: Smoke test — Docker run, navigate menu, exit clean

**Description.** End-to-end smoke test of the skeleton.

**Acceptance criteria.**

- [ ] `test/e2e/phase-0/smoke.sh` script: 1. Builds binary (`make build-amd64`). 2. Builds Docker image (`make test-docker-build`). 3. Runs container, executes `openclaw-multi` non-interactively
      using a TUI scripting helper (e.g. `expect` or `vhs` from
      Charm) that:
      a. Confirms first-run admin setup with default candidate.
      b. Verifies the main menu appears with all 10 items.
      c. Picks each menu item 1..9 and verifies the "Not
      implemented yet (phase N)" panel appears with correct
      phase.
      d. Picks "0. Выход" and verifies clean exit with code 0. 4. Confirms `state.db` was created with admin row. 5. Confirms `audit.log` has at least `ActionStartup` and
      `ActionAdminSet` entries.
- [ ] `make test-phase-0` runs this script and exits 0.
- [ ] CI runs this on every PR (added to `.github/workflows/ci.yml`).

**Test plan.** `make test-phase-0` passes.

---

### T26: `docs/architecture.md` skeleton

**Acceptance criteria.**

- [ ] File present, ≤ 100 lines.
- [ ] Sections: "Overview", "Three binaries", "State storage", "Audit
      log", "Tests".
- [ ] Each section has 1-2 paragraphs and a link to the relevant
      section of the master plan
      (`OPENCLAW_OVERLAY_PLAN_RU.md`) or to the SQL schema /
      package docs.
- [ ] Mermaid diagram showing the three binaries + state.db + audit log
      (Mermaid is rendered by GitHub).

**Test plan.** Manual review.

---

### T27: `docs/contributing.md`

**Acceptance criteria.**

- [ ] PR/branch/commit conventions from `OPENCLAW_OVERLAY_DEV_PROCESS.md` §8.
- [ ] How to run `make ci` locally.
- [ ] Where to find the current phase doc.
- [ ] How to add a new task to a phase doc (only if owner approves).
- [ ] Code review expectations (DoD checks).

**Test plan.** Manual review.

---

### T28: `CHANGELOG.md` skeleton

**Acceptance criteria.**

- [ ] File present.
- [ ] Header: "All notable changes to this project will be documented
      in this file." + Keep-a-Changelog format link.
- [ ] First entry:
      ``` ## v0.0.0 - YYYY-MM-DD

      ### Added
      - Initial project skeleton (phase 0).
      - Three binary stubs.
      - TUI shell with main menu placeholders.
      - SQLite state store with admin identification.
      - JSONL audit log writer.
      - CI pipeline (lint, test, build × 2 arches).
      - Docker-based smoke test.
      ```

**Test plan.** Manual review.

---

### T29: Tag `v0.0.0` release

**Description.** Annotated git tag, GitHub release with built
binaries attached.

**Acceptance criteria.**

- [ ] `git tag -a v0.0.0 -m "phase-0 skeleton"` on the merge commit
      that closes T28.
- [ ] GitHub release created automatically by
      `.github/workflows/release.yml` (added in this task) — uploads
      `openclaw-multi-linux-amd64`, `openclaw-multi-linux-arm64`,
      `openclaw-overlay-api-linux-amd64`, `openclaw-overlay-api-linux-arm64`,
      `openclaw-overlay-watcher-linux-amd64`,
      `openclaw-overlay-watcher-linux-arm64`, plus checksums.
- [ ] Release notes auto-populated from CHANGELOG `## v0.0.0`.

**Test plan.** Visit release page on GitHub, download a binary,
`./openclaw-multi --version` (added trivially in T08 if not yet)
prints `v0.0.0`.

---

## 6. Definition of Done for Phase 0

- [ ] All 29 tasks have status `completed`
- [ ] All acceptance criteria of every task checked
- [ ] `make ci` passes on `main`
- [ ] `make test-phase-0` passes (Docker smoke test)
- [ ] `make build-amd64 build-arm64` produces six binaries with no
      errors
- [ ] Coverage ≥ 80 % on `internal/state`, `internal/audit`,
      `internal/admin`
- [ ] CI badge in `README.md` shows green
- [ ] Release `v0.0.0` tagged and published
- [ ] `docs/phases/phase-0-retro.md` written

---

## 7. Phase smoke test suite

Defined in T25 and T26. Single canonical entrypoint: `make test-phase-0`.

| Test               | What it proves                                                                                                                                   |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `phase-0/smoke.sh` | Skeleton boots in clean Ubuntu 22.04 container, runs admin first-time setup, navigates main menu, exits cleanly, persists state.db and audit log |

---

## 8. Documentation deliverables

- [x] `README.md` (T01)
- [x] `docs/architecture.md` (T26)
- [x] `docs/contributing.md` (T27)
- [x] `CHANGELOG.md` (T28)
- [x] Inline godoc comments on every exported function/type in
      `internal/state`, `internal/audit`, `internal/admin`, `internal/tui`
      (verified by `golangci-lint`'s `revive` rule).

---

## 9. Risks and mitigations (for this phase)

| Risk                                                                               | Mitigation                                                                                                                               |
| ---------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| Bubble Tea API changes between when this doc was written and when phase 0 starts   | Pin exact versions in T09; revisit if breaking                                                                                           |
| `modernc.org/sqlite` performance edge cases                                        | Phase 0 uses < 100 rows, performance is irrelevant; revisit in Phase 5 if state.db grows                                                 |
| Cross-compilation on macOS/Apple Silicon dev machines                              | Use `GOOS=linux GOARCH=amd64 go build`; CI is the source of truth, dev machines are best-effort                                          |
| TUI tests are flaky (TUI testing is hard)                                          | Use `vhs` (Charm) or simple `expect` scripts for E2E; favour pure unit tests for `Update()` logic                                        |
| Audit log file ownership conflicts when running both as root and as a regular user | Phase 0 always writes to a path owned by the current user (during dev); Phase 9 sets up `/var/log/openclaw-multi/` with proper ownership |

---

## 10. Open questions to resolve before phase ends

(None at start. If any arise during implementation, log them here for
the retrospective.)
