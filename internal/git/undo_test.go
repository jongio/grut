package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Undo commit
// ---------------------------------------------------------------------------

func TestUndoManager_UndoCommit(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Create a file, stage, and commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"file.txt"}))
	hash, err := client.Commit(ctx, "Add file", CommitOpts{})
	require.NoError(t, err)

	um.RecordAction(UndoAction{
		Type:      "commit",
		RefBefore: hash,
		Metadata:  map[string]string{"message": "Add file", "hash": hash},
	})

	assert.True(t, um.CanUndo())
	assert.False(t, um.CanRedo())

	// Undo the commit.
	desc, err := um.Undo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "commit")

	// File should still be staged (soft reset).
	statuses, err := client.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, StatusAdded, statuses[0].StagedStatus)

	assert.False(t, um.CanUndo())
	assert.True(t, um.CanRedo())
}

// ---------------------------------------------------------------------------
// Undo stage / unstage
// ---------------------------------------------------------------------------

func TestUndoManager_UndoStage(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Create and stage a file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("content\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"staged.txt"}))

	um.RecordAction(UndoAction{
		Type:     "stage",
		Metadata: map[string]string{"paths": JoinPaths([]string{"staged.txt"})},
	})

	// Verify staged.
	statuses, err := client.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, StatusAdded, statuses[0].StagedStatus)

	// Undo stage → file should be unstaged (untracked).
	desc, err := um.Undo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "stage")
	assert.Contains(t, desc, "staged.txt")

	statuses, err = client.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, StatusUntracked, statuses[0].StagedStatus)
}

func TestUndoManager_UndoUnstage(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Create, stage, then unstage a file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "toggle.txt"), []byte("data\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"toggle.txt"}))
	require.NoError(t, client.Unstage(ctx, []string{"toggle.txt"}))

	um.RecordAction(UndoAction{
		Type:     "unstage",
		Metadata: map[string]string{"paths": JoinPaths([]string{"toggle.txt"})},
	})

	// File is untracked after unstage.
	statuses, err := client.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, StatusUntracked, statuses[0].StagedStatus)

	// Undo unstage → file should be staged again.
	desc, err := um.Undo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "unstage")

	statuses, err = client.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, StatusAdded, statuses[0].StagedStatus)
}

// ---------------------------------------------------------------------------
// Undo branch delete
// ---------------------------------------------------------------------------

func TestUndoManager_UndoBranchDelete(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Create a branch, get its hash, then delete it.
	require.NoError(t, client.BranchCreate(ctx, "feature-undo", ""))

	branches, err := client.BranchList(ctx)
	require.NoError(t, err)
	var branchHash string
	for _, b := range branches {
		if b.Name == "feature-undo" {
			branchHash = b.Hash
			break
		}
	}
	require.NotEmpty(t, branchHash)

	require.NoError(t, client.BranchDelete(ctx, "feature-undo", false))

	um.RecordAction(UndoAction{
		Type:     "branch_delete",
		Metadata: map[string]string{"name": "feature-undo", "hash": branchHash},
	})

	// Verify branch is gone.
	branches, err = client.BranchList(ctx)
	require.NoError(t, err)
	for _, b := range branches {
		assert.NotEqual(t, "feature-undo", b.Name)
	}

	// Undo → branch should be recreated.
	desc, err := um.Undo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "feature-undo")

	branches, err = client.BranchList(ctx)
	require.NoError(t, err)
	var found bool
	for _, b := range branches {
		if b.Name == "feature-undo" {
			found = true
			break
		}
	}
	assert.True(t, found, "branch should be recreated after undo")
}

// ---------------------------------------------------------------------------
// Undo checkout
// ---------------------------------------------------------------------------

