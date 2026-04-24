#!/usr/bin/env bash
# Phase-0 smoke test: build → Docker → navigate TUI → verify state.db + audit.log
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${REPO_ROOT}"

echo "==> [smoke] Building linux/amd64 binary..."
make build-amd64

echo "==> [smoke] Building Docker test image..."
make test-docker-build

echo "==> [smoke] Running TUI smoke test in container..."

TMPDIR_HOST="$(mktemp -d)"
trap 'rm -rf "${TMPDIR_HOST}"' EXIT

docker run --rm \
  --env OPENCLAW_STATE_DIR=/tmp/overlay-test \
  --env OPENCLAW_AUDIT_LOG=/tmp/overlay-test/audit.log \
  --env SUDO_USER="" \
  --user opadmin \
  --entrypoint /bin/bash \
  openclaw-multi-test \
  -c '
    set -euo pipefail
    mkdir -p /tmp/overlay-test

    # Run TUI non-interactively via expect script.
    # The first-run screen prompts to confirm admin, then we navigate and quit.
    expect -c "
      set timeout 10
      spawn env OPENCLAW_STATE_DIR=/tmp/overlay-test \
                OPENCLAW_AUDIT_LOG=/tmp/overlay-test/audit.log \
                openclaw-multi
      # Confirm first-run admin setup (press Y)
      expect -re {First-run|admin candidate}
      send \"y\"
      # Main menu should appear
      expect -re {Установка с нуля|OpenClaw}
      # Quit
      send \"q\"
      expect eof
    "

    echo "==> [smoke] Verifying state.db was created..."
    test -f /tmp/overlay-test/state.db || { echo "ERROR: state.db not found"; exit 1; }

    echo "==> [smoke] Verifying admin row in state.db..."
    ADMIN_ROW=$(sqlite3 /tmp/overlay-test/state.db "SELECT username FROM admin LIMIT 1;")
    test -n "${ADMIN_ROW}" || { echo "ERROR: no admin row in state.db"; exit 1; }
    echo "  admin: ${ADMIN_ROW}"

    echo "==> [smoke] Verifying audit.log was created..."
    test -f /tmp/overlay-test/audit.log || { echo "ERROR: audit.log not found"; exit 1; }

    echo "==> [smoke] Verifying audit.log has startup entry..."
    grep -q "startup\|admin_set" /tmp/overlay-test/audit.log || \
      { echo "ERROR: no startup/admin_set entry in audit.log"; exit 1; }

    echo "==> [smoke] All checks passed."
  '

echo "==> [smoke] Phase-0 smoke test PASSED."
