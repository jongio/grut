package ops

import (
	"context"
	"errors"
	"fmt"

	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/git/gittest"
)

// errMockProvider is a reusable sentinel error for mock provider tests.
var errMockProvider = errors.New("mock provider error")

// ---------------------------------------------------------------------------
// Mock AI provider
// ---------------------------------------------------------------------------

// mockAIProvider is a configurable test double for ai.AIProvider.
// It returns preconfigured responses and is used across all ops tests.
type mockAIProvider struct {
	name      string
	available bool
	response  ai.CompletionResponse
	err       error
}

var _ ai.AIProvider = (*mockAIProvider)(nil)

func (m *mockAIProvider) Name() string { return m.name }
func (m *mockAIProvider) Available(_ context.Context) (bool, error) {
	return m.available, nil
}

func (m *mockAIProvider) Complete(_ context.Context, _ ai.CompletionRequest) (ai.CompletionResponse, error) {
	return m.response, m.err
}

func (m *mockAIProvider) CompleteStream(_ context.Context, _ ai.CompletionRequest) (<-chan ai.StreamChunk, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockAIProvider) Close() error { return nil }

// ---------------------------------------------------------------------------
// Mock git client
// ---------------------------------------------------------------------------

type mockGitClient = gittest.MockClient

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newMockGitClient returns a mock configured with a current branch and repo root.
func newMockGitClient(branch, repoRoot string) *mockGitClient {
	return &mockGitClient{
		BranchListFunc: func(_ context.Context) ([]git.Branch, error) {
			return []git.Branch{{Name: branch, IsCurrent: true}}, nil
		},
		RepoRootFunc: func(_ context.Context) (string, error) {
			return repoRoot, nil
		},
	}
}
