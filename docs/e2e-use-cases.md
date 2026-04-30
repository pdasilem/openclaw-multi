# E2E and Smoke Use Cases

This is the owner-run VPS validation checklist. Automated tests prove package
logic and smoke behavior where possible; this document captures workflows that
must also be exercised on a real VPS before the product is considered reliable.

Status values:

- `planned`: not yet owner-validated on a VPS.
- `passed`: owner validated on a VPS and no product change is needed.
- `failed`: owner validated and found a product/doc gap.
- `blocked`: cannot run until a later phase lands.

When a check fails, capture the command output, relevant journal logs, current
config snippets with secrets redacted, and whether the VPS was newly installed
or already had OpenClaw/cloudflared/Tailscale state.

## Phase 0: Repository and Binary Smoke

### UC-0001: Build and Run Binaries

- Phase: 0
- Scenario type: `automated-smoke`, `owner-vps`
- Status: `planned`

**Preconditions.**

- Repo checked out on the target VPS.
- Go toolchain installed at the repository baseline version.

**Steps.**

1. Run `make build`.
2. Run `bin/openclaw-multi --help` if the flag exists, or start the TUI and
   exit without changing state.
3. Run `bin/openclaw-overlay-api` and confirm that before Phase 6 it is only a
   known stub, and after Phase 6 it starts as a daemon.
4. Run `bin/openclaw-overlay-watcher` and confirm that before Phase 7 it is only
   a known stub.

**Expected result.**

- All binaries build.
- Stub behavior matches the current phase boundary.
- No unexpected files are created outside configured overlay paths.

**Capture on failure.**

- `go version`
- `make build` output
- binary stderr/stdout

### UC-0002: Admin Terminal Tab Is Present

- Phase: 0
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- `openclaw-multi` TUI is running.
- Admin login/first-run setup is complete.

**Steps.**

1. Open each admin screen after login: main menu, fresh install, user
   management, backup/restore, health, network/firewall, overlay publication,
   watcher, logs, and settings.
2. Confirm the bottom area contains a terminal tab on each screen.
3. Open the terminal tab and run a harmless command such as `whoami`.
4. Return from the terminal tab to the previous admin screen.
5. Open the welcome/login screen and confirm it does not expose the terminal
   tab.

**Expected result.**

- Every admin screen after login has a bottom terminal tab.
- Welcome/login has no terminal tab.
- The terminal tab runs commands without leaving the admin panel.
- Returning from the terminal tab preserves the current admin screen context.

**Capture on failure.**

- TUI screenshot/transcript.
- Command entered in the terminal tab.
- Terminal tab output.

## Phase 1: Fresh Install

### UC-0101: Fresh Install on Clean VPS

- Phase: 1
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- Clean Ubuntu VPS or disposable test VPS.
- SSH access as admin user `ubuntu` through Tailscale.
- Root shell is available through passwordless `sudo -i` for manual system
  checks.
- `openclaw-multi` is started as `ubuntu` through `sudo openclaw-multi`, not
  from a `sudo -i` root shell.
- `tailscaled` is active and logged in.
- UFW keeps public inbound closed and allows admin SSH through Tailscale.
- No production OpenClaw data on the host.

**Steps.**

1. Start `openclaw-multi` with `sudo openclaw-multi` from the `ubuntu` SSH
   session.
2. Complete the first-run admin setup.
3. Run menu item `1. Fresh install`.
4. Choose the account/named Cloudflare Tunnel path when credentials are
   available.
5. Confirm Node.js target handling: project Node.js is `24` and the VPS
   path uses `nvm`, not a root-global Node runtime.
6. Confirm host hardening paths exist with expected permissions:
   `/etc/profile.d/openclaw.sh` and `/var/cache/openclaw-compile`.
7. After the wizard finishes, inspect generated files:
   `/etc/openclaw-multi/config.yml`, `/etc/systemd/system/cloudflared.service`,
   `/etc/systemd/system/openclaw-overlay-api.service`, and overlay templates.
