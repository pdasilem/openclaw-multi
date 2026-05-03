# Conventions

## Go Style
- Packages are small and domain-named under `internal/`.
- Side-effecting packages expose narrow structs such as `Manager`, `Checker`, `Store`, `Service`, or `Hardener`.
- Constructors usually accept explicit dependencies: `state.Store`, `shell.Executor`, `shell.FS`, config, route publisher, and audit logger.
- Errors are returned, not logged and swallowed.
- CLI entrypoints keep process exit handling in `main` and delegate behavior to `run` helpers.

## Side Effects
- Command execution goes through `internal/shell.Executor`.
- Filesystem access goes through `internal/shell.FS` where testability matters.
- Privileged writes use explicit privileged filesystem helpers instead of scattering sudo shell snippets.
- Audit events are emitted by domain operations that mutate host, user, route, backup, or hardening state.

## Configuration
- Overlay config is a YAML file loaded by `internal/config`.
- Defaults are centralized in `config.Defaults`.
- Templates use `${VAR}` substitution through `config.Render`; unknown variables fail.
- Secrets and tokens are represented in config/state flows but command display/redaction must use shell redaction helpers.

## TUI
- Bubble Tea models are split by screen or workflow.
- TUI package orchestrates domain services; domain behavior remains in `internal/users`, `internal/network`, `internal/doctor`, `internal/backup`, and related packages.
- Long or wrapped terminal text should use existing wrapping helpers instead of direct `fmt.Fprintf` output in TUI views.

## Tests
- Unit tests are colocated with packages using `_test.go`.
- Tests prefer fake executors, memory filesystems, and temp stores over live host mutation.
- E2E and Docker checks are separate from the default package test layout.

