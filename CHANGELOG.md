# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] - 2026-05-18

### Contributors

Thanks to the following people for their contributions to this release:

- **Copilot**
- **Jon Gallant**

New contributors: **Copilot** — welcome!

### Added
- Full inline editor with standard actions: copy (Ctrl+C), cut (Ctrl+X), paste (Ctrl+V), select all (Ctrl+A), undo (Ctrl+Z), redo (Ctrl+Y)
- Mouse support in edit mode: click-to-position cursor, drag-select, double-click for word selection
- Async clipboard operations via Bubble Tea Cmd pattern (non-blocking UI)
- Bracketed paste support (tea.PasteMsg) with automatic CRLF normalization
- Word navigation (Ctrl+Left/Right), line operations (Home/End, Ctrl+Home/End)
- 21 documented edit mode keybindings in help overlay and docs/keybindings.md
- Preview dual-mode diff toggle - press `f` in the preview panel to switch between file-on-disk and contextual diff view. The diff shown adapts to your context: commit diff when browsing commit files, branch comparison diff in branch mode, PR diff for pull requests, and working tree diff with git filter active. Title shows `[diff]` suffix when in diff mode.
- Edit blocked in diff mode - press `f` to return to file view before editing with `e`
- Inline editor mode — press `e` in the preview panel to edit files directly, `Ctrl+S` to save, `Escape` to discard
- Branch diff filter — `g` now cycles through three modes: all files → git changed → branch diff (files changed compared to default branch)
- Lazy-loading pagination for GitHub tabs — issues, PRs, Actions, and releases load on demand as you scroll
- `Ctrl+C` copies selected text in preview instead of quitting (when a selection exists)
- Text selection in preview pane — click+drag to select, double-click for word selection, `y` to copy to clipboard, Escape to clear
- Worktree double-click opens the selected worktree directory
- Checkout with dirty-tree detection — automatically stashes, switches branch, and re-applies
- Automated contributor recognition in changelogs, GitHub Releases, and CONTRIBUTORS.md hall of fame
- Welcome screen overlay with animated grüt banner and first-run keyboard reference
- `--reset-welcome` flag to re-show the welcome screen on next launch
- Right-click context menu with action picker for all panels
- Configurable click actions with mouse support for all panels
- Settings dialog overlay for preview position (comma key)
- Mouse drag resize on split borders
- Vertical resize keybindings (ctrl+up/ctrl+down)
- Configurable preview position cycling (ctrl+b p)
- Tab bar with canonical 1-5 preset ordering
- Mage build system with preflight checks (`mage install`, `mage preflight`)

### Changed
- Go toolchain upgraded from 1.26.1 to 1.26.3 (7 stdlib CVE fixes)
- Bubble Tea upgraded to v2.0.6, Lipgloss to v2.0.3
- `x/net` upgraded to v0.54.0, `x/sys` and `x/text` updated
- Git preset simplified to 2-pane layout (filetree + preview)
- Panel borders use NormalBorder (square corners, no title)
- Preflight gate expanded from 11 to 14 checks (added race, WSL, gofumpt)
- Extracted 4 shared panel utilities (`EnsureCursorVisible`, `ClampCursor`, `ColorOf`, `OrDefault`) eliminating 27 duplicated functions

### Fixed
- Branch name in status bar no longer reverts to previous branch after checkout (stale async response race)
- Double-click first-use flow survives async modal delay by storing pending path
- Git test suite isolated from user/system config to prevent hangs
- Eliminate Windows clipboard command injection (CWE-78)
- Input validation for OpenInEditor and OpenInBrowser
- Modal mouse click and Tab cycling support
- Bottom border rendering and clean footer layout
- Preview panel content wrapping with truecolor ANSI codes
- Folders no longer auto-expand on startup
- Wire 18 broken keybindings across file tree, branches, stash, gitinfo, and global handlers
  - File tree: `n` (create), `e`/`F2` (rename), `o` (open in editor), `y` (copy path)
  - Branches: `n` (create), `d` (delete), `e`/`F2` (rename), `o` (open in browser), `y` (copy name)
  - Stash: `n` (push new, alternate for `s`), `y` (copy stash reference)
  - Git info: `n` (create), `d` (delete), `e`/`F2` (rename), `o` (open in browser), `y` (copy)
  - Global: `focus_left`/`focus_right` (vim scheme), `git_remotes` tab switch
- Branches panel `o` key now constructs proper URL from remote instead of passing raw branch name
- Test no longer opens real browser to example.com during `go test`

### Security
- Block read/write access to `.git/` internals, `.env`, `*.pem`, `*.key` via MCP and AI chat file tools
- Reject credential-embedded URLs (`user:pass@host`) in browser URL validation
- Block Windows UNC paths and reserved device names (CON, NUL, etc.) in MCP PathJail
- Validate file paths before `git add` in filetree staging operations
- Filter secret environment variables from GitHub CLI subprocess execution
- Redact credential-embedded git remote URLs (`https://token@host`) in AI context
- Cap diff line length at 10,000 characters to prevent context bloat from binary files
- Add `go mod verify` integrity check to preflight build gate
- Propagate `Close` errors in MCP file tools to prevent silent data corruption
- Add 22 security tests covering git message validation, MCP tool injection, path traversal, and extension install safety

### Performance
- Cache syntax highlight lexer/style/formatter on Preview struct — eliminates 160+ lookups per keystroke in edit mode
- Bounded LRU caches with size limits to prevent unbounded memory growth
- Fix goroutine leak in filesystem watcher cleanup
- Auto-cleanup exited AI agents from AgentTracker after 30-second grace period
- Optimize gitstatus render path with dirty flag, style cache, and `strings.Builder`
- Cache markdown renderer instance across frames instead of re-creating per render
- Cap inline notification count to prevent render stalls
- `--pprof PORT` flag for live profiling via `net/http/pprof`
- Struct field reordering via `betteralign` across 70+ types, eliminating 800+ bytes of padding
  (`config.Config` −56 B, `git/types` −200 B, `chat.Model` −24 B, `mcp/agent_tracker` −80 B)
- `gitdiff` slice reuse: `[:0]` reset on `rebuildLines()` instead of nil — backing arrays
  persist across frames; benchmarks show **−39% sec/op** (side-by-side, 1000 lines),
  **−16% B/op** (inline, 1000 lines), **−20% sec/op** (PairDiffLines)
- `tui/app.go` render loops: replace `+=` string concat with `strings.Builder` + `Reset()`
  in `renderPanel()` and `buildOuterBorder()` — ~196 fewer allocations per frame (~11,700/sec at 60 fps)
- Added `--cpu-profile` and `--mem-profile` CLI flags for on-demand `runtime/pprof` profiling
- Benchmark baseline committed to `perf/baselines/main.txt`; CI tracks regressions on every PR

## [0.1.0] - 2026-03-09

### Added
- Terminal file explorer with Bubble Tea v2
- Git integration (status, diff, log, branches, stash, conflicts)
- 5 preset layouts (Explorer, Git, Review, Agent, Full)
- Syntax highlighting with Chroma
- Nerd Font icon support
- Mouse click navigation
- Fuzzy finder for files and commands
- Bookmarks panel
- Help overlay
- Split panel management (horizontal/vertical/close/zoom)
- Tab management with presets
- TOML-based keybinding configuration
- Theme system with Default, Catppuccin, Tokyo Night, and Gruvbox palettes
