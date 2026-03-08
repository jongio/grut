//go:build screenshots

// Command screenshots generates ANSI text captures of every TUI state
// used on the grut website and writes them as JSON data files containing
// raw ANSI escape sequences and theme configuration.
//
// Usage:
//
//	go run -tags screenshots ./cmd/screenshots [--out DIR]
//
// The companion render.mjs script renders each JSON file through xterm.js
// (a real terminal emulator) in Playwright and saves the result as PNG:
//
//	node cmd/screenshots/render.mjs [DIR]
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jongio/grut/internal/demo"
	"github.com/jongio/grut/internal/tui"
)

const (
	defaultOutDir = "web/public/screenshots"
	termWidth     = 120
	termHeight    = 40
)

// screenshotData is the JSON payload consumed by render.mjs.
type screenshotData struct {
	Name       string          `json:"name"`
	SubDir     string          `json:"subDir"`
	Cols       int             `json:"cols"`
	Rows       int             `json:"rows"`
	FG         string          `json:"fg"`
	BG         string          `json:"bg"`
	Palette    [16]string      `json:"palette"`
	ANSI       string          `json:"ansi"`
	Highlights []highlightData `json:"highlights,omitempty"`
}

type highlightData struct {
	Row  int `json:"row"`
	Col  int `json:"col"`
	Rows int `json:"rows"`
	Cols int `json:"cols"`
}

func main() {
	outDir := defaultOutDir

	for i, arg := range os.Args[1:] {
		if arg == "--out" && i+2 < len(os.Args) {
			outDir = os.Args[i+2]
		}
	}

	// Resolve outDir to an absolute path before chdir into the demo project.
	if !filepath.IsAbs(outDir) {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
			os.Exit(1)
		}
		outDir = filepath.Join(cwd, outDir)
	}

	// Create a demo project so screenshots show realistic data.
	demoDir, cleanup, err := demo.SetupProject()
	if err != nil {
		fmt.Fprintf(os.Stderr, "set up demo project: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	if err := os.Chdir(demoDir); err != nil {
		fmt.Fprintf(os.Stderr, "chdir to demo: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Demo project created at %s\n", demoDir)

	// Force terminal color environment.
	os.Setenv("CLICOLOR_FORCE", "1")
	os.Setenv("COLORTERM", "truecolor")

	fmt.Printf("Capturing %dx%d screenshots\n", termWidth, termHeight)

	shots, err := tui.CaptureScreenshots(termWidth, termHeight)
	if err != nil {
		fmt.Fprintf(os.Stderr, "capture failed: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "creating output dir: %v\n", err)
		os.Exit(1)
	}

	for _, s := range shots {
		dir := outDir
		if s.SubDir != "" {
			dir = filepath.Join(outDir, s.SubDir)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "creating dir %s: %v\n", dir, err)
			os.Exit(1)
		}

		data := screenshotData{
			Name:    s.Name,
			SubDir:  s.SubDir,
			Cols:    termWidth,
			Rows:    termHeight,
			FG:      s.FG,
			BG:      s.BG,
			Palette: s.Palette,
			ANSI:    s.ANSI,
		}
		for _, h := range s.Highlights {
			data.Highlights = append(data.Highlights, highlightData{
				Row: h.Row, Col: h.Col, Rows: h.Rows, Cols: h.Cols,
			})
		}

		jsonBytes, err := json.Marshal(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshaling %s: %v\n", s.Name, err)
			os.Exit(1)
		}
		jsonPath := filepath.Join(dir, s.Name+".json")
		if err := os.WriteFile(jsonPath, jsonBytes, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "writing %s: %v\n", jsonPath, err)
			os.Exit(1)
		}
	}

	fmt.Printf("Wrote %d JSON files to %s\n", len(shots), outDir)
	fmt.Println("Run: node cmd/screenshots/render.mjs")
}
