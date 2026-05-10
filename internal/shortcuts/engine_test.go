package shortcuts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/git"
)

// mockGitClient implements git.GitClient for testing. It records calls and
// can be configured to fail on specific operations.
type mockGitClient struct {
	calls    []string
	failOps  map[string]error
	branches []git.Branch
}

func newMockGitClient() *mockGitClient {
	return &mockGitClient{
		failOps: make(map[string]error),
		branches: []git.Branch{
			{Name: "main", IsCurrent: true},
			{Name: "feature-a", IsCurrent: false},
		},
	}
}

func (m *mockGitClient) record(op string) error {
	m.calls = append(m.calls, op)
	if err, ok := m.failOps[op]; ok {
		return err
	}
	return nil
}

func (m *mockGitClient) Status(_ context.Context) ([]git.FileStatus, error) {
	return nil, m.record("status")
}

func (m *mockGitClient) Diff(_ context.Context, _ git.DiffOpts) ([]git.FileDiff, error) {
	return nil, m.record("diff")
}

func (m *mockGitClient) Log(_ context.Context, _ git.LogOpts) ([]git.Commit, error) {
	return nil, m.record("log")
}

func (m *mockGitClient) Blame(_ context.Context, _ string) ([]git.BlameLine, error) {
	return nil, m.record("blame")
}

func (m *mockGitClient) RepoRoot(_ context.Context) (string, error) {
	return "/repo", m.record("repo_root")
}
func (m *mockGitClient) IsRepo(_ context.Context) (bool, error) { return true, m.record("is_repo") }
func (m *mockGitClient) DiffTreeFiles(_ context.Context, _ string) ([]string, error) {
	return nil, m.record("diff_tree_files")
}

func (m *mockGitClient) DiffFileNames(_ context.Context, _, _ string) ([]string, error) {
	return nil, m.record("diff_file_names")
}

func (m *mockGitClient) Stage(_ context.Context, _ []string) error   { return m.record("stage") }
func (m *mockGitClient) Unstage(_ context.Context, _ []string) error { return m.record("unstage") }
func (m *mockGitClient) StageHunk(_ context.Context, _ string, _ git.Hunk) error {
	return m.record("stage_hunk")
}

func (m *mockGitClient) UnstageHunk(_ context.Context, _ string, _ git.Hunk) error {
	return m.record("unstage_hunk")
}

func (m *mockGitClient) StageLine(_ context.Context, _ string, _ git.Hunk, _ int) error {
	return m.record("stage_line")
}

func (m *mockGitClient) UnstageLine(_ context.Context, _ string, _ git.Hunk, _ int) error {
	return m.record("unstage_line")
}

func (m *mockGitClient) Commit(_ context.Context, _ string, _ git.CommitOpts) (string, error) {
	return "abc1234", m.record("commit")
}

func (m *mockGitClient) BranchList(_ context.Context) ([]git.Branch, error) {
	return m.branches, m.record("branch_list")
}

func (m *mockGitClient) BranchCreate(_ context.Context, _, _ string) error {
	return m.record("branch_create")
}

func (m *mockGitClient) BranchDelete(_ context.Context, _ string, _ bool) error {
	return m.record("branch_delete")
}

func (m *mockGitClient) BranchRename(_ context.Context, _, _ string) error {
	return m.record("branch_rename")
}
func (m *mockGitClient) Checkout(_ context.Context, _ string) error { return m.record("checkout") }

func (m *mockGitClient) Push(_ context.Context, _ git.PushOpts) error   { return m.record("push") }
func (m *mockGitClient) Pull(_ context.Context, _ git.PullOpts) error   { return m.record("pull") }
func (m *mockGitClient) Fetch(_ context.Context, _ git.FetchOpts) error { return m.record("fetch") }

func (m *mockGitClient) WorktreeList(_ context.Context) ([]git.Worktree, error) {
	return nil, m.record("worktree_list")
}

