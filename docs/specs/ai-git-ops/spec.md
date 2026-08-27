# AI-Assisted Git Operations — Spec

> Part of [grut](../../../README.md) | See also: [plan-architecture.md](../plan-architecture.md)

> [!IMPORTANT]
> **Status, reconciled 2026-08-27.** The AI layer described here is built. See
> [tasks.md](tasks.md) for the per-task status with implementing files.
>
> One part of this spec was **not** built and has been dropped: the CLI examples
> below assume `grut merge`, `commit`, `diff`, `branch`, `log`, and `push`
> subcommands. Those never existed. Git operations live in the TUI, and the CLI
> is a management surface (`doctor`, `config`, `theme`, `keys`, `report`, `run`,
> `status`, `clean`, `update`, `ext`, `mcp`). Read every `**CLI**:` line below as
> historical design intent, not as shipped behaviour. The global `--no-ai` flag
> is the one piece of that surface that did ship.

## Problem

Git operations like rebasing, merging, and conflict resolution are cognitively expensive. Developers must understand the intent behind conflicting changes, remember the purpose of each branch, and manually reconcile diffs that an LLM could reason about in seconds. Grut already wraps the full git CLI with structured types (23 files in `internal/git/`), but today every merge conflict, commit message, and PR description is a manual effort.

This feature adds an **AI layer** on top of grut's existing git client, allowing LLMs to assist with conflict resolution, commit authoring, code review, and other git operations. The system supports multiple LLM providers (starting with GitHub Copilot SDK and Anthropic Claude SDK) via a provider-agnostic interface, configurable through grut's TOML config.

## Non-Goals

- Replacing git itself — AI is advisory/assistive, never autonomous without user approval
- Building a general-purpose AI chat interface — this is git-operation-specific
- Training or fine-tuning models — we consume existing APIs
- Auto-pushing or auto-merging without explicit user confirmation

## Design Principles

1. **Human-in-the-loop** — AI suggests, user approves. Every AI action is reviewable before application
2. **Provider-agnostic** — `AIProvider` interface abstracts LLM details; providers are swappable via config
3. **Git-native** — AI operates on grut's existing `FileDiff`, `Hunk`, `FileStatus` types, not raw text
4. **Offline-tolerant** — graceful degradation when no AI provider is configured or reachable
5. **Auditable** — every AI-assisted operation logs what was suggested vs. what was applied
6. **Security-first** — no code is sent to AI providers without user awareness; configurable redaction

## Architecture

### Provider Abstraction Layer

```
internal/ai/
├── provider.go          # AIProvider interface + registry
├── provider_copilot.go  # GitHub Copilot SDK implementation
├── provider_claude.go   # Anthropic Claude SDK implementation
├── config.go            # Provider config loading, validation
├── context.go           # Git context builder (repo state → prompt)
├── redact.go            # Sensitive content redaction (.env, secrets)
├── audit.go             # AI operation audit logging
└── ops/
    ├── conflict.go      # Merge/rebase conflict resolution
    ├── commit.go        # Commit message generation
    ├── review.go        # Diff code review
    ├── pr.go            # PR description generation
    ├── rebase.go        # Interactive rebase assistance
    ├── branch.go        # Branch cleanup suggestions
    ├── bisect.go        # Bisect analysis
    ├── changelog.go     # Changelog generation
    └── split.go         # Commit splitting suggestions
```

### AIProvider Interface

```go
// AIProvider abstracts an LLM backend for git-aware operations.
type AIProvider interface {
    // Name returns the provider identifier (e.g., "copilot", "claude").
    Name() string

    // Available reports whether the provider is configured and reachable.
    Available(ctx context.Context) (bool, error)

    // Complete sends a structured prompt and returns the completion.
    // The prompt includes git context (diffs, file contents, commit history).
    Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)

    // CompleteStream is like Complete but streams tokens for real-time UI.
    CompleteStream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)

    // Close releases provider resources.
    Close() error
}

// CompletionRequest carries structured git context to the LLM.
type CompletionRequest struct {
    Operation   string            // "conflict_resolve", "commit_message", etc.
    SystemPrompt string           // Operation-specific system prompt
    GitContext  GitContext         // Structured repo state
    UserPrompt  string            // User's additional instructions
    MaxTokens   int               // Response token limit
    Temperature float64           // Creativity control (lower = more deterministic)
}

// GitContext provides structured repository state for AI operations.
type GitContext struct {
    RepoRoot    string
    CurrentBranch string
    TargetBranch  string          // For merge/rebase operations
    Diffs       []git.FileDiff    // Relevant diffs
    Conflicts   []ConflictFile    // For conflict resolution
    Log         []git.Commit      // Recent commit history
    FileContents map[string]string // Relevant file contents (redacted)
    Status      []git.FileStatus  // Current working tree status
}

// ConflictFile represents a file with merge conflicts.
type ConflictFile struct {
    Path        string
    OursContent   string          // Current branch version
    TheirsContent string          // Incoming branch version
    BaseContent   string          // Common ancestor version
    ConflictMarkers []ConflictRegion // Parsed conflict regions
}

// ConflictRegion represents a single conflict within a file.
type ConflictRegion struct {
    StartLine   int
    EndLine     int
    Ours        string            // Current branch content
    Theirs      string            // Incoming branch content
    Base        string            // Common ancestor content (if available)
}
```

### Response Types

```go
// CompletionResponse holds the full AI response for non-streaming calls.
type CompletionResponse struct {
    Content     string            // Raw text response
    TokensUsed  TokenUsage        // Input/output token counts
    FinishReason string           // "stop", "length", "error"
    Metadata    map[string]string // Provider-specific metadata
}

// TokenUsage tracks token consumption per request.
type TokenUsage struct {
    InputTokens  int
    OutputTokens int
}

// StreamChunk represents a single streaming token or event.
type StreamChunk struct {
    Delta       string // Incremental text
    Done        bool   // True when stream is complete
    TokensUsed  *TokenUsage // Only set on final chunk
    Err         error  // Non-nil if stream encountered an error
}
```

