package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gh "github.com/google/go-github/v88/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// GetJobLogs — redirect limit enforcement
// ---------------------------------------------------------------------------

func TestClient_GetJobLogs_NetworkError(t *testing.T) {
	// GetJobLogs builds its own http.Client to fetch the log URL.
	// When the log URL points to an unreachable host, the fetch should fail
	// and the error should be wrapped with "fetch job N logs".
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/actions/jobs/77/logs" {
			// Redirect to a URL that won't resolve — closed port.
			http.Redirect(w, r, "http://127.0.0.1:1/unreachable", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	ctx := context.Background()
	_, err := client.GetJobLogs(ctx, "owner", "repo", 77)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch job 77 logs")
}

func TestClient_GetJobLogs_OversizedLog(t *testing.T) {
	// Serve a log body that exceeds 10 MiB. The reader uses
	// io.LimitReader(resp.Body, 10<<20), so it should truncate.
	const maxLogSize = 10 << 20 // must match production constant
	bigContent := strings.Repeat("A", maxLogSize+1024)

	logsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(bigContent))
	}))
	t.Cleanup(logsServer.Close)

	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/actions/jobs/88/logs" {
			http.Redirect(w, r, logsServer.URL+"/logs.txt", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	ctx := context.Background()
	logs, err := client.GetJobLogs(ctx, "owner", "repo", 88)
	require.NoError(t, err)
	assert.Equal(t, maxLogSize, len(logs), "log should be truncated to maxLogSize")
}

func TestClient_GetJobLogs_NilURL(t *testing.T) {
	// When the API returns an error status for job logs, go-github
	// returns an error before we reach the nil URL check.
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/actions/jobs/55/logs" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})

	ctx := context.Background()
	_, err := client.GetJobLogs(ctx, "owner", "repo", 55)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get job 55 logs")
}

// ---------------------------------------------------------------------------
// ListWorkflows — pagination aggregation
// ---------------------------------------------------------------------------

func TestClient_ListWorkflows_MultiPage(t *testing.T) {
	page := 0
	var srvURL string
	client, srv := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/actions/workflows" {
			http.NotFound(w, r)
			return
		}
		page++
		switch page {
		case 1:
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/owner/repo/actions/workflows?page=2>; rel="next"`, srvURL))
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(&gh.Workflows{
				TotalCount: ptr(3),
				Workflows: []*gh.Workflow{
					{ID: ptr(int64(1)), Name: ptr("CI")},
					{ID: ptr(int64(2)), Name: ptr("Deploy")},
				},
			})
		case 2:
			respondJSON(w, http.StatusOK, &gh.Workflows{
				TotalCount: ptr(3),
				Workflows: []*gh.Workflow{
					{ID: ptr(int64(3)), Name: ptr("Release")},
				},
			})
		default:
			t.Fatal("unexpected page request beyond 2")
		}
	})
	srvURL = srv.URL
	_ = client

	ctx := context.Background()
	workflows, err := client.ListWorkflows(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	require.Len(t, workflows, 3)
	assert.Equal(t, "CI", workflows[0].GetName())
	assert.Equal(t, "Deploy", workflows[1].GetName())
	assert.Equal(t, "Release", workflows[2].GetName())
}

func TestClient_ListWorkflows_CacheHit(t *testing.T) {
	callCount := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		http.NotFound(w, r)
	})

	cached := []*gh.Workflow{
		{ID: ptr(int64(1)), Name: ptr("Cached")},
	}
	local := gh.ListOptions{}
	local.Page = 0
	key := fmt.Sprintf("workflows:%s/%s:%+v", "owner", "repo", local)
	client.cache.Set(key, cached)

	ctx := context.Background()
	workflows, err := client.ListWorkflows(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	require.Len(t, workflows, 1)
	assert.Equal(t, "Cached", workflows[0].GetName())
	assert.Equal(t, 0, callCount)
}

func TestClient_ListWorkflows_CacheTypeError(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	local := gh.ListOptions{}
	local.Page = 0
	key := fmt.Sprintf("workflows:%s/%s:%+v", "owner", "repo", local)
	client.cache.Set(key, "wrong type")

	ctx := context.Background()
	_, err := client.ListWorkflows(ctx, "owner", "repo", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected cache type")
}

func TestClient_ListWorkflows_APIError(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"message": "Internal Server Error",
		})
	})

	ctx := context.Background()
	_, err := client.ListWorkflows(ctx, "owner", "repo", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list workflows")
}

// ---------------------------------------------------------------------------
// ListReleases — pagination aggregation
// ---------------------------------------------------------------------------

func TestClient_ListReleases_MultiPage(t *testing.T) {
	page := 0
	var srvURL string
	client, srv := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases" {
			http.NotFound(w, r)
			return
		}
		page++
		switch page {
		case 1:
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/owner/repo/releases?page=2>; rel="next"`, srvURL))
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]*gh.RepositoryRelease{
				{ID: ptr(int64(1)), TagName: ptr("v1.0.0")},
				{ID: ptr(int64(2)), TagName: ptr("v1.1.0")},
			})
		case 2:
			respondJSON(w, http.StatusOK, []*gh.RepositoryRelease{
				{ID: ptr(int64(3)), TagName: ptr("v2.0.0")},
			})
		default:
			t.Fatal("unexpected page request beyond 2")
		}
	})
	srvURL = srv.URL
	_ = client

	ctx := context.Background()
	releases, err := client.ListReleases(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	require.Len(t, releases, 3)
	assert.Equal(t, "v1.0.0", releases[0].GetTagName())
	assert.Equal(t, "v1.1.0", releases[1].GetTagName())
	assert.Equal(t, "v2.0.0", releases[2].GetTagName())
}

