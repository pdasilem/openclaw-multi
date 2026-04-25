---
phase: 5
title: "Network management"
slug: network-management
estimated_duration: "1 week"
status: implemented
approved_at: "2026-04-25"
approved_by: owner
master_plan_section: "§6.6 Network and firewall, §9 Phase 5"
prior_retros: ["phase-0-retro.md", "phase-1-retro.md", "phase-2-retro.md", "phase-3-retro.md", "phase-4-retro.md"]
external_docs_checked:
  - "https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/"
  - "https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/do-more-with-tunnels/local-management/"
  - "https://developers.cloudflare.com/api/resources/dns/subresources/records/methods/create/"
  - "https://developers.cloudflare.com/dns/manage-dns-records/how-to/create-dns-records/"
  - "https://tailscale.com/kb/1242/tailscale-serve"
---

# Phase 5: Network Management

> **Test-only phase.** No code in this phase runs real `tailscale`,
> `cloudflared`, `ufw`, `ss`, `lsof`, `curl`, `ping`, Cloudflare API calls, DNS
> writes, service restarts, or credential rotation during tests. All system and
> HTTP interaction must go through injectable interfaces and be covered with
> mocks.
>
> Docker is not used as a VPS simulator. Final system verification remains
> deferred to owner-run checks on an actual VPS.

---

## 1. Goals

1. TUI menu item **5. Network and firewall** becomes a real network-management screen.
2. Add structured network models for Tailscale, Cloudflare Tunnel, UFW, open
   ports, DNS wildcard state, and connectivity probes.
3. Provide read-only status and diagnostics for:
   - Tailscale admin-only state;
   - Cloudflare Tunnel state and route inventory;
   - UFW policy/rules;
   - open host ports;
   - public URL connectivity.
4. Add mockable Cloudflare DNS API wrapper for wildcard record create/update
   planning and execution.
5. Keep cloudflared config regeneration and SIGHUP out of this phase. That is
   Phase 6 overlay-API daemon work.
6. Keep live-tail logs out of this phase. That is Phase 8.
7. `make ci` passes.

---

## 2. Out of Scope

- Live VPS execution, Docker VPS simulation, or real network integration tests.
- Starting, stopping, restarting, or reloading `cloudflared`, `tailscaled`, or
  UFW during tests.
- Mutating `/etc/cloudflared/config.yml`, regenerating ingress config, or
  sending SIGHUP to `cloudflared`. These belong to Phase 6.
- Implementing `openclaw-overlay-api` UNIX-socket daemon or HTTP endpoints.
  That is Phase 6.
- Per-user plugin watcher and automatic plugin route publishing. That is Phase
  7.
- Live-tail logs for cloudflared/systemd. That is Phase 8.
- Full security audit workflow. That is Phase 9.
- Diagnostic snapshot archive. That is Phase 10.
- Real Cloudflare credential rotation. Phase 5 may model a confirmation flow
  and document the operational requirements, but actual rotation is deferred
  until there is a tested live VPS procedure.

---

## 3. Inputs

- Code state at start: main after Phase 4 commit `8035a09`.
- Go baseline: `go 1.26.2`.
- TUI baseline: `charm.land/bubbletea/v2 v2.0.6`; `View()` returns
  `tea.View`, tests use `tea.KeyPressMsg`.
- Existing abstractions:
  - `shell.Executor` and `shell.FS` are the system boundaries.
  - `internal/deps` already has initial wrappers/parsers for Tailscale,
    cloudflared, and UFW install flows from Phase 1.
  - `internal/doctor` already has shallow read-only service/port checks from
    Phase 4.
  - `internal/state` stores users and routes.
  - Phase 2 route publisher is state-backed only; real cloudflared mutation is
    still deferred to Phase 6.
- Current external constraints:
  - Cloudflare Tunnel is outbound-only; UFW does not need inbound rules for
    cloudflared itself.
  - Cloudflare DNS records are managed through the DNS Records API, including
    `POST /zones/{zone_id}/dns_records` for create.
  - Cloudflare docs currently recommend remotely-managed tunnels for most use
    cases, but this project still uses locally-managed tunnel conventions until
    the master plan is revised.

---

## 4. Architecture for This Phase

### Package ownership

Add a new package:

