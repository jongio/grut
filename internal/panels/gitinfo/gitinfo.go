// Package gitinfo implements a combined panel that displays branches,
// worktrees, and remotes as switchable tabs within a single panel.
// Tabs are selected with 1/2/3 keys or by clicking the tab bar.
package gitinfo

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	gh "github.com/google/go-github/v68/github"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	ghclient "github.com/jongio/grut/internal/github"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/rightclick"
	"github.com/mattn/go-runewidth"
)

// gitOps defines the git operations required by the gitinfo panel.
// This narrow interface is satisfied by *git.Client and makes the
// panel easy to mock in tests.
type gitOps interface {
	BranchList(ctx context.Context) ([]git.Branch, error)
	BranchCreate(ctx context.Context, name string, base string) error
	BranchDelete(ctx context.Context, name string, force bool) error
	BranchRename(ctx context.Context, oldName, newName string) error
	Checkout(ctx context.Context, ref string) error
	WorktreeList(ctx context.Context) ([]git.Worktree, error)
	WorktreeAdd(ctx context.Context, path, branch string) error
	WorktreeRemove(ctx context.Context, path string, force bool) error
	RemoteList(ctx context.Context) ([]git.Remote, error)
	RemoteAdd(ctx context.Context, name, url string) error
	RemoteRemove(ctx context.Context, name string) error
	Fetch(ctx context.Context, opts git.FetchOpts) error
	StashList(ctx context.Context) ([]git.StashEntry, error)
	StashApply(ctx context.Context, index int) error
	StashPop(ctx context.Context, index int) error
	StashDrop(ctx context.Context, index int) error
	TagList(ctx context.Context) ([]git.Tag, error)
	TagCreate(ctx context.Context, name, ref, message string) error
	TagDelete(ctx context.Context, name string) error
	TagPush(ctx context.Context, remote, name string) error
	Reflog(ctx context.Context, ref string, limit int) ([]git.ReflogEntry, error)
}

// ---------------------------------------------------------------------------
// Internal types
// ---------------------------------------------------------------------------
// itemKind distinguishes item types in the flat display list.
type itemKind int

const (
	kindLocalBranch  itemKind = iota // local branch
	kindRemoteBranch                 // remote branch
	kindWorktree                     // worktree entry
	kindRemote                       // remote name
	kindRemoteSub                    // remote sub-detail (fetch/push URL)
	kindStashEntry                   // stash entry
	kindIssue                        // GitHub issue
	kindPR                           // GitHub pull request
	kindActionRun                    // GitHub Actions workflow run
	kindWorkflow                     // GitHub Actions workflow definition
	kindRelease                      // GitHub release
	kindTag                          // local tag
	kindRemoteTag                    // remote-only tag
	kindReflogEntry                  // reflog entry
)

// PanelMode controls which subset of tabs a gitinfo panel displays.
type PanelMode int

const (
	ModeAll    PanelMode = iota // show all tabs (backwards compat)
	ModeGit                     // git tabs only: branches, worktrees, remotes, stash, tags, reflog
	ModeGitHub                  // GitHub tabs only: issues, PRs, actions, workflows, releases
)

// tabID identifies the three tabs.
type tabID int

const (
	tabBranches tabID = iota
	tabWorktrees
	tabRemotes
	tabStash
	tabTags
	tabReflog
	tabIssues
	tabPRs
	tabActions
	tabWorkflows
	tabReleases
	tabCount // sentinel for array sizing
)

// listItem represents a single row in a tab's item list.
type listItem struct {
	tag       git.Tag         // tag data (valid when kind == kindTag/kindRemoteTag)
	reflog    git.ReflogEntry // reflog data (valid when kind == kindReflogEntry)
	issue     ghIssueItem     // issue data (valid when kind == kindIssue)
	release   ghReleaseItem   // release data (valid when kind == kindRelease)
	actionRun ghActionItem    // action run data (valid when kind == kindActionRun)
	pr        ghPRItem        // PR data (valid when kind == kindPR)
	stash     git.StashEntry  // stash data (valid when kind == kindStashEntry)
	branch    git.Branch      // branch data (valid when kind == kindLocalBranch/kindRemoteBranch)
	workflow  ghWorkflowItem  // workflow definition (valid when kind == kindWorkflow)
	remote    git.Remote      // remote data (valid when kind == kindRemote)
	text      string          // display text for sub-items (kind == kindRemoteSub)
	hash      string          // hash for clipboard copy (extracted for click targeting)
	worktree  git.Worktree    // worktree data (valid when kind == kindWorktree)
	kind      itemKind
}

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
var watchFrames = []string{"●", "◐", "○", "◑"}

// checkMark is the success icon used in status indicators.
const checkMark = "✓"

// crossMark is the failure icon used in status indicators.
const crossMark = "✗"

// actionsWatchTickInterval is the polling interval for the GitHub Actions
// watch animation frame rate.
const actionsWatchTickInterval = 1000 * time.Millisecond

// IssueFilterKind identifies the active quick-filter for the Issues tab.
type IssueFilterKind int

const (
	issueFilterAll       IssueFilterKind = iota
	issueFilterAssigned                  // assignee == current user
	issueFilterMentioned                 // placeholder — shows all
	issueFilterCreated                   // author == current user
)

func (f IssueFilterKind) String() string {
	switch f {
	case issueFilterAssigned:
		return "Assigned"
	case issueFilterMentioned:
		return "Mentioned"
	case issueFilterCreated:
		return "Created"
	default:
		return "All"
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
		return "Draft"
	default:
		return "All"
	}
}

// pendingOp identifies which operation is awaiting modal input.
type pendingOp int

const (
	opNone                     pendingOp = iota
	opBranchCreate                       // awaiting new branch name
	opBranchDelete                       // awaiting delete confirmation
	opBranchRename                       // awaiting new name
	opBranchCheckout                     // awaiting checkout confirmation
	opWorktreeCreate                     // awaiting new branch name for worktree
	opWorktreeDelete                     // awaiting delete confirmation
	opRemoteAdd                          // awaiting remote name
	opRemoteAddURL                       // awaiting remote URL (second step)
	opRemoteDelete                       // awaiting delete confirmation
	opStashAction                        // awaiting stash action (apply/pop/drop)
	opFirstUseConfirm                    // awaiting first-use double-click confirmation
	opRightClickPick                     // awaiting right-click action picker result
	opTagCreate                          // awaiting new tag name
	opTagMessage                         // awaiting optional tag message
	opTagDelete                          // awaiting tag delete confirmation
	opTagPush                            // awaiting tag push confirmation
	opTagCheckout                        // awaiting tag checkout confirmation (detached HEAD)
	opWorkflowDispatch                   // awaiting workflow dispatch ref input
	opWorkflowDispatchInputs             // awaiting workflow dispatch inputs (key=value)
	opPRMergeStrategy                    // awaiting merge strategy selection
	opPRMergeConfirm                     // awaiting merge confirmation
	opPRDeleteBranchAfterMerge           // awaiting post-merge branch deletion confirmation
)

// ---------------------------------------------------------------------------
// Internal messages (async result messages)
// ---------------------------------------------------------------------------
// dataLoadedMsg carries the result of an async data load.
type dataLoadedMsg struct {
	err       error
	branches  []git.Branch
	worktrees []git.Worktree
	remotes   []git.Remote
	stashes   []git.StashEntry
	tags      []git.Tag
	reflog    []git.ReflogEntry
}

// opResultMsg carries the result of an async operation.
type opResultMsg struct {
	err  error
	op   string // e.g. "checkout", "branch_created", "worktree_added"
	name string // name involved
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

// ---------------------------------------------------------------------------
// Default colors (Dracula-inspired, consistent with other panels)
// ---------------------------------------------------------------------------
var defaultColors = struct {
	Current    string
	Local      string
	Remote     string
	Header     string
	Hash       string
	CursorBg   string
	Dim        string
	Worktree   string
	RemoteC    string
	URL        string
	Issue      string
	PR         string
	PRConflict string
	PRUnstable string
	PRBlocked  string
	PRUnknown  string
	PRClosed   string
	PRDraft    string
	PRMerged   string
	ActionOK   string
	ActionFail string
	ActionRun  string
	Tag        string
	RemoteTag  string
	Release    string
	RelDraft   string
	RelPre     string
	Workflow   string
}{
	Current:    "#50FA7B",
	Local:      "#F8F8F2",
	Remote:     "#BD93F9",
	Header:     "#8BE9FD",
	Hash:       "#6272A4",
	CursorBg:   "#44475A",
	Dim:        "#666666",
	Worktree:   "#FFB86C",
	RemoteC:    "#FF79C6",
	URL:        "#6272A4",
	Issue:      "#F8F8F2",
	PR:         "#50FA7B",
	PRConflict: "#FF5555",
	PRUnstable: "#F1FA8C",
	PRBlocked:  "#FFB86C",
	PRUnknown:  "#6272A4",
	PRClosed:   "#994444",
	PRDraft:    "#FFB86C",
	PRMerged:   "#BD93F9",
	ActionOK:   "#50FA7B",
	ActionFail: "#FF5555",
	ActionRun:  "#F1FA8C",
	Tag:        "#FF79C6",
	RemoteTag:  "#BD93F9",
	Release:    "#50FA7B",
	RelDraft:   "#6272A4",
	RelPre:     "#FFB86C",
	Workflow:   "#8BE9FD",
}

// ---------------------------------------------------------------------------
// Panel
// ---------------------------------------------------------------------------
// Panel is the gitinfo panel. It implements [panels.Panel].
type Panel struct {
	actionsCfg config.ActionsConfig
	git        gitOps
	ctx        context.Context
	// GitHub integration fields.
	ghClient    ghclient.Client // may be nil if auth fails
	ghErr       error           // non-nil if GitHub init failed
	iconMode    string          // "nerd" or "ascii"
	repoRoot    string
	pendingName string // name for pending operation
	ghOwner     string
	ghRepo      string
	ghUser      string               // authenticated user login
	tabItems    [tabCount][]listItem // items per tab
	// Cached data for rebuild.
	lastBranches  []git.Branch
	lastWorktrees []git.Worktree
	lastRemotes   []git.Remote
	lastStashes   []git.StashEntry
	lastTags      []git.Tag
	lastReflog    []git.ReflogEntry
	allIssues     []ghIssueItem // unfiltered issue list
	allPRs        []ghPRItem    // unfiltered PR list
	ghCfg         config.GitHubConfig
	panels.BasePanel
	cfg               config.GitConfig
	tabCursor         [tabCount]int   // cursor per tab
	tabOffset         [tabCount]int   // viewport offset per tab
	mode              PanelMode       // which tab subset to display
	activeTab         tabID           // currently active tab
	remoteCount       int             // actual number of remotes (distinct from tabItems len which includes sub-rows)
	pending           pendingOp       // operation awaiting modal result
	issueFilter       IssueFilterKind // current issue quick-filter
	prFilter          PRFilterKind    // current PR quick-filter
	actionsWatchFrame int             // current animation frame index into watchFrames
	lastWidth         int             // last rendered width, used for click zone calculation
	// Repo visibility — true when the GitHub repo is private.
	repoPrivate bool
	// CI watch animation state — animated indicator when in-progress runs exist.
	actionsWatching bool // true when in-progress/queued runs exist AND polling is active
}

// Compile-time interface check.
var _ panels.Panel = (*Panel)(nil)

// isGitTab returns true if t is a git-only tab (branches through reflog).
func isGitTab(t tabID) bool { return t <= tabReflog }

// isGitHubTab returns true if t is a tab shown in the GitHub pane.
// This includes branches and tags (shared with git pane) plus the
// GitHub-only tabs (issues through releases).
func isGitHubTab(t tabID) bool {
	return t == tabBranches || t == tabTags || (t >= tabIssues && t < tabCount)
}

// visibleTabs returns the ordered slice of tab IDs that should be
// visible for the panel's current mode. This is used for Tab/Shift+Tab
// cycling within the panel.
func (p *Panel) visibleTabs() []tabID {
	switch p.mode {
	case ModeGit:
		return []tabID{tabBranches, tabWorktrees, tabRemotes, tabStash, tabTags, tabReflog}
	case ModeGitHub:
		return []tabID{tabBranches, tabTags, tabIssues, tabPRs, tabActions, tabWorkflows, tabReleases}
	default: // ModeAll
		if isGitTab(p.activeTab) {
			return []tabID{tabBranches, tabWorktrees, tabRemotes, tabStash, tabTags, tabReflog}
		}
		return []tabID{tabBranches, tabTags, tabIssues, tabPRs, tabActions, tabWorkflows, tabReleases}
	}
}

// tabBarHeight returns the number of rows occupied by the tab bar
// based on the panel's mode and GitHub availability.
func (p *Panel) tabBarHeight() int {
	switch p.mode {
	case ModeGit:
		return 1 // git row only
	case ModeGitHub:
		return 1 // GitHub row only
	default: // ModeAll
		if p.ghClient != nil {
			return 2 // git row + GitHub row
		}
		return 1 // git row only (GitHub unavailable)
	}
}

// SetActiveTab switches the active tab by name.
// Valid names: "branches", "worktrees", "remotes", "stash", "tags", "reflog", "issues", "prs", "actions".
func (p *Panel) SetActiveTab(name string) {
	switch name {
	case "branches":
		p.activeTab = tabBranches
	case "worktrees":
		p.activeTab = tabWorktrees
	case "remotes":
		p.activeTab = tabRemotes
	case "stash":
		p.activeTab = tabStash
	case "tags":
		p.activeTab = tabTags
	case "reflog":
		p.activeTab = tabReflog
	case "issues":
		p.activeTab = tabIssues
	case "prs":
		p.activeTab = tabPRs
	case "actions":
		p.activeTab = tabActions
	case "workflows":
		p.activeTab = tabWorkflows
	case "releases":
		p.activeTab = tabReleases
	}
}

// New creates a new gitinfo panel showing only git tabs (branches,
// worktrees, remotes, stash, tags, reflog).
func New(gitOps gitOps, cfg config.GitConfig, ghCfg config.GitHubConfig, actionsCfg config.ActionsConfig, repoRoot, iconMode string) *Panel {
	return &Panel{
		BasePanel:  panels.BasePanel{PanelTitle: "gitinfo"},
		mode:       ModeGit,
		git:        gitOps,
		cfg:        cfg,
		ghCfg:      ghCfg,
		actionsCfg: actionsCfg,
		iconMode:   iconMode,
		repoRoot:   repoRoot,
	}
}

// NewGitHub creates a gitinfo panel showing only GitHub tabs (issues,
// PRs, actions, workflows, releases).
func NewGitHub(gitOps gitOps, cfg config.GitConfig, ghCfg config.GitHubConfig, actionsCfg config.ActionsConfig, repoRoot, iconMode string) *Panel {
	return &Panel{
		BasePanel:  panels.BasePanel{PanelTitle: "github"},
		mode:       ModeGitHub,
		activeTab:  tabIssues,
		git:        gitOps,
		cfg:        cfg,
		ghCfg:      ghCfg,
		actionsCfg: actionsCfg,
		iconMode:   iconMode,
		repoRoot:   repoRoot,
	}
}

// ---------------------------------------------------------------------------
// panels.Panel interface
// ---------------------------------------------------------------------------
// Init implements panels.Panel.
func (p *Panel) Init(ctx context.Context) tea.Cmd {
	p.ctx = ctx
	// Resolve GitHub owner/repo from config or git remote.
	p.ghOwner, p.ghRepo = p.ghCfg.ResolveGitHubRepo(ctx, p.repoRoot)
	// Only create GitHub client when we have a valid owner/repo.
	if p.ghOwner != "" && p.ghRepo != "" {
		client, err := ghclient.NewClient(ctx)
		if err != nil {
			p.ghErr = fmt.Errorf("GitHub auth unavailable: %w", err)
		} else {
			p.ghClient = client
		}
	}
	// Load git data + GitHub data in parallel.
	cmds := []tea.Cmd{p.loadData()}
	if p.ghClient != nil {
		cmds = append(cmds, p.loadGitHubData(), p.githubPollTickCmd())
	}
	return tea.Batch(cmds...)
}

// handleRepoChanged replaces the git client and resets all tab data for the
// new repository after a directory change.
func (p *Panel) handleRepoChanged(msg panels.RepoChangedMsg) (panels.Panel, tea.Cmd) {
	client, err := git.NewClient(msg.Path)
	if err != nil {
		p.git = nil
	} else {
		p.git = client
	}
	p.repoRoot = msg.Path
	// Clear all cached git data.
	p.lastBranches = nil
	p.lastWorktrees = nil
	p.lastRemotes = nil
	p.lastStashes = nil
	p.lastTags = nil
	p.lastReflog = nil
	for i := range p.tabItems {
		p.tabItems[i] = nil
		p.tabCursor[i] = 0
		p.tabOffset[i] = 0
	}
	// Reset GitHub client — the new directory may be a different repo.
	p.ghClient = nil
	p.ghOwner = ""
	p.ghRepo = ""
	p.ghUser = ""
	p.ghErr = nil
	p.allIssues = nil
	p.allPRs = nil
	p.actionsWatching = false
	p.actionsWatchFrame = 0
	p.repoPrivate = false
	// Re-resolve GitHub owner/repo for the new directory.
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	p.ghOwner, p.ghRepo = p.ghCfg.ResolveGitHubRepo(ctx, p.repoRoot)
	if p.ghOwner != "" && p.ghRepo != "" {
		ghc, ghErr := ghclient.NewClient(ctx)
		if ghErr != nil {
			p.ghErr = fmt.Errorf("GitHub auth unavailable: %w", ghErr)
		} else {
			p.ghClient = ghc
		}
	}
	if p.git == nil {
		return p, nil
	}
	cmds := []tea.Cmd{p.loadData()}
	if p.ghClient != nil {
		cmds = append(cmds, p.loadGitHubData(), p.githubPollTickCmd())
	}
	return p, tea.Batch(cmds...)
}

func (p *Panel) loadData() tea.Cmd {
	g := p.git
	ctx := p.ctx
	return func() tea.Msg {
		branches, brErr := g.BranchList(ctx)
		if brErr != nil {
			return dataLoadedMsg{err: brErr}
		}
		worktrees, wtErr := g.WorktreeList(ctx)
		if wtErr != nil {
			return dataLoadedMsg{err: wtErr}
		}
		remotes, rmErr := g.RemoteList(ctx)
		if rmErr != nil {
			return dataLoadedMsg{err: rmErr}
		}
		stashes, stErr := g.StashList(ctx)
		if stErr != nil {
			return dataLoadedMsg{err: stErr}
		}
		tags, tgErr := g.TagList(ctx)
		if tgErr != nil {
			return dataLoadedMsg{err: tgErr}
		}
		reflog, rlErr := g.Reflog(ctx, "HEAD", 100)
		if rlErr != nil {
			return dataLoadedMsg{err: rlErr}
		}
		return dataLoadedMsg{
			branches:  branches,
			worktrees: worktrees,
			remotes:   remotes,
			stashes:   stashes,
			tags:      tags,
			reflog:    reflog,
		}
	}
}

// githubPollTickCmd returns a tea.Tick that triggers periodic GitHub data refresh.
// The tick is wrapped in a TargetedPanelMsg so only the gitinfo panel receives
// it, avoiding a full broadcast to all panels.
func (p *Panel) githubPollTickCmd() tea.Cmd {
	if p.ghClient == nil || p.ghCfg.PollInterval <= 0 {
		return nil
	}
	d := time.Duration(p.ghCfg.PollInterval) * time.Second
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return panels.TargetedPanelMsg{
			Target: "gitinfo",
			Inner:  githubPollTickMsg{t},
		}
	})
}

