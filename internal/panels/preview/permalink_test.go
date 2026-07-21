package preview

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildPermalink_SingleLine(t *testing.T) {
	link := buildPermalink(
		"https://github.com/jongio/grut.git",
		"abc123",
		"internal/panels/preview/preview.go",
		42, 42,
	)
	assert.Equal(
		t,
		"https://github.com/jongio/grut/blob/abc123/internal/panels/preview/preview.go#L42",
		link,
	)
}

func TestBuildPermalink_Range(t *testing.T) {
	link := buildPermalink(
		"https://github.com/jongio/grut.git",
		"deadbeef",
		"main.go",
		10, 20,
	)
	assert.Equal(
		t,
		"https://github.com/jongio/grut/blob/deadbeef/main.go#L10-L20",
		link,
	)
}

func TestBuildPermalink_SSHRemote(t *testing.T) {
	link := buildPermalink(
		"git@github.com:jongio/grut.git",
		"abc123",
		"README.md",
		1, 1,
	)
	assert.Equal(
		t,
		"https://github.com/jongio/grut/blob/abc123/README.md#L1",
		link,
	)
}

func TestBuildPermalink_PathEscaping(t *testing.T) {
	link := buildPermalink(
		"https://github.com/jongio/grut.git",
		"abc123",
		"docs/my notes/file #1.md",
		3, 3,
	)
	assert.Equal(
		t,
		"https://github.com/jongio/grut/blob/abc123/docs/my%20notes/file%20%231.md#L3",
		link,
	)
}

func TestBuildPermalink_NoAnchorWhenLineZero(t *testing.T) {
	link := buildPermalink(
		"https://github.com/jongio/grut.git",
		"abc123",
		"main.go",
		0, 0,
	)
	assert.Equal(t, "https://github.com/jongio/grut/blob/abc123/main.go", link)
}

func TestBuildPermalink_NonGitHubRemote(t *testing.T) {
	link := buildPermalink(
		"https://gitlab.com/jongio/grut.git",
		"abc123",
		"main.go",
		5, 5,
	)
	assert.Empty(t, link)
}

func TestBuildPermalink_EmptyRemote(t *testing.T) {
	assert.Empty(t, buildPermalink("", "abc123", "main.go", 1, 1))
}

func TestBuildPermalink_EmptySHA(t *testing.T) {
	assert.Empty(t, buildPermalink("https://github.com/jongio/grut.git", "", "main.go", 1, 1))
}

func TestPermalinkLineRange_NoSelection(t *testing.T) {
	p := newTestPreview([]string{"a", "b", "c"})
	p.scrollY = 5
	start, end := p.permalinkLineRange()
	assert.Equal(t, 6, start)
	assert.Equal(t, 6, end)
}

func TestPermalinkLineRange_WithSelection(t *testing.T) {
	p := newTestPreview([]string{"a", "b", "c", "d"})
	p.selAnchor = &selPoint{Line: 1, Col: 0}
	p.selEnd = &selPoint{Line: 3, Col: 2}
	start, end := p.permalinkLineRange()
	assert.Equal(t, 2, start)
	assert.Equal(t, 4, end)
}

func TestCopyPermalink_NoOpWhenNoFile(t *testing.T) {
	p := newTestPreview([]string{"a"})
	p.filePath = ""
	_, cmd := p.copyPermalink()
	assert.Nil(t, cmd)
}

func TestCopyPermalink_NoOpInGitHubMode(t *testing.T) {
	p := newTestPreview([]string{"a"})
	p.filePath = "main.go"
	p.ghMode = true
	_, cmd := p.copyPermalink()
	assert.Nil(t, cmd)
}

func TestBuildLocalLocation_SingleLine(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "internal", "git", "status.go")

	location := buildLocalLocation(root, path, 42, 42)

	assert.Equal(t, "internal/git/status.go:42", location)
}

func TestBuildLocalLocation_Range(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "internal", "panels", "preview", "preview.go")

	location := buildLocalLocation(root, path, 10, 14)

	assert.Equal(t, "internal/panels/preview/preview.go:10-14", location)
}

func TestBuildLocalLocation_OutsideRepoFallsBackToAbsolutePath(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.go")

	location := buildLocalLocation(root, outside, 7, 7)

	assert.Equal(t, filepath.ToSlash(filepath.Clean(outside))+":7", location)
}

func TestOpenOnGitHub_NoOpWhenNoFile(t *testing.T) {
	p := newTestPreview([]string{"a"})
	p.filePath = ""
	_, cmd := p.openOnGitHub()
	assert.Nil(t, cmd)
}

func TestOpenOnGitHub_NoOpInGitHubMode(t *testing.T) {
	p := newTestPreview([]string{"a"})
	p.filePath = "main.go"
	p.ghMode = true
	_, cmd := p.openOnGitHub()
	assert.Nil(t, cmd)
}
