package chat

import (
	"context"
	"testing"

	"github.com/jongio/grut/internal/ai"
	"github.com/stretchr/testify/assert"
)

func TestIsBlockedEnvVar_BlocksKnownSecrets(t *testing.T) {
	blocked := []string{
		"ANTHROPIC_API_KEY",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"OPENAI_API_KEY",
		"AZURE_STORAGE_KEY",
		"AZURE_ACCOUNT_SECRET",
		"CUSTOM_SECRET",
		"MY_APP_API_KEY",
	}
	for _, key := range blocked {
		t.Run(key, func(t *testing.T) {
			assert.True(t, isBlockedEnvVar(key), "%q should be blocked", key)
		})
	}
}

func TestIsBlockedEnvVar_AllowsGitHubVars(t *testing.T) {
	allowed := []string{
		"GITHUB_TOKEN",
		"GITHUB_SECRET",
		"GH_TOKEN",
		"GH_SECRET",
		"GITHUB_API_KEY",
	}
	for _, key := range allowed {
		t.Run(key, func(t *testing.T) {
			assert.False(t, isBlockedEnvVar(key), "%q should NOT be blocked", key)
		})
	}
}

func TestIsBlockedEnvVar_AllowsStandardVars(t *testing.T) {
	allowed := []string{
		"PATH",
		"HOME",
		"SHELL",
		"USER",
		"LANG",
		"TERM",
		"GOPATH",
		"GOROOT",
	}
	for _, key := range allowed {
		t.Run(key, func(t *testing.T) {
			assert.False(t, isBlockedEnvVar(key), "%q should NOT be blocked", key)
		})
	}
}

func TestFilterEnvForGH_ReturnsFilteredList(t *testing.T) {
	// filterEnvForGH reads os.Environ() which we can't mock easily,
	// but we can verify it returns a non-empty list and that PATH is
	// preserved (it's always present).
	env := filterEnvForGH()
	assert.NotEmpty(t, env, "filtered env should not be empty")

	hasPath := false
	for _, kv := range env {
		if len(kv) >= 5 && kv[:5] == "PATH=" {
			hasPath = true
			break
		}
	}
	assert.True(t, hasPath, "PATH should be preserved in filtered env")
}

// ---------------------------------------------------------------------------
// isBlockedEnvVar — edge cases
// ---------------------------------------------------------------------------

func TestIsBlockedEnvVar_AzurePrefixEdgeCases(t *testing.T) {
	// AZURE_ variables are allowed unless they end in _KEY or _SECRET.
	assert.False(t, isBlockedEnvVar("AZURE_TENANT_ID"), "AZURE_TENANT_ID should NOT be blocked")
	assert.False(t, isBlockedEnvVar("AZURE_SUBSCRIPTION_ID"), "AZURE_SUBSCRIPTION_ID should NOT be blocked")
	assert.True(t, isBlockedEnvVar("AZURE_STORAGE_KEY"), "AZURE_STORAGE_KEY should be blocked")
	assert.True(t, isBlockedEnvVar("AZURE_CLIENT_SECRET"), "AZURE_CLIENT_SECRET should be blocked")
}

func TestIsBlockedEnvVar_CaseInsensitive(t *testing.T) {
	assert.True(t, isBlockedEnvVar("anthropic_api_key"), "lowercase should be blocked")
	assert.True(t, isBlockedEnvVar("Anthropic_Api_Key"), "mixed-case should be blocked")
	assert.True(t, isBlockedEnvVar("OPENAI_API_KEY"), "uppercase should be blocked")
	assert.True(t, isBlockedEnvVar("openai_api_key"), "lowercase should be blocked")
}

func TestIsBlockedEnvVar_SuffixMatchExcludesGitHub(t *testing.T) {
	// Suffix-based blocking should never block GITHUB_ or GH_ variables.
	assert.False(t, isBlockedEnvVar("GITHUB_SECRET"), "GITHUB_SECRET should NOT be blocked")
	assert.False(t, isBlockedEnvVar("GH_API_KEY"), "GH_API_KEY should NOT be blocked")
	// But non-GitHub variables with suffix should be blocked.
	assert.True(t, isBlockedEnvVar("MY_APP_SECRET"), "MY_APP_SECRET should be blocked")
	assert.True(t, isBlockedEnvVar("CUSTOM_API_KEY"), "CUSTOM_API_KEY should be blocked")
	assert.True(t, isBlockedEnvVar("DB_SECRET_KEY"), "DB_SECRET_KEY should be blocked")
}

