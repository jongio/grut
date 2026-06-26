package fuzzyfinder

const (
	categoryBookmark   = "bookmark"
	categoryCommand    = "command"
	categoryDirectory  = "directory"
	categoryFile       = "file"
	categoryGitChanged = "git"
	actionCursorDown   = "cursor_down"
	dirGit             = ".git"
)

// DefaultCategoryFile returns the file category name for overlay factories.
func DefaultCategoryFile() string { return categoryFile }

// DefaultCategoryCommand returns the command category name for overlay factories.
func DefaultCategoryCommand() string { return categoryCommand }

// DefaultCategoryDirectory returns the directory category name for overlay factories.
func DefaultCategoryDirectory() string { return categoryDirectory }
