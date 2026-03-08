# AI Features — Consolidated Implementation Plan

> Combines: [AI-Git-Ops Spec](spec.md) + Chat Box Feature
> Source repo analysis: Go 1.24, Bubble Tea v2, Lipgloss v2, Cobra CLI
> Existing packages: `internal/ai/` (not yet created), `internal/git/`, `internal/tui/`, `internal/panels/`, `internal/layout/`, `internal/config/`, `internal/keymap/`

## Overview

Two complementary AI features built on a shared foundation:

1. **AI-Git-Ops** (automatic/contextual) — AI activates when you merge (conflict resolution), commit (message generation), view diffs (inline review), create PRs (description generation), rebase (squash suggestions), etc. No user interaction needed beyond approval.

2. **Chat Box** (user-initiated/conversational) — Always-visible footer where users type natural language ("stage all modified Go files", "explain the diff in auth.go") and the AI responds with tool calls executed against grut's git and file operations.

Both consume the same `internal/ai/` provider layer. No duplication.

---

## Phase 1: Shared AI Foundation (`internal/ai/`)

All types, interfaces, and implementations below are used by BOTH features.

### 1.1 Provider Interface (`internal/ai/provider.go`)

```go
type AIProvider interface {
    Name() string
    Available(ctx context.Context) (bool, error)
    Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
    CompleteStream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)
    Close() error
}

type CompletionRequest struct {
    Operation    string            // "conflict_resolve", "commit_message", "chat", etc.
    SystemPrompt string
    GitContext   GitContext        // Structured repo state (used by git-ops)
    Messages     []ChatMessage    // Multi-turn conversation (used by chat)
    UserPrompt   string
    Tools        []ToolDefinition // Function calling tools (used by chat)
    MaxTokens    int
    Temperature  float64
}

type CompletionResponse struct {
    Content      string
    ToolCalls    []ToolCall       // AI-requested function calls
    TokensUsed   TokenUsage
    FinishReason string           // "stop", "length", "tool_calls", "error"
    Metadata     map[string]string
}

type ChatMessage struct {
    Role      string          // "user", "assistant", "tool", "system"
    Content   string
    ToolCalls []ToolCall      // If role=assistant
    ToolID    string          // If role=tool
}

type ToolDefinition struct {
    Name        string
    Description string
    Parameters  map[string]any  // JSON Schema
}

type ToolCall struct {
    ID        string
    Name      string
    Arguments map[string]any
}

type TokenUsage struct {
    InputTokens  int
    OutputTokens int
}

type StreamChunk struct {
    Delta      string
    Done       bool
    ToolCalls  []ToolCall      // Tool calls may arrive in stream
    TokensUsed *TokenUsage     // Set on final chunk
    Err        error
}

type GitContext struct {
    RepoRoot      string
    CurrentBranch string
    TargetBranch  string
    Diffs         []git.FileDiff
    Conflicts     []ConflictFile
    Log           []git.Commit
    FileContents  map[string]string
    Status        []git.FileStatus
}

type ConflictFile struct {
    Path            string
    OursContent     string
    TheirsContent   string
    BaseContent     string
    ConflictMarkers []ConflictRegion
}

type ConflictRegion struct {
    StartLine int
    EndLine   int
    Ours      string
    Theirs    string
    Base      string
}
```

**Usage pattern:**
- Git-ops uses: `Operation` + `SystemPrompt` + `GitContext` + `UserPrompt` (no Tools, no Messages)
- Chat uses: `SystemPrompt` + `Messages` + `Tools` (minimal GitContext for system prompt context)

### 1.2 Provider Registry (`internal/ai/registry.go`)

```go
type Registry struct {
    mu        sync.RWMutex
    providers map[string]AIProvider
    primary   string
    fallback  string
}

func NewRegistry(cfg config.AIConfig) (*Registry, error)
func (r *Registry) Get(ctx context.Context) (AIProvider, error)      // Primary with fallback
func (r *Registry) GetByName(name string) (AIProvider, bool)
func (r *Registry) Close() error
```

