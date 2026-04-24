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

> After this phase: the admin can run `openclaw-multi` on a clean Ubuntu 22.04+
> VPS, go through the fresh-install wizard end-to-end, and have a fully
> configured overlay host (Node.js + Tailscale + Cloudflare Tunnel + UFW +
> hardening + openclaw global + overlay-API stub service) with no users yet.
>
> All install steps are idempotent (§2.5): running the wizard twice on a
> partially-configured VPS is safe.

---

## 1. Goals

1. `internal/preflight` package detects distro, disk/RAM, open ports, required
   tools — all checks pass or emit actionable warnings.
2. `internal/deps` package installs/verifies Node.js, Tailscale, cloudflared,
   UFW idempotently (skips what is already configured).
3. Config templates (`templates/`) render correctly via envsubst; generated
   files are written to the correct locations.
4. TUI wizard (menu item 1 "Установка с нуля") leads the admin through all
   10 sub-steps of §6.2, showing progress and status per step.
5. `internal/shell` safe executor wraps `os/exec` calls: captures stdout/stderr,
   logs to audit, enforces timeouts, reports errors back to TUI.
6. Smoke test in Docker (ubuntu:22.04, no real network) passes: wizard
   completes, all idempotency re-runs succeed, state.db reflects installed state.

---

## 2. Out of scope

- Actual Tailscale OAuth flow (we stub the interactive `tailscale up` step).
- Actual cloudflared tunnel creation (we stub DNS/CNAME write; local config
  generation is real).
- Adding users (Phase 2).
- overlay-API HTTP server (Phase 6); only a systemd unit stub is installed.
- Backup, uninstall, health check screens (later phases).

---

## 3. Inputs

- Code state at start: `v0.0.0` tag (phase-0 skeleton).
- Master plan sections: §2.5, §5 (directory layout), §6.2, §9 Phase 1.
- Prior retros: `phase-0-retro.md`.
- Open questions resolved before starting:
  - Go 1.25.0 required (see retro).
  - golangci-lint v2 config format applies.
  - Commits: 1–2 per phase, squashed.

---

## 4. Architecture for this phase

New packages added in Phase 1:

```
internal/preflight/
  checker.go      ← distro, disk, RAM, systemd, ports checks
  checker_test.go

internal/deps/
  node.go         ← Node.js detect + install
  tailscale.go    ← Tailscale detect + install + up
  cloudflared.go  ← cloudflared detect + install + config generate
  ufw.go          ← UFW detect + configure
  deps_test.go    ← unit tests with mock executor

internal/shell/
  exec.go         ← safe os/exec wrapper (timeout, audit log, stdout/stderr capture)
  exec_test.go

internal/config/
  overlay.go      ← /etc/openclaw-multi/config.yml read/write
  templates.go    ← envsubst renderer for templates/

internal/hardening/
  sysctl.go       ← /etc/sysctl.d/openclaw-overlay.conf write + sysctl --system
  proc.go         ← /etc/fstab hidepid=2 write + mount remount
  profile.go      ← /etc/profile.d/openclaw.sh write
```

TUI changes:

```
internal/tui/
  wizard/
    wizard.go        ← generic step-by-step wizard model
    freshinstall.go  ← Phase 1 wizard: 10 steps from §6.2
    step.go          ← Step interface: Name, Run, Idempotent
```

The wizard wires into the existing TUI by replacing the "Not implemented yet"
placeholder for menu item 1.

Key constraint: **no CGo**, pure Go only (already established in phase-0).
`internal/shell.Exec` uses `os/exec`, not CGo.

---

## 5. Atomic tasks

