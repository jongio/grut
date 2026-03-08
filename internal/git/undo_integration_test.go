package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// undoRevert / redoRevert — 0% coverage
// ---------------------------------------------------------------------------

func TestUndoManager_UndoRevert_Integration(t *testing.T) {
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Make a commit to revert.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "revert-me.txt"), []byte("content"), 0o644))
	_, err = client.run(ctx, "add", "revert-me.txt")
	require.NoError(t, err)
	_, err = client.Commit(ctx, "add revert-me", CommitOpts{})
	require.NoError(t, err)

	// Get the commit hash.
	hash, err := client.run(ctx, "rev-parse", "HEAD")
	require.NoError(t, err)
	hash = hash[:len(hash)-1] // trim newline

	// Record the HEAD before revert.
	refBefore, err := client.run(ctx, "rev-parse", "HEAD")
	require.NoError(t, err)
	refBefore = refBefore[:len(refBefore)-1]

	// Revert the commit.
	err = client.Revert(ctx, hash)
	require.NoError(t, err)

	// Now undo the revert.
	um := NewUndoManager(client)
	action := UndoAction{
		Type:      "revert",
		RefBefore: refBefore,
		Metadata:  map[string]string{"hash": hash},
	}

	desc, err := um.undoRevert(ctx, action)
	require.NoError(t, err)
	assert.Contains(t, desc, "revert of")
}

func TestUndoManager_UndoRevert_MissingRefBefore(t *testing.T) {
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	um := NewUndoManager(client)
	action := UndoAction{
		Type:      "revert",
		RefBefore: "",
	}

	_, err = um.undoRevert(ctx, action)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing RefBefore")
}

func TestUndoManager_RedoRevert_Integration(t *testing.T) {
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Make a commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "redo-revert.txt"), []byte("content"), 0o644))
	_, err = client.run(ctx, "add", "redo-revert.txt")
	require.NoError(t, err)
	_, err = client.Commit(ctx, "add redo-revert", CommitOpts{})
	require.NoError(t, err)

	hash, err := client.run(ctx, "rev-parse", "HEAD")
	require.NoError(t, err)
	hash = hash[:len(hash)-1]

	um := NewUndoManager(client)
	action := UndoAction{
		Type:     "revert",
		Metadata: map[string]string{"hash": hash},
	}

	desc, err := um.redoRevert(ctx, action)
	require.NoError(t, err)
	assert.Contains(t, desc, "revert")
}

func TestUndoManager_RedoRevert_MissingHash(t *testing.T) {
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	um := NewUndoManager(client)
	action := UndoAction{
		Type:     "revert",
		Metadata: map[string]string{},
	}

	_, err = um.redoRevert(ctx, action)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing hash")
}

// ---------------------------------------------------------------------------
// undoAmend / redoAmend — 0% coverage
// ---------------------------------------------------------------------------

func TestUndoManager_UndoAmend_Integration(t *testing.T) {
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Make a commit, then amend it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "amend.txt"), []byte("v1"), 0o644))
	_, err = client.run(ctx, "add", "amend.txt")
	require.NoError(t, err)
	_, err = client.Commit(ctx, "original", CommitOpts{})
	require.NoError(t, err)

	// Amend by changing the file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "amend.txt"), []byte("v2"), 0o644))
	_, err = client.run(ctx, "add", "amend.txt")
	require.NoError(t, err)
	_, err = client.Commit(ctx, "amended", CommitOpts{Amend: true})
	require.NoError(t, err)

	// Undo the amend.
	um := NewUndoManager(client)
	desc, err := um.undoAmend(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "amend")
	assert.Contains(t, desc, "staged")
}

func TestUndoManager_RedoAmend_Integration(t *testing.T) {
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Create a commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "redo-amend.txt"), []byte("v1"), 0o644))
	_, err = client.run(ctx, "add", "redo-amend.txt")
	require.NoError(t, err)
	_, err = client.Commit(ctx, "original", CommitOpts{})
	require.NoError(t, err)

	// Stage a change.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "redo-amend.txt"), []byte("v2"), 0o644))
	_, err = client.run(ctx, "add", "redo-amend.txt")
	require.NoError(t, err)

	um := NewUndoManager(client)
	action := UndoAction{
		Type:     "amend",
		Metadata: map[string]string{"message": "amended message"},
	}

	desc, err := um.redoAmend(ctx, action)
	require.NoError(t, err)
	assert.Contains(t, desc, "amend")
}

func TestUndoManager_RedoAmend_MissingMessage(t *testing.T) {
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	um := NewUndoManager(client)
	action := UndoAction{
		Type:     "amend",
		Metadata: map[string]string{},
	}

	_, err = um.redoAmend(ctx, action)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing commit message")
}

// ---------------------------------------------------------------------------
// UnstageHunk — 0% coverage (tests runWithStdin too)
// ---------------------------------------------------------------------------

func TestClient_UnstageHunk_Integration(t *testing.T) {
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Create a file with multiple lines.
	content := "line1\nline2\nline3\nline4\n"
	path := "unstage-hunk.txt"
	require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644))
	_, err = client.run(ctx, "add", path)
	require.NoError(t, err)
	_, err = client.Commit(ctx, "add file", CommitOpts{})
	require.NoError(t, err)

	// Modify the file and stage it.
	newContent := "line1\nmodified\nline3\nline4\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte(newContent), 0o644))
	_, err = client.run(ctx, "add", path)
	require.NoError(t, err)

	// Get the staged diff to construct a hunk.
	diffs, err := client.Diff(ctx, DiffOpts{Staged: true})
	require.NoError(t, err)
	require.NotEmpty(t, diffs, "expected at least one staged diff")

	// Find our file's diff.
	var targetDiff *FileDiff
	for i := range diffs {
		if diffs[i].Path == path {
			targetDiff = &diffs[i]
			break
		}
	}
	require.NotNil(t, targetDiff, "diff for %s not found", path)
	require.NotEmpty(t, targetDiff.Hunks)

	// Unstage the hunk.
	err = client.UnstageHunk(ctx, path, targetDiff.Hunks[0])
	assert.NoError(t, err)
}

func TestClient_UnstageHunk_InvalidPath(t *testing.T) {
	dir := initTestRepo(t)
	client, err := NewClient(dir)
	require.NoError(t, err)
	ctx := context.Background()

	hunk := Hunk{
		Header: "@@ -1,1 +1,1 @@",
		Lines: []DiffLine{
			{Type: DiffLineContext, Content: " line1"},
		},
	}

	err = client.UnstageHunk(ctx, "../../etc/passwd", hunk)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path")
}

// ---------------------------------------------------------------------------
// CheckGitInstalled — 75% coverage
// ---------------------------------------------------------------------------

func TestCheckGitInstalled_Available(t *testing.T) {
	t.Parallel()

	err := CheckGitInstalled()
	assert.NoError(t, err)
}
