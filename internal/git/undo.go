package git

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// maxUndoDepth is the maximum number of actions retained in the undo stack.
// Older actions are discarded when the limit is exceeded.
const maxUndoDepth = 50

// UndoAction represents a recorded git operation that can be undone or redone.
// The Type field determines the undo strategy; Metadata carries type-specific
// context (e.g., file paths, branch names, commit messages).
type UndoAction struct {
	Type      string            // "commit", "stage", "unstage", "branch_delete", "checkout", "discard", "revert", "reset", "amend"
	RefBefore string            // Git ref (hash) before the operation
	Metadata  map[string]string // Additional context keyed by operation type
}

// UndoManager provides undo/redo functionality for git operations.
// It maintains in-memory stacks of recorded actions and applies inverse
// operations on undo. Stacks are not persisted across application restarts.
// All methods are safe for concurrent use.
type UndoManager struct {
	mu        sync.Mutex
	client    *Client
	undoStack []UndoAction
	redoStack []UndoAction
}

// NewUndoManager creates a new UndoManager for the given git client.
func NewUndoManager(client *Client) *UndoManager {
	return &UndoManager{client: client}
}

// RecordAction pushes a new undoable action onto the stack.
// Recording a new action clears the redo stack (linear history).
func (u *UndoManager) RecordAction(action UndoAction) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.undoStack = append(u.undoStack, action)
	if len(u.undoStack) > maxUndoDepth {
		u.undoStack = u.undoStack[len(u.undoStack)-maxUndoDepth:]
	}
	u.redoStack = nil
}

// CanUndo returns true if there are actions that can be undone.
func (u *UndoManager) CanUndo() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.undoStack) > 0
}

// CanRedo returns true if there are undone actions that can be reapplied.
func (u *UndoManager) CanRedo() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.redoStack) > 0
}

// PeekUndo returns the top undo action without removing it. Returns the
// action and true, or a zero value and false if the stack is empty.
func (u *UndoManager) PeekUndo() (UndoAction, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.undoStack) == 0 {
		return UndoAction{}, false
	}
	return u.undoStack[len(u.undoStack)-1], true
}

// PeekRedo returns the top redo action without removing it.
func (u *UndoManager) PeekRedo() (UndoAction, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.redoStack) == 0 {
		return UndoAction{}, false
	}
	return u.redoStack[len(u.redoStack)-1], true
}

// NeedsConfirmation returns true if the next undo operation would be
// destructive and should require user confirmation via a modal dialog.
func (u *UndoManager) NeedsConfirmation() bool {
	action, ok := u.PeekUndo()
	if !ok {
		return false
	}
	return isDestructive(action)
}

// RedoNeedsConfirmation returns true if the next redo operation would be
// destructive and should require user confirmation.
func (u *UndoManager) RedoNeedsConfirmation() bool {
	action, ok := u.PeekRedo()
	if !ok {
		return false
	}
	return isRedoDestructive(action)
}

// isDestructive returns true if undoing the given action requires confirmation.
// Currently only force-push-related undos are considered destructive.
func isDestructive(action UndoAction) bool {
	// Undoing a commit that was pushed would require a force push.
	if action.Type == "commit" && action.Metadata["pushed"] == "true" { //nolint:goconst // inline string is more readable here
		return true
	}
	return false
}

// isRedoDestructive returns true if redo-ing the given action requires
// confirmation. Deleting a branch is a destructive redo operation.
func isRedoDestructive(action UndoAction) bool {
	return action.Type == "branch_delete" //nolint:goconst // inline string is more readable here
}

// Undo reverses the most recent recorded action. Returns a human-readable
// description of what was undone, or an error if the undo fails. On failure
// the action is restored to the undo stack so the user can retry.
func (u *UndoManager) Undo(ctx context.Context) (string, error) {
	u.mu.Lock()
	if len(u.undoStack) == 0 {
		u.mu.Unlock()
		return "", fmt.Errorf("nothing to undo")
	}

	action := u.undoStack[len(u.undoStack)-1]
	u.undoStack = u.undoStack[:len(u.undoStack)-1]
	u.mu.Unlock()

	desc, err := u.executeUndo(ctx, action)

	u.mu.Lock()
	defer u.mu.Unlock()
	if err != nil {
		u.undoStack = append(u.undoStack, action)
		return "", err
	}

	u.redoStack = append(u.redoStack, action)
	return desc, nil
}

