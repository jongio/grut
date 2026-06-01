// GitHub integration: issues, PRs, actions, workflows, and releases.
package gitinfo

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	gh "github.com/google/go-github/v68/github"
	ghclient "github.com/jongio/grut/internal/github"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/mattn/go-runewidth"
)

// ghIssueItem holds display data for a GitHub issue.
type ghIssueItem struct {
	Title    string
	Body     string
	State    string
	Author   string // issue creator login
	Assignee string // first assignee login
	HTMLURL  string // GitHub web URL
	Labels   []string
	Number   int
}

// ghPRItem holds display data for a GitHub pull request.
type ghPRItem struct {
	Title            string
	State            string // "open", "closed", "merged", "draft"
	HeadBranch       string
	Author           string
	HTMLURL          string // GitHub web URL
	Number           int
	MergeableState   string // "clean", "dirty", "unstable", "blocked", "unknown", ""
	ActionStatus     string // matched action run status: "success", "failure", "in_progress", "queued", ""
	ActionConclusion string // matched action run conclusion
}

// prStateMerged is the canonical value for a merged pull request state.
const prStateMerged = "merged"

// prStateOpen is the canonical value for an open pull request state.
const prStateOpen = "open"

// ghActionItem holds display data for a GitHub Actions workflow run.
type ghActionItem struct {
	WorkflowName string
	Status       string
	Conclusion   string
	Branch       string
	CreatedAt    string
	HTMLURL      string // GitHub web URL
	RunID        int64
	RunNumber    int
}

// ghWorkflowItem holds display data for a GitHub Actions workflow definition.
type ghWorkflowItem struct {
	Name    string
	Path    string // e.g. ".github/workflows/ci.yml"
	State   string // "active", "disabled_manually", etc.
	HTMLURL string // GitHub web URL
	ID      int64
}

// ghReleaseItem holds display data for a GitHub release.
type ghReleaseItem struct {
	TagName     string
	Name        string
	Author      string
	CreatedAt   string
	Body        string
	HTMLURL     string // GitHub web URL
	ID          int64
	AssetsCount int
	Draft       bool
	Prerelease  bool
}

// prDetailsLoadedMsg carries asynchronously-fetched PR files and commits.
type prDetailsLoadedMsg struct {
	err     error
	files   []panels.PRFile
	commits []panels.PRCommit
	number  int
}

// actionJobsLoadedMsg carries asynchronously-fetched jobs for a workflow run.
type actionJobsLoadedMsg struct {
	err   error
	jobs  []panels.ActionJob
	runID int64
}

// actionLogLoadedMsg carries log output for a failed job.
type actionLogLoadedMsg struct {
	err   error
	log   string
	runID int64
	jobID int64
}

// actionRerunResultMsg carries the result of a rerun-failed-jobs operation.
type actionRerunResultMsg struct {
	err   error
	runID int64
}

// actionCancelResultMsg carries the result of a cancel-workflow-run operation.
type actionCancelResultMsg struct {
	err   error
	runID int64
}

// workflowDispatchResultMsg carries the result of a workflow dispatch operation.
type workflowDispatchResultMsg struct {
	err          error
	workflowName string
}

// prMergeResultMsg carries the result of a PR merge operation.
type prMergeResultMsg struct {
	number     int
	strategy   string
	headBranch string
	err        error
}

// prBranchDeleteResultMsg carries the result of deleting a branch after PR merge.
type prBranchDeleteResultMsg struct {
	branch    string
	remoteErr error
	localErr  error
}

// workflowInputsFetchedMsg carries the result of fetching workflow_dispatch
// input definitions from the workflow YAML file.
type workflowInputsFetchedMsg struct {
	workflowName string
	ref          string
	inputs       []ghclient.WorkflowInput
	workflowID   int64
}

// githubPollTickMsg triggers periodic GitHub data refresh.
type githubPollTickMsg struct{ time.Time }

// actionsWatchTickMsg triggers the next animation frame for the
// "watching CI" indicator in the Actions tab bar.
type actionsWatchTickMsg struct{ time.Time }

// watchFrames defines the 4-frame cycle for the CI watch animation.
var watchFrames = []string{runDot, "◐", "○", "◑"}

// checkMark is the success icon used in status indicators.
const checkMark = "✓"

// crossMark is the failure icon used in status indicators.
const crossMark = "✗"

// runDot is the running/in-progress icon used in status indicators.
const runDot = "●"

// GitHub Actions conclusion strings used in switch cases.
const (
	conclusionSuccess  = "success"
	conclusionFailure  = "failure"
	conclusionTimedOut = "timed_out"
)

// GitHub Actions status strings used in run-state checks.
const (
	statusInProgress = "in_progress"
	statusQueued     = "queued"
)

// actionsWatchTickInterval is the polling interval for the GitHub Actions
// watch animation frame rate.
const actionsWatchTickInterval = 1000 * time.Millisecond

// ghLoadMoreThreshold triggers a page fetch when the cursor is within
// this many items of the end of the loaded list.
const ghLoadMoreThreshold = 5

// ghDebounceInterval prevents rapid-fire API calls during fast scrolling.
const ghDebounceInterval = 200 * time.Millisecond

// tabPagination tracks lazy-loading state for one GitHub tab.
type tabPagination struct {
	lastLoadAt time.Time // debounce timer
	nextPage   int       // next API page number (0 = no more)
	loading    bool      // true while a page fetch is in flight
	allLoaded  bool      // true when the last page has been reached
}

// IssueFilterKind identifies the active quick-filter for the Issues tab.
type IssueFilterKind int

const (
	issueFilterAll      IssueFilterKind = iota
	issueFilterAssigned                 // assignee == current user
	issueFilterCreated                  // author == current user
)

func (f IssueFilterKind) String() string {
	switch f {
	case issueFilterAssigned:
		return "Assigned"
	case issueFilterCreated:
		return "Created"
	default:
		return labelAll
	}
}

// PRFilterKind identifies the active quick-filter for the PRs tab.
type PRFilterKind int

const (
	prFilterAll         PRFilterKind = iota
	prFilterNeedsReview              // open PRs not authored by current user
	prFilterMine                     // author == current user
	prFilterDraft                    // draft PRs
)

func (f PRFilterKind) String() string {
	switch f {
	case prFilterNeedsReview:
		return "Needs Review"
	case prFilterMine:
		return "Mine"
	case prFilterDraft:
		return labelDraft
	default:
		return labelAll
	}
}

// ghDataLoadedMsg carries the result of an async GitHub data load.
type ghDataLoadedMsg struct {
	err         error
	user        string
	issues      []ghIssueItem
	prs         []ghPRItem
	actions     []ghActionItem
	workflows   []ghWorkflowItem
	releases    []ghReleaseItem
	repoPrivate bool
}

// ghMetaLoadedMsg carries repo metadata and current user info.
type ghMetaLoadedMsg struct {
	user        string
	repoPrivate bool
}

// ghIssuesPageMsg carries one page of issues from the GitHub API.
type ghIssuesPageMsg struct {
	issues   []ghIssueItem
	nextPage int
	replace  bool // true = first page (replace all), false = append
}

// ghPRsPageMsg carries one page of PRs from the GitHub API.
type ghPRsPageMsg struct {
	prs      []ghPRItem
	nextPage int
	replace  bool
}

// ghActionsPageMsg carries one page of workflow runs from the GitHub API.
type ghActionsPageMsg struct {
	actions  []ghActionItem
	nextPage int
	replace  bool
}

// ghWorkflowsPageMsg carries one page of workflow definitions.
type ghWorkflowsPageMsg struct {
	workflows []ghWorkflowItem
	nextPage  int
	replace   bool
}

// ghReleasesPageMsg carries one page of releases.
type ghReleasesPageMsg struct {
	releases []ghReleaseItem
	nextPage int
	replace  bool
}

