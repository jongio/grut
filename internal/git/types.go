package git

import "time"

// ---------------------------------------------------------------------------
// Format constants used across git output parsing
// ---------------------------------------------------------------------------

const (
	// FieldSep is the ASCII Record Separator (\x1e) used as the field
	// delimiter in structured git format strings. This allows body text
	// containing newlines without breaking record boundaries.
	FieldSep = "\x1e"

	// RecordEnd is the ASCII Unit Separator (\x1f) used to mark the end
	// of a record in multi-record git output (e.g., git log).
	RecordEnd = "\x1f"

	// ShortHashLen is the standard truncation length for commit short hashes
	// displayed in the UI.
	ShortHashLen = 7
)

// StatusCode represents the status of a file in the index or worktree.
type StatusCode byte

const (
	StatusUnmodified StatusCode = ' '
	StatusModified   StatusCode = 'M'
	StatusAdded      StatusCode = 'A'
	StatusDeleted    StatusCode = 'D'
	StatusRenamed    StatusCode = 'R'
	StatusCopied     StatusCode = 'C'
	StatusUntracked  StatusCode = '?'
	StatusIgnored    StatusCode = '!'
	StatusConflict   StatusCode = 'U'
)

// String returns the single-character representation of the status code.
func (s StatusCode) String() string {
	return string(s)
}

// FileStatus represents the status of a single file from git status.
type FileStatus struct {
	Path           string
	StagedStatus   StatusCode // Index status
	WorktreeStatus StatusCode // Worktree status
	OrigPath       string     // For renames
}

// Branch represents a git branch with tracking information.
type Branch struct {
	Name      string
	IsRemote  bool
	IsCurrent bool
	Upstream  string
	Ahead     int
	Behind    int
	Hash      string
}

// Commit represents a parsed git log entry.
type Commit struct {
	Hash        string
	ShortHash   string
	Author      string
	AuthorEmail string
	Date        time.Time
	Subject     string
	Body        string
	Parents     []string // Parent hashes for graph rendering
	Refs        []string // Branch/tag refs
}

// DiffOpts configures a git diff operation.
type DiffOpts struct {
	Staged    bool   // Compare index vs HEAD (--cached)
	Path      string // Limit diff to a specific path
	Context   int    // Lines of context around changes (default 3)
	CommitA   string // Compare two commits
	CommitB   string
	NameOnly  bool // Only show file names
	StatOnly  bool // Only show stat summary
	IgnoreAll bool // Ignore all whitespace changes
}

// FileDiff represents the diff output for a single file.
type FileDiff struct {
	Path     string
	OldPath  string // For renames
	Hunks    []Hunk
	IsBinary bool
}

// Hunk represents a contiguous block of changes in a diff.
type Hunk struct {
	OldStart     int
	OldLines     int
	NewStart     int
	NewLines     int
	Header       string
	Lines        []DiffLine
	NoNewlineEOF bool // true when hunk ends with "\ No newline at end of file"
}

// DiffLineType identifies the kind of line in a diff.
type DiffLineType int

const (
	DiffLineContext DiffLineType = iota
	DiffLineAdded
	DiffLineRemoved
)

// DiffLine represents a single line within a diff hunk.
type DiffLine struct {
	Type    DiffLineType
	Content string
	OldLine int
	NewLine int
}

// LogOpts configures a git log operation.
type LogOpts struct {
	MaxCount int    // Max number of commits to return
	Skip     int    // Number of commits to skip (for pagination)
	Since    string // Show commits after this date
	Until    string // Show commits before this date
	Author   string // Filter by author
	Grep     string // Filter by commit message
	Path     string // Filter by path
	All      bool   // Show all refs
	Graph    bool   // Include graph data
	Ref      string // Starting ref (default HEAD)
}

// CommitOpts configures a git commit operation.
type CommitOpts struct {
	AllowEmpty bool
	Amend      bool
	RewordOnly bool   // When true with Amend, only change message (--only flag)
	Fixup      string // Commit hash to fixup (--fixup=<hash>)
	Sign       bool
	Author     string // Override author "Name <email>"
}

// PushOpts configures a git push operation.
type PushOpts struct {
	Remote      string
	Branch      string
	Force       bool
	ForceWith   bool // --force-with-lease
	SetUpstream bool
	Tags        bool
}

// PullOpts configures a git pull operation.
type PullOpts struct {
	Remote   string
	Branch   string
	Rebase   bool
	NoRebase bool
}

// FetchOpts configures a git fetch operation.
type FetchOpts struct {
	Remote string
	Prune  bool
	Tags   bool
	All    bool
}

// StashOpts configures a git stash push operation.
type StashOpts struct {
	Message   string
	KeepIndex bool
	Staged    bool
	Paths     []string // Specific paths to stash
}

// MergeOpts configures a git merge operation.
type MergeOpts struct {
	NoFF    bool
	FFOnly  bool
	Squash  bool
	Message string
}

// RebaseOpts configures a git rebase operation.
type RebaseOpts struct {
	Interactive bool
}

// Tag represents a git tag.
type Tag struct {
	Name        string
	Hash        string
	Message     string // Empty for lightweight tags
	IsAnnotated bool
	Tagger      string
	Date        time.Time
}

// Worktree represents a git worktree entry.
type Worktree struct {
	Path   string
	Head   string
	Branch string
	Bare   bool
}

// StashEntry represents a single stash entry.
type StashEntry struct {
	Index   int
	Message string
	Branch  string
	Date    time.Time
	Hash    string
}

// BlameLine represents a single line from git blame output.
type BlameLine struct {
	Hash    string
	Author  string
	Date    time.Time
	LineNo  int
	Content string
}

// Remote represents a configured git remote.
type Remote struct {
	Name     string
	FetchURL string
	PushURL  string
}

// ReflogEntry represents a single reflog entry.
type ReflogEntry struct {
	Hash    string
	Action  string
	Message string
	Date    time.Time
}
