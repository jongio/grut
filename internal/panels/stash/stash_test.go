package stash

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock git client
// ---------------------------------------------------------------------------

type mockGit struct {
	entries   []git.StashEntry
	listErr   error
	pushErr   error
	popErr    error
	applyErr  error
	dropErr   error
	cherryErr error
	showDiff  string
	showErr   error

	// Recorded calls for verification.
	pushOpts   git.StashOpts
	popIndex   int
	applyIndex int
	dropIndex  int
	cherryHash string
	dropCalls  int
	showIndex  int
}

func (m *mockGit) StashList(_ context.Context) ([]git.StashEntry, error) {
	return m.entries, m.listErr
}

func (m *mockGit) StashPush(_ context.Context, opts git.StashOpts) error {
	m.pushOpts = opts
	return m.pushErr
}

func (m *mockGit) StashPop(_ context.Context, index int) error {
	m.popIndex = index
	return m.popErr
}

func (m *mockGit) StashApply(_ context.Context, index int) error {
	m.applyIndex = index
	return m.applyErr
}

func (m *mockGit) StashDrop(_ context.Context, index int) error {
	m.dropIndex = index
	m.dropCalls++
	return m.dropErr
}

func (m *mockGit) CherryPick(_ context.Context, hash string) error {
	m.cherryHash = hash
	return m.cherryErr
}

func (m *mockGit) StashShow(_ context.Context, index int) (string, error) {
	m.showIndex = index
	return m.showDiff, m.showErr
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sampleEntries returns a set of stash entries for testing.
func sampleEntries() []git.StashEntry {
	return []git.StashEntry{
		{Index: 0, Message: "On main: WIP on fuzzy finder", Hash: "aaa111", Date: time.Now().Add(-2 * time.Hour)},
		{Index: 1, Message: "On feature: save before rebase", Hash: "bbb222", Date: time.Now().Add(-24 * time.Hour)},
		{Index: 2, Message: "On dev: experiments", Hash: "ccc333"},
	}
}

// newTestPanel creates a Panel pre-loaded with entries for testing.
func newTestPanel(t *testing.T, mg *mockGit) *Panel {
	t.Helper()
	p := New(mg, nil)
	p.entries = mg.entries
	p.SetSize(80, 24)
	return p
}

// runCmd executes a tea.Cmd and returns the resulting message.
func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	return cmd()
}

// ---------------------------------------------------------------------------
// Compile-time checks
// ---------------------------------------------------------------------------

func TestPanelImplementsPanel(t *testing.T) {
	var _ panels.Panel = (*Panel)(nil)
}

// ---------------------------------------------------------------------------
// Construction and Init
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	mg := &mockGit{}
	p := New(mg, nil)
	assert.Equal(t, "stash (0)", p.Title())
	assert.NotNil(t, p.KeyBindings())
	assert.Empty(t, p.entries)
}

func TestInit(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := New(mg, nil)
	cmd := p.Init(context.Background())
	require.NotNil(t, cmd)

	msg := runCmd(t, cmd)
	loaded, ok := msg.(stashLoadedMsg)
	require.True(t, ok, "expected stashLoadedMsg")
	assert.Len(t, loaded.entries, 3)
	assert.NoError(t, loaded.err)
}

func TestInitError(t *testing.T) {
	mg := &mockGit{listErr: errors.New("git not found")}
	p := New(mg, nil)
	cmd := p.Init(context.Background())
	msg := runCmd(t, cmd)
	loaded, ok := msg.(stashLoadedMsg)
	require.True(t, ok)
	assert.Error(t, loaded.err)
}

// ---------------------------------------------------------------------------
// View rendering
// ---------------------------------------------------------------------------

func TestViewEmptyStash(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	view := p.View(80, 24)
	assert.Contains(t, view, "No stash entries")
}

func TestViewZeroSize(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	assert.Empty(t, p.View(0, 24))
	assert.Empty(t, p.View(80, 0))
	assert.Empty(t, p.View(-1, 10))
}

func TestViewWithEntries(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	view := p.View(80, 24)
	assert.Contains(t, view, "stash@{0}")
	assert.Contains(t, view, "WIP on fuzzy finder")
	assert.Contains(t, view, "stash@{1}")
	assert.Contains(t, view, "save before rebase")
	assert.Contains(t, view, "stash@{2}")
}

func TestViewWithDateFormatting(t *testing.T) {
	mg := &mockGit{entries: []git.StashEntry{
		{Index: 0, Message: "test", Hash: "abc", Date: time.Now().Add(-2 * time.Hour)},
	}}
	p := newTestPanel(t, mg)
	view := p.View(80, 5)
	assert.Contains(t, view, "hours ago")
}

