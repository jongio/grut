package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// DeleteBranch — 0% → tested
// ---------------------------------------------------------------------------

func TestClient_DeleteBranch_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		branch string
	}{
		{name: "simple branch name", branch: "feature-x"},
		{name: "slash in branch name", branch: "user/feature"},
		{name: "long branch name", branch: "very-long-branch-name-with-many-parts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var deletedRef string
			client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deletedRef = r.URL.Path
					w.WriteHeader(http.StatusNoContent)
					return
				}
				http.NotFound(w, r)
			})

			err := client.DeleteBranch(context.Background(), "owner", "repo", tt.branch)
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("/repos/owner/repo/git/refs/heads/%s", tt.branch), deletedRef)
		})
	}
}

func TestClient_DeleteBranch_APIError(t *testing.T) {
	t.Parallel()
	client, _ := setupMockClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message": "Reference does not exist"}`))
	})

	err := client.DeleteBranch(context.Background(), "owner", "repo", "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `delete branch "nonexistent"`)
}

// ---------------------------------------------------------------------------
// GetJobLogs — redirect limit enforcement (too many redirects)
// ---------------------------------------------------------------------------

func TestClient_GetJobLogs_TooManyRedirects(t *testing.T) {
	t.Parallel()

	// Create a server that always redirects to itself, exceeding the 5-redirect limit.
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.String(), http.StatusFound)
	}))
	t.Cleanup(redirectServer.Close)

	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/actions/jobs/42/logs" {
			// Redirect to the infinite-redirect server (uses HTTPS scheme to pass scheme check).
			http.Redirect(w, r, redirectServer.URL+"/logs", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	_, err := client.GetJobLogs(context.Background(), "owner", "repo", 42)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch job 42 logs")
}

// ---------------------------------------------------------------------------
// GetJobLogs — successful fetch and caching
// ---------------------------------------------------------------------------

func TestClient_GetJobLogs_SuccessfulFetchIsCached(t *testing.T) {
	t.Parallel()
	const logContent = "2024-01-01 build succeeded\n"

	logsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(logContent))
	}))
	t.Cleanup(logsServer.Close)

	callCount := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/actions/jobs/55/logs" {
			callCount++
			http.Redirect(w, r, logsServer.URL+"/logs.txt", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	ctx := context.Background()

	// First call fetches from API.
	logs, err := client.GetJobLogs(ctx, "owner", "repo", 55)
	require.NoError(t, err)
	assert.Equal(t, logContent, logs)
	assert.Equal(t, 1, callCount)

	// Second call should hit cache.
	logs2, err := client.GetJobLogs(ctx, "owner", "repo", 55)
	require.NoError(t, err)
	assert.Equal(t, logContent, logs2)
	assert.Equal(t, 1, callCount, "should not call API again due to cache")
}
