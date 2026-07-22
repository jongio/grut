package config

// ---------------------------------------------------------------------------
// Getter methods for config sub-structs.
//
// These exist so that concrete config types satisfy narrow interfaces defined
// in consumer packages (e.g. filetree.Config, preview.Config).  Panels depend
// on the interface, not on this package, which inverts the dependency and
// enables testing with lightweight stubs.
//
// See CONTRIBUTING.md § "Config Interface Pattern" for the full rationale.
// ---------------------------------------------------------------------------

// --- FileTreeConfig getters ------------------------------------------------

func (c FileTreeConfig) GetIconMode() string           { return c.IconMode }
func (c FileTreeConfig) GetMaxDepth() int              { return c.MaxDepth }
func (c FileTreeConfig) GetShowHidden() bool           { return c.ShowHidden }
func (c FileTreeConfig) GetShowIcons() bool            { return c.ShowIcons }
func (c FileTreeConfig) GetSortDirectoriesFirst() bool { return c.SortDirectoriesFirst }
func (c FileTreeConfig) GetGitStatusMarkers() bool     { return c.GitStatusMarkers }
func (c FileTreeConfig) GetFollowSymlinks() bool       { return c.FollowSymlinks }
func (c FileTreeConfig) GetPermanentDelete() bool      { return c.PermanentDelete }

// --- PreviewConfig getters -------------------------------------------------

func (c PreviewConfig) GetTheme() string            { return c.Theme }
func (c PreviewConfig) GetMaxFileSize() int         { return c.MaxFileSize }
func (c PreviewConfig) GetSyntaxHighlighting() bool { return c.SyntaxHighlighting }
func (c PreviewConfig) GetLineNumbers() bool        { return c.LineNumbers }
func (c PreviewConfig) GetWordWrap() bool           { return c.WordWrap }
func (c PreviewConfig) GetRenderMarkdown() bool     { return c.RenderMarkdown }
