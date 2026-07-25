// GitHub integration: issues, PRs, actions, workflows, and releases.
package gitinfo

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	gh "github.com/google/go-github/v89/github"
	"github.com/jongio/grut/internal/crashlog"
	"github.com/jongio/grut/internal/git"
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
	HeadRepoOwner    string
	Author           string
	HTMLURL          string // GitHub web URL
	Number           int
	IsFork           bool
	MergeableState   string // "clean", "dirty", "unstable", "blocked", "unknown", ""
	ActionStatus     string // matched action run status: "success", "failure", "in_progress", "queued", ""
	ActionConclusion string // matched action run conclusion
}

// prStateMerged is the canonical value for a merged pull request state.
const prStateMerged = "merged"

// prStateOpen is the canonical value for an open pull request state.
const (
	prStateOpen             = "open"
	githubStaleToastMessage = "GitHub data may be stale — refresh failed"
)

func ghPRItemFromPullRequest(pr *gh.PullRequest, fallbackBaseOwner string) ghPRItem {
	state := pr.GetState()
	if pr.GetDraft() {
		state = stateDraft
	}
	if pr.GetMerged() {
		state = prStateMerged
	}
	author := ""
	if pr.User != nil {
		author = pr.User.GetLogin()
	}
	headOwner := pr.GetHead().GetRepo().GetOwner().GetLogin()
	baseOwner := pr.GetBase().GetRepo().GetOwner().GetLogin()
	if baseOwner == "" {
		baseOwner = fallbackBaseOwner
	}
	// Only treat a PR as same-repo (and reuse its raw head branch name on
	// checkout) when both owners are known and identical. A deleted-fork PR
	// reports an empty head owner; treating it as same-repo would fetch the
	// PR head into a local branch named after the attacker-controlled head ref
	// (e.g. "main"), clobbering it. Anything we cannot confirm as same-repo is
	// checked out under the synthetic "pr-N" branch instead.
	sameRepo := headOwner != "" && baseOwner != "" && headOwner == baseOwner
	return ghPRItem{
		Number:        pr.GetNumber(),
		Title:         pr.GetTitle(),
		State:         state,
		HeadBranch:    pr.GetHead().GetRef(),
		HeadRepoOwner: headOwner,
		Author:        author,
		HTMLURL:       pr.GetHTMLURL(),
		IsFork:        !sameRepo,
	}
}

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