func TestClient_ListReleases_CacheHit(t *testing.T) {
	callCount := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		http.NotFound(w, r)
	})

	cached := []*gh.RepositoryRelease{
		{ID: ptr(int64(1)), TagName: ptr("cached-tag")},
	}
	local := gh.ListOptions{}
	local.Page = 0
	key := fmt.Sprintf("releases:%s/%s:%+v", "owner", "repo", local)
	client.cache.Set(key, cached)

	ctx := context.Background()
	releases, err := client.ListReleases(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	require.Len(t, releases, 1)
	assert.Equal(t, "cached-tag", releases[0].GetTagName())
	assert.Equal(t, 0, callCount)
}

func TestClient_ListReleases_CacheTypeError(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	local := gh.ListOptions{}
	local.Page = 0
	key := fmt.Sprintf("releases:%s/%s:%+v", "owner", "repo", local)
	client.cache.Set(key, 42)

	ctx := context.Background()
	_, err := client.ListReleases(ctx, "owner", "repo", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected cache type")
}

func TestClient_ListReleases_APIError(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"message": "Internal Server Error",
		})
	})

	ctx := context.Background()
	_, err := client.ListReleases(ctx, "owner", "repo", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list releases")
}

// ---------------------------------------------------------------------------
// GetPRDiff — happy-path success test
// ---------------------------------------------------------------------------

func TestClient_GetPRDiff_Success(t *testing.T) {
	expectedDiff := `diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
`
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		// go-github's PullRequests.GetRaw sends GET to /repos/:owner/:repo/pulls/:number
		// with Accept: application/vnd.github.v3.diff
		if r.URL.Path == "/repos/owner/repo/pulls/7" {
			accept := r.Header.Get("Accept")
			if strings.Contains(accept, "diff") {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(expectedDiff))
				return
			}
		}
		http.NotFound(w, r)
	})

	ctx := context.Background()
	diff, err := client.GetPRDiff(ctx, "owner", "repo", 7)
	require.NoError(t, err)
	assert.Equal(t, expectedDiff, diff)

	// Second call should use cache.
	diff2, err := client.GetPRDiff(ctx, "owner", "repo", 7)
	require.NoError(t, err)
	assert.Equal(t, expectedDiff, diff2)
}

func TestClient_GetPRDiff_APIError(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, map[string]string{
			"message": "Not Found",
		})
	})

	ctx := context.Background()
	_, err := client.GetPRDiff(ctx, "owner", "repo", 999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get PR #999 diff")
}

// ---------------------------------------------------------------------------
// RerunWorkflow — success and cache invalidation
// ---------------------------------------------------------------------------

func TestClient_RerunWorkflow_Success(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/actions/runs/200/rerun", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusCreated)
	})

	// Pre-populate caches that should be invalidated.
	client.cache.Set("run:owner/repo:200", "run-data")
	client.cache.Set("jobs:owner/repo:200", "jobs-data")

	ctx := context.Background()
	err := client.RerunWorkflow(ctx, "owner", "repo", 200)
	require.NoError(t, err)

	// Verify caches were invalidated.
	_, ok := client.cache.Get("run:owner/repo:200")
	assert.False(t, ok, "run cache should be invalidated after rerun")
	_, ok = client.cache.Get("jobs:owner/repo:200")
	assert.False(t, ok, "jobs cache should be invalidated after rerun")
}

