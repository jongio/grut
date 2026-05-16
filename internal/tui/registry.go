package tui

// registry.go centralises overlay panel construction so that app.go depends
// on the panels.Panel interface rather than importing concrete panel packages
// (bookmarks, fuzzyfinder, help, settings, welcome) directly.

import (
	"os"

	bm "github.com/jongio/grut/internal/bookmarks"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/keymap"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/panels"
	bmpanel "github.com/jongio/grut/internal/panels/bookmarks"
	"github.com/jongio/grut/internal/panels/fuzzyfinder"
	helppanel "github.com/jongio/grut/internal/panels/help"
	settingspanel "github.com/jongio/grut/internal/panels/settings"
	welcomepanel "github.com/jongio/grut/internal/panels/welcome"
	"github.com/jongio/grut/internal/theme"
)

// OverlayFactory constructs overlay panels (bookmarks, fuzzy finder, help,
// settings, welcome) behind the panels.Panel interface. The root Model uses
// the factory instead of importing each panel package, reducing coupling.
type OverlayFactory struct {
	theme       *theme.Theme
	bookmarkMgr *bm.Manager
}

// NewOverlayFactory creates a factory wired with the shared dependencies
// that overlay panels need.
func NewOverlayFactory(th *theme.Theme, bmMgr *bm.Manager) *OverlayFactory {
	return &OverlayFactory{theme: th, bookmarkMgr: bmMgr}
}

// NewBookmarkPanel creates the bookmarks overlay panel.
func (f *OverlayFactory) NewBookmarkPanel() panels.Panel {
	return bmpanel.New(f.bookmarkMgr, f.theme)
}

// NewHelpPanel creates the help overlay panel.
func (f *OverlayFactory) NewHelpPanel() panels.Panel {
	return helppanel.New(f.theme)
}

// NewWelcomePanel creates the welcome overlay panel.
func (f *OverlayFactory) NewWelcomePanel() panels.Panel {
	return welcomepanel.New(f.theme)
}

// NewSettingsPanel creates the settings overlay panel with the given
// configuration state.
func (f *OverlayFactory) NewSettingsPanel(
	currentPos layout.PreviewPosition,
	currentTheme string,
	themeNames []string,
	actionsCfg config.ActionsConfig,
) panels.Panel {
	return settingspanel.New(currentPos, currentTheme, themeNames, actionsCfg, f.theme)
}

// NewFuzzyFinder creates a fuzzy finder overlay for the given mode.
// Supported modes: "files", "commands", "directories".
// The bindings parameter is used for the "commands" mode source.
func (f *OverlayFactory) NewFuzzyFinder(mode string, bindings []keymap.Binding) panels.Panel {
	var sources []fuzzyfinder.Source
	switch mode {
	case "files":
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		sources = append(sources, fuzzyfinder.NewFileSource(cwd))
	case "commands":
		sources = append(sources, fuzzyfinder.NewCommandSource(bindings))
	case "directories":
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		sources = append(sources, fuzzyfinder.NewDirectorySource(cwd, fuzzyfinder.DefaultDirectorySourceMaxDepth))
	}
	return fuzzyfinder.New(f.theme, sources...)
}
