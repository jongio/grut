# Smart Contextual Commits Pane

## Summary

The commits pane should dynamically filter its content based on what the user selects in other panes — files, folders, branches, worktrees, remotes, stashes — and selecting a specific commit should update the files pane to show the files changed in that commit.

## Description

Currently the commits pane operates in a single mode: it shows commits for the current HEAD or a branch after checkout (`BranchChangedMsg`). There is no contextual filtering — clicking a file, folder, worktree, remote, or stash in their respective panes has no effect on what commits are displayed.

The goal is to make the commits pane "ultra smart" so it reacts to selection events across all sibling panes, showing contextually relevant commits:

| User Selects | Commits Pane Shows |
|---|---|
| A file in filetree | All commits that touched that file (`git log -- <path>`) |
| A folder in filetree | All commits that touched any file in that folder (`git log -- <dir>`) |
| A branch in gitinfo | All commits on that branch (`git log <branch>`) |
| A worktree in gitinfo | All commits on the worktree's branch |
| A remote branch in gitinfo | All commits on that remote branch (`git log <remote>/<branch>`) |
| A stash entry in gitinfo | The commit(s) that make up that stash (`git stash show -p` context) |
| An individual commit in commits | Files pane updates to show files changed in that commit |

Additionally, the commits pane header/label should reflect the active filter context (e.g., "Commits: src/main.go", "Commits: feature/auth", "Commits: origin/main").

## Technical Details

### Architecture (message-driven, broadcast model)

All inter-panel communication flows through the layout engine's broadcast system (`internal/layout/engine.go`). Input events route to the focused panel only; all other messages broadcast to every panel in the active tab. This means the commits pane just needs to handle new message types — no architectural changes required.

### Existing Infrastructure

- **`LogOpts.Path` field exists** in `internal/git/types.go` but is never used by the commits pane's `loadCommitsCmd()`. Wiring this up enables file/folder filtering immediately.
- **`BranchChangedMsg`** already triggers commit reload for the checked-out branch. This handles post-checkout scenarios.
- **`BranchSelectedMsg`** exists (emitted by gitinfo on cursor movement in branches tab) but the commits pane ignores it. Handling this would show commits for the highlighted branch without requiring checkout.
- **`FileSelectedMsg`** exists (emitted by filetree on file selection) but the commits pane ignores it.
- **`CommitSelectedMsg`** exists (emitted by commits pane) but the files pane ignores it.

### New Messages Needed

- **`FolderSelectedMsg{Path string}`** — emitted by filetree when a directory is selected (currently `emitCursorFileSelected()` skips directories with `if n.isDir { return nil }`).
- **`WorktreeSelectedMsg{Path, Branch string}`** — emitted by gitinfo worktrees tab on cursor selection (currently only `SwitchWorktreeMsg` exists for actual switching).
- **`RemoteSelectedMsg{Name, Branch string}`** — emitted by gitinfo remotes tab on cursor selection (currently no selection message for remotes).
- **`StashSelectedMsg{Index int, Hash string}`** — emitted by gitinfo stash tab on cursor selection (currently no selection message for stash entries).

### Key Files to Modify

| File | Change |
|---|---|
| `internal/panels/messages.go` | Add `FolderSelectedMsg`, `WorktreeSelectedMsg`, `RemoteSelectedMsg`, `StashSelectedMsg` |
| `internal/panels/commits/commits.go` | Handle all new selection messages + `BranchSelectedMsg` + `FileSelectedMsg`; use `LogOpts.Path` for file/folder filtering; update `refLabel` for header context |
| `internal/panels/filetree/filetree.go` | Emit `FolderSelectedMsg` when directory is selected (remove the `isDir` early return in `emitCursorFileSelected`) |
| `internal/panels/gitinfo/gitinfo.go` | Emit `WorktreeSelectedMsg`, `RemoteSelectedMsg`, `StashSelectedMsg` on cursor movement in respective tabs |
| `internal/git/log.go` | Ensure `LogOpts.Path` is passed as `-- <path>` argument to `git log` |
| `internal/panels/filetree/filetree.go` | Handle `CommitSelectedMsg` — load and display files changed in the selected commit (`git diff-tree --no-commit-id --name-only -r <hash>`) |

