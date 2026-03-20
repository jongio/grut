package theme

import (
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/adrg/xdg"
	toml "github.com/pelletier/go-toml/v2"
)

//go:embed themes/*.toml
var builtinThemes embed.FS

// builtinNames lists the theme names that ship with the binary.
var builtinNames = []string{"default", "catppuccin", "tokyonight", "gruvbox"}

// ---------------------------------------------------------------------------
// TOML file structure — matches the nested theme file format.
// ---------------------------------------------------------------------------

type themeFile struct {
	Meta   fileMeta   `toml:"meta"`
	Colors fileColors `toml:"colors"`
	UI     fileUI     `toml:"ui"`
	Syntax fileSyntax `toml:"syntax"`
	Diff   fileDiff   `toml:"diff"`
	Git    fileGit    `toml:"git"`
	Notify fileNotify `toml:"notify"`
	Files  fileFiles  `toml:"files"`
}

type fileMeta struct {
	Name    string `toml:"name"`
	Variant string `toml:"variant"`
}

type fileColors struct {
	Background string   `toml:"background"`
	Foreground string   `toml:"foreground"`
	Cursor     string   `toml:"cursor"`
	Selection  string   `toml:"selection"`
	Normal     fileANSI `toml:"normal"`
	Bright     fileANSI `toml:"bright"`
}

type fileANSI struct {
	Black   string `toml:"black"`
	Red     string `toml:"red"`
	Green   string `toml:"green"`
	Yellow  string `toml:"yellow"`
	Blue    string `toml:"blue"`
	Magenta string `toml:"magenta"`
	Cyan    string `toml:"cyan"`
	White   string `toml:"white"`
}

type fileUI struct {
	Border         string `toml:"border"`
	BorderFocused  string `toml:"border_focused"`
	StatusBarBg    string `toml:"status_bar_bg"`
	StatusBarFg    string `toml:"status_bar_fg"`
	TitleBg        string `toml:"title_bg"`
	TitleFocusedBg string `toml:"title_focused_bg"`
	TabActiveBg    string `toml:"tab_active_bg"`
	TabActiveFg    string `toml:"tab_active_fg"`
	TabInactiveBg  string `toml:"tab_inactive_bg"`
	TabInactiveFg  string `toml:"tab_inactive_fg"`
	SelectionBg    string `toml:"selection_bg"`
	SelectionFg    string `toml:"selection_fg"`
	CursorLine     string `toml:"cursor_line"`
}

type fileSyntax struct {
	Keyword  string `toml:"keyword"`
	String   string `toml:"string"`
	Number   string `toml:"number"`
	Comment  string `toml:"comment"`
	Function string `toml:"function"`
	Type     string `toml:"type"`
	Operator string `toml:"operator"`
}

type fileDiff struct {
	Added   string `toml:"added"`
	Removed string `toml:"removed"`
	Header  string `toml:"header"`
	Hunk    string `toml:"hunk"`
	Context string `toml:"context"`
}

type fileGit struct {
	Staged    string `toml:"staged"`
	Unstaged  string `toml:"unstaged"`
	Untracked string `toml:"untracked"`
	Conflict  string `toml:"conflict"`
	Branch    string `toml:"branch"`
	Tag       string `toml:"tag"`
}

type fileNotify struct {
	Info    string `toml:"info"`
	Warn    string `toml:"warn"`
	Error   string `toml:"error"`
	Success string `toml:"success"`
}

type fileFiles struct {
	Directory  string `toml:"directory"`
	Default    string `toml:"default"`
	Executable string `toml:"executable"`
	Symlink    string `toml:"symlink"`
}