### Operation-Specific Response Types

Each AI operation parses the raw `CompletionResponse.Content` into a structured result:

```go
// ConflictResolution is the parsed result of AI conflict resolution.
type ConflictResolution struct {
    File         string              // File path
    Regions      []ResolvedRegion    // One per conflict region
    FullResolved string              // Complete resolved file content
}

type ResolvedRegion struct {
    OriginalRegion ConflictRegion    // The input conflict
    Resolution     string            // AI-suggested resolved content
    Explanation    string            // Brief reasoning (1-2 sentences)
    Confidence     float64           // 0.0-1.0, provider-estimated confidence
}

// ReviewFinding is a single AI review annotation on a diff.
type ReviewFinding struct {
    File       string
    Line       int                   // Line number in the new file
    Severity   ReviewSeverity        // Error, Warning, Info
    Category   string                // "security", "bug", "style", "performance", "test"
    Message    string                // Human-readable finding
    Suggestion string                // Optional: suggested fix (as code)
}

type ReviewSeverity int

const (
    ReviewInfo ReviewSeverity = iota
    ReviewWarning
    ReviewError
)

// CommitSuggestion is the AI-generated commit message.
type CommitSuggestion struct {
    Subject string                   // First line (50 char convention)
    Body    string                   // Extended description
    Type    string                   // Conventional commit type: feat, fix, refactor, etc.
    Scope   string                   // Conventional commit scope (optional)
}

// PRDescription is the AI-generated PR description.
type PRDescription struct {
    Title         string
    Summary       string             // 1-3 sentence overview
    Changes       []string           // Bullet list of changes
    TestingNotes  string             // How to test
    Breaking      []string           // Breaking changes (empty if none)
    FullMarkdown  string             // Complete formatted PR body
}

// RebaseSuggestion is the AI's recommendation for a rebase plan.
type RebaseSuggestion struct {
    Commits []RebaseAction
}

type RebaseAction struct {
    Hash        string
    OriginalOp  string               // "pick" (all start as pick)
    SuggestedOp string               // "pick", "squash", "fixup", "reword", "drop"
    Reason      string               // Why this suggestion
    GroupWith   string               // Hash of commit to group with (for squash/fixup)
}

// BranchRecommendation is the AI's analysis of a single branch.
type BranchRecommendation struct {
    Name       string
    Action     string                // "keep", "delete", "archive", "rename"
    Reason     string
    IsMerged   bool
    LastCommit time.Time
    StaleDays  int
}

// ChangelogEntry is a categorized changelog item.
type ChangelogEntry struct {
    Category    string               // "added", "changed", "fixed", "removed", "security", "deprecated"
    Description string               // Human-readable description
    Commits     []string             // Contributing commit hashes
}

// SplitPlan is the AI's suggestion for splitting a commit.
type SplitPlan struct {
    OriginalHash string
    Pieces       []SplitPiece
}

type SplitPiece struct {
    Message string                   // Suggested commit message
    Files   []string                 // Files to include in this commit
    Reason  string                   // Why these files belong together
}
```

### Provider Registry

```go
// Registry manages available AI providers.
type Registry struct {
    mu        sync.RWMutex
    providers map[string]AIProvider
    primary   string                 // From config: ai.provider
    fallback  string                 // From config: ai.fallback_provider
}

// NewRegistry creates a registry from config, initializing configured providers.
func NewRegistry(cfg config.AIConfig) (*Registry, error)

// Get returns the primary provider, falling back if unavailable.
// Returns nil if no providers are configured or reachable.
func (r *Registry) Get(ctx context.Context) (AIProvider, error)

// GetByName returns a specific provider by name.
func (r *Registry) GetByName(name string) (AIProvider, bool)

// Close shuts down all providers.
func (r *Registry) Close() error
```

### Context Builder

The context builder transforms grut's git data into `GitContext`, managing token budgets:

```go
// Builder constructs GitContext from a git.Client, respecting token limits.
type Builder struct {
    client     git.GitClient
    redactor   *Redactor
    maxTokens  int                   // Token budget for context (not response)
}

// NewBuilder creates a context builder.
func NewBuilder(client git.GitClient, redactor *Redactor, maxTokens int) *Builder

// ForConflict builds context for conflict resolution.
// Includes: conflicted files, branch histories (last N commits each side),
// surrounding file content (configurable lines), merge base diff.
func (b *Builder) ForConflict(ctx context.Context, conflictFiles []string) (GitContext, error)

// ForCommit builds context for commit message generation.
// Includes: staged diff, recent N commits (style matching), branch name.
func (b *Builder) ForCommit(ctx context.Context) (GitContext, error)

// ForReview builds context for code review.
// Includes: diff, affected file contents, recent commits for change context.
func (b *Builder) ForReview(ctx context.Context, opts git.DiffOpts) (GitContext, error)

// ForPR builds context for PR description.
// Includes: full branch diff, all branch commits, branch name, target branch.
func (b *Builder) ForPR(ctx context.Context, targetBranch string) (GitContext, error)

// ForRebase builds context for rebase suggestions.
// Includes: commits to rebase with their individual diffs.
func (b *Builder) ForRebase(ctx context.Context, onto string) (GitContext, error)

// ForBisect builds context for bisect analysis.
// Includes: diff between good and bad, current commit changes, commit log in range.
func (b *Builder) ForBisect(ctx context.Context, good, bad string) (GitContext, error)

// ForChangelog builds context for changelog generation.
// Includes: all commits in range with subjects and bodies.
func (b *Builder) ForChangelog(ctx context.Context, fromRef, toRef string) (GitContext, error)

// ForSplit builds context for commit splitting.
// Includes: full diff of the commit, file dependency graph.
func (b *Builder) ForSplit(ctx context.Context, commitHash string) (GitContext, error)
```

