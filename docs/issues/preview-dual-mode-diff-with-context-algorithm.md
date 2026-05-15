# Preview dual-mode diff with file context algorithm

## Summary

Add a toggle in the preview panel between "file on disk" and "contextual diff" modes, with an algorithm that determines the correct diff to show based on the filetree's current mode (commit, PR, branch, git-filter, or normal).

## Description

Currently the preview panel shows file content with an appended diff section when changes exist. This is neither clean file viewing nor clean diff viewing - it's a hybrid that doesn't serve either purpose well.

The feature adds:
1. A `v` keybinding (when preview is focused) to toggle between file-on-disk mode and contextual diff mode
2. A DiffContext algorithm attached to FileSelectedMsg that determines the correct diff to show based on the filetree's current navigation state

The filetree mode is the single source of truth for diff context, with this precedence:
- commitFilesMode: show commit's diff (parent..commit)
- prFilesMode: show PR diff (base..head)
- branchFilesMode: show branch comparison diff (base...HEAD)
- gitFilter active: show working tree diff
- Normal mode: show working tree diff (unstaged, fallback staged)

## Technical Details

### Key files to modify
- `internal/panels/messages.go` - Add DiffContext struct + type enum, extend FileSelectedMsg
- `internal/panels/preview/preview.go` - Mode field, context handling, `v` key, rendering
- `internal/panels/filetree/filetree.go` - Emit DiffContext in emitCursorFileSelected()
- `internal/keymap/schemes/default.toml` - Add `v` binding with `context = "preview"`
- `internal/tui/app.go` - Add `toggle_diff_mode` action routing

### Architecture context
- Preview currently has `gitDiffOnly` bool (set by GitFilterActiveMsg) and `diffLines` (loaded via loadDiffCmd for working-tree only)
- The gitdiff panel handles ref-based diffs via ShowDiffMsg - preview needs similar capability
- Keymap dispatch: Global > Context-specific panel > General panel. Using `context = "preview"` ensures `v` only fires when preview is focused
- `emitCursorFileSelected()` in filetree.go (line 1061) is the single point where file selection originates - this is where DiffContext gets attached

### Current gap
When in commitFilesMode, only FileSelectedMsg{Path} is sent (no commit hash context). The preview loads working-tree diff via loadDiffCmd, NOT the commit's diff. Branch-files mode partially works because it sends ShowDiffMsg to the gitdiff panel, but not to preview.

### Keybinding rationale
- `d` is taken globally as `discard_file` (default.toml line 215-218)
- `v` is completely unbound everywhere - mnemonic for "view toggle"
- Uses keymap `context = "preview"` for isolation - no global conflicts

## Acceptance Criteria

- [ ] `v` toggles preview between file-on-disk and contextual diff mode when preview is focused
- [ ] `d` (lowercase) continues to fire `discard_file` globally - no regression
- [ ] Commit-files mode: selecting a file shows commit's diff (not working tree)
- [ ] Branch-files mode: selecting a file shows branch comparison diff
- [ ] PR-files mode: selecting a file shows PR diff
- [ ] Git-filter mode: selecting a file shows working tree diff, preview auto-enters diff mode
- [ ] Normal mode: selecting a file defaults to working tree diff context
- [ ] Panel title shows `[diff]` suffix when in diff mode
- [ ] Mode persists across file selections (switching files doesn't reset mode)
- [ ] Empty diff in diff mode shows "No changes" centered message
- [ ] File mode: syntax-highlighted file content only, zero diff information shown
- [ ] Diff mode: only contextual diff with +/- coloring, no file content below

## Quality Gates (MANDATORY)

- [ ] Zero lint/type errors (`go build ./...`, `go vet ./...`)
- [ ] All tests pass (existing + new tests for this feature)
- [ ] mq (max-quality) passes clean
- [ ] test-health grade >= B+
- [ ] idiomatic audit clean
- [ ] Architecture review: no violations
- [ ] Gut-check: no over-engineering or YAGNI violations

## Done Definition

This issue is DONE when ALL acceptance criteria AND quality gates pass.
"It compiles" is not done. "Tests pass" is not done. ALL gates must be green.

## Related

- `internal/panels/preview/preview.go` - Primary target
- `internal/panels/filetree/filetree.go` - Context emission source
- `internal/panels/gitdiff/gitdiff.go` - Reference for ref-based diff loading
- `internal/panels/messages.go` - Message types
- `internal/keymap/schemes/default.toml` - Keybindings