// Redo re-applies the most recently undone action. Returns a human-readable
// description, or an error if the redo fails. On failure the action is
// restored to the redo stack.
func (u *UndoManager) Redo(ctx context.Context) (string, error) {
	u.mu.Lock()
	if len(u.redoStack) == 0 {
		u.mu.Unlock()
		return "", fmt.Errorf("nothing to redo")
	}

	action := u.redoStack[len(u.redoStack)-1]
	u.redoStack = u.redoStack[:len(u.redoStack)-1]
	u.mu.Unlock()

	desc, err := u.executeRedo(ctx, action)

	u.mu.Lock()
	defer u.mu.Unlock()
	if err != nil {
		u.redoStack = append(u.redoStack, action)
		return "", err
	}

	u.undoStack = append(u.undoStack, action)
	return desc, nil
}

// executeUndo performs the inverse operation for the given action.
func (u *UndoManager) executeUndo(ctx context.Context, action UndoAction) (string, error) {
	switch action.Type {
	case "commit":
		return u.undoCommit(ctx)
	case "stage":
		return u.undoStage(ctx, action)
	case "unstage":
		return u.undoUnstage(ctx, action)
	case "branch_delete":
		return u.undoBranchDelete(ctx, action)
	case "checkout":
		return u.undoCheckout(ctx, action)
	case "discard": //nolint:goconst // inline string is more readable here
		return u.undoDiscard(ctx, action)
	case "revert":
		return u.undoRevert(ctx, action)
	case "reset":
		return u.undoReset(ctx, action)
	case "amend":
		return u.undoAmend(ctx)
	default:
		return "", fmt.Errorf("unknown undo action type: %s", action.Type)
	}
}

// executeRedo re-applies the given action.
func (u *UndoManager) executeRedo(ctx context.Context, action UndoAction) (string, error) {
	switch action.Type {
	case "commit":
		return u.redoCommit(ctx, action)
	case "stage":
		return u.redoStage(ctx, action)
	case "unstage":
		return u.redoUnstage(ctx, action)
	case "branch_delete":
		return u.redoBranchDelete(ctx, action)
	case "checkout":
		return u.redoCheckout(ctx, action)
	case "discard":
		return u.redoDiscard(ctx, action)
	case "revert":
		return u.redoRevert(ctx, action)
	case "reset":
		return u.redoReset(ctx, action)
	case "amend":
		return u.redoAmend(ctx, action)
	default:
		return "", fmt.Errorf("unknown redo action type: %s", action.Type)
	}
}

// undoCommit reverses a commit via git reset --soft HEAD~1.
// Changes remain staged so the user can re-commit if desired.
func (u *UndoManager) undoCommit(ctx context.Context) (string, error) {
	_, err := u.client.run(ctx, "reset", "--soft", "HEAD~1")
	if err != nil {
		return "", fmt.Errorf("undo commit: %w", err)
	}
	u.client.cache.Invalidate()
	return "commit (changes kept staged)", nil
}

// redoCommit re-applies a commit with the original message.
func (u *UndoManager) redoCommit(ctx context.Context, action UndoAction) (string, error) {
	msg := action.Metadata["message"]
	if msg == "" {
		return "", fmt.Errorf("redo commit: missing commit message in metadata")
	}
	_, err := u.client.Commit(ctx, msg, CommitOpts{})
	if err != nil {
		return "", fmt.Errorf("redo commit: %w", err)
	}
	return fmt.Sprintf("commit: %s", truncateStr(msg, 50)), nil
}

// undoStage reverses staging by unstaging the paths.
func (u *UndoManager) undoStage(ctx context.Context, action UndoAction) (string, error) {
	paths := splitPaths(action.Metadata["paths"])
	if len(paths) == 0 {
		return "", fmt.Errorf("undo stage: no paths in metadata")
	}
	if err := u.client.Unstage(ctx, paths); err != nil {
		return "", fmt.Errorf("undo stage: %w", err)
	}
	return fmt.Sprintf("stage %s", summarizePaths(paths)), nil
}

// redoStage re-applies staging.
func (u *UndoManager) redoStage(ctx context.Context, action UndoAction) (string, error) {
	paths := splitPaths(action.Metadata["paths"])
	if len(paths) == 0 {
		return "", fmt.Errorf("redo stage: no paths in metadata")
	}
	if err := u.client.Stage(ctx, paths); err != nil {
		return "", fmt.Errorf("redo stage: %w", err)
	}
	return fmt.Sprintf("stage %s", summarizePaths(paths)), nil
}