func (m *mockGitClient) WorktreeAdd(_ context.Context, _, _ string) error {
	return m.record("worktree_add")
}

func (m *mockGitClient) WorktreeRemove(_ context.Context, _ string, _ bool) error {
	return m.record("worktree_remove")
}

func (m *mockGitClient) StashList(_ context.Context) ([]git.StashEntry, error) {
	return nil, m.record("stash_list")
}

func (m *mockGitClient) StashShow(_ context.Context, _ int) (string, error) {
	return "", m.record("stash_show")
}

func (m *mockGitClient) StashPush(_ context.Context, _ git.StashOpts) error {
	return m.record("stash_push")
}
func (m *mockGitClient) StashPop(_ context.Context, _ int) error   { return m.record("stash_pop") }
func (m *mockGitClient) StashApply(_ context.Context, _ int) error { return m.record("stash_apply") }
func (m *mockGitClient) StashDrop(_ context.Context, _ int) error  { return m.record("stash_drop") }

func (m *mockGitClient) TagList(_ context.Context) ([]git.Tag, error) {
	return nil, m.record("tag_list")
}

func (m *mockGitClient) TagCreate(_ context.Context, _, _, _ string) error {
	return m.record("tag_create")
}

func (m *mockGitClient) TagDelete(_ context.Context, _ string) error { return m.record("tag_delete") }

func (m *mockGitClient) TagListRemote(_ context.Context, _ string) ([]git.Tag, error) {
	return nil, m.record("tag_list_remote")
}

func (m *mockGitClient) TagPush(_ context.Context, _, _ string) error { return m.record("tag_push") }

func (m *mockGitClient) TagPushAll(_ context.Context, _ string) error {
	return m.record("tag_push_all")
}

func (m *mockGitClient) Merge(_ context.Context, _ string, _ git.MergeOpts) error {
	return m.record("merge")
}
func (m *mockGitClient) MergeAbort(_ context.Context) error { return m.record("merge_abort") }
func (m *mockGitClient) Rebase(_ context.Context, _ string, _ git.RebaseOpts) error {
	return m.record("rebase")
}

func (m *mockGitClient) RebaseContinue(_ context.Context) error { return m.record("rebase_continue") }

func (m *mockGitClient) RebaseAbort(_ context.Context) error { return m.record("rebase_abort") }

func (m *mockGitClient) CherryPick(_ context.Context, _ string) error { return m.record("cherry_pick") }

func (m *mockGitClient) BisectStart(_ context.Context, _, _ string) error {
	return m.record("bisect_start")
}

func (m *mockGitClient) BisectGood(_ context.Context) (string, error) {
	return "", m.record("bisect_good")
}

func (m *mockGitClient) BisectBad(_ context.Context) (string, error) {
	return "", m.record("bisect_bad")
}
func (m *mockGitClient) BisectReset(_ context.Context) error { return m.record("bisect_reset") }

func (m *mockGitClient) Reflog(_ context.Context, _ string, _ int) ([]git.ReflogEntry, error) {
	return nil, m.record("reflog")
}

func (m *mockGitClient) RemoteList(_ context.Context) ([]git.Remote, error) {
	return nil, m.record("remote_list")
}

func (m *mockGitClient) RemoteAdd(_ context.Context, _, _ string) error {
	return m.record("remote_add")
}

func (m *mockGitClient) RemoteRemove(_ context.Context, _ string) error {
	return m.record("remote_remove")
}

func (m *mockGitClient) DiscardFile(_ context.Context, _ string) error {
	return m.record("discard_file")
}

func (m *mockGitClient) DiscardAllUnstaged(_ context.Context) error {
	return m.record("discard_all_unstaged")
}

func (m *mockGitClient) Revert(_ context.Context, _ string) error { return m.record("revert") }
func (m *mockGitClient) RevertContinue(_ context.Context) error   { return m.record("revert_continue") }

func (m *mockGitClient) RevertAbort(_ context.Context) error { return m.record("revert_abort") }