// ---------------------------------------------------------------------------
// GitHub tab bar click handling
// ---------------------------------------------------------------------------
// handleGitHubTabBarClick switches the active tab based on column position
// within the GitHub tab row. In ModeGitHub the layout includes Branches and
// Tags: " Branches N · Tags N · Issues N · PRs N · Actions X · Workflows N · Releases N".
// In ModeAll: " Issues N · PRs N · Actions X · Workflows N · Releases N".
func (p *Panel) handleGitHubTabBarClick(col int) {
	// Build tab definitions matching the render order.
	type tabEntry struct {
		name, short string
		count       string
		id          tabID
	}
	var tabs []tabEntry
	// In ModeGitHub, Branches and Tags are prepended to the tab row.
	if p.mode == ModeGitHub {
		tabs = append(
			tabs,
			tabEntry{id: tabBranches, name: labelBranches, short: "Br", count: fmt.Sprintf("%d", len(p.tabItems[tabBranches]))},
			tabEntry{id: tabTags, name: labelTags, short: "Tg", count: fmt.Sprintf("%d", len(p.tabItems[tabTags]))},
		)
	}
	issuesCount := fmt.Sprintf("%d", len(p.tabItems[tabIssues]))
	if p.gh.issueFilter != issueFilterAll {
		issuesCount = p.gh.issueFilter.String()
	}
	prsCount := fmt.Sprintf("%d", len(p.tabItems[tabPRs]))
	if p.gh.prFilter != prFilterAll {
		prsCount = p.gh.prFilter.String()
	}
	tabs = append(
		tabs,
		tabEntry{id: tabIssues, name: labelIssues, short: "Iss", count: issuesCount},
		tabEntry{id: tabPRs, name: labelPRs, short: shortPRs, count: prsCount},
		tabEntry{id: tabActions, name: labelActions, short: shortAct, count: p.actionsStatusIcon()},
		tabEntry{id: tabWorkflows, name: "Workflows", short: "Wf", count: fmt.Sprintf("%d", len(p.tabItems[tabWorkflows]))},
		tabEntry{id: tabReleases, name: "Releases", short: "Rel", count: fmt.Sprintf("%d", len(p.tabItems[tabReleases]))},
	)
	// Determine whether abbreviations are active (same logic as renderRow).
	plain := make([]struct{ name, short, count string }, len(tabs))
	for i, t := range tabs {
		plain[i] = struct{ name, short, count string }{t.name, t.short, t.count}
	}
	useShort := tabRowUseShort(plain, p.lastWidth)
	pos := 1 // leading space
	for i, t := range tabs {
		w := p.ghTabLabelWidth(t.name, t.short, t.count, useShort)
		end := pos + w
		if col >= pos && col < end {
			p.activeTab = t.id
			return
		}
		if i < len(tabs)-1 {
			pos = end + 3 // " · " separator
		}
	}
}

// ghTabLabelWidth returns the display width of a tab label "Name count",
// using abbreviated names when the tab bar would be too wide.
func (p *Panel) ghTabLabelWidth(name, short, count string, useShort bool) int {
	if useShort && short != "" {
		name = short
	}
	return runewidth.StringWidth(fmt.Sprintf("%s %s", name, count))
}

// tabRowUseShort returns true when tab labels should be abbreviated to fit
// the available width. It mirrors the abbreviation logic in renderRow.
func tabRowUseShort(tabs []struct{ name, short, count string }, width int) bool {
	fullWidth := 1 // leading space
	for i, t := range tabs {
		fullWidth += runewidth.StringWidth(t.name) + 1 + runewidth.StringWidth(t.count) // "Name count"
		if i < len(tabs)-1 {
			fullWidth += 3 // " · "
		}
	}
	return fullWidth > width
}

// ---------------------------------------------------------------------------
// GitHub data loading
// ---------------------------------------------------------------------------
// loadGitHubData returns a tea.Cmd that fetches issues, PRs, and Actions
// from the GitHub API asynchronously.

func (p *Panel) loadGitHubData() tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		var result ghDataLoadedMsg
		// Fetch repo metadata (visibility).
		repoInfo, err := client.RepoInfo(ctx, owner, repo)
		if err != nil {
			slog.Warn("github: fetch repo info failed", "owner", owner, "repo", repo, "err", err)
		} else if repoInfo != nil {
			result.repoPrivate = repoInfo.GetPrivate()
		}
		// Get current user.
		user, err := client.CurrentUser(ctx)
		if err != nil {
			slog.Warn("github: fetch current user failed", "err", err)
		} else if user != nil && user.Login != nil {
			result.user = *user.Login
		}
		// Fetch issues (first page, open).
		issues, err := client.ListIssues(ctx, owner, repo, &gh.IssueListByRepoOptions{
			State:       prStateOpen,
			ListOptions: gh.ListOptions{PerPage: 50},
		})
		if err != nil {
			slog.Warn("github: fetch issues failed", "owner", owner, "repo", repo, "err", err)
		} else {
			for _, iss := range issues {
				if iss.PullRequestLinks != nil {
					continue // skip PRs returned in issue list
				}
				var labels []string
				for _, l := range iss.Labels {
					if l.Name != nil {
						labels = append(labels, *l.Name)
					}
				}
				author := ""
				if iss.User != nil {
					author = iss.User.GetLogin()
				}
				assignee := ""
				if len(iss.Assignees) > 0 && iss.Assignees[0] != nil {
					assignee = iss.Assignees[0].GetLogin()
				}
				result.issues = append(result.issues, ghIssueItem{
					Number:   iss.GetNumber(),
					Title:    iss.GetTitle(),
					Body:     iss.GetBody(),
					State:    iss.GetState(),
					Labels:   labels,
					Author:   author,
					Assignee: assignee,
					HTMLURL:  iss.GetHTMLURL(),
				})
			}
		}
		// Fetch PRs.
		prs, err := client.ListPRs(ctx, owner, repo, &gh.PullRequestListOptions{
			State:       prStateOpen,
			ListOptions: gh.ListOptions{PerPage: 50},
		})
		if err != nil {
			slog.Warn("github: fetch PRs failed", "owner", owner, "repo", repo, "err", err)
		} else {
			for _, pr := range prs {
				state := pr.GetState()
				if pr.GetDraft() {
					state = stateDraft //nolint:goconst // inline string is more readable here
				}
				if pr.GetMerged() {
					state = prStateMerged
				}
				author := ""
				if pr.User != nil {
					author = pr.User.GetLogin()
				}
				result.prs = append(result.prs, ghPRItem{
					Number:     pr.GetNumber(),
					Title:      pr.GetTitle(),
					State:      state,
					HeadBranch: pr.GetHead().GetRef(),
					Author:     author,
					HTMLURL:    pr.GetHTMLURL(),
				})
			}
		}
		// Fetch action runs.
		runs, err := client.ListWorkflowRuns(ctx, owner, repo, &gh.ListWorkflowRunsOptions{
			ListOptions: gh.ListOptions{PerPage: 20},
		})
		if err != nil {
			slog.Warn("github: fetch workflow runs failed", "owner", owner, "repo", repo, "err", err)
		} else {
			for _, run := range runs {
				result.actions = append(result.actions, ghActionItem{
					RunID:        run.GetID(),
					WorkflowName: run.GetName(),
					RunNumber:    run.GetRunNumber(),
					Status:       run.GetStatus(),
					Conclusion:   run.GetConclusion(),
					Branch:       run.GetHeadBranch(),
					CreatedAt:    run.GetCreatedAt().Format("Jan 2 15:04"),
					HTMLURL:      run.GetHTMLURL(),
				})
			}
		}
		// Cross-reference action runs to PRs by head branch.
		if len(result.prs) > 0 && len(result.actions) > 0 {
			actionByBranch := make(map[string]ghActionItem, len(result.actions))
			for _, action := range result.actions {
				if _, exists := actionByBranch[action.Branch]; !exists {
					actionByBranch[action.Branch] = action // first = most recent
				}
			}
			for i, pr := range result.prs {
				if action, ok := actionByBranch[pr.HeadBranch]; ok {
					result.prs[i].ActionStatus = action.Status
					result.prs[i].ActionConclusion = action.Conclusion
				}
			}
		}

		// Fetch workflow definitions.
		workflows, err := client.ListWorkflows(ctx, owner, repo, &gh.ListOptions{PerPage: 50})
		if err != nil {
			slog.Warn("github: fetch workflows failed", "owner", owner, "repo", repo, "err", err)
		} else {
			for _, wf := range workflows {
				result.workflows = append(result.workflows, ghWorkflowItem{
					ID:      wf.GetID(),
					Name:    wf.GetName(),
					Path:    wf.GetPath(),
					State:   wf.GetState(),
					HTMLURL: wf.GetHTMLURL(),
				})
			}
		}
		// Fetch releases.
		releases, err := client.ListReleases(ctx, owner, repo, &gh.ListOptions{PerPage: 30})
		if err != nil {
			slog.Warn("github: fetch releases failed", "owner", owner, "repo", repo, "err", err)
		} else {
			for _, rel := range releases {
				author := ""
				if rel.Author != nil {
					author = rel.Author.GetLogin()
				}
				assetsCount := 0
				if rel.Assets != nil {
					assetsCount = len(rel.Assets)
				}
				name := rel.GetName()
				if name == "" {
					name = rel.GetTagName()
				}
				result.releases = append(result.releases, ghReleaseItem{
					ID:          rel.GetID(),
					TagName:     rel.GetTagName(),
					Name:        name,
					Author:      author,
					CreatedAt:   rel.GetCreatedAt().Format("Jan 2 15:04"),
					Draft:       rel.GetDraft(),
					Prerelease:  rel.GetPrerelease(),
					Body:        rel.GetBody(),
					AssetsCount: assetsCount,
					HTMLURL:     rel.GetHTMLURL(),
				})
			}
		}
		return result
	}
}

