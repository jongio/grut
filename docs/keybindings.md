# Keybindings Reference

grut supports three keybinding schemes: **default**, **vim**, and **classic**. Set the scheme in `~/.config/grut/config.toml`:

```toml
[general]
keybinding_scheme = "default"  # "default", "vim", "classic", or path to custom .toml
```

The bindings below document the **default** scheme. All schemes share the same global and number-key bindings. This file is generated from `internal/keybindings/keybindings.json` — do not edit by hand.

---

## Global

Always active regardless of focused panel.

| Key | Action |
|-----|--------|
| `1-5` | Focus panel by number |
| `ctrl+b z` | Zoom focused panel |
| `R` | Refresh all data + preview |
| `P` | Push to remote |
| `F` | Fetch all remotes |
| `?` | Help overlay |
| `,` | Settings |
| `/` | Fuzzy finder |
| `:` | Command palette |
| `~` | Change directory |
| `ctrl+space` | Toggle AI chat |
| `ctrl+z` | Undo last git action |
| `ctrl+y` | Redo |
| `ctrl+c` | Quit |

---

## Navigation

Consistent in every focused panel.

| Key | Action |
|-----|--------|
| `j/k` | Cursor down/up |
| `g/G` | Jump to top/bottom |
| `PgDn/PgUp` | Page down/up |
| `Enter` | Select / open / expand |
| `Esc` | Back / close submode |
| `d` | Discard changes to selected file |
| `u` | Unstage selected file |

---

## File Tree

File Tree panel (panel 1).

| Key | Action |
|-----|--------|
| `h/l` | Collapse/expand directory |
| `H/L` | Collapse/expand all directories |
| `o` | Open in external editor |
| `.` | Toggle hidden files |
| `f` | Toggle git filter |
| `v` | Toggle tree/list view |
| `n` | New file |
| `N` | New directory |
| `x` | Delete file/directory |
| `e / F2` | Rename |
| `y` | Copy path to clipboard |
| `c` | Copy file |
| `p` | Paste file |
| `space` | Toggle stage/unstage |
| `I` | Add selected entry to .gitignore |
| `J/K` | Scroll preview down/up |

---

## Git Status

Active when the file tree is in git filter mode (press f to cycle).

| Key | Action |
|-----|--------|
| `s` | Stage file/hunk/line |
| `u` | Unstage file/hunk/line |
| `d` | Discard unstaged changes |
| `y` | Copy the current diff hunk (or file path) to the clipboard |
| `c` | Commit staged changes |
| `p` | Pull from remote |
| `a` | Stage all |
| `U` | Unstage all |
| `space` | Toggle select for bulk |
| `enter/l` | Expand file diff |
| `h` | Enter hunk mode |
| `X` | Clean untracked files (preview and select) |
| `Esc` | Exit hunk/line mode |

---

## Git Info

Git Info panel (panel 2).

| Key | Action |
|-----|--------|
| `Tab` | Next tab |
| `Shift+Tab` | Previous tab |
| `b` | Branches tab |
| `w` | Worktrees tab |
| `r` | Remotes tab |
| `s` | Stash tab |
| `t` | Tags tab |
| `l` | Reflog tab |
| `n` | Create new item |
| `x` | Delete selected |
| `e / F2` | Rename |
| `o` | Open in browser |
| `y` | Copy to clipboard |
| `f` | Fetch / filter |
| `P` | Push tag |

---

## GitHub

GitHub panel (panel 3).