// toColors converts the nested TOML representation into the flat Colors
// struct that the rest of the application consumes.
func (tf *themeFile) toColors() Colors {
	titleBg := tf.UI.TitleBg
	if titleBg == "" {
		titleBg = tf.UI.StatusBarBg // sensible fallback
	}
	titleFocusedBg := tf.UI.TitleFocusedBg
	if titleFocusedBg == "" {
		titleFocusedBg = tf.UI.StatusBarBg
	}

	return Colors{
		// Base
		Background: tf.Colors.Background,
		Foreground: tf.Colors.Foreground,
		Cursor:     tf.Colors.Cursor,

		// ANSI normal
		NormalBlack:   tf.Colors.Normal.Black,
		NormalRed:     tf.Colors.Normal.Red,
		NormalGreen:   tf.Colors.Normal.Green,
		NormalYellow:  tf.Colors.Normal.Yellow,
		NormalBlue:    tf.Colors.Normal.Blue,
		NormalMagenta: tf.Colors.Normal.Magenta,
		NormalCyan:    tf.Colors.Normal.Cyan,
		NormalWhite:   tf.Colors.Normal.White,

		// ANSI bright
		BrightBlack:   tf.Colors.Bright.Black,
		BrightRed:     tf.Colors.Bright.Red,
		BrightGreen:   tf.Colors.Bright.Green,
		BrightYellow:  tf.Colors.Bright.Yellow,
		BrightBlue:    tf.Colors.Bright.Blue,
		BrightMagenta: tf.Colors.Bright.Magenta,
		BrightCyan:    tf.Colors.Bright.Cyan,
		BrightWhite:   tf.Colors.Bright.White,

		// UI
		BorderFocused:   tf.UI.BorderFocused,
		BorderUnfocused: tf.UI.Border,
		StatusBarBg:     tf.UI.StatusBarBg,
		StatusBarFg:     tf.UI.StatusBarFg,
		TitleBg:         titleBg,
		TitleFocusedBg:  titleFocusedBg,
		TabActiveBg:     tf.UI.TabActiveBg,
		TabActiveFg:     tf.UI.TabActiveFg,
		TabInactiveBg:   tf.UI.TabInactiveBg,
		TabInactiveFg:   tf.UI.TabInactiveFg,
		SelectionBg:     tf.UI.SelectionBg,
		SelectionFg:     tf.UI.SelectionFg,
		CursorLine:      tf.UI.CursorLine,

		// Syntax
		SyntaxKeyword:  tf.Syntax.Keyword,
		SyntaxString:   tf.Syntax.String,
		SyntaxNumber:   tf.Syntax.Number,
		SyntaxComment:  tf.Syntax.Comment,
		SyntaxFunction: tf.Syntax.Function,
		SyntaxType:     tf.Syntax.Type,
		SyntaxOperator: tf.Syntax.Operator,

		// Diff
		DiffAdded:   tf.Diff.Added,
		DiffRemoved: tf.Diff.Removed,
		DiffContext: tf.Diff.Context,
		DiffHeader:  tf.Diff.Header,
		DiffHunk:    tf.Diff.Hunk,

		// Git
		GitStaged:    tf.Git.Staged,
		GitUnstaged:  tf.Git.Unstaged,
		GitUntracked: tf.Git.Untracked,
		GitConflict:  tf.Git.Conflict,
		GitBranch:    tf.Git.Branch,
		GitTag:       tf.Git.Tag,

		// Notifications
		NotifyInfo:    tf.Notify.Info,
		NotifyWarn:    tf.Notify.Warn,
		NotifyError:   tf.Notify.Error,
		NotifySuccess: tf.Notify.Success,

		// Files
		FileDirectory:  tf.Files.Directory,
		FileDefault:    tf.Files.Default,
		FileExecutable: tf.Files.Executable,
		FileSymlink:    tf.Files.Symlink,
	}
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Load resolves a theme by name or file path and returns the fully
// initialised Theme with pre-built Styles.
//
// Resolution order for plain names (no path separators):
//  1. Custom theme directory (~/.config/grut/themes/{name}.toml)
//  2. Built-in embedded themes
//
// If the name looks like a file path it is read directly from disk.
// On any failure for a custom path, Load falls back to the "default"
// theme and logs a warning.
func Load(name string) (*Theme, error) {
	// File path — read directly from disk.
	if looksLikePath(name) {
		data, err := os.ReadFile(name)
		if err != nil {
			slog.Warn("custom theme not found, falling back to default",
				"path", name,
				"error", err,
			)
			return Load("default")
		}
		return parse(name, data)
	}

	// 1. Custom theme directory takes precedence.
	customPath := filepath.Join(ThemeDir(), name+".toml")
	if data, err := os.ReadFile(customPath); err == nil {
		return parse(name, data)
	}

	// 2. Built-in theme.
	if data, err := builtinThemes.ReadFile("themes/" + name + ".toml"); err == nil {
		return parse(name, data)
	}

	// Unknown name — fall back to "default" so a stale config value
	// (e.g. a removed theme) doesn't prevent the app from launching.
	if name != "default" {
		slog.Warn("unknown theme, falling back to default",
			"theme", name,
			"available", builtinNames,
		)
		return Load("default")
	}
	return nil, fmt.Errorf("unknown theme %q (available: %s)",
		name, strings.Join(builtinNames, ", "))
}

// LoadFromFile loads a theme from a specific file path on disk.
// Unlike Load, it does not fall back to defaults on error.
func LoadFromFile(path string) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading theme file %s: %w", path, err)
	}
	name := strings.TrimSuffix(filepath.Base(path), ".toml")
	return parse(name, data)
}

// ListThemes returns the names of all available themes (built-in and
// custom). Custom themes in the theme directory are included and may
// shadow built-in names.
func ListThemes() []string {
	seen := make(map[string]struct{})
	var names []string

	// Built-in themes.
	for _, n := range builtinNames {
		if _, ok := seen[n]; !ok {
			seen[n] = struct{}{}
			names = append(names, n)
		}
	}

	// Custom themes from theme directory.
	dir := ThemeDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Directory may not exist; that's fine.
		slices.Sort(names)
		return names
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		n := strings.TrimSuffix(e.Name(), ".toml")
		if _, ok := seen[n]; !ok {
			seen[n] = struct{}{}
			names = append(names, n)
		}
	}

	slices.Sort(names)
	return names
}

