# Architecture

## Shape
- The project is a Go control plane for multi-user OpenClaw deployments on one VPS.
- System-space components own host setup, state, Cloudflare tunnel configuration, audit logs, and privileged operations.
- Tenant-space components run as Linux users and publish OpenClaw gateway/plugin route intent back to the system daemon.

## Main Runtime Flow
- `openclaw-multi` starts the Bubble Tea TUI and wires admin, state, audit, backup, network, users, doctor, and shell services.
- First-run and wizard flows validate host readiness, install/check dependencies, apply hardening, configure Cloudflare/Tailscale/UFW, and add initial managed users.
- User lifecycle operations in `internal/users.Manager` update Linux users, OpenClaw files, systemd user services, gateway routes, state, and audit logs.

## Overlay Flow
- `openclaw-overlay-api` loads overlay config and SQLite state, listens on a UNIX socket, and publishes desired routes to Cloudflare tunnel config.
- `internal/api.RouteService` owns route CRUD semantics over state and route publishing.
- `internal/cloudflared.Manager` renders, writes, backs up, and reloads Cloudflare tunnel configuration.
- `openclaw-overlay-watcher` runs per managed user, reads OpenClaw plugin config, diffs callback routes against a local snapshot, and calls the overlay API.

## Shared Boundaries
- `internal/shell` abstracts command execution, filesystem access, PTY usage, auditing, and test doubles.
- `internal/state` abstracts SQLite storage and migrations.
- `internal/audit` writes structured audit events.
- `internal/config` handles overlay config defaults, YAML load/save, and template rendering.

## Dependency Direction
- Commands under `cmd/` compose internal packages.
- TUI and wizard packages orchestrate domain packages but should not own low-level host behavior.
- Domain packages depend on `shell`, `state`, `config`, and `audit` abstractions for side effects.
- Tests commonly use mock executors, memory filesystems, and temporary SQLite stores.