func TestUndoManager_UndoCheckout(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)
	defaultBranch := detectDefaultBranch(t, dir)

	// Create and checkout a new branch.
	require.NoError(t, client.BranchCreate(ctx, "dev-undo", ""))
	require.NoError(t, client.Checkout(ctx, "dev-undo"))

	um.RecordAction(UndoAction{
		Type:     "checkout",
		Metadata: map[string]string{"from": defaultBranch, "to": "dev-undo"},
	})

	// Verify on dev-undo.
	branches, err := client.BranchList(ctx)
	require.NoError(t, err)
	for _, b := range branches {
		if b.Name == "dev-undo" {
			assert.True(t, b.IsCurrent)
		}
	}

	// Undo checkout → back to default branch.
	desc, err := um.Undo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "checkout")
	assert.Contains(t, desc, defaultBranch)

	branches, err = client.BranchList(ctx)
	require.NoError(t, err)
	for _, b := range branches {
		if b.Name == defaultBranch {
			assert.True(t, b.IsCurrent)
		}
	}
}

func TestUndoManager_UndoRedoDiscard(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)
	readmePath := filepath.Join(dir, "README.md")
	modified := "# Test\nmodified\n"
	require.NoError(t, os.WriteFile(readmePath, []byte(modified), 0o644))

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = testGitEnv()
		out, runErr := cmd.CombinedOutput()
		require.NoError(t, runErr, "git %v failed: %s", args, string(out))
		return strings.TrimSpace(string(out))
	}

	run("stash", "push", "-m", "discard-safety", "--", "README.md")
	stashRef := "stash@{0}"
	run("stash", "apply", stashRef)

	require.NoError(t, client.DiscardFile(ctx, "README.md"))
	data, err := os.ReadFile(readmePath)
	require.NoError(t, err)
	assert.Equal(t, "# Test\n", string(data))

	um.RecordAction(UndoAction{
		Type:     "discard",
		Metadata: map[string]string{"path": "README.md", "stash_ref": stashRef},
	})

	desc, err := um.Undo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "discard README.md")
	data, err = os.ReadFile(readmePath)
	require.NoError(t, err)
	assert.Equal(t, modified, string(data))

	desc, err = um.Redo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "discard README.md")
	data, err = os.ReadFile(readmePath)
	require.NoError(t, err)
	assert.Equal(t, "# Test\n", string(data))
}

func TestUndoManager_UndoRedoResetHard(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "reset.txt"), []byte("reset\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"reset.txt"}))
	secondHash, err := client.Commit(ctx, "Reset target", CommitOpts{})
	require.NoError(t, err)

	commits, err := client.Log(ctx, LogOpts{MaxCount: 2})
	require.NoError(t, err)
	require.Len(t, commits, 2)
	firstHash := commits[1].Hash

	currentHead := func() string {
		t.Helper()
		cmd := exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = dir
		cmd.Env = testGitEnv()
		out, headErr := cmd.Output()
		require.NoError(t, headErr)
		return strings.TrimSpace(string(out))
	}

	require.NoError(t, client.Reset(ctx, firstHash, ResetHard))
	assert.Equal(t, firstHash, currentHead())

	um.RecordAction(UndoAction{
		Type:      "reset",
		RefBefore: secondHash,
		Metadata: map[string]string{
			"ref":  firstHash,
			"mode": string(ResetHard),
		},
	})

	desc, err := um.Undo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "reset")
	assert.Equal(t, secondHash, currentHead())

	desc, err = um.Redo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "reset --hard")
	assert.Equal(t, firstHash, currentHead())
}

// ---------------------------------------------------------------------------
// Redo
// ---------------------------------------------------------------------------

func TestUndoManager_RedoAfterUndo(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Create, stage, commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "redo.txt"), []byte("redo\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"redo.txt"}))
	hash, err := client.Commit(ctx, "Redo test", CommitOpts{})
	require.NoError(t, err)

	um.RecordAction(UndoAction{
		Type:      "commit",
		RefBefore: hash,
		Metadata:  map[string]string{"message": "Redo test", "hash": hash},
	})

	// Undo the commit.
	_, err = um.Undo(ctx)
	require.NoError(t, err)

	// File should be staged.
	statuses, err := client.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, StatusAdded, statuses[0].StagedStatus)

	// Redo → re-commit.
	assert.True(t, um.CanRedo())
	desc, err := um.Redo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "commit")
	assert.Contains(t, desc, "Redo test")

	// Status should be clean after redo commit.
	statuses, err = client.Status(ctx)
	require.NoError(t, err)
	assert.Empty(t, statuses)

	// Redo stack should be empty; undo stack should have the action back.
	assert.False(t, um.CanRedo())
	assert.True(t, um.CanUndo())
}

