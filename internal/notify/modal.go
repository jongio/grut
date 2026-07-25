package notify

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Package-level modal styles. Width-dependent properties (.Width) are
// applied per-render via lipgloss's copy-on-write, so these bases are
// never mutated.
var (
	modalTitleBase = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Align(lipgloss.Center)

	modalMsgBase = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CCCCCC"))

	modalHintBase = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Italic(true).
			Align(lipgloss.Center)

	modalBoxBase = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Background(lipgloss.Color("#1E1E2E")).
			Padding(1, 2)

	modalSelectedBtn = lipgloss.NewStyle().
				Background(lipgloss.Color("#7D56F4")).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).
				Padding(0, 3)

	modalNormalBtn = lipgloss.NewStyle().
			Background(lipgloss.Color("#444444")).
			Foreground(lipgloss.Color("#CCCCCC")).
			Padding(0, 3)

	modalCenterBase = lipgloss.NewStyle().Align(lipgloss.Center)

	modalInputBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#7D56F4")).
				Padding(0, 1)

	modalCheckIcon = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#BD93F9"))

	modalCheckLabelNormal = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#CCCCCC"))

	modalCheckLabelFocused = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF"))

	modalActionSelected = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true)

	modalActionNormal = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#CCCCCC"))

	modalActionCursor = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7D56F4")).
				Bold(true)
)

// modalState holds the state of an active modal dialog. Only one modal
// can be active at a time (enforced by the Manager).
type modalState struct {
	title         string
	message       string
	placeholder   string
	input         string         // current text input value (ModalInput only)
	checkboxLabel string         // label text for checkbox (ModalConfirmWithCheckbox only)
	actions       []ActionOption // available actions (ModalActionPicker only)
	kind          ModalKind
	cursor        int  // cursor position in input
	focusIdx      int  // focused element index for Tab navigation (ModalConfirmWithCheckbox: 0=Yes,1=No,2=Checkbox)
	actionCursor  int  // selected action index (ModalActionPicker only)
	selected      bool // true if "Yes" is highlighted (ModalConfirm / ModalConfirmWithCheckbox)
	checked       bool // checkbox state (ModalConfirmWithCheckbox only)
}

// handleKey processes a key press while the modal is active. Returns a
// tea.Cmd that produces a ModalResultMsg when the user confirms or
// cancels.
func (ms *modalState) handleKey(mgr *Manager, msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	switch ms.kind {
	case ModalConfirm:
		return ms.handleConfirmKey(mgr, key)
	case ModalInput:
		return ms.handleInputKey(mgr, key, msg)
	case ModalConfirmWithCheckbox:
		return ms.handleConfirmWithCheckboxKey(mgr, key)
	case ModalActionPicker:
		return ms.handleActionPickerKey(mgr, key)
	case ModalActionPickerWithCheckbox:
		return ms.handleActionPickerWithCheckboxKey(mgr, key)
	case ModalMultilineInput:
		return ms.handleMultilineInputKey(mgr, key, msg)
	}
	return nil
}

// handleConfirmKey handles key input for a confirmation modal.
func (ms *modalState) handleConfirmKey(mgr *Manager, key string) tea.Cmd {
	switch key {
	case "left", "h": //nolint:goconst // inline string is more readable here
		ms.selected = true // Yes
		return nil
	case "right", "l": //nolint:goconst // inline string is more readable here
		ms.selected = false // No
		return nil
	case "tab", "shift+tab": //nolint:goconst // inline string is more readable here
		ms.selected = !ms.selected
		return nil
	case "y", "Y":
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: true}
		}
	case "n", "N", "esc": //nolint:goconst // inline string is more readable here
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: false}
		}
	case "enter": //nolint:goconst // inline string is more readable here
		accept := ms.selected
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: accept}
		}
	}
	return nil
}

