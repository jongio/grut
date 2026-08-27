// Package git wraps the git CLI to provide typed, safe access to git
// operations. All index-mutating operations are serialised through an
// OpQueue to prevent concurrent writes from corrupting repository state.
// Read operations run concurrently against the same queue.
//
// On the file count: this package spans many domains (blame, bisect, branch,
// diff, difftree, log, merge, queue, reflog, remote, reset, revert,
// stage_patch, stash, tag, undo, url, worktree), and an automated review
// (issue #167) flagged that as a god package to be split into sub-packages.
// It is deliberately not split, for three reasons.
//
// First, Go does not permit it directly. Every domain file holds methods on
// *Client, and methods cannot be declared on a type owned by another package.
// Splitting would mean either fragmenting Client into per-domain clients,
// which destroys the single GitClient interface that the AI middleware
// implements and every panel consumes, or demoting the methods to free
// functions, which changes every call site and leaves the sub-packages
// importing git for the Client type anyway.
//
// Second, the shared state is the point. Client carries one OpQueue and one
// Cache. Serialisation and caching are correctness properties that hold
// across domains: a branch operation and a stash operation must not race.
// Per-domain packages would each need a reference back to that shared state,
// so the coupling would move rather than disappear.
//
// Third, the cost is not being paid. The package is roughly 4,000 lines
// across 30 files, averaging under 140 lines each, already separated by
// domain and named for it. Navigation happens by file, and the file layout is
// exactly the split the review asked for.
//
// The signal worth watching is file size rather than file count. If a single
// domain file outgrows ~500 lines, split that file. If a domain grows genuine
// state of its own, independent of the queue and cache, that is the point to
// reconsider a sub-package for it.
package git