```text
internal/network/
  models.go          <- NetworkReport, TailscaleInfo, CloudflareInfo, UFWInfo, PortInfo, ProbeResult
  manager.go         <- Manager with Snapshot/Probe/PlanWildcardDNS/ApplyWildcardDNS
  cloudflare_dns.go  <- DNS client interface + HTTP implementation
  parsers.go         <- tailscale/cloudflared/ufw/ss/curl parsers
  manager_test.go    <- MockExecutor + fake DNS client + temp state DB
```

`network.Manager` must not import `internal/tui` or call `os/exec` directly.
Subprocess calls go through `shell.Executor`. Filesystem operations go through
`shell.FS`. Cloudflare API calls go through an injectable HTTP/DNS client.

### Result model

Use structured models, not raw terminal text:

```go
type NetworkReport struct {
    Tailscale  TailscaleInfo
    Cloudflare CloudflareInfo
    UFW         UFWInfo
    Ports       []PortInfo
    Probes      []ProbeResult
}
```

Keep display formatting in TUI. Backend objects should be stable and testable.

### Tailscale section

Phase 5 Tailscale is **admin-only**:

- show installed/running/logged-in state;
- show tailnet IP if available;
- show machine name / user / backend state when present in `tailscale status
  --json`;
- show a prominent note: managed OpenClaw users do not need Tailscale identity;
- do not add users to Tailscale;
- do not run `tailscale up` from the network screen.

This can reuse or refine `internal/deps.EnsureTailscale` parsing, but Phase 5
should expose a read-only `InspectTailscale` path.

### Cloudflare Tunnel section

Phase 5 Cloudflare is diagnostic plus DNS management:

- inspect `cloudflared --version`;
- inspect `systemctl is-active cloudflared`;
- inspect configured `TunnelID`, `TunnelMode`, `domain`, and `subdomain`;
- list state-backed routes from `state.routes`;
- show each route as hostname / target local port / owner user / enabled state /
  last_seen cache if available;
- parse `cloudflared tunnel info <tunnel>` or equivalent output if available;
- validate intended ingress config with `cloudflared tunnel ingress validate`
  only when a config path exists;
- do not write cloudflared config;
- do not SIGHUP/restart cloudflared.

Cloudflare docs currently distinguish locally-managed and remotely-managed
tunnels. The Phase 5 implementation should keep the current locally-managed
path but record this as a design pressure for a future master-plan update if
the project switches to remotely-managed tunnels later.

### Cloudflare DNS wildcard

Phase 5 may manage the wildcard DNS record for:

```text
*.${subdomain}.${domain}
```

The DNS wrapper must:

- use injected client, never raw `http.DefaultClient` directly in business
  logic;
- support dry-run planning;
- list existing records by name/type;
- create missing CNAME records through Cloudflare DNS Records API;
- update existing matching records only after explicit confirmation;
- never delete records in Phase 5 unless the record was created by this tool and
  the user explicitly confirms.

The DNS target content depends on the tunnel mode:

- account/named tunnel: target should be the Cloudflare tunnel hostname format
  derived from tunnel identity if available;
- quick tunnel: DNS management should be disabled because the URL is ephemeral.

If required identifiers like Cloudflare zone ID or API token are not in config,
the TUI should show a clear precondition error and avoid mutation.

### UFW section

Phase 5 UFW should inspect and optionally apply a conservative policy:

- parse `ufw status verbose`;
- show active/inactive;
- show default incoming/outgoing policy;
- show allowed TCP ports;
- show whether required extra public ports from config are present;
- optionally call existing `deps.EnsureUFW` through a network manager action
  after explicit confirmation.

UFW must not be reset. Existing rules must be preserved.

### Open ports section

Read-only inspection:

- run `ss -ltnp` through `Executor`;
- parse listener protocol/address/port/process when present;
- flag public listeners on `0.0.0.0` or `[::]`;
- mark expected ports from state/config separately from unknown listeners.

This complements Phase 4 doctor output but provides a richer per-port table.

### Connectivity probes

Phase 5 connectivity checks are explicit actions:

- probe public gateway URLs from `state.routes` with `curl -fsS --max-time`;
- optionally probe DNS resolution with `getent hosts` or `dig` if available;
- ping is optional and should not be required for success because ICMP may be
  blocked;
- results are stored only in the in-memory report unless a later phase adds
  persisted network observations.

No probe should run automatically on TUI screen open.

### Cloudflared last_seen parser

