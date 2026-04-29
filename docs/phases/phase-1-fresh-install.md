---
phase: 1
title: "Bootstrap fresh install"
slug: fresh-install
estimated_duration: "2 weeks (10 working days)"
status: draft
approved_at: null
approved_by: null
master_plan_section: "§9 Phase 1, §2.5 (идемпотентность), §6.2 (fresh install wizard)"
prior_retros: ["phase-0-retro.md"]
---

# Phase 1: Bootstrap fresh install

> **Test-only phase.** No code in this phase runs on a real VPS. All system
> interaction (apt-get, npm, systemctl, tailscale, cloudflared, ufw, sysctl)
> is abstracted behind injectable interfaces and tested exclusively with mocks.
> Live execution is deferred until the complete product is built.
>
> After this phase: the fresh-install wizard (TUI menu item 1) is fully
> implemented, all logic is unit-tested with mock executors, and the code
> compiles for linux-amd64/arm64. A real admin will be able to run this on a
> VPS only after the owner approves a deployment run post-MVP.

---

## 1. Goals

1. `internal/shell.Executor` interface — the single abstraction for all
   subprocess calls. Real implementation uses `os/exec`; test implementation
   is a fully controllable mock.
2. `internal/preflight` — checks distro, disk, RAM, systemd, port conflicts.
   Fully unit-tested via mock filesystem reads and mock executor.
3. `internal/deps` — idempotent ensure-functions for Node.js, Tailscale,
   cloudflared (Variant A / Variant B), UFW. Every function takes an
   `Executor` and is 100% tested with mocks. No real installs.
4. `internal/config` — overlay config YAML + envsubst template renderer.
   Unit-tested with in-memory fixtures.
5. `internal/hardening` — sysctl, hidepid, profile.d writes. Mock-executor
   based; no real file writes in tests.
6. `internal/tui/wizard` — generic Bubble Tea step-wizard model + the
   fresh-install wizard wiring all steps together. Unit-tested via model
   Update() calls (no real TUI rendering needed in tests).
7. `make ci` passes: lint clean, all tests green, both arches build.

---

## 2. Out of scope

- Any real execution on a VPS (deferred until post-MVP).
- Actual Tailscale OAuth, actual cloudflared tunnel creation on the network.
- Adding users (Phase 2).
- overlay-API HTTP server (Phase 6).
- Backup, health check, uninstall screens (later phases).
- Docker smoke tests that install real packages.

---

## 3. Inputs

- Code state at start: `v0.0.0` / `phase-0` skeleton.
- Go version: **1.26.2** current repository baseline after the dependency
  refresh to Bubble Tea v2 / Lip Gloss v2 / modernc.org/sqlite v1.49.1.
- Master plan: §2.5 (idempotency), §5 (dirs), §6.2 (wizard steps), §9 Phase 1.
- Prior retros: `phase-0-retro.md`.
- Constraints from DEV_PROCESS §0: no live installs, no VPS, tests only.

---

## 4. Architecture for this phase

### Core abstraction: `shell.Executor`

Every package that needs to run a subprocess depends on this interface, not
on `os/exec` directly. This is the key that makes everything testable:

```go
// internal/shell/exec.go
type Executor interface {
    Run(ctx context.Context, opts ExecOpts) (ExecResult, error)
}

type ExecOpts struct {
    Cmd     []string
    Timeout time.Duration
    Sudo    bool
    Env     []string
}

type ExecResult struct {
    Stdout   string
    Stderr   string
    ExitCode int
}

// RealExecutor uses os/exec. MockExecutor is in exec_mock_test.go.
type RealExecutor struct{ logger audit.Logger }
```

### New packages

