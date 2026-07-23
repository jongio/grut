package preview

import (
	"strings"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubmoduleSelectedPreview(t *testing.T) {
	t.Parallel()
	p := New(config.PreviewConfig{}, config.EditorConfig{}, nil)
	updated, cmd := p.Update(panels.SubmoduleSelectedMsg{
		Path:     "deps/module",
		Commit:   "0123456789abcdef0123456789abcdef01234567",
		State:    "clean",
		Describe: "v1.0.0",
	})
	require.Nil(t, cmd)
	preview, ok := updated.(*Preview)
	require.True(t, ok)
	assert.True(t, preview.ghMode)
	assert.Equal(t, "Submodule deps/module", preview.ghTitle)
	content := strings.Join(preview.lines, "\n")
	assert.Contains(t, content, "deps/module")
	assert.Contains(t, content, "0123456789abcdef0123456789abcdef01234567")
	assert.Contains(t, content, "clean")
	assert.Contains(t, content, "v1.0.0")
}