type releaseCompareLoadedMsg struct {
	err       error
	files     []panels.PRFile
	commits   []panels.PRCommit
	baseTag   string
	headTag   string
	summary   string
	fileCount int
	additions int
	deletions int
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

// actionFollowLogMsg carries a live-follow log refresh result.
type actionFollowLogMsg struct {
	err      error
	log      string
	runID    int64
	jobID    int64
	terminal bool
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

// commentResultMsg carries the result of posting a conversation-level comment
// on an issue or PR.
type commentResultMsg struct {
	number int
	kind   string
	err    error
}

// prReviewCommentResultMsg carries the result of a diff review comment or its
// plain PR comment fallback.
type prReviewCommentResultMsg struct {
	err      error
	number   int
	fallback bool
}

// prRequestReviewersResultMsg carries the result of a request-reviewers operation.
type prRequestReviewersResultMsg struct {
	err       error
	reviewers []string
	number    int
}

// prBranchDeleteResultMsg carries the result of deleting a branch after PR merge.
type prBranchDeleteResultMsg struct {
	branch    string
	remoteErr error
	localErr  error
}

// prCreateResultMsg carries the result of creating a pull request.
type prCreateResultMsg struct {
	pr  ghPRItem
	err error
}

// issueCreateResultMsg carries the result of a create-issue operation.
type issueCreateResultMsg struct {
	err    error
	title  string
	number int
}

// workflowInputsFetchedMsg carries the result of fetching workflow_dispatch
// input definitions from the workflow YAML file.
type workflowInputsFetchedMsg struct {
	workflowName string
	ref          string
	inputs       []ghclient.WorkflowInput
	workflowID   int64
	inputsKnown  bool // true when the workflow inputs were read successfully
}

// workflow_dispatch input type identifiers (GitHub Actions).
const (
	inputTypeChoice  = "choice"
	inputTypeBoolean = "boolean"
	boolTrue         = "true"
	boolFalse        = "false"
)

// boolActions is the picker option set for a boolean workflow_dispatch input.
var boolActions = []notify.ActionOption{
	{ID: boolTrue, Label: boolTrue},
	{ID: boolFalse, Label: boolFalse},
}

// githubPollTickMsg triggers periodic GitHub data refresh.
type githubPollTickMsg struct{ time.Time }

// actionsWatchTickMsg triggers the next animation frame for the
// "watching CI" indicator in the Actions tab bar.
type actionsWatchTickMsg struct{ time.Time }

// actionFollowTickMsg triggers a live GitHub Actions log refresh.
type actionFollowTickMsg struct{ time.Time }

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

// actionFollowInterval controls live job log refresh cadence.
const actionFollowInterval = 3 * time.Second

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

// stateFilterKind identifies which item states (open/closed/all) are fetched
// for the Issues and PRs tabs. Unlike the client-side quick-filters, this
// controls the server-side State parameter, so closed and merged items are
// only fetched when the filter allows it.
type stateFilterKind int

const (
	stateFilterOpen stateFilterKind = iota
	stateFilterClosed
	stateFilterAll
)

func (s stateFilterKind) String() string {
	switch s {
	case stateFilterClosed:
		return "Closed"
	case stateFilterAll:
		return labelAll
	default:
		return "Open"
	}
}

// apiValue returns the value passed to the GitHub API State parameter.
func (s stateFilterKind) apiValue() string {
	switch s {
	case stateFilterClosed:
		return stateClosed
	case stateFilterAll:
		return "all"
	default:
		return prStateOpen
	}
}

// ghDataLoadedMsg carries the result of an async GitHub data load.
type ghDataLoadedMsg struct {
	err           error
	user          string
	defaultBranch string
	issues        []ghIssueItem
	prs           []ghPRItem
	actions       []ghActionItem
	workflows     []ghWorkflowItem
	releases      []ghReleaseItem
	repoPrivate   bool
	issuesOK      bool
	prsOK         bool
	actionsOK     bool
	workflowsOK   bool
	releasesOK    bool
}

// ghMetaLoadedMsg carries repo metadata and current user info.
type ghMetaLoadedMsg struct {
	user          string
	defaultBranch string
	repoPrivate   bool
}

// ghIssuesPageMsg carries one page of issues from the GitHub API.
type ghIssuesPageMsg struct {
	err      error
	issues   []ghIssueItem
	nextPage int
	replace  bool // true = first page (replace all), false = append
}

// ghPRsPageMsg carries one page of PRs from the GitHub API.
type ghPRsPageMsg struct {
	err      error
	prs      []ghPRItem
	nextPage int
	replace  bool
}

// ghActionsPageMsg carries one page of workflow runs from the GitHub API.
type ghActionsPageMsg struct {
	err      error
	actions  []ghActionItem
	nextPage int
	replace  bool
}

// ghWorkflowsPageMsg carries one page of workflow definitions.
type ghWorkflowsPageMsg struct {
	err       error
	workflows []ghWorkflowItem
	nextPage  int
	replace   bool
}

// ghReleasesPageMsg carries one page of releases.
type ghReleasesPageMsg struct {
	err      error
	releases []ghReleaseItem
	nextPage int
	replace  bool
}

var ghPagedTabs = [...]tabID{tabIssues, tabPRs, tabActions, tabWorkflows, tabReleases}

func (p *Panel) beginGitHubRefreshBurst() {
	p.gh.staleToastShown = false
	for _, tab := range ghPagedTabs {
		p.tabPaging[tab].loading = true
		if p.gh.freshness[tab].fetchedAt.IsZero() {
			p.tabPaging[tab].nextPage = 1
			p.tabPaging[tab].allLoaded = false
		}
	}
}

func (p *Panel) markGitHubTabFresh(tab tabID) {
	p.gh.freshness[tab] = ghTabFreshness{
		fetchedAt: time.Now(),
		owner:     p.gh.owner,
		repo:      p.gh.repo,
		stale:     false,
	}
}

func (p *Panel) markGitHubTabStale(tab tabID) tea.Cmd {
	if p.gh.freshness[tab].fetchedAt.IsZero() {
		return nil
	}
	p.gh.freshness[tab].stale = true
	if p.gh.staleToastShown {
		return nil
	}
	p.gh.staleToastShown = true
	return func() tea.Msg {
		return notify.ShowToastMsg{Message: githubStaleToastMessage, Level: notify.Warn}
	}
}

func (p *Panel) ghStaleBadge(tab tabID) string {
	fresh := p.gh.freshness[tab]
	if !fresh.stale || fresh.fetchedAt.IsZero() {
		return ""
	}
	return "stale • " + formatGitHubDataAge(time.Since(fresh.fetchedAt)) + " ago"
}

func currentGitHubActions(items []listItem) []ghActionItem {
	actions := make([]ghActionItem, 0, len(items))
	for _, item := range items {
		if item.kind == kindActionRun {
			actions = append(actions, item.actionRun)
		}
	}
	return actions
}

func currentGitHubWorkflows(items []listItem) []ghWorkflowItem {
	workflows := make([]ghWorkflowItem, 0, len(items))
	for _, item := range items {
		if item.kind == kindWorkflow {
			workflows = append(workflows, item.workflow)
		}
	}
	return workflows
}

func currentGitHubReleases(items []listItem) []ghReleaseItem {
	releases := make([]ghReleaseItem, 0, len(items))
	for _, item := range items {
		if item.kind == kindRelease {
			releases = append(releases, item.release)
		}
	}
	return releases
}

func formatGitHubDataAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return relativeDateJustNow
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
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
		tabEntry{id: tabNotifications, name: labelNotifications, short: "Ntf", count: p.ghNotifCountStr()},
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
			p.switchActiveTab(t.id)
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
	return guardedGitHubCmd("gitinfo.loadGitHubData", func() tea.Msg {
		var result ghDataLoadedMsg
		// Fetch repo metadata (visibility).
		repoInfo, err := client.RepoInfo(ctx, owner, repo)
		if err != nil {
			slog.Warn("github: fetch repo info failed", "owner", owner, "repo", repo, "err", err)
		} else if repoInfo != nil {
			result.repoPrivate = repoInfo.GetPrivate()
			result.defaultBranch = repoInfo.GetDefaultBranch()
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
			result.issuesOK = true
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
			result.prsOK = true
			for _, pr := range prs {
				result.prs = append(result.prs, ghPRItemFromPullRequest(pr, owner))
			}
		}
		// Fetch action runs.
		runs, err := client.ListWorkflowRuns(ctx, owner, repo, &gh.ListWorkflowRunsOptions{
			ListOptions: gh.ListOptions{PerPage: 20},
		})
		if err != nil {
			slog.Warn("github: fetch workflow runs failed", "owner", owner, "repo", repo, "err", err)
		} else {
			result.actionsOK = true
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
			result.workflowsOK = true
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
			result.releasesOK = true
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
	})
}

func (p *Panel) handleGHDataLoaded(msg ghDataLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		p.gh.err = msg.err
		return p, nil
	}
	if msg.user != "" {
		p.gh.user = msg.user
	}
	if msg.defaultBranch != "" {
		p.gh.defaultBranch = msg.defaultBranch
	}
	p.gh.repoPrivate = msg.repoPrivate
	issues := msg.issues
	if !msg.issuesOK && len(msg.issues) == 0 && !p.gh.freshness[tabIssues].fetchedAt.IsZero() {
		issues = append([]ghIssueItem(nil), p.gh.allIssues...)
		p.gh.freshness[tabIssues].stale = true
	} else {
		p.markGitHubTabFresh(tabIssues)
	}
	prs := msg.prs
	if !msg.prsOK && len(msg.prs) == 0 && !p.gh.freshness[tabPRs].fetchedAt.IsZero() {
		prs = append([]ghPRItem(nil), p.gh.allPRs...)
		p.gh.freshness[tabPRs].stale = true
	} else {
		p.markGitHubTabFresh(tabPRs)
	}
	actions := msg.actions
	if !msg.actionsOK && len(msg.actions) == 0 && !p.gh.freshness[tabActions].fetchedAt.IsZero() {
		actions = currentGitHubActions(p.tabItems[tabActions])
		p.gh.freshness[tabActions].stale = true
	} else {
		p.markGitHubTabFresh(tabActions)
	}
	workflows := msg.workflows
	if !msg.workflowsOK && len(msg.workflows) == 0 && !p.gh.freshness[tabWorkflows].fetchedAt.IsZero() {
		workflows = currentGitHubWorkflows(p.tabItems[tabWorkflows])
		p.gh.freshness[tabWorkflows].stale = true
	} else {
		p.markGitHubTabFresh(tabWorkflows)
	}
	releases := msg.releases
	if !msg.releasesOK && len(msg.releases) == 0 && !p.gh.freshness[tabReleases].fetchedAt.IsZero() {
		releases = currentGitHubReleases(p.tabItems[tabReleases])
		p.gh.freshness[tabReleases].stale = true
	} else {
		p.markGitHubTabFresh(tabReleases)
	}
	p.buildGitHubItems(issues, prs, actions, workflows, releases)
	// If a create/merge flow requested a specific PR be reselected after this
	// refresh, honor it now that the list has been rebuilt.
	if p.gh.pendingSelectPR != 0 {
		p.selectPRByNumber(p.gh.pendingSelectPR)
		p.gh.pendingSelectPR = 0
	}
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
	return guardedGitHubCmd("gitinfo.loadGitHubMeta", func() tea.Msg {
		var result ghMetaLoadedMsg
		repoInfo, err := client.RepoInfo(ctx, owner, repo)
		if err != nil {
			slog.Warn("github: fetch repo info failed", "owner", owner, "repo", repo, "err", err)
		} else if repoInfo != nil {
			result.repoPrivate = repoInfo.GetPrivate()
			result.defaultBranch = repoInfo.GetDefaultBranch()
		}
		user, err := client.CurrentUser(ctx)
		if err != nil {
			slog.Warn("github: fetch current user failed", "err", err)
		} else if user != nil && user.Login != nil {
			result.user = *user.Login
		}
		return result
	})
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
	state := p.gh.issueState.apiValue()
	return guardedGitHubCmd("gitinfo.loadIssuesPage", func() tea.Msg {
		issues, pr, err := client.ListIssuesPage(ctx, owner, repo, &gh.IssueListByRepoOptions{
			State:       state,
			ListOptions: gh.ListOptions{Page: page, PerPage: pageSize},
		})
		if err != nil {
			slog.Warn("github: fetch issues page failed", "owner", owner, "repo", repo, "page", page, "err", err)
			return ghIssuesPageMsg{err: err, nextPage: 0, replace: replace}
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
	})
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
	state := p.gh.prState.apiValue()
	return guardedGitHubCmd("gitinfo.loadPRsPage", func() tea.Msg {
		prs, pr, err := client.ListPRsPage(ctx, owner, repo, &gh.PullRequestListOptions{
			State:       state,
			ListOptions: gh.ListOptions{Page: page, PerPage: pageSize},
		})
		if err != nil {
			slog.Warn("github: fetch PRs page failed", "owner", owner, "repo", repo, "page", page, "err", err)
			return ghPRsPageMsg{err: err, nextPage: 0, replace: replace}
		}
		var items []ghPRItem
		for _, ghPR := range prs {
			items = append(items, ghPRItemFromPullRequest(ghPR, owner))
		}
		return ghPRsPageMsg{prs: items, nextPage: pr.NextPage, replace: replace}
	})
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
	return guardedGitHubCmd("gitinfo.loadActionsPage", func() tea.Msg {
		runs, pr, err := client.ListWorkflowRunsPage(ctx, owner, repo, &gh.ListWorkflowRunsOptions{
			ListOptions: gh.ListOptions{Page: page, PerPage: pageSize},
		})
		if err != nil {
			slog.Warn("github: fetch actions page failed", "owner", owner, "repo", repo, "page", page, "err", err)
			return ghActionsPageMsg{err: err, nextPage: 0, replace: replace}
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
	})
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
	return guardedGitHubCmd("gitinfo.loadWorkflowsPage", func() tea.Msg {
		workflows, pr, err := client.ListWorkflowsPage(ctx, owner, repo, &gh.ListOptions{Page: page, PerPage: pageSize})
		if err != nil {
			slog.Warn("github: fetch workflows page failed", "owner", owner, "repo", repo, "page", page, "err", err)
			return ghWorkflowsPageMsg{err: err, nextPage: 0, replace: replace}
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
	})
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
	return guardedGitHubCmd("gitinfo.loadReleasesPage", func() tea.Msg {
		releases, pr, err := client.ListReleasesPage(ctx, owner, repo, &gh.ListOptions{Page: page, PerPage: pageSize})
		if err != nil {
			slog.Warn("github: fetch releases page failed", "owner", owner, "repo", repo, "page", page, "err", err)
			return ghReleasesPageMsg{err: err, nextPage: 0, replace: replace}
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
	})
}

// ---------------------------------------------------------------------------
// Per-tab page handlers
// ---------------------------------------------------------------------------

// handleMetaLoaded processes repo metadata and user info.

func (p *Panel) handleMetaLoaded(msg ghMetaLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.user != "" {
		p.gh.user = msg.user
	}
	if msg.defaultBranch != "" {
		p.gh.defaultBranch = msg.defaultBranch
	}
	p.gh.repoPrivate = msg.repoPrivate
	return p, nil
}

// handleIssuesPage processes a page of issues.
func (p *Panel) handleIssuesPage(msg ghIssuesPageMsg) (panels.Panel, tea.Cmd) {
	p.tabPaging[tabIssues].loading = false
	if msg.err != nil {
		if !p.gh.freshness[tabIssues].fetchedAt.IsZero() {
			if !msg.replace {
				p.tabPaging[tabIssues].nextPage = 0
				p.tabPaging[tabIssues].allLoaded = true
			}
			return p, p.markGitHubTabStale(tabIssues)
		}
		p.tabPaging[tabIssues].nextPage = 0
		p.tabPaging[tabIssues].allLoaded = true
		p.gh.allIssues = nil
		p.applyIssueFilter()
		return p, nil
	}
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
	p.markGitHubTabFresh(tabIssues)
	// After a full refresh, select the just-created issue if one is pending.
	if msg.replace && p.gh.pendingSelectIssue != 0 {
		found := p.selectIssueByNumber(p.gh.pendingSelectIssue)
		p.gh.pendingSelectIssue = 0
		if found {
			return p, p.issueSelectedCmd()
		}
	}
	return p, nil
}

// handlePRsPage processes a page of pull requests.
func (p *Panel) handlePRsPage(msg ghPRsPageMsg) (panels.Panel, tea.Cmd) {
	p.tabPaging[tabPRs].loading = false
	if msg.err != nil {
		if !p.gh.freshness[tabPRs].fetchedAt.IsZero() {
			if !msg.replace {
				p.tabPaging[tabPRs].nextPage = 0
				p.tabPaging[tabPRs].allLoaded = true
			}
			return p, p.markGitHubTabStale(tabPRs)
		}
		p.tabPaging[tabPRs].nextPage = 0
		p.tabPaging[tabPRs].allLoaded = true
		p.gh.allPRs = nil
		p.applyPRFilter()
		return p, nil
	}
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
	p.markGitHubTabFresh(tabPRs)
	return p, nil
}

// handleActionsPage processes a page of workflow runs.
func (p *Panel) handleActionsPage(msg ghActionsPageMsg) (panels.Panel, tea.Cmd) {
	p.tabPaging[tabActions].loading = false
	if msg.err != nil {
		if !p.gh.freshness[tabActions].fetchedAt.IsZero() {
			if !msg.replace {
				p.tabPaging[tabActions].nextPage = 0
				p.tabPaging[tabActions].allLoaded = true
			}
			return p, p.markGitHubTabStale(tabActions)
		}
		p.tabPaging[tabActions].nextPage = 0
		p.tabPaging[tabActions].allLoaded = true
		p.tabItems[tabActions] = nil
		p.tabCursor[tabActions] = 0
		p.tabOffset[tabActions] = 0
		return p, nil
	}
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
	p.markGitHubTabFresh(tabActions)
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
	if msg.err != nil {
		if !p.gh.freshness[tabWorkflows].fetchedAt.IsZero() {
			if !msg.replace {
				p.tabPaging[tabWorkflows].nextPage = 0
				p.tabPaging[tabWorkflows].allLoaded = true
			}
			return p, p.markGitHubTabStale(tabWorkflows)
		}
		p.tabPaging[tabWorkflows].nextPage = 0
		p.tabPaging[tabWorkflows].allLoaded = true
		p.tabItems[tabWorkflows] = nil
		p.tabCursor[tabWorkflows] = 0
		p.tabOffset[tabWorkflows] = 0
		return p, nil
	}
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
	p.markGitHubTabFresh(tabWorkflows)
	return p, nil
}

// handleReleasesPage processes a page of releases.
func (p *Panel) handleReleasesPage(msg ghReleasesPageMsg) (panels.Panel, tea.Cmd) {
	p.tabPaging[tabReleases].loading = false
	if msg.err != nil {
		if !p.gh.freshness[tabReleases].fetchedAt.IsZero() {
			if !msg.replace {
				p.tabPaging[tabReleases].nextPage = 0
				p.tabPaging[tabReleases].allLoaded = true
			}
			return p, p.markGitHubTabStale(tabReleases)
		}
		p.tabPaging[tabReleases].nextPage = 0
		p.tabPaging[tabReleases].allLoaded = true
		p.tabItems[tabReleases] = nil
		p.tabCursor[tabReleases] = 0
		p.tabOffset[tabReleases] = 0
		return p, nil
	}
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
	p.markGitHubTabFresh(tabReleases)
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

// cycleIssueStateFilter advances the Issues state filter (open -> closed ->
// all) and reloads from GitHub, since closed issues are not held locally.
func (p *Panel) cycleIssueStateFilter() (panels.Panel, tea.Cmd) {
	if p.gh.client == nil {
		return p, nil
	}
	p.gh.issueState = (p.gh.issueState + 1) % 3
	p.gh.allIssues = nil
	p.tabItems[tabIssues] = nil
	p.tabCursor[tabIssues] = 0
	p.tabOffset[tabIssues] = 0
	p.tabPaging[tabIssues] = tabPagination{loading: true, nextPage: 1}
	return p, p.loadIssuesPage(1, true)
}

// cyclePRStateFilter advances the PRs state filter (open -> closed -> all) and
// reloads from GitHub, since closed and merged PRs are not held locally.
func (p *Panel) cyclePRStateFilter() (panels.Panel, tea.Cmd) {
	if p.gh.client == nil {
		return p, nil
	}
	p.gh.prState = (p.gh.prState + 1) % 3
	p.gh.allPRs = nil
	p.tabItems[tabPRs] = nil
	p.tabCursor[tabPRs] = 0
	p.tabOffset[tabPRs] = 0
	p.tabPaging[tabPRs] = tabPagination{loading: true, nextPage: 1}
	return p, p.loadPRsPage(1, true)
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
	p.reapplyActiveGitHubFilter(tabIssues)
}

func (p *Panel) reapplyActiveGitHubFilter(tab tabID) {
	if p.activeTab == tab && isGitHubFilterTab(tab) && p.filterQuery != "" {
		p.applyFilter()
	}
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

// ---------------------------------------------------------------------------
// Issue creation
// ---------------------------------------------------------------------------

// parseIssueLabels splits a comma-separated labels string into a trimmed,
// de-duplicated slice, dropping empty entries.
func parseIssueLabels(raw string) []string {
	seen := make(map[string]bool)
	var labels []string
	for _, part := range strings.Split(raw, ",") {
		label := strings.TrimSpace(part)
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
	}
	return labels
}

// createIssueCmd returns a tea.Cmd that creates a new issue asynchronously
// and reports the outcome via issueCreateResultMsg.
func (p *Panel) createIssueCmd(title, body string, labels []string) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		req := &gh.IssueRequest{Title: gh.Ptr(title)}
		if body != "" {
			req.Body = gh.Ptr(body)
		}
		if len(labels) > 0 {
			req.Labels = &labels
		}
		issue, err := client.CreateIssue(ctx, owner, repo, req)
		if err != nil {
			return issueCreateResultMsg{err: err, title: title}
		}
		return issueCreateResultMsg{number: issue.GetNumber(), title: issue.GetTitle()}
	}
}

// handleIssueCreateResult processes the async result of creating an issue.
// On success it refreshes the Issues list and queues selection of the new item.
func (p *Panel) handleIssueCreateResult(msg issueCreateResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Create issue failed: " + errStr,
				Level:   notify.Error,
			}
		}
	}
	// Make sure the refreshed list is visible and reset pagination for a page-1 reload.
	p.switchActiveTab(tabIssues)
	p.gh.pendingSelectIssue = msg.number
	p.tabPaging[tabIssues] = tabPagination{loading: true, nextPage: 1}
	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Created issue #%d", msg.number),
				Level:   notify.Success,
			}
		},
		p.loadIssuesPage(1, true),
	)
}