// ---------------------------------------------------------------------------
// Keyboard navigation
// ---------------------------------------------------------------------------

func TestNavigateDown(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()
	assert.Equal(t, 0, p.cursor)

	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 1, p.cursor)

	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 2, p.cursor)

	// Should not go past the end.
	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 2, p.cursor)
}

func TestNavigateUp(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()
	p.cursor = 2

	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 1, p.cursor)

	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 0, p.cursor)

	// Should not go past the beginning.
	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 0, p.cursor)
}

func TestNavigateUnfocused(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	// Panel is not focused — keys should be ignored.
	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 0, p.cursor)
}

// ---------------------------------------------------------------------------
// Apply operation
// ---------------------------------------------------------------------------

func TestApply(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'a'})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)
	done, ok := msg.(stashOpDoneMsg)
	require.True(t, ok, "expected stashOpDoneMsg")
	assert.NoError(t, done.err)
	assert.Contains(t, done.action, "Applied stash@{0}")
	assert.Equal(t, 0, mg.applyIndex)
}

func TestApplyWithEnter(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)
	done, ok := msg.(stashOpDoneMsg)
	require.True(t, ok)
	assert.NoError(t, done.err)
	assert.Equal(t, 0, mg.applyIndex)
}

func TestApplyEmptyStash(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'a'})
	assert.Nil(t, cmd)
}

func TestApplyError(t *testing.T) {
	mg := &mockGit{entries: sampleEntries(), applyErr: errors.New("conflict")}
	p := newTestPanel(t, mg)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'a'})
	msg := runCmd(t, cmd)
	done, ok := msg.(stashOpDoneMsg)
	require.True(t, ok)
	assert.Error(t, done.err)
}

// ---------------------------------------------------------------------------
// Pop operation
// ---------------------------------------------------------------------------

func TestPop(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'p'})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)
	done, ok := msg.(stashOpDoneMsg)
	require.True(t, ok)
	assert.NoError(t, done.err)
	assert.Contains(t, done.action, "Popped stash@{0}")
	assert.Equal(t, 0, mg.popIndex)
}

func TestPopSelectedEntry(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()
	p.cursor = 1

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'p'})
	msg := runCmd(t, cmd)
	done, ok := msg.(stashOpDoneMsg)
	require.True(t, ok)
	assert.Equal(t, 1, mg.popIndex)
	assert.Contains(t, done.action, "stash@{1}")
}

func TestPopError(t *testing.T) {
	mg := &mockGit{entries: sampleEntries(), popErr: errors.New("merge conflict")}
	p := newTestPanel(t, mg)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'p'})
	msg := runCmd(t, cmd)
	done := msg.(stashOpDoneMsg)
	assert.Error(t, done.err)
}

// ---------------------------------------------------------------------------
// Drop operation (with modal confirmation)
// ---------------------------------------------------------------------------

func TestDropShowsConfirmation(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'd'})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)

	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "expected ShowModalMsg")
	assert.Equal(t, notify.ModalConfirm, modal.Kind)
	assert.Contains(t, modal.Message, "stash@{0}")
	assert.NotNil(t, p.pending)
}

func TestDropConfirmed(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()

	// Trigger drop (sets pending).
	p.Update(tea.KeyPressMsg{Code: 'd'})

	// Confirm modal.
	_, cmd := p.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)
	done, ok := msg.(stashOpDoneMsg)
	require.True(t, ok)
	assert.NoError(t, done.err)
	assert.Contains(t, done.action, "Dropped stash@{0}")
	assert.Equal(t, 0, mg.dropIndex)
}

func TestDropCancelled(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()

	p.Update(tea.KeyPressMsg{Code: 'd'})

	_, cmd := p.Update(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd)
	assert.Nil(t, p.pending)
}

func TestDropEmptyStash(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'd'})
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Drop All operation (with modal confirmation)
// ---------------------------------------------------------------------------

func TestDropAllShowsConfirmation(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'D'})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)

	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, notify.ModalConfirm, modal.Kind)
	assert.Contains(t, modal.Message, "3 stash entries")
}

func TestDropAllConfirmed(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()

	p.Update(tea.KeyPressMsg{Code: 'D'})

	_, cmd := p.Update(notify.ModalResultMsg{Accept: true})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)
	done, ok := msg.(stashOpDoneMsg)
	require.True(t, ok)
	assert.NoError(t, done.err)
	assert.Contains(t, done.action, "Dropped all 3")
	assert.Equal(t, 3, mg.dropCalls)
}

