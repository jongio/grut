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
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	ghclient "github.com/jongio/grut/internal/github"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/rightclick"
	"github.com/jongio/grut/internal/theme"
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
	Status(ctx context.Context) ([]git.FileStatus, error)
	StashPush(ctx context.Context, opts git.StashOpts) error
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

// pendingOp identifies which operation is awaiting modal input.
type pendingOp int

const (
	opNone                     pendingOp = iota
	opBranchCreate                       // awaiting new branch name
	opBranchDelete                       // awaiting delete confirmation
	opBranchRename                       // awaiting new name
	opBranchCheckout                     // awaiting checkout confirmation
	opBranchCheckoutStash                // awaiting dirty-tree stash-and-switch confirmation
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

// checkoutDirtyMsg is sent after checking dirty state before checkout.
type checkoutDirtyMsg struct {
	ref   string
	dirty bool
	err   error
}

// ---------------------------------------------------------------------------
// Default colors (Dracula-inspired, consistent with other panels)
// ---------------------------------------------------------------------------
type panelColors struct {
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
}

func initColors(th *theme.Theme) panelColors {
	c := panelColors{
		Current:    colorGreen,
		Local:      "#D4D4D4",
		Remote:     colorYellow,
		Header:     "#7A9EBF",
		Hash:       colorGray,
		CursorBg:   "#2A2A2A",
		Dim:        colorGray,
		Worktree:   colorOrange,
		RemoteC:    colorYellow,
		URL:        colorGray,
		Issue:      "#D4D4D4",
		PR:         colorGreen,
		PRConflict: "#C44B4B",
		PRUnstable: "#D4B84A",
		PRBlocked:  colorOrange,
		PRUnknown:  colorGray,
		PRClosed:   "#8B3A3A",
		PRDraft:    colorOrange,
		PRMerged:   colorYellow,
		ActionOK:   colorGreen,
		ActionFail: "#C44B4B",
		ActionRun:  "#D4B84A",
		Tag:        colorYellow,
		RemoteTag:  colorYellow,
		Release:    colorGreen,
		RelDraft:   colorGray,
		RelPre:     colorOrange,
		Workflow:   "#7A9EBF",
	}
	if th != nil {
		c.Current = th.Colors.GitBranch
		c.Local = th.Colors.Foreground
		c.Remote = th.Colors.NormalYellow
		c.Header = th.Colors.BrightBlue
		c.Hash = th.Colors.BrightBlack
		c.CursorBg = th.Colors.SelectionBg
		c.Dim = th.Colors.BrightBlack
		c.Worktree = th.Colors.NormalMagenta
		c.RemoteC = th.Colors.NormalYellow
		c.URL = th.Colors.BrightBlack
		c.Issue = th.Colors.Foreground
		c.PR = th.Colors.NormalGreen
		c.PRConflict = th.Colors.GitConflict
		c.PRUnstable = th.Colors.NormalYellow
		c.PRBlocked = th.Colors.NormalMagenta
		c.PRUnknown = th.Colors.BrightBlack
		c.PRClosed = th.Colors.NormalRed
		c.PRDraft = th.Colors.NormalMagenta
		c.PRMerged = th.Colors.NormalYellow
		c.ActionOK = th.Colors.NormalGreen
		c.ActionFail = th.Colors.NormalRed
		c.ActionRun = th.Colors.NormalYellow
		c.Tag = th.Colors.GitTag
		c.RemoteTag = th.Colors.GitTag
		c.Release = th.Colors.NormalGreen
		c.RelDraft = th.Colors.BrightBlack
		c.RelPre = th.Colors.NormalMagenta
		c.Workflow = th.Colors.BrightBlue
	}
	return c
}

// defaultColors provides hardcoded fallback colors used by package-level
// helper functions (prColor, prActionIcon) that are called from tests.
var defaultColors = initColors(nil)

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
	pendingPath string // path captured at double-click time (survives async modal delay)
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
	colors            panelColors
	theme             *theme.Theme
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
	// Per-tab pagination state for lazy-loading GitHub tabs.
	tabPaging  [tabCount]tabPagination
	ghPageSize int // effective page size from config or default
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
	case sectionBranches:
		p.activeTab = tabBranches
	case sectionWorktrees:
		p.activeTab = tabWorktrees
	case sectionRemotes:
		p.activeTab = tabRemotes
	case sectionStash:
		p.activeTab = tabStash
	case sectionTags:
		p.activeTab = tabTags
	case sectionReflog:
		p.activeTab = tabReflog
	case sectionIssues:
		p.activeTab = tabIssues
	case sectionPRs:
		p.activeTab = tabPRs
	case sectionActions:
		p.activeTab = tabActions
	case "workflows":
		p.activeTab = tabWorkflows
	case "releases":
		p.activeTab = tabReleases
	}
}