**Token budget management**: The builder estimates token count using a simple heuristic (4 chars ≈ 1 token). When context exceeds the budget, it prioritizes:
1. Conflict regions / direct diffs (never truncated)
2. Immediately surrounding context (configurable lines)
3. Related file contents (truncated to most relevant sections)
4. Commit history (reduced to subjects only, then trimmed)

### Redaction Engine

```go
// Redactor removes sensitive content before it reaches an AI provider.
type Redactor struct {
    patterns     []glob.Glob          // File patterns to exclude entirely
    secretRegexps []*regexp.Regexp    // Content patterns to replace
}

// NewRedactor creates a redactor from config patterns.
// Default secret patterns (always active):
//   - AWS keys: AKIA[0-9A-Z]{16}
//   - Generic API keys: (api[_-]?key|secret|token|password)\s*[:=]\s*\S+
//   - PEM blocks: -----BEGIN .* PRIVATE KEY-----
//   - Connection strings: (postgres|mysql|mongodb|redis)://\S+
func NewRedactor(cfg config.AIConfig) *Redactor

// ShouldExcludeFile returns true if the file should be excluded entirely.
func (r *Redactor) ShouldExcludeFile(path string) bool

// RedactContent replaces detected secrets in content with placeholders.
// Returns the redacted content and count of redactions made.
func (r *Redactor) RedactContent(content string) (string, int)
```

### Audit Logger

```go
// AuditLogger records every AI operation for transparency and debugging.
type AuditLogger struct {
    path   string                    // Log file path
    mu     sync.Mutex
}

// AuditEntry represents a single AI operation log entry.
type AuditEntry struct {
    Timestamp   time.Time
    Operation   string               // "conflict_resolve", "commit_message", etc.
    Provider    string               // "copilot", "claude"
    FilesSent   []string             // File paths included in context
    Redactions  int                  // Number of secrets redacted
    TokensIn    int
    TokensOut   int
    Result      string               // "accepted", "rejected", "modified", "error"
    Error       string               // Error message if Result == "error"
}

// Log writes an audit entry. Thread-safe.
func (a *AuditLogger) Log(entry AuditEntry) error
```

### System Prompts

Each operation uses a specific system prompt. Prompts are embedded as Go constants (not fetched from external sources):

```go
// Conflict resolution prompt
const conflictSystemPrompt = `You are a git merge conflict resolver. You receive:
- The base version (common ancestor)
- "Ours" (current branch changes)
- "Theirs" (incoming branch changes)
- Recent commit history from both branches explaining intent

Your job:
1. Understand the INTENT behind each side's changes (not just the text diff)
2. Produce a merged result that preserves both intents
3. If intents genuinely conflict (e.g., both sides rename the same variable differently),
   prefer the incoming branch but note the conflict in your explanation
4. Never introduce new code that wasn't in either side
5. Return ONLY the resolved content for each conflict region, plus a brief explanation

Output format (JSON):
{"regions": [{"resolution": "resolved content here", "explanation": "why this resolution"}]}`

// Commit message prompt
const commitSystemPrompt = `You are a commit message generator. You receive:
- The staged diff (what changed)
- Recent commit history (for style matching)
- The current branch name

Your job:
1. Write a commit message matching the repository's existing style
2. If the repo uses conventional commits (feat:, fix:, etc.), follow that convention
3. Subject line: imperative mood, <=72 chars, no period
4. Body (if needed): explain WHY, not WHAT (the diff shows WHAT)
5. If the diff is trivial (whitespace, typo), keep the message simple

Output format (JSON):
{"subject": "...", "body": "...", "type": "feat|fix|refactor|...", "scope": "optional"}`

// Similar prompts exist for: review, pr, rebase, branch, bisect, changelog, split
// (See internal/ai/prompts.go for all prompts)
```

#### GitHub Copilot SDK

Uses the official GitHub Copilot SDK for Go. Authentication via existing GitHub CLI token (`gh auth token`) or environment variable. Leverages Copilot's code-aware models optimized for development tasks.

```go
type CopilotProvider struct {
    client  *copilot.Client         // Official SDK client (or HTTP client for REST fallback)
    model   string                   // Model override (default: SDK default)
}

// NewCopilotProvider initializes using gh CLI token.
// Token discovery order:
//   1. GITHUB_TOKEN env var
//   2. `gh auth token` command output
//   3. GitHub CLI config file (~/.config/gh/hosts.yml)
func NewCopilotProvider(cfg config.CopilotConfig) (*CopilotProvider, error)

// Available checks token validity and API reachability.
func (p *CopilotProvider) Available(ctx context.Context) (bool, error)

// Complete sends request using Copilot chat completions API.
// Maps CompletionRequest to Copilot's message format:
//   - SystemPrompt → system message
//   - GitContext rendered as structured user message (formatted diffs, file contents)
//   - UserPrompt → appended to user message
func (p *CopilotProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)

// CompleteStream uses Copilot's streaming API.
func (p *CopilotProvider) CompleteStream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)
```

**Fallback**: If the official Go SDK (`github.com/github/go-copilot`) is unavailable, implement against the Copilot REST API directly:
- Endpoint: `https://api.githubcopilot.com/chat/completions`
- Auth: `Authorization: Bearer <gh_token>`
- Format: OpenAI-compatible chat completions API

#### Anthropic Claude SDK

Uses the official Anthropic Go SDK (`github.com/anthropics/anthropic-sdk-go`). Authentication via API key in environment variable. Leverages Claude's strong reasoning for complex conflict resolution and code understanding.

