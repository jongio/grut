package preview

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jongio/grut/internal/git/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateGist_NoOpWhenNoFile(t *testing.T) {
	p := newTestPreview([]string{"a"})
	p.filePath = ""
	_, cmd := p.createGist()
	assert.Nil(t, cmd)
}

func TestCreateGist_NoOpInGitHubMode(t *testing.T) {
	p := newTestPreview([]string{"a"})
	p.filePath = "main.go"
	p.ghMode = true
	_, cmd := p.createGist()
	assert.Nil(t, cmd)
}

func TestCreateGist_ReturnsCommandForLocalFile(t *testing.T) {
	p := newTestPreview([]string{"a"})
	p.filePath = "main.go"
	p.ghMode = false
	_, cmd := p.createGist()
	// A local on-disk file yields a command; it is not executed here because
	// doing so would shell out to the gh CLI and create a real gist.
	assert.NotNil(t, cmd)
}

func repoRootMock(root string) *gittest.MockClient {
	return &gittest.MockClient{
		RepoRootFunc: func(context.Context) (string, error) { return root, nil },
	}
}

func TestGistPathWithinRepo_RegularFileNoRepoAllowed(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(file, []byte("hi"), 0o644))
	assert.NoError(t, gistPathWithinRepo(context.Background(), nil, file))
}

func TestGistPathWithinRepo_SymlinkNoRepoBlocked(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(target, []byte("secret"), 0o600))
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}
	err := gistPathWithinRepo(context.Background(), nil, link)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

func TestGistPathWithinRepo_FileInsideRepoAllowed(t *testing.T) {
	repo := t.TempDir()
	file := filepath.Join(repo, "src", "main.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
	require.NoError(t, os.WriteFile(file, []byte("package main"), 0o644))
	assert.NoError(t, gistPathWithinRepo(context.Background(), repoRootMock(repo), file))
}

func TestGistPathWithinRepo_SymlinkEscapingRepoBlocked(t *testing.T) {
	repo := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "id_rsa")
	require.NoError(t, os.WriteFile(secret, []byte("KEY"), 0o600))
	link := filepath.Join(repo, "innocent.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}
	err := gistPathWithinRepo(context.Background(), repoRootMock(repo), link)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the repository")
}
