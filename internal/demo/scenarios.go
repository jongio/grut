package demo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jongio/grut/internal/proctree"
)

const (
	scenarioListName           = "list"
	scenarioBranchReview       = "branch-review"
	scenarioConflictResolution = "conflict-resolution"
	scenarioExtensions         = "extensions"
)

// Scenario describes a guided demo walkthrough and its initial UI state.
type Scenario struct {
	Name        string
	Description string
	Layout      string
	FocusPanel  string
	Guide       []string
	setup       func(string) error
}

// ScenarioSetup describes the project path and selected scenario returned by
// SetupProjectWithOptions.
type ScenarioSetup struct {
	Dir       string
	Scenario  *Scenario
	GuidePath string
}

// SetupOptions controls demo project creation.
type SetupOptions struct {
	Scenario string
	Keep     bool
}

// Scenarios returns the guided demo scenarios in CLI display order.
func Scenarios() []Scenario {
	return []Scenario{
		{
			Name:        scenarioBranchReview,
			Description: "Review branches, commits, changed files, and stashes in a seeded repository.",
			Layout:      "explorer",
			FocusPanel:  "gitinfo",
			Guide: []string{
				"Goal: learn how grut connects branch, commit, file, and preview panels.",
				"1. Inspect the git info panel for local branches and tags.",
				"2. Select feature/websocket-support, then review its commits and changed files.",
				"3. Open src/api/websocket.go in preview and compare it with main.",
				"4. Check the stash list for the three deterministic WIP entries.",
			},
		},
		{
			Name:        scenarioConflictResolution,
			Description: "Open a repository already in a merge conflict for practicing conflict review.",
			Layout:      "full",
			FocusPanel:  "gitstatus",
			Guide: []string{
				"Goal: practice finding and resolving a real merge conflict.",
				"1. Start in the git status panel and find src/middleware/ratelimit.go.",
				"2. Open the conflicted file in preview and inspect both sides of the merge.",
				"3. Use the file tree and preview together to edit or discard conflict markers.",
				"4. Stage the resolved file when the conflict is fixed.",
			},
			setup: setupConflictResolutionScenario,
		},
		{
			Name:        scenarioExtensions,
			Description: "Explore the sample .grut extension and configuration files.",
			Layout:      "explorer",
			FocusPanel:  "filetree",
			Guide: []string{
				"Goal: discover how grut loads local configuration and extensions.",
				"1. Expand .grut/ and open config.toml.",
				"2. Open .grut/extensions/hello.lua to inspect the sample Lua extension.",
				"3. Use the command palette to find extension-related commands.",
				"4. Edit the sample extension, then review the unstaged diff.",
			},
		},
	}
}

// LookupScenario returns a scenario by name.
func LookupScenario(name string) (Scenario, bool) {
	trimmed := strings.TrimSpace(name)
	for _, scenario := range Scenarios() {
		if scenario.Name == trimmed {
			return scenario, true
		}
	}
	return Scenario{}, false
}

// IsScenarioListName reports whether name requests scenario listing.
func IsScenarioListName(name string) bool {
	return strings.TrimSpace(name) == scenarioListName
}

// FormatScenarioList renders all scenarios with one-line descriptions.
func FormatScenarioList() string {
	var b strings.Builder
	b.WriteString("Available demo scenarios:\n")
	for _, scenario := range Scenarios() {
		fmt.Fprintf(&b, "  %-20s %s\n", scenario.Name, scenario.Description)
	}
	return b.String()
}

func applyScenario(dir string, name string) (*ScenarioSetup, error) {
	if strings.TrimSpace(name) == "" {
		return &ScenarioSetup{Dir: dir}, nil
	}
	scenario, ok := LookupScenario(name)
	if !ok {
		return nil, fmt.Errorf("unknown demo scenario %q (run grut --demo --scenario list)", name)
	}
	if scenario.setup != nil {
		if err := scenario.setup(dir); err != nil {
			return nil, fmt.Errorf("set up scenario %q: %w", scenario.Name, err)
		}
	}
	guidePath, err := writeScenarioGuide(dir, scenario)
	if err != nil {
		return nil, err
	}
	return &ScenarioSetup{Dir: dir, Scenario: &scenario, GuidePath: guidePath}, nil
}

func writeScenarioGuide(dir string, scenario Scenario) (string, error) {
	guidePath := filepath.Join(dir, ".grut", "demo-scenario.md")
	var b strings.Builder
	fmt.Fprintf(&b, "# grut demo: %s\n\n", scenario.Name)
	b.WriteString(scenario.Description)
	b.WriteString("\n\n")
	b.WriteString("## Guided steps\n\n")
	for _, step := range scenario.Guide {
		b.WriteString("- ")
		b.WriteString(step)
		b.WriteString("\n")
	}
	b.WriteString("\n## Reproduce\n\n")
	fmt.Fprintf(&b, "Run `grut --demo --scenario %s --demo-keep` to keep this repository after exit.\n", scenario.Name)
	if err := os.WriteFile(guidePath, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("write scenario guide: %w", err)
	}
	return guidePath, nil
}

func setupConflictResolutionScenario(dir string) error {
	if err := runDemoGit(dir, "reset", "--hard"); err != nil {
		return fmt.Errorf("reset working tree: %w", err)
	}
	mergeOut, err := demoGitOutput(dir, "merge", "fix/rate-limit-bypass")
	if err == nil {
		return fmt.Errorf("expected merge conflict but merge succeeded: %s", strings.TrimSpace(mergeOut))
	}
	status, statusErr := demoGitOutput(dir, "status", "--porcelain")
	if statusErr != nil {
		return statusErr
	}
	if !strings.Contains(status, "UU src/middleware/ratelimit.go") {
		return fmt.Errorf("expected ratelimit.go conflict; merge output: %q; status: %q",
			strings.TrimSpace(mergeOut), strings.TrimSpace(status))
	}
	return nil
}

// demoGitEnv returns the process environment with a demo git identity so git
// commands that create commits (e.g. a clean merge) succeed on machines and CI
// runners that have no global user.name/user.email configured.
func demoGitEnv() []string {
	return append(
		os.Environ(),
		"GIT_AUTHOR_NAME=grut demo",
		"GIT_AUTHOR_EMAIL=demo@grut.dev",
		"GIT_COMMITTER_NAME=grut demo",
		"GIT_COMMITTER_EMAIL=demo@grut.dev",
	)
}

func runDemoGit(dir string, args ...string) error {
	cmd := proctree.Command(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = demoGitEnv()
	return proctree.Run(cmd)
}

func demoGitOutput(dir string, args ...string) (string, error) {
	cmd := proctree.Command(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = demoGitEnv()
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := proctree.Run(cmd)
	return out.String(), err
}
