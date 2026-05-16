# Contributing to grüt

Thank you for your interest in contributing to grüt! This document provides guidelines and instructions for contributing.

## Getting Started

### Prerequisites

- **Go 1.26+** (check with `go version`)
- **Git** (check with `git --version`)
- **Mage** (optional, install with `go install github.com/magefile/mage@latest`)

### Setup

1. Fork and clone the repository:

   ```bash
   git clone https://github.com/<your-username>/grut.git
   cd grut
   ```

2. Install dependencies:

   ```bash
   go mod download
   ```

3. Build and install the dev binary:

   ```bash
   mage install
   ```

   This runs tests, builds `grut-dev` with version info, adds it to your PATH, and verifies the install. You can now run `grut-dev` alongside any release version of `grut`.

   Or without mage:

   ```bash
   go build -o grut .
   ```

## Installing a Release

To install the latest release (or test against a specific version):

```sh
# Latest — Linux / macOS
curl -fsSL https://raw.githubusercontent.com/jongio/grut/main/install.sh | sh

# Specific version — Linux / macOS
curl -fsSL https://raw.githubusercontent.com/jongio/grut/main/install.sh | sh -s -- v0.1.0

# Latest — Windows (PowerShell)
irm https://raw.githubusercontent.com/jongio/grut/main/install.ps1 | iex

# Specific version — Windows (PowerShell)
$v="v0.1.0"; irm https://raw.githubusercontent.com/jongio/grut/main/install.ps1 | iex
```

## Development Workflow

### Building

```bash
go build ./...
```

### Testing

```bash
go test ./...
```

### Full Preflight Check

Run all checks before submitting a PR:

```bash
mage preflight
```

This runs 14 checks: `fmt → tidy → mod verify → vet → lint → build → test → race test → WSL test → vulncheck → gofumpt → deadcode → benchmark smoke → benchmark regression`

Some checks (golangci-lint, govulncheck, gofumpt, deadcode, WSL) are skipped if the tool is not installed, with instructions printed to install them.

### Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Use `golangci-lint` for linting
- Keep functions focused and well-named
- Add tests for new functionality

## Submitting Changes

### Pull Request Process

1. Create a feature branch from `main`:

   ```bash
   git checkout -b feature/my-feature
   ```

2. Make your changes with clear, descriptive commits.

3. Run the preflight checks:

   ```bash
   mage preflight
   ```

4. Push your branch and open a Pull Request.

5. Describe your changes clearly in the PR description.

### Commit Messages

Use clear, descriptive commit messages:

- `feat: add worktree switching support`
- `fix: resolve path traversal in file tree`
- `docs: update README with new keybindings`
- `test: add coverage for git status parser`

### What We Look For

- **Tests**: New features should include tests
- **Documentation**: Update docs for user-facing changes
- **Backward Compatibility**: Avoid breaking existing behavior
- **Clean Code**: Follow existing patterns in the codebase

## Reporting Issues

- Use [GitHub Issues](https://github.com/jongio/grut/issues) to report bugs
- Include steps to reproduce, expected behavior, and actual behavior
- Include your OS, terminal emulator, and grüt version (`grut --version`)

## Recognition

All contributors are automatically recognized in:
- The **CHANGELOG.md** entry for each release
- **GitHub Release** notes
- The **CONTRIBUTORS.md** hall of fame

Your first contribution gets a special "New contributor" callout! We extract
contributors from git commit authors and `Co-authored-by:` trailers, so
pair-programming and AI-assisted contributions are credited too.

To manually regenerate the contributors list:

```bash
mage contributors
```

## Library Choices

grut uses specific canonical libraries for each domain. Don't introduce alternatives
without discussing in an issue first - consistency matters more than marginal gains.

| Domain | Library | Notes |
|--------|---------|-------|
| **TUI framework** | [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) | Elm-architecture TUI. All panels are `tea.Model` implementations. |
| **TUI styling** | [Lip Gloss v2](https://github.com/charmbracelet/lipgloss) | Layout and styling for all TUI components. |
| **TUI components** | [Bubbles v2](https://github.com/charmbracelet/bubbles) | Reusable TUI widgets (viewport, textinput, list, etc.). |
| **Markdown rendering** | [Glamour v2](https://github.com/charmbracelet/glamour) | Terminal markdown rendering for preview and chat panels. |
| **CLI framework** | [Cobra](https://github.com/spf13/cobra) | Command and flag parsing. All commands live in `cmd/`. |
| **Config (TOML)** | [go-toml/v2](https://github.com/pelletier/go-toml/v2) | User configuration in `grut.toml`. |
| **Config (YAML)** | [yaml.v3](https://github.com/go-yaml/yaml) | YAML parsing where needed. |
| **Logging** | `log/slog` (stdlib) | Structured logging throughout. No third-party loggers. |
| **Testing** | stdlib `testing` + [testify](https://github.com/stretchr/testify) | `require`/`assert` for assertions. No other test frameworks. |
| **Git operations** | `os/exec` calling `git` CLI | Shell out to git - no go-git. See `internal/git/`. |
| **GitHub API** | [go-github/v68](https://github.com/google/go-github) | GitHub REST API client. See `internal/github/`. |
| **AI providers** | [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go), [copilot-sdk/go](https://github.com/github/copilot-sdk/go) | Claude and Copilot integrations. See `internal/ai/`. |
| **MCP** | [mcp-go](https://github.com/mark3labs/mcp-go) | Model Context Protocol server. See `internal/mcp/`. |
| **Extension runtimes** | [wazero](https://github.com/tetratelabs/wazero) (Wasm), [gopher-lua](https://github.com/yuin/gopher-lua) (Lua) | Plugin execution sandboxes. See `internal/extension/runtime/`. |
| **Syntax highlighting** | [chroma/v2](https://github.com/alecthomas/chroma) | Code highlighting in preview and diff panels. |
| **Fuzzy matching** | [fuzzy](https://github.com/sahilm/fuzzy) | Fuzzy finder panel filtering. |
| **File watching** | [fsnotify](https://github.com/fsnotify/fsnotify) | File system change notifications. |

**Rules:**

- Prefer stdlib when it covers the need (e.g., `log/slog`, `os/exec`, `testing`).
- If you need something a listed library already handles, use that library.
- Proposing a new dependency? Open an issue explaining why existing libraries don't suffice.

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you agree to uphold this code.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
