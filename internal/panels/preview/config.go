package preview

// Config defines the configuration subset needed by the preview panel.
// The concrete config.PreviewConfig satisfies this interface, but tests
// and embedders can supply lightweight stubs without importing the config
// package.
//
// This follows the narrow-interface injection pattern described in
// CONTRIBUTING.md § "Config Interface Pattern".
type Config interface {
	GetTheme() string
	GetMaxFileSize() int
	GetSyntaxHighlighting() bool
	GetLineNumbers() bool
	GetWordWrap() bool
	GetRenderMarkdown() bool
}