// handleConfirmWithCheckboxKey handles key input for a confirmation modal
// with a "remember this choice" checkbox. Tab cycles through Yes, No, and
// checkbox. Space toggles the checkbox regardless of focus. Enter confirms
// the highlighted button, or toggles the checkbox when the checkbox is
// focused. The Remember field in the result is true only when the user
// accepts and the checkbox is checked.
func (ms *modalState) handleConfirmWithCheckboxKey(mgr *Manager, key string) tea.Cmd {
	switch key {
	case "left", "h":
		ms.selected = true // Yes
		ms.focusIdx = 0
		return nil
	case "right", "l":
		ms.selected = false // No
		ms.focusIdx = 1
		return nil
	case "tab":
		ms.focusIdx = (ms.focusIdx + 1) % 3
		switch ms.focusIdx {
		case 0:
			ms.selected = true
		case 1:
			ms.selected = false
			// case 2: checkbox focused — selected stays as-is
		}
		return nil
	case "shift+tab":
		ms.focusIdx = (ms.focusIdx + 2) % 3 // go backwards
		switch ms.focusIdx {
		case 0:
			ms.selected = true
		case 1:
			ms.selected = false
		}
		return nil
	case " ", "space":
		ms.checked = !ms.checked
		return nil
	case "y", "Y":
		checked := ms.checked
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: true, Remember: checked}
		}
	case "n", "N", "esc":
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: false}
		}
	case "enter":
		if ms.focusIdx == 2 {
			// Focused on checkbox — toggle it
			ms.checked = !ms.checked
			return nil
		}
		accept := ms.selected
		checked := ms.checked
		mgr.dismissModal()
		if accept {
			return func() tea.Msg {
				return ModalResultMsg{Accept: true, Remember: checked}
			}
		}
		return func() tea.Msg {
			return ModalResultMsg{Accept: false}
		}
	}
	return nil
}

// handleInputKey handles key input for a text input modal.
func (ms *modalState) handleInputKey(mgr *Manager, key string, msg tea.KeyPressMsg) tea.Cmd {
	switch key {
	case "esc":
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: false}
		}
	case "enter":
		value := ms.input
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: true, Value: value}
		}
	case "backspace":
		if ms.cursor > 0 {
			runes := []rune(ms.input)
			ms.input = string(runes[:ms.cursor-1]) + string(runes[ms.cursor:])
			ms.cursor--
		}
		return nil
	case "left":
		if ms.cursor > 0 {
			ms.cursor--
		}
		return nil
	case "right":
		if ms.cursor < len([]rune(ms.input)) {
			ms.cursor++
		}
		return nil
	default:
		// Only process single-character printable input
		text := msg.Text
		if text != "" {
			runes := []rune(ms.input)
			newRunes := make([]rune, 0, len(runes)+len([]rune(text)))
			newRunes = append(newRunes, runes[:ms.cursor]...)
			newRunes = append(newRunes, []rune(text)...)
			newRunes = append(newRunes, runes[ms.cursor:]...)
			ms.input = string(newRunes)
			ms.cursor += len([]rune(text))
		}
		return nil
	}
}

// handleMultilineInputKey handles key input for a multi-line text composer.
// Enter inserts a newline, Ctrl+D submits the composed value, and Esc
// cancels. Backspace, arrow keys, and printable characters edit the text.
func (ms *modalState) handleMultilineInputKey(mgr *Manager, key string, msg tea.KeyPressMsg) tea.Cmd {
	switch key {
	case "esc":
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: false}
		}
	case "ctrl+d":
		value := ms.input
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: true, Value: value}
		}
	case "enter":
		ms.insertText("\n")
		return nil
	case "backspace":
		if ms.cursor > 0 {
			runes := []rune(ms.input)
			ms.input = string(runes[:ms.cursor-1]) + string(runes[ms.cursor:])
			ms.cursor--
		}
		return nil
	case "left":
		if ms.cursor > 0 {
			ms.cursor--
		}
		return nil
	case "right":
		if ms.cursor < len([]rune(ms.input)) {
			ms.cursor++
		}
		return nil
	default:
		ms.insertText(msg.Text)
		return nil
	}
}

// insertText inserts text at the current cursor position and advances the
// cursor. Empty text is a no-op.
func (ms *modalState) insertText(text string) {
	if text == "" {
		return
	}
	runes := []rune(ms.input)
	inserted := []rune(text)
	newRunes := make([]rune, 0, len(runes)+len(inserted))
	newRunes = append(newRunes, runes[:ms.cursor]...)
	newRunes = append(newRunes, inserted...)
	newRunes = append(newRunes, runes[ms.cursor:]...)
	ms.input = string(newRunes)
	ms.cursor += len(inserted)
}

