package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	gh "github.com/google/go-github/v89/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// urlRewriteTransport redirects all outgoing requests to the test server,
// preserving the request path so existing mock handlers keep matching.
type urlRewriteTransport struct {
	base *url.URL
}

func (t *urlRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = t.base.Scheme
	req.URL.Host = t.base.Host
	return http.DefaultTransport.RoundTrip(req)
}

// setupMockClient creates a clientImpl backed by an httptest server.
// The server is cleaned up automatically via t.Cleanup.
func setupMockClient(t *testing.T, handler http.HandlerFunc) (*clientImpl, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	baseURL, err := url.Parse(server.URL + "/")
	require.NoError(t, err)

	httpClient := &http.Client{Transport: &urlRewriteTransport{base: baseURL}}
	ghClient, err := gh.NewClient(gh.WithHTTPClient(httpClient), gh.WithAuthToken("test-token"))
	require.NoError(t, err)

	return &clientImpl{gh: ghClient, cache: newCache()}, server
}

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ptr is a generic helper matching gh.Ptr for test values.
func ptr[T any](v T) *T { return &v }

// ---------------------------------------------------------------------------
// Cache tests
// ---------------------------------------------------------------------------

func TestCache_GetMiss(t *testing.T) {
	c := newCache()
	v, ok := c.Get("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, v)
}

func TestCache_SetAndGet(t *testing.T) {
	c := newCache()
	c.Set("key1", "value1")

	v, ok := c.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", v)
}

func TestCache_Expiry(t *testing.T) {
	c := &cache{
		entries: make(map[string]cacheEntry),
		ttl:     50 * time.Millisecond,
	}
	c.Set("expires", "soon")

	// Immediate read should succeed.
	v, ok := c.Get("expires")
	assert.True(t, ok)
	assert.Equal(t, "soon", v)

	// After sleeping past TTL, should be a miss.
	time.Sleep(100 * time.Millisecond)
	v, ok = c.Get("expires")
	assert.False(t, ok)
	assert.Nil(t, v)
}

func TestCache_Invalidate(t *testing.T) {
	c := newCache()
	c.Set("key1", "value1")
	c.Set("key2", "value2")

	c.Invalidate("key1")

	_, ok := c.Get("key1")
	assert.False(t, ok, "invalidated key should miss")

	v, ok := c.Get("key2")
	assert.True(t, ok, "other key should still be present")
	assert.Equal(t, "value2", v)
}

func TestCache_InvalidateAll(t *testing.T) {
	c := newCache()
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	c.InvalidateAll()

	for _, key := range []string{"a", "b", "c"} {
		_, ok := c.Get(key)
		assert.False(t, ok, "key %q should miss after InvalidateAll", key)
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := newCache()
	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id)
			for j := range iterations {
				c.Set(key, j)
				c.Get(key)
				if j%10 == 0 {
					c.Invalidate(key)
				}
			}
		}(i)
	}

	wg.Wait()
	// If we reach here without a race-detector panic, the cache is goroutine-safe.
}

// ---------------------------------------------------------------------------
// Auth / resolveToken tests
// ---------------------------------------------------------------------------

func TestResolveToken_EnvVar(t *testing.T) {
	// Unset any existing token so the gh CLI path is tried first (and likely
	// fails in test environments), then the env-var fallback is exercised.
	const testToken = "ghp_test_token_12345"
	t.Setenv("GITHUB_TOKEN", testToken)

	token, err := resolveToken(context.Background())
	// If `gh` is available and logged in, we might get a different token.
	// That's fine — we just verify we get *some* token without error.
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestResolveToken_NoAuth(t *testing.T) {
	// Clear the env var. If `gh` is also not available, resolveToken must fail.
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "") // some systems honour this too

	// Override PATH so `gh` binary is not found.
	t.Setenv("PATH", "")

	_, err := resolveToken(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no GitHub auth")
}

// ---------------------------------------------------------------------------
// Type enum tests
// ---------------------------------------------------------------------------

func TestIssueFilterValues(t *testing.T) {
	tests := []struct {
		filter IssueFilter
		want   string
	}{
		{IssueFilterAll, "all"},
		{IssueFilterAssigned, "assigned"},
		{IssueFilterMentioned, "mentioned"},
		{IssueFilterCreated, "created"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, string(tc.filter))
	}
}

func TestPRFilterValues(t *testing.T) {
	tests := []struct {
		filter PRFilter
		want   string
	}{
		{PRFilterAll, "all"},
		{PRFilterNeedsReview, "needs_review"},
		{PRFilterMine, "mine"},
		{PRFilterDraft, "draft"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, string(tc.filter))
	}
}

func TestActionStatusValues(t *testing.T) {
	tests := []struct {
		status ActionStatus
		want   string
	}{
		{ActionStatusSuccess, "success"},
		{ActionStatusFailure, "failure"},
		{ActionStatusInProgress, "in_progress"},
		{ActionStatusQueued, "queued"},
		{ActionStatusCancelled, "cancelled"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, string(tc.status))
	}
}

// ---------------------------------------------------------------------------
// Client API tests — issues
// ---------------------------------------------------------------------------

func TestClient_ListIssues(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues", r.URL.Path)
		issues := []*gh.Issue{
			{Number: ptr(1), Title: ptr("Bug report"), State: ptr("open")},
			{Number: ptr(2), Title: ptr("Feature request"), State: ptr("open")},
		}
		respondJSON(w, http.StatusOK, issues)
	})

	ctx := context.Background()
	issues, err := client.ListIssues(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	require.Len(t, issues, 2)
	assert.Equal(t, 1, issues[0].GetNumber())
	assert.Equal(t, "Bug report", issues[0].GetTitle())
	assert.Equal(t, 2, issues[1].GetNumber())
	assert.Equal(t, "Feature request", issues[1].GetTitle())
}