func TestUndoManager_RedoStage(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Create and stage.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "restage.txt"), []byte("data\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"restage.txt"}))

	um.RecordAction(UndoAction{
		Type:     "stage",
		Metadata: map[string]string{"paths": JoinPaths([]string{"restage.txt"})},
	})

	// Undo stage (unstages).
	_, err = um.Undo(ctx)
	require.NoError(t, err)

	// Redo stage (re-stages).
	desc, err := um.Redo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "stage")

	statuses, err := client.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, StatusAdded, statuses[0].StagedStatus)
}

func TestUndoManager_RedoBranchDelete(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Create a branch, note hash, delete it.
	require.NoError(t, client.BranchCreate(ctx, "temp-branch", ""))
	branches, err := client.BranchList(ctx)
	require.NoError(t, err)
	var hash string
	for _, b := range branches {
		if b.Name == "temp-branch" {
			hash = b.Hash
		}
	}
	require.NotEmpty(t, hash)
	require.NoError(t, client.BranchDelete(ctx, "temp-branch", false))

	um.RecordAction(UndoAction{
		Type:     "branch_delete",
		Metadata: map[string]string{"name": "temp-branch", "hash": hash},
	})

	// Undo → recreate.
	_, err = um.Undo(ctx)
	require.NoError(t, err)

	// Redo → re-delete.
	desc, err := um.Redo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "temp-branch")

	// Branch should be gone again.
	branches, err = client.BranchList(ctx)
	require.NoError(t, err)
	for _, b := range branches {
		assert.NotEqual(t, "temp-branch", b.Name)
	}
}

func TestUndoManager_RedoCheckout(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)
	defaultBranch := detectDefaultBranch(t, dir)

	require.NoError(t, client.BranchCreate(ctx, "redo-checkout", ""))
	require.NoError(t, client.Checkout(ctx, "redo-checkout"))

	um.RecordAction(UndoAction{
		Type:     "checkout",
		Metadata: map[string]string{"from": defaultBranch, "to": "redo-checkout"},
	})

	// Undo → back to default.
	_, err = um.Undo(ctx)
	require.NoError(t, err)

	// Redo → back to redo-checkout.
	desc, err := um.Redo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "redo-checkout")

	branches, err := client.BranchList(ctx)
	require.NoError(t, err)
	for _, b := range branches {
		if b.Name == "redo-checkout" {
			assert.True(t, b.IsCurrent)
		}
	}
}

// ---------------------------------------------------------------------------
// Stack limits
// ---------------------------------------------------------------------------

func TestUndoManager_StackLimit(t *testing.T) {
	dir := initTestRepo(t)

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Push more than maxUndoDepth actions.
	for i := range maxUndoDepth + 20 {
		um.RecordAction(UndoAction{
			Type:     "stage",
			Metadata: map[string]string{"paths": "file.txt", "index": strings.Repeat("x", i)},
		})
	}

	assert.Len(t, um.undoStack, maxUndoDepth, "stack should be capped at maxUndoDepth")
}

// ---------------------------------------------------------------------------
// New action clears redo stack
// ---------------------------------------------------------------------------

func TestUndoManager_NewActionClearsRedoStack(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Stage a file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "clear.txt"), []byte("x\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"clear.txt"}))

	um.RecordAction(UndoAction{
		Type:     "stage",
		Metadata: map[string]string{"paths": JoinPaths([]string{"clear.txt"})},
	})

	// Undo → redo stack has one entry.
	_, err = um.Undo(ctx)
	require.NoError(t, err)
	assert.True(t, um.CanRedo())

	// Record a new action → redo stack should be cleared.
	um.RecordAction(UndoAction{
		Type:     "stage",
		Metadata: map[string]string{"paths": JoinPaths([]string{"other.txt"})},
	})
	assert.False(t, um.CanRedo())
}

// ---------------------------------------------------------------------------
// Empty stack errors
// ---------------------------------------------------------------------------