// actionsWatchTickCmd returns a tea.Tick that advances the watch animation
// frame every actionsWatchTickInterval. The tick self-terminates when
// actionsWatching is false. Wrapped in TargetedPanelMsg to avoid broadcast.
func (p *Panel) actionsWatchTickCmd() tea.Cmd {
	if !p.actionsWatching {
		return nil
	}
	return tea.Tick(actionsWatchTickInterval, func(t time.Time) tea.Msg {
		return panels.TargetedPanelMsg{
			Target: "gitinfo",
			Inner:  actionsWatchTickMsg{t},
		}
	})
}

// Update implements panels.Panel.
func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case dataLoadedMsg:
		return p.handleDataLoaded(msg)
	case ghDataLoadedMsg:
		return p.handleGHDataLoaded(msg)
	case githubPollTickMsg:
		return p, tea.Batch(p.loadGitHubData(), p.githubPollTickCmd())
	case actionsWatchTickMsg:
		if !p.actionsWatching {
			return p, nil // stop ticking — no in-progress runs
		}
		p.actionsWatchFrame = (p.actionsWatchFrame + 1) % len(watchFrames)
		return p, p.actionsWatchTickCmd()
	case opResultMsg:
		return p.handleOpResult(msg)
	case tea.KeyPressMsg:
		return p.handleKey(msg)
	case panels.PanelMouseClickMsg:
		return p.handleMouseClick(msg)
	case panels.PanelMouseDoubleClickMsg:
		return p.handleMouseDoubleClick(msg)
	case panels.PanelHeaderDoubleClickMsg:
		return p.openRepoInBrowser()
	case panels.PanelMouseRightClickMsg:
		return p.handleMouseRightClick(msg)
	case tea.MouseWheelMsg:
		return p.handleMouseWheel(msg)
	case notify.ModalResultMsg:
		return p.handleModalResult(msg)
	case panels.BranchChangedMsg, panels.RefreshBranchesMsg:
		return p, p.loadData()
	case panels.WorktreeChangedMsg:
		return p, p.loadData()
	case panels.RemoteChangedMsg:
		return p, p.loadData()
	case panels.StashChangedMsg:
		return p, p.loadData()
	case panels.TagChangedMsg:
		return p, p.loadData()
	case panels.RepoChangedMsg:
		return p.handleRepoChanged(msg)
	case prDetailsLoadedMsg:
		return p.handlePRDetailsLoaded(msg)
	case actionJobsLoadedMsg:
		return p.handleActionJobsLoaded(msg)
	case actionLogLoadedMsg:
		return p.handleActionLogLoaded(msg)
	case actionRerunResultMsg:
		return p.handleActionRerunResult(msg)
	case actionCancelResultMsg:
		return p.handleActionCancelResult(msg)
	case workflowDispatchResultMsg:
		return p.handleWorkflowDispatchResult(msg)
	case workflowInputsFetchedMsg:
		return p.handleWorkflowInputsFetched(msg)

	case prMergeResultMsg:
		return p.handlePRMergeResult(msg)
	case prBranchDeleteResultMsg:
		return p.handlePRBranchDeleteResult(msg)

	// CRUD actions dispatched via keymap.
	case panels.ItemCreateMsg:
		if !p.Focused {
			return p, nil
		}
		return p.doCreate()
	case panels.ItemDeleteMsg:
		if !p.Focused {
			return p, nil
		}
		return p.doDelete()
	case panels.ItemEditMsg:
		if !p.Focused {
			return p, nil
		}
		return p.doRename()
	case panels.ItemOpenMsg:
		if !p.Focused {
			return p, nil
		}
		return p.doOpenInBrowser()
	case panels.ItemCopyMsg:
		if !p.Focused {
			return p, nil
		}
		return p.copyHashToClipboard()
	}
	return p, nil
}

// View implements panels.Panel.
func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	// Tab bar: height depends on mode and GitHub availability.
	tbh := p.tabBarHeight()
	tabBar := p.renderTabBar(width)
	contentHeight := height - tbh
	if contentHeight <= 0 {
		return tabBar
	}
	items := p.tabItems[p.activeTab]
	if len(items) == 0 {
		label := "No items"
		if p.activeTab >= tabIssues && p.ghErr != nil {
			label = "GitHub unavailable"
		}
		empty := lipgloss.NewStyle().
			Width(width).Height(contentHeight).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color(defaultColors.Dim)).
			Render(label)
		return tabBar + "\n" + empty
	}
	cursor := p.tabCursor[p.activeTab]
	offset := p.tabOffset[p.activeTab]
	lines := make([]string, 0, contentHeight)
	end := offset + contentHeight
	if end > len(items) {
		end = len(items)
	}
	for i := offset; i < end; i++ {
		lines = append(lines, p.renderLine(items[i], width, i == cursor))
	}
	// Pad remaining height with blank lines.
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	for len(lines) < contentHeight {
		lines = append(lines, emptyLine)
	}
	return tabBar + "\n" + strings.Join(lines, "\n")
}

// Title implements panels.Panel, overriding BasePanel.
func (p *Panel) Title() string {
	if p.mode == ModeGitHub {
		if p.repoPrivate {
			if p.iconMode == "nerd" {
				return "GitHub \uf023"
			}
			return "GitHub (private)"
		}
		return "GitHub"
	}
	return "Git"
}

// KeyBindings implements panels.Panel.
func (p *Panel) KeyBindings() []panels.KeyBinding {
	bindings := []panels.KeyBinding{
		{Key: "b", Description: "Branches tab", Action: "tab_branches"},
		{Key: "w", Description: "Worktrees / Workflows tab", Action: "tab_worktrees"},
		{Key: "r", Description: "Remotes tab / Rerun action", Action: "tab_remotes"},
		{Key: "s", Description: "Stash tab", Action: "tab_stash"},
		{Key: "t", Description: "Tags tab", Action: "tab_tags"},
		{Key: "l", Description: "Reflog / Releases tab", Action: "tab_reflog"},
		{Key: "j/↓", Description: "Move cursor down", Action: "cursor_down"},
		{Key: "k/↑", Description: "Move cursor up", Action: "cursor_up"},
		{Key: "d", Description: "Page down", Action: "page_down"},
		{Key: "u", Description: "Page up", Action: "page_up"},
		{Key: "enter", Description: "Context action", Action: "action"},
		{Key: "n", Description: "Create new item", Action: "item_create"},
		{Key: "x", Description: "Delete / Cancel action", Action: "item_delete"},
		{Key: "e/F2", Description: "Edit/rename item", Action: "item_edit"},
		{Key: "o", Description: "Open in browser", Action: "item_open"},
		{Key: "y", Description: "Copy to clipboard", Action: "item_copy"},
		{Key: "f", Description: "Fetch / Filter", Action: "fetch_or_filter"},
		{Key: "g", Description: "Go to first item", Action: "first"},
		{Key: "G", Description: "Go to last item", Action: "last"},
		{Key: "P", Description: "Push tag", Action: "push_tag"},
		{Key: "D", Description: "Dispatch workflow", Action: "workflow_dispatch"},
	}
	if p.ghClient != nil {
		bindings = append(bindings,
			panels.KeyBinding{Key: "i", Description: "Issues tab", Action: "tab_issues"},
			panels.KeyBinding{Key: "p", Description: "PRs tab", Action: "tab_prs"},
			panels.KeyBinding{Key: "a", Description: "Actions tab", Action: "tab_actions"},
			panels.KeyBinding{Key: "W", Description: "Workflows tab", Action: "tab_workflows"},
			panels.KeyBinding{Key: "L", Description: "Releases tab", Action: "tab_releases"},
		)
	}
	return bindings
}

// ---------------------------------------------------------------------------
// Message handlers
// ---------------------------------------------------------------------------
func (p *Panel) handleDataLoaded(msg dataLoadedMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errMsg := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Git info load error: " + errMsg, Level: notify.Error}
		}
	}
	p.buildItems(msg.branches, msg.worktrees, msg.remotes, msg.stashes, msg.tags, msg.reflog)
	return p, nil
}