| ID  | Title                                                       | Est   | Depends on     | Status  |
|-----|-------------------------------------------------------------|-------|----------------|---------|
| T01 | `internal/shell`: safe exec wrapper + tests                 | 0.5d  | —              | pending |
| T02 | `internal/preflight`: distro + disk/RAM + systemd checks    | 0.5d  | T01            | pending |
| T03 | `internal/preflight`: port conflict check (18789–19999)     | 0.25d | T02            | pending |
| T04 | `internal/preflight`: unit tests (mock fs + mock exec)      | 0.5d  | T02, T03       | pending |
| T05 | `internal/config`: overlay config YAML read/write           | 0.5d  | —              | pending |
| T06 | `internal/config`: envsubst template renderer               | 0.5d  | T05            | pending |
| T07 | templates: sysctl-overlay.conf, fstab-proc-hidepid,         | 0.5d  | —              | pending |
|     | profile-d-openclaw.sh, cloudflared-config.tmpl,             |       |                |         |
|     | openclaw-overlay-api.service.tmpl                           |       |                |         |
| T08 | `internal/hardening`: sysctl + hidepid + profile writes     | 0.5d  | T01, T06, T07  | pending |
| T09 | `internal/hardening`: unit tests                            | 0.5d  | T08            | pending |
| T10 | `internal/deps/node`: detect Node.js + version check        | 0.5d  | T01            | pending |
| T11 | `internal/deps/tailscale`: detect + install stub + up stub  | 0.5d  | T01            | pending |
| T12 | `internal/deps/cloudflared`: detect + install + config gen  | 1d    | T01, T06       | pending |
| T13 | `internal/deps/ufw`: detect + configure idempotently        | 0.5d  | T01            | pending |
| T14 | `internal/deps`: unit tests (mock executor for all deps)    | 1d    | T10-T13        | pending |
| T15 | `internal/tui/wizard`: generic step wizard Bubble Tea model | 0.5d  | —              | pending |
| T16 | `internal/tui/wizard/freshinstall`: steps 1–5 (preflight,   | 1d    | T04, T14, T15  | pending |
|     | deps, tailscale, cloudflared variant A/B, UFW)              |       |                |         |
| T17 | `internal/tui/wizard/freshinstall`: steps 6–10 (hardening,  | 1d    | T09, T16       | pending |
|     | npm install openclaw, overlay-API stub install, summary)    |       |                |         |
| T18 | Wire wizard into TUI menu item 1                            | 0.25d | T17            | pending |
| T19 | Idempotency integration test (run wizard twice in Docker)   | 0.5d  | T18            | pending |
| T20 | `docs/install.md` — fresh install guide                     | 0.25d | T17            | pending |

**Total estimate:** ~11 person-days. Calendar: 2 weeks with parallel work on
T01/T05/T07/T15 in first days.

---

### T01: `internal/shell` — safe exec wrapper

**Description.** All subprocess calls in the overlay go through this wrapper.
Captures stdout+stderr, enforces a configurable timeout, emits an audit log
entry on completion (action=`shell_exec`, target=command, result=ok/error).

**Acceptance criteria.**

- [ ] `func Exec(ctx context.Context, opts ExecOpts) (ExecResult, error)` where
  `ExecOpts{Cmd []string; Timeout time.Duration; AuditLogger audit.Logger; Sudo bool}`.
- [ ] `Sudo: true` prepends `sudo` to the command.
- [ ] `ExecResult{Stdout, Stderr string; ExitCode int}`.
- [ ] Timeout enforced via `context.WithTimeout`; on expiry returns `ErrTimeout`.
- [ ] Audit log entry emitted for every call (ok or error).
- [ ] Unit tests: success, non-zero exit, timeout, sudo prepend. Coverage ≥ 80%.

---

### T02: `internal/preflight` — distro + resource checks

**Description.** Checks that the VPS is a supported distro (Ubuntu 22.04+/
Debian 12+), has ≥ 5 GB free disk, ≥ 1 GB RAM, and that `systemd`, `useradd`,
`loginctl` are present in PATH.

**Acceptance criteria.**

- [ ] `func CheckAll(ctx context.Context, exec shell.Executor) ([]CheckResult, error)`.
- [ ] `CheckResult{Name, Status string; OK bool; Detail string}`.
- [ ] Distro parsed from `/etc/os-release`.
- [ ] Disk checked via `syscall.Statfs` (pure Go, no exec).
- [ ] RAM checked via `/proc/meminfo` parsing.
- [ ] Tool presence checked via `exec.LookPath`-equivalent.
- [ ] Each check is individually pass/warn/fail with human-readable `Detail`.

---

### T03: `internal/preflight` — port conflict check

**Description.** Runs `ss -tlnp` (or parses `/proc/net/tcp`) to detect
anything listening on ports 18789–19999 before we allocate from that pool.

**Acceptance criteria.**

- [ ] Returns list of `{Port, PID, Process}` for occupied ports in range.
- [ ] If list is non-empty, emits a warning (not a hard error) — admin decides.
- [ ] Works with mock executor in tests.

---

### T04: `internal/preflight` unit tests

**Acceptance criteria.**

- [ ] `TestDistroCheckUbuntu22` (mock `/etc/os-release`).
- [ ] `TestDistroCheckUnsupported` (e.g. Alpine).
- [ ] `TestDiskCheckPass` / `TestDiskCheckFail`.
- [ ] `TestRAMCheckPass` / `TestRAMCheckFail`.
- [ ] `TestPortConflictDetected` (mock `ss` output).
- [ ] Coverage ≥ 80% for `internal/preflight`.

