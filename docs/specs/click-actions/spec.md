# Configurable Click Actions — Specification

## Overview

Every clickable item in grut has default **single-click** and **double-click**
actions. Users can override any default through a first-use confirmation prompt
and the Settings panel. All preferences persist in `~/.config/grut/config.toml`.

---

## 1. Default Click Action Matrix

### 1.1 GitInfo Panel (7 tabs)

| Tab | Item Kind | Single Click | Double Click (Enter) | Other Keys |
|-----|-----------|-------------|---------------------|------------|
| Branches | `kindLocalBranch` | Select | Confirm → checkout | `n` create, `d` delete, `r` rename, `y` copy |
| Branches | `kindRemoteBranch` | Select | Confirm → checkout (local copy) | `d` delete, `y` copy |
| Worktrees | `kindWorktree` | Select | Switch / open terminal (config) | `n` create, `d` delete |
| Remotes | `kindRemote` | Select | Open in browser (SSH→HTTPS) | `n` add, `d` remove |
| Remotes | `kindRemoteSub` | Select | — (no action) | — |
| Stash | `kindStashEntry` | Select | Prompt: apply/pop/drop | `d` drop |
| Issues | `kindIssue` | Select | Open in browser | — |
| PRs | `kindPR` | Select | Open in browser | — |
| Actions | `kindActionRun` | Select | Open in browser | `r` rerun, `x` cancel |

### 1.2 Branches Panel (standalone)

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| Branch | Select | Checkout | `n` create, `d` delete, `r` rename, `m` merge |

### 1.3 Worktrees Panel (standalone)

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| Worktree | Select | Switch / open terminal | `n` create, `d` delete |

### 1.4 File Tree Panel

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| File | Select + preview | Open in `$EDITOR` | `d` delete, `R` rename, `c` copy path |
| Directory | Select + expand/collapse | Expand/collapse | `a` new file, `A` new dir |

### 1.5 Commits Panel

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| Commit | Select | Show detail view | `y` copy hash |

### 1.6 Git Status Panel

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| File entry | Select | Expand inline diff | `s` stage, `u` unstage, `h` hunk mode |

### 1.7 Git Log Panel

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| Commit | Select | Show detail view | `y` copy hash, `/` search |

### 1.8 Stash Panel (standalone)

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| Stash entry | Select | Apply stash | `p` pop, `d` drop, `s` push, `D` drop all |

### 1.9 Conflicts Panel

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| Conflicted file | Select | Open file diff | `o` ours, `t` theirs, `c` continue, `a` abort |

### 1.10 Review Panel

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| Changed file | Select | Enter diff mode | `n`/`N` next/prev file, `s` submit |

### 1.11 Context Panel

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| Context file | Select | Preview file | `d` remove, `c` clear all, `e` export |

### 1.12 Bookmarks Panel

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| Bookmark | Select | Jump to bookmark | `d` delete |

### 1.13 Extensions Panel

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| Extension | Select | Expand/collapse details | `e` enable/disable |

### 1.14 Agents Panel

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| Agent | Select | Expand/collapse output | — |

### 1.15 Settings Panel

| Item | Single Click | Double Click (Enter) | Other Keys |
|------|-------------|---------------------|------------|
| Setting | Select | Cycle value | — |

### 1.16 Other Panels (no clickable items)

| Panel | Notes |
|-------|-------|
| Git Diff | Content viewer — scroll only, no selectable items |
| Preview | Content viewer — scroll only |
| Help | Content viewer — scroll only |
| Fuzzy Finder | Input + result list: Enter selects top match |
| AI Conflict | Mode-specific; keyboard-driven conflict resolution |

---

## 2. Current Mouse Support Status

| Panel | Single Click | Double Click | Scroll | Status |
|-------|:---:|:---:|:---:|--------|
| GitInfo | ✓ | ✓ | ✓ | Complete |
| Branches | ✓ | ✓ | ✓ | Complete |
| Worktrees | ✓ | ✓ | ✓ | Complete |
| Commits | ✓ | ✗ | ✓ | Missing double-click |
| File Tree | ✓ | ✗ | ✗ | Missing double-click + scroll |
| Git Status | ✗ | ✗ | ✗ | No mouse at all |
| Git Log | ✗ | ✗ | ✗ | No mouse at all |
| Git Diff | ✗ | ✗ | ✗ | No mouse at all |
| Preview | ✗ | ✗ | ✓ | Scroll only |
| Stash | ✗ | ✗ | ✗ | No mouse at all |
| Bookmarks | ✗ | ✗ | ✗ | No mouse at all |
| Settings | ✗ | ✗ | ✗ | No mouse at all |
| Help | ✗ | ✗ | ✗ | No mouse at all |
| Conflicts | ✗ | ✗ | ✗ | No mouse at all |
| Review | ✗ | ✗ | ✗ | No mouse at all |
| Agents | ✗ | ✗ | ✗ | No mouse at all |
| Extensions | ✗ | ✗ | ✗ | No mouse at all |
| Context | ✗ | ✗ | ✗ | No mouse at all |
| Fuzzy Finder | ✗ | ✗ | ✗ | No mouse at all |
| AI Conflict | ✗ | ✗ | ✗ | No mouse at all |

