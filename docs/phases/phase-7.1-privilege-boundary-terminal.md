---
phase: 7.1
title: "Privilege Boundary and Embedded Terminal"
slug: privilege-boundary-terminal
estimated_duration: "1 week"
status: draft
approved_at: null
approved_by: null
master_plan_section: "Architecture correction between Phase 7 and Phase 8"
prior_retros:
  - "phase-6-retro.md"
  - "phase-7-retro.md"
external_docs_checked:
  - "https://pkg.go.dev/github.com/charmbracelet/bubbletea/v2"
  - "https://pkg.go.dev/github.com/creack/pty"
  - "https://pkg.go.dev/golang.org/x/term"
---

# Phase 7.1: Privilege Boundary and Embedded Terminal

> **Approval required before implementation.** This phase changes the
> architecture of how the admin TUI runs privileged work. It must be reviewed
> and approved before code changes continue.

---

## 1. Problem

The current VPS test exposed a core identity bug:

- `sudo openclaw-multi` runs the TUI process as `root`.
- The real admin actor is the SSH user, for example `ubuntu`.
- The code sometimes checks or records the process identity (`root`) where it
  should use the admin actor identity.

This creates repeated ambiguity:

- auth sees `root` instead of configured admin;
- audit can record the wrong actor;
- Cloudflare CLI state can land under `/root/.cloudflared`;
- future terminal/password prompts are hard to support because the whole TUI is
  already running as root.

The target architecture is:

- start the TUI as the admin user, without `sudo`;
- run privileged operations through explicit `sudo` commands only when needed;
- show command execution and interactive sudo/password prompts in a collapsed
  terminal panel inside the TUI.

---

## 2. Goals

1. Make `openclaw-multi` TUI run as the configured admin user without `sudo`.
2. Keep root privileges only at the operation boundary: `sudo systemctl`,
   `sudo install`, `sudo useradd`, `sudo ufw`, and other explicit privileged
   commands.
3. Add a collapsed bottom terminal panel to every TUI screen, including
   welcome/login/first-run.
4. Mirror every command started by the TUI into the terminal panel, including
   command, working directory, stdout, stderr, exit status, and duration.
5. Support interactive commands through a PTY so `sudo` can ask for a password
   when the VPS requires one.
6. Update install/runbook/E2E docs so owner-run VPS testing follows the new
   flow.

---

## 3. Non-Goals

- Do not implement Phase 8 monitoring dashboard.
- Do not add scripts as a workaround for missing privilege handling.
- Do not store root/admin passwords.
- Do not make the overlay daemon public.
- Do not replace Cloudflare locally-managed tunnel architecture.
- Do not continue with `sudo openclaw-multi` as the normal TUI launch path.
- Do not include terminal history in diagnostic snapshots. Terminal and logs
  are separate surfaces.

---

## 4. Target Architecture

### 4.1 Identities

Use explicit terms everywhere:

```text
actor/admin user
  The configured Linux admin username. Starts the TUI. Used for auth, audit,
  Cloudflare CLI state, and ownership of the checkout.

process user
  The OS user running the current process. For normal TUI this equals the
  admin user. For privileged child commands this is root.

managed user
  Tenant Linux user managed by OpenClaw Multi. Never admin. Never sudo.
```

### 4.2 Process Model

Normal launch:

```bash
openclaw-multi
```

System bootstrap or binary installation can still use root explicitly:

```bash
sudo make install
sudo openclaw-multi system-prepare
```

The normal interactive TUI must reject direct root launch unless a dedicated
recovery command is used.

### 4.3 Privileged Operations

All privileged operations go through one command gateway:

```text
internal/shell/
  runner.go        command lifecycle, audit event, terminal event stream
  sudo.go          explicit sudo command construction
  pty.go           interactive PTY runner
  fileops.go       privileged write/install helpers
```

Direct writes to `/etc`, `/var/lib`, `/var/log`, `/run`, and systemd paths must
be audited and either:

- performed during `system-prepare`; or
- performed through explicit privileged file operations.

No package should silently call `os.WriteFile` into privileged paths from the
admin-user TUI process.

### 4.4 Embedded Terminal

The TUI gets a bottom terminal panel:

```text
┌──────────────────────────────────────────────────────────────┐
│ active screen                                                │
│                                                              │
├─ Terminal: collapsed | last: sudo systemctl status ... OK ───┤
```

