# AI-Assisted Git Operations — Tasks

> Spec: [spec.md](spec.md)

<!-- NEXT: 23 -->

Status last reconciled against the codebase on 2026-08-27. Every item below was
verified by locating the implementing file or test, not by reading commit
history.

## TODO

### 20. TUI branch panel inline annotations
Enhance `internal/panels/branches/`: when AI enabled, stale/merged/abandoned branches auto-annotated with status indicators and AI reasoning. Priority: P3.

The analysis engine already exists in `internal/ai/ops/branch.go` (task 12); only the panel wiring is outstanding. `ops.Branch` is currently referenced from no non-test panel code.

**Blocked on #418.** The wiring is not panel-local. `AnalyzeBranches` lives on the concrete `*middleware.AIGitClient` rather than the `git.GitClient` interface, panels are registered with the plain client before the AI client is built (`cmd/root.go:232` vs `:283`), and the layer that converts op results into panel messages does not exist yet. The same gap blocks six other shipped ops, so the access pattern should be decided once in #418 rather than invented here.

## IN PROGRESS

(none)

## DONE

### 0. Re-evaluate codebase and validate spec
Completed 2026-08-27. Confirmed the provider layer, redaction, audit, config, and ops packages match the shapes the spec assumed. One drift found and resolved: task 16 assumed CLI git subcommands that were never built (see Dropped below).

### 1. Provider abstraction layer
`internal/ai/provider.go`, `context.go`, `registry.go`.

### 2. Content redaction engine
`internal/ai/redact.go`. Covered by `redact_test.go` including AWS keys, PEM blocks, connection strings, GitHub tokens, and line-count preservation.

### 3. Audit logging
`internal/ai/audit.go`, `audit_test.go`.

### 4. Copilot SDK provider
`internal/ai/provider_copilot.go`.

### 5. Claude SDK provider
`internal/ai/provider_claude.go`.

### 6. AI config extension
`internal/config/`, with `[ai]`, `[ai.mcp]`, `[ai.copilot]`, `[ai.claude]`, `[ai.review]`, `[ai.conflict]`, `[ai.changelog]`, `[ai.commit_split]`, and `[ai.chat]` sections in `defaults.toml`.

### 7. Smart conflict resolution engine
`internal/ai/ops/conflict.go`.

### 8. Commit message generation
`internal/ai/ops/commit.go`.

### 9. Code review of diffs
`internal/ai/ops/review.go`.

### 10. PR description generation
`internal/ai/ops/pr.go`.

### 11. Interactive rebase assistance
`internal/ai/ops/rebase.go`.

### 12. Branch cleanup suggestions
`internal/ai/ops/branch.go`. The engine is complete; surfacing it in the branches panel is task 20.

### 13. Bisect analysis
`internal/ai/ops/bisect.go`.

### 14. Changelog generation
`internal/ai/ops/changelog.go`.

### 15. Commit splitting suggestions
`internal/ai/ops/split.go`.

### 17. TUI conflict resolution view
`internal/panels/aiconflict/`.

### 18. TUI diff panel inline review
`internal/panels/gitdiff/` carries `reviewFindings` and renders `panels.AIReviewFinding` with the severity constants in `gitdiff/constants.go`.

### 19. TUI commit message pre-fill
Covered by `TestCommitWithAISuggestionPrefillsExtra`.

### 21. Provider integration tests
`internal/ai/integration_test.go`, covering registry-to-provider commit generation, review, and the context-builder/redaction path.

### 22. Security tests
`internal/ai/security_test.go` plus the redaction suite in `redact_test.go`.

## Dropped

### 16. CLI integration (native flags)
Dropped 2026-08-27. The task assumed `grut merge`, `rebase`, `commit`, `diff`, `branch`, `log`, and `push` subcommands would carry AI flags. Those subcommands were never built, and the CLI has since settled into a different and deliberate split:

- The **CLI** is a management surface: `doctor`, `config`, `theme`, `keys`, `report`, `run`, `status`, `clean`, `update`, `ext`, `mcp`.
- The **TUI** is where git operations live, so that is where AI assists them (tasks 17 to 19).

The one part of this task that is not tied to those subcommands did ship: the global `--no-ai` flag is registered in `cmd/root.go`.

Reopen this only if grut grows non-interactive git subcommands. AI flags are a consequence of having those commands, not a reason to add them.