// selectIssueByNumber moves the Issues-tab cursor to the issue with the given
// number, if present in the current filtered view. Returns true when found.
func (p *Panel) selectIssueByNumber(number int) bool {
	if p.activeTab == tabIssues && p.filteredIdx != nil {
		for i := range p.activeItemCount() {
			item, ok := p.activeItemAt(i)
			if ok && item.kind == kindIssue && item.issue.Number == number {
				p.tabCursor[tabIssues] = i
				p.ensureCursorVisible()
				return true
			}
		}
		return false
	}
	for i, item := range p.tabItems[tabIssues] {
		if item.kind == kindIssue && item.issue.Number == number {
			p.tabCursor[tabIssues] = i
			p.ensureCursorVisible()
			return true
		}
	}
	return false
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
	p.reapplyActiveGitHubFilter(tabPRs)
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
			Number:   iss.Number,
			Title:    iss.Title,
			Body:     iss.Body,
			State:    iss.State,
			Author:   iss.Author,
			Assignee: iss.Assignee,
			Labels:   append([]string(nil), iss.Labels...),
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

func releaseCompareTags(items []listItem, cursor int) (string, string, bool) {
	if cursor < 0 || cursor >= len(items) || items[cursor].kind != kindRelease {
		return "", "", false
	}
	head := items[cursor].release.TagName
	if head == "" {
		return "", "", false
	}
	for i := cursor + 1; i < len(items); i++ {
		if items[i].kind != kindRelease {
			continue
		}
		base := items[i].release.TagName
		if base == "" {
			return "", "", false
		}
		return base, head, true
	}
	return "", "", false
}

func (p *Panel) compareRelease() (panels.Panel, tea.Cmd) {
	base, head, ok := releaseCompareTags(p.tabItems[tabReleases], p.tabCursor[tabReleases])
	if !ok {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "No previous release to compare", Level: notify.Info}
		}
	}
	return p, p.loadReleaseCompare(base, head)
}

