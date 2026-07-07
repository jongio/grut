# Configuration Reference

grut uses TOML configuration with sensible defaults. User overrides are loaded from:

- **Linux/macOS:** `~/.config/grut/config.toml`
- **Windows:** `%APPDATA%\grut\config.toml`

All paths support tilde (`~`) expansion. Only set values you want to change — everything has a built-in default.

---

## General

```toml
[general]
keybinding_scheme = "default"    # "default", "vim", "classic", or path to .toml file
default_layout = "explorer"      # "explorer", "git", "review", "agent"
auto_save_session = true         # Save/restore tab layout on exit/start
show_first_run_help = true       # Show help overlay on first launch
```

---

## File Tree

```toml
[file_tree]
show_hidden = true               # Show hidden files and directories
show_icons = true                # Show file/directory icons
icon_mode = "auto"               # "nerd" (nerd fonts), "ascii" (fallback), "auto" (detect)
sort_directories_first = true    # Directories listed before files
git_status_markers = true        # Show git status indicators (M, A, ?)
follow_symlinks = false          # Follow symbolic links when traversing
max_depth = 20                   # Maximum directory depth (≥ 1)
```

---

## Preview

```toml
[preview]
enabled = true                   # Enable file preview pane
width = 40                       # Preview width as percentage (1–100)
syntax_highlighting = true       # Enable syntax highlighting
max_file_size = 1048576          # Max file size to preview in bytes (default: 1 MB)
line_numbers = true              # Show line numbers
word_wrap = false                # Enable word wrapping
```

---

## Git

```toml
[git]
refresh_method = "fsnotify"      # "fsnotify" (event-based) or "poll" (interval-based)
refresh_fallback_interval = "2s" # Polling interval when fsnotify unavailable
default_branch = "main"          # Default branch name for new repos
worktree_first = true            # Prioritize worktrees over branches in listings
worktree_merge_method = "merge"  # "merge", "rebase", or "squash"
auto_fetch_interval = "5m"       # Automatic fetch interval
show_commit_graph = true         # Display commit graph in git log
max_log_entries = 500            # Maximum git log entries to display (≥ 1)
sign_commits = false             # Sign commits with GPG
diff_word_highlight = true       # Highlight changed words within lines in the diff view (toggle with w)
```

---

## Terminal

```toml
[terminal]
shell = ""                       # Shell command (empty = auto-detect)
scrollback = 10000               # Scrollback buffer lines (≥ 1)
render_fps = 30                  # Terminal render FPS (≥ 1)
prefix_key = "ctrl+b"            # Prefix key for terminal commands
```

**Auto-detection:** When `shell` is empty, grut uses `$SHELL` on Unix (fallback `/bin/sh`) and `cmd.exe` on Windows.

---

## AI

```toml
[ai]
auto_install_deps = false        # Auto-install dependencies for AI features
context_mode = "manual"          # "manual" or "smart" context collection
token_model = "gpt-4"            # Token counting model
```

---

## Theme

```toml
[theme]
name = "default"                 # "default", "catppuccin", "tokyonight", "gruvbox", or path to .toml
```

Custom themes are loaded from `~/.config/grut/themes/`.

---

## Session

```toml
[session]
enabled = true                   # Enable session persistence
max_age = 30                     # Session expiry in days
```

Sessions are stored in `~/.local/share/grut/sessions/` and restore tab layout, focused panel, and active tab on restart.

---

## Bookmarks

```toml
[bookmarks]
paths = []                       # Initial bookmark paths
show_in_sidebar = false          # Show bookmarks in sidebar
```

Bookmarks are persisted separately in `~/.config/grut/bookmarks.toml` and can also be managed interactively with the `b` key.

---

## Logging

```toml
[logging]
level = "warn"                   # "debug", "info", "warn", "error"
file = ""                        # Log file path (empty = no file logging)
max_size_mb = 10                 # Log file rotation size in MB (≥ 0)
max_backups = 3                  # Max backup log files (≥ 0)
```

---

## Example: Full Config

```toml
[general]
keybinding_scheme = "vim"
default_layout = "explorer"
auto_save_session = true

[file_tree]
show_hidden = true
icon_mode = "nerd"

[preview]
max_file_size = 5242880  # 5 MB

[git]
sign_commits = true
auto_fetch_interval = "10m"

[terminal]
shell = "/bin/zsh"
scrollback = 50000

[theme]
name = "catppuccin"

[bookmarks]
paths = ["~/projects", "~/notes"]

[logging]
level = "info"
file = "~/.local/share/grut/grut.log"
```