func (p *Panel) handleOpResult(msg opResultMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errText := fmt.Sprintf("%s error: %s", msg.op, msg.err)
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: errText, Level: notify.Error}
		}
	}
	op := msg.op
	name := msg.name
	cmds := []tea.Cmd{p.loadData()}
	switch op {
	case "checkout":
		cmds = append(cmds,
			func() tea.Msg { return panels.BranchChangedMsg{Name: name} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Switched to " + name, Level: notify.Success}
			},
		)
	case "branch_created":
		cmds = append(cmds, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Branch created: " + name, Level: notify.Success}
		})
	case "branch_deleted":
		cmds = append(cmds, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Branch deleted: " + name, Level: notify.Success}
		})
	case "branch_renamed":
		cmds = append(cmds, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Branch renamed to: " + name, Level: notify.Success}
		})
	case "worktree_added":
		cmds = append(cmds,
			func() tea.Msg { return panels.WorktreeChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Worktree created: " + name, Level: notify.Success}
			},
		)
	case "worktree_removed":
		cmds = append(cmds,
			func() tea.Msg { return panels.WorktreeChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Worktree removed: " + name, Level: notify.Success}
			},
		)
	case "worktree_switch":
		cmds = append(cmds, func() tea.Msg {
			return panels.SwitchWorktreeMsg{Path: name}
		})
	case "remote_added":
		cmds = append(cmds,
			func() tea.Msg { return panels.RemoteChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Remote added: " + name, Level: notify.Success}
			},
		)
	case "remote_removed":
		cmds = append(cmds,
			func() tea.Msg { return panels.RemoteChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Remote removed: " + name, Level: notify.Success}
			},
		)
	case "fetched":
		cmds = append(cmds, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Fetched: " + name, Level: notify.Success}
		})
	case "stash_applied":
		cmds = append(cmds,
			func() tea.Msg { return panels.StashChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Applied " + name, Level: notify.Success}
			},
		)
	case "stash_popped":
		cmds = append(cmds,
			func() tea.Msg { return panels.StashChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Popped " + name, Level: notify.Success}
			},
		)
	case "stash_dropped":
		cmds = append(cmds,
			func() tea.Msg { return panels.StashChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Dropped " + name, Level: notify.Success}
			},
		)
	case "tag_created":
		cmds = append(cmds,
			func() tea.Msg { return panels.TagChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Tag created: " + name, Level: notify.Success}
			},
		)
	case "tag_deleted":
		cmds = append(cmds,
			func() tea.Msg { return panels.TagChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Tag deleted: " + name, Level: notify.Success}
			},
		)
	case "tag_pushed":
		cmds = append(cmds,
			func() tea.Msg { return panels.TagChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Tag pushed: " + name, Level: notify.Success}
			},
		)
	case "tag_checkout":
		cmds = append(cmds,
			func() tea.Msg { return panels.BranchChangedMsg{Name: name} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Checked out tag: " + name + " (detached HEAD)", Level: notify.Success}
			},
		)
	default:
		successMsg := fmt.Sprintf("%s: %s", op, name)
		cmds = append(cmds, func() tea.Msg {
			return notify.ShowToastMsg{Message: successMsg, Level: notify.Success}
		})
	}
	return p, tea.Batch(cmds...)
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------
func (p *Panel) handleKey(msg tea.KeyPressMsg) (panels.Panel, tea.Cmd) {
	if !p.Focused {
		return p, nil
	}
	switch msg.String() {
	case "tab":
		tabs := p.visibleTabs()
		for i, t := range tabs {
			if t == p.activeTab {
				p.activeTab = tabs[(i+1)%len(tabs)]
				return p, p.activeTabSelectionCmd()
			}
		}
		return p, nil
	case "shift+tab":
		tabs := p.visibleTabs()
		for i, t := range tabs {
			if t == p.activeTab {
				p.activeTab = tabs[(i-1+len(tabs))%len(tabs)]
				return p, p.activeTabSelectionCmd()
			}
		}
		return p, nil
	case "b":
		p.activeTab = tabBranches
		return p, p.activeTabSelectionCmd()
	case "w":
		if p.mode == ModeGitHub {
			p.activeTab = tabWorkflows
			return p, p.activeTabSelectionCmd()
		}
		p.activeTab = tabWorktrees
		return p, p.activeTabSelectionCmd()
	case "r":
		if p.activeTab == tabActions && p.ghClient != nil {
			return p.doActionsRerun()
		}
		if p.mode != ModeGitHub {
			p.activeTab = tabRemotes
			return p, p.activeTabSelectionCmd()
		}
	case "x":
		if p.activeTab == tabActions && p.ghClient != nil {
			return p.doActionsCancel()
		}
		return p.doDelete()
	case "s":
		if p.mode != ModeGitHub {
			p.activeTab = tabStash
			return p, p.activeTabSelectionCmd()
		}
	case "t":
		p.activeTab = tabTags
		return p, p.activeTabSelectionCmd()
	case "l":
		if p.mode == ModeGitHub {
			p.activeTab = tabReleases
			return p, p.activeTabSelectionCmd()
		}
		p.activeTab = tabReflog
		return p, p.activeTabSelectionCmd()
	case "i":
		if p.mode != ModeGit && p.ghClient != nil {
			p.activeTab = tabIssues
			return p, p.activeTabSelectionCmd()
		}
	case "p":
		if p.mode != ModeGit && p.ghClient != nil {
			p.activeTab = tabPRs
			return p, p.activeTabSelectionCmd()
		}
	case "a":
		if p.mode != ModeGit && p.ghClient != nil {
			p.activeTab = tabActions
			return p, p.activeTabSelectionCmd()
		}
	case "D":
		if p.activeTab == tabWorkflows && p.ghClient != nil {
			return p.doWorkflowDispatch()
		}
	case "m":
		if p.activeTab == tabPRs && p.ghClient != nil {
			return p.doMergePR()
		}
	case "d":
		p.pageDown()
		return p, p.activeTabSelectionCmd()
	case "u":
		p.pageUp()
		return p, p.activeTabSelectionCmd()
	case "n":
		return p.doCreate()
	case "e", "F2":
		return p.doRename()
	case "o":
		return p.doOpenInBrowser()
	case "y":
		return p.copyHashToClipboard()
	case "j", "down":
		p.moveCursorDown()
		return p, p.activeTabSelectionCmd()
	case "k", "up":
		p.moveCursorUp()
		return p, p.activeTabSelectionCmd()
	case "enter":
		if p.activeTab == tabReflog {
			return p.doReflogCheckout()
		}
		return p.doAction()
	case "f":
		switch p.activeTab {
		case tabIssues:
			return p.cycleIssueFilter()
		case tabPRs:
			return p.cyclePRFilter()
		default:
			return p.doFetch()
		}
	case "g":
		p.moveToFirst()
		return p, p.activeTabSelectionCmd()
	case "G":
		p.moveToLast()
		return p, p.activeTabSelectionCmd()
	case "P":
		if p.activeTab == tabTags {
			return p.doTagPush()
		}
	case "esc":
		// In ModeGitHub, Esc returns to the first GitHub tab (issues);
		// in ModeGit/ModeAll, Esc returns to branches.
		defaultTab := tabBranches
		if p.mode == ModeGitHub {
			defaultTab = tabIssues
		}
		switch p.activeTab { //nolint:exhaustive // only relevant cases handled
		case tabIssues:
			if p.mode == ModeGitHub {
				// Already on the default GitHub tab — just deselect.
				return p, tea.Batch(
					func() tea.Msg { return panels.IssueDeselectedMsg{} },
					p.activeTabSelectionCmd(),
				)
			}
			p.activeTab = defaultTab
			return p, tea.Batch(
				func() tea.Msg { return panels.IssueDeselectedMsg{} },
				p.activeTabSelectionCmd(),
			)
		case tabPRs:
			p.activeTab = defaultTab
			return p, tea.Batch(
				func() tea.Msg { return panels.PRDeselectedMsg{} },
				p.activeTabSelectionCmd(),
			)
		case tabActions:
			p.activeTab = defaultTab
			return p, tea.Batch(
				func() tea.Msg { return panels.ActionRunDeselectedMsg{} },
				p.activeTabSelectionCmd(),
			)
		case tabWorkflows, tabReleases:
			p.activeTab = defaultTab
			return p, p.activeTabSelectionCmd()
		case tabBranches, tabTags:
			if p.mode == ModeGitHub {
				p.activeTab = defaultTab
				return p, p.activeTabSelectionCmd()
			}
		}
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Mouse handling
// ---------------------------------------------------------------------------
// handleMouseClick processes a single click in the gitinfo panel.
// Row 0 is the git tab bar; row 1 is the GitHub tab bar (if available);
// remaining rows are content items.
func (p *Panel) handleMouseClick(msg panels.PanelMouseClickMsg) (panels.Panel, tea.Cmd) {
	tbh := p.tabBarHeight()
	if msg.ContentRow < tbh {
		switch p.mode {
		case ModeGit:
			p.handleTabBarClick(msg.ContentCol)
		case ModeGitHub:
			p.handleGitHubTabBarClick(msg.ContentCol)
		default: // ModeAll
			if msg.ContentRow == 0 {
				p.handleTabBarClick(msg.ContentCol)
			} else {
				p.handleGitHubTabBarClick(msg.ContentCol)
			}
		}
		return p, p.activeTabSelectionCmd()
	}
	// Content area click — select the item at the clicked row.
	items := p.tabItems[p.activeTab]
	offset := p.tabOffset[p.activeTab]
	idx := offset + (msg.ContentRow - tbh)
	if idx < 0 || idx >= len(items) {
		return p, nil
	}
	p.tabCursor[p.activeTab] = idx
	p.ensureCursorVisible()
	return p, p.activeTabSelectionCmd()
}

// handleMouseDoubleClick processes a double-click in the gitinfo panel.
// Performs the context action for the item under the cursor.
// Tab bar double-clicks are ignored — tab switching is handled by single
// click, and header double-clicks (opening repo) are handled via
// PanelHeaderDoubleClickMsg.
func (p *Panel) handleMouseDoubleClick(msg panels.PanelMouseDoubleClickMsg) (panels.Panel, tea.Cmd) {
	tbh := p.tabBarHeight()
	if msg.ContentRow < tbh {
		// Tab bar double-click — no action; tab switching is handled by
		// single click, repo-open by header double-click.
		return p, nil
	}
	// Content area double-click — move cursor then execute action.
	items := p.tabItems[p.activeTab]
	offset := p.tabOffset[p.activeTab]
	idx := offset + (msg.ContentRow - tbh)
	if idx < 0 || idx >= len(items) {
		return p, nil
	}
	p.tabCursor[p.activeTab] = idx
	p.ensureCursorVisible()
	return p.doAction()
}

// handleMouseRightClick shows a context menu for the item at the clicked row.
func (p *Panel) handleMouseRightClick(msg panels.PanelMouseRightClickMsg) (panels.Panel, tea.Cmd) {
	tbh := p.tabBarHeight()
	if msg.ContentRow < tbh {
		return p, nil
	}
	items := p.tabItems[p.activeTab]
	offset := p.tabOffset[p.activeTab]
	idx := offset + (msg.ContentRow - tbh)
	if idx < 0 || idx >= len(items) {
		return p, nil
	}
	p.tabCursor[p.activeTab] = idx
	p.ensureCursorVisible()
	item := items[idx]
	itemType := p.itemTypeForKind(item.kind)
	if itemType == "" {
		return p, nil
	}
	label := p.rightClickLabel(item)
	cmd, directAction := rightclick.Cmd(p.actionsCfg, itemType, label)
	if cmd != nil {
		p.pending = opRightClickPick
		return p, cmd
	}
	if directAction != "" {
		return p.executeRightClickAction(directAction)
	}
	return p, nil
}

// rightClickLabel returns a human-readable label for the right-click menu.
func (p *Panel) rightClickLabel(item listItem) string {
	switch item.kind {
	case kindLocalBranch, kindRemoteBranch:
		return item.branch.Name
	case kindWorktree:
		return item.worktree.Branch
	case kindRemote:
		return item.remote.Name
	case kindStashEntry:
		return fmt.Sprintf("stash@{%d}", item.stash.Index)
	case kindIssue:
		return fmt.Sprintf("#%d %s", item.issue.Number, item.issue.Title)
	case kindPR:
		return fmt.Sprintf("#%d %s", item.pr.Number, item.pr.Title)
	case kindActionRun:
		return fmt.Sprintf("#%d %s", item.actionRun.RunNumber, item.actionRun.WorkflowName)
	case kindWorkflow:
		return item.workflow.Name
	case kindRelease:
		return item.release.TagName + " " + item.release.Name
	case kindTag, kindRemoteTag:
		return item.tag.Name
	default:
		return item.text
	}
}

// handleMouseWheel scrolls the active tab's viewport.
func (p *Panel) handleMouseWheel(msg tea.MouseWheelMsg) (panels.Panel, tea.Cmd) {
	m := msg.Mouse()
	tab := p.activeTab
	tbh := p.tabBarHeight()
	switch m.Button {
	case tea.MouseWheelUp:
		p.tabOffset[tab] -= panels.ScrollDelta
		if p.tabOffset[tab] < 0 {
			p.tabOffset[tab] = 0
		}
	case tea.MouseWheelDown:
		items := p.tabItems[tab]
		maxOffset := len(items) - (p.Height - tbh)
		if maxOffset < 0 {
			maxOffset = 0
		}
		p.tabOffset[tab] += panels.ScrollDelta
		if p.tabOffset[tab] > maxOffset {
			p.tabOffset[tab] = maxOffset
		}
	}
	return p, nil
}

// handleTabBarClick switches the active tab based on the column position
// of the click. Tab labels are laid out as: " Branches N · Worktrees N · Remotes N · Stash N".
func (p *Panel) handleTabBarClick(col int) {
	// Tab definitions matching the git tab row in renderTabBar.
	type tabEntry struct {
		name, short string
		count       string
		id          tabID
	}
	tabs := []tabEntry{
		{id: tabBranches, name: "Branches", short: "Br", count: fmt.Sprintf("%d", len(p.tabItems[tabBranches]))},
		{id: tabWorktrees, name: "Worktrees", short: "Wt", count: fmt.Sprintf("%d", len(p.tabItems[tabWorktrees]))},
		{id: tabRemotes, name: "Remotes", short: "Rm", count: fmt.Sprintf("%d", p.remoteCount)},
		{id: tabStash, name: "Stash", short: "St", count: fmt.Sprintf("%d", len(p.tabItems[tabStash]))},
		{id: tabTags, name: "Tags", short: "Tg", count: fmt.Sprintf("%d", len(p.tabItems[tabTags]))},
		{id: tabReflog, name: "Reflog", short: "Rl", count: fmt.Sprintf("%d", len(p.tabItems[tabReflog]))},
	}
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

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------
func (p *Panel) moveCursorDown() {
	items := p.tabItems[p.activeTab]
	if p.tabCursor[p.activeTab] < len(items)-1 {
		p.tabCursor[p.activeTab]++
		p.ensureCursorVisible()
	}
}

func (p *Panel) moveCursorUp() {
	if p.tabCursor[p.activeTab] > 0 {
		p.tabCursor[p.activeTab]--
		p.ensureCursorVisible()
	}
}

func (p *Panel) moveToFirst() {
	p.tabCursor[p.activeTab] = 0
	p.ensureCursorVisible()
}

func (p *Panel) moveToLast() {
	items := p.tabItems[p.activeTab]
	if len(items) > 0 {
		p.tabCursor[p.activeTab] = len(items) - 1
	}
	p.ensureCursorVisible()
}

// pageDown moves the cursor down by one page (viewport height minus tab bar).
func (p *Panel) pageDown() {
	tbh := p.tabBarHeight()
	viewH := p.Height - tbh
	if viewH <= 0 {
		return
	}
	items := p.tabItems[p.activeTab]
	p.tabCursor[p.activeTab] += viewH
	if p.tabCursor[p.activeTab] >= len(items) {
		p.tabCursor[p.activeTab] = len(items) - 1
	}
	if p.tabCursor[p.activeTab] < 0 {
		p.tabCursor[p.activeTab] = 0
	}
	p.ensureCursorVisible()
}

// pageUp moves the cursor up by one page (viewport height minus tab bar).
func (p *Panel) pageUp() {
	tbh := p.tabBarHeight()
	viewH := p.Height - tbh
	if viewH <= 0 {
		return
	}
	p.tabCursor[p.activeTab] -= viewH
	if p.tabCursor[p.activeTab] < 0 {
		p.tabCursor[p.activeTab] = 0
	}
	p.ensureCursorVisible()
}

func (p *Panel) ensureCursorVisible() {
	if p.Height <= 0 {
		return
	}
	tab := p.activeTab
	// Account for tab bar height.
	tbh := p.tabBarHeight()
	viewH := p.Height - tbh
	if viewH <= 0 {
		return
	}
	if p.tabCursor[tab] < p.tabOffset[tab] {
		p.tabOffset[tab] = p.tabCursor[tab]
	}
	if p.tabCursor[tab] >= p.tabOffset[tab]+viewH {
		p.tabOffset[tab] = p.tabCursor[tab] - viewH + 1
	}
}

// selectedBranch returns the branch at the cursor, or nil.
func (p *Panel) selectedBranch() *git.Branch {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return nil
	}
	item := items[cursor]
	if item.kind != kindLocalBranch && item.kind != kindRemoteBranch {
		return nil
	}
	b := item.branch
	return &b
}

// branchSelectedCmd returns a Cmd that emits BranchSelectedMsg for the
// branch under the cursor. Returns nil if the cursor is not on a branch.
func (p *Panel) branchSelectedCmd() tea.Cmd {
	if p.activeTab != tabBranches {
		return nil
	}
	b := p.selectedBranch()
	if b == nil {
		return nil
	}
	name := b.Name
	return func() tea.Msg {
		return panels.BranchSelectedMsg{Name: name}
	}
}

// worktreeSelectedCmd returns a Cmd that emits WorktreeSelectedMsg for the
// worktree under the cursor. Returns nil if not on worktrees tab or no item.
func (p *Panel) worktreeSelectedCmd() tea.Cmd {
	if p.activeTab != tabWorktrees {
		return nil
	}
	wt := p.selectedWorktree()
	if wt == nil {
		return nil
	}
	path := wt.Path
	branch := wt.Branch
	return func() tea.Msg {
		return panels.WorktreeSelectedMsg{Path: path, Branch: branch}
	}
}

// remoteSelectedCmd returns a Cmd that emits RemoteSelectedMsg for the
// remote under the cursor. Returns nil if not on remotes tab or no item.
func (p *Panel) remoteSelectedCmd() tea.Cmd {
	if p.activeTab != tabRemotes {
		return nil
	}
	r := p.selectedRemote()
	if r == nil {
		return nil
	}
	name := r.Name
	return func() tea.Msg {
		return panels.RemoteSelectedMsg{Name: name}
	}
}

// stashSelectedCmd returns a Cmd that emits StashSelectedMsg for the
// stash entry under the cursor. Returns nil if not on stash tab or no item.
func (p *Panel) stashSelectedCmd() tea.Cmd {
	if p.activeTab != tabStash {
		return nil
	}
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return nil
	}
	if items[cursor].kind != kindStashEntry {
		return nil
	}
	s := items[cursor].stash
	idx := s.Index
	hash := s.Hash
	return func() tea.Msg {
		return panels.StashSelectedMsg{Index: idx, Hash: hash}
	}
}

// activeTabSelectionCmd returns the selection Cmd for the currently active tab.
func (p *Panel) activeTabSelectionCmd() tea.Cmd {
	switch p.activeTab { //nolint:exhaustive // only relevant cases handled
	case tabBranches:
		return p.branchSelectedCmd()
	case tabWorktrees:
		return p.worktreeSelectedCmd()
	case tabRemotes:
		return p.remoteSelectedCmd()
	case tabStash:
		return p.stashSelectedCmd()
	case tabTags:
		return nil // Tags tab has no persistent selection message
	case tabIssues:
		return p.issueSelectedCmd()
	case tabPRs:
		return p.prSelectedCmd()
	case tabActions:
		return p.actionRunSelectedCmd()
	case tabWorkflows:
		return p.workflowSelectedCmd()
	}
	return nil
}

// selectedWorktree returns the worktree at the cursor, or nil.
func (p *Panel) selectedWorktree() *git.Worktree {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return nil
	}
	if items[cursor].kind != kindWorktree {
		return nil
	}
	wt := items[cursor].worktree
	return &wt
}

