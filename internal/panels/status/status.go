// Package status implements the compact repository status dashboard panel.
package status

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/theme"
)

type GitClient interface {
	Status(ctx context.Context) ([]git.FileStatus, error)
	CurrentBranch(ctx context.Context) (git.Branch, error)
	IsRepo(ctx context.Context) (bool, error)
}

type loadResultMsg struct {
	err    error
	branch git.Branch
	files  []git.FileStatus
}

type summary struct {
	staged    int
	unstaged  int
	untracked int
	conflicts int
}

type Panel struct {
	git gitClientFactory
	ctx context.Context
	err error
	panels.BasePanel
	branch  git.Branch
	summary summary
	loading bool
	dim     lipgloss.Style
	ok      lipgloss.Style
	warn    lipgloss.Style
}

type gitClientFactory interface {
	GitClient
}

var _ panels.Panel = (*Panel)(nil)

func New(client GitClient, th *theme.Theme) *Panel {
	tc := themeColors(th)
	return &Panel{
		BasePanel: panels.BasePanel{PanelTitle: "status"},
		git:       client,
		dim:       lipgloss.NewStyle().Foreground(lipgloss.Color(tc.Dim)),
		ok:        lipgloss.NewStyle().Foreground(lipgloss.Color(tc.Success)),
		warn:      lipgloss.NewStyle().Foreground(lipgloss.Color(tc.Warning)),
	}
}

func (p *Panel) Init(ctx context.Context) tea.Cmd {
	p.ctx = ctx
	if p.git == nil {
		return nil
	}
	p.loading = true
	return p.loadCmd()
}

func (p *Panel) Update(msg tea.Msg) (panels.Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case loadResultMsg:
		p.loading = false
		p.err = msg.err
		p.branch = msg.branch
		p.summary = summarize(msg.files)
	case panels.RepoChangedMsg:
		return p.handleRepoChanged(msg)
	case tea.KeyPressMsg:
		if msg.String() == "r" || msg.String() == "ctrl+r" {
			return p, p.reload()
		}
	}
	return p, nil
}

func (p *Panel) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := p.lines()
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		lines[i] = trimLine(line, width)
	}
	return strings.Join(lines, "\n")
}

func (p *Panel) KeyBindings() []panels.KeyBinding {
	return []panels.KeyBinding{
		{Key: "r", Description: "Refresh status", Action: "refresh"},
	}
}

func (p *Panel) reload() tea.Cmd {
	if p.git == nil {
		return nil
	}
	p.loading = true
	return p.loadCmd()
}

func (p *Panel) loadCmd() tea.Cmd {
	client := p.git
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return func() tea.Msg {
		ok, err := client.IsRepo(ctx)
		if err != nil || !ok {
			return loadResultMsg{err: fmt.Errorf("not a git repository")}
		}
		branch, branchErr := client.CurrentBranch(ctx)
		files, statusErr := client.Status(ctx)
		if statusErr != nil {
			return loadResultMsg{err: statusErr}
		}
		if branchErr != nil {
			branch = git.Branch{Name: "unknown", IsCurrent: true}
		}
		return loadResultMsg{branch: branch, files: files}
	}
}

func (p *Panel) handleRepoChanged(msg panels.RepoChangedMsg) (panels.Panel, tea.Cmd) {
	client, err := git.NewClient(msg.Path)
	if err != nil {
		p.git = nil
		p.err = nil
		p.branch = git.Branch{}
		p.summary = summary{}
		p.loading = false
		return p, nil
	}
	ok, err := client.IsRepo(context.Background())
	if err != nil || !ok {
		p.git = nil
		p.err = nil
		p.branch = git.Branch{}
		p.summary = summary{}
		p.loading = false
		return p, nil
	}
	p.git = client
	return p, p.reload()
}

func (p *Panel) lines() []string {
	if p.git == nil {
		return []string{"No git repository", p.dim.Render("Open a repository to see status.")}
	}
	if p.loading {
		return []string{"Loading status..."}
	}
	if p.err != nil {
		return []string{"Status unavailable", p.warn.Render(p.err.Error())}
	}

	state := p.ok.Render("clean")
	if p.summary.dirty() {
		state = p.warn.Render("dirty")
	}
	branch := p.branch.Name
	if branch == "" {
		branch = "unknown"
	}
	sync := "up to date"
	if p.branch.Ahead > 0 || p.branch.Behind > 0 {
		sync = fmt.Sprintf("ahead %d, behind %d", p.branch.Ahead, p.branch.Behind)
	}

	return []string{
		"Repository",
		fmt.Sprintf("Branch:    %s", branch),
		fmt.Sprintf("State:     %s", state),
		fmt.Sprintf("Sync:      %s", sync),
		"",
		"Changes",
		fmt.Sprintf("Staged:    %d", p.summary.staged),
		fmt.Sprintf("Unstaged:  %d", p.summary.unstaged),
		fmt.Sprintf("Untracked: %d", p.summary.untracked),
		fmt.Sprintf("Conflicts: %d", p.summary.conflicts),
	}
}

func summarize(files []git.FileStatus) summary {
	var s summary
	for _, file := range files {
		if file.StagedStatus == git.StatusConflict || file.WorktreeStatus == git.StatusConflict {
			s.conflicts++
			continue
		}
		if file.StagedStatus == git.StatusUntracked || file.WorktreeStatus == git.StatusUntracked {
			s.untracked++
			continue
		}
		if isChanged(file.StagedStatus) {
			s.staged++
		}
		if isChanged(file.WorktreeStatus) {
			s.unstaged++
		}
	}
	return s
}

func (s summary) dirty() bool {
	return s.staged+s.unstaged+s.untracked+s.conflicts > 0
}

func isChanged(code git.StatusCode) bool {
	if code == 0 {
		return false
	}
	return code != git.StatusUnmodified && code != git.StatusIgnored && code != git.StatusUntracked
}

type colors struct {
	Dim     string
	Success string
	Warning string
}

func themeColors(th *theme.Theme) colors {
	if th == nil {
		return colors{Dim: "#666666", Success: "#50FA7B", Warning: "#F1FA8C"}
	}
	return colors{
		Dim:     panels.OrDefault(th.Colors.BrightBlack, "#666666"),
		Success: panels.OrDefault(th.Colors.NotifySuccess, "#50FA7B"),
		Warning: panels.OrDefault(th.Colors.NotifyWarn, "#F1FA8C"),
	}
}

func trimLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(line)
	if len(runes) <= width {
		return line
	}
	return string(runes[:width])
}
