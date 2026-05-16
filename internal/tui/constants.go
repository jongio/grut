package tui

// Action name constants used in handleAction dispatching and AsyncOpDoneMsg descriptions.
const (
	actionPush  = "push"
	actionFetch = "fetch"
)

// Keyboard hint strings shown in the status bar.
const (
	hintFind     = "/:find"
	hintTabFocus = "Tab:focus"
	hintHelp     = "?:help"
	hintScroll   = "j/k:scroll"
)

// metaKeyMessage is the metadata key used when recording undo actions.
const metaKeyMessage = "message"
