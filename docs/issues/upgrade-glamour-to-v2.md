# Upgrade Glamour to v2 (charm.land/glamour/v2) — Complete Charm v2 Migration

## Summary

Migrate the single remaining Charm v1 dependency — glamour v1.0.0 (`github.com/charmbracelet/glamour`) — to glamour v2 (`charm.land/glamour/v2`), completing grut's full Charm ecosystem v2 migration and eliminating the transitive v1 lipgloss indirect dependency.

## Description

Grut has already completed the heavy lifting of the Charm v2 migration. The core TUI framework is fully on v2:

| Package | Current Version | Import Path | Status |
|---------|----------------|-------------|--------|
| Bubble Tea | v2.0.2 | `charm.land/bubbletea/v2` | ✅ Done |
| Bubbles | v2.0.0 | `charm.land/bubbles/v2` | ✅ Done |
| Lip Gloss | v2.0.2 | `charm.land/lipgloss/v2` | ✅ Done |
| **Glamour** | **v1.0.0** | **`github.com/charmbracelet/glamour`** | **❌ Needs upgrade** |

The only remaining v1 dependency is glamour. This is significant because:

1. **Transitive v1 lipgloss**: glamour v1.0.0 pulls in `github.com/charmbracelet/lipgloss@v1.1.1` as an indirect dependency. Upgrading glamour to v2 eliminates this, giving grut a clean dependency tree with zero v1 Charm imports.
2. **Consistency**: All other Charm packages use the new `charm.land/*` vanity import domain. Glamour is the lone holdout.
3. **Glamour v2 benefits**: Improved text wrapping (via Lip Gloss v2 Unicode handling), OSC8 hyperlink support in compatible terminals, and better color handling.

## Technical Details

### Blast Radius

**Minimal** — only 1 source file is affected:

- `internal/markdown/render.go` (~50 lines, 3 glamour import lines)

Zero test files import glamour directly.

### Current Usage (internal/markdown/render.go)

```go
import (
    "github.com/charmbracelet/glamour"
    gansi "github.com/charmbracelet/glamour/ansi"
    gstyles "github.com/charmbracelet/glamour/styles"
)
```

The file uses:
- `gstyles.DarkStyleConfig` — accesses the pre-built dark theme `StyleConfig` struct
- `gansi.StyleConfig` — as a return type from the `Style()` function
- `s.H2.Prefix`, `s.H3.Prefix`, ..., `s.H6.Prefix` — direct field mutation to remove heading hash prefixes
- `glamour.NewTermRenderer()` — constructs the renderer
- `glamour.WithStyles()` — passes the custom `StyleConfig`
- `glamour.WithWordWrap()` — sets word wrap width

### Required Changes

**1. Import path update (mechanical)**
```go
// Before
"github.com/charmbracelet/glamour"
gansi "github.com/charmbracelet/glamour/ansi"
gstyles "github.com/charmbracelet/glamour/styles"

// After
"charm.land/glamour/v2"
gansi "charm.land/glamour/v2/ansi"
gstyles "charm.land/glamour/v2/styles"
```

**2. API verification needed**
- Confirm `gstyles.DarkStyleConfig` is still exported (glamour v2 may prefer `WithStandardStyle("dark")` but the struct may still exist for custom modification)
- Confirm `gansi.StyleConfig` struct and its `H2`–`H6` field types remain compatible
- Confirm `glamour.NewTermRenderer`, `glamour.WithStyles`, `glamour.WithWordWrap` signatures are unchanged
- If `DarkStyleConfig` is removed, adapt to use `WithStandardStyle("dark")` and find an alternative way to strip heading prefixes

**3. go.mod update**
```
go get charm.land/glamour/v2@latest
go mod tidy
```

This should:
- Add `charm.land/glamour/v2` as a direct dependency
- Remove `github.com/charmbracelet/glamour v1.0.0`
- Remove `github.com/charmbracelet/lipgloss v1.1.1` indirect dependency (since glamour was its only consumer)

### What Does NOT Need to Change

Grut's v2 migration is otherwise complete. For reference, here is the scope that is already done:

- **370 Go files** in the project, **180 test files**
- **78 files** import `charm.land/bubbletea/v2` — all migrated
- **39 files** import `charm.land/lipgloss/v2` — all migrated
- **0 files** import v1 bubbletea or v1 bubbles
- **0 files** import v1 lipgloss directly (the indirect dep is solely from glamour)
- `View()` returns `tea.View` (v2 pattern) ✅
- No `tea.KeyMsg` (v1) — uses custom keymap system via TOML ✅
- No `tea.MouseMsg` (v1) — no legacy mouse types ✅
- No `lipgloss.AdaptiveColor` ✅
- No `lipgloss.NewRenderer` ✅
- `tools.go` already references `charm.land/bubbles/v2/viewport` ✅

### Indirect Dependencies Eliminated

After glamour v2 upgrade and `go mod tidy`, these indirect v1 deps should be removed:

| Indirect Dependency | Pulled By | Will Be Removed |
|---------------------|-----------|-----------------|
| `github.com/charmbracelet/lipgloss v1.1.1` | glamour v1.0.0 | ✅ Yes |

## Migration Strategy

This is a single-phase, single-commit migration due to minimal blast radius.

| Phase | Scope | Risk |
|-------|-------|------|
| 1 | `go get charm.land/glamour/v2@latest` | LOW |
| 2 | Update 3 import paths in `internal/markdown/render.go` | LOW |
| 3 | Verify/adapt API usage (DarkStyleConfig, StyleConfig fields) | LOW-MEDIUM |
| 4 | `go mod tidy` to clean indirect deps | LOW |
| 5 | Build + test + vet validation | -- |

## Acceptance Criteria

- [ ] `github.com/charmbracelet/glamour` removed from go.mod (no v1 glamour)
- [ ] `github.com/charmbracelet/lipgloss v1.1.1` removed from go.mod (no v1 lipgloss indirect)
- [ ] `charm.land/glamour/v2` added as direct dependency
- [ ] `internal/markdown/render.go` uses `charm.land/glamour/v2` imports
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes (all 180+ test files)
- [ ] `go vet ./...` clean
- [ ] Markdown rendering in preview pane and chat panel works identically (heading prefixes stripped, word wrap correct)
- [ ] Zero v1 Charm imports remain anywhere in go.mod or source files

## Risks and Mitigations

| Risk | Likelihood | Mitigation |
|------|-----------|-----------|
| `DarkStyleConfig` struct removed in glamour v2 | Low | Use `WithStandardStyle("dark")` and find alternative for heading prefix removal |
| `ansi.StyleConfig` field types changed | Low | Check glamour v2 godoc; adapt field access |
| Subtle rendering differences in markdown output | Low | Visual comparison of chat panel and preview pane before/after |
| New transitive deps introduced by glamour v2 | Very Low | `go mod tidy` handles this; review go.sum diff |

## Comparison with Dispatch Migration

For context, the sibling project [dispatch](https://github.com/jongio/dispatch/blob/main/docs/issues/upgrade-charm-ecosystem-to-v2.md) required a full v1→v2 migration:

| Metric | Dispatch | Grut |
|--------|----------|------|
| Files affected | ~45 files, ~5,500 lines | **1 file, ~50 lines** |
| Import path changes | All packages | Glamour only |
| View() signature change | Yes (string → tea.View) | Already done |
| Key message migration | 60+ instances | N/A (custom keymap) |
| Mouse message migration | 15+ test instances | N/A |
| AdaptiveColor removal | 13 instances | 0 instances |
| Migration phases | 11 phases | 1 phase |
| Risk level | CRITICAL | LOW |

Grut's v2 migration was already completed for the hard parts. This issue covers the final cleanup step.

## Related

- Glamour v2 releases: https://github.com/charmbracelet/glamour/releases
- Glamour v2 Upgrade Guide: https://github.com/charmbracelet/glamour/blob/main/UPGRADE_GUIDE_V2.md
- Dispatch's full v2 migration analysis: https://github.com/jongio/dispatch/blob/main/docs/issues/upgrade-charm-ecosystem-to-v2.md
- Charm v2 announcement: https://charm.land/blog/v2/
- Affected file: `internal/markdown/render.go`