---

### T05: `internal/config` — overlay config YAML

**Description.** Read/write `/etc/openclaw-multi/config.yml`. Schema:

```yaml
domain: ""           # Cloudflare domain (e.g. example.com)
subdomain: "openclaw" # wildcard prefix
tunnel_id: ""        # cloudflared tunnel UUID
tunnel_mode: "account"  # "account" | "quick"
port_range_start: 18789
port_range_step: 20
node_version_min: "22.16.0"
notifications:
  telegram_token: ""
  telegram_chat_id: ""
```

**Acceptance criteria.**

- [ ] `func Load(path string) (*OverlayConfig, error)` — returns defaults if
  file absent.
- [ ] `func Save(path string, cfg *OverlayConfig) error` — writes atomically
  (temp file + rename).
- [ ] JSON-Schema for config in `schemas/overlay-config.schema.json`.
- [ ] Unit tests: load defaults, load from file, save, save+reload roundtrip.

---

### T06: `internal/config` — envsubst template renderer

**Description.** Renders files in `templates/` by substituting `${VAR}` tokens
from a `map[string]string`. Used to generate systemd units, cloudflared config,
sysctl file, etc.

**Acceptance criteria.**

- [ ] `func Render(tmplPath string, vars map[string]string) (string, error)`.
- [ ] Unknown `${VAR}` tokens → error (fail fast, no silent empty strings).
- [ ] `func RenderToFile(tmplPath, outPath string, vars map[string]string, mode os.FileMode) error`
  — writes atomically.
- [ ] Unit tests with fixture templates. Coverage ≥ 80%.

---

### T07: Config templates

**Description.** Create all template files needed for Phase 1 install steps.

**Acceptance criteria.**

- [ ] `templates/sysctl-overlay.conf` — kernel hardening params
  (`kernel.yama.ptrace_scope=2`, `kernel.dmesg_restrict=1`,
  `net.ipv4.conf.all.rp_filter=1`).
- [ ] `templates/fstab-proc-hidepid` — single fstab line fragment for hidepid=2.
- [ ] `templates/profile-d-openclaw.sh` — sets `umask 0077` and
  `NODE_COMPILE_CACHE=/var/cache/openclaw-compile`.
- [ ] `templates/cloudflared-config.tmpl` — minimal cloudflared config with
  `${TUNNEL_ID}`, `${TUNNEL_CREDENTIALS_FILE}`, catch-all 404 ingress.
- [ ] `templates/openclaw-overlay-api.service.tmpl` — systemd unit for the
  daemon (stub for now: `ExecStart=/usr/local/bin/openclaw-overlay-api`).
- [ ] `templates/openclaw-gateway.service.tmpl` — per-user systemd --user unit
  (used in Phase 2, stub here).
- [ ] `make schema-check` validates all templates exist.

---

### T08: `internal/hardening` — host hardening writes

**Description.** Applies sysctl, hidepid, profile.d using the shell executor
and template renderer. All operations are idempotent (check before write).

**Acceptance criteria.**

- [ ] `func ApplySysctl(ctx, exec, renderer) error` — writes
  `/etc/sysctl.d/openclaw-overlay.conf`, runs `sudo sysctl --system`.
  Skip if file already identical.
- [ ] `func ApplyHidepid(ctx, exec, renderer) error` — adds hidepid=2 line to
  `/etc/fstab` if not present; runs `sudo mount -o remount /proc`.
- [ ] `func ApplyProfile(ctx, exec, renderer) error` — writes
  `/etc/profile.d/openclaw.sh`; creates `/var/cache/openclaw-compile` with
  `chmod 1777`.
- [ ] Each function idempotent: calling twice produces no error and no change.
- [ ] Audit log entry per operation.

---

### T09: `internal/hardening` unit tests

**Acceptance criteria.**

- [ ] Mock executor captures what commands were called.
- [ ] `TestApplySysctlIdempotent` — second call issues no sudo commands.
- [ ] `TestApplyHidepidAddsLine`.
- [ ] `TestApplyProfileCreatesFile`.
- [ ] Coverage ≥ 80%.

---

### T10: `internal/deps/node` — Node.js detect + install

**Description.** Idempotent: if Node.js ≥ 22.16 is already present, skip.

**Acceptance criteria.**

- [ ] `func EnsureNode(ctx, exec, cfg) error`.
- [ ] Detect: `node --version` → parse semver, compare to `cfg.NodeVersionMin`.
- [ ] If missing or too old: run nodesource setup script (stubbed in tests).
- [ ] Returns `ErrNodeUnsupportedDistro` if distro is not Debian/Ubuntu.