Owner decisions:

- terminal is available on every TUI screen;
- terminal is collapsed by default;
- terminal mirrors all TUI command execution;
- terminal also allows manual work without leaving the TUI;
- terminal history limit is configurable in settings;
- default terminal history limit is 1000 lines;
- terminal history is not included in diagnostic snapshots.

Behavior:

- collapsed by default;
- visible on all screens, including welcome/login/first-run;
- expands/collapses with one documented keybinding;
- when expanded, command output scrolls inside the terminal area;
- interactive command focus can be entered and exited explicitly;
- PTY mode passes keystrokes to the child process only while terminal focus is
  active;
- command history is capped by the configured line limit;
- passwords typed into PTY are not stored separately by OpenClaw Multi.

The terminal panel is both an output mirror for TUI actions and an interactive
admin terminal. Manual commands entered there must be visible in terminal
history and audit metadata, but their output remains terminal history, not
application logs.

Phase 7.1 implements the terminal panel directly with PTY primitives. The
implementation preserves TUI layout, command mirroring, PTY password input, and
history limits without starting an external terminal emulator.

### 4.5 Sudo Policy

There are two sudo strategies:

```text
sudoers allowlist
  OpenClaw Multi creates /etc/sudoers.d/openclaw-multi with a narrow list of
  commands that the admin user may run without a password.

existing sudo policy + PTY
  OpenClaw Multi does not create sudoers rules. If sudo needs a password, the
  embedded terminal displays the prompt and the admin types the password there.
```

Owner decision: Phase 7.1 uses existing VPS sudo policy plus PTY password
entry. `system-prepare` must not create `/etc/sudoers.d/openclaw-multi`.
Passwordless sudoers allowlist is not part of this architecture.

---

## 5. Atomic Tasks

### T01: Freeze Normal Launch Contract

**Description.** Make the normal interactive launch `openclaw-multi` without
sudo.

**Acceptance criteria.**

- [x] TUI refuses direct root launch for normal mode with a clear error.
- [x] TUI accepts launch as the configured admin user.
- [x] Existing first-run stores the admin actor as the current non-root user.
- [x] Recovery/system commands are separate from normal TUI mode.

**Test plan.** Unit tests for admin resolver and TUI startup auth.

### T02: Split Identity Model

**Description.** Replace ambiguous current-user usage with explicit actor,
process, and managed-user concepts.

**Acceptance criteria.**

- [x] `internal/admin` exposes actor/admin identity resolution.
- [x] Audit startup events use actor/admin username.
- [x] Authorization checks compare configured admin to actor/admin user, not
      process user.
- [x] Managed-user code never treats admin actor as a tenant.

**Test plan.** Unit tests for direct user, sudo environment, root rejection, and
admin mismatch.

### T03: Audit Privileged Path Writes

**Description.** Find and classify direct file writes and directory creation in
privileged paths.

**Acceptance criteria.**

- [x] Inventory covers `/etc`, `/var/lib`, `/var/log`, `/run`,
      `/etc/systemd/system`, and `/etc/cloudflared`.
- [x] Each write is assigned to either system-prepare, privileged file op, or
      non-privileged admin-owned path.
- [x] No direct privileged write remains in the normal admin-user TUI path.

**Test plan.** Static grep plus unit tests around file writer interfaces.

### T04: Add Command Event Stream

**Description.** Extend the command runner so every TUI-started command emits
structured terminal events.

**Acceptance criteria.**

- [x] Events include command, args, cwd, start, stdout chunks, stderr chunks,
      exit code, error, and duration.
- [x] Events feed the TUI terminal panel.
- [x] Events also keep existing audit logging.
- [x] Secrets are redacted in command display where they are known config
      values.

**Test plan.** Unit tests with mock executor and fake terminal sink.

### T05: Add PTY Runner for Interactive Commands

**Description.** Add an interactive PTY execution path for commands that may
need password input or full terminal behavior.

**Acceptance criteria.**

- [x] PTY runner supports resize.
- [x] PTY runner streams output to the terminal panel.
- [x] TUI can send keystrokes to PTY only while terminal focus is active.
- [x] PTY process termination returns control to the TUI.
- [x] Password text is not separately logged by OpenClaw Multi.
- [x] Manual admin commands can run from the terminal without leaving the TUI.

**Implementation notes.**

- `github.com/creack/pty` supports starting commands with a pseudo-terminal and
  resizing it.
