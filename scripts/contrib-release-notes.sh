#!/usr/bin/env bash
# contrib-release-notes.sh — generate contributor sections for release notes.
#
# Usage: contrib-release-notes.sh <from-tag> <to-ref> [changelog|release|contributors]
#
# Outputs formatted contributor text to stdout.
# Default format is "changelog" if not specified.
#
# This is a thin wrapper that invokes `go run` with the internal contributor
# package. For manual use, prefer `mage contributors`.

set -euo pipefail

FROM_TAG="${1:?Usage: contrib-release-notes.sh <from-tag> <to-ref> [changelog|release|contributors]}"
TO_REF="${2:?}"
FORMAT="${3:-changelog}"

cd "$(git rev-parse --show-toplevel)"

go run ./cmd/contrib-notes -from="$FROM_TAG" -to="$TO_REF" -format="$FORMAT"
