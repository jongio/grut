# Intelligent contextual keyboard shortcut hints

## Summary

Replace the hardcoded hints bar with a dynamic, state-aware system where panels declare which shortcuts are relevant right now, with priority scoring to surface the most useful actions first.

## Description

grut has 20+ panels, each with 5-20 keyboard shortcuts, across 3 key modes (global/panel/input) and multiple internal states. The current hints bar (`renderHintsBar()` in `internal/tui/app.go`) is hardcoded with static strings per panel name in a `switch` statement. It does not adapt to internal panel state -- for example, the gitstatus panel shows the same hints whether you are in file mode, hunk mode, or line mode. It does not highlight actions that are most relevant given the current state (e.g., promoting "commit" when staged files exist).

This makes it hard for users to discover and remember which shortcuts matter at any given moment, especially across panels with deep state like gitstatus (file/hunk/line modes), filetree (dir vs file cursor, multi-select, commit-files mode), and preview (normal vs edit mode).

## Technical Details

### Current architecture

- `internal/panels/panel.go` defines `Panel` interface with `KeyBindings() []KeyBinding` -- every panel already returns its full binding list
- `internal/tui/app.go:renderHintsBar()` (lines 2074-2110) uses a hardcoded `switch focusedName` with static hint strings per panel
- `internal/tui/app.go:renderHintLine()` (lines 2112-2143) handles styling and width-filling
- `internal/keymap/keymap.go` defines `Binding` struct with Mode, Context, Key, Action, Description
- Keymap scheme files (`.toml`) in `internal/keymap/schemes/` define bindings with context names: `filetree`, `gitstatus`, `gitinfo`, `github`, `branches`, `stash`, `worktrees`

### Proposed design

1. **New types in `internal/panels/hints.go`**:
   - `HintPriority` type with `HintNormal` and `HintPromoted` constants
   - `Hint` struct: `Key string`, `Description string`, `Priority HintPriority`
   - `HintProvider` optional interface: `ContextualHints() []Hint`
   - `KeyBindingsToHints([]KeyBinding) []Hint` conversion helper for fallback

2. **Refactored `renderHintsBar()`**:
   - Query focused panel via `HintProvider` type assertion
   - Fall back to `KeyBindings()` auto-conversion for panels without `HintProvider`
   - Sort promoted hints first
   - Width-aware truncation with `?:+N more` overflow indicator
   - Global hints (chat, help, tabs) always appended

3. **State-aware panel implementations** (priority panels):
   - `GitStatus`: file/hunk/line mode hints, promote commit when staged, promote push when ahead
   - `FileTree`: dir vs file actions, multi-select actions, commit-files mode
   - `Preview`: normal vs edit mode, promote save when unsaved changes
   - `Branches`: current branch vs other branch actions
   - `GitLog`: commit navigation context

### Key files affected

- `internal/panels/hints.go` (new)
- `internal/tui/app.go` (refactor `renderHintsBar()`)
- `internal/panels/gitstatus/gitstatus.go` (add `ContextualHints()`)
- `internal/panels/filetree/filetree.go` (add `ContextualHints()`)
- `internal/panels/preview/preview.go` (add `ContextualHints()`)
- `internal/panels/branches/branches.go` (add `ContextualHints()`)
- `internal/panels/gitlog/gitlog.go` (add `ContextualHints()`)
- Additional panels for basic implementations

## Acceptance Criteria

- [ ] Hints bar content changes when switching between panels (Tab key)
- [ ] Hints bar content changes when panel internal state changes (e.g., entering hunk mode in gitstatus)
- [ ] Promoted hints appear first in the bar with visually distinct styling (brighter/bolder)
- [ ] When terminal is narrow (< 80 cols), overflow shows `?:+N more` with correct count
- [ ] When terminal is wide (> 160 cols), all or most hints fit without overflow
- [ ] Global hints (help, chat) always appear regardless of focused panel
- [ ] GitStatus: `c:commit` promoted when staged files exist, hidden when none staged
- [ ] GitStatus: hints switch for file/hunk/line modes
- [ ] FileTree: cursor on directory shows dir-specific actions, cursor on file shows file actions
- [ ] FileTree: with files selected, bulk actions (delete, copy) promoted
- [ ] Preview: normal mode shows scroll/edit hints, edit mode shows save/cancel hints
- [ ] Panels without `HintProvider` fall back to `KeyBindings()` conversion with no regressions
- [ ] All existing tests pass, new tests cover hint types and state-aware logic

## Quality Gates (MANDATORY)

- [ ] Zero lint/type errors (`go vet ./...`, `go build ./...`)
- [ ] All tests pass (existing + new tests for this feature)
- [ ] mq (max-quality) passes clean
- [ ] test-health grade >= B+
- [ ] idiomatic audit clean
- [ ] Architecture review: no violations
- [ ] Gut-check: no over-engineering or YAGNI violations
- [ ] `mage preflight` passes

## Done Definition

This issue is DONE when ALL acceptance criteria AND quality gates pass.
"It compiles" is not done. "Tests pass" is not done. ALL gates must be green.

## Related

- `internal/panels/panel.go` - Panel interface with KeyBindings()
- `internal/panels/panel.go` - `SelectionCopier` and `Closer` optional interfaces (pattern to follow)
- `internal/tui/app.go` - `renderHintsBar()`, `renderHintLine()`
- `internal/keymap/keymap.go` - Keymap dispatch system
- `internal/panels/help/help.go` - Full help overlay (unchanged, complementary)
