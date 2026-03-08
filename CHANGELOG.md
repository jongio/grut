# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Right-click context menu with action picker for all panels
- Configurable click actions with mouse support for all panels
- Settings dialog overlay for preview position (comma key)
- Mouse drag resize on split borders
- Vertical resize keybindings (ctrl+up/ctrl+down)
- Configurable preview position cycling (ctrl+b p)
- Tab bar with canonical 1-5 preset ordering
- Mage build system with preflight checks (`mage install`, `mage preflight`)

### Changed
- Git preset simplified to 2-pane layout (filetree + preview)
- Panel borders use NormalBorder (square corners, no title)

### Fixed
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
- Add `go mod verify` integrity check to preflight build gate (now 11 steps)

### Performance
- Batch bottom border rendering calls

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
