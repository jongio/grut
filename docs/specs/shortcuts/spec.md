# Shortcuts: AI-Powered Git Workflow Shortcuts

## Overview

Shortcuts let users define concise aliases for multi-step git workflows. Because grut is AI-native, every shortcut benefits from AI — conflict resolution during rebase, intelligent commit messages during commit, smart prompts when a shortcut is ambiguous, and natural-language custom shortcut definitions.

Users configure shortcuts in `config.toml` under a `[shortcuts]` section. Grut ships with a set of built-in shortcuts covering the most common git workflows. Users can override built-ins, disable them, or define entirely new shortcuts using plain English descriptions.

## Goals

- Reduce repetitive multi-step git workflows to single commands
- Leverage grut's AI subsystem for every step (commit messages, conflict resolution, rebase suggestions)
- Let users define custom shortcuts in plain English (AI interprets and builds the execution plan)
- Provide smart prompts when a shortcut needs clarification (e.g., which remote, which branch)
- Work in both TUI mode (via command palette / keybinding) and CLI mode (`grut run <shortcut>`)

## Non-Goals

- Shell alias management (this is grut-specific, not `.bashrc` aliases)
- Arbitrary shell command execution (shortcuts compose grut/git operations only)
- Replacing the full TUI git workflow (shortcuts are accelerators, not replacements)

## Architecture

### Execution Model

```
User invokes shortcut (TUI or CLI)
  → ShortcutEngine resolves shortcut definition
  → AI interprets any natural-language steps or ambiguity
  → Engine builds an execution plan (ordered list of git operations)
  → User sees the plan and confirms (or auto-executes if configured)
  → Engine executes each step through AIGitClient (inheriting AI middleware)
  → If any step fails: AI attempts resolution, or stops and reports
```

### Integration Points

| Component | Role |
|-----------|------|
| `internal/config/config.go` | New `ShortcutsConfig` section in Config struct |
| `internal/shortcuts/` | New package: engine, parser, built-in definitions |
| `internal/ai/middleware/` | Shortcuts execute through AIGitClient (conflict resolution, commit msgs) |
| `internal/ai/ops/` | New `shortcut.go` op for AI interpretation of natural-language shortcuts |
| `cmd/root.go` | New `grut run <shortcut> [args...]` subcommand |
| `internal/panels/` | Command palette integration for TUI shortcut invocation |
| `internal/chat/tools.go` | New `shortcut_run` tool for MCP/chat |

### Config Schema

```toml
[shortcuts]
# Enable/disable the shortcuts system
enabled = true

# When true, execute shortcuts without showing the plan first
auto_execute = false

# When true, AI interprets ambiguous shortcuts interactively
interactive_prompts = true

# Built-in shortcuts (can be overridden or disabled)
# Set to false to disable a built-in
[shortcuts.overrides]
# sc = false  # disable the built-in "sc" shortcut

# Custom shortcut definitions
[[shortcuts.custom]]
name = "deploy"
description = "Run tests, stage everything, commit with AI message, push to origin, create PR"
# Optional: explicit steps override AI interpretation
# steps = ["test", "stage_all", "commit_ai", "push origin", "pr_create"]

[[shortcuts.custom]]
name = "morning"
description = "Fetch all remotes, rebase my branch on upstream/main, show status"

[[shortcuts.custom]]
name = "oops"
description = "Undo the last commit but keep changes staged"
```

### Shortcut Definition Model

```go
type Shortcut struct {
    Name        string   // Trigger name (e.g., "sc", "rb", "deploy")
    Description string   // Human-readable purpose
    Steps       []Step   // Ordered operations to execute
    Args        []Arg    // Named arguments the shortcut accepts
    Builtin     bool     // True for shipped defaults
    Confirm     bool     // Require confirmation before executing (default: true)
}

type Step struct {
    Op       string            // Operation key (e.g., "stage_all", "commit", "fetch")
    Params   map[string]string // Parameters (e.g., {"remote": "upstream", "branch": "main"})
    OnFail   string            // "stop" | "continue" | "ask" (default: "stop")
    AIAssist bool              // Whether AI should assist this step (default: true)
}

type Arg struct {
    Name     string // Argument name (e.g., "remote", "branch")
    Default  string // Default value if not provided
    Prompt   string // Question to ask the user if not provided
    Required bool   // Whether the argument is mandatory
}
```

## Built-in Shortcuts

### Quick Operations