---

### T11: `internal/deps/tailscale` — detect + install stub

**Description.** Idempotent install. Interactive `tailscale up --ssh` is
stub-able for non-interactive/test environments.

**Acceptance criteria.**

- [ ] `func EnsureTailscale(ctx, exec, interactive bool) (TailscaleStatus, error)`.
- [ ] `TailscaleStatus{Installed, Running, LoggedIn bool; IP string}`.
- [ ] If `tailscale status` exits 0 — already logged in, skip `tailscale up`.
- [ ] If `interactive=false` (test/CI) — skip `tailscale up`, return status with
  `LoggedIn=false` and no error.
- [ ] Install path: `curl -fsSL https://tailscale.com/install.sh | sudo sh`
  (stubbed in tests).

---

### T12: `internal/deps/cloudflared` — detect + install + config gen

**Description.** Most complex dep step. Two paths (§6.2 step 4):
- **Variant A**: account-based, creates named tunnel, generates config.yml.
- **Variant B**: quick tunnel fallback (ephemeral URL, no account).

**Acceptance criteria.**

- [ ] `func EnsureCloudflared(ctx, exec, cfg, mode string) error`.
- [ ] Detect: `command -v cloudflared` + check cert.pem / credentials file.
- [ ] Install: download `.deb` from cloudflared GitHub releases, `dpkg -i`.
- [ ] Variant A: call `cloudflared tunnel create openclaw-multi`, parse tunnel
  ID, write credentials JSON path to config, render `cloudflared-config.tmpl`,
  install and start `cloudflared.service`.
- [ ] Variant B: write a minimal quick-tunnel config, warn admin about ephemeral
  URLs.
- [ ] If existing `config.yml` found: timestamped backup + ask overwrite
  (represented as a `ConflictPolicy` enum: `Backup | Skip | Overwrite`).
- [ ] Validate config after write: `cloudflared tunnel ingress validate`.
- [ ] Audit log entry on every mutating step.

---

### T13: `internal/deps/ufw` — detect + configure idempotently

**Description.** UFW may already be active with existing rules. We must not
reset those — only ensure our requirements are met.

**Acceptance criteria.**

- [ ] `func EnsureUFW(ctx, exec, extraPorts []int) error`.
- [ ] Parse `ufw status verbose` to understand current state.
- [ ] If inactive: `ufw default deny incoming`, `ufw default allow outgoing`,
  `ufw allow <extraPorts>`, `ufw enable`.
- [ ] If active: verify `default deny incoming` is set; add our rules without
  disturbing others.
- [ ] Idempotent: second call on already-configured UFW is a no-op.

---

### T14: `internal/deps` unit tests

**Acceptance criteria.**

- [ ] Mock executor records calls; assertions verify correct command sequences.
- [ ] `TestEnsureNodeAlreadyInstalled` — no install commands issued.
- [ ] `TestEnsureNodeMissing` — install script called.
- [ ] `TestEnsureTailscaleRunning` — no-op.
- [ ] `TestEnsureCloudflaredVariantA` — creates tunnel, writes config, starts service.
- [ ] `TestEnsureCloudflaredExistingConfig_Backup` — backup created.
- [ ] `TestEnsureUFWAlreadyActive` — only missing rules added.
- [ ] Coverage ≥ 80% for each `internal/deps/*` file.

---

### T15: `internal/tui/wizard` — generic step wizard model

**Description.** Reusable Bubble Tea model for a linear step-by-step wizard.
Each step has a name, runs a function, shows a spinner while running, shows
result (✓ / ✗ / ⚠), and proceeds to next step automatically on success.

**Acceptance criteria.**

- [ ] `type Step interface { Name() string; Run(ctx) error; IsIdempotent() bool }`.
- [ ] `type WizardModel struct` — Bubble Tea model implementing Init/Update/View.
- [ ] Progress bar: `[3/10] Installing cloudflared...` spinner.
- [ ] On step error: shows error detail + "Retry / Skip / Abort" options.
- [ ] On all steps complete: dispatches `WizardDoneMsg`.
- [ ] Unit tests: step sequencing, error → retry flow.

---

### T16: Fresh install wizard steps 1–5

**Description.** Implement wizard steps for the first half of §6.2:
1. Pre-flight check (distro, disk, RAM, tools, ports).
2. Install/verify Node.js.
3. Install/verify Tailscale (interactive flag from TUI context).
4. Configure Cloudflare Tunnel (ask Variant A or B, collect domain/token).
5. Configure UFW (ask for extra ports).

