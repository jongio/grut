package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/require"
)

type renderFixturePanel struct {
	panels.BasePanel
	content string
}

func newRenderFixturePanel(title, content string) *renderFixturePanel {
	return &renderFixturePanel{
		BasePanel: panels.BasePanel{PanelTitle: title},
		content:   content,
	}
}

func (p *renderFixturePanel) Init(context.Context) tea.Cmd {
	return nil
}

func (p *renderFixturePanel) Update(tea.Msg) (panels.Panel, tea.Cmd) {
	return p, nil
}

func (p *renderFixturePanel) View(_, _ int) string {
	return p.content
}

func renderFixtureContent(width, height int) string {
	var result strings.Builder
	result.Grow((width + 1) * height)
	for row := 0; row < height; row++ {
		if row > 0 {
			result.WriteByte('\n')
		}
		switch row % 5 {
		case 0:
			result.WriteString("short")
		case 1:
			result.WriteString(strings.Repeat("long-content-", width/6+2))
		case 2:
			result.WriteString("\x1b[31mANSI red\x1b[0m and plain text")
		case 3:
			result.WriteString("界🙂e\u0301 wide Unicode")
		case 4:
		}
	}
	return result.String()
}

func legacyRenderPanel(m Model, p panels.Panel, rect layout.Rect) string {
	if p == nil {
		return ""
	}
	contentW := rect.Width
	contentH := rect.Height
	if contentW < 1 {
		contentW = 1
	}
	if contentH < 1 {
		contentH = 1
	}
	pad := layout.PanelPadH
	innerW := contentW - 2*pad
	if innerW < 1 {
		innerW = 1
	}
	leftPad := strings.Repeat(" ", pad)
	rightPad := strings.Repeat(" ", pad)
	content := p.View(innerW, contentH)
	lines := strings.Split(content, "\n")
	if len(lines) > contentH {
		lines = lines[:contentH]
	}
	for len(lines) < contentH {
		lines = append(lines, strings.Repeat(" ", innerW))
	}
	var row strings.Builder
	for i, line := range lines {
		row.Reset()
		w := lipgloss.Width(line)
		if w > innerW {
			line = ansi.Truncate(line, innerW, "")
		}
		row.WriteString(leftPad)
		row.WriteString(line)
		if w < innerW {
			for range innerW - w {
				row.WriteByte(' ')
			}
		}
		row.WriteString(rightPad)
		lines[i] = row.String()
	}
	return strings.Join(lines, "\n")
}

func legacyBuildOuterBorder(
	m Model,
	content string,
	contentWidth, contentHeight int,
	borderColorStr string,
	tree layout.Node,
	topTitles []borderTitle,
) string {
	border := lipgloss.RoundedBorder()
	bdr := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColorStr))
	topJ := map[int]bool{}
	bottomJ := map[int]bool{}
	leftJ := map[int]bool{}
	rightJ := map[int]bool{}
	if tree != nil {
		area := layout.Rect{X: 0, Y: 0, Width: contentWidth, Height: contentHeight}
		collectJunctions(tree, area, contentWidth, contentHeight, topJ, bottomJ, leftJ, rightJ)
	}
	var junctionCols []int
	for col := 0; col < contentWidth; col++ {
		if topJ[col] {
			junctionCols = append(junctionCols, col)
		}
	}
	topLine := m.buildTopBorderWithTitles(contentWidth, junctionCols, topTitles, bdr, border)
	contentLines := strings.Split(content, "\n")
	for len(contentLines) < contentHeight {
		contentLines = append(contentLines, strings.Repeat(" ", contentWidth))
	}
	if len(contentLines) > contentHeight {
		contentLines = contentLines[:contentHeight]
	}
	renderedLeft := bdr.Render(border.Left)
	renderedRight := bdr.Render(border.Right)
	renderedLeftJ := bdr.Render("├")
	renderedRightJ := bdr.Render("┤")
	var row strings.Builder
	for i, line := range contentLines {
		row.Reset()
		if leftJ[i] {
			row.WriteString(renderedLeftJ)
		} else {
			row.WriteString(renderedLeft)
		}
		row.WriteString(line)
		if rightJ[i] {
			row.WriteString(renderedRightJ)
		} else {
			row.WriteString(renderedRight)
		}
		contentLines[i] = row.String()
	}
	var bottomParts []string
	bottomParts = append(bottomParts, bdr.Render(border.BottomLeft))
	runStart := 0
	for col := 0; col <= contentWidth; col++ {
		if col == contentWidth || bottomJ[col] {
			if run := col - runStart; run > 0 {
				bottomParts = append(bottomParts, bdr.Render(strings.Repeat(border.Bottom, run)))
			}
			if col < contentWidth {
				bottomParts = append(bottomParts, bdr.Render("┴"))
			}
			runStart = col + 1
		}
	}
	bottomParts = append(bottomParts, bdr.Render(border.BottomRight))
	bottomLine := strings.Join(bottomParts, "")
	return topLine + "\n" + strings.Join(contentLines, "\n") + "\n" + bottomLine
}