```
internal/shell/
  exec.go          ← Executor interface + RealExecutor
  exec_test.go     ← tests for RealExecutor (no sudo, no system cmds)

internal/preflight/
  checker.go       ← CheckAll, individual check funcs
  checker_test.go  ← all mocked

internal/deps/
  node.go          ← EnsureNode(ctx, exec, cfg)
  tailscale.go     ← EnsureTailscale(ctx, exec, interactive bool)
  cloudflared.go   ← EnsureCloudflared(ctx, exec, cfg, renderer, mode)
  ufw.go           ← EnsureUFW(ctx, exec, extraPorts []int)
  deps_test.go     ← MockExecutor-based tests for all four

internal/config/
  overlay.go       ← OverlayConfig struct, Load/Save
  templates.go     ← Render, RenderToFile
  config_test.go   ← in-memory fixtures

internal/hardening/
  hardening.go     ← ApplySysctl, ApplyHidepid, ApplyProfile
  hardening_test.go

internal/tui/wizard/
  wizard.go        ← generic WizardModel (Bubble Tea)
  wizard_test.go
  freshinstall.go  ← 10 steps wired to internal packages
  freshinstall_test.go

templates/
  sysctl-overlay.conf
  fstab-proc-hidepid
  profile-d-openclaw.sh
  cloudflared-config.tmpl
  openclaw-overlay-api.service.tmpl
  openclaw-gateway.service.tmpl      ← stub, used in Phase 2
```

---

## 5. Atomic tasks

| ID  | Title                                                     | Est   | Depends on    | Status  |
|-----|-----------------------------------------------------------|-------|---------------|---------|
| T01 | `internal/shell`: Executor interface + RealExecutor       | 0.5d  | —             | pending |
| T02 | `internal/shell`: MockExecutor + RealExecutor tests       | 0.5d  | T01           | pending |
| T03 | `internal/preflight`: distro + disk/RAM + tools checks    | 0.5d  | T01           | pending |
| T04 | `internal/preflight`: port conflict check + unit tests    | 0.5d  | T02, T03      | pending |
| T05 | `internal/config`: OverlayConfig YAML + Load/Save         | 0.5d  | —             | pending |
| T06 | `internal/config`: envsubst renderer + tests              | 0.5d  | T05           | pending |
| T07 | templates: sysctl, fstab, profile.d, cloudflared, units   | 0.25d | T06           | pending |
| T08 | `internal/hardening`: sysctl + hidepid + profile          | 0.5d  | T01, T06, T07 | pending |
| T09 | `internal/hardening`: mock-based tests, coverage ≥ 80%    | 0.5d  | T02, T08      | pending |
| T10 | `internal/deps/node`: EnsureNode + mock tests             | 0.5d  | T02           | pending |
| T11 | `internal/deps/tailscale`: EnsureTailscale + mock tests   | 0.5d  | T02           | pending |
| T12 | `internal/deps/cloudflared`: EnsureCloudflared A+B        | 1d    | T02, T06      | pending |
| T13 | `internal/deps/cloudflared`: mock tests, coverage ≥ 80%   | 0.5d  | T02, T12      | pending |
| T14 | `internal/deps/ufw`: EnsureUFW + mock tests               | 0.5d  | T02           | pending |
| T15 | `internal/tui/wizard`: generic WizardModel                | 0.5d  | —             | pending |
| T16 | `internal/tui/wizard`: WizardModel unit tests             | 0.5d  | T15           | pending |
| T17 | `internal/tui/wizard/freshinstall`: steps 1–5             | 1d    | T04, T11, T15 | pending |
| T18 | `internal/tui/wizard/freshinstall`: steps 6–10            | 1d    | T09, T12, T17 | pending |
| T19 | freshinstall wizard unit tests (mock executor throughout) | 0.5d  | T02, T18      | pending |
| T20 | Wire wizard into TUI menu item 1; `docs/install.md`       | 0.25d | T19           | pending |

**Total: ~11 person-days.**

---

### T01: `internal/shell` — Executor interface + RealExecutor

**Description.** Single abstraction for all subprocess calls. Every package in
this project that needs to run a command depends on `Executor`, never on
`os/exec` directly. This is what makes the entire codebase testable without
a VPS.