```go
type ClaudeProvider struct {
    client  *anthropic.Client       // Official SDK client
    model   string                   // e.g., "claude-sonnet-4-20250514"
    maxTok  int                      // Max output tokens
}

// NewClaudeProvider initializes from ANTHROPIC_API_KEY env var.
// Returns error if env var not set.
func NewClaudeProvider(cfg config.ClaudeConfig) (*ClaudeProvider, error)

// Available checks API key validity with a minimal request.
func (p *ClaudeProvider) Available(ctx context.Context) (bool, error)

// Complete sends request using Claude messages API.
// Maps CompletionRequest to Claude's message format:
//   - SystemPrompt → system parameter
//   - GitContext rendered as structured content blocks (diffs as code blocks, commit history as lists)
//   - UserPrompt → user message
//   - Temperature, MaxTokens from request (overrides config defaults if set)
func (p *ClaudeProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)

// CompleteStream uses Claude's streaming messages API.
// Emits StreamChunks for each content_block_delta event.
func (p *ClaudeProvider) CompleteStream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)
```

#### Adding a New Provider (Extensibility)

To add a third provider (e.g., OpenAI, Ollama, local model):

1. Create `internal/ai/provider_<name>.go` implementing `AIProvider` (5 methods)
2. Add `[ai.<name>]` config section to `config.go`
3. Register in `NewRegistry()` — recognized by `ai.provider` config value
4. No changes needed to any `ops/` code, TUI code, or CLI code

### Provider Selection

```toml
# config.toml
[ai]
provider = "copilot"              # Default provider: "copilot" | "claude"
fallback_provider = "claude"      # Fallback if primary unavailable
redact_patterns = [".env", "*.key", "*.pem"]  # Files to redact before sending

[ai.copilot]
# Uses gh CLI token by default; no explicit config needed
# model = "gpt-4o"               # Optional model override

[ai.claude]
# api_key via ANTHROPIC_API_KEY env var (never in config file)
model = "claude-sonnet-4-20250514"
max_tokens = 8192
```

## AI-Assisted Operations

### 1. Smart Conflict Resolution

**Activation**: Automatic — when `Merge()` or `Rebase()` returns a conflict error, grut detects conflict markers and immediately activates AI resolution instead of dumping raw markers at the user.

**Two Modes** (configurable default, switchable per-operation):

- **Auto-resolve then review**: AI resolves all conflicts, presents unified diff for user approval
- **Interactive per-conflict**: Step through each conflict region, AI suggests resolution, user accepts/modifies/skips

**How it works**:
1. Parse conflict markers from conflicted files (ours/theirs/base extraction)
2. Build `GitContext` with: both branch histories, the diffs leading to conflict, surrounding file content
3. Send to AI provider with conflict-specific system prompt that instructs semantic merge (not just text diff)
4. AI returns resolved content per conflict region with brief explanation
5. User reviews in TUI diff panel (side-by-side: conflict → resolution) or CLI output
6. On approval, write resolved content and `git add` the file

**TUI**: Conflict resolution replaces the standard conflict view automatically. Three-way diff (base/ours/theirs) with AI suggestion highlighted. Keybindings: `a` accept AI suggestion, `o` keep ours, `t` keep theirs, `e` edit manually, `n` next conflict.

**CLI**: `grut merge feature-branch` — if conflicts arise, AI resolution flow starts automatically. `--no-ai` to fall back to standard conflict markers.

### 2. Commit Message Generation

**Activation**: Automatic — when user opens commit message input (TUI) or runs `grut commit` (CLI), AI pre-generates a commit message from staged changes. No special command needed.

**How it works**:
1. Get staged diff via `Diff(ctx, DiffOpts{Staged: true})`
2. Get recent commit history for style matching via `Log(ctx, LogOpts{MaxCount: 10})`
3. Build context: staged diff + commit history (for style/convention matching) + branch name
4. AI generates conventional commit message following repo's existing patterns
5. Message appears pre-filled; user edits freely or accepts as-is

**TUI**: When commit input opens, AI suggestion appears as pre-filled text (dimmed until user interacts). `Tab` to accept, start typing to replace, `Esc` to clear.

**CLI**: `grut commit` — AI message pre-fills the editor. User edits and saves as normal. `--no-ai` to skip generation.

### 3. Code Review of Diffs

**Activation**: Automatic — AI review annotations appear inline whenever the user views a diff (TUI) or uses `grut diff --review` (CLI). Optionally runs as a pre-push gate (`auto_review_on_push = true`).

**How it works**:
1. Get diff (staged, unstaged, or between branches)
2. Build context: diff + affected file contents + recent commits
3. AI reviews for: bugs, security issues, style violations, missing tests, logic errors
4. Presents findings as annotated diff (inline comments at relevant lines)

**TUI**: Review annotations appear inline in the diff viewer automatically. Severity badges (🔴 error, 🟡 warning, 🔵 info) at relevant lines. Toggle with keybinding if user wants clean diff view.

**CLI**: `grut diff --review` — diff with AI annotations. `grut push` with `auto_review_on_push` — blocks push if critical issues found (user can override). Standard `grut diff` shows clean diff (AI annotations opt-in via flag or config default).

### 4. PR Description Generation

**Activation**: Automatic — when user runs `grut pr` to create a PR, AI pre-generates the description. No separate command.

**How it works**:
1. Get diff between current branch and target (usually main/master)
2. Get all commits on the branch via `Log()`
3. Build context: full branch diff + commit history + branch name
4. AI generates structured PR description: summary, changes list, testing notes, breaking changes

**CLI**: `grut pr` — AI-generated description pre-fills the PR body. User edits before submitting. Output also works as pipe: `grut pr --body-only | gh pr create --body-file -`.

### 5. Interactive Rebase Assistance

**Activation**: Automatic — when user starts interactive rebase, AI suggestions appear as ghost annotations next to each commit in the rebase editor.

**How it works**:
1. Get commits to be rebased via `Log()`
2. Analyze commit contents (diffs per commit)
3. AI suggests: squash candidates (related commits), reorder for logical grouping, fixup targets, commits to split
4. Present as ghost annotations alongside standard pick/squash/fixup controls