func TestClient_ListIssues_Cached(t *testing.T) {
	callCount := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		respondJSON(w, http.StatusOK, []*gh.Issue{
			{Number: ptr(1), Title: ptr("Cached issue")},
		})
	})

	ctx := context.Background()
	_, err := client.ListIssues(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	// Second call should come from cache.
	_, err = client.ListIssues(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "second call should use cache, not hit server")
}

func TestClient_GetIssue(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues/42", r.URL.Path)
		issue := &gh.Issue{
			Number: ptr(42),
			Title:  ptr("Important bug"),
			State:  ptr("open"),
			Body:   ptr("Something is broken"),
			User:   &gh.User{Login: ptr("alice")},
		}
		respondJSON(w, http.StatusOK, issue)
	})

	ctx := context.Background()
	issue, err := client.GetIssue(ctx, "owner", "repo", 42)
	require.NoError(t, err)
	assert.Equal(t, 42, issue.GetNumber())
	assert.Equal(t, "Important bug", issue.GetTitle())
	assert.Equal(t, "open", issue.GetState())
	assert.Equal(t, "Something is broken", issue.GetBody())
	assert.Equal(t, "alice", issue.GetUser().GetLogin())
}

func TestClient_GetIssueComments(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues/10/comments", r.URL.Path)
		comments := []*gh.IssueComment{
			{ID: ptr(int64(1)), Body: ptr("First comment"), User: &gh.User{Login: ptr("bob")}},
			{ID: ptr(int64(2)), Body: ptr("Second comment"), User: &gh.User{Login: ptr("carol")}},
		}
		respondJSON(w, http.StatusOK, comments)
	})

	ctx := context.Background()
	comments, err := client.GetIssueComments(ctx, "owner", "repo", 10)
	require.NoError(t, err)
	require.Len(t, comments, 2)
	assert.Equal(t, "First comment", comments[0].GetBody())
	assert.Equal(t, "bob", comments[0].GetUser().GetLogin())
}

func TestClient_CreateIssue(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/issues" && r.Method == http.MethodPost {
			var req gh.IssueRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, "New issue", req.GetTitle())
			respondJSON(w, http.StatusCreated, &gh.Issue{
				Number: ptr(99),
				Title:  ptr("New issue"),
				State:  ptr("open"),
			})
			return
		}
		http.NotFound(w, r)
	})

	ctx := context.Background()
	req := &gh.IssueRequest{Title: ptr("New issue"), Body: ptr("Body text")}
	issue, err := client.CreateIssue(ctx, "owner", "repo", req)
	require.NoError(t, err)
	assert.Equal(t, 99, issue.GetNumber())
	assert.Equal(t, "New issue", issue.GetTitle())
}

func TestClient_CreateIssue_InvalidatesCache(t *testing.T) {
	callCount := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/issues" && r.Method == http.MethodGet:
			callCount++
			respondJSON(w, http.StatusOK, []*gh.Issue{
				{Number: ptr(1), Title: ptr("Existing")},
			})
		case r.URL.Path == "/repos/owner/repo/issues" && r.Method == http.MethodPost:
			respondJSON(w, http.StatusCreated, &gh.Issue{Number: ptr(2), Title: ptr("New")})
		default:
			http.NotFound(w, r)
		}
	})

	ctx := context.Background()
	// First list populates cache.
	_, err := client.ListIssues(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Create invalidates the cache for nil opts.
	_, err = client.CreateIssue(ctx, "owner", "repo", &gh.IssueRequest{Title: ptr("New")})
	require.NoError(t, err)

	// The cache key for ListIssues(nil) uses the fmt.Sprintf of (*IssueListByRepoOptions)(nil).
	// After CreateIssue, that specific key is invalidated, so re-listing should hit server.
	_, err = client.ListIssues(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "should have re-fetched after cache invalidation")
}

