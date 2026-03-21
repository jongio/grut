# PR Tab: Color by Mergeable State + Action Run Indicator

## Summary

Two improvements to the GitHub pane's PRs tab:

1. **Mergeable state coloring**: PRs are always rendered green regardless of their mergeable status. PRs should be red when they have merge conflicts, yellow when checks are failing, etc.
2. **Action run indicator**: Show the same status icon used in the Actions tab (✓/✗/●) on each PR line when there is an active or completed workflow run for that PR's head branch.

## Description

### Problem 1: Always-Green PRs

Currently, the `renderPR()` function in `internal/panels/gitinfo/gitinfo.go` uses a simple switch on the `State` field (`"open"`, `"closed"`, `"draft"`, `"merged"`) to assign colors. Both `"open"` and `"closed"` PRs default to the same green (`#50FA7B`). There is no concept of mergeability in the PR data model or rendering.

The GitHub REST API returns additional fields on each PR that are not currently fetched or used:

- `Mergeable` (bool) — whether the PR can be merged without conflicts
- `MergeableState` (string) — `"clean"`, `"dirty"`, `"unstable"`, `"blocked"`, `"unknown"`

### Problem 2: No Visibility into Action Runs from PR Tab

The Actions tab already renders workflow run status with clear icons and colors:
- `✓` green (`#50FA7B`) for success
- `✗` red (`#FF5555`) for failure/timed_out
- `●` yellow (`#F1FA8C`) for in_progress/queued
- `●` dim (`#666666`) for unknown

Action runs already carry a `Branch` field (`ghActionItem.Branch`), so matching a PR's head branch to an action run is straightforward. Reusing the existing action status icons on the PR line gives users immediate CI visibility without switching tabs.

## Technical Details

### Current Data Model

```go
// internal/panels/gitinfo/gitinfo.go ~line 134
type ghPRItem struct {
    Title      string
    State      string       // "open", "closed", "merged", "draft"
    HeadBranch string
    Author     string
    HTMLURL    string
    Number     int
}
```

### Current Color Logic

```go
// internal/panels/gitinfo/gitinfo.go ~line 4009
switch pr.State {
case "draft":
    fg = defaultColors.PRDraft    // Orange #FFB86C
case prStateMerged:
    fg = defaultColors.PRMerged   // Purple #BD93F9
default:
    fg = defaultColors.PR         // Green #50FA7B — used for BOTH "open" AND "closed"
}
```

### Current Color Definitions

```go
// ~line 370
PR:       "#50FA7B"   // Green
PRDraft:  "#FFB86C"   // Orange
PRMerged: "#BD93F9"   // Purple
```

### Proposed New Fields on `ghPRItem`

```go
type ghPRItem struct {
    // ... existing fields ...
    Mergeable      bool   // from pr.GetMergeable()
    MergeableState string // from pr.GetMergeableState(): "clean", "dirty", "unstable", "blocked", "unknown"
    ReviewDecision string // "APPROVED", "CHANGES_REQUESTED", "REVIEW_REQUIRED" (requires GraphQL)
    ActionStatus   string // matched from ghActionItem by head branch: "", "success", "failure", "in_progress", "queued"
    ActionConclusion string // matched from ghActionItem by head branch
}
```

### Action Run Indicator on PR Line

Reuse the same icons and colors from `renderActionRun()`:

| Action State | Icon | Color | Hex |
|---|---|---|---|
| Success | `✓` | Green | `#50FA7B` (ActionOK) |
| Failure / Timed Out | `✗` | Red | `#FF5555` (ActionFail) |
| In Progress / Queued | `●` | Yellow | `#F1FA8C` (ActionRun) |
| No run / Unknown | (none) | — | omit indicator |

**Rendering**: Append the action icon after the PR state indicator on the right side of the line, e.g.:
```
  #42 Fix login redirect    open ●
  #38 Update deps           open ✓
  #35 Refactor auth         open ✗
```