**Acceptance criteria.**

- [ ] `internal/shell/exec.go` exports `Executor` interface, `ExecOpts`,
  `ExecResult`, `ErrTimeout`, `ErrNonZeroExit`.
- [ ] `RealExecutor` implements `Executor` using `os/exec` + `context.WithTimeout`.
- [ ] `Sudo: true` prepends `sudo` to `cmd[0]`.
- [ ] Stdout and Stderr always captured (never streamed to os.Stdout in tests).
- [ ] Emits `audit.Event{Action: ActionShellExec}` after every call.
- [ ] `ActionShellExec` added to `internal/audit/events.go`.

**Test plan.** T02 covers this. No system commands called in T01 tests —
only that the struct satisfies the interface.

---

### T02: `internal/shell` — MockExecutor + RealExecutor tests

**Description.** `MockExecutor` is the test double used by every other package.
It records calls and returns pre-programmed responses.

**Acceptance criteria.**

- [ ] `MockExecutor` in `internal/shell/mock.go` (exported, usable from other
  packages' tests):
  ```go
  type MockExecutor struct {
      Calls    []ExecOpts
      Responses []ExecResult // consumed in order; last repeated if exhausted
      Errors   []error
  }
  func (m *MockExecutor) Run(ctx, opts) (ExecResult, error)
  ```
- [ ] `TestRealExecutor_SimpleCommand` — runs `echo hello`, asserts stdout.
- [ ] `TestRealExecutor_Timeout` — runs `sleep 10` with 50ms timeout, asserts
  `ErrTimeout`.
- [ ] `TestRealExecutor_NonZeroExit` — runs `false`, asserts `ErrNonZeroExit`.
- [ ] `TestMockExecutor_RecordsCalls` — verifies call recording.
- [ ] Coverage ≥ 80%.

---

### T03: `internal/preflight` — distro + resource checks

**Description.** Checks that the host is a supported distro, has sufficient
disk/RAM, and has required tools. Uses `Executor` for tool presence checks,
pure Go for disk/RAM (no exec needed).

**Acceptance criteria.**

- [ ] `type Checker struct{ Exec shell.Executor; OSReleasePath string }`.
- [ ] `func (c Checker) CheckAll(ctx) ([]Result, error)` where
  `Result{Name, Detail string; OK bool; Level string /* "ok"|"warn"|"fail" */}`.
- [ ] Distro: parse `OSReleasePath` (default `/etc/os-release`); accept Ubuntu
  22.04+ and Debian 12+; return `Level:"warn"` for others (not hard fail —
  admin decides).
- [ ] Disk: `syscall.Statfs` on `/`, warn if < 5 GB free.
- [ ] RAM: parse `/proc/meminfo`, warn if < 1 GB total.
- [ ] Tools: `which systemd`, `which useradd`, `which loginctl` via Executor.
- [ ] `OSReleasePath` injectable so tests use a fixture file, not the real one.

---

### T04: `internal/preflight` — port check + unit tests

**Description.** Detect processes listening on ports 18789–19999 by parsing
`/proc/net/tcp` (pure Go, no exec) or via mock executor for `ss -tlnp`.

**Acceptance criteria.**

- [ ] `func (c Checker) CheckPorts(ctx) ([]PortConflict, error)` where
  `PortConflict{Port int; PID int; Process string}`.
- [ ] Prefers `/proc/net/tcp` parsing (pure Go); falls back to `ss -tlnp` via
  Executor if `/proc/net/tcp` is unavailable.
- [ ] Unit tests (all via fixtures/mock, no real network sockets):
  - `TestDistroCheckUbuntu22` — fixture `/etc/os-release`.
  - `TestDistroCheckUnknown` — warns, does not fail.
  - `TestDiskCheckWarnBelowThreshold`.
  - `TestRAMCheckWarnBelowThreshold`.
  - `TestPortConflictDetected` — mock `/proc/net/tcp` or mock `ss` output.
  - `TestCheckAllReturnsAllResults`.
- [ ] Coverage ≥ 80%.

---

### T05: `internal/config` — OverlayConfig YAML

**Description.** Read/write `/etc/openclaw-multi/config.yml`. All paths
injectable for tests.

**Acceptance criteria.**

- [ ] `OverlayConfig` struct with fields matching master plan §5 config:
  `Domain`, `Subdomain`, `TunnelID`, `TunnelMode` (`"account"|"quick"`),
  `PortRangeStart`, `PortRangeStep`, `NodeVersionMin`,
  `Notifications{TelegramToken, TelegramChatID string}`.
- [ ] `func Load(path string) (*OverlayConfig, error)` — returns struct with
  defaults if file absent (`ErrNotFound` wrapped).
- [ ] `func Save(path string, cfg *OverlayConfig) error` — atomic write
  (temp file + `os.Rename`).
- [ ] `schemas/overlay-config.schema.json` — JSON Schema for the config.
- [ ] Unit tests: load defaults, load fixture, save + reload roundtrip,
  save to non-existent dir returns error.
- [ ] Coverage ≥ 80%.

---

### T06: `internal/config` — envsubst renderer

**Description.** Render `templates/*.tmpl` files by substituting `${VAR}`
tokens. Used to produce systemd units, cloudflared config, sysctl file, etc.

**Acceptance criteria.**

- [ ] `func Render(tmpl string, vars map[string]string) (string, error)` —
  takes template content as string (caller reads the file).
- [ ] `func RenderFile(tmplPath string, vars map[string]string) (string, error)`
  — reads file, calls Render.
- [ ] `func WriteFile(tmplPath, outPath string, vars map[string]string, mode os.FileMode) error`
  — renders + writes atomically.
- [ ] Unknown `${VAR}` in template → `ErrUnknownVar{Var string}` (fail fast).
- [ ] Unit tests with inline template strings (no real file I/O). Coverage ≥ 80%.

---

### T07: Config templates

**Description.** All template files for Phase 1 install steps.

**Acceptance criteria.**

- [ ] `templates/sysctl-overlay.conf` — kernel hardening:
  `kernel.yama.ptrace_scope = 2`, `kernel.dmesg_restrict = 1`,
  `net.ipv4.conf.all.rp_filter = 1`.
- [ ] `templates/fstab-proc-hidepid` — single fstab fragment:
  `proc /proc proc defaults,hidepid=2,gid=adm 0 0`.
- [ ] `templates/profile-d-openclaw.sh` — `umask 0077` +
  `export NODE_COMPILE_CACHE=/var/cache/openclaw-compile`.
- [ ] `templates/cloudflared-config.tmpl` — minimal cloudflared config with
  `${TUNNEL_ID}`, `${TUNNEL_CREDENTIALS_FILE}`, catch-all 404.
- [ ] `templates/openclaw-overlay-api.service.tmpl` — systemd unit for the
  overlay-API daemon.
- [ ] `templates/openclaw-gateway.service.tmpl` — per-user `--user` unit stub
  (filled in Phase 2).
- [ ] `make schema-check` extended to verify all templates exist.

---

### T08: `internal/hardening` — host hardening logic

**Description.** Idempotent functions to apply sysctl, hidepid, profile.d.
All file writes go through injectable `fs` helper (interface with `ReadFile`,
`WriteFile`, `Stat`) so tests never touch real files.

**Acceptance criteria.**

- [ ] `type FS interface{ ReadFile, WriteFile, Stat, MkdirAll }` in
  `internal/shell/fs.go` (alongside Executor).
- [ ] `RealFS` uses `os` package. `MemFS` is the in-memory test double.
- [ ] `func ApplySysctl(ctx, exec Executor, fs FS, renderer config.Renderer) error`
  — writes `/etc/sysctl.d/openclaw-overlay.conf` if content differs; then
  `sudo sysctl --system` via Executor.
- [ ] `func ApplyHidepid(ctx, exec Executor, fs FS) error` — appends
  hidepid line to `/etc/fstab` if not present; then
  `sudo mount -o remount /proc` via Executor.
- [ ] `func ApplyProfile(ctx, exec Executor, fs FS, renderer config.Renderer) error`
  — writes `/etc/profile.d/openclaw.sh`; `sudo mkdir -p /var/cache/openclaw-compile`,
  `sudo chmod 1777 ...` via Executor.
- [ ] Each function emits audit log entries.

---

### T09: `internal/hardening` unit tests

**Acceptance criteria.**

- [ ] All tests use `MemFS` + `MockExecutor`.
- [ ] `TestApplySysctlWritesFile` — MemFS contains the rendered content.
- [ ] `TestApplySysctlIdempotent` — second call: file unchanged, zero
  executor calls for sysctl.
- [ ] `TestApplyHidepidAppendsLine` — fstab gets the hidepid fragment.
- [ ] `TestApplyHidepidIdempotent` — line already present, no duplicate.
- [ ] `TestApplyProfileCreatesFile`.
- [ ] Coverage ≥ 80%.

---

### T10: `internal/deps/node` — EnsureNode

**Acceptance criteria.**

- [ ] `func EnsureNode(ctx, exec Executor, minVersion string) (NodeStatus, error)`.
- [ ] `NodeStatus{Installed bool; Version string; Skipped bool}`.
- [ ] Detect: `node --version` via Executor, parse semver, compare to minVersion.
- [ ] If already ≥ minVersion: `Skipped: true`, no further calls.
- [ ] If missing/old: no install command is emitted. System-space Node.js is not
  required by Fresh Install.
- [ ] Unit tests: already installed (correct version), already installed (old
  version), not installed. Coverage ≥ 80%.

---

### T11: `internal/deps/tailscale` — EnsureTailscale

**Acceptance criteria.**

- [ ] `func EnsureTailscale(ctx, exec Executor, interactive bool) (TailscaleStatus, error)`.
- [ ] `TailscaleStatus{Installed, Running, LoggedIn bool; IP string}`.
- [ ] Detect: `tailscale status --json` via Executor; parse JSON into status.
- [ ] Already running+logged in → `Skipped: true`.
- [ ] Not installed → install command sequence via Executor.
- [ ] `interactive=false` (always in tests and CI): skip `tailscale up`, return
  `LoggedIn: false` without error.
- [ ] Unit tests: all three states (running, not running, not installed).
  Coverage ≥ 80%.

---

### T12: `internal/deps/cloudflared` — EnsureCloudflared

**Description.** Most complex dep. Two modes: Variant A (named tunnel with
Cloudflare account) and Variant B (quick tunnel, no account).

**Acceptance criteria.**

- [ ] `func EnsureCloudflared(ctx, exec Executor, fs FS, renderer config.Renderer, cfg *config.OverlayConfig, mode string) error`.
- [ ] Detect: `cloudflared --version` + check credentials file existence via FS.
- [ ] **Variant A** command sequence (via Executor):
  - `cloudflared tunnel create openclaw-multi` → parse tunnel ID from output.
  - `cloudflared tunnel route dns openclaw-multi *.${subdomain}.${domain}`.
  - Render `cloudflared-config.tmpl` → write via FS.
  - `cloudflared tunnel ingress validate /etc/cloudflared/config.yml`.
  - `sudo systemctl enable --now cloudflared`.
- [ ] **Variant B** command sequence: write minimal quick-tunnel config, start
  with `--url` flag.
- [ ] `ConflictPolicy` enum (`Backup | Skip | Overwrite`) applied when existing
  config.yml detected.
- [ ] Backup: write `config.yml.<timestamp>` via FS before overwrite.
- [ ] Audit entries for every mutating step.

---

### T13: `internal/deps/cloudflared` — mock tests

**Acceptance criteria.**

- [ ] All via `MockExecutor` + `MemFS`.
- [ ] `TestVariantA_FreshInstall` — full command sequence verified.
- [ ] `TestVariantA_AlreadyInstalled` — only validation called, no install.
- [ ] `TestVariantB_QuickTunnel` — correct minimal config written.
- [ ] `TestConflictPolicy_Backup` — backup file created, original overwritten.
- [ ] `TestConflictPolicy_Skip` — no write.
- [ ] Coverage ≥ 80%.

---

### T14: `internal/deps/ufw` — EnsureUFW

**Acceptance criteria.**

- [ ] `func EnsureUFW(ctx, exec Executor, extraPorts []int) (UFWStatus, error)`.
- [ ] `UFWStatus{Active bool; DenyIncoming, AllowOutgoing bool; AddedPorts []int}`.
- [ ] Parse `ufw status verbose` output via Executor to understand current state.
- [ ] If not active: full setup sequence via Executor.
- [ ] If active: verify `default deny incoming`; only add missing rules.
- [ ] Idempotent: re-run with same ports → zero Executor calls for existing rules.
- [ ] Unit tests: inactive UFW, active UFW (correct), active UFW (missing rule).
  Coverage ≥ 80%.

---

### T15: `internal/tui/wizard` — generic WizardModel

**Description.** Reusable Bubble Tea step-wizard. Each step is a `Step`
interface implementation; the model drives them sequentially, shows progress,
handles errors.

**Acceptance criteria.**

- [ ] `type Step interface{ Name() string; Run(ctx) error }`.
- [ ] `type WizardModel` with `Init() / Update() / View()`.
- [ ] Progress indicator: `[2/10] Installing cloudflared...` with spinner.
- [ ] On step error: display stderr/error + options `[R]etry [S]kip [A]bort`.
- [ ] On all steps done: dispatch `WizardDoneMsg{Outcomes []StepOutcome}`.
- [ ] Window resize handled.

---

### T16: `internal/tui/wizard` unit tests

**Acceptance criteria.**

- [ ] `TestWizardAdvancesOnSuccess` — mock steps all return nil → model
  reaches done state.
- [ ] `TestWizardShowsErrorOnFailure` — step returns error → error screen shown.
- [ ] `TestWizardRetry` — Retry key re-runs the failed step.
- [ ] `TestWizardSkip` — Skip key moves to next step.
- [ ] `TestWizardAbort` — Abort key dispatches quit cmd.
- [ ] Coverage ≥ 80%.

---

### T17: freshinstall wizard — steps 1–5

**Description.** Concrete `Step` implementations for first half of §6.2.
Each step wraps the corresponding `internal/` package call.

**Acceptance criteria.**

- [ ] `Step1PreFlight` — wraps `preflight.CheckAll`; on warn shows detail but
  continues; on fail aborts.
- [ ] No Fresh Install Node/OpenClaw step. Node.js and OpenClaw CLI are prepared
  only inside managed users during add-user.
- [ ] `Step2Tailscale` — wraps `deps.EnsureTailscale(interactive=false)` during
  tests; `true` in real execution.
- [ ] `Step4Cloudflared` — before running, TUI asks variant A or B + collects
  domain/token via `bubbles/textinput`; saves to `OverlayConfig`.
- [ ] `Step5UFW` — asks for extra ports (comma-separated input); wraps
  `deps.EnsureUFW`.
- [ ] Steps 4–5 update `OverlayConfig` and call `config.Save`.
- [ ] All injectable (Executor, FS, Renderer injected at construction time).

---

### T18: freshinstall wizard — steps 6–10

**Acceptance criteria.**

- [ ] `Step6Hardening` — wraps `hardening.Apply*`.
- [ ] No global OpenClaw install step. Fresh Install must not install OpenClaw globally.
- [ ] `Step7OverlayAPI` — copies binary path to `/usr/local/bin/` (via FS),
  renders and writes systemd unit (via FS + Renderer), then
  `sudo systemctl daemon-reload && sudo systemctl enable --now openclaw-overlay-api`
  (via Executor).
- [ ] `Step9AddUser` — info-only screen: "Phase complete. Use menu item 3 to
  add users."
- [ ] `Step10Summary` — aggregates `[]StepOutcome` from `WizardDoneMsg`,
  renders a table: step name / status / key paths. Shows `config.yml` path,
  audit log path, `tailscale ip` output.
- [ ] `meta` key `install_completed` written to state.db with RFC3339 timestamp.

---

### T19: freshinstall wizard unit tests

**Acceptance criteria.**

- [ ] All steps constructed with `MockExecutor` + `MemFS` + fixture configs.
- [ ] `TestStep1PreFlight_Pass` / `_Warn` / `_Fail`.
- [ ] `TestStep2Node_Skip` (already installed) / `_Install`.
- [ ] `TestStep3Tailscale_AlreadyRunning` / `_NotInstalled`.
- [ ] `TestStep4Cloudflared_VariantA` / `_VariantB`.
- [ ] `TestStep5UFW_FreshSetup` / `_Idempotent`.
- [ ] `TestStep6Hardening_WritesFiles`.
- [ ] `TestStep7OpenClaw_InstallCommand`.
- [ ] `TestStep8OverlayAPI_WritesUnitAndEnables`.
- [ ] `TestStep10Summary_RendersTable`.
- [ ] Coverage ≥ 80% for `internal/tui/wizard/freshinstall`.

---

### T20: Wire into TUI + docs

**Acceptance criteria.**

- [ ] Menu item 1 launches `freshinstall.New(store, logger, exec, fs, renderer)`
  instead of the placeholder.
- [ ] `Esc`/`q` mid-wizard prompts confirm-abort.
- [ ] `docs/install.md` (≤ 300 lines): prerequisites, running the wizard,
  Variant A vs B CF choice, expected outcome, troubleshooting section.
- [ ] CHANGELOG `## v0.1.0` entry added.

---

## 6. Definition of Done for Phase 1

- [ ] All 20 tasks completed
- [ ] `make ci` passes: lint clean, all tests green, amd64+arm64 build
- [ ] Coverage ≥ 80% on every new `internal/*` package
- [ ] `docs/install.md` merged
- [ ] CHANGELOG updated under `## v0.1.0`
- [ ] Tag `v0.1.0` created (no public release)
- [ ] `docs/phases/phase-1-retro.md` written

---

## 7. Phase smoke test suite

There are **no Docker/integration tests** that execute real system commands in
Phase 1. All verification is via unit tests with `MockExecutor` + `MemFS`.

| Test level  | What runs                                              | Where               |
|-------------|--------------------------------------------------------|---------------------|
| Unit        | MockExecutor + MemFS, no real FS/network/packages      | `go test ./...`     |
| Build check | Both arches compile                                    | `make build-amd64 build-arm64` |

A real live smoke test on a VPS is deferred until the owner authorises a
deployment run after the MVP is complete.

---

## 8. Documentation deliverables

- [ ] `docs/install.md` (T20)
- [ ] Godoc on all exported types in new packages
- [ ] CHANGELOG `## v0.1.0`

---

## 9. Risks and mitigations

| Risk                                                   | Mitigation                                          |
|--------------------------------------------------------|-----------------------------------------------------|
| cloudflared CLI flags change (output format)           | Use Context7 to fetch cloudflared docs before T12   |
| Tailscale JSON status schema changes                   | Same — Context7 before T11                          |
| Mock tests pass but real execution fails               | Accepted: real testing deferred to post-MVP by owner decision |
| hidepid=2 breaks some systemd services in production   | Document as known risk in install.md; make it optional in Step6 |

---

## 10. Open questions to resolve before phase ends

- (none at start)