func (p *Panel) loadReleaseCompare(base, head string) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		comparison, err := client.CompareCommits(ctx, owner, repo, base, head)
		if err != nil {
			return releaseCompareLoadedMsg{err: fmt.Errorf("release compare: %w", err), baseTag: base, headTag: head}
		}
		return buildReleaseCompareLoadedMsg(base, head, comparison)
	}
}

func buildReleaseCompareLoadedMsg(base, head string, comparison *gh.CommitsComparison) releaseCompareLoadedMsg {
	result := releaseCompareLoadedMsg{baseTag: base, headTag: head}
	if comparison == nil {
		result.summary = formatReleaseCompareSummary(base, head, 0, 0, 0, 0, nil)
		return result
	}
	for _, f := range comparison.Files {
		additions := f.GetAdditions()
		deletions := f.GetDeletions()
		result.additions += additions
		result.deletions += deletions
		result.files = append(result.files, panels.PRFile{
			Filename:  f.GetFilename(),
			Status:    f.GetStatus(),
			Additions: additions,
			Deletions: deletions,
			Patch:     f.GetPatch(),
		})
	}
	result.fileCount = len(result.files)
	for _, c := range comparison.Commits {
		result.commits = append(result.commits, repositoryCommitToPanelCommit(c))
	}
	result.summary = formatReleaseCompareSummary(
		base,
		head,
		comparison.GetTotalCommits(),
		result.fileCount,
		result.additions,
		result.deletions,
		result.commits,
	)
	return result
}

func repositoryCommitToPanelCommit(c *gh.RepositoryCommit) panels.PRCommit {
	if c == nil {
		return panels.PRCommit{}
	}
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
		date = c.Commit.Author.Date.Format(time.RFC3339)
	}
	return panels.PRCommit{
		SHA:     c.GetSHA(),
		Message: msg,
		Author:  author,
		Date:    date,
	}
}

func formatReleaseCompareSummary(base, head string, commitCount, fileCount, additions, deletions int, commits []panels.PRCommit) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Release compare: %s..%s\n\n", base, head)
	fmt.Fprintf(&b, "Commits: %d\n", commitCount)
	fmt.Fprintf(&b, "Changed files: %d\n", fileCount)
	fmt.Fprintf(&b, "Additions: %d\n", additions)
	fmt.Fprintf(&b, "Deletions: %d\n\n", deletions)
	b.WriteString("Commits\n")
	limit := min(len(commits), 20)
	if limit == 0 {
		b.WriteString("  No commits in this range\n")
		return b.String()
	}
	for i := range limit {
		commit := commits[i]
		subject := commit.Message
		if idx := strings.Index(subject, "\n"); idx > 0 {
			subject = subject[:idx]
		}
		short := commit.SHA
		if len(short) > git.ShortHashLen {
			short = short[:git.ShortHashLen]
		}
		if short == "" {
			short = stateUnknown
		}
		fmt.Fprintf(&b, "  %s %s\n", short, subject)
	}
	if len(commits) > limit {
		fmt.Fprintf(&b, "  ...and %d more\n", len(commits)-limit)
	}
	return b.String()
}

func (p *Panel) handleReleaseCompareLoaded(msg releaseCompareLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "GitHub: " + errStr, Level: notify.Error}
		}
	}
	title := fmt.Sprintf("Release %s..%s", msg.baseTag, msg.headTag)
	return p, tea.Batch(
		func() tea.Msg {
			return panels.ReleaseComparePreviewMsg{Title: title, Content: msg.summary}
		},
		func() tea.Msg {
			return panels.ReleaseCompareFilesLoadedMsg{BaseTag: msg.baseTag, HeadTag: msg.headTag, Files: msg.files}
		},
		func() tea.Msg {
			return panels.ReleaseCompareCommitsLoadedMsg{BaseTag: msg.baseTag, HeadTag: msg.headTag, Commits: msg.commits}
		},
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

func actionRunIsRunning(status string) bool {
	return status == statusInProgress || status == statusQueued
}

func (p *Panel) selectedActionRun() (ghActionItem, bool) {
	if p.activeTab != tabActions {
		return ghActionItem{}, false
	}
	item, ok := p.currentItem()
	if !ok || item.kind != kindActionRun {
		return ghActionItem{}, false
	}
	return item.actionRun, true
}

func primaryActionJobID(jobs []*gh.WorkflowJob) int64 {
	if len(jobs) == 0 {
		return 0
	}
	for _, job := range jobs {
		if job.GetConclusion() == conclusionFailure {
			return job.GetID()
		}
	}
	return jobs[0].GetID()
}

// doActionsFollowToggle starts/stops live log following for the selected run.
func (p *Panel) doActionsFollowToggle() (panels.Panel, tea.Cmd) {
	if p.gh.client == nil || p.activeTab != tabActions {
		return p, nil
	}
	if p.actionFollowing {
		p.actionFollowing = false
		p.actionFollowPaused = false
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Action log follow stopped", Level: notify.Info}
		}
	}
	run, ok := p.selectedActionRun()
	if !ok {
		return p, nil
	}
	if !actionRunIsRunning(run.Status) {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Action log follow requires a queued or running job", Level: notify.Info}
		}
	}
	p.actionFollowing = true
	p.actionFollowPaused = false
	p.actionFollowRunID = run.RunID
	p.actionFollowJobID = 0
	p.actionFollowLastErr = ""
	return p, tea.Batch(
		func() tea.Msg { return notify.ShowToastMsg{Message: "Action log follow started", Level: notify.Info} },
		p.fetchActionFollowLogCmd(run.RunID, 0),
	)
}

func (p *Panel) doActionsFollowPauseToggle() (panels.Panel, tea.Cmd) {
	if p.gh.client == nil || p.activeTab != tabActions || !p.actionFollowing {
		return p, nil
	}
	p.actionFollowPaused = !p.actionFollowPaused
	message := "Action log follow resumed"
	if p.actionFollowPaused {
		message = "Action log follow paused"
	}
	return p, func() tea.Msg { return notify.ShowToastMsg{Message: message, Level: notify.Info} }
}