func (p *Panel) handleGHDataLoaded(msg ghDataLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		p.gh.err = msg.err
		return p, nil
	}
	if msg.user != "" {
		p.gh.user = msg.user
	}
	p.gh.repoPrivate = msg.repoPrivate
	p.buildGitHubItems(msg.issues, msg.prs, msg.actions, msg.workflows, msg.releases)
	// Determine if any workflow run is still in progress or queued.
	wasWatching := p.actionsWatching
	p.actionsWatching = false
	for _, a := range msg.actions {
		if a.Status == statusInProgress || a.Status == statusQueued {
			p.actionsWatching = true
			break
		}
	}
	// Start the watch animation tick when transitioning to watching state.
	if p.actionsWatching && !wasWatching {
		p.actionsWatchFrame = 0
		return p, p.actionsWatchTickCmd()
	}
	return p, nil
}

// buildGitHubItems constructs listItem slices for the GitHub tabs.
func (p *Panel) buildGitHubItems(issues []ghIssueItem, prs []ghPRItem, actionRuns []ghActionItem, workflows []ghWorkflowItem, releases []ghReleaseItem) {
	p.gh.allIssues = issues
	p.gh.allPRs = prs
	p.applyIssueFilter()
	p.applyPRFilter()
	p.tabItems[tabActions] = nil
	for _, action := range actionRuns {
		p.tabItems[tabActions] = append(p.tabItems[tabActions], listItem{
			kind:      kindActionRun,
			actionRun: action,
		})
	}
	p.tabItems[tabWorkflows] = nil
	for _, wf := range workflows {
		p.tabItems[tabWorkflows] = append(p.tabItems[tabWorkflows], listItem{
			kind:     kindWorkflow,
			workflow: wf,
		})
	}
	p.tabItems[tabReleases] = nil
	for _, rel := range releases {
		p.tabItems[tabReleases] = append(p.tabItems[tabReleases], listItem{
			kind:    kindRelease,
			release: rel,
		})
	}
	p.tabCursor[tabActions] = 0
	p.tabOffset[tabActions] = 0
	p.tabCursor[tabWorkflows] = 0
	p.tabOffset[tabWorkflows] = 0
	p.tabCursor[tabReleases] = 0
	p.tabOffset[tabReleases] = 0
}

// ---------------------------------------------------------------------------
// Per-tab pagination loaders
// ---------------------------------------------------------------------------

// loadGitHubMeta fetches repo info and current user asynchronously.
func (p *Panel) loadGitHubMeta() tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		var result ghMetaLoadedMsg
		repoInfo, err := client.RepoInfo(ctx, owner, repo)
		if err != nil {
			slog.Warn("github: fetch repo info failed", "owner", owner, "repo", repo, "err", err)
		} else if repoInfo != nil {
			result.repoPrivate = repoInfo.GetPrivate()
		}
		user, err := client.CurrentUser(ctx)
		if err != nil {
			slog.Warn("github: fetch current user failed", "err", err)
		} else if user != nil && user.Login != nil {
			result.user = *user.Login
		}
		return result
	}
}

// loadIssuesPage fetches a single page of issues.
func (p *Panel) loadIssuesPage(page int, replace bool) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	pageSize := p.gh.pageSize
	return func() tea.Msg {
		issues, pr, err := client.ListIssuesPage(ctx, owner, repo, &gh.IssueListByRepoOptions{
			State:       prStateOpen,
			ListOptions: gh.ListOptions{Page: page, PerPage: pageSize},
		})
		if err != nil {
			slog.Warn("github: fetch issues page failed", "owner", owner, "repo", repo, "page", page, "err", err)
			return ghIssuesPageMsg{nextPage: 0, replace: replace}
		}
		var items []ghIssueItem
		for _, iss := range issues {
			if iss.PullRequestLinks != nil {
				continue // skip PRs returned in issue list
			}
			var labels []string
			for _, l := range iss.Labels {
				if l.Name != nil {
					labels = append(labels, *l.Name)
				}
			}
			author := ""
			if iss.User != nil {
				author = iss.User.GetLogin()
			}
			assignee := ""
			if len(iss.Assignees) > 0 && iss.Assignees[0] != nil {
				assignee = iss.Assignees[0].GetLogin()
			}
			items = append(items, ghIssueItem{
				Number:   iss.GetNumber(),
				Title:    iss.GetTitle(),
				Body:     iss.GetBody(),
				State:    iss.GetState(),
				Labels:   labels,
				Author:   author,
				Assignee: assignee,
				HTMLURL:  iss.GetHTMLURL(),
			})
		}
		return ghIssuesPageMsg{issues: items, nextPage: pr.NextPage, replace: replace}
	}
}

// loadPRsPage fetches a single page of pull requests.
func (p *Panel) loadPRsPage(page int, replace bool) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	pageSize := p.gh.pageSize
	return func() tea.Msg {
		prs, pr, err := client.ListPRsPage(ctx, owner, repo, &gh.PullRequestListOptions{
			State:       prStateOpen,
			ListOptions: gh.ListOptions{Page: page, PerPage: pageSize},
		})
		if err != nil {
			slog.Warn("github: fetch PRs page failed", "owner", owner, "repo", repo, "page", page, "err", err)
			return ghPRsPageMsg{nextPage: 0, replace: replace}
		}
		var items []ghPRItem
		for _, ghPR := range prs {
			state := ghPR.GetState()
			if ghPR.GetDraft() {
				state = stateDraft
			}
			if ghPR.GetMerged() {
				state = prStateMerged
			}
			author := ""
			if ghPR.User != nil {
				author = ghPR.User.GetLogin()
			}
			items = append(items, ghPRItem{
				Number:     ghPR.GetNumber(),
				Title:      ghPR.GetTitle(),
				State:      state,
				HeadBranch: ghPR.GetHead().GetRef(),
				Author:     author,
				HTMLURL:    ghPR.GetHTMLURL(),
			})
		}
		return ghPRsPageMsg{prs: items, nextPage: pr.NextPage, replace: replace}
	}
}

// loadActionsPage fetches a single page of workflow runs.
func (p *Panel) loadActionsPage(page int, replace bool) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	pageSize := p.gh.pageSize
	return func() tea.Msg {
		runs, pr, err := client.ListWorkflowRunsPage(ctx, owner, repo, &gh.ListWorkflowRunsOptions{
			ListOptions: gh.ListOptions{Page: page, PerPage: pageSize},
		})
		if err != nil {
			slog.Warn("github: fetch actions page failed", "owner", owner, "repo", repo, "page", page, "err", err)
			return ghActionsPageMsg{nextPage: 0, replace: replace}
		}
		var items []ghActionItem
		for _, run := range runs {
			items = append(items, ghActionItem{
				RunID:        run.GetID(),
				WorkflowName: run.GetName(),
				RunNumber:    run.GetRunNumber(),
				Status:       run.GetStatus(),
				Conclusion:   run.GetConclusion(),
				Branch:       run.GetHeadBranch(),
				CreatedAt:    run.GetCreatedAt().Format("Jan 2 15:04"),
				HTMLURL:      run.GetHTMLURL(),
			})
		}
		return ghActionsPageMsg{actions: items, nextPage: pr.NextPage, replace: replace}
	}
}