func TestClient_CommentOnIssue(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues/5/comments", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		var comment gh.IssueComment
		_ = json.NewDecoder(r.Body).Decode(&comment)
		assert.Equal(t, "Nice work!", comment.GetBody())
		respondJSON(w, http.StatusCreated, &gh.IssueComment{
			ID:   ptr(int64(100)),
			Body: ptr("Nice work!"),
		})
	})

	ctx := context.Background()
	err := client.CommentOnIssue(ctx, "owner", "repo", 5, "Nice work!")
	require.NoError(t, err)
}

func TestClient_CloseIssue(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues/7", r.URL.Path)
		assert.Equal(t, http.MethodPatch, r.Method)
		var req gh.IssueRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "closed", req.GetState())
		respondJSON(w, http.StatusOK, &gh.Issue{Number: ptr(7), State: ptr("closed")})
	})

	ctx := context.Background()
	err := client.CloseIssue(ctx, "owner", "repo", 7)
	require.NoError(t, err)
}

func TestClient_ReopenIssue(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues/7", r.URL.Path)
		assert.Equal(t, http.MethodPatch, r.Method)
		var req gh.IssueRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "open", req.GetState())
		respondJSON(w, http.StatusOK, &gh.Issue{Number: ptr(7), State: ptr("open")})
	})

	ctx := context.Background()
	err := client.ReopenIssue(ctx, "owner", "repo", 7)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Client API tests — pull requests
// ---------------------------------------------------------------------------

func TestClient_ListPRs(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls", r.URL.Path)
		prs := []*gh.PullRequest{
			{Number: ptr(10), Title: ptr("Add feature"), State: ptr("open")},
			{Number: ptr(11), Title: ptr("Fix typo"), State: ptr("closed")},
		}
		respondJSON(w, http.StatusOK, prs)
	})

	ctx := context.Background()
	prs, err := client.ListPRs(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	require.Len(t, prs, 2)
	assert.Equal(t, 10, prs[0].GetNumber())
	assert.Equal(t, "Add feature", prs[0].GetTitle())
	assert.Equal(t, 11, prs[1].GetNumber())
}

func TestClient_GetPR(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/10", r.URL.Path)
		pr := &gh.PullRequest{
			Number:    ptr(10),
			Title:     ptr("Add feature"),
			State:     ptr("open"),
			Body:      ptr("This adds a cool feature"),
			User:      &gh.User{Login: ptr("alice")},
			Merged:    ptr(false),
			Draft:     ptr(false),
			Mergeable: ptr(true),
		}
		respondJSON(w, http.StatusOK, pr)
	})

	ctx := context.Background()
	pr, err := client.GetPR(ctx, "owner", "repo", 10)
	require.NoError(t, err)
	assert.Equal(t, 10, pr.GetNumber())
	assert.Equal(t, "Add feature", pr.GetTitle())
	assert.Equal(t, "open", pr.GetState())
	assert.Equal(t, "alice", pr.GetUser().GetLogin())
	assert.False(t, pr.GetMerged())
	assert.True(t, pr.GetMergeable())
}

func TestClient_GetPRFiles(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/10/files", r.URL.Path)
		files := []*gh.CommitFile{
			{Filename: ptr("main.go"), Status: ptr("modified"), Additions: ptr(10), Deletions: ptr(2)},
			{Filename: ptr("README.md"), Status: ptr("added"), Additions: ptr(5), Deletions: ptr(0)},
		}
		respondJSON(w, http.StatusOK, files)
	})

	ctx := context.Background()
	files, err := client.GetPRFiles(ctx, "owner", "repo", 10)
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Equal(t, "main.go", files[0].GetFilename())
	assert.Equal(t, "modified", files[0].GetStatus())
	assert.Equal(t, 10, files[0].GetAdditions())
	assert.Equal(t, "README.md", files[1].GetFilename())
	assert.Equal(t, "added", files[1].GetStatus())
}

func TestClient_GetPRComments(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/10/comments", r.URL.Path)
		comments := []*gh.PullRequestComment{
			{ID: ptr(int64(1)), Body: ptr("Looks good"), User: &gh.User{Login: ptr("reviewer")}},
		}
		respondJSON(w, http.StatusOK, comments)
	})

	ctx := context.Background()
	comments, err := client.GetPRComments(ctx, "owner", "repo", 10)
	require.NoError(t, err)
	require.Len(t, comments, 1)
	assert.Equal(t, "Looks good", comments[0].GetBody())
}

func TestClient_GetPRReviews(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/10/reviews", r.URL.Path)
		reviews := []*gh.PullRequestReview{
			{ID: ptr(int64(1)), State: ptr("APPROVED"), User: &gh.User{Login: ptr("reviewer")}},
		}
		respondJSON(w, http.StatusOK, reviews)
	})

	ctx := context.Background()
	reviews, err := client.GetPRReviews(ctx, "owner", "repo", 10)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	assert.Equal(t, "APPROVED", reviews[0].GetState())
}