**TUI**: Rebase editor shows AI suggestions as dimmed annotations (e.g., "← squash with #3: both modify auth.go"). User applies with keybinding or ignores.

**CLI**: `grut rebase -i main` — interactive rebase editor includes AI annotations. `--no-ai` to use plain rebase.

### 6. Branch Cleanup

**Activation**: Automatic — branch list panel always shows AI annotations for stale/merged branches when AI is enabled.

**How it works**:
1. List all branches via `BranchList()`
2. Get merge status (is each branch merged into main?)
3. Get last commit date per branch
4. AI analyzes: fully merged branches (safe to delete), stale branches (no commits in N days), branches with naming issues, branches that appear abandoned
5. Annotations appear inline in branch list

**TUI**: Branch panel shows status indicators (✓ merged, ⚠ stale, 🗑 safe to delete) with AI reasoning on hover/select. `d` on annotated branch shows confirmation.

**CLI**: `grut branch --cleanup` — branch list with cleanup annotations and interactive delete prompts.

### 7. Bisect Assistance

**Activation**: Automatic — when user runs `grut bisect start`, AI analysis runs alongside standard bisect.

**How it works**:
1. During bisect, get the diff between good and bad commits
2. Get the current bisect commit's changes
3. AI analyzes code changes to suggest which commit most likely introduced the bug
4. Narrows search faster than manual binary search by reasoning about change content

**TUI**: Bisect view shows AI probability indicators next to each remaining candidate commit. AI reasoning appears in info panel.

**CLI**: `grut bisect good/bad` — standard bisect commands with AI analysis printed after each step. Shows "AI suspects commit abc1234 (confidence: high): modifies error handling in auth.go".

### 8. Changelog Generation

**Activation**: Automatic — when user creates a tag or views a commit range, AI changelog is available.

**How it works**:
1. Get commits between two refs (tags, branches, or arbitrary commits) via `Log()`
2. AI categorizes commits: features, fixes, breaking changes, dependencies, docs
3. Generates formatted changelog (Keep a Changelog format or custom)
4. Groups by category, deduplicates, and produces human-readable descriptions

**TUI**: When viewing commit log for a range, `c` keybinding generates and displays AI changelog summary.

**CLI**: `grut log v1.0..v2.0 --changelog` — generates changelog for the range. `grut tag v2.0 --changelog` — creates tag and generates changelog since last tag.

### 9. Commit Splitting

**Activation**: When user views a commit with many files changed, AI automatically suggests split points. Also available via explicit flag.

**How it works**:
1. Get the diff of the target commit
2. AI analyzes: logical groupings of changes, file relationships, functional boundaries
3. Suggests split plan: N commits with descriptions and file assignments
4. User reviews and approves the plan
5. Executes: soft reset, selective staging per suggested commit, commit with AI-generated messages

**TUI**: When viewing a large commit (configurable threshold, default: 10+ files), a "split suggestion" annotation appears. Selecting it shows the proposed split plan.

**CLI**: `grut show HEAD --split` — shows split suggestions for a commit. `grut reset HEAD~ --split` — soft resets and applies AI-suggested split interactively.

## Native Integration — AI is Built Into Every Git Operation

AI is not a separate command or mode — it is woven into the standard git workflows. When AI is enabled (`[ai] enabled = true`), every relevant git operation gains AI intelligence automatically. Users interact with git the same way they always do; AI enhances the experience transparently.

### How AI Activates (No Special Commands)

| Standard Git Operation | AI Enhancement (Automatic) |
|------------------------|---------------------------|
| Merge/rebase hits a conflict | AI conflict resolution panel appears (replaces raw conflict markers) |
| User opens commit message input | AI pre-fills a suggested commit message from staged diff |
| User views a diff | AI review annotations appear inline (bugs, issues, suggestions) |
| User pushes | AI review runs as pre-push gate (configurable) |
| User starts interactive rebase | AI suggests squash/fixup/reorder alongside standard controls |
| User views branch list | Stale/merged branches are AI-annotated with cleanup suggestions |
| User runs bisect | AI narrows candidates by analyzing code changes |
| User creates a tag/release | AI generates changelog from commit history since last tag |
| User views a large commit | AI suggests split points for atomic commits |

### TUI — AI is Inline, Not a Separate Panel

AI suggestions appear **inside existing panels**, not in a separate "AI panel":

| Panel | AI Enhancement |
|-------|---------------|
| **Conflict resolution** (new) | Three-way diff (base/ours/theirs) with AI-suggested resolution. Appears automatically when merge/rebase conflicts. Keybindings: `a` accept AI, `o` ours, `t` theirs, `e` edit, `n` next |
| **Diff viewer** | Inline AI review annotations (severity badges at relevant lines). Auto-populated when viewing diffs if AI enabled |
| **Commit input** | AI-generated commit message pre-filled when user starts typing. `Tab` to accept, edit freely, `Esc` to clear |
| **Branch list** | Stale/merged indicators with AI reasoning. `d` on AI-suggested branch shows confirmation with context |
| **Rebase editor** | AI squash/fixup suggestions shown as ghost annotations next to each commit |
| **Log/history** | AI changelog summary when viewing commit ranges |

### CLI — AI Enhances Existing Commands

No `grut ai <x>` subcommands. AI enhances the standard workflow:

```bash
# Standard grut operations — AI activates automatically when configured
grut merge feature-branch     # If conflicts → AI resolution flow
grut commit                   # AI pre-generates commit message
grut diff --review            # Diff with AI annotations (--review flag, not separate command)
grut rebase main              # If conflicts → AI resolution; interactive rebase gets AI suggestions
grut branch --cleanup         # Branch list with AI cleanup recommendations
grut log v1.0..HEAD --changelog  # AI-generated changelog for the range
grut bisect start             # AI-enhanced bisect with code analysis
grut push                     # Optional AI review gate before push
```

The only AI-specific CLI flag is `--no-ai` to disable AI for a single operation when needed.