func (p *Panel) fetchActionFollowLogCmd(runID, jobID int64) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		selectedJobID := jobID
		if selectedJobID == 0 {
			jobs, err := client.ListWorkflowJobs(ctx, owner, repo, runID)
			if err != nil {
				return actionFollowLogMsg{runID: runID, err: fmt.Errorf("workflow jobs: %w", err)}
			}
			selectedJobID = primaryActionJobID(jobs)
			if selectedJobID == 0 {
				return actionFollowLogMsg{runID: runID, err: fmt.Errorf("workflow jobs: no jobs found")}
			}
		}
		log, err := client.GetJobLogsFresh(ctx, owner, repo, selectedJobID)
		if err != nil {
			return actionFollowLogMsg{runID: runID, jobID: selectedJobID, err: err}
		}
		run, err := client.GetWorkflowRun(ctx, owner, repo, runID)
		if err != nil {
			return actionFollowLogMsg{runID: runID, jobID: selectedJobID, err: fmt.Errorf("workflow run status: %w", err)}
		}
		terminal := run != nil && !actionRunIsRunning(run.GetStatus())
		return actionFollowLogMsg{runID: runID, jobID: selectedJobID, log: log, terminal: terminal}
	}
}

func (p *Panel) handleActionFollowLog(msg actionFollowLogMsg) (panels.Panel, tea.Cmd) {
	if !p.actionFollowing || msg.runID != p.actionFollowRunID {
		return p, nil
	}
	if msg.err != nil {
		errStr := msg.err.Error()
		cmds := []tea.Cmd{p.actionFollowTickCmd()}
		if errStr != p.actionFollowLastErr {
			p.actionFollowLastErr = errStr
			cmds = append(cmds, func() tea.Msg {
				return notify.ShowToastMsg{Message: "Follow logs: " + errStr, Level: notify.Error}
			})
		}
		return p, tea.Batch(cmds...)
	}
	p.actionFollowLastErr = ""
	if msg.jobID != 0 {
		p.actionFollowJobID = msg.jobID
	}
	cmds := []tea.Cmd{func() tea.Msg {
		return panels.ActionLogMsg{RunID: msg.runID, JobID: msg.jobID, Log: msg.log, Follow: true, Replace: true}
	}}
	if msg.terminal {
		p.actionFollowing = false
		p.actionFollowPaused = false
		cmds = append(cmds, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Action log follow stopped: job finished", Level: notify.Info}
		})
	} else {
		cmds = append(cmds, p.actionFollowTickCmd())
	}
	return p, tea.Batch(cmds...)
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
	if client == nil {
		return ghClientUnavailableCmd()
	}
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

// handleWorkflowInputsFetched receives the parsed workflow_dispatch inputs and
// begins collecting values. When the inputs are known it prompts once per
// input — a picker for choice/boolean inputs (so the user selects a valid
// value), a text field for free-form inputs (so the user can enter a custom
// value) — then dispatches. A workflow with no inputs dispatches immediately.
// When the inputs could not be read (fetch/parse failed) it falls back to a
// free-form key=value composer so the user can still pass inputs.
func (p *Panel) handleWorkflowInputsFetched(msg workflowInputsFetchedMsg) (panels.Panel, tea.Cmd) {
	if !msg.inputsKnown {
		// Inputs unreadable — offer the free-form composer as a fallback.
		p.pending = opWorkflowDispatchRaw
		p.pendingName = fmt.Sprintf("%d:%s:%s", msg.workflowID, msg.workflowName, msg.ref)
		title := fmt.Sprintf("Inputs for %s", msg.workflowName)
		return p, notify.ShowMultilineInput(title, "key=value per line (empty to skip) • ctrl+d submit")
	}
	p.wfDispatch = workflowDispatchDraft{
		workflowID:   msg.workflowID,
		workflowName: msg.workflowName,
		ref:          msg.ref,
		inputs:       msg.inputs,
		values:       make(map[string]any),
	}
	return p.promptNextWorkflowInput()
}

// promptNextWorkflowInput advances the workflow-dispatch flow: it prompts for
// the next declared input, or fires the dispatch once every input has been
// collected. Choice and boolean inputs use an action picker preselected on the
// declared default; free-form inputs use a text field pre-filled with the
// default so the user can accept it or type a custom value.
func (p *Panel) promptNextWorkflowInput() (panels.Panel, tea.Cmd) {
	d := &p.wfDispatch
	if d.idx >= len(d.inputs) {
		return p.fireWorkflowDispatch()
	}
	input := d.inputs[d.idx]
	p.pending = opWorkflowDispatchInputs
	title := fmt.Sprintf("%s — input %d of %d", d.workflowName, d.idx+1, len(d.inputs))
	prompt := workflowInputPrompt(input)
	switch {
	case input.Type == inputTypeChoice && len(input.Options) > 0:
		return p, notify.ShowActionPickerWithSelection(title, prompt, optionsToActions(input.Options), input.Default)
	case input.Type == inputTypeBoolean:
		selected := boolFalse
		if input.Default == boolTrue {
			selected = boolTrue
		}
		return p, notify.ShowActionPickerWithSelection(title, prompt, boolActions, selected)
	default:
		return p, notify.ShowInputWithValue(title, prompt, input.Default)
	}
}

// fireWorkflowDispatch dispatches the workflow with the values collected across
// the per-input prompts, then resets the draft.
func (p *Panel) fireWorkflowDispatch() (panels.Panel, tea.Cmd) {
	d := p.wfDispatch
	p.wfDispatch = workflowDispatchDraft{}
	var inputs map[string]any
	if len(d.values) > 0 {
		inputs = d.values
	}
	return p, p.dispatchWorkflowCmd(d.workflowID, d.workflowName, d.ref, inputs)
}

// dispatchWorkflowCmd builds the command that dispatches a workflow run. It
// returns nil when the target is incomplete (missing id/ref), and surfaces a
// nil client as a failure toast rather than a nil dereference that would crash
// the TUI. Shared by the per-input flow and the free-form fallback.
func (p *Panel) dispatchWorkflowCmd(workflowID int64, workflowName, ref string, inputs map[string]any) tea.Cmd {
	if workflowID == 0 || ref == "" {
		return nil
	}
	owner, repo := p.gh.owner, p.gh.repo
	ghClient := p.gh.client
	ctx := p.ctx
	if ghClient == nil {
		return func() tea.Msg {
			return workflowDispatchResultMsg{workflowName: workflowName, err: errGitHubClientUnavailable}
		}
	}
	return guardedGitHubCmd("gitinfo.dispatchWorkflow", func() tea.Msg {
		err := ghClient.DispatchWorkflow(ctx, owner, repo, workflowID, ref, inputs)
		return workflowDispatchResultMsg{workflowName: workflowName, err: err}
	})
}

// workflowInputPrompt builds the prompt line shown while collecting a single
// workflow_dispatch input, favouring the description and flagging required
// inputs.
func workflowInputPrompt(in ghclient.WorkflowInput) string {
	label := in.Description
	if label == "" {
		label = in.Name
	}
	if in.Name != "" && label != in.Name {
		label = in.Name + ": " + label
	}
	if in.Required {
		label += " (required)"
	}
	return label
}

// optionsToActions converts declared choice options into picker actions.
func optionsToActions(opts []string) []notify.ActionOption {
	actions := make([]notify.ActionOption, 0, len(opts))
	for _, o := range opts {
		actions = append(actions, notify.ActionOption{ID: o, Label: o})
	}
	return actions
}

// ghCmdPanicMsg is emitted by guardedGitHubCmd when a GitHub command goroutine
// panics (after the panic is captured to a crash report). The panel handles it
// by clearing any in-flight loading state — which the panicking command's
// normal typed result would otherwise have cleared — so the panel is not left
// stuck showing "Loading…", and by surfacing an error toast.
type ghCmdPanicMsg struct{ label string }

// guardedGitHubCmd wraps a GitHub command closure so a panic in the Bubble Tea
// command goroutine is captured to a crash report and surfaced as an error
// toast, instead of killing the whole TUI. crashlog.GuardTUI only covers the
// model's Init/Update/View on the main goroutine; command goroutines (run by
// Bubble Tea's execBatchMsg) are otherwise unprotected, so an unexpected panic
// on the GitHub data path — e.g. a nil field in live API data — would abort the
// program with a swallowed stack (issue #361).
func guardedGitHubCmd(label string, fn func() tea.Msg) tea.Cmd {
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				crashlog.CaptureCmdPanic(r, label)
				msg = ghCmdPanicMsg{label: label}
			}
		}()
		return fn()
	}
}

