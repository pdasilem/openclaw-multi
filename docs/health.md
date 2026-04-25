# Health Check / Doctor

> Phase 4 is test-only. The code models intended VPS checks through
> `shell.Executor` and `shell.FS`; live validation is deferred to a real VPS
> check.

Menu item **4. Health check / Doctor** runs structured health checks and renders
results grouped by category.

## Categories

- `system`: disk, memory, load average, `ptrace_scope`, `/proc hidepid`, and
  required command presence.
- `services`: shallow `systemctl is-active` checks for `tailscaled`,
  `cloudflared`, and `openclaw-overlay-api`.
- `users`: per-user linger, gateway service, watcher service, and paused-user
  handling.
- `filesystem`: sensitive OpenClaw path permissions.
- `network`: read-only gateway port listener checks.
- `openclaw`: `openclaw doctor` output per active managed user.

Statuses are normalized to `ok`, `warn`, `fail`, and `skipped`.

## Read-only Boundary

The default health run is read-only. It does not start/stop services, install
packages, create Linux users, change routes, rotate credentials, restore
backups, or mutate network configuration.

Later-phase services can report `skipped` instead of `fail` when they are not
implemented yet.

Paused users are skipped for runtime checks and `openclaw doctor`.

## OpenClaw Doctor

The OpenClaw doctor action runs:

```bash
su - <user> -c "openclaw doctor --json"
```

If JSON output is available, it is parsed into structured results. If JSON is
not available, the fallback parser maps obvious text `warn`, `error`, and
`fail` markers into health statuses.

## Auto-fix

Auto-fix is opt-in and limited to the allowlist below:

- `chmod-openclaw-dir`: `chmod 0700 /home/<user>/.openclaw`
- `chmod-openclaw-config`: `chmod 0600 /home/<user>/.openclaw/openclaw.json`
- `chmod-overlay-dir`: `chmod 0700 /home/<user>/.openclaw-overlay`
- `repair-umask-profile`: rewrite the overlay-managed umask profile to
  `umask 0077`

No other mutation is allowed through the Phase 4 fix API.

## Later Phases

Phase 4 does not implement deep network management, live log tailing, the full
security-audit workflow, or diagnostic snapshot archives. Those remain assigned
to later phases in the master plan.