8. Run `systemctl is-active tailscaled cloudflared openclaw-overlay-api`.

**Expected result.**

- Required packages and services are installed or reused.
- Existing host state is not overwritten without an explicit confirmation.
- Overlay config contains domain, subdomain, tunnel mode, and port range.
- Overlay config defaults match the plan unless intentionally changed:
  `port_range_start: 18789`, `port_range_step: 20`,
  `node_version_min: 24`,
  `openclaw_update_source: pdasilem/openclaw:latest`.
- `cloudflared` and `openclaw-overlay-api` service states match the phase:
   Phase 1 may install a stub service; Phase 6 must run the real daemon.

**Capture on failure.**

- Wizard transcript/screenshots.
- `journalctl -u cloudflared -u openclaw-overlay-api --no-pager -n 200`
- Redacted `/etc/openclaw-multi/config.yml`
- `node --version`, `command -v node`, and nvm path evidence.

### UC-0102: Idempotent Fresh Install Rerun

- Phase: 1
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- UC-0101 has run once.

**Steps.**

1. Run menu item `1. Fresh install` again.
2. Choose to reuse existing installed components.
3. Decline any destructive overwrite prompts unless intentionally testing
   overwrite behavior.
4. Confirm existing Tailscale identity remains unchanged with `tailscale status`.
5. Confirm existing `/etc/cloudflared/config.yml` is backed up before overwrite.

**Expected result.**

- Existing Tailscale identity is not reset.
- Existing cloudflared tunnel/config is backed up before overwrite.
- Re-running the wizard does not duplicate systemd units or corrupt config.

**Capture on failure.**

- Wizard transcript/screenshots.
- `systemctl cat cloudflared openclaw-overlay-api`
- `/var/lib/openclaw-multi/snapshots` listing

## Phase 2: User Lifecycle

### UC-0201: Add Managed User and Gateway State

- Phase: 2
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- Fresh install completed.
- `/etc/openclaw-multi/config.yml` has `domain` and `subdomain`.
- Phase 6 is required before public cloudflared route publication is expected.

**Steps.**

1. Open menu item `3. User management`.
2. Add a user named `alice`.
3. Check Linux user state with `id alice`.
4. Check linger with `loginctl show-user alice -p Linger`.
5. Switch only through root shell into tenant context: `su - alice`.
6. Confirm `node --version` reports major version `24` and comes from Alice's
   `nvm` path.
7. Confirm OpenClaw CLI wrapper exists inside tenant:
   `test -x /home/alice/.local/bin/openclaw`.
8. Confirm non-interactive onboarding ran inside tenant with daemon install:
   `/home/alice/.local/bin/openclaw doctor` and
   `systemctl --user status openclaw-gateway`.
9. Confirm gateway config received overlay env decisions:
   `OPENCLAW_GATEWAY_PORT`, `OPENCLAW_GATEWAY_TOKEN`, and
   `OPENCLAW_GATEWAY_BIND=loopback` are reflected in generated OpenClaw config
   or service environment without exposing token value in shared evidence.
10. Inspect route state in `state.db` or through the TUI route list.
11. Confirm TUI shows the public gateway URL and generated token to the admin.

**Expected result.**

- User `alice` exists.
- Linger is enabled.
- OpenClaw gateway service is installed/running for the user.
- OpenClaw onboarding is non-interactive and is not run as root or admin
  `ubuntu`; it runs as `alice`.
- Tenant uses Node.js major version `24` through `nvm`.
- OpenClaw CLI source is `pdasilem/openclaw:latest` unless
  `openclaw_update_command` was changed in overlay settings.
- Gateway URL has the form
  `https://gateway-alice.<subdomain>.<domain>`.
- Before Phase 6, the route is state-only. After Phase 6, cloudflared config
  includes the route.

**Capture on failure.**

- TUI transcript/screenshots.
- `journalctl --user -u openclaw-gateway` for `alice`
- `su - alice -c 'node --version; command -v node; test -x ~/.local/bin/openclaw; ~/.local/bin/openclaw doctor'`
- Relevant rows from state DB with tokens redacted.

