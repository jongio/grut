package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/ai/middleware"
	"github.com/jongio/grut/internal/ai/ops"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock AI provider (black-box — reimplemented in _test package)
// ---------------------------------------------------------------------------

// mockProvider implements ai.AIProvider for integration tests.
type mockProvider struct {
	name         string
	available    bool
	completeResp ai.CompletionResponse
	completeErr  error
	// capturedReq stores the last CompletionRequest sent to Complete.
	capturedReq *ai.CompletionRequest
	// streamChunks are delivered when CompleteStream is called.
	streamChunks []ai.StreamChunk
}

var _ ai.AIProvider = (*mockProvider)(nil)

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Available(_ context.Context) (bool, error) {
	return m.available, nil
}

func (m *mockProvider) Complete(_ context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	m.capturedReq = &req
	return m.completeResp, m.completeErr
}

func (m *mockProvider) CompleteStream(_ context.Context, req ai.CompletionRequest) (<-chan ai.StreamChunk, error) {
	m.capturedReq = &req
	if m.completeErr != nil {
		return nil, m.completeErr
	}
	ch := make(chan ai.StreamChunk, len(m.streamChunks)+1)
	for _, c := range m.streamChunks {
		ch <- c
	}
	// Always end with a Done chunk.
	ch <- ai.StreamChunk{Done: true}
	close(ch)
	return ch, nil
}

func (m *mockProvider) Close() error { return nil }

// ---------------------------------------------------------------------------
// Mock git client (black-box — mirrors helpers_test.go pattern)
// ---------------------------------------------------------------------------

type mockGitClient struct {
	branches     []git.Branch
	StatusFunc   func(ctx context.Context) ([]git.FileStatus, error)
	DiffFunc     func(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error)
	LogFunc      func(ctx context.Context, opts git.LogOpts) ([]git.Commit, error)
	BlameFunc    func(ctx context.Context, path string) ([]git.BlameLine, error)
	RepoRootFunc func(ctx context.Context) (string, error)
	CommitFunc   func(ctx context.Context, msg string, opts git.CommitOpts) (string, error)
}

var _ git.GitClient = (*mockGitClient)(nil)

func (m *mockGitClient) Status(ctx context.Context) ([]git.FileStatus, error) {
	if m.StatusFunc != nil {
		return m.StatusFunc(ctx)
	}
	return nil, nil
}

func (m *mockGitClient) Diff(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
	if m.DiffFunc != nil {
		return m.DiffFunc(ctx, opts)
	}
	return nil, nil
}

func (m *mockGitClient) Log(ctx context.Context, opts git.LogOpts) ([]git.Commit, error) {
	if m.LogFunc != nil {
		return m.LogFunc(ctx, opts)
	}
	return nil, nil
}

func (m *mockGitClient) Blame(ctx context.Context, path string) ([]git.BlameLine, error) {
	if m.BlameFunc != nil {
		return m.BlameFunc(ctx, path)
	}
	return nil, nil
}

func (m *mockGitClient) RepoRoot(ctx context.Context) (string, error) {
	if m.RepoRootFunc != nil {
		return m.RepoRootFunc(ctx)
	}
	return "", nil
}

func (m *mockGitClient) IsRepo(_ context.Context) (bool, error) { return true, nil }
func (m *mockGitClient) DiffTreeFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (m *mockGitClient) DiffFileNames(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}
func (m *mockGitClient) Stage(_ context.Context, _ []string) error                        { return nil }
func (m *mockGitClient) Unstage(_ context.Context, _ []string) error                      { return nil }
func (m *mockGitClient) StageHunk(_ context.Context, _ string, _ git.Hunk) error          { return nil }
func (m *mockGitClient) UnstageHunk(_ context.Context, _ string, _ git.Hunk) error        { return nil }
func (m *mockGitClient) StageLine(_ context.Context, _ string, _ git.Hunk, _ int) error   { return nil }
func (m *mockGitClient) UnstageLine(_ context.Context, _ string, _ git.Hunk, _ int) error { return nil }
func (m *mockGitClient) Commit(ctx context.Context, msg string, opts git.CommitOpts) (string, error) {
	if m.CommitFunc != nil {
		return m.CommitFunc(ctx, msg, opts)
	}
	return "abc123", nil
}