### 1.3 Copilot Provider (`internal/ai/provider_copilot.go`)

- Auth: `GITHUB_TOKEN` env var → `gh auth token` → `~/.config/gh/hosts.yml`
- Endpoint: `https://api.githubcopilot.com/chat/completions` (OpenAI-compatible)
- Supports tool/function calling natively
- Streaming via SSE
- Model: configurable, default from SDK

### 1.4 Claude Provider (`internal/ai/provider_claude.go`)

- Auth: `ANTHROPIC_API_KEY` env var (never in config file)
- Uses `anthropic-sdk-go` SDK
- Model: configurable, default `claude-sonnet-4-20250514`
- Supports tool use natively
- Streaming via SSE

### 1.5 Context Builder (`internal/ai/context.go`)

Transforms grut's git data into `GitContext`, managing token budgets:

```go
type Builder struct {
    client    git.GitClient
    redactor  *Redactor
    maxTokens int
}

func NewBuilder(client git.GitClient, redactor *Redactor, maxTokens int) *Builder
func (b *Builder) ForConflict(ctx context.Context, files []string) (GitContext, error)
func (b *Builder) ForCommit(ctx context.Context) (GitContext, error)
func (b *Builder) ForReview(ctx context.Context, opts git.DiffOpts) (GitContext, error)
func (b *Builder) ForPR(ctx context.Context, targetBranch string) (GitContext, error)
func (b *Builder) ForRebase(ctx context.Context, onto string) (GitContext, error)
func (b *Builder) ForBisect(ctx context.Context, good, bad string) (GitContext, error)
func (b *Builder) ForChangelog(ctx context.Context, fromRef, toRef string) (GitContext, error)
func (b *Builder) ForSplit(ctx context.Context, commitHash string) (GitContext, error)
func (b *Builder) ForChat(ctx context.Context) (GitContext, error)  // Lightweight: branch + status only
```

Token budget priority: conflict regions > surrounding context > file contents > commit history.

### 1.6 Content Redaction (`internal/ai/redact.go`)

```go
type Redactor struct {
    patterns      []glob.Glob
    secretRegexps []*regexp.Regexp
}

func NewRedactor(cfg config.AIConfig) *Redactor
func (r *Redactor) ShouldExcludeFile(path string) bool
func (r *Redactor) RedactContent(content string) (string, int)
```

Default patterns always active: AWS keys, API keys, PEM blocks, connection strings, `.env` files.

### 1.7 Audit Logger (`internal/ai/audit.go`)

```go
type AuditLogger struct {
    path string
    mu   sync.Mutex
}

type AuditEntry struct {
    Timestamp  time.Time
    Operation  string
    Provider   string
    FilesSent  []string
    Redactions int
    TokensIn   int
    TokensOut  int
    Result     string    // "accepted", "rejected", "modified", "error"
    Error      string
}

func NewAuditLogger(path string) *AuditLogger
func (a *AuditLogger) Log(entry AuditEntry) error
```

Writes to `~/.config/grut/ai-audit.log` with rotation.

### 1.8 AI Config (`internal/config/config.go` extension)

```toml
[ai]
enabled = true
provider = "copilot"                    # "copilot" | "claude" | "none"
fallback_provider = "claude"
redact_patterns = [".env", "*.key", "*.pem", "*.secret"]
auto_commit_message = true
auto_review_diff = false
temperature = 0.2
max_context_files = 20
max_context_tokens = 100000

[ai.copilot]
# model = "gpt-4o"                     # Optional override

[ai.claude]
model = "claude-sonnet-4-20250514"
max_tokens = 8192

[ai.review]
severity_threshold = "warning"
auto_review_on_push = false
categories = ["security", "bug", "performance", "style", "test"]

[ai.conflict]
default_mode = "interactive"            # "auto" | "interactive"
include_surrounding_context = 50
auto_accept_high_confidence = false

[ai.changelog]
format = "keepachangelog"
categories = ["added", "changed", "fixed", "removed", "security", "deprecated"]

[ai.commit_split]
threshold = 10

[ai.chat]
enabled = true
collapsed_height = 3
expanded_height = 8
system_prompt = ""                      # Custom override (empty = default)
```

