// Package git wraps the git CLI to provide typed, safe access to git
// operations. All index-mutating operations are serialised through an
// OpQueue to prevent concurrent writes from corrupting repository state.
// Read operations run concurrently against the same queue.
//
// TODO(arch): This package has grown to 27+ files spanning 15+ distinct
// domains (blame, bisect, branch, diff, difftree, log, merge, queue, reflog,
// remote, reset, revert, stage_patch, stash, tag, undo, url, worktree).
// A future refactor should split it into sub-packages (e.g. git/branch,
// git/stash, git/log) to restore single-responsibility at the package level.
// See https://github.com/jongio/grut/issues/167 for context.
package git