func (m *mockGitClient) BranchList(_ context.Context) ([]git.Branch, error) {
	return m.branches, nil
}
func (m *mockGitClient) BranchCreate(_ context.Context, _, _ string) error      { return nil }
func (m *mockGitClient) BranchDelete(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockGitClient) BranchRename(_ context.Context, _, _ string) error      { return nil }
func (m *mockGitClient) Checkout(_ context.Context, _ string) error             { return nil }
func (m *mockGitClient) Push(_ context.Context, _ git.PushOpts) error           { return nil }
func (m *mockGitClient) Pull(_ context.Context, _ git.PullOpts) error           { return nil }
func (m *mockGitClient) Fetch(_ context.Context, _ git.FetchOpts) error         { return nil }
func (m *mockGitClient) WorktreeList(_ context.Context) ([]git.Worktree, error) { return nil, nil }
func (m *mockGitClient) WorktreeAdd(_ context.Context, _, _ string) error       { return nil }
func (m *mockGitClient) WorktreeRemove(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockGitClient) StashList(_ context.Context) ([]git.StashEntry, error) { return nil, nil }
func (m *mockGitClient) StashShow(_ context.Context, _ int) (string, error)    { return "", nil }
func (m *mockGitClient) StashPush(_ context.Context, _ git.StashOpts) error    { return nil }
func (m *mockGitClient) StashPop(_ context.Context, _ int) error               { return nil }
func (m *mockGitClient) StashApply(_ context.Context, _ int) error             { return nil }
func (m *mockGitClient) StashDrop(_ context.Context, _ int) error              { return nil }
func (m *mockGitClient) TagList(_ context.Context) ([]git.Tag, error)          { return nil, nil }
func (m *mockGitClient) TagCreate(_ context.Context, _, _, _ string) error     { return nil }
func (m *mockGitClient) TagDelete(_ context.Context, _ string) error           { return nil }
func (m *mockGitClient) TagListRemote(_ context.Context, _ string) ([]git.Tag, error) {
	return nil, nil
}
func (m *mockGitClient) TagPush(_ context.Context, _, _ string) error { return nil }
func (m *mockGitClient) TagPushAll(_ context.Context, _ string) error { return nil }
func (m *mockGitClient) Merge(_ context.Context, _ string, _ git.MergeOpts) error {
	return nil
}
func (m *mockGitClient) MergeAbort(_ context.Context) error { return nil }
func (m *mockGitClient) Rebase(_ context.Context, _ string, _ git.RebaseOpts) error {
	return nil
}
func (m *mockGitClient) RebaseContinue(_ context.Context) error           { return nil }
func (m *mockGitClient) RebaseAbort(_ context.Context) error              { return nil }
func (m *mockGitClient) CherryPick(_ context.Context, _ string) error     { return nil }
func (m *mockGitClient) BisectStart(_ context.Context, _, _ string) error { return nil }
func (m *mockGitClient) BisectGood(_ context.Context) (string, error)     { return "", nil }
func (m *mockGitClient) BisectBad(_ context.Context) (string, error)      { return "", nil }
func (m *mockGitClient) BisectReset(_ context.Context) error              { return nil }
func (m *mockGitClient) Reflog(_ context.Context, _ string, _ int) ([]git.ReflogEntry, error) {
	return nil, nil
}

func (m *mockGitClient) RemoteList(_ context.Context) ([]git.Remote, error)       { return nil, nil }
func (m *mockGitClient) RemoteAdd(_ context.Context, _, _ string) error           { return nil }
func (m *mockGitClient) RemoteRemove(_ context.Context, _ string) error           { return nil }
func (m *mockGitClient) DiscardFile(_ context.Context, _ string) error            { return nil }
func (m *mockGitClient) DiscardAllUnstaged(_ context.Context) error               { return nil }
func (m *mockGitClient) Revert(_ context.Context, _ string) error                 { return nil }
func (m *mockGitClient) RevertContinue(_ context.Context) error                   { return nil }
func (m *mockGitClient) RevertAbort(_ context.Context) error                      { return nil }
func (m *mockGitClient) Reset(_ context.Context, _ string, _ git.ResetMode) error { return nil }

// newMockGitClient builds a mock with a current branch, repo root, and
// staged diff ready for commit message generation.
func newMockGitClient(branch, repoRoot string) *mockGitClient {
	return &mockGitClient{
		branches: []git.Branch{{Name: branch, IsCurrent: true}},
		RepoRootFunc: func(_ context.Context) (string, error) {
			return repoRoot, nil
		},
	}
}

// ---------------------------------------------------------------------------
// Helper: set up a registry with a mock provider.
// ---------------------------------------------------------------------------

func setupRegistry(provider *mockProvider, primary string) *ai.Registry {
	cfg := config.AIConfig{
		Enabled:  true,
		Provider: primary,
	}
	reg := ai.NewRegistry(cfg)
	reg.Register(provider.Name(), provider)
	return reg
}

// ==========================================================================
// Integration tests
// ==========================================================================

// ---------------------------------------------------------------------------
// 1. Registry → Provider → Ops: Commit message generation
// ---------------------------------------------------------------------------

func TestIntegration_Registry_Provider_CommitGenerate(t *testing.T) {
	t.Parallel()

	commitJSON := `{"type":"feat","scope":"auth","subject":"add login endpoint","body":"supports OAuth2 flow"}`
	provider := &mockProvider{
		name:      "test-provider",
		available: true,
		completeResp: ai.CompletionResponse{
			Content:      commitJSON,
			FinishReason: "stop",
			TokensUsed:   ai.TokenUsage{InputTokens: 100, OutputTokens: 50},
		},
	}

	reg := setupRegistry(provider, "test-provider")
	gitClient := newMockGitClient("main", "/repo")
	gitClient.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		if opts.Staged {
			return []git.FileDiff{
				{Path: "auth/handler.go", Hunks: []git.Hunk{{Header: "@@ -0,0 +1,10 @@"}}},
			}, nil
		}
		return nil, nil
	}

	builder := ai.NewBuilder(gitClient, nil, 0) // no redaction, unlimited tokens
	gen := ops.NewCommitGenerator(reg, builder)

	suggestion, err := gen.Generate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, suggestion)

	assert.Equal(t, "feat", suggestion.Type)
	assert.Equal(t, "auth", suggestion.Scope)
	assert.Equal(t, "add login endpoint", suggestion.Subject)
	assert.Equal(t, "supports OAuth2 flow", suggestion.Body)
	assert.Equal(t, "feat(auth): add login endpoint\n\nsupports OAuth2 flow", suggestion.String())

	// Verify the provider received the right operation and context.
	require.NotNil(t, provider.capturedReq)
	assert.Equal(t, "commit_message", provider.capturedReq.Operation)
	assert.NotEmpty(t, provider.capturedReq.SystemPrompt)
	assert.Equal(t, "main", provider.capturedReq.GitContext.CurrentBranch)
	assert.Len(t, provider.capturedReq.GitContext.Diffs, 1)
}