- `golang.org/x/term` supports raw mode and password input primitives.
- Bubble Tea `ExecProcess` is useful for handing off to external interactive
  programs, but the embedded panel requires a dedicated model/event stream.

**Test plan.** Unit tests for PTY lifecycle with a harmless interactive command;
owner-run VPS test for `sudo -k` followed by a command that prompts.

### T06: Add Collapsed Bottom Terminal Panel

**Description.** Add a reusable TUI component rendered below active screens.

**Acceptance criteria.**

- [x] Panel is collapsed by default.
- [x] Panel is shown on welcome/login/first-run.
- [x] Panel shows last command summary while collapsed.
- [x] Expanded panel supports scrollback.
- [x] Scrollback line limit comes from settings.
- [x] Default scrollback line limit is 1000 lines.
- [x] Focus mode is visually obvious and routes keys to PTY.
- [x] Layout remains stable on narrow terminals.

**Test plan.** TUI model tests for collapsed/expanded/focused states and key
routing.

### T07: Convert System Operations to Explicit Sudo

**Description.** Move privileged commands from implicit root-process execution
to explicit sudo operations.

**Acceptance criteria.**

- [x] `systemctl`, `loginctl`, `useradd`, `usermod`, `userdel`, `ufw`,
      `install`, and cloudflared service operations use explicit sudo where
      required.
- [x] Cloudflare account CLI operations run as admin user without sudo.
- [x] Tenant operations run as managed user, not root.
- [x] Commands that may need password input can use PTY mode.

**Test plan.** Unit tests assert command construction and sudo boundaries.

### T08: Add System Prepare Command

**Description.** Add a one-time command for preparing root-owned directories,
permissions, and installed assets.

**Acceptance criteria.**

- [x] Command is explicit, for example `sudo openclaw-multi system-prepare`.
- [x] Creates required system directories with documented ownership and modes.
- [x] Does not run the interactive TUI.
- [x] Is idempotent.
- [x] Prints exact directory operations and completion status.

**Test plan.** Unit tests for planned filesystem operations; owner-run VPS
test on Ubuntu 24.

### T09: Update Documentation and E2E Use Cases

**Description.** Replace the old `sudo openclaw-multi` normal flow.

**Acceptance criteria.**

- [x] `docs/install.md` uses `sudo` only for install/system prepare.
- [x] `docs/vps-runbook.md` launches TUI as `openclaw-multi`.
- [x] `docs/e2e-use-cases.md` adds terminal panel checks.
- [x] Cloudflare instructions stay under admin user.
- [x] Existing Tailscale/UFW constraints remain unchanged.

**Test plan.** Documentation review plus owner-run VPS checklist.

---

## 6. E2E Checks to Add

1. Launch TUI as `ubuntu` with `openclaw-multi`; confirm no root warning.
2. Confirm direct `sudo openclaw-multi` normal launch is rejected with guidance.
3. Run Fresh install from admin-user TUI; terminal panel records each command.
4. Expand terminal panel during a long command; confirm output streams live.
5. Force sudo password prompt with `sudo -k`; run a harmless privileged command
   from TUI; enter password in terminal focus; confirm command completes.
6. Confirm password is not present in audit log, terminal history export, or
   state DB.
7. Confirm Cloudflare CLI files stay under `/home/ubuntu/.cloudflared`.
8. Confirm system files are created under `/etc`, `/var/lib`, `/var/log` only
   through system prepare or explicit sudo operations.

---

## 7. Fixed Owner Decisions

- Terminal is available always and everywhere.
- Terminal is collapsed by default.
- Terminal mirrors everything the TUI runs and also allows manual commands.
- Terminal scrollback is a setting; default is 1000 lines.
- Terminal history is not included in diagnostic snapshots. Terminal separate,
  logs separate.
- Sudo model is existing VPS sudo policy plus PTY password entry.
- `system-prepare` does not create a sudoers allowlist.
- Terminal implementation uses direct PTY primitives, not an external terminal
  emulator widget/process.

---

## 8. Definition of Done

- `make ci` passes.
- Normal TUI launch is non-root.
- Privileged operations are explicit and visible.
- Embedded terminal panel works collapsed and expanded.
- Interactive sudo prompt works through PTY on VPS.
- Docs and E2E use cases reflect the new flow.
- Retro documents any remaining privilege-boundary risks before Phase 8.
