package shortcuts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/git/gittest"
)

type mockGitClient = gittest.MockClient

type mockGitClientState struct {
	calls    []string
	failOps  map[string]error
	branches []git.Branch
}

var mockGitClientStates = map[*mockGitClient]*mockGitClientState{}

func newMockGitClient() *mockGitClient {
	mock := &mockGitClient{}
	state := &mockGitClientState{
		failOps: make(map[string]error),
		branches: []git.Branch{
			{Name: "main", IsCurrent: true},
			{Name: "feature-a", IsCurrent: false},
		},
	}
	mockGitClientStates[mock] = state

	mock.StatusFunc = func(context.Context) ([]git.FileStatus, error) {
		return nil, recordMockGitCall(mock, "status")
	}
	mock.DiffFunc = func(context.Context, git.DiffOpts) ([]git.FileDiff, error) {
		return nil, recordMockGitCall(mock, "diff")
	}
	mock.LogFunc = func(context.Context, git.LogOpts) ([]git.Commit, error) {
		return nil, recordMockGitCall(mock, "log")
	}
	mock.BlameFunc = func(context.Context, string) ([]git.BlameLine, error) {
		return nil, recordMockGitCall(mock, "blame")
	}
	mock.RepoRootFunc = func(context.Context) (string, error) {
		return "/repo", recordMockGitCall(mock, "repo_root")
	}
	mock.IsRepoFunc = func(context.Context) (bool, error) {
		return true, recordMockGitCall(mock, "is_repo")
	}
	mock.DiffTreeFilesFunc = func(context.Context, string) ([]string, error) {
		return nil, recordMockGitCall(mock, "diff_tree_files")
	}
	mock.DiffFileNamesFunc = func(context.Context, string, string) ([]string, error) {
		return nil, recordMockGitCall(mock, "diff_file_names")
	}
	mock.StageFunc = func(context.Context, []string) error { return recordMockGitCall(mock, "stage") }
	mock.UnstageFunc = func(context.Context, []string) error { return recordMockGitCall(mock, "unstage") }
	mock.StageHunkFunc = func(context.Context, string, git.Hunk) error { return recordMockGitCall(mock, "stage_hunk") }
	mock.UnstageHunkFunc = func(context.Context, string, git.Hunk) error { return recordMockGitCall(mock, "unstage_hunk") }
	mock.StageLineFunc = func(context.Context, string, git.Hunk, int) error { return recordMockGitCall(mock, "stage_line") }
	mock.UnstageLineFunc = func(context.Context, string, git.Hunk, int) error { return recordMockGitCall(mock, "unstage_line") }
	mock.CommitFunc = func(context.Context, string, git.CommitOpts) (string, error) {
		return "abc1234", recordMockGitCall(mock, "commit")
	}
	mock.BranchListFunc = func(context.Context) ([]git.Branch, error) {
		return mockGitState(mock).branches, recordMockGitCall(mock, "branch_list")
	}
	mock.BranchCreateFunc = func(context.Context, string, string) error { return recordMockGitCall(mock, "branch_create") }
	mock.BranchDeleteFunc = func(context.Context, string, bool) error { return recordMockGitCall(mock, "branch_delete") }
	mock.BranchRenameFunc = func(context.Context, string, string) error { return recordMockGitCall(mock, "branch_rename") }
	mock.CheckoutFunc = func(context.Context, string) error { return recordMockGitCall(mock, "checkout") }
	mock.PushFunc = func(context.Context, git.PushOpts) error { return recordMockGitCall(mock, "push") }
	mock.PullFunc = func(context.Context, git.PullOpts) error { return recordMockGitCall(mock, "pull") }
	mock.FetchFunc = func(context.Context, git.FetchOpts) error { return recordMockGitCall(mock, "fetch") }
	mock.WorktreeListFunc = func(context.Context) ([]git.Worktree, error) {
		return nil, recordMockGitCall(mock, "worktree_list")
	}
	mock.WorktreeAddFunc = func(context.Context, string, string) error { return recordMockGitCall(mock, "worktree_add") }
	mock.WorktreeRemoveFunc = func(context.Context, string, bool) error { return recordMockGitCall(mock, "worktree_remove") }
	mock.StashListFunc = func(context.Context) ([]git.StashEntry, error) {
		return nil, recordMockGitCall(mock, "stash_list")
	}
	mock.StashShowFunc = func(context.Context, int) (string, error) {
		return "", recordMockGitCall(mock, "stash_show")
	}
	mock.StashPushFunc = func(context.Context, git.StashOpts) error { return recordMockGitCall(mock, "stash_push") }
	mock.StashPopFunc = func(context.Context, int) error { return recordMockGitCall(mock, "stash_pop") }
	mock.StashApplyFunc = func(context.Context, int) error { return recordMockGitCall(mock, "stash_apply") }
	mock.StashDropFunc = func(context.Context, int) error { return recordMockGitCall(mock, "stash_drop") }
	mock.TagListFunc = func(context.Context) ([]git.Tag, error) {
		return nil, recordMockGitCall(mock, "tag_list")
	}
	mock.TagCreateFunc = func(context.Context, string, string, string) error { return recordMockGitCall(mock, "tag_create") }
	mock.TagDeleteFunc = func(context.Context, string) error { return recordMockGitCall(mock, "tag_delete") }
	mock.TagListRemoteFunc = func(context.Context, string) ([]git.Tag, error) {
		return nil, recordMockGitCall(mock, "tag_list_remote")
	}
	mock.TagPushFunc = func(context.Context, string, string) error { return recordMockGitCall(mock, "tag_push") }
	mock.TagPushAllFunc = func(context.Context, string) error { return recordMockGitCall(mock, "tag_push_all") }
	mock.MergeFunc = func(context.Context, string, git.MergeOpts) error { return recordMockGitCall(mock, "merge") }
	mock.MergeAbortFunc = func(context.Context) error { return recordMockGitCall(mock, "merge_abort") }
	mock.RebaseFunc = func(context.Context, string, git.RebaseOpts) error { return recordMockGitCall(mock, "rebase") }
	mock.RebaseContinueFunc = func(context.Context) error { return recordMockGitCall(mock, "rebase_continue") }
	mock.RebaseAbortFunc = func(context.Context) error { return recordMockGitCall(mock, "rebase_abort") }
	mock.CherryPickFunc = func(context.Context, string) error { return recordMockGitCall(mock, "cherry_pick") }
	mock.BisectStartFunc = func(context.Context, string, string) error { return recordMockGitCall(mock, "bisect_start") }
	mock.BisectGoodFunc = func(context.Context) (string, error) { return "", recordMockGitCall(mock, "bisect_good") }
	mock.BisectBadFunc = func(context.Context) (string, error) { return "", recordMockGitCall(mock, "bisect_bad") }
	mock.BisectResetFunc = func(context.Context) error { return recordMockGitCall(mock, "bisect_reset") }
	mock.ReflogFunc = func(context.Context, string, int) ([]git.ReflogEntry, error) {
		return nil, recordMockGitCall(mock, "reflog")
	}
	mock.RemoteListFunc = func(context.Context) ([]git.Remote, error) {
		return nil, recordMockGitCall(mock, "remote_list")
	}
	mock.RemoteAddFunc = func(context.Context, string, string) error { return recordMockGitCall(mock, "remote_add") }
	mock.RemoteRemoveFunc = func(context.Context, string) error { return recordMockGitCall(mock, "remote_remove") }
	mock.DiscardFileFunc = func(context.Context, string) error { return recordMockGitCall(mock, "discard_file") }
	mock.DiscardAllFunc = func(context.Context) error { return recordMockGitCall(mock, "discard_all_unstaged") }
	mock.RevertFunc = func(context.Context, string) error { return recordMockGitCall(mock, "revert") }
	mock.RevertContinueFunc = func(context.Context) error { return recordMockGitCall(mock, "revert_continue") }
	mock.RevertAbortFunc = func(context.Context) error { return recordMockGitCall(mock, "revert_abort") }
	mock.ResetFunc = func(context.Context, string, git.ResetMode) error { return recordMockGitCall(mock, "reset") }

	return mock
}

