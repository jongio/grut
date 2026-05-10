package theme

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// LoadFromFile
// ---------------------------------------------------------------------------

func TestLoadFromFileSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ocean.toml")
	require.NoError(t, os.WriteFile(path, []byte(completeTestTOML), 0o600))

	th, err := LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "ocean", th.Name) // derived from filename
	assert.Equal(t, "#000000", th.Colors.Background)
	assert.Equal(t, "#FFFFFF", th.Colors.Foreground)
}

func TestLoadFromFileReturnsErrorOnMissing(t *testing.T) {
	_, err := LoadFromFile("/does/not/exist.toml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading theme file")
}

func TestLoadFromFileReturnsErrorOnInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	require.NoError(t, os.WriteFile(path, []byte("{{invalid"), 0o600))

	_, err := LoadFromFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing theme")
}

func TestLoadFromFileReturnsErrorOnMissingColors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sparse.toml")
	sparse := `[meta]
name = "sparse"
[colors]
background = "#000000"
`
	require.NoError(t, os.WriteFile(path, []byte(sparse), 0o600))

	_, err := LoadFromFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing color values")
}

// ---------------------------------------------------------------------------
// ListThemes — built-in only (no custom dir)
// ---------------------------------------------------------------------------

func TestListThemesIncludesAllBuiltin(t *testing.T) {
	themes := ListThemes()
	for _, name := range BuiltinNames() {
		assert.Contains(t, themes, name, "ListThemes should include built-in %q", name)
	}
}

func TestListThemesReturnsSorted(t *testing.T) {
	themes := ListThemes()
	for i := 1; i < len(themes); i++ {
		assert.LessOrEqual(t, themes[i-1], themes[i],
			"ListThemes should be sorted: %q > %q", themes[i-1], themes[i])
	}
}

// ---------------------------------------------------------------------------
// ListThemes — with custom theme directory
// ---------------------------------------------------------------------------

func TestListThemesIncludesCustom(t *testing.T) {
	dir := t.TempDir()
	themeDir := filepath.Join(dir, "themes")
	require.NoError(t, os.MkdirAll(themeDir, 0o755))

	// Override the theme directory for this test.
	themeDirOverride = themeDir
	t.Cleanup(func() { themeDirOverride = "" })

	// Create a custom theme file.
	customPath := filepath.Join(themeDir, "ocean.toml")
	require.NoError(t, os.WriteFile(customPath, []byte(completeTestTOML), 0o600))

	themes := ListThemes()
	assert.Contains(t, themes, "ocean", "ListThemes should include custom theme")
	// Built-in themes should still be present.
	assert.Contains(t, themes, "default")
}

func TestListThemesIgnoresNonTOML(t *testing.T) {
	dir := t.TempDir()
	themeDir := filepath.Join(dir, "themes")
	require.NoError(t, os.MkdirAll(themeDir, 0o755))

	themeDirOverride = themeDir
	t.Cleanup(func() { themeDirOverride = "" })

	// Create a non-TOML file.
	require.NoError(t, os.WriteFile(
		filepath.Join(themeDir, "readme.txt"),
		[]byte("not a theme"), 0o600,
	))

	themes := ListThemes()
	assert.NotContains(t, themes, "readme")
}

// ---------------------------------------------------------------------------
// Custom theme overrides built-in
// ---------------------------------------------------------------------------

func TestCustomThemeOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	themeDir := filepath.Join(dir, "themes")
	require.NoError(t, os.MkdirAll(themeDir, 0o755))

	themeDirOverride = themeDir
	t.Cleanup(func() { themeDirOverride = "" })

	// Create a custom "default" theme with a distinctive color.
	require.NoError(t, os.WriteFile(
		filepath.Join(themeDir, "default.toml"),
		[]byte(completeTestTOML), 0o600,
	))

	th, err := Load("default")
	require.NoError(t, err)
	// Custom file has "#000000" as background, built-in has "#1A1A2E".
	assert.Equal(t, "#000000", th.Colors.Background,
		"custom theme should override built-in")
}

// ---------------------------------------------------------------------------
// ThemeDir
// ---------------------------------------------------------------------------

func TestThemeDirEndsWithThemes(t *testing.T) {
	td := ThemeDir()
	assert.True(t, filepath.Base(td) == "themes",
		"ThemeDir should end with 'themes', got %q", td)
	assert.Contains(t, td, "grut")
}

// ---------------------------------------------------------------------------
// Theme TOML parsing — all sections populated
// ---------------------------------------------------------------------------

