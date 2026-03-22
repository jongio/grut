# Confirm Merge Yes/No Buttons Not Mouse-Clickable

## Summary

The Yes/No buttons in the "Confirm Merge" modal dialog do not respond to mouse clicks, despite mouse click handling code existing in the codebase.

## Description

When merging a PR from the PRs tab, the "Confirm Merge" dialog appears with Yes/No buttons. These buttons can only be activated via keyboard (left/right arrows + enter). Clicking them with the mouse does nothing.

Mouse click handling infrastructure exists in `internal/notify/modal.go` (`handleMouseClick` and `clickConfirmButton`), and the routing is wired in `internal/tui/app.go` (line ~490). However, the click coordinate calculation may be incorrect for longer messages that wrap across multiple lines in the dialog.

The existing tests (`TestModalConfirmMouseClickYes`, `TestModalConfirmMouseClickNo`) only cover short single-line messages like "Are you sure?". The actual merge confirm message is much longer:

```
Merge PR #35 "feat: gitinfo enhancements, contributor recognition, and deadcode cleanup" using squash and merge?
```

This wraps across 3-4 lines in the modal's 46-character content width, which shifts the button row position. The `headerLines` calculation (`titleH + 1 + msgH + 1`) should account for this via `lipgloss.Height()`, but there may be a mismatch between the rendered layout and the coordinate math.

## Technical Details

### Relevant Files

- `internal/notify/modal.go` — `handleMouseClick()` (line 295), `clickConfirmButton()` (line 460), `renderConfirmButtons()` (line 575)
- `internal/notify/notify.go` — `updateModalMouseClick()` (line 186), mouse event routing (line 166)
- `internal/tui/app.go` — `tea.MouseClickMsg` routing to notify manager (line 490)
- `internal/notify/notify_test.go` — Existing mouse click tests (line 1190)
- `internal/panels/gitinfo/gitinfo.go` — Merge confirm trigger (line 2482)

### Coordinate Calculation Flow

1. `app.go` receives `tea.MouseClickMsg` with terminal-absolute `X, Y`
2. Routes to `notify.Update()` → `updateModalMouseClick()` → `handleMouseClick()`
3. `handleMouseClick()` computes box position: `padLeft = (screenWidth - bw) / 2`, `padTop = (screenHeight - bh) / 2`
4. Converts to relative: `relY = mouseY - padTop - 2`, `relX = mouseX - padLeft - 3`
5. For `ModalConfirm`: checks `relY == headerLines` to detect button row click
6. `headerLines = titleH + 1 + msgH + 1` where `msgH = lipgloss.Height(msgStyle.Render(ms.message))`

### Suspected Root Cause

The `headerLines` calculation or the `padTop` calculation may not account correctly for multi-line wrapped messages, causing `relY == headerLines` to never match the actual button row position when the user clicks it.

## Steps to Reproduce

1. Open grut and navigate to the PRs tab
2. Select a PR with a long title (e.g., "feat: gitinfo enhancements, contributor recognition, and deadcode cleanup")
3. Initiate merge (press enter or use merge action)
4. Select a merge strategy (e.g., squash and merge)
5. In the "Confirm Merge" dialog, try clicking "Yes" or "No" with the mouse
6. **Expected**: Button activates and merge proceeds/cancels
7. **Actual**: Nothing happens; must use keyboard

## Acceptance Criteria

- [ ] Yes/No buttons in ModalConfirm dialogs respond to mouse left-click
- [ ] Works correctly with multi-line wrapped messages (long PR titles)
- [ ] Add test coverage for mouse click on confirm dialog with long wrapped messages
- [ ] Existing keyboard navigation continues to work

## Related

- Existing mouse click tests: `TestModalConfirmMouseClickYes`, `TestModalConfirmMouseClickNo` in `internal/notify/notify_test.go`
- Mouse support enabled via `tea.MouseModeCellMotion` in `internal/tui/app.go:1339`