// selectedRemote returns the remote at the cursor, or nil.
func (p *Panel) selectedRemote() *git.Remote {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return nil
	}
	if items[cursor].kind != kindRemote {
		return nil
	}
	r := items[cursor].remote
	return &r
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------
func (p *Panel) doAction() (panels.Panel, tea.Cmd) {
	// Guard: don't start a new action while a pending operation is active.
	// This prevents Enter key-repeat after modal dismissal from triggering
	// duplicate actions (e.g. dispatching a workflow twice).
	if p.pending != opNone {
		return p, nil
	}
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return p, nil
	}
	item := items[cursor]
	// Determine the item type for the action registry.
	itemType := p.itemTypeForKind(item.kind)
	if itemType == "" {
		// No registry entry (e.g., kindRemoteSub) -- skip.
		return p, nil
	}
	// Check if user has confirmed this action type.
	if !p.actionsCfg.IsConfirmed(string(itemType)) {
		p.pending = opFirstUseConfirm
		p.pendingName = string(itemType)
		return p, rightclick.FirstUseCmd(itemType)
	}
	// Already confirmed -- execute the configured action.
	action := actions.ActionID(p.actionsCfg.GetDoubleClickAction(string(itemType)))
	return p.executeRightClickAction(action)
}

// itemTypeForKind maps internal itemKind values to the action registry
// ItemType constants.
func (p *Panel) itemTypeForKind(kind itemKind) actions.ItemType {
	switch kind {
	case kindLocalBranch:
		return actions.ItemLocalBranch
	case kindRemoteBranch:
		return actions.ItemRemoteBranch
	case kindWorktree:
		return actions.ItemWorktree
	case kindRemote:
		return actions.ItemRemote
	case kindStashEntry:
		return actions.ItemStashEntry
	case kindIssue:
		return actions.ItemIssue
	case kindPR:
		return actions.ItemPR
	case kindActionRun:
		return actions.ItemActionRun
	case kindWorkflow:
		return actions.ItemWorkflow
	case kindRelease:
		return actions.ItemRelease
	case kindTag, kindRemoteTag:
		return actions.ItemTag
	default:
		return ""
	}
}

// executeAction performs the actual action for the given item. This is called
// after any first-use confirmation has been accepted (or skipped because the
// user chose "Always").
func (p *Panel) executeAction(item listItem) (panels.Panel, tea.Cmd) {
	switch item.kind { //nolint:exhaustive // only relevant cases handled
	case kindLocalBranch, kindRemoteBranch:
		return p.requestCheckout()
	case kindWorktree:
		return p.requestWorktreeSwitch()
	case kindRemote:
		url := remoteToHTTPS(item.remote.FetchURL)
		if url == "" {
			return p, func() tea.Msg {
				return notify.ShowToastMsg{Message: "No URL for remote " + item.remote.Name, Level: notify.Warn}
			}
		}
		return p, func() tea.Msg {
			if err := panels.OpenInBrowser(url); err != nil {
				return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
			}
			return notify.ShowToastMsg{Message: "Opened " + item.remote.Name, Level: notify.Info}
		}
	case kindStashEntry:
		s := item.stash
		p.pending = opStashAction
		p.pendingName = fmt.Sprintf("%d", s.Index)
		return p, notify.ShowInputWithValue("Stash Action",
			"apply, pop, or drop", "apply")
	case kindIssue:
		url := item.issue.HTMLURL
		if url == "" {
			return p, nil
		}
		return p, func() tea.Msg {
			if err := panels.OpenInBrowser(url); err != nil {
				return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
			}
			return notify.ShowToastMsg{Message: fmt.Sprintf("Opened issue #%d", item.issue.Number), Level: notify.Info}
		}
	case kindPR:
		url := item.pr.HTMLURL
		if url == "" {
			return p, nil
		}
		return p, func() tea.Msg {
			if err := panels.OpenInBrowser(url); err != nil {
				return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
			}
			return notify.ShowToastMsg{Message: fmt.Sprintf("Opened PR #%d", item.pr.Number), Level: notify.Info}
		}
	case kindActionRun:
		url := item.actionRun.HTMLURL
		if url == "" {
			return p, nil
		}
		return p, func() tea.Msg {
			if err := panels.OpenInBrowser(url); err != nil {
				return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
			}
			return notify.ShowToastMsg{Message: fmt.Sprintf("Opened run #%d", item.actionRun.RunNumber), Level: notify.Info}
		}
	case kindWorkflow:
		return p.doWorkflowDispatch()
	case kindRelease:
		url := item.release.HTMLURL
		if url == "" {
			return p, nil
		}
		return p, func() tea.Msg {
			if err := panels.OpenInBrowser(url); err != nil {
				return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
			}
			return notify.ShowToastMsg{Message: "Opened release " + item.release.TagName, Level: notify.Info}
		}
	case kindTag, kindRemoteTag:
		tg := item.tag
		p.pending = opTagCheckout
		p.pendingName = tg.Name
		return p, notify.ShowConfirm("Checkout Tag",
			fmt.Sprintf("Checkout tag %q? This will detach HEAD.", tg.Name))
	}
	return p, nil
}