// ---------------------------------------------------------------------------
// 2. Registry → Provider → Ops: Code review
// ---------------------------------------------------------------------------

func TestIntegration_Registry_Provider_Review(t *testing.T) {
	t.Parallel()

	findingsJSON := `[
		{"file":"main.go","line":42,"severity":"error","category":"security","message":"SQL injection via string concatenation","suggestion":"Use parameterized queries"},
		{"file":"main.go","line":10,"severity":"info","category":"style","message":"Consider renaming variable","suggestion":""}
	]`

	provider := &mockProvider{
		name:      "review-provider",
		available: true,
		completeResp: ai.CompletionResponse{
			Content: findingsJSON,
		},
	}

	reg := setupRegistry(provider, "review-provider")
	gitClient := newMockGitClient("feature/review", "/repo")
	gitClient.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return []git.FileDiff{
			{Path: "main.go", Hunks: []git.Hunk{{
				Header: "@@ -1,5 +1,10 @@",
				Lines:  []git.DiffLine{{Type: git.DiffLineAdded, Content: "new code"}},
			}}},
		}, nil
	}

	builder := ai.NewBuilder(gitClient, nil, 0)
	reviewer := ops.NewReviewer(reg, builder)

	findings, err := reviewer.Review(context.Background(), git.DiffOpts{Staged: true})
	require.NoError(t, err)
	require.Len(t, findings, 2)

	// Findings are sorted by severity: error before info.
	assert.Equal(t, "error", findings[0].Severity)
	assert.Equal(t, "security", findings[0].Category)
	assert.Equal(t, "info", findings[1].Severity)

	// Provider received the code_review operation.
	require.NotNil(t, provider.capturedReq)
	assert.Equal(t, "code_review", provider.capturedReq.Operation)
}

// ---------------------------------------------------------------------------
// 3. Context Builder → Redaction → Provider: Secrets stripped before AI
// ---------------------------------------------------------------------------

func TestIntegration_ContextBuilder_Redaction_Provider(t *testing.T) {
	t.Parallel()

	// Create a temp dir with a file containing a secret.
	tmpDir := t.TempDir()
	secretFile := filepath.Join(tmpDir, "config.go")
	secretContent := `package config
const APIKey = "AKIA1234567890ABCDEF"
const Host = "example.com"
`
	require.NoError(t, os.WriteFile(secretFile, []byte(secretContent), 0o644))

	// Also create a .env file that should be excluded entirely.
	envFile := filepath.Join(tmpDir, ".env")
	require.NoError(t, os.WriteFile(envFile, []byte("SECRET=hunter2"), 0o644))

	provider := &mockProvider{
		name:      "redact-provider",
		available: true,
		completeResp: ai.CompletionResponse{
			Content: `{"type":"chore","subject":"update config"}`,
		},
	}

	reg := setupRegistry(provider, "redact-provider")
	redactor := ai.NewRedactor(nil) // built-in patterns only

	gitClient := newMockGitClient("main", tmpDir)
	gitClient.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		if opts.Staged {
			return []git.FileDiff{
				{Path: "config.go", Hunks: []git.Hunk{{
					Header: "@@ -1,3 +1,3 @@",
					Lines: []git.DiffLine{
						{Type: git.DiffLineAdded, Content: `const APIKey = "AKIA1234567890ABCDEF"`},
					},
				}}},
				{Path: ".env", Hunks: []git.Hunk{{
					Header: "@@ -0,0 +1 @@",
					Lines:  []git.DiffLine{{Type: git.DiffLineAdded, Content: "SECRET=hunter2"}},
				}}},
			}, nil
		}
		return nil, nil
	}

	builder := ai.NewBuilder(gitClient, redactor, 0)
	gen := ops.NewCommitGenerator(reg, builder)

	_, err := gen.Generate(context.Background())
	require.NoError(t, err)

	// The provider should NOT have received the .env file in its diffs.
	require.NotNil(t, provider.capturedReq)
	for _, d := range provider.capturedReq.GitContext.Diffs {
		assert.NotEqual(t, ".env", d.Path,
			"redactor should exclude .env files from diffs sent to provider")
	}
}

// ---------------------------------------------------------------------------
// 4. Redactor: ShouldExcludeFile + RedactContent integration
// ---------------------------------------------------------------------------