// handleActionPickerKey handles key input for an action picker modal.
func (ms *modalState) handleActionPickerKey(mgr *Manager, key string) tea.Cmd {
	switch key {
	case "up", "k":
		if ms.actionCursor > 0 {
			ms.actionCursor--
		}
		return nil
	case "down", "j", "tab":
		if ms.actionCursor < len(ms.actions)-1 {
			ms.actionCursor++
		}
		return nil
	case "shift+tab":
		if ms.actionCursor > 0 {
			ms.actionCursor--
		}
		return nil
	case "enter":
		if len(ms.actions) == 0 {
			mgr.dismissModal()
			return func() tea.Msg {
				return ModalResultMsg{Accept: false}
			}
		}
		value := ms.actions[ms.actionCursor].ID
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: true, Value: value}
		}
	case "esc":
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: false}
		}
	default:
		return nil
	}
}

// handleActionPickerWithCheckboxKey handles key input for an action picker
// modal with a "remember this choice" checkbox. Up/Down navigate actions,
// Tab cycles between the action list and checkbox, Space toggles the
// checkbox, Enter confirms the selection.
func (ms *modalState) handleActionPickerWithCheckboxKey(mgr *Manager, key string) tea.Cmd {
	// focusIdx: 0 = action list, 1 = checkbox
	switch key {
	case "up", "k":
		if ms.focusIdx == 0 && ms.actionCursor > 0 {
			ms.actionCursor--
		}
		return nil
	case "down", "j":
		if ms.focusIdx == 0 && ms.actionCursor < len(ms.actions)-1 {
			ms.actionCursor++
		}
		return nil
	case "tab":
		ms.focusIdx = (ms.focusIdx + 1) % 2
		return nil
	case "shift+tab":
		ms.focusIdx = (ms.focusIdx + 1) % 2
		return nil
	case " ", "space":
		ms.checked = !ms.checked
		return nil
	case "enter":
		if ms.focusIdx == 1 {
			// Focused on checkbox — toggle it
			ms.checked = !ms.checked
			return nil
		}
		if len(ms.actions) == 0 {
			mgr.dismissModal()
			return func() tea.Msg {
				return ModalResultMsg{Accept: false}
			}
		}
		value := ms.actions[ms.actionCursor].ID
		checked := ms.checked
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: true, Value: value, Remember: checked}
		}
	case "esc":
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: false}
		}
	default:
		return nil
	}
}

