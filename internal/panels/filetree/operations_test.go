package filetree

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// copyFile tests
// ---------------------------------------------------------------------------

func TestCopyFile_RegularFile(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "original.txt")
	dst := filepath.Join(root, "copy.txt")
	content := []byte("hello world")
	require.NoError(t, os.WriteFile(src, content, 0o644))

	err := copyFile(context.Background(), root, src, dst)
	require.NoError(t, err)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, got)

	// Source should still exist.
	_, err = os.Stat(src)
	assert.NoError(t, err)
}

func TestCopyFile_DirectoryRecursive(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "srcdir")
	require.NoError(t, os.Mkdir(srcDir, 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(srcDir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "sub", "b.txt"), []byte("b"), 0o644))

	dstDir := filepath.Join(root, "dstdir")
	err := copyFile(context.Background(), root, srcDir, dstDir)
	require.NoError(t, err)

	// Verify copied structure.
	gotA, err := os.ReadFile(filepath.Join(dstDir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("a"), gotA)

	gotB, err := os.ReadFile(filepath.Join(dstDir, "sub", "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("b"), gotB)
}

func TestCopyFile_PreservesPermissions(t *testing.T) {
	if os.Getenv("OS") == "Windows_NT" || filepath.Separator == '\\' {
		t.Skip("file permission tests are not reliable on Windows")
	}

	root := t.TempDir()
	src := filepath.Join(root, "script.sh")
	dst := filepath.Join(root, "script_copy.sh")
	require.NoError(t, os.WriteFile(src, []byte("#!/bin/sh"), 0o755))

	err := copyFile(context.Background(), root, src, dst)
	require.NoError(t, err)

	info, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

// ---------------------------------------------------------------------------
// moveFile tests
// ---------------------------------------------------------------------------

func TestMoveFile(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "original.txt")
	dst := filepath.Join(root, "moved.txt")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))

	err := moveFile(context.Background(), root, src, dst)
	require.NoError(t, err)

	// Destination should exist.
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), got)

	// Source should not exist.
	_, err = os.Stat(src)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestMoveFile_Directory(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "old")
	require.NoError(t, os.Mkdir(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "f.txt"), []byte("f"), 0o644))

	dstDir := filepath.Join(root, "new")
	err := moveFile(context.Background(), root, srcDir, dstDir)
	require.NoError(t, err)

	// Verify moved.
	got, err := os.ReadFile(filepath.Join(dstDir, "f.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("f"), got)

	_, err = os.Stat(srcDir)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

// ---------------------------------------------------------------------------
// deleteFile tests
// ---------------------------------------------------------------------------

func TestDeleteFile_RegularFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "to_delete.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))

	err := deleteFile(context.Background(), root, path, true)
	require.NoError(t, err)

	_, err = os.Stat(path)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestDeleteFile_DirectoryRecursive(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dir_to_delete")
	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "f.txt"), []byte("f"), 0o644))

	err := deleteFile(context.Background(), root, dir, true)
	require.NoError(t, err)

	_, err = os.Stat(dir)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestBulkDelete(t *testing.T) {
	root := t.TempDir()
	files := []string{
		filepath.Join(root, "a.txt"),
		filepath.Join(root, "b.txt"),
		filepath.Join(root, "c.txt"),
	}
	for _, f := range files {
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
	}

	for _, f := range files {
		require.NoError(t, deleteFile(context.Background(), root, f, true))
	}

	for _, f := range files {
		_, err := os.Stat(f)
		assert.True(t, errors.Is(err, fs.ErrNotExist), "expected %q to be deleted", f)
	}
}

// ---------------------------------------------------------------------------
// renameFile tests
// ---------------------------------------------------------------------------

func TestRenameFile(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old_name.txt")
	require.NoError(t, os.WriteFile(oldPath, []byte("data"), 0o644))

	err := renameFile(context.Background(), root, oldPath, "new_name.txt")
	require.NoError(t, err)

	newPath := filepath.Join(root, "new_name.txt")
	got, err := os.ReadFile(newPath)
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), got)

	_, err = os.Stat(oldPath)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestRenameFile_InvalidName(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))

	tests := []struct {
		name    string
		newName string
	}{
		{"empty", ""},
		{"dot-dot", ".."},
		{"dot", "."},
		{"slash", "sub/name"},
		{"backslash", `sub\name`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := renameFile(context.Background(), root, path, tt.newName)
			assert.Error(t, err, "rename to %q should fail", tt.newName)
		})
	}
}