func mockGitState(mock *mockGitClient) *mockGitClientState {
	state, ok := mockGitClientStates[mock]
	if !ok {
		panic("mock git client state not initialized")
	}
	return state
}

func recordMockGitCall(mock *mockGitClient, op string) error {
	state := mockGitState(mock)
	state.calls = append(state.calls, op)
	if err, ok := state.failOps[op]; ok {
		return err
	}
	return nil
}

func mockGitCalls(mock *mockGitClient) []string {
	return mockGitState(mock).calls
}

func mockGitFailOps(mock *mockGitClient) map[string]error {
	return mockGitState(mock).failOps
}

func setMockGitBranches(mock *mockGitClient, branches []git.Branch) {
	mockGitState(mock).branches = branches
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
	if len(mockGitCalls(mock)) < 2 {
		t.Fatalf("expected at least 2 calls, got %d", len(mockGitCalls(mock)))
	}
	if mockGitCalls(mock)[0] != "stage" {
		t.Errorf("first call should be 'stage', got %q", mockGitCalls(mock)[0])
	}
	if mockGitCalls(mock)[1] != "commit" {
		t.Errorf("second call should be 'commit', got %q", mockGitCalls(mock)[1])
	}
}

func TestExecuteStopOnFailure(t *testing.T) {
	mock := newMockGitClient()
	mockGitFailOps(mock)["commit"] = fmt.Errorf("commit failed")
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
	mockGitFailOps(mock)["fetch"] = fmt.Errorf("network error")
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
	if mockGitCalls(mock)[0] != "unstage" {
		t.Errorf("expected 'unstage', got %q", mockGitCalls(mock)[0])
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
	if mockGitCalls(mock)[0] != "push" {
		t.Errorf("expected 'push', got %q", mockGitCalls(mock)[0])
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
	if mockGitCalls(mock)[0] != "pull" {
		t.Errorf("expected 'pull', got %q", mockGitCalls(mock)[0])
	}
}

func TestExecuteStepRebase(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpRebase, Params: map[string]string{"onto": "origin/main"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mockGitCalls(mock)[0] != "rebase" {
		t.Errorf("expected 'rebase', got %q", mockGitCalls(mock)[0])
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
	if mockGitCalls(mock)[0] != "merge" {
		t.Errorf("expected 'merge', got %q", mockGitCalls(mock)[0])
	}
}

func TestExecuteStepCheckout(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpCheckout, Params: map[string]string{"ref": "develop"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mockGitCalls(mock)[0] != "checkout" {
		t.Errorf("expected 'checkout', got %q", mockGitCalls(mock)[0])
	}
}

func TestExecuteStepBranch(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpBranch, Params: map[string]string{"name": "feat-x", "base": "main"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mockGitCalls(mock)[0] != "branch_create" {
		t.Errorf("expected 'branch_create', got %q", mockGitCalls(mock)[0])
	}
}

func TestExecuteStepResetSoft(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpReset, Params: map[string]string{"mode": "soft", "ref": "HEAD~1"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mockGitCalls(mock)[0] != "checkout" {
		t.Errorf("expected 'checkout' for soft reset, got %q", mockGitCalls(mock)[0])
	}
}

func TestExecuteStepResetHard(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpReset, Params: map[string]string{"mode": "hard", "ref": "HEAD~1"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if len(mockGitCalls(mock)) != 2 || mockGitCalls(mock)[0] != "checkout" || mockGitCalls(mock)[1] != "unstage" {
		t.Errorf("expected [checkout, unstage], got %v", mockGitCalls(mock))
	}
}

func TestExecuteStepResetMixed(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpReset, Params: map[string]string{"mode": "mixed"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mockGitCalls(mock)[0] != "unstage" {
		t.Errorf("expected 'unstage' for mixed reset, got %q", mockGitCalls(mock)[0])
	}
}

func TestExecuteStepResetEmptyMode(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpReset, Params: map[string]string{}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mockGitCalls(mock)[0] != "unstage" {
		t.Errorf("expected 'unstage' for empty mode reset, got %q", mockGitCalls(mock)[0])
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
	if mockGitCalls(mock)[0] != "branch_delete" {
		t.Errorf("expected 'branch_delete', got %q", mockGitCalls(mock)[0])
	}
}

func TestExecuteStepDeleteMerged(t *testing.T) {
	mock := newMockGitClient()
	setMockGitBranches(mock, []git.Branch{
		{Name: "main", IsCurrent: false},
		{Name: "master", IsCurrent: false},
		{Name: "develop", IsCurrent: false},
		{Name: "current", IsCurrent: true},
		{Name: "remote-only", IsRemote: true},
		{Name: "feature-done", IsCurrent: false, IsRemote: false},
	})
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpDelete, Params: map[string]string{"merged": "true"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	// Should have called branch_list + branch_delete only for "feature-done".
	if len(mockGitCalls(mock)) != 2 {
		t.Fatalf("expected 2 calls (list + delete), got %v", mockGitCalls(mock))
	}
	if mockGitCalls(mock)[0] != "branch_list" {
		t.Errorf("expected 'branch_list' first, got %q", mockGitCalls(mock)[0])
	}
	if mockGitCalls(mock)[1] != "branch_delete" {
		t.Errorf("expected 'branch_delete' second, got %q", mockGitCalls(mock)[1])
	}
}

func TestDeleteMergedBranchesListError(t *testing.T) {
	mock := newMockGitClient()
	mockGitFailOps(mock)["branch_list"] = fmt.Errorf("list failed")
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
	setMockGitBranches(mock, []git.Branch{
		{Name: "main", IsCurrent: true},
		{Name: "feat-a"},
		{Name: "feat-b"},
	})
	mockGitFailOps(mock)["branch_delete"] = fmt.Errorf("delete failed")
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
	if mockGitCalls(mock)[0] != "stash_push" {
		t.Errorf("expected 'stash_push', got %q", mockGitCalls(mock)[0])
	}
}

func TestExecuteStepStashPop(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpStashPop, Params: map[string]string{}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mockGitCalls(mock)[0] != "stash_pop" {
		t.Errorf("expected 'stash_pop', got %q", mockGitCalls(mock)[0])
	}
}

func TestExecuteStepBranchRename(t *testing.T) {
	mock := newMockGitClient()
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpBranchRename, Params: map[string]string{"new_name": "better-name"}})
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if mockGitCalls(mock)[0] != "branch_rename" {
		t.Errorf("expected 'branch_rename', got %q", mockGitCalls(mock)[0])
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
	mockGitFailOps(mock)["push"] = fmt.Errorf("push rejected")
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
	mockGitFailOps(mock)["checkout"] = fmt.Errorf("checkout failed")
	engine := NewEngine(mock)
	sr := engine.executeStep(context.Background(), Step{Op: OpReset, Params: map[string]string{"mode": "hard", "ref": "HEAD~1"}})
	if sr.Err == nil {
		t.Fatal("expected error when checkout fails in hard reset")
	}
	// Should NOT have called unstage since checkout failed first.
	for _, call := range mockGitCalls(mock) {
		if call == "unstage" {
			t.Error("unstage should not be called when checkout fails in hard reset")
		}
	}
}