Phase 5 should add a parser for cloudflared logs to extract hostname hits and
timestamps from supplied text fixtures. It should not implement live-tail. It
may update `state.routes.last_seen_cached_at` and `last_seen_value` only through
a clearly named method called by an explicit TUI action.

If log format is too unstable, the implementation should keep parser behavior
conservative and record the limitation in the retro.

### TUI screen

Menu item 5 should render:

```text
Network and Firewall

Tailscale (admin-only):
  [ok] running, logged in, 100.x.y.z
  Note: managed users do not need Tailscale identity.

Cloudflare Tunnel:
  [ok] cloudflared active
  mode: account
  wildcard: *.openclaw.example.com
  routes: 3 active, 0 disabled

UFW:
  [ok] active, default deny incoming / allow outgoing
  allowed: 8080/tcp

Open ports:
  [ok] 127.0.0.1:18789 alice gateway
  [warn] 0.0.0.0:8080 public listener

r refresh   p probe URLs   d wildcard DNS   u review UFW   q back
```

Actions:

- `r`: refresh read-only snapshot;
- `p`: run explicit connectivity probes;
- `d`: review/apply wildcard DNS plan;
- `u`: review/apply UFW conservative ensure plan;
- `q`/`Esc`: back.

All mutating actions must show a review step before execution.

---

## 5. Atomic Tasks

| ID  | Title                                                   | Est.  | Depends | Parallel | Status  | PR  |
|-----|---------------------------------------------------------|-------|---------|----------|---------|-----|
| T01 | Add network result models                               | 0.25d | —       | yes      | done    | —   |
| T02 | Create `internal/network.Manager` skeleton              | 0.5d  | T01     | no       | done    | —   |
| T03 | Implement read-only Tailscale inspector                 | 0.5d  | T02     | yes      | done    | —   |
| T04 | Implement Cloudflare Tunnel inspector                   | 0.75d | T02     | yes      | done    | —   |
| T05 | Implement UFW inspector and conservative ensure plan     | 0.75d | T02     | yes      | done    | —   |
| T06 | Implement open-port parser and inspector                | 0.5d  | T02     | yes      | done    | —   |
| T07 | Implement connectivity probe runner                     | 0.5d  | T02     | no       | done    | —   |
| T08 | Implement Cloudflare DNS client interface + fake        | 0.75d | T02     | no       | done    | —   |
| T09 | Implement wildcard DNS plan/apply flow                  | 0.75d | T08     | no       | done    | —   |
| T10 | Implement cloudflared last_seen log parser              | 0.5d  | T02     | yes      | done    | —   |
| T11 | Add audit actions for network refresh/probe/DNS/UFW     | 0.25d | T07-T09 | yes      | done    | —   |
| T12 | Wire menu item 5 to a real TUI screen                   | 1d    | T02-T11 | no       | done    | —   |
| T13 | Add `docs/network.md`                                   | 0.5d  | T12     | yes      | done    | —   |
| T14 | Update CHANGELOG and write Phase 5 retro                | 0.25d | all     | no       | done    | —   |

---

### T01: Network Result Models

**Description.** Add stable data models for network status and plans.

**Acceptance criteria.**

- [x] Models cover Tailscale, Cloudflare Tunnel, UFW, routes, open ports, DNS
      plan, and probe results.
- [x] Status enum uses `ok`, `warn`, `fail`, `skipped`.
- [x] Exported types have Godoc.
- [x] Summary counts are deterministic.

**Test plan.** Unit tests for summary counts and ordering.

---

### T02: `internal/network.Manager`

**Description.** Create the orchestration type.

**Acceptance criteria.**

- [x] Constructor takes `Store`, `Executor`, config, DNS client, and audit
      logger.
- [x] No direct `os/exec`.
- [x] No direct `http.DefaultClient` in business logic.
- [x] Package does not import `internal/tui`.
- [x] Missing dependencies return clear errors.

**Test plan.** Ready-state tests and compile-time interface checks.

---

### T03: Tailscale Inspector

**Description.** Inspect admin-only Tailscale state.

**Acceptance criteria.**

- [x] Runs `tailscale status --json` through `Executor`.
- [x] Parses backend state, tailnet IP, self node, and login state when present.
- [x] Tailscale remains admin-only in docs and UI boundary.
- [x] Does not run `tailscale up`.
- [x] Malformed JSON produces `warn`, not panic.

**Test plan.** Parser tests for running, stopped, not logged in, malformed JSON,
and command failure.

---

### T04: Cloudflare Tunnel Inspector