// handleMouseClick processes a mouse click while the modal is active.
// mouseX and mouseY are terminal-absolute coordinates. screenWidth and
// screenHeight are the full terminal dimensions (needed to calculate the
// centred modal position). Returns a tea.Cmd that produces a
// ModalResultMsg when a clickable element is activated.
func (ms *modalState) handleMouseClick(mgr *Manager, mouseX, mouseY, screenWidth, screenHeight int) tea.Cmd {
	// Input modals only support keyboard interaction.
	if ms.kind == ModalInput || ms.kind == ModalMultilineInput {
		return nil
	}
	// Compute the box width identically to view().
	boxWidth := 50
	if boxWidth > screenWidth-4 {
		boxWidth = screenWidth - 4
	}
	if boxWidth < 20 {
		boxWidth = 20
	}
	cw := boxWidth - 6 // text/content width: boxWidth minus border (1+1) minus padding (2+2)
	// Measure the title and message heights using the same styles as
	// view() so the line offsets match exactly.
	titleStyle := modalTitleBase.Width(cw)
	msgStyle := modalMsgBase.Width(cw)
	titleH := lipgloss.Height(titleStyle.Render(ms.title))
	msgH := lipgloss.Height(msgStyle.Render(ms.message))
	// The header section: title + blank + message + blank.
	//
	// Line layout (0-indexed inside the content area):
	//   0..titleH-1     title
	//   titleH          blank  (\n\n after title)
	//   titleH+1..+msgH message
	//   titleH+1+msgH   blank  (\n\n after message)
	//
	// Kind-specific content starts at headerLines.
	headerLines := titleH + 1 + msgH + 1
	// Rebuild the full content string so we can render it through the
	// box style and obtain the exact rendered dimensions.
	var content strings.Builder
	content.WriteString(titleStyle.Render(ms.title))
	content.WriteString("\n\n")
	content.WriteString(msgStyle.Render(ms.message))
	content.WriteString("\n\n")
	switch ms.kind { //nolint:exhaustive // only relevant cases handled
	case ModalConfirm:
		content.WriteString(ms.renderConfirmButtons(cw))
	case ModalConfirmWithCheckbox:
		content.WriteString(ms.renderCheckbox(cw))
		content.WriteString("\n")
		hintStyle := modalHintBase.Width(cw)
		content.WriteString(hintStyle.Render("tab cycle • space toggle • settings (,)"))
		content.WriteString("\n\n")
		content.WriteString(ms.renderConfirmButtons(cw))
	case ModalActionPicker:
		content.WriteString(ms.renderActionPicker(cw))
		content.WriteString("\n\n")
		hintStyle := modalHintBase.Width(cw)
		content.WriteString(hintStyle.Render("↑↓ navigate • enter select • esc cancel"))
	case ModalActionPickerWithCheckbox:
		content.WriteString(ms.renderActionPicker(cw))
		content.WriteString("\n")
		content.WriteString(ms.renderActionPickerCheckbox(cw))
		content.WriteString("\n")
		hintStyle := modalHintBase.Width(cw)
		content.WriteString(hintStyle.Render("↑↓ navigate • tab checkbox • space toggle • esc cancel"))
		content.WriteString("\n")
		content.WriteString(hintStyle.Render("You can change this later in settings (,)"))
	}
	// Render through the same box style as view().
	boxStyle := modalBoxBase.Width(boxWidth)
	box := boxStyle.Render(content.String())
	bw := lipgloss.Width(box)
	bh := lipgloss.Height(box)
	// Compute centred position (must match view() exactly).
	padLeft := (screenWidth - bw) / 2
	padTop := (screenHeight - bh) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	if padTop < 0 {
		padTop = 0
	}
	// Content starts after border (1 row) + padding (1 row) = 2 rows from
	// the top of the box, and border (1 col) + padding (2 cols) = 3 cols
	// from the left.
	relY := mouseY - padTop - 2
	relX := mouseX - padLeft - 3
	if relX < 0 || relX >= cw || relY < 0 {
		return nil
	}
	switch ms.kind { //nolint:exhaustive // only relevant cases handled
	case ModalConfirm:
		// Buttons are on the first line after the header.
		if relY == headerLines {
			return ms.clickConfirmButton(mgr, relX, cw)
		}
	case ModalConfirmWithCheckbox:
		// Checkbox is on the first line after the header.
		if relY == headerLines {
			ms.checked = !ms.checked
			ms.focusIdx = 2
			return nil
		}
		// Buttons are 3 lines below the checkbox:
		//   checkbox   (headerLines)
		//   hint       (headerLines + 1)
		//   blank      (headerLines + 2)
		//   buttons    (headerLines + 3)
		if relY == headerLines+3 {
			return ms.clickConfirmWithCheckboxButton(mgr, relX, cw)
		}
	case ModalActionPicker:
		// Each action occupies one line starting at headerLines.
		actionIdx := relY - headerLines
		if actionIdx >= 0 && actionIdx < len(ms.actions) {
			value := ms.actions[actionIdx].ID
			mgr.dismissModal()
			return func() tea.Msg {
				return ModalResultMsg{Accept: true, Value: value}
			}
		}
	case ModalActionPickerWithCheckbox:
		// Actions occupy lines starting at headerLines.
		actionIdx := relY - headerLines
		if actionIdx >= 0 && actionIdx < len(ms.actions) {
			// Click on an action selects and confirms it.
			value := ms.actions[actionIdx].ID
			checked := ms.checked
			mgr.dismissModal()
			return func() tea.Msg {
				return ModalResultMsg{Accept: true, Value: value, Remember: checked}
			}
		}
		// Checkbox is 1 line after actions (blank line between is part of renderActionPicker).
		// Layout: actions (N lines) + \n (1 line from Join) + checkbox line
		checkboxLine := headerLines + len(ms.actions) + 1
		if relY == checkboxLine {
			ms.checked = !ms.checked
			ms.focusIdx = 1
			return nil
		}
	}
	return nil
}

