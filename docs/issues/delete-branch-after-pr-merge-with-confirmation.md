# Delete branch after PR merge with confirmation

## Summary

After a PR is successfully merged from the PRs tab, offer the user an option to delete the source branch (both remote and local) with a confirmation step before deletion.

## Description

The current PR merge flow (strategy picker to confirmation to async merge) includes a Delete branch after merge checkbox toggle in the confirmation step. However, the branch deletion should be a post-merge confirmation flow rather than a pre-merge checkbox, giving users a clear moment to decide after seeing the merge succeed.

The desired UX is:
1. User merges a PR via the PRs tab (existing flow)
2. Merge succeeds - user sees success feedback
3. A confirmation prompt appears: Delete branch feature/xyz? (remote + local)
4. User confirms or dismisses
5. If confirmed: delete the remote branch via GitHub API, delete the local branch if it exists, and show success/error feedback

This is safer than the pre-merge checkbox approach because the user makes the deletion decision only after confirming the merge succeeded.

## Technical Details

### Existing infrastructure

- Merge action: internal/panels/gitinfo/gitinfo.go with ActionMergePR multi-step modal flow
- GitHub client: internal/github/ with DeleteBranch method for remote branch deletion via git ref API
- Messages: PRMergeRequestedMsg / PRMergeFailedMsg for cross-panel messaging
- Merge result handling: prMergeResultMsg case in the Update() switch dispatches after async merge completes

### Implementation approach

1. After a successful merge result (prMergeResultMsg with no error), show a confirmation modal asking whether to delete the branch
2. On confirm: call DeleteBranch for remote, attempt local branch deletion (git branch -d) if the branch exists locally
3. Show feedback: Branch deleted or error message
4. On dismiss: do nothing, branch remains

### Key files

- internal/panels/gitinfo/gitinfo.go - merge flow, result handling, modal rendering
- internal/panels/gitinfo/gitinfo_extra_test.go - merge-related tests
- internal/github/github.go - DeleteBranch method

## Acceptance Criteria

- [ ] After successful PR merge, a confirmation prompt asks the user whether to delete the source branch
- [ ] Confirmation clearly shows the branch name being deleted
- [ ] On confirm: remote branch is deleted via GitHub API
- [ ] On confirm: local branch is deleted if it exists (with git branch -d)
- [ ] On dismiss: branch is preserved, no action taken
- [ ] Success feedback shown after branch deletion
- [ ] Error feedback shown if deletion fails (e.g., protected branch)
- [ ] Existing merge tests continue to pass
- [ ] New tests cover the post-merge branch deletion flow

## Related

- PR #20: feat: add PR merge action with strategy picker and branch cleanup
- Issue #19: Original PR merge feature request
- internal/panels/gitinfo/gitinfo.go - primary implementation file
- internal/github/github.go - GitHub API client with DeleteBranch