// ---------------------------------------------------------------------------
// createDir tests
// ---------------------------------------------------------------------------

func TestCreateDir(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "newdir")

	err := createDir(context.Background(), root, path)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// ---------------------------------------------------------------------------
// createFile tests
// ---------------------------------------------------------------------------

func TestCreateFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "newfile.txt")

	err := createFile(context.Background(), root, path)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.Equal(t, int64(0), info.Size())
}

func TestCreateFile_AlreadyExists(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "existing.txt")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))

	err := createFile(context.Background(), root, path)
	assert.Error(t, err, "creating an existing file should fail")
}

// ---------------------------------------------------------------------------
// Path jailing tests
// ---------------------------------------------------------------------------

func TestIsWithinRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))

	assert.True(t, isWithinRoot(root, sub))
	assert.True(t, isWithinRoot(root, root))
	assert.True(t, isWithinRoot(root, filepath.Join(root, "file.txt")))
}

func TestIsWithinRoot_RejectEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	assert.False(t, isWithinRoot(root, outside))
	assert.False(t, isWithinRoot(root, filepath.Join(root, "..", "escape")))
}

func TestPathJailing_CopyPreventsEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "f.txt"), []byte("x"), 0o644))

	err := copyFile(context.Background(), root, filepath.Join(root, "f.txt"), filepath.Join(outside, "f.txt"))
	assert.Error(t, err, "copy to outside root should fail")
}

func TestPathJailing_MovePreventsEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "f.txt"), []byte("x"), 0o644))

	err := moveFile(context.Background(), root, filepath.Join(root, "f.txt"), filepath.Join(outside, "f.txt"))
	assert.Error(t, err, "move to outside root should fail")
}

func TestPathJailing_DeletePreventsEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("secret"), 0o644))

	err := deleteFile(context.Background(), root, outsideFile, true)
	assert.Error(t, err, "delete outside root should fail")

	// Verify file still exists.
	_, err = os.Stat(outsideFile)
	assert.NoError(t, err)
}

func TestPathJailing_RenamePreventsEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "file.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("x"), 0o644))

	err := renameFile(context.Background(), root, outsideFile, "new.txt")
	assert.Error(t, err, "rename outside root should fail")
}

func TestPathJailing_CreateDirPreventsEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	err := createDir(context.Background(), root, filepath.Join(outside, "newdir"))
	assert.Error(t, err, "createDir outside root should fail")
}

func TestPathJailing_CreateFilePreventsEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	err := createFile(context.Background(), root, filepath.Join(outside, "newfile.txt"))
	assert.Error(t, err, "createFile outside root should fail")
}

// ---------------------------------------------------------------------------
// Operations on non-existent files
// ---------------------------------------------------------------------------

func TestCopyFile_NonExistent(t *testing.T) {
	root := t.TempDir()
	err := copyFile(context.Background(), root, filepath.Join(root, "nope.txt"), filepath.Join(root, "dst.txt"))
	assert.Error(t, err)
}

func TestMoveFile_NonExistent(t *testing.T) {
	root := t.TempDir()
	err := moveFile(context.Background(), root, filepath.Join(root, "nope.txt"), filepath.Join(root, "dst.txt"))
	assert.Error(t, err)
}

func TestDeleteFile_NonExistent(t *testing.T) {
	root := t.TempDir()
	// os.RemoveAll does not error on non-existent paths.
	err := deleteFile(context.Background(), root, filepath.Join(root, "nope.txt"), true)
	assert.NoError(t, err, "deleteFile on non-existent path should not error (RemoveAll semantics)")
}

func TestRenameFile_NonExistent(t *testing.T) {
	root := t.TempDir()
	err := renameFile(context.Background(), root, filepath.Join(root, "nope.txt"), "new.txt")
	assert.Error(t, err)
}
