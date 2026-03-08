package aiconflict

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/panels"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestPanel creates a panel loaded with sample conflict data for testing.
func newTestPanel() *Panel {
	p := New(nil)
	p.SetSize(80, 24)

	files := []ConflictFileData{
		{
			Path: "file1.go",
			Regions: []ConflictRegionData{
				{
					StartLine:    2,
					EndLine:      5,
					Ours:         "func A() {}",
					Theirs:       "func B() {}",
					AIResolution: "func AB() {}",
					Explanation:  "merged both functions",
					Confidence:   0.95,
				},
				{
					StartLine:    10,
					EndLine:      15,
					Ours:         "// ours v2",
					Theirs:       "// theirs v2",
					AIResolution: "// combined v2",
					Explanation:  "combined comments",
					Confidence:   0.8,
				},
			},
		},
		{
			Path: "file2.go",
			Regions: []ConflictRegionData{
				{
					StartLine:    1,
					EndLine:      3,
					Ours:         "var x = 1",
					Theirs:       "var x = 2",
					AIResolution: "var x = 3",
					Explanation:  "chose new value",
					Confidence:   0.7,
				},
			},
		},
	}

	msg := SetConflictsMsg{Files: files}
	p.Update(msg)
	return p
}

// keyPress creates a tea.KeyPressMsg for a single rune.
func keyPress(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	p := New(nil)
	if p == nil {
		t.Fatal("New returned nil")
	}
	if p.Title() != "aiconflict" {
		t.Errorf("Title() = %q, want %q", p.Title(), "aiconflict")
	}
}

func TestInit(t *testing.T) {
	p := New(nil)
	ctx := context.Background()
	cmd := p.Init(ctx)
	if cmd != nil {
		t.Error("Init should return nil cmd")
	}
}

func TestSetConflicts(t *testing.T) {
	p := newTestPanel()

	if len(p.files) != 2 {
		t.Fatalf("expected 2 conflict files, got %d", len(p.files))
	}
	if p.currentFile != 0 {
		t.Errorf("currentFile = %d, want 0", p.currentFile)
	}
	if p.currentRegion != 0 {
		t.Errorf("currentRegion = %d, want 0", p.currentRegion)
	}
}

func TestNavigateNextRegion(t *testing.T) {
	p := newTestPanel()
	p.Focused = true

	// File1 has 2 regions. Start at region 0.
	if p.currentRegion != 0 {
		t.Fatalf("expected region 0, got %d", p.currentRegion)
	}

	// Move to region 1.
	p.Update(keyPress('n'))
	if p.currentRegion != 1 {
		t.Errorf("after n: currentRegion = %d, want 1", p.currentRegion)
	}
	if p.currentFile != 0 {
		t.Errorf("after n: currentFile = %d, want 0", p.currentFile)
	}

	// Move to file2, region 0.
	p.Update(keyPress('n'))
	if p.currentFile != 1 {
		t.Errorf("after 2nd n: currentFile = %d, want 1", p.currentFile)
	}
	if p.currentRegion != 0 {
		t.Errorf("after 2nd n: currentRegion = %d, want 0", p.currentRegion)
	}

	// At last file/region — should not advance further.
	p.Update(keyPress('n'))
	if p.currentFile != 1 || p.currentRegion != 0 {
		t.Errorf("past end: file=%d region=%d, want 1/0", p.currentFile, p.currentRegion)
	}
}

func TestNavigatePrevRegion(t *testing.T) {
	p := newTestPanel()
	p.Focused = true

	// Move to file2.
	p.Update(keyPress('n'))
	p.Update(keyPress('n'))
	if p.currentFile != 1 {
		t.Fatalf("setup: expected file 1, got %d", p.currentFile)
	}

	// Move back to file1 last region.
	p.Update(keyPress('p'))
	if p.currentFile != 0 || p.currentRegion != 1 {
		t.Errorf("after p: file=%d region=%d, want 0/1", p.currentFile, p.currentRegion)
	}

	// Move to file1 region 0.
	p.Update(keyPress('p'))
	if p.currentFile != 0 || p.currentRegion != 0 {
		t.Errorf("after 2nd p: file=%d region=%d, want 0/0", p.currentFile, p.currentRegion)
	}

	// At start — should not go further.
	p.Update(keyPress('p'))
	if p.currentFile != 0 || p.currentRegion != 0 {
		t.Errorf("before start: file=%d region=%d, want 0/0", p.currentFile, p.currentRegion)
	}
}

func TestAcceptAI(t *testing.T) {
	p := newTestPanel()
	p.Focused = true

	result, cmd := p.Update(keyPress('a'))
	p = result.(*Panel)

	choice, ok := p.resolved["file1.go"][0]
	if !ok {
		t.Fatal("expected resolution to be stored")
	}
	if choice != choiceAI {
		t.Errorf("choice = %q, want %q", choice, choiceAI)
	}

	// Should NOT emit AIConflictResolvedMsg yet (file1 has 2 regions).
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(panels.AIConflictResolvedMsg); ok {
			t.Error("should not emit AIConflictResolvedMsg with 1 of 2 regions resolved")
		}
	}
}

func TestChooseOurs(t *testing.T) {
	p := newTestPanel()
	p.Focused = true

	p.Update(keyPress('o'))
	choice := p.resolved["file1.go"][0]
	if choice != choiceOurs {
		t.Errorf("choice = %q, want %q", choice, choiceOurs)
	}
}

func TestChooseTheirs(t *testing.T) {
	p := newTestPanel()
	p.Focused = true

	p.Update(keyPress('t'))
	choice := p.resolved["file1.go"][0]
	if choice != choiceTheirs {
		t.Errorf("choice = %q, want %q", choice, choiceTheirs)
	}
}

