package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/panels"
)

const defaultNavHistoryLimit = 50

type navEntry struct {
	msg        tea.Msg
	kind       string
	key        string
	focusPanel string
}

type navHistory struct {
	entries []navEntry
	index   int
	limit   int
}

func (h *navHistory) record(entry navEntry) {
	limit := h.limit
	if limit <= 0 {
		limit = defaultNavHistoryLimit
	}
	if len(h.entries) > 0 && h.index >= 0 && h.index < len(h.entries) && h.entries[h.index].sameContext(entry) {
		return
	}
	if len(h.entries) > 0 && h.index >= 0 && h.index < len(h.entries)-1 {
		h.entries = h.entries[:h.index+1]
	}
	h.entries = append(h.entries, entry)
	if len(h.entries) > limit {
		drop := len(h.entries) - limit
		h.entries = h.entries[drop:]
	}
	h.index = len(h.entries) - 1
}

func (h *navHistory) back() (navEntry, bool) {
	if len(h.entries) == 0 || h.index <= 0 {
		return navEntry{}, false
	}
	h.index--
	return h.entries[h.index], true
}

func (h *navHistory) forward() (navEntry, bool) {
	if len(h.entries) == 0 || h.index >= len(h.entries)-1 {
		return navEntry{}, false
	}
	h.index++
	return h.entries[h.index], true
}

func (h *navHistory) reset() {
	h.entries = nil
	h.index = 0
}

func (m Model) restoreNavigation(entry navEntry) (tea.Model, tea.Cmd) {
	if entry.focusPanel != "" {
		m.engine.FocusByName(entry.focusPanel)
	}
	return m, m.engine.Update(entry.msg)
}

func (e navEntry) sameContext(other navEntry) bool {
	return e.kind == other.kind && e.key == other.key
}

func navEntryFromMsg(msg tea.Msg) (navEntry, bool) {
	switch msg := msg.(type) {
	case panels.FileSelectedMsg:
		return navEntry{
			msg:        msg,
			kind:       navKindFile,
			key:        fileSelectedKey(msg),
			focusPanel: panelFileTree,
		}, true
	case panels.CommitSelectedMsg:
		return navEntry{
			msg:        msg,
			kind:       navKindCommit,
			key:        msg.Hash,
			focusPanel: panelCommits,
		}, true
	case panels.BranchSelectedMsg:
		return navEntry{
			msg:        msg,
			kind:       navKindBranch,
			key:        msg.Name,
			focusPanel: panelBranches,
		}, true
	case panels.IssueSelectedMsg:
		return navEntry{
			msg:        msg,
			kind:       navKindIssue,
			key:        fmt.Sprint(msg.Number),
			focusPanel: panelGitHub,
		}, true
	case panels.PRSelectedMsg:
		return navEntry{
			msg:        msg,
			kind:       navKindPR,
			key:        fmt.Sprint(msg.Number),
			focusPanel: panelGitHub,
		}, true
	case panels.WorkflowSelectedMsg:
		return navEntry{
			msg:        msg,
			kind:       navKindWorkflow,
			key:        msg.Path + "\x00" + msg.Name,
			focusPanel: panelGitHub,
		}, true
	default:
		return navEntry{}, false
	}
}

func fileSelectedKey(msg panels.FileSelectedMsg) string {
	key := fmt.Sprintf("%s\x00%d", msg.Path, msg.Line)
	if msg.DiffContext == nil {
		return key
	}
	return fmt.Sprintf(
		"%s\x00%d\x00%s\x00%s\x00%t",
		key,
		msg.DiffContext.Type,
		msg.DiffContext.CommitA,
		msg.DiffContext.CommitB,
		msg.DiffContext.ThreeDot,
	)
}