func TestIntegration_Redactor_FileExclusion_And_ContentRedaction(t *testing.T) {
	t.Parallel()

	redactor := ai.NewRedactor([]string{"*.credentials"})

	// Built-in file exclusions.
	assert.True(t, redactor.ShouldExcludeFile(".env"))
	assert.True(t, redactor.ShouldExcludeFile("config/.env.local"))
	assert.True(t, redactor.ShouldExcludeFile("server.key"))
	assert.True(t, redactor.ShouldExcludeFile("id_rsa"))
	assert.True(t, redactor.ShouldExcludeFile("cert.pem"))

	// User-supplied pattern.
	assert.True(t, redactor.ShouldExcludeFile("db.credentials"))

	// Non-sensitive files pass through.
	assert.False(t, redactor.ShouldExcludeFile("main.go"))
	assert.False(t, redactor.ShouldExcludeFile("README.md"))

	// Content redaction: AWS key.
	redacted, count := redactor.RedactContent(`token = "AKIA1234567890ABCDEF"`)
	assert.Greater(t, count, 0)
	assert.Contains(t, redacted, ai.RedactedPlaceholder)
	assert.NotContains(t, redacted, "AKIA1234567890ABCDEF")

	// Content redaction: GitHub token.
	redacted, count = redactor.RedactContent(`GH_TOKEN=ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn`)
	assert.Greater(t, count, 0)
	assert.Contains(t, redacted, ai.RedactedPlaceholder)

	// Content redaction: PEM private key block.
	pemContent := "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA0Z...\n-----END RSA PRIVATE KEY-----"
	redacted, count = redactor.RedactContent(pemContent)
	assert.Greater(t, count, 0)
	assert.NotContains(t, redacted, "BEGIN RSA PRIVATE KEY")

	// Clean content passes through unchanged.
	cleanContent := "func main() { fmt.Println(\"hello\") }"
	redacted, count = redactor.RedactContent(cleanContent)
	assert.Equal(t, 0, count)
	assert.Equal(t, cleanContent, redacted)
}

// ---------------------------------------------------------------------------
// 5. Registry: Primary → fallback resolution
// ---------------------------------------------------------------------------

func TestIntegration_Registry_PrimaryFallback(t *testing.T) {
	t.Parallel()

	primary := &mockProvider{name: "primary", available: false}
	fallback := &mockProvider{name: "fallback", available: true}

	cfg := config.AIConfig{
		Enabled:          true,
		Provider:         "primary",
		FallbackProvider: "fallback",
	}
	reg := ai.NewRegistry(cfg)
	reg.Register("primary", primary)
	reg.Register("fallback", fallback)

	ctx := context.Background()

	// Primary is unavailable → should get fallback.
	got, err := reg.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, "fallback", got.Name())

	// Make primary available → should get primary.
	primary.available = true
	got, err = reg.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, "primary", got.Name())
}

// ---------------------------------------------------------------------------
// 6. Registry: No available providers → error
// ---------------------------------------------------------------------------

func TestIntegration_Registry_NoAvailableProvider(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{name: "offline", available: false}
	reg := setupRegistry(provider, "offline")

	_, err := reg.Get(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no available AI provider")
}

// ---------------------------------------------------------------------------
// 7. Registry: GetByName retrieves specific provider
// ---------------------------------------------------------------------------

func TestIntegration_Registry_GetByName(t *testing.T) {
	t.Parallel()

	p1 := &mockProvider{name: "alpha", available: true}
	p2 := &mockProvider{name: "beta", available: true}

	cfg := config.AIConfig{Provider: "alpha"}
	reg := ai.NewRegistry(cfg)
	reg.Register("alpha", p1)
	reg.Register("beta", p2)

	got, ok := reg.GetByName("beta")
	assert.True(t, ok)
	assert.Equal(t, "beta", got.Name())

	_, ok = reg.GetByName("gamma")
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// 8. Audit Logger integration: provider calls produce log entries
// ---------------------------------------------------------------------------

func TestIntegration_AuditLogger_ProducesEntries(t *testing.T) {
	t.Parallel()

	logPath := filepath.Join(t.TempDir(), "audit.log")
	logger, err := ai.NewAuditLogger(logPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = logger.Close() })

	// Simulate a series of AI operations.
	entries := []ai.AuditEntry{
		{
			Timestamp:  time.Now(),
			Operation:  "commit_message",
			Provider:   "test-provider",
			FilesSent:  []string{"main.go", "auth.go"},
			Redactions: 2,
			TokensIn:   500,
			TokensOut:  100,
			Result:     "accepted",
		},
		{
			Timestamp: time.Now(),
			Operation: "code_review",
			Provider:  "test-provider",
			TokensIn:  1000,
			TokensOut: 200,
			Result:    "accepted",
		},
		{
			Timestamp: time.Now(),
			Operation: "chat",
			Provider:  "test-provider",
			Result:    "error",
			Error:     "context cancelled",
		},
	}

	for _, e := range entries {
		require.NoError(t, logger.Log(e))
	}

	// Read the log file and verify entries.
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 3, "expected 3 log entries")

	// Parse the first entry and verify structure.
	var parsed ai.AuditEntry
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &parsed))
	assert.Equal(t, "commit_message", parsed.Operation)
	assert.Equal(t, "test-provider", parsed.Provider)
	assert.Equal(t, []string{"main.go", "auth.go"}, parsed.FilesSent)
	assert.Equal(t, 2, parsed.Redactions)
	assert.Equal(t, 500, parsed.TokensIn)
	assert.Equal(t, 100, parsed.TokensOut)
	assert.Equal(t, "accepted", parsed.Result)

	// Parse the error entry and verify error field.
	require.NoError(t, json.Unmarshal([]byte(lines[2]), &parsed))
	assert.Equal(t, "error", parsed.Result)
	assert.Equal(t, "context cancelled", parsed.Error)
}

// ---------------------------------------------------------------------------
// 9. Context Builder → ForCommit: builds correct context
// ---------------------------------------------------------------------------

