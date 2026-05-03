# Concerns

## Security And Privilege
- The project performs privileged host operations: user creation, systemd/systemctl, loginctl, sudo shell commands, root-owned file writes, cloudflared reloads, and service installation.
- Secrets include Cloudflare API tokens, Cloudflared credentials, Telegram token, generated tenant tokens, and OpenClaw runtime config. Redaction and file permissions are critical.
- Overlay API authorization depends on UNIX peer credentials; route mutations should stay constrained to the requesting tenant/admin context.
- Shell command construction should continue to flow through `internal/shell` helpers and package-level quoting/redaction.

## Operational Risk
- The system spans persistent SQLite state, host users, systemd services, Cloudflare tunnel config, DNS records, and tenant files. Partial failures can create drift.
- `internal/users.Manager` is high-risk because it coordinates state, shell, files, service enablement, gateway config, tokens, and route publication.
- `internal/cloudflared.Manager` is high-risk because it rewrites Cloudflare tunnel config and reloads the service.
- Backup/restore and remove flows need careful ordering so tenant data is preserved before destructive lifecycle steps.

## Testing Gaps To Watch
- Unit tests are broad, but real VPS behavior still depends on systemd, Cloudflared, Tailscale, UFW, Linux user semantics, and OpenClaw runtime behavior.
- Parser tests should be expanded when new distro, command output, or provider-output variants are encountered.
- E2E evidence should be kept for add/remove/backup/restore, route publication, watcher updates, and doctor readiness gates.

## Maintainability
- Keep orchestration in TUI/wizard thin; move durable behavior into domain packages with tests.
- Avoid introducing ad hoc shell execution outside `internal/shell`.
- Keep docs, templates, and install checks synchronized when service files or runtime paths change.
- Preserve existing dependency direction: commands compose, TUI orchestrates, domain packages own behavior, shell/state/audit/config own side effects.