func legacyRenderNode(
	m Model,
	node layout.Node,
	rects map[string]layout.Rect,
	allPanels map[string]panels.Panel,
	focusedName string,
	borderColorStr string,
) string {
	switch n := node.(type) {
	case *layout.LeafNode:
		rect, ok := rects[n.Panel]
		if !ok {
			return ""
		}
		return legacyRenderPanel(m, allPanels[n.Panel], rect)
	case *layout.SplitNode:
		firstContent := legacyRenderNode(m, n.First, rects, allPanels, focusedName, borderColorStr)
		secondContent := legacyRenderNode(m, n.Second, rects, allPanels, focusedName, borderColorStr)
		sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColorStr))
		if n.Direction == layout.Horizontal {
			height := lipgloss.Height(firstContent)
			if secondHeight := lipgloss.Height(secondContent); secondHeight > height {
				height = secondHeight
			}
			styledSeparator := sepStyle.Render("│")
			separatorLines := make([]string, height)
			for i := range separatorLines {
				separatorLines[i] = styledSeparator
			}
			separator := strings.Join(separatorLines, "\n")
			return lipgloss.JoinHorizontal(lipgloss.Top, firstContent, separator, secondContent)
		}
		width := lipgloss.Width(firstContent)
		if secondWidth := lipgloss.Width(secondContent); secondWidth > width {
			width = secondWidth
		}
		bottomPanelName := layout.FirstPanelOf(n.Second)
		separator := m.renderSeparatorWithTitle(width, bottomPanelName, allPanels, focusedName, sepStyle)
		return lipgloss.JoinVertical(lipgloss.Left, firstContent, separator, secondContent)
	}
	return ""
}

func legacyRenderLayout(m Model) string {
	rects := m.engine.PanelRects()
	if len(rects) == 0 {
		return ""
	}
	focusedName := m.engine.FocusedName()
	allPanels := m.engine.Panels()
	borderColorStr := m.theme.Colors.BorderFocused
	var innerContent string
	if m.engine.IsZoomed() {
		innerContent = legacyRenderPanel(m, allPanels[focusedName], rects[focusedName])
	} else {
		innerContent = legacyRenderNode(
			m,
			m.engine.TabManager().ActiveTab().Tree,
			rects,
			allPanels,
			focusedName,
			borderColorStr,
		)
	}
	innerArea := m.engine.InnerArea()
	var tree layout.Node
	if !m.engine.IsZoomed() {
		tree = m.engine.TabManager().ActiveTab().Tree
	}
	topTitles := m.computeTopBorderTitles(rects, allPanels, focusedName)
	panelArea := legacyBuildOuterBorder(
		m,
		innerContent,
		innerArea.Width,
		innerArea.Height,
		borderColorStr,
		tree,
		topTitles,
	)
	components := make([]string, 0, 4)
	if tabBar := m.renderTabBar(); tabBar != "" {
		components = append(components, tabBar)
	}
	components = append(components, panelArea, m.renderHintsBar(), m.renderStatusBar())
	return lipgloss.JoinVertical(lipgloss.Left, components...)
}