// handleGHCmdPanic clears any in-flight loading state stranded by a panicking
// GitHub command (its normal typed result never arrived to clear it) and shows
// an error toast. The crash report was already written by guardedGitHubCmd.
func (p *Panel) handleGHCmdPanic(_ ghCmdPanicMsg) (panels.Panel, tea.Cmd) {
	for i := range p.tabPaging {
		p.tabPaging[i].loading = false
	}
	p.gh.notifLoading = false
	return p, func() tea.Msg {
		return notify.ShowToastMsg{
			Message: "GitHub request failed unexpectedly — details saved to the crash log",
			Level:   notify.Error,
		}
	}
}

// ghClientUnavailableCmd surfaces a failure toast when a GitHub action is
// attempted with no client configured, instead of dereferencing a nil client
// in a command goroutine and crashing the TUI. Its loader siblings return nil
// (silent no-op); an explicit action instead tells the user why nothing
// happened.
func ghClientUnavailableCmd() tea.Cmd {
	return func() tea.Msg {
		return notify.ShowToastMsg{
			Message: errGitHubClientUnavailable.Error(),
			Level:   notify.Error,
		}
	}
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

func (p *Panel) selectedPR() (ghPRItem, bool) {
	item, ok := p.currentItem()
	if !ok || item.kind != kindPR {
		return ghPRItem{}, false
	}
	return item.pr, true
}

func (p *Panel) checkoutPR() (panels.Panel, tea.Cmd) {
	pr, ok := p.selectedPR()
	if !ok {
		return p, nil
	}
	p.clearPending()
	p.pending = opPRCheckout
	p.pendingPRCheckout = pr

	actions := []notify.ActionOption{
		{ID: actionPRCheckoutCurrent, Label: "Current worktree"},
		{ID: actionPRCheckoutWorktree, Label: "Sibling worktree"},
	}
	title := fmt.Sprintf("Check out PR #%d", pr.Number)
	message := fmt.Sprintf("%s\nBranch: %s", pr.Title, prCheckoutBranch(pr))
	return p, notify.ShowActionPickerWithMessage(title, message, actions)
}

func prCheckoutBranch(pr ghPRItem) string {
	if pr.IsFork {
		return fmt.Sprintf("pr-%d", pr.Number)
	}
	return pr.HeadBranch
}

func prFetchRefspec(pr ghPRItem) string {
	return fmt.Sprintf("pull/%d/head:%s", pr.Number, prCheckoutBranch(pr))
}

func prWorktreePath(repoRoot string, number int) string {
	parent := filepath.Dir(repoRoot)
	repoName := filepath.Base(repoRoot)
	return filepath.Join(parent, fmt.Sprintf("%s-pr-%d", repoName, number))
}

func (p *Panel) handlePRCheckout(a modalArgs) (panels.Panel, tea.Cmd) {
	pr := a.prCheckout
	branch := prCheckoutBranch(pr)
	switch a.msg.Value {
	case actionPRCheckoutCurrent:
		return p, p.prCheckoutCurrentCmd(pr, branch)
	case actionPRCheckoutWorktree:
		path := prWorktreePath(p.repoRoot, pr.Number)
		return p, p.prCheckoutWorktreeCmd(pr, branch, path)
	default:
		return p, nil
	}
}

func (p *Panel) prCheckoutCurrentCmd(pr ghPRItem, branch string) tea.Cmd {
	g, ctx := p.git, p.ctx
	refspec := prFetchRefspec(pr)
	return func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic during PR checkout", "number", pr.Number, "panic", r)
			}
		}()
		files, err := g.Status(ctx)
		if err != nil {
			return opResultMsg{op: eventPRCheckout, name: branch, err: err}
		}
		if len(files) > 0 {
			return opResultMsg{op: eventPRCheckout, name: branch, err: fmt.Errorf("dirty worktree; choose sibling worktree checkout")}
		}
		if err := g.Fetch(ctx, git.FetchOpts{Remote: remoteOrigin, Refspec: refspec}); err != nil {
			return opResultMsg{op: eventPRCheckout, name: branch, err: err}
		}
		err = g.Checkout(ctx, branch)
		return opResultMsg{op: eventPRCheckout, name: branch, err: err}
	}
}

func (p *Panel) prCheckoutWorktreeCmd(pr ghPRItem, branch, path string) tea.Cmd {
	g, ctx := p.git, p.ctx
	refspec := prFetchRefspec(pr)
	return func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic during PR worktree checkout", "number", pr.Number, "path", path, "panic", r)
			}
		}()
		if err := g.Fetch(ctx, git.FetchOpts{Remote: remoteOrigin, Refspec: refspec}); err != nil {
			return opResultMsg{op: eventPRWorktree, name: path, err: err}
		}
		err := g.WorktreeAdd(ctx, path, branch)
		return opResultMsg{op: eventPRWorktree, name: path, err: err}
	}
}

// mergePRCmd returns a tea.Cmd that executes the merge asynchronously.
func (p *Panel) mergePRCmd(number int, strategy string, headBranch string) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return ghClientUnavailableCmd()
	}
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

// commentKindIssue and commentKindPR label the target of a comment for the
// pending-op name and result toast.
const (
	commentKindIssue = "issue"
	commentKindPR    = "PR"
)

// doCommentOnItem opens a multi-line composer to post a conversation-level
// comment on the selected issue or PR. This is distinct from inline diff
// review comments; it uses the shared issue-comment endpoint.
func (p *Panel) doCommentOnItem() (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return p, nil
	}
	if p.gh.client == nil {
		return p, nil
	}

	var number int
	var title, kind string
	switch items[cursor].kind {
	case kindIssue:
		number = items[cursor].issue.Number
		title = items[cursor].issue.Title
		kind = commentKindIssue
	case kindPR:
		number = items[cursor].pr.Number
		title = items[cursor].pr.Title
		kind = commentKindPR
	default:
		return p, nil
	}

	p.clearPending()
	p.pending = opIssuePRComment
	p.pendingName = fmt.Sprintf("%s:%d", kind, number)

	modalTitle := fmt.Sprintf("Comment on %s #%d: %s", kind, number, title)
	return p, notify.ShowMultilineInput(modalTitle, "Write a comment...")
}

// commentCmd returns a tea.Cmd that posts the comment asynchronously.
func (p *Panel) commentCmd(number int, body, kind string) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return ghClientUnavailableCmd()
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		err := client.CommentOnIssue(ctx, owner, repo, number, body)
		return commentResultMsg{number: number, kind: kind, err: err}
	}
}

// handleCommentResult processes the async result of posting a comment. On
// success it refreshes the conversation shown in the preview.
func (p *Panel) handleCommentResult(msg commentResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Comment on %s #%d failed: %s", msg.kind, msg.number, errStr),
				Level:   notify.Error,
			}
		}
	}

	toastMsg := fmt.Sprintf("Comment posted on %s #%d", msg.kind, msg.number)
	cmds := []tea.Cmd{
		func() tea.Msg {
			return notify.ShowToastMsg{
				Message: toastMsg,
				Level:   notify.Success,
			}
		},
	}
	if refresh := p.activeTabSelectionCmd(); refresh != nil {
		cmds = append(cmds, refresh)
	}
	return p, tea.Batch(cmds...)
}

// postPRReviewCommentCmd posts a line-anchored review comment when line data
// is available, otherwise it falls back to a regular PR conversation comment.
func (p *Panel) postPRReviewCommentCmd(msg panels.PostPRReviewCommentMsg) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return func() tea.Msg {
			return prReviewCommentResultMsg{
				number: msg.Number,
				err:    fmt.Errorf("GitHub client unavailable"),
			}
		}
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		if !msg.HasLine {
			err := client.CommentOnIssue(ctx, owner, repo, msg.Number, msg.Body)
			return prReviewCommentResultMsg{number: msg.Number, fallback: true, err: err}
		}
		pr, err := client.GetPR(ctx, owner, repo, msg.Number)
		if err != nil {
			return prReviewCommentResultMsg{number: msg.Number, err: err}
		}
		if pr == nil {
			return prReviewCommentResultMsg{
				number: msg.Number,
				err:    fmt.Errorf("PR details unavailable"),
			}
		}
		commitID := pr.GetHead().GetSHA()
		if commitID == "" {
			return prReviewCommentResultMsg{
				number: msg.Number,
				err:    fmt.Errorf("PR head SHA unavailable"),
			}
		}
		err = client.CreateReviewComment(ctx, owner, repo, msg.Number, commitID, msg.Path, msg.Line, msg.Body)
		return prReviewCommentResultMsg{number: msg.Number, err: err}
	}
}