| Shortcut | Description | Steps | AI Role |
|----------|------------|-------|---------|
| `sc` | Stage & commit | `stage_all` → `commit` | Generate commit message |
| `scp` | Stage, commit & push | `stage_all` → `commit` → `push` | Generate commit message |
| `amend` | Stage & amend last commit | `stage_all` → `commit --amend` | Update commit message |
| `wip` | Save work-in-progress | `stage_all` → `commit "WIP: <branch>"` | Auto-prefix with branch context |
| `save` | Stash with auto-name | `stash push -m "<ai-name>"` | Generate descriptive stash name |
| `undo` | Undo last commit (keep changes) | `reset --soft HEAD~1` | None |
| `unstage` | Unstage all files | `reset HEAD` | None |
| `fixup` | Stage & fixup last commit | `stage_all` → `commit --fixup HEAD` | None |

### Sync Operations

| Shortcut | Description | Steps | AI Role |
|----------|------------|-------|---------|
| `rb [remote] [branch]` | Fetch & rebase | `fetch <remote>` → `rebase <remote>/<branch>` | Conflict resolution, rebase suggestions |
| `sync` | Sync with upstream main | `fetch --all` → `rebase upstream/main` | Conflict resolution |
| `pull` | Smart pull with rebase | `fetch origin` → `rebase origin/<current>` | Conflict resolution |
| `up` | Update branch from default | `fetch origin` → `rebase origin/<default>` | Conflict resolution |
| `fresh` | Fresh start on current branch | `stash` → `checkout <default>` → `pull` → `checkout -` → `stash pop` | Conflict resolution on stash pop |

### Branch Operations

| Shortcut | Description | Steps | AI Role |
|----------|------------|-------|---------|
| `nb <name>` | New branch from default | `fetch origin` → `checkout -b <name> origin/<default>` | Suggest branch name if not provided |
| `done` | Merge current branch to default | `checkout <default>` → `merge <prev-branch>` → `branch -d <prev-branch>` | Conflict resolution |
| `cleanup` | Delete merged local branches | Identify merged → `branch -d` each | Confirm which to delete |
| `rename <new>` | Rename current branch | `branch -m <new>` → `push origin :<old> <new>` | Suggest name if ambiguous |

### Review & Ship

| Shortcut | Description | Steps | AI Role |
|----------|------------|-------|---------|
| `review` | AI review of staged changes | `diff --staged` → AI review | Full code review |
| `pr` | Push & create PR | `push -u origin` → create PR | Generate PR title & description |
| `ship` | Full ship workflow | `review` → `sc` → `push` | Review + commit message + push |
| `squash [n]` | Squash last N commits | `rebase -i HEAD~N` | Suggest squash plan |

### Cleanup Operations

| Shortcut | Description | Steps | AI Role |
|----------|------------|-------|---------|
| `tidy` | Prune stale remotes & branches | `remote prune origin` → delete merged local branches | Confirm deletions |
| `nuke` | Hard reset to remote | `fetch origin` → `reset --hard origin/<current>` | Warn about data loss |
| `discard [path]` | Discard changes to file(s) | `checkout -- <path>` or `checkout -- .` | Warn about data loss |

## Custom Shortcut Definition (Plain English)

Users define custom shortcuts with a `description` field in plain English. Grut's AI interprets the description and builds an execution plan.

### How It Works

1. User adds a `[[shortcuts.custom]]` entry in `config.toml` with `name` and `description`
2. On first invocation, the AI parses the description into a `Step` sequence
3. The AI caches the parsed plan alongside the shortcut definition
4. User is shown the plan and asked to confirm (first time only, unless config changes)
5. Subsequent invocations use the cached plan directly

### AI Interpretation Prompt

```
You are a git workflow assistant. The user has defined a custom shortcut.
Convert their plain-English description into an ordered list of git operations.

Available operations:
- stage_all, stage <path>, unstage, unstage_all
- commit (AI message), commit_msg "<msg>", commit_amend
- fetch <remote>, fetch_all
- pull <remote> <branch>, push <remote> <branch>
- rebase <remote>/<branch>, merge <branch>
- checkout <branch>, checkout_new <branch> [from]
- branch_delete <branch>, branch_rename <old> <new>
- stash_push [msg], stash_pop, stash_apply
- reset_soft <ref>, reset_hard <ref>
- tag_create <name>, tag_delete <name>
- cherry_pick <ref>
- diff, diff_staged, status, log
- review (AI code review of staged changes)
- pr_create

For each step, specify:
- operation name
- parameters
- on_fail behavior (stop/continue/ask)
- whether AI should assist

If the description is ambiguous, list what questions to ask the user.
```

### Example Custom Shortcuts

