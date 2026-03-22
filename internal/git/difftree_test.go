package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_DiffTreeFiles(t *testing.T) {
	dir := initTestRepo(t) // creates repo with README.md + initial commit
	c, err := NewClient(dir)
	require.NoError(t, err)

	// Get the initial commit hash.
	commits, err := c.Log(context.Background(), LogOpts{MaxCount: 1})
	require.NoError(t, err)
	require.Len(t, commits, 1)

	files, err := c.DiffTreeFiles(context.Background(), commits[0].Hash)
	require.NoError(t, err)
	assert.Equal(t, []string{"README.md"}, files)
}

func TestClient_DiffTreeFiles_MultipleFiles(t *testing.T) {
	dir := initTestRepo(t)
	c, err := NewClient(dir)
	require.NoError(t, err)

	// Add multiple files in a second commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("b\n"), 0o644))

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	run("add", ".")
	run("commit", "-m", "Add files")

	commits, err := c.Log(context.Background(), LogOpts{MaxCount: 1})
	require.NoError(t, err)
	require.Len(t, commits, 1)

	files, err := c.DiffTreeFiles(context.Background(), commits[0].Hash)
	require.NoError(t, err)
	assert.Contains(t, files, "a.txt")
	assert.Contains(t, files, "sub/b.txt")
	assert.Len(t, files, 2)
}

func TestClient_DiffTreeFiles_InvalidHash(t *testing.T) {
	dir := initTestRepo(t)
	c, err := NewClient(dir)
	require.NoError(t, err)

	_, err = c.DiffTreeFiles(context.Background(), "")
	assert.Error(t, err, "empty hash should fail validation")
}

func TestClient_DiffTreeFiles_NonexistentHash(t *testing.T) {
	dir := initTestRepo(t)
	c, err := NewClient(dir)
	require.NoError(t, err)

	_, err = c.DiffTreeFiles(context.Background(), "deadbeef1234567890abcdef1234567890abcdef")
	assert.Error(t, err, "nonexistent hash should fail")
}

// ---------------------------------------------------------------------------
// DiffFileNames tests
// ---------------------------------------------------------------------------

func TestClient_DiffFileNames(t *testing.T) {
	dir := initTestRepo(t) // creates repo with README.md + initial commit on default branch
	c, err := NewClient(dir)
	require.NoError(t, err)

	defaultBranch := detectDefaultBranch(t, dir)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	// Create a feature branch and add files.
	run("checkout", "-b", "feature/test-branch")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new-file.txt"), []byte("new\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pkg", "lib.go"), []byte("package pkg\n"), 0o644))
	run("add", ".")
	run("commit", "-m", "Add feature files")

	// DiffFileNames using three-dot syntax: default...feature
	files, err := c.DiffFileNames(context.Background(), defaultBranch, "feature/test-branch")
	require.NoError(t, err)
	assert.Contains(t, files, "new-file.txt")
	assert.Contains(t, files, "pkg/lib.go")
	assert.Len(t, files, 2)
}

