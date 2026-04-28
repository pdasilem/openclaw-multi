# Overlay API

Phase 6 replaces the `openclaw-overlay-api` stub with a local HTTP daemon on a
UNIX socket.

## Socket

Default path:

```text
/run/openclaw-overlay.sock
```

Managed users may connect to the socket. Authorization is still enforced by
Linux peer credentials (`SO_PEERCRED`):

- root UID `0` may manage every user;
- the configured admin UID may manage every user;
- a managed user UID may manage only that user's routes.

## Endpoints

- `GET /health`
- `GET /users`
- `POST /users/<username>/gateway-route`
- `DELETE /users/<username>/gateway-route`
- `POST /users/<username>/routes`
- `DELETE /users/<username>/routes/<route_id>`
- `GET /users/<username>/routes`
- `GET /users/<username>/routes/<route_id>/last-seen`
- `POST /users/<username>/disable`
- `POST /users/<username>/enable`
- `POST /cloudflared/reload`

All responses are JSON. Error responses use:

```json
{"error":"message"}
```

## Route IDs

Gateway routes use:

```text
gateway:<username>
```

Plugin route IDs are daemon-derived from:

```text
plugin:<username>:<plugin_id>:<hostname_hint>
```

Repeated plugin route requests with the same tuple update the existing route.

## Cloudflared Publication

The daemon renders `/etc/cloudflared/config.yml` from enabled routes in
`state.db`. Disabled routes stay in state but are omitted from generated
ingress rules. A final `http_status:404` catch-all is always rendered.

The credentials file comes from `cloudflared_credentials_file`. If empty, the
daemon derives:

```text
/etc/cloudflared/<tunnel_id>.json
```

Publication fails before writing config when the resolved credentials file does
not exist.

Publication flow:

1. render candidate config;
2. back up existing config under `/var/lib/openclaw-multi/snapshots/`;
3. write candidate config;
4. run `cloudflared tunnel ingress validate --config <path>`;
5. send SIGHUP to `cloudflared`;
6. roll back config on validation or SIGHUP failure.