// themeDirOverride allows tests to redirect the custom theme directory.
// When empty, the default XDG-based path is used.
var themeDirOverride string

// ThemeDir returns the path to the user's custom theme directory.
func ThemeDir() string {
	if themeDirOverride != "" {
		return themeDirOverride
	}
	return filepath.Join(xdg.ConfigHome, "grut", "themes")
}

// BuiltinNames returns the list of themes that ship with the binary.
func BuiltinNames() []string {
	out := make([]string, len(builtinNames))
	copy(out, builtinNames)
	return out
}

// ---------------------------------------------------------------------------
// Internals
// ---------------------------------------------------------------------------

// parse unmarshals TOML bytes into a Theme and builds Styles.
func parse(name string, data []byte) (*Theme, error) {
	var tf themeFile
	if err := toml.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parsing theme %q: %w", name, err)
	}

	colors := tf.toColors()

	if err := validateColors(colors); err != nil {
		return nil, fmt.Errorf("theme %q: %w", name, err)
	}

	return &Theme{
		Name:    name,
		Variant: tf.Meta.Variant,
		Colors:  colors,
		Styles:  buildStyles(colors),
	}, nil
}

// validateColors ensures every color field is populated (non-empty string).
// Uses explicit field checks rather than reflection for compile-time safety.
func validateColors(c Colors) error {
	checks := []struct {
		name  string
		value string
	}{
		// Base
		{"background", c.Background},
		{"foreground", c.Foreground},
		{"cursor", c.Cursor},

		// ANSI normal
		{"normal.black", c.NormalBlack},
		{"normal.red", c.NormalRed},
		{"normal.green", c.NormalGreen},
		{"normal.yellow", c.NormalYellow},
		{"normal.blue", c.NormalBlue},
		{"normal.magenta", c.NormalMagenta},
		{"normal.cyan", c.NormalCyan},
		{"normal.white", c.NormalWhite},

		// ANSI bright
		{"bright.black", c.BrightBlack},
		{"bright.red", c.BrightRed},
		{"bright.green", c.BrightGreen},
		{"bright.yellow", c.BrightYellow},
		{"bright.blue", c.BrightBlue},
		{"bright.magenta", c.BrightMagenta},
		{"bright.cyan", c.BrightCyan},
		{"bright.white", c.BrightWhite},

		// UI
		{"ui.border", c.BorderUnfocused},
		{"ui.border_focused", c.BorderFocused},
		{"ui.status_bar_bg", c.StatusBarBg},
		{"ui.status_bar_fg", c.StatusBarFg},
		{"ui.tab_active_bg", c.TabActiveBg},
		{"ui.tab_active_fg", c.TabActiveFg},
		{"ui.tab_inactive_bg", c.TabInactiveBg},
		{"ui.tab_inactive_fg", c.TabInactiveFg},
		{"ui.selection_bg", c.SelectionBg},
		{"ui.selection_fg", c.SelectionFg},
		{"ui.cursor_line", c.CursorLine},

		// Syntax
		{"syntax.keyword", c.SyntaxKeyword},
		{"syntax.string", c.SyntaxString},
		{"syntax.number", c.SyntaxNumber},
		{"syntax.comment", c.SyntaxComment},
		{"syntax.function", c.SyntaxFunction},
		{"syntax.type", c.SyntaxType},
		{"syntax.operator", c.SyntaxOperator},

		// Diff
		{"diff.added", c.DiffAdded},
		{"diff.removed", c.DiffRemoved},
		{"diff.context", c.DiffContext},
		{"diff.header", c.DiffHeader},
		{"diff.hunk", c.DiffHunk},

		// Git
		{"git.staged", c.GitStaged},
		{"git.unstaged", c.GitUnstaged},
		{"git.untracked", c.GitUntracked},
		{"git.conflict", c.GitConflict},
		{"git.branch", c.GitBranch},
		{"git.tag", c.GitTag},

		// Notifications
		{"notify.info", c.NotifyInfo},
		{"notify.warn", c.NotifyWarn},
		{"notify.error", c.NotifyError},
		{"notify.success", c.NotifySuccess},

		// Files
		{"files.directory", c.FileDirectory},
		{"files.default", c.FileDefault},
		{"files.executable", c.FileExecutable},
		{"files.symlink", c.FileSymlink},
	}

	var missing []string
	for _, check := range checks {
		if check.value == "" {
			missing = append(missing, check.name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing color values: %s", strings.Join(missing, ", "))
	}
	return nil
}

// looksLikePath returns true if the name contains a path separator or
// common file extension, suggesting it is a file path rather than a
// built-in theme name.
func looksLikePath(name string) bool {
	return strings.ContainsAny(name, `/\`) || strings.HasSuffix(name, ".toml")
}