// loadWorkflowsPage fetches a single page of workflow definitions.
func (p *Panel) loadWorkflowsPage(page int, replace bool) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	pageSize := p.gh.pageSize
	return func() tea.Msg {
		workflows, pr, err := client.ListWorkflowsPage(ctx, owner, repo, &gh.ListOptions{Page: page, PerPage: pageSize})
		if err != nil {
			slog.Warn("github: fetch workflows page failed", "owner", owner, "repo", repo, "page", page, "err", err)
			return ghWorkflowsPageMsg{nextPage: 0, replace: replace}
		}
		var items []ghWorkflowItem
		for _, wf := range workflows {
			items = append(items, ghWorkflowItem{
				ID:      wf.GetID(),
				Name:    wf.GetName(),
				Path:    wf.GetPath(),
				State:   wf.GetState(),
				HTMLURL: wf.GetHTMLURL(),
			})
		}
		return ghWorkflowsPageMsg{workflows: items, nextPage: pr.NextPage, replace: replace}
	}
}

// loadReleasesPage fetches a single page of releases.
func (p *Panel) loadReleasesPage(page int, replace bool) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	pageSize := p.gh.pageSize
	return func() tea.Msg {
		releases, pr, err := client.ListReleasesPage(ctx, owner, repo, &gh.ListOptions{Page: page, PerPage: pageSize})
		if err != nil {
			slog.Warn("github: fetch releases page failed", "owner", owner, "repo", repo, "page", page, "err", err)
			return ghReleasesPageMsg{nextPage: 0, replace: replace}
		}
		var items []ghReleaseItem
		for _, rel := range releases {
			author := ""
			if rel.Author != nil {
				author = rel.Author.GetLogin()
			}
			assetsCount := 0
			if rel.Assets != nil {
				assetsCount = len(rel.Assets)
			}
			name := rel.GetName()
			if name == "" {
				name = rel.GetTagName()
			}
			items = append(items, ghReleaseItem{
				ID:          rel.GetID(),
				TagName:     rel.GetTagName(),
				Name:        name,
				Author:      author,
				CreatedAt:   rel.GetCreatedAt().Format("Jan 2 15:04"),
				Draft:       rel.GetDraft(),
				Prerelease:  rel.GetPrerelease(),
				Body:        rel.GetBody(),
				AssetsCount: assetsCount,
				HTMLURL:     rel.GetHTMLURL(),
			})
		}
		return ghReleasesPageMsg{releases: items, nextPage: pr.NextPage, replace: replace}
	}
}

// ---------------------------------------------------------------------------
// Per-tab page handlers
// ---------------------------------------------------------------------------

// handleMetaLoaded processes repo metadata and user info.

func (p *Panel) handleMetaLoaded(msg ghMetaLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.user != "" {
		p.gh.user = msg.user
	}
	p.gh.repoPrivate = msg.repoPrivate
	return p, nil
}

// handleIssuesPage processes a page of issues.
func (p *Panel) handleIssuesPage(msg ghIssuesPageMsg) (panels.Panel, tea.Cmd) {
	p.tabPaging[tabIssues].loading = false
	p.tabPaging[tabIssues].nextPage = msg.nextPage
	if msg.nextPage == 0 {
		p.tabPaging[tabIssues].allLoaded = true
	}
	savedCursor := p.tabCursor[tabIssues]
	savedOffset := p.tabOffset[tabIssues]
	if msg.replace {
		p.gh.allIssues = msg.issues
	} else {
		p.gh.allIssues = append(p.gh.allIssues, msg.issues...)
	}
	if len(p.gh.allIssues) > ghclient.MaxPaginationItems {
		p.gh.allIssues = p.gh.allIssues[:ghclient.MaxPaginationItems]
		p.tabPaging[tabIssues].allLoaded = true
	}
	p.applyIssueFilter()
	if !msg.replace {
		p.tabCursor[tabIssues] = savedCursor
		p.tabOffset[tabIssues] = savedOffset
	}
	return p, nil
}

// handlePRsPage processes a page of pull requests.
func (p *Panel) handlePRsPage(msg ghPRsPageMsg) (panels.Panel, tea.Cmd) {
	p.tabPaging[tabPRs].loading = false
	p.tabPaging[tabPRs].nextPage = msg.nextPage
	if msg.nextPage == 0 {
		p.tabPaging[tabPRs].allLoaded = true
	}
	savedCursor := p.tabCursor[tabPRs]
	savedOffset := p.tabOffset[tabPRs]
	if msg.replace {
		p.gh.allPRs = msg.prs
	} else {
		p.gh.allPRs = append(p.gh.allPRs, msg.prs...)
	}
	if len(p.gh.allPRs) > ghclient.MaxPaginationItems {
		p.gh.allPRs = p.gh.allPRs[:ghclient.MaxPaginationItems]
		p.tabPaging[tabPRs].allLoaded = true
	}
	p.crossRefPRsActions()
	p.applyPRFilter()
	if !msg.replace {
		p.tabCursor[tabPRs] = savedCursor
		p.tabOffset[tabPRs] = savedOffset
	}
	return p, nil
}

// handleActionsPage processes a page of workflow runs.
func (p *Panel) handleActionsPage(msg ghActionsPageMsg) (panels.Panel, tea.Cmd) {
	p.tabPaging[tabActions].loading = false
	p.tabPaging[tabActions].nextPage = msg.nextPage
	if msg.nextPage == 0 {
		p.tabPaging[tabActions].allLoaded = true
	}
	if msg.replace {
		p.tabItems[tabActions] = nil
	}
	for _, action := range msg.actions {
		p.tabItems[tabActions] = append(p.tabItems[tabActions], listItem{
			kind:      kindActionRun,
			actionRun: action,
		})
	}
	if msg.replace {
		p.tabCursor[tabActions] = 0
		p.tabOffset[tabActions] = 0
	}
	// Cross-reference PRs with newly loaded actions.
	p.crossRefPRsActions()
	// Re-apply PR filter to pick up updated action status.
	savedPRCursor := p.tabCursor[tabPRs]
	savedPROffset := p.tabOffset[tabPRs]
	p.applyPRFilter()
	p.tabCursor[tabPRs] = savedPRCursor
	p.tabOffset[tabPRs] = savedPROffset
	// Update actions watching state.
	wasWatching := p.actionsWatching
	p.actionsWatching = false
	for _, item := range p.tabItems[tabActions] {
		if item.actionRun.Status == statusInProgress || item.actionRun.Status == statusQueued {
			p.actionsWatching = true
			break
		}
	}
	if p.actionsWatching && !wasWatching {
		p.actionsWatchFrame = 0
		return p, p.actionsWatchTickCmd()
	}
	return p, nil
}

// handleWorkflowsPage processes a page of workflow definitions.
func (p *Panel) handleWorkflowsPage(msg ghWorkflowsPageMsg) (panels.Panel, tea.Cmd) {
	p.tabPaging[tabWorkflows].loading = false
	p.tabPaging[tabWorkflows].nextPage = msg.nextPage
	if msg.nextPage == 0 {
		p.tabPaging[tabWorkflows].allLoaded = true
	}
	if msg.replace {
		p.tabItems[tabWorkflows] = nil
	}
	for _, wf := range msg.workflows {
		p.tabItems[tabWorkflows] = append(p.tabItems[tabWorkflows], listItem{
			kind:     kindWorkflow,
			workflow: wf,
		})
	}
	if msg.replace {
		p.tabCursor[tabWorkflows] = 0
		p.tabOffset[tabWorkflows] = 0
	}
	return p, nil
}