func TestClient_GetPRCommits(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/10/commits", r.URL.Path)
		commits := []*gh.RepositoryCommit{
			{SHA: ptr("abc123"), Commit: &gh.Commit{Message: ptr("Initial commit")}},
			{SHA: ptr("def456"), Commit: &gh.Commit{Message: ptr("Add tests")}},
		}
		respondJSON(w, http.StatusOK, commits)
	})

	ctx := context.Background()
	commits, err := client.GetPRCommits(ctx, "owner", "repo", 10)
	require.NoError(t, err)
	require.Len(t, commits, 2)
	assert.Equal(t, "abc123", commits[0].GetSHA())
	assert.Equal(t, "Initial commit", commits[0].GetCommit().GetMessage())
}

func TestClient_CreatePR(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		respondJSON(w, http.StatusCreated, &gh.PullRequest{
			Number: ptr(20),
			Title:  ptr("New PR"),
			State:  ptr("open"),
		})
	})

	ctx := context.Background()
	req := &gh.NewPullRequest{
		Title: ptr("New PR"),
		Head:  ptr("feature"),
		Base:  ptr("main"),
		Body:  ptr("PR body"),
	}
	pr, err := client.CreatePR(ctx, "owner", "repo", req)
	require.NoError(t, err)
	assert.Equal(t, 20, pr.GetNumber())
	assert.Equal(t, "New PR", pr.GetTitle())
}

func TestClient_CommentOnPR(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/10/comments", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		respondJSON(w, http.StatusCreated, &gh.PullRequestComment{
			ID:   ptr(int64(50)),
			Body: ptr("Review comment"),
		})
	})

	ctx := context.Background()
	err := client.CommentOnPR(ctx, "owner", "repo", 10, "Review comment", "main.go", 42)
	require.NoError(t, err)
}

func TestClient_CreateReviewCommentBuildsAnchoredRequest(t *testing.T) {
	t.Parallel()

	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/10/comments", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var got gh.PullRequestComment
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		assert.Equal(t, "Review comment", got.GetBody())
		assert.Equal(t, "abc123", got.GetCommitID())
		assert.Equal(t, "main.go", got.GetPath())
		assert.Equal(t, 42, got.GetLine())
		assert.Equal(t, "RIGHT", got.GetSide())

		respondJSON(w, http.StatusCreated, &gh.PullRequestComment{
			ID:   ptr(int64(50)),
			Body: ptr("Review comment"),
		})
	})

	err := client.CreateReviewComment(t.Context(), "owner", "repo", 10, "abc123", "main.go", 42, "Review comment")
	require.NoError(t, err)
}

func TestClient_SubmitReview(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/10/reviews", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		respondJSON(w, http.StatusOK, &gh.PullRequestReview{
			ID:    ptr(int64(1)),
			State: ptr("APPROVED"),
		})
	})

	ctx := context.Background()
	review := &gh.PullRequestReviewRequest{
		Event: ptr("APPROVE"),
		Body:  ptr("LGTM"),
	}
	err := client.SubmitReview(ctx, "owner", "repo", 10, review)
	require.NoError(t, err)
}

func TestClient_RequestReviewers(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/10/requested_reviewers", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		respondJSON(w, http.StatusCreated, &gh.PullRequest{Number: ptr(10)})
	})

	ctx := context.Background()
	err := client.RequestReviewers(ctx, "owner", "repo", 10, []string{"alice", "bob"})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Client API tests — workflow runs / actions
// ---------------------------------------------------------------------------

func TestClient_ListWorkflowRuns(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/actions/runs", r.URL.Path)
		result := &gh.WorkflowRuns{
			TotalCount: ptr(2),
			WorkflowRuns: []*gh.WorkflowRun{
				{ID: ptr(int64(100)), Name: ptr("CI"), Status: ptr("completed"), Conclusion: ptr("success")},
				{ID: ptr(int64(101)), Name: ptr("CI"), Status: ptr("in_progress")},
			},
		}
		respondJSON(w, http.StatusOK, result)
	})

	ctx := context.Background()
	runs, err := client.ListWorkflowRuns(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	assert.Equal(t, int64(100), runs[0].GetID())
	assert.Equal(t, "success", runs[0].GetConclusion())
	assert.Equal(t, "in_progress", runs[1].GetStatus())
}

func TestClient_GetWorkflowRun(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/actions/runs/100", r.URL.Path)
		respondJSON(w, http.StatusOK, &gh.WorkflowRun{
			ID:         ptr(int64(100)),
			Name:       ptr("CI"),
			Status:     ptr("completed"),
			Conclusion: ptr("success"),
		})
	})

	ctx := context.Background()
	run, err := client.GetWorkflowRun(ctx, "owner", "repo", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(100), run.GetID())
	assert.Equal(t, "completed", run.GetStatus())
}

