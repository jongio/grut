// Package theme provides the color theme system for grut.
// Themes define all colors used throughout the TUI and are loaded from
// embedded TOML files (built-in) or user-supplied files on disk.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme holds the resolved color palette and pre-built lipgloss styles
// for the entire application. It is loaded once at startup and shared
// across all panels.
type Theme struct {
	Styles  Styles
	Colors  Colors
	Name    string
	Variant string
	Mode    Mode
}

// Colors defines every color slot used throughout the TUI.
// Values are CSS-style hex strings (#RRGGBB). Fields are populated by
// the TOML loader from the nested theme file format; the struct itself
// is kept flat so that panel code can access any colour without
// navigating nested types.
type Colors struct {
	// Base
	Background string
	Foreground string
	Cursor     string
	// ANSI normal palette
	NormalBlack   string
	NormalRed     string
	NormalGreen   string
	NormalYellow  string
	NormalBlue    string
	NormalMagenta string
	NormalCyan    string
	NormalWhite   string
	// ANSI bright palette
	BrightBlack   string
	BrightRed     string
	BrightGreen   string
	BrightYellow  string
	BrightBlue    string
	BrightMagenta string
	BrightCyan    string
	BrightWhite   string
	// UI — borders
	BorderFocused   string
	BorderUnfocused string
	// UI — status bar
	StatusBarBg string
	StatusBarFg string
	// UI — tabs
	TabActiveBg   string
	TabActiveFg   string
	TabInactiveBg string
	TabInactiveFg string
	// UI — titles
	TitleBg        string
	TitleFocusedBg string
	// UI — selection / cursor
	SelectionBg string
	SelectionFg string
	CursorLine  string
	// Syntax highlighting
	SyntaxKeyword  string
	SyntaxString   string
	SyntaxNumber   string
	SyntaxComment  string
	SyntaxFunction string
	SyntaxType     string
	SyntaxOperator string
	// Diff
	DiffAdded   string
	DiffRemoved string
	DiffContext string
	DiffHeader  string
	DiffHunk    string
	// Git status
	GitStaged    string
	GitUnstaged  string
	GitUntracked string
	GitConflict  string
	GitBranch    string
	GitTag       string
	// Notifications
	NotifyInfo    string
	NotifyWarn    string
	NotifyError   string
	NotifySuccess string
	// File types (for tree icons)
	FileDirectory  string
	FileDefault    string
	FileExecutable string
	FileSymlink    string
}

// ANSIPalette returns the 16-color ANSI palette from this theme.
// Indices 0–7 are the normal colors (black through white),
// indices 8–15 are the bright variants.
func (c Colors) ANSIPalette() [16]string {
	return [16]string{
		c.NormalBlack, c.NormalRed, c.NormalGreen, c.NormalYellow,
		c.NormalBlue, c.NormalMagenta, c.NormalCyan, c.NormalWhite,
		c.BrightBlack, c.BrightRed, c.BrightGreen, c.BrightYellow,
		c.BrightBlue, c.BrightMagenta, c.BrightCyan, c.BrightWhite,
	}
}

// Color returns a lipgloss.Color (which implements image/color.Color)
// for the given hex string.
func (Colors) Color(hex string) color.Color {
	return lipgloss.Color(hex)
}

// Styles holds pre-built lipgloss styles derived from the theme Colors.
// Panels use these directly instead of rebuilding styles on every render.
type Styles struct {
	// BorderFocused is the style for a focused panel border.
	BorderFocused lipgloss.Style
	// BorderUnfocused is the style for an unfocused panel border.
	BorderUnfocused lipgloss.Style
	// StatusBar is the base style for the bottom status bar.
	StatusBar lipgloss.Style
	// Title is the style for panel titles (unfocused).
	Title lipgloss.Style
	// TitleFocused is the style for panel titles when focused.
	TitleFocused lipgloss.Style
	// Brand is the style for the application brand text in the status bar.
	Brand lipgloss.Style
	// Tab styles
	TabActive   lipgloss.Style
	TabInactive lipgloss.Style
	// Git status styles
	GitStaged    lipgloss.Style
	GitUnstaged  lipgloss.Style
	GitUntracked lipgloss.Style
	GitConflict  lipgloss.Style
	GitBranch    lipgloss.Style
	GitTag       lipgloss.Style
	// Notification styles
	NotifyInfo    lipgloss.Style
	NotifyWarn    lipgloss.Style
	NotifyError   lipgloss.Style
	NotifySuccess lipgloss.Style
	// Diff styles
	DiffAdded   lipgloss.Style
	DiffRemoved lipgloss.Style
	DiffContext lipgloss.Style
	DiffHeader  lipgloss.Style
	DiffHunk    lipgloss.Style
	// Syntax styles
	SyntaxKeyword  lipgloss.Style
	SyntaxString   lipgloss.Style
	SyntaxNumber   lipgloss.Style
	SyntaxComment  lipgloss.Style
	SyntaxFunction lipgloss.Style
	SyntaxType     lipgloss.Style
	SyntaxOperator lipgloss.Style
	// Selection styles
	Selection  lipgloss.Style
	CursorLine lipgloss.Style
}

