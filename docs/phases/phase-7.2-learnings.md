---
phase: 7.2
phase_name: "Idempotent User Bootstrap and Readiness Gate"
source_artifacts:
  - "docs/phases/phase-7.2-idempotent-user-bootstrap.md"
missing_artifacts:
  - "PLAN.md"
  - "SUMMARY.md"
  - "VERIFICATION.md"
  - "UAT.md"
  - ".planning/STATE.md"
counts:
  decisions: 4
  lessons: 4
  patterns: 4
  surprises: 3
---

# Phase 7.2 Learnings: Idempotent User Bootstrap and Readiness Gate

## Decisions

- `Add user` is the repair path. A separate repair command was rejected because it would split lifecycle convergence across multiple mental models.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

- `Add user` now means `EnsureActiveManagedUser(username)`: converge a tenant to one target state, `state.db records user as active only after readiness passes`.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

- Readiness is a shared lifecycle contract reused by add, reactivate, doctor, and E2E validation.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

- OpenClaw doctor must run in the same tenant runtime context as user services, using runtime env and gateway token source rather than assuming a plain shell environment.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

## Lessons

- Existing active state is not enough evidence of runtime readiness. Repeated `Add user` must re-check readiness even when state says `active`.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

- Partial failure handling must be visible and conservative: show the failing command/output, leave health failed, and avoid silently presenting a tenant as ready.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

- E2E checks that only prove Linux account creation are too weak for this system. Readiness must include OpenClaw config, overlay config, systemd user services, gateway route, doctor, and state.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

- Doctor warnings about missing config/token/runtime context are not acceptable green E2E results because they mean the process was launched outside the required tenant runtime context.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

## Patterns

- Lifecycle operations should converge toward a target state instead of branching by caller intent. `Add`, repeat add, paused reactivation, and partial recovery all use the same readiness path.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

- State writes should occur after external readiness checks when the state field claims operational readiness.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

- Gateway port reuse must be stable for existing state users so idempotent reconciliation does not create route churn.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

- TUI lifecycle flows should stream every reconcile step to the embedded terminal and summarize success/failure in terms of readiness.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

## Surprises

- A user can exist in Linux, state, or both while still not being runtime-ready; the lifecycle has to treat these as convergence inputs, not success conditions.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

- The gateway token source matters for doctor. Storing or omitting token context changes whether doctor reflects the real service environment.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md

- Rollback is non-destructive: failed add/reconcile must not delete tenant home data.
  **Source:** docs/phases/phase-7.2-idempotent-user-bootstrap.md