### UC-0202: Deactivate and Reactivate Managed User

- Phase: 2
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- UC-0201 created user `alice`.

**Steps.**

1. Deactivate `alice` from menu item `3`.
2. Check that linger is disabled.
3. Check that `openclaw-gateway` and watcher services are stopped or absent for
   the paused user.
4. After Phase 6, request the gateway URL and verify it no longer routes to the
   user's gateway.
5. Reactivate `alice`.
6. Check that linger and services are restored.
7. After Phase 6, request the gateway URL again.

**Expected result.**

- Deactivate is idempotent and does not delete user data.
- Reactivate restores the user's intended runtime state.
- After Phase 6, cloudflared route publication follows the enabled/disabled
  route state.

**Capture on failure.**

- `loginctl show-user alice -p Linger`
- `su - alice -c 'systemctl --user status openclaw-gateway openclaw-overlay-watcher'`
- `curl -vk https://gateway-alice.<subdomain>.<domain>/`

### UC-0203: Remove User Is Backup-First and Frees Port

- Phase: 2, 3
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- User `alice` exists.
- Phase 3 backup support is present.

**Steps.**

1. Remove `alice` from menu item `3`.
2. Confirm the destructive prompt by typing the exact username.
3. Verify a backup was created before user deletion.
4. Verify `id alice` fails.
5. Add a new user and verify the freed gateway port can be reused when it is the
   lowest valid free port.

**Expected result.**

- Remove aborts if pre-remove backup fails.
- Backup metadata survives user deletion.
- Linux user, route state, and active user services are removed.
- Port reuse follows the allocator rule.

**Capture on failure.**

- Backup command output.
- Backup metadata row.
- User manager audit events: `backup_create`, `delete_user`, `delete_route`.

## Phase 3: Backup and Restore

### UC-0301: Create and Verify User Backup

- Phase: 3
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- Managed user exists and has OpenClaw config/data.

**Steps.**

1. Select the user in menu item `3`.
2. Press `b` to create a backup.
3. Inspect `/var/lib/openclaw-multi/backups/<username>/`.
4. Confirm `/etc/openclaw-multi/master.key` exists and has mode `0600`.
5. Confirm backup metadata appears in the TUI restore list or state DB.

**Expected result.**

- `openclaw backup create --verify` succeeds under the target user.
- Encrypted backup archive is created.
- SHA-256, size, path, and timestamp are recorded.

**Capture on failure.**

- `openclaw backup create` stdout/stderr.
- File listing with permissions.
- Audit event `backup_create`.

### UC-0302: Restore Existing Managed User

- Phase: 3
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- UC-0301 created at least one backup.
- Target user still exists.

**Steps.**

1. Change a harmless user config value or add a small test artifact in
   `~/.openclaw`.
2. Restore the selected backup with `r` from menu item `3`.
3. Verify the gateway service stops during restore and starts after restore.
4. Verify restored OpenClaw config/data matches the backup.

**Expected result.**

- Restore verifies the backup before replacing `~/.openclaw`.
- Ownership and permissions are correct after restore.
- Gateway service is running after restore for an active user.

**Capture on failure.**

- Restore command output.
- `ls -la /home/<username>/.openclaw`
- `journalctl --user -u openclaw-gateway`

## Phase 4: Health Check / Doctor

### UC-0401: Run Read-only Health Check

- Phase: 4
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- At least one active managed user exists.

**Steps.**

1. Open menu item `4. Health check / Doctor`.
2. Run a normal health check.
3. Review system, services, users, filesystem, network, and OpenClaw sections.
4. Confirm no mutation happens during the read-only run.

**Expected result.**

- Results are grouped by category with `ok`, `warn`, `fail`, or `skipped`.
- Later-phase components that are not yet implemented show `skipped`, not false
  failures.
- Paused users are skipped for runtime checks.

**Capture on failure.**

- TUI screenshot/transcript.
- `journalctl -u openclaw-overlay-api -u cloudflared --no-pager -n 100`
- `su - <user> -c '/home/<user>/.local/bin/openclaw doctor --json'`

