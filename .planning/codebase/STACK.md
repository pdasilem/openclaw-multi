# Stack

## Runtime
- Go module: `github.com/pdasilem/openclaw-multi`
- Go version: `1.26.2`
- Primary target: Linux VPS administration, system services, and per-user OpenClaw runtime management.

## Binaries
- `cmd/openclaw-multi`: administrative TUI plus `system-prepare` and `backup` subcommands.
- `cmd/openclaw-overlay-api`: system daemon exposing HTTP over a UNIX socket and publishing Cloudflare tunnel routes.
- `cmd/openclaw-overlay-watcher`: per-user watcher that discovers plugin callbacks and syncs routes through the overlay API.

## Direct Dependencies
- `charm.land/bubbletea/v2`: terminal UI event loop.
- `charm.land/lipgloss/v2`: terminal styling.
- `github.com/creack/pty`: pseudo-terminal support for embedded terminal flows.
- `github.com/fsnotify/fsnotify`: watcher filesystem notifications.
- `gopkg.in/yaml.v3`: overlay and watcher config parsing.
- `modernc.org/sqlite`: pure-Go SQLite state store.

## Tooling
- Build and CI are Makefile-driven.
- `make build` builds all three binaries into `bin/`.
- `make test` runs `go test -race -coverprofile=coverage.out ./...`.
- `make lint` runs `golangci-lint run ./...`.
- `make ci` runs lint, tests, build, schema checks, install checks, and shellcheck.

