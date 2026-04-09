# Keybindings Reference

grut supports three keybinding schemes: **default**, **vim**, and **classic**. Set the scheme in `~/.config/grut/config.toml`:

```toml
[general]
keybinding_scheme = "default"  # "default", "vim", "classic", or path to custom .toml
```

The bindings below document the **default** scheme. All schemes share the same global and number-key bindings. See `docs/keybindings.json` for the machine-readable source of truth.

---

## Global

Always active regardless of focused panel.

| Key | Action |
|-----|--------|
| `1` | Focus File Tree panel |
| `2` | Focus Git Info panel |
| `3` | Focus GitHub panel |
| `4` | Focus Commits panel |
| `5` | Focus Preview panel |
| `Tab` | Focus next panel |
| `Shift+Tab` | Focus previous panel |
| `R` | Refresh all data + preview |
| `P` | Push |
| `F` | Fetch all remotes |
| `?` | Help overlay |
| `,` | Settings |
| `/` | Fuzzy finder |
| `:` | Command palette |
| `~` | Change directory |
| `Ctrl+c` | Quit |
| `Ctrl+Space` | Toggle AI chat |
| `Ctrl+z` | Undo last git action |
| `Ctrl+y` | Redo |

---

## Navigation (all panels)

| Key | Action |
|-----|--------|
| `j` / `↓` | Cursor down |
| `k` / `↑` | Cursor up |
| `g` | Jump to top |
| `G` | Jump to bottom |
| `d` | Page down |
| `u` | Page up |
| `Enter` | Select / open / expand |
| `Esc` | Back / close submode |

---

## File Tree (panel 1)

| Key | Action |
|-----|--------|
| `h` / `←` | Collapse directory |
| `l` / `→` | Expand directory |
| `Enter` | Expand dir / select file |
| `o` | Open file in external editor |
| `.` | Toggle hidden files |
| `g` | Toggle git filter (changed files only) |
| `v` | Toggle tree / list view |
| `n` | New file |
| `N` | New directory |
| `x` | Delete file / directory |
| `e` / `F2` | Rename |
| `y` | Copy path to clipboard |
| `c` | Copy file |
| `p` | Paste file |
| `s` | Stage file (in git filter mode) |
| `Space` | Toggle stage / unstage |
| `J` | Scroll preview down |
| `K` | Scroll preview up |

---

## Git Info (panel 2)

### Tabs

| Key | Tab |
|-----|-----|
| `b` | Branches |
| `w` | Worktrees |
| `r` | Remotes |
| `s` | Stash |
| `t` | Tags |
| `l` | Reflog |

### Actions

| Key | Action |
|-----|--------|
| `Enter` | Checkout / apply |
| `n` | Create new item |
| `x` | Delete selected |
| `e` / `F2` | Rename |
| `o` | Open in browser |
| `y` | Copy to clipboard |
| `f` | Fetch / filter |

---

## GitHub (panel 3)

### Tabs

| Key | Tab |
|-----|-----|
| `b` | Branches |
| `t` | Tags |
| `i` | Issues |
| `p` | Pull Requests |
| `a` | Actions |
| `w` | Workflows |
| `l` | Releases |

### Actions

| Key | Action |
|-----|--------|
| `Enter` | Select (show in preview) |
| `o` | Open in browser |
| `y` | Copy to clipboard |
| `m` | Merge PR (PRs tab only) |
| `r` | Rerun (Actions tab only) |
| `x` | Cancel (Actions tab only) |
| `D` | Dispatch workflow (Workflows tab only) |

---

## Commits (panel 4)

| Key | Action |
|-----|--------|
| `Enter` | View commit detail |
| `Esc` | Back to list |
| `o` | Open in browser |
| `y` | Copy SHA |
| `A` | Amend last commit |
| `r` | Reword last commit |
| `/` | Search commits |

---

## Preview (panel 5)

| Key | Action |
|-----|--------|
| `j` / `k` | Scroll content |
| `g` / `G` | Jump to top / bottom |
| `d` / `u` | Page down / up |
| `y` / `Ctrl+C` | Copy selection to clipboard |
| `Esc` | Clear selection |

Click and drag to select text. Double-click to select a word.


