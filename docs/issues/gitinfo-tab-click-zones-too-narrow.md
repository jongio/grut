# Gitinfo tab click zones don't cover full tab label + count

## Summary

The mouse click target area for tabs in the git and GitHub panes does not span the full visual width of each tab label (name + count/status), requiring precise clicking to switch tabs.

## Description

When clicking tabs in the gitinfo panel (both the git row and the GitHub row), the clickable area is smaller than the rendered text. Users must carefully position their click within a narrow zone instead of being able to click anywhere on the tab name or its trailing count/status indicator. This is a usability regression — the entire visual area of a tab (name + space + count) should be a click target.

Two root causes have been identified:

### 1. `ghTabLabelWidth` uses `len()` (byte count) instead of display width

`ghTabLabelWidth` calculates tab width using `len(fmt.Sprintf("%s %s", name, count))`, which counts **bytes** not **display columns**. For ASCII-only strings these are equal, but the Actions tab count uses multi-byte Unicode characters (`✓`, `✗`, `●`, `◐`, `○`, `◑`) where byte count ≠ display width:

| Character | Bytes (UTF-8) | Display Columns |
|-----------|--------------|-----------------|
| `✓` (U+2713) | 3 | 1 |
| `✗` (U+2717) | 3 | 1 |
| `●` (U+25CF) | 3 | 1 |
| `◐` (U+25D0) | 3 | 1 |
| `○` (U+25CB) | 3 | 1 |
| `◑` (U+25D1) | 3 | 1 |

This means for `Actions ✓`, the click zone width is calculated as 10 bytes but the rendered width is only 9 display columns. The Actions tab click zone extends 1 column past its visual boundary, and every tab **after** Actions (Workflows, Releases) has its click zone shifted 1-2 columns to the right of where it visually appears.

### 2. `tabRowUseShort` has the same `len()` issue

The abbreviation-mode detection function `tabRowUseShort` also uses `len()` for width calculation, which can cause incorrect abbreviation decisions when Unicode characters are present in tab counts. While both render and click paths currently use the same incorrect calculation, neither agrees with the actual terminal display.

## Technical Details

### Affected files

- `internal/panels/gitinfo/gitinfo.go`

### Key functions

**Click zone calculation** — `ghTabLabelWidth` (line ~3055):
```go
func (p *Panel) ghTabLabelWidth(name, short, count string, useShort bool) int {
    if useShort && short != "" {
        name = short
    }
    return len(fmt.Sprintf("%s %s", name, count))  // BUG: len() counts bytes, not display columns
}
```

**Git tab click handler** — `handleTabBarClick` (line ~1361):
```go
pos := 1 // leading space
for i, t := range tabs {
    w := p.ghTabLabelWidth(t.name, t.short, t.count, useShort)
    end := pos + w
    if col >= pos && col < end { ... }
    if i < len(tabs)-1 {
        pos = end + 3 // " · " separator
    }
}
```

**GitHub tab click handler** — `handleGitHubTabBarClick` (line ~3003): Same algorithm.

**Abbreviation detection** — `tabRowUseShort` (line ~3064):
```go
fullWidth += len(t.name) + 1 + len(t.count) // BUG: same len() issue
```

**Rendering** — `renderRow` inside `renderTabBar` (line ~2695):
```go
fullWidth += len(t.name) + 1 + len(t.count) // Same len() issue in render path
// But actual display width is determined by lipgloss/terminal correctly
```

### Unicode status icons source

- `actionsStatusIcon()` (line ~2776) returns `✓`, `✗`, `●`, or animated `watchFrames` (`●◐○◑`)
- These are the primary source of multi-byte characters in tab counts

### Coordinate flow

1. `engine.go:handleMouseClick` converts terminal coords → `contentCol = innerX - r.X - PanelPadH`
2. `gitinfo.go:handleMouseClick` routes to `handleTabBarClick(msg.ContentCol)` or `handleGitHubTabBarClick(msg.ContentCol)`
3. Click handlers iterate tabs accumulating `pos` by `ghTabLabelWidth` + 3 (separator)

## Acceptance Criteria

- [ ] Clicking anywhere on a tab's rendered name activates that tab
- [ ] Clicking on the count/status indicator after the tab name activates that tab
- [ ] Click zones remain correct when Actions tab shows Unicode status icons (✓, ✗, ●, ◐, ○, ◑)
- [ ] Click zones remain correct when issue/PR filters show text labels (Assigned, Mentioned, Needs Review, etc.)
- [ ] Click zones remain correct in abbreviated mode (short names)
- [ ] All existing `handleTabBarClick` / `handleGitHubTabBarClick` tests still pass

## Suggested Fix

Replace `len()` with `lipgloss.Width()` (or `runewidth.StringWidth()` from `github.com/mattn/go-runewidth`) in:

1. `ghTabLabelWidth` — use display width instead of byte count
2. `tabRowUseShort` — use display width for abbreviation calculation
3. `renderRow` in `renderTabBar` — use display width for the `useShort` decision

Example fix for `ghTabLabelWidth`:
```go
func (p *Panel) ghTabLabelWidth(name, short, count string, useShort bool) int {
    if useShort && short != "" {
        name = short
    }
    return lipgloss.Width(fmt.Sprintf("%s %s", name, count))
}
```

## Related

- `internal/panels/gitinfo/gitinfo.go` — all affected code is in this file
- `internal/layout/engine.go` — coordinate translation (PanelPadH = 1)
- `internal/panels/messages.go` — PanelMouseClickMsg definition
