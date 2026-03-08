# grüt Roadmap

This roadmap outlines what's shipping and what's coming next. Priorities shift based on community feedback.

## v1.0 — First Release

A reactive terminal UI for Git and GitHub where every panel talks to every other panel.

### Reactive Panel System
- **Context-Aware Panels** — Select a branch and commits update. Select a file and commits filter to that file's history. Select a PR and three panels sync simultaneously: file tree shows changed files, commits show PR history, preview shows the summary.
- **Commit-Files Mode** — Press Enter on a commit and the file tree switches to show only that commit's changed files. Escape restores the full tree.
- **Git Filter** — Press `g` to toggle the file tree between all files and git-changed files only. Preview switches to diff-only mode automatically.

### File Explorer
- Browse, preview, create, rename, delete files with syntax highlighting
- Nerd Font icons with auto-detection
- Git status markers on every file and directory
- Fuzzy finder with `/` for instant file navigation
- Bookmarks for pinning frequently used directories

### Git
- **Status & Staging** — Stage, unstage, and discard with single keystrokes. Partial staging for individual hunks and lines.
- **Branches** — Create, checkout, rename, delete, merge with full cascade updates across all panels.
- **Worktrees** — Create and manage parallel working directories.
- **Stash, Tags, Remotes, Reflog** — Each as a dedicated tab in the Git panel.
- **Log** — Commit history with ASCII graph visualization.
- **Diff** — Inline and side-by-side modes with syntax highlighting and hunk navigation.
- **Merge & Rebase** — Full conflict resolution with AI-powered suggestions.
- **Blame, Bisect, Undo/Redo** — History investigation and recovery tools.
- **Commit Amend, Reset, Revert, Cherry-Pick** — History editing with confirmation for destructive operations.

### GitHub
- **Issues, Pull Requests, Actions, Workflows, Releases** — Each as a dedicated tab with filters, status tracking, and preview integration.
- **Workflow Dispatch** — Trigger CI/CD workflows with input parameters directly from the TUI.
- **PR Triple-Panel Sync** — Select a PR and file tree, commits, and preview all update simultaneously.

### AI Chat
- Built-in chat powered by GitHub Copilot that understands your repo.
- Read and write files, run git commands, search your codebase, interact with GitHub issues and PRs.
- AI-powered commit messages, conflict resolution, code review, branch naming, changelog generation, and PR descriptions.
- Collapsed, expanded, and full-screen overlay modes.

### Interface
- **Tabs & Splits** — Multi-tab layout with horizontal/vertical panel splits.
- **Layout Presets** — Five built-in layouts: explorer, git, review, agent, full.
- **Themes** — Default, Catppuccin, Tokyo Night, Gruvbox, plus custom TOML themes.
- **Settings Panel** — Configure double-click actions, right-click menus, and preferences from the TUI.
- **Session Persistence** — Saves and restores tab layout on restart.
- **Notifications** — Toast, inline, and modal notification system.
- **Self-Update** — `grut update` checks GitHub Releases and upgrades in-place.

### Extensions
- **Lua & WASM Runtimes** — Extend grüt with custom panel logic and commands.
- **Context Builder** — Select files and count tokens for AI context windows.
- **Embedded Terminal** — Shell with insert/normal modes.

## Coming Next

Ideas and improvements we're working on. No version numbers — these ship when they're ready.

### History Surgery
- **Commit Reword** — Edit any commit message in history via a simplified workflow.
- **Squash & Fixup** — Combine commits with a dedicated UI beyond interactive rebase.
- **Interactive Rebase UI** — Visual todo-list editing for interactive rebase.
- **Command Log** — Scrollable history of every git command executed.

### GitHub Enhancements
- **Labels & Milestones** — Manage labels and milestones from the TUI.
- **Review Requests** — Request and manage PR reviewers.
- **GitHub Notifications** — Browse and triage GitHub notifications.

### Advanced Workflows
- **Submodule Support** — Init, update, sync, and status for git submodules.
- **Bulk Operations** — Stage/unstage/discard multiple selected files at once.
- **Custom Actions** — User-defined keybindings that run shell commands.
- **Multi-Repo** — Manage multiple repositories in a single session.

### Extension Security
- **Extension Signing** — Cryptographic signatures for Lua/WASM extensions.
- **MCP Authentication** — Token-based auth for the local MCP server.
- **Permission Enforcement** — Runtime enforcement of declared extension permissions.

---

Have a feature request? [Open an issue](https://github.com/jongio/grut/issues) on GitHub.