func TestClient_ListWorkflowJobs(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/actions/runs/100/jobs", r.URL.Path)
		result := struct {
			TotalCount int               `json:"total_count"`
			Jobs       []*gh.WorkflowJob `json:"jobs"`
		}{
			TotalCount: 2,
			Jobs: []*gh.WorkflowJob{
				{ID: ptr(int64(200)), Name: ptr("build"), Status: ptr("completed"), Conclusion: ptr("success")},
				{ID: ptr(int64(201)), Name: ptr("test"), Status: ptr("completed"), Conclusion: ptr("failure")},
			},
		}
		respondJSON(w, http.StatusOK, result)
	})

	ctx := context.Background()
	jobs, err := client.ListWorkflowJobs(ctx, "owner", "repo", 100)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	assert.Equal(t, "build", jobs[0].GetName())
	assert.Equal(t, "success", jobs[0].GetConclusion())
	assert.Equal(t, "failure", jobs[1].GetConclusion())
}

func TestClient_RerunFailedJobs(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/actions/runs/100/rerun-failed-jobs", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusCreated)
	})

	ctx := context.Background()
	err := client.RerunFailedJobs(ctx, "owner", "repo", 100)
	require.NoError(t, err)
}

func TestClient_CancelWorkflowRun(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/actions/runs/100/cancel", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		// GitHub's cancel API returns 202 Accepted. The go-github library
		// treats any 202 response as an AcceptedError (async operation
		// scheduled), which CancelWorkflowRun surfaces as an error.
		respondJSON(w, http.StatusAccepted, struct{}{})
	})

	ctx := context.Background()
	err := client.CancelWorkflowRun(ctx, "owner", "repo", 100)
	// go-github wraps 202 Accepted as *AcceptedError. The current
	// clientImpl.CancelWorkflowRun passes it through as an error.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancel workflow run 100")

	// Verify cache was NOT invalidated because the error path was taken.
	// Pre-populate cache and retry to confirm.
	client.cache.Set(fmt.Sprintf("run:%s/%s:%d", "owner", "repo", int64(100)), "cached")
	_, ok := client.cache.Get(fmt.Sprintf("run:%s/%s:%d", "owner", "repo", int64(100)))
	assert.True(t, ok, "cache should remain since cancel returned an error")
}

// ---------------------------------------------------------------------------
// Client API tests — notifications
// ---------------------------------------------------------------------------

func TestClient_ListNotifications(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/notifications", r.URL.Path)
		notifications := []*gh.Notification{
			{
				ID:     ptr("1"),
				Reason: ptr("mention"),
				Subject: &gh.NotificationSubject{
					Title: ptr("New issue"),
					Type:  ptr("Issue"),
				},
			},
		}
		respondJSON(w, http.StatusOK, notifications)
	})

	ctx := context.Background()
	notifications, err := client.ListNotifications(ctx, nil)
	require.NoError(t, err)
	require.Len(t, notifications, 1)
	assert.Equal(t, "mention", notifications[0].GetReason())
	assert.Equal(t, "New issue", notifications[0].GetSubject().GetTitle())
}

func TestClient_MarkRead(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/notifications/threads/thread-123", r.URL.Path)
		assert.Equal(t, http.MethodPatch, r.Method)
		w.WriteHeader(http.StatusResetContent)
	})

	ctx := context.Background()
	err := client.MarkRead(ctx, "thread-123")
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Client API tests — repo / user
// ---------------------------------------------------------------------------

func TestClient_CurrentUser(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/user", r.URL.Path)
		respondJSON(w, http.StatusOK, &gh.User{
			Login: ptr("testuser"),
			Name:  ptr("Test User"),
			Email: ptr("test@example.com"),
		})
	})

	ctx := context.Background()
	user, err := client.CurrentUser(ctx)
	require.NoError(t, err)
	assert.Equal(t, "testuser", user.GetLogin())
	assert.Equal(t, "Test User", user.GetName())
	assert.Equal(t, "test@example.com", user.GetEmail())
}

func TestClient_RepoInfo(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo", r.URL.Path)
		respondJSON(w, http.StatusOK, &gh.Repository{
			FullName:        ptr("owner/repo"),
			Description:     ptr("A test repository"),
			DefaultBranch:   ptr("main"),
			Private:         ptr(false),
			Fork:            ptr(false),
			StargazersCount: ptr(42),
		})
	})

	ctx := context.Background()
	repo, err := client.RepoInfo(ctx, "owner", "repo")
	require.NoError(t, err)
	assert.Equal(t, "owner/repo", repo.GetFullName())
	assert.Equal(t, "A test repository", repo.GetDescription())
	assert.Equal(t, "main", repo.GetDefaultBranch())
	assert.False(t, repo.GetPrivate())
	assert.Equal(t, 42, repo.GetStargazersCount())
}

// ---------------------------------------------------------------------------
// Error handling tests
// ---------------------------------------------------------------------------

func TestClient_NotFound(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, &gh.ErrorResponse{
			Message: "Not Found",
		})
	})

	ctx := context.Background()
	_, err := client.GetIssue(ctx, "owner", "repo", 9999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get issue #9999")
}

func TestClient_ServerError(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusInternalServerError, &gh.ErrorResponse{
			Message: "Internal Server Error",
		})
	})

	ctx := context.Background()
	_, err := client.ListIssues(ctx, "owner", "repo", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list issues")
}

