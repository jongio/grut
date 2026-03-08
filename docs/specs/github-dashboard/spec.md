# GitHub Dashboard Mode

## Problem

GRUT is currently repo-scoped: it always launches in the context of a specific git repository. Users who want a bird's-eye view of their GitHub activity — open PRs across all repos, assigned issues, review requests, notifications — must leave the terminal and visit github.com or use multiple `gh` CLI commands.

There is no way to launch GRUT as a general-purpose GitHub dashboard that aggregates activity across all of a user's repositories.

## Proposed Solution

Add a **dashboard mode** to GRUT that shows the authenticated user's cross-repo GitHub activity in a dedicated set of panels. This mode activates automatically when GRUT is launched outside a git repository, or explicitly via `grut --dashboard`.

### Core Principle

Dashboard mode uses the **GitHub API only** — no git client required. The panels query the authenticated user's activity across all repos they have access to. When the user drills into a specific PR or issue, GRUT renders details in the preview panel (markdown). If the repo is cloned locally, GRUT offers to switch context into that repo for full git integration.

## User Experience

### Launch Behavior

| Context | Behavior |
|---------|----------|
| `grut` inside a git repo | Current behavior (file explorer + git panels) |
| `grut` outside a git repo | Dashboard mode (cross-repo GitHub panels) |
| `grut --dashboard` anywhere | Force dashboard mode regardless of git context |
| `grut --dashboard` inside a repo | Dashboard mode, but repo panels available via layout switch |

### Dashboard Layout

Default preset: `DashboardPreset`

```
┌─────────────────────┬──────────────────────────────┐
│                     │                              │
│   PR List           │   Preview                    │
│   (left panel)      │   (PR/issue body rendered    │
│                     │    as markdown, diff view,   │
│                     │    comments thread)           │
│                     │                              │
├─────────────────────┤                              │
│                     │                              │
│   Issue List        │                              │
│   (bottom-left)     │                              │
│                     │                              │
└─────────────────────┴──────────────────────────────┘
```

Alternative layouts accessible via existing layout switching:
- **PRs focused**: Full-width PR list + preview
- **Issues focused**: Full-width issue list + preview
- **Notifications**: Notification list + preview
- **Combined**: All three lists stacked + preview

## Panels

### 1. Dashboard PR Panel (`dashprs`)

Shows all PRs relevant to the authenticated user across all repos.

**Data sources** (GitHub API):
- `GET /search/issues?q=author:@me+type:pr+state:open` — PRs I authored
- `GET /search/issues?q=review-requested:@me+type:pr+state:open` — PRs requesting my review
- `GET /search/issues?q=mentions:@me+type:pr+state:open` — PRs mentioning me

**Display columns**:
- Status indicator: `●` draft, `○` open, `✓` merged, `✗` closed
- Review state: approved/changes-requested/pending (from PR reviews API)
- CI status: `✓` passing, `✗` failing, `○` pending
- Repository: `owner/repo` (short form)
- Title (truncated to fit)
- Updated time (relative: "2h ago", "3d ago")
- Author (for review-requested view)

**Tabs/filters** (cycle with Tab):
- **My PRs** — authored by me
- **Review Requests** — assigned to me for review
- **Mentioned** — PRs where I'm mentioned

**Interactions**:
- `Enter` — Load PR details + diff in preview panel
- `o` — Open in browser (`Start-Process` / `open` / `xdg-open`)
- `c` — Checkout PR branch locally (if repo is cloned; prompt for clone location if not)
- `/` — Filter/search within current list
- `r` — Refresh data
- `1`/`2`/`3` — Switch between My PRs / Review Requests / Mentioned tabs

**Sorting**: By updated time (most recent first), toggleable to created time.

### 2. Dashboard Issue Panel (`dashissues`)

Shows issues assigned to or created by the authenticated user.

**Data sources** (GitHub API):
- `GET /issues?filter=assigned&state=open` — Issues assigned to me
- `GET /issues?filter=created&state=open` — Issues I created
- `GET /issues?filter=mentioned&state=open` — Issues mentioning me

