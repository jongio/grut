package extensions

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/extension"
	"github.com/stretchr/testify/assert"
)

func TestSetActionsCfg(t *testing.T) {
	p := New(&mockExtManager{})
	cfg := config.ActionsConfig{
		RightClick: map[string]string{"extension": "toggle_enable"},
		Confirmed:  map[string]bool{"extension": true},
	}

	p.SetActionsCfg(cfg)

	assert.Equal(t, cfg, p.actionsCfg)
}

func TestHandleMouseWheel_Down(t *testing.T) {
	t.Run("scrolls down and clamps to max offset", func(t *testing.T) {
		p := newTestPanel(t, &mockExtManager{extensions: []extension.ExtensionInfo{
			makeExt("a", "1.0.0", "lua", true),
			makeExt("b", "1.0.0", "lua", true),
			makeExt("c", "1.0.0", "lua", true),
			makeExt("d", "1.0.0", "lua", true),
		}})
		p.Height = 1

		updated, cmd := p.handleMouseWheel(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
		panel := updated.(*Panel)

		assert.Nil(t, cmd)
		assert.Equal(t, 3, panel.offset)
	})
}

func TestHandleMouseWheel_Up(t *testing.T) {
	t.Run("scrolls up and clamps to zero", func(t *testing.T) {
		p := newTestPanel(t, &mockExtManager{extensions: []extension.ExtensionInfo{
			makeExt("a", "1.0.0", "lua", true),
			makeExt("b", "1.0.0", "lua", true),
			makeExt("c", "1.0.0", "lua", true),
			makeExt("d", "1.0.0", "lua", true),
		}})
		p.Height = 1
		p.offset = 2

		updated, cmd := p.handleMouseWheel(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
		panel := updated.(*Panel)

		assert.Nil(t, cmd)
		assert.Equal(t, 0, panel.offset)
	})
}

func TestEnsureCursorVisible(t *testing.T) {
	tests := []struct {
		name       string
		height     int
		cursor     int
		offset     int
		wantOffset int
	}{
		{name: "zero height leaves offset unchanged", height: 0, cursor: 0, offset: 2, wantOffset: 2},
		{name: "cursor above offset moves viewport up", height: 3, cursor: 1, offset: 4, wantOffset: 1},
		{name: "cursor below viewport moves viewport down", height: 3, cursor: 5, offset: 1, wantOffset: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(&mockExtManager{})
			p.Height = tt.height
			p.cursor = tt.cursor
			p.offset = tt.offset

			p.ensureCursorVisible()

			assert.Equal(t, tt.wantOffset, p.offset)
		})
	}
}