## Native Integration Hooks

AI plugs into grut's existing code at specific points. No new commands; existing functions gain AI awareness.

### Where AI Hooks Into Existing Code

```
internal/git/merge.go → Merge() returns error
  ↓ error contains "CONFLICT" or "Automatic merge failed"
  ↓ internal/ai/ops/conflict.go detects conflict state
  ↓ Parses conflict markers from working tree files
  ↓ Sends to AI provider → presents resolution
  ↓ On accept: writes resolved file, calls Stage()

internal/git/branch.go → Stage()/Unstage() complete, user initiates commit
  ↓ internal/ai/ops/commit.go reads staged diff
  ↓ Sends to AI provider → returns CommitSuggestion
  ↓ Pre-fills commit message input (TUI or CLI editor)
  ↓ User edits/accepts → calls Commit()

internal/git/diff.go → Diff() returns []FileDiff
  ↓ internal/ai/ops/review.go receives diff data
  ↓ Sends to AI provider → returns []ReviewFinding
  ↓ Annotations rendered inline in diff output (TUI panel or CLI)

internal/git/remote.go → Push() called
  ↓ If config ai.review.auto_review_on_push = true
  ↓ internal/ai/ops/review.go runs review first
  ↓ If critical findings → block push, show findings
  ↓ User overrides or fixes → Push() proceeds

internal/git/merge.go → Rebase() with Interactive flag
  ↓ internal/ai/ops/rebase.go analyzes commits in range
  ↓ Sends to AI provider → returns RebaseSuggestion
  ↓ Ghost annotations shown alongside rebase editor controls

internal/git/branch.go → BranchList() returns []Branch
  ↓ internal/ai/ops/branch.go analyzes branch metadata
  ↓ Sends to AI provider → returns []BranchRecommendation
  ↓ Annotations added to branch list display

internal/git/bisect.go → BisectGood()/BisectBad() called
  ↓ internal/ai/ops/bisect.go gets remaining candidate range
  ↓ Sends to AI provider → returns analysis with probability estimates
  ↓ Printed after each bisect step

internal/git/log.go → Log() returns []Commit for a range
  ↓ internal/ai/ops/changelog.go categorizes commits
  ↓ Sends to AI provider → returns []ChangelogEntry
  ↓ Rendered as formatted changelog
```

### AI Middleware Pattern

The integration uses a middleware/decorator pattern. The `AIGitClient` wraps the existing `git.Client`:

```go
// AIGitClient wraps a git.Client with AI capabilities.
// It implements the same git.GitClient interface, intercepting operations
// that benefit from AI and delegating everything else.
type AIGitClient struct {
    inner    git.GitClient           // The real git client
    registry *Registry               // AI provider registry
    builder  *Builder                // Context builder
    audit    *AuditLogger
    cfg      config.AIConfig
}

// Merge delegates to inner.Merge(), then checks for conflicts.
// If conflicts detected and AI enabled, automatically starts resolution.
func (a *AIGitClient) Merge(ctx context.Context, branch string, opts git.MergeOpts) error {
    err := a.inner.Merge(ctx, branch, opts)
    if err != nil && isConflictError(err) && a.cfg.Enabled {
        return a.handleConflicts(ctx, branch, err)
    }
    return err
}

// Rebase delegates to inner.Rebase(), then checks for conflicts.
func (a *AIGitClient) Rebase(ctx context.Context, onto string, opts git.RebaseOpts) error {
    err := a.inner.Rebase(ctx, onto, opts)
    if err != nil && isConflictError(err) && a.cfg.Enabled {
        return a.handleConflicts(ctx, onto, err)
    }
    return err
}

// All other GitClient methods delegate directly to inner.
```

This means **no existing code changes** in `internal/git/`. The AI layer is purely additive — it wraps the client at the call site (in `internal/tui/` or `cmd/`).

### Initialization Flow

```go
// In cmd/root.go or internal/tui/app.go:

gitClient := git.NewClient(repoDir)

if cfg.AI.Enabled {
    registry, err := ai.NewRegistry(cfg.AI)
    if err != nil {
        // Log warning, continue without AI
        slog.Warn("AI initialization failed", "error", err)
    } else {
        redactor := ai.NewRedactor(cfg.AI)
        builder := ai.NewBuilder(gitClient, redactor, estimateTokenBudget(cfg.AI))
        audit := ai.NewAuditLogger(auditLogPath)
        gitClient = ai.NewAIGitClient(gitClient, registry, builder, audit, cfg.AI)
    }
}

// Everything downstream uses gitClient — doesn't know or care if AI is active
```

### Streaming UX in TUI

For operations where AI generates text visible to the user (commit messages, PR descriptions, changelog):

1. Start streaming via `CompleteStream()`
2. Render tokens as they arrive into the target UI element (commit input, preview pane)
3. Show a subtle spinner/indicator while streaming
4. On stream complete, content becomes fully editable
5. If stream errors mid-way, show partial content with error toast, let user edit/retry

For operations where AI produces structured data (conflict resolution, review findings, rebase suggestions):

1. Use non-streaming `Complete()` (structured JSON needs complete response)
2. Show loading indicator in the relevant panel
3. On complete, parse JSON response and render structured UI elements
4. On error, show toast and fall back to non-AI behavior

### Graceful Degradation

When AI is unavailable (not configured, provider down, rate limited):

```
Tier 1: Primary provider configured and reachable → use it
Tier 2: Primary fails → try fallback provider
Tier 3: Both fail → silent degradation:
  - Merge conflicts: show standard conflict markers (existing behavior)
  - Commit: empty commit message input (existing behavior)
  - Diff: clean diff without annotations (existing behavior)
  - Branch list: standard branch list (existing behavior)
  - No error modals; only a subtle toast: "AI unavailable, using manual mode"
```

## Security & Privacy

### Content Redaction

