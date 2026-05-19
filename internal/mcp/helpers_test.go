package mcp

import (
	"context"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// mockGitClient is a test double implementing git.GitClient.
// Each method field can be set to control the mock's behaviour.
// Unset methods return zero values.
type mockGitClient struct {
	StatusFunc         func(ctx context.Context) ([]git.FileStatus, error)
	DiffFunc           func(ctx context.Context, opts git.DiffOpts) ([]git.FileDiff, error)
	LogFunc            func(ctx context.Context, opts git.LogOpts) ([]git.Commit, error)
	BlameFunc          func(ctx context.Context, path string) ([]git.BlameLine, error)
	RepoRootFunc       func(ctx context.Context) (string, error)
	IsRepoFunc         func(ctx context.Context) (bool, error)
	StageFunc          func(ctx context.Context, paths []string) error
	UnstageFunc        func(ctx context.Context, paths []string) error
	StageHunkFunc      func(ctx context.Context, path string, hunk git.Hunk) error
	UnstageHunkFunc    func(ctx context.Context, path string, hunk git.Hunk) error
	StageLineFunc      func(ctx context.Context, path string, hunk git.Hunk, lineIdx int) error
	UnstageLineFunc    func(ctx context.Context, path string, hunk git.Hunk, lineIdx int) error
	CommitFunc         func(ctx context.Context, msg string, opts git.CommitOpts) (string, error)
	BranchListFunc     func(ctx context.Context) ([]git.Branch, error)
	BranchCreateFunc   func(ctx context.Context, name, base string) error
	BranchDeleteFunc   func(ctx context.Context, name string, force bool) error
	BranchRenameFunc   func(ctx context.Context, old, newName string) error
	CheckoutFunc       func(ctx context.Context, ref string) error
	PushFunc           func(ctx context.Context, opts git.PushOpts) error
	PullFunc           func(ctx context.Context, opts git.PullOpts) error
	FetchFunc          func(ctx context.Context, opts git.FetchOpts) error
	WorktreeListFunc   func(ctx context.Context) ([]git.Worktree, error)
	WorktreeAddFunc    func(ctx context.Context, path, branch string) error
	WorktreeRemoveFunc func(ctx context.Context, path string, force bool) error
	StashListFunc      func(ctx context.Context) ([]git.StashEntry, error)
	StashPushFunc      func(ctx context.Context, opts git.StashOpts) error
	StashPopFunc       func(ctx context.Context, index int) error
	StashApplyFunc     func(ctx context.Context, index int) error
	StashDropFunc      func(ctx context.Context, index int) error
	TagListFunc        func(ctx context.Context) ([]git.Tag, error)
	TagCreateFunc      func(ctx context.Context, name, ref, message string) error
	TagDeleteFunc      func(ctx context.Context, name string) error
	MergeFunc          func(ctx context.Context, branch string, opts git.MergeOpts) error
	MergeAbortFunc     func(ctx context.Context) error
	RebaseFunc         func(ctx context.Context, onto string, opts git.RebaseOpts) error
	RebaseContinueFunc func(ctx context.Context) error
	RebaseAbortFunc    func(ctx context.Context) error
	CherryPickFunc     func(ctx context.Context, hash string) error
	BisectStartFunc    func(ctx context.Context, bad, good string) error
	BisectGoodFunc     func(ctx context.Context) (string, error)
	BisectBadFunc      func(ctx context.Context) (string, error)
	BisectResetFunc    func(ctx context.Context) error
	ReflogFunc         func(ctx context.Context, ref string, limit int) ([]git.ReflogEntry, error)
}

// Compile-time check that mockGitClient implements git.GitClient.
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

func (m *mockGitClient) IsRepo(ctx context.Context) (bool, error) {
	if m.IsRepoFunc != nil {
		return m.IsRepoFunc(ctx)
	}
	return true, nil
}

func (m *mockGitClient) DiffTreeFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (m *mockGitClient) DiffFileNames(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

func (m *mockGitClient) Stage(ctx context.Context, paths []string) error {
	if m.StageFunc != nil {
		return m.StageFunc(ctx, paths)
	}
	return nil
}

func (m *mockGitClient) Unstage(ctx context.Context, paths []string) error {
	if m.UnstageFunc != nil {
		return m.UnstageFunc(ctx, paths)
	}
	return nil
}

func (m *mockGitClient) StageHunk(ctx context.Context, path string, hunk git.Hunk) error {
	if m.StageHunkFunc != nil {
		return m.StageHunkFunc(ctx, path, hunk)
	}
	return nil
}

func (m *mockGitClient) UnstageHunk(ctx context.Context, path string, hunk git.Hunk) error {
	if m.UnstageHunkFunc != nil {
		return m.UnstageHunkFunc(ctx, path, hunk)
	}
	return nil
}

func (m *mockGitClient) StageLine(ctx context.Context, path string, hunk git.Hunk, lineIdx int) error {
	if m.StageLineFunc != nil {
		return m.StageLineFunc(ctx, path, hunk, lineIdx)
	}
	return nil
}

func (m *mockGitClient) UnstageLine(ctx context.Context, path string, hunk git.Hunk, lineIdx int) error {
	if m.UnstageLineFunc != nil {
		return m.UnstageLineFunc(ctx, path, hunk, lineIdx)
	}
	return nil
}

func (m *mockGitClient) Commit(ctx context.Context, msg string, opts git.CommitOpts) (string, error) {
	if m.CommitFunc != nil {
		return m.CommitFunc(ctx, msg, opts)
	}
	return "abc123", nil
}

func (m *mockGitClient) BranchList(ctx context.Context) ([]git.Branch, error) {
	if m.BranchListFunc != nil {
		return m.BranchListFunc(ctx)
	}
	return nil, nil
}

func (m *mockGitClient) CurrentBranch(_ context.Context) (git.Branch, error) {
	return git.Branch{IsCurrent: true}, nil
}

func (m *mockGitClient) BranchCreate(ctx context.Context, name, base string) error {
	if m.BranchCreateFunc != nil {
		return m.BranchCreateFunc(ctx, name, base)
	}
	return nil
}

func (m *mockGitClient) BranchDelete(ctx context.Context, name string, force bool) error {
	if m.BranchDeleteFunc != nil {
		return m.BranchDeleteFunc(ctx, name, force)
	}
	return nil
}

func (m *mockGitClient) BranchRename(ctx context.Context, old, newName string) error {
	if m.BranchRenameFunc != nil {
		return m.BranchRenameFunc(ctx, old, newName)
	}
	return nil
}

func (m *mockGitClient) Checkout(ctx context.Context, ref string) error {
	if m.CheckoutFunc != nil {
		return m.CheckoutFunc(ctx, ref)
	}
	return nil
}

func (m *mockGitClient) Push(ctx context.Context, opts git.PushOpts) error {
	if m.PushFunc != nil {
		return m.PushFunc(ctx, opts)
	}
	return nil
}

func (m *mockGitClient) Pull(ctx context.Context, opts git.PullOpts) error {
	if m.PullFunc != nil {
		return m.PullFunc(ctx, opts)
	}
	return nil
}

func (m *mockGitClient) Fetch(ctx context.Context, opts git.FetchOpts) error {
	if m.FetchFunc != nil {
		return m.FetchFunc(ctx, opts)
	}
	return nil
}

func (m *mockGitClient) WorktreeList(ctx context.Context) ([]git.Worktree, error) {
	if m.WorktreeListFunc != nil {
		return m.WorktreeListFunc(ctx)
	}
	return nil, nil
}

func (m *mockGitClient) WorktreeAdd(ctx context.Context, path, branch string) error {
	if m.WorktreeAddFunc != nil {
		return m.WorktreeAddFunc(ctx, path, branch)
	}
	return nil
}

func (m *mockGitClient) WorktreeRemove(ctx context.Context, path string, force bool) error {
	if m.WorktreeRemoveFunc != nil {
		return m.WorktreeRemoveFunc(ctx, path, force)
	}
	return nil
}

func (m *mockGitClient) StashList(ctx context.Context) ([]git.StashEntry, error) {
	if m.StashListFunc != nil {
		return m.StashListFunc(ctx)
	}
	return nil, nil
}

func (m *mockGitClient) StashShow(_ context.Context, _ int) (string, error) {
	return "", nil
}

func (m *mockGitClient) StashPush(ctx context.Context, opts git.StashOpts) error {
	if m.StashPushFunc != nil {
		return m.StashPushFunc(ctx, opts)
	}
	return nil
}

func (m *mockGitClient) StashPop(ctx context.Context, index int) error {
	if m.StashPopFunc != nil {
		return m.StashPopFunc(ctx, index)
	}
	return nil
}

func (m *mockGitClient) StashApply(ctx context.Context, index int) error {
	if m.StashApplyFunc != nil {
		return m.StashApplyFunc(ctx, index)
	}
	return nil
}

func (m *mockGitClient) StashDrop(ctx context.Context, index int) error {
	if m.StashDropFunc != nil {
		return m.StashDropFunc(ctx, index)
	}
	return nil
}

func (m *mockGitClient) TagList(ctx context.Context) ([]git.Tag, error) {
	if m.TagListFunc != nil {
		return m.TagListFunc(ctx)
	}
	return nil, nil
}

func (m *mockGitClient) TagCreate(ctx context.Context, name, ref, message string) error {
	if m.TagCreateFunc != nil {
		return m.TagCreateFunc(ctx, name, ref, message)
	}
	return nil
}

func (m *mockGitClient) TagDelete(ctx context.Context, name string) error {
	if m.TagDeleteFunc != nil {
		return m.TagDeleteFunc(ctx, name)
	}
	return nil
}

func (m *mockGitClient) TagListRemote(_ context.Context, _ string) ([]git.Tag, error) {
	return nil, nil
}

func (m *mockGitClient) TagPush(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockGitClient) TagPushAll(_ context.Context, _ string) error {
	return nil
}

func (m *mockGitClient) Merge(ctx context.Context, branch string, opts git.MergeOpts) error {
	if m.MergeFunc != nil {
		return m.MergeFunc(ctx, branch, opts)
	}
	return nil
}

func (m *mockGitClient) MergeAbort(ctx context.Context) error {
	if m.MergeAbortFunc != nil {
		return m.MergeAbortFunc(ctx)
	}
	return nil
}

func (m *mockGitClient) Rebase(ctx context.Context, onto string, opts git.RebaseOpts) error {
	if m.RebaseFunc != nil {
		return m.RebaseFunc(ctx, onto, opts)
	}
	return nil
}

func (m *mockGitClient) RebaseContinue(ctx context.Context) error {
	if m.RebaseContinueFunc != nil {
		return m.RebaseContinueFunc(ctx)
	}
	return nil
}

func (m *mockGitClient) RebaseAbort(ctx context.Context) error {
	if m.RebaseAbortFunc != nil {
		return m.RebaseAbortFunc(ctx)
	}
	return nil
}

func (m *mockGitClient) CherryPick(ctx context.Context, hash string) error {
	if m.CherryPickFunc != nil {
		return m.CherryPickFunc(ctx, hash)
	}
	return nil
}

func (m *mockGitClient) BisectStart(ctx context.Context, bad, good string) error {
	if m.BisectStartFunc != nil {
		return m.BisectStartFunc(ctx, bad, good)
	}
	return nil
}

func (m *mockGitClient) BisectGood(ctx context.Context) (string, error) {
	if m.BisectGoodFunc != nil {
		return m.BisectGoodFunc(ctx)
	}
	return "bisect good result", nil
}

func (m *mockGitClient) BisectBad(ctx context.Context) (string, error) {
	if m.BisectBadFunc != nil {
		return m.BisectBadFunc(ctx)
	}
	return "bisect bad result", nil
}

func (m *mockGitClient) BisectReset(ctx context.Context) error {
	if m.BisectResetFunc != nil {
		return m.BisectResetFunc(ctx)
	}
	return nil
}

func (m *mockGitClient) Reflog(ctx context.Context, ref string, limit int) ([]git.ReflogEntry, error) {
	if m.ReflogFunc != nil {
		return m.ReflogFunc(ctx, ref, limit)
	}
	return nil, nil
}

func (m *mockGitClient) RemoteList(ctx context.Context) ([]git.Remote, error) {
	return nil, nil
}

func (m *mockGitClient) RemoteAdd(ctx context.Context, name, url string) error {
	return nil
}

func (m *mockGitClient) RemoteRemove(ctx context.Context, name string) error {
	return nil
}

func (m *mockGitClient) DiscardFile(ctx context.Context, path string) error           { return nil }
func (m *mockGitClient) DiscardAllUnstaged(ctx context.Context) error                 { return nil }
func (m *mockGitClient) Revert(ctx context.Context, hash string) error                { return nil }
func (m *mockGitClient) RevertContinue(ctx context.Context) error                     { return nil }
func (m *mockGitClient) RevertAbort(ctx context.Context) error                        { return nil }
func (m *mockGitClient) Reset(ctx context.Context, ref string, _ git.ResetMode) error { return nil }

// stubConfig returns a minimal config suitable for tests.
func stubConfig() *config.Config {
	return &config.Config{
		MCP: config.MCPConfig{
			Security: stubSecurityConfig(),
		},
	}
}

// stubSecurityConfig returns a security config with audit logging disabled
// and generous rate limits for tests.
func stubSecurityConfig() config.MCPSecurityConfig {
	return config.MCPSecurityConfig{
		RateLimitRead:  10000,
		RateLimitWrite: 10000,
		AuditLog:       false,
	}
}

// newTestServer creates an MCP Server with a mock git client for testing.
// The repo root is set to the given directory.
func newTestServer(t *testing.T, mock *mockGitClient, root string) *Server {
	t.Helper()
	cfg := stubConfig()
	srv, err := NewServer(mock, root, cfg)
	require.NoError(t, err)
	return srv
}

// callTool is a test helper that directly invokes a registered tool handler.
// It builds a CallToolRequest with the given arguments and calls the handler
// returned by the server's MCPServer.
func callTool(t *testing.T, srv *Server, toolName string, args map[string]any) *mcplib.CallToolResult {
	t.Helper()
	ctx := context.Background()

	st := srv.mcp.GetTool(toolName)
	require.NotNil(t, st, "tool %q should be registered", toolName)

	req := mcplib.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = args

	result, err := st.Handler(ctx, req)
	require.NoError(t, err, "tool handler should not return protocol error")
	require.NotNil(t, result, "result should not be nil")
	return result
}

// resultText extracts the text content from a CallToolResult.
func resultText(t *testing.T, result *mcplib.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content, "result should have content")
	for _, c := range result.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatal("no text content in result")
	return ""
}