func TestThemeTOMLParsing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "full.toml")
	require.NoError(t, os.WriteFile(path, []byte(completeTestTOML), 0o600))

	th, err := LoadFromFile(path)
	require.NoError(t, err)

	// Base colors
	assert.Equal(t, "#000000", th.Colors.Background)
	assert.Equal(t, "#FFFFFF", th.Colors.Foreground)
	assert.Equal(t, "#FFFFFF", th.Colors.Cursor)

	// ANSI normal
	assert.Equal(t, "#111111", th.Colors.NormalBlack)
	assert.Equal(t, "#FF0000", th.Colors.NormalRed)
	assert.Equal(t, "#00FF00", th.Colors.NormalGreen)

	// ANSI bright
	assert.Equal(t, "#222222", th.Colors.BrightBlack)
	assert.Equal(t, "#FFFFFF", th.Colors.BrightWhite)

	// UI
	assert.Equal(t, "#FF0000", th.Colors.BorderFocused)
	assert.Equal(t, "#333333", th.Colors.BorderUnfocused)
	assert.Equal(t, "#FF0000", th.Colors.TabActiveBg)
	assert.Equal(t, "#000000", th.Colors.TabActiveFg)
	assert.Equal(t, "#444444", th.Colors.SelectionBg)
	assert.Equal(t, "#222222", th.Colors.CursorLine)

	// Syntax
	assert.Equal(t, "#FF0000", th.Colors.SyntaxKeyword)
	assert.Equal(t, "#00FF00", th.Colors.SyntaxString)
	assert.Equal(t, "#0000FF", th.Colors.SyntaxNumber)
	assert.Equal(t, "#888888", th.Colors.SyntaxComment)
	assert.Equal(t, "#FFFF00", th.Colors.SyntaxFunction)
	assert.Equal(t, "#FF00FF", th.Colors.SyntaxType)
	assert.Equal(t, "#00FFFF", th.Colors.SyntaxOperator)

	// Diff
	assert.Equal(t, "#00FF00", th.Colors.DiffAdded)
	assert.Equal(t, "#FF0000", th.Colors.DiffRemoved)
	assert.Equal(t, "#FFFF00", th.Colors.DiffHeader)
	assert.Equal(t, "#FF00FF", th.Colors.DiffHunk)
	assert.Equal(t, "#888888", th.Colors.DiffContext)

	// Git
	assert.Equal(t, "#00FF00", th.Colors.GitStaged)
	assert.Equal(t, "#FF00FF", th.Colors.GitBranch)
	assert.Equal(t, "#00FFFF", th.Colors.GitTag)

	// Notifications
	assert.Equal(t, "#0000FF", th.Colors.NotifyInfo)
	assert.Equal(t, "#00FF00", th.Colors.NotifySuccess)

	// Files
	assert.Equal(t, "#0000FF", th.Colors.FileDirectory)
	assert.Equal(t, "#00FFFF", th.Colors.FileSymlink)
}

func TestThemeTOMLMetaVariant(t *testing.T) {
	dir := t.TempDir()
	content := completeTestTOML
	// The completeTestTOML does not set variant — default is empty.
	path := filepath.Join(dir, "novar.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	th, err := LoadFromFile(path)
	require.NoError(t, err)
	assert.Empty(t, th.Variant)
}

// ---------------------------------------------------------------------------
// containsDotDot — path traversal detection
// ---------------------------------------------------------------------------

func TestContainsDotDot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"simple.toml", false},
		{"/absolute/path/theme.toml", false},
		{"relative/path/theme.toml", false},
		{"../etc/passwd", true},
		{"a/../../b", true},
		{`..\..\windows\system32`, true},
		{"theme..toml", false}, // double dot in filename, not a component
		{"a/.../b", false},     // triple dot is not traversal
		{".hidden/theme.toml", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			got := containsDotDot(tt.path)
			assert.Equal(t, tt.want, got, "containsDotDot(%q)", tt.path)
		})
	}
}

// ---------------------------------------------------------------------------
// Load — path traversal rejection
// ---------------------------------------------------------------------------

func TestLoadRejectsTraversalPath(t *testing.T) {
	// A theme name with path traversal should fall back to default,
	// not read from an arbitrary location.
	th, err := Load("../../etc/passwd")
	require.NoError(t, err)
	// Should have fallen back to the built-in "default" theme.
	assert.Equal(t, "default", th.Name)
}

func TestLoadAcceptsCleanAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.toml")
	require.NoError(t, os.WriteFile(path, []byte(completeTestTOML), 0o600))

	th, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "#000000", th.Colors.Background)
}