func TestNextRenderRow(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{name: "empty", content: "", want: []string{""}},
		{name: "single", content: "one", want: []string{"one"}},
		{name: "multiple", content: "one\ntwo", want: []string{"one", "two"}},
		{name: "trailing newline", content: "one\n", want: []string{"one", ""}},
		{name: "leading newline", content: "\none", want: []string{"", "one"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := 0
			var got []string
			for {
				row, next, ok := nextRenderRow(tt.content, start)
				if !ok {
					break
				}
				got = append(got, row)
				start = next
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRenderPanelByteForByte(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		width, height int
	}{
		{name: "empty", content: "", width: 10, height: 3},
		{name: "short", content: "abc\nx", width: 12, height: 4},
		{name: "long", content: "0123456789abcdef\nsecond line beyond width", width: 12, height: 3},
		{name: "ANSI", content: "\x1b[31mred\x1b[0m\n\x1b[1;34mblue and long\x1b[0m", width: 14, height: 4},
		{name: "wide Unicode", content: "界🙂e\u0301\n日本語とemoji🙂🙂", width: 12, height: 4},
		{name: "80x24", content: renderFixtureContent(80, 28), width: 80, height: 24},
		{name: "200x60", content: renderFixtureContent(200, 64), width: 200, height: 60},
	}
	m := newTestModel(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			panel := newRenderFixturePanel("Fixture", tt.content)
			rect := layout.Rect{Width: tt.width, Height: tt.height}
			require.Equal(t, legacyRenderPanel(m, panel, rect), m.renderPanel("fixture", panel, rect, true))
		})
	}
}

func TestBuildOuterBorderByteForByte(t *testing.T) {
	splitTree := &layout.SplitNode{
		Direction: layout.Horizontal,
		Ratio:     0.5,
		First: &layout.SplitNode{
			Direction: layout.Vertical,
			Ratio:     0.5,
			First:     &layout.LeafNode{Panel: "top-left"},
			Second:    &layout.LeafNode{Panel: "bottom-left"},
		},
		Second: &layout.LeafNode{Panel: "right"},
	}
	tests := []struct {
		name          string
		content       string
		width, height int
		tree          layout.Node
		titles        []borderTitle
	}{
		{name: "empty", content: "", width: 10, height: 3},
		{name: "short", content: "abc\nx", width: 12, height: 4},
		{name: "long", content: "0123456789abcdef\nsecond line beyond width", width: 12, height: 3},
		{name: "ANSI", content: "\x1b[31mred\x1b[0m\n\x1b[34mblue\x1b[0m", width: 14, height: 4},
		{name: "wide Unicode", content: "界🙂e\u0301\n日本語", width: 12, height: 4},
		{
			name:    "junctions titles and focus",
			content: renderFixtureContent(40, 8),
			width:   40,
			height:  8,
			tree:    splitTree,
			titles: []borderTitle{
				{title: "Focused 界", startCol: 0, endCol: 18, focused: true},
				{title: "Other 🙂", startCol: 21, endCol: 39},
			},
		},
	}
	m := newTestModel(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := legacyBuildOuterBorder(
				m,
				tt.content,
				tt.width,
				tt.height,
				m.theme.Colors.BorderFocused,
				tt.tree,
				tt.titles,
			)
			got := m.buildOuterBorder(
				tt.content,
				tt.width,
				tt.height,
				m.theme.Colors.BorderFocused,
				tt.tree,
				tt.titles,
			)
			require.Equal(t, want, got)
		})
	}
}

func TestRenderLayoutByteForByte(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
	}{
		{name: "80x24", width: 80, height: 24},
		{name: "200x60", width: 200, height: 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			m = updated.(Model)
			m.Init()
			require.Equal(t, legacyRenderLayout(m), m.renderLayout())
		})
	}
}