// clickConfirmButton handles a click on the Yes/No button row of a
// ModalConfirm dialog. The left half of the row activates Yes, the right
// half activates No.
func (ms *modalState) clickConfirmButton(mgr *Manager, relX, contentWidth int) tea.Cmd {
	if relX < contentWidth/2 {
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: true}
		}
	}
	mgr.dismissModal()
	return func() tea.Msg {
		return ModalResultMsg{Accept: false}
	}
}

// clickConfirmWithCheckboxButton handles a click on the Yes/No button row
// of a ModalConfirmWithCheckbox dialog.
func (ms *modalState) clickConfirmWithCheckboxButton(mgr *Manager, relX, contentWidth int) tea.Cmd {
	checked := ms.checked
	if relX < contentWidth/2 {
		ms.focusIdx = 0
		mgr.dismissModal()
		return func() tea.Msg {
			return ModalResultMsg{Accept: true, Remember: checked}
		}
	}
	ms.focusIdx = 1
	mgr.dismissModal()
	return func() tea.Msg {
		return ModalResultMsg{Accept: false}
	}
}

// view renders the modal dialog centered on the screen with a dimmed
// background.
func (ms *modalState) view(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	// Modal box dimensions
	boxWidth := 50
	if boxWidth > width-4 {
		boxWidth = width - 4
	}
	if boxWidth < 20 {
		boxWidth = 20
	}
	// Build the modal content.
	// Content width = boxWidth minus border (1+1) minus padding (2+2).
	// In lipgloss v2, Width(n) sets the total rendered width including
	// border and padding, so the usable content area is n-6.
	cw := boxWidth - 6
	var content strings.Builder
	// Title
	titleStyle := modalTitleBase.Width(cw)
	content.WriteString(titleStyle.Render(ms.title))
	content.WriteString("\n\n")
	// Message
	msgStyle := modalMsgBase.Width(cw)
	content.WriteString(msgStyle.Render(ms.message))
	content.WriteString("\n\n")
	// Kind-specific content
	switch ms.kind {
	case ModalConfirm:
		content.WriteString(ms.renderConfirmButtons(cw))
	case ModalInput:
		content.WriteString(ms.renderInputField(cw))
	case ModalMultilineInput:
		content.WriteString(ms.renderMultilineInputField(cw))
		content.WriteString("\n")
		hintStyle := modalHintBase.Width(cw)
		content.WriteString(hintStyle.Render("enter newline • ctrl+d submit • esc cancel"))
	case ModalConfirmWithCheckbox:
		content.WriteString(ms.renderCheckbox(cw))
		content.WriteString("\n")
		hintStyle := modalHintBase.Width(cw)
		content.WriteString(hintStyle.Render("tab cycle • space toggle • settings (,)"))
		content.WriteString("\n\n")
		content.WriteString(ms.renderConfirmButtons(cw))
	case ModalActionPicker:
		content.WriteString(ms.renderActionPicker(cw))
		content.WriteString("\n\n")
		hintStyle := modalHintBase.Width(cw)
		content.WriteString(hintStyle.Render("↑↓ navigate • enter select • esc cancel"))
	case ModalActionPickerWithCheckbox:
		content.WriteString(ms.renderActionPicker(cw))
		content.WriteString("\n")
		content.WriteString(ms.renderActionPickerCheckbox(cw))
		content.WriteString("\n")
		hintStyle := modalHintBase.Width(cw)
		content.WriteString(hintStyle.Render("↑↓ navigate • tab checkbox • space toggle • esc cancel"))
		content.WriteString("\n")
		content.WriteString(hintStyle.Render("You can change this later in settings (,)"))
	}
	// Box style
	boxStyle := modalBoxBase.Width(boxWidth)
	box := boxStyle.Render(content.String())
	// Return the rendered box without manual centering. The caller
	// (app.View) uses lipgloss.Place to center the overlay, which must
	// match the coordinate math in handleMouseClick.
	return box
}