### UC-0402: Apply Allowed Permission Fix

- Phase: 4
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- A disposable managed user exists.

**Steps.**

1. Intentionally set a fixable permission drift, for example
   `chmod 0755 /home/<user>/.openclaw`.
2. Run menu item `4`.
3. Apply only the offered allowlisted fix.
4. Re-run health check.

**Expected result.**

- Only allowlisted fixes are offered.
- Permission is corrected to the documented mode.
- No unrelated system mutation happens.

**Capture on failure.**

- Before/after `stat` output.
- `doctor_fix` audit event.

## Phase 5: Network and Firewall

### UC-0501: Inspect Network Snapshot

- Phase: 5
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- Tailscale/cloudflared/UFW installed according to chosen install path.
- At least one managed route exists in state.

**Steps.**

1. Open menu item `5. Network and firewall`.
2. Press `r` to refresh.
3. Review Tailscale, Cloudflare Tunnel, UFW, routes, and listening ports.

**Expected result.**

- Tailscale is shown as admin-only.
- Cloudflared version/service/config-derived fields are visible.
- Route inventory shows hostname, local port, owner, enabled state, and cached
  last_seen if available.
- Public listeners are flagged.

**Capture on failure.**

- TUI screenshot/transcript.
- `tailscale status --json`
- `cloudflared --version`
- `ufw status verbose`
- `ss -ltnp`

### UC-0502: Plan and Apply Wildcard DNS

- Phase: 5
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- Account/named tunnel mode.
- Config has `domain`, `subdomain`, `tunnel_id`, `cloudflare_zone_id`, and
  `cloudflare_api_token`.

**Steps.**

1. Open menu item `5`.
2. Press `d` to build the wildcard DNS plan.
3. Review the planned record:
   `*.${subdomain}.${domain} CNAME ${tunnel_id}.cfargotunnel.com`.
4. Apply the plan.
5. Verify the DNS record in Cloudflare dashboard or API.

**Expected result.**

- Missing credentials produce a clear precondition error and no mutation.
- Existing matching record results in `noop`.
- Missing record is created.
- Differing matching record is updated only after explicit review.

**Capture on failure.**

- DNS plan text.
- Redacted Cloudflare API response.
- `dns_update` audit event.

### UC-0503: Plan and Apply UFW Route Ports

- Phase: 5
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- UFW installed.
- One or more enabled routes exist.

**Steps.**

1. Open menu item `5`.
2. Press `u` to build the UFW plan.
3. Review missing enabled route ports.
4. Apply the plan.
5. Run `ufw status verbose`.

**Expected result.**

- Existing rules are preserved.
- UFW is not reset.
- Required route ports are allowed.
- Default policy remains deny incoming / allow outgoing.

**Capture on failure.**

- Before/after `ufw status verbose`.
- `ufw_update` audit event.

### UC-0504: Probe Gateway URL

- Phase: 5, 6
- Scenario type: `owner-vps`
- Status: `blocked`

**Preconditions.**

- Phase 6 route publication is implemented.
- Gateway route exists and DNS is configured.

**Steps.**

1. Open menu item `5`.
2. Press `p` to probe enabled gateway URLs.
3. Independently run `curl -vk https://gateway-<user>.<subdomain>.<domain>/`.

**Expected result.**

- Probe reaches the user's gateway through Cloudflare Tunnel.
- Per-route failures are shown without aborting the full probe batch.

**Capture on failure.**

- TUI probe result.
- `curl -vk` output.
- `journalctl -u cloudflared --no-pager -n 200`.

## Phase 6: Overlay-API and Cloudflared Publication

### UC-0601: SIGHUP Reloads Local Cloudflared Config

- Phase: 6
- Scenario type: `automated-smoke`, `owner-vps`
- Status: `planned`

**Preconditions.**

- Account/named Cloudflare Tunnel is running as `cloudflared.service`.
- Wildcard DNS points at the tunnel.
- Phase 6 overlay-API is installed and running.
- Test user `alice` exists with a running local gateway.

