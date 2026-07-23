package git

import "context"

// StatusReader provides read-only git status queries.
type StatusReader interface {
	Status(ctx context.Context) ([]FileStatus, error)
	Diff(ctx context.Context, opts DiffOpts) ([]FileDiff, error)
	Log(ctx context.Context, opts LogOpts) ([]Commit, error)
	Blame(ctx context.Context, path string) ([]BlameLine, error)
	RepoRoot(ctx context.Context) (string, error)
	IsRepo(ctx context.Context) (bool, error)
	DiffTreeFiles(ctx context.Context, hash string) ([]string, error)
	DiffFileNames(ctx context.Context, commitA, commitB string) ([]string, error)
}

// IgnoreChecker can report which paths are ignored by .gitignore rules.
type IgnoreChecker interface {
	IgnoredPaths(ctx context.Context) ([]string, error)
}

// IndexMutator provides index-mutating operations (serialized via queue).
type IndexMutator interface {
	Stage(ctx context.Context, paths []string) error
	Unstage(ctx context.Context, paths []string) error
	StageHunk(ctx context.Context, path string, hunk Hunk) error
	UnstageHunk(ctx context.Context, path string, hunk Hunk) error
	StageLine(ctx context.Context, path string, hunk Hunk, lineIdx int) error
	UnstageLine(ctx context.Context, path string, hunk Hunk, lineIdx int) error
	Commit(ctx context.Context, msg string, opts CommitOpts) (string, error)
}

// BranchManager provides branch and checkout operations.
type BranchManager interface {
	BranchList(ctx context.Context) ([]Branch, error)
	CurrentBranch(ctx context.Context) (Branch, error)
	BranchCreate(ctx context.Context, name string, base string) error
	BranchDelete(ctx context.Context, name string, force bool) error
	BranchRename(ctx context.Context, oldName, newName string) error
	Checkout(ctx context.Context, ref string) error
	HeadSHA(ctx context.Context) (string, error)
}

// RemoteOps provides remote operations.
type RemoteOps interface {
	Push(ctx context.Context, opts PushOpts) error
	Pull(ctx context.Context, opts PullOpts) error
	Fetch(ctx context.Context, opts FetchOpts) error
}

// RemoteListOps provides remote listing and management operations.
type RemoteListOps interface {
	RemoteList(ctx context.Context) ([]Remote, error)
	RemoteAdd(ctx context.Context, name, url string) error
	RemoteRemove(ctx context.Context, name string) error
}

// WorktreeOps provides worktree operations.
type WorktreeOps interface {
	WorktreeList(ctx context.Context) ([]Worktree, error)
	WorktreeAdd(ctx context.Context, path, branch string) error
	WorktreeRemove(ctx context.Context, path string, force bool) error
}

// StashOps provides stash operations.
type StashOps interface {
	StashList(ctx context.Context) ([]StashEntry, error)
	StashShow(ctx context.Context, index int) (string, error)
	StashPush(ctx context.Context, opts StashOpts) error
	StashPop(ctx context.Context, index int) error
	StashApply(ctx context.Context, index int) error
	StashDrop(ctx context.Context, index int) error
}

// TagOps provides tag operations.
type TagOps interface {
	TagList(ctx context.Context) ([]Tag, error)
	TagCreate(ctx context.Context, name, ref, message string) error
	TagDelete(ctx context.Context, name string) error
	TagListRemote(ctx context.Context, remote string) ([]Tag, error)
	TagPush(ctx context.Context, remote, name string) error
	TagPushAll(ctx context.Context, remote string) error
}

// MergeRebaseOps provides merge and rebase operations.
type MergeRebaseOps interface {
	Merge(ctx context.Context, branch string, opts MergeOpts) error
	MergeAbort(ctx context.Context) error
	Rebase(ctx context.Context, onto string, opts RebaseOpts) error
	RebaseContinue(ctx context.Context) error
	RebaseAbort(ctx context.Context) error
	CherryPick(ctx context.Context, commitHash string) error
}

// BisectOps provides bisect operations.
type BisectOps interface {
	BisectStart(ctx context.Context, bad, good string) error
	BisectGood(ctx context.Context) (string, error)
	BisectBad(ctx context.Context) (string, error)
	BisectReset(ctx context.Context) error
}

// ReflogOps provides reflog operations.
type ReflogOps interface {
	Reflog(ctx context.Context, ref string, limit int) ([]ReflogEntry, error)
}

// SubmoduleOps provides submodule status operations.
type SubmoduleOps interface {
	Submodules(ctx context.Context) ([]Submodule, error)
}

// DiscardOps provides operations for discarding unstaged changes.
type DiscardOps interface {
	DiscardFile(ctx context.Context, path string) error
	DiscardAllUnstaged(ctx context.Context) error
}

// RevertOps provides commit revert operations.
type RevertOps interface {
	Revert(ctx context.Context, hash string) error
	RevertContinue(ctx context.Context) error
	RevertAbort(ctx context.Context) error
}

// ResetOps provides reset operations with mode control.
type ResetOps interface {
	Reset(ctx context.Context, ref string, mode ResetMode) error
}

// GitClient composes all sub-interfaces.
type GitClient interface {
	StatusReader
	IndexMutator
	BranchManager
	RemoteOps
	RemoteListOps
	WorktreeOps
	StashOps
	TagOps
	MergeRebaseOps
	BisectOps
	ReflogOps
	SubmoduleOps
	DiscardOps
	RevertOps
	ResetOps
}
