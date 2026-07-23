package tui

// registry.go defines the OverlayCreator interface consumed by the TUI model.
// The concrete implementation lives in internal/overlayreg, keeping this
// package free of direct imports to overlay panel packages.

import (
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/keymap"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/panels"
)

// OverlayCreator abstracts overlay panel construction so that the tui package
// does not import concrete overlay panel packages (bookmarks, fuzzyfinder,
// help, settings, welcome) directly.
type OverlayCreator interface {
	NewBookmarkPanel() panels.Panel
	NewCommandLogPanel() panels.Panel
	NewHelpPanel() panels.Panel
	NewWelcomePanel() panels.Panel
	NewSettingsPanel(currentPos layout.PreviewPosition, currentTheme string, themeNames []string, actionsCfg config.ActionsConfig) panels.Panel
	NewFuzzyFinder(mode string, bindings []keymap.Binding) panels.Panel
}
