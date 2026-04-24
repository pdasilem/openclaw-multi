# openclaw-multi

> **Status: pre-alpha** — not ready for production use. Install: not yet available.

`openclaw-multi` is an administrative TUI utility that automates multi-user
[OpenClaw](https://github.com/openclaw/openclaw) deployments on a single VPS.
It handles user isolation (Linux users + systemd --user), Cloudflare Tunnel
management for per-user Control UI and plugin callbacks, Tailscale for admin
access, and the full user lifecycle (add / remove / backup / restore).

See the full architecture and plan in
[OPENCLAW_OVERLAY_PLAN_RU.md](./OPENCLAW_OVERLAY_PLAN_RU.md).

## CI Status

![CI](https://github.com/pdasilem/openclaw-multi/actions/workflows/ci.yml/badge.svg)

## Development

```bash
make build   # build all three binaries to bin/
make test    # run tests with coverage
make lint    # golangci-lint
make ci      # lint + test + build + schema-check
make dev     # run the TUI locally
```

See [docs/contributing.md](./docs/contributing.md) for branch/PR conventions.