func TestAllResolvedEmitsMessage(t *testing.T) {
	p := newTestPanel()
	p.Focused = true

	// Resolve both regions of file1.
	p.Update(keyPress('a'))           // region 0
	p.Update(keyPress('n'))           // move to region 1
	_, cmd := p.Update(keyPress('o')) // region 1

	if cmd == nil {
		t.Fatal("expected a cmd when all regions in file resolved")
	}
	msg := cmd()
	resolved, ok := msg.(panels.AIConflictResolvedMsg)
	if !ok {
		t.Fatalf("expected AIConflictResolvedMsg, got %T", msg)
	}
	if resolved.Path != "file1.go" {
		t.Errorf("resolved path = %q, want %q", resolved.Path, "file1.go")
	}
}

func TestScrollJK(t *testing.T) {
	p := newTestPanel()
	p.Focused = true

	if p.scrollY != 0 {
		t.Fatalf("initial scrollY = %d, want 0", p.scrollY)
	}

	p.Update(keyPress('j'))
	if p.scrollY != 1 {
		t.Errorf("after j: scrollY = %d, want 1", p.scrollY)
	}

	p.Update(keyPress('k'))
	if p.scrollY != 0 {
		t.Errorf("after k: scrollY = %d, want 0", p.scrollY)
	}

	// k at 0 should not go negative.
	p.Update(keyPress('k'))
	if p.scrollY != 0 {
		t.Errorf("k at 0: scrollY = %d, want 0", p.scrollY)
	}
}

func TestViewEmpty(t *testing.T) {
	p := New(nil)
	p.SetSize(80, 24)
	out := p.View(80, 24)
	if !strings.Contains(out, "No conflicts") {
		t.Errorf("expected 'No conflicts' in empty view, got:\n%s", out)
	}
}

func TestViewZeroDimensions(t *testing.T) {
	p := newTestPanel()
	if p.View(0, 24) != "" {
		t.Error("View(0, 24) should return empty string")
	}
	if p.View(80, 0) != "" {
		t.Error("View(80, 0) should return empty string")
	}
}

func TestViewShowsContent(t *testing.T) {
	p := newTestPanel()
	out := p.View(80, 40)

	checks := []string{
		"file1.go",
		"Region 1 of 2",
		"OURS",
		"THEIRS",
		"AI SUGGESTION",
		"func A() {}",
		"func B() {}",
		"func AB() {}",
		"merged both functions",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("View output missing %q", want)
		}
	}
}

func TestViewShowsResolvedIndicator(t *testing.T) {
	p := newTestPanel()
	p.Focused = true

	p.Update(keyPress('a'))
	out := p.View(80, 40)
	if !strings.Contains(out, "[resolved: ai]") {
		t.Errorf("expected '[resolved: ai]' in view after accepting AI")
	}
}

func TestKeyBindings(t *testing.T) {
	p := New(nil)
	bindings := p.KeyBindings()
	if len(bindings) == 0 {
		t.Fatal("expected non-empty key bindings")
	}

	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Key] = true
	}
	for _, want := range []string{"a", "o", "t", "e", "n", "p", "j/k"} {
		if !keys[want] {
			t.Errorf("missing key binding %q", want)
		}
	}
}

func TestFocusBlur(t *testing.T) {
	p := New(nil)
	p.Focus()
	if !p.Focused {
		t.Error("expected Focused=true after Focus()")
	}
	p.Blur()
	if p.Focused {
		t.Error("expected Focused=false after Blur()")
	}
}

func TestUnfocusedIgnoresKeys(t *testing.T) {
	p := newTestPanel()
	// Panel starts unfocused.
	p.Focused = false

	p.Update(keyPress('n'))
	if p.currentRegion != 0 {
		t.Error("unfocused panel should ignore key presses")
	}
}

func TestAcceptAIWithoutResolution(t *testing.T) {
	p := New(nil)
	p.SetSize(80, 24)
	p.Focused = true

	// Load conflicts without AI resolutions.
	files := []ConflictFileData{
		{
			Path: "nores.go",
			Regions: []ConflictRegionData{
				{StartLine: 1, EndLine: 5, Ours: "a", Theirs: "b"},
			},
		},
	}
	p.Update(SetConflictsMsg{Files: files})

	_, cmd := p.Update(keyPress('a'))
	if cmd != nil {
		t.Error("accept AI without resolution should be no-op (nil cmd)")
	}
	if len(p.resolved["nores.go"]) != 0 {
		t.Error("should not store choice when no AI resolution available")
	}
}

// strings import needed for Contains — imported at package level.
func TestNavigateEmptyConflicts(t *testing.T) {
	p := New(nil)
	p.Focused = true

	// Navigation on empty panel should be safe.
	p.Update(keyPress('n'))
	p.Update(keyPress('p'))
	// Should not panic.
}

func TestSetConflictsResetsState(t *testing.T) {
	p := newTestPanel()
	p.Focused = true

	// Make some changes.
	p.Update(keyPress('n'))
	p.Update(keyPress('a'))

	// Reload conflicts — state should reset.
	p.Update(SetConflictsMsg{
		Files: []ConflictFileData{
			{Path: "new.go", Regions: []ConflictRegionData{
				{StartLine: 1, EndLine: 2, Ours: "x", Theirs: "y"},
			}},
		},
	})

	if p.currentFile != 0 || p.currentRegion != 0 {
		t.Error("SetConflicts should reset navigation")
	}
	if len(p.resolved) != 0 {
		t.Error("SetConflicts should reset resolved map")
	}
}
