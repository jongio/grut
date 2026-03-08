package theme

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Built-in theme loading
// ---------------------------------------------------------------------------

func TestLoadBuiltinThemes(t *testing.T) {
	for _, name := range BuiltinNames() {
		t.Run(name, func(t *testing.T) {
			th, err := Load(name)
			require.NoError(t, err)
			assert.Equal(t, name, th.Name)
		})
	}
}

func TestLoadDefaultThemePreservesHardcodedColors(t *testing.T) {
	th, err := Load("default")
	require.NoError(t, err)

	// The default theme should carry forward the original hardcoded values.
	assert.Equal(t, "#7D56F4", th.Colors.BorderFocused)
	assert.Equal(t, "#444444", th.Colors.BorderUnfocused)
	assert.Equal(t, "#333333", th.Colors.StatusBarBg)
	assert.Equal(t, "#FAFAFA", th.Colors.StatusBarFg)
}

// ---------------------------------------------------------------------------
// Unknown theme
// ---------------------------------------------------------------------------

func TestLoadUnknownThemeReturnsError(t *testing.T) {
	_, err := Load("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown theme")
	assert.Contains(t, err.Error(), "nonexistent")
}

// ---------------------------------------------------------------------------
// Custom theme file path
// ---------------------------------------------------------------------------

func TestLoadCustomThemeFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my-theme.toml")

	require.NoError(t, os.WriteFile(path, []byte(completeTestTOML), 0o600))

	th, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "#000000", th.Colors.Background)
	assert.Equal(t, "#FFFFFF", th.Colors.Foreground)
}

func TestLoadCustomThemeNotFoundFallsBackToDefault(t *testing.T) {
	th, err := Load("/nonexistent/path/theme.toml")
	require.NoError(t, err, "should fall back to default, not error")
	assert.Equal(t, "default", th.Name)
}

// ---------------------------------------------------------------------------
// Color completeness — all fields populated for every built-in theme
// ---------------------------------------------------------------------------

