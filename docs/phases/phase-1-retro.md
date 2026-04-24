# Phase 1 Retrospective

## What we built

Full fresh-install wizard infrastructure with zero live VPS execution.
Seven new Go packages: `shell` (Executor + FS interfaces + mocks),
`preflight` (distro/disk/RAM/port checks), `config` (YAML overlay config +
envsubst renderer), `hardening` (sysctl/hidepid/profile.d — idempotent),
`deps` (EnsureNode/Tailscale/Cloudflared/UFW — all mock-tested), and
`tui/wizard` (generic Bubble Tea step-wizard + 10-step fresh-install wizard).
TUI menu item 1 now launches the real wizard instead of a placeholder.
All six template files written. Coverage ≥ 80% on every new package.

## Deviations from plan

- **Test-only constraint enforced**: no live VPS execution, no Docker smoke
  tests with real installs. All verification via `MockExecutor` + `MemFS`.
- **`shell.Logger` interface**: added to avoid circular deps between `shell`
  and `audit`; `*audit.Logger` satisfies it.
- **`state.ErrNoAdmin` propagation**: `GetAdmin` returns sentinel error, not
  `(nil, nil)`. Required updates to `tui/app.go` and `admin/resolver.go`.
- **Module path corrected**: `github.com/SergeiM/openclaw-multi` →
  `github.com/pdasilem/openclaw-multi` — fixed across all files in one shot.
- **Go 1.26.0** (confirmed via Context7): bubbletea v1.3.10 + modernc.org
  deps require it. CI updated accordingly.
- **golangci-lint v2 (2.11.4)**: `revive.exported` rule disabled (too noisy
  for Phase 1 internal packages); `gosimple` removed (merged into
  `staticcheck`); formatters in separate `formatters:` section.
- **`tui/wizard.Model.Update`** must return `(tea.Model, tea.Cmd)`, not
  `(Model, tea.Cmd)` — standard Bubble Tea interface requirement.
- **`hardening.emit`** simplified to not take `action`/`result` params
  (always `ActionShellExec` / `ResultOk`) after `unparam` lint.
- **T19 (Docker idempotency test)** not implemented — deferred per
  test-only constraint. Replaced by unit-level idempotency tests in
  each package.

## Lessons for next phase

- Always run `gofmt -w` on every new file before committing — saves lint
  cycles.
- Define injectable interfaces (`Executor`, `FS`, `Logger`) upfront in
  shared package; never import `audit.*` directly from `shell.*`.
- Use `context.Background()` in step constructors, not pass-through ctx, to
  avoid `unparam` warnings when ctx is always the same.
- `tea.Model.Update` signature is non-negotiable — always returns
  `(tea.Model, tea.Cmd)`. Use type-assertion helper in tests.

## Open issues carried forward

- `tui` package overall coverage 32% (TUI rendering is hard to unit-test).
  Acceptable for now; will improve with E2E tests in later phases.
- Docker smoke test (`make test-phase-1`) not written — deferred per
  test-only constraint. Will be added post-MVP.
- `schemas/overlay-config.schema.json` referenced in plan T05 but not yet
  created (not needed for runtime; deferred to Phase 9 hardening).

## Master plan updates suggested

- `go 1.26.0` is now the minimum required Go version (update §4.1).
- golangci-lint v2 config format applies to all phases (update §2).
- Test-only constraint (no live VPS execution until full MVP) — add as
  explicit process rule (already added to DEV_PROCESS §0).
