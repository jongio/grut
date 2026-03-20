# Add PR Merge from PRs Tab

## Summary

Add the ability to merge a pull request directly from the PRs tab in the GitHub pane, with full support for all merge strategies (merge commit, squash, rebase) and branch cleanup options.

## Description

The PRs tab in the GitHub pane currently supports browsing, filtering, checking out, and opening PRs in the browser — but the most critical PR lifecycle action, **merging**, requires leaving the TUI to visit GitHub's web UI. This is a significant workflow gap for users who want to stay in the terminal.

The backend infrastructure already exists: `MergePR()` is fully implemented in `internal/github/client.go` using the `go-github` v68 SDK, and the `PRWriter` interface in `internal/github/interfaces.go` already declares the method. What's missing is the TUI surface: action registration, keybinding, a merge strategy picker, confirmation flow, and post-merge state updates.

This feature should feel native to grüt's existing PR interaction model: select a PR, press a key (or use context menu), pick a merge strategy, confirm, done — all without leaving the terminal.

## Technical Details

### Existing Infrastructure (Already Implemented)

**GitHub client** (`internal/github/client.go:361-369`):
```go
func (c *clientImpl) MergePR(ctx context.Context, owner, repo string, number int, msg string, opts *gh.PullRequestOptions) error {
    _, _, err := c.gh.PullRequests.Merge(ctx, owner, repo, number, msg, opts)
    // ... cache invalidation
}
```

**Interface** (`internal/github/interfaces.go:39`):
```go
type PRWriter interface {
    MergePR(ctx context.Context, owner, repo string, number int, msg string, opts *gh.PullRequestOptions) error
}
```

**PR data model** (`internal/panels/gitinfo/gitinfo.go:135-143`):
```go
type ghPRItem struct {
    Number     int
    Title      string
    State      string    // "open", "closed", "merged", "draft"
    HeadBranch string
    Author     string
    HTMLURL    string
}
```

### Integration Points (Work Required)

1. **Action Registry** (`internal/actions/registry.go`):
   - Add `ActionMergePR` action ID
   - Register it in the `ItemPR` action set as a right-click/alternative action
   - Assign a keybinding (suggest `m` for merge)

2. **Merge Strategy Picker UI**:
   - A modal/overlay presenting three merge strategies:
     - **Merge commit** — preserves all commits, creates a merge commit
     - **Squash and merge** — combines all commits into one
     - **Rebase and merge** — replays commits onto the base branch
   - Optional: checkbox for "Delete branch after merge"
   - Use existing modal/confirmation patterns from the codebase (e.g., the checkout confirmation flow)

3. **Merge Handler** (`internal/panels/gitinfo/gitinfo.go`):
   - Add `ActionMergePR` case in `executeRightClickAction()` (around line 1937)
   - Only enabled for open (non-draft) PRs
   - Flow: select PR → trigger merge action → show strategy picker → confirm → call `MergePR()` → update local state → show notification

4. **Post-Merge State Updates**:
   - Update the PR's `State` to `"merged"` in the local list
   - Invalidate cached PR data (already handled by `MergePR()`)
   - Show success notification with PR number and merge strategy used
   - Optionally refresh the PR list

5. **Error Handling**:
   - Merge conflicts → clear error message: "PR #N has merge conflicts — resolve on GitHub"
   - Required reviews not met → "PR #N requires approving reviews before merge"
   - Branch protection violations → show the specific protection rule that blocked
   - Network errors → standard retry/error notification

6. **Guard Rails**:
   - Disable merge action for draft PRs (show "Convert to ready first")
   - Disable merge action for already-merged or closed PRs
   - Confirmation modal is mandatory — no single-keystroke merge
   - Consider requiring the PR to be the currently checked-out branch (optional safety)

### Message Types Needed (`internal/panels/messages.go`)

```go
// PRMergeRequestedMsg triggers the merge strategy picker
type PRMergeRequestedMsg struct {
    Number     int
    Title      string
    HeadBranch string
}

// PRMergedMsg reports successful merge
type PRMergedMsg struct {
    Number   int
    Strategy string // "merge", "squash", "rebase"
}

// PRMergeFailedMsg reports merge failure
type PRMergeFailedMsg struct {
    Number int
    Err    error
}
```