Validation: reject embedded API keys, temperature in [0,1], max_context_files >= 1.

---

## Phase 2: AI-Git-Ops — Structured Operations (`internal/ai/ops/`)

Each operation uses `CompletionRequest` with `GitContext` + operation-specific system prompt. AI returns structured JSON parsed into typed results.

### 2.1 Conflict Resolution (`internal/ai/ops/conflict.go`)

- Parse conflict markers from files → `ConflictFile` structs
- Build `GitContext` with branch histories + surrounding context
- Send with conflict-specific system prompt (JSON output format)
- Return `ConflictResolution` (per-region resolutions with explanations + confidence scores)
- Two modes: auto-resolve-then-review vs. interactive per-conflict

**Response types:**
```go
type ConflictResolution struct {
    File         string
    Regions      []ResolvedRegion
    FullResolved string
}
type ResolvedRegion struct {
    OriginalRegion ConflictRegion
    Resolution     string
    Explanation    string
    Confidence     float64
}
```

### 2.2 Commit Message Generation (`internal/ai/ops/commit.go`)

- Get staged diff + recent commits (style matching) + branch name
- Generate conventional commit message matching repo's existing patterns
- Return `CommitSuggestion` (subject, body, type, scope)

### 2.3 Code Review (`internal/ai/ops/review.go`)

- Take diff + affected file contents + recent commits
- Return `[]ReviewFinding` (file, line, severity, category, message, suggestion)
- Categories: security, bug, performance, style, test

### 2.4 PR Description (`internal/ai/ops/pr.go`)

- Diff current branch vs target + all branch commits
- Return `PRDescription` (title, summary, changes, testing notes, breaking changes, full markdown)

### 2.5 Rebase Assistance (`internal/ai/ops/rebase.go`)

- Analyze commits to rebase + their diffs
- Return `RebaseSuggestion` (per-commit: pick/squash/fixup/reword/drop with reasoning)

### 2.6 Branch Cleanup (`internal/ai/ops/branch.go`)

- All branches + merge status + staleness
- Return `[]BranchRecommendation` (keep/delete/archive/rename with reasoning)

### 2.7 Bisect Analysis (`internal/ai/ops/bisect.go`)

- Diff between good/bad + candidate commits
- Return analysis with probability estimates for most likely culprit

### 2.8 Changelog Generation (`internal/ai/ops/changelog.go`)

- Commits between refs → categorized changelog
- Return `[]ChangelogEntry` (category, description, commit hashes)
- Keep a Changelog format

### 2.9 Commit Splitting (`internal/ai/ops/split.go`)

- Large commit diff → logical groupings
- Return `SplitPlan` (pieces with file assignments + commit messages)

### 2.10 AIGitClient Middleware (`internal/ai/middleware.go`)

Wraps `git.Client` to intercept operations with AI:

```go
type AIGitClient struct {
    inner    git.GitClient
    registry *Registry
    builder  *Builder
    audit    *AuditLogger
    cfg      config.AIConfig
}
```

- `Merge()` → detects conflicts → auto-triggers AI resolution
- `Commit()` → AI pre-generates message
- `Diff()` → AI review annotations available
- `Push()` → optional AI review gate
- All other methods delegate directly to `inner`
- No changes to `internal/git/` — purely additive wrapper

---

## Phase 3: Chat Box (`internal/chat/`)

### 3.1 Architecture

Always-visible footer component, NOT a panel in the layout engine split tree:

```
┌─────────────────────────────────────────────────┐
│ Tab Bar (1 line)                                │
├─────────────────────────────────────────────────┤
│ Panel Area (variable)                           │
├─────────────────────────────────────────────────┤
│ Chat Footer (3-8 lines)                         │
│  [response area]    > input line                │
├─────────────────────────────────────────────────┤
│ Hints Bar (1 line) │ Status Bar (1 line)        │
└─────────────────────────────────────────────────┘
```

### 3.2 Chat Model (`internal/chat/chat.go`)

```go
type Model struct {
    input        textinput.Model
    messages     []ai.ChatMessage
    scrollOffset int
    streaming    bool
    streamBuf    strings.Builder
    registry     *ai.Registry
    tools        *ToolRegistry
    executor     *ToolExecutor
    confirming   *PendingConfirmation
    redactor     *ai.Redactor
    audit        *ai.AuditLogger
    focused      bool
    width, height int
    expanded     bool
    theme        *theme.Theme
    ctx          context.Context
}
```

### 3.3 Tool Registry (`internal/chat/tools.go`)

30+ tools the AI can call, classified as SAFE or DESTRUCTIVE:

**File Operations:**
- `file_read(path)` [SAFE], `file_write(path, content)` [DESTRUCTIVE], `file_delete(path)` [DESTRUCTIVE]
- `file_rename(old_path, new_path)` [DESTRUCTIVE], `file_list(path, recursive)` [SAFE], `file_mkdir(path)` [SAFE]

**Git Read** (all SAFE):
- `git_status`, `git_diff`, `git_log`, `git_blame`, `git_branch_list`, `git_stash_list`, `git_worktree_list`

**Git Write:**
- `git_stage` [SAFE], `git_unstage` [SAFE], `git_commit` [SAFE], `git_push` [DESTRUCTIVE if force]
- `git_pull` [SAFE], `git_fetch` [SAFE], `git_checkout` [SAFE], `git_branch_create` [SAFE]
- `git_branch_delete` [DESTRUCTIVE], `git_merge` [SAFE], `git_rebase` [DESTRUCTIVE]
- `git_stash_push` [SAFE], `git_stash_pop` [SAFE], `git_reset` [DESTRUCTIVE]
- `git_tag_create` [SAFE], `git_tag_delete` [DESTRUCTIVE], `git_discard` [DESTRUCTIVE]

**Navigation & Search** (all SAFE):
- `navigate_to`, `search_files`, `search_content`, `explain`

**Bulk Operations:**
- `bulk_stage` [SAFE], `bulk_delete` [DESTRUCTIVE], `bulk_rename` [DESTRUCTIVE]

### 3.4 Tool Executor (`internal/chat/executor.go`)

- Maps `ToolCall` → `git.GitClient` methods and `os` file operations
- All paths validated through path sandboxing (restricted to repo root)
- Rate limiting for tool execution
- Returns structured string results for AI consumption

### 3.5 Smart Confirmation (`internal/chat/confirm.go`)

- SAFE ops: execute immediately, return result to AI
- DESTRUCTIVE ops: render `⚠ Delete "foo.txt"? [y/n]`, wait for user input
- User `y` → execute; `n` → return "user declined" to AI

### 3.6 System Prompt (`internal/chat/system.go`)

Context-aware, rebuilt on each message:
```
You are grut's assistant. Tools: file ops, git ops, navigation, search.
Current dir: {cwd} | Branch: {branch} ({clean|dirty})
Status: {abbreviated}
Rules: Use tools to act. Explain before destructive ops. Be concise.
```

### 3.7 View Rendering (`internal/chat/view.go`)

- **Collapsed** (3 lines): separator + latest response (truncated) + `💬 > ` input
- **Expanded** (8 lines): separator + 6 scrollable response lines + input
- Streaming indicator: `⟳` while tokens arrive
- Confirmation mode: `⚠ Delete "foo.txt"? [y/n]` replaces input

### 3.8 Message Flow

