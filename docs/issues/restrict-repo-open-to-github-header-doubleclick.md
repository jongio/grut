# Restrict repo open to "GitHub" header double-click only

## Summary

Double-clicking on any tab within the GitHub or Git panels currently opens the GitHub repo in a browser. The repo should only open when the user double-clicks specifically on the "GitHub" text in the GitHub pane header.

## Description

In the gitinfo panel (`internal/panels/gitinfo/gitinfo.go`), the `handleMouseDoubleClick` method routes all content-area double-clicks through `doAction()`, which resolves the configured double-click action (typically `ActionOpenInBrowser`) and opens URLs in the default browser.

The problem is that double-clicking on tab labels in the tab bar (e.g., "Issues", "PRs", "Actions", "Branches", "Worktrees") triggers unintended browser navigation. The current guard at the top of `handleMouseDoubleClick` only returns early for tab-bar clicks without taking any action — but the layout engine may still be delivering double-click messages that reach the content area logic, or the tab-bar height calculation may not correctly cover all header rows in all modes (`ModeGit`, `ModeGitHub`, `ModeAll`).

The desired behavior is:

- **Double-click on "GitHub" text in the pane header** → open the repo URL in the browser.
- **Double-click on any tab label** (Issues, PRs, Actions, Branches, etc.) → no action (tab switching is handled by single-click already).
- **Double-click on content items** (a specific issue, PR, branch, etc.) → existing behavior (open that item).

## Technical Details

### Key files

| File | Relevance |
|------|-----------|
| `internal/layout/engine.go:355-394` | Double-click detection (500ms threshold), creates `PanelMouseDoubleClickMsg` |
| `internal/panels/messages.go:177-183` | `PanelMouseDoubleClickMsg` struct definition |
| `internal/panels/gitinfo/gitinfo.go:1266-1282` | `handleMouseDoubleClick()` — routes double-clicks, guards tab bar |
| `internal/panels/gitinfo/gitinfo.go:1235-1262` | `handleMouseClick()` — tab switching on single click |
| `internal/panels/gitinfo/gitinfo.go:1629-1657` | `doAction()` — resolves and executes configured double-click action |
| `internal/panels/gitinfo/gitinfo.go:1856-1930` | `executeAction()` — item-specific action dispatch |
| `internal/panels/gitinfo/gitinfo.go:1973-1980` | `openURLAndToast()` — opens URL in browser |
| `internal/panels/gitinfo/gitinfo.go:2697-2800+` | `renderTabBar()` — tab header rendering |
| `internal/panels/open.go:114-126` | `OpenInBrowser()` — cross-platform browser launch |

### Current double-click flow

```
Layout engine detects double-click (engine.go:364-394)
  → PanelMouseDoubleClickMsg sent to panel
  → gitinfo.handleMouseDoubleClick() (gitinfo.go:1266)
    → if ContentRow < tabBarHeight: return (no-op)
    → else: resolve item index, call doAction()
      → doAction() → executeRightClickAction(ActionOpenInBrowser)
        → openURLAndToast() → OpenInBrowser(url)
```

### Proposed change

Add logic so that when the user double-clicks within the pane header area, only a double-click on the "GitHub" label text opens the repo URL. All other header/tab-bar double-clicks remain no-ops. This likely involves:

1. Detecting whether the double-click is on the pane header row that contains "GitHub" text (vs. sub-tab rows like Issues/PRs/etc.).
2. Hit-testing the column position against the rendered "GitHub" label bounds.
3. Calling `openURLAndToast()` with the repo URL only when the hit-test matches.

### Tab bar layout context

In `ModeAll`, the tab bar has two rows:
- Row 0: Git tabs (Branches, Worktrees, Remotes, Stash, Tags, Reflog)
- Row 1: GitHub tabs (Issues, PRs, Actions, Workflows, Releases)

The pane header text ("GitHub" / "Git") may be rendered separately above these rows by the layout engine.

## Acceptance Criteria

- [ ] Double-clicking on the "GitHub" text in the GitHub pane header opens the repo in the browser
- [ ] Double-clicking on tab labels (Issues, PRs, Actions, Branches, etc.) does NOT open the repo
- [ ] Double-clicking on content items (specific issue, PR, branch) retains existing behavior
- [ ] Behavior is correct in all panel modes: `ModeGit`, `ModeGitHub`, `ModeAll`
- [ ] No regressions in single-click tab switching
- [ ] Build passes (`go build ./...`) and tests pass (`go test ./...`)

## Related

- `internal/panels/gitinfo/gitinfo.go` — primary file to modify
- `internal/layout/engine.go` — double-click detection logic (likely no changes needed)
