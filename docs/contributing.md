# Contributing

## Branch and commit conventions

Branch names follow `phase-<N>/<task-id>-<slug>`:

```
phase-0/T03-makefile
phase-1/T12-ufw-detect
```

Commit messages follow `phase-<N>/<task-id>: <imperative subject>`:

```
phase-0/T08: implement TUI shell with main menu
```

## Local development

```bash
# Run all CI checks locally (lint, test, build, schema-check):
make ci

# Run the TUI locally (no Docker required):
make dev

# Build both architectures:
make build-amd64 build-arm64

# Run unit tests with race detector:
make test-unit
```

Requirements: Go 1.22+, `golangci-lint` v1.64+, `sqlite3`, `shellcheck`.

## Phase documents

The current phase's task list lives in `docs/phases/`. Before picking a task:

1. Read the phase document for the current phase.
2. Check the task status in the table (must be `pending`).
3. Pick the highest-priority `pending` task that doesn't have unmet `dependsOn`.
4. Update the task `status` to `in_progress` and open a PR per the conventions above.
5. After merge, update status to `completed` with PR link.

New tasks may only be added to a phase document with the owner's approval.

## Pull request checklist

- [ ] Branch name matches `phase-<N>/<task-id>-<slug>`
- [ ] Commit message matches `phase-<N>/<task-id>: <subject>`
- [ ] PR description uses the PULL_REQUEST_TEMPLATE
- [ ] All acceptance criteria from the phase doc task are checked
- [ ] `make ci` passes locally
- [ ] No scope creep beyond the task

## Code review expectations

Reviewer checks:
1. All task acceptance criteria met.
2. `make lint test` passes.
3. No scope creep.
4. Any mutating action emits an audit log event.
