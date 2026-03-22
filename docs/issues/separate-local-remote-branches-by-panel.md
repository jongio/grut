# Separate local and remote branches by panel mode

## Summary

The Git and GitHub panels should scope their Branches tab to show only contextually relevant branches: Git shows local branches, GitHub shows remote branches.

## Description

Currently both the `gitinfo` panel (ModeGit) and the `github` panel (ModeGitHub) display the same combined list of all branches (local then remote) in their Branches tab. This is redundant and confusing — users see the same data in two places with no clear distinction.

The natural mental model is:
- **Git → Branches** = local branches (what I have checked out on this machine)
- **GitHub → Branches** = remote branches (what exists on the remote/GitHub)

Splitting them this way eliminates redundancy, reduces visual noise, and makes each panel's purpose clearer.

## Technical Details

### Current behavior

Both panels call `buildGitItems()` which iterates all branches from `git.BranchList()` and appends them in order: local branches (`kindLocalBranch`) then remote branches (`kindRemoteBranch`).

**Key file:** `internal/panels/gitinfo/gitinfo.go` — `buildGitItems()` function (lines ~2575-2598):

```go
var local, remote []git.Branch
for _, b := range branches {
    if b.IsRemote {
        remote = append(remote, b)
    } else {
        local = append(local, b)
    }
}
// Both local and remote appended to tabBranches regardless of panel mode
for _, b := range local {
    p.tabItems[tabBranches] = append(...)
}
for _, b := range remote {
    p.tabItems[tabBranches] = append(...)
}
```

### Panel modes

- `ModeGit` — registered as `"gitinfo"` panel, shows: branches, worktrees, remotes, stash, tags, reflog
- `ModeGitHub` — registered as `"github"` panel, shows: branches, tags, issues, PRs, actions, workflows, releases

Both modes use the same `buildGitItems()` path for the Branches tab.

### Proposed change

In `buildGitItems()`, filter branches based on `p.mode`:
- `ModeGit` → only append `kindLocalBranch` items
- `ModeGitHub` → only append `kindRemoteBranch` items
- `ModeAll` (if used) → append both (current behavior)

The `branches.go` standalone panel (if still used) should continue showing both for backward compatibility.

### Affected files

- `internal/panels/gitinfo/gitinfo.go` — `buildGitItems()` branch filtering
- Possibly `internal/panels/gitinfo/gitinfo_test.go` — update tests for mode-aware filtering

## Acceptance Criteria

- [ ] Git panel Branches tab shows only local branches
- [ ] GitHub panel Branches tab shows only remote branches
- [ ] Tab headers or counts reflect the filtered list (not total)
- [ ] Keyboard navigation and delete operations work correctly on filtered lists
- [ ] No regression in branch operations (checkout, delete, rename)
- [ ] Tests updated to verify mode-aware branch filtering

## Related

- Issue #45: x-key delete fails after first use in branches panel
- `internal/panels/gitinfo/gitinfo.go` — main implementation file
- `internal/layout/preset.go` — panel layout (gitinfo and github side-by-side)
- `internal/panels/registry.go` — panel registration (ModeGit vs ModeGitHub)
