# Workflow Dispatch Triggers Duplicate Run

## Summary

Manually executing a workflow via `workflow_dispatch` (e.g., the Release workflow) causes it to appear twice in the GitHub Actions tab — the workflow effectively runs twice.

## Description

When triggering a workflow like `release.yml` through the GitHub UI "Run workflow" button, two workflow runs are created instead of one. This wastes CI minutes and can cause race conditions when both runs attempt to push tags, commits, or release artifacts concurrently.

The Release workflow is the most impactful case because it:
- Creates and pushes a git tag
- Pushes a commit to `main` (perf baselines, CHANGELOG, CONTRIBUTORS)
- Creates a GitHub Release via GoReleaser
- Deploys the website

Two concurrent release runs could produce duplicate tags, conflicting pushes, or partial releases.

## Technical Details

### Workflow Structure

**`release.yml`** — triggered by `workflow_dispatch`:
1. Calls `ci.yml` as a reusable workflow (`uses: ./.github/workflows/ci.yml`, line 22)
2. `release` job (needs `ci`):
   - Pushes a new tag: `git push origin "v..."` (line 87)
   - Pushes a commit to `main`: `git push` (line 187) — commit message includes `[skip ci]`
3. `pages` job (needs `release`): calls `deploy-web.yml` as reusable workflow (line 198)

**`ci.yml`** — triggered by `push` (main), `pull_request`, and `workflow_call`:
```yaml
on:
  push:
    branches: [main]
    paths-ignore: ['**.md', 'docs/**', 'LICENSE', '.github/*.md']
  pull_request:
    paths-ignore: [...]
  workflow_call:
```

**`deploy-web.yml`** — triggered by `push` (main, paths: `web/**`), `workflow_dispatch`, and `workflow_call`:
```yaml
on:
  push:
    branches: [main]
    paths: ['web/**']
  workflow_dispatch:
  workflow_call:
```

### Potential Causes

#### 1. GITHUB_TOKEN push triggering downstream workflows
The release job uses the default `GITHUB_TOKEN` (from `actions/checkout`) for git operations. GitHub's documentation states that pushes made with `GITHUB_TOKEN` **should not** trigger additional workflow runs — but this behavior has had edge cases and bugs historically.

- **Tag push** (line 87): `git push origin "v$VERSION"` — tag pushes don't match `branches: [main]` in `ci.yml`, so this shouldn't trigger CI.
- **Commit push** (line 187): `git push` — pushes to `main`, but commit message includes `[skip ci]`. If `[skip ci]` is not respected or `GITHUB_TOKEN` restriction doesn't apply, this push could trigger `ci.yml` independently.

#### 2. Reusable workflow call appearing as separate run
When `release.yml` calls `ci.yml` via `workflow_call`, GitHub may display the reusable workflow as a separate entry in the Actions tab in addition to the Release workflow. This could give the appearance of a "double run" even though it's a single logical execution.

#### 3. GoReleaser creating a GitHub Release event
GoReleaser (line 189-194) creates a GitHub Release using `GITHUB_TOKEN`. The `release` event type could theoretically trigger other workflows if any are configured for it (none currently are in this repo, but worth ruling out).

#### 4. Race between tag push and commit push
The release job pushes a tag (line 87) and then later pushes a commit (line 187). If GitHub processes these as separate push events before the `GITHUB_TOKEN` restriction kicks in, both could trigger `ci.yml`.

### Existing Mitigations

- **Concurrency groups**: `release.yml` has `concurrency: { group: release, cancel-in-progress: false }` — this prevents concurrent release runs but does NOT prevent a second run from queuing.
- **`[skip ci]` in commit message**: The perf baseline commit (line 186) uses `[skip ci]`, which should prevent push-triggered workflows.
- **`ci.yml` concurrency**: `concurrency: { group: ci-${{ github.ref }}, cancel-in-progress: true }` — if CI is triggered twice for the same ref, the second run cancels the first.

### Affected Files

- `.github/workflows/release.yml` — primary workflow exhibiting the issue
- `.github/workflows/ci.yml` — reusable workflow called by release; also has push trigger
- `.github/workflows/deploy-web.yml` — reusable workflow called by release; also has push/dispatch triggers

## Acceptance Criteria

- [ ] Identify the exact mechanism causing the duplicate run (check Actions tab run history for trigger types)
- [ ] Ensure `workflow_dispatch` on release.yml produces exactly one workflow run
- [ ] CI does not trigger independently when release pushes tags or commits
- [ ] Deploy-web does not trigger independently when called as reusable workflow from release
- [ ] Verify `[skip ci]` is respected for the perf baseline commit push
- [ ] Add integration safeguard if needed (e.g., `if` condition checking trigger context)

## Possible Fixes

1. **Add `tags-ignore` to `ci.yml`**: Explicitly exclude tag pushes (already excluded by `branches: [main]` but belt-and-suspenders):
   ```yaml
   on:
     push:
       branches: [main]
       tags-ignore: ['v*']
   ```

2. **Use a conditional on commit-triggered CI**: Add `if: github.actor != 'github-actions[bot]'` to prevent bot-pushed commits from triggering CI.

3. **Collapse the reusable workflow call**: If the "double run" is just the UI showing `ci.yml` separately, this is cosmetic — but could be addressed by inlining CI steps into release.yml (trade-off: DRY violation).

4. **Use a PAT with restricted permissions**: If `GITHUB_TOKEN` behavior is unreliable, switch to a fine-grained PAT that explicitly cannot trigger workflows.

## Related

- [GitHub Docs: Events that trigger workflows — GITHUB_TOKEN](https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication#using-the-github_token-in-a-workflow)
- [GitHub Docs: Skipping workflow runs](https://docs.github.com/en/actions/managing-workflow-runs-and-deployments/managing-workflow-runs/skipping-workflow-runs)
- `.github/workflows/release.yml` (lines 82-87: tag push, lines 174-187: commit push)
- `.github/workflows/ci.yml` (lines 1-18: trigger configuration)