**Steps.**

1. Record current cloudflared process ID:
   `systemctl show cloudflared -p MainPID`.
2. Through overlay-API, publish or update
   `gateway-alice.<subdomain>.<domain>` to Alice's local gateway port.
3. Verify `/etc/cloudflared/config.yml` contains the new ingress rule and final
   `http_status:404`.
4. Verify configured credentials file exists. Default expected path:
   `/etc/cloudflared/<tunnel_id>.json`, unless `cloudflared_credentials_file`
   overrides it.
5. Send SIGHUP to cloudflared from root shell:
   `kill -HUP $(systemctl show cloudflared -p MainPID --value)`.
6. Verify the process did not exit unexpectedly:
   `systemctl is-active cloudflared` and `systemctl show cloudflared -p MainPID`.
7. Request the new route:
   `curl -vk https://gateway-alice.<subdomain>.<domain>/`.
8. Disable the route through overlay-API, send SIGHUP again, and request the
   same URL.
9. Record cloudflared version:
   `cloudflared --version`.
10. Record cloudflared service unit:
   `systemctl cat cloudflared`.

**Expected result.**

- SIGHUP is the selected Phase 6 reload mechanism.
- Cloudflared stays active after SIGHUP.
- Newly added route becomes reachable after SIGHUP.
- Disabled route stops reaching Alice's gateway after SIGHUP and falls through
  to the catch-all behavior.
- Recorded version and service unit identify the exact cloudflared target that
  was tested.

**Capture on failure.**

- `/etc/cloudflared/config.yml` before/after with secrets redacted.
- `journalctl -u cloudflared --no-pager -n 300`.
- `systemctl status cloudflared`.
- `cloudflared --version`.
- `systemctl cat cloudflared`.
- `curl -vk` output for enabled and disabled route.

### UC-0602: Overlay-API Rejects Wrong User UID

- Phase: 6
- Scenario type: `automated-smoke`, `owner-vps`
- Status: `planned`

**Preconditions.**

- Users `alice` and `bob` exist.
- overlay-API is listening on `/run/openclaw-overlay.sock`.

**Steps.**

1. As `alice`, call a route endpoint for `alice`.
2. As `alice`, call the same route endpoint for `bob`.
3. As the configured admin, call the route endpoint for `bob`.
4. As root, call the route endpoint for `bob`.

**Expected result.**

- Alice can manage Alice's allowed route operations.
- Alice receives authorization failure for Bob's routes.
- The configured admin can manage Bob's routes.
- Root can manage Bob's routes.

**Capture on failure.**

- HTTP status and JSON body.
- overlay-API journal logs.
- State DB route rows for both users.

### UC-0603: Cloudflared Config Rollback

- Phase: 6
- Scenario type: `automated-smoke`, `owner-vps`
- Status: `planned`

**Preconditions.**

- overlay-API is running with a known-good cloudflared config.

**Steps.**

1. Trigger a route publication that makes validation fail in a controlled way
   on a disposable VPS or smoke harness.
2. Inspect `/etc/cloudflared/config.yml`.
3. Inspect `/var/lib/openclaw-multi/snapshots/`.
4. Check overlay-API response and audit events.

**Expected result.**

- Invalid candidate config is not activated.
- Previous config is restored.
- Audit reports error or rollback result.

**Capture on failure.**

- Candidate and restored configs.
- overlay-API response body.
- `cloudflared_config_publish` / reload audit events.

### UC-0604: Daemon-derived Plugin Route IDs Are Idempotent

- Phase: 6, 7
- Scenario type: `automated-smoke`, `owner-vps`
- Status: `planned`

**Preconditions.**

- Phase 6 overlay-API is installed and running.
- Test user `alice` exists.
- A disposable local callback service is listening on a known port.

**Steps.**

1. Call `POST /users/alice/routes` through the UNIX socket with
   `plugin_id=test-plugin`, `hostname_hint=callback`, and the callback local
   port.