**Matching logic**: For each PR, find the most recent `ghActionItem` where `actionItem.Branch == pr.HeadBranch`. Use that run's `Status`/`Conclusion` to pick the icon. Action runs are already fetched and stored; this is a cross-reference at render time.

### Proposed Color Scheme (Mergeable State)

| State | Condition | Color | Hex | Rationale |
|-------|-----------|-------|-----|-----------|
| Open + Clean | Mergeable, checks pass | Green | `#50FA7B` | Ready to merge |
| Open + Dirty | Merge conflicts | Red | `#FF5555` | Needs conflict resolution |
| Open + Unstable | Checks failing | Yellow | `#F1FA8C` | Checks need attention |
| Open + Blocked | Reviews missing or changes requested | Orange | `#FFB86C` | Waiting on review action |
| Open + Unknown | GitHub still computing | Dim/gray | `#6272A4` | Status pending |
| Draft | Draft PR | Orange | `#FFB86C` | (unchanged) |
| Merged | Already merged | Purple | `#BD93F9` | (unchanged) |
| Closed | Closed without merge | Red dim | `#FF5555` at 50% | Rejected / abandoned |

### API Considerations

- **REST API limitation**: The `Mergeable` and `MergeableState` fields are only available when fetching a **single** PR (`GET /repos/{owner}/{repo}/pulls/{number}`), not from the list endpoint (`GET /repos/{owner}/{repo}/pulls`). This means either:
  - **Option A**: Fetch the list first, then make individual requests for each PR's mergeable state (N+1 queries, but can be parallelized and cached)
  - **Option B**: Switch to GitHub's GraphQL API which can return `mergeable` and `mergeStateStatus` in a single batched query for all PRs
  - **Option C**: Fetch mergeable state lazily — show base color on load, then enrich in the background as individual PR details are fetched

- **Review decision** (`APPROVED`, `CHANGES_REQUESTED`, `REVIEW_REQUIRED`) is only available via GraphQL's `reviewDecision` field on PullRequest.

### Affected Files

- `internal/panels/gitinfo/gitinfo.go` — `ghPRItem` struct, `renderPR()`, color definitions, PR fetch logic
- `internal/github/client.go` — May need individual PR fetch or GraphQL support
- `internal/github/interfaces.go` — Interface updates if new methods added
- Theme/color definitions (if externalized to theme system)

## Acceptance Criteria

### Mergeable State Colors
- [ ] PRs with merge conflicts render in red (`#FF5555`)
- [ ] PRs with failing checks render in yellow (`#F1FA8C`)
- [ ] PRs blocked on review render in orange (`#FFB86C`)
- [ ] Closed (not merged) PRs render differently from open PRs
- [ ] Clean/mergeable open PRs remain green (`#50FA7B`)
- [ ] Mergeable state is fetched without significantly degrading load time (lazy/background fetch acceptable)
- [ ] Color scheme respects existing theme system and Dracula-inspired palette
- [ ] New colors are added to `defaultColors` struct for theme overridability

### Action Run Indicator
- [ ] Each PR line shows the action status icon (✓/✗/●) when a matching workflow run exists for that PR's head branch
- [ ] Icons and colors match the Actions tab exactly (ActionOK, ActionFail, ActionRun)
- [ ] No icon shown when no matching action run exists
- [ ] Cross-reference uses already-fetched action run data (no additional API calls)

## Related

- Existing PR color constants: `defaultColors.PR`, `defaultColors.PRDraft`, `defaultColors.PRMerged`
- GitHub REST API: [Pull Requests](https://docs.github.com/en/rest/pulls/pulls)
- GitHub GraphQL: [PullRequest.mergeable](https://docs.github.com/en/graphql/reference/enums#mergeablestate), [PullRequest.mergeStateStatus](https://docs.github.com/en/graphql/reference/enums#mergestatestatus)
