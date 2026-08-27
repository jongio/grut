#!/usr/bin/env bash
# bench-interleaved.sh — sample baseline and current benchmarks in alternating
# order so runner drift cannot masquerade as a code regression.
#
# Why: running every benchmark for one version and then every benchmark for the
# other puts the two groups in separate time windows.  Anything that changes the
# runner between those windows (cold caches, CPU frequency ramp, a noisy
# neighbour on a shared VM) shifts one whole group in one direction.  benchstat
# then reports a large, uniform, statistically significant "regression" across
# entire packages, including packages the change never touched.  Tight variance
# and a small p-value do not rule this out: the comparison controls for
# runner-to-runner variance, but not for one runner drifting over time.
#
# This script compiles both versions up front, then alternates between them one
# round at a time, so both are sampled across the same window and drift applies
# to both equally.  The version that runs first alternates each round, so
# neither systematically absorbs any per-round warm-up cost.
#
# Usage: bench-interleaved.sh <base_ref> <rounds> <baseline_out> <current_out> <pkg>...

set -euo pipefail

BASE_REF="${1:?Usage: bench-interleaved.sh <base_ref> <rounds> <baseline_out> <current_out> <pkg>...}"
ROUNDS="${2:?missing rounds}"
BASELINE_OUT="${3:?missing baseline output path}"
CURRENT_OUT="${4:?missing current output path}"
shift 4

if [ "$#" -eq 0 ]; then
  echo "error: no benchmark packages given" >&2
  exit 2
fi
PKGS=("$@")

BIN_DIR="$(mktemp -d)"
CURRENT_REF="$(git rev-parse HEAD)"

# Always return the worktree to the commit under test, even on failure, so a
# crash here cannot leave the checkout parked on the base commit.
cleanup() {
  git checkout "$CURRENT_REF" --quiet 2>/dev/null || true
  rm -rf "$BIN_DIR"
}
trap cleanup EXIT

# ./internal/panels/filetree/ -> internal_panels_filetree
pkg_slug() {
  local p="${1#./}"
  p="${p%/}"
  echo "${p//\//_}"
}

compile() {
  local pkg="$1" dest="$2"
  # A package with no test files compiles successfully but emits no binary,
  # so the caller checks for the file rather than trusting the exit code.
  go test -c -o "$dest" "$pkg" >/dev/null 2>&1 || return 1
  [ -f "$dest" ]
}

mkdir -p "$BIN_DIR/current" "$BIN_DIR/baseline"

echo "Compiling current benchmarks ($CURRENT_REF)…"
for pkg in "${PKGS[@]}"; do
  slug="$(pkg_slug "$pkg")"
  if ! compile "$pkg" "$BIN_DIR/current/$slug.test"; then
    echo "error: failed to build benchmarks for $pkg" >&2
    exit 1
  fi
done

echo "Compiling baseline benchmarks ($BASE_REF)…"
git checkout "$BASE_REF" --quiet
VALID=()
for pkg in "${PKGS[@]}"; do
  slug="$(pkg_slug "$pkg")"
  # The PR may add packages that have no counterpart on the base commit.
  if compile "$pkg" "$BIN_DIR/baseline/$slug.test"; then
    VALID+=("$pkg")
  else
    echo "  skipping $pkg (absent or has no benchmarks on base)"
  fi
done
git checkout "$CURRENT_REF" --quiet

# Create the outputs before any early exit, so callers that upload them as
# artifacts always find a file.
: >"$BASELINE_OUT"
: >"$CURRENT_OUT"

if [ "${#VALID[@]}" -eq 0 ]; then
  echo "No benchmark packages are comparable against the base commit."
  exit 3
fi

run_one() {
  "$1" -test.bench=. -test.benchmem -test.count=1 -test.run='^$' -test.timeout=15m >>"$2"
}

for ((round = 1; round <= ROUNDS; round++)); do
  echo "Round $round/$ROUNDS…"
  for pkg in "${VALID[@]}"; do
    slug="$(pkg_slug "$pkg")"
    if ((round % 2 == 1)); then
      run_one "$BIN_DIR/baseline/$slug.test" "$BASELINE_OUT"
      run_one "$BIN_DIR/current/$slug.test" "$CURRENT_OUT"
    else
      run_one "$BIN_DIR/current/$slug.test" "$CURRENT_OUT"
      run_one "$BIN_DIR/baseline/$slug.test" "$BASELINE_OUT"
    fi
  done
done

echo "Collected $ROUNDS interleaved samples for ${#VALID[@]} package(s)."
