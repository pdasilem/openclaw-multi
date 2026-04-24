# OpenClaw Multi-User Overlay — Development Process

> Iterative, phase-by-phase development process for the
> `openclaw-multi` overlay project.
>
> Master architecture: [OPENCLAW_OVERLAY_PLAN_RU.md](./OPENCLAW_OVERLAY_PLAN_RU.md) (in Russian).
> Working language for code, docs, commit messages, CI logs:
> **English** (Russian is reserved for owner-facing planning docs).

---

## 1. Purpose

`openclaw-multi` is a complex project. Trying to design every
single PR upfront produces a giant document that goes stale before it
ships. Trying to write zero design and let agents improvise produces
incoherent code.

This document describes the middle path: **Per-phase just-in-time
detailing**. The master plan defines the architecture and breaks the
work into ~11 phases. Each phase is detailed _only when its turn comes_,
right after the previous one is verified done.

The benefits:

- Each phase document is fresh — informed by what was actually learned
  in earlier phases.
- Documents stay short (30–50 atomic tasks per phase) and stay relevant.
- Re-planning is cheap when surprises happen.
- Multiple agents can work in parallel within one phase, on independent
  tasks.

---

## 2. Roles

- **Owner** (you). Approves the master plan, approves each phase
  detail document, runs the agent at phase boundaries, makes go/no-go
  decisions.
- **Phase planner agent**. Given the master plan + the previous phase's
  retrospective, produces the next phase's detailed task list. One agent
  per phase planning round.
- **Implementer agent(s)**. Given the approved phase doc, work through
  the tasks (one or many in parallel). Each task → one PR (or one
  commit, depending on agent setup).
- **Reviewer agent**. After each PR, reviews against the task's
  acceptance criteria + general code quality. Blocks merge on hard
  failures.
- **Verifier agent**. At end of each phase, runs the smoke test suite
  defined in the phase doc, confirms the Definition of Done, writes a
  retrospective.

These can all be the same agent in different "modes" (just clear prompt
boundaries). They do not need to be different model instances.

---

## 3. Repository structure for the overlay project

The overlay is a **separate repository** 
(see master plan §11.2). Suggested name: `openclaw-multi`.

```
openclaw-multi/
├── .github/
│   ├── workflows/
│   │   ├── ci.yml                 # lint + test + build
│   │   └── release.yml            # tagged releases
│   ├── ISSUE_TEMPLATE/
│   └── PULL_REQUEST_TEMPLATE.md
│
├── cmd/                           # Go entry points
│   ├── openclaw-multi/             # TUI binary
│   ├── openclaw-overlay-api/       # systemd daemon
│   └── openclaw-overlay-watcher/   # per-user systemd --user daemon
│
├── internal/                      # Go internal packages
│   ├── tui/                       # Bubble Tea models, views, messages
│   ├── state/                     # SQLite state.db access
│   ├── audit/                     # JSONL audit log writer
│   ├── admin/                     # admin identification logic
│   ├── api/                       # overlay-API HTTP/UNIX-socket
│   ├── watcher/                   # inotify config watcher
│   ├── cloudflared/               # cloudflared config gen + reload
│   ├── tailscale/                 # tailscale wrappers
│   ├── ufw/                       # UFW wrappers
│   ├── notifications/             # Telegram alerts
│   ├── backup/                    # openclaw backup wrappers
│   ├── ports/                     # port allocator
│   └── shell/                     # safe sh exec helpers
│
├── pkg/                           # Public Go packages (if any)
│
├── templates/                     # Config templates (envsubst format)
│   ├── openclaw.json.tmpl
│   ├── openclaw-gateway.service.tmpl
│   ├── openclaw-overlay-watcher.service.tmpl
│   ├── cloudflared-config.tmpl
│   ├── sysctl-overlay.conf
│   └── profile-d-openclaw.sh
│
├── scripts/                       # Bash helper scripts (shebang #!/bin/bash)
│   ├── bootstrap-user.sh
│   ├── deactivate-user.sh
│   ├── activate-user.sh
│   ├── backup-user.sh
│   ├── restore-user.sh
│   ├── healthcheck.sh
│   ├── network-check.sh
│   ├── uninstall-user.sh
│   └── uninstall-overlay.sh
│
├── schemas/
│   ├── state.sql                  # SQLite DDL
│   ├── overlay-config.schema.json # JSON-Schema for /etc/openclaw-multi/config.yml
│   └── notifications.schema.json
│
├── test/
│   ├── docker/                    # Dockerfile.test (ubuntu:22.04 + systemd)
│   ├── fixtures/                  # mock plugin manifests, sample configs
│   ├── e2e/                       # full-VM scenarios (KVM/QEMU scripts)
│   └── integration/               # Go integration tests
│
├── docs/
│   ├── README.md                  # project landing
│   ├── install.md
│   ├── users.md
│   ├── network.md
│   ├── uninstall.md
│   ├── upgrading-openclaw.md
│   ├── multi-vps-future.md
│   ├── security.md
│   ├── architecture.md
│   ├── troubleshooting.md
│   ├── contributing.md
│   └── phases/
│       ├── phase-0-skeleton.md
│       ├── phase-0-retro.md
│       ├── phase-1-fresh-install.md
│       ├── phase-1-retro.md
│       └── ...                    # added as we go
│
├── man/
│   └── openclaw-multi.1
│
├── go.mod
├── go.sum
├── Makefile
├── LICENSE                        # MIT (matches OpenClaw)
├── README.md
├── CHANGELOG.md
└── .gitignore
```