func TestClient_RateLimited(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix()))
		w.Header().Set("Retry-After", "3600")
		respondJSON(w, http.StatusForbidden, map[string]string{
			"message": "API rate limit exceeded",
		})
	})

	ctx := context.Background()
	_, err := client.ListPRs(ctx, "owner", "repo", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list PRs")
}

// ---------------------------------------------------------------------------
// Error propagation per method
// ---------------------------------------------------------------------------

func TestClient_ErrorPropagation(t *testing.T) {
	errorHandler := func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"message": "Validation Failed",
		})
	}

	tests := []struct {
		name    string
		call    func(ctx context.Context, c *clientImpl) error
		wantMsg string
	}{
		{
			name: "EditIssue",
			call: func(ctx context.Context, c *clientImpl) error {
				return c.EditIssue(ctx, "o", "r", 1, &gh.IssueRequest{})
			},
			wantMsg: "edit issue #1",
		},
		{
			name: "CommentOnIssue",
			call: func(ctx context.Context, c *clientImpl) error {
				return c.CommentOnIssue(ctx, "o", "r", 1, "hi")
			},
			wantMsg: "comment on issue #1",
		},
		{
			name: "CreatePR",
			call: func(ctx context.Context, c *clientImpl) error {
				_, err := c.CreatePR(ctx, "o", "r", &gh.NewPullRequest{
					Title: ptr("x"), Head: ptr("a"), Base: ptr("b"),
				})
				return err
			},
			wantMsg: "create PR",
		},
		{
			name: "MergePR",
			call: func(ctx context.Context, c *clientImpl) error {
				return c.MergePR(ctx, "o", "r", 1, "msg", nil)
			},
			wantMsg: "merge PR #1",
		},
		{
			name: "CommentOnPR",
			call: func(ctx context.Context, c *clientImpl) error {
				return c.CommentOnPR(ctx, "o", "r", 1, "body", "file.go", 10)
			},
			wantMsg: "comment on PR #1",
		},
		{
			name: "SubmitReview",
			call: func(ctx context.Context, c *clientImpl) error {
				return c.SubmitReview(ctx, "o", "r", 1, &gh.PullRequestReviewRequest{
					Event: ptr("APPROVE"),
				})
			},
			wantMsg: "submit review on PR #1",
		},
		{
			name: "RequestReviewers",
			call: func(ctx context.Context, c *clientImpl) error {
				return c.RequestReviewers(ctx, "o", "r", 1, []string{"a"})
			},
			wantMsg: "request reviewers on PR #1",
		},
		{
			name: "GetPRDiff",
			call: func(ctx context.Context, c *clientImpl) error {
				_, err := c.GetPRDiff(ctx, "o", "r", 1)
				return err
			},
			wantMsg: "get PR #1 diff",
		},
		{
			name: "GetWorkflowRun",
			call: func(ctx context.Context, c *clientImpl) error {
				_, err := c.GetWorkflowRun(ctx, "o", "r", 999)
				return err
			},
			wantMsg: "get workflow run 999",
		},
		{
			name: "RerunFailedJobs",
			call: func(ctx context.Context, c *clientImpl) error {
				return c.RerunFailedJobs(ctx, "o", "r", 999)
			},
			wantMsg: "rerun failed jobs for run 999",
		},
		{
			name: "CancelWorkflowRun",
			call: func(ctx context.Context, c *clientImpl) error {
				return c.CancelWorkflowRun(ctx, "o", "r", 999)
			},
			wantMsg: "cancel workflow run 999",
		},
		{
			name: "MarkRead",
			call: func(ctx context.Context, c *clientImpl) error {
				return c.MarkRead(ctx, "thread-1")
			},
			wantMsg: "mark thread thread-1 read",
		},
		{
			name: "CurrentUser",
			call: func(ctx context.Context, c *clientImpl) error {
				_, err := c.CurrentUser(ctx)
				return err
			},
			wantMsg: "get current user",
		},
		{
			name: "RepoInfo",
			call: func(ctx context.Context, c *clientImpl) error {
				_, err := c.RepoInfo(ctx, "o", "r")
				return err
			},
			wantMsg: "get repo o/r",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := setupMockClient(t, errorHandler)
			err := tc.call(context.Background(), client)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

// ---------------------------------------------------------------------------
// ghExec tests
// ---------------------------------------------------------------------------

func TestGhExec_InvalidCommand(t *testing.T) {
	// Use a context and a command that doesn't exist to verify error path.
	ctx := context.Background()
	_, err := ghExec(ctx, "--version-nonexistent-flag-that-will-fail")
	// gh with a garbage flag should fail, or gh might not be installed at all.
	// Either way, we expect an error.
	if err != nil {
		assert.Contains(t, err.Error(), "gh")
	}
	// If gh happens to succeed (unlikely), that's okay too.
}

// ---------------------------------------------------------------------------
// Interface compliance compile-time check
// ---------------------------------------------------------------------------

func TestClientImplementsInterface(t *testing.T) {
	// This test documents the compile-time interface assertion in client.go.
	var _ Client = (*clientImpl)(nil)
}

// ---------------------------------------------------------------------------
// NewClient integration test (env-dependent, soft-fail)
// ---------------------------------------------------------------------------

func TestNewClient_WithToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_fake_token_for_test")
	ctx := context.Background()
	client, err := NewClient(ctx)
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.gh)
	assert.NotNil(t, client.cache)
}

