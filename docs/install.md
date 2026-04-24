# Installation Guide

> **Important:** openclaw-multi does not yet run on real VPS infrastructure.
> This guide describes the intended workflow. Live execution will be enabled
> after the full MVP is complete and the owner authorises a deployment run.

## Prerequisites

- Ubuntu 22.04+ or Debian 12+ VPS (other distros: unsupported, proceed at own risk)
- ≥ 5 GB free disk, ≥ 1 GB RAM
- A non-root user with `sudo` access (e.g. `ubuntu`, `opadmin`)
- Internet access from the VPS

## Running the wizard

```bash
sudo openclaw-multi
```

The TUI opens. Select **1. Установка с нуля (fresh install)**.

The wizard runs 10 steps in order:

| Step | What it does |
|------|-------------|
| 1. Pre-flight check | Verifies distro, disk, RAM, required tools, port conflicts |
| 2. Install Node.js | Ensures Node.js ≥ 22.16.0 via NodeSource |
| 3. Install Tailscale | Installs + authenticates Tailscale (admin-only access) |
| 4. Configure Cloudflare Tunnel | Variant A (account) or Variant B (quick tunnel) |
| 5. Configure UFW firewall | Sets default deny incoming, adds your ports |
| 6. Apply host hardening | sysctl, hidepid=2, umask 0077 |
| 7. Install OpenClaw | `npm install -g openclaw@latest` (no onboarding yet) |
| 8. Install overlay-API service | Systemd unit for the overlay daemon |
| 9. Ready | Information panel — use menu item 3 to add users |
| 10. Finalise | Marks installation complete in state.db |

Any step can be retried or skipped if it fails (press `R` / `S` / `A`).

## Cloudflare Tunnel: Variant A vs Variant B

**Variant A — With Cloudflare account (recommended)**

- Free Cloudflare tier is sufficient.
- Provides stable HTTPS URLs for Control UI and plugin callbacks.
- Requires: Cloudflare account, a registered domain, API token.
- The wizard calls `cloudflared tunnel create`, sets up wildcard DNS
  `*.openclaw.<your-domain>`, and generates `/etc/cloudflared/config.yml`.

**Variant B — Quick tunnel (no account)**

- No Cloudflare account required.
- URLs are ephemeral (change on every cloudflared restart).
- Suitable for testing only, not recommended for production.

## Troubleshooting

**Pre-flight fails: missing tools**

```
✗ Pre-flight check
  Error: preflight failed: tools — missing: useradd
```

Install the missing tool: `sudo apt-get install -y passwd`.

**UFW conflict: existing rules**

The wizard never resets existing UFW rules. It only adds rules that are missing.
If you need to start clean: `sudo ufw reset` before running the wizard.

**Node.js version too old**

The wizard will attempt to install via NodeSource for Ubuntu/Debian.
On other distros, install Node.js ≥ 22.16.0 manually before running the wizard.

**Port conflicts (18789–19999)**

Another service is already listening on a port in our range.
Move the conflicting service to a different port before continuing.

**Step fails and you want to skip**

Press `S` to skip the failed step. You can re-run the wizard later —
all steps are idempotent (safe to run multiple times).

## After installation

Use **menu item 3 — Управление пользователями** to add the first OpenClaw user.
Each user gets their own Linux account, systemd --user services, and a personal
Cloudflare Tunnel URL for their Control UI.
