package theme

import (
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// Mode is the resolved display color mode.
type Mode string

const (
	ModeColor Mode = "color"
	ModeMono  Mode = "mono"
)

const (
	noColorEnv      = "NO_COLOR"
	configModeAuto  = "auto"
	configModeColor = "color"
	configModeMono  = "mono"
)

// ResolveColorMode resolves the configured mode and NO_COLOR presence.
func ResolveColorMode(configMode string, noColorSet bool) Mode {
	switch strings.ToLower(strings.TrimSpace(configMode)) {
	case configModeColor:
		return ModeColor
	case configModeMono:
		return ModeMono
	case configModeAuto, "":
		if noColorSet {
			return ModeMono
		}
		return ModeColor
	default:
		return ModeColor
	}
}

// ResolveEnvironmentColorMode resolves color mode using NO_COLOR presence.
func ResolveEnvironmentColorMode(configMode string) Mode {
	_, noColorSet := os.LookupEnv(noColorEnv)
	return ResolveColorMode(configMode, noColorSet)
}

// ApplyColorMode returns t configured for the resolved display mode.
func ApplyColorMode(t Theme, mode Mode) Theme {
	if mode == ModeMono {
		return t.MonoVariant()
	}
	t.Mode = ModeColor
	return t
}

// MonoVariant returns a monochrome variant with state expressed by markers.
func (t Theme) MonoVariant() Theme {
	c := t.Colors
	fg := c.Foreground

	c.Cursor = fg
	c.NormalRed = fg
	c.NormalGreen = fg
	c.NormalYellow = fg
	c.NormalBlue = fg
	c.NormalMagenta = fg
	c.NormalCyan = fg
	c.BrightRed = fg
	c.BrightGreen = fg
	c.BrightYellow = fg
	c.BrightBlue = fg
	c.BrightMagenta = fg
	c.BrightCyan = fg
	c.BorderFocused = c.BorderUnfocused
	c.StatusBarFg = fg
	c.TabActiveFg = fg
	c.TabInactiveFg = fg
	c.TitleFocusedBg = c.TitleBg
	c.SyntaxKeyword = fg
	c.SyntaxString = fg
	c.SyntaxNumber = fg
	c.SyntaxComment = fg
	c.SyntaxFunction = fg
	c.SyntaxType = fg
	c.SyntaxOperator = fg
	c.DiffAdded = fg
	c.DiffRemoved = fg
	c.DiffContext = fg
	c.DiffHeader = fg
	c.DiffHunk = fg
	c.GitStaged = fg
	c.GitUnstaged = fg
	c.GitUntracked = fg
	c.GitConflict = fg
	c.GitBranch = fg
	c.GitTag = fg
	c.NotifyInfo = fg
	c.NotifyWarn = fg
	c.NotifyError = fg
	c.NotifySuccess = fg
	c.FileDirectory = fg
	c.FileDefault = fg
	c.FileExecutable = fg
	c.FileSymlink = fg

	t.Colors = c
	t.Styles = buildStyles(c)
	t.Styles.Selection = lipgloss.NewStyle().Reverse(true)
	t.Styles.CursorLine = lipgloss.NewStyle().Reverse(true)
	t.Mode = ModeMono
	return t
}