// undoUnstage reverses unstaging by re-staging the paths.
func (u *UndoManager) undoUnstage(ctx context.Context, action UndoAction) (string, error) {
	paths := splitPaths(action.Metadata["paths"])
	if len(paths) == 0 {
		return "", fmt.Errorf("undo unstage: no paths in metadata")
	}
	if err := u.client.Stage(ctx, paths); err != nil {
		return "", fmt.Errorf("undo unstage: %w", err)
	}
	return fmt.Sprintf("unstage %s", summarizePaths(paths)), nil
}

// redoUnstage re-applies unstaging.
func (u *UndoManager) redoUnstage(ctx context.Context, action UndoAction) (string, error) {
	paths := splitPaths(action.Metadata["paths"])
	if len(paths) == 0 {
		return "", fmt.Errorf("redo unstage: no paths in metadata")
	}
	if err := u.client.Unstage(ctx, paths); err != nil {
		return "", fmt.Errorf("redo unstage: %w", err)
	}
	return fmt.Sprintf("unstage %s", summarizePaths(paths)), nil
}

// undoBranchDelete recreates a deleted branch from the stored hash.
func (u *UndoManager) undoBranchDelete(ctx context.Context, action UndoAction) (string, error) {
	name := action.Metadata["name"]
	hash := action.Metadata["hash"]
	if name == "" || hash == "" {
		return "", fmt.Errorf("undo branch delete: missing name or hash in metadata")
	}
	if err := u.client.BranchCreate(ctx, name, hash); err != nil {
		return "", fmt.Errorf("undo branch delete: %w", err)
	}
	return fmt.Sprintf("delete branch %s", name), nil
}

// redoBranchDelete re-deletes a branch.
func (u *UndoManager) redoBranchDelete(ctx context.Context, action UndoAction) (string, error) {
	name := action.Metadata["name"]
	if name == "" {
		return "", fmt.Errorf("redo branch delete: missing name in metadata")
	}
	if err := u.client.BranchDelete(ctx, name, true); err != nil {
		return "", fmt.Errorf("redo branch delete: %w", err)
	}
	return fmt.Sprintf("delete branch %s", name), nil
}

// undoCheckout returns to the previous branch.
func (u *UndoManager) undoCheckout(ctx context.Context, action UndoAction) (string, error) {
	from := action.Metadata["from"]
	if from == "" {
		return "", fmt.Errorf("undo checkout: missing 'from' branch in metadata")
	}
	if err := u.client.Checkout(ctx, from); err != nil {
		return "", fmt.Errorf("undo checkout: %w", err)
	}
	to := action.Metadata["to"]
	return fmt.Sprintf("checkout to %s (back to %s)", to, from), nil
}

// redoCheckout re-applies the checkout to the target branch.
func (u *UndoManager) redoCheckout(ctx context.Context, action UndoAction) (string, error) {
	to := action.Metadata["to"]
	if to == "" {
		return "", fmt.Errorf("redo checkout: missing 'to' branch in metadata")
	}
	if err := u.client.Checkout(ctx, to); err != nil {
		return "", fmt.Errorf("redo checkout: %w", err)
	}
	return fmt.Sprintf("checkout to %s", to), nil
}

// undoDiscard restores discarded changes by popping the safety stash.
// RecordAction metadata must include "stash_ref" (the stash created before
// discard) and "path" (the affected file).
func (u *UndoManager) undoDiscard(ctx context.Context, action UndoAction) (string, error) {
	stashRef := action.Metadata["stash_ref"]
	path := action.Metadata["path"]
	if stashRef == "" {
		return "", fmt.Errorf("undo discard: missing stash_ref in metadata")
	}
	// Apply the stash to restore discarded changes, keep the stash for redo.
	_, err := u.client.run(ctx, "stash", "apply", stashRef)
	if err != nil {
		u.client.cache.Invalidate() // Invalidate even on failure; working tree may be partially modified.
		return "", fmt.Errorf("undo discard: %w", err)
	}
	u.client.cache.Invalidate()
	desc := "discard"
	if path != "" {
		desc += " " + path
	}
	return desc, nil
}

