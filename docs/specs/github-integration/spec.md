# GitHub Integration Spec

## Overview

Add native GitHub integration inside grut: issues, pull requests, Actions, comments, reviews — all with AI-first workflows. GitHub data lives alongside local git data in the same panels. Users can triage issues, review PRs, respond to comments, and fix code without leaving the terminal.

## Design Philosophy

GitHub is **not a mode** — it's a native data source. The existing gitinfo pane gains a second row of tabs (Issues, PRs, Actions) alongside the existing git tabs (Branches, Worktrees, Remotes, Stash). Selecting a GitHub tab contextually updates the other panes (Files, Commits, Preview) just like selecting a branch updates commits today.

**Principles**:
1. **No mode switch** — GitHub tabs live alongside Git tabs in the same pane
2. **Contextual reaction** — selecting a PR updates Files+Commits+Preview; selecting an issue only updates Preview
3. **PR = commit-files mode** — reuses the same pattern as clicking a commit (files pane shows changed files)
4. **Escape unwinds** — same progressive escape: deselect item → switch to default tab → nothing
5. **Always available** — Issues/PRs/Actions counts visible even when not active tab
6. **Status at a glance** — Actions tab shows ✓/✗ for latest CI status
7. **Attention-first** — "Needs Review" and "Assigned" filters surface what needs your action immediately

## Architecture

### SDK Choice: `google/go-github`

**Decision**: Use `google/go-github` SDK with `gh auth token` for authentication bootstrap. NOT the `gh` CLI for API calls.

**Rationale** (evaluated 3 options):

| Factor | `gh` CLI | `google/go-github` | `cli/go-gh` |
|--------|----------|-------------------|-------------|
| Per-call cost | 300-700ms (process spawn) | 50-200ms (HTTP only) | 50-200ms (HTTP only) |
| Pagination | Manual (`--limit`, `--paginate`) | Built-in `ListOptions` | Manual |
| Auth | Automatic | One-time `gh auth token` | Automatic |
| Typing | JSON parsing | Native Go structs | Minimal types |
| Rate limiting | Hidden | `Response.Rate` struct | Manual |
| Community | CLI users | 10k+ GitHub stars, Google-maintained | Small, CLI-focused |

Key insight: git operations are local (~5ms spawn cost is negligible), but GitHub API is remote — the ~300-700ms `gh` CLI spawn overhead compounds badly when fetching issues, PRs, and comments in parallel.

**Auth strategy**:
```go
func newGitHubClient(ctx context.Context) (*github.Client, error) {
    // 1. Try gh auth token (one-time shell-out, cached)
    token, err := exec.Command("gh", "auth", "token").Output()
    if err == nil {
        return github.NewClient(nil).WithAuthToken(strings.TrimSpace(string(token))), nil
    }
    // 2. Fallback to GITHUB_TOKEN env var
    if t := os.Getenv("GITHUB_TOKEN"); t != "" {
        return github.NewClient(nil).WithAuthToken(t), nil
    }
    return nil, fmt.Errorf("no GitHub auth: run 'gh auth login' or set GITHUB_TOKEN")
}
```

**Escape hatch**: Keep a `ghExec()` helper for complex operations better handled by the CLI (e.g., `gh pr merge --auto`, `gh run rerun`).

### Compositional Interfaces

Follow the existing `internal/git/interfaces.go` pattern — small focused interfaces, not one monolithic `GitHubOps`:

```go
// internal/github/interfaces.go

type IssueReader interface {
    ListIssues(ctx context.Context, owner, repo string, opts *github.IssueListByRepoOptions) ([]*github.Issue, error)
    GetIssue(ctx context.Context, owner, repo string, number int) (*github.Issue, error)
    GetIssueComments(ctx context.Context, owner, repo string, number int) ([]*github.IssueComment, error)
}

type IssueWriter interface {
    CreateIssue(ctx context.Context, owner, repo string, req *github.IssueRequest) (*github.Issue, error)
    EditIssue(ctx context.Context, owner, repo string, number int, req *github.IssueRequest) error
    CommentOnIssue(ctx context.Context, owner, repo string, number int, body string) error
    CloseIssue(ctx context.Context, owner, repo string, number int) error
    ReopenIssue(ctx context.Context, owner, repo string, number int) error
}

type PRReader interface {
    ListPRs(ctx context.Context, owner, repo string, opts *github.PullRequestListOptions) ([]*github.PullRequest, error)
    GetPR(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error)
    GetPRFiles(ctx context.Context, owner, repo string, number int) ([]*github.CommitFile, error)
    GetPRComments(ctx context.Context, owner, repo string, number int) ([]*github.PullRequestComment, error)
    GetPRReviews(ctx context.Context, owner, repo string, number int) ([]*github.PullRequestReview, error)
    GetPRDiff(ctx context.Context, owner, repo string, number int) (string, error)
    GetPRCommits(ctx context.Context, owner, repo string, number int) ([]*github.RepositoryCommit, error)
}

type PRWriter interface {
    CreatePR(ctx context.Context, owner, repo string, req *github.NewPullRequest) (*github.PullRequest, error)
    MergePR(ctx context.Context, owner, repo string, number int, msg string, opts *github.PullRequestOptions) error
    CommentOnPR(ctx context.Context, owner, repo string, number int, body string, path string, line int) error
    SubmitReview(ctx context.Context, owner, repo string, number int, review *github.PullRequestReviewRequest) error
    RequestReviewers(ctx context.Context, owner, repo string, number int, reviewers []string) error
    ResolveThread(ctx context.Context, owner, repo string, threadID int64) error
}

type ActionReader interface {
    ListWorkflowRuns(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) ([]*github.WorkflowRun, error)
    GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (*github.WorkflowRun, error)
    ListWorkflowJobs(ctx context.Context, owner, repo string, runID int64) ([]*github.WorkflowJob, error)
    GetJobLogs(ctx context.Context, owner, repo string, jobID int64) (string, error)
}

type ActionWriter interface {
    RerunFailedJobs(ctx context.Context, owner, repo string, runID int64) error
    CancelWorkflowRun(ctx context.Context, owner, repo string, runID int64) error
}

type NotificationReader interface {
    ListNotifications(ctx context.Context, opts *github.NotificationListOptions) ([]*github.Notification, error)
    MarkRead(ctx context.Context, threadID string) error
}

// Combined interface for the full client
type Client interface {
    IssueReader
    IssueWriter
    PRReader
    PRWriter
    ActionReader
    ActionWriter
    NotificationReader
    RepoInfo(ctx context.Context, owner, repo string) (*github.Repository, error)
    CurrentUser(ctx context.Context) (*github.User, error)
}
```

### Package Structure

```
internal/github/
├── client.go           # Client implementation wrapping google/go-github
├── interfaces.go       # Compositional interfaces (above)
├── types.go            # App-specific types (filters, view models)
├── auth.go             # Auth: gh auth token → GITHUB_TOKEN fallback
├── cache.go            # In-memory cache with TTL for API responses
├── exec.go             # ghExec() escape hatch for CLI-only operations
└── client_test.go      # Tests with httptest mock server
```

## UI Layout: Integrated Dual-Row Tab Bar

The gitinfo pane (`internal/panels/gitinfo/`) gains a second tab row. No new panels are created — GitHub data is rendered within the existing gitinfo pane using the existing tab infrastructure.

### Tab Bar Design

```
├─ Git ────────────────────────────────────────────┤
│ Branches 5 · Worktrees 1 · Remotes 2 · Stash 0  │  ← Row 1 (Git)
│ Issues 12 · PRs 3 · Actions ✓                    │  ← Row 2 (GitHub)
```