**Display columns**:
- State: `○` open, `✓` closed
- Labels (first 2, color-rendered via lipgloss)
- Repository: `owner/repo`
- Title
- Updated time (relative)
- Assignee (for created-by view)

**Tabs/filters**:
- **Assigned** — assigned to me
- **Created** — created by me
- **Mentioned** — mentioning me

**Interactions**:
- `Enter` — Load issue body + comments in preview panel
- `o` — Open in browser
- `/` — Filter/search
- `r` — Refresh

### 3. Dashboard Notifications Panel (`dashnotify`)

Shows GitHub notifications for the authenticated user.

**Data source**:
- `GET /notifications` — All notifications
- `GET /notifications?participating=true` — Only participating

**Display columns**:
- Unread indicator: `●` unread, `○` read
- Type icon: PR / Issue / Release / Discussion
- Repository: `owner/repo`
- Title
- Reason: review_requested, mention, assign, ci_activity, etc.
- Updated time

**Interactions**:
- `Enter` — Load referenced PR/issue in preview
- `o` — Open in browser
- `m` — Mark as read
- `M` — Mark all as read
- `r` — Refresh

### 4. Preview Panel (Enhanced)

The existing preview panel (`internal/panels/preview/`) gains a new rendering mode for GitHub content:

- **PR preview**: Title, body (markdown rendered), file list, review status, CI checks, comments thread
- **Issue preview**: Title, body (markdown rendered), labels, assignees, comments thread
- **Diff preview**: When viewing a PR, `d` key switches to diff view (unified diff of the PR)

This mode already has partial support via the existing `GitHub content mode` in preview.

## Configuration

```toml
[dashboard]
# Auto-launch dashboard when not in a git repo
auto_launch = true

# Default active tab when dashboard opens
default_view = "prs"    # "prs" | "issues" | "notifications"

# Refresh interval for dashboard data (seconds)
refresh_interval = 120

# Maximum items to fetch per category
max_items = 100

# Show repos from these orgs/users (empty = all accessible)
# If empty, queries the authenticated user's activity across everything
include_orgs = []

# Exclude specific repos from dashboard
exclude_repos = []
```

### Repo Context Resolution

When the user selects a PR/issue and wants to interact with the repo:

1. Check if `owner/repo` is cloned locally by scanning:
   - Common directories: `~/code/`, `~/projects/`, `~/repos/`, `~/src/`
   - Configurable scan paths via `[dashboard] local_repo_paths = ["~/code", "~/work"]`
   - Cache discovered repo-to-path mappings for fast lookup
2. If found locally: offer to switch GRUT context to that repo (broadcasts `RepoChangedMsg`)
3. If not found: show preview-only mode (markdown rendering of PR/issue content)

## Architecture

### New Packages

```
internal/panels/dashprs/        # Dashboard PR list panel
internal/panels/dashissues/     # Dashboard issue list panel
internal/panels/dashnotify/     # Dashboard notifications panel
internal/dashboard/             # Dashboard mode controller
  dashboard.go                  # Mode detection, panel coordination
  cache.go                      # API response caching with TTL
  resolver.go                   # Local repo path resolver
```

### Mode Detection (cmd/root.go)

```
1. Parse --dashboard flag
2. If --dashboard: enter dashboard mode
3. Else: attempt git.NewClient(cwd)
   a. If git client succeeds: normal repo mode
   b. If git client fails (not a git repo):
      i.  If dashboard.auto_launch config is true: enter dashboard mode
      ii. If false: show file explorer only (current degraded behavior)
```

### Layout Integration

New preset registered in `layout.RegisterDefaults()`:

```
DashboardPreset:
  Vertical split (0.4):
    First: Vertical split (0.5):
      First:  dashprs
      Second: dashissues
    Second: preview
```

### GitHub Client Changes

