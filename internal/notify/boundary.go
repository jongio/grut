package notify

import (
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/crashlog"
	"github.com/jongio/grut/internal/panels"
)

// SafeUpdate wraps a panel's Update call in a recovery boundary. If the
// panel panics, the panic is caught, logged via slog, and the original
// (pre-panic) panel is returned along with a command that produces a
// ShowToastMsg with the error details.
func SafeUpdate(p panels.Panel, msg tea.Msg) (result panels.Panel, cmd tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			errMsg := fmt.Sprintf("panel %q crashed during Update: %v", p.Title(), r)
			slog.Error(errMsg, "stack", stack)

			// Persist crash report for later filing via 'grut report'.
			report := crashlog.NewReport(r, []byte(stack), fmt.Sprintf("panel:%s:Update", p.Title()))
			if _, writeErr := crashlog.Write(report); writeErr != nil {
				slog.Warn("failed to write crash report", "error", writeErr)
			}

			result = p
			cmd = func() tea.Msg {
				return ShowToastMsg{
					Message: fmt.Sprintf("Panel crashed: %v", r),
					Level:   Error,
				}
			}
		}
	}()

	return p.Update(msg)
}

// SafeView wraps a panel's View call in a recovery boundary. If the
// panel panics, the panic is caught, logged via slog, and an error
// placeholder is returned instead of the panel content.
func SafeView(p panels.Panel, w, h int) (content string) {
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			errMsg := fmt.Sprintf("panel %q crashed during View: %v", p.Title(), r)
			slog.Error(errMsg, "stack", stack)

			// Persist crash report for later filing via 'grut report'.
			report := crashlog.NewReport(r, []byte(stack), fmt.Sprintf("panel:%s:View", p.Title()))
			if _, writeErr := crashlog.Write(report); writeErr != nil {
				slog.Warn("failed to write crash report", "error", writeErr)
			}

			content = renderErrorState(p.Title(), fmt.Sprintf("%v", r), w, h)
		}
	}()

	return p.View(w, h)
}

// renderErrorState produces a simple error display for a crashed panel.
func renderErrorState(panelTitle, errText string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	header := fmt.Sprintf("Panel crashed: %s", panelTitle)
	detail := fmt.Sprintf("Error: %s", errText)
	hint := "Press Ctrl+Z to restart panel"

	// Truncate to fit width (UTF-8 safe — F31)
	header = truncateUTF8(header, width)
	detail = truncateUTF8(detail, width)
	hint = truncateUTF8(hint, width)

	var b strings.Builder
	b.WriteString(header)
	b.WriteByte('\n')
	b.WriteString(detail)
	b.WriteString("\n\n")
	b.WriteString(hint)

	// Pad to fill height
	lines := 4 // header + detail + blank + hint
	for i := lines; i < height; i++ {
		b.WriteByte('\n')
	}

	return b.String()
}

// truncateUTF8 truncates s to at most maxRunes runes, preserving valid UTF-8
// boundaries. If the string is shorter, it is returned unchanged (F31).
func truncateUTF8(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes])
}
