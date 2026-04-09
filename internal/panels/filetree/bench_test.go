package filetree

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jongio/grut/internal/config"
)

// benchResultStr prevents the compiler from optimizing away string results.
var benchResultStr string

// benchResultInt prevents the compiler from optimizing away int results.
var benchResultInt int

// ---------------------------------------------------------------------------
// Helpers: create realistic directory trees for benchmarking
// ---------------------------------------------------------------------------

// createFlatDir creates a directory with n files (no subdirectories).
func createFlatDir(b *testing.B, n int) string {
	b.Helper()
	dir := b.TempDir()
	for i := range n {
		name := fmt.Sprintf("file_%04d.go", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package main\n"), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	return dir
}

// createNestedDir creates a directory tree with the given depth and files per level.
func createNestedDir(b *testing.B, depth, filesPerLevel int) string {
	b.Helper()
	root := b.TempDir()

	var create func(parent string, level int)
	create = func(parent string, level int) {
		if level >= depth {
			return
		}
		// Create files at this level.
		for i := range filesPerLevel {
			name := fmt.Sprintf("mod_%d_%04d.go", level, i)
			if err := os.WriteFile(filepath.Join(parent, name), []byte("package mod\n"), 0o644); err != nil {
				b.Fatal(err)
			}
		}
		// Create 2-3 subdirectories at each level.
		subdirs := 2
		if level == 0 {
			subdirs = 3
		}
		for d := range subdirs {
			sub := filepath.Join(parent, fmt.Sprintf("pkg_%d_%d", level, d))
			if err := os.MkdirAll(sub, 0o755); err != nil {
				b.Fatal(err)
			}
			create(sub, level+1)
		}
	}

	create(root, 0)
	return root
}

// createLargeDir creates a directory tree with approximately n total nodes.
func createLargeDir(b *testing.B, targetNodes int) string {
	b.Helper()
	root := b.TempDir()
	created := 0

	// Create a broad structure: 10 top-level dirs, each with subdirs and files.
	topDirs := 10
	filesPerDir := targetNodes / (topDirs * 3)
	if filesPerDir < 1 {
		filesPerDir = 1
	}

	for t := range topDirs {
		topDir := filepath.Join(root, fmt.Sprintf("module_%02d", t))
		if err := os.MkdirAll(topDir, 0o755); err != nil {
			b.Fatal(err)
		}
		created++

		for s := range 3 {
			subDir := filepath.Join(topDir, fmt.Sprintf("sub_%d", s))
			if err := os.MkdirAll(subDir, 0o755); err != nil {
				b.Fatal(err)
			}
			created++

			for f := range filesPerDir {
				if created >= targetNodes {
					return root
				}
				name := fmt.Sprintf("impl_%04d.go", f)
				if err := os.WriteFile(filepath.Join(subDir, name), []byte("package impl\n"), 0o644); err != nil {
					b.Fatal(err)
				}
				created++
			}
		}
	}
	return root
}

// newBenchTree creates a FileTree for benchmarking with the given root.
func newBenchTree(root string) *FileTree {
	cfg := config.FileTreeConfig{
		ShowHidden:           false,
		ShowIcons:            false,
		IconMode:             "ascii",
		SortDirectoriesFirst: true,
		MaxDepth:             20,
	}
	ft := New(cfg, root, nil)
	ft.loadChildren(ft.root)
	return ft
}

// expandAll recursively expands and loads all directory nodes.
func expandAll(ft *FileTree, n *node) {
	if n.isDir {
		n.expanded = true
		ft.loadChildren(n)
		for _, child := range n.children {
			expandAll(ft, child)
		}
	}
}

// ---------------------------------------------------------------------------
// Benchmarks: rebuildVisible (tree flattening)
// ---------------------------------------------------------------------------

func BenchmarkTreeRebuildVisible(b *testing.B) {
	b.Run("flat_50_files", func(b *testing.B) {
		root := createFlatDir(b, 50)
		ft := newBenchTree(root)
		ft.root.expanded = true
		ft.rebuildVisible()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			ft.rebuildVisible()
		}
	})

	b.Run("nested_5_deep_200_nodes", func(b *testing.B) {
		root := createNestedDir(b, 5, 8)
		ft := newBenchTree(root)
		expandAll(ft, ft.root)
		ft.rebuildVisible()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			ft.rebuildVisible()
		}
	})

	b.Run("large_1000_nodes", func(b *testing.B) {
		root := createLargeDir(b, 1000)
		ft := newBenchTree(root)
		expandAll(ft, ft.root)
		ft.rebuildVisible()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			ft.rebuildVisible()
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmarks: renderLine
// ---------------------------------------------------------------------------

func BenchmarkRenderLine(b *testing.B) {
	b.Run("file_node", func(b *testing.B) {
		root := createFlatDir(b, 10)
		ft := newBenchTree(root)
		ft.root.expanded = true
		ft.rebuildVisible()
		if len(ft.visible) == 0 {
			b.Skip("no visible nodes")
		}
		n := ft.visible[0]
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr = ft.renderLine(n, 80, false)
		}
	})

	b.Run("dir_node_cursor", func(b *testing.B) {
		root := createNestedDir(b, 3, 5)
		ft := newBenchTree(root)
		ft.root.expanded = true
		ft.focused = true
		ft.rebuildVisible()
		// Find a directory node.
		var dirNode *node
		for _, v := range ft.visible {
			if v.isDir {
				dirNode = v
				break
			}
		}
		if dirNode == nil {
			b.Skip("no directory node found")
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr = ft.renderLine(dirNode, 80, true)
		}
	})

	b.Run("with_icons", func(b *testing.B) {
		root := createFlatDir(b, 10)
		cfg := config.FileTreeConfig{
			ShowHidden:           false,
			ShowIcons:            true,
			IconMode:             "ascii",
			SortDirectoriesFirst: true,
			MaxDepth:             20,
		}
		ft := New(cfg, root, nil)
		ft.loadChildren(ft.root)
		ft.root.expanded = true
		ft.rebuildVisible()
		if len(ft.visible) == 0 {
			b.Skip("no visible nodes")
		}
		n := ft.visible[0]
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr = ft.renderLine(n, 80, false)
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmarks: loadChildren (lazy directory reading)
// ---------------------------------------------------------------------------

func BenchmarkLazyLoad(b *testing.B) {
	b.Run("50_entries", func(b *testing.B) {
		root := createFlatDir(b, 50)
		cfg := config.FileTreeConfig{
			SortDirectoriesFirst: true,
			MaxDepth:             20,
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			n := &node{
				name:  filepath.Base(root),
				path:  root,
				isDir: true,
				depth: 0,
			}
			loadChildrenStatic(n, cfg)
		}
	})

	b.Run("200_entries_nested", func(b *testing.B) {
		root := createNestedDir(b, 3, 20)
		cfg := config.FileTreeConfig{
			SortDirectoriesFirst: true,
			MaxDepth:             20,
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			n := &node{
				name:  filepath.Base(root),
				path:  root,
				isDir: true,
				depth: 0,
			}
			loadChildrenStatic(n, cfg)
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmarks: display width helpers
// ---------------------------------------------------------------------------

func BenchmarkDisplayWidth(b *testing.B) {
	b.Run("ascii", func(b *testing.B) {
		s := "src/internal/panels/filetree/handler.go"
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultInt = displayWidth(s)
		}
	})

	b.Run("mixed_unicode", func(b *testing.B) {
		s := "docs/\u2192 README.md [modified]"
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultInt = displayWidth(s)
		}
	})
}

func BenchmarkTruncateToWidth(b *testing.B) {
	b.Run("no_truncation", func(b *testing.B) {
		s := "short.go"
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr = truncateToWidth(s, 80)
		}
	})

	b.Run("needs_truncation", func(b *testing.B) {
		s := "very/deep/nested/directory/structure/with/many/segments/and/a/long/filename_that_exceeds_terminal_width.go"
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchResultStr = truncateToWidth(s, 40)
		}
	})
}