// New creates a new gitinfo panel showing only git tabs (branches,
// worktrees, remotes, stash, tags, reflog).
func New(gitOps gitOps, cfg config.GitConfig, ghCfg config.GitHubConfig, actionsCfg config.ActionsConfig, repoRoot, iconMode string, th *theme.Theme) *Panel {
	return &Panel{
		BasePanel:  panels.BasePanel{PanelTitle: panelGitinfo},
		mode:       ModeGit,
		git:        gitOps,
		cfg:        cfg,
		ghCfg:      ghCfg,
		actionsCfg: actionsCfg,
		iconMode:   iconMode,
		repoRoot:   repoRoot,
		colors:     initColors(th),
		theme:      th,
	}
}

// NewGitHub creates a gitinfo panel showing only GitHub tabs (issues,
// PRs, actions, workflows, releases).
func NewGitHub(gitOps gitOps, cfg config.GitConfig, ghCfg config.GitHubConfig, actionsCfg config.ActionsConfig, repoRoot, iconMode string, th *theme.Theme) *Panel {
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
		colors:     initColors(th),
		theme:      th,
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
		p.ghPageSize = p.ghCfg.EffectivePageSize()
		for _, tab := range []tabID{tabIssues, tabPRs, tabActions, tabWorkflows, tabReleases} {
			p.tabPaging[tab] = tabPagination{loading: true, nextPage: 1}
		}
		cmds = append(
			cmds,
			p.loadGitHubMeta(),
			p.loadIssuesPage(1, true),
			p.loadPRsPage(1, true),
			p.loadActionsPage(1, true),
			p.loadWorkflowsPage(1, true),
			p.loadReleasesPage(1, true),
			p.githubPollTickCmd(),
		)
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
	// Reset pagination state.
	for i := range p.tabPaging {
		p.tabPaging[i] = tabPagination{}
	}
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
		p.ghPageSize = p.ghCfg.EffectivePageSize()
		for _, tab := range []tabID{tabIssues, tabPRs, tabActions, tabWorkflows, tabReleases} {
			p.tabPaging[tab] = tabPagination{loading: true, nextPage: 1}
		}
		cmds = append(
			cmds,
			p.loadGitHubMeta(),
			p.loadIssuesPage(1, true),
			p.loadPRsPage(1, true),
			p.loadActionsPage(1, true),
			p.loadWorkflowsPage(1, true),
			p.loadReleasesPage(1, true),
			p.githubPollTickCmd(),
		)
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
			Target: panelGitinfo,
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
			Target: panelGitinfo,
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
	case ghMetaLoadedMsg:
		return p.handleMetaLoaded(msg)
	case ghIssuesPageMsg:
		return p.handleIssuesPage(msg)
	case ghPRsPageMsg:
		return p.handlePRsPage(msg)
	case ghActionsPageMsg:
		return p.handleActionsPage(msg)
	case ghWorkflowsPageMsg:
		return p.handleWorkflowsPage(msg)
	case ghReleasesPageMsg:
		return p.handleReleasesPage(msg)
	case githubPollTickMsg:
		// Reset pagination state for fresh page-1 loads.
		for _, tab := range []tabID{tabIssues, tabPRs, tabActions, tabWorkflows, tabReleases} {
			p.tabPaging[tab] = tabPagination{loading: true, nextPage: 1}
		}
		return p, tea.Batch(
			p.loadGitHubMeta(),
			p.loadIssuesPage(1, true),
			p.loadPRsPage(1, true),
			p.loadActionsPage(1, true),
			p.loadWorkflowsPage(1, true),
			p.loadReleasesPage(1, true),
			p.githubPollTickCmd(),
		)
	case actionsWatchTickMsg:
		if !p.actionsWatching {
			return p, nil // stop ticking — no in-progress runs
		}
		p.actionsWatchFrame = (p.actionsWatchFrame + 1) % len(watchFrames)
		return p, p.actionsWatchTickCmd()
	case opResultMsg:
		return p.handleOpResult(msg)
	case checkoutDirtyMsg:
		return p.handleCheckoutDirty(msg)
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
			Foreground(lipgloss.Color(p.colors.Dim)).
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
	// Loading indicator for GitHub tabs.
	if p.activeTab >= tabIssues && p.activeTab <= tabReleases && p.tabPaging[p.activeTab].loading && len(lines) < contentHeight {
		loadingLine := lipgloss.NewStyle().
			Width(width).
			Foreground(lipgloss.Color(defaultColors.Dim)).
			Render("  Loading...")
		lines = append(lines, loadingLine)
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
		{Key: "PgDn", Description: "Page down", Action: "page_down"},
		{Key: "PgUp", Description: "Page up", Action: "page_up"},
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
		bindings = append(
			bindings,
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
	case opCheckout:
		cmds = append(
			cmds,
			func() tea.Msg { return panels.BranchChangedMsg{Name: name} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Switched to " + name, Level: notify.Success}
			},
		)
	case "checkout_stashed":
		cmds = append(
			cmds,
			func() tea.Msg { return panels.BranchChangedMsg{Name: name} },
			func() tea.Msg { return panels.StashChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Stashed changes, switched to " + name, Level: notify.Success}
			},
		)
	case eventBranchCreated:
		cmds = append(cmds, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Branch created: " + name, Level: notify.Success}
		})
	case eventBranchDeleted:
		cmds = append(cmds, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Branch deleted: " + name, Level: notify.Success}
		})
	case eventBranchRenamed:
		cmds = append(cmds, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Branch renamed to: " + name, Level: notify.Success}
		})
	case eventWorktreeAdded:
		cmds = append(
			cmds,
			func() tea.Msg { return panels.WorktreeChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Worktree created: " + name, Level: notify.Success}
			},
		)
	case eventWorktreeRemoved:
		cmds = append(
			cmds,
			func() tea.Msg { return panels.WorktreeChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Worktree removed: " + name, Level: notify.Success}
			},
		)
	case eventWorktreeSwitch:
		cmds = append(cmds, func() tea.Msg {
			return panels.ChangeDirectoryMsg{Path: name}
		})
	case eventRemoteAdded:
		cmds = append(
			cmds,
			func() tea.Msg { return panels.RemoteChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Remote added: " + name, Level: notify.Success}
			},
		)
	case eventRemoteRemoved:
		cmds = append(
			cmds,
			func() tea.Msg { return panels.RemoteChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Remote removed: " + name, Level: notify.Success}
			},
		)
	case opFetched:
		cmds = append(cmds, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Fetched: " + name, Level: notify.Success}
		})
	case eventStashApplied:
		cmds = append(
			cmds,
			func() tea.Msg { return panels.StashChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Applied " + name, Level: notify.Success}
			},
		)
	case eventStashPopped:
		cmds = append(
			cmds,
			func() tea.Msg { return panels.StashChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Popped " + name, Level: notify.Success}
			},
		)
	case eventStashDropped:
		cmds = append(
			cmds,
			func() tea.Msg { return panels.StashChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Dropped " + name, Level: notify.Success}
			},
		)
	case eventTagCreated:
		cmds = append(
			cmds,
			func() tea.Msg { return panels.TagChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Tag created: " + name, Level: notify.Success}
			},
		)
	case eventTagDeleted:
		cmds = append(
			cmds,
			func() tea.Msg { return panels.TagChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Tag deleted: " + name, Level: notify.Success}
			},
		)
	case eventTagPushed:
		cmds = append(
			cmds,
			func() tea.Msg { return panels.TagChangedMsg{} },
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Tag pushed: " + name, Level: notify.Success}
			},
		)
	case eventTagCheckout:
		cmds = append(
			cmds,
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
	case "pgdown":
		p.pageDown()
		return p, tea.Batch(p.activeTabSelectionCmd(), p.loadMoreIfNeeded())
	case "pgup":
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
		return p, tea.Batch(p.activeTabSelectionCmd(), p.loadMoreIfNeeded())
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
		return p, tea.Batch(p.activeTabSelectionCmd(), p.loadMoreIfNeeded())
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
		p.clearPending()
		p.pending = opRightClickPick
		if item.kind == kindWorktree {
			p.pendingPath = item.worktree.Path
		} else {
			p.pendingPath = ""
		}
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
		return panels.StripANSI(item.branch.Name)
	case kindWorktree:
		return panels.StripANSI(item.worktree.Branch)
	case kindRemote:
		return panels.StripANSI(item.remote.Name)
	case kindStashEntry:
		return fmt.Sprintf("stash@{%d}", item.stash.Index)
	case kindIssue:
		return fmt.Sprintf("#%d %s", item.issue.Number, panels.StripANSI(item.issue.Title))
	case kindPR:
		return fmt.Sprintf("#%d %s", item.pr.Number, panels.StripANSI(item.pr.Title))
	case kindActionRun:
		return fmt.Sprintf("#%d %s", item.actionRun.RunNumber, panels.StripANSI(item.actionRun.WorkflowName))
	case kindWorkflow:
		return panels.StripANSI(item.workflow.Name)
	case kindRelease:
		return panels.StripANSI(item.release.TagName) + " " + panels.StripANSI(item.release.Name)
	case kindTag, kindRemoteTag:
		return panels.StripANSI(item.tag.Name)
	default:
		return panels.StripANSI(item.text)
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
	return p, p.loadMoreIfNeeded()
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
		{id: tabBranches, name: labelBranches, short: "Br", count: fmt.Sprintf("%d", len(p.tabItems[tabBranches]))},
		{id: tabWorktrees, name: labelWorktrees, short: "Wt", count: fmt.Sprintf("%d", len(p.tabItems[tabWorktrees]))},
		{id: tabRemotes, name: labelRemotes, short: "Rm", count: fmt.Sprintf("%d", p.remoteCount)},
		{id: tabStash, name: labelStash, short: "St", count: fmt.Sprintf("%d", len(p.tabItems[tabStash]))},
		{id: tabTags, name: labelTags, short: "Tg", count: fmt.Sprintf("%d", len(p.tabItems[tabTags]))},
		{id: tabReflog, name: labelReflog, short: "Rl", count: fmt.Sprintf("%d", len(p.tabItems[tabReflog]))},
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
		p.clearPending()
		p.pending = opFirstUseConfirm
		p.pendingName = string(itemType)
		// Capture path at double-click time so it survives cursor resets
		// that may happen during the async modal delay (e.g. data reload).
		if item.kind == kindWorktree {
			p.pendingPath = item.worktree.Path
		} else {
			p.pendingPath = ""
		}
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
			if err := panels.OpenInBrowser(p.ctx, url); err != nil {
				return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
			}
			return notify.ShowToastMsg{Message: "Opened " + item.remote.Name, Level: notify.Info}
		}
	case kindStashEntry:
		s := item.stash
		p.clearPending()
		p.pending = opStashAction
		p.pendingName = fmt.Sprintf("%d", s.Index)
		return p, notify.ShowInputWithValue("Stash Action",
			"apply, pop, or drop", actionApply)
	case kindIssue:
		url := item.issue.HTMLURL
		if url == "" {
			return p, nil
		}
		return p, func() tea.Msg {
			if err := panels.OpenInBrowser(p.ctx, url); err != nil {
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
			if err := panels.OpenInBrowser(p.ctx, url); err != nil {
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
			if err := panels.OpenInBrowser(p.ctx, url); err != nil {
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
			if err := panels.OpenInBrowser(p.ctx, url); err != nil {
				return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
			}
			return notify.ShowToastMsg{Message: "Opened release " + item.release.TagName, Level: notify.Info}
		}
	case kindTag, kindRemoteTag:
		tg := item.tag
		p.clearPending()
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
		wtPath := p.pendingPath
		if wtPath == "" {
			wtPath = item.worktree.Path
		}
		switch action { //nolint:exhaustive // only relevant cases handled
		case actions.ActionChangeDirectory:
			// pendingPath is consumed inside requestWorktreeSwitch
			return p.requestWorktreeSwitch()
		case actions.ActionOpenTerminal:
			p.pendingPath = ""
			return p, func() tea.Msg {
				if err := panels.OpenInTerminal(p.ctx, wtPath); err != nil {
					return notify.ShowToastMsg{Message: "Terminal error: " + err.Error(), Level: notify.Error}
				}
				return notify.ShowToastMsg{Message: "Opened terminal at " + wtPath, Level: notify.Success}
			}
		case actions.ActionCopyPath:
			p.pendingPath = ""
			return p.copyAndToast(wtPath)
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
			p.clearPending()
			p.pending = opStashAction
			p.pendingName = fmt.Sprintf("%d", s.Index)
			return p, notify.ShowInputWithValue("Stash Action", "apply, pop, or drop", actionApply)
		case actions.ActionApply:
			idx := s.Index
			return p, func() tea.Msg {
				err := p.git.StashApply(p.ctx, idx)
				return opResultMsg{op: eventStashApplied, name: fmt.Sprintf("stash@{%d}", idx), err: err}
			}
		case actions.ActionPop:
			idx := s.Index
			return p, func() tea.Msg {
				err := p.git.StashPop(p.ctx, idx)
				return opResultMsg{op: eventStashPopped, name: fmt.Sprintf("stash@{%d}", idx), err: err}
			}
		case actions.ActionDrop:
			idx := s.Index
			return p, func() tea.Msg {
				err := p.git.StashDrop(p.ctx, idx)
				return opResultMsg{op: eventStashDropped, name: fmt.Sprintf("stash@{%d}", idx), err: err}
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
			p.clearPending()
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
			p.clearPending()
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
		if err := panels.OpenInBrowser(p.ctx, url); err != nil {
			return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Opened " + label, Level: notify.Info}
	}
}

func (p *Panel) doCreate() (panels.Panel, tea.Cmd) {
	switch p.activeTab { //nolint:exhaustive // only relevant cases handled
	case tabBranches:
		p.clearPending()
		p.pending = opBranchCreate
		return p, notify.ShowInput("New Branch", "branch-name")
	case tabWorktrees:
		p.clearPending()
		p.pending = opWorktreeCreate
		return p, notify.ShowInput("New Worktree Branch", "branch-name")
	case tabRemotes:
		p.clearPending()
		p.pending = opRemoteAdd
		return p, notify.ShowInput("Remote Name", "remote-name")
	case tabTags:
		p.clearPending()
		p.pending = opTagCreate
		return p, notify.ShowInput("Tag Name", "tag-name")
	case tabIssues:
		if p.ghOwner != "" && p.ghRepo != "" {
			url := fmt.Sprintf("https://github.com/%s/%s/issues/new", p.ghOwner, p.ghRepo)
			return p, func() tea.Msg {
				if err := panels.OpenInBrowser(p.ctx, url); err != nil {
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
		p.clearPending()
		p.pending = opBranchDelete
		p.pendingName = b.Name
		return p, notify.ShowConfirm("Delete Branch", fmt.Sprintf("Delete branch %q?", b.Name))
	case kindRemoteBranch:
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot delete remote branch locally", Level: notify.Warn}
		}
	case kindWorktree:
		wt := item.worktree
		p.clearPending()
		p.pending = opWorktreeDelete
		p.pendingName = wt.Path
		return p, notify.ShowConfirm("Remove Worktree", fmt.Sprintf("Remove worktree at %q?", wt.Path))
	case kindRemote:
		r := item.remote
		p.clearPending()
		p.pending = opRemoteDelete
		p.pendingName = r.Name
		return p, notify.ShowConfirm("Remove Remote", fmt.Sprintf("Remove remote %q?", r.Name))
	case kindTag:
		tg := item.tag
		p.clearPending()
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
		if err := panels.OpenInBrowser(p.ctx, url); err != nil {
			return notify.ShowToastMsg{Message: "Open failed: " + err.Error(), Level: notify.Error}
		}
		return notify.ShowToastMsg{Message: "Opened " + label, Level: notify.Info}
	}
}

// ---------------------------------------------------------------------------
// Modal result handling
// ---------------------------------------------------------------------------

// clearPending resets all pending-operation state so that no stale values
// leak across interactions. Call this before setting new pending state.
func (p *Panel) clearPending() {
	p.pending = opNone
	p.pendingName = ""
	p.pendingPath = ""
}

func (p *Panel) handleModalResult(msg notify.ModalResultMsg) (panels.Panel, tea.Cmd) {
	op := p.pending
	name := p.pendingName
	pendingPath := p.pendingPath
	p.clearPending()
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
			return opResultMsg{op: eventBranchCreated, name: newName, err: err}
		}
	case opBranchDelete:
		return p, func() tea.Msg {
			err := g.BranchDelete(ctx, name, false)
			return opResultMsg{op: eventBranchDeleted, name: name, err: err}
		}
	case opBranchRename:
		newName := strings.TrimSpace(msg.Value)
		if newName == "" || newName == name {
			return p, nil
		}
		return p, func() tea.Msg {
			err := g.BranchRename(ctx, name, newName)
			return opResultMsg{op: eventBranchRenamed, name: newName, err: err}
		}
	case opWorktreeCreate:
		branch := strings.TrimSpace(msg.Value)
		if branch == "" {
			return p, nil
		}
		path := worktreePath(p.repoRoot, branch)
		return p, func() tea.Msg {
			err := g.WorktreeAdd(ctx, path, branch)
			return opResultMsg{op: eventWorktreeAdded, name: branch, err: err}
		}
	case opWorktreeDelete:
		return p, func() tea.Msg {
			err := g.WorktreeRemove(ctx, name, false)
			return opResultMsg{op: eventWorktreeRemoved, name: name, err: err}
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
			return opResultMsg{op: eventRemoteAdded, name: remoteName, err: err}
		}
	case opRemoteDelete:
		return p, func() tea.Msg {
			err := g.RemoteRemove(ctx, name)
			return opResultMsg{op: eventRemoteRemoved, name: name, err: err}
		}
	case opBranchCheckout:
		ref := name
		return p, func() tea.Msg {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic during branch checkout", "ref", ref, "panic", r)
				}
			}()
			files, err := g.Status(ctx)
			if err != nil {
				return checkoutDirtyMsg{ref: ref, err: err}
			}
			return checkoutDirtyMsg{ref: ref, dirty: len(files) > 0}
		}
	case opBranchCheckoutStash:
		ref := name
		return p, func() tea.Msg {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic during stash checkout", "ref", ref, "panic", r)
				}
			}()
			err := g.StashPush(ctx, git.StashOpts{Message: "grut: auto-stash before switching to " + ref})
			if err != nil {
				return opResultMsg{op: opCheckout, name: ref, err: fmt.Errorf("stash failed: %w", err)}
			}
			err = g.Checkout(ctx, ref)
			if err != nil {
				_ = g.StashPop(ctx, 0) // restore stash on checkout failure
				return opResultMsg{op: opCheckout, name: ref, err: err}
			}
			return opResultMsg{op: "checkout_stashed", name: ref}
		}
	case opStashAction:
		action := strings.TrimSpace(strings.ToLower(msg.Value))
		idx, err := strconv.Atoi(name)
		if err != nil {
			return p, nil
		}
		switch action {
		case actionApply, "a":
			return p, func() tea.Msg {
				err := g.StashApply(ctx, idx)
				return opResultMsg{op: eventStashApplied, name: fmt.Sprintf("stash@{%d}", idx), err: err}
			}
		case actionPop, "p":
			return p, func() tea.Msg {
				err := g.StashPop(ctx, idx)
				return opResultMsg{op: eventStashPopped, name: fmt.Sprintf("stash@{%d}", idx), err: err}
			}
		case actionDrop, "d":
			return p, func() tea.Msg {
				err := g.StashDrop(ctx, idx)
				return opResultMsg{op: eventStashDropped, name: fmt.Sprintf("stash@{%d}", idx), err: err}
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
		p.pendingPath = pendingPath // restore for executeRightClickAction
		return p.executeRightClickAction(actions.ActionID(msg.Value))
	case opRightClickPick:
		p.pendingPath = pendingPath // restore for executeRightClickAction
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
			return opResultMsg{op: eventTagCreated, name: tagName, err: err}
		}
	case opTagDelete:
		return p, func() tea.Msg {
			err := g.TagDelete(ctx, name)
			return opResultMsg{op: eventTagDeleted, name: name, err: err}
		}
	case opTagPush:
		tagName := name
		return p, func() tea.Msg {
			err := g.TagPush(ctx, "origin", tagName)
			return opResultMsg{op: eventTagPushed, name: tagName, err: err}
		}
	case opTagCheckout:
		return p, func() tea.Msg {
			err := g.Checkout(ctx, name)
			return opResultMsg{op: eventTagCheckout, name: name, err: err}
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
	// Branches tab — filter by panel mode:
	//   ModeGit    → local branches only
	//   ModeGitHub → remote branches only
	//   ModeAll    → both (backwards compat)
	p.tabItems[tabBranches] = nil
	if p.mode != ModeGitHub {
		for _, b := range local {
			hash := b.Hash
			p.tabItems[tabBranches] = append(p.tabItems[tabBranches], listItem{kind: kindLocalBranch, branch: b, hash: hash})
		}
	}
	if p.mode != ModeGit {
		for _, b := range remote {
			hash := b.Hash
			p.tabItems[tabBranches] = append(p.tabItems[tabBranches], listItem{kind: kindRemoteBranch, branch: b, hash: hash})
		}
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
		Foreground(lipgloss.Color(p.colors.Header)).
		Underline(true)
	activeCountStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.colors.Hash))
	inactiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.colors.Dim))
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.colors.Dim))
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
		{id: tabBranches, name: labelBranches, short: "Br", count: fmt.Sprintf("%d", len(p.tabItems[tabBranches]))},
		{id: tabWorktrees, name: labelWorktrees, short: "Wt", count: fmt.Sprintf("%d", len(p.tabItems[tabWorktrees]))},
		{id: tabRemotes, name: labelRemotes, short: "Rm", count: fmt.Sprintf("%d", p.remoteCount)},
		{id: tabStash, name: labelStash, short: "St", count: fmt.Sprintf("%d", len(p.tabItems[tabStash]))},
		{id: tabTags, name: labelTags, short: "Tg", count: fmt.Sprintf("%d", len(p.tabItems[tabTags]))},
		{id: tabReflog, name: labelReflog, short: "Rl", count: fmt.Sprintf("%d", len(p.tabItems[tabReflog]))},
	}
	// Build GitHub tab row with status icons for Actions.
	actionsCount := p.actionsStatusIcon()
	issuesCount := p.ghTabCountStr(tabIssues)
	if p.issueFilter != issueFilterAll {
		issuesCount = p.issueFilter.String()
	}
	prsCount := p.ghTabCountStr(tabPRs)
	if p.prFilter != prFilterAll {
		prsCount = p.prFilter.String()
	}
	ghTabs := []tabDef{
		{id: tabIssues, name: labelIssues, short: "Iss", count: issuesCount},
		{id: tabPRs, name: labelPRs, short: shortPRs, count: prsCount},
		{id: tabActions, name: labelActions, short: shortAct, count: actionsCount},
		{id: tabWorkflows, name: "Workflows", short: "Wf", count: p.ghTabCountStr(tabWorkflows)},
		{id: tabReleases, name: "Releases", short: "Rel", count: p.ghTabCountStr(tabReleases)},
	}
	// In ModeGitHub, prepend Branches and Tags to the GitHub tab row.
	if p.mode == ModeGitHub {
		ghTabs = append([]tabDef{
			{id: tabBranches, name: labelBranches, short: "Br", count: fmt.Sprintf("%d", len(p.tabItems[tabBranches]))},
			{id: tabTags, name: labelTags, short: "Tg", count: fmt.Sprintf("%d", len(p.tabItems[tabTags]))},
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
	case conclusionSuccess:
		return checkMark
	case conclusionFailure, conclusionTimedOut:
		return crossMark
	}
	if latest.Status == statusInProgress || latest.Status == statusQueued {
		if p.actionsWatching {
			return watchFrames[p.actionsWatchFrame%len(watchFrames)]
		}
		return "●" //nolint:goconst // inline string is more readable here
	}
	return fmt.Sprintf("%d", len(items))
}

// when additional pages have not been loaded yet.
// mergeStrategyLabel returns a human-readable label for a merge strategy ID.
// mergeable status, using the given color palette.

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