func TestClient_RerunWorkflow_APIError(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, map[string]string{
			"message": "Not Found",
		})
	})

	ctx := context.Background()
	err := client.RerunWorkflow(ctx, "owner", "repo", 404)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rerun workflow run 404")
}

// ---------------------------------------------------------------------------
// GetReleaseByTag — success and error cases
// ---------------------------------------------------------------------------

func TestClient_GetReleaseByTag_Success(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/releases/tags/v1.2.3", r.URL.Path)
		respondJSON(w, http.StatusOK, &gh.RepositoryRelease{
			ID:      ptr(int64(42)),
			TagName: ptr("v1.2.3"),
			Name:    ptr("Release v1.2.3"),
		})
	})

	ctx := context.Background()
	release, err := client.GetReleaseByTag(ctx, "owner", "repo", "v1.2.3")
	require.NoError(t, err)
	assert.Equal(t, int64(42), release.GetID())
	assert.Equal(t, "v1.2.3", release.GetTagName())
	assert.Equal(t, "Release v1.2.3", release.GetName())

	// Second call should use cache.
	release2, err := client.GetReleaseByTag(ctx, "owner", "repo", "v1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", release2.GetTagName())
}

func TestClient_GetReleaseByTag_NotFound(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, map[string]string{
			"message": "Not Found",
		})
	})

	ctx := context.Background()
	_, err := client.GetReleaseByTag(ctx, "owner", "repo", "v99.99.99")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get release by tag v99.99.99")
}

func TestClient_GetReleaseByTag_CacheHit(t *testing.T) {
	callCount := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		http.NotFound(w, r)
	})

	cached := &gh.RepositoryRelease{ID: ptr(int64(10)), TagName: ptr("v0.1.0")}
	client.cache.Set("release-tag:owner/repo:v0.1.0", cached)

	ctx := context.Background()
	release, err := client.GetReleaseByTag(ctx, "owner", "repo", "v0.1.0")
	require.NoError(t, err)
	assert.Equal(t, "v0.1.0", release.GetTagName())
	assert.Equal(t, 0, callCount)
}

func TestClient_GetReleaseByTag_CacheTypeError(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	client.cache.Set("release-tag:owner/repo:v1.0.0", "wrong type")

	ctx := context.Background()
	_, err := client.GetReleaseByTag(ctx, "owner", "repo", "v1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected cache type")
}

// ---------------------------------------------------------------------------
// RerunFailedJobs — cache invalidation verification
// ---------------------------------------------------------------------------

func TestClient_RerunFailedJobs_CacheInvalidation(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/actions/runs/300/rerun-failed-jobs", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusCreated)
	})

	// Pre-populate caches.
	client.cache.Set("run:owner/repo:300", "run-data")
	client.cache.Set("jobs:owner/repo:300", "jobs-data")

	ctx := context.Background()
	err := client.RerunFailedJobs(ctx, "owner", "repo", 300)
	require.NoError(t, err)

	_, ok := client.cache.Get("run:owner/repo:300")
	assert.False(t, ok, "run cache should be invalidated")
	_, ok = client.cache.Get("jobs:owner/repo:300")
	assert.False(t, ok, "jobs cache should be invalidated")
}

// ---------------------------------------------------------------------------
// GetRelease — success path
// ---------------------------------------------------------------------------

func TestClient_GetRelease_Success(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/releases/50", r.URL.Path)
		respondJSON(w, http.StatusOK, &gh.RepositoryRelease{
			ID:      ptr(int64(50)),
			TagName: ptr("v3.0.0"),
			Name:    ptr("Major Release"),
		})
	})

	ctx := context.Background()
	release, err := client.GetRelease(ctx, "owner", "repo", 50)
	require.NoError(t, err)
	assert.Equal(t, int64(50), release.GetID())
	assert.Equal(t, "v3.0.0", release.GetTagName())
}

func TestClient_GetRelease_CacheHit(t *testing.T) {
	callCount := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		http.NotFound(w, r)
	})

	cached := &gh.RepositoryRelease{ID: ptr(int64(50)), TagName: ptr("cached-release")}
	client.cache.Set("release:owner/repo:50", cached)

	ctx := context.Background()
	release, err := client.GetRelease(ctx, "owner", "repo", 50)
	require.NoError(t, err)
	assert.Equal(t, "cached-release", release.GetTagName())
	assert.Equal(t, 0, callCount)
}

func TestClient_GetRelease_CacheTypeError(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	client.cache.Set("release:owner/repo:50", 42)

	ctx := context.Background()
	_, err := client.GetRelease(ctx, "owner", "repo", 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected cache type")
}
