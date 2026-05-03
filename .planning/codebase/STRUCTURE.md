# Structure

## Top Level
- `cmd/`: binary entrypoints.
- `internal/`: application packages and host/domain behavior.
- `schemas/`: SQLite schema.
- `templates/`: systemd, profile, sysctl, fstab, and Cloudflare templates.
- `docs/`: architecture, operations, network, users, backups, health, and phase history.
- `test/`: Docker and shell-based E2E support.
- `Makefile`: build, test, lint, install, schema, and Docker targets.

## Internal Packages
- `admin`: resolves and persists the admin user.
- `api`: UNIX-socket HTTP API, auth, client, contracts, and route service.
- `audit`: append-only audit logging.
- `backup`: backup, restore, timers, and removal hooks.
- `cloudflared`: Cloudflare tunnel config rendering/publishing/reload.
- `config`: overlay config defaults, YAML, and template rendering.
- `deps`: dependency checks/install/setup for Cloudflared, Node, Tailscale, and UFW.
- `doctor`: host and tenant health checks plus fix plans.
- `hardening`: sysctl, hidepid, and profile hardening.
- `network`: network status parsing, DNS planning, Cloudflare DNS client, and port checks.
- `preflight`: distro, disk, RAM, tool, and port checks.
- `shell`: executor, filesystem, PTY, events, mocks, and redaction.
- `state`: SQLite models, migrations, and store methods.
- `systemprep`: sudo preparation for root-owned config directories.
- `tui`: Bubble Tea app, menu, users, network, doctor, terminal, and first-run UI.
- `tui/wizard`: fresh-install wizard steps.
- `users`: managed-user lifecycle, route publishing, validation, and port allocation.
- `watcher`: OpenClaw callback discovery, snapshot diffing, route sync, and fsnotify loop.

## Docs And Templates
- `docs/architecture.md` and `docs/structurizr.dsl` describe system containers.
- `docs/vps-runbook.md` is the owner-run operational reference.
- `docs/phases/` records implementation history and retro notes.
- Templates are installed with binaries and are used by user, backup, API, watcher, gateway, Cloudflare, sysctl, profile, and fstab flows.

