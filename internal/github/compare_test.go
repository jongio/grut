package github

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCompareCommitsRequestConstruction(t *testing.T) {
	t.Parallel()

	var gotPath string
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		respondJSON(w, http.StatusOK, map[string]any{
			"total_commits": 1,
			"commits": []map[string]any{
				{
					"sha": "abc123",
					"commit": map[string]any{
						"message": "release change",
					},
				},
			},
			"files": []map[string]any{
				{
					"filename":  "README.md",
					"status":    "modified",
					"additions": 7,
					"deletions": 2,
					"changes":   9,
				},
			},
		})
	})

	comparison, err := client.CompareCommits(context.Background(), "owner", "repo", "v1.0.0", "v1.1.0")
	require.NoError(t, err)

	assert.Equal(t, "/repos/owner/repo/compare/v1.0.0...v1.1.0", gotPath)
	assert.Equal(t, 1, comparison.GetTotalCommits())
	require.Len(t, comparison.Files, 1)
	assert.Equal(t, "README.md", comparison.Files[0].GetFilename())
	assert.Equal(t, 7, comparison.Files[0].GetAdditions())
	assert.Equal(t, 2, comparison.Files[0].GetDeletions())
}
