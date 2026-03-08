# Copilot Instructions for grut

## Build After Every Change (MANDATORY)

After every code change (file edit, create, or delete), run a full global build to verify nothing is broken:

```
go build ./...
```

This is non-negotiable. Do not batch multiple changes and then build once at the end. Build after EACH change to catch errors immediately.

If the build fails, fix the issue before making any further changes.

## Test Verification

After all changes for a task are complete and building, run the full test suite:

```
go test ./...
```

All previously-passing packages must continue to pass.

## Deploy After Task Completion (MANDATORY)

After all changes for a task are complete, tests pass, and you've committed, run the install command:

```
mage install
```

This runs tests, kills running grut processes, builds the dev binary with version info, ensures it's in PATH, and verifies the deployment. Run this as the final step before reporting task completion.

## Project Context

- Language: Go
- Framework: Bubble Tea v2 (terminal UI)
- Styling: lipgloss
- Build: `go build ./...`
- Test: `go test ./...`
- Lint: `go vet ./...`

## Icon & Visual Style Guidelines (MANDATORY)

This is a professional, high-end terminal application. All visual indicators must look clean and polished.

### Allowed: Clean Unicode Symbols

Use simple typographic and geometric Unicode characters for status indicators, markers, and UI chrome:

- **Status**: `✓` (success), `✗` (failure/error), `⚠` (warning), `ℹ` (info), `•` (neutral)
- **Selection/cursor**: `▸` (pointer), `●` (active/selected), `○` (inactive/unselected), `◐` (partial/mixed)
- **Expand/collapse**: `▸` (collapsed), `▾` (expanded), `▼` (expanded alt)
- **Box drawing**: `─` `│` `┌` `┐` `└` `┘` `├` `┤` `┬` `┴` `╭` `╮` `╯` `╰` and related characters
- **Nerd Font file icons**: Only behind `icon_mode=nerd` config gate (see `internal/panels/filetree/icons.go`)

### Forbidden: Emoji

Never use emoji characters (colored pictographic Unicode) in the application UI. This includes but is not limited to:

- Colored circles: `🟢` `🔴` `🟡` `🔵` `⚪`
- Pictographic status: `✅` `❌` `⬜` `🔶`
- Objects: `📦` `🔌` `🌙` `🔥` `📝` `✏️` `🚀` `💡` `🗑️` etc.
- Faces/hands: `👍` `👎` `🎉` etc.

Emoji render inconsistently across terminals, break column alignment, and look cheap in a professional tool.

### How to Apply Color

Color comes from **lipgloss styling**, not from the icon character itself. For example:

```go
// Correct: plain Unicode + lipgloss color
statusIcon = "✓"
style := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B"))
rendered := style.Render(statusIcon)

// Wrong: emoji with baked-in color
statusIcon = "✅"
```

### Reference: Icon Vocabulary

| Symbol | Meaning | Used In |
|--------|---------|---------|
| `✓` | Success, enabled, resolved, approved | notifications, conflicts, extensions, review, CLI |
| `✗` | Error, disabled, failed, rejected | notifications, conflicts, extensions, review, CLI |
| `⚠` | Warning | notifications, diff annotations |
| `ℹ` | Info | notifications |
| `•` | Default/neutral | notifications |
| `●` | Selected, active, running | settings, gitstatus, aiconflict, agents |
| `○` | Unselected, pending | review |
| `◐` | Partial/mixed state | review |
| `▸` | Cursor, collapsed, pointer | filetree, settings, review, app tabs |
| `▾` | Expanded | filetree |
| `▼` | Expanded (alt) | gitstatus |
| `λ` | Lua runtime | extensions |
| `◈` | Wasm runtime | extensions |