- Row 1 and Row 2 use the same styling: active = bold + cyan (#8BE9FD) + underline, inactive = dim (#666666)
- Only one tab across both rows is active at a time
- Clicking a GitHub tab deactivates the git tab and vice versa

### Cross-Panel Integration Points

| Active Tab | Files Pane | Commits Pane | Preview Pane |
|------------|-----------|-------------|-------------|
| Branches | Local file tree | Branch commits | File preview |
| Worktrees | Local file tree | HEAD commits | File preview |
| Remotes | Local file tree | HEAD commits | File preview |
| Stash | Local file tree | HEAD commits | Stash diff |
| **Issues** | Local file tree (unchanged) | Branch commits (unchanged) | **Issue body (markdown)** |
| **PRs** | **PR changed files** | **PR commits** | **PR file diff** |
| **Actions** | Local file tree (unchanged) | Branch commits (unchanged) | **Job logs** |

### Quick Filters (Attention-First Views)

PRs and Issues tabs have built-in quick filters cycled with `f`. Filter counts display as a sub-header.

**PRs tab filters:**
```
 All 12 · Needs Review 3 · Mine 4 · Draft 2
         ^^^^^^^^^^^^^^
```
- **All**: every open PR (default)
- **Needs Review**: PRs requesting your review that you haven't responded to
- **Mine**: PRs you authored
- **Draft**: draft PRs

**Issues tab filters:**
```
 All 42 · Assigned 5 · Mentioned 2 · Created 8
          ^^^^^^^^^^
```
- **All**: every open issue (default)
- **Assigned**: issues assigned to you
- **Mentioned**: issues where you're mentioned
- **Created**: issues you created

### Wireframes

#### Default View (Git tabs active — same as today, with GitHub counts visible)

```
╭─ Files ──────────┬─ main.go ─────────────────────────────────────╮
│ ▸ .github/       │  1  package main                              │
│ ▾ cmd/           │  2                                            │
│   ▾ root/        │  3  import (                                  │
│     root.go      │  4      "fmt"                                 │
│ ▾ internal/      │  5  )                                         │
│   ▸ ai/          │                                               │
│   ▸ git/         │                                               │
├─ Git ────────────┤                                               │
│ Branches 5 · Worktrees 1 · Remotes 2 · Stash 0                  │
│ Issues 12 · PRs 3 · Actions ✓                                    │
│ ● main                                                           │
│   feature/auth                                                   │
│   fix/click-bug                                                  │
├─ Commits ────────┤                                               │
│ Fix click handling on last file    abc1234                        │
│ Add stash tab to gitinfo panel     def5678                        │
│ Redesign tab bar header            9ab0cde                        │
╰──────────────────┴───────────────────────────────────────────────╯
```

#### Issues Tab — Selected Issue

```
╭─ Files ──────────┬─ #42 Fix auth token refresh ──────────────────╮
│ ▸ .github/       │                                               │
│ ▾ cmd/           │  ## Description                               │
│   ▾ root/        │  When the OAuth token expires during a        │
│     root.go      │  long session, the app crashes instead of     │
│ ▾ internal/      │  refreshing the token silently.               │
│   ▸ ai/          │                                               │
│   ▸ git/         │  ## Steps to Reproduce                        │
├─ Git ────────────┤  1. Start the app                             │
│ Branches 5 · Worktrees 1 · Remotes 2 · Stash 0                  │
│ Issues 12 · PRs 3 · Actions ✓                                    │
│ ● #42 Fix auth token refresh          bug                        │
│   #38 Add dark mode toggle            enhancement                │
│   #35 Crash on large repo             bug                        │
├─ Commits ────────┤  ## Labels                                    │
│ Fix click handling on last file    abc1234                        │
│ Add stash tab to gitinfo panel     def5678                        │
╰──────────────────┴───────────────────────────────────────────────╯
 j/k scroll  enter view  n new  / search              ctrl+space chat
```

- **GitInfo**: Shows issue list (number, title, labels right-aligned)
- **Preview**: Shows selected issue body (rendered markdown)
- **Files**: Unchanged (still your local files)
- **Commits**: Unchanged (still your branch commits)

#### PRs Tab — PR Selected (contextual panel updates)

```
╭─ Files: PR #41 ──┬─ PR #41 Diff ─────────────────────────────────╮
│   internal/      │  diff --git a/internal/git/client.go          │
│     git/         │  --- a/internal/git/client.go                 │
│ M    client.go   │  +++ b/internal/git/client.go                 │
│ A    github.go   │  @@ -65,7 +65,15 @@                          │
│     panels/      │  +func (c *Client) ghExec(ctx context.Context │
│ M    filetree.go │  +    args ...string) (string, error) {       │
│                  │  +    cmd := exec.CommandContext(ctx, "gh",    │
├─ Git ────────────┤  +        args...)                             │
│ Branches 5 · Worktrees 1 · Remotes 2 · Stash 0                  │
│ Issues 12 · PRs 3 · Actions ✓                                    │
│   #43 Update dependencies            draft                       │
│ ● #41 Add GitHub client              review                      │
│   #39 Fix typo in README             merged                      │
├─ Commits: PR #41 ┤                                               │
│ Add GitHubClient interface         1a2b3c4                        │
│ Implement issue list endpoint      5d6e7f8                        │
│ Add tests for github client        9g0h1i2                        │
╰──────────────────┴───────────────────────────────────────────────╯
 j/k scroll  enter checkout  d diff  / search          ctrl+space chat
```

- **Files**: Switches to show **PR changed files** (reuses commit-files mode)
- **Preview**: Shows the **PR diff** for the selected file
- **Commits**: Shows **PR commits** (not branch commits)
- All contextually linked — click a PR, everything updates

#### PRs Tab — "Needs Review" Filter Active

```
╭─ Files ──────────┬─ PR #41 Add GitHub client ────────────────────╮
│ ▸ .github/       │                                               │
│ ▾ cmd/           │  ## Summary                                   │
│   ▾ root/        │  Adds `google/go-github` SDK integration      │
│     root.go      │  with compositional interfaces matching       │
│ ▾ internal/      │  the existing git client pattern.             │
│   ▸ ai/          │                                               │
│   ▸ git/         │  ## Changes                                   │
├─ Git ────────────┤  - New `internal/github/` package             │
│ Branches 5 · Worktrees 1 · Remotes 2 · Stash 0                  │
│ Issues 12 · PRs 3 · Actions ✓                                    │
│ Needs Review 3 ─────────────────────                             │
│ ● #41 Add GitHub client     @coworker  2h ago                    │
│   #39 Fix auth flow         @teammate  1d ago                    │
│   #36 Update deps           @bot       3d ago                    │
├─ Commits ────────┤                                               │
╰──────────────────┴───────────────────────────────────────────────╯
 j/k scroll  enter diff  f filter  / search            ctrl+space chat
```

#### Issues Tab — "Assigned to Me" Filter Active

```
╭─ Files ──────────┬─ #42 Fix auth token refresh ──────────────────╮
│ ▸ .github/       │                                               │
│ ▾ cmd/           │  ## Description                               │
│   ▾ root/        │  Token expires during long sessions...        │
│     root.go      │                                               │
│ ▾ internal/      │                                               │
│   ▸ ai/          │                                               │
│   ▸ git/         │                                               │
├─ Git ────────────┤                                               │
│ Branches 5 · Worktrees 1 · Remotes 2 · Stash 0                  │
│ Issues 12 · PRs 3 · Actions ✓                                    │
│ Assigned to me 5 ───────────────────                             │
│ ● #42 Fix auth token refresh          bug  P0                    │
│   #38 Add dark mode toggle            enh  P1                    │
│   #35 Crash on large repo             bug  P1                    │
│   #28 Improve test coverage           task P2                    │
│   #22 Update docs                     docs P3                    │
├─ Commits ────────┤                                               │
╰──────────────────┴───────────────────────────────────────────────╯
 j/k scroll  enter view  f filter  / search            ctrl+space chat
```

#### Actions Tab — Selected Run

```
╭─ Files ──────────┬─ CI / Build (Run #1234) ──────────────────────╮
│ ▸ .github/       │                                               │
│ ▾ cmd/           │  ✓ Setup Go           12s                     │
│   ▾ root/        │  ✓ Install deps       45s                     │
│     root.go      │  ✓ Build              1m 23s                  │
│ ▾ internal/      │  ✗ Test               2m 01s                  │
│   ▸ ai/          │    --- FAIL: TestAuth (0.34s)                 │
│   ▸ git/         │        auth_test.go:42: expected nil           │
├─ Git ────────────┤        got: token expired                     │
│ Branches 5 · Worktrees 1 · Remotes 2 · Stash 0                  │
│ Issues 12 · PRs 3 · Actions ✓→✗                                  │
│ ✓ CI / Build #1233   main           2m ago                       │
│ ✗ CI / Build #1234   fix/auth       5m ago                       │
│ ● CI / Test  #1235   fix/auth       3m ago                       │
├─ Commits ────────┤                                               │
│ Fix click handling on last file    abc1234                        │
╰──────────────────┴───────────────────────────────────────────────╯
 j/k scroll  enter logs  r rerun  / search             ctrl+space chat
```

#### AI Chat Overlay (ctrl+space with GitHub context)

```
╭─ Files ──────────┬─ #42 Fix auth token refresh ──────────────────╮
│ ▸ .github/       │                                               │
│ ▾ cmd/           │  ## Description                               │
│   ▾ root/        │  When the OAuth token expires...              │
│     root.go      │                                               │
│                  ├──────────────────────────────────────────────╮ │
│                  │ Looking at issue #42, the token refresh      │ │
│                  │ logic in internal/ai/provider_copilot.go     │ │
├─ Git ────────────┤ doesn't handle expiration. I recommend:      │ │
│ ...              │                                              │ │
│ ● #42 Fix auth   │ 1. Add a token refresh interceptor           │ │
│   #38 Add dark   │ 2. Wrap the HTTP client with retry logic     │ │
├─ Commits ────────┤                                              │ │
│ ...              │ Want me to implement this fix?                │ │
│                  ╰──────────────────────────────────────────────╯ │
│                  │ > How should we handle the refresh?           │
╰──────────────────┴───────────────────────────────────────────────╯
```

Chat automatically has context of the selected issue/PR/action.

## Interactions

### Issues Tab
- `j/k`: Navigate issues
- `enter`: Open issue detail view (preview pane shows body)
- `/`: Search issues (title, body, labels, author, assignee)
- `f`: Cycle filters (All → Assigned → Mentioned → Created)
- `s`: Sort (created, updated, comments, reactions)
- `c`: Create new issue
- `r`: Reply to comment (in detail view)
- `e`: Edit issue (title, body, labels, assignees, milestone)
- `x`: Close/reopen issue
- `a`: Assign/unassign
- `l`: Add/remove labels
- `click`: Select issue (mouse support)
- `esc`: Deselect → switch to default tab

### PRs Tab
- `j/k`: Navigate PRs / scroll diff
- `enter`: Select PR (files+commits+preview update contextually)
- `/`: Search PRs (title, author, branch, label, review status)
- `f`: Cycle filters (All → Needs Review → Mine → Draft)
- `d`: View diff (opens in preview pane)
- `r`: Start review / reply to thread
- `m`: Merge PR (with strategy selection: merge, squash, rebase)
- `R`: Request reviewer / re-review
- `a`: Approve PR
- `c`: Request changes
- `t`: Toggle resolve/unresolve thread
- `esc`: Deselect PR → switch to default tab

### Actions Tab
- `j/k`: Navigate workflows/runs
- `enter`: Drill into run → jobs → steps (preview shows logs)
- `r`: Re-run failed jobs
- `c`: Cancel running workflow
- `l`: View full logs (opens in preview pane)
- `d`: Download artifacts
- `esc`: Back up one level → switch to default tab

## Messages (`internal/panels/messages.go`)

New cross-panel message types:

```go
IssueSelectedMsg{Number int, Title string, Body string, State string}
IssueDeselectedMsg{}
IssueCommentAddedMsg{Number int, Body string}
IssueStateChangedMsg{Number int, State string}
PRSelectedMsg{Number int, Title string, State string, HeadBranch string}
PRDeselectedMsg{}
PRReviewSubmittedMsg{Number int, ReviewState string}
PRCommentAddedMsg{Number int, ThreadID int64, Body string}
PRThreadResolvedMsg{Number int, ThreadID int64}
PRMergedMsg{Number int, Strategy string}
PRFilesLoadedMsg{Number int, Files []PRFile}
PRCommitsLoadedMsg{Number int, Commits []Commit}
ActionRunSelectedMsg{RunID int64, WorkflowName string, Status string}
ActionRunDeselectedMsg{}
ActionJobsLoadedMsg{RunID int64, Jobs []Job}
ActionLogMsg{RunID int64, JobID int64, Log string}
GitHubContextMsg{Owner string, Repo string}
GitHubFilterChangedMsg{Tab string, Filter string}
GitHubUserMsg{Login string}
```

**Cross-panel flows**:
- `PRSelectedMsg` → filetree enters PR-files mode (like commit-files mode)
- `PRCommitsLoadedMsg` → commits panel shows PR commits
- `PRDeselectedMsg` → filetree/commits restore to normal
- `IssueSelectedMsg` → preview shows issue body; chat gets issue context
- `IssueDeselectedMsg` → preview restores to file preview
- `ActionRunSelectedMsg` → preview shows job steps + logs
- `ActionRunDeselectedMsg` → preview restores
- `GitHubFilterChangedMsg` → refresh list with new filter
- `GitHubUserMsg` → used by quick filters (Assigned, Needs Review)

## Chat Tools (`internal/chat/`)

### Issue tools
- `gh_issue_list`: List/search issues with filters
- `gh_issue_view`: Get full issue with comments
- `gh_issue_create`: Create new issue (requires confirmation)
- `gh_issue_comment`: Add comment to issue (requires confirmation)
- `gh_issue_edit`: Edit issue metadata (requires confirmation)
- `gh_issue_close`: Close issue (requires confirmation)
- `gh_issue_analyze`: AI reads issue + codebase, suggests fix approach and affected files

### PR tools
- `gh_pr_list`: List/search PRs with filters
- `gh_pr_view`: Get PR with diff, comments, reviews
- `gh_pr_create`: Create PR from current branch (requires confirmation)
- `gh_pr_comment`: Add comment to PR (requires confirmation)
- `gh_pr_review`: Submit review (approve/request changes/comment) (requires confirmation)
- `gh_pr_merge`: Merge PR (requires confirmation)
- `gh_pr_request_reviewer`: Request reviewer (requires confirmation)
- `gh_pr_resolve_thread`: Resolve/unresolve review thread (requires confirmation)
- `gh_pr_fix`: AI reads review comments, generates code fixes, applies them
- `gh_pr_respond`: AI drafts response to review comment based on codebase analysis
- `gh_pr_describe`: AI generates PR description from commits and diff

### Actions tools
- `gh_actions_list`: List workflow runs
- `gh_actions_view`: Get run details with job/step status
- `gh_actions_logs`: Get job logs
- `gh_actions_rerun`: Re-run failed jobs (requires confirmation)
- `gh_actions_debug`: AI analyzes failure logs, suggests fixes

### Notification tools
- `gh_notif_list`: List notifications
- `gh_notif_mark_read`: Mark notification read

## AI Workflows

### 1. Issue triage
User selects an issue → presses `?` or asks chat → AI analyzes the issue against codebase:
- Identifies affected files/functions
- Suggests labels (bug, feature, enhancement)
- Estimates complexity
- Suggests assignee based on git blame of affected files
- Proposes implementation approach

### 2. PR review assist
User selects a PR → asks chat "review this PR" → AI:
- Reads the full diff
- Checks for common issues (security, performance, style)
- Generates review comments with file:line references
- User approves/edits each comment before submission

### 3. Comment response
User views a review comment → asks chat "respond to this" → AI:
- Reads the comment + surrounding code context
- Checks if the suggestion is valid
- Either: generates a code fix + response, or drafts a respectful disagreement with rationale
- User approves before submitting

### 4. CI failure fix
User sees a failed action run → asks chat "fix this" → AI:
- Fetches failure logs
- Identifies the failing step and error
- Cross-references with recent commits
- Generates a code fix
- User reviews and applies

### 5. Issue-to-PR pipeline
User selects an issue → asks chat "fix this issue" → AI:
- Analyzes the issue
- Creates a branch
- Implements the fix
- Runs tests
- Creates a PR with description referencing the issue
- User reviews each step

## Configuration (`internal/config/`)

```toml
[github]
# Auto-detected from git remote; override if needed
owner = ""
repo = ""

# Polling interval for notifications/actions (seconds, 0=manual only)
poll_interval = 60

# Default filters
default_issue_filter = "all"   # all, assigned, mentioned, created
default_pr_filter = "all"      # all, needs_review, mine, draft

# Review preferences
auto_checkout_pr_branch = true
review_diff_context_lines = 3
```

## Implementation Phases

### Phase 1: Foundation
- `internal/github/client.go`: Client wrapping `google/go-github`
- `internal/github/interfaces.go`: Compositional interfaces (IssueReader, PRReader, etc.)
- `internal/github/auth.go`: `gh auth token` bootstrap with `GITHUB_TOKEN` fallback
- `internal/github/types.go`: App-specific view model types
- `internal/github/cache.go`: In-memory cache with TTL
- `internal/github/exec.go`: `ghExec()` escape hatch for CLI-only ops
- `internal/github/client_test.go`: Tests with httptest mock server
- Config: `[github]` section in config
- `go.mod`: Add `github.com/google/go-github/v68`
- Message types in `messages.go`

### Phase 2: GitInfo Tab Bar Extension
- Add second tab row to `gitinfo.go` for Issues, PRs, Actions
- Fetch counts on startup (issue count, PR count, latest action status)
- Tab switching between git and GitHub rows (only one active)
- `GitHubUserMsg` for authenticated user identity (needed by filters)
- Background goroutine for periodic count refresh

### Phase 3: Issues Tab
- Issue list rendering in gitinfo pane (number, title, labels)
- `IssueSelectedMsg` → preview shows issue body as rendered markdown
- Quick filters: All / Assigned / Mentioned / Created
- Search with `/`
- Mouse click support
- Chat context injection (selected issue available to AI)
- Chat tools: `gh_issue_list`, `gh_issue_view`, `gh_issue_create`, `gh_issue_comment`, `gh_issue_edit`, `gh_issue_close`

### Phase 4: PRs Tab
- PR list rendering (number, title, author, status, review state)
- `PRSelectedMsg` → filetree enters PR-files mode, commits show PR commits, preview shows diff
- Quick filters: All / Needs Review / Mine / Draft
- Cross-panel wiring (reuses commit-files mode pattern)
- `PRDeselectedMsg` → restore all panels
- Chat tools: `gh_pr_list`, `gh_pr_view`, `gh_pr_create`, `gh_pr_comment`, `gh_pr_review`, `gh_pr_merge`, `gh_pr_request_reviewer`, `gh_pr_resolve_thread`

### Phase 5: Actions Tab
- Workflow run list rendering (status icon, name, branch, time ago)
- `ActionRunSelectedMsg` → preview shows job steps + logs
- Drill-down: run → jobs → steps
- Re-run / cancel actions
- Chat tools: `gh_actions_list`, `gh_actions_view`, `gh_actions_logs`, `gh_actions_rerun`

### Phase 6: AI Workflows + Polish
- Chat tools: `gh_issue_analyze`, `gh_pr_fix`, `gh_pr_respond`, `gh_pr_describe`, `gh_actions_debug`
- AI operation wrappers in `internal/ai/ops/`
- Background polling for notifications/actions status
- Keyboard shortcut refinements
- Review preset enhancements

## Non-Goals (Out of Scope)
- Separate GitHub panels (integrated into gitinfo instead)
- GitHub preset layout (no mode switch — always integrated)
- GitHub Projects board view (future)
- GitHub Discussions (future)
- GitHub Releases management (future)
- GitHub wiki editing (future)
- Multi-repo support in a single session (future)
- OAuth flow — rely on `gh auth` being pre-configured
- Notifications panel as a separate pane (future — could be integrated similar to issues/PRs)
