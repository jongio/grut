# Wire Shortcuts Engine into TUI

## Summary

The `internal/shortcuts/` package has a fully-functional multi-step git workflow engine with 24 built-in shortcuts, but it is completely disconnected from the TUI — users cannot discover or execute shortcuts from the app.

## Description

Grut includes a powerful shortcuts engine (`internal/shortcuts/`) that defines 24 built-in multi-step git workflows like `sync` (fetch all + rebase upstream/main), `rb` (fetch + rebase onto remote/branch), `up` (fetch all + pull --rebase), `sc` (stage all + AI commit), and many more. The engine supports:

- Multi-step sequential execution with per-step failure policies (`stop`, `continue`, `ask`)
- User-supplied arguments with defaults and placeholders (`{{remote}}`, `{{branch}}`)
- Custom shortcut registration that overrides built-ins
- Dry-run planning via `Plan()` method
- AI-assisted steps (e.g., AI commit messages)

However, none of this is accessible from the TUI. The command palette (`:` key) only surfaces keymap bindings via `CommandSource`, not shortcuts. The `shortcuts.Engine` is never instantiated in the TUI model. Users wanting to run common multi-step git workflows (like fetch + rebase) must do each step manually.

## Technical Details

### Existing Infrastructure (Ready to Use)

- **Shortcuts engine**: `internal/shortcuts/engine.go` — `NewEngine()`, `Resolve()`, `List()`, `Plan()`, `Execute()`
- **Built-in shortcuts**: `internal/shortcuts/builtins.go` — 24 shortcuts covering stage+commit, push, pull, rebase, branch management, stash, and more
- **Command palette**: `internal/panels/fuzzyfinder/` — fuzzy finder with `CommandSource` for keybindings, emits `CommandSelectedMsg`
- **Async op pattern**: `internal/tui/gitops.go` — established pattern for async git operations with status bar indicator, ESC cancellation, toast notifications
- **Modal input system**: `internal/notify/modal.go` — supports confirmation, text input, and action picker modals
- **Conflict resolution**: `internal/panels/conflicts/` + `internal/panels/aiconflict/` — existing panels for merge conflict handling

### What Needs to Be Built

1. **ShortcutSource** (`internal/panels/fuzzyfinder/source.go`): New `Source` implementation wrapping `shortcuts.Engine.List()` to surface shortcuts in the command palette alongside keybindings
2. **ShortcutSelectedMsg** (`internal/panels/messages.go`): New message type for shortcut selection, distinct from `CommandSelectedMsg`
3. **Engine integration** (`internal/tui/app.go`): Add `shortcuts.Engine` to the `Model` struct, initialize when `gitClient` is available, handle `ShortcutSelectedMsg` in `Update()`
4. **Execution flow** (`internal/tui/shortcutops.go`): New file implementing async shortcut execution with progress labels, toast results, git status refresh, and conflict detection on rebase steps
5. **Arg collection**: Sequential modal prompts for shortcuts requiring user input (e.g., branch name for `rb`, `nb`)

### Key Shortcuts Users Will Gain Access To

| Shortcut | Steps | Description |
|----------|-------|-------------|
| `sync` | fetch --all + rebase upstream/main | Sync with upstream |
| `rb` | fetch + rebase onto remote/branch | Rebase onto any branch |
| `up` | fetch --all + pull --rebase | Quick update current branch |
| `sc` | stage all + AI commit | Smart commit |
| `scp` | stage all + AI commit + push | Commit and push |
| `fresh` | stash + checkout main + pull + checkout back + rebase + stash pop | Full refresh cycle |
| `ship` | checkout target + merge + push + delete branch | Ship a feature |
| `nb` | fetch + branch create + checkout | New branch from latest |

## Acceptance Criteria

- [ ] Shortcuts appear in the command palette (`:` key) alongside keybindings
- [ ] Users can fuzzy-search shortcuts by name and description
- [ ] Selecting a shortcut with no args and `Confirm: false` executes immediately
- [ ] Selecting a shortcut with `Confirm: true` shows a confirmation modal first
- [ ] Selecting a shortcut with `Args` prompts for each arg via modal input
- [ ] Async execution shows progress in the status bar (e.g., "running sync...")
- [ ] Toast notifications show success or failure on completion
- [ ] Git status refreshes after shortcut execution
- [ ] Rebase conflicts transition to the conflicts panel
- [ ] All existing tests continue to pass
- [ ] New tests cover ShortcutSource, selection flow, and execution

## Related

- `internal/shortcuts/` — existing engine and built-in definitions
- `internal/panels/fuzzyfinder/source.go` — existing `CommandSource` pattern to follow
- `internal/tui/gitops.go` — existing async operation pattern to follow
- `internal/tui/app.go` — TUI model where integration happens
- `internal/panels/messages.go` — cross-panel message types