// handleReleasesPage processes a page of releases.
func (p *Panel) handleReleasesPage(msg ghReleasesPageMsg) (panels.Panel, tea.Cmd) {
	p.tabPaging[tabReleases].loading = false
	p.tabPaging[tabReleases].nextPage = msg.nextPage
	if msg.nextPage == 0 {
		p.tabPaging[tabReleases].allLoaded = true
	}
	if msg.replace {
		p.tabItems[tabReleases] = nil
	}
	for _, rel := range msg.releases {
		p.tabItems[tabReleases] = append(p.tabItems[tabReleases], listItem{
			kind:    kindRelease,
			release: rel,
		})
	}
	if msg.replace {
		p.tabCursor[tabReleases] = 0
		p.tabOffset[tabReleases] = 0
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Pagination helpers
// ---------------------------------------------------------------------------

// crossRefPRsActions matches action run statuses to PRs by head branch.
func (p *Panel) crossRefPRsActions() {
	if len(p.gh.allPRs) == 0 || len(p.tabItems[tabActions]) == 0 {
		return
	}
	actionByBranch := make(map[string]ghActionItem, len(p.tabItems[tabActions]))
	for _, item := range p.tabItems[tabActions] {
		if item.kind != kindActionRun {
			continue
		}
		if _, exists := actionByBranch[item.actionRun.Branch]; !exists {
			actionByBranch[item.actionRun.Branch] = item.actionRun
		}
	}
	for i, pr := range p.gh.allPRs {
		if action, ok := actionByBranch[pr.HeadBranch]; ok {
			p.gh.allPRs[i].ActionStatus = action.Status
			p.gh.allPRs[i].ActionConclusion = action.Conclusion
		}
	}
}

// loadMoreIfNeeded triggers pagination for the active GitHub tab when the
// cursor or viewport is near the bottom of loaded data.
func (p *Panel) loadMoreIfNeeded() tea.Cmd {
	tab := p.activeTab
	if tab < tabIssues || tab > tabReleases {
		return nil // git tabs don't need pagination
	}
	paging := &p.tabPaging[tab]
	if paging.loading || paging.allLoaded || paging.nextPage == 0 {
		return nil
	}
	items := p.tabItems[tab]
	if len(items) == 0 {
		return nil
	}
	cursor := p.tabCursor[tab]
	offset := p.tabOffset[tab]
	tbh := p.tabBarHeight()
	viewH := p.Height - tbh
	viewEnd := offset + viewH
	triggerIdx := len(items) - ghLoadMoreThreshold
	if cursor < triggerIdx && viewEnd < triggerIdx {
		return nil
	}
	now := time.Now()
	if now.Sub(paging.lastLoadAt) < ghDebounceInterval {
		return nil
	}
	paging.loading = true
	paging.lastLoadAt = now
	switch tab {
	case tabIssues:
		return p.loadIssuesPage(paging.nextPage, false)
	case tabPRs:
		return p.loadPRsPage(paging.nextPage, false)
	case tabActions:
		return p.loadActionsPage(paging.nextPage, false)
	case tabWorkflows:
		return p.loadWorkflowsPage(paging.nextPage, false)
	case tabReleases:
		return p.loadReleasesPage(paging.nextPage, false)
	default:
		// Non-paging tabs (branches, worktrees, remotes, stash, tags, reflog)
		// don't use cursor-based GitHub pagination.
		return nil
	}
}

// ghTabCountStr returns the display count for a GitHub tab, appending "+"

func (p *Panel) ghTabCountStr(tab tabID) string {
	count := fmt.Sprintf("%d", len(p.tabItems[tab]))
	if tab >= tabIssues && tab <= tabReleases && !p.tabPaging[tab].allLoaded && p.tabPaging[tab].nextPage > 0 {
		count += "+"
	}
	return count
}

// ---------------------------------------------------------------------------
// Quick filter cycling
// ---------------------------------------------------------------------------
func (p *Panel) cycleIssueFilter() (panels.Panel, tea.Cmd) {
	p.gh.issueFilter = (p.gh.issueFilter + 1) % 3
	p.applyIssueFilter()
	filter := p.gh.issueFilter.String()
	return p, func() tea.Msg {
		return panels.GitHubFilterChangedMsg{
			Tab:    sectionIssues,
			Filter: filter,
		}
	}
}

func (p *Panel) cyclePRFilter() (panels.Panel, tea.Cmd) {
	p.gh.prFilter = (p.gh.prFilter + 1) % 4
	p.applyPRFilter()
	filter := p.gh.prFilter.String()
	return p, func() tea.Msg {
		return panels.GitHubFilterChangedMsg{
			Tab:    sectionPRs,
			Filter: filter,
		}
	}
}

func (p *Panel) applyIssueFilter() {
	p.tabItems[tabIssues] = nil
	for _, iss := range p.gh.allIssues {
		if p.matchesIssueFilter(iss) {
			p.tabItems[tabIssues] = append(p.tabItems[tabIssues], listItem{
				kind:  kindIssue,
				issue: iss,
			})
		}
	}
	p.tabCursor[tabIssues] = 0
	p.tabOffset[tabIssues] = 0
}

func (p *Panel) matchesIssueFilter(iss ghIssueItem) bool {
	switch p.gh.issueFilter {
	case issueFilterAssigned:
		return iss.Assignee == p.gh.user
	case issueFilterCreated:
		return iss.Author == p.gh.user
	default:
		return true
	}
}

func (p *Panel) applyPRFilter() {
	p.tabItems[tabPRs] = nil
	for _, pr := range p.gh.allPRs {
		if p.matchesPRFilter(pr) {
			p.tabItems[tabPRs] = append(p.tabItems[tabPRs], listItem{
				kind: kindPR,
				pr:   pr,
			})
		}
	}
	p.tabCursor[tabPRs] = 0
	p.tabOffset[tabPRs] = 0
}

func (p *Panel) matchesPRFilter(pr ghPRItem) bool {
	switch p.gh.prFilter {
	case prFilterNeedsReview:
		return pr.Author != p.gh.user && pr.State == prStateOpen
	case prFilterMine:
		return pr.Author == p.gh.user
	case prFilterDraft:
		return pr.State == stateDraft
	default:
		return true
	}
}

// ---------------------------------------------------------------------------
// GitHub selection commands
// ---------------------------------------------------------------------------
// issueSelectedCmd returns a Cmd that emits IssueSelectedMsg for the
// issue under the cursor.

func (p *Panel) issueSelectedCmd() tea.Cmd {
	if p.activeTab != tabIssues {
		return nil
	}
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) || items[cursor].kind != kindIssue {
		return nil
	}
	iss := items[cursor].issue
	return func() tea.Msg {
		return panels.IssueSelectedMsg{
			Number: iss.Number,
			Title:  iss.Title,
			Body:   iss.Body,
			State:  iss.State,
		}
	}
}

// prSelectedCmd returns a Cmd that emits PRSelectedMsg for the PR under the cursor
// and kicks off async loading of PR files and commits.
func (p *Panel) prSelectedCmd() tea.Cmd {
	if p.activeTab != tabPRs {
		return nil
	}
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) || items[cursor].kind != kindPR {
		return nil
	}
	pr := items[cursor].pr
	number := pr.Number
	title := pr.Title
	state := pr.State
	headBranch := pr.HeadBranch
	cmds := []tea.Cmd{
		func() tea.Msg {
			return panels.PRSelectedMsg{
				Number:     number,
				Title:      title,
				State:      state,
				HeadBranch: headBranch,
			}
		},
	}
	// Only load PR details if we have a GitHub client.
	if p.gh.client != nil {
		cmds = append(cmds, p.loadPRDetails(number))
	}
	return tea.Batch(cmds...)
}

