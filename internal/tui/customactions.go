package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/customactions"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

const customActionOutputLimit = 160

var runCustomAction = customactions.Run

type customActionDoneMsg struct {
	result customactions.Result
}

func (m Model) customActionForName(name string) (config.CustomAction, bool) {
	if m.cfg == nil {
		return config.CustomAction{}, false
	}
	for _, action := range m.cfg.CustomActions {
		if action.Name == name {
			return action, true
		}
	}
	return config.CustomAction{}, false
}

func (m Model) customActionForKey(key string) (config.CustomAction, bool) {
	if m.cfg == nil || key == "" {
		return config.CustomAction{}, false
	}
	for _, action := range m.cfg.CustomActions {
		if action.Key == key {
			return action, true
		}
	}
	return config.CustomAction{}, false
}

func (m Model) handleRunCustomAction(name string) (tea.Model, tea.Cmd) {
	action, ok := m.customActionForName(name)
	if !ok {
		return m, showWarnToast("Custom action not found: " + name)
	}
	if m.asyncOp != "" {
		return m, showWarnToast("Operation in progress: " + m.asyncOp)
	}
	if action.Confirm {
		m.pendingAction = pendingActionCustom
		m.pendingCustomAction = action.Name
		message := action.Prompt
		if strings.TrimSpace(message) == "" {
			message = action.Command
		}
		return m, notify.ShowConfirm("Run "+action.Name+"?", message)
	}
	return m.executeCustomAction(action.Name)
}

func (m Model) executeCustomAction(name string) (tea.Model, tea.Cmd) {
	action, ok := m.customActionForName(name)
	if !ok {
		return m, showWarnToast("Custom action not found: " + name)
	}
	if m.asyncOp != "" {
		return m, showWarnToast("Operation in progress: " + m.asyncOp)
	}

	m.asyncOp = "running " + action.Name + "..."
	ctx, cancel := context.WithCancel(m.ctx)
	m.asyncCancel = cancel
	defaultDir, err := os.Getwd()
	if err != nil {
		defaultDir = "."
	}
	shell := ""
	if m.cfg != nil {
		shell = m.cfg.Terminal.Shell
	}
	return m, func() tea.Msg {
		return customActionDoneMsg{
			result: runCustomAction(ctx, action, defaultDir, shell),
		}
	}
}

func (m Model) handleCustomActionDone(msg customActionDoneMsg) (tea.Model, tea.Cmd) {
	m.asyncOp = ""
	m.asyncCancel = nil

	result := msg.result
	level := notify.Success
	status := "succeeded"
	if result.Err != nil {
		level = notify.Error
		status = fmt.Sprintf("failed (%d)", result.ExitCode)
	}
	message := "Custom action " + result.Action.Name + " " + status
	output := truncateCustomActionOutput(result.Output, customActionOutputLimit)
	if output != "" {
		message += ": " + output
	}

	var cmds []tea.Cmd
	cmds = append(cmds, func() tea.Msg {
		return notify.ShowToastMsg{Message: message, Level: level}
	})
	if result.Err == nil {
		cmds = append(
			cmds,
			func() tea.Msg { return panels.RefreshGitStatusMsg{} },
			func() tea.Msg { return panels.RefreshPreviewMsg{} },
			func() tea.Msg { return panels.RefreshGitChangedFilesMsg{} },
			m.loadBranchInfo(),
		)
	}
	return m, tea.Batch(cmds...)
}

func truncateCustomActionOutput(output string, limit int) string {
	output = strings.TrimSpace(strings.ReplaceAll(output, "\r\n", "\n"))
	output = strings.ReplaceAll(output, "\r", "\n")
	output = strings.Join(strings.Fields(output), " ")
	if len(output) <= limit {
		return output
	}
	if limit <= 3 {
		return output[:limit]
	}
	return output[:limit-3] + "..."
}
