package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// FormatPatch
// ---------------------------------------------------------------------------

func TestClient_FormatPatch(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("hello patch\n"), 0o644))
	require.NoError(t, client.Stage(ctx, []string{"feature.txt"}))
	_, err = client.Commit(ctx, "Add a shiny feature", CommitOpts{})
	require.NoError(t, err)

	patch, err := client.FormatPatch(ctx, "HEAD")
	require.NoError(t, err)

	// A format-patch mailbox begins with a "From " line and carries the
	// commit subject plus the diff of the change.
	assert.True(t, strings.HasPrefix(patch, "From "), "patch should start with a From line")
	assert.Contains(t, patch, "Subject: [PATCH] Add a shiny feature")
	assert.Contains(t, patch, "feature.txt")
	assert.Contains(t, patch, "+hello patch")
}

func TestClient_FormatPatchValidation(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	ctx := context.Background()
	client, err := NewClient(dir)
	require.NoError(t, err)

	tests := []struct {
		name    string
		hash    string
		wantErr string
	}{
		{name: "empty hash", hash: "", wantErr: "format-patch hash"},
		{name: "invalid ref with shell chars", hash: "abc;rm", wantErr: "format-patch hash"},
		{name: "leading dash", hash: "--stdout", wantErr: "format-patch hash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := client.FormatPatch(ctx, tt.hash)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
