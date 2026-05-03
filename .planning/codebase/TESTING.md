# Testing

## Commands
- `make test`: race-enabled Go tests with coverage for all packages.
- `make test-unit`: race-enabled Go tests excluding `/test/` packages.
- `make build`: builds all binaries.
- `make schema-check`: validates `schemas/state.sql` with SQLite and JSON schemas with `jq`.
- `make install-check`: verifies binaries and templates are installed to expected paths under a temporary `DESTDIR`.
- `make ci`: lint, test, build, schema-check, install-check, and shellcheck.

## Coverage Shape
- Package-level tests exist for state, watcher, shell, preflight, cloudflared, hardening, API, deps, admin, network, users, doctor, backup, audit, TUI, wizard, config, systemprep, and watcher command entrypoint behavior.
- Default tests exercise command construction and parsing through mocks, not live system mutation.
- Docker smoke support exists under `test/docker` and `test/e2e/phase-0`.

## Test Patterns
- Use `shell.MockExecutor` or package-specific mock executors for command flows.
- Use `shell.NewMemFS` or temp directories for filesystem flows.
- Use temporary SQLite stores for state behavior.
- Validate generated service/config content through rendered output and install checks.

## Risk-Based Test Needs
- User lifecycle, privileged command generation, route publishing, and Cloudflare config writes need focused regression tests for every behavior change.
- TUI changes need model update/view tests plus wrap/layout checks for long operational messages.
- Network and doctor parsers need fixture-style tests for real command output variants.
- Changes that touch templates should run `make install-check` and targeted tests for the package rendering or consuming the template.