---

## 3. Configurable Double-Click System

### 3.1 Design Goals

1. Every double-click action has a **sensible default** (as shown in section 1).
2. Users can **override** defaults per item type.
3. First time a user double-clicks, a **confirmation prompt** appears.
4. The prompt includes a **"Always perform this action"** option.
5. Overrides are **persisted** in the TOML config.
6. The **Settings panel** shows all double-click mappings and lets users change them.

### 3.2 Action Registry

Each clickable item type defines a set of **available actions** — one is the
default, and the rest are alternatives the user can switch to:

```
item_type          → default_action    | alternatives
─────────────────────────────────────────────────────
local_branch       → checkout          | copy_name, open_in_browser
remote_branch      → checkout          | copy_name
worktree           → switch            | open_terminal, copy_path
remote             → open_in_browser   | copy_url
stash_entry        → prompt_action     | apply, pop, drop
issue              → open_in_browser   | copy_url, copy_number
pr                 → open_in_browser   | copy_url, copy_number, checkout_branch
action_run         → open_in_browser   | rerun, copy_url
file (filetree)    → open_in_editor    | copy_path, stage, preview
directory          → expand_collapse   | copy_path
commit             → show_detail       | copy_hash, open_in_browser
status_file        → expand_diff       | stage_unstage, copy_path
log_commit         → show_detail       | copy_hash
stash (standalone) → apply             | pop, drop, show_detail
conflict_file      → open_diff         | resolve_ours, resolve_theirs
review_file        → expand_diff       | approve, copy_path
context_file       → preview           | remove, copy_path
bookmark           → jump              | delete, copy_path
extension          → toggle_details    | enable_disable
agent              → toggle_output     | —
setting            → cycle_value       | —
```

### 3.3 Config Schema

```toml
[actions]
# Per-item-type double-click overrides.
# Keys are item types, values are action names from the registry.
# Omitted keys use the built-in default.

# [actions.double_click]
# local_branch = "checkout"      # default
# remote_branch = "checkout"     # default
# worktree = "switch"            # default
# remote = "open_in_browser"     # default
# stash_entry = "prompt_action"  # default
# issue = "open_in_browser"      # default
# pr = "open_in_browser"         # default
# action_run = "open_in_browser" # default
# file = "open_in_editor"        # default
# directory = "expand_collapse"  # default
# commit = "show_detail"         # default
# status_file = "expand_diff"    # default
# log_commit = "show_detail"     # default
# stash = "apply"                # default
# conflict_file = "open_diff"    # default
# review_file = "expand_diff"    # default
# context_file = "preview"       # default
# bookmark = "jump"              # default
# extension = "toggle_details"   # default
# agent = "toggle_output"        # default
# setting = "cycle_value"        # default

[actions.confirmed]
# Tracks which item types the user has seen the first-use prompt for.
# When true, the prompt is skipped and the action runs immediately.
# local_branch = true
# issue = true
```

### 3.4 First-Use Confirmation Prompt

When a user double-clicks an item type for the first time (no entry in
`[actions.confirmed]`), grut shows a modal:

```
┌─────────────────────────────────────────┐
│         Double-Click Action             │
│                                         │
│  Do you want to checkout this branch?   │
│                                         │
│  ☐ Always perform this on double-click  │
│                                         │
│  You can change this in Settings (S).   │
│                                         │
│         [ Yes ]    [ No ]               │
└─────────────────────────────────────────┘
```

**Behavior:**
- **Yes** → Performs the action. If checkbox is checked, sets
  `[actions.confirmed.<item_type>] = true` in config.
- **No** → Cancels. Does nothing. Does NOT mark as confirmed.
- If "Always" is checked, subsequent double-clicks skip the prompt entirely.
- The prompt message dynamically fills in the action description (e.g.,
  "open this issue in your browser", "apply this stash entry").

### 3.5 New Modal Type: ConfirmWithCheckbox

The existing modal system supports `ModalConfirm` (yes/no) and `ModalInput`
(text entry). A new `ModalConfirmWithCheckbox` kind is needed:

```go
type ModalKind int

const (
    ModalConfirm             ModalKind = iota
    ModalInput
    ModalConfirmWithCheckbox // NEW
)

type ModalResultMsg struct {
    Accept   bool
    Value    string
    Remember bool   // NEW — true when checkbox was checked
}
```

