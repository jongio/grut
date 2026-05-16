// Package filetree provides file operation helpers for the grut file explorer.
// All operations validate that paths stay within the tree's root directory
// (path jailing) to prevent directory-traversal attacks.
package filetree

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// opKind identifies the type of pending file operation.
type opKind int

const (
	opDelete opKind = iota
	opPaste
	opRename
	opNewFile
	opNewDir
	opRightClickPick
	opFirstUseConfirm
)

// pendingOperation holds state for an operation that requires user
// confirmation (e.g. delete confirmation modal) before execution.
type pendingOperation struct {
	destDir string   // destination directory (paste)
	name    string   // item type name (for opFirstUseConfirm)
	paths   []string // source paths
	kind    opKind
}

// clipboard holds cut/copy state for the filetree panel.
type clipboard struct {
	paths []string
	cut   bool // true = move on paste, false = copy
}

// ---------------------------------------------------------------------------
// Path jailing
// ---------------------------------------------------------------------------
// isWithinRoot returns true if the given path resolves to a location
// inside the tree's root directory. This prevents directory-traversal
// attacks via "../" sequences and symlink escapes (CWE-59).
func isWithinRoot(root, target string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	// Resolve symlinks to prevent symlink escape attacks.
	// If EvalSymlinks fails (target doesn't exist yet for writes),
	// fall back to lexical check on the cleaned absolute path.
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		resolvedRoot = filepath.Clean(absRoot)
	}
	resolvedTarget, err := filepath.EvalSymlinks(absTarget)
	if err != nil {
		// Target doesn't exist yet (write operation). Resolve the parent
		// directory to normalize short paths (e.g. RUNNER~1 → runneradmin on
		// Windows CI), then re-append the base name.
		parent := filepath.Dir(absTarget)
		resolvedParent, perr := filepath.EvalSymlinks(parent)
		if perr != nil {
			resolvedParent = filepath.Clean(parent)
		}
		resolvedTarget = filepath.Join(resolvedParent, filepath.Base(absTarget))
	}
	// Target must be under root or equal to root.
	rel, err := filepath.Rel(resolvedRoot, resolvedTarget)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ---------------------------------------------------------------------------
// File operations
// ---------------------------------------------------------------------------
// copyFile copies src to dst. If src is a directory, the copy is recursive.
// Both src and dst must be within root. The context allows cancellation of
// long-running copy operations (F20).
func copyFile(ctx context.Context, root, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !isWithinRoot(root, src) {
		return fmt.Errorf("source path %q is outside root", src)
	}
	if !isWithinRoot(root, dst) {
		return fmt.Errorf("destination path %q is outside root", dst)
	}
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("stat %q: %w", src, err)
	}
	if info.IsDir() {
		return copyDir(ctx, root, src, dst)
	}
	return copyRegularFile(src, dst, info.Mode())
}

// copyRegularFile copies a single file, preserving permissions.
func copyRegularFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %q: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create %q: %w", dst, err)
	}
	// Use a flag to prevent double-close: defer is the safety net,
	// explicit close captures the write-flush error.
	closed := false
	defer func() {
		if !closed {
			_ = out.Close()
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %q → %q: %w", src, dst, err)
	}
	closed = true
	return out.Close()
}

// copyDir recursively copies a directory tree.
func copyDir(ctx context.Context, root, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %q: %w", src, err)
	}
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("mkdir %q: %w", dst, err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("readdir %q: %w", src, err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		srcChild := filepath.Join(src, entry.Name())
		dstChild := filepath.Join(dst, entry.Name())
		if !isWithinRoot(root, srcChild) || !isWithinRoot(root, dstChild) {
			continue // skip anything that escapes root
		}
		if entry.IsDir() {
			if err := copyDir(ctx, root, srcChild, dstChild); err != nil {
				return err
			}
		} else {
			info, err := entry.Info()
			if err != nil {
				return fmt.Errorf("info %q: %w", srcChild, err)
			}
			if err := copyRegularFile(srcChild, dstChild, info.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

// moveFile moves src to dst. Both must be within root.
// The context allows cancellation before the operation starts (F20).
func moveFile(ctx context.Context, root, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !isWithinRoot(root, src) {
		return fmt.Errorf("source path %q is outside root", src)
	}
	if !isWithinRoot(root, dst) {
		return fmt.Errorf("destination path %q is outside root", dst)
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("move %q → %q: %w", src, dst, err)
	}
	return nil
}

// deleteFile removes the file or directory at path (recursive for dirs).
// The path must be within root.
// The context allows cancellation before the operation starts (F20).
func deleteFile(ctx context.Context, root, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !isWithinRoot(root, path) {
		return fmt.Errorf("path %q is outside root", path)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("delete %q: %w", path, err)
	}
	return nil
}

// renameFile renames the file at oldPath to newName within the same directory.
// The result must remain within root.
// The context allows cancellation before the operation starts (F20).
func renameFile(ctx context.Context, root, oldPath, newName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !isWithinRoot(root, oldPath) {
		return fmt.Errorf("path %q is outside root", oldPath)
	}
	// newName must be a plain filename — no path separators or traversal.
	if strings.ContainsAny(newName, `/\`) || newName == ".." || newName == "." || newName == "" {
		return fmt.Errorf("invalid name %q", newName)
	}
	dir := filepath.Dir(oldPath)
	newPath := filepath.Join(dir, newName)
	if !isWithinRoot(root, newPath) {
		return fmt.Errorf("new path %q is outside root", newPath)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("rename %q → %q: %w", oldPath, newPath, err)
	}
	return nil
}

// createDir creates a new directory at path. The path must be within root.
// The context allows cancellation before the operation starts (F20).
func createDir(ctx context.Context, root, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !isWithinRoot(root, path) {
		return fmt.Errorf("path %q is outside root", path)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		return fmt.Errorf("create dir %q: %w", path, err)
	}
	return nil
}

// createFile creates a new empty file at path. The path must be within root.
// The context allows cancellation before the operation starts (F20).
func createFile(ctx context.Context, root, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !isWithinRoot(root, path) {
		return fmt.Errorf("path %q is outside root", path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create file %q: %w", path, err)
	}
	return f.Close()
}