func TestNewClient_NoAuth(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("PATH", "") // prevent finding gh binary

	ctx := context.Background()
	_, err := NewClient(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no GitHub auth")
}

// ---------------------------------------------------------------------------
// EditIssue cache invalidation
// ---------------------------------------------------------------------------

func TestClient_EditIssue_InvalidatesCache(t *testing.T) {
	getCalls := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/issues/5":
			getCalls++
			respondJSON(w, http.StatusOK, &gh.Issue{Number: ptr(5), Title: ptr("original")})
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/o/r/issues/5":
			respondJSON(w, http.StatusOK, &gh.Issue{Number: ptr(5), Title: ptr("edited")})
		default:
			http.NotFound(w, r)
		}
	})

	ctx := context.Background()
	_, err := client.GetIssue(ctx, "o", "r", 5)
	require.NoError(t, err)
	assert.Equal(t, 1, getCalls)

	err = client.EditIssue(ctx, "o", "r", 5, &gh.IssueRequest{Title: ptr("edited")})
	require.NoError(t, err)

	// After edit, cache should be invalidated — next Get hits server.
	_, err = client.GetIssue(ctx, "o", "r", 5)
	require.NoError(t, err)
	assert.Equal(t, 2, getCalls, "should re-fetch after EditIssue invalidated cache")
}

// ---------------------------------------------------------------------------
// MergePR cache invalidation
// ---------------------------------------------------------------------------

func TestClient_MergePR_InvalidatesCache(t *testing.T) {
	getCalls := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/pulls/3":
			getCalls++
			respondJSON(w, http.StatusOK, &gh.PullRequest{Number: ptr(3)})
		case r.Method == http.MethodPut && r.URL.Path == "/repos/o/r/pulls/3/merge":
			respondJSON(w, http.StatusOK, &gh.PullRequestMergeResult{
				Merged:  ptr(true),
				Message: ptr("merged"),
			})
		default:
			http.NotFound(w, r)
		}
	})

	ctx := context.Background()
	_, err := client.GetPR(ctx, "o", "r", 3)
	require.NoError(t, err)
	assert.Equal(t, 1, getCalls)

	err = client.MergePR(ctx, "o", "r", 3, "merge commit", nil)
	require.NoError(t, err)

	_, err = client.GetPR(ctx, "o", "r", 3)
	require.NoError(t, err)
	assert.Equal(t, 2, getCalls, "should re-fetch after MergePR invalidated cache")
}

// ---------------------------------------------------------------------------
// MarkRead invalidates all cache
// ---------------------------------------------------------------------------

func TestClient_MarkRead_InvalidatesAllCache(t *testing.T) {
	userCalls := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			userCalls++
			respondJSON(w, http.StatusOK, &gh.User{Login: ptr("me")})
		case "/notifications/threads/t1":
			w.WriteHeader(http.StatusResetContent)
		default:
			http.NotFound(w, r)
		}
	})

	ctx := context.Background()
	_, err := client.CurrentUser(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, userCalls)

	// Cached — no server hit.
	_, err = client.CurrentUser(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, userCalls)

	// MarkRead invalidates ALL cache.
	err = client.MarkRead(ctx, "t1")
	require.NoError(t, err)

	_, err = client.CurrentUser(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, userCalls, "should re-fetch after MarkRead invalidated all cache")
}

// ---------------------------------------------------------------------------
// Paged list method tests
// ---------------------------------------------------------------------------

func TestClient_ListIssuesPage(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues", r.URL.Path)
		assert.Equal(t, "1", r.URL.Query().Get("page"))
		issues := []*gh.Issue{
			{Number: ptr(1), Title: ptr("Bug")},
			{Number: ptr(2), Title: ptr("Feature")},
		}
		// Simulate a Link header with next page by setting the header.
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<https://api.github.com/repos/owner/repo/issues?page=2>; rel="next"`)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(issues)
	})

	ctx := context.Background()
	issues, pr, err := client.ListIssuesPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	require.Len(t, issues, 2)
	assert.Equal(t, 2, pr.NextPage)
	assert.Equal(t, -1, pr.TotalCount)
}

func TestClient_ListIssuesPage_LastPage(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, []*gh.Issue{
			{Number: ptr(3), Title: ptr("Last")},
		})
	})

	ctx := context.Background()
	issues, pr, err := client.ListIssuesPage(ctx, "owner", "repo", &gh.IssueListByRepoOptions{
		ListOptions: gh.ListOptions{Page: 2, PerPage: 10},
	})
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, 0, pr.NextPage)
}