func TestIntegration_ContextBuilder_ForCommit(t *testing.T) {
	t.Parallel()

	gitClient := newMockGitClient("feature/login", "/repo")
	gitClient.StatusFunc = func(_ context.Context) ([]git.FileStatus, error) {
		return []git.FileStatus{
			{Path: "auth.go", StagedStatus: git.StatusAdded},
		}, nil
	}
	gitClient.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		if opts.Staged {
			return []git.FileDiff{
				{Path: "auth.go", Hunks: []git.Hunk{{Header: "@@ -0,0 +1,20 @@"}}},
			}, nil
		}
		return nil, nil
	}
	gitClient.LogFunc = func(_ context.Context, _ git.LogOpts) ([]git.Commit, error) {
		return []git.Commit{
			{Hash: "abc123", Subject: "feat: initial commit"},
			{Hash: "def456", Subject: "fix: typo"},
		}, nil
	}

	builder := ai.NewBuilder(gitClient, nil, 0)
	ctx, err := builder.ForCommit(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "feature/login", ctx.CurrentBranch)
	assert.Equal(t, "/repo", ctx.RepoRoot)
	assert.Len(t, ctx.Diffs, 1)
	assert.Equal(t, "auth.go", ctx.Diffs[0].Path)
	assert.Len(t, ctx.Log, 2)
	assert.Len(t, ctx.Status, 1)
}

// ---------------------------------------------------------------------------
// 10. Context Builder with token budget: truncation
// ---------------------------------------------------------------------------

func TestIntegration_ContextBuilder_TokenBudget(t *testing.T) {
	t.Parallel()

	gitClient := newMockGitClient("main", "/repo")

	// Return a large diff that exceeds a small token budget.
	bigContent := strings.Repeat("x", 1000) // ~250 tokens
	gitClient.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		if opts.Staged {
			return []git.FileDiff{
				{Path: "big.go", Hunks: []git.Hunk{{
					Header: "@@ -0,0 +1,100 @@",
					Lines:  []git.DiffLine{{Type: git.DiffLineAdded, Content: bigContent}},
				}}},
			}, nil
		}
		return nil, nil
	}
	gitClient.LogFunc = func(_ context.Context, _ git.LogOpts) ([]git.Commit, error) {
		var commits []git.Commit
		for i := 0; i < 50; i++ {
			commits = append(commits, git.Commit{
				Hash:    fmt.Sprintf("hash%d", i),
				Subject: fmt.Sprintf("commit %d with a reasonably long subject line for testing", i),
			})
		}
		return commits, nil
	}

	// Set a very small budget: only the diff should fit (approx).
	builder := ai.NewBuilder(gitClient, nil, 300)
	ctx, err := builder.ForCommit(context.Background())
	require.NoError(t, err)

	// If diff fits, it should be present. Log should be truncated.
	assert.Equal(t, "main", ctx.CurrentBranch)
	// The log should have fewer than 50 entries if budget is tight.
	assert.Less(t, len(ctx.Log), 50,
		"token budget should truncate the commit log")
}

// ---------------------------------------------------------------------------
// 11. Middleware: AIGitClient commit interception with AutoCommitMsg
// ---------------------------------------------------------------------------

func TestIntegration_Middleware_CommitInterception(t *testing.T) {
	t.Parallel()

	commitJSON := `{"type":"fix","subject":"correct null check"}`
	provider := &mockProvider{
		name:      "middleware-provider",
		available: true,
		completeResp: ai.CompletionResponse{
			Content: commitJSON,
		},
	}

	var committedMsg string
	inner := newMockGitClient("main", "/repo")
	inner.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		if opts.Staged {
			return []git.FileDiff{
				{Path: "handler.go", Hunks: []git.Hunk{{Header: "@@ -10,3 +10,5 @@"}}},
			}, nil
		}
		return nil, nil
	}
	inner.CommitFunc = func(_ context.Context, msg string, _ git.CommitOpts) (string, error) {
		committedMsg = msg
		return "sha-1234", nil
	}

	reg := setupRegistry(provider, "middleware-provider")
	builder := ai.NewBuilder(inner, nil, 0)

	aiCfg := config.AIConfig{
		AutoCommitMsg: true,
	}

	aiClient := middleware.NewAIGitClient(inner, reg, builder, nil, aiCfg)

	// Commit with empty message → should trigger AI generation.
	hash, err := aiClient.Commit(context.Background(), "", git.CommitOpts{})
	require.NoError(t, err)
	assert.Equal(t, "sha-1234", hash)

	// The commit message should have been auto-generated.
	assert.Contains(t, committedMsg, "fix:")
	assert.Contains(t, committedMsg, "correct null check")
}

// ---------------------------------------------------------------------------
// 12. Middleware: Pass-through when AutoCommitMsg is off
// ---------------------------------------------------------------------------

func TestIntegration_Middleware_NoInterception_WhenDisabled(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{
		name:      "noop-provider",
		available: true,
	}

	var committedMsg string
	inner := newMockGitClient("main", "/repo")
	inner.CommitFunc = func(_ context.Context, msg string, _ git.CommitOpts) (string, error) {
		committedMsg = msg
		return "sha-5678", nil
	}

	reg := setupRegistry(provider, "noop-provider")
	builder := ai.NewBuilder(inner, nil, 0)
	aiCfg := config.AIConfig{AutoCommitMsg: false}

	aiClient := middleware.NewAIGitClient(inner, reg, builder, nil, aiCfg)

	hash, err := aiClient.Commit(context.Background(), "manual: my commit", git.CommitOpts{})
	require.NoError(t, err)
	assert.Equal(t, "sha-5678", hash)
	assert.Equal(t, "manual: my commit", committedMsg)
	assert.Nil(t, provider.capturedReq, "provider should NOT be called when AutoCommitMsg is off")
}

