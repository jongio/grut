# Double-click GitHub pane header to open repo in browser

## Summary

When the user double-clicks on the GitHub pane header text, open the GitHub repository URL in the default browser.

## Description

The GitHub pane (`gitinfo` panel in `ModeGitHub`) displays a header title ("GitHub" or "GitHub (private)"). Currently, double-clicking within the tab bar area of the panel is a no-op — the handler returns `nil`. This feature adds a natural mouse-driven shortcut to open the repository's GitHub page directly from the pane header.

This is a small quality-of-life improvement that makes the GitHub pane header interactive and gives users quick browser access to the repo without needing a keyboard shortcut.

## Technical Details

### Current behavior

In `internal/panels/gitinfo/gitinfo.go`, `handleMouseDoubleClick()` checks if the click is in the tab bar region (`msg.ContentRow < tbh`) and returns early with no action:

```go
if msg.ContentRow < tbh {
    return p, nil
}
```

### Proposed change

When a double-click lands on the header/title area (row 0, which is the panel title rendered by the layout border), detect this and open the repo URL in the browser using the existing `OpenInBrowser()` utility.

**Note**: The panel title is rendered by the layout engine in the border, not inside the panel's content area. The implementation needs to account for how the layout dispatches mouse events — the double-click on the border/title may need to be handled at the layout level, or the panel needs a way to receive header-area clicks.

### Key code references

- **Panel title**: `internal/panels/gitinfo/gitinfo.go` — `Title()` method (lines ~864-875)
- **Double-click handler**: `internal/panels/gitinfo/gitinfo.go` — `handleMouseDoubleClick()` (lines ~1264-1282)
- **Mouse dispatch**: `internal/panels/messages.go` — `PanelMouseDoubleClickMsg` (lines ~169-191)
- **Remote URL access**: `internal/panels/gitinfo/gitinfo.go` — `guessBranchRemoteURL()` (lines ~1984-1989)
- **SSH-to-HTTPS conversion**: `internal/git/url.go` — `RemoteToHTTPS()` (lines ~8-34)
- **Browser open utility**: `internal/panels/open.go` — `OpenInBrowser()` (lines ~113-126)
- **Layout border/header rendering**: `internal/layout/` — where panel titles are drawn into borders

### Implementation approach

1. Determine how header double-clicks are dispatched (layout-level vs panel-level).
2. When a double-click on the GitHub pane header is detected, resolve the repo URL from `p.lastRemotes[0].FetchURL` via `RemoteToHTTPS()`.
3. Call `OpenInBrowser(url)` and emit an info toast ("Opened repository in browser").
4. If no remote URL is available, emit a warning toast.

## Acceptance Criteria

- [ ] Double-clicking the GitHub pane header text opens the repo in the default browser
- [ ] Works with HTTPS, SSH, and `git@` style remote URLs (via existing `RemoteToHTTPS`)
- [ ] Shows info toast on success ("Opened repository in browser")
- [ ] Shows warning toast if no remote URL is available
- [ ] No change to existing double-click behavior in the content area or tab bar

## Related

- `internal/panels/gitinfo/gitinfo.go` — main panel implementation
- `internal/panels/open.go` — browser open utility
- `internal/git/url.go` — remote URL conversion
- `internal/layout/` — border/header rendering and mouse dispatch
