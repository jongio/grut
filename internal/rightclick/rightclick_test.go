package rightclick

import (
	"testing"

	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/notify"
)

func TestCmd_ContextMenu(t *testing.T) {
	cfg := config.ActionsConfig{} // empty = use defaults
	cmd, action := Cmd(cfg, actions.ItemCommit, "abc123 fix bug")
	if cmd == nil {
		t.Fatal("expected non-nil cmd for context menu")
	}
	if action != "" {
		t.Errorf("expected empty action for context menu, got %q", action)
	}
	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	if !ok {
		t.Fatalf("expected ShowModalMsg, got %T", msg)
	}
	if modal.Kind != notify.ModalActionPicker {
		t.Errorf("expected ModalActionPicker, got %d", modal.Kind)
	}
	if len(modal.Actions) == 0 {
		t.Error("expected non-empty actions list")
	}
}

func TestCmd_DirectAction(t *testing.T) {
	cfg := config.ActionsConfig{
		RightClick: map[string]string{
			string(actions.ItemCommit): string(actions.ActionCopyHash),
		},
	}
	cmd, action := Cmd(cfg, actions.ItemCommit, "abc123")
	if cmd != nil {
		t.Error("expected nil cmd for direct action")
	}
	if action != actions.ActionCopyHash {
		t.Errorf("expected %q, got %q", actions.ActionCopyHash, action)
	}
}

func TestFirstUseCmd_WithActions(t *testing.T) {
	cmd := FirstUseCmd(actions.ItemCommit)
	if cmd == nil {
		t.Fatal("expected non-nil cmd for item type with actions")
	}
	msg := cmd()
	modal, ok := msg.(notify.ShowModalMsg)
	if !ok {
		t.Fatalf("expected ShowModalMsg, got %T", msg)
	}
	if modal.Kind != notify.ModalActionPickerWithCheckbox {
		t.Errorf("expected ModalActionPickerWithCheckbox, got %d", modal.Kind)
	}
	if len(modal.Actions) == 0 {
		t.Error("expected non-empty actions list")
	}
	if modal.Message == "" {
		t.Error("expected non-empty message")
	}
	if modal.CheckboxLabel == "" {
		t.Error("expected non-empty checkbox label")
	}
}