// loadPRDetails returns a Cmd that fetches PR files and commits from GitHub.
func (p *Panel) loadPRDetails(number int) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		var result prDetailsLoadedMsg
		result.number = number
		// Fetch PR files.
		files, err := client.GetPRFiles(ctx, owner, repo, number)
		if err != nil {
			result.err = fmt.Errorf("PR files: %w", err)
			return result
		}
		for _, f := range files {
			result.files = append(result.files, panels.PRFile{
				Filename:  f.GetFilename(),
				Status:    f.GetStatus(),
				Additions: f.GetAdditions(),
				Deletions: f.GetDeletions(),
				Patch:     f.GetPatch(),
			})
		}
		// Fetch PR commits.
		commits, err := client.GetPRCommits(ctx, owner, repo, number)
		if err != nil {
			result.err = fmt.Errorf("PR commits: %w", err)
			return result
		}
		for _, c := range commits {
			msg := ""
			if c.Commit != nil {
				msg = c.Commit.GetMessage()
			}
			author := ""
			if c.Author != nil {
				author = c.Author.GetLogin()
			} else if c.Commit != nil && c.Commit.Author != nil {
				author = c.Commit.Author.GetName()
			}
			date := ""
			if c.Commit != nil && c.Commit.Author != nil && c.Commit.Author.Date != nil {
				date = c.Commit.Author.Date.Format("Jan 2 15:04")
			}
			result.commits = append(result.commits, panels.PRCommit{
				SHA:     c.GetSHA(),
				Message: msg,
				Author:  author,
				Date:    date,
			})
		}
		return result
	}
}

// handlePRDetailsLoaded processes the async PR detail fetch result and emits
// PRFilesLoadedMsg and PRCommitsLoadedMsg for downstream panels.
func (p *Panel) handlePRDetailsLoaded(msg prDetailsLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "GitHub: " + errStr, Level: notify.Error}
		}
	}
	number := msg.number
	files := msg.files
	commits := msg.commits
	return p, tea.Batch(
		func() tea.Msg { return panels.PRFilesLoadedMsg{Number: number, Files: files} },
		func() tea.Msg { return panels.PRCommitsLoadedMsg{Number: number, Commits: commits} },
	)
}

// actionRunSelectedCmd returns a Cmd that emits ActionRunSelectedMsg for the
// action run under the cursor and kicks off async loading of job details.
func (p *Panel) actionRunSelectedCmd() tea.Cmd {
	if p.activeTab != tabActions {
		return nil
	}
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) || items[cursor].kind != kindActionRun {
		return nil
	}
	run := items[cursor].actionRun
	runID := run.RunID
	workflowName := run.WorkflowName
	status := run.Status
	cmds := []tea.Cmd{
		func() tea.Msg {
			return panels.ActionRunSelectedMsg{
				RunID:        runID,
				WorkflowName: workflowName,
				Status:       status,
			}
		},
	}
	// Only load job details if we have a GitHub client.
	if p.gh.client != nil {
		cmds = append(cmds, p.loadActionJobs(runID))
	}
	return tea.Batch(cmds...)
}

// workflowSelectedCmd emits a WorkflowSelectedMsg so the preview pane can
// display the workflow file contents.
func (p *Panel) workflowSelectedCmd() tea.Cmd {
	if p.activeTab != tabWorkflows {
		return nil
	}
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) || items[cursor].kind != kindWorkflow {
		return nil
	}
	wf := items[cursor].workflow
	name := wf.Name
	path := wf.Path
	return func() tea.Msg {
		return panels.WorkflowSelectedMsg{
			Name: name,
			Path: path,
		}
	}
}

// loadActionJobs returns a Cmd that fetches jobs (with steps) for a workflow run.
func (p *Panel) loadActionJobs(runID int64) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		var result actionJobsLoadedMsg
		result.runID = runID
		ghJobs, err := client.ListWorkflowJobs(ctx, owner, repo, runID)
		if err != nil {
			result.err = fmt.Errorf("workflow jobs: %w", err)
			return result
		}
		for _, j := range ghJobs {
			job := panels.ActionJob{
				ID:         j.GetID(),
				Name:       j.GetName(),
				Status:     j.GetStatus(),
				Conclusion: j.GetConclusion(),
			}
			if j.StartedAt != nil {
				job.StartedAt = j.StartedAt.Format("2006-01-02T15:04:05Z")
			}
			if j.CompletedAt != nil {
				job.CompletedAt = j.CompletedAt.Format("2006-01-02T15:04:05Z")
			}
			for _, s := range j.Steps {
				job.Steps = append(job.Steps, panels.ActionStep{
					Number:     s.GetNumber(),
					Name:       s.GetName(),
					Status:     s.GetStatus(),
					Conclusion: s.GetConclusion(),
				})
			}
			result.jobs = append(result.jobs, job)
		}
		return result
	}
}

// handleActionJobsLoaded processes the async job fetch and emits ActionJobsLoadedMsg
// for downstream panels. If any job failed, it also fetches logs for that job.
func (p *Panel) handleActionJobsLoaded(msg actionJobsLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "GitHub: " + errStr, Level: notify.Error}
		}
	}
	runID := msg.runID
	jobs := msg.jobs
	cmds := []tea.Cmd{
		func() tea.Msg {
			return panels.ActionJobsLoadedMsg{RunID: runID, Jobs: jobs}
		},
	}
	// Auto-fetch logs for the first failed job.
	if p.gh.client != nil {
		for _, j := range jobs {
			if j.Conclusion == conclusionFailure {
				cmds = append(cmds, p.loadActionLog(runID, j.ID))
				break // only fetch logs for the first failed job
			}
		}
	}
	return p, tea.Batch(cmds...)
}

// loadActionLog returns a Cmd that fetches logs for a specific job.
func (p *Panel) loadActionLog(runID, jobID int64) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		log, err := client.GetJobLogs(ctx, owner, repo, jobID)
		if err != nil {
			return actionLogLoadedMsg{runID: runID, jobID: jobID, err: err}
		}
		return actionLogLoadedMsg{runID: runID, jobID: jobID, log: log}
	}
}

// handleActionLogLoaded processes log fetch results and emits ActionLogMsg.
func (p *Panel) handleActionLogLoaded(msg actionLogLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Logs: " + errStr, Level: notify.Error}
		}
	}
	runID := msg.runID
	jobID := msg.jobID
	log := msg.log
	return p, func() tea.Msg {
		return panels.ActionLogMsg{RunID: runID, JobID: jobID, Log: log}
	}
}

// rerunFailedJobsCmd returns a Cmd that reruns failed jobs for a workflow run.

func (p *Panel) rerunFailedJobsCmd(runID int64) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		err := client.RerunFailedJobs(ctx, owner, repo, runID)
		return actionRerunResultMsg{runID: runID, err: err}
	}
}

// handleActionRerunResult processes the rerun result and refreshes data.
func (p *Panel) handleActionRerunResult(msg actionRerunResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Rerun: " + errStr, Level: notify.Error}
		}
	}
	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{Message: "Rerunning failed jobs…", Level: notify.Info}
		},
		p.loadData(),
	)
}

// cancelWorkflowRunCmd returns a Cmd that cancels a workflow run.
func (p *Panel) cancelWorkflowRunCmd(runID int64) tea.Cmd {
	client := p.gh.client
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		err := client.CancelWorkflowRun(ctx, owner, repo, runID)
		return actionCancelResultMsg{runID: runID, err: err}
	}
}

// handleActionCancelResult processes the cancel result and refreshes data.
func (p *Panel) handleActionCancelResult(msg actionCancelResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cancel: " + errStr, Level: notify.Error}
		}
	}
	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{Message: "Workflow run cancelled", Level: notify.Info}
		},
		p.loadData(),
	)
}

// doActionsRerun triggers rerunning failed jobs for the selected action run.
func (p *Panel) doActionsRerun() (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) || items[cursor].kind != kindActionRun {
		return p, nil
	}
	run := items[cursor].actionRun
	return p, p.rerunFailedJobsCmd(run.RunID)
}

// doActionsCancel triggers cancellation of the selected action run.
func (p *Panel) doActionsCancel() (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) || items[cursor].kind != kindActionRun {
		return p, nil
	}
	run := items[cursor].actionRun
	return p, p.cancelWorkflowRunCmd(run.RunID)
}

// doWorkflowDispatch triggers a dispatch of the selected workflow.
// Uses a multi-step flow: confirm → ref input → inputs input → dispatch.
func (p *Panel) doWorkflowDispatch() (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) || items[cursor].kind != kindWorkflow {
		return p, nil
	}
	wf := items[cursor].workflow
	p.clearPending()
	p.pending = opWorkflowDispatch
	p.pendingName = fmt.Sprintf("%d:%s", wf.ID, wf.Name)
	branch := p.currentBranch()
	return p, notify.ShowInputWithValue("Dispatch Workflow",
		fmt.Sprintf("Dispatch %q — enter branch/ref:", wf.Name), branch)
}

