---
phase: 7.2
title: "Idempotent User Bootstrap and Readiness Gate"
slug: idempotent-user-bootstrap
estimated_duration: "3-5 days"
status: implemented
approved_at: 2026-05-01
approved_by: null
master_plan_section: "Architecture correction after VPS E2E"
prior_retros:
  - "phase-7.1-retro.md"
e2e_evidence:
  - "e2e-tests.md"
---

# Phase 7.2: Idempotent User Bootstrap and Readiness Gate

> **Approval required before implementation.** This phase changes the user
> lifecycle contract. It is not a narrow bug fix.

---

## 1. Problem

VPS E2E showed that `Add user` can leave a tenant in a partially created state:

- Linux user exists.
- `nvm`/Node/OpenClaw CLI can exist.
- `linger` can be enabled.
- OpenClaw Multi state can mark the user as `active`.
- But OpenClaw runtime is not ready:
  - `~/.openclaw` missing;
  - `~/.openclaw/openclaw.json` missing;
  - `~/.openclaw-overlay` missing;
  - `openclaw-gateway.service` not active;
  - `openclaw-overlay-watcher.service` not active.

Current health output proves the mismatch:

```text
Users:
  [fail] pdasilem: gateway not active
  [ok] pdasilem: linger enabled
  [ok] pdasilem: tenant OpenClaw available
  [fail] pdasilem: watcher not active

Filesystem:
  [skipped] pdasilem: /home/pdasilem/.openclaw missing
  [skipped] pdasilem: /home/pdasilem/.openclaw-overlay missing
  [skipped] pdasilem: /home/pdasilem/.openclaw/openclaw.json missing
```

This is an architectural bug: `active` currently means "the add flow reached
state write", not "the tenant is actually ready".

There must be no separate `repair user` path. A separate repair path creates a
second lifecycle command and guarantees drift between add and repair behavior.

---

## 2. Decision

`Add user` becomes an idempotent reconcile operation:

```text
Add user = EnsureActiveManagedUser(username)
```

For a requested username, the command must converge the tenant to one target
state:

```text
Linux user exists
linger enabled
user systemd manager started
Node 24 available through tenant nvm
OpenClaw CLI available through tenant wrapper
OpenClaw non-interactive onboarding completed
~/.openclaw exists with expected mode
~/.openclaw/openclaw.json exists with expected mode
~/.openclaw-overlay exists with expected mode
gateway and watcher user units exist
gateway and watcher user units are active
gateway route exists in overlay state
state.db records user as active only after readiness passes
```

Repeated `Add user` for the same username must be safe and must not require a
separate menu action.

---

## 3. Goals

1. Make `Add user` idempotent across these starting states:
   - no Linux user, no state;
   - Linux user exists, no state;
   - Linux user exists, partial tenant home, no state;
   - state user exists but tenant runtime is incomplete;
   - state user exists as `active` but services/files are broken;
   - state user exists as `paused`.
2. Prevent `active` state writes until tenant readiness is verified.
3. Reuse one readiness checker for:
   - end of `Add user`;
   - TUI health;
   - E2E validation.
4. Keep the architecture strict:
   - no forbidden tenant shell switching;
   - tenant shell commands use `sudo -u <user> -H bash -lc`;
   - user systemd commands use explicit `XDG_RUNTIME_DIR` and
     `DBUS_SESSION_BUS_ADDRESS`.
5. Make command output visible in the embedded terminal for every reconcile
   step.
6. Update E2E docs so UC-0201 validates readiness, not only Linux account
   creation.

---

## 4. Non-Goals

- Do not add a `repair user` menu item.
- Do not add fallback paths.
- Do not support interactive OpenClaw onboarding.
- Do not store plaintext gateway token in `~/.openclaw/openclaw.json`.
- Do not make managed users sudo-capable.
- Do not mark missing active-user runtime files as `skipped`.

---

## 5. Target Behavior

### 5.1 Add User From Clean State

Input:

```text
username=pdasilem
```

Expected flow:

1. Validate username.
2. Load config and validate `domain`, `subdomain`, tunnel settings.
3. Detect Linux user state.
4. Create Linux user if missing.
5. Enable linger.
6. Resolve UID.
7. Start `user@<uid>.service`.
8. Install or repair tenant NVM/Node/OpenClaw.
9. Write tenant wrappers.
10. Run non-interactive onboarding.
11. Write gateway and watcher user units.
12. Reload and enable/start user units.
13. Verify readiness.
14. Upsert state as `active`.
15. Upsert gateway route.
16. Emit audit event.