func (p *Panel) handlePRReviewCommentResult(msg prReviewCommentResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Comment on PR #%d failed: %s", msg.number, errStr),
				Level:   notify.Error,
			}
		}
	}

	text := fmt.Sprintf("Review comment posted on PR #%d", msg.number)
	if msg.fallback {
		text = fmt.Sprintf("PR comment posted on PR #%d (no line anchor)", msg.number)
	}
	cmds := []tea.Cmd{
		func() tea.Msg {
			return notify.ShowToastMsg{Message: text, Level: notify.Success}
		},
		func() tea.Msg { return panels.RefreshPreviewMsg{} },
	}
	if p.gh.client != nil {
		cmds = append(cmds, p.loadPRDetails(msg.number))
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

// doRequestReviewers initiates the request-reviewers flow for the selected PR.
// It opens a single-line input for one or more comma-separated reviewer logins.
func (p *Panel) doRequestReviewers() (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) || items[cursor].kind != kindPR {
		return p, nil
	}
	pr := items[cursor].pr

	// Guard: only request reviewers on open PRs.
	if pr.State != prStateOpen {
		stateLabel := pr.State
		if stateLabel == "" {
			stateLabel = stateUnknown
		}
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Cannot request reviewers on PR #%d: state is %s", pr.Number, stateLabel),
				Level:   notify.Warn,
			}
		}
	}

	if p.gh.client == nil {
		return p, nil
	}

	p.clearPending()
	p.pending = opPRRequestReviewers
	p.pendingName = fmt.Sprintf("%d", pr.Number)

	title := fmt.Sprintf("Request reviewers for PR #%d", pr.Number)
	return p, notify.ShowInput(title, "logins, comma separated (e.g. octocat, hubot)")
}

// parseReviewerLogins splits a comma-separated string of GitHub logins into a
// trimmed, de-duplicated slice, preserving first-seen order. A leading "@" on
// any login is stripped and empty entries are dropped. De-duplication is
// case-insensitive since GitHub logins are case-insensitive.
func parseReviewerLogins(input string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, part := range strings.Split(input, ",") {
		login := strings.TrimSpace(part)
		login = strings.TrimPrefix(login, "@")
		login = strings.TrimSpace(login)
		if login == "" {
			continue
		}
		key := strings.ToLower(login)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, login)
	}
	return out
}

// requestReviewersCmd returns a tea.Cmd that requests reviewers asynchronously.
func (p *Panel) requestReviewersCmd(number int, reviewers []string) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return ghClientUnavailableCmd()
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		err := client.RequestReviewers(ctx, owner, repo, number, reviewers)
		return prRequestReviewersResultMsg{number: number, reviewers: reviewers, err: err}
	}
}

// handlePRRequestReviewersResult processes the async result of a
// request-reviewers operation, showing a notification and refreshing PR detail.
func (p *Panel) handlePRRequestReviewersResult(msg prRequestReviewersResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Request reviewers on PR #%d failed: %s", msg.number, errStr),
				Level:   notify.Error,
			}
		}
	}

	toastMsg := fmt.Sprintf("Requested %s on PR #%d", strings.Join(msg.reviewers, ", "), msg.number)
	cmds := []tea.Cmd{
		func() tea.Msg {
			return notify.ShowToastMsg{Message: toastMsg, Level: notify.Success}
		},
	}
	// Refresh PR detail so requested reviewers show in the preview.
	if p.gh.client != nil {
		cmds = append(cmds, p.loadPRDetails(msg.number))
	}
	return p, tea.Batch(cmds...)
}

// issueStateResultMsg carries the result of closing or reopening an issue.
type issueStateResultMsg struct {
	newState string // "open" or "closed"
	err      error
	number   int
}

// doCloseReopenIssue closes or reopens the issue under the cursor on the
// Issues tab. Open issues are closed; closed issues are reopened.
func (p *Panel) doCloseReopenIssue() (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) || items[cursor].kind != kindIssue {
		return p, nil
	}
	return p.doCloseReopenIssueFor(items[cursor].issue)
}

// doCloseReopenIssueFor prepares a confirmation modal to close or reopen the
// given issue. The target state is derived from the issue's current state.
func (p *Panel) doCloseReopenIssueFor(iss ghIssueItem) (panels.Panel, tea.Cmd) {
	if p.gh.client == nil || iss.Number == 0 {
		return p, nil
	}
	target, verb := stateClosed, "Close"
	if strings.EqualFold(iss.State, stateClosed) {
		target, verb = prStateOpen, "Reopen"
	}
	p.clearPending()
	p.pending = opIssueCloseReopen
	// Encode the issue number and target state for the modal handler.
	p.pendingName = fmt.Sprintf("%d:%s", iss.Number, target)
	return p, notify.ShowConfirm(
		fmt.Sprintf("%s Issue #%d", verb, iss.Number),
		iss.Title,
	)
}

// handleIssueCloseReopenConfirm runs the async close/reopen once the user
// confirms the modal. The pending name is "<number>:<targetState>".
func (p *Panel) handleIssueCloseReopenConfirm(a modalArgs) (panels.Panel, tea.Cmd) {
	number, target, ok := parseIssueStateName(a.name)
	if !ok {
		return p, nil
	}
	return p, p.closeReopenIssueCmd(number, target)
}

// parseIssueStateName splits a "<number>:<state>" pending name.
func parseIssueStateName(name string) (number int, state string, ok bool) {
	idx := strings.LastIndex(name, ":")
	if idx <= 0 || idx == len(name)-1 {
		return 0, "", false
	}
	n, err := strconv.Atoi(name[:idx])
	if err != nil {
		return 0, "", false
	}
	return n, name[idx+1:], true
}

// closeReopenIssueCmd returns a tea.Cmd that closes or reopens an issue.
func (p *Panel) closeReopenIssueCmd(number int, targetState string) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return ghClientUnavailableCmd()
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		var err error
		if targetState == prStateOpen {
			err = client.ReopenIssue(ctx, owner, repo, number)
		} else {
			err = client.CloseIssue(ctx, owner, repo, number)
		}
		return issueStateResultMsg{number: number, newState: targetState, err: err}
	}
}

// handleIssueStateResult processes the async result of a close/reopen op.
func (p *Panel) handleIssueStateResult(msg issueStateResultMsg) (panels.Panel, tea.Cmd) {
	verb := stateClosed
	if msg.newState == prStateOpen {
		verb = "reopened"
	}
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Issue #%d %s failed: %s", msg.number, verb, errStr),
				Level:   notify.Error,
			}
		}
	}
	// Update cached issue state.
	for i := range p.gh.allIssues {
		if p.gh.allIssues[i].Number == msg.number {
			p.gh.allIssues[i].State = msg.newState
			break
		}
	}
	// Update the visible tab item too.
	for i := range p.tabItems[tabIssues] {
		if p.tabItems[tabIssues][i].kind == kindIssue && p.tabItems[tabIssues][i].issue.Number == msg.number {
			p.tabItems[tabIssues][i].issue.State = msg.newState
			break
		}
	}
	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Issue #%d %s", msg.number, verb),
				Level:   notify.Success,
			}
		},
		p.loadGitHubData(),
	)
}

// ---------------------------------------------------------------------------
// GitHub PR creation
// ---------------------------------------------------------------------------

// doCreatePR starts the multi-step flow to open a pull request from the TUI.
// Head is prefilled with the current local branch and base with the repo
// default branch; both remain editable in the modal.
func (p *Panel) doCreatePR() (panels.Panel, tea.Cmd) {
	if p.gh.client == nil {
		return p, nil
	}
	head := p.currentLocalBranch()
	if head == "" {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Cannot open PR: no current branch detected",
				Level:   notify.Warn,
			}
		}
	}
	if !p.branchIsPushed(head) {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Push branch %q before opening a PR", head),
				Level:   notify.Warn,
			}
		}
	}
	base := p.gh.defaultBranch
	if base == "" {
		base = branchMain
	}
	p.clearPending()
	p.prDraft = prCreateDraft{head: head, base: base}
	p.pending = opPRCreateHead
	return p, notify.ShowInputWithValue("PR Head Branch", "head-branch", head)
}

