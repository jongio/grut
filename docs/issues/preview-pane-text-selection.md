# Allow user to select text in the preview pane

## Summary

The preview pane currently provides no way to select text. Users should be able to click-and-drag (or use keyboard shortcuts) to select text in the preview pane and copy it to the clipboard.

## Description

When previewing files in grut, users often want to copy snippets — a function signature, an error message, a config value. Today there is no way to do this: the terminal's native text selection is captured by Bubble Tea's mouse handling, and the preview panel itself has no selection mechanism.

This feature adds text selection to the preview pane so users can highlight a range of text and copy it. This is a foundational capability that could later be extended to other text-heavy panels (diff, log, blame).

## Technical Details

### Current State

**Preview panel** (`internal/panels/preview/preview.go`):
- Model struct (lines 30-64): No selection state fields.
- Mouse handling (lines 301-309): Only `tea.MouseWheelMsg` for scrolling. No click, drag, or release handling.
- Rendering (`renderContent`, lines 812-899): Manual line slicing with `scrollY` offset, line-by-line rendering via `ansi.Truncate()`. No highlight/selection styling.
- No viewport library — hand-rolled scroll with `scrollY`, `scrollUp(n)`, `scrollDown(n)`.

**Layout engine** (`internal/layout/engine.go`):
- Routes `tea.MouseClickMsg` → panels as `PanelMouseClickMsg` with content-relative coordinates (lines 293-411).
- Routes `tea.MouseMotionMsg` and `tea.MouseReleaseMsg` only for split-resize drag (lines 228-238). When not dragging, calls `updateFocused(msg)` which does forward to the focused panel — but panels don't handle these messages today.
- Double-click detection exists (500ms threshold, lines 288-291).

**Clipboard** (`internal/panels/clipboard.go`, lines 19-37):
- `CopyToClipboard(ctx, text)` already works cross-platform (clip/pbcopy/xclip).
- `StripANSI(s)` strips escape sequences before copying.
- Used by many panels for copying paths, hashes, names — but never for arbitrary text ranges.

### Proposed Approach

#### 1. Selection State in Preview

Add selection tracking fields to the `Preview` struct:

```go
type selectionPoint struct {
    line int // absolute line index in p.lines
    col  int // character (rune) offset within the line
}

type Preview struct {
    // ... existing fields ...
    selStart    *selectionPoint // anchor (where click began)
    selEnd      *selectionPoint // cursor (where drag/shift-move reached)
    selecting   bool            // true while mouse button is held
}
```

#### 2. Mouse Event Handling

Handle these messages in `Preview.Update()`:

| Message | Action |
|---------|--------|
| `PanelMouseClickMsg` (left) | Clear existing selection; set `selStart` at click position; set `selecting = true` |
| `tea.MouseMotionMsg` | If `selecting`, update `selEnd` to current position |
| `tea.MouseReleaseMsg` | Set `selecting = false`; finalize selection |
| `PanelMouseDoubleClickMsg` | Select word under cursor |

Coordinate mapping: convert `ContentRow` → absolute line index via `p.scrollY + contentRow`, then map `ContentCol` → rune offset accounting for line numbers, tab expansion, and ANSI sequences.

#### 3. Layout Engine — Motion Forwarding

`tea.MouseMotionMsg` and `tea.MouseReleaseMsg` are already forwarded to the focused panel via `updateFocused(msg)` when the layout engine is not in its own split-resize drag. However, the messages arrive in raw terminal coordinates — panels would need content-relative coordinates (like `PanelMouseClickMsg` provides `ContentRow`/`ContentCol`).

Options:
- **Option A**: Create `PanelMouseMotionMsg` and `PanelMouseReleaseMsg` panel message types (consistent with existing `PanelMouseClickMsg` pattern) and have the engine translate coordinates before forwarding.
- **Option B**: Let the preview panel do its own coordinate translation from raw `tea.MouseMotionMsg`. Simpler but less consistent.

Option A is recommended for consistency with the existing pattern.

#### 4. Visual Highlighting

In `renderContent()`, after building each visible line, check if any portion falls within the selection range. If so, wrap the selected substring in a lipgloss style with a distinct background color (e.g., the theme's selection color).

```go
selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#44475A"))
```

Must handle ANSI sequences carefully — selection operates on display columns, not raw string bytes.

#### 5. Copy to Clipboard

Add a keybinding (e.g., `y` or `Ctrl+C` when selection is active) that:
1. Extracts selected text from `p.lines[selStart.line..selEnd.line]`.
2. Strips ANSI via `panels.StripANSI()`.
3. Calls `panels.CopyToClipboard()`.
4. Shows a toast: "Copied N lines to clipboard".
5. Clears the selection.

### Affected Files

| File | Change |
|------|--------|
| `internal/panels/preview/preview.go` | Add selection state, mouse handlers, highlight rendering, copy command |
| `internal/panels/messages.go` | Add `PanelMouseMotionMsg` and `PanelMouseReleaseMsg` types (if Option A) |
| `internal/layout/engine.go` | Translate coordinates and forward motion/release as panel messages (if Option A) |
| `internal/panels/preview/preview_test.go` | Tests for selection logic, coordinate mapping, copy |

### Edge Cases

- **Word wrap**: Selection coordinates must account for soft-wrapped lines.
- **Line numbers**: Column offset must subtract the line number gutter width.
- **Syntax highlighting**: ANSI sequences must be stripped for column calculations and clipboard content.
- **Blame mode**: Selection should work in blame view too (or be disabled with a toast).
- **Binary/large files**: Selection should be disabled when `isBinary` or `isLarge` is true.
- **Scroll during drag**: If the user drags past the top/bottom edge, auto-scroll the viewport.

## Acceptance Criteria

- [ ] User can click-and-drag in the preview pane to select a range of text.
- [ ] Selected text is visually highlighted with a distinct background color.
- [ ] User can copy the selection to the clipboard (via keybinding).
- [ ] A toast confirms the copy with the number of lines/characters copied.
- [ ] Selection works correctly with line numbers enabled/disabled.
- [ ] Selection works correctly with word wrap enabled/disabled.
- [ ] Double-click selects the word under the cursor.
- [ ] Pressing Escape or clicking elsewhere clears the selection.
- [ ] Mouse wheel scrolling still works during/after selection.
- [ ] Existing preview panel behavior (scroll, blame, markdown) is unaffected.

## Related

- `internal/panels/preview/preview.go` — Preview panel implementation
- `internal/panels/clipboard.go` — Existing clipboard utility
- `internal/layout/engine.go:215-248` — Mouse event routing
- `internal/panels/messages.go:173-203` — Panel mouse message types
- `internal/panels/preview/preview_test.go` — Existing tests