// ---------------------------------------------------------------------------
// 13. Middleware: AI failure does not block commit
// ---------------------------------------------------------------------------

func TestIntegration_Middleware_AIFailure_DoesNotBlockCommit(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{
		name:        "failing-provider",
		available:   true,
		completeErr: fmt.Errorf("AI service unavailable"),
	}

	var committedMsg string
	inner := newMockGitClient("main", "/repo")
	inner.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		if opts.Staged {
			return []git.FileDiff{{Path: "a.go"}}, nil
		}
		return nil, nil
	}
	inner.CommitFunc = func(_ context.Context, msg string, _ git.CommitOpts) (string, error) {
		committedMsg = msg
		return "sha-9999", nil
	}

	reg := setupRegistry(provider, "failing-provider")
	builder := ai.NewBuilder(inner, nil, 0)
	aiCfg := config.AIConfig{AutoCommitMsg: true}

	aiClient := middleware.NewAIGitClient(inner, reg, builder, nil, aiCfg)

	// Commit should succeed even though AI fails.
	hash, err := aiClient.Commit(context.Background(), "", git.CommitOpts{})
	require.NoError(t, err)
	assert.Equal(t, "sha-9999", hash)
	// The message stays empty because AI failed — the commit still goes through.
	assert.Equal(t, "", committedMsg)
}

// ---------------------------------------------------------------------------
// 14. Middleware: ReviewDiff integration through AIGitClient
// ---------------------------------------------------------------------------

func TestIntegration_Middleware_ReviewDiff(t *testing.T) {
	t.Parallel()

	findingsJSON := `[{"file":"api.go","line":5,"severity":"warning","category":"bug","message":"unused variable","suggestion":"remove it"}]`
	provider := &mockProvider{
		name:      "review-mid-provider",
		available: true,
		completeResp: ai.CompletionResponse{
			Content: findingsJSON,
		},
	}

	inner := newMockGitClient("develop", "/repo")
	inner.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return []git.FileDiff{
			{Path: "api.go", Hunks: []git.Hunk{{
				Header: "@@ -1,5 +1,8 @@",
				Lines:  []git.DiffLine{{Type: git.DiffLineAdded, Content: "new code"}},
			}}},
		}, nil
	}

	reg := setupRegistry(provider, "review-mid-provider")
	builder := ai.NewBuilder(inner, nil, 0)
	aiClient := middleware.NewAIGitClient(inner, reg, builder, nil, config.AIConfig{})

	findings, err := aiClient.ReviewDiff(context.Background(), git.DiffOpts{Staged: true})
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "api.go", findings[0].File)
	assert.Equal(t, "warning", findings[0].Severity)
	assert.Equal(t, "bug", findings[0].Category)
}

// ---------------------------------------------------------------------------
// 15. Middleware + Audit: operations produce audit entries
// ---------------------------------------------------------------------------

func TestIntegration_Middleware_AuditLogging(t *testing.T) {
	t.Parallel()

	logPath := filepath.Join(t.TempDir(), "mw-audit.log")
	logger, err := ai.NewAuditLogger(logPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = logger.Close() })

	commitJSON := `{"type":"chore","subject":"update deps"}`
	provider := &mockProvider{
		name:      "audit-provider",
		available: true,
		completeResp: ai.CompletionResponse{
			Content: commitJSON,
		},
	}

	inner := newMockGitClient("main", "/repo")
	inner.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		if opts.Staged {
			return []git.FileDiff{{Path: "go.mod", Hunks: []git.Hunk{{Header: "@@"}}}}, nil
		}
		return nil, nil
	}
	inner.CommitFunc = func(_ context.Context, _ string, _ git.CommitOpts) (string, error) {
		return "sha-audit", nil
	}

	reg := setupRegistry(provider, "audit-provider")
	builder := ai.NewBuilder(inner, nil, 0)
	aiCfg := config.AIConfig{AutoCommitMsg: true}

	aiClient := middleware.NewAIGitClient(inner, reg, builder, logger, aiCfg)

	_, err = aiClient.Commit(context.Background(), "", git.CommitOpts{})
	require.NoError(t, err)

	// Close the logger to flush and release the file.
	require.NoError(t, logger.Close())

	// Read audit log and verify at least one entry was written.
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.NotEmpty(t, lines, "audit log should contain at least one entry")

	var entry ai.AuditEntry
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &entry))
	assert.Equal(t, "commit_message", entry.Operation)
	assert.Equal(t, "accepted", entry.Result)
}

// ---------------------------------------------------------------------------
// 16. Config → Registry → Provider setup integration
// ---------------------------------------------------------------------------

