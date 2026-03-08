package stash

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestSetActionsCfg(t *testing.T) {
	p := New(&mockGit{})
	cfg := config.ActionsConfig{
		DoubleClick: map[string]string{"stash_entry": "apply"},
		Confirmed:   map[string]bool{"stash_entry": true},
	}

	p.SetActionsCfg(cfg)

	assert.Equal(t, cfg, p.actionsCfg)
}

func TestHandleMouseWheel_Down(t *testing.T) {
	t.Run("scrolls down and clamps to max offset", func(t *testing.T) {
		p := newTestPanel(t, &mockGit{entries: sampleEntries()})
		p.Height = 1
		p.offset = 0

		updated, cmd := p.handleMouseWheel(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
		panel := updated.(*Panel)

		assert.Nil(t, cmd)
		assert.Equal(t, 2, panel.offset)
	})
}

func TestHandleMouseWheel_Up(t *testing.T) {
	t.Run("scrolls up and clamps to zero", func(t *testing.T) {
		p := newTestPanel(t, &mockGit{entries: sampleEntries()})
		p.Height = 1
		p.offset = 2

		updated, cmd := p.handleMouseWheel(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
		panel := updated.(*Panel)

		assert.Nil(t, cmd)
		assert.Equal(t, 0, panel.offset)
	})
}

func TestRenderPreview_EdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		width         int
		height        int
		previewOffset int
		wantOffset    int
		wantContains  []string
		wantLines     int
	}{
		{
			name:          "negative offset is clamped and long lines are truncated",
			content:       "123456\n+added",
			width:         4,
			height:        2,
			previewOffset: -3,
			wantOffset:    0,
			wantContains:  []string{"1234", "+add"},
			wantLines:     2,
		},
		{
			name:          "offset beyond content clamps to last line and pads remaining height",
			content:       "first\nsecond",
			width:         8,
			height:        3,
			previewOffset: 99,
			wantOffset:    1,
			wantContains:  []string{"second"},
			wantLines:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(&mockGit{})
			p.previewContent = tt.content
			p.previewOffset = tt.previewOffset

			got := p.renderPreview(tt.width, tt.height)

			assert.Equal(t, tt.wantOffset, p.previewOffset)
			assert.Len(t, strings.Split(got, "\n"), tt.wantLines)
			for _, want := range tt.wantContains {
				assert.Contains(t, got, want)
			}
		})
	}
}
