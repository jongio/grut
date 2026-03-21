#!/usr/bin/env bash
# bench-regress.sh — detect statistically significant benchmark regressions.
#
# Usage: bench-regress.sh <before.txt> <after.txt> [threshold_pct] [mem_threshold_pct]
#
# Exits 0 if no significant regressions, 1 if regressions exceed threshold.
# A regression is flagged when:
#   - the change is positive (slower / more memory)
#   - the p-value is < 0.05 (statistically significant)
#   - the magnitude exceeds threshold_pct for timing metrics (default: 15%)
#   - the magnitude exceeds mem_threshold_pct for memory metrics (default: 10%)
#     Memory metrics: heap-inuse-b/op, gc-cycles/op

set -euo pipefail

BEFORE="${1:?Usage: bench-regress.sh <before.txt> <after.txt> [threshold_pct] [mem_threshold_pct]}"
AFTER="${2:?Usage: bench-regress.sh <before.txt> <after.txt> [threshold_pct] [mem_threshold_pct]}"
THRESHOLD="${3:-15}"
MEM_THRESHOLD="${4:-10}"

OUTPUT=$(benchstat "$BEFORE" "$AFTER")

# Parse benchstat output for significant regressions.
# benchstat v2 line format (any metric section):
#   BenchmarkName/sub-32   1.23µ ± 1%   1.45µ ± 2%   +17.89% (p=0.008 n=6)
# Memory metrics (heap-inuse-b/op, gc-cycles/op) use the lower mem threshold.
REGRESSIONS=$(echo "$OUTPUT" | awk -v threshold="$THRESHOLD" -v mem_threshold="$MEM_THRESHOLD" '
  # Match lines that have a positive percentage change and a p-value
  /\+[0-9]+\.[0-9]+%.*\(p=/ {
    # Extract percentage magnitude (POSIX-compatible: no capture groups)
    pct = 0
    if (match($0, /\+[0-9]+\.[0-9]+%/)) {
      pct = substr($0, RSTART+1, RLENGTH-2) + 0
    } else {
      next
    }
    # Extract p-value (POSIX-compatible)
    pval = 1
    if (match($0, /\(p=[0-9.]+/)) {
      pval = substr($0, RSTART+3, RLENGTH-3) + 0
    } else {
      next
    }
    # Use lower threshold for memory metrics
    is_mem = ($0 ~ /heap-inuse-b\/op/ || $0 ~ /gc-cycles\/op/)
    t = is_mem ? mem_threshold : threshold
    # Report if statistically significant AND large enough
    if (pval < 0.05 && pct > t) {
      print
    }
  }
')

if [ -n "$REGRESSIONS" ]; then
  echo ""
  echo "Statistically significant regressions (timing >${THRESHOLD}%, memory >${MEM_THRESHOLD}%, p<0.05):"
  echo "$REGRESSIONS"
  exit 1
fi

echo "No significant regressions (timing threshold: ${THRESHOLD}%, memory threshold: ${MEM_THRESHOLD}%, p<0.05)."
exit 0