**Description.** Inspect cloudflared and state-backed route inventory.

**Acceptance criteria.**

- [x] Runs `cloudflared --version` and `systemctl is-active cloudflared`.
- [x] Reads `TunnelID`, `TunnelMode`, `domain`, and `subdomain` from config.
- [x] Lists `state.routes` by user and route kind.
- [ ] Validates existing cloudflared config only when present. Deferred because Phase 6 owns config generation/reload.
- [x] Does not write config, SIGHUP, restart, or reload cloudflared.

**Test plan.** MockExecutor/MemFS/state tests for active, inactive, missing
config, and routes.

---

### T05: UFW Inspector and Ensure Plan

**Description.** Show firewall state and optionally apply conservative missing
rules.

**Acceptance criteria.**

- [x] Parses `ufw status verbose`.
- [x] Shows active state, default policies, and allowed TCP ports.
- [x] Produces a reviewable plan before calling `deps.EnsureUFW`.
- [x] Never resets UFW.
- [x] Preserves existing rules.

**Test plan.** Parser tests and MockExecutor command-order tests.

---

### T06: Open Port Inspector

**Description.** Parse `ss -ltnp` into structured listener rows.

**Acceptance criteria.**

- [x] Parses protocol/address/port/process when present.
- [x] Flags `0.0.0.0` and `[::]` listeners as public.
- [x] Marks expected gateway ports from state.
- [x] Unknown public listeners render as warnings.

**Test plan.** Fixture tests for loopback, wildcard IPv4, wildcard IPv6, and
malformed lines.

---

### T07: Connectivity Probes

**Description.** Probe public URLs on explicit TUI action.

**Acceptance criteria.**

- [x] Builds probe list from enabled routes with hostnames.
- [x] Runs `curl -fsS --max-time <n> <url>` through `Executor`.
- [ ] Optional DNS probe uses injected command execution. Deferred; HTTPS probe is sufficient for Phase 5.
- [x] Does not run probes automatically on screen open.
- [x] Probe failures are structured per URL.

**Test plan.** MockExecutor tests for success, timeout/non-zero, and no routes.

---

### T08: Cloudflare DNS Client Interface

**Description.** Add a mockable DNS Records API boundary.

**Acceptance criteria.**

- [x] Interface supports list/create/update for DNS records needed by wildcard
      management.
- [x] HTTP implementation accepts injected `*http.Client` or small Doer
      interface.
- [x] Uses API token bearer auth.
- [x] Does not log API tokens.
- [x] Fake client covers tests without network.

**Test plan.** HTTP request-construction tests with fake RoundTripper and fake
client tests.

---

### T09: Wildcard DNS Plan / Apply

**Description.** Plan and apply wildcard DNS record management.

**Acceptance criteria.**

- [x] Requires `domain`, `subdomain`, zone ID, API token, and non-quick tunnel
      mode before mutation.
- [x] Dry-run plan reports create/update/no-op.
- [x] Apply creates missing CNAME wildcard record through DNS client.
- [x] Apply updates existing matching record only after explicit confirmation.
- [x] Quick tunnel mode disables DNS management with clear message.

**Test plan.** Unit tests for missing preconditions, create, no-op, update, API
failure, and quick tunnel skip.

---

### T10: Cloudflared `last_seen` Parser

**Description.** Parse supplied cloudflared logs for route hit timestamps.

**Acceptance criteria.**

- [x] Parser accepts text fixtures and returns hostname/timestamp observations.
- [x] Unknown formats are ignored conservatively.
- [x] Explicit update method can write `last_seen_cached_at` /
      `last_seen_value` to state for known routes.
- [x] No live-tail implementation.

**Test plan.** Fixture parser tests and state update tests.

---

### T11: Audit Events

**Description.** Add audit actions for network operations.

**Acceptance criteria.**

- [x] New actions: `network_refresh`, `network_probe`, `dns_update`,
      `ufw_update`.
- [x] Mutating DNS/UFW actions emit result and target details.
- [x] Probe actions emit summary counts but no secrets.
- [x] Error paths emit `result=error`.

**Test plan.** Audit recorder tests in `internal/network`.

---

### T12: TUI Network Screen

**Description.** Replace menu item 5 placeholder with a real screen.

**Acceptance criteria.**

