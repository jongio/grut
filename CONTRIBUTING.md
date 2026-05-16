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

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you agree to uphold this code.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