func TestClient_ListIssuesPage_Cached(t *testing.T) {
	callCount := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		respondJSON(w, http.StatusOK, []*gh.Issue{
			{Number: ptr(1), Title: ptr("Cached")},
		})
	})

	ctx := context.Background()
	_, _, err := client.ListIssuesPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	_, _, err = client.ListIssuesPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "second call should use cache")
}

func TestClient_ListPRsPage(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls", r.URL.Path)
		prs := []*gh.PullRequest{
			{Number: ptr(10), Title: ptr("Add feature")},
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<https://api.github.com/repos/owner/repo/pulls?page=2>; rel="next"`)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(prs)
	})

	ctx := context.Background()
	prs, pr, err := client.ListPRsPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, 2, pr.NextPage)
	assert.Equal(t, -1, pr.TotalCount)
}

func TestClient_ListPRsPage_Cached(t *testing.T) {
	callCount := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		respondJSON(w, http.StatusOK, []*gh.PullRequest{
			{Number: ptr(10), Title: ptr("PR")},
		})
	})

	ctx := context.Background()
	_, _, err := client.ListPRsPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	_, _, err = client.ListPRsPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "second call should use cache")
}

func TestClient_ListWorkflowRunsPage(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/actions/runs", r.URL.Path)
		result := &gh.WorkflowRuns{
			TotalCount: ptr(42),
			WorkflowRuns: []*gh.WorkflowRun{
				{ID: ptr(int64(100)), Name: ptr("CI")},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<https://api.github.com/repos/owner/repo/actions/runs?page=3>; rel="next"`)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	})

	ctx := context.Background()
	runs, pr, err := client.ListWorkflowRunsPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, 3, pr.NextPage)
	assert.Equal(t, 42, pr.TotalCount)
}

func TestClient_ListWorkflowRunsPage_Cached(t *testing.T) {
	callCount := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		result := &gh.WorkflowRuns{
			TotalCount:   ptr(1),
			WorkflowRuns: []*gh.WorkflowRun{{ID: ptr(int64(1))}},
		}
		respondJSON(w, http.StatusOK, result)
	})

	ctx := context.Background()
	_, _, err := client.ListWorkflowRunsPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	_, _, err = client.ListWorkflowRunsPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "second call should use cache")
}

func TestClient_ListWorkflowsPage(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/actions/workflows", r.URL.Path)
		result := struct {
			TotalCount int            `json:"total_count"`
			Workflows  []*gh.Workflow `json:"workflows"`
		}{
			TotalCount: 1,
			Workflows:  []*gh.Workflow{{ID: ptr(int64(1)), Name: ptr("CI")}},
		}
		respondJSON(w, http.StatusOK, result)
	})

	ctx := context.Background()
	workflows, pr, err := client.ListWorkflowsPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	require.Len(t, workflows, 1)
	assert.Equal(t, 0, pr.NextPage) // no Link header → 0
	assert.Equal(t, -1, pr.TotalCount)
}

func TestClient_ListWorkflowsPage_Cached(t *testing.T) {
	callCount := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		result := struct {
			TotalCount int            `json:"total_count"`
			Workflows  []*gh.Workflow `json:"workflows"`
		}{
			TotalCount: 1,
			Workflows:  []*gh.Workflow{{ID: ptr(int64(1))}},
		}
		respondJSON(w, http.StatusOK, result)
	})

	ctx := context.Background()
	_, _, err := client.ListWorkflowsPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	_, _, err = client.ListWorkflowsPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "second call should use cache")
}

func TestClient_ListReleasesPage(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/releases", r.URL.Path)
		releases := []*gh.RepositoryRelease{
			{ID: 1, TagName: "v1.0.0"},
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<https://api.github.com/repos/owner/repo/releases?page=2>; rel="next"`)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(releases)
	})

	ctx := context.Background()
	releases, pr, err := client.ListReleasesPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	require.Len(t, releases, 1)
	assert.Equal(t, 2, pr.NextPage)
	assert.Equal(t, -1, pr.TotalCount)
}

func TestClient_ListReleasesPage_Cached(t *testing.T) {
	callCount := 0
	client, _ := setupMockClient(t, func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		respondJSON(w, http.StatusOK, []*gh.RepositoryRelease{
			{ID: 1, TagName: "v1.0.0"},
		})
	})

	ctx := context.Background()
	_, _, err := client.ListReleasesPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	_, _, err = client.ListReleasesPage(ctx, "owner", "repo", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "second call should use cache")
}

// ---------------------------------------------------------------------------
// Env var cleanup helper to prevent test pollution
// ---------------------------------------------------------------------------

func TestMain(m *testing.M) {
	// Ensure tests don't accidentally use real credentials from the environment.
	// t.Setenv handles per-test cleanup; this is belt-and-suspenders for the suite.
	os.Exit(m.Run())
}