// executeRightClickAction dispatches a right-click action based on the
// current item kind and the selected action ID.
func (p *Panel) executeRightClickAction(action actions.ActionID) (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return p, nil
	}
	item := items[cursor]
	switch item.kind { //nolint:exhaustive // only relevant cases handled
	case kindLocalBranch:
		switch action { //nolint:exhaustive // only relevant cases handled
		case actions.ActionCheckout:
			return p.requestCheckout()
		case actions.ActionCopyName:
			return p.copyAndToast(item.branch.Name)
		case actions.ActionOpenInBrowser:
			url := remoteToHTTPS(p.guessBranchRemoteURL(item.branch))
			if url == "" {
				return p, nil
			}
			return p.openURLAndToast(url, "branch")
		}
	case kindRemoteBranch:
		switch action { //nolint:exhaustive // only relevant cases handled
		case actions.ActionCheckout:
			return p.requestCheckout()
		case actions.ActionCopyName:
			return p.copyAndToast(item.branch.Name)
		}
	case kindWorktree:
		switch action { //nolint:exhaustive // only relevant cases handled
		case actions.ActionSwitch:
			return p.requestWorktreeSwitch()
		case actions.ActionOpenTerminal:
			path := item.worktree.Path
			return p, func() tea.Msg {
				if err := panels.OpenInTerminal(path); err != nil {
					return notify.ShowToastMsg{Message: "Terminal error: " + err.Error(), Level: notify.Error}
				}
				return notify.ShowToastMsg{Message: "Opened terminal at " + path, Level: notify.Success}
			}
		case actions.ActionCopyPath:
			return p.copyAndToast(item.worktree.Path)
		}
	case kindRemote:
		switch action { //nolint:exhaustive // only relevant cases handled
		case actions.ActionOpenInBrowser:
			return p.executeAction(item)
		case actions.ActionCopyURL:
			url := item.remote.FetchURL
			if url == "" {
				return p, nil
			}
			return p.copyAndToast(url)
		}
	case kindStashEntry:
		s := item.stash
		switch action { //nolint:exhaustive // only relevant cases handled
		case actions.ActionPromptAction:
			p.pending = opStashAction
			p.pendingName = fmt.Sprintf("%d", s.Index)
			return p, notify.ShowInputWithValue("Stash Action", "apply, pop, or drop", "apply")
		case actions.ActionApply:
			idx := s.Index
			return p, func() tea.Msg {
				err := p.git.StashApply(p.ctx, idx)
				return opResultMsg{op: "stash_applied", name: fmt.Sprintf("stash@{%d}", idx), err: err}
			}
		case actions.ActionPop:
			idx := s.Index
			return p, func() tea.Msg {
				err := p.git.StashPop(p.ctx, idx)
				return opResultMsg{op: "stash_popped", name: fmt.Sprintf("stash@{%d}", idx), err: err}
			}
		case actions.ActionDrop:
			idx := s.Index
			return p, func() tea.Msg {
				err := p.git.StashDrop(p.ctx, idx)
				return opResultMsg{op: "stash_dropped", name: fmt.Sprintf("stash@{%d}", idx), err: err}
			}
		}
	case kindIssue:
		switch action { //nolint:exhaustive // only relevant cases handled
		case actions.ActionOpenInBrowser:
			if item.issue.HTMLURL == "" {
				return p, nil
			}
			return p.openURLAndToast(item.issue.HTMLURL, fmt.Sprintf("issue #%d", item.issue.Number))
		case actions.ActionCopyURL:
			return p.copyAndToast(item.issue.HTMLURL)
		case actions.ActionCopyNumber:
			return p.copyAndToast(fmt.Sprintf("%d", item.issue.Number))
		}
	case kindPR:
		switch action { //nolint:exhaustive // only relevant cases handled
		case actions.ActionOpenInBrowser:
			if item.pr.HTMLURL == "" {
				return p, nil
			}
			return p.openURLAndToast(item.pr.HTMLURL, fmt.Sprintf("PR #%d", item.pr.Number))
		case actions.ActionCopyURL:
			return p.copyAndToast(item.pr.HTMLURL)
		case actions.ActionCopyNumber:
			return p.copyAndToast(fmt.Sprintf("%d", item.pr.Number))
		case actions.ActionCheckoutBranch:
			if item.pr.HeadBranch == "" {
				return p, nil
			}
			ref := item.pr.HeadBranch
			p.pending = opBranchCheckout
			p.pendingName = ref
			return p, notify.ShowConfirm("Checkout PR Branch", fmt.Sprintf("Switch to branch %q?", ref))
		case actions.ActionMergePR:
			return p.doMergePR()
		}
	case kindActionRun:
		switch action { //nolint:exhaustive // only relevant cases handled
		case actions.ActionOpenInBrowser:
			if item.actionRun.HTMLURL == "" {
				return p, nil
			}
			return p.openURLAndToast(item.actionRun.HTMLURL, fmt.Sprintf("run #%d", item.actionRun.RunNumber))
		case actions.ActionRerun:
			return p.doActionsRerun()
		case actions.ActionCopyURL:
			return p.copyAndToast(item.actionRun.HTMLURL)
		}
	case kindWorkflow:
		switch action { //nolint:exhaustive // only relevant cases handled
		case actions.ActionDispatch:
			return p.doWorkflowDispatch()
		case actions.ActionOpenInBrowser:
			if item.workflow.HTMLURL == "" {
				return p, nil
			}
			return p.openURLAndToast(item.workflow.HTMLURL, item.workflow.Name)
		case actions.ActionCopyURL:
			return p.copyAndToast(item.workflow.HTMLURL)
		}
	case kindRelease:
		switch action { //nolint:exhaustive // only relevant cases handled
		case actions.ActionOpenInBrowser:
			if item.release.HTMLURL == "" {
				return p, nil
			}
			return p.openURLAndToast(item.release.HTMLURL, "release "+item.release.TagName)
		case actions.ActionDownloadAssets:
			if item.release.HTMLURL == "" {
				return p, nil
			}
			return p.openURLAndToast(item.release.HTMLURL, "release assets")
		case actions.ActionCopyURL:
			return p.copyAndToast(item.release.HTMLURL)
		case actions.ActionCopyName:
			return p.copyAndToast(item.release.TagName)
		}
	case kindTag, kindRemoteTag:
		switch action { //nolint:exhaustive // only relevant cases handled
		case actions.ActionCheckout:
			tg := item.tag
			p.pending = opTagCheckout
			p.pendingName = tg.Name
			return p, notify.ShowConfirm("Checkout Tag",
				fmt.Sprintf("Checkout tag %q? This will detach HEAD.", tg.Name))
		case actions.ActionPush:
			return p.doTagPush()
		case actions.ActionDelete:
			return p.doTagDelete()
		case actions.ActionCopyName:
			return p.copyAndToast(item.tag.Name)
		case actions.ActionCopyHash:
			return p.copyAndToast(item.tag.Hash)
		}
	}
	return p, nil
}

// copyAndToast copies text to the clipboard and shows a toast notification.
func (p *Panel) copyAndToast(text string) (panels.Panel, tea.Cmd) {
	if text == "" {
		return p, nil
	}
	if err := panels.CopyToClipboard(p.ctx, text); err != nil {
		errMsg := err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Copy failed: " + errMsg, Level: notify.Error}
		}
	}
	copied := text
	if len(copied) > 40 {
		copied = copied[:37] + "..."
	}
	return p, func() tea.Msg {
		return notify.ShowToastMsg{Message: "Copied: " + copied, Level: notify.Success}
	}
}

// openRepoInBrowser opens the repository's GitHub page in the default browser.
// Returns nil cmd when owner/repo are unavailable (e.g. pure git mode).
func (p *Panel) openRepoInBrowser() (panels.Panel, tea.Cmd) {
	if p.ghOwner == "" || p.ghRepo == "" {
		return p, nil
	}
	url := fmt.Sprintf("https://github.com/%s/%s", p.ghOwner, p.ghRepo)
	return p.openURLAndToast(url, p.ghOwner+"/"+p.ghRepo)
}

// openURLAndToast opens a URL in the browser and shows a toast notification.
func (p *Panel) openURLAndToast(url, label string) (panels.Panel, tea.Cmd) {
	return p, func() tea.Msg {
		if err := panels.OpenInBrowser(url); err != nil {
			return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Opened " + label, Level: notify.Info}
	}
}

// guessBranchRemoteURL returns the fetch URL of the first remote, used to
// construct a browser-openable URL for local branches.
func (p *Panel) guessBranchRemoteURL(_ git.Branch) string {
	if len(p.lastRemotes) > 0 {
		return p.lastRemotes[0].FetchURL
	}
	return ""
}

func (p *Panel) requestCheckout() (panels.Panel, tea.Cmd) {
	b := p.selectedBranch()
	if b == nil {
		return p, nil
	}
	if b.IsCurrent {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Already on " + b.Name, Level: notify.Info}
		}
	}
	ref := b.Name
	if b.IsRemote {
		if idx := strings.IndexByte(ref, '/'); idx >= 0 {
			ref = ref[idx+1:]
		}
	}
	p.pending = opBranchCheckout
	p.pendingName = ref
	return p, notify.ShowConfirm("Switch Branch", fmt.Sprintf("Switch to branch %q?", ref))
}

func (p *Panel) doReflogCheckout() (panels.Panel, tea.Cmd) {
	items := p.tabItems[tabReflog]
	cursor := p.tabCursor[tabReflog]
	if cursor < 0 || cursor >= len(items) {
		return p, nil
	}
	item := items[cursor]
	hash := item.reflog.Hash
	if len(hash) > 10 {
		hash = hash[:10]
	}
	p.pending = opBranchCheckout
	p.pendingName = item.reflog.Hash
	return p, notify.ShowConfirm("Checkout Reflog Entry", fmt.Sprintf("Checkout %s (%s)?", hash, item.reflog.Message))
}

func (p *Panel) requestWorktreeSwitch() (panels.Panel, tea.Cmd) {
	wt := p.selectedWorktree()
	if wt == nil {
		return p, nil
	}
	path := wt.Path
	if p.cfg.WorktreeOpenMode == "new_terminal" {
		return p, func() tea.Msg {
			if err := panels.OpenInTerminal(path); err != nil {
				errMsg := err.Error()
				return notify.ShowToastMsg{Message: "Terminal error: " + errMsg, Level: notify.Error}
			}
			return notify.ShowToastMsg{Message: "Opened terminal at " + path, Level: notify.Success}
		}
	}
	return p, func() tea.Msg {
		return opResultMsg{op: "worktree_switch", name: path}
	}
}

func (p *Panel) doCreate() (panels.Panel, tea.Cmd) {
	switch p.activeTab { //nolint:exhaustive // only relevant cases handled
	case tabBranches:
		p.pending = opBranchCreate
		return p, notify.ShowInput("New Branch", "branch-name")
	case tabWorktrees:
		p.pending = opWorktreeCreate
		return p, notify.ShowInput("New Worktree Branch", "branch-name")
	case tabRemotes:
		p.pending = opRemoteAdd
		return p, notify.ShowInput("Remote Name", "remote-name")
	case tabTags:
		p.pending = opTagCreate
		return p, notify.ShowInput("Tag Name", "tag-name")
	case tabIssues:
		if p.ghOwner != "" && p.ghRepo != "" {
			url := fmt.Sprintf("https://github.com/%s/%s/issues/new", p.ghOwner, p.ghRepo)
			return p, func() tea.Msg {
				if err := panels.OpenInBrowser(url); err != nil {
					return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
				}
				return notify.ShowToastMsg{Message: "Opened new issue page", Level: notify.Info}
			}
		}
	}
	return p, nil
}

func (p *Panel) doDelete() (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return p, nil
	}
	item := items[cursor]
	switch item.kind { //nolint:exhaustive // only relevant cases handled
	case kindLocalBranch:
		b := item.branch
		if b.IsCurrent {
			return p, func() tea.Msg {
				return notify.ShowToastMsg{Message: "Cannot delete current branch", Level: notify.Warn}
			}
		}
		p.pending = opBranchDelete
		p.pendingName = b.Name
		return p, notify.ShowConfirm("Delete Branch", fmt.Sprintf("Delete branch %q?", b.Name))
	case kindRemoteBranch:
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot delete remote branch locally", Level: notify.Warn}
		}
	case kindWorktree:
		wt := item.worktree
		p.pending = opWorktreeDelete
		p.pendingName = wt.Path
		return p, notify.ShowConfirm("Remove Worktree", fmt.Sprintf("Remove worktree at %q?", wt.Path))
	case kindRemote:
		r := item.remote
		p.pending = opRemoteDelete
		p.pendingName = r.Name
		return p, notify.ShowConfirm("Remove Remote", fmt.Sprintf("Remove remote %q?", r.Name))
	case kindTag:
		tg := item.tag
		p.pending = opTagDelete
		p.pendingName = tg.Name
		return p, notify.ShowConfirm("Delete Tag", fmt.Sprintf("Delete tag %q?", tg.Name))
	case kindRemoteTag:
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot delete remote-only tag locally", Level: notify.Warn}
		}
	}
	return p, nil
}

func (p *Panel) doRename() (panels.Panel, tea.Cmd) {
	b := p.selectedBranch()
	if b == nil {
		return p, nil
	}
	if b.IsRemote {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot rename remote branch", Level: notify.Warn}
		}
	}
	p.pending = opBranchRename
	p.pendingName = b.Name
	return p, notify.ShowInput("Rename Branch", b.Name)
}

// doOpenInBrowser opens the selected item in the default browser.
// The behaviour varies by active tab and item kind.
func (p *Panel) doOpenInBrowser() (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return p, nil
	}
	item := items[cursor]
	var url, label string
	switch item.kind { //nolint:exhaustive // only relevant cases handled
	case kindLocalBranch:
		raw := remoteToHTTPS(p.guessBranchRemoteURL(item.branch))
		if raw != "" {
			url = raw + "/tree/" + item.branch.Name
			label = "branch " + item.branch.Name
		}
	case kindRemoteBranch:
		raw := remoteToHTTPS(p.guessBranchRemoteURL(item.branch))
		if raw != "" {
			name := item.branch.Name
			if idx := strings.IndexByte(name, '/'); idx >= 0 {
				name = name[idx+1:]
			}
			url = raw + "/tree/" + name
			label = "branch " + name
		}
	case kindRemote:
		url = remoteToHTTPS(item.remote.FetchURL)
		label = "remote " + item.remote.Name
	case kindIssue:
		url = item.issue.HTMLURL
		label = fmt.Sprintf("issue #%d", item.issue.Number)
	case kindPR:
		url = item.pr.HTMLURL
		label = fmt.Sprintf("PR #%d", item.pr.Number)
	case kindActionRun:
		url = item.actionRun.HTMLURL
		label = fmt.Sprintf("run #%d", item.actionRun.RunNumber)
	case kindWorkflow:
		url = item.workflow.HTMLURL
		label = item.workflow.Name
	case kindRelease:
		url = item.release.HTMLURL
		label = "release " + item.release.TagName
	case kindTag, kindRemoteTag:
		raw := remoteToHTTPS(p.guessBranchRemoteURL(git.Branch{}))
		if raw != "" {
			url = raw + "/releases/tag/" + item.tag.Name
			label = "tag " + item.tag.Name
		}
	}
	if url == "" {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "No URL available", Level: notify.Warn}
		}
	}
	return p, func() tea.Msg {
		if err := panels.OpenInBrowser(url); err != nil {
			return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Opened " + label, Level: notify.Info}
	}
}

func (p *Panel) doFetch() (panels.Panel, tea.Cmd) {
	r := p.selectedRemote()
	g := p.git
	ctx := p.ctx
	if r != nil {
		name := r.Name
		return p, tea.Batch(
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Fetching " + name + "...", Level: notify.Info}
			},
			func() tea.Msg {
				err := g.Fetch(ctx, git.FetchOpts{Remote: name, Prune: true})
				return opResultMsg{op: "fetched", name: name, err: err}
			},
		)
	}
	// Fetch all if not on a remote item.
	return p, tea.Batch(
		func() tea.Msg {
			return notify.ShowToastMsg{Message: "Fetching all...", Level: notify.Info}
		},
		func() tea.Msg {
			err := g.Fetch(ctx, git.FetchOpts{All: true, Prune: true})
			return opResultMsg{op: "fetched", name: "all remotes", err: err}
		},
	)
}

