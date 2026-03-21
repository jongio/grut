#!/usr/bin/env bash
# bench-release-notes.sh — generate a ### Performance section for CHANGELOG.md.
#
# Usage: bench-release-notes.sh <before.txt> <current.txt> <version>
#
# Outputs a keep-a-changelog-formatted ### Performance block to stdout.
# Pipe it or capture it to inject into CHANGELOG.md before goreleaser runs.

set -euo pipefail

BEFORE="${1:?Usage: bench-release-notes.sh <before.txt> <current.txt> <version>}"
AFTER="${2:?}"
VERSION="${3:?}"

COMPARISON=$(benchstat "$BEFORE" "$AFTER" 2>&1)

# Extract lines with statistically significant changes (both improvements and regressions).
SIGNIFICANT=$(echo "$COMPARISON" | awk '
  /(\+|-)[0-9]+\.[0-9]+%.*\(p=/ {
    # Extract p-value (POSIX-compatible: no capture groups)
    if (match($0, /\(p=[0-9.]+/)) {
      if (substr($0, RSTART+3, RLENGTH-3) + 0 < 0.05) print
    }
  }
')

cat <<EOF
### Performance

Benchmark comparison: \`${VERSION}\` vs previous release.

\`\`\`
${COMPARISON}
\`\`\`

EOF

if [ -n "$SIGNIFICANT" ]; then
  echo "**Statistically significant changes (p < 0.05):**"
  echo ""
  echo "$SIGNIFICANT" | while IFS= read -r line; do
    # Determine direction for bullet prefix
    if echo "$line" | grep -qE '^\s*-'; then
      prefix="▼"  # improvement
    else
      prefix="▲"  # regression
    fi
    # Extract benchmark name (first field) and percentage (last +/-X%)
    name=$(echo "$line" | awk '{print $1}')
    pct=$(echo "$line" | grep -oE '[+-][0-9]+\.[0-9]+%' | tail -1)
    echo "- ${prefix} \`${name}\`: ${pct}"
  done
  echo ""
fi

echo "Full benchmark data archived in \`perf/baselines/${VERSION}.txt\`."
