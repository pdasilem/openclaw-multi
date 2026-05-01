# Phase 7.1 Retro: Privilege Boundary and Embedded Terminal

Date: 2026-05-01

## Delivered

Phase 7.1 corrected the privilege model before Phase 8. The normal interactive
TUI path is now a non-root admin-user process, and privileged work is explicit:
one-time system preparation is done through `sudo openclaw-multi
system-prepare`, while runtime privileged operations use explicit `sudo`
commands from the admin session.

Implemented:

- Normal `openclaw-multi` launch rejects UID 0 with a clear error.
- First-run admin identity is the current non-root user, not `$SUDO_USER`.
- `system-prepare` creates required system directories and repairs ownership
  of existing state/log files left from the pre-7.1 root-run flow:
  `/var/lib/openclaw-multi`, `/var/log/openclaw-multi`,
  `/etc/openclaw-multi`, and `/etc/cloudflared`.
- `system-prepare` prints each ensured directory operation and final completion
  status.
- `internal/shell.RealExecutor` emits command lifecycle events for the TUI
  terminal while preserving audit logging.
- Known config secrets are redacted in terminal command display.
- `internal/shell.PrivilegedFS` routes system path writes through explicit
  `sudo install`, `sudo mv`, and `sudo rm`.
- `internal/shell` now has a PTY runner based on `github.com/creack/pty`.
- The TUI renders a bottom terminal panel on every screen, including first-run.
  It is collapsed by default, has focus mode, keeps configurable scrollback,
  and can run manual commands without leaving the TUI.
- Default config now uses `node_version_min: "24"` and
  `terminal_history_lines: 1000`.
- Cloudflared ingress validation runs through `sudo` because the generated
  `/etc/cloudflared/config.yml` path is a root-owned system file.
- Install/runbook/E2E docs were updated to the new flow:
  `sudo make install`, `sudo openclaw-multi system-prepare`, then
  `openclaw-multi` as `ubuntu`.

Phase decisions fixed in code and docs:

- No sudoers allowlist.
- Existing VPS sudo policy plus PTY password entry.
- Terminal is always available and collapsed by default.
- Terminal implementation uses direct PTY primitives, not an external terminal
  emulator.
- Terminal history is separate from logs and diagnostic snapshots.

## Verification

Passed:

```text
XDG_CACHE_HOME=/tmp/openclaw-multi-cache GOCACHE=/tmp/openclaw-multi-go-cache GOTMPDIR=/tmp make ci
```

Result:

- `golangci-lint run ./...`: `0 issues`.
- `go test -race -coverprofile=coverage.out ./...`: passed.
- Build for `openclaw-multi`, `openclaw-overlay-api`, and
  `openclaw-overlay-watcher`: passed.
- `schemas/state.sql` schema check: passed.
- `shellcheck`: no failures reported by `make ci`.

Additional static documentation check:

- No remaining matches for old contradictory instructions such as
  `sudo openclaw-multi` as normal launch, `root shell`, `node_version_min: 22`,
  terminal hidden from welcome/login, sudoers allowlist, or fallback wording in
  active docs/code.

## Deviations

- RealExecutor command output is captured and emitted after command completion.
  PTY-launched manual commands stream live. If owner VPS testing shows that
  fresh-install long commands need live output before completion, those command
  paths should be moved to PTY execution explicitly.
- `system-prepare` is covered by CI compilation and command construction review,
  but it has not been executed on the real Ubuntu VPS in this agent session.
- Interactive sudo password entry through PTY is implemented, but the actual
  password prompt behavior must still be confirmed on the VPS with `sudo -k`.
- No new smoke script or `make test-phase-7.1` target was added. Validation
  remains owner-run through `docs/e2e-use-cases.md`.
- The working tree still contains pre-existing untracked files not owned by this
  retro, including `.codex` and `internal/tui/firstrun_test.go`; they were not
  removed.

## Process Notes

- During retro review, `system-prepare` was found to print only a generic
  success line while the phase task required exact operation output. That gap
  was fixed before finalizing this retro, and `make ci` was rerun.
- Phase 7.1 changed architecture, not just behavior. Updating only code would
  have left install/runbook/E2E docs contradictory, so the docs were treated as
  part of the implementation.
- The phase is implemented and CI-clean, but not owner-VPS validated. It should
  not be treated as production-proven until the E2E cases below are run on the
  target VPS.

## Follow-Up

- Run the Phase 7.1 E2E checks on the Ubuntu 24 VPS:
  normal `openclaw-multi`, rejected `sudo openclaw-multi`, terminal visibility
  on all screens, fresh install command mirroring, and `sudo -k` password prompt
  through terminal focus.
- Confirm `system-prepare` creates the expected ownership and modes on the VPS:
  admin-owned `/var/lib/openclaw-multi` and `/var/log/openclaw-multi`, root-owned
  `/etc/openclaw-multi` and `/etc/cloudflared`.
- Confirm cloudflared CLI state stays under `/home/ubuntu/.cloudflared`, while
  service credentials are installed under `/etc/cloudflared/<tunnel_id>.json`.
- If long-running fresh-install commands need live output before completion,
  convert those specific operations to PTY-backed execution instead of changing
  the whole executor.
- After VPS validation, update `docs/e2e-use-cases.md` with real status and
  captured failures or mark the checks passed.

## Open Questions

No architecture questions remain open for Phase 7.1. Remaining items are VPS
validation results, not design decisions.