### go-github Merge Options

The `go-github` `PullRequestOptions` struct supports:
- `MergeMethod` — `"merge"`, `"squash"`, or `"rebase"`
- `CommitTitle` — custom commit title (for squash/merge commit)
- `CommitMessage` — custom commit message
- `SHA` — expected HEAD SHA (for conflict detection)
- `DontDefaultIfBlank` — control default commit message behavior

## UX Design

### Merge Flow

```
PRs Tab → Select open PR → Press 'm' (or right-click → Merge)
    ↓
┌─────────────────────────────────┐
│  Merge PR #42                   │
│  "Add user authentication"      │
│                                 │
│  ▸ Merge commit                 │
│    Squash and merge             │
│    Rebase and merge             │
│                                 │
│  ○ Delete branch after merge    │
│                                 │
│  [Enter] Merge  [Esc] Cancel    │
└─────────────────────────────────┘
    ↓
Confirmation: "Merge PR #42 using squash?"
    ↓
✓ PR #42 merged (squash)
```

### Context Menu Addition

Current context menu for PRs:
```
  Open in browser
  Copy URL
  Copy PR number
  Checkout branch
```

After this feature:
```
  Open in browser
  Merge PR          ← NEW
  Checkout branch
  Copy URL
  Copy PR number
```

### State-Dependent Behavior

| PR State | Merge Action |
|----------|-------------|
| Open | Enabled — show merge strategy picker |
| Draft | Disabled — tooltip: "Convert to ready for review first" |
| Merged | Disabled — tooltip: "Already merged" |
| Closed | Disabled — tooltip: "PR is closed" |

## Documentation Updates

### Files to Update

1. **`web/src/pages/docs/github-pull-requests.astro`** — Add merge feature to PR documentation page
2. **`docs/keybindings.md`** — Add `m` keybinding for merge in PR context
3. **`README.md`** — Update GitHub section to mention merge capability
4. **`ROADMAP.md`** — Move merge from implicit gap to "Implemented" or remove from "Coming Next"
5. **`web/` screenshots** — Capture new screenshots showing:
   - Merge option in context menu
   - Merge strategy picker modal
   - Success notification after merge

### Web Documentation Content

Add a new section to the PR docs page:

```markdown
### Merge Pull Requests

Merge PRs directly from the terminal without switching to the browser.

- Press `m` on any open PR to start the merge flow
- Choose your merge strategy: merge commit, squash, or rebase
- Optionally delete the source branch after merge
- Works with all GitHub branch protection rules — if merge is blocked,
  grüt shows the specific reason
```

## Acceptance Criteria

- [ ] `m` keybinding triggers merge flow on selected open PR
- [ ] Right-click context menu includes "Merge PR" option
- [ ] Merge strategy picker modal shows all three options (merge, squash, rebase)
- [ ] "Delete branch after merge" toggle available in picker
- [ ] Confirmation step before executing merge (no accidental merges)
- [ ] Successful merge shows notification with PR number and strategy
- [ ] Merge errors show clear, actionable messages (conflicts, reviews needed, protection rules)
- [ ] Draft/merged/closed PRs have merge action disabled with appropriate tooltip
- [ ] PR list updates state to "merged" after successful merge
- [ ] Cache invalidation works correctly (PR list refreshes)
- [ ] `web/src/pages/docs/github-pull-requests.astro` updated with merge docs
- [ ] `docs/keybindings.md` updated with `m` keybinding
- [ ] `README.md` updated to mention merge capability
- [ ] `ROADMAP.md` updated
- [ ] New screenshots captured for web docs
- [ ] All existing tests pass
- [ ] New tests cover merge action registration, strategy selection, error handling

## Related

- **Existing impl**: `internal/github/client.go` — `MergePR()` method (line 361)
- **Interface**: `internal/github/interfaces.go` — `PRWriter` interface (line 39)
- **Action registry**: `internal/actions/registry.go` — `ItemPR` actions
- **PR handler**: `internal/panels/gitinfo/gitinfo.go` — `executeRightClickAction()` (line ~1937)
- **PR messages**: `internal/panels/messages.go` — PR message types
- **Web docs**: `web/src/pages/docs/github-pull-requests.astro`
- **Keybindings doc**: `docs/keybindings.md`