func TestDropAllEmptyStash(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 'D'})
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Push operation (with input modal)
// ---------------------------------------------------------------------------

func TestPushShowsInputModal(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()

	_, cmd := p.Update(tea.KeyPressMsg{Code: 's'})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)

	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, notify.ModalInput, modal.Kind)
	assert.Equal(t, "WIP", modal.Placeholder)
	assert.NotNil(t, p.pending)
}

func TestPushConfirmed(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()

	p.Update(tea.KeyPressMsg{Code: 's'})

	_, cmd := p.Update(notify.ModalResultMsg{Accept: true, Value: "save progress"})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)
	done, ok := msg.(stashOpDoneMsg)
	require.True(t, ok)
	assert.NoError(t, done.err)
	assert.Equal(t, "save progress", mg.pushOpts.Message)
	assert.Equal(t, "Changes stashed", done.action)
}

func TestPushError(t *testing.T) {
	mg := &mockGit{pushErr: errors.New("no local changes")}
	p := newTestPanel(t, mg)
	p.Focus()

	p.Update(tea.KeyPressMsg{Code: 's'})

	_, cmd := p.Update(notify.ModalResultMsg{Accept: true, Value: "msg"})
	msg := runCmd(t, cmd)
	done := msg.(stashOpDoneMsg)
	assert.Error(t, done.err)
	assert.Contains(t, done.err.Error(), "no local changes")
}

func TestPushCancelled(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()

	p.Update(tea.KeyPressMsg{Code: 's'})

	_, cmd := p.Update(notify.ModalResultMsg{Accept: false})
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Cherry-pick message handling
// ---------------------------------------------------------------------------

func TestCherryPick(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(panels.CherryPickMsg{Hash: "abc123def456"})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)

	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg")
	assert.Equal(t, notify.Success, toast.Level)
	assert.Contains(t, toast.Message, "abc123d")
	assert.Equal(t, "abc123def456", mg.cherryHash)
}

func TestCherryPickShortHash(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(panels.CherryPickMsg{Hash: "abc"})
	msg := runCmd(t, cmd)

	toast := msg.(notify.ShowToastMsg)
	assert.Contains(t, toast.Message, "abc")
}

func TestCherryPickError(t *testing.T) {
	mg := &mockGit{cherryErr: errors.New("conflict")}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(panels.CherryPickMsg{Hash: "abc123"})
	msg := runCmd(t, cmd)

	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "conflict")
}

// ---------------------------------------------------------------------------
// Preview (stash show) operation
// ---------------------------------------------------------------------------

func TestPreviewStashEntry(t *testing.T) {
	mg := &mockGit{
		entries: []git.StashEntry{
			{Index: 0, Message: "WIP on main", Hash: "abc1234"},
		},
		showDiff: "diff --git a/file.go b/file.go\n+added line\n-removed line",
	}
	p := newTestPanel(t, mg)
	p.Focus()

	// Press v to preview
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'v'})
	require.NotNil(t, cmd, "v key should trigger stash show command")

	msg := runCmd(t, cmd)
	show, ok := msg.(stashShowMsg)
	require.True(t, ok, "expected stashShowMsg")
	assert.NoError(t, show.err)
	assert.Equal(t, 0, mg.showIndex, "should show stash at index 0")
	assert.Contains(t, show.diff, "+added line")
}

func TestPreviewStashEntryError(t *testing.T) {
	mg := &mockGit{
		entries:  sampleEntries(),
		showErr:  errors.New("bad stash"),
		showDiff: "",
	}
	p := newTestPanel(t, mg)
	p.Focus()

	// Trigger show
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'v'})
	require.NotNil(t, cmd)

	// Execute the command, get the stashShowMsg
	msg := runCmd(t, cmd)
	show, ok := msg.(stashShowMsg)
	require.True(t, ok)
	assert.Error(t, show.err)

	// Feed the error message into Update
	_, toastCmd := p.Update(show)
	require.NotNil(t, toastCmd)
	toastMsg := runCmd(t, toastCmd)
	toast, ok := toastMsg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "Stash show:")
}

func TestPreviewShowMsgSetsState(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)

	diff := "diff --git a/main.go b/main.go\n+new code"
	p.Update(stashShowMsg{diff: diff})
	assert.True(t, p.showingPreview)
	assert.Equal(t, diff, p.previewContent)
	assert.Equal(t, 0, p.previewOffset)
}

