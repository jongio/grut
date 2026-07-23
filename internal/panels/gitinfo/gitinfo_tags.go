// Tag-related operations for the gitinfo panel.
package gitinfo

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// ---------------------------------------------------------------------------
// Clipboard
// ---------------------------------------------------------------------------
// copyHashToClipboard copies the hash of the item under cursor to clipboard.
func (p *Panel) copyHashToClipboard() (panels.Panel, tea.Cmd) {
	item, ok := p.currentItem()
	if !ok {
		return p, nil
	}
	hash := item.hash
	if hash == "" {
		return p, nil
	}
	if err := panels.CopyToClipboard(p.ctx, hash); err != nil {
		errMsg := err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Copy failed: " + errMsg, Level: notify.Error}
		}
	}
	return p, func() tea.Msg {
		return notify.ShowToastMsg{Message: "Copied: " + hash, Level: notify.Success}
	}
}

// doTagPush prompts for push confirmation and pushes the tag under the cursor.
func (p *Panel) doTagPush() (panels.Panel, tea.Cmd) {
	item, ok := p.currentItem()
	if !ok {
		return p, nil
	}
	if item.kind != kindTag && item.kind != kindRemoteTag {
		return p, nil
	}
	tg := item.tag
	p.clearPending()
	p.pending = opTagPush
	p.pendingName = tg.Name
	return p, notify.ShowConfirm("Push Tag",
		fmt.Sprintf("Push tag %q to origin?", tg.Name))
}

// doTagDelete prompts for delete confirmation and deletes the tag under the cursor.
func (p *Panel) doTagDelete() (panels.Panel, tea.Cmd) {
	item, ok := p.currentItem()
	if !ok {
		return p, nil
	}
	if item.kind == kindRemoteTag {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Cannot delete remote-only tag locally", Level: notify.Warn}
		}
	}
	if item.kind != kindTag {
		return p, nil
	}
	tg := item.tag
	p.clearPending()
	p.pending = opTagDelete
	p.pendingName = tg.Name
	return p, notify.ShowConfirm("Delete Tag", fmt.Sprintf("Delete tag %q?", tg.Name))
}

func (p *Panel) renderTag(item listItem, width int, isCursor bool) string {
	tg := item.tag
	prefix := "  "
	// Build right side — annotated badge + hash.
	rightSide := ""
	if tg.IsAnnotated {
		rightSide += " [annotated]"
	}
	if tg.Hash != "" {
		rightSide += " " + tg.Hash
	}
	// Calculate available width for the name — truncate name, never hash.
	prefixLen := len(prefix)
	rightLen := lipgloss.Width(rightSide)
	nameWidth := width - prefixLen - rightLen - 1 // -1 for min gap
	name := panels.StripANSI(tg.Name)
	if nameWidth > 0 && len(name) > nameWidth {
		if nameWidth > 1 {
			name = name[:nameWidth-1] + "…"
		} else {
			name = name[:nameWidth]
		}
	} else if nameWidth <= 0 {
		name = ""
	}
	leftSide := prefix + name
	usedWidth := lipgloss.Width(leftSide) + lipgloss.Width(rightSide)
	gap := ""
	if usedWidth < width {
		gap = strings.Repeat(" ", width-usedWidth)
	}
	line := leftSide + gap + rightSide
	// Color based on tag type.
	fg := p.colors.Tag
	if item.kind == kindRemoteTag {
		fg = p.colors.RemoteTag
	}
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Foreground(lipgloss.Color(fg))
	if isCursor {
		style = style.Background(lipgloss.Color(p.colors.CursorBg))
	}
	return style.Render(line)
}
