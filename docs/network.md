# Network Management

Phase 5 adds the TUI **Network and firewall** screen and the
`internal/network` package. The screen is read-only by default. Mutating DNS and
UFW operations require an explicit review step before apply.

## Boundaries

- Tailscale is admin-only. Managed OpenClaw users do not get Tailscale
  identities and the network screen never runs `tailscale up`.
- Cloudflare Tunnel remains the public inbound path. The VPS only needs
  outbound connectivity for `cloudflared`.
- Phase 5 does not regenerate `/etc/cloudflared/config.yml`, reload
  `cloudflared`, restart services, or publish per-user ingress routes. Phase 6
  implements that path in `openclaw-overlay-api`.
- Tests never call live Tailscale, cloudflared, UFW, DNS, curl, ping, or
  Cloudflare APIs. All system commands use `shell.Executor`; DNS uses an
  injectable client.

## TUI Actions

- `r` refreshes a read-only snapshot for Tailscale, cloudflared, UFW, routes,
  and listening ports.
- `p` probes enabled gateway URLs with `curl -fsS --max-time 5` through the
  executor. Probe failures are shown per route and do not fail the whole batch.
- `d` builds a Cloudflare wildcard DNS plan and opens a review/apply screen.
- `u` builds a UFW plan for enabled route ports and opens a review/apply screen.
- `q` or `Esc` returns to the main menu.

## Cloudflare DNS

Wildcard DNS management requires:

- `domain`
- `subdomain`
- `tunnel_id`
- `cloudflare_zone_id`
- `cloudflare_api_token`
- account/named tunnel mode, not quick tunnel mode

The managed record is:

```text
*.${subdomain}.${domain} CNAME ${tunnel_id}.cfargotunnel.com
```

The planner reports `create`, `update`, or `noop`. Apply only performs the
reviewed action. Phase 5 does not delete DNS records.

## UFW

The expected policy remains deny incoming / allow outgoing. Phase 5 parses
`ufw status verbose`, shows active state, default policies, allowed ports, and
missing enabled route ports. Apply calls the existing conservative
`deps.EnsureUFW` helper; it does not reset UFW and preserves existing rules.

## Last Seen

`internal/network` includes a conservative cloudflared log parser for supplied
text fixtures. It can update `routes.last_seen_cached_at` and
`routes.last_seen_value` for known hostnames through an explicit method. There
is no live-tail implementation in Phase 5.