func TestUndoManager_UndoEmptyStack(t *testing.T) {
	dir := initTestRepo(t)

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	_, err = um.Undo(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to undo")
}

func TestUndoManager_RedoEmptyStack(t *testing.T) {
	dir := initTestRepo(t)

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	_, err = um.Redo(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to redo")
}

// ---------------------------------------------------------------------------
// NeedsConfirmation
// ---------------------------------------------------------------------------

func TestUndoManager_NeedsConfirmation(t *testing.T) {
	dir := initTestRepo(t)

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Normal stage action — no confirmation needed.
	um.RecordAction(UndoAction{
		Type:     "stage",
		Metadata: map[string]string{"paths": "file.txt"},
	})
	assert.False(t, um.NeedsConfirmation())

	// Pushed commit — needs confirmation.
	um.RecordAction(UndoAction{
		Type:     "commit",
		Metadata: map[string]string{"message": "pushed commit", "pushed": "true"},
	})
	assert.True(t, um.NeedsConfirmation())
}

func TestUndoManager_RedoNeedsConfirmation(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Create and delete a branch.
	require.NoError(t, client.BranchCreate(ctx, "confirm-test", ""))
	branches, err := client.BranchList(ctx)
	require.NoError(t, err)
	var hash string
	for _, b := range branches {
		if b.Name == "confirm-test" {
			hash = b.Hash
		}
	}
	require.NotEmpty(t, hash)
	require.NoError(t, client.BranchDelete(ctx, "confirm-test", false))

	um.RecordAction(UndoAction{
		Type:     "branch_delete",
		Metadata: map[string]string{"name": "confirm-test", "hash": hash},
	})

	// Undo (recreate branch).
	_, err = um.Undo(ctx)
	require.NoError(t, err)

	// Redo of branch_delete should need confirmation.
	assert.True(t, um.RedoNeedsConfirmation())
}

// ---------------------------------------------------------------------------
// Unknown action type
// ---------------------------------------------------------------------------

func TestUndoManager_UnknownActionType(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	um.RecordAction(UndoAction{
		Type:     "unknown_type",
		Metadata: map[string]string{},
	})

	_, err = um.Undo(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown action type")
}

// ---------------------------------------------------------------------------
// Missing metadata errors
// ---------------------------------------------------------------------------

func TestUndoManager_UndoStageMissingPaths(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	um.RecordAction(UndoAction{
		Type:     "stage",
		Metadata: map[string]string{},
	})

	_, err = um.Undo(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no paths")

	// Action should be restored to undo stack on failure.
	assert.True(t, um.CanUndo(), "action should be restored on failure")
}

func TestUndoManager_UndoBranchDeleteMissingMetadata(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	um.RecordAction(UndoAction{
		Type:     "branch_delete",
		Metadata: map[string]string{},
	})

	_, err = um.Undo(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing name or hash")
}

func TestUndoManager_UndoCheckoutMissingFrom(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	um.RecordAction(UndoAction{
		Type:     "checkout",
		Metadata: map[string]string{},
	})

	_, err = um.Undo(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'from'")
}

// ---------------------------------------------------------------------------
// Peek
// ---------------------------------------------------------------------------

func TestUndoManager_PeekUndo(t *testing.T) {
	dir := initTestRepo(t)

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Empty stack.
	_, ok := um.PeekUndo()
	assert.False(t, ok)

	um.RecordAction(UndoAction{Type: "stage", Metadata: map[string]string{"paths": "a.txt"}})
	um.RecordAction(UndoAction{Type: "commit", Metadata: map[string]string{"message": "test"}})

	action, ok := um.PeekUndo()
	assert.True(t, ok)
	assert.Equal(t, "commit", action.Type)
}

func TestUndoManager_PeekRedo(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Empty redo stack.
	_, ok := um.PeekRedo()
	assert.False(t, ok)

	// Stage, undo → redo stack has one entry.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "peek.txt"), []byte("x\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"peek.txt"}))

	um.RecordAction(UndoAction{
		Type:     "stage",
		Metadata: map[string]string{"paths": JoinPaths([]string{"peek.txt"})},
	})
	_, err = um.Undo(ctx)
	require.NoError(t, err)

	action, ok := um.PeekRedo()
	assert.True(t, ok)
	assert.Equal(t, "stage", action.Type)
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

func TestJoinPaths(t *testing.T) {
	assert.Equal(t, "a.txt\nb.txt", JoinPaths([]string{"a.txt", "b.txt"}))
	assert.Equal(t, "single.txt", JoinPaths([]string{"single.txt"}))
	assert.Equal(t, "", JoinPaths(nil))
}

func TestSplitPaths(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "two paths", input: "a.txt\nb.txt", want: []string{"a.txt", "b.txt"}},
		{name: "single path", input: "one.txt", want: []string{"one.txt"}},
		{name: "empty", input: "", want: nil},
		{name: "trailing newline", input: "a.txt\n", want: []string{"a.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, splitPaths(tt.input))
		})
	}
}

func TestSummarizePaths(t *testing.T) {
	assert.Equal(t, "", summarizePaths(nil))
	assert.Equal(t, "file.txt", summarizePaths([]string{"file.txt"}))
	assert.Equal(t, "a.txt (+2 more)", summarizePaths([]string{"a.txt", "b.txt", "c.txt"}))
}

func TestTruncateStr(t *testing.T) {
	assert.Equal(t, "hello", truncateStr("hello", 10))
	assert.Equal(t, "hel...", truncateStr("hello world", 6))
	assert.Equal(t, "he", truncateStr("hello", 2))
	assert.Equal(t, "hello world", truncateStr("hello world", 50))
}

// ---------------------------------------------------------------------------
// Multiple paths stage/unstage
// ---------------------------------------------------------------------------

func TestUndoManager_UndoStageMultiplePaths(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()

	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Create and stage multiple files.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"a.txt", "b.txt"}))

	um.RecordAction(UndoAction{
		Type:     "stage",
		Metadata: map[string]string{"paths": JoinPaths([]string{"a.txt", "b.txt"})},
	})

	// Undo stage.
	desc, err := um.Undo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "a.txt")
	assert.Contains(t, desc, "+1 more")

	// Both files should be untracked.
	statuses, err := client.Status(ctx)
	require.NoError(t, err)
	assert.Len(t, statuses, 2)
	for _, s := range statuses {
		assert.Equal(t, StatusUntracked, s.StagedStatus)
	}
}

// ---------------------------------------------------------------------------
// Redo unstage (covers redoUnstage at 0%)
// ---------------------------------------------------------------------------

func TestUndoManager_RedoUnstage(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Create a file, stage it, then unstage (simulating user action).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "u.txt"), []byte("u\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"u.txt"}))
	require.NoError(t, client.Unstage(ctx, []string{"u.txt"}))

	um.RecordAction(UndoAction{
		Type:     "unstage",
		Metadata: map[string]string{"paths": "u.txt"},
	})

	// Undo the unstage → re-stages the file.
	desc, err := um.Undo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "unstage")

	statuses, err := client.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, StatusAdded, statuses[0].StagedStatus)

	// Redo the unstage → unstages the file again.
	desc, err = um.Redo(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "unstage")

	statuses, err = client.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, StatusUntracked, statuses[0].StagedStatus)
}

// ---------------------------------------------------------------------------
// Redo failure restores action to redo stack
// ---------------------------------------------------------------------------

func TestUndoManager_RedoFailureRestoresStack(t *testing.T) {
	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Create a commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("f\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"f.txt"}))
	_, err = client.Commit(ctx, "test commit", CommitOpts{})
	require.NoError(t, err)

	// Record commit action with missing message metadata.
	um.RecordAction(UndoAction{
		Type:     "commit",
		Metadata: map[string]string{}, // no "message" → redoCommit will fail
	})

	// Undo succeeds (undoCommit just does git reset --soft HEAD~1).
	_, err = um.Undo(ctx)
	require.NoError(t, err)

	// Redo fails because redoCommit needs a message.
	_, err = um.Redo(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing commit message")

	// Action should be restored to redo stack.
	assert.True(t, um.CanRedo())
}