The `docs/phases/` folder holds the per-phase detailed task lists and
retrospectives. **One folder, owned by the owner, never deleted.**
Older phase docs become historical record.

---

## 4. Master plan vs phase docs

| Document                                        | Audience                            | Update frequency                                    | Scope                                             |
| ----------------------------------------------- | ----------------------------------- | --------------------------------------------------- | ------------------------------------------------- |
| `OPENCLAW_OVERLAY_PLAN_RU.md` (master)          | owner, planner agent                | rarely (only when fundamental architecture changes) | full project, 11 phases, architecture, principles |
| `OPENCLAW_OVERLAY_DEV_PROCESS.md` (this)        | owner, all agents                   | rarely (only when process itself changes)           | how we work                                       |
| `docs/phases/phase-N-*.md` (phase docs)         | implementer + reviewer agent        | one per phase, frozen after approval                | atomic tasks for one phase                        |
| `docs/phases/phase-N-retro.md` (retrospectives) | owner, planner agent for next phase | one per phase, frozen after written                 | what actually happened, lessons, deviations       |

**Master plan changes** require the owner's explicit approval. If a
phase reveals an architecture mistake, the implementer agent does
**not** silently change the master plan — it surfaces a question, and
the owner amends the master plan first.

---

## 5. Phase lifecycle

Each phase goes through six steps:

```
                      ┌────────────────────┐
                      │ 1. Plan the phase  │
                      └─────────┬──────────┘
                                ↓
                      ┌────────────────────┐
                      │ 2. Owner approves  │
                      └─────────┬──────────┘
                                ↓
                      ┌────────────────────┐
                      │ 3. Implement       │ ← parallel agents possible
                      └─────────┬──────────┘
                                ↓
                      ┌────────────────────┐
                      │ 4. Review each PR  │
                      └─────────┬──────────┘
                                ↓
                      ┌────────────────────┐
                      │ 5. Verify phase    │
                      └─────────┬──────────┘
                                ↓
                      ┌────────────────────┐
                      │ 6. Retrospective   │
                      └─────────┬──────────┘
                                ↓
                          (next phase)
```

### 5.1. Step 1 — Plan the phase

Owner runs the planner agent with prompt:

> "Read `OPENCLAW_OVERLAY_PLAN_RU.md` §<phase-section>. Read all
> previous phase retrospectives in `docs/phases/`. Read the current
> repo state. Produce `docs/phases/phase-<N>-<slug>.md` following the
> template in §6 of `OPENCLAW_OVERLAY_DEV_PROCESS.md`."

The planner agent does NOT touch code. It only produces the phase
document.

### 5.2. Step 2 — Owner approves

Owner reads the phase doc, requests changes if needed, ultimately marks
the doc as **approved** by adding a frontmatter field:

```yaml
status: approved
approved_at: 2026-04-25
approved_by: <owner-name>
```

After approval, the phase doc is **frozen**. Any change requires a new
approval round.

### 5.3. Step 3 — Implement

Owner kicks off implementer agent(s). Each task in the phase doc
becomes one PR. Tasks marked `parallel: true` can be picked up
simultaneously by multiple agents. Tasks with `dependsOn: [Tx, Ty]`
must wait for those to land first.

For each task, the agent:

1. Implements only what the task says.
2. Includes tests required by the task's acceptance criteria.
3. Squashes work into **1–2 commits per phase** (не на каждую задачу отдельный коммит).
   Commit message format: `phase-<N>: <summary>`.
4. Pushes via `gh` CLI using `git -c user.name/user.email` flags — never via raw git config.
5. Updates the task status in the phase doc (`status: completed`) after push.

### 5.4. Step 4 — Review each PR

Reviewer agent on each PR:

1. Verifies all acceptance criteria of the task are met.
2. Runs `make lint test` locally / via CI.
3. Confirms no scope creep beyond the task.
4. Checks the audit log emits the documented event type for any
   user-visible action added.
5. Approves or requests changes.

After merge, the implementer agent updates the phase doc task status
to `completed` with PR link and commit SHA.

### 5.5. Step 5 — Verify phase

Verifier agent at end of phase:

1. Runs the **Phase smoke test suite** declared in the phase doc.
2. Confirms every task is `completed`.
3. Confirms every Definition of Done item is checked.
4. Creates a git tag `v0.<N>.0` — **no GitHub Release** until owner explicitly requests one.

If any DoD item fails — phase is **not** complete; new tasks are added
to the phase doc (with owner's re-approval).

### 5.6. Step 6 — Retrospective

Verifier (or the owner) writes `docs/phases/phase-<N>-retro.md`:

```markdown
# Phase <N> Retrospective

## What we built

<one-paragraph summary>

## Deviations from plan

<tasks added mid-flight, scope changes, surprises>

## Lessons for next phase

<what to do differently>

## Open issues carried forward

<bugs deferred, debt incurred, with ticket links>

## Master plan updates suggested

<if any architectural assumption proved wrong>
```

This retrospective is the **first input** to the next phase's planning.
It is **frozen** once written.

---

## 6. Phase document template

Every `docs/phases/phase-<N>-<slug>.md` MUST follow this skeleton:

```markdown
---
phase: <N>
title: "<short title>"
slug: <slug-form-of-title>
estimated_duration: "<X weeks>"
status: draft | approved | in_progress | completed
approved_at: <YYYY-MM-DD or null>
approved_by: <owner-name or null>
master_plan_section: "<section reference in master plan>"
prior_retros: [<list of phase-N-retro.md filenames consumed>]
---

# Phase <N>: <title>

## 1. Goals (what this phase delivers)

<3–5 bullet points, each ending in a verifiable outcome>

## 2. Out of scope

<3–5 bullet points, what we are explicitly NOT doing this phase>

## 3. Inputs

- Code state at start: <git tag or SHA>
- Master plan section: <link>
- Prior retros: <links>
- Open questions resolved before starting: <list>

## 4. Architecture for this phase

<concise — only what is new for this phase. Link to master plan for context.>

## 5. Atomic tasks

| ID  | Title | Est  | Depends on | Parallel | Status  | PR  |
| --- | ----- | ---- | ---------- | -------- | ------- | --- |
| T01 | ...   | 0.5d | —          | yes      | pending | —   |
| T02 | ...   | 1d   | T01        | yes      | pending | —   |
| ... |

### T01: <title>

**Description.** What needs to be done.

**Acceptance criteria.**

- [ ] Criterion 1 (verifiable, e.g. "file X exists with content Y")
- [ ] Criterion 2
- [ ] ...

**Implementation notes.** (optional) any non-obvious detail.

**Test plan.** What test(s) prove this task is done.

---

### T02: ...

(same structure)

## 6. Definition of Done for the phase

- [ ] All tasks T01..TNN have status `completed`
- [ ] All acceptance criteria checked
- [ ] `make ci` passes on `main`
- [ ] Phase smoke test suite (§7) passes end-to-end
- [ ] Documentation deliverables (§8) merged
- [ ] Release `v0.<N>.0` tagged
- [ ] Retrospective `docs/phases/phase-<N>-retro.md` written

## 7. Phase smoke test suite

End-to-end tests that prove the phase works. Each test is a script in
`test/e2e/phase-<N>/` and must be runnable as `make test-phase-<N>`.

| Test | What it proves |
| ---- | -------------- |
| ...  | ...            |

## 8. Documentation deliverables

- [ ] `docs/<...>.md` updated/created
- [ ] Inline godoc on new public APIs
- [ ] CHANGELOG.md updated under `## v0.<N>.0`

## 9. Risks and mitigations

| Risk | Mitigation |
| ---- | ---------- |

## 10. Open questions to resolve before phase ends

(if any — these become inputs to the retrospective)
```

---

## 7. Atomic task sizing

Aim for tasks that are **0.25 to 1.5 working days**.

| Size                    | When                                                            |
| ----------------------- | --------------------------------------------------------------- |
| **0.25d** (a few hours) | trivial: add a single file, single function, single linter rule |
| **0.5d**                | typical: small feature, single test, small refactor             |
| **1d**                  | a substantial unit: a new module with tests                     |
| **1.5d max**            | only when splitting would create artificial coupling            |

If a task feels bigger than 1.5d → split it. If a task feels smaller
than 0.25d → merge with a related task.

A phase of 1 calendar week ≈ 5 working days × 1.5–2 tasks/day per
agent ≈ **15–30 atomic tasks**. Phase 0 (skeleton) sits at the lower
end (~25). Heavier phases (Phase 6, 7) may have 30–40.

---

## 8. Commit conventions

- **1–2 commits per phase** (squash all task work). No per-task commits or PRs.
- Commit message format: `phase-<N>: <summary of what the phase delivers>`.
  Second commit (if needed): `phase-<N>: <specific fix or retro>`.
- Always use `git -c user.name="pdasilem" -c user.email="74730932+pdasilem@users.noreply.github.com"`.
- Push via `gh auth setup-git` + `git push origin main`.
- Linear history on `main` — force push allowed when squashing within a phase.

---

## 9. CI requirements (constant across phases)

Every PR must pass:

1. `make lint` — `golangci-lint run` clean.
2. `make test` — `go test ./...` clean, with coverage report.
3. `make build` — both `linux-amd64` and `linux-arm64` binaries build.
4. `make schema-check` — `state.sql` and JSON-Schemas are valid.
5. `shellcheck scripts/` — bash scripts pass.

Phase 0 sets these up; later phases inherit them.

---

## 10. Audit log discipline

Every user-visible action (TUI menu choice that mutates state, API
call that writes anywhere, any `sudo` invocation) **must** emit a JSONL
entry as defined in master plan §12 OQ-1. The reviewer agent rejects
PRs that introduce mutating actions without audit log calls.

---

## 11. When to update the master plan

The master plan is the source of truth for architecture. It should be
updated when:

- An architectural assumption proves wrong (e.g., cloudflared CLI changes
  break a planned flow → adopt alternative, document in master).
- A phase introduces a major component not foreseen in the original
  plan (e.g., we add a Phase 5.5 between 5 and 6).
- An open question (§12) is resolved and the resolution affects multiple
  phases.

Updates **happen only with owner approval**, applied as a single PR to
the openclaw repo (where the master plan lives), referenced from the
relevant phase retrospective.

Cosmetic updates (typos, link fixes) — direct PR, no ceremony.

---

## 12. Owner workflow at a glance

```
[Phase 0 done]
     ↓
"Run planner agent: produce phase-1 doc"
     ↓
[planner agent writes docs/phases/phase-1-fresh-install.md]
     ↓
[owner reads, requests edits, approves]
     ↓
"Run implementer agent on phase-1 tasks"
     ↓
[implementer + reviewer agents iterate, merging PRs]
     ↓
"Run verifier agent: phase-1 DoD + smoke + retro"
     ↓
[phase-1-retro.md written, v0.1.0 tagged]
     ↓
"Run planner agent: produce phase-2 doc"
     ↓
... (repeat 11 times) ...
     ↓
[v1.0.0 release]
```

Owner's per-phase touchpoints: ~3 (approve plan, mid-phase course
correct if needed, accept retro).

Estimated owner time per phase: **2–4 hours** of focused review +
optional async questions. Total owner time across all 11 phases: **~30–
40 hours over 13–14 weeks**, mostly in async bursts.

---

## 13. When NOT to use this process

This process assumes the master plan is a faithful blueprint. It is
optimized for **execution**, not for **discovery**.

Use a different mode if:

- We need to spike an unknown technology (build a throw-away prototype
  first, separately from this repo).
- The master plan itself is wrong (back to design discussion, no
  per-phase docs until master is fixed).
- A bug in production overrides the phase order (hotfix mode: branch
  off `main`, fix, ship, never on the phase backlog).

---

## 14. Quickstart for the owner

1. **Read the master plan once** end-to-end (`OPENCLAW_OVERLAY_PLAN_RU.md`).
2. **Read this process doc** (you're doing it).
3. **Read `OPENCLAW_OVERLAY_PHASE_0.md`** — this is the first phase,
   already detailed and waiting for your approval.
4. Once Phase 0 is approved, run the implementer agent with prompt:

   > "You are working on `openclaw-multi`. Read
   > `docs/phases/phase-0-skeleton.md`. Pick the next pending task by
   > ID, implement it, open a PR following §8 of
   > `OPENCLAW_OVERLAY_DEV_PROCESS.md`. After PR is merged, update
   > task status in the phase doc, repeat. Stop when all tasks are
   > completed and DoD is met."

5. After Phase 0 ships, ask the planner agent:

   > "Phase 0 is complete. Read `docs/phases/phase-0-retro.md`. Produce
   > `docs/phases/phase-1-fresh-install.md` per the template in §6 of
   > `OPENCLAW_OVERLAY_DEV_PROCESS.md`."

6. Approve, run implementer, repeat.