func TestPreviewEscapeCloses(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()
	p.showingPreview = true
	p.previewContent = "some diff"

	p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.False(t, p.showingPreview)
	assert.Empty(t, p.previewContent)
	assert.Equal(t, 0, p.previewOffset)
}

func TestPreviewVToggle(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()
	p.showingPreview = true
	p.previewContent = "some diff"

	// Pressing v again should close the preview
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'v'})
	assert.Nil(t, cmd)
	assert.False(t, p.showingPreview)
	assert.Empty(t, p.previewContent)
}

func TestPreviewScrollDown(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()
	p.showingPreview = true
	p.previewContent = "line1\nline2\nline3"
	p.previewOffset = 0

	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 1, p.previewOffset)

	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 2, p.previewOffset)
}

func TestPreviewScrollUp(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()
	p.showingPreview = true
	p.previewContent = "line1\nline2\nline3"
	p.previewOffset = 2

	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 1, p.previewOffset)

	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 0, p.previewOffset)

	// Should not go below zero
	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 0, p.previewOffset)
}

func TestPreviewScrollDoesNotMoveCursor(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.Focus()
	p.showingPreview = true
	p.previewContent = "line1\nline2"
	p.cursor = 0

	// j/k should scroll preview, not move cursor
	p.Update(tea.KeyPressMsg{Code: 'j'})
	assert.Equal(t, 0, p.cursor, "cursor should not move during preview")
	assert.Equal(t, 1, p.previewOffset)

	p.Update(tea.KeyPressMsg{Code: 'k'})
	assert.Equal(t, 0, p.cursor, "cursor should not move during preview")
	assert.Equal(t, 0, p.previewOffset)
}

func TestPreviewRendersInView(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.showingPreview = true
	p.previewContent = "diff --git a/file.go b/file.go\n+added\n-removed"

	view := p.View(80, 10)
	assert.Contains(t, view, "added")
	assert.Contains(t, view, "removed")
	// Should NOT contain stash entry list
	assert.NotContains(t, view, "stash@{0}")
}

func TestPreviewEmptyStash(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)
	p.Focus()

	// v on empty stash should be a no-op
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'v'})
	assert.Nil(t, cmd)
}

func TestPreviewKeyBinding(t *testing.T) {
	mg := &mockGit{}
	p := New(mg, nil)
	bindings := p.KeyBindings()

	var found bool
	for _, b := range bindings {
		if b.Key == "v" && b.Action == "preview" {
			found = true
			break
		}
	}
	assert.True(t, found, "v key binding should be present")
}

// ---------------------------------------------------------------------------
// StashOpDoneMsg handling (reload + toast + StashChangedMsg)
// ---------------------------------------------------------------------------

func TestStashOpDoneReloads(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(stashOpDoneMsg{action: "Applied stash@{0}"})
	require.NotNil(t, cmd)

	// The batch should produce multiple messages.
	// Execute the batch and check that one of them is StashChangedMsg.
	msgs := executeBatch(t, cmd)

	var foundToast, foundChanged, foundLoaded bool
	for _, m := range msgs {
		switch m.(type) {
		case notify.ShowToastMsg:
			foundToast = true
		case panels.StashChangedMsg:
			foundChanged = true
		case stashLoadedMsg:
			foundLoaded = true
		}
	}
	assert.True(t, foundToast, "expected ShowToastMsg in batch")
	assert.True(t, foundChanged, "expected StashChangedMsg in batch")
	assert.True(t, foundLoaded, "expected stashLoadedMsg in batch")
}

func TestStashOpDoneError(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(stashOpDoneMsg{err: errors.New("oops")})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)

	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
	assert.Contains(t, toast.Message, "oops")
}

// ---------------------------------------------------------------------------
// Stash loaded message handling
// ---------------------------------------------------------------------------

func TestStashLoadedUpdatesEntries(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)

	p.Update(stashLoadedMsg{entries: sampleEntries()})
	assert.Len(t, p.entries, 3)
}

func TestStashLoadedError(t *testing.T) {
	mg := &mockGit{}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(stashLoadedMsg{err: errors.New("fail")})
	require.NotNil(t, cmd)
	msg := runCmd(t, cmd)

	toast, ok := msg.(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Error, toast.Level)
}

func TestStashLoadedClampsCursor(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.cursor = 10 // out of range

	p.Update(stashLoadedMsg{entries: sampleEntries()})
	assert.Equal(t, 2, p.cursor) // clamped to last entry
}

// ---------------------------------------------------------------------------
// Modal result with no pending (no-op)
// ---------------------------------------------------------------------------