```
User: "stage all modified files"
  → Build ChatRequest (messages + tools + system prompt)
  → Provider.CompleteStream → AI returns tool_call: git_status()
  → Execute (SAFE) → return file list to AI
  → Provider again → AI returns tool_call: git_stage(paths)
  → Execute (SAFE) → return result to AI
  → Provider again → AI responds: "Staged 2 files"
  → Display response + emit ChatRefreshMsg → panels refresh
```

---

## Phase 4: TUI Integration

### 4.1 App Model Changes (`internal/tui/app.go`)

- Add `chat *chat.Model` and `aiRegistry *ai.Registry` fields
- `View()`: `lipgloss.JoinVertical(tabBar, panelArea, chatFooter, hintsBar, statusBar)`
- `WindowSizeMsg`: `m.engine.SetSize(msg.Width, msg.Height - 1 - m.chat.Height())`
- When chat focused, route key events to `m.chat.Update(msg)` instead of layout engine
- Construct chat with: registry, gitClient, redactor, audit, theme

### 4.2 AI-Git-Ops TUI Panels

- **Conflict panel** (`internal/panels/aiconflict/`): Three-way diff (base/ours/theirs) + AI suggestion. Auto-activates on merge/rebase conflicts. Keys: `a` accept AI, `o` ours, `t` theirs, `e` edit, `n` next.
- **Diff panel enhancement**: Inline AI review annotations (severity badges) when AI enabled. Toggle keybinding.
- **Commit input enhancement**: AI-generated message pre-fills when commit input opens. `Tab` accept, type to replace, `Esc` clear.
- **Branch panel enhancement**: Stale/merged/abandoned annotations with AI reasoning.
- **Rebase editor enhancement**: Ghost annotations (squash/fixup suggestions) next to each commit.

### 4.3 Keybindings

Chat-specific (new):
- `ctrl+/` — Toggle chat focus (global)
- `Enter` — Send message (chat focused)
- `Escape` — Unfocus chat
- `ctrl+e` — Toggle expanded/collapsed
- `ctrl+l` — Clear chat history
- `Up/Down` — Scroll response history
- `y`/`n` — Confirm/deny destructive op

AI-git-ops (existing panel enhancements):
- Conflict panel: `a`/`o`/`t`/`e`/`n`
- Diff review toggle
- Commit AI accept: `Tab`

### 4.4 Inter-Panel Messages (`internal/panels/messages.go`)

New messages:
```go
type ChatFocusMsg struct{}
type ChatNavigateMsg struct{ Path string }
type ChatRefreshMsg struct{}
type AIConflictResolvedMsg struct{ Path string }
type AIReviewReadyMsg struct{ Findings []ai.ReviewFinding }
type AICommitSuggestionMsg struct{ Suggestion ai.CommitSuggestion }
```

---

## Phase 5: CLI Integration

AI enhances existing CLI commands (no `grut ai` subcommand group):

| Command | AI Enhancement |
|---------|---------------|
| `grut merge <branch>` | Conflicts → AI resolution flow |
| `grut rebase <onto>` | Conflicts → AI resolution; interactive → AI suggestions |
| `grut commit` | AI pre-fills commit message |
| `grut diff --review` | Diff with AI annotations |
| `grut push` | Optional AI review gate (`auto_review_on_push`) |
| `grut branch --cleanup` | Branch list with AI cleanup recommendations |
| `grut log <range> --changelog` | AI-generated changelog |
| `grut bisect start` | AI-enhanced bisect with code analysis |
| `grut show <hash> --split` | AI commit splitting suggestions |

Global flag: `--no-ai` disables AI for any single operation.

---

## Phase 6: Security

- **Redaction**: All content passes through `Redactor` before reaching any provider
- **PathJail**: All file paths sandboxed to repo root
- **Auth**: Copilot via gh CLI token, Claude via env var — never in config files
- **Config validation**: Rejects embedded API keys with clear error
- **Audit**: Every AI operation logged with files sent, tokens used, user decision
- **Rate limiting**: Tool execution rate-limited
- **Confirmation**: Destructive chat ops require explicit Y/N
- **Graceful degradation**: No provider → silent fallback to non-AI behavior, subtle toast only
- **Chat history**: Not persisted to disk