2. Record the returned `route_id`, hostname, and URL.
3. Call the same endpoint again with the same `plugin_id`, `hostname_hint`, and
   port.
4. Verify the second response returns the same `route_id` and updates the same
   state row rather than creating a duplicate.
5. Change only the local port and call the same endpoint again.
6. Verify the route keeps the same `route_id` and points to the new local port.
7. Call the endpoint with the same `plugin_id` but a different
   `hostname_hint`, for example `oauth`.
8. Verify a second deterministic route is created for the new hint.
9. Verify both plugin routes are present in `/etc/cloudflared/config.yml` after
   publication.

**Expected result.**

- overlay-API derives plugin route IDs from
  `(username, plugin_id, hostname_hint)`.
- Repeating the same tuple is idempotent and does not create duplicate routes.
- Updating the local port for the same tuple updates the existing route.
- A different hostname hint creates a separate deterministic route.
- Both plugin routes are rendered into cloudflared config with unique hostnames.

**Capture on failure.**

- Request/response JSON with secrets redacted.
- State DB route rows for `alice`.
- Rendered cloudflared config before and after each call.
- overlay-API journal logs.

### UC-0605: Sign-up Publishes Gateway Route Through Overlay-API

- Phase: 6
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- Phase 6 overlay-API is installed and running.
- `cloudflared.service` is active.
- Wildcard DNS points at the tunnel.
- Test username `alice-signup` does not exist, or has been fully removed with a
  backup/snapshot captured first.

**Steps.**

1. Start the admin TUI as the configured admin.
2. Create managed user `alice-signup` through the normal sign-up/user creation
   flow.
3. Confirm the created user's gateway service is running locally.
4. Confirm state has an enabled gateway route for `alice-signup`.
5. Confirm `/etc/cloudflared/config.yml` contains
   `gateway-alice-signup.<subdomain>.<domain>` and the final
   `http_status:404` rule.
6. Confirm overlay-API audit contains config publication and SIGHUP events.
7. Request `https://gateway-alice-signup.<subdomain>.<domain>/`.
8. Disable the user through the TUI.
9. Confirm the route remains in state but is disabled and omitted from the
   rendered cloudflared config.
10. Send or confirm overlay-API SIGHUP reload and request the same URL again.
11. Confirm overlay-API and cloudflared logs for the publication and reload:
    `journalctl -u openclaw-overlay-api --no-pager -n 300` and
    `journalctl -u cloudflared --no-pager -n 300`.

**Expected result.**

- Sign-up creates the managed user and publishes the gateway route through
  overlay-API, not by directly editing cloudflared config from the TUI.
- The public gateway URL reaches the user's local gateway after publication and
  reload.
- Disabling the user removes the active ingress rule from cloudflared config and
  the public URL no longer reaches the user's gateway.
- Overlay-API and cloudflared logs contain the expected publication and reload
  entries without errors.

**Capture on failure.**

- TUI action log or terminal output.
- overlay-API response, if visible in the TUI or terminal.
- `journalctl -u openclaw-overlay-api --no-pager -n 300`.
- `journalctl -u cloudflared --no-pager -n 300`.
- Rendered cloudflared config with secrets redacted.
- State DB route row for `alice-signup`.
- `curl -vk` output for enabled and disabled URL.

## Phase 7: Per-User Overlay Watcher

### UC-0701: Watcher Publishes Plugin Callback Route

- Phase: 7
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- Phase 6 overlay-API is installed and running.
- Managed user `alice` exists and is active.
- `openclaw-overlay-watcher.service` is running as `alice`.
- A test plugin or config fixture creates a callback-capable plugin entry in
  `~/.openclaw/openclaw.json`.

**Steps.**

1. As `alice`, add or update the callback-capable plugin entry in
   `~/.openclaw/openclaw.json`.
2. Wait for `openclaw-overlay-watcher.service` to process the change.
3. Check `journalctl --user -u openclaw-overlay-watcher --no-pager -n 200` as
   `alice`.
