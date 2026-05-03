# Integrations

## Host Services
- `systemd` and `systemctl` are used for system services, user services, timers, and daemon reloads.
- `loginctl` is used by user lifecycle flows to enable user linger for managed tenants.
- Linux users are the tenant isolation boundary.
- Root-owned files under `/etc/openclaw-multi`, `/etc/cloudflared`, `/opt/openclaw-multi`, and service/template paths are managed through privileged shell and filesystem helpers.

## Network Providers
- Cloudflare Tunnel is configured through `internal/cloudflared` and `templates/cloudflared-config.tmpl`.
- Cloudflare DNS support lives in `internal/network/cloudflare_dns.go`.
- Tailscale status and admin-access checks live under `internal/network` and `internal/deps`.
- UFW status and setup are handled through `internal/deps/ufw.go` and network health parsing.

## OpenClaw Runtime
- Managed users run OpenClaw in user space.
- `internal/users` provisions gateway services, watcher services, token/config files, and lifecycle state.
- `internal/watcher` discovers plugin callback routes from user OpenClaw config and syncs them to the overlay API.
- Backup flows call tenant shell commands and user systemd commands through `internal/backup`.

## Persistence And IPC
- SQLite state schema is in `schemas/state.sql`.
- `internal/state.Store` is the application persistence boundary.
- Overlay API listens on a UNIX socket and authorizes requests through peer credentials.
- TUI and watcher code use `internal/api.Client` to call the local overlay API.

## Templates And Runtime Files
- Systemd templates live in `templates/`.
- Overlay config parsing and default values live in `internal/config`.
- Install target copies templates into `/opt/openclaw-multi/templates`.

