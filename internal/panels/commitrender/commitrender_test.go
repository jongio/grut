package commitrender

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func plainStyles() Styles {
	return Styles{
		Hash:    lipgloss.NewStyle(),
		Date:    lipgloss.NewStyle(),
		Author:  lipgloss.NewStyle(),
		Subject: lipgloss.NewStyle(),
		Ref:     lipgloss.NewStyle(),
		Graph:   lipgloss.NewStyle(),
		Cursor:  lipgloss.NewStyle().Reverse(true),
	}
}

func TestRenderLine_BasicSubjectAndHash(t *testing.T) {
	p := Params{
		Commit: git.Commit{
			ShortHash: "abc1234",
			Subject:   "fix: resolve nil pointer",
			Date:      time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		Width:  80,
		Styles: plainStyles(),
	}

	line := RenderLine(p)
	assert.Contains(t, line, "abc1234")
	assert.Contains(t, line, "fix: resolve nil pointer")
	assert.Equal(t, 80, lipgloss.Width(line))
}

func TestRenderLine_WithAuthorAndDate(t *testing.T) {
	p := Params{
		Commit: git.Commit{
			ShortHash: "def5678",
			Subject:   "feat: add panel",
			Author:    "Alice",
			Date:      time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC),
		},
		Width:      120,
		ShowAuthor: true,
		ShowDate:   true,
		Styles:     plainStyles(),
	}

	line := RenderLine(p)
	assert.Contains(t, line, "def5678")
	assert.Contains(t, line, "Alice")
	assert.Contains(t, line, "2025-03-20")
	assert.Equal(t, 120, lipgloss.Width(line))
}

func TestRenderLine_WithRefs(t *testing.T) {
	p := Params{
		Commit: git.Commit{
			ShortHash: "aaa1111",
			Subject:   "chore: bump deps",
			Refs:      []string{"main", "v1.0.0"},
			Date:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		},
		Width:    100,
		ShowRefs: true,
		Styles:   plainStyles(),
	}

	line := RenderLine(p)
	assert.Contains(t, line, "main")
	assert.Contains(t, line, "v1.0.0")
	assert.Equal(t, 100, lipgloss.Width(line))
}

func TestRenderLine_AuthorTruncation(t *testing.T) {
	p := Params{
		Commit: git.Commit{
			ShortHash: "bbb2222",
			Subject:   "docs: update readme",
			Author:    "VeryLongAuthorName1234567890",
			Date:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Width:      120,
		ShowAuthor: true,
		Styles:     plainStyles(),
	}

	line := RenderLine(p)
	// Author should be truncated to AuthorColMaxWidth (14)
	assert.NotContains(t, line, "VeryLongAuthorName1234567890")
	assert.Equal(t, 120, lipgloss.Width(line))
}

func TestRenderLine_NarrowWidth(t *testing.T) {
	p := Params{
		Commit: git.Commit{
			ShortHash: "ccc3333",
			Subject:   "fix: a very long subject that should get truncated in narrow terminal",
			Author:    "Bob",
			Date:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Width:      40,
		ShowAuthor: true,
		ShowDate:   true,
		Styles:     plainStyles(),
	}

	line := RenderLine(p)
	assert.Equal(t, 40, lipgloss.Width(line))
	assert.Contains(t, line, "ccc3333")
}

func TestRenderLine_WithGraphPrefix(t *testing.T) {
	p := Params{
		Commit: git.Commit{
			ShortHash: "ddd4444",
			Subject:   "merge: feature branch",
			Date:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Width:       80,
		GraphPrefix: "* |",
		Styles:      plainStyles(),
	}

	line := RenderLine(p)
	assert.Contains(t, line, "* |")
	assert.Equal(t, 80, lipgloss.Width(line))
}

func TestRenderLine_CursorHighlight(t *testing.T) {
	p := Params{
		Commit: git.Commit{
			ShortHash: "eee5555",
			Subject:   "test: cursor row",
			Date:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Width:    80,
		IsCursor: true,
		Styles:   plainStyles(),
	}

	line := RenderLine(p)
	require.Equal(t, 80, lipgloss.Width(line))
}

func TestRenderLine_SelectedHighlight(t *testing.T) {
	p := Params{
		Commit: git.Commit{
			ShortHash: "fff6666",
			Subject:   "test: selected row",
			Date:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Width:      80,
		IsSelected: true,
		SelectedBg: "#333333",
		Styles:     plainStyles(),
	}

	line := RenderLine(p)
	assert.Equal(t, 80, lipgloss.Width(line))
}

func TestRenderLine_ANSIStripped(t *testing.T) {
	// Simulate ANSI injection in commit data
	p := Params{
		Commit: git.Commit{
			ShortHash: "\x1b[31mevil\x1b[0m",
			Subject:   "\x1b[32minjected\x1b[0m subject",
			Date:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Width:  80,
		Styles: plainStyles(),
	}

	line := RenderLine(p)
	// Raw ANSI escape sequences from the commit data should be stripped
	assert.NotContains(t, line, "\x1b[31m")
	assert.NotContains(t, line, "\x1b[32m")
	assert.Contains(t, line, "evil")
	assert.Contains(t, line, "injected")
}

func TestTruncateOrPad_Exact(t *testing.T) {
	s := "hello"
	result := TruncateOrPad(s, 5)
	assert.Equal(t, 5, lipgloss.Width(result))
}

func TestTruncateOrPad_Shorter(t *testing.T) {
	s := "hi"
	result := TruncateOrPad(s, 10)
	assert.Equal(t, 10, lipgloss.Width(result))
	assert.True(t, strings.HasPrefix(result, "hi"))
}

func TestTruncateOrPad_Longer(t *testing.T) {
	s := "a very long string that exceeds the width"
	result := TruncateOrPad(s, 15)
	assert.Equal(t, 15, lipgloss.Width(result))
}

func TestStyleSubjectWithRefs(t *testing.T) {
	sub := lipgloss.NewStyle()
	ref := lipgloss.NewStyle()

	result := styleSubjectWithRefs("fix something (main, v1)", sub, ref)
	assert.Contains(t, result, "fix something ")
	assert.Contains(t, result, "(main, v1)")
}

func TestStyleSubjectWithRefs_NoParens(t *testing.T) {
	sub := lipgloss.NewStyle()
	ref := lipgloss.NewStyle()

	result := styleSubjectWithRefs("fix something", sub, ref)
	assert.Equal(t, sub.Render("fix something"), result)
}
