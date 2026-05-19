# Double-click file to open in default OS app

## Summary

Add an "open in default app" action so double-clicking a file in the filetree opens it with the OS-registered application for that file type, rather than always opening in a code editor.

## Description

Currently, double-clicking a file in the filetree triggers `ActionOpenInEditor`, which checks `$VISUAL`, `$EDITOR`, then tries VS Code/Cursor, and only falls back to the OS default launcher as a last resort. This means a `.pdf`, `.png`, or `.docx` always opens in a code editor if one is available -- not in the appropriate viewer/app.

Users expect double-click to behave like a file manager: open the file in whatever app the OS associates with that extension. The code editor should be an explicit alternative, not the default for all file types.

## Technical Details

**Current flow (filetree double-click on a file):**
1. `internal/panels/filetree/filetree.go:986` -- `handleMouseDoubleClick`
2. Reads configured action via `actionsCfg.GetDoubleClickAction("file")` (defaults to `ActionOpenInEditor`)
3. Dispatches to `executeRightClickAction` in `internal/panels/filetree/fileops.go:402`
4. `ActionOpenInEditor` calls `panels.OpenInEditor()` in `internal/panels/open.go:99`
5. `OpenInEditor` tries editors first, OS default last

**Proposed changes:**

1. Add `ActionOpenInDefaultApp` constant to `internal/actions/registry.go`
2. Add an `OpenInDefaultApp(ctx, path)` function to `internal/panels/open.go` that goes directly to platform launchers:
   - Windows: `cmd /c start "" <path>`
   - macOS: `open <path>`
   - Linux: `xdg-open <path>`
3. Change the default action for `ItemFile` in the registry from `ActionOpenInEditor` to `ActionOpenInDefaultApp`
4. Add `ActionOpenInEditor` to the alternatives list for `ItemFile` so users can still choose it
5. Handle `ActionOpenInDefaultApp` in `executeRightClickAction` in `fileops.go` (and equivalent handlers in other panels that use `ItemFile`)
6. Add the label "open in default app" to `actionLabels` map

**Affected files:**
- `internal/actions/registry.go` -- new ActionID, registry entry change
- `internal/panels/open.go` -- new `OpenInDefaultApp` function
- `internal/panels/filetree/fileops.go` -- handle new action in switch
- `internal/panels/gitstatus/gitstatus.go` -- if it handles file double-click
- `internal/panels/context/context.go` -- if it handles file double-click
- `internal/panels/review/review.go` -- if it handles file double-click

**Path validation:** Reuse existing `ValidateEditorPath` (it validates shell metacharacters) before passing to OS launcher.

## Acceptance Criteria

- [ ] Double-clicking a file in the filetree opens it in the OS default app for that file type
- [ ] "Open in editor" remains available as an alternative action (right-click menu or settings)
- [ ] Double-clicking a directory still expands/collapses it (no regression)
- [ ] Path validation prevents shell injection via malicious filenames
- [ ] Works on Windows, macOS, and Linux

## Quality Gates (MANDATORY)

- [ ] Zero lint/type errors
- [ ] All tests pass (existing + new tests for this feature)
- [ ] mq (max-quality) passes clean
- [ ] test-health grade >= B+
- [ ] idiomatic audit clean
- [ ] Architecture review: no violations
- [ ] Gut-check: no over-engineering or YAGNI violations

## Done Definition

This issue is DONE when ALL acceptance criteria AND quality gates pass.
"It compiles" is not done. "Tests pass" is not done. ALL gates must be green.

## Related

- `internal/actions/registry.go` -- action registry
- `internal/panels/open.go` -- file/browser/terminal opening utilities
- `internal/panels/filetree/fileops.go` -- filetree action dispatch
- `internal/config/actions.go` -- user action configuration persistence
