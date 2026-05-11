// Command genmd regenerates docs/keybindings.md from the embedded JSON source
// of truth in the keybindings package.
//
// Usage:
//
//	go run ./internal/keybindings/cmd/genmd
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jongio/grut/internal/keybindings"
)

func main() {
	root := repoRoot()
	out := filepath.Join(root, "docs", "keybindings.md")
	md := keybindings.GenerateMarkdown()
	if err := os.WriteFile(out, []byte(md), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", out, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d bytes)\n", out, len(md))
}

func repoRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "cannot determine repo root")
		os.Exit(1)
	}
	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			fmt.Fprintln(os.Stderr, "cannot find repo root (go.mod)")
			os.Exit(1)
		}
		dir = parent
	}
}