Before sending any content to an AI provider:
1. Files matching `redact_patterns` are excluded entirely
2. Known secret patterns are detected and replaced (`REDACTED_SECRET`)
3. File paths are optionally anonymized (configurable)
4. User can review exactly what will be sent (TUI: preview panel; CLI: `--dry-run`)

### Authentication

- **Copilot**: GitHub CLI token (`gh auth token`), no secrets in config
- **Claude**: Environment variable `ANTHROPIC_API_KEY`, never stored in config file
- Config file validation rejects embedded API keys with clear error message

### Audit Log

Every AI operation is logged to `~/.config/grut/ai-audit.log`:
```
timestamp | operation | provider | files_sent | tokens_used | result (accepted/rejected/modified)
```

## Configuration

Extends existing `config.toml`. When `[ai]` section is absent, AI is disabled (zero-config safe). All fields have sensible defaults:

```toml
[ai]
enabled = true                          # Master kill switch (default: false if section absent)
provider = "copilot"                    # Primary: "copilot" | "claude" | "none"
fallback_provider = "claude"            # Fallback if primary fails (default: "")
redact_patterns = [".env", "*.key", "*.pem", "*.secret"]  # Files excluded from AI context
auto_commit_message = true              # Pre-fill commit messages when staging (default: true)
auto_review_diff = false                # Auto-run review when viewing diffs (default: false)
temperature = 0.2                       # Lower = more deterministic (0.0-1.0, default: 0.2)
max_context_files = 20                  # Limit files sent per request (default: 20)
max_context_tokens = 100000             # Token budget for context (default: 100000)

[ai.copilot]
# Authentication: automatic via gh auth token
# No config required for basic usage
# model = "gpt-4o"                      # Optional model override

[ai.claude]
# Authentication: ANTHROPIC_API_KEY env var (NEVER in config file)
model = "claude-sonnet-4-20250514"           # Model name (default: claude-sonnet-4-20250514)
max_tokens = 8192                       # Max output tokens per request (default: 8192)

[ai.review]
severity_threshold = "warning"          # Minimum severity to show: "error" | "warning" | "info"
auto_review_on_push = false             # Run AI review before push (default: false)
categories = ["security", "bug", "performance", "style", "test"]  # Review categories to check

[ai.conflict]
default_mode = "interactive"            # "auto" | "interactive" (default: "interactive")
include_surrounding_context = 50        # Lines of context around conflicts (default: 50)
auto_accept_high_confidence = false     # Auto-accept resolutions with confidence > 0.95 (default: false)

[ai.changelog]
format = "keepachangelog"               # "keepachangelog" | "conventional" | "simple"
categories = ["added", "changed", "fixed", "removed", "security", "deprecated"]

[ai.commit_split]
threshold = 10                          # Suggest splitting commits with >= N files changed (default: 10)
```

**Config validation rules** (enforced in `internal/config/validate.go`):
- `ai.provider` must be one of: "copilot", "claude", "none"
- `ai.temperature` must be in range [0.0, 1.0]
- `ai.max_context_files` must be >= 1
- Config file must NOT contain `api_key`, `secret`, or `token` fields under `[ai.*]` — reject with error message directing user to use env vars
- If `ai.claude` is configured but `ANTHROPIC_API_KEY` env var is not set, warn on startup (not error — user may configure later)

**Corresponding Go config structs** (extends existing `internal/config/config.go`):

```go
type AIConfig struct {
    Enabled            bool            `toml:"enabled"`
    Provider           string          `toml:"provider"`
    FallbackProvider   string          `toml:"fallback_provider"`
    RedactPatterns     []string        `toml:"redact_patterns"`
    AutoCommitMessage  bool            `toml:"auto_commit_message"`
    AutoReviewDiff     bool            `toml:"auto_review_diff"`
    Temperature        float64         `toml:"temperature"`
    MaxContextFiles    int             `toml:"max_context_files"`
    MaxContextTokens   int             `toml:"max_context_tokens"`
    Copilot            CopilotConfig   `toml:"copilot"`
    Claude             ClaudeConfig    `toml:"claude"`
    Review             ReviewConfig    `toml:"review"`
    Conflict           ConflictConfig  `toml:"conflict"`
    Changelog          ChangelogConfig `toml:"changelog"`
    CommitSplit        CommitSplitConfig `toml:"commit_split"`
}

type CopilotConfig struct {
    Model string `toml:"model"`
}

type ClaudeConfig struct {
    Model    string `toml:"model"`
    MaxTokens int   `toml:"max_tokens"`
}

type ReviewConfig struct {
    SeverityThreshold  string   `toml:"severity_threshold"`
    AutoReviewOnPush   bool     `toml:"auto_review_on_push"`
    Categories         []string `toml:"categories"`
}

type ConflictConfig struct {
    DefaultMode             string `toml:"default_mode"`
    SurroundingContext      int    `toml:"include_surrounding_context"`
    AutoAcceptHighConfidence bool  `toml:"auto_accept_high_confidence"`
}

type ChangelogConfig struct {
    Format     string   `toml:"format"`
    Categories []string `toml:"categories"`
}

type CommitSplitConfig struct {
    Threshold int `toml:"threshold"`
}
```

## Error Handling

| Scenario | Behavior |
|----------|----------|
| No provider configured | Operations work without AI; AI keybindings show "AI not configured" toast |
| Provider unreachable | Toast notification, fall back to fallback provider, then degrade gracefully |
| Rate limited | Queue operations, show progress, retry with backoff |
| Large diff (exceeds token limit) | Chunk by file, process sequentially, merge results |
| AI suggestion is nonsensical | User rejects in review step; log for audit; suggest manual resolution |
| Redaction removes critical context | Warn user that redaction may affect quality; offer to proceed or abort |

## Testing Strategy

### Unit Tests (all `ops/` functions)

Mock `AIProvider` via interface. Each operation has deterministic tests:

```go
// Example: conflict resolution test
func TestResolveConflicts(t *testing.T) {
    mock := &MockProvider{
        response: `{"regions": [{"resolution": "merged content", "explanation": "combined both"}]}`,
    }
    resolver := ops.NewConflictResolver(mock, redactor, builder)

    conflicts := []ai.ConflictFile{{
        Path: "main.go",
        ConflictMarkers: []ai.ConflictRegion{{
            Ours:   "func hello() { fmt.Println(\"hello\") }",
            Theirs: "func hello() { fmt.Println(\"hi\") }",
            Base:   "func hello() { fmt.Println(\"hey\") }",
        }},
    }}

    result, err := resolver.Resolve(ctx, conflicts)
    require.NoError(t, err)
    assert.Equal(t, "merged content", result[0].Regions[0].Resolution)
    assert.Equal(t, "conflict_resolve", mock.lastRequest.Operation)
}
```

### Golden File Tests

Known conflict scenarios with expected resolutions. Located in `internal/ai/testdata/`:

```
testdata/
├── conflicts/
│   ├── simple-variable-rename/
│   │   ├── base.go
│   │   ├── ours.go
│   │   ├── theirs.go
│   │   └── expected.go            # Expected resolution
│   ├── function-signature-change/
│   ├── import-conflict/
│   ├── both-add-to-same-list/
│   └── complex-refactor/
├── commits/
│   ├── feature-add/
│   │   ├── diff.patch
│   │   ├── history.json           # Recent commits for style matching
│   │   └── expected-message.txt
│   └── bugfix/
└── reviews/
    ├── security-issue/
    │   ├── diff.patch
    │   └── expected-findings.json
    └── clean-diff/
```

### Integration Tests (env-var-gated)

```go
// Only run when GRUT_TEST_COPILOT=1 or GRUT_TEST_CLAUDE=1
func TestCopilotIntegration(t *testing.T) {
    if os.Getenv("GRUT_TEST_COPILOT") != "1" {
        t.Skip("GRUT_TEST_COPILOT not set")
    }
    provider, err := ai.NewCopilotProvider(config.CopilotConfig{})
    require.NoError(t, err)

    ok, err := provider.Available(context.Background())
    require.NoError(t, err)
    assert.True(t, ok)

    // Test with a simple, known conflict
    resp, err := provider.Complete(ctx, smallConflictRequest)
    require.NoError(t, err)
    assert.NotEmpty(t, resp.Content)
    assert.Greater(t, resp.TokensUsed.OutputTokens, 0)
}
```

### Security Tests

```go
func TestRedactionPreventsSecretLeaks(t *testing.T) {
    redactor := ai.NewRedactor(config.AIConfig{
        RedactPatterns: []string{".env", "*.key"},
    })

    // Test file exclusion
    assert.True(t, redactor.ShouldExcludeFile(".env"))
    assert.True(t, redactor.ShouldExcludeFile("certs/server.key"))
    assert.False(t, redactor.ShouldExcludeFile("main.go"))

    // Test content redaction
    content := `password = "s3cret"\nAKIAIOSFODNN7EXAMPLE`
    redacted, count := redactor.RedactContent(content)
    assert.Equal(t, 2, count)
    assert.NotContains(t, redacted, "s3cret")
    assert.NotContains(t, redacted, "AKIAIOSFODNN7EXAMPLE")
}

func TestConfigRejectsEmbeddedAPIKeys(t *testing.T) {
    cfg := `[ai.claude]\napi_key = "sk-ant-..."`
    _, err := config.Load(cfg)
    assert.ErrorContains(t, err, "API keys must not be stored in config")
}
```

### TUI Tests

Bubble Tea test framework for panel interactions:

```go
func TestConflictPanelAcceptKeybinding(t *testing.T) {
    panel := aiconflict.New(mockConflicts, mockResolutions)
    panel.SetSize(80, 24)

    // Press 'a' to accept AI suggestion
    updated, cmd := panel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
    // Verify the first conflict region is marked as resolved
    assert.True(t, updated.(*aiconflict.Panel).CurrentRegionResolved())
}
```

### Conflict Marker Parsing

The conflict parser handles standard git conflict markers and `diff3` style (with base):

```go
// ParseConflictMarkers reads a file with conflict markers and extracts regions.
// Handles both two-way and three-way (diff3) conflict markers:
//
// Two-way:
//   <<<<<<< HEAD
//   our changes
//   =======
//   their changes
//   >>>>>>> feature-branch
//
// Three-way (diff3, enabled via merge.conflictstyle=diff3):
//   <<<<<<< HEAD
//   our changes
//   ||||||| merged common ancestors
//   base content
//   =======
//   their changes
//   >>>>>>> feature-branch
//
func ParseConflictMarkers(content string) ([]ConflictRegion, error)

// ParseConflictFiles reads all conflicted files from the working tree.
// Uses git.Status() to find files with StatusConflict ('U'),
// then parses each file for conflict markers.
// Also retrieves base content via `git show :1:<path>` (merge base stage).
func ParseConflictFiles(ctx context.Context, client git.GitClient) ([]ConflictFile, error)
```

## Dependencies

| Package | Purpose | Status |
|---------|---------|--------|
| `github.com/github/go-copilot` | GitHub Copilot SDK (or equivalent) | To be confirmed |
| `github.com/anthropics/anthropic-sdk-go` | Anthropic Claude SDK | Available |
| (existing) `internal/git` | All git operations | Implemented |
| (existing) `internal/config` | TOML config with `AIConfig` struct | Partially implemented |

## Open Questions

1. **Copilot SDK availability**: The official Go SDK for GitHub Copilot may need to be confirmed. Alternative: use the Copilot REST API directly with `gh auth token` for auth.
2. **Token cost awareness**: Should grut show estimated token cost before operations? Useful for Claude (paid API), less relevant for Copilot (subscription).
3. **Offline mode**: Should grut cache previous AI suggestions for similar conflicts (local embeddings/hashing)?
