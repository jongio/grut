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
	OrigPath       string     // For renames
	StagedStatus   StatusCode // Index status
	WorktreeStatus StatusCode // Worktree status
}

// Branch represents a git branch with tracking information.
type Branch struct {
	Name      string
	Upstream  string
	Hash      string
	Ahead     int
	Behind    int
	IsRemote  bool
	IsCurrent bool
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
	Path      string // Limit diff to a specific path
	CommitA   string // Compare two commits
	CommitB   string
	Context   int  // Lines of context around changes (default 3)
	Staged    bool // Compare index vs HEAD (--cached)
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
	Header       string
	Lines        []DiffLine
	OldStart     int
	OldLines     int
	NewStart     int
	NewLines     int
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
	Content string
	Type    DiffLineType
	OldLine int
	NewLine int
}

// LogOpts configures a git log operation.
type LogOpts struct {
	Since    string // Show commits after this date
	Until    string // Show commits before this date
	Author   string // Filter by author
	Grep     string // Filter by commit message
	Path     string // Filter by path
	Ref      string // Starting ref (default HEAD)
	MaxCount int    // Max number of commits to return
	Skip     int    // Number of commits to skip (for pagination)
	All      bool   // Show all refs
	Graph    bool   // Include graph data
}

// CommitOpts configures a git commit operation.
type CommitOpts struct {
	Fixup      string // Commit hash to fixup (--fixup=<hash>)
	Author     string // Override author "Name <email>"
	AllowEmpty bool
	Amend      bool
	RewordOnly bool // When true with Amend, only change message (--only flag)
	Sign       bool
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
	Paths     []string // Specific paths to stash
	KeepIndex bool
	Staged    bool
}

// MergeOpts configures a git merge operation.
type MergeOpts struct {
	Message string
	NoFF    bool
	FFOnly  bool
	Squash  bool
}

// RebaseOpts configures a git rebase operation.
type RebaseOpts struct {
	Interactive bool
}

// Tag represents a git tag.
type Tag struct {
	Date        time.Time
	Name        string
	Hash        string
	Message     string // Empty for lightweight tags
	Tagger      string
	IsAnnotated bool
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
	Date    time.Time
	Message string
	Branch  string
	Hash    string
	Index   int
}

// BlameLine represents a single line from git blame output.
type BlameLine struct {
	Date    time.Time
	Hash    string
	Author  string
	Content string
	LineNo  int
}

// Remote represents a configured git remote.
type Remote struct {
	Name     string
	FetchURL string
	PushURL  string
}

// ReflogEntry represents a single reflog entry.
type ReflogEntry struct {
	Date    time.Time
	Hash    string
	Action  string
	Message string
}
