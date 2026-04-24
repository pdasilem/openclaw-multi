# Phase 0 Retrospective

## What we built

Go monorepo skeleton with three binary stubs (`openclaw-multi`, `openclaw-overlay-api`,
`openclaw-overlay-watcher`). Bubble Tea TUI with a working main menu (10 items),
lipgloss styles, status bar, placeholder screens, and a first-run admin setup flow.
SQLite state store with idempotent migrations. JSONL audit log writer. Admin
identification and verification logic. CI pipeline (lint + test + build amd64/arm64 +
schema-check + shellcheck). Docker test image and Phase 0 smoke test script.

## Deviations from plan

- **golangci-lint**: plan specified v1.64.8; actual is v2.11.4 (latest). v2 has a
  breaking config format change (`gosimple` removed, formatters split out). Config
  updated accordingly.
- **Go version**: plan specified `go 1.22`; actual is `go 1.25.0` because
  `bubbletea v1.3.10` and `modernc.org/libc v1.72.0` require it. CI updated to
  `go-version: "1.25.x"`.
- **`state.ErrNoAdmin`**: `GetAdmin` returns a sentinel error instead of `(nil, nil)`
  to satisfy `nilnil` linter rule. Callers updated.
- **`openclaw/` upstream mirror**: excluded from git tracking (added to `.gitignore`);
  it lives on disk but is not committed to the overlay repo.
- **No public GitHub Release**: v0.0.0 tag created for version tracking only; a full
  release is deferred until MVP is ready.

## Lessons for next phase

- Pin golangci-lint version range in CI (`>= v2`) to avoid config breakage.
- All future commits: one commit per logical phase chunk, max 2-3 per phase.
- Use `git -c user.name/user.email` flags for authorship; never touch global git config.
- Use `gh` CLI (not raw git config) for push authentication.
- Validate CI YAML locally with `python3 -c "import yaml; yaml.safe_load(...)"` before
  pushing.

## Open issues carried forward

- None blocking Phase 1.

## Master plan updates suggested

- Note that `go 1.25+` is required due to current bubbletea/modernc.org deps.
- golangci-lint v2 config format applies to all phases.