func TestModalResultNoPending(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(notify.ModalResultMsg{Accept: true, Value: "test"})
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Focus / Blur
// ---------------------------------------------------------------------------

func TestFocusBlur(t *testing.T) {
	mg := &mockGit{}
	p := New(mg, nil)
	assert.False(t, p.Focused)

	p.Focus()
	assert.True(t, p.Focused)

	p.Blur()
	assert.False(t, p.Focused)
}

// ---------------------------------------------------------------------------
// timeAgo helper
// ---------------------------------------------------------------------------

func TestTimeAgo(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "just now"},
		{1 * time.Minute, "1 minute ago"},
		{5 * time.Minute, "5 minutes ago"},
		{1 * time.Hour, "1 hour ago"},
		{3 * time.Hour, "3 hours ago"},
		{24 * time.Hour, "1 day ago"},
		{72 * time.Hour, "3 days ago"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			result := timeAgo(time.Now().Add(-tt.d))
			assert.Equal(t, tt.want, result)
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// executeBatch runs a tea.Cmd that may be a tea.Batch, collecting all
// produced messages. If the cmd is a simple function, it returns a
// single-element slice.
func executeBatch(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}

	// tea.Batch returns a function that itself returns a tea.BatchMsg
	// (which is []tea.Cmd). We need to handle both cases.
	msg := cmd()
	if msg == nil {
		return nil
	}

	// Check if the result is a BatchMsg ([]tea.Cmd).
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			if c != nil {
				if m := c(); m != nil {
					msgs = append(msgs, m)
				}
			}
		}
		return msgs
	}

	return []tea.Msg{msg}
}

// ---------------------------------------------------------------------------
// Mouse handling tests
// ---------------------------------------------------------------------------

func TestMouseClick_SelectsEntry(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)

	assert.Equal(t, 0, p.cursor)

	// Click on row 1 (second entry).
	p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursor)

	// Click on row 2 (third entry).
	p.Update(panels.PanelMouseClickMsg{ContentRow: 2, ContentCol: 5})
	assert.Equal(t, 2, p.cursor)
}

func TestMouseClick_OutOfBoundsIgnored(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)

	p.Update(panels.PanelMouseClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, 0, p.cursor, "out-of-bounds click should not move cursor")
}

func TestMouseClick_IgnoredDuringPreview(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.showingPreview = true

	p.Update(panels.PanelMouseClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 0, p.cursor, "click during preview should not move cursor")
}

func TestMouseDoubleClick_AppliesEntry(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	// Pre-confirm so the first-use prompt is skipped.
	p.actionsCfg.Confirmed = map[string]bool{string(actions.ItemStash): true}

	// Double-click on row 1 (second entry, index 1).
	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursor)
	require.NotNil(t, cmd, "double-click should trigger apply command")

	msg := runCmd(t, cmd)
	result, ok := msg.(stashOpDoneMsg)
	require.True(t, ok, "expected stashOpDoneMsg, got %T", msg)
	assert.NoError(t, result.err)
	assert.Equal(t, 1, mg.applyIndex)
}

func TestMouseDoubleClick_FirstUseShowsConfirm(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	// No confirmations — should prompt first.

	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 1, p.cursor)
	require.NotNil(t, cmd, "first-use double-click should produce a command")
	msg := runCmd(t, cmd)
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "expected ShowModalMsg, got %T", msg)
	assert.Equal(t, notify.ModalActionPickerWithCheckbox, modal.Kind)
}

func TestMouseDoubleClick_OutOfBoundsIgnored(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Equal(t, 0, p.cursor)
	assert.Nil(t, cmd)
}

func TestMouseDoubleClick_IgnoredDuringPreview(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)
	p.showingPreview = true

	_, cmd := p.Update(panels.PanelMouseDoubleClickMsg{ContentRow: 1, ContentCol: 5})
	assert.Equal(t, 0, p.cursor)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Right-click tests
// ---------------------------------------------------------------------------

func TestRightClickShowsActionPicker(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(panels.PanelMouseRightClickMsg{ContentRow: 1, ContentCol: 5})
	require.NotNil(t, cmd, "right-click on stash entry should produce a command")

	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	require.True(t, ok, "expected ShowModalMsg, got %T", msg)
	assert.Equal(t, notify.ModalActionPicker, modal.Kind)
}

func TestRightClickOutOfBounds(t *testing.T) {
	mg := &mockGit{entries: sampleEntries()}
	p := newTestPanel(t, mg)

	_, cmd := p.Update(panels.PanelMouseRightClickMsg{ContentRow: 99, ContentCol: 5})
	assert.Nil(t, cmd, "right-click out of bounds should return nil cmd")
}
