# Improve First Start Experience with Welcome Screen

## Summary

Replace the current first-run help overlay with a polished welcome screen that shows the grut logo, highlights essential keyboard shortcuts (especially `g` for git-changed file filtering), and provides OK / Don't Show Again controls.

## Description

The current first-run experience triggers `FirstRunMsg` which calls `toggleHelp()` to show a generic keybinding overlay. While functional, this misses the opportunity to give new users a focused, guided introduction to grut's most important features.

A dedicated welcome screen should:

- Feel like a proper onboarding moment — not just a wall of keybindings
- Show the grut logo/banner as ASCII art to reinforce brand identity
- Highlight the 3–5 most essential shortcuts a new user needs to know
- Strongly emphasize `g` (toggle git-changed files filter) since it's grut's signature workflow
- Provide clear dismiss controls: OK (dismiss), Don't Show Again (dismiss + never show)
- Respect the existing `show_first_run_help` config flag and `.first-run-done` marker file

## Technical Details

### Existing Infrastructure (reuse these)

| Component | Location | Purpose |
|-----------|----------|---------|
| First-run detection | `internal/session/firstrun.go` | `IsFirstRun()` checks marker file at `~/.local/share/grut/.first-run-done` |
| Config flag | `internal/config/config.go:49` | `GeneralConfig.ShowFirstRunHelp` (default `true` in `defaults.toml`) |
| First-run trigger | `internal/tui/app.go:153-157` | Sends `FirstRunMsg` on init when first run detected |
| Message type | `internal/panels/messages.go` | `FirstRunMsg{}` struct |
| Modal system | `internal/notify/modal.go` | Full modal with `ModalConfirmWithCheckbox` kind |
| Modal messages | `internal/notify/messages.go` | `ShowModalMsg` (with `CheckboxLabel`) and `ModalResultMsg` (with `Remember` bool) |
| Help overlay | `internal/panels/help/help.go` | Current first-run target — scrollable keybinding list |

### Proposed Approach

**Option A: Dedicated Welcome Panel (Recommended)**

Create a new `internal/panels/welcome/welcome.go` panel that renders as a centered overlay (similar to the help panel pattern). This gives full control over layout, content, and styling.

- Render ASCII art logo at top using lipgloss styling
- Curated content sections below (not auto-generated from keymap)
- Footer with `[Enter] OK` and `[d] Don't Show Again` controls
- On dismiss: call `session.MarkFirstRunDone()` and optionally set `show_first_run_help = false`

**Option B: Enhanced Modal**

Use the existing `ModalConfirmWithCheckbox` kind from `internal/notify/modal.go`. Less flexible for layout but reuses existing infrastructure directly.

### Welcome Screen Content (suggested)

```
╭─────────────────────────────────────────────╮
│                                             │
│              ┏━━━┓                           │
│              grut                            │
│         Git Review Utility for TUI          │
│                                             │
├─────────────────────────────────────────────┤
│                                             │
│  Welcome! Here are the essentials:          │
│                                             │
│  Navigation                                 │
│    Tab / Shift+Tab   Switch panels          │
│    ↑/↓ or j/k        Navigate items        │
│    Enter              Open / expand         │
│    ?                  Full help             │
│                                             │
│  ★ Key Feature                              │
│    g   Show only git-changed files          │
│        in the file explorer — your          │
│        fastest path to what matters         │
│                                             │
│  Git                                        │
│    s          Stage / unstage file          │
│    c          Commit                        │
│    Space      Toggle diff preview           │
│                                             │
├─────────────────────────────────────────────┤
│  [Enter] OK    [d] Don't Show Again         │
╰─────────────────────────────────────────────╯
```

### Key Behaviors

- **Enter / OK**: Dismiss welcome screen, mark first run done (marker file). Screen will not appear again because `.first-run-done` exists.
- **d / Don't Show Again**: Dismiss + set `show_first_run_help = false` in user config. This persists even if marker file is deleted.
- **Esc**: Same as OK (dismiss).
- Welcome screen captures all keyboard input while visible (modal behavior).
- Must respect terminal size — scale/truncate gracefully on small terminals.

### ASCII Logo

No ASCII art logo currently exists. One needs to be created. Options:
- Hand-crafted minimal ASCII using box-drawing characters
- Generate from existing `assets/logo.svg` using a converter
- Simple styled text using lipgloss (bold, colored "grut" text)

### Files to Create/Modify

| Action | File | Change |
|--------|------|--------|
| Create | `internal/panels/welcome/welcome.go` | New welcome panel with logo, content, controls |
| Modify | `internal/tui/app.go` | Handle `FirstRunMsg` → show welcome panel instead of help overlay |
| Modify | `internal/tui/app.go` | Handle welcome dismiss result → `MarkFirstRunDone()` + optional config update |
| Possibly modify | `internal/config/config.go` | If config write-back for "Don't Show Again" needs new helper |

## Acceptance Criteria

- [ ] First launch shows a centered welcome screen overlay (not the generic help panel)
- [ ] Welcome screen displays grut logo/banner (ASCII art or styled text)
- [ ] Welcome screen highlights essential keyboard shortcuts in a curated, readable layout
- [ ] The `g` shortcut (git-changed files filter) is prominently featured/emphasized
- [ ] Enter or Esc dismisses the welcome screen (marks first run done)
- [ ] "Don't Show Again" option persists the choice via config so it never appears even on fresh installs
- [ ] Existing `show_first_run_help` config flag and `.first-run-done` marker are respected
- [ ] Welcome screen is modal — captures keyboard input while visible
- [ ] Graceful rendering on small terminals (minimum ~60x20)
- [ ] No emoji — only clean Unicode symbols per project style guidelines
- [ ] All existing tests continue to pass

## Related

- `internal/session/firstrun.go` — first-run detection logic
- `internal/notify/modal.go` — existing modal system
- `internal/panels/help/help.go` — current first-run target (help overlay)
- `internal/config/defaults.toml` — `show_first_run_help = true` default
- `assets/logo.svg` — existing logo asset (graphic, not ASCII)
