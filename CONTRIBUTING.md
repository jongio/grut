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
curl -fsSL https://raw.githubusercontent.com/jongio/grut/main/install.sh | sh -s -- v0.8.0

# Latest — Windows (PowerShell)
irm https://raw.githubusercontent.com/jongio/grut/main/install.ps1 | iex

# Specific version — Windows (PowerShell)
$v="v0.8.0"; irm https://raw.githubusercontent.com/jongio/grut/main/install.ps1 | iex
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

#### Test file naming

Test files follow a layered naming scheme so each concern lives in its own
file:

| Suffix | Purpose | Example |
|---|---|---|
| `feature_test.go` | Core unit tests for the happy path and basic error cases | `client_test.go` |
| `feature_extra_test.go` | Edge cases, coverage gaps, and hardening scenarios | `executor_extra_test.go` |
| `feature_integration_test.go` | Integration tests that exercise multiple packages together | `undo_integration_test.go` |
| `feature_bench_test.go` | Benchmarks (`Benchmark*` functions) | `bench_test.go` |

When adding tests, put them in the file that matches their purpose. Create a
new `_extra_test.go` or `_integration_test.go` file when the existing unit-test
file would become too large or when the tests have different setup needs.

#### `TestMain` isolation

Packages that shell out to external tools (e.g. `internal/git`,
`internal/github`) use a `TestMain` function to isolate the test process from
the host environment:

```go
func TestMain(m *testing.M) {
    os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
    os.Setenv("GIT_CONFIG_GLOBAL", "")
    os.Setenv("GIT_TERMINAL_PROMPT", "0")
    os.Exit(m.Run())
}
```

This prevents user-level git configuration (GPG signing, credential helpers, LFS
filters) from causing interactive prompts or hangs during CI. If your package
invokes an external tool that reads user config, add a `TestMain` with the
appropriate env-var overrides.

#### Test helpers

Mark reusable test setup functions with `t.Helper()` so that failure messages
report the caller's line number instead of the helper's:

```go
func setupTestRepo(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()
    // ... init a git repo ...
    return dir
}
```

Use `t.TempDir()` for throwaway directories (cleaned up automatically) and
`t.Setenv()` for scoped environment changes.

#### Running tests

| Command | What it does |
|---|---|
| `go test ./...` | Run all unit tests |
| `go test -race ./...` | Run with the race detector |
| `go test -bench=. ./internal/ai/` | Run benchmarks in a specific package |
| `mage preflight` | Full 14-step preflight (see below) |

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

### Comment Style

Follow these conventions to keep documentation consistent across the codebase.

#### Package doc comments

Every package must have a doc comment on (or immediately before) the `package` line. The comment starts with `// Package <name>` and describes what the package does:

```go
// Package git wraps the git CLI to provide typed, safe access to git
// operations.
package git
```

#### GoDoc on exported identifiers

Every exported type, function, method, and constant must have a comment that starts with the identifier name. Keep the first sentence short and specific:

```go
// PathJail restricts file operations to within the git repository root.
type PathJail struct { ... }

// NewPathJail creates a PathJail anchored at root.
func NewPathJail(root string, followSymlinks bool) (*PathJail, error) { ... }
```

Unexported helpers don't require a doc comment, but add one when the intent isn't obvious from the name and signature.

#### Section separators

Use box-drawing line separators (`─`) to group related functions or test cases within a file. Two styles are used in the codebase:

```go
// ────────────── Section Name ──────────────
```

```go
// ──────────────────────────────────────────────────────────────────────────
// Section Name
// ──────────────────────────────────────────────────────────────────────────
```

The short form works well in test files to mark individual test groups. The block form works well for larger logical sections in production or security-sensitive code. Pick whichever is already used in the file you're editing, or the short form for new files.

#### No redundant comments

Don't restate what the code already says. Comments should explain **why**, not **what**:

```go
// Bad - restates the code:
// Increment counter by one.
counter++

// Good - explains intent:
// Rate-limit retries to avoid hammering a failing remote.
time.Sleep(backoff)
```

### Error Handling

The codebase follows a small set of error handling rules. Stick to these so
error messages stay consistent and debuggable.

#### 1. Always wrap with context

Use `fmt.Errorf` with `%w` at every call site so the full chain of operations
is visible in the final error message:

```go
cfg, err := loadConfig(path)
if err != nil {
    return fmt.Errorf("init server: %w", err)
}
```

The prefix describes what the current function was trying to do, not the
function that failed.

#### 2. No sentinel errors

The codebase does not use module-level sentinel values like
`var ErrNotFound = errors.New(...)`. Errors are ad-hoc and contextual -
create them inline where they occur:

```go
// Good
return fmt.Errorf("repo %q not found", name)

// Avoid
var ErrNotFound = errors.New("not found")
```

#### 3. No custom error types

Use the standard `error` interface everywhere. Context is conveyed through
the wrapped message chain, not through type assertions or `errors.As`:

```go
// Good - message chain carries all the context
return fmt.Errorf("fetch remote %q: %w", remote, err)

// Avoid - custom type for carrying context
type FetchError struct { Remote string; Err error }
```

#### 4. Intentional nil returns

When a function intentionally returns `nil` instead of an error (e.g., a
missing file is not an error for that code path), annotate with `//nolint:nilerr`
and a brief explanation:

```go
if errors.Is(err, os.ErrNotExist) {
    return nil //nolint:nilerr // missing config is fine, use defaults
}
```

#### 5. No silent swallowing

Errors must always be returned or logged. If you intentionally discard an
error, make it explicit with a blank identifier and a comment:

```go
defer func() { _ = f.Close() }() // best-effort cleanup
```

#### 6. In tests

Use `require.NoError` when the test cannot continue without success, and
`assert.NoError` or `assert.Equal` for non-fatal checks:

```go
resp, err := client.Do(req)
require.NoError(t, err)                // fatal - stops the test
assert.Equal(t, http.StatusOK, resp.StatusCode) // non-fatal - keeps running
```

## Config Interface Pattern

Panels should depend on **narrow interfaces** rather than concrete config types
from `internal/config`. This inverts the dependency (panels define what they
need; the config package satisfies it) and enables lightweight test stubs
without importing `internal/config`.

### How it works

1. **Define an interface in the consumer package** with only the getters the
   panel actually uses:

   ```go
   // internal/panels/filetree/config.go
   package filetree

   type Config interface {
       GetIconMode() string
       GetMaxDepth() int
       GetShowHidden() bool
       // ... only what this panel needs
   }
   ```

2. **Add getter methods to the config struct** so it satisfies the interface
   implicitly (value receivers keep zero-copy passing):

   ```go
   // internal/config/getters.go
   func (c FileTreeConfig) GetIconMode() string { return c.IconMode }
   ```

3. **Accept the interface in the constructor**:

   ```go
   func New(cfg Config, rootPath string, th *theme.Theme) *FileTree { ... }
   ```

4. **Use getter calls** instead of direct field access:

   ```go
   // Before:  ft.cfg.MaxDepth
   // After:   ft.cfg.GetMaxDepth()
   ```

Callers (e.g. `internal/layout/registry.go`) need **zero changes** because
`config.FileTreeConfig` already satisfies `filetree.Config` via Go's implicit
interface satisfaction.

### Packages converted so far

| Package | Interface | Config struct |
|---------|-----------|---------------|
| `filetree` | `filetree.Config` | `config.FileTreeConfig` |
| `preview` | `preview.Config` | `config.PreviewConfig` |

### Packages still using concrete types

The `ActionsConfig` type is shared across many panels via `SetActionsCfg` and
the `rightclick` package. Converting it to an interface requires coordinating
changes in `rightclick.Cmd` and `config.SaveDoubleClickChoice` - a separate,
larger refactoring pass. Track progress in issue #68.

### Writing tests with stubs

Instead of importing `config` in tests, implement the interface directly:

```go
type stubConfig struct{}
func (stubConfig) GetIconMode() string           { return "ascii" }
func (stubConfig) GetMaxDepth() int              { return 10 }
func (stubConfig) GetShowHidden() bool           { return false }
func (stubConfig) GetShowIcons() bool            { return true }
func (stubConfig) GetSortDirectoriesFirst() bool { return true }
func (stubConfig) GetGitStatusMarkers() bool     { return true }
func (stubConfig) GetFollowSymlinks() bool       { return false }
```

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

## Interface Naming

Interfaces use a small set of suffixes that signal their purpose at a glance.
Avoid generic names like `Service` or `Handler` - pick the suffix that describes
what the interface actually does.

| Suffix | Meaning | Examples |
|---|---|---|
| **Reader** | Read-only queries, no side effects | `StatusReader`, `IgnoreChecker` |
| **Manager** | Stateful management of a resource | `BranchManager`, `UndoManager` |
| **Ops** | A collection of related operations | `RemoteOps`, `WorktreeOps`, `StashOps`, `TagOps`, `MergeRebaseOps`, `BisectOps`, `ReflogOps`, `DiscardOps`, `RevertOps`, `ResetOps` |
| **Provider** | A pluggable backend implementation | `AIProvider` |
| **Logger** | Logging or auditing | `AuditLogger` |
| **Checker** | A boolean query | `IgnoreChecker` |

**Rules:**

- **Compose, don't monolith.** Prefer many small, focused interfaces over a
  single large one. A `StatusReader` with three methods is better than a
  `GitService` with thirty.
- **Composite interfaces** (e.g., `GitClient`) embed multiple micro-interfaces
  and are defined in a dedicated `interfaces.go` file.
- **Compile-time compliance** is verified with a blank identifier assignment:

  ```go
  var _ GitClient = (*Client)(nil)
  ```

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you agree to uphold this code.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
