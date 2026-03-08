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
// NewUndoManager — constructor, zero-coverage
// ---------------------------------------------------------------------------

func TestNewUndoManager_Constructor(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	client, err := NewClient(dir)
	assert.NoError(t, err)

	um := NewUndoManager(client)
	assert.NotNil(t, um)
	assert.False(t, um.CanUndo())
	assert.False(t, um.CanRedo())
}

// ---------------------------------------------------------------------------
// redoDiscard — zero-coverage
// ---------------------------------------------------------------------------

func TestUndoManager_RedoDiscard(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Missing path in metadata → error.
	_, err = um.redoDiscard(ctx, UndoAction{
		Type:     "discard",
		Metadata: map[string]string{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing path")

	// Valid path with actual file change.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified\n"), 0o644))
	desc, err := um.redoDiscard(ctx, UndoAction{
		Type:     "discard",
		Metadata: map[string]string{"path": "README.md"},
	})
	require.NoError(t, err)
	assert.Contains(t, desc, "README.md")

	// Verify the file was reverted.
	content, err := os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Test\n", string(content))
}

// ---------------------------------------------------------------------------
// redoReset — zero-coverage
// ---------------------------------------------------------------------------

func TestUndoManager_RedoReset(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	um := NewUndoManager(client)

	// Missing metadata → error.
	_, err = um.redoReset(ctx, UndoAction{
		Type:     "reset",
		Metadata: map[string]string{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing ref or mode")

	// Missing mode → error.
	_, err = um.redoReset(ctx, UndoAction{
		Type:     "reset",
		Metadata: map[string]string{"ref": "HEAD"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing ref or mode")

	// Valid redo-reset to HEAD with soft mode.
	desc, err := um.redoReset(ctx, UndoAction{
		Type:     "reset",
		Metadata: map[string]string{"ref": "HEAD", "mode": "soft"},
	})
	require.NoError(t, err)
	assert.Contains(t, desc, "soft")
}