---

## Implementation Tasks (Ordered by Dependency)

### Phase 1: Shared Foundation

| # | Task | Package | Description |
|---|------|---------|-------------|
| 1 | Provider interface + types | `internal/ai/provider.go` | AIProvider interface, CompletionRequest/Response (with Messages+Tools+GitContext), ChatMessage, ToolDefinition, ToolCall, TokenUsage, StreamChunk, ConflictFile, ConflictRegion, GitContext |
| 2 | Provider registry | `internal/ai/registry.go` | Registry with primary/fallback resolution, Register/Get/GetByName/Close |
| 3 | Copilot provider | `internal/ai/provider_copilot.go` | CopilotProvider: auth (GITHUB_TOKEN→gh auth token→gh config), OpenAI-compatible API, tool calling, SSE streaming |
| 4 | Claude provider | `internal/ai/provider_claude.go` | ClaudeProvider: anthropic-sdk-go, ANTHROPIC_API_KEY auth, tool use, SSE streaming |
| 5 | Content redaction | `internal/ai/redact.go` | Redactor: file glob exclusion + content regex (AWS keys, API keys, PEM, connection strings) |
| 6 | Audit logging | `internal/ai/audit.go` | AuditLogger: structured entries, file rotation, thread-safe |
| 7 | Context builder | `internal/ai/context.go` | Builder: ForConflict/ForCommit/ForReview/ForPR/ForRebase/ForBisect/ForChangelog/ForSplit/ForChat. Token budget management |
| 8 | AI config | `internal/config/config.go` | Expand AIConfig struct + all sub-configs. Add [ai.*] sections to defaults.toml. Validation rules |

### Phase 2: AI-Git-Ops Operations

| # | Task | Package | Description |
|---|------|---------|-------------|
| 9 | Conflict resolution | `internal/ai/ops/conflict.go` | Parse conflict markers, build context, resolve via AI, return ConflictResolution. Auto + interactive modes |
| 10 | Commit message gen | `internal/ai/ops/commit.go` | Staged diff + style matching → CommitSuggestion (subject, body, type, scope) |
| 11 | Code review | `internal/ai/ops/review.go` | Diff → []ReviewFinding (file, line, severity, category, message, suggestion) |
| 12 | PR description | `internal/ai/ops/pr.go` | Branch diff + commits → PRDescription (title, summary, changes, testing, breaking, markdown) |
| 13 | Rebase assistance | `internal/ai/ops/rebase.go` | Commits → RebaseSuggestion (pick/squash/fixup/reword/drop per commit) |
| 14 | Branch cleanup | `internal/ai/ops/branch.go` | All branches → []BranchRecommendation (keep/delete/archive/rename) |
| 15 | Bisect analysis | `internal/ai/ops/bisect.go` | Good/bad range → probability analysis of culprit commits |
| 16 | Changelog gen | `internal/ai/ops/changelog.go` | Commit range → []ChangelogEntry (categorized, Keep a Changelog format) |
| 17 | Commit splitting | `internal/ai/ops/split.go` | Large commit → SplitPlan (pieces with files + messages) |
| 18 | AIGitClient middleware | `internal/ai/middleware.go` | Wrap git.Client: intercept Merge/Rebase (conflicts), Commit (pre-fill), Diff (review), Push (gate) |

### Phase 3: Chat Box

| # | Task | Package | Description |
|---|------|---------|-------------|
| 19 | Tool registry | `internal/chat/tools.go` | 30+ ToolDefinition structs with JSON Schema params. Safe/Destructive classification |
| 20 | Tool executor | `internal/chat/executor.go` | Map ToolCall → git.GitClient/os ops. PathJail validation. Rate limiting |
| 21 | Confirmation system | `internal/chat/confirm.go` | PendingConfirmation state, Y/N handling, classification logic |
| 22 | Chat model | `internal/chat/chat.go` | Model struct, Init/Update, message history, streaming state machine, multi-turn tool calling loop |
| 23 | Chat view | `internal/chat/view.go` | Collapsed/expanded rendering, separator, scrollback, input, streaming indicator, confirmation |
| 24 | System prompt | `internal/chat/system.go` | Context-aware prompt builder (cwd, branch, status) |

