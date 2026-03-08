package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/git"
)

// SystemPromptBuilder constructs context-aware system prompts for the
// chat AI. The prompt is rebuilt on each message to reflect current
// repository state.
type SystemPromptBuilder struct {
	client   git.GitClient
	override string // User-provided custom prompt override
}

// NewSystemPromptBuilder creates a builder. If override is non-empty,
// it replaces the default prompt entirely.
func NewSystemPromptBuilder(client git.GitClient, override string) *SystemPromptBuilder {
	return &SystemPromptBuilder{client: client, override: override}
}

// Build generates the current system prompt with live repo context.
// If git operations fail, the prompt is still returned with placeholder
// values so the AI remains functional.
func (b *SystemPromptBuilder) Build(ctx context.Context) string {
	if b.override != "" {
		return b.override
	}

	repoRoot := "(unknown)"
	if root, err := b.client.RepoRoot(ctx); err == nil {
		repoRoot = root
	}

	branch := "(detached)"
	if branches, err := b.client.BranchList(ctx); err == nil {
		for _, br := range branches {
			if br.IsCurrent {
				// Sanitize branch name to prevent prompt injection via
				// malicious branch names (e.g. "feature/ignore-all-instructions").
				branch = ai.SanitizeBranchName(br.Name)
				break
			}
		}
	}

	statusSummary := "clean" //nolint:goconst // inline string is more readable here
	changeCount := 0
	if statuses, err := b.client.Status(ctx); err == nil {
		changeCount = len(statuses)
		if changeCount > 0 {
			statusSummary = buildStatusSummary(statuses)
		}
	}

	cleanOrChanges := "clean"
	if changeCount > 0 {
		cleanOrChanges = fmt.Sprintf("%d uncommitted changes", changeCount)
	}

	return fmt.Sprintf(`You are grut's AI assistant. You help users with git operations and file management through tool calls.

Available capabilities:
- File operations: read, write, delete, rename, list, mkdir
- Git operations: status, diff, log, blame, stage, unstage, commit, push, pull, fetch, checkout, branch, merge, rebase, stash, tag, reset
- Navigation: navigate to files/directories in the UI
- Search: find files by name pattern, search file contents

Current repository:
- Root: %s
- Branch: %s (%s)
- Status: %s

Rules:
- Use tools to perform actions — don't just describe what to do.
- Before destructive operations (delete, reset, force push), explain what will happen.
- Be concise in responses. Show results, not process.
- When multiple files are involved, use bulk operations when available.`, repoRoot, branch, cleanOrChanges, statusSummary)
}

// buildStatusSummary creates an abbreviated status string like
// "3 modified, 1 untracked" from a slice of file statuses.
func buildStatusSummary(statuses []git.FileStatus) string {
	var modified, added, deleted, untracked, renamed, conflicted int

	for _, s := range statuses {
		switch {
		case s.StagedStatus == git.StatusConflict || s.WorktreeStatus == git.StatusConflict:
			conflicted++
		case s.StagedStatus == git.StatusRenamed:
			renamed++
		case s.StagedStatus == git.StatusAdded:
			added++
		case s.StagedStatus == git.StatusDeleted || s.WorktreeStatus == git.StatusDeleted:
			deleted++
		case s.StagedStatus == git.StatusUntracked && s.WorktreeStatus == git.StatusUntracked:
			untracked++
		default:
			modified++
		}
	}

	var parts []string
	if modified > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", modified))
	}
	if added > 0 {
		parts = append(parts, fmt.Sprintf("%d added", added))
	}
	if deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", deleted))
	}
	if renamed > 0 {
		parts = append(parts, fmt.Sprintf("%d renamed", renamed))
	}
	if untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", untracked))
	}
	if conflicted > 0 {
		parts = append(parts, fmt.Sprintf("%d conflicted", conflicted))
	}

	if len(parts) == 0 {
		return "clean"
	}
	return strings.Join(parts, ", ")
}