// ---------------------------------------------------------------------------
// GitHub tool validation — input validation paths (no gh CLI required)
//
// These tests exercise early-return validation in each gh* handler.
// The handlers call ghExec() which requires the `gh` CLI, but the
// validation checks run BEFORE ghExec, so missing-argument errors are
// returned without ever spawning a child process.
// ---------------------------------------------------------------------------

func TestGHIssueView_MissingNumber(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "gh-1",
		Name:      "gh_issue_view",
		Arguments: map[string]any{},
	})
	assert.Contains(t, result.Error, "number is required")
}

func TestGHPRView_MissingNumber(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "gh-2",
		Name:      "gh_pr_view",
		Arguments: map[string]any{},
	})
	assert.Contains(t, result.Error, "number is required")
}

func TestGHPRDiff_MissingNumber(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "gh-3",
		Name:      "gh_pr_diff",
		Arguments: map[string]any{},
	})
	assert.Contains(t, result.Error, "number is required")
}

func TestGHActionsLogs_MissingRunID(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "gh-4",
		Name:      "gh_actions_logs",
		Arguments: map[string]any{},
	})
	assert.Contains(t, result.Error, "run_id is required")
}

func TestGHComment_MissingNumber(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "gh-5",
		Name:      "gh_comment",
		Arguments: map[string]any{"body": "test comment"},
	})
	assert.Contains(t, result.Error, "number is required")
}

func TestGHComment_MissingBody(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "gh-6",
		Name:      "gh_comment",
		Arguments: map[string]any{"number": float64(42)},
	})
	assert.Contains(t, result.Error, "body is required")
}

func TestGHPRReview_MissingNumber(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "gh-7",
		Name:      "gh_pr_review",
		Arguments: map[string]any{"body": "lgtm", "action": "approve"},
	})
	assert.Contains(t, result.Error, "number is required")
}

func TestGHPRReview_MissingBody(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "gh-8",
		Name:      "gh_pr_review",
		Arguments: map[string]any{"number": float64(10), "action": "approve"},
	})
	assert.Contains(t, result.Error, "body is required")
}

func TestGHPRReview_MissingAction(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "gh-9",
		Name:      "gh_pr_review",
		Arguments: map[string]any{"number": float64(10), "body": "lgtm"},
	})
	assert.Contains(t, result.Error, "action is required")
}

func TestGHPRReview_InvalidAction(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:   "gh-10",
		Name: "gh_pr_review",
		Arguments: map[string]any{
			"number": float64(10),
			"body":   "lgtm",
			"action": "merge",
		},
	})
	assert.Contains(t, result.Error, "action must be approve, request-changes, or comment")
}

func TestGHPRReview_ValidActions(t *testing.T) {
	// Verify valid actions pass validation (they'll fail at ghExec since
	// there's no gh CLI in the test env, but the error won't be a
	// validation error).
	validActions := []string{"approve", "request-changes", "comment"}
	for _, action := range validActions {
		t.Run(action, func(t *testing.T) {
			mock := &executorMockGitClient{}
			exec, _ := newTestExecutor(t, mock)

			result := exec.Execute(context.Background(), ai.ToolCall{
				ID:   "gh-valid",
				Name: "gh_pr_review",
				Arguments: map[string]any{
					"number": float64(10),
					"body":   "lgtm",
					"action": action,
				},
			})
			// Should NOT contain validation errors.
			assert.NotContains(t, result.Error, "action must be")
			assert.NotContains(t, result.Error, "number is required")
			assert.NotContains(t, result.Error, "body is required")
		})
	}
}

func TestGHActionsRerun_MissingRunID(t *testing.T) {
	mock := &executorMockGitClient{}
	exec, _ := newTestExecutor(t, mock)

	result := exec.Execute(context.Background(), ai.ToolCall{
		ID:        "gh-11",
		Name:      "gh_actions_rerun",
		Arguments: map[string]any{},
	})
	assert.Contains(t, result.Error, "run_id is required")
}
