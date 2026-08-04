#!/usr/bin/env bash
# cleanup-merged-branches.sh — delete remote refs for branches already merged to main.
#
# Usage: cleanup-merged-branches.sh [--dry-run]
#
# These branches were merged via their respective PRs but the remote refs
# were not deleted automatically. Run this script to clean them up.
#
# References:
#   feature/branch-diff-mode            — PR #55, merged 2026-04-09
#   feature/github-tab-pagination       — PR #54, merged 2026-04-09
#   feature/preview-editor              — PR #56, merged 2026-04-09
#   fix/worktree-switch-pending-path    — PR #53, merged 2026-04-09

set -euo pipefail

BRANCHES=(
  "feature/branch-diff-mode"
  "feature/github-tab-pagination"
  "feature/preview-editor"
  "fix/worktree-switch-pending-path"
)

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=true
fi

for branch in "${BRANCHES[@]}"; do
  if git ls-remote --heads origin "$branch" | grep -q .; then
    if $DRY_RUN; then
      echo "[dry-run] would delete origin/$branch"
    else
      echo "Deleting origin/$branch ..."
      git push origin --delete "$branch"
    fi
  else
    echo "Already deleted: origin/$branch"
  fi
done
