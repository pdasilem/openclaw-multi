# User Management

> Phase 2 is still test-only. The code models the intended system operations,
> but no Docker VPS simulation is used and final system verification is deferred
> to a real VPS owner-run check.

Menu item **3. Управление пользователями** manages OpenClaw users recorded in
`state.db`.

## Preconditions

- `domain` and `subdomain` must be configured before adding users.
- The generated gateway URL has this form:
  `https://gateway-<username>.<subdomain>.<domain>`.
- If either value is missing, Add User fails before any state, route, filesystem,
  or command mutation.

## Add User

The TUI asks only for a username. `Add user` is idempotent: it ensures the
requested Linux tenant exists and is ready as an active managed OpenClaw user.
Run it again for the same username when a previous add was interrupted or when
health shows partial tenant runtime. There is no separate repair-user action.

The gateway token is generated automatically; manual token entry is
intentionally out of scope.

Backend flow:

1. Validate username.
2. Validate `domain` and `subdomain`.
3. Reuse existing state port when present; otherwise allocate the lowest free
   gateway port from `port_range_start + n*port_range_step`.
4. Create the Linux user if missing; reuse it if already present.
5. Enable linger and start `user@<uid>.service`.
6. Install/repair tenant NVM, Node 24, OpenClaw CLI, wrappers, onboarding, and
   user units.
7. Start gateway and watcher user services.
8. Verify readiness:
   `~/.openclaw`, `~/.openclaw/openclaw.json`, `~/.openclaw-overlay`, gateway
   unit, watcher unit, gateway active, watcher active.
9. Record the user as `active` only after readiness passes.
10. Record/enable the gateway route in state.
11. Emit `bootstrap_user` audit event.

If readiness fails, Add User returns an error and does not create a new active
state row.

## Deactivate / Activate

Deactivate stops user services, disables linger, disables state routes, sets
`status=paused`, and emits `disable_user`.

Activate uses the same reconcile/readiness path as Add User, then emits
`enable_user`.

## Remove

Remove is destructive. After Phase 3, remove is backup-first: a pre-remove
backup runs before any route deletion, OpenClaw uninstall, linger disable,
`userdel`, state deletion, or port reuse. If backup creation fails, remove
aborts.

For active users, the TUI follows a deactivate-first flow before hard delete.
Hard delete requires typing the exact username.

After removal:

- routes are deleted from state;
- the user row is deleted;
- backup metadata remains queryable by the original username;
- the previous gateway port is immediately available for reuse;
- `delete_user` audit event is emitted.

## Backup / Restore

The user-management screen exposes `b` to create an encrypted backup for the
selected user and `r` to restore one of that user's backups.

Restore is limited to existing managed users in Phase 3. Restore-to-missing-user
is deferred.

## Test Boundary

Phase 2 tests use `MockExecutor`, `MemFS`, temp SQLite databases, and TUI model
tests. They do not create Linux users, mutate cloudflared config, or run live
`systemctl`/`loginctl` on the host.