### Commit → Files Flow

When a user clicks an individual commit, the files pane should show the files changed in that commit. This requires:

1. Commits pane emits `CommitSelectedMsg{Hash}` on click/Enter (already happens).
2. Files pane handles `CommitSelectedMsg` — runs `git diff-tree --no-commit-id --name-only -r <hash>` to get changed file paths.
3. Files pane switches to a "commit files" view mode showing only those files (similar to how `GitChangedFilesMsg` filters to changed files).
4. Selecting a file in this filtered view could show the diff for that file in that commit via `ShowDiffMsg`.

### State Management

The commits pane needs a `filterContext` field to track what's currently filtering:

```go
type filterKind int

const (
    filterNone filterKind = iota
    filterFile
    filterFolder
    filterBranch
    filterWorktree
    filterRemote
    filterStash
)

// Added to Panel struct:
filterKind  filterKind
filterPath  string // for file/folder
filterRef   string // for branch/worktree/remote
filterLabel string // display label for header
```

When a new selection arrives, the pane updates these fields, resets pagination, and reloads with the appropriate `LogOpts`.

### Clear Filter

There should be a way to clear the filter and return to the default view (all commits on current branch). Options:
- Escape key when commits pane is focused
- A "clear filter" indicator in the header that can be clicked
- Selecting the same item again toggles the filter off

### Panel Title Updates

Both the commits and files panel titles (shown in the border) must dynamically reflect what is currently being displayed:

**Commits panel** (`Title()` in `commits.go` lines 206-214):
- Already returns `"Commits: <refLabel>"` — extend to show file/folder/stash context too
- Examples: `"Commits: src/main.go"`, `"Commits: internal/panels/"`, `"Commits: origin/main"`, `"Commits: stash@{0}"`

**Files panel** (`Title()` in `filetree.go` lines 332-342):
- Currently returns `"Files [tree]"` or `"Files [list]"` or `"Files [tree] (git changed)"`
- **Remove the `[tree]`/`[list]` bracketed mode indicator** — it adds noise and isn't useful
- When showing files changed in a commit, update to `"Files: <short-hash> <subject>"` (e.g., `"Files: abc1234 Fix auth bug"`)
- When returning to normal mode, revert to `"Files"` (or `"Files (git changed)"` if git filter active)

Title rendering happens in `internal/tui/app.go` — titles are automatically truncated with `...` if they exceed available width, so long titles are safe.

## Acceptance Criteria

- [ ] Clicking a file in filetree shows commits that touched that file in the commits pane
- [ ] Clicking a folder in filetree shows commits that touched files in that folder
- [ ] Selecting (not checking out) a branch shows commits on that branch
- [ ] Selecting a worktree shows commits on the worktree's branch
- [ ] Selecting a remote branch shows commits on that remote branch
- [ ] Selecting a stash entry shows relevant commit context
- [ ] Clicking an individual commit updates the files pane to show files changed in that commit
- [ ] Commits pane title dynamically reflects the active filter context (file path, branch name, etc.)
- [ ] Files pane title updates to show commit info when displaying commit-changed files
- [ ] Files pane title no longer shows `[tree]` or `[list]` in brackets
- [ ] Filter can be cleared to return to default view
- [ ] Pagination still works when filtering by path
- [ ] Existing keyboard navigation and search work with filtered views
- [ ] No regressions in current commits pane behavior

## Related

- `internal/panels/commits/commits.go` — main commits panel (773 lines)
- `internal/panels/messages.go` — all inter-panel message types
- `internal/git/log.go` — git log command builder
- `internal/git/types.go` — `Commit` and `LogOpts` structs
- `internal/panels/filetree/filetree.go` — file tree panel
- `internal/panels/gitinfo/gitinfo.go` — branches/worktrees/remotes/stash tabs
- `internal/layout/engine.go` — message broadcast system
