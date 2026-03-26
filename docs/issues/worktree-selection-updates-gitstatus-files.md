# Worktree Selection Should Update Git Status File List

## Summary

Selecting a worktree in the worktrees panel should update the gitstatus panel to show the changed files for that worktree, rather than always showing the status of the current working directory.

## Description

Currently the worktrees panel (`internal/panels/worktrees/worktrees.go`) and the gitstatus panel (`internal/panels/gitstatus/gitstatus.go`) operate independently. The gitstatus panel always runs `git status --porcelain=v2 --branch -uall` against the current repo root and displays staged/unstaged/untracked files for that single context.

When a user selects (highlights) a worktree in the worktrees panel, nothing changes in the gitstatus panel — it continues to show the files from the main worktree. The expected behavior is that selecting a worktree should cause the gitstatus panel to refresh and display the changed files *within that worktree's directory*, giving the user a quick view of what's happening in each worktree without having to switch into it.

This is important for multi-branch workflows where developers use worktrees to work on several features or fixes in parallel. Being able to glance at the status of each worktree from the main UI without switching context is a significant productivity improvement.

## Technical Details

### Current Architecture

- **Worktrees panel** (`internal/panels/worktrees/worktrees.go`): Lists worktrees via `git worktree list --porcelain`, parsed through `WorktreeList()` in `internal/git/worktree.go`. Each `worktreeItem` contains a `git.Worktree` with `Path`, `Head`, `Branch`, and `Bare` fields.
- **GitStatus panel** (`internal/panels/gitstatus/gitstatus.go`): Calls `git.Status(ctx)` (which runs `git status --porcelain=v2`) to populate `files []git.FileStatus`. The `rebuildRows()` method flattens these into section headers, file entries, and expandable diff hunks.
- **Git client** (`internal/git/client.go`): The `Status()` call in `internal/git/status.go` runs against the repo root. It does not accept an alternate working directory parameter.
- **Message bus**: The worktree panel emits `panels.WorktreeChangedMsg` on create/remove. The gitstatus panel listens for `panels.RefreshGitStatusMsg` to reload. There is no message for "worktree cursor moved" or "worktree selected."

### Key Files

| File | Role |
|------|------|
| `internal/panels/worktrees/worktrees.go` | Worktree list UI, selection, operations |
| `internal/panels/gitstatus/gitstatus.go` | Git status file list, staging, diff display |
| `internal/git/worktree.go` | `WorktreeList()`, `WorktreeAdd()`, `WorktreeRemove()` |
| `internal/git/status.go` | `Status()` — runs `git status --porcelain=v2` |
| `internal/git/client.go` | Git client interface and implementation |
| `internal/git/types.go` | `Worktree`, `FileStatus`, `StatusCode` types |
| `internal/panels/panels.go` | Shared message types (`WorktreeChangedMsg`, `RefreshGitStatusMsg`, etc.) |

### Proposed Approach

1. **New message type**: Add a `WorktreeSelectedMsg` (or similar) to `internal/panels/panels.go` carrying the selected worktree's filesystem path.
2. **Worktree panel emits on cursor move**: When the user moves the cursor in the worktrees panel and lands on a different worktree, emit `WorktreeSelectedMsg{ Path: selectedWorktree.Path }`.
3. **Git status accepts alternate directory**: Extend `Status()` (or add a variant) so it can run `git status` with `-C <path>` targeting an arbitrary worktree directory.
4. **GitStatus panel listens**: On receiving `WorktreeSelectedMsg`, re-run status against the given path and rebuild rows. The panel title or a subtitle could indicate which worktree's status is being shown.
5. **Diff loading**: Ensure diff expansion also works in the context of the selected worktree (i.e., `git diff -C <path>`).
6. **Reset on deselect**: If the user navigates away from the worktrees panel or selects the main worktree, revert to showing the normal (current repo) status.

## Acceptance Criteria

- [ ] Selecting a worktree in the worktrees panel updates the gitstatus panel to show files changed in that worktree
- [ ] The gitstatus panel indicates which worktree's status is being displayed (e.g., branch name or path in title)
- [ ] Diff expansion works correctly for the selected worktree's files
- [ ] Staging/unstaging operations work correctly against the selected worktree
- [ ] Selecting the main worktree (or navigating away) reverts the gitstatus panel to its default behavior
- [ ] No regression in gitstatus behavior when the worktrees panel is not active

## Related

- Worktree panel: `internal/panels/worktrees/worktrees.go`
- GitStatus panel: `internal/panels/gitstatus/gitstatus.go`
- Git status parsing: `internal/git/status.go`
- Message bus types: `internal/panels/panels.go`
