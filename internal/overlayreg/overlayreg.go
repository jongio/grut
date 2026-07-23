// Package overlayreg provides a concrete overlay panel factory that imports
// individual overlay panel packages. This isolates the import coupling in a
// single bootstrap package so that the tui package depends only on the
// OverlayCreator interface.
package overlayreg

import (
	"os"

	bm "github.com/jongio/grut/internal/bookmarks"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/keymap"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/panels"
	bmpanel "github.com/jongio/grut/internal/panels/bookmarks"
	commandlogpanel "github.com/jongio/grut/internal/panels/commandlog"
	"github.com/jongio/grut/internal/panels/fuzzyfinder"
	helppanel "github.com/jongio/grut/internal/panels/help"
	settingspanel "github.com/jongio/grut/internal/panels/settings"
	welcomepanel "github.com/jongio/grut/internal/panels/welcome"
	"github.com/jongio/grut/internal/theme"
)

// Factory constructs overlay panels (bookmarks, fuzzy finder, help,
// settings, welcome) behind the panels.Panel interface.
type Factory struct {
	theme       *theme.Theme
	bookmarkMgr *bm.Manager
}

// New creates a Factory wired with the shared dependencies that overlay
// panels need.
func New(th *theme.Theme, bmMgr *bm.Manager) *Factory {
	return &Factory{theme: th, bookmarkMgr: bmMgr}
}

// NewBookmarkPanel creates the bookmarks overlay panel.
func (f *Factory) NewBookmarkPanel() panels.Panel {
	return bmpanel.New(f.bookmarkMgr, f.theme)
}

// NewCommandLogPanel creates the git command log overlay panel.
func (f *Factory) NewCommandLogPanel() panels.Panel {
	return commandlogpanel.New(f.theme)
}

// NewHelpPanel creates the help overlay panel.
func (f *Factory) NewHelpPanel() panels.Panel {
	return helppanel.New(f.theme)
}

// NewWelcomePanel creates the welcome overlay panel.
func (f *Factory) NewWelcomePanel() panels.Panel {
	return welcomepanel.New(f.theme)
}

// NewSettingsPanel creates the settings overlay panel with the given
// configuration state.
func (f *Factory) NewSettingsPanel(
	currentPos layout.PreviewPosition,
	currentTheme string,
	themeNames []string,
	actionsCfg config.ActionsConfig,
) panels.Panel {
	return settingspanel.New(currentPos, currentTheme, themeNames, actionsCfg, f.theme)
}

// NewFuzzyFinder creates a fuzzy finder overlay for the given mode.
// Supported modes: "files", "commands", "directories", "todos".
// The bindings parameter is used for the "commands" mode source.
func (f *Factory) NewFuzzyFinder(mode string, bindings []keymap.Binding) panels.Panel {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	sources := []fuzzyfinder.Source{
		fuzzyfinder.NewFileSource(cwd),
		fuzzyfinder.NewDirectorySource(cwd, fuzzyfinder.DefaultDirectorySourceMaxDepth),
		fuzzyfinder.NewCommandSource(bindings),
		fuzzyfinder.NewBookmarkSource(f.bookmarkMgr),
		fuzzyfinder.NewGitChangedSource(cwd),
		fuzzyfinder.NewTodoSource(cwd),
	}
	defaultCategories := []string{fuzzyfinder.DefaultCategoryFile()}
	switch mode {
	case "files":
		defaultCategories = []string{fuzzyfinder.DefaultCategoryFile()}
	case "commands":
		defaultCategories = []string{fuzzyfinder.DefaultCategoryCommand()}
	case "directories":
		defaultCategories = []string{fuzzyfinder.DefaultCategoryDirectory()}
	case "todos":
		defaultCategories = []string{fuzzyfinder.DefaultCategoryTodo()}
	}
	return fuzzyfinder.NewWithDefaultCategories(f.theme, defaultCategories, sources...)
}
