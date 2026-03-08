//go:build screenshots

package tui

import (
	"strings"
	"testing"

	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/theme"
)

// ---------------------------------------------------------------------------
// compositeOverlay
// ---------------------------------------------------------------------------

func TestCompositeOverlay_OverlayReplacesBackground(t *testing.T) {
	bg := "line1\nline2\nline3"
	fg := "OVERLAY1\nOVERLAY2\nOVERLAY3"
	result := compositeOverlay(bg, fg)
	for _, part := range []string{"OVERLAY1", "OVERLAY2", "OVERLAY3"} {
		if !strings.Contains(result, part) {
			t.Errorf("expected %q in result", part)
		}
	}
}

func TestCompositeOverlay_BlankFGShowsBG(t *testing.T) {
	bg := "line1\nline2\nline3"
	fg := "\n\n"
	result := compositeOverlay(bg, fg)
	lines := strings.Split(result, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(lines))
	}
	if lines[0] != "line1" {
		t.Errorf("line 0: expected %q, got %q", "line1", lines[0])
	}
	if lines[1] != "line2" {
		t.Errorf("line 1: expected %q, got %q", "line2", lines[1])
	}
}

func TestCompositeOverlay_FGLongerThanBG(t *testing.T) {
	bg := "bg1"
	fg := "fg1\nfg2\nfg3"
	result := compositeOverlay(bg, fg)
	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "fg1" {
		t.Errorf("expected fg1, got %q", lines[0])
	}
}

func TestCompositeOverlay_BGLongerThanFG(t *testing.T) {
	bg := "bg1\nbg2\nbg3"
	fg := "fg1"
	result := compositeOverlay(bg, fg)
	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "fg1" {
		t.Errorf("expected fg1, got %q", lines[0])
	}
	if lines[1] != "bg2" {
		t.Errorf("expected bg2, got %q", lines[1])
	}
}

func TestCompositeOverlay_EmptyStrings(t *testing.T) {
	result := compositeOverlay("", "")
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

func TestCompositeOverlay_WithANSI(t *testing.T) {
	bg := "background"
	fg := "\x1b[0m   \x1b[0m"
	result := compositeOverlay(bg, fg)
	if result != "background" {
		t.Errorf("expected background for ANSI-only overlay, got %q", result)
	}
}

func TestCompositeOverlay_MixedANSIAndContent(t *testing.T) {
	bg := "bg1\nbg2"
	fg := "\x1b[31mHello\x1b[0m\n\x1b[0m  \x1b[0m"
	result := compositeOverlay(bg, fg)
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "Hello") {
		t.Errorf("expected overlay content, got %q", lines[0])
	}
	if lines[1] != "bg2" {
		t.Errorf("expected background for blank overlay line, got %q", lines[1])
	}
}

// ---------------------------------------------------------------------------
// newScreenshotModel
// ---------------------------------------------------------------------------

func TestNewScreenshotModel_SetsSize(t *testing.T) {
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("loading theme: %v", err)
	}
	m, err := newScreenshotModel(120, 40, defaultScreenshotPreset(), th)
	if err != nil {
		t.Fatalf("newScreenshotModel failed: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil model")
	}
	if m.width != 120 {
		t.Errorf("width: expected 120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("height: expected 40, got %d", m.height)
	}
	if !m.ready {
		t.Error("ready should be true")
	}
}

func TestNewScreenshotModel_SmallSize(t *testing.T) {
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("loading theme: %v", err)
	}
	m, err := newScreenshotModel(40, 15, defaultScreenshotPreset(), th)
	if err != nil {
		t.Fatalf("newScreenshotModel failed: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil model")
	}
	if m.width != 40 || m.height != 15 {
		t.Errorf("dimensions: got %dx%d", m.width, m.height)
	}
}

// ---------------------------------------------------------------------------
// ansiStripRe
// ---------------------------------------------------------------------------

func TestAnsiStripRe(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"\x1b[0mhello\x1b[0m", "hello"},
		{"\x1b[31;1mred bold\x1b[0m", "red bold"},
		{"no ansi", "no ansi"},
		{"", ""},
	}
	for _, tt := range tests {
		got := ansiStripRe.ReplaceAllString(tt.input, "")
		if got != tt.want {
			t.Errorf("strip(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// CaptureScreenshots
// ---------------------------------------------------------------------------

func TestCaptureScreenshots_ReturnsNonEmpty(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("APPDATA", cfgDir)
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	shots, err := CaptureScreenshots(120, 30)
	if err != nil {
		t.Fatalf("CaptureScreenshots failed: %v", err)
	}
	if len(shots) == 0 {
		t.Fatal("expected at least one screenshot")
	}

	seen := make(map[string]bool)
	seenThemes := make(map[string]bool)
	for _, s := range shots {
		if s.Name == "" {
			t.Error("screenshot has empty name")
		}
		if s.ANSI == "" {
			t.Errorf("screenshot %q has empty ANSI", s.Name)
		}
		if s.SubDir == "" {
			t.Errorf("screenshot %q has empty SubDir", s.Name)
		}
		if s.BG == "" {
			t.Errorf("screenshot %q has empty BG", s.Name)
		}
		seen[s.Name] = true
		seenThemes[s.SubDir] = true
	}

	expected := []string{
		"hero-main",
		"file-explorer",
		"file-preview",
		"git-status",
		"git-log",
		"git-branches",
		"git-commits",
		"git-diff",
		"git-stash",
		"git-worktrees",
		"git-info",
		"git-info-issues",
		"git-info-prs",
		"git-info-actions",
		"review-layout",
		"agent-layout",
		"full-layout",
		"fuzzy-finder",
		"settings",
		"help",
		"bookmarks",
	}
	for _, name := range expected {
		if !seen[name] {
			t.Errorf("missing expected screenshot %q", name)
		}
	}

	// Verify all built-in themes are captured.
	for _, themeName := range theme.BuiltinNames() {
		if !seenThemes[themeName] {
			t.Errorf("missing expected theme %q", themeName)
		}
	}
}

// defaultScreenshotPreset returns the default explorer preset for tests.
func defaultScreenshotPreset() layout.Preset {
	return layout.ExplorerPreset()
}
