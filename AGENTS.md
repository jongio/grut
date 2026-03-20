# Contributing Guide for AI Agents

Instructions for AI coding agents working on this project.

## Build & Test

```bash
go build ./...          # Build — run after every change
go test ./... -count=1  # Test — all packages must pass
go vet ./...            # Lint
mage preflight          # Full preflight (fmt, tidy, vet, lint, build, test, race, vulncheck, gofumpt, deadcode)
mage install            # Full deploy (test + build + install)
```

### Preflight Before Push (MANDATORY)

Run `mage preflight` before any commit/push. It executes 12 checks matching CI:
`fmt → tidy → mod verify → vet → lint → build → test → race test → WSL test → vulncheck → gofumpt → deadcode`.
This catches formatting (`gofmt`), linting, and other issues that `go build` and `go vet` alone miss.

## Project Structure

- **Language:** Go
- **TUI framework:** [Bubble Tea v2](https://github.com/charmbracelet/bubbletea)
- **Styling:** [lipgloss](https://github.com/charmbracelet/lipgloss)
- **Key packages:**
  - `cmd/` — CLI commands (root, run, update, version, demo, ext)
  - `internal/tui/` — TUI model, components, styles
  - `internal/panels/` — UI panels (filetree, diff, status, chat, etc.)
  - `internal/git/` — Git operations
  - `internal/ai/` — AI integration
  - `internal/config/` — User configuration
  - `internal/extension/` — Extension system (Lua, WASM)
  - `internal/theme/` — Theme and color management
  - `internal/keymap/` — Keyboard bindings
  - `internal/layout/` — Layout management
