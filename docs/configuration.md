# Configuration Reference

grut uses TOML configuration with sensible defaults. User overrides are loaded from:

- **Linux/macOS:** `~/.config/grut/config.toml`
- **Windows:** `%APPDATA%\grut\config.toml`

All paths support tilde (`~`) expansion. Only set values you want to change — everything has a built-in default.

To print the exact paths grut resolves at runtime, run:

```sh
grut config path
```

Example output:

```text
Config: C:\Users\me\AppData\Roaming\grut\config.toml
Data:   C:\Users\me\AppData\Local\grut
```

Run `grut doctor` to validate the active config, selected theme, terminal compatibility, Git/GitHub auth, AI provider readiness, and config/data directory writability. Use `grut doctor --json` to attach a stable machine-readable report to bug reports or CI logs.

---

## General

```toml
[general]
keybinding_scheme = "default"    # "default", "vim", "classic", or path to .toml file
default_layout = "explorer"      # "explorer", "git", "review", "agent", "full"
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
permanent_delete = false        # Permanently delete instead of moving file-tree deletes to trash
max_depth = 20                   # Maximum directory depth (≥ 1)
```

File-tree deletes move files and directories to a per-repository grut trash under the user data directory by default. Press `u` in the file tree to restore the most recent trashed item; set `permanent_delete = true` to keep the old permanent-delete behavior.

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
secret_guard = true              # Scan files for secrets before staging
secret_guard_mode = "warn"       # "warn" (confirm) or "block" (refuse)
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

## Custom Actions

Custom actions are user-defined shell commands shown in the command palette. Set `confirm = true` for destructive actions; grut asks before running them. If `key` is set and does not collide with an existing binding, pressing it runs the same action.

```toml
[[custom_actions]]
name = "Test"
command = "go test ./..."
cwd = "."
key = "ctrl+t"
prompt = "Run the full test suite?"
confirm = true
```

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
color_mode = "auto"              # "auto" honors NO_COLOR, "color" forces color, "mono" forces markers-only color-safe mode
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

## MCP Server Security

`grut mcp` runs an MCP (Model Context Protocol) server that exposes git and file
operations as tools for AI agents. Every file operation is confined to the
repository root (a "path jail" that rejects `..` traversal, UNC paths, and — by
default — symlinks). These settings tighten what the server allows.

```toml
[mcp.security]
allowed_commands = []         # Tool allowlist; empty = all tools allowed
require_confirmation = true   # Write tools require an explicit "_confirmed": true argument
allowed_write_paths = []      # Restrict file_write to these repo paths; empty = anywhere in the jail
rate_limit_read = 1000        # Max read tool calls per minute
rate_limit_write = 100        # Max write tool calls per minute
follow_symlinks = false       # When false, reject paths that traverse a symlink
audit_log = true              # Record tool invocations to an audit log
audit_log_path = ""           # Audit log file (empty = <data dir>/mcp-audit.log)
max_agent_processes = 5       # Max concurrent agent processes
agent_timeout = 1800          # Agent process timeout in seconds
```

`allowed_write_paths` entries are interpreted relative to the repository root
(absolute paths are also accepted) and may not contain `..` or UNC prefixes.
When the list is non-empty, `file_write` is refused for any path that resolves
outside every listed location — this narrowing applies on top of the always-on
repository jail. An empty list (the default) leaves the jail as the only
boundary.

> **Note:** `socket_auth` is reserved and not yet enforced ([#174](https://github.com/jongio/grut/issues/174)); setting it currently has no effect.

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

---

## Environment Variables

A few settings are controlled by environment variables rather than the config file:

| Variable | Effect |
|---|---|
| `GRUT_LOG` | Path to a log file. Setting it enables structured debug logging for the session. |
| `GRUT_NERD_FONT` | Set to `1` to force Nerd Font icons in any terminal, regardless of `file_tree.icon_mode`. |
| `GRUT_FORCE_TERMINAL` | Set to any non-empty value (for example `1`) to bypass the MinTTY/MSYS compatibility check on Windows. |