func TestIntegration_Config_Registry_ProviderSetup(t *testing.T) {
	t.Parallel()

	// Simulate loading a config with AI settings.
	cfg := config.AIConfig{
		Enabled:          true,
		Provider:         "copilot",
		FallbackProvider: "claude",
		RedactPatterns:   []string{"*.secrets.yaml"},
		AutoCommitMsg:    true,
		AutoReviewDiff:   true,
		Temperature:      0.3,
		MaxContextTokens: 8000,
	}

	// Create the registry from config.
	reg := ai.NewRegistry(cfg)

	// Register mock providers matching the config names.
	copilot := &mockProvider{name: "copilot", available: true}
	claude := &mockProvider{name: "claude", available: true}
	reg.Register("copilot", copilot)
	reg.Register("claude", claude)

	// Primary should be preferred.
	got, err := reg.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "copilot", got.Name())

	// Verify redaction patterns can be used with the config.
	redactor := ai.NewRedactor(cfg.RedactPatterns)
	assert.True(t, redactor.ShouldExcludeFile("app.secrets.yaml"))
	assert.False(t, redactor.ShouldExcludeFile("app.config.yaml"))

	// Build a context builder with the configured token budget.
	gitClient := newMockGitClient("main", "/repo")
	builder := ai.NewBuilder(gitClient, redactor, cfg.MaxContextTokens)

	// Verify the builder works with the configured budget.
	gitCtx, err := builder.ForCommit(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "main", gitCtx.CurrentBranch)

	// Registry cleanup.
	require.NoError(t, reg.Close())
}

// ---------------------------------------------------------------------------
// 17. Context Builder → ForReview: diffs + file contents + log
// ---------------------------------------------------------------------------

func TestIntegration_ContextBuilder_ForReview(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, "handler.go"),
		[]byte("package main\nfunc Handle() {}"),
		0o644,
	))

	gitClient := newMockGitClient("review-branch", tmpDir)
	gitClient.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return []git.FileDiff{
			{Path: "handler.go", Hunks: []git.Hunk{{Header: "@@ -1,2 +1,3 @@"}}},
		}, nil
	}
	gitClient.LogFunc = func(_ context.Context, _ git.LogOpts) ([]git.Commit, error) {
		return []git.Commit{
			{Hash: "aaa", Subject: "feat: add handler"},
		}, nil
	}
	gitClient.StatusFunc = func(_ context.Context) ([]git.FileStatus, error) {
		return []git.FileStatus{
			{Path: "handler.go", StagedStatus: git.StatusModified},
		}, nil
	}

	builder := ai.NewBuilder(gitClient, nil, 0)
	gitCtx, err := builder.ForReview(context.Background(), git.DiffOpts{Staged: true})
	require.NoError(t, err)

	assert.Equal(t, "review-branch", gitCtx.CurrentBranch)
	assert.Len(t, gitCtx.Diffs, 1)
	assert.Contains(t, gitCtx.FileContents, "handler.go")
	assert.Contains(t, gitCtx.FileContents["handler.go"], "func Handle()")
	assert.Len(t, gitCtx.Log, 1)
	assert.Len(t, gitCtx.Status, 1)
}

// ---------------------------------------------------------------------------
// 18. End-to-end: Redaction → Context → Commit → Audit
// ---------------------------------------------------------------------------

func TestIntegration_EndToEnd_Redaction_Context_Commit_Audit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Write a source file with a secret embedded.
	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, "db.go"),
		[]byte(`package db
const connStr = "postgres://user:s3cret_pass@localhost/mydb"
func Connect() {}
`),
		0o644,
	))

	logPath := filepath.Join(tmpDir, "audit.log")
	logger, err := ai.NewAuditLogger(logPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = logger.Close() })

	commitJSON := `{"type":"fix","scope":"db","subject":"fix connection pooling"}`
	provider := &mockProvider{
		name:      "e2e-provider",
		available: true,
		completeResp: ai.CompletionResponse{
			Content:    commitJSON,
			TokensUsed: ai.TokenUsage{InputTokens: 200, OutputTokens: 40},
		},
	}

	var committedMsg string
	inner := newMockGitClient("fix/pool", tmpDir)
	inner.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		if opts.Staged {
			return []git.FileDiff{
				{Path: "db.go", Hunks: []git.Hunk{{
					Header: "@@ -1,3 +1,4 @@",
					Lines: []git.DiffLine{
						{Type: git.DiffLineAdded, Content: `const connStr = "postgres://user:s3cret_pass@localhost/mydb"`},
					},
				}}},
			}, nil
		}
		return nil, nil
	}
	inner.CommitFunc = func(_ context.Context, msg string, _ git.CommitOpts) (string, error) {
		committedMsg = msg
		return "sha-e2e", nil
	}

	redactor := ai.NewRedactor(nil)
	reg := setupRegistry(provider, "e2e-provider")
	builder := ai.NewBuilder(inner, redactor, 0)
	aiCfg := config.AIConfig{AutoCommitMsg: true}

	aiClient := middleware.NewAIGitClient(inner, reg, builder, logger, aiCfg)

	hash, err := aiClient.Commit(context.Background(), "", git.CommitOpts{})
	require.NoError(t, err)
	assert.Equal(t, "sha-e2e", hash)
	assert.Contains(t, committedMsg, "fix(db):")

	// Close the logger to flush.
	require.NoError(t, logger.Close())

	// Verify audit log was written.
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.NotEmpty(t, string(data))

	var entry ai.AuditEntry
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.NotEmpty(t, lines)
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &entry))
	assert.Equal(t, "commit_message", entry.Operation)
}

// ---------------------------------------------------------------------------
// 19. Registry Close: all providers closed
// ---------------------------------------------------------------------------

func TestIntegration_Registry_Close(t *testing.T) {
	t.Parallel()

	p1 := &mockProvider{name: "a", available: true}
	p2 := &mockProvider{name: "b", available: true}

	cfg := config.AIConfig{Provider: "a"}
	reg := ai.NewRegistry(cfg)
	reg.Register("a", p1)
	reg.Register("b", p2)

	// Close should not error for well-behaved providers.
	require.NoError(t, reg.Close())
}

