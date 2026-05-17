//go:build screenshots

package gitinfo

// InjectDemoGitHubData populates the panel with realistic fake GitHub data
// for screenshot captures. This method is only compiled with the screenshots
// build tag and avoids relying on a live GitHub API connection.
func (p *Panel) InjectDemoGitHubData() {
	// Clear any authentication error so the tab renders data instead of
	// "GitHub unavailable".
	p.gh.err = nil
	p.gh.user = "demo-user"
	p.gh.owner = "jongio"
	p.gh.repo = "grut"
	p.gh.repoPrivate = true

	issues := []ghIssueItem{
		{
			Number:  42,
			Title:   "Add keyboard shortcut customization",
			State:   "open",
			Labels:  []string{"enhancement"},
			Author:  "alice",
			HTMLURL: "https://github.com/jongio/grut/issues/42",
		},
		{
			Number:  38,
			Title:   "File preview doesn't render markdown tables",
			State:   "open",
			Labels:  []string{"bug"},
			Author:  "bob",
			HTMLURL: "https://github.com/jongio/grut/issues/38",
		},
		{
			Number:  35,
			Title:   "Support .gitignore syntax highlighting",
			State:   "open",
			Labels:  []string{"enhancement"},
			Author:  "carol",
			HTMLURL: "https://github.com/jongio/grut/issues/35",
		},
		{
			Number:   31,
			Title:    "Crash when opening binary files > 10MB",
			State:    "open",
			Labels:   []string{"bug", "priority/high"},
			Author:   "dave",
			Assignee: "alice",
			HTMLURL:  "https://github.com/jongio/grut/issues/31",
		},
		{
			Number:  28,
			Title:   "Add tree-sitter based syntax highlighting",
			State:   "open",
			Labels:  []string{"enhancement"},
			Author:  "alice",
			HTMLURL: "https://github.com/jongio/grut/issues/28",
		},
	}

	prs := []ghPRItem{
		{
			Number:     44,
			Title:      "feat: add worktree merge strategy options",
			State:      "open",
			HeadBranch: "feature/worktree-merge",
			Author:     "alice",
			HTMLURL:    "https://github.com/jongio/grut/pull/44",
		},
		{
			Number:     41,
			Title:      "fix: resolve stash apply conflict handling",
			State:      "open",
			HeadBranch: "fix/stash-conflicts",
			Author:     "bob",
			HTMLURL:    "https://github.com/jongio/grut/pull/41",
		},
		{
			Number:     39,
			Title:      "docs: update keybinding reference for v0.3",
			State:      "open",
			HeadBranch: "docs/keybindings",
			Author:     "carol",
			HTMLURL:    "https://github.com/jongio/grut/pull/39",
		},
	}

	actions := []ghActionItem{
		{
			RunID:        900156,
			WorkflowName: "CI",
			RunNumber:    156,
			Status:       "completed",
			Conclusion:   "success",
			Branch:       "main",
			CreatedAt:    "Jun 8 14:32",
			HTMLURL:      "https://github.com/jongio/grut/actions/runs/900156",
		},
		{
			RunID:        900155,
			WorkflowName: "CI",
			RunNumber:    155,
			Status:       "completed",
			Conclusion:   "success",
			Branch:       "feature/worktree",
			CreatedAt:    "Jun 8 11:05",
			HTMLURL:      "https://github.com/jongio/grut/actions/runs/900155",
		},
		{
			RunID:        900012,
			WorkflowName: "Release",
			RunNumber:    12,
			Status:       "completed",
			Conclusion:   "success",
			Branch:       "v0.3.0",
			CreatedAt:    "Jun 7 09:20",
			HTMLURL:      "https://github.com/jongio/grut/actions/runs/900012",
		},
		{
			RunID:        900154,
			WorkflowName: "CI",
			RunNumber:    154,
			Status:       "in_progress",
			Conclusion:   "",
			Branch:       "fix/stash-conflicts",
			CreatedAt:    "Jun 8 15:01",
			HTMLURL:      "https://github.com/jongio/grut/actions/runs/900154",
		},
	}

	workflows := []ghWorkflowItem{
		{
			ID:      1001,
			Name:    "CI",
			Path:    ".github/workflows/ci.yml",
			State:   "active",
			HTMLURL: "https://github.com/jongio/grut/actions/workflows/ci.yml",
		},
		{
			ID:      1002,
			Name:    "Release",
			Path:    ".github/workflows/release.yml",
			State:   "active",
			HTMLURL: "https://github.com/jongio/grut/actions/workflows/release.yml",
		},
	}

	releases := []ghReleaseItem{
		{
			ID:          3001,
			TagName:     "v0.3.0",
			Name:        "v0.3.0",
			Author:      "jongio",
			CreatedAt:   "Jun 7 2025",
			Body:        "Added worktree management, improved diff viewer, stash operations",
			AssetsCount: 6,
			HTMLURL:     "https://github.com/jongio/grut/releases/tag/v0.3.0",
		},
		{
			ID:          2001,
			TagName:     "v0.2.0",
			Name:        "v0.2.0",
			Author:      "jongio",
			CreatedAt:   "May 15 2025",
			Body:        "Added fuzzy finder, bookmarks panel, keyboard shortcuts",
			AssetsCount: 6,
			HTMLURL:     "https://github.com/jongio/grut/releases/tag/v0.2.0",
		},
		{
			ID:          1001,
			TagName:     "v0.1.0",
			Name:        "v0.1.0",
			Author:      "jongio",
			CreatedAt:   "Apr 20 2025",
			Body:        "Initial release with file explorer and git integration",
			AssetsCount: 6,
			HTMLURL:     "https://github.com/jongio/grut/releases/tag/v0.1.0",
		},
	}

	p.buildGitHubItems(issues, prs, actions, workflows, releases)
}
