# Backups and Restore

> Phase 3 is test-only. The code models the intended VPS operations through
> `shell.Executor` and `shell.FS`, but live validation is deferred to a real VPS
> check.

OpenClaw Multi stores encrypted per-user OpenClaw backups and keeps backup
metadata in `state.db`.

## Storage

Default paths:

- master key: `/etc/openclaw-multi/master.key`
- backup root: `/var/lib/openclaw-multi/backups`
- per-user directory: `/var/lib/openclaw-multi/backups/<username>`
- encrypted archive:
  `/var/lib/openclaw-multi/backups/<username>/<timestamp>.tar.gz.enc`

The master key is generated automatically if it is missing and written with
mode `0600`. Empty master-key files are rejected.

Backups intentionally survive user deletion. Deleting a managed user removes
routes, user state, and frees the port, but backup metadata remains queryable by
the original username.

## Create

The backend flow is:

1. Verify the managed user exists.
2. Ensure the master key exists.
3. Run `sudo -u <user> -H bash -lc "/home/<user>/.local/bin/openclaw backup create --output ~/.openclaw-backup-tmp --verify"`.
4. Parse the produced archive path from stdout.
5. Encrypt with `openssl enc -aes-256-cbc -pbkdf2 -salt`.
6. Compute SHA-256 and size of the encrypted archive.
7. Record metadata in `state.db`.
8. Emit `backup_create`.

TUI user management exposes `b` for backup on the selected user.

The non-interactive timer entrypoint is:

```bash
openclaw-multi backup <username>
```

## Restore

Phase 3 restores into existing managed users only. Restore-to-missing-user is
deferred.

The backend flow is:

1. Verify target user exists.
2. Load backup metadata and verify ownership.
3. Decrypt the archive to a temporary `.tar.gz`.
4. Run `openclaw backup verify`.
5. Stop `openclaw-gateway` for the user.
6. Extract to a restore temp directory.
7. Replace `~/.openclaw`.
8. Fix ownership.
9. Start `openclaw-gateway`.
10. Emit `backup_restore`.

TUI user management exposes `r` to list backups for the selected user and
restore the selected backup.

## Remove Safety

User removal is backup-first after Phase 3. If the pre-remove backup fails, the
remove operation aborts before route deletion, OpenClaw uninstall, linger
disable, `userdel`, state deletion, or port reuse.

Active users still follow the Phase 2 deactivate-first flow before hard delete.

## Auto Timer

Phase 3 adds systemd templates:

- `/etc/systemd/system/openclaw-backup@.service`
- `/etc/systemd/system/openclaw-backup@.timer`

The installer renders the templates through `shell.FS` and runs:

```bash
systemctl daemon-reload
systemctl enable --now openclaw-backup@<username>.timer
```

No real timer execution happens in tests.
