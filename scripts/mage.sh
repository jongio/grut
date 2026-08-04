#!/bin/sh
# Run mage targets using a project-local mage binary in bin/.
#
# Installs mage into the project's bin/ directory (not a temp directory) and
# forwards all arguments to it.  On Windows this avoids Defender false-
# positive blocks; on all platforms it keeps the binary local to the project.
#
# Usage:
#   ./scripts/mage.sh install
#   ./scripts/mage.sh preflight
#   ./scripts/mage.sh -l              # List available targets

set -eu

# ---------------------------------------------------------------------------
# Resolve paths
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$REPO_ROOT/bin"
MAGE_BIN="$BIN_DIR/mage"

# ---------------------------------------------------------------------------
# Ensure mage is installed in bin/
# ---------------------------------------------------------------------------
if [ ! -x "$MAGE_BIN" ]; then
    printf '  Installing mage to bin/...\n'
    mkdir -p "$BIN_DIR"
    GOBIN="$BIN_DIR" go install github.com/magefile/mage@latest
    if [ ! -x "$MAGE_BIN" ]; then
        printf 'ERROR: mage binary not found at %s after install\n' "$MAGE_BIN" >&2
        exit 1
    fi
    printf '  Installed mage to %s\n' "$MAGE_BIN"
fi

# ---------------------------------------------------------------------------
# Forward all arguments to mage
# ---------------------------------------------------------------------------
exec "$MAGE_BIN" "$@"
