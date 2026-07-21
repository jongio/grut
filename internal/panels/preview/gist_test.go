package preview

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