// renderConfirmButtons renders the Yes/No buttons for a confirm modal.
func (ms *modalState) renderConfirmButtons(width int) string {
	var yesBtn, noBtn string
	if ms.selected {
		yesBtn = modalSelectedBtn.Render("Yes")
		noBtn = modalNormalBtn.Render("No")
	} else {
		yesBtn = modalNormalBtn.Render("Yes")
		noBtn = modalSelectedBtn.Render("No")
	}
	buttons := yesBtn + "  " + noBtn
	return modalCenterBase.
		Width(width).
		Render(buttons)
}

// renderInputField renders the text input field for an input modal.
func (ms *modalState) renderInputField(width int) string {
	display := ms.input
	if display == "" && ms.placeholder != "" {
		display = ms.placeholder
	}
	inputStyle := modalInputBorder.Width(width - 4)
	return inputStyle.Render(display)
}

// renderMultilineInputField renders the text composer for a multi-line
// input modal. The field keeps a minimum height so it reads as a composer
// even when empty, and shows the placeholder until the user types.
func (ms *modalState) renderMultilineInputField(width int) string {
	const minLines = 4
	display := ms.input
	if display == "" && ms.placeholder != "" {
		display = ms.placeholder
	}
	lines := strings.Split(display, "\n")
	for len(lines) < minLines {
		lines = append(lines, "")
	}
	inputStyle := modalInputBorder.Width(width - 4)
	return inputStyle.Render(strings.Join(lines, "\n"))
}

// ShowConfirm returns a tea.Cmd that produces a ShowModalMsg for a
// yes/no confirmation dialog.
func ShowConfirm(title, message string) tea.Cmd {
	return func() tea.Msg {
		return ShowModalMsg{
			Title:   title,
			Message: message,
			Kind:    ModalConfirm,
		}
	}
}

// ShowInput returns a tea.Cmd that produces a ShowModalMsg for a text
// input dialog.
func ShowInput(title, placeholder string) tea.Cmd {
	return func() tea.Msg {
		return ShowModalMsg{
			Title:       title,
			Placeholder: placeholder,
			Kind:        ModalInput,
		}
	}
}

// ShowInputWithValue returns a tea.Cmd that produces a ShowModalMsg for a
// text input dialog pre-filled with the given value.
func ShowInputWithValue(title, placeholder, value string) tea.Cmd {
	return func() tea.Msg {
		return ShowModalMsg{
			Title:       title,
			Placeholder: placeholder,
			Value:       value,
			Kind:        ModalInput,
		}
	}
}

// ShowMultilineInput returns a tea.Cmd that produces a ShowModalMsg for a
// multi-line text composer. Enter inserts a newline, Ctrl+D submits, and
// Esc cancels.
func ShowMultilineInput(title, placeholder string) tea.Cmd {
	return func() tea.Msg {
		return ShowModalMsg{
			Title:       title,
			Placeholder: placeholder,
			Kind:        ModalMultilineInput,
		}
	}
}

// ShowMultilineInputWithValue returns a tea.Cmd that produces a ShowModalMsg
// for a multi-line text composer pre-filled with the given value. Enter
// inserts a newline, Ctrl+D submits, and Esc cancels.
func ShowMultilineInputWithValue(title, placeholder, value string) tea.Cmd {
	return func() tea.Msg {
		return ShowModalMsg{
			Title:       title,
			Placeholder: placeholder,
			Value:       value,
			Kind:        ModalMultilineInput,
		}
	}
}

// ShowConfirmWithCheckbox returns a tea.Cmd that produces a ShowModalMsg
// for a confirmation dialog with a "remember this choice" checkbox.
func ShowConfirmWithCheckbox(title, message, checkboxLabel string) tea.Cmd {
	return func() tea.Msg {
		return ShowModalMsg{
			Title:         title,
			Message:       message,
			Kind:          ModalConfirmWithCheckbox,
			CheckboxLabel: checkboxLabel,
		}
	}
}

