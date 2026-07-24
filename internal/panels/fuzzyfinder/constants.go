package fuzzyfinder

const (
	categoryBookmark     = "bookmark"
	categoryCommand      = "command"
	categoryCustomAction = "custom action"
	categoryDirectory    = "directory"
	categoryFile         = "file"
	categoryGitChanged   = "git"
	categoryTodo         = "todo"
	actionCursorDown     = "cursor_down"
	dirGit               = ".git"
)

// Source names, returned by Source.Name and used as prefix labels.
const (
	sourceNameFiles         = "files"
	sourceNameDirectories   = "directories"
	sourceNameCommands      = "commands"
	sourceNameCustomActions = "custom actions"
	sourceNameBookmarks     = "bookmarks"
	sourceNameGitChanged    = "git changed"
	sourceNameTodos         = "todos"
)

// DefaultCategoryFile returns the file category name for overlay factories.
func DefaultCategoryFile() string { return categoryFile }

// DefaultCategoryCommand returns the command category name for overlay factories.
func DefaultCategoryCommand() string { return categoryCommand }

// DefaultCategoryCustomAction returns the custom action category name for overlay factories.
func DefaultCategoryCustomAction() string { return categoryCustomAction }

// DefaultCategoryDirectory returns the directory category name for overlay factories.
func DefaultCategoryDirectory() string { return categoryDirectory }

// DefaultCategoryTodo returns the todo category name for overlay factories.
func DefaultCategoryTodo() string { return categoryTodo }