// currentLocalBranch returns the name of the current local branch by scanning
// the full branch list, which includes local branches even in GitHub mode.
// It returns "" when no current local branch can be determined.
func (p *Panel) currentLocalBranch() string {
	for _, b := range p.gitData.lastBranches {
		if !b.IsRemote && b.IsCurrent {
			return b.Name
		}
	}
	return ""
}

// remoteBranchName strips the leading remote name from a remote-tracking ref,
// e.g. "origin/main" becomes "main".
func remoteBranchName(name string) string {
	if i := strings.IndexByte(name, '/'); i >= 0 {
		return name[i+1:]
	}
	return name
}

// branchIsPushed reports whether the named local branch appears to exist on a
// remote: either it tracks an upstream, or a remote branch with a matching
// short name is present in the branch list.
func (p *Panel) branchIsPushed(name string) bool {
	for _, b := range p.gitData.lastBranches {
		if !b.IsRemote && b.Name == name && b.Upstream != "" {
			return true
		}
		if b.IsRemote && remoteBranchName(b.Name) == name {
			return true
		}
	}
	return false
}

// createPRCmd returns a tea.Cmd that opens the pull request asynchronously.
func (p *Panel) createPRCmd(head, base, title, body string) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return ghClientUnavailableCmd()
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	return func() tea.Msg {
		req := &gh.NewPullRequest{
			Title: gh.Ptr(title),
			Head:  gh.Ptr(head),
			Base:  gh.Ptr(base),
		}
		if body != "" {
			req.Body = gh.Ptr(body)
		}
		created, err := client.CreatePR(ctx, owner, repo, req)
		if err != nil {
			return prCreateResultMsg{err: err}
		}
		if created == nil {
			return prCreateResultMsg{err: fmt.Errorf("no pull request returned")}
		}
		author := ""
		if created.User != nil {
			author = created.User.GetLogin()
		}
		state := created.GetState()
		if created.GetDraft() {
			state = stateDraft
		}
		return prCreateResultMsg{pr: ghPRItem{
			Number:     created.GetNumber(),
			Title:      created.GetTitle(),
			State:      state,
			HeadBranch: created.GetHead().GetRef(),
			Author:     author,
			HTMLURL:    created.GetHTMLURL(),
		}}
	}
}

// handlePRCreateResult processes the async result of opening a pull request.
func (p *Panel) handlePRCreateResult(msg prCreateResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Create PR failed: " + errStr,
				Level:   notify.Error,
			}
		}
	}
	// Show all PRs so the new one is guaranteed visible, then insert it
	// optimistically for immediate feedback ahead of the server refresh.
	p.gh.prFilter = prFilterAll
	exists := false
	for i := range p.gh.allPRs {
		if p.gh.allPRs[i].Number == msg.pr.Number {
			p.gh.allPRs[i] = msg.pr
			exists = true
			break
		}
	}
	if !exists {
		p.gh.allPRs = append([]ghPRItem{msg.pr}, p.gh.allPRs...)
	}
	p.applyPRFilter()
	p.selectPRByNumber(msg.pr.Number)
	// Refresh from the server and reselect the new PR once it arrives.
	p.gh.pendingSelectPR = msg.pr.Number
	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("PR #%d created", msg.pr.Number),
				Level:   notify.Success,
			}
		},
		p.loadGitHubData(),
	)
}

// selectPRByNumber moves the PRs-tab cursor to the PR with the given number,
// if it is present in the currently visible list.
func (p *Panel) selectPRByNumber(number int) {
	if number == 0 {
		return
	}
	if p.activeTab == tabPRs && p.filteredIdx != nil {
		for i := range p.activeItemCount() {
			item, ok := p.activeItemAt(i)
			if ok && item.kind == kindPR && item.pr.Number == number {
				p.tabCursor[tabPRs] = i
				p.ensureCursorVisible()
				return
			}
		}
		return
	}
	for i, item := range p.tabItems[tabPRs] {
		if item.kind == kindPR && item.pr.Number == number {
			p.tabCursor[tabPRs] = i
			if p.activeTab == tabPRs {
				p.ensureCursorVisible()
			}
			return
		}
	}
}

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

// assignSelfResultMsg carries the result of assigning an issue or PR to the
// current user.
type assignSelfResultMsg struct {
	kind   string // "issue" or "PR"
	login  string
	err    error
	number int
}

// doAssignSelf assigns the item under the cursor (Issues or PRs tab) to the
// current user.
func (p *Panel) doAssignSelf() (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return p, nil
	}
	switch items[cursor].kind { //nolint:exhaustive // only issues and PRs are assignable
	case kindIssue:
		return p.doAssignSelfFor(assignKindIssue, items[cursor].issue.Number)
	case kindPR:
		return p.doAssignSelfFor(assignKindPR, items[cursor].pr.Number)
	default:
		return p, nil
	}
}

// doAssignSelfFor prepares a confirmation modal to assign the given issue or
// PR to the current user.
func (p *Panel) doAssignSelfFor(kind string, number int) (panels.Panel, tea.Cmd) {
	if p.gh.client == nil || number == 0 {
		return p, nil
	}
	p.clearPending()
	p.pending = opAssignSelf
	// Encode the kind and number for the modal handler.
	p.pendingName = fmt.Sprintf("%s:%d", kind, number)
	who := p.gh.user
	if who == "" {
		who = "yourself"
	}
	return p, notify.ShowConfirm(
		fmt.Sprintf("Assign %s #%d", kind, number),
		fmt.Sprintf("Assign this %s to %s?", kind, who),
	)
}

// handleAssignSelfConfirm runs the async assignment after modal confirmation.
// The pending name is "<kind>:<number>".
func (p *Panel) handleAssignSelfConfirm(a modalArgs) (panels.Panel, tea.Cmd) {
	kind, number, ok := parseAssignSelfName(a.name)
	if !ok {
		return p, nil
	}
	return p, p.assignSelfCmd(kind, number)
}

// parseAssignSelfName splits a "<kind>:<number>" pending name.
func parseAssignSelfName(name string) (kind string, number int, ok bool) {
	idx := strings.LastIndex(name, ":")
	if idx <= 0 || idx == len(name)-1 {
		return "", 0, false
	}
	n, err := strconv.Atoi(name[idx+1:])
	if err != nil {
		return "", 0, false
	}
	return name[:idx], n, true
}

// assignSelfCmd assigns the issue/PR to the current user asynchronously. If the
// current user login is not cached, it is fetched first.
func (p *Panel) assignSelfCmd(kind string, number int) tea.Cmd {
	client := p.gh.client
	if client == nil {
		return ghClientUnavailableCmd()
	}
	owner, repo := p.gh.owner, p.gh.repo
	ctx := p.ctx
	login := p.gh.user
	return func() tea.Msg {
		who := login
		if who == "" {
			user, err := client.CurrentUser(ctx)
			if err != nil {
				return assignSelfResultMsg{kind: kind, number: number, err: err}
			}
			if user != nil && user.Login != nil {
				who = *user.Login
			}
		}
		if who == "" {
			return assignSelfResultMsg{kind: kind, number: number, err: fmt.Errorf("could not determine current user")}
		}
		err := client.AddAssignees(ctx, owner, repo, number, []string{who})
		return assignSelfResultMsg{kind: kind, number: number, login: who, err: err}
	}
}

// handleAssignSelfResult processes the async result of an assign-to-me op.
func (p *Panel) handleAssignSelfResult(msg assignSelfResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errStr := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Assign %s #%d failed: %s", msg.kind, msg.number, errStr),
				Level:   notify.Error,
			}
		}
	}
	// Reflect assignment locally for issues (PR items do not display assignee).
	if msg.kind == assignKindIssue {
		for i := range p.gh.allIssues {
			if p.gh.allIssues[i].Number == msg.number {
				p.gh.allIssues[i].Assignee = msg.login
				break
			}
		}
		for i := range p.tabItems[tabIssues] {
			if p.tabItems[tabIssues][i].kind == kindIssue && p.tabItems[tabIssues][i].issue.Number == msg.number {
				p.tabItems[tabIssues][i].issue.Assignee = msg.login
				break
			}
		}
	}
	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Assigned %s #%d to %s", msg.kind, msg.number, msg.login),
				Level:   notify.Success,
			}
		},
		p.loadGitHubData(),
	)
}