// redoDiscard re-applies the discard by checking out the file again.
func (u *UndoManager) redoDiscard(ctx context.Context, action UndoAction) (string, error) {
	path := action.Metadata["path"]
	if path == "" {
		return "", fmt.Errorf("redo discard: missing path in metadata")
	}
	if err := u.client.DiscardFile(ctx, path); err != nil {
		return "", fmt.Errorf("redo discard: %w", err)
	}
	return "discard " + path, nil
}

// undoRevert undoes a revert by resetting to the pre-revert ref.
func (u *UndoManager) undoRevert(ctx context.Context, action UndoAction) (string, error) {
	refBefore := action.RefBefore
	if refBefore == "" {
		return "", fmt.Errorf("undo revert: missing RefBefore")
	}
	_, err := u.client.run(ctx, "reset", "--hard", refBefore)
	if err != nil {
		return "", fmt.Errorf("undo revert: %w", err)
	}
	u.client.cache.Invalidate()
	hash := action.Metadata["hash"]
	return fmt.Sprintf("revert of %s", truncateStr(hash, 8)), nil
}

// redoRevert re-applies the revert.
func (u *UndoManager) redoRevert(ctx context.Context, action UndoAction) (string, error) {
	hash := action.Metadata["hash"]
	if hash == "" {
		return "", fmt.Errorf("redo revert: missing hash in metadata")
	}
	if err := u.client.Revert(ctx, hash); err != nil {
		return "", fmt.Errorf("redo revert: %w", err)
	}
	return fmt.Sprintf("revert %s", truncateStr(hash, 8)), nil
}

// undoReset moves HEAD back to the pre-reset ref.
func (u *UndoManager) undoReset(ctx context.Context, action UndoAction) (string, error) {
	refBefore := action.RefBefore
	if refBefore == "" {
		return "", fmt.Errorf("undo reset: missing RefBefore")
	}
	_, err := u.client.run(ctx, "reset", "--hard", refBefore)
	if err != nil {
		return "", fmt.Errorf("undo reset: %w", err)
	}
	u.client.cache.Invalidate()
	return fmt.Sprintf("reset to %s", truncateStr(action.Metadata["ref"], 8)), nil
}

// redoReset re-applies the reset with the original mode.
func (u *UndoManager) redoReset(ctx context.Context, action UndoAction) (string, error) {
	ref := action.Metadata["ref"]
	mode := action.Metadata["mode"]
	if ref == "" || mode == "" {
		return "", fmt.Errorf("redo reset: missing ref or mode in metadata")
	}
	if err := u.client.Reset(ctx, ref, ResetMode(mode)); err != nil {
		return "", fmt.Errorf("redo reset: %w", err)
	}
	return fmt.Sprintf("reset --%s to %s", mode, truncateStr(ref, 8)), nil
}

// undoAmend reverses an amend by resetting to the pre-amend HEAD.
// The amended changes remain staged so the user can re-commit.
func (u *UndoManager) undoAmend(ctx context.Context) (string, error) {
	_, err := u.client.run(ctx, "reset", "--soft", "HEAD@{1}")
	if err != nil {
		return "", fmt.Errorf("undo amend: %w", err)
	}
	u.client.cache.Invalidate()
	return "amend (changes kept staged)", nil
}

// redoAmend re-applies the amend with the original message.
func (u *UndoManager) redoAmend(ctx context.Context, action UndoAction) (string, error) {
	msg := action.Metadata["message"]
	if msg == "" {
		return "", fmt.Errorf("redo amend: missing commit message in metadata")
	}
	_, err := u.client.Commit(ctx, msg, CommitOpts{Amend: true})
	if err != nil {
		return "", fmt.Errorf("redo amend: %w", err)
	}
	return fmt.Sprintf("amend: %s", truncateStr(msg, 50)), nil
}

// JoinPaths joins paths with newline separators for storage in UndoAction
// metadata. Newlines are forbidden in git paths (see ValidatePath), making
// this a safe separator.
func JoinPaths(paths []string) string {
	return strings.Join(paths, "\n")
}

// splitPaths splits a newline-separated path list from UndoAction metadata.
func splitPaths(s string) []string {
	if s == "" {
		return nil
	}
	var paths []string
	for _, p := range strings.Split(s, "\n") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// summarizePaths returns a short human-readable description of affected paths.
func summarizePaths(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	if len(paths) == 1 {
		return paths[0]
	}
	return fmt.Sprintf("%s (+%d more)", paths[0], len(paths)-1)
}

// truncateStr shortens s to maxLen characters, appending "..." if truncated.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