Rendering adds a checkbox row between the message and buttons, toggled with
`Space`. The `Tab` key moves focus between the checkbox and the Yes/No buttons.

### 3.6 Settings Panel Enhancement

The Settings panel gains a new section: **Double-Click Actions**. Each item type
is listed with its current action, and the user can cycle through alternatives:

```
Settings
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Display
  ──────────────────────────────────
  Preview Position      ◂ Right ▸
  Theme                 ◂ Dracula ▸

  Double-Click Actions
  ──────────────────────────────────
  Branch                ◂ checkout ▸
  Worktree              ◂ switch ▸
  Remote                ◂ open in browser ▸
  Stash                 ◂ prompt action ▸
  Issue                 ◂ open in browser ▸
  PR                    ◂ open in browser ▸
  Action Run            ◂ open in browser ▸
  File                  ◂ open in editor ▸
  Commit                ◂ show detail ▸
  Status File           ◂ expand diff ▸
  Conflict File         ◂ open diff ▸
  Review File           ◂ expand diff ▸
  Context File          ◂ preview ▸
  Bookmark              ◂ jump ▸

  Confirmations
  ──────────────────────────────────
  Reset all prompts     [ Reset ]
```

Navigation: `j`/`k` to move between rows, `h`/`l` or `Enter` to cycle values,
`Esc` to close. Changes persist immediately via `config.SaveUserSetting()`.

---

## 4. Implementation Plan

### Phase 1: Action Registry & Config (foundation)
1. Define `ActionID` type and registry in `internal/actions/registry.go`
2. Add `[actions]` section to config structs and `defaults.toml`
3. Add `GetDoubleClickAction(itemType string) ActionID` helper
4. Add `IsConfirmed(itemType string) bool` helper
5. Add `SetConfirmed(itemType string)` persistence helper

### Phase 2: ConfirmWithCheckbox Modal
1. Add `ModalConfirmWithCheckbox` kind to `internal/notify/modal.go`
2. Add `Remember` field to `ModalResultMsg`
3. Add checkbox rendering and `Space` toggle
4. Add `ShowConfirmWithCheckbox(title, message, checkboxLabel)` constructor

### Phase 3: Wire Double-Click Through Registry
1. Update `doAction()` in each panel to look up the configured action
2. Insert first-use confirmation check before executing
3. On "Always" checkbox, persist confirmation flag
4. Start with gitinfo panel (most item types), then branches, worktrees, etc.

### Phase 4: Settings Panel Enhancement
1. Add double-click actions section to settings panel
2. Load current values from config
3. Support cycling through alternatives per item type
4. Add "Reset all prompts" option
5. Persist changes via `config.SaveUserSetting()`

### Phase 5: Add Missing Mouse Support
1. Add single-click + double-click to remaining panels (gitstatus, gitlog, commits, etc.)
2. Add scroll support where missing
3. Each panel gets `handleMouseClick` and `handleMouseDoubleClick` methods

---

## 5. Action Descriptions (for prompts)

These are the human-readable descriptions shown in the first-use prompt:

| Item Type | Action | Prompt Text |
|-----------|--------|-------------|
| local_branch | checkout | "switch to this branch" |
| remote_branch | checkout | "check out this remote branch locally" |
| worktree | switch | "switch to this worktree" |
| worktree | open_terminal | "open a terminal in this worktree" |
| remote | open_in_browser | "open this remote in your browser" |
| stash_entry | prompt_action | "choose a stash action (apply/pop/drop)" |
| issue | open_in_browser | "open this issue in your browser" |
| pr | open_in_browser | "open this pull request in your browser" |
| action_run | open_in_browser | "open this workflow run in your browser" |
| file | open_in_editor | "open this file in your editor" |
| commit | show_detail | "show details for this commit" |
| status_file | expand_diff | "expand the inline diff for this file" |
| conflict_file | open_diff | "open the diff for this conflicted file" |
| review_file | expand_diff | "expand the diff for this file" |
| bookmark | jump | "jump to this bookmark" |

---

## 6. Open Questions

1. **Single-click overrides?** — Current spec only makes double-click
   configurable. Single-click is always "select". Should single-click also
   be configurable? (Recommendation: no — selection is a fundamental UX
   primitive that should be consistent.)

2. **Per-panel vs per-item-type?** — Branches exist in both GitInfo and the
   standalone Branches panel. Should they share the same double-click config?
   (Recommendation: yes — same item type = same behavior everywhere.)

3. **Right-click context menus?** — Future enhancement. Would show all
   available actions for an item without needing to configure defaults.
   Out of scope for this spec.
