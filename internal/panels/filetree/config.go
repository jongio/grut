package filetree

// Config defines the configuration subset needed by the file tree panel.
// The concrete config.FileTreeConfig satisfies this interface, but tests
// and embedders can supply lightweight stubs without importing the config
// package.
//
// This follows the narrow-interface injection pattern described in
// CONTRIBUTING.md § "Config Interface Pattern".
type Config interface {
	GetIconMode() string
	GetMaxDepth() int
	GetShowHidden() bool
	GetShowIcons() bool
	GetSortDirectoriesFirst() bool
	GetGitStatusMarkers() bool
	GetFollowSymlinks() bool
	GetPermanentDelete() bool
}