// buildStyles pre-computes all lipgloss styles from the given Colors.
func buildStyles(c Colors) Styles {
	return Styles{
		BorderFocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(c.BorderFocused)),
		BorderUnfocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(c.BorderUnfocused)),
		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.StatusBarFg)).
			Background(lipgloss.Color(c.StatusBarBg)),
		Title: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Foreground)).
			Background(lipgloss.Color(c.TitleBg)).
			Bold(true).
			PaddingLeft(1).
			PaddingRight(1),
		TitleFocused: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.BorderFocused)).
			Background(lipgloss.Color(c.TitleFocusedBg)).
			Bold(true).
			PaddingLeft(1).
			PaddingRight(1),
		Brand: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.BorderFocused)).
			Bold(true),
		// Tabs
		TabActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.TabActiveFg)).
			Background(lipgloss.Color(c.TabActiveBg)).
			Bold(true),
		TabInactive: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.TabInactiveFg)).
			Background(lipgloss.Color(c.TabInactiveBg)),
		// Git
		GitStaged:    lipgloss.NewStyle().Foreground(lipgloss.Color(c.GitStaged)),
		GitUnstaged:  lipgloss.NewStyle().Foreground(lipgloss.Color(c.GitUnstaged)),
		GitUntracked: lipgloss.NewStyle().Foreground(lipgloss.Color(c.GitUntracked)),
		GitConflict:  lipgloss.NewStyle().Foreground(lipgloss.Color(c.GitConflict)),
		GitBranch:    lipgloss.NewStyle().Foreground(lipgloss.Color(c.GitBranch)),
		GitTag:       lipgloss.NewStyle().Foreground(lipgloss.Color(c.GitTag)),
		// Notifications
		NotifyInfo:    lipgloss.NewStyle().Foreground(lipgloss.Color(c.NotifyInfo)),
		NotifyWarn:    lipgloss.NewStyle().Foreground(lipgloss.Color(c.NotifyWarn)),
		NotifyError:   lipgloss.NewStyle().Foreground(lipgloss.Color(c.NotifyError)),
		NotifySuccess: lipgloss.NewStyle().Foreground(lipgloss.Color(c.NotifySuccess)),
		// Diff
		DiffAdded:   lipgloss.NewStyle().Foreground(lipgloss.Color(c.DiffAdded)),
		DiffRemoved: lipgloss.NewStyle().Foreground(lipgloss.Color(c.DiffRemoved)),
		DiffContext: lipgloss.NewStyle().Foreground(lipgloss.Color(c.DiffContext)),
		DiffHeader:  lipgloss.NewStyle().Foreground(lipgloss.Color(c.DiffHeader)).Bold(true),
		DiffHunk:    lipgloss.NewStyle().Foreground(lipgloss.Color(c.DiffHunk)),
		// Syntax
		SyntaxKeyword:  lipgloss.NewStyle().Foreground(lipgloss.Color(c.SyntaxKeyword)),
		SyntaxString:   lipgloss.NewStyle().Foreground(lipgloss.Color(c.SyntaxString)),
		SyntaxNumber:   lipgloss.NewStyle().Foreground(lipgloss.Color(c.SyntaxNumber)),
		SyntaxComment:  lipgloss.NewStyle().Foreground(lipgloss.Color(c.SyntaxComment)).Italic(true),
		SyntaxFunction: lipgloss.NewStyle().Foreground(lipgloss.Color(c.SyntaxFunction)),
		SyntaxType:     lipgloss.NewStyle().Foreground(lipgloss.Color(c.SyntaxType)),
		SyntaxOperator: lipgloss.NewStyle().Foreground(lipgloss.Color(c.SyntaxOperator)),
		// Selection
		Selection: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.SelectionFg)).
			Background(lipgloss.Color(c.SelectionBg)),
		CursorLine: lipgloss.NewStyle().
			Background(lipgloss.Color(c.CursorLine)),
	}
}
