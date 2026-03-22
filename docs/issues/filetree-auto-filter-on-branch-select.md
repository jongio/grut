# FileTree: Auto-filter to show changed files when a branch is selected

## Summary

When a user clicks/selects a branch in either the git info or GitHub panes, the FileTree panel should automatically switch to showing only the files changed in that branch (compared to the default branch), similar to how commit-files mode and PR-files mode already work.

## Description

Currently, selecting a branch in the gitinfo panel emits a `BranchSelectedMsg` that is consumed by the gitlog panel (to reload commits) but **not** by the filetree panel. The filetree continues showing the full working directory regardless of which branch is selected.

The filetree already has two analogous modes:

- **Commit-files mode** — triggered by `CommitSelectedMsg`, calls `DiffTreeFiles()` to show files changed in a specific commit.
- **PR-files mode** — triggered by `PRFilesLoadedMsg`, shows files changed in a pull request.

A new **branch-files mode** should follow the same pattern: when a branch is selected, diff it against the default branch and filter the filetree to only show the changed files.

## Technical Details

### Current message flow

```
User clicks branch in gitinfo panel
  → handleMouseClick() (gitinfo.go:1235)
  → branchSelectedCmd() (gitinfo.go:1502)
  → emits panels.BranchSelectedMsg{Name: "branch-name"}
  → consumed by gitlog (gitlog.go:264) — reloads commits for that branch
  → filetree: DOES NOT react (gap)
```

### Key files

| Component | Path |
|-----------|------|
| Git/GitHub Info Panel | `internal/panels/gitinfo/gitinfo.go` |
| FileTree Panel | `internal/panels/filetree/filetree.go` |
| Message Definitions | `internal/panels/messages.go` |
| Diff Operations | `internal/git/diff.go` |
| DiffTree Operations | `internal/git/difftree.go` |
| Git Interfaces | `internal/git/interfaces.go` |

### Existing patterns to follow

**Commit-files mode** (`filetree.go:479-545`):
- `handleCommitSelected()` calls `gc.DiffTreeFiles(ctx, hash)` to get changed files.
- Sets `commitFilesMode = true`, populates `commitFiles` and `commitChangedPaths`.
- Calls `expandDirsInSet()` and `rebuildVisible()` to filter the tree.
- Exited via `CommitDeselectedMsg`.

**PR-files mode** (`filetree.go:550-565`):
- `handlePRFilesLoaded()` sets `prFilesMode = true`, populates `prFiles`.
- Similar expansion and filtering logic.
- Exited via `PRDeselectedMsg`.

### Proposed approach

1. **Add `BranchSelectedMsg` handler in filetree** — When received, run a diff between the selected branch and the default branch using `git.Diff(ctx, DiffOpts{CommitA: defaultBranch, CommitB: selectedBranch, NameOnly: true})`.
2. **Add `branchFilesMode`** — New boolean flag on `FileTree`, analogous to `commitFilesMode` and `prFilesMode`. When active, filter the tree to only show files in the diff result.
3. **Add `BranchDeselectedMsg`** (or reuse deselection on clicking the same branch again) — exits branch-files mode and restores the full tree.
4. **Handle "current branch" edge case** — If the user selects the branch they're already on, show working-tree changes (staged + unstaged) instead of a branch diff, or simply clear the filter.
5. **Visual indicator** — Show a label like `"branch: feature/xyz"` in the filetree header (similar to how commit-files mode shows the commit hash).

### Git operation for branch diff

```go
opts := git.DiffOpts{
    CommitA:  "main",            // or detected default branch
    CommitB:  "feature/branch",  // selected branch
    NameOnly: true,
}
diffs, err := gitClient.Diff(ctx, opts)
```

This returns all files changed between the two branches.

## Acceptance Criteria

- [ ] Clicking a branch in the gitinfo panel's Branches tab filters the filetree to show only files changed in that branch (vs default branch)
- [ ] Clicking a branch in the gitinfo panel's GitHub/remote branches also triggers the same filtering
- [ ] A visual indicator shows which branch is filtering the filetree (e.g., header label)
- [ ] Selecting the same branch again (or a deselect action) exits branch-files mode and restores the full tree
- [ ] The branch-files mode follows the same UX patterns as commit-files mode and PR-files mode
- [ ] Auto-expand directories containing changed files (consistent with commit-files mode behavior)
- [ ] Edge case: selecting the current/active branch behaves sensibly (show working-tree changes or no filter)

## Related

- Commit-files mode: `internal/panels/filetree/filetree.go` lines 479-545
- PR-files mode: `internal/panels/filetree/filetree.go` lines 550-565
- Branch selection: `internal/panels/gitinfo/gitinfo.go` lines 1502-1516
- `BranchSelectedMsg`: `internal/panels/messages.go` lines 92-97
- Git diff: `internal/git/diff.go`
