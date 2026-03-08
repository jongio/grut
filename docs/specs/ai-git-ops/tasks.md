# AI-Assisted Git Operations — Tasks

> Spec: [spec.md](spec.md)

<!-- NEXT: 0 -->

## TODO

### 0. Re-evaluate codebase and validate spec
Before any implementation: re-analyze `internal/git/`, `internal/config/`, `cmd/`, and `internal/panels/` to confirm the spec's assumptions still hold (interfaces, types, config structs, panel architecture). Check that `git.Client` methods, `MergeRebaseOps` interface, `AIConfig` struct, and panel interfaces haven't changed since spec was written. Adjust spec and tasks if drift detected. Then run `mq` on the spec itself (code review the spec for completeness, consistency, missing edge cases, and technical accuracy). Priority: P0 (MUST complete before any other task).

### 1. Provider abstraction layer
Implement `internal/ai/provider.go`: `AIProvider` interface, `CompletionRequest`/`CompletionResponse`/`StreamChunk` types, provider registry with `Register()`/`Get()`, `GitContext` and `ConflictFile`/`ConflictRegion` types. Include `context.go` for building `GitContext` from grut's existing `git.Client` data. Priority: P0 (everything depends on this).

### 2. Content redaction engine
Implement `internal/ai/redact.go`: scan file contents against configurable patterns (glob + regex), replace detected secrets (env vars, API keys, PEM blocks) with `REDACTED` placeholders, support `.env` file exclusion, test with golden files. Priority: P0 (required before any content leaves the process).

### 3. Audit logging
Implement `internal/ai/audit.go`: structured log writer for AI operations (timestamp, operation, provider, files sent, tokens used, user decision). Write to `~/.config/grut/ai-audit.log`. Include log rotation. Priority: P1.

### 4. Copilot SDK provider
Implement `internal/ai/provider_copilot.go`: `CopilotProvider` struct implementing `AIProvider`. Auth via `gh auth token`. Handle streaming. Confirm SDK availability or implement against REST API. Priority: P0.

### 5. Claude SDK provider
Implement `internal/ai/provider_claude.go`: `ClaudeProvider` struct implementing `AIProvider`. Auth via `ANTHROPIC_API_KEY` env var. Use official `anthropic-sdk-go`. Handle streaming. Priority: P0.

### 6. AI config extension
Extend `internal/config/config.go`: expand `AIConfig` struct with provider selection, fallback, redaction patterns, temperature, max context files. Add `[ai.copilot]`, `[ai.claude]`, `[ai.review]`, `[ai.conflict]` sub-sections. Add validation rejecting embedded API keys. Update `defaults.toml`. Priority: P0.

### 7. Smart conflict resolution engine
Implement `internal/ai/ops/conflict.go`: parse conflict markers from files, extract ours/theirs/base regions, build `ConflictFile` data, format conflict-specific prompts with branch history context, process AI response into resolved file content. Support both auto-resolve and per-conflict interactive modes. Priority: P0 (core feature).

### 8. Commit message generation
Implement `internal/ai/ops/commit.go`: collect staged diff + recent commit log for style matching + branch name, generate conventional commit message, return for user review/edit. Priority: P1.

### 9. Code review of diffs
Implement `internal/ai/ops/review.go`: take diff (staged, unstaged, or range), build context with affected file contents, return annotated findings with severity/file/line. Priority: P1.

### 10. PR description generation
Implement `internal/ai/ops/pr.go`: diff current branch vs target, collect all branch commits, generate structured PR description (summary, changes, testing notes, breaking changes). Output markdown. Priority: P1.

### 11. Interactive rebase assistance
Implement `internal/ai/ops/rebase.go`: analyze commits to be rebased, suggest squash/fixup/reorder based on commit content analysis, return annotated rebase plan. Priority: P2.

### 12. Branch cleanup suggestions
Implement `internal/ai/ops/branch.go`: analyze all branches (merge status, staleness, naming), categorize as safe-to-delete/stale/abandoned, return recommendations. Priority: P2.

### 13. Bisect analysis
Implement `internal/ai/ops/bisect.go`: during bisect, analyze diffs between good/bad to suggest most likely culprit commits, reason about change content. Priority: P2.

### 14. Changelog generation
Implement `internal/ai/ops/changelog.go`: get commits between refs, categorize (features/fixes/breaking/deps/docs), generate Keep a Changelog format output. Priority: P2.

### 15. Commit splitting suggestions
Implement `internal/ai/ops/split.go`: analyze large commit diff, identify logical groupings, suggest split plan with file assignments and commit messages. Priority: P3.

### 16. CLI integration (native flags)
Enhance existing `cmd/` commands with AI flags: `grut merge/rebase` auto-triggers AI on conflicts, `grut commit` pre-fills AI message, `grut diff --review` adds annotations, `grut branch --cleanup` shows suggestions, `grut log --changelog` generates changelog, `grut push` with optional AI review gate. Add global `--no-ai` flag. No separate `grut ai` command group. Priority: P1 (parallel with TUI work).

### 17. TUI conflict resolution view
Implement `internal/panels/aiconflict/`: three-way diff view (base/ours/theirs) with AI suggestion. Activates automatically when merge/rebase produces conflicts (replaces raw conflict marker view). Keybindings: accept/ours/theirs/edit/next. Integrates with existing layout engine. Priority: P1.

### 18. TUI diff panel inline review
Enhance `internal/panels/gitdiff/`: when AI enabled, inline review annotations auto-populate when viewing diffs. Severity badges at relevant lines. Toggle keybinding to show/hide. Priority: P2.

### 19. TUI commit message pre-fill
Enhance commit input flow: when AI enabled, AI-generated commit message pre-fills automatically when commit input opens. `Tab` to accept, start typing to replace, `Esc` to clear. Priority: P2.

### 20. TUI branch panel inline annotations
Enhance `internal/panels/branches/`: when AI enabled, stale/merged/abandoned branches auto-annotated with status indicators and AI reasoning. Priority: P3.

### 21. Provider integration tests
Write integration tests with real providers (env-var-gated for CI). Test conflict resolution, commit messages, and review against known scenarios. Golden file regression tests. Priority: P1.

### 22. Security tests
Verify redaction: no secrets leak, .env excluded, PEM blocks replaced. Test config validation rejects embedded API keys. Priority: P1.

## IN PROGRESS

(none)

## DONE

(none)
