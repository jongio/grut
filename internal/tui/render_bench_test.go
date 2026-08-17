package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/layout"
)

var renderBenchmarkResult string

func BenchmarkRenderPanel(b *testing.B) {
	tests := []struct {
		name          string
		width, height int
	}{
		{name: "80x24", width: 80, height: 24},
		{name: "200x60", width: 200, height: 60},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			m := newTestModel(b)
			panel := newRenderFixturePanel("Benchmark", renderFixtureContent(tt.width, tt.height+4))
			rect := layout.Rect{Width: tt.width, Height: tt.height}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				renderBenchmarkResult = m.renderPanel("benchmark", panel, rect, true)
			}
		})
	}
}

func BenchmarkRenderLayout(b *testing.B) {
	tests := []struct {
		name          string
		width, height int
	}{
		{name: "80x24", width: 80, height: 24},
		{name: "200x60", width: 200, height: 60},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			m := newTestModel(b)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			m = updated.(Model)
			m.Init()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				renderBenchmarkResult = m.renderLayout()
			}
		})
	}
}