// ---------------------------------------------------------------------------
// Modal result handling
// ---------------------------------------------------------------------------
func (p *Panel) handleModalResult(msg notify.ModalResultMsg) (panels.Panel, tea.Cmd) {
	op := p.pending
	name := p.pendingName
	p.pending = opNone
	p.pendingName = ""
	if !msg.Accept {
		return p, nil
	}
	g := p.git
	ctx := p.ctx
	switch op { //nolint:exhaustive // only relevant cases handled
	case opBranchCreate:
		newName := strings.TrimSpace(msg.Value)
		if newName == "" {
			return p, nil
		}
		return p, func() tea.Msg {
			err := g.BranchCreate(ctx, newName, "")
			return opResultMsg{op: "branch_created", name: newName, err: err}
		}
	case opBranchDelete:
		return p, func() tea.Msg {
			err := g.BranchDelete(ctx, name, false)
			return opResultMsg{op: "branch_deleted", name: name, err: err}
		}
	case opBranchRename:
		newName := strings.TrimSpace(msg.Value)
		if newName == "" || newName == name {
			return p, nil
		}
		return p, func() tea.Msg {
			err := g.BranchRename(ctx, name, newName)
			return opResultMsg{op: "branch_renamed", name: newName, err: err}
		}
	case opWorktreeCreate:
		branch := strings.TrimSpace(msg.Value)
		if branch == "" {
			return p, nil
		}
		path := worktreePath(p.repoRoot, branch)
		return p, func() tea.Msg {
			err := g.WorktreeAdd(ctx, path, branch)
			return opResultMsg{op: "worktree_added", name: branch, err: err}
		}
	case opWorktreeDelete:
		return p, func() tea.Msg {
			err := g.WorktreeRemove(ctx, name, false)
			return opResultMsg{op: "worktree_removed", name: name, err: err}
		}
	case opRemoteAdd:
		remoteName := strings.TrimSpace(msg.Value)
		if remoteName == "" {
			return p, nil
		}
		// Two-step: first get name, then URL.
		p.pending = opRemoteAddURL
		p.pendingName = remoteName
		return p, notify.ShowInput("Remote URL", "https://github.com/user/repo")
	case opRemoteAddURL:
		url := strings.TrimSpace(msg.Value)
		if url == "" {
			return p, nil
		}
		remoteName := name
		return p, func() tea.Msg {
			err := g.RemoteAdd(ctx, remoteName, url)
			return opResultMsg{op: "remote_added", name: remoteName, err: err}
		}
	case opRemoteDelete:
		return p, func() tea.Msg {
			err := g.RemoteRemove(ctx, name)
			return opResultMsg{op: "remote_removed", name: name, err: err}
		}
	case opBranchCheckout:
		return p, func() tea.Msg {
			err := g.Checkout(ctx, name)
			return opResultMsg{op: "checkout", name: name, err: err}
		}
	case opStashAction:
		action := strings.TrimSpace(strings.ToLower(msg.Value))
		idx, err := strconv.Atoi(name)
		if err != nil {
			return p, nil
		}
		switch action {
		case "apply", "a":
			return p, func() tea.Msg {
				err := g.StashApply(ctx, idx)
				return opResultMsg{op: "stash_applied", name: fmt.Sprintf("stash@{%d}", idx), err: err}
			}
		case "pop", "p":
			return p, func() tea.Msg {
				err := g.StashPop(ctx, idx)
				return opResultMsg{op: "stash_popped", name: fmt.Sprintf("stash@{%d}", idx), err: err}
			}
		case "drop", "d":
			return p, func() tea.Msg {
				err := g.StashDrop(ctx, idx)
				return opResultMsg{op: "stash_dropped", name: fmt.Sprintf("stash@{%d}", idx), err: err}
			}
		default:
			return p, func() tea.Msg {
				return notify.ShowToastMsg{Message: "Unknown stash action: " + action, Level: notify.Warn}
			}
		}
	case opFirstUseConfirm:
		if msg.Remember {
			config.SaveDoubleClickChoice(&p.actionsCfg, name, msg.Value)
		}
		return p.executeRightClickAction(actions.ActionID(msg.Value))
	case opRightClickPick:
		return p.executeRightClickAction(actions.ActionID(msg.Value))
	case opTagCreate:
		tagName := strings.TrimSpace(msg.Value)
		if tagName == "" {
			return p, nil
		}
		p.pending = opTagMessage
		p.pendingName = tagName
		return p, notify.ShowInput("Tag Message", "(leave empty for lightweight)")
	case opTagMessage:
		tagName := name
		message := strings.TrimSpace(msg.Value)
		return p, func() tea.Msg {
			err := g.TagCreate(ctx, tagName, "", message)
			return opResultMsg{op: "tag_created", name: tagName, err: err}
		}
	case opTagDelete:
		return p, func() tea.Msg {
			err := g.TagDelete(ctx, name)
			return opResultMsg{op: "tag_deleted", name: name, err: err}
		}
	case opTagPush:
		tagName := name
		return p, func() tea.Msg {
			err := g.TagPush(ctx, "origin", tagName)
			return opResultMsg{op: "tag_pushed", name: tagName, err: err}
		}
	case opTagCheckout:
		return p, func() tea.Msg {
			err := g.Checkout(ctx, name)
			return opResultMsg{op: "tag_checkout", name: name, err: err}
		}
	case opWorkflowDispatch:
		// Step 1 complete: got the ref. Fetch workflow inputs before
		// showing the inputs dialog so we can pre-populate fields.
		ref := strings.TrimSpace(msg.Value)
		if ref == "" {
			ref = p.currentBranch()
		}
		// Parse workflow ID and name from pendingName ("id:name").
		var workflowID int64
		var workflowName string
		if parts := strings.SplitN(name, ":", 2); len(parts) == 2 {
			workflowID, _ = strconv.ParseInt(parts[0], 10, 64)
			workflowName = parts[1]
		}
		if workflowID == 0 {
			return p, nil
		}
		// Look up the workflow path from the cached items.
		var workflowPath string
		for _, item := range p.tabItems[tabWorkflows] {
			if item.kind == kindWorkflow && item.workflow.ID == workflowID {
				workflowPath = item.workflow.Path
				break
			}
		}
		// Fetch workflow_dispatch inputs asynchronously.
		owner, repo := p.ghOwner, p.ghRepo
		ghClient := p.ghClient
		return p, func() tea.Msg {
			var wfInputs []ghclient.WorkflowInput
			if ghClient != nil && workflowPath != "" {
				fetched, err := ghClient.GetWorkflowInputs(ctx, owner, repo, workflowPath, ref)
				if err != nil {
					// Non-fatal: fall back to generic dialog.
					_ = err
				} else {
					wfInputs = fetched
				}
			}
			return workflowInputsFetchedMsg{
				workflowID:   workflowID,
				workflowName: workflowName,
				ref:          ref,
				inputs:       wfInputs,
			}
		}
	case opWorkflowDispatchInputs:
		// Step 2 complete: got the inputs. Parse and dispatch.
		// pendingName format: "id:name:ref"
		var workflowID int64
		var workflowName, ref string
		parts := strings.SplitN(name, ":", 3)
		if len(parts) == 3 {
			workflowID, _ = strconv.ParseInt(parts[0], 10, 64)
			workflowName = parts[1]
			ref = parts[2]
		}
		if workflowID == 0 || ref == "" {
			return p, nil
		}
		// Parse inputs from "key=value" lines.
		var inputs map[string]any
		inputText := strings.TrimSpace(msg.Value)
		if inputText != "" {
			inputs = make(map[string]any)
			for _, line := range strings.Split(inputText, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if kv := strings.SplitN(line, "=", 2); len(kv) == 2 {
					inputs[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
				}
			}
			if len(inputs) == 0 {
				inputs = nil
			}
		}
		owner, repo := p.ghOwner, p.ghRepo
		ghClient := p.ghClient
		return p, func() tea.Msg {
			err := ghClient.DispatchWorkflow(ctx, owner, repo, workflowID, ref, inputs)
			return workflowDispatchResultMsg{workflowName: workflowName, err: err}
		}

	case opPRMergeStrategy:
		// User selected a merge strategy from the picker.
		// pendingName format: "number:headBranch:title"
		parts := strings.SplitN(name, ":", 3)
		if len(parts) < 3 {
			return p, nil
		}
		prNumber, _ := strconv.Atoi(parts[0])
		headBranch := parts[1]
		prTitle := parts[2]
		if prNumber == 0 {
			return p, nil
		}

		strategy := msg.Value // "merge", "squash", or "rebase"

		// Store merge details for the confirmation step.
		p.pending = opPRMergeConfirm
		p.pendingName = fmt.Sprintf("%d:%s:%s", prNumber, strategy, headBranch)

		label := mergeStrategyLabel(strategy)
		confirmMsg := fmt.Sprintf("Merge PR #%d %q using %s?", prNumber, prTitle, label)
		return p, notify.ShowConfirm("Confirm Merge", confirmMsg)

	case opPRMergeConfirm:
		// User confirmed the merge. Execute it.
		// pendingName format: "number:strategy:headBranch"
		parts := strings.SplitN(name, ":", 3)
		if len(parts) < 3 {
			return p, nil
		}
		prNumber, _ := strconv.Atoi(parts[0])
		strategy := parts[1]
		headBranch := parts[2]
		if prNumber == 0 {
			return p, nil
		}
		return p, p.mergePRCmd(prNumber, strategy, headBranch)

	case opPRDeleteBranchAfterMerge:
		// User confirmed post-merge branch deletion.
		branch := name
		if branch == "" {
			return p, nil
		}
		client := p.ghClient
		owner, repo := p.ghOwner, p.ghRepo
		g := p.git
		return p, func() tea.Msg {
			remoteErr := client.DeleteBranch(ctx, owner, repo, branch)
			var localErr error
			if g != nil {
				localErr = g.BranchDelete(ctx, branch, false)
			}
			return prBranchDeleteResultMsg{
				branch:    branch,
				remoteErr: remoteErr,
				localErr:  localErr,
			}
		}
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Item list building
// ---------------------------------------------------------------------------
// buildItems constructs the per-tab item lists and positions cursors.
func (p *Panel) buildItems(branches []git.Branch, worktrees []git.Worktree, remotes []git.Remote, stashes []git.StashEntry, tags []git.Tag, reflog []git.ReflogEntry) {
	// Store data for rebuilds.
	p.lastBranches = branches
	p.lastWorktrees = worktrees
	p.lastRemotes = remotes
	p.lastStashes = stashes
	p.lastTags = tags
	p.lastReflog = reflog
	p.doBuildItems()
}

// doBuildItems constructs items from cached data into per-tab lists.
func (p *Panel) doBuildItems() {
	branches := p.lastBranches
	worktrees := p.lastWorktrees
	remotes := p.lastRemotes
	stashes := p.lastStashes
	tags := p.lastTags
	reflog := p.lastReflog
	var local, remote []git.Branch
	for _, b := range branches {
		if b.IsRemote {
			remote = append(remote, b)
		} else {
			local = append(local, b)
		}
	}
	// Branches tab
	p.tabItems[tabBranches] = nil
	for _, b := range local {
		hash := b.Hash
		p.tabItems[tabBranches] = append(p.tabItems[tabBranches], listItem{kind: kindLocalBranch, branch: b, hash: hash})
	}
	for _, b := range remote {
		hash := b.Hash
		p.tabItems[tabBranches] = append(p.tabItems[tabBranches], listItem{kind: kindRemoteBranch, branch: b, hash: hash})
	}
	// Worktrees tab
	p.tabItems[tabWorktrees] = nil
	for _, wt := range worktrees {
		hash := wt.Head
		if len(hash) > git.ShortHashLen {
			hash = hash[:git.ShortHashLen]
		}
		p.tabItems[tabWorktrees] = append(p.tabItems[tabWorktrees], listItem{kind: kindWorktree, worktree: wt, hash: hash})
	}
	// Remotes tab
	p.tabItems[tabRemotes] = nil
	p.remoteCount = len(remotes)
	for _, r := range remotes {
		p.tabItems[tabRemotes] = append(p.tabItems[tabRemotes], listItem{kind: kindRemote, remote: r})
		p.tabItems[tabRemotes] = append(p.tabItems[tabRemotes], listItem{
			kind: kindRemoteSub,
			text: "fetch: " + r.FetchURL,
		})
		if r.PushURL != r.FetchURL {
			p.tabItems[tabRemotes] = append(p.tabItems[tabRemotes], listItem{
				kind: kindRemoteSub,
				text: "push:  " + r.PushURL,
			})
		}
	}
	// Stash tab
	p.tabItems[tabStash] = nil
	for _, s := range stashes {
		p.tabItems[tabStash] = append(p.tabItems[tabStash], listItem{
			kind:  kindStashEntry,
			stash: s,
		})
	}
	// Tags tab
	p.tabItems[tabTags] = nil
	for _, tg := range tags {
		p.tabItems[tabTags] = append(p.tabItems[tabTags], listItem{
			kind: kindTag,
			tag:  tg,
			hash: tg.Hash,
		})
	}
	// Reflog tab
	p.tabItems[tabReflog] = nil
	for _, r := range reflog {
		hash := r.Hash
		if len(hash) > git.ShortHashLen {
			hash = hash[:git.ShortHashLen]
		}
		p.tabItems[tabReflog] = append(p.tabItems[tabReflog], listItem{
			kind:   kindReflogEntry,
			reflog: r,
			hash:   hash,
		})
	}
	// Default cursor to first item in branches tab; prefer current branch.
	p.tabCursor[tabBranches] = 0
	for i, item := range p.tabItems[tabBranches] {
		if (item.kind == kindLocalBranch || item.kind == kindRemoteBranch) && item.branch.IsCurrent {
			p.tabCursor[tabBranches] = i
			break
		}
	}
	p.tabCursor[tabWorktrees] = 0
	p.tabCursor[tabRemotes] = 0
	p.tabCursor[tabStash] = 0
	p.tabCursor[tabTags] = 0
	p.tabCursor[tabReflog] = 0
	p.tabOffset[tabBranches] = 0
	p.tabOffset[tabWorktrees] = 0
	p.tabOffset[tabRemotes] = 0
	p.tabOffset[tabStash] = 0
	p.tabOffset[tabTags] = 0
	p.tabOffset[tabReflog] = 0
	p.ensureCursorVisible()
}

// rebuildFromCurrent rebuilds items preserving cursor position within each tab.
func (p *Panel) rebuildFromCurrent() {
	savedCursors := p.tabCursor
	p.doBuildItems()
	// Restore cursors, clamping to valid range.
	for i := tabID(0); i < tabCount; i++ {
		if savedCursors[i] < len(p.tabItems[i]) {
			p.tabCursor[i] = savedCursors[i]
		} else if len(p.tabItems[i]) > 0 {
			p.tabCursor[i] = len(p.tabItems[i]) - 1
		} else {
			p.tabCursor[i] = 0
		}
	}
	p.ensureCursorVisible()
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------
func (p *Panel) renderLine(item listItem, width int, isCursor bool) string {
	switch item.kind {
	case kindLocalBranch, kindRemoteBranch:
		return p.renderBranch(item, width, isCursor)
	case kindWorktree:
		return p.renderWorktree(item, width, isCursor)
	case kindRemote:
		return p.renderRemote(item, width, isCursor)
	case kindRemoteSub:
		return p.renderRemoteSub(item, width, isCursor)
	case kindStashEntry:
		return p.renderStashEntry(item, width, isCursor)
	case kindIssue:
		return p.renderIssue(item, width, isCursor)
	case kindPR:
		return p.renderPR(item, width, isCursor)
	case kindActionRun:
		return p.renderActionRun(item, width, isCursor)
	case kindWorkflow:
		return p.renderWorkflow(item, width, isCursor)
	case kindRelease:
		return p.renderRelease(item, width, isCursor)
	case kindTag, kindRemoteTag:
		return p.renderTag(item, width, isCursor)
	case kindReflogEntry:
		return p.renderReflogEntry(item, width, isCursor)
	}
	return ""
}

// renderTabBar renders the clickable tab bar at the top of the panel.
// In ModeGit only the git tabs row is shown; in ModeGitHub only the
// GitHub row; in ModeAll both rows are shown when GitHub is available.
// When tab labels exceed the available width, names are automatically
// abbreviated to fit (e.g. "Branches" → "Br", "Workflows" → "Wf").
func (p *Panel) renderTabBar(width int) string {
	p.lastWidth = width
	activeNameStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(defaultColors.Header)).
		Underline(true)
	activeCountStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultColors.Hash))
	inactiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultColors.Dim))
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultColors.Dim))
	sep := sepStyle.Render(" · ")
	type tabDef struct {
		name  string
		short string
		count string
		id    tabID
	}
	// Render a row of tabs, abbreviating names when the full row is too wide.
	renderRow := func(tabs []tabDef, isActiveRow bool) string {
		// Calculate plain-text width of the row using full names.
		// Format: " Name count · Name count · ..."
		fullWidth := 1 // leading space
		for i, t := range tabs {
			fullWidth += runewidth.StringWidth(t.name) + 1 + runewidth.StringWidth(t.count) // "Name count"
			if i < len(tabs)-1 {
				fullWidth += 3 // " · "
			}
		}
		useShort := fullWidth > width
		parts := make([]string, 0, len(tabs))
		for _, t := range tabs {
			name := t.name
			if useShort && t.short != "" {
				name = t.short
			}
			if isActiveRow && t.id == p.activeTab {
				parts = append(parts, activeNameStyle.Render(name)+activeCountStyle.Render(" "+t.count))
			} else {
				parts = append(parts, inactiveStyle.Render(name+" "+t.count))
			}
		}
		line := " " + strings.Join(parts, sep)
		visW := lipgloss.Width(line)
		if visW < width {
			line += strings.Repeat(" ", width-visW)
		}
		return lipgloss.NewStyle().MaxWidth(width).Render(line)
	}
	gitTabs := []tabDef{
		{id: tabBranches, name: "Branches", short: "Br", count: fmt.Sprintf("%d", len(p.tabItems[tabBranches]))},
		{id: tabWorktrees, name: "Worktrees", short: "Wt", count: fmt.Sprintf("%d", len(p.tabItems[tabWorktrees]))},
		{id: tabRemotes, name: "Remotes", short: "Rm", count: fmt.Sprintf("%d", p.remoteCount)},
		{id: tabStash, name: "Stash", short: "St", count: fmt.Sprintf("%d", len(p.tabItems[tabStash]))},
		{id: tabTags, name: "Tags", short: "Tg", count: fmt.Sprintf("%d", len(p.tabItems[tabTags]))},
		{id: tabReflog, name: "Reflog", short: "Rl", count: fmt.Sprintf("%d", len(p.tabItems[tabReflog]))},
	}
	// Build GitHub tab row with status icons for Actions.
	actionsCount := p.actionsStatusIcon()
	issuesCount := fmt.Sprintf("%d", len(p.tabItems[tabIssues]))
	if p.issueFilter != issueFilterAll {
		issuesCount = p.issueFilter.String()
	}
	prsCount := fmt.Sprintf("%d", len(p.tabItems[tabPRs]))
	if p.prFilter != prFilterAll {
		prsCount = p.prFilter.String()
	}
	ghTabs := []tabDef{
		{id: tabIssues, name: "Issues", short: "Iss", count: issuesCount},
		{id: tabPRs, name: "PRs", short: "PRs", count: prsCount},
		{id: tabActions, name: "Actions", short: "Act", count: actionsCount},
		{id: tabWorkflows, name: "Workflows", short: "Wf", count: fmt.Sprintf("%d", len(p.tabItems[tabWorkflows]))},
		{id: tabReleases, name: "Releases", short: "Rel", count: fmt.Sprintf("%d", len(p.tabItems[tabReleases]))},
	}
	// In ModeGitHub, prepend Branches and Tags to the GitHub tab row.
	if p.mode == ModeGitHub {
		ghTabs = append([]tabDef{
			{id: tabBranches, name: "Branches", short: "Br", count: fmt.Sprintf("%d", len(p.tabItems[tabBranches]))},
			{id: tabTags, name: "Tags", short: "Tg", count: fmt.Sprintf("%d", len(p.tabItems[tabTags]))},
		}, ghTabs...)
	}
	switch p.mode {
	case ModeGit:
		return renderRow(gitTabs, isGitTab(p.activeTab))
	case ModeGitHub:
		return renderRow(ghTabs, isGitHubTab(p.activeTab))
	default: // ModeAll
		gitRow := renderRow(gitTabs, isGitTab(p.activeTab))
		if p.ghClient == nil {
			return gitRow
		}
		ghRow := renderRow(ghTabs, !isGitTab(p.activeTab))
		return gitRow + "\n" + ghRow
	}
}

