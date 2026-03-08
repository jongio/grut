# Extension Development Guide

grut supports two extension runtimes: **Lua** and **WASM**. Extensions are sandboxed with a permission system that controls access to files, git, network, and UI.

## Quick Start

```bash
# Create a new extension from a template
grut ext create my-extension --template lua

# Available templates: lua, wasm-go
```

This scaffolds an extension directory with a manifest and entry point file.

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `grut ext create <name> --template <type>` | Scaffold a new extension |
| `grut ext install <url-or-path>` | Install from HTTPS URL or local path |
| `grut ext list` | List installed extensions |
| `grut ext info <name>` | Show extension details |
| `grut ext enable <name>` | Enable a disabled extension |
| `grut ext disable <name>` | Disable without removing |
| `grut ext remove <name>` | Delete an extension |

---

## Manifest (`extension.toml`)

Every extension requires a manifest file at its root:

```toml
name = "my-extension"
version = "1.0.0"
description = "A short description of what it does"
author = "Your Name"
license = "MIT"
runtime = "lua"              # "lua" or "wasm"
entry_point = "main.lua"     # Path to the entry point
permissions = ["notify"]     # Required permissions
min_grut = "0.1.0"          # Minimum grut version
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Lowercase alphanumeric, hyphens, underscores (1–128 chars) |
| `version` | Yes | Semantic version (`major.minor.patch`) |
| `description` | No | Human-readable summary |
| `author` | No | Author name |
| `license` | No | License identifier (e.g., `MIT`, `Apache-2.0`) |
| `runtime` | Yes | `"lua"` or `"wasm"` |
| `entry_point` | No | Path to the main file |
| `permissions` | No | List of permission strings |
| `min_grut` | No | Minimum compatible grut version |

---

## Permissions

Extensions declare the permissions they need. Users are prompted to approve on install.

| Permission | Grants |
|------------|--------|
| `file_read` | Read files from the repository |
| `file_write` | Write and modify files |
| `git_read` | Read git status, log, branches |
| `git_write` | Stage, commit, push, and other git writes |
| `network` | Network access |
| `process` | Spawn and manage subprocesses |
| `clipboard` | Read/write system clipboard |
| `notify` | Display toast notifications |

---

## Lua Extensions

Lua extensions run in a sandboxed [GopherLua](https://github.com/yuin/gopher-lua) VM with dangerous modules (`os`, `io`, `debug`) and globals (`loadfile`, `dofile`) removed.

**Default timeout:** 100ms (configurable via `extensions.lua_timeout_ms`)

### Host API

Access the host API through the `grut` global:

#### `grut.toast(title, message, [level])`

Display a notification toast.

```lua
grut.toast("Build", "Compilation complete", "info")
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `title` | string | Notification title |
| `message` | string | Notification body |
| `level` | string | `"info"` (default), `"warn"`, `"error"` |

#### `grut.register_command(name, description, callback)`

Register a command in the command palette.

```lua
grut.register_command("hello", "Say hello", function()
    grut.toast("Hello", "Hello from my extension!", "info")
end)
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Command name (shown in palette) |
| `description` | string | Human-readable description |
| `callback` | function | Called when command is invoked |

#### `grut.set_status(key, value)`

Set a status bar item.

```lua
grut.set_status("word_count", "1,234 words")
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `key` | string | Status item identifier |
| `value` | string | Display value |

### Example

```lua
-- main.lua
grut.register_command("timestamp", "Insert current timestamp", function()
    grut.toast("Timestamp", os.date("%Y-%m-%d %H:%M:%S"), "info")
end)

grut.set_status("ext", "my-extension loaded")
```

---

## WASM Extensions

WASM extensions run in a [wazero](https://wazero.io/) sandbox with no filesystem access.

**Memory limit:** 64 MiB (configurable via `extensions.wasm_memory_limit`)

### Host Imports

WASM modules import functions from the `"grut"` module. All strings are passed as `(pointer, length)` pairs into linear memory.

#### `toast(title_ptr, title_len, msg_ptr, msg_len, level)`

Display a notification.

| Parameter | Type | Description |
|-----------|------|-------------|
| `title_ptr` | i32 | Pointer to title string |
| `title_len` | i32 | Length of title |
| `msg_ptr` | i32 | Pointer to message string |
| `msg_len` | i32 | Length of message |
| `level` | i32 | `0` = info, `1` = warn, `2` = error |

#### `set_status(key_ptr, key_len, val_ptr, val_len)`

Set a status bar item.

#### `log(msg_ptr, msg_len)`

Write a message to the host log.

### Example (Go → WASM)

```go
package main

//go:wasmimport grut toast
func toast(titlePtr, titleLen, msgPtr, msgLen, level uint32)

//go:wasmimport grut log
func hostLog(msgPtr, msgLen uint32)

func main() {
    title := "Hello"
    msg := "Extension loaded"
    toast(stringPtr(title), uint32(len(title)),
          stringPtr(msg), uint32(len(msg)), 0)
}
```

Build with:

```bash
GOOS=wasip1 GOARCH=wasm go build -o extension.wasm .
```

---

## Directory Structure

Installed extensions live in `~/.local/share/grut/extensions/`:

```
~/.local/share/grut/extensions/
├── my-lua-ext/
│   ├── extension.toml
│   └── main.lua
└── my-wasm-ext/
    ├── extension.toml
    └── extension.wasm
```

---

## Custom Themes

Themes use the same extension directory or `~/.config/grut/themes/`. A theme is a TOML file:

```toml
[meta]
name = "my-theme"

[colors]
background = "#1a1b26"
foreground = "#c0caf5"
cursor = "#c0caf5"

[colors.normal]
black   = "#15161e"
red     = "#f7768e"
green   = "#9ece6a"
yellow  = "#e0af68"
blue    = "#7aa2f7"
magenta = "#bb9af7"
cyan    = "#7dcfff"
white   = "#a9b1d6"

[colors.bright]
black   = "#414868"
red     = "#f7768e"
green   = "#9ece6a"
yellow  = "#e0af68"
blue    = "#7aa2f7"
magenta = "#bb9af7"
cyan    = "#7dcfff"
white   = "#c0caf5"

[ui]
border          = "#414868"
border_focused  = "#7aa2f7"
status_bar_bg   = "#1a1b26"
status_bar_fg   = "#c0caf5"
tab_active_bg   = "#7aa2f7"
tab_active_fg   = "#1a1b26"
tab_inactive_bg = "#24283b"
tab_inactive_fg = "#565f89"
selection_bg    = "#283457"
selection_fg    = "#c0caf5"
cursor_line     = "#24283b"

[syntax]
keyword  = "#bb9af7"
string   = "#9ece6a"
number   = "#ff9e64"
comment  = "#565f89"
function = "#7aa2f7"
type     = "#2ac3de"
operator = "#89ddff"

[diff]
added   = "#9ece6a"
removed = "#f7768e"
header  = "#7aa2f7"
hunk    = "#bb9af7"
context = "#565f89"

[git]
staged    = "#9ece6a"
unstaged  = "#e0af68"
untracked = "#565f89"
conflict  = "#f7768e"
branch    = "#7aa2f7"
tag       = "#bb9af7"

[notify]
info    = "#7aa2f7"
warn    = "#e0af68"
error   = "#f7768e"
success = "#9ece6a"

[files]
directory  = "#7aa2f7"
default    = "#c0caf5"
executable = "#9ece6a"
symlink    = "#2ac3de"
```

Set in config:

```toml
[theme]
name = "my-theme"  # Matches filename without .toml extension
```

Built-in themes: `default`, `catppuccin`, `tokyonight`, `gruvbox`.