// ShowActionPicker returns a tea.Cmd that produces a ShowModalMsg for an
// action picker dialog with a selectable list of actions.
func ShowActionPicker(title string, actions []ActionOption) tea.Cmd {
	return func() tea.Msg {
		return ShowModalMsg{
			Kind:    ModalActionPicker,
			Title:   title,
			Actions: actions,
		}
	}
}

// ShowActionPickerWithMessage returns a tea.Cmd that produces a ShowModalMsg
// for an action picker dialog with a title, subtitle message, and selectable
// list of actions.
func ShowActionPickerWithMessage(title, message string, actions []ActionOption) tea.Cmd {
	return func() tea.Msg {
		return ShowModalMsg{
			Kind:    ModalActionPicker,
			Title:   title,
			Message: message,
			Actions: actions,
		}
	}
}

// ShowActionPickerWithSelection returns a tea.Cmd that produces a
// ShowModalMsg for an action picker whose cursor starts on the action with
// the given ID. An empty or unknown selectedID starts on the first action.
func ShowActionPickerWithSelection(title, message string, actions []ActionOption, selectedID string) tea.Cmd {
	return func() tea.Msg {
		return ShowModalMsg{
			Kind:       ModalActionPicker,
			Title:      title,
			Message:    message,
			Actions:    actions,
			SelectedID: selectedID,
		}
	}
}

// actionIndexByID returns the index of the action with the given ID, or 0
// when the ID is empty or absent.
func actionIndexByID(actions []ActionOption, id string) int {
	if id == "" {
		return 0
	}
	for i, a := range actions {
		if a.ID == id {
			return i
		}
	}
	return 0
}

// renderCheckbox renders the checkbox toggle for a ConfirmWithCheckbox modal.
func (ms *modalState) renderCheckbox(width int) string {
	icon := "○" // unchecked
	if ms.checked {
		icon = "●" // checked
	}
	label := ms.checkboxLabel
	if label == "" {
		label = "Always perform this action"
	}
	// Show focus indicator when checkbox is focused
	prefix := "  "
	if ms.focusIdx == 2 {
		prefix = "▸ " // focused cursor
	}
	labelStyle := modalCheckLabelNormal
	// Use brighter label color when focused
	if ms.focusIdx == 2 {
		labelStyle = modalCheckLabelFocused
	}
	return lipgloss.NewStyle().
		Width(width).
		Render(prefix + modalCheckIcon.Render(icon) + " " + labelStyle.Render(label))
}

// renderActionPicker renders the selectable action list for an action picker modal.
func (ms *modalState) renderActionPicker(width int) string {
	var lines []string
	for i, action := range ms.actions {
		if i == ms.actionCursor {
			line := modalActionCursor.Render("▸ ") + modalActionSelected.Render(action.Label)
			lines = append(lines, line)
		} else {
			line := "  " + modalActionNormal.Render(action.Label)
			lines = append(lines, line)
		}
	}
	return lipgloss.NewStyle().
		Width(width).
		Render(strings.Join(lines, "\n"))
}

// renderActionPickerCheckbox renders the checkbox for an action picker with
// checkbox modal. focusIdx 1 = checkbox focused.
func (ms *modalState) renderActionPickerCheckbox(width int) string {
	icon := "○"
	if ms.checked {
		icon = "●"
	}
	label := ms.checkboxLabel
	if label == "" {
		label = "Always do this action on double click"
	}
	prefix := "  "
	if ms.focusIdx == 1 {
		prefix = "▸ "
	}
	labelStyle := modalCheckLabelNormal
	if ms.focusIdx == 1 {
		labelStyle = modalCheckLabelFocused
	}
	return lipgloss.NewStyle().
		Width(width).
		Render(prefix + modalCheckIcon.Render(icon) + " " + labelStyle.Render(label))
}

// ShowActionPickerWithCheckbox returns a tea.Cmd that produces a
// ShowModalMsg for an action picker dialog with a "remember this choice"
// checkbox.
func ShowActionPickerWithCheckbox(title, message, checkboxLabel string, actions []ActionOption) tea.Cmd {
	return func() tea.Msg {
		return ShowModalMsg{
			Kind:          ModalActionPickerWithCheckbox,
			Title:         title,
			Message:       message,
			CheckboxLabel: checkboxLabel,
			Actions:       actions,
		}
	}
}
