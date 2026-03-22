# x-key delete fails after first use

## Summary

Pressing `x` to delete an item works the first time, but subsequent delete attempts either fail silently or don't trigger at all. Observed in branches and worktrees panels; may affect other panels too.

## Description

When a user presses `x` to delete an item (e.g., a branch or worktree), the first deletion succeeds — the confirmation modal appears, the item is removed, and the list refreshes. However, pressing `x` again to delete another item either does nothing (no modal, no feedback) or inconsistently fails to trigger the delete flow.

This suggests that some state is not being properly reset after the first deletion completes, preventing subsequent `ItemDeleteMsg` messages from being handled correctly.

## Technical Details

### Affected Panels (confirmed)

- **branches** (`internal/panels/branches/branches.go`) — `requestDelete()` / `handleOpResult()`
- **gitinfo/worktrees** (`internal/panels/gitinfo/gitinfo.go`) — `doDelete()` / `handleModalResult()`

### Potentially affected panels

- **filetree** (`internal/panels/filetree/fileops.go`) — `requestDelete()`
- **stash** (`internal/panels/stash/stash.go`) — `requestDrop()`

### Key binding

All five panels bind `x` → `item_delete` action in `internal/keymap/schemes/default.toml`.

### Message flow

```
x pressed → keymap dispatches "item_delete" → app.go handleAction() → ItemDeleteMsg
→ focused panel Update() → requestDelete()/doDelete()
→ confirmation modal → handleModalResult() → git operation
→ loadData()/loadBranches() → buildItems() → UI refresh
```

### Suspected root causes

1. **Pending operation not cleared**: Panels track `p.pending` (e.g., `opDelete`) to correlate the modal confirmation with the operation. If `p.pending` is not reset to its zero value after completion, subsequent delete requests may be silently ignored or short-circuited.

2. **Focus state lost after modal**: The confirmation modal may not correctly restore panel focus after dismissal. If `p.Focused` becomes `false`, the `ItemDeleteMsg` guard (`if !p.Focused { return p, nil }`) silently drops the message.

3. **Cursor lands on non-selectable item**: After `buildItems()` rebuilds the list, the cursor may land on a header row or an out-of-bounds index. `selectedBranch()` and similar functions return `nil` in these cases, causing `requestDelete()` to no-op.

4. **Modal result handler mismatch**: If `handleModalResult()` doesn't match the pending operation (due to stale `p.pending` state), the confirmation result may be silently discarded, leaving the panel in a broken state for future operations.

### Relevant code paths

- `branches.go`: `requestDelete()` sets `p.pending = opDelete` — check if cleared after `handleOpResult()`
- `gitinfo.go`: `doDelete()` sets `p.pending = opBranchDelete` / `opWorktreeRemove` — check if cleared after `handleModalResult()`
- `gitinfo.go`: `doBuildItems()` resets all `tabCursor` values to 0, which may move cursor to a header
- `branches.go`: `buildItems()` resets `p.cursor = 0` then seeks first non-header — verify this works after deletion

## Steps to Reproduce

1. Open grut in a repository with multiple local branches (or worktrees)
2. Navigate to the branches panel (or gitinfo panel, worktrees tab)
3. Select a branch/worktree and press `x`
4. Confirm deletion in the modal → item is deleted (works correctly)
5. Select another branch/worktree and press `x` again
6. **Expected**: Confirmation modal appears for the second item
7. **Actual**: Nothing happens — no modal, no feedback. Sometimes works inconsistently.

## Acceptance Criteria

- [ ] Pressing `x` to delete works reliably on consecutive items without requiring panel switch or navigation reset
- [ ] `p.pending` state is explicitly cleared after every operation completes (success or failure)
- [ ] Panel focus is correctly restored after modal dismissal
- [ ] Cursor position after deletion lands on a valid, selectable item
- [ ] Verified in all panels that bind `item_delete`: branches, gitinfo, filetree, stash

## Related

- Key binding definitions: `internal/keymap/schemes/default.toml` (lines 252–429)
- Action dispatch: `internal/tui/app.go` (`handleAction`)
- Message type: `internal/panels/messages.go` (`ItemDeleteMsg`)
- Modal system: `internal/panels/notify/` package
