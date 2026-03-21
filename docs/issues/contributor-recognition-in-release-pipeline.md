# Build World-Class Contributor Recognition into Release Pipeline

## Summary

Add automated contributor recognition throughout the grut release pipeline so every contributor is called out by name and thanked for their contributions in changelogs, GitHub Releases, and project documentation.

## Description

grut has a well-structured, professionally-configured release system with excellent automation (GoReleaser, GitHub Actions, automated benchmarks, performance injection into changelogs). However, it currently has **zero formal contributor recognition**:

- `CHANGELOG.md` lists changes but never credits who made them
- GitHub Releases (via GoReleaser) contain install instructions and performance data but no contributor acknowledgments
- No `CONTRIBUTORS.md` or acknowledgments file exists
- No automated contributor extraction from git history or PRs
- `CONTRIBUTING.md` doesn't mention how contributors will be recognized

For an open-source project, contributor recognition is a force multiplier — it incentivizes contributions, builds community, and signals a healthy project culture. This should be world-class, not an afterthought.

## Technical Details

### Current Release Infrastructure

**Release workflow**: `.github/workflows/release.yml`
- Manual trigger with `bump` input (patch/minor/major)
- Runs CI → computes next version → creates git tag → runs benchmarks → generates perf notes → injects into CHANGELOG → runs GoReleaser → deploys web docs

**GoReleaser config**: `.goreleaser.yml`
- Changelog groups: Features, Bug Fixes, Performance, Other
- Release header: install instructions only
- No contributor-related template variables used

**CHANGELOG.md**: Follows Keep a Changelog v1.1.0 + SemVer
- Sections: Added, Changed, Fixed, Security, Performance
- No Contributors section exists

**Magefile**: `magefile.go`
- `mage install` builds dev binary with version info
- No contributor-related targets

**Performance integration**: `scripts/bench-release-notes.sh`
- Generates benchmark comparison for changelog injection
- Pattern to follow for contributor injection

### What Needs to Change

#### 1. Contributor Extraction Script (`scripts/contrib-release-notes.sh` or Go equivalent)

Create a script/tool that extracts contributors between two git tags:

```
git log v0.0.1..v0.1.0 --format='%aN <%aE>' | sort -u
```

Also extract from `Co-authored-by:` trailers:

```
git log v0.0.1..v0.1.0 --format='%(trailers:key=Co-authored-by,valueonly)' | sort -u
```

And from GitHub PR data (merged PRs in the range):

```
gh pr list --state merged --base main --json author,mergedAt --limit 100
```

Deduplicate, filter bots, format as a thank-you section.

#### 2. CHANGELOG.md Enhancement

Add a `### Contributors` section to each version entry:

```markdown
## [0.2.0] - 2026-04-01

### Added
- ...

### Contributors

Thanks to the following people for their contributions to this release:

- **Jon Gallant** (@jongio)
- **Jane Developer** (@janedev)
- **Alex Contributor** (@alexc)

New contributors: **Jane Developer** (@janedev) — welcome! 🎉
```

"New contributors" highlights first-time contributors to celebrate their entry.

#### 3. GoReleaser Release Notes

Update `.goreleaser.yml` to include contributor acknowledgments in the GitHub Release body. GoReleaser supports `release.header` and `release.footer` templates, plus custom extra files. The contributor section could be injected via the release workflow (similar to how perf notes are injected).

#### 4. Release Workflow Integration

In `.github/workflows/release.yml`, add a step after benchmark generation:

```yaml
- name: Generate Contributor Notes
  run: |
    PREV_TAG=$(git describe --tags --abbrev=0 HEAD^)
    scripts/contrib-release-notes.sh $PREV_TAG ${{ env.NEW_TAG }} > contrib-notes.md
    # Inject into CHANGELOG.md under the new version's section
```

#### 5. CONTRIBUTORS.md (All-Time Hall of Fame)

Create and auto-maintain `CONTRIBUTORS.md` at the repo root listing all-time contributors:

```markdown
# Contributors

Thank you to everyone who has contributed to grut!

## Core Maintainers

- **Jon Gallant** (@jongio) — Creator & Lead

## Contributors

(auto-generated from git history + GitHub PRs)

- **Name** (@handle) — N contributions
```

This file should be auto-updated during the release process.

#### 6. Mage Target

Add a `mage contributors` target that regenerates `CONTRIBUTORS.md` from git history. This can be called during preflight or release.

#### 7. CONTRIBUTING.md Update

Add a section explaining how contributors are recognized:

```markdown
## Recognition

All contributors are automatically recognized in:
- The CHANGELOG.md entry for each release
- GitHub Release notes
- The CONTRIBUTORS.md hall of fame

Your first contribution gets a special "New contributor" callout!
```

### Files Affected

| File | Change |
|------|--------|
| `scripts/contrib-release-notes.sh` (or `.go`) | **New** — contributor extraction script |
| `.goreleaser.yml` | Update release header/footer with contributor section |
| `.github/workflows/release.yml` | Add contributor note generation step |
| `CHANGELOG.md` | Add Contributors section to Unreleased |
| `CONTRIBUTORS.md` | **New** — all-time contributor list |
| `CONTRIBUTING.md` | Add recognition section |
| `magefile.go` | Add `mage contributors` target |

### Design Considerations

- **Bot filtering**: Exclude `github-actions[bot]`, `dependabot[bot]`, `copilot-swe-agent[bot]` from contributor lists (but keep `Co-authored-by: Copilot` as it represents human-AI collaboration)
- **Deduplication**: Same person may appear with different email addresses — use GitHub username as canonical identity where possible
- **Privacy**: Use GitHub handles (`@username`) rather than email addresses in public-facing docs
- **First-time detection**: Compare current release contributors against all previous tags to identify new contributors
- **Graceful degradation**: If `gh` CLI isn't available, fall back to git-only extraction

## Acceptance Criteria

- [ ] Contributor extraction script exists and correctly identifies all contributors between two tags
- [ ] `Co-authored-by:` trailers are parsed and included
- [ ] Bot accounts are filtered out
- [ ] First-time contributors are identified and highlighted
- [ ] CHANGELOG.md includes a Contributors section for each release
- [ ] GitHub Releases include contributor acknowledgments
- [ ] `CONTRIBUTORS.md` exists and is auto-updated during releases
- [ ] `CONTRIBUTING.md` documents the recognition process
- [ ] `mage contributors` target works for manual regeneration
- [ ] Release workflow (`.github/workflows/release.yml`) includes contributor generation step
- [ ] End-to-end test: a mock release correctly generates contributor notes

## Related

- Current release workflow: `.github/workflows/release.yml`
- GoReleaser config: `.goreleaser.yml`
- Benchmark integration (pattern to follow): `scripts/bench-release-notes.sh`
- Contributing guide: `CONTRIBUTING.md`
- Changelog: `CHANGELOG.md`
- Prior art: [all-contributors](https://github.com/all-contributors/all-contributors), [GitHub auto-generated release notes](https://docs.github.com/en/repositories/releasing-projects-on-github/automatically-generated-release-notes)