The existing `internal/github/client.go` needs minimal changes:
- Currently requires `owner` and `repo` to be set (from config or git remote)
- Dashboard mode creates a client with no owner/repo context
- New methods use user-scoped API endpoints (no repo needed)
- Add methods: `ListMyPRs()`, `ListMyIssues()`, `ListNotifications()`, `MarkNotificationRead()`

### Authentication

No changes needed. The existing auth flow (`gh auth token` or `GITHUB_TOKEN` env var) works for user-scoped API calls. If no auth is available, dashboard mode shows an error with setup instructions.

### Caching Strategy

- **TTL-based**: Cache API responses with configurable TTL (default: 2 minutes)
- **Background refresh**: Refresh data in background goroutine on timer
- **ETag support**: Use GitHub API conditional requests (`If-None-Match`) to avoid rate limit waste
- **Stale-while-revalidate**: Show cached data immediately, update in background

### Rate Limit Management

- GitHub API: 5000 requests/hour (authenticated)
- Dashboard refresh: 3 API calls per refresh cycle (PRs + Issues + Notifications)
- At 2-minute intervals: ~90 calls/hour (well within limits)
- ETag conditional requests don't count against rate limit when returning 304

## Implementation Phases

### Phase 1: Core Dashboard (v1-compatible)

Read-only dashboard panels that work within the current single-tab architecture.

1. Create `dashprs` panel with PR list and tab switching
2. Create `dashissues` panel with issue list
3. Enhance preview panel for PR/issue rendering
4. Add dashboard mode detection in `cmd/root.go`
5. Add `DashboardPreset` layout
6. Add `[dashboard]` config section
7. Add `--dashboard` CLI flag
8. GitHub client: add user-scoped query methods

**Outcome**: User can launch `grut --dashboard` or launch `grut` outside a git repo and see their PRs and issues across all repos.

### Phase 2: Notifications + Interactions

1. Create `dashnotify` panel
2. Add "open in browser" action for all dashboard panels
3. Add "checkout PR branch" flow (with local repo resolution)
4. Add mark-as-read for notifications
5. Add background refresh with configurable interval
6. Add ETag caching for API efficiency

### Phase 3: Repo Context Switching (v2 multi-tab)

1. Local repo path resolver (scan configurable directories)
2. "Switch to repo" action that transitions from dashboard to repo mode
3. Multi-tab support: keep dashboard in one tab, repo in another
4. Back-navigation: return to dashboard from repo context

## Open Questions

1. **Should dashboard mode support write operations?** (e.g., close issue, merge PR, submit review) — Recommendation: Phase 1 is read-only. Phase 2 adds browser-open. Phase 3 considers write ops with confirmation dialogs.

2. **Should we support GitHub Enterprise?** — The `google/go-github` client already supports custom base URLs. Config: `[github] api_url = "https://github.example.com/api/v3"`. Low effort to support.

3. **Should dashboard panels be available in repo mode too?** — Yes, as optional panels in the registry. User could add `dashprs` to any layout via the panel system. This gives repo-mode users quick access to their cross-repo PR list without leaving their current context.

4. **Keyboard shortcut to toggle between dashboard and repo mode?** — Consider `Ctrl+D` for dashboard toggle when in a git repo. When outside a git repo, dashboard is the only available mode.

## Non-Goals

- Full GitHub web UI replacement (no project boards, no wiki, no Actions management)
- Repository browsing without local clone (use `gh` CLI for that)
- Multi-account support (single authenticated user only)
- Offline mode (requires GitHub API access)

## Success Criteria

- Launch `grut` outside any git repo → dashboard appears with user's PRs and issues
- Launch `grut --dashboard` inside a git repo → dashboard appears
- PRs show status, CI state, review state, and repo name
- Issues show labels, assignees, and repo name
- Selecting a PR/issue renders full details in preview panel
- `o` key opens selected item in browser
- Data refreshes automatically on configurable interval
- API calls stay well within GitHub rate limits
- No regression in normal repo-mode functionality