// ---------------------------------------------------------------------------
// 20. Ops: empty diff returns nil suggestion (no AI call)
// ---------------------------------------------------------------------------

func TestIntegration_Ops_EmptyDiff_NoAICall(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{
		name:      "nocall-provider",
		available: true,
		completeResp: ai.CompletionResponse{
			Content: `{"type":"fix","subject":"should not be called"}`,
		},
	}

	gitClient := newMockGitClient("main", "/repo")
	// No staged diffs.
	gitClient.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return nil, nil
	}

	reg := setupRegistry(provider, "nocall-provider")
	builder := ai.NewBuilder(gitClient, nil, 0)
	gen := ops.NewCommitGenerator(reg, builder)

	suggestion, err := gen.Generate(context.Background())
	require.NoError(t, err)
	assert.Nil(t, suggestion, "empty diff should return nil suggestion")
	assert.Nil(t, provider.capturedReq, "provider should NOT be called for empty diffs")
}

// ---------------------------------------------------------------------------
// 21. Ops: review with empty diff returns nil findings
// ---------------------------------------------------------------------------

func TestIntegration_Ops_EmptyDiff_NoReview(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{
		name:      "empty-review",
		available: true,
	}

	gitClient := newMockGitClient("main", "/repo")
	gitClient.DiffFunc = func(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
		return nil, nil
	}

	reg := setupRegistry(provider, "empty-review")
	builder := ai.NewBuilder(gitClient, nil, 0)
	reviewer := ops.NewReviewer(reg, builder)

	findings, err := reviewer.Review(context.Background(), git.DiffOpts{Staged: true})
	require.NoError(t, err)
	assert.Empty(t, findings)
	assert.Nil(t, provider.capturedReq)
}

// ---------------------------------------------------------------------------
// 22. Middleware: StatusReader methods pass through unchanged
// ---------------------------------------------------------------------------

func TestIntegration_Middleware_PassThrough(t *testing.T) {
	t.Parallel()

	inner := newMockGitClient("main", "/repo")
	inner.StatusFunc = func(_ context.Context) ([]git.FileStatus, error) {
		return []git.FileStatus{{Path: "foo.go", StagedStatus: git.StatusModified}}, nil
	}

	reg := setupRegistry(&mockProvider{name: "x", available: true}, "x")
	builder := ai.NewBuilder(inner, nil, 0)
	aiClient := middleware.NewAIGitClient(inner, reg, builder, nil, config.AIConfig{})

	ctx := context.Background()

	// Status passes through.
	statuses, err := aiClient.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, "foo.go", statuses[0].Path)

	// RepoRoot passes through.
	root, err := aiClient.RepoRoot(ctx)
	require.NoError(t, err)
	assert.Equal(t, "/repo", root)

	// IsRepo passes through.
	isRepo, err := aiClient.IsRepo(ctx)
	require.NoError(t, err)
	assert.True(t, isRepo)

	// BranchList passes through.
	branches, err := aiClient.BranchList(ctx)
	require.NoError(t, err)
	require.Len(t, branches, 1)
	assert.Equal(t, "main", branches[0].Name)
}

// ---------------------------------------------------------------------------
// 23. Commit response with markdown code fences
// ---------------------------------------------------------------------------

func TestIntegration_Ops_CommitResponse_WithCodeFences(t *testing.T) {
	t.Parallel()

	// Some AI models wrap their JSON in markdown fences.
	fencedJSON := "```json\n{\"type\":\"refactor\",\"subject\":\"extract helper\"}\n```"
	provider := &mockProvider{
		name:      "fence-provider",
		available: true,
		completeResp: ai.CompletionResponse{
			Content: fencedJSON,
		},
	}

	gitClient := newMockGitClient("main", "/repo")
	gitClient.DiffFunc = func(_ context.Context, opts git.DiffOpts) ([]git.FileDiff, error) {
		if opts.Staged {
			return []git.FileDiff{{Path: "util.go", Hunks: []git.Hunk{{Header: "@@"}}}}, nil
		}
		return nil, nil
	}

	reg := setupRegistry(provider, "fence-provider")
	builder := ai.NewBuilder(gitClient, nil, 0)
	gen := ops.NewCommitGenerator(reg, builder)

	suggestion, err := gen.Generate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, suggestion)
	assert.Equal(t, "refactor", suggestion.Type)
	assert.Equal(t, "extract helper", suggestion.Subject)
}

// ---------------------------------------------------------------------------
// 24. Context Builder → ForChat: lightweight (no diffs, no file contents)
// ---------------------------------------------------------------------------

func TestIntegration_ContextBuilder_ForChat(t *testing.T) {
	t.Parallel()

	gitClient := newMockGitClient("main", "/repo")
	gitClient.StatusFunc = func(_ context.Context) ([]git.FileStatus, error) {
		return []git.FileStatus{
			{Path: "a.go", StagedStatus: git.StatusModified},
			{Path: "b.go", WorktreeStatus: git.StatusUntracked},
		}, nil
	}

	builder := ai.NewBuilder(gitClient, nil, 0)
	gitCtx, err := builder.ForChat(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "main", gitCtx.CurrentBranch)
	assert.Equal(t, "/repo", gitCtx.RepoRoot)
	assert.Len(t, gitCtx.Status, 2)
	// ForChat does NOT populate diffs, file contents, or log.
	assert.Nil(t, gitCtx.Diffs)
	assert.Nil(t, gitCtx.FileContents)
	assert.Nil(t, gitCtx.Log)
}