- [x] Menu item 5 opens the Network and Firewall screen.
- [x] `r` refreshes read-only snapshot.
- [x] `p` runs URL probes.
- [x] `d` opens wildcard DNS review/apply flow.
- [x] `u` opens UFW review/apply flow.
- [x] Empty, loading, success, warning, failure, and service-unavailable states
      render clearly.

**Test plan.** Bubble Tea model tests with fake network service.

---

### T13: `docs/network.md`

**Description.** Document network-management behavior and boundaries.

**Acceptance criteria.**

- [x] Documents Tailscale admin-only boundary.
- [x] Documents Cloudflare Tunnel outbound-only model.
- [x] Documents UFW policy expectations.
- [x] Documents wildcard DNS prerequisites.
- [x] Documents Phase 5 vs Phase 6 boundary.
- [x] Documents test-only/live VPS boundary.

**Test plan.** Manual doc review.

---

### T14: CHANGELOG and Retro

**Description.** Update release notes and write the Phase 5 retrospective after
implementation.

**Acceptance criteria.**

- [x] `CHANGELOG.md` has Phase 5 additions under `## v0.5.0`.
- [x] `docs/phases/phase-5-retro.md` is written.
- [x] Retro records deviations from this plan and real coverage numbers.
- [x] Retro explicitly records any Cloudflare/Tailscale CLI/API mismatch found
      during implementation.

**Test plan.** Manual doc review.

---

## 6. Definition of Done for Phase 5

- [x] All 14 tasks completed, with documented deferrals for optional DNS probe and config validation.
- [x] `make ci` passes.
- [x] Coverage >= 80% on new `internal/network` package.
- [x] TUI menu item 5 is no longer a placeholder.
- [x] Network code uses only injected `shell.Executor` and DNS
      client interfaces for external access.
- [x] No live Tailscale, cloudflared, UFW, DNS, curl, ping, or Cloudflare API
      execution in tests.
- [x] DNS/UFW mutations require explicit review/apply flow.
- [x] No cloudflared config regeneration or SIGHUP is implemented in Phase 5.
- [x] `docs/network.md` merged.
- [x] `CHANGELOG.md` updated.
- [x] `docs/phases/phase-5-retro.md` written.

---

## 7. Phase Smoke Test Suite

There are **no Docker or live network integration tests** in Phase 5.
Verification is limited to unit tests, TUI model tests, builds, and CI.

| Test level  | What runs                                                   | Where           |
|-------------|-------------------------------------------------------------|-----------------|
| Unit        | Network manager, parsers, DNS fake, UFW/DNS plans, audit    | `go test ./...` |
| TUI model   | Network screen flows with fake network service              | `go test ./...` |
| Build check | All three binaries compile                                  | `make ci`       |
| CI          | Lint + race tests + builds + schema checks                  | `make ci`       |

Optional manual dev check: `make dev`, open menu item 5, verify network groups,
probe flow, DNS review, UFW review, and error states render. This must not
mutate live network services unless the owner explicitly runs it later on a VPS.

---

## 8. Documentation Deliverables

- [x] `docs/network.md`
- [x] CHANGELOG entry for Phase 5
- [x] Godoc on exported `internal/network` types
- [x] `docs/phases/phase-5-retro.md`

---

## 9. Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Phase 5 drifts into Phase 6 overlay-API work | Explicitly ban cloudflared config writes/SIGHUP and daemon endpoints in acceptance criteria |
| Cloudflare Tunnel docs favor remotely-managed tunnels while current plan uses local config | Keep implementation compatible with current master plan, document pressure to revise architecture in retro if needed |
| DNS API changes or token permissions fail on VPS | Use official DNS Records API shape, mock request construction, and surface precise precondition/API errors |
| Connectivity probes create noisy false negatives | Probes are explicit actions, per-route, and do not gate normal status rendering |
| UFW changes could lock out admin access | No reset; review plan before apply; preserve existing rules; only add conservative missing rules |
| Cloudflared logs are format-unstable | Conservative parser, fixture tests, and no live-tail until Phase 8 |

---

## 10. Decisions

- Phase 5 owns Network and Firewall TUI only.
- Tailscale remains admin-only; managed users do not get Tailscale identities.
- Cloudflare Tunnel remains the public inbound path and is outbound-only from
  the VPS perspective.
- UFW default remains deny incoming / allow outgoing.
- DNS/UFW mutation is explicit review/apply, never automatic on screen open.
- Cloudflared config regeneration and reload remain Phase 6.
- Docker is not used for VPS simulation.