func (m *mockGitClient) Reset(_ context.Context, _ string, _ git.ResetMode) error {
	return m.record("reset")
}

// --- Engine tests ---

func TestResolveBuiltin(t *testing.T) {
	engine := NewEngine(newMockGitClient())
	sc, ok := engine.Resolve("sc")
	if !ok {
		t.Fatal("expected to resolve built-in 'sc'")
	}
	if sc.Name != "sc" {
		t.Errorf("expected name 'sc', got %q", sc.Name)
	}
	if !sc.Builtin {
		t.Error("expected sc to be builtin")
	}
}

func TestResolveUnknown(t *testing.T) {
	engine := NewEngine(newMockGitClient())
	_, ok := engine.Resolve("nonexistent")
	if ok {
		t.Error("expected false for unknown shortcut")
	}
}

func TestCustomOverridesBuiltin(t *testing.T) {
	engine := NewEngine(newMockGitClient())
	engine.RegisterCustom(Shortcut{
		Name:        "sc",
		Description: "custom sc",
		Steps:       []Step{{Op: OpCommit, Params: map[string]string{}}},
	})

	sc, ok := engine.Resolve("sc")
	if !ok {
		t.Fatal("expected to resolve overridden 'sc'")
	}
	if sc.Builtin {
		t.Error("custom override should not be marked as builtin")
	}
	if sc.Description != "custom sc" {
		t.Errorf("expected custom description, got %q", sc.Description)
	}
}

func TestListMergesCustomAndBuiltin(t *testing.T) {
	engine := NewEngine(newMockGitClient())
	engine.RegisterCustom(Shortcut{
		Name:        "deploy",
		Description: "custom deploy",
		Steps:       []Step{{Op: OpPush, Params: map[string]string{}}},
	})

	all := engine.List()
	found := make(map[string]bool)
	for _, s := range all {
		found[s.Name] = true
	}

	if !found["deploy"] {
		t.Error("expected custom 'deploy' in list")
	}
	if !found["sc"] {
		t.Error("expected built-in 'sc' in list")
	}
}

func TestExecuteSC(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)

	result, err := engine.Execute(context.Background(), "sc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("execution error: %v", result.Err)
	}
	if len(result.StepResults) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(result.StepResults))
	}

	// Verify operations were called in order.
	if len(mock.calls) < 2 {
		t.Fatalf("expected at least 2 calls, got %d", len(mock.calls))
	}
	if mock.calls[0] != "stage" {
		t.Errorf("first call should be 'stage', got %q", mock.calls[0])
	}
	if mock.calls[1] != "commit" {
		t.Errorf("second call should be 'commit', got %q", mock.calls[1])
	}
}

