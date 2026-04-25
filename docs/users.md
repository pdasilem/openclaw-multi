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

The TUI asks only for a username. Phase 2 generates the gateway token
automatically; manual token entry is intentionally out of scope.

Backend flow:

1. Validate username.
2. Validate `domain` and `subdomain`.
3. Allocate the lowest free gateway port from `port_range_start + n*port_range_step`.
4. Generate gateway token.
5. Run intended system commands through `shell.Executor`.
6. Write the per-user watcher unit through `shell.FS`.
7. Record the user and gateway route in state.
8. Emit `bootstrap_user` audit event.

## Deactivate / Activate

Deactivate stops user services, disables linger, disables state routes, sets
`status=paused`, and emits `disable_user`.

Activate enables the gateway route, enables linger, starts user services, sets
`status=active`, and emits `enable_user`.

## Remove

Remove is destructive. Backup before remove belongs to Phase 3 and is not
implemented yet.

For active users, the TUI follows a deactivate-first flow before hard delete.
Hard delete requires typing the exact username.

After removal:

- routes are deleted from state;
- the user row is deleted;
- the previous gateway port is immediately available for reuse;
- `delete_user` audit event is emitted.

## Test Boundary

Phase 2 tests use `MockExecutor`, `MemFS`, temp SQLite databases, and TUI model
tests. They do not create Linux users, mutate cloudflared config, or run live
`systemctl`/`loginctl` on the host.