func TestAllColorFieldsPopulated(t *testing.T) {
	for _, name := range BuiltinNames() {
		t.Run(name, func(t *testing.T) {
			th, err := Load(name)
			require.NoError(t, err)

			v := reflect.ValueOf(th.Colors)
			typ := v.Type()
			for i := range typ.NumField() {
				field := typ.Field(i)
				val := v.Field(i).String()
				assert.NotEmpty(t, val, "color field %s is empty in theme %q", field.Name, name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// New color fields are present
// ---------------------------------------------------------------------------

func TestNewColorFieldsPopulated(t *testing.T) {
	for _, name := range BuiltinNames() {
		t.Run(name, func(t *testing.T) {
			th, err := Load(name)
			require.NoError(t, err)

			// ANSI normal
			assert.NotEmpty(t, th.Colors.NormalBlack, "NormalBlack")
			assert.NotEmpty(t, th.Colors.NormalRed, "NormalRed")
			assert.NotEmpty(t, th.Colors.NormalGreen, "NormalGreen")
			assert.NotEmpty(t, th.Colors.NormalYellow, "NormalYellow")
			assert.NotEmpty(t, th.Colors.NormalBlue, "NormalBlue")
			assert.NotEmpty(t, th.Colors.NormalMagenta, "NormalMagenta")
			assert.NotEmpty(t, th.Colors.NormalCyan, "NormalCyan")
			assert.NotEmpty(t, th.Colors.NormalWhite, "NormalWhite")

			// ANSI bright
			assert.NotEmpty(t, th.Colors.BrightBlack, "BrightBlack")
			assert.NotEmpty(t, th.Colors.BrightWhite, "BrightWhite")

			// Syntax
			assert.NotEmpty(t, th.Colors.SyntaxKeyword, "SyntaxKeyword")
			assert.NotEmpty(t, th.Colors.SyntaxString, "SyntaxString")
			assert.NotEmpty(t, th.Colors.SyntaxNumber, "SyntaxNumber")
			assert.NotEmpty(t, th.Colors.SyntaxComment, "SyntaxComment")
			assert.NotEmpty(t, th.Colors.SyntaxFunction, "SyntaxFunction")
			assert.NotEmpty(t, th.Colors.SyntaxType, "SyntaxType")
			assert.NotEmpty(t, th.Colors.SyntaxOperator, "SyntaxOperator")

			// Tabs
			assert.NotEmpty(t, th.Colors.TabActiveBg, "TabActiveBg")
			assert.NotEmpty(t, th.Colors.TabActiveFg, "TabActiveFg")
			assert.NotEmpty(t, th.Colors.TabInactiveBg, "TabInactiveBg")
			assert.NotEmpty(t, th.Colors.TabInactiveFg, "TabInactiveFg")

			// Cursor
			assert.NotEmpty(t, th.Colors.Cursor, "Cursor")

			// Diff hunk
			assert.NotEmpty(t, th.Colors.DiffHunk, "DiffHunk")

			// Git branch/tag
			assert.NotEmpty(t, th.Colors.GitBranch, "GitBranch")
			assert.NotEmpty(t, th.Colors.GitTag, "GitTag")

			// Title backgrounds
			assert.NotEmpty(t, th.Colors.TitleBg, "TitleBg")
			assert.NotEmpty(t, th.Colors.TitleFocusedBg, "TitleFocusedBg")
		})
	}
}

// ---------------------------------------------------------------------------
// Styles conversion
// ---------------------------------------------------------------------------

func TestBuildStylesProducesNonZeroStyles(t *testing.T) {
	th, err := Load("default")
	require.NoError(t, err)

	// Verify that key styles have been initialised (non-zero values).
	borderStr := th.Styles.BorderFocused.Render("x")
	assert.NotEmpty(t, borderStr)

	statusStr := th.Styles.StatusBar.Render("test")
	assert.NotEmpty(t, statusStr)

	titleStr := th.Styles.TitleFocused.Render("title")
	assert.NotEmpty(t, titleStr)
}

func TestBuildStylesAllThemes(t *testing.T) {
	for _, name := range BuiltinNames() {
		t.Run(name, func(t *testing.T) {
			th, err := Load(name)
			require.NoError(t, err)

			// Each theme should produce renderable styles without panicking.
			assert.NotEmpty(t, th.Styles.BorderFocused.Render("x"))
			assert.NotEmpty(t, th.Styles.BorderUnfocused.Render("x"))
			assert.NotEmpty(t, th.Styles.StatusBar.Render("x"))
			assert.NotEmpty(t, th.Styles.GitStaged.Render("x"))
			assert.NotEmpty(t, th.Styles.DiffAdded.Render("x"))
			assert.NotEmpty(t, th.Styles.Selection.Render("x"))
		})
	}
}

func TestNewStylesProducedForAllThemes(t *testing.T) {
	for _, name := range BuiltinNames() {
		t.Run(name, func(t *testing.T) {
			th, err := Load(name)
			require.NoError(t, err)

			// Tab styles
			assert.NotEmpty(t, th.Styles.TabActive.Render("x"))
			assert.NotEmpty(t, th.Styles.TabInactive.Render("x"))

			// Syntax styles
			assert.NotEmpty(t, th.Styles.SyntaxKeyword.Render("x"))
			assert.NotEmpty(t, th.Styles.SyntaxString.Render("x"))
			assert.NotEmpty(t, th.Styles.SyntaxNumber.Render("x"))
			assert.NotEmpty(t, th.Styles.SyntaxComment.Render("x"))
			assert.NotEmpty(t, th.Styles.SyntaxFunction.Render("x"))
			assert.NotEmpty(t, th.Styles.SyntaxType.Render("x"))
			assert.NotEmpty(t, th.Styles.SyntaxOperator.Render("x"))

			// Diff hunk
			assert.NotEmpty(t, th.Styles.DiffHunk.Render("x"))

			// Git branch/tag
			assert.NotEmpty(t, th.Styles.GitBranch.Render("x"))
			assert.NotEmpty(t, th.Styles.GitTag.Render("x"))
		})
	}
}

// ---------------------------------------------------------------------------
// Variant
// ---------------------------------------------------------------------------

func TestCatppuccinVariant(t *testing.T) {
	th, err := Load("catppuccin")
	require.NoError(t, err)
	assert.Equal(t, "mocha", th.Variant)
}

func TestDefaultThemeHasNoVariant(t *testing.T) {
	th, err := Load("default")
	require.NoError(t, err)
	assert.Empty(t, th.Variant)
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func TestValidateColorsRejectsMissingFields(t *testing.T) {
	err := validateColors(Colors{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing color values")
}

func TestValidateColorsRejectsSingleMissing(t *testing.T) {
	th, err := Load("default")
	require.NoError(t, err)

	c := th.Colors
	c.Background = ""
	err = validateColors(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "background")
}

// ---------------------------------------------------------------------------
// BuiltinNames
// ---------------------------------------------------------------------------

func TestBuiltinNamesReturnsExpected(t *testing.T) {
	names := BuiltinNames()
	assert.Contains(t, names, "default")
	assert.Contains(t, names, "catppuccin")
	assert.Contains(t, names, "tokyonight")
	assert.Contains(t, names, "gruvbox")
	assert.Len(t, names, 4)
}

func TestBuiltinNamesReturnsCopy(t *testing.T) {
	a := BuiltinNames()
	b := BuiltinNames()
	a[0] = "mutated"
	assert.NotEqual(t, a[0], b[0], "BuiltinNames should return a copy")
}

// ---------------------------------------------------------------------------
// looksLikePath
// ---------------------------------------------------------------------------

func TestLooksLikePath(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"default", false},
		{"catppuccin", false},
		{"/path/to/theme.toml", true},
		{`C:\themes\custom.toml`, true},
		{"my-theme.toml", true},
		{"themes/dark", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, looksLikePath(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// Color accessor
// ---------------------------------------------------------------------------

func TestColorsColorMethod(t *testing.T) {
	c := Colors{Background: "#FF0000"}
	clr := c.Color("#FF0000")
	assert.NotNil(t, clr, "Color() should return a non-nil image/color.Color")
}

// ---------------------------------------------------------------------------
// Helper — complete theme TOML for custom-file tests
// ---------------------------------------------------------------------------

const completeTestTOML = `[meta]
name = "test"

[colors]
background = "#000000"
foreground = "#FFFFFF"
cursor     = "#FFFFFF"
selection  = "#333333"

[colors.normal]
black   = "#111111"
red     = "#FF0000"
green   = "#00FF00"
yellow  = "#FFFF00"
blue    = "#0000FF"
magenta = "#FF00FF"
cyan    = "#00FFFF"
white   = "#EEEEEE"

[colors.bright]
black   = "#222222"
red     = "#FF0000"
green   = "#00FF00"
yellow  = "#FFFF00"
blue    = "#0000FF"
magenta = "#FF00FF"
cyan    = "#00FFFF"
white   = "#FFFFFF"

[ui]
border          = "#333333"
border_focused  = "#FF0000"
status_bar_bg   = "#111111"
status_bar_fg   = "#EEEEEE"
tab_active_bg   = "#FF0000"
tab_active_fg   = "#000000"
tab_inactive_bg = "#333333"
tab_inactive_fg = "#EEEEEE"
selection_bg    = "#444444"
selection_fg    = "#FFFFFF"
cursor_line     = "#222222"

[syntax]
keyword  = "#FF0000"
string   = "#00FF00"
number   = "#0000FF"
comment  = "#888888"
function = "#FFFF00"
type     = "#FF00FF"
operator = "#00FFFF"

[diff]
added   = "#00FF00"
removed = "#FF0000"
header  = "#FFFF00"
hunk    = "#FF00FF"
context = "#888888"

[git]
staged    = "#00FF00"
unstaged  = "#FFFF00"
untracked = "#888888"
conflict  = "#FF0000"
branch    = "#FF00FF"
tag       = "#00FFFF"

[notify]
info    = "#0000FF"
warn    = "#FFFF00"
error   = "#FF0000"
success = "#00FF00"

[files]
directory  = "#0000FF"
default    = "#CCCCCC"
executable = "#00FF00"
symlink    = "#00FFFF"
`