| Key | Action |
|-----|--------|
| `Tab` | Next tab |
| `Shift+Tab` | Previous tab |
| `b` | Branches tab |
| `t` | Tags tab |
| `i` | Issues tab |
| `p` | Pull Requests tab |
| `a` | Actions tab |
| `w` | Workflows tab |
| `l` | Releases tab |
| `n` | Create issue (Issues tab) / Create pull request (PRs tab) |
| `N` | Notifications tab |
| `Enter` | Open notification in browser (Notifications tab) |
| `o` | Open in browser |
| `y` | Copy to clipboard |
| `m` | Merge PR (PRs tab) / Mark read (Notifications tab) |
| `M` | Copy issue or PR as a markdown link (Issues/PRs tabs) |
| `A` | Assign to me (Issues/PRs tab) |
| `R` | Request reviewers (PRs tab) |
| `C` | Comment on issue/PR (Issues/PRs tab) |
| `c` | Close/reopen issue (Issues tab) |
| `S` | Cycle state filter open/closed/all (Issues/PRs tab) |
| `r` | Rerun (Actions tab) / Refresh (Notifications tab) |
| `x` | Cancel (Actions tab) |
| `D` | Dispatch workflow |

---

## Commits

Commits panel (panel 4).

| Key | Action |
|-----|--------|
| `Enter` | View commit detail |
| `Esc` | Back to list |
| `o` | Open commit on GitHub |
| `y` | Copy SHA |
| `a` | Filter the commit log by the selected commit's author |
| `x` | Export the selected commit as a .patch file |
| `A` | Amend last commit |
| `r` | Reword last commit |
| `/` | Search commits |
| `S` | Search commit content (pickaxe) |
| `M` | Search commit messages (grep) |

---

## Preview

Preview panel (panel 5).

| Key | Action |
|-----|--------|
| `e` | Edit file inline |
| `f` | Toggle diff view |
| `j/k` | Scroll content |
| `g/G` | Jump to top/bottom |
| `L` | Go to line |
| `PgDn/PgUp` | Page down/up |
| `W` | Toggle word wrap |
| `n` | Toggle line numbers |
| `m` | Toggle markdown render |
| `B` | Toggle blame |
| `y/Ctrl+C` | Copy selection |
| `Y` | Copy GitHub permalink |
| `Esc` | Clear selection |

Navigation keys (j/k/g/G/PgDn/PgUp) scroll content. Click+drag to select text, double-click to select a word.

---

## Diff Viewer

Diff viewer panel for file, commit, branch, and PR diffs.

| Key | Action |
|-----|--------|
| `j/k` | Scroll up/down |
| `d/u` | Page down/up |
| `g/G` | Jump to top/bottom |
| `t` | Toggle inline/side-by-side |
| `n/N` | Next/previous hunk |
| `[/]` | Previous/next file |
| `w` | Toggle word-level diff |
| `W` | Toggle ignore whitespace |
| `y` | Copy the current hunk to the clipboard |
| `R` | Toggle review annotations |
| `+/-` | More/less diff context lines |
| `0` | Reset diff context to default |

---

## Edit Mode

Active when editing a file inline (press e in Preview to enter).

| Key | Action |
|-----|--------|
| `Ctrl+S` | Save file |
| `Ctrl+Z` | Undo |
| `Ctrl+Y` | Redo |
| `Ctrl+C` | Copy selection (or line) |
| `Ctrl+X` | Cut selection (or line) |
| `Ctrl+V` | Paste from clipboard |
| `Ctrl+A` | Select all |
| `Ctrl+D` | Duplicate line |
| `Ctrl+Shift+K` | Delete line |
| `Alt+Up/Down` | Move line up/down |
| `Ctrl+Left/Right` | Word navigation |
| `Ctrl+Backspace` | Delete word left |
| `Ctrl+Delete` | Delete word right |
| `Ctrl+Home/End` | Jump to file start/end |
| `Home/End` | Jump to line start/end |
| `Shift+Arrows` | Extend selection |
| `Shift+Home/End` | Select to line start/end |
| `Ctrl+Shift+Left/Right` | Select word |
| `Tab` | Indent |
| `Shift+Tab` | Dedent |
| `Esc` | Exit edit mode |

Mouse: click to position cursor, drag to select, double-click to select word. Paste also works via terminal bracketed paste (Ctrl+V in most terminals).