### 5.2 Add User When Linux User Already Exists

If `getent passwd <username>` succeeds but state has no user:

- reuse the Linux user;
- do not run `useradd`;
- enforce home ownership/modes needed by overlay;
- continue full reconcile;
- write state only after readiness passes.

### 5.3 Add User When State Already Exists as Active

If state has `status=active`, `Add user` must still verify runtime readiness.

If readiness passes:

- no destructive work;
- return existing user.

If readiness fails:

- rerun reconcile steps for missing/broken runtime parts;
- do not allocate a new port;
- do not rotate gateway token unless the existing token source is absent and
  the service cannot be made ready without generating a new one;
- verify readiness again;
- keep user `active` only if readiness passes.

### 5.4 Add User When State Exists as Paused

`Add user` is an active-user intent. For a paused user:

- reconcile filesystem, OpenClaw install, wrappers, and units;
- enable linger;
- start services;
- enable route;
- mark state `active` only after readiness passes.

This replaces the need for a separate repair path. Existing `Activate` remains
valid, but its implementation should call the same reconcile/readiness logic
instead of only starting services.

### 5.5 Partial Failure

If any step fails:

- show the failing command and output in the embedded terminal;
- return error to TUI;
- do not write `active` for a user that is not ready;
- preserve existing user files unless the failing step created a temporary file;
- audit the failed step.

For a pre-existing `active` state that is discovered broken:

- health must show `fail`;
- repeated `Add user` must attempt convergence;
- if convergence fails, state must not be silently presented as ready.

---

## 6. Readiness Contract

Introduce a shared readiness model:

```text
TenantReadiness
  linux_user_exists
  uid_resolved
  linger_enabled
  user_manager_available
  node_available
  openclaw_available
  openclaw_state_dir_exists
  openclaw_config_exists
  overlay_state_dir_exists
  gateway_unit_exists
  watcher_unit_exists
  gateway_active
  watcher_active
  gateway_port_listening
  gateway_route_state_exists
```

Readiness result must carry:

- status: `ok`, `warn`, `fail`;
- exact failing command or path;
- stdout/stderr where relevant;
- remediation handled by `Add user`, not by a separate repair action.

For an `active` user:

- missing `~/.openclaw` is `fail`, not `skipped`;
- missing `~/.openclaw/openclaw.json` is `fail`, not `skipped`;
- missing `~/.openclaw-overlay` is `fail`, not `skipped`;
- inactive gateway/watcher is `fail` with command stderr/status details.

For a `paused` user:

- inactive gateway/watcher is allowed;
- filesystem/config checks still run and report drift.

---

## 7. OpenClaw Doctor Context

OpenClaw doctor must run in the same tenant runtime context used by user
services.

Command shape:

```bash
sudo -u <user> -H env \
  XDG_RUNTIME_DIR=/run/user/<uid> \
  DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/<uid>/bus \
  OPENCLAW_GATEWAY_TOKEN=<token-source> \
  /home/<user>/.local/bin/openclaw doctor --json
```

The token value must come from the overlay-managed token source used by the
gateway unit. Doctor must not require the token to be stored as plaintext in
`~/.openclaw/openclaw.json`.

The warnings below must not be accepted as a green E2E result:

```text
Gateway token is managed via SecretRef and is currently unavailable.
Unable to verify gateway service token drift: gateway.auth.token SecretRef is configured but unresolved.
systemd user services are unavailable.
```

They mean the doctor process was launched without the required tenant runtime
environment.

---

## 8. Code Impact

### 8.1 `internal/users`

Refactor `Manager.Add` into explicit phases:

```text
EnsureActiveManagedUser
  LoadOrPlanUser
  EnsureLinuxUser
  EnsureLinger
  EnsureUserManager
  EnsureTenantToolchain
  EnsureOpenClawOnboarding
  EnsureTenantFiles
  EnsureUserUnits
  EnsureServicesStarted
  CheckReadiness
  CommitStateAndRoute
```

Rules:

- `UserExists` in state must not short-circuit add.
- `useradd already exists` is not an error.
- existing Linux user without state is a supported input.
- existing state with broken runtime is a supported input.
- route/state write happens after readiness, not before.
- gateway port reuse must be stable for existing state users.

### 8.2 `internal/doctor`

Update health checker:

- use shared readiness checks;
- show stderr/status details for inactive user services;
- treat missing active-user runtime paths as `fail`;
- run OpenClaw doctor with runtime env and gateway token source.

### 8.3 `internal/state`

Evaluate whether state needs an explicit intermediate status:

```text
provisioning
```

If added, it must be internal lifecycle state only. TUI should not present
`provisioning` as ready.

If not added, failed reconcile must avoid writing a new active row and must not
convert paused users to active until readiness passes.

### 8.4 TUI

`Add user` screen:

- allow entering an existing username;
- describe action as ensuring active tenant state;
- stream every step to terminal;
- on success show `user ready`;
- on failure show failing readiness item and point to terminal output.

User list:

- `active` means readiness passed at last reconcile;
- broken active users should be visually distinguishable after health check.

---

## 9. Documentation Impact

Update:

- `docs/users.md`;
- `docs/e2e-use-cases.md`;
- `docs/vps-runbook.md`;
- `docs/phases/phase-2-user-management.md`;
- `docs/phases/phase-4-doctor-health.md`;
- `docs/OPENCLAW_OVERLAY_PLAN_RU.md`.

Required doc changes:

- replace "Add user creates user" with "Add user ensures active managed user";
- state that repeated add is the supported recovery path;
- remove any implication that a separate repair command exists;
- document readiness checks as mandatory E2E criteria;
- document OpenClaw doctor runtime env requirements.

---

## 10. E2E Changes

### UC-0201 Add User

Add required checks:

```bash
id <user>
getent passwd <user>
loginctl show-user <user> -p Linger
sudo -u <user> -H bash -lc 'test -d ~/.openclaw'
sudo -u <user> -H bash -lc 'test -f ~/.openclaw/openclaw.json'
sudo -u <user> -H bash -lc 'test -d ~/.openclaw-overlay'
uid=$(id -u <user>)
sudo -u <user> env XDG_RUNTIME_DIR=/run/user/$uid DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$uid/bus systemctl --user is-active openclaw-gateway openclaw-overlay-watcher
```

Expected:

```text
active
active
```

### UC-0201 Repeat Add

Run `Add user` again for the same username.

Expected:

- no duplicate Linux user;
- no duplicate state row;
- no new port;
- no duplicate routes;
- services remain active;
- missing/broken tenant files are recreated if intentionally removed before the
  repeat add.

### UC-0202 Deactivate/Reactivate

Reactivate must call the same converge/readiness path used by `Add user`.

Expected:

- deactivate stops services and disables route;
- activate restores readiness;
- repeated add after deactivate makes the user active and ready.

---

## 11. Test Plan

Unit tests:

- `Add` with no Linux user creates and converges.
- `Add` with existing Linux user but no state converges.
- `Add` with existing active state and missing runtime files reruns converge.
- `Add` with paused state activates through readiness gate.
- readiness failure prevents `active` state write.
- route write does not happen before readiness passes.
- forbidden tenant shell forms remain absent.
- OpenClaw doctor receives runtime env and gateway token source.

Integration-style mocked executor tests:

- command order for clean add;
- command order for partial add;
- command order for existing active broken runtime;
- stderr from failed `systemctl --user is-active` appears in health result.

Manual VPS tests:

- fresh add;
- repeat add;
- delete `~/.openclaw` then repeat add;
- stop user services then repeat add;
- deactivate/reactivate;
- TUI health after each step.

---

## 12. Acceptance Criteria

- [x] `Add user` is idempotent for clean, partial, existing, active, and paused
      users.
- [x] No separate `repair user` command or menu item is introduced.
- [x] State user is not marked `active` unless readiness passes.
- [x] Active users with missing runtime files are reported as `fail`, not
      `skipped`.
- [x] TUI health includes actionable stderr/status for inactive user services.
- [x] OpenClaw doctor runs with tenant runtime env and gateway token source.
- [x] E2E UC-0201 includes repeat-add and partial-state recovery checks.
- [x] Repo-wide forbidden tenant shell search remains empty for all blocked
      root/tenant switching forms documented in Phase 7.1.
- [x] `make ci` passes.

---

## 13. Rollback

No destructive rollback is required.

If Phase 7.2 implementation fails during testing:

- keep existing Linux users and tenant homes;
- keep current state DB backup;
- rebuild previous binary;
- rerun health to document the pre-7.2 broken state.

The implementation must not delete tenant home data during add/reconcile.