4. Confirm overlay-API has a plugin route for `alice` with expected
   `plugin_id`, hostname hint, local port, and enabled state.
5. Confirm `/etc/cloudflared/config.yml` contains the plugin callback hostname.
6. Confirm OpenClaw config contains callback URL and callback port values
   written by the watcher.
7. Request the callback URL with `curl -vk`.

**Expected result.**

- Watcher detects the config change and calls overlay-API as `alice`.
- overlay-API returns a daemon-derived plugin route ID.
- Cloudflared config contains the callback ingress rule.
- OpenClaw config contains the returned callback URL and port.
- Callback URL reaches the local callback service.

**Capture on failure.**

- `~/.openclaw/openclaw.json` with secrets redacted.
- `~/.openclaw-overlay/watcher.state`.
- `journalctl --user -u openclaw-overlay-watcher --no-pager -n 300`.
- `journalctl -u openclaw-overlay-api --no-pager -n 300`.
- Rendered cloudflared config with secrets redacted.
- State DB route row for the plugin route.

### UC-0702: Watcher Resync Is Idempotent

- Phase: 7
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- UC-0701 has passed for `alice`.
- Watcher state exists at `~/.openclaw-overlay/watcher.state`.

**Steps.**

1. Restart `openclaw-overlay-watcher.service` as `alice`.
2. Touch or rewrite `~/.openclaw/openclaw.json` without changing callback
   plugin data.
3. Wait for watcher processing.
4. List plugin routes for `alice` through overlay-API or state DB.
5. Compare route ID, hostname, URL, and local port with the values from
   UC-0701.

**Expected result.**

- Watcher does not create duplicate plugin routes.
- Route ID remains stable.
- OpenClaw callback URL and port remain stable.
- Watcher logs show no error during restart/resync.

**Capture on failure.**

- Previous and current `watcher.state`.
- State DB route rows for `alice`.
- Watcher journal logs.
- overlay-API journal logs.

### UC-0703: Plugin Removal Removes Callback Route

- Phase: 7
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- UC-0701 has passed for `alice`.
- The plugin callback route is enabled and present in cloudflared config.

**Steps.**

1. As `alice`, remove the callback-capable plugin entry from
   `~/.openclaw/openclaw.json`.
2. Wait for watcher processing.
3. Confirm watcher journal logs show the removal was processed.
4. Confirm the plugin callback route is removed or disabled according to Phase 7
   implementation behavior.
5. Confirm `/etc/cloudflared/config.yml` no longer contains an active ingress
   rule for the removed plugin callback.
6. Request the old callback URL with `curl -vk`.

**Expected result.**

- Watcher detects plugin removal.
- Removed plugin callback route is no longer active.
- Old public callback URL no longer reaches Alice's callback service.
- Watcher state no longer marks the removed plugin as synced.

**Capture on failure.**

- OpenClaw config before/after with secrets redacted.
- `watcher.state` before/after.
- State DB route rows for `alice`.
- Watcher and overlay-API journal logs.
- Rendered cloudflared config.
- `curl -vk` output for old callback URL.

### UC-0704: Watcher Restart Uses Persisted Snapshot

- Phase: 7
- Scenario type: `owner-vps`
- Status: `planned`

**Preconditions.**

- UC-0701 has passed for `alice`.
- `~/.openclaw-overlay/watcher.state` exists.

**Steps.**

1. Stop `openclaw-overlay-watcher.service` as `alice`.
2. Start `openclaw-overlay-watcher.service` again.
3. Check watcher logs for startup and initial sync.
4. Confirm no duplicate plugin routes are created.
5. Confirm existing callback URL remains reachable.
6. Confirm `watcher.state` still matches overlay-API route response.

**Expected result.**

- Watcher loads persisted snapshot on restart.
- Initial sync is idempotent.
- Existing plugin callback route remains stable and reachable.
- No duplicate route rows appear.

**Capture on failure.**

- Watcher journal logs.
- `watcher.state`.
- State DB route rows for `alice`.
- overlay-API journal logs.
- Rendered cloudflared config.