**Acceptance criteria.**

- [ ] Each step is a concrete `Step` implementation wiring into `internal/preflight`
  and `internal/deps`.
- [ ] Step 4 shows a two-option selector (Variant A / Variant B) before running.
- [ ] Domain + API token input uses a `bubbles/textinput` form.
- [ ] Collected config is saved to `/etc/openclaw-multi/config.yml` after step 4.
- [ ] All steps idempotent: re-running the wizard from step 1 skips already-done
  steps (checks state before acting).

---

### T17: Fresh install wizard steps 6–10

**Description.** Second half of §6.2:
6. Host hardening (sysctl, hidepid, profile.d).
7. `npm install -g openclaw@latest` (with version check, no onboard).
8. Install overlay-API stub systemd service.
9. (Skipped: "Add first user" — deferred to Phase 2. Wizard shows info screen.)
10. Summary screen: what was done, file paths, next steps.

**Acceptance criteria.**

- [ ] Steps 6–8 wire into `internal/hardening` and `internal/deps`.
- [ ] Step 8: copies `bin/openclaw-overlay-api` to `/usr/local/bin/`, renders
  `openclaw-overlay-api.service.tmpl`, runs `systemctl enable --now`.
- [ ] Step 9: info panel "Next: use menu item 3 to add users."
- [ ] Step 10: summary lists each step's outcome (✓/✗/skipped) and key paths
  (`/etc/openclaw-multi/config.yml`, `/var/log/openclaw-multi/audit.log`).
- [ ] State persisted in state.db: `meta` key `install_completed = true` with
  timestamp.

---

### T18: Wire wizard into TUI menu item 1

**Acceptance criteria.**

- [ ] Menu item 1 launches `freshinstall.WizardModel` instead of placeholder.
- [ ] Pressing `q`/`Esc` mid-wizard prompts "Abort installation? (y/n)".
- [ ] On wizard completion, returns to main menu with status bar updated
  (shows installed state placeholders until Phase 4 health check is built).

---

### T19: Idempotency integration test

**Description.** Docker-based test (ubuntu:22.04) that runs the wizard twice
and verifies the second run is a no-op for already-done steps.

**Acceptance criteria.**

- [ ] `test/e2e/phase-1/idempotency.sh`:
  1. Runs wizard (non-interactively via `expect`) with mock cloudflared/tailscale.
  2. Verifies `meta.install_completed` in state.db.
  3. Re-runs wizard — checks that no error occurs and audit log shows
     "already configured, skipping" entries.
- [ ] `make test-phase-1` runs this script.
- [ ] CI updated to include `test-phase-1` in the `shellcheck` + build gate.

---

### T20: `docs/install.md`

**Acceptance criteria.**

- [ ] Covers: prerequisites, running `openclaw-multi`, going through wizard,
  Variant A vs B Cloudflare choice, expected outcome.
- [ ] Includes troubleshooting section for common failures (UFW conflict, port
  conflict, Node.js version too old).
- [ ] ≤ 300 lines.

---

## 6. Definition of Done for Phase 1

- [ ] All 20 tasks completed
- [ ] `make ci` passes on `main`
- [ ] `make test-phase-1` passes (Docker idempotency test)
- [ ] Coverage ≥ 80% on all new `internal/*` packages
- [ ] `docs/install.md` merged
- [ ] CHANGELOG updated under `## v0.1.0`
- [ ] Tag `v0.1.0` created (no public release)
- [ ] `docs/phases/phase-1-retro.md` written

---

## 7. Phase smoke test suite

| Test                    | What it proves                                                  |
|-------------------------|-----------------------------------------------------------------|
| `phase-1/idempotency.sh`| Wizard runs twice, second run is no-op, state.db reflects done |

---

## 8. Documentation deliverables

- [ ] `docs/install.md` (T20)
- [ ] Inline godoc on all exported types in new packages
- [ ] CHANGELOG `## v0.1.0` entry

---

## 9. Risks and mitigations

| Risk                                              | Mitigation                                                         |
|---------------------------------------------------|--------------------------------------------------------------------|
| cloudflared CLI changes tunnel create flags       | Use Context7 to fetch current cloudflared docs before implementing |
| Tailscale interactive flow is untestable in CI    | Stub with `interactive=false` path; real flow manual-tested        |
| hidepid=2 breaks some systemd services            | Document known incompatibilities; make hidepid optional            |
| npm install hangs in Docker (no network)          | Mock npm in Docker test; real network only in e2e VM tests         |

---

## 10. Open questions to resolve before phase ends

- (none at start)
