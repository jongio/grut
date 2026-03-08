package conflicts

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestSetActionsCfg(t *testing.T) {
	p := New(&mockGit{})
	cfg := config.ActionsConfig{
		RightClick: map[string]string{"conflict_file": "open"},
		Confirmed:  map[string]bool{"conflict_file": true},
	}

	p.SetActionsCfg(cfg)

	assert.Equal(t, cfg, p.actionsCfg)
}

func TestHandleMouseWheel_Down(t *testing.T) {
	t.Run("scrolls down and clamps to viewport max", func(t *testing.T) {
		p := newTestPanel(t, &mockGit{})
		p.files = []string{"file1.go", "file2.go", "file3.go", "file4.go", "file5.go"}
		p.Height = 4

		updated, cmd := p.handleMouseWheel(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
		panel := updated.(*Panel)

		assert.Nil(t, cmd)
		assert.Equal(t, 3, panel.offset)
	})
}

func TestHandleMouseWheel_Up(t *testing.T) {
	t.Run("scrolls up and clamps to zero", func(t *testing.T) {
		p := newTestPanel(t, &mockGit{})
		p.files = []string{"file1.go", "file2.go", "file3.go", "file4.go"}
		p.Height = 4
		p.offset = 2

		updated, cmd := p.handleMouseWheel(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
		panel := updated.(*Panel)

		assert.Nil(t, cmd)
		assert.Equal(t, 0, panel.offset)
	})
}

func TestAdjustOffset(t *testing.T) {
	tests := []struct {
		name       string
		height     int
		cursor     int
		offset     int
		wantOffset int
	}{
		{name: "zero height leaves offset unchanged", height: 0, cursor: 0, offset: 2, wantOffset: 2},
		{name: "cursor above offset moves viewport up", height: 6, cursor: 1, offset: 4, wantOffset: 1},
		{name: "cursor below viewport moves viewport down", height: 6, cursor: 7, offset: 1, wantOffset: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestPanel(t, &mockGit{})
			p.files = []string{"file1.go", "file2.go", "file3.go", "file4.go", "file5.go", "file6.go", "file7.go", "file8.go"}
			p.Height = tt.height
			p.cursor = tt.cursor
			p.offset = tt.offset

			p.adjustOffset()

			assert.Equal(t, tt.wantOffset, p.offset)
		})
	}
}