// handleWorkflowDispatchResult handles the async result of a workflow dispatch.
func (p *Panel) handleWorkflowDispatchResult(msg workflowDispatchResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Dispatch failed: " + msg.err.Error(),
				Level:   notify.Error,
			}
		}
	}
	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Dispatched %s", msg.workflowName),
				Level:   notify.Info,
			}
		},
		p.loadGitHubData(),
	)
}

// handleWorkflowInputsFetched receives the parsed workflow_dispatch inputs
// from the YAML file and shows a pre-populated input dialog.
func (p *Panel) handleWorkflowInputsFetched(msg workflowInputsFetchedMsg) (panels.Panel, tea.Cmd) {
	// Wire up pending state so the next modal result hits opWorkflowDispatchInputs.
	p.pending = opWorkflowDispatchInputs
	p.pendingName = fmt.Sprintf("%d:%s:%s", msg.workflowID, msg.workflowName, msg.ref)
	// Build pre-populated value with actual field names and defaults.
	var prePopulated string
	if len(msg.inputs) > 0 {
		lines := make([]string, 0, len(msg.inputs))
		for _, input := range msg.inputs {
			lines = append(lines, input.Name+"="+input.Default)
		}
		prePopulated = strings.Join(lines, "\n")
	}
	title := fmt.Sprintf("Inputs for %s", msg.workflowName)
	placeholder := "key=value per line (empty to skip)"
	if len(msg.inputs) > 0 {
		placeholder = "edit values below (empty to skip)"
	}
	return p, notify.ShowInputWithValue(title, placeholder, prePopulated)
}

// ---------------------------------------------------------------------------
// PR merge
// ---------------------------------------------------------------------------

func mergeStrategyLabel(strategy string) string {
	switch strategy {
	case strategySquash:
		return "squash and merge"
	case strategyRebase:
		return "rebase and merge"
	default:
		return mergeCommitLabel
	}
}

// doMergePR initiates the merge flow for the selected PR.
// Shows an action picker with three merge strategies.
// Only enabled for open (non-draft) PRs.
func (p *Panel) doMergePR() (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) || items[cursor].kind != kindPR {
		return p, nil
	}
	pr := items[cursor].pr

	// Guard: only allow merge on open PRs.
	if pr.State != prStateOpen {
		stateLabel := pr.State
		if stateLabel == "" {
			stateLabel = stateUnknown
		}
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Cannot merge PR #%d: state is %s", pr.Number, stateLabel),
				Level:   notify.Warn,
			}
		}
	}

	if p.gh.client == nil {
		return p, nil
	}

	// Store PR details for multi-step flow.
	p.clearPending()
	p.pending = opPRMergeStrategy
	p.pendingName = fmt.Sprintf("%d:%s:%s", pr.Number, pr.HeadBranch, pr.Title)

	mergeActions := []notify.ActionOption{
		{ID: strategyMerge, Label: "Merge commit"},
		{ID: strategySquash, Label: "Squash and merge"},
		{ID: strategyRebase, Label: "Rebase and merge"},
	}

	title := fmt.Sprintf("Merge PR #%d", pr.Number)
	message := pr.Title
	return p, notify.ShowActionPickerWithMessage(title, message, mergeActions)
}

// mergePRCmd returns a tea.Cmd that executes the merge asynchronously.
func (p *Panel) mergePRCmd(number int, strategy string, headBranch string) tea.Cmd {
	client := p.gh.client
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		opts := &gh.PullRequestOptions{MergeMethod: strategy}
		err := client.MergePR(ctx, owner, repo, number, "", opts)
		if err != nil {
			return prMergeResultMsg{number: number, strategy: strategy, err: err}
		}

		return prMergeResultMsg{
			number:     number,
			strategy:   strategy,
			headBranch: headBranch,
		}
	}
}

// handlePRMergeResult processes the async result of a PR merge operation.
func (p *Panel) handlePRMergeResult(msg prMergeResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Merge PR #%d failed: %s", msg.number, errStr),
				Level:   notify.Error,
			}
		}
	}

	// Update local PR state to "merged".
	for i := range p.gh.allPRs {
		if p.gh.allPRs[i].Number == msg.number {
			p.gh.allPRs[i].State = prStateMerged
			break
		}
	}
	// Also update in the visible tab items.
	for i := range p.tabItems[tabPRs] {
		if p.tabItems[tabPRs][i].kind == kindPR && p.tabItems[tabPRs][i].pr.Number == msg.number {
			p.tabItems[tabPRs][i].pr.State = prStateMerged
			break
		}
	}

	label := mergeStrategyLabel(msg.strategy)
	toastMsg := fmt.Sprintf("PR #%d merged (%s)", msg.number, label)

	cmds := []tea.Cmd{
		func() tea.Msg {
			return notify.ShowToastMsg{
				Message: toastMsg,
				Level:   notify.Success,
			}
		},
		func() tea.Msg {
			return panels.PRMergedMsg{
				Number:   msg.number,
				Strategy: msg.strategy,
			}
		},
		p.loadGitHubData(),
	}

	// Offer to delete branch after successful merge.
	if msg.headBranch != "" {
		p.pending = opPRDeleteBranchAfterMerge
		p.pendingName = msg.headBranch
		cmds = append(cmds, notify.ShowConfirm(
			"Delete Branch",
			fmt.Sprintf("Delete branch %q? (remote + local)", msg.headBranch),
		))
	}

	return p, tea.Batch(cmds...)
}

// handlePRBranchDeleteResult processes the result of a post-merge branch deletion.
func (p *Panel) handlePRBranchDeleteResult(msg prBranchDeleteResultMsg) (panels.Panel, tea.Cmd) {
	if msg.remoteErr != nil && msg.localErr != nil {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Failed to delete branch %s: %s", msg.branch, msg.remoteErr),
				Level:   notify.Error,
			}
		}
	}
	if msg.remoteErr != nil {
		return p, tea.Batch(
			func() tea.Msg {
				return notify.ShowToastMsg{
					Message: fmt.Sprintf("Branch %s deleted locally, remote deletion failed: %s", msg.branch, msg.remoteErr),
					Level:   notify.Warn,
				}
			},
			p.loadData(),
		)
	}

	toastMsg := fmt.Sprintf("Branch %s deleted", msg.branch)
	level := notify.Success
	if msg.localErr != nil {
		// Local deletion failed (branch may not exist locally) — still report success for remote.
		toastMsg = fmt.Sprintf("Remote branch %s deleted (local not found)", msg.branch)
	}

	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{
				Message: toastMsg,
				Level:   level,
			}
		},
		p.loadData(),
	)
}

// ---------------------------------------------------------------------------
// GitHub item rendering
// ---------------------------------------------------------------------------
// renderIssue renders a GitHub issue line: "  #42 Fix auth token...   bug"
func (p *Panel) renderIssue(item listItem, width int, isCursor bool) string {
	iss := item.issue
	prefix := "  "
	number := fmt.Sprintf("#%d ", iss.Number)
	// Right side: first label, if any.
	rightSide := ""
	if len(iss.Labels) > 0 {
		rightSide = " " + panels.StripANSI(iss.Labels[0])
	}
	// Calculate available width for the title.
	prefixLen := lipgloss.Width(prefix) + lipgloss.Width(number)
	rightLen := lipgloss.Width(rightSide)
	titleWidth := width - prefixLen - rightLen - 1
	title := panels.StripANSI(iss.Title)
	titleRunes := []rune(title)
	if titleWidth > 0 && len(titleRunes) > titleWidth {
		if titleWidth > 1 {
			title = string(titleRunes[:titleWidth-1]) + "…"
		} else {
			title = string(titleRunes[:titleWidth])
		}
	} else if titleWidth <= 0 {
		title = ""
	}
	leftSide := prefix + number + title
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide)
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	line := leftSide + gap + rightSide
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(p.colors.Issue))
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(line)
}