// actionsStatusIcon returns a status icon for the Actions tab count.
// Uses ✓ for success, ✗ for failure, animated frame for in_progress, or the count.
// When actionsWatching is true the icon cycles through watchFrames to signal
// that the panel is actively polling a running CI workflow.
func (p *Panel) actionsStatusIcon() string {
	items := p.tabItems[tabActions]
	if len(items) == 0 {
		return "0"
	}
	// Check the latest run's status/conclusion.
	latest := items[0].actionRun
	switch latest.Conclusion {
	case "success": //nolint:goconst // inline string is more readable here
		return checkMark
	case "failure", "timed_out": //nolint:goconst // inline string is more readable here
		return crossMark
	}
	if latest.Status == "in_progress" || latest.Status == "queued" { //nolint:goconst // inline string is more readable here
		if p.actionsWatching {
			return watchFrames[p.actionsWatchFrame%len(watchFrames)]
		}
		return "●" //nolint:goconst // inline string is more readable here
	}
	return fmt.Sprintf("%d", len(items))
}

func (p *Panel) renderBranch(item listItem, width int, isCursor bool) string {
	b := item.branch
	prefix := "  "
	if b.IsCurrent {
		prefix = "* "
	}
	// Build right side — hash is always shown, never truncated.
	var rightParts []string
	if b.Ahead > 0 {
		rightParts = append(rightParts, fmt.Sprintf("↑%d", b.Ahead))
	}
	if b.Behind > 0 {
		rightParts = append(rightParts, fmt.Sprintf("↓%d", b.Behind))
	}
	rightSide := ""
	if len(rightParts) > 0 {
		rightSide = " " + strings.Join(rightParts, " ")
	}
	if b.Hash != "" {
		rightSide += " " + b.Hash
	}
	// Calculate available width for the name — truncate name, never hash.
	prefixLen := len(prefix)
	rightLen := lipgloss.Width(rightSide)
	nameWidth := width - prefixLen - rightLen - 1 // -1 for min gap
	name := b.Name
	if nameWidth > 0 && len(name) > nameWidth {
		if nameWidth > 1 {
			name = name[:nameWidth-1] + "…"
		} else {
			name = name[:nameWidth]
		}
	} else if nameWidth <= 0 {
		name = ""
	}
	leftSide := prefix + name
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide)
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	line := leftSide + gap + rightSide
	style := lipgloss.NewStyle().Width(width).MaxWidth(width)
	if isCursor {
		style = style.Background(lipgloss.Color(defaultColors.CursorBg))
	}
	if b.IsCurrent {
		style = style.Foreground(lipgloss.Color(defaultColors.Current)).Bold(true)
	} else if b.IsRemote {
		style = style.Foreground(lipgloss.Color(defaultColors.Remote))
	} else {
		style = style.Foreground(lipgloss.Color(defaultColors.Local))
	}
	return style.Render(line)
}

func (p *Panel) renderWorktree(item listItem, width int, isCursor bool) string {
	wt := item.worktree
	// Right side: branch + short hash — always shown.
	rightSide := ""
	if wt.Branch != "" {
		rightSide = " " + wt.Branch
	}
	short := wt.Head
	if len(short) > git.ShortHashLen {
		short = short[:git.ShortHashLen]
	}
	if short != "" {
		rightSide += " " + short
	}
	// Truncate path (left side) to fit, never truncate hash.
	prefix := "  "
	rightLen := lipgloss.Width(rightSide)
	pathWidth := width - len(prefix) - rightLen - 1
	path := wt.Path
	if pathWidth > 0 && len(path) > pathWidth {
		if pathWidth > 1 {
			path = path[:pathWidth-1] + "…"
		} else {
			path = path[:pathWidth]
		}
	} else if pathWidth <= 0 {
		path = ""
	}
	leftSide := prefix + path
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide)
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	line := leftSide + gap + rightSide
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(defaultColors.Worktree))
	if isCursor {
		style = style.Background(lipgloss.Color(defaultColors.CursorBg))
	}
	return style.Render(line)
}

func (p *Panel) renderRemote(item listItem, width int, isCursor bool) string {
	leftSide := "  " + item.remote.Name
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(defaultColors.RemoteC)).Bold(true)
	if isCursor {
		style = style.Background(lipgloss.Color(defaultColors.CursorBg))
	}
	return style.Render(leftSide)
}

func (p *Panel) renderRemoteSub(item listItem, width int, isCursor bool) string {
	leftSide := "    " + item.text
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(defaultColors.URL))
	if isCursor {
		style = style.Background(lipgloss.Color(defaultColors.CursorBg))
	}
	return style.Render(leftSide)
}

func (p *Panel) renderStashEntry(item listItem, width int, isCursor bool) string {
	s := item.stash
	label := fmt.Sprintf("  stash@{%d}: %s", s.Index, s.Message)
	// Truncate label to fit width.
	if len(label) > width {
		if width > 4 {
			label = label[:width-3] + "..."
		} else if width > 0 {
			label = label[:width]
		} else {
			label = ""
		}
	}
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(defaultColors.Worktree))
	if isCursor {
		style = style.Background(lipgloss.Color(defaultColors.CursorBg))
	}
	return style.Render(label)
}

func (p *Panel) renderReflogEntry(item listItem, width int, isCursor bool) string {
	r := item.reflog
	hash := r.Hash
	if len(hash) > git.ShortHashLen {
		hash = hash[:git.ShortHashLen]
	}
	age := reflogRelativeDate(r.Date)
	label := fmt.Sprintf("  %s %s %s (%s)", hash, r.Action, r.Message, age)
	if len(label) > width {
		if width > 4 {
			label = label[:width-3] + "..."
		} else if width > 0 {
			label = label[:width]
		} else {
			label = ""
		}
	}
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(defaultColors.Dim))
	if isCursor {
		style = style.Background(lipgloss.Color(defaultColors.CursorBg))
	}
	return style.Render(label)
}

