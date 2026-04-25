# Phase 5 Retro: Network Management

Date: 2026-04-25

## Delivered

- Added `internal/network` with structured report models, read-only snapshot,
  Tailscale/cloudflared/UFW/port inspectors, explicit gateway probes, wildcard
  DNS plan/apply, UFW plan/apply, and conservative last_seen parsing.
- Wired TUI menu item 5 to the real **Network and firewall** screen.
- Added config fields `cloudflare_zone_id` and `cloudflare_api_token`.
- Added audit actions `network_refresh`, `network_probe`, `dns_update`, and
  `ufw_update`.
- Added `state.ListRoutes` and `state.SetRouteLastSeen`.
- Removed remaining non-English strings from `internal` TUI code.
- Added `docs/network.md` and updated this Phase 5 plan and changelog.

## Verification

- `go test ./...` passed with `GOCACHE=/tmp/codex-go-cache` and
  `GOTMPDIR=/tmp/codex-go-tmp`.
- `go test -cover ./internal/network` passed with 80.8% coverage.
- `make ci` passed with `GOCACHE=/tmp/codex-go-cache`,
  `GOTMPDIR=/tmp/codex-go-tmp`, and
  `GOLANGCI_LINT_CACHE=/tmp/codex-golangci-cache`.
- `rg -n "[А-Яа-яЁё]" internal` returned no matches.

## Deviations

- Optional DNS-resolution probing was deferred. HTTPS gateway probing is enough
  for Phase 5 and avoids adding another tool dependency.
- Existing cloudflared config validation was deferred. Phase 6 owns config
  generation/reload, so Phase 5 only inspects service/version/config-derived
  state and routes.
- `network.Manager` does not take `shell.FS` because the implemented Phase 5
  operations do not need filesystem access.

## Notes For Phase 6

- Route state is now visible in network diagnostics, but cloudflared ingress
  publication is still not implemented.
- The DNS target uses the named tunnel CNAME shape
  `${tunnel_id}.cfargotunnel.com`; live VPS verification should confirm the
  exact tunnel identity and permissions.
- All code-facing menu labels and prompts must remain English-only.