func TestClient_DiffFileNames_EmptyDiff(t *testing.T) {
	dir := initTestRepo(t)
	c, err := NewClient(dir)
	require.NoError(t, err)

	defaultBranch := detectDefaultBranch(t, dir)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	// Create a branch with no changes.
	run("checkout", "-b", "feature/empty")

	files, err := c.DiffFileNames(context.Background(), defaultBranch, "feature/empty")
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestClient_DiffFileNames_InvalidRef(t *testing.T) {
	dir := initTestRepo(t)
	c, err := NewClient(dir)
	require.NoError(t, err)

	// Empty commitA.
	_, err = c.DiffFileNames(context.Background(), "", "main")
	assert.Error(t, err, "empty commitA should fail validation")

	// Empty commitB.
	_, err = c.DiffFileNames(context.Background(), "main", "")
	assert.Error(t, err, "empty commitB should fail validation")
}

// ---------------------------------------------------------------------------
// DiffFileNames security / injection tests
// ---------------------------------------------------------------------------

// TestClient_DiffFileNames_InjectionPatterns verifies that DiffFileNames
// rejects all common injection patterns through its ValidateRef calls.
// This is an integration-level gate: even though ValidateRef is tested
// independently in sanitize_test.go, these tests prove the validation is
// wired up in DiffFileNames and would catch regressions if someone
// accidentally removes the ValidateRef calls.
func TestClient_DiffFileNames_InjectionPatterns(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	c, err := NewClient(dir)
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name    string
		commitA string
		commitB string
		errMsg  string
	}{
		// Option injection (CWE-88): leading dashes interpreted as git flags.
		{
			name:    "commitA option injection --upload-pack",
			commitA: "--upload-pack=evil",
			commitB: "main",
			errMsg:  "must not start with '-'",
		},
		{
			name:    "commitB option injection --config",
			commitA: "main",
			commitB: "--config=core.gitProxy=evil",
			errMsg:  "must not start with '-'",
		},
		{
			name:    "commitA short flag -o",
			commitA: "-o",
			commitB: "main",
			errMsg:  "must not start with '-'",
		},

		// Shell metacharacter injection: semicolons, pipes, ampersands.
		{
			name:    "commitA semicolon injection",
			commitA: "main;rm -rf /",
			commitB: "HEAD",
			errMsg:  "forbidden character",
		},
		{
			name:    "commitB pipe injection",
			commitA: "HEAD",
			commitB: "main|cat /etc/passwd",
			errMsg:  "forbidden character",
		},
		{
			name:    "commitA ampersand injection",
			commitA: "main&evil",
			commitB: "HEAD",
			errMsg:  "forbidden character",
		},

		// Command substitution: $() and backtick.
		{
			name:    "commitA dollar-paren command substitution",
			commitA: "$(whoami)",
			commitB: "main",
			errMsg:  "forbidden character",
		},
		{
			name:    "commitB backtick command substitution",
			commitA: "main",
			commitB: "`whoami`",
			errMsg:  "forbidden character",
		},

		// Null byte injection (CWE-626): can truncate strings in C backends.
		{
			name:    "commitA null byte",
			commitA: "main\x00--evil",
			commitB: "HEAD",
			errMsg:  "null byte",
		},
		{
			name:    "commitB null byte",
			commitA: "HEAD",
			commitB: "feature\x00;rm -rf /",
			errMsg:  "null byte",
		},

		// Newline injection: could split commands in some contexts.
		{
			name:    "commitA newline injection",
			commitA: "main\nevil",
			commitB: "HEAD",
			errMsg:  "forbidden character",
		},

		// Redirect injection.
		{
			name:    "commitB redirect injection",
			commitA: "HEAD",
			commitB: "main>/tmp/evil",
			errMsg:  "forbidden character",
		},

		// Both refs malicious.
		{
			name:    "both refs malicious",
			commitA: "--evil",
			commitB: ";rm -rf /",
			errMsg:  "must not start with '-'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := c.DiffFileNames(ctx, tt.commitA, tt.commitB)
			require.Error(t, err, "DiffFileNames should reject injection pattern")
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

// TestClient_DiffFileNames_CancelledContext verifies that a cancelled context
// is handled gracefully (returns an error, does not hang).
func TestClient_DiffFileNames_CancelledContext(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	c, err := NewClient(dir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = c.DiffFileNames(ctx, "HEAD", "HEAD")
	assert.Error(t, err, "cancelled context should cause an error")
}

// ---------------------------------------------------------------------------
// parseNameOnlyOutput tests
// ---------------------------------------------------------------------------

func TestParseNameOnlyOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{name: "empty string", input: "", expected: []string{}},
		{name: "whitespace only", input: "  \n\n  ", expected: []string{}},
		{name: "single file", input: "main.go\n", expected: []string{"main.go"}},
		{name: "multiple files", input: "a.go\nb.go\nc.go\n", expected: []string{"a.go", "b.go", "c.go"}},
		{name: "with carriage returns", input: "a.go\r\nb.go\r\n", expected: []string{"a.go", "b.go"}},
		{name: "with blank lines", input: "a.go\n\nb.go\n\n", expected: []string{"a.go", "b.go"}},
		{name: "paths with dirs", input: "src/main.go\npkg/lib.go\n", expected: []string{"src/main.go", "pkg/lib.go"}},
		{name: "no trailing newline", input: "a.go", expected: []string{"a.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseNameOnlyOutput(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