// reflogRelativeDate formats a time.Time as a short relative date string.
func reflogRelativeDate(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		months := int(d.Hours() / 24 / 30)
		if months <= 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
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
		tabs = append(tabs,
			tabEntry{id: tabBranches, name: "Branches", short: "Br", count: fmt.Sprintf("%d", len(p.tabItems[tabBranches]))},
			tabEntry{id: tabTags, name: "Tags", short: "Tg", count: fmt.Sprintf("%d", len(p.tabItems[tabTags]))},
		)
	}
	issuesCount := fmt.Sprintf("%d", len(p.tabItems[tabIssues]))
	if p.issueFilter != issueFilterAll {
		issuesCount = p.issueFilter.String()
	}
	prsCount := fmt.Sprintf("%d", len(p.tabItems[tabPRs]))
	if p.prFilter != prFilterAll {
		prsCount = p.prFilter.String()
	}
	tabs = append(tabs,
		tabEntry{id: tabIssues, name: "Issues", short: "Iss", count: issuesCount},
		tabEntry{id: tabPRs, name: "PRs", short: "PRs", count: prsCount},
		tabEntry{id: tabActions, name: "Actions", short: "Act", count: p.actionsStatusIcon()},
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
	client := p.ghClient
	owner, repo := p.ghOwner, p.ghRepo
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
			State:       "open",
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
			State:       "open",
			ListOptions: gh.ListOptions{PerPage: 50},
		})
		if err != nil {
			slog.Warn("github: fetch PRs failed", "owner", owner, "repo", repo, "err", err)
		} else {
			for _, pr := range prs {
				state := pr.GetState()
				if pr.GetDraft() {
					state = "draft" //nolint:goconst // inline string is more readable here
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
		// Enrich open PRs with mergeable state (parallel individual fetches).
		if len(result.prs) > 0 {
			var wg sync.WaitGroup
			var mu sync.Mutex
			for i, pr := range result.prs {
				if pr.State != "open" {
					continue
				}
				wg.Add(1)
				go func(idx int, num int) {
					defer wg.Done()
					detail, err := client.GetPR(ctx, owner, repo, num)
					if err != nil {
						slog.Warn("github: fetch PR detail for mergeable state failed", "number", num, "err", err)
						return
					}
					mu.Lock()
					result.prs[idx].MergeableState = detail.GetMergeableState()
					mu.Unlock()
				}(i, pr.Number)
			}
			wg.Wait()
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
		p.ghErr = msg.err
		return p, nil
	}
	if msg.user != "" {
		p.ghUser = msg.user
	}
	p.repoPrivate = msg.repoPrivate
	p.buildGitHubItems(msg.issues, msg.prs, msg.actions, msg.workflows, msg.releases)
	// Determine if any workflow run is still in progress or queued.
	wasWatching := p.actionsWatching
	p.actionsWatching = false
	for _, a := range msg.actions {
		if a.Status == "in_progress" || a.Status == "queued" {
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
	p.allIssues = issues
	p.allPRs = prs
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
// Quick filter cycling
// ---------------------------------------------------------------------------
func (p *Panel) cycleIssueFilter() (panels.Panel, tea.Cmd) {
	p.issueFilter = (p.issueFilter + 1) % 4
	p.applyIssueFilter()
	filter := p.issueFilter.String()
	return p, func() tea.Msg {
		return panels.GitHubFilterChangedMsg{
			Tab:    "issues",
			Filter: filter,
		}
	}
}

func (p *Panel) cyclePRFilter() (panels.Panel, tea.Cmd) {
	p.prFilter = (p.prFilter + 1) % 4
	p.applyPRFilter()
	filter := p.prFilter.String()
	return p, func() tea.Msg {
		return panels.GitHubFilterChangedMsg{
			Tab:    "prs",
			Filter: filter,
		}
	}
}

func (p *Panel) applyIssueFilter() {
	p.tabItems[tabIssues] = nil
	for _, iss := range p.allIssues {
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
	switch p.issueFilter {
	case issueFilterAssigned:
		return iss.Assignee == p.ghUser
	case issueFilterMentioned:
		// GitHub list API doesn't expose "mentioned" — show all for now.
		return true
	case issueFilterCreated:
		return iss.Author == p.ghUser
	default:
		return true
	}
}

func (p *Panel) applyPRFilter() {
	p.tabItems[tabPRs] = nil
	for _, pr := range p.allPRs {
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
	switch p.prFilter {
	case prFilterNeedsReview:
		return pr.Author != p.ghUser && pr.State == "open" //nolint:goconst // inline string is more readable here
	case prFilterMine:
		return pr.Author == p.ghUser
	case prFilterDraft:
		return pr.State == "draft"
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
	if p.ghClient != nil {
		cmds = append(cmds, p.loadPRDetails(number))
	}
	return tea.Batch(cmds...)
}

// loadPRDetails returns a Cmd that fetches PR files and commits from GitHub.
func (p *Panel) loadPRDetails(number int) tea.Cmd {
	client := p.ghClient
	owner, repo := p.ghOwner, p.ghRepo
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
	if p.ghClient != nil {
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
	client := p.ghClient
	owner, repo := p.ghOwner, p.ghRepo
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
	if p.ghClient != nil {
		for _, j := range jobs {
			if j.Conclusion == "failure" {
				cmds = append(cmds, p.loadActionLog(runID, j.ID))
				break // only fetch logs for the first failed job
			}
		}
	}
	return p, tea.Batch(cmds...)
}

// loadActionLog returns a Cmd that fetches logs for a specific job.
func (p *Panel) loadActionLog(runID, jobID int64) tea.Cmd {
	client := p.ghClient
	owner, repo := p.ghOwner, p.ghRepo
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
	client := p.ghClient
	owner, repo := p.ghOwner, p.ghRepo
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
	client := p.ghClient
	owner, repo := p.ghOwner, p.ghRepo
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

// mergeStrategyLabel returns a human-readable label for a merge strategy ID.
func mergeStrategyLabel(strategy string) string {
	switch strategy {
	case "squash":
		return "squash and merge"
	case "rebase":
		return "rebase and merge"
	default:
		return "merge commit"
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
	if pr.State != "open" {
		stateLabel := pr.State
		if stateLabel == "" {
			stateLabel = "unknown"
		}
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: fmt.Sprintf("Cannot merge PR #%d: state is %s", pr.Number, stateLabel),
				Level:   notify.Warn,
			}
		}
	}

	if p.ghClient == nil {
		return p, nil
	}

	// Store PR details for multi-step flow.
	p.pending = opPRMergeStrategy
	p.pendingName = fmt.Sprintf("%d:%s:%s", pr.Number, pr.HeadBranch, pr.Title)

	mergeActions := []notify.ActionOption{
		{ID: "merge", Label: "Merge commit"},
		{ID: "squash", Label: "Squash and merge"},
		{ID: "rebase", Label: "Rebase and merge"},
	}

	title := fmt.Sprintf("Merge PR #%d", pr.Number)
	message := pr.Title
	return p, notify.ShowActionPickerWithMessage(title, message, mergeActions)
}

// mergePRCmd returns a tea.Cmd that executes the merge asynchronously.
func (p *Panel) mergePRCmd(number int, strategy string, headBranch string) tea.Cmd {
	client := p.ghClient
	owner, repo := p.ghOwner, p.ghRepo
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
	for i := range p.allPRs {
		if p.allPRs[i].Number == msg.number {
			p.allPRs[i].State = prStateMerged
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
		rightSide = " " + iss.Labels[0]
	}
	// Calculate available width for the title.
	prefixLen := lipgloss.Width(prefix) + lipgloss.Width(number)
	rightLen := lipgloss.Width(rightSide)
	titleWidth := width - prefixLen - rightLen - 1
	title := iss.Title
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
		Foreground(lipgloss.Color(defaultColors.Issue))
	if isCursor {
		style = style.Background(lipgloss.Color(defaultColors.CursorBg))
	}
	return style.Render(line)
}

// prColor returns the foreground color for a PR based on its state and
// mergeable status.
func prColor(pr ghPRItem) string {
	switch pr.State {
	case "draft":
		return defaultColors.PRDraft
	case prStateMerged:
		return defaultColors.PRMerged
	case "closed":
		return defaultColors.PRClosed
	default: // "open"
		switch pr.MergeableState {
		case "dirty":
			return defaultColors.PRConflict
		case "unstable":
			return defaultColors.PRUnstable
		case "blocked":
			return defaultColors.PRBlocked
		case "unknown":
			return defaultColors.PRUnknown
		default: // "clean" or ""
			return defaultColors.PR
		}
	}
}

// prActionIcon returns the status icon and its color for the action run
// associated with a PR. Returns empty strings when no action run exists.
func prActionIcon(pr ghPRItem) (icon string, color string) {
	switch pr.ActionConclusion {
	case "success":
		return checkMark, defaultColors.ActionOK
	case "failure", "timed_out":
		return crossMark, defaultColors.ActionFail
	}
	switch pr.ActionStatus {
	case "in_progress", "queued":
		return "●", defaultColors.ActionRun
	}
	return "", ""
}

// renderPR renders a GitHub PR line: "  #41 Add GitHub client   draft"
// Open PRs are colored by mergeable state, and an action-run status icon
// is appended when a matching workflow run exists for the PR’s head branch.
func (p *Panel) renderPR(item listItem, width int, isCursor bool) string {
	pr := item.pr
	prefix := "  "
	number := fmt.Sprintf("#%d ", pr.Number)

	// Action status icon (shown after state text).
	actionIcon, actionColor := prActionIcon(pr)
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
	title := pr.Title
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
	fg := prColor(pr)
	var line string
	if iconSuffix != "" {
		iconStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(actionColor))
		if isCursor {
			iconStyle = iconStyle.Background(lipgloss.Color(defaultColors.CursorBg))
		}
		line = leftSide + gap + rightSide + iconStyle.Render(iconSuffix)
	} else {
		line = leftSide + gap + rightSide
	}

	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(fg))
	if isCursor {
		style = style.Background(lipgloss.Color(defaultColors.CursorBg))
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
	case "success":
		icon = checkMark
		fg = defaultColors.ActionOK
	case "failure", "timed_out":
		icon = crossMark
		fg = defaultColors.ActionFail
	default:
		if run.Status == "in_progress" || run.Status == "queued" {
			icon = "●"
			fg = defaultColors.ActionRun
		} else {
			icon = "●"
			fg = defaultColors.Dim
		}
	}
	prefix := "  "
	left := fmt.Sprintf("%s %s #%d", icon, run.WorkflowName, run.RunNumber)
	// Right side: branch + timestamp.
	rightSide := ""
	if run.Branch != "" {
		rightSide += " " + run.Branch
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
		style = style.Background(lipgloss.Color(defaultColors.CursorBg))
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
	case "active":
		icon = "●"
		fg = defaultColors.Workflow
	case "disabled_manually", "disabled_inactivity":
		icon = "○"
		fg = defaultColors.Dim
	default:
		icon = "○"
		fg = defaultColors.Dim
	}
	prefix := "  "
	left := fmt.Sprintf("%s %s", icon, wf.Name)
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
		style = style.Background(lipgloss.Color(defaultColors.CursorBg))
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
		icon = "●"
		fg = defaultColors.RelDraft
	case rel.Prerelease:
		icon = "⚠"
		fg = defaultColors.RelPre
	default:
		icon = checkMark
		fg = defaultColors.Release
	}
	prefix := "  "
	left := fmt.Sprintf("%s %s", icon, rel.TagName)
	if rel.Name != "" && rel.Name != rel.TagName {
		left += "  " + rel.Name
	}
	rightSide := ""
	if rel.Author != "" {
		rightSide += " " + rel.Author
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
		style = style.Background(lipgloss.Color(defaultColors.CursorBg))
	}
	return style.Render(line)
}

func (p *Panel) renderTag(item listItem, width int, isCursor bool) string {
	tg := item.tag
	prefix := "  "
	// Build right side — annotated badge + hash.
	rightSide := ""
	if tg.IsAnnotated {
		rightSide += " [annotated]"
	}
	if tg.Hash != "" {
		rightSide += " " + tg.Hash
	}
	// Calculate available width for the name — truncate name, never hash.
	prefixLen := len(prefix)
	rightLen := lipgloss.Width(rightSide)
	nameWidth := width - prefixLen - rightLen - 1 // -1 for min gap
	name := tg.Name
	if nameWidth > 0 && len(name) > nameWidth {
		if nameWidth > 1 {
			name = name[:nameWidth-1] + "…"
		} else {
			name = name[:nameWidth]
		}
	} else if nameWidth <= 0 {
		name = ""
	}
	leftSide := prefix + name
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide)
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	line := leftSide + gap + rightSide
	// Color based on tag type.
	fg := defaultColors.Tag
	if item.kind == kindRemoteTag {
		fg = defaultColors.RemoteTag
	}
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(fg))
	if isCursor {
		style = style.Background(lipgloss.Color(defaultColors.CursorBg))
	}
	return style.Render(line)
}

// ---------------------------------------------------------------------------
// Clipboard
// ---------------------------------------------------------------------------
// copyHashToClipboard copies the hash of the item under cursor to clipboard.
func (p *Panel) copyHashToClipboard() (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return p, nil
	}
	hash := items[cursor].hash
	if hash == "" {
		return p, nil
	}
	if err := panels.CopyToClipboard(p.ctx, hash); err != nil {
		errMsg := err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Copy failed: " + errMsg, Level: notify.Error}
		}
	}
	return p, func() tea.Msg {
		return notify.ShowToastMsg{Message: "Copied: " + hash, Level: notify.Success}
	}
}

// doTagPush prompts for push confirmation and pushes the tag under the cursor.
func (p *Panel) doTagPush() (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return p, nil
	}
	item := items[cursor]
	if item.kind != kindTag && item.kind != kindRemoteTag {
		return p, nil
	}
	tg := item.tag
	p.pending = opTagPush
	p.pendingName = tg.Name
	return p, notify.ShowConfirm("Push Tag",
		fmt.Sprintf("Push tag %q to origin?", tg.Name))
}

// doTagDelete prompts for delete confirmation and deletes the tag under the cursor.
func (p *Panel) doTagDelete() (panels.Panel, tea.Cmd) {
	items := p.tabItems[p.activeTab]
	cursor := p.tabCursor[p.activeTab]
	if cursor < 0 || cursor >= len(items) {
		return p, nil
	}
	item := items[cursor]
	if item.kind == kindRemoteTag {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot delete remote-only tag locally", Level: notify.Warn}
		}
	}
	if item.kind != kindTag {
		return p, nil
	}
	tg := item.tag
	p.pending = opTagDelete
	p.pendingName = tg.Name
	return p, notify.ShowConfirm("Delete Tag", fmt.Sprintf("Delete tag %q?", tg.Name))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
// worktreePath computes the worktree directory for a branch following the
// convention: <parent>/.worktrees/<repo-name>/<branch-slug>
// currentBranch returns the name of the current (checked-out) branch, or "main" as fallback.
func (p *Panel) currentBranch() string {
	for _, item := range p.tabItems[tabBranches] {
		if item.kind == kindLocalBranch && item.branch.IsCurrent {
			return item.branch.Name
		}
	}
	return "main"
}

// worktreePath is an alias to the canonical implementation in the git package.
// See git.WorktreePath for the convention details.
func worktreePath(repoRoot, branch string) string {
	return git.WorktreePath(repoRoot, branch)
}

// remoteToHTTPS is a package-local alias for git.RemoteToHTTPS.
func remoteToHTTPS(raw string) string { return git.RemoteToHTTPS(raw) }
