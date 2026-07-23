package preview

import (
	"testing"

	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPRCommentComposerAnchorsSelectedDiffLine(t *testing.T) {
	t.Parallel()

	p := newTestPRCommentPreview()
	p.scrollY = 3

	_, cmd := p.Update(keyMsg("c"))
	require.NotNil(t, cmd)
	modal, ok := cmd().(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Equal(t, notify.ModalMultilineInput, modal.Kind)
	assert.Contains(t, modal.Title, "src/main.go:11")

	_, cmd = p.Update(notify.ModalResultMsg{Accept: true, Value: "Looks good"})
	require.NotNil(t, cmd)
	msg, ok := cmd().(panels.PostPRReviewCommentMsg)
	require.True(t, ok)
	assert.Equal(t, 42, msg.Number)
	assert.Equal(t, "src/main.go", msg.Path)
	assert.Equal(t, 11, msg.Line)
	assert.True(t, msg.HasLine)
	assert.Equal(t, "Looks good", msg.Body)
}

func TestPRCommentComposerFallbackWhenLineUnavailable(t *testing.T) {
	t.Parallel()

	p := newTestPRCommentPreview()
	p.scrollY = 2

	_, cmd := p.Update(keyMsg("c"))
	require.NotNil(t, cmd)
	modal, ok := cmd().(notify.ShowModalMsg)
	require.True(t, ok)
	assert.Contains(t, modal.Title, "src/main.go (no line anchor)")

	_, cmd = p.Update(notify.ModalResultMsg{Accept: true, Value: "Question"})
	require.NotNil(t, cmd)
	msg, ok := cmd().(panels.PostPRReviewCommentMsg)
	require.True(t, ok)
	assert.False(t, msg.HasLine)
	assert.Zero(t, msg.Line)
}

func TestPRCommentComposerRejectsEmptyBody(t *testing.T) {
	t.Parallel()

	p := newTestPRCommentPreview()
	p.scrollY = 3
	_, cmd := p.Update(keyMsg("c"))
	require.NotNil(t, cmd)
	_ = cmd()

	_, cmd = p.Update(notify.ModalResultMsg{Accept: true, Value: " \n\t "})
	require.NotNil(t, cmd)
	toast, ok := cmd().(notify.ShowToastMsg)
	require.True(t, ok)
	assert.Equal(t, notify.Warn, toast.Level)
	assert.Nil(t, p.pendingPRComment)
}

func TestPRCommentModalResultRequiresPendingComment(t *testing.T) {
	t.Parallel()

	p := newTestPRCommentPreview()
	p.pendingPRComment = nil

	_, cmd := p.Update(notify.ModalResultMsg{Accept: true, Value: "not posted"})
	assert.Nil(t, cmd)
}

func newTestPRCommentPreview() *Preview {
	p := newTestPreview([]string{"package main"})
	p.prContext = true
	p.prNumber = 42
	p.diffMode = true
	p.filePath = "src/main.go"
	p.diffLines = []string{
		"@@ -1,3 +10,4 @@",
		" context",
		"-old",
		"+new",
		" tail",
	}
	return p
}