// prColorFrom returns the foreground color for a PR based on its state and

func prColorFrom(c panelColors, pr ghPRItem) string {
	switch pr.State {
	case stateDraft:
		return c.PRDraft
	case prStateMerged:
		return c.PRMerged
	case stateClosed:
		return c.PRClosed
	default: // prStateOpen
		switch pr.MergeableState {
		case stateDirty:
			return c.PRConflict
		case "unstable":
			return c.PRUnstable
		case "blocked":
			return c.PRBlocked
		case stateUnknown:
			return c.PRUnknown
		default: // "clean" or ""
			return c.PR
		}
	}
}

// prColor returns the foreground color using package-level defaults (for tests).
func prColor(pr ghPRItem) string { return prColorFrom(defaultColors, pr) }

// prActionIconFrom returns the status icon and its color for the action run
// associated with a PR, using the given color palette.
func prActionIconFrom(c panelColors, pr ghPRItem) (icon string, color string) {
	switch pr.ActionConclusion {
	case conclusionSuccess:
		return checkMark, c.ActionOK
	case conclusionFailure, conclusionTimedOut:
		return crossMark, c.ActionFail
	}
	switch pr.ActionStatus {
	case statusInProgress, statusQueued:
		return runDot, c.ActionRun
	}
	return "", ""
}

// prActionIcon returns the status icon using package-level defaults (for tests).
func prActionIcon(pr ghPRItem) (icon string, color string) {
	return prActionIconFrom(defaultColors, pr)
}

// renderPR renders a GitHub PR line: "  #41 Add GitHub client   draft"
// Open PRs are colored by mergeable state, and an action-run status icon
// is appended when a matching workflow run exists for the PR’s head branch.
func (p *Panel) renderPR(item listItem, width int, isCursor bool) string {
	pr := item.pr
	prefix := "  "
	number := fmt.Sprintf("#%d ", pr.Number)

	// Action status icon (shown after state text).
	actionIcon, actionColor := prActionIconFrom(p.colors, pr)
	iconSuffix := ""
	iconVisualWidth := 0
	if actionIcon != "" {
		iconSuffix = " " + actionIcon // space + icon char
		iconVisualWidth = 1 + runewidth.StringWidth(actionIcon)
	}

	// Right side: state + optional action icon.
	rightSide := " " + pr.State

	// Calculate available width for the title.
	prefixLen := lipgloss.Width(prefix) + lipgloss.Width(number)
	rightLen := lipgloss.Width(rightSide) + iconVisualWidth
	titleWidth := width - prefixLen - rightLen - 1
	title := panels.StripANSI(pr.Title)
	titleRunes := []rune(title)
	if titleWidth > 0 && len(titleRunes) > titleWidth {
		if titleWidth > 1 {
			title = string(titleRunes[:titleWidth-1]) + "…"
		} else {
			title = string(titleRunes[:titleWidth])
		}
	} else if titleWidth <= 0 {
		title = ""
	}

	leftSide := prefix + number + title
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide) + iconVisualWidth
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}

	// Build the line: if we have an action icon, render it with its own color
	// so it stands out from the PR state color.
	fg := prColorFrom(p.colors, pr)
	var line string
	if iconSuffix != "" {
		iconStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(actionColor))
		if isCursor {
			iconStyle = iconStyle.Background(lipgloss.Color(p.colors.CursorBg))
		}
		line = leftSide + gap + rightSide + iconStyle.Render(iconSuffix)
	} else {
		line = leftSide + gap + rightSide
	}

	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(fg))
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(line)
}

// renderActionRun renders a GitHub Actions run line:
// "  ✓ CI / Build #1233   main  Jan 2 15:04"
func (p *Panel) renderActionRun(item listItem, width int, isCursor bool) string {
	run := item.actionRun
	// Status icon.
	var icon string
	var fg string
	switch run.Conclusion {
	case conclusionSuccess:
		icon = checkMark
		fg = p.colors.ActionOK
	case conclusionFailure, conclusionTimedOut:
		icon = crossMark
		fg = p.colors.ActionFail
	default:
		if run.Status == statusInProgress || run.Status == statusQueued {
			icon = runDot
			fg = p.colors.ActionRun
		} else {
			icon = runDot
			fg = p.colors.Dim
		}
	}
	prefix := "  "
	left := fmt.Sprintf("%s %s #%d", icon, panels.StripANSI(run.WorkflowName), run.RunNumber)
	// Right side: branch + timestamp.
	rightSide := ""
	if run.Branch != "" {
		rightSide += " " + panels.StripANSI(run.Branch)
	}
	if run.CreatedAt != "" {
		rightSide += " " + run.CreatedAt
	}
	// Truncate left text to fit.
	maxLeft := width - lipgloss.Width(prefix) - lipgloss.Width(rightSide) - 1
	leftRunes := []rune(left)
	if maxLeft > 0 && len(leftRunes) > maxLeft {
		if maxLeft > 1 {
			left = string(leftRunes[:maxLeft-1]) + "…"
		} else {
			left = string(leftRunes[:maxLeft])
		}
	} else if maxLeft <= 0 {
		left = ""
	}
	leftSide := prefix + left
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide)
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	line := leftSide + gap + rightSide
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(fg))
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(line)
}

// renderWorkflow renders a GitHub workflow definition line:
// "  ● CI   .github/workflows/ci.yml   active"
func (p *Panel) renderWorkflow(item listItem, width int, isCursor bool) string {
	wf := item.workflow
	var icon string
	var fg string
	switch wf.State {
	case stateActive:
		icon = runDot
		fg = p.colors.Workflow
	case stateDisabledManually, stateDisabledInactivity:
		icon = "○"
		fg = p.colors.Dim
	default:
		icon = "○"
		fg = p.colors.Dim
	}
	prefix := "  "
	left := fmt.Sprintf("%s %s", icon, panels.StripANSI(wf.Name))
	rightSide := ""
	if wf.Path != "" {
		rightSide += " " + wf.Path
	}
	rightSide += " " + wf.State
	maxLeft := width - lipgloss.Width(prefix) - lipgloss.Width(rightSide) - 1
	leftRunes := []rune(left)
	if maxLeft > 0 && len(leftRunes) > maxLeft {
		if maxLeft > 1 {
			left = string(leftRunes[:maxLeft-1]) + "…"
		} else {
			left = string(leftRunes[:maxLeft])
		}
	} else if maxLeft <= 0 {
		left = ""
	}
	leftSide := prefix + left
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide)
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	line := leftSide + gap + rightSide
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(fg))
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(line)
}

// renderRelease renders a GitHub release line:
// "  ✓ v1.2.3  Release Title   author  Jan 2"
func (p *Panel) renderRelease(item listItem, width int, isCursor bool) string {
	rel := item.release
	var icon string
	var fg string
	switch {
	case rel.Draft:
		icon = runDot
		fg = p.colors.RelDraft
	case rel.Prerelease:
		icon = "⚠"
		fg = p.colors.RelPre
	default:
		icon = checkMark
		fg = p.colors.Release
	}
	prefix := "  "
	left := fmt.Sprintf("%s %s", icon, panels.StripANSI(rel.TagName))
	if rel.Name != "" && rel.Name != rel.TagName {
		left += "  " + panels.StripANSI(rel.Name)
	}
	rightSide := ""
	if rel.Author != "" {
		rightSide += " " + panels.StripANSI(rel.Author)
	}
	if rel.CreatedAt != "" {
		rightSide += " " + rel.CreatedAt
	}
	if rel.AssetsCount > 0 {
		rightSide += fmt.Sprintf(" %d assets", rel.AssetsCount)
	}
	maxLeft := width - lipgloss.Width(prefix) - lipgloss.Width(rightSide) - 1
	leftRunes := []rune(left)
	if maxLeft > 0 && len(leftRunes) > maxLeft {
		if maxLeft > 1 {
			left = string(leftRunes[:maxLeft-1]) + "…"
		} else {
			left = string(leftRunes[:maxLeft])
		}
	} else if maxLeft <= 0 {
		left = ""
	}
	leftSide := prefix + left
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide)
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	line := leftSide + gap + rightSide
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(fg))
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(line)
}
