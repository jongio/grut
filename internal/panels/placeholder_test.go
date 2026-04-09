package panels

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPlaceholder(t *testing.T) {
	p := NewPlaceholder("test-panel", nil)
	assert.Equal(t, "test-panel", p.Title())
	assert.Empty(t, p.KeyBindings())
}

func TestPlaceholderInit(t *testing.T) {
	p := NewPlaceholder("test", nil)
	cmd := p.Init(context.Background())
	assert.Nil(t, cmd)
}

func TestPlaceholderUpdate(t *testing.T) {
	p := NewPlaceholder("test", nil)
	updated, cmd := p.Update(nil)
	assert.Equal(t, p, updated)
	assert.Nil(t, cmd)
}

func TestPlaceholderView(t *testing.T) {
	p := NewPlaceholder("filetree", nil)

	content := p.View(40, 10)
	assert.Contains(t, content, "filetree")
}

func TestPlaceholderViewZeroSize(t *testing.T) {
	p := NewPlaceholder("test", nil)
	assert.Empty(t, p.View(0, 10))
	assert.Empty(t, p.View(10, 0))
	assert.Empty(t, p.View(0, 0))
	assert.Empty(t, p.View(-1, 10))
}

func TestPlaceholderViewFocused(t *testing.T) {
	p := NewPlaceholder("filetree", nil)
	p.Focus()
	content := p.View(40, 10)
	assert.Contains(t, content, "filetree")
}

func TestPlaceholderFocusBlur(t *testing.T) {
	p := NewPlaceholder("test", nil)
	assert.False(t, p.Focused)

	p.Focus()
	assert.True(t, p.Focused)

	p.Blur()
	assert.False(t, p.Focused)
}

func TestPlaceholderSetSize(t *testing.T) {
	p := NewPlaceholder("test", nil)
	p.SetSize(80, 24)
	assert.Equal(t, 80, p.Width)
	assert.Equal(t, 24, p.Height)
}

func TestPlaceholderString(t *testing.T) {
	p := NewPlaceholder("filetree", nil)
	assert.Equal(t, "Placeholder(filetree)", p.String())
}

func TestPlaceholderImplementsPanel(t *testing.T) {
	// Compile-time check
	var _ Panel = (*Placeholder)(nil)
}
