package settings

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/layout"
	"github.com/stretchr/testify/assert"
)

func TestPositionLabel(t *testing.T) {
	tests := []struct {
		name string
		pos  layout.PreviewPosition
		want string
	}{
		{name: "right", pos: layout.PreviewRight, want: "Right"},
		{name: "bottom", pos: layout.PreviewBottom, want: "Bottom"},
		{name: "left", pos: layout.PreviewLeft, want: "Left"},
		{name: "top", pos: layout.PreviewTop, want: "Top"},
		{name: "unknown defaults to right", pos: layout.PreviewPosition(99), want: "Right"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, positionLabel(tt.pos))
		})
	}
}

func TestHandleMouseWheel(t *testing.T) {
	tests := []struct {
		name        string
		button      tea.MouseButton
		startOffset int
		totalLines  int
		height      int
		wantOffset  int
	}{
		{name: "scroll down clamps to max offset", button: tea.MouseWheelDown, startOffset: 0, totalLines: 10, height: 4, wantOffset: 3},
		{name: "scroll up clamps to zero", button: tea.MouseWheelUp, startOffset: 2, totalLines: 10, height: 4, wantOffset: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(layout.PreviewRight, "default", testThemes, config.ActionsConfig{}, nil)
			p.offset = tt.startOffset
			p.totalLines = tt.totalLines
			p.Height = tt.height

			updated, cmd := p.handleMouseWheel(tea.MouseWheelMsg(tea.Mouse{Button: tt.button}))
			panel := updated.(*Panel)

			assert.Nil(t, cmd)
			assert.Equal(t, tt.wantOffset, panel.offset)
		})
	}
}

func TestCyclePreviewPosition(t *testing.T) {
	tests := []struct {
		name    string
		current layout.PreviewPosition
		want    layout.PreviewPosition
	}{
		{name: "right to bottom", current: layout.PreviewRight, want: layout.PreviewBottom},
		{name: "bottom to left", current: layout.PreviewBottom, want: layout.PreviewLeft},
		{name: "left to top", current: layout.PreviewLeft, want: layout.PreviewTop},
		{name: "top wraps to right", current: layout.PreviewTop, want: layout.PreviewRight},
		{name: "unknown falls back to first option", current: layout.PreviewPosition(99), want: layout.PreviewRight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cyclePreviewPosition(tt.current))
		})
	}
}

func TestCycleTheme_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		themeNames []string
		current    string
		want       string
	}{
		{name: "empty theme list keeps current", themeNames: nil, current: "custom", want: "custom"},
		{name: "empty current defaults to default then advances", themeNames: testThemes, current: "", want: "catppuccin"},
		{name: "unknown current falls back to first theme", themeNames: testThemes, current: "unknown", want: "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(layout.PreviewRight, "default", tt.themeNames, config.ActionsConfig{}, nil)
			assert.Equal(t, tt.want, p.cycleTheme(tt.current))
		})
	}
}