func TestExecuteStopOnFailure(t *testing.T) {
	mock := newMockGitClient()
	mock.failOps["commit"] = fmt.Errorf("commit failed")
	engine := NewEngine(mock)

	result, err := engine.Execute(context.Background(), "scp", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Err == nil {
		t.Fatal("expected execution error when commit fails")
	}
	// Should have 2 step results (stage OK, commit FAIL), not 3 (no push).
	if len(result.StepResults) != 2 {
		t.Errorf("expected 2 step results (stopped at commit), got %d", len(result.StepResults))
	}
}

func TestExecuteContinueOnFailure(t *testing.T) {
	mock := newMockGitClient()
	mock.failOps["fetch"] = fmt.Errorf("network error")
	engine := NewEngine(mock)

	// cleanup has on_fail=continue for both steps.
	result, err := engine.Execute(context.Background(), "cleanup", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both steps should have been attempted despite fetch failure.
	if len(result.StepResults) != 2 {
		t.Errorf("expected 2 step results, got %d", len(result.StepResults))
	}
}

func TestExecuteUnknownShortcut(t *testing.T) {
	engine := NewEngine(newMockGitClient())
	_, err := engine.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown shortcut")
	}
}

func TestPlanWithArgs(t *testing.T) {
	engine := NewEngine(newMockGitClient())
	steps, err := engine.Plan("rb", map[string]string{
		"remote": "upstream",
		"branch": "develop",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	// Verify fetch remote was substituted.
	if steps[0].Params["remote"] != "upstream" {
		t.Errorf("expected fetch remote 'upstream', got %q", steps[0].Params["remote"])
	}
	// Verify rebase onto was substituted.
	if steps[1].Params["onto"] != "upstream/develop" {
		t.Errorf("expected rebase onto 'upstream/develop', got %q", steps[1].Params["onto"])
	}
}

func TestPlanDefaultArgs(t *testing.T) {
	engine := NewEngine(newMockGitClient())
	steps, err := engine.Plan("rb", map[string]string{
		"branch": "main",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Remote should default to "origin".
	if steps[0].Params["remote"] != "origin" {
		t.Errorf("expected default remote 'origin', got %q", steps[0].Params["remote"])
	}
}

func TestPlanMissingRequiredArg(t *testing.T) {
	engine := NewEngine(newMockGitClient())
	_, err := engine.Plan("nb", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing required 'name' arg")
	}
}

func TestResolveArgs(t *testing.T) {
	defs := []Arg{
		{Name: "a", Default: "default_a"},
		{Name: "b", Required: true},
	}

	// Happy path.
	resolved, err := resolveArgs(defs, map[string]string{"b": "val_b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved["a"] != "default_a" {
		t.Errorf("expected default for 'a', got %q", resolved["a"])
	}
	if resolved["b"] != "val_b" {
		t.Errorf("expected 'val_b' for 'b', got %q", resolved["b"])
	}

	// Missing required.
	_, err = resolveArgs(defs, map[string]string{})
	if err == nil {
		t.Error("expected error for missing required arg 'b'")
	}
}

func TestSubstituteStep(t *testing.T) {
	step := Step{
		Op:     OpFetch,
		Params: map[string]string{"remote": "{{remote}}", "ref": "{{remote}}/{{branch}}"},
		OnFail: OnFailStop,
	}
	args := map[string]string{"remote": "origin", "branch": "main"}

	result := substituteStep(step, args)
	if result.Params["remote"] != "origin" {
		t.Errorf("expected 'origin', got %q", result.Params["remote"])
	}
	if result.Params["ref"] != "origin/main" {
		t.Errorf("expected 'origin/main', got %q", result.Params["ref"])
	}
}

func TestParsePaths(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", []string{"."}},
		{".", []string{"."}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a b c", []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		got := parsePaths(tt.input)
		if len(got) != len(tt.expected) {
			t.Errorf("parsePaths(%q): expected %v, got %v", tt.input, tt.expected, got)
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("parsePaths(%q)[%d]: expected %q, got %q", tt.input, i, tt.expected[i], got[i])
			}
		}
	}
}

// --- executeStep operation coverage ---

func TestExecuteStepUnstage(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpUnstage, Params: map[string]string{"paths": "a,b"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "unstage" {
		t.Errorf("expected 'unstage', got %q", mock.calls[0])
	}
}

func TestExecuteStepPush(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpPush, Params: map[string]string{
		"remote": "origin", "branch": "main", "force": "true", "set_upstream": "true",
	}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "push" {
		t.Errorf("expected 'push', got %q", mock.calls[0])
	}
}

func TestExecuteStepPull(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpPull, Params: map[string]string{
		"remote": "origin", "branch": "main", "rebase": "true",
	}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "pull" {
		t.Errorf("expected 'pull', got %q", mock.calls[0])
	}
}

func TestExecuteStepRebase(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpRebase, Params: map[string]string{"onto": "origin/main"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "rebase" {
		t.Errorf("expected 'rebase', got %q", mock.calls[0])
	}
}

func TestExecuteStepMerge(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpMerge, Params: map[string]string{
		"branch": "feature", "squash": "true", "no_ff": "true", "message": "merge msg",
	}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "merge" {
		t.Errorf("expected 'merge', got %q", mock.calls[0])
	}
}

func TestExecuteStepCheckout(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpCheckout, Params: map[string]string{"ref": "develop"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "checkout" {
		t.Errorf("expected 'checkout', got %q", mock.calls[0])
	}
}

func TestExecuteStepBranch(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpBranch, Params: map[string]string{"name": "feat-x", "base": "main"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "branch_create" {
		t.Errorf("expected 'branch_create', got %q", mock.calls[0])
	}
}

func TestExecuteStepResetSoft(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpReset, Params: map[string]string{"mode": "soft", "ref": "HEAD~1"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "checkout" {
		t.Errorf("expected 'checkout' for soft reset, got %q", mock.calls[0])
	}
}

func TestExecuteStepResetHard(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpReset, Params: map[string]string{"mode": "hard", "ref": "HEAD~1"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if len(mock.calls) != 2 || mock.calls[0] != "checkout" || mock.calls[1] != "unstage" {
		t.Errorf("expected [checkout, unstage], got %v", mock.calls)
	}
}

func TestExecuteStepResetMixed(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpReset, Params: map[string]string{"mode": "mixed"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "unstage" {
		t.Errorf("expected 'unstage' for mixed reset, got %q", mock.calls[0])
	}
}

func TestExecuteStepResetEmptyMode(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpReset, Params: map[string]string{}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "unstage" {
		t.Errorf("expected 'unstage' for empty mode reset, got %q", mock.calls[0])
	}
}

func TestExecuteStepResetUnsupported(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpReset, Params: map[string]string{"mode": "bogus"}})
	if sr.Err == nil {
		t.Fatal("expected error for unsupported reset mode")
	}
	if !errors.Is(sr.Err, ErrUnsupportedResetMode) {
		t.Errorf("error should wrap ErrUnsupportedResetMode, got: %v", sr.Err)
	}
}

func TestExecuteStepDeleteBranch(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpDelete, Params: map[string]string{"branch": "old-branch"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "branch_delete" {
		t.Errorf("expected 'branch_delete', got %q", mock.calls[0])
	}
}

func TestExecuteStepDeleteMerged(t *testing.T) {
	mock := newMockGitClient()
	mock.branches = []git.Branch{
		{Name: "main", IsCurrent: false},
		{Name: "master", IsCurrent: false},
		{Name: "develop", IsCurrent: false},
		{Name: "current", IsCurrent: true},
		{Name: "remote-only", IsRemote: true},
		{Name: "feature-done", IsCurrent: false, IsRemote: false},
	}
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpDelete, Params: map[string]string{"merged": "true"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	// Should have called branch_list + branch_delete only for "feature-done".
	if len(mock.calls) != 2 {
		t.Fatalf("expected 2 calls (list + delete), got %v", mock.calls)
	}
	if mock.calls[0] != "branch_list" {
		t.Errorf("expected 'branch_list' first, got %q", mock.calls[0])
	}
	if mock.calls[1] != "branch_delete" {
		t.Errorf("expected 'branch_delete' second, got %q", mock.calls[1])
	}
}

func TestDeleteMergedBranchesListError(t *testing.T) {
	mock := newMockGitClient()
	mock.failOps["branch_list"] = fmt.Errorf("list failed")
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpDelete, Params: map[string]string{"merged": "true"}})
	if sr.Err == nil {
		t.Fatal("expected error when branch list fails")
	}
	if !strings.Contains(sr.Err.Error(), "list branches") {
		t.Errorf("unexpected error: %v", sr.Err)
	}
}

func TestDeleteMergedBranchesPartialFailure(t *testing.T) {
	mock := newMockGitClient()
	mock.branches = []git.Branch{
		{Name: "main", IsCurrent: true},
		{Name: "feat-a"},
		{Name: "feat-b"},
	}
	mock.failOps["branch_delete"] = fmt.Errorf("delete failed")
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpDelete, Params: map[string]string{"merged": "true"}})
	if sr.Err == nil {
		t.Fatal("expected error when delete fails")
	}
	if !strings.Contains(sr.Err.Error(), "some branches could not be deleted") {
		t.Errorf("unexpected error: %v", sr.Err)
	}
}

func TestExecuteStepStash(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpStash, Params: map[string]string{"message": "wip"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "stash_push" {
		t.Errorf("expected 'stash_push', got %q", mock.calls[0])
	}
}

func TestExecuteStepStashPop(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpStashPop, Params: map[string]string{}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "stash_pop" {
		t.Errorf("expected 'stash_pop', got %q", mock.calls[0])
	}
}

func TestExecuteStepBranchRename(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpBranchRename, Params: map[string]string{"new_name": "better-name"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mock.calls[0] != "branch_rename" {
		t.Errorf("expected 'branch_rename', got %q", mock.calls[0])
	}
}

func TestExecuteStepUnknownOp(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: "warp_drive", Params: map[string]string{}})
	if sr.Err == nil {
		t.Fatal("expected error for unknown operation")
	}
	if !errors.Is(sr.Err, ErrUnknownOperation) {
		t.Errorf("error should wrap ErrUnknownOperation, got: %v", sr.Err)
	}
}

// --- OnFail=ask mode ---

func TestExecuteOnFailAsk(t *testing.T) {
	mock := newMockGitClient()
	mock.failOps["push"] = fmt.Errorf("push rejected")
	engine := NewEngine(mock)
	engine.RegisterCustom(Shortcut{
		Name: "ask-test",
		Steps: []Step{
			{Op: OpPush, Params: map[string]string{}, OnFail: OnFailAsk},
		},
	})
	result, err := engine.Execute(context.Background(), "ask-test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Err == nil {
		t.Fatal("expected execution error for ask-mode failure")
	}
	if !strings.Contains(result.Err.Error(), "would ask") {
		t.Errorf("expected 'would ask' in error, got %q", result.Err.Error())
	}
}

// --- Plan for unknown shortcut ---

func TestPlanUnknownShortcut(t *testing.T) {
	engine := NewEngine(newMockGitClient())
	_, err := engine.Plan("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown shortcut")
	}
	if !errors.Is(err, ErrUnknownShortcut) {
		t.Errorf("error should wrap ErrUnknownShortcut, got: %v", err)
	}
}

// --- resolveArgs pass-through extras ---

func TestResolveArgsPassThroughExtras(t *testing.T) {
	defs := []Arg{{Name: "known", Default: "val"}}
	resolved, err := resolveArgs(defs, map[string]string{"known": "val", "extra": "bonus"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved["extra"] != "bonus" {
		t.Errorf("expected extra arg 'bonus', got %q", resolved["extra"])
	}
}

// --- substituteStep with unresolved placeholder ---

func TestSubstituteStepMissingPlaceholder(t *testing.T) {
	step := Step{
		Op:     OpFetch,
		Params: map[string]string{"remote": "{{missing}}"},
		OnFail: OnFailStop,
	}
	result := substituteStep(step, map[string]string{})
	if result.Params["remote"] != "{{missing}}" {
		t.Errorf("expected placeholder to remain '{{missing}}', got %q", result.Params["remote"])
	}
}

// --- Reset hard with checkout failure ---

func TestExecuteStepResetHardCheckoutFails(t *testing.T) {
	mock := newMockGitClient()
	mock.failOps["checkout"] = fmt.Errorf("checkout failed")
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpReset, Params: map[string]string{"mode": "hard", "ref": "HEAD~1"}})
	if sr.Err == nil {
		t.Fatal("expected error when checkout fails in hard reset")
	}
	// Should NOT have called unstage since checkout failed first.
	for _, call := range mock.calls {
		if call == "unstage" {
			t.Error("unstage should not be called when checkout fails in hard reset")
		}
	}
}
