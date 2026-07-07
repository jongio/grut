package gitstatus

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/secretscan"
)

// secretGuardModeBlock refuses staging outright when a secret is detected,
// rather than offering a confirmation. Any other value (the default) warns.
const secretGuardModeBlock = "block"

// fileFindings groups the secret-scan findings for a single path.
type fileFindings struct {
	path     string
	findings []secretscan.Finding
}

// stageScanMsg carries the result of scanning files before they are staged.
type stageScanMsg struct {
	err     error
	paths   []string
	flagged []fileFindings
}

// stage is the guarded entry point for staging whole files. When the secret
// guard is disabled it stages immediately; otherwise it scans the working-tree
// content and filenames first and only stages once the scan is clear or the
// user confirms.
func (p *GitStatus) stage(paths []string) (panels.Panel, tea.Cmd) {
	if len(paths) == 0 {
		return p, nil
	}
	if !p.secretGuard {
		return p, p.stageCmd(paths)
	}
	return p, p.scanStageCmd(paths)
}

// scanStageCmd reads each path's working-tree content and scans it for
// secrets, returning a stageScanMsg with any findings.
func (p *GitStatus) scanStageCmd(paths []string) tea.Cmd {
	ctx := p.ctx
	client := p.git
	targets := append([]string(nil), paths...)
	return func() tea.Msg {
		var flagged []fileFindings
		for _, path := range targets {
			content, err := readForScan(ctx, client, path)
			if err != nil {
				// A read failure (for example a deleted or unreadable file)
				// should not silently bypass the guard: surface it so the
				// user can decide.
				return stageScanMsg{paths: targets, err: err}
			}
			if findings := secretscan.Scan(content, path); len(findings) > 0 {
				flagged = append(flagged, fileFindings{path: path, findings: findings})
			}
		}
		return stageScanMsg{paths: targets, flagged: flagged}
	}
}

// readForScan fetches the working-tree content for a path, tolerating a nil
// content result from the client.
func readForScan(ctx context.Context, client GitClient, path string) ([]byte, error) {
	data, err := client.WorktreeFile(ctx, path)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// handleStageScan acts on a completed pre-stage scan. With no findings it
// stages immediately. In block mode any finding refuses the stage; otherwise
// it asks the user to confirm.
func (p *GitStatus) handleStageScan(msg stageScanMsg) (panels.Panel, tea.Cmd) {
	if msg.err != nil {
		errText := msg.err.Error()
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Secret scan failed, staging cancelled: " + errText,
				Level:   notify.Error,
			}
		}
	}
	if len(msg.flagged) == 0 {
		return p, p.stageCmd(msg.paths)
	}

	summary := summarizeFindings(msg.flagged)
	if strings.EqualFold(p.secretGuardMode, secretGuardModeBlock) {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Staging blocked: possible secrets detected. " + summary,
				Level:   notify.Error,
			}
		}
	}

	p.clearPending()
	p.pendingOp = opStageGuard
	p.pendingStagePaths = msg.paths
	return p, notify.ShowConfirm("Possible secrets detected",
		summary+" Stage anyway?")
}

// summarizeFindings builds a short, value-free description of the findings for
// a confirmation or error message.
func summarizeFindings(flagged []fileFindings) string {
	var parts []string
	const maxFiles = 3
	for i, ff := range flagged {
		if i >= maxFiles {
			parts = append(parts, fmt.Sprintf("and %d more", len(flagged)-maxFiles))
			break
		}
		rules := ruleList(ff.findings)
		parts = append(parts, fmt.Sprintf("%s (%s)", filepath.Base(ff.path), rules))
	}
	return strings.Join(parts, ", ") + "."
}

// ruleList returns a comma-separated, de-duplicated list of rule ids.
func ruleList(findings []secretscan.Finding) string {
	seen := make(map[string]struct{}, len(findings))
	var rules []string
	for _, f := range findings {
		if _, dup := seen[f.Rule]; dup {
			continue
		}
		seen[f.Rule] = struct{}{}
		rules = append(rules, f.Rule)
	}
	return strings.Join(rules, ", ")
}