```toml
[[shortcuts.custom]]
name = "hotfix"
description = "Create a hotfix branch from main, cherry-pick the last commit from current branch, push the hotfix branch"

[[shortcuts.custom]]
name = "rewind"
description = "Undo the last 3 commits but keep all the changes unstaged"

[[shortcuts.custom]]
name = "backup"
description = "Create a backup tag with today's date, then push the tag to origin"

[[shortcuts.custom]]
name = "contrib"
description = "Fork the repo if not forked, add upstream remote, fetch upstream, create a feature branch from upstream/main"

[[shortcuts.custom]]
name = "standup"
description = "Show me what I committed in the last 24 hours across all branches"
```

## CLI Interface

### New Subcommand: `grut run`

```
grut run <shortcut> [args...]       Execute a shortcut
grut run --list                     List all available shortcuts
grut run --list --filter <text>     Only show shortcuts matching this text
grut run --describe <shortcut>      Show what a shortcut will do
grut run --dry-run <shortcut>       Show execution plan without running
grut run --no-confirm <shortcut>    Skip confirmation prompt
```

### Examples

```bash
grut run sc                    # Stage all + AI commit
grut run rb upstream main      # Fetch upstream, rebase on upstream/main
grut run rb                    # Prompts: "Which remote?" "Which branch?"
grut run nb feature/login      # New branch from default
grut run squash 3              # Squash last 3 commits
grut run deploy                # Custom shortcut
grut run --dry-run ship        # Show what "ship" would do
```

## TUI Interface

### Command Palette

- Keybinding `:` or `Ctrl+P` opens command palette
- Type shortcut name to filter and execute
- Shows description and step preview before execution

### Shortcut Bar (optional)

- Configurable bar at bottom of TUI showing common shortcuts
- e.g., `[sc] commit  [rb] rebase  [sync] sync  [pr] PR`

## AI Prompting for Ambiguous Shortcuts

When a shortcut needs more info (e.g., `rb` without specifying remote/branch):

1. **Auto-detect defaults**: If there's only one remote, use it. If the branch tracks an upstream, use that.
2. **Smart prompts**: AI generates contextual questions based on the repo state
   - "You have 3 remotes (origin, upstream, fork). Which one do you want to rebase from?"
   - "Current branch `feature/auth` tracks `origin/feature/auth`. Rebase onto that, or onto `main`?"
3. **Remember choices**: After the user answers, grut can remember the choice for this shortcut+repo combo

## Error Handling & Safety

### Destructive Operation Warnings

Shortcuts that include destructive operations (`reset --hard`, `branch -d`, `push --force`) display a warning and require explicit confirmation, regardless of `auto_execute` setting.

### Step Failure Behavior

Each step has an `on_fail` policy:
- **stop** (default): Halt execution, show error, leave repo in current state
- **continue**: Log warning, proceed to next step
- **ask**: Prompt user for decision (retry, skip, abort)

### AI Failure Fallback

If AI fails during any step (provider down, token limit, etc.):
- The step falls back to non-AI behavior (e.g., commit without AI message prompts for manual message)
- Logged to audit trail
- Never blocks the workflow

### Undo Support

Every shortcut execution is recorded in grut's undo system. `grut run undo` (or the TUI undo keybinding) reverses the entire shortcut as a single unit.

## File Structure

```
internal/
  shortcuts/
    engine.go          # ShortcutEngine: resolve, plan, execute
    parser.go          # Parse TOML config into Shortcut structs
    builtins.go        # Built-in shortcut definitions
    ai_interpret.go    # AI interpretation of plain-English shortcuts
    plan.go            # ExecutionPlan: ordered steps with rollback
    types.go           # Shortcut, Step, Arg, ExecutionPlan types
    engine_test.go     # Engine tests
    parser_test.go     # Parser tests
    builtins_test.go   # Built-in definition tests
    ai_interpret_test.go # AI interpretation tests
cmd/
  run.go               # `grut run` CLI subcommand
internal/config/
  config.go            # Add ShortcutsConfig to Config struct
  defaults.toml        # Add [shortcuts] defaults
  validate.go          # Add shortcuts validation
```

## Open Questions

1. **Shortcut namespacing**: Should custom shortcuts be able to shadow built-ins? Current design: yes, with a warning.
2. **Shortcut sharing**: Should there be an import/export mechanism for shortcut definitions (e.g., `grut shortcuts export > my-shortcuts.toml`)?
3. **Shortcut chaining**: Should shortcuts be able to reference other shortcuts in their steps (e.g., `ship` uses `sc` internally)?
4. **Repo-local shortcuts**: Support `.grut/shortcuts.toml` in repo root for project-specific shortcuts?
5. **Execution history**: Should grut keep a log of shortcut executions for auditability?