### Phase 4: TUI Integration

| # | Task | Package | Description |
|---|------|---------|-------------|
| 25 | Chat footer in app | `internal/tui/app.go` | Add chat field, modify View() for footer, WindowSizeMsg height, construct with registry+gitClient |
| 26 | Chat focus/routing | `internal/tui/app.go` + `internal/keymap/` | ctrl+/ toggle, Escape unfocus, key routing when focused |
| 27 | Inter-panel messages | `internal/panels/messages.go` | ChatFocusMsg, ChatNavigateMsg, ChatRefreshMsg, AI-related msgs |
| 28 | Conflict panel | `internal/panels/aiconflict/` | Three-way diff + AI suggestion. Auto-activates on conflicts. Accept/ours/theirs/edit/next |
| 29 | Diff review annotations | `internal/panels/gitdiff/` | Inline AI review severity badges. Toggle keybinding |
| 30 | Commit message pre-fill | `internal/tui/` (commit input) | AI suggestion pre-fills on commit. Tab accept, type replace, Esc clear |
| 31 | Branch annotations | `internal/panels/branches/` | Stale/merged/abandoned AI indicators |

### Phase 5: CLI Integration

| # | Task | Package | Description |
|---|------|---------|-------------|
| 32 | CLI AI hooks | `cmd/` | merge/rebase auto-trigger AI on conflicts, commit pre-fills, diff --review, branch --cleanup, log --changelog, push review gate. Global --no-ai flag |

### Phase 6: Testing

| # | Task | Description |
|---|------|-------------|
| 33 | Provider tests | Mock HTTP for Copilot/Claude, SSE parsing, auth resolution, redaction, audit |
| 34 | Ops tests | Unit tests per operation with mock AIProvider, golden file regression |
| 35 | Chat tests | Tool executor (all mappings, classification, PathJail), chat model (flow, streaming, confirmation) |
| 36 | Integration tests | E2E: conflict resolution flow, chat message → tool call → execution, CLI AI hooks |
| 37 | Security tests | Redaction (no secrets leak), config validation (no embedded keys), PathJail enforcement |

---

## Dependency Graph

```
Phase 1 (foundation):
  1 (provider interface) ──┬── 2 (registry)
                           ├── 3 (copilot) ──────────────────┐
                           ├── 4 (claude) ───────────────────┤
                           ├── 5 (redaction)                 │
                           ├── 6 (audit)                     │
                           ├── 7 (context builder)           │
                           └── 8 (config)                    │
                                                             │
Phase 2 (git-ops):                                           │
  7+5 ── 9-17 (all ops, parallelizable) ── 18 (middleware)   │
                                                             │
Phase 3 (chat):                                              │
  1 ── 19 (tools) ──┬── 20 (executor)                       │
                    └── 21 (confirm) ──┬── 22 (model) ◄──────┘
                                       ├── 23 (view)
                                       └── 24 (system prompt)

Phase 4 (TUI):
  22+18 ── 25 (app integration) ── 26 (focus) + 27 (messages)
  9 ── 28 (conflict panel)
  11 ── 29 (diff annotations)
  10 ── 30 (commit pre-fill)
  14 ── 31 (branch annotations)

Phase 5 (CLI):
  18 ── 32 (CLI hooks)

Phase 6 (testing):
  All above ── 33-37
```

**Parallelism opportunities:**
- Tasks 3, 4, 5, 6, 7, 8 can all start after task 1
- Tasks 9-17 can all run in parallel after 7+5
- Tasks 19, 24 can start after task 1
- Phase 4 tasks 28-31 are independent of each other
