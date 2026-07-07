// Modal result handlers for the gitinfo panel.
// Each handler corresponds to a pendingOp case extracted from handleModalResult.
package gitinfo

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/actions"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	ghclient "github.com/jongio/grut/internal/github"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// modalArgs holds the snapshot of pending-operation state captured before
// the switch in handleModalResult. Passing it explicitly keeps handler
// signatures uniform and avoids re-reading (already-cleared) panel fields.
type modalArgs struct {
	msg         notify.ModalResultMsg
	name        string
	pendingPath string
	issueTitle  string // captured new-issue draft title (opIssueCreate* flow)
	issueBody   string // captured new-issue draft body (opIssueCreate* flow)
	git         gitOps
	ctx         context.Context
}

func (p *Panel) handleBranchCreate(a modalArgs) (panels.Panel, tea.Cmd) {
	newName := strings.TrimSpace(a.msg.Value)
	if newName == "" {
		return p, nil
	}
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		err := g.BranchCreate(ctx, newName, "")
		return opResultMsg{op: eventBranchCreated, name: newName, err: err}
	}
}

func (p *Panel) handleBranchDelete(a modalArgs) (panels.Panel, tea.Cmd) {
	name := a.name
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		err := g.BranchDelete(ctx, name, false)
		return opResultMsg{op: eventBranchDeleted, name: name, err: err}
	}
}

func (p *Panel) handleBranchRename(a modalArgs) (panels.Panel, tea.Cmd) {
	newName := strings.TrimSpace(a.msg.Value)
	if newName == "" || newName == a.name {
		return p, nil
	}
	name := a.name
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		err := g.BranchRename(ctx, name, newName)
		return opResultMsg{op: eventBranchRenamed, name: newName, err: err}
	}
}

func (p *Panel) handleWorktreeCreate(a modalArgs) (panels.Panel, tea.Cmd) {
	branch := strings.TrimSpace(a.msg.Value)
	if branch == "" {
		return p, nil
	}
	path := worktreePath(p.repoRoot, branch)
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		err := g.WorktreeAdd(ctx, path, branch)
		return opResultMsg{op: eventWorktreeAdded, name: branch, err: err}
	}
}

func (p *Panel) handleWorktreeDelete(a modalArgs) (panels.Panel, tea.Cmd) {
	name := a.name
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		err := g.WorktreeRemove(ctx, name, false)
		return opResultMsg{op: eventWorktreeRemoved, name: name, err: err}
	}
}

func (p *Panel) handleRemoteAdd(a modalArgs) (panels.Panel, tea.Cmd) {
	remoteName := strings.TrimSpace(a.msg.Value)
	if remoteName == "" {
		return p, nil
	}
	// Two-step: first get name, then URL.
	p.pending = opRemoteAddURL
	p.pendingName = remoteName
	return p, notify.ShowInput("Remote URL", "https://github.com/user/repo")
}

func (p *Panel) handleRemoteAddURL(a modalArgs) (panels.Panel, tea.Cmd) {
	url := strings.TrimSpace(a.msg.Value)
	if url == "" {
		return p, nil
	}
	remoteName := a.name
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		err := g.RemoteAdd(ctx, remoteName, url)
		return opResultMsg{op: eventRemoteAdded, name: remoteName, err: err}
	}
}

func (p *Panel) handleRemoteDelete(a modalArgs) (panels.Panel, tea.Cmd) {
	name := a.name
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		err := g.RemoteRemove(ctx, name)
		return opResultMsg{op: eventRemoteRemoved, name: name, err: err}
	}
}

func (p *Panel) handleBranchCheckout(a modalArgs) (panels.Panel, tea.Cmd) {
	ref := a.name
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic during branch checkout", "ref", ref, "panic", r)
			}
		}()
		files, err := g.Status(ctx)
		if err != nil {
			return checkoutDirtyMsg{ref: ref, err: err}
		}
		return checkoutDirtyMsg{ref: ref, dirty: len(files) > 0}
	}
}

func (p *Panel) handleBranchCheckoutStash(a modalArgs) (panels.Panel, tea.Cmd) {
	ref := a.name
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic during stash checkout", "ref", ref, "panic", r)
			}
		}()
		err := g.StashPush(ctx, git.StashOpts{Message: "grut: auto-stash before switching to " + ref})
		if err != nil {
			return opResultMsg{op: opCheckout, name: ref, err: fmt.Errorf("stash failed: %w", err)}
		}
		err = g.Checkout(ctx, ref)
		if err != nil {
			_ = g.StashPop(ctx, 0) // restore stash on checkout failure
			return opResultMsg{op: opCheckout, name: ref, err: err}
		}
		return opResultMsg{op: "checkout_stashed", name: ref}
	}
}

func (p *Panel) handleStashAction(a modalArgs) (panels.Panel, tea.Cmd) {
	action := strings.TrimSpace(strings.ToLower(a.msg.Value))
	idx, err := strconv.Atoi(a.name)
	if err != nil {
		return p, nil
	}
	g, ctx := a.git, a.ctx
	switch action {
	case actionApply, "a":
		return p, func() tea.Msg {
			err := g.StashApply(ctx, idx)
			return opResultMsg{op: eventStashApplied, name: fmt.Sprintf("stash@{%d}", idx), err: err}
		}
	case actionPop, "p":
		return p, func() tea.Msg {
			err := g.StashPop(ctx, idx)
			return opResultMsg{op: eventStashPopped, name: fmt.Sprintf("stash@{%d}", idx), err: err}
		}
	case actionDrop, "d":
		return p, func() tea.Msg {
			err := g.StashDrop(ctx, idx)
			return opResultMsg{op: eventStashDropped, name: fmt.Sprintf("stash@{%d}", idx), err: err}
		}
	default:
		return p, func() tea.Msg {
			return notify.ShowToastMsg{Message: "Unknown stash action: " + action, Level: notify.Warn}
		}
	}
}

func (p *Panel) handleFirstUseConfirm(a modalArgs) (panels.Panel, tea.Cmd) {
	if a.msg.Remember {
		config.SaveDoubleClickChoice(&p.actionsCfg, a.name, a.msg.Value)
	}
	p.pendingPath = a.pendingPath // restore for executeRightClickAction
	return p.executeRightClickAction(actions.ActionID(a.msg.Value))
}

func (p *Panel) handleRightClickPick(a modalArgs) (panels.Panel, tea.Cmd) {
	p.pendingPath = a.pendingPath // restore for executeRightClickAction
	return p.executeRightClickAction(actions.ActionID(a.msg.Value))
}

func (p *Panel) handleTagCreate(a modalArgs) (panels.Panel, tea.Cmd) {
	tagName := strings.TrimSpace(a.msg.Value)
	if tagName == "" {
		return p, nil
	}
	p.pending = opTagMessage
	p.pendingName = tagName
	return p, notify.ShowInput("Tag Message", "(leave empty for lightweight)")
}

func (p *Panel) handleTagMessage(a modalArgs) (panels.Panel, tea.Cmd) {
	tagName := a.name
	message := strings.TrimSpace(a.msg.Value)
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		err := g.TagCreate(ctx, tagName, "", message)
		return opResultMsg{op: eventTagCreated, name: tagName, err: err}
	}
}

func (p *Panel) handleTagDelete(a modalArgs) (panels.Panel, tea.Cmd) {
	name := a.name
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		err := g.TagDelete(ctx, name)
		return opResultMsg{op: eventTagDeleted, name: name, err: err}
	}
}

func (p *Panel) handleTagPush(a modalArgs) (panels.Panel, tea.Cmd) {
	tagName := a.name
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		err := g.TagPush(ctx, "origin", tagName)
		return opResultMsg{op: eventTagPushed, name: tagName, err: err}
	}
}

func (p *Panel) handleTagCheckout(a modalArgs) (panels.Panel, tea.Cmd) {
	name := a.name
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		err := g.Checkout(ctx, name)
		return opResultMsg{op: eventTagCheckout, name: name, err: err}
	}
}

func (p *Panel) handleWorkflowDispatch(a modalArgs) (panels.Panel, tea.Cmd) {
	// Step 1 complete: got the ref. Fetch workflow inputs before
	// showing the inputs dialog so we can pre-populate fields.
	ref := strings.TrimSpace(a.msg.Value)
	if ref == "" {
		ref = p.currentBranch()
	}
	// Parse workflow ID and name from pendingName ("id:name").
	var workflowID int64
	var workflowName string
	if parts := strings.SplitN(a.name, ":", 2); len(parts) == 2 {
		workflowID, _ = strconv.ParseInt(parts[0], 10, 64)
		workflowName = parts[1]
	}
	if workflowID == 0 {
		return p, nil
	}
	// Look up the workflow path from the cached items.
	var workflowPath string
	for _, item := range p.tabItems[tabWorkflows] {
		if item.kind == kindWorkflow && item.workflow.ID == workflowID {
			workflowPath = item.workflow.Path
			break
		}
	}
	// Fetch workflow_dispatch inputs asynchronously.
	owner, repo := p.gh.owner, p.gh.repo
	ghClient := p.gh.client
	ctx := a.ctx
	return p, func() tea.Msg {
		var wfInputs []ghclient.WorkflowInput
		if ghClient != nil && workflowPath != "" {
			fetched, err := ghClient.GetWorkflowInputs(ctx, owner, repo, workflowPath, ref)
			if err != nil {
				// Non-fatal: fall back to generic dialog.
				_ = err
			} else {
				wfInputs = fetched
			}
		}
		return workflowInputsFetchedMsg{
			workflowID:   workflowID,
			workflowName: workflowName,
			ref:          ref,
			inputs:       wfInputs,
		}
	}
}

func (p *Panel) handleWorkflowDispatchInputs(a modalArgs) (panels.Panel, tea.Cmd) {
	// Step 2 complete: got the inputs. Parse and dispatch.
	// pendingName format: "id:name:ref"
	var workflowID int64
	var workflowName, ref string
	parts := strings.SplitN(a.name, ":", 3)
	if len(parts) == 3 {
		workflowID, _ = strconv.ParseInt(parts[0], 10, 64)
		workflowName = parts[1]
		ref = parts[2]
	}
	if workflowID == 0 || ref == "" {
		return p, nil
	}
	// Parse inputs from "key=value" lines.
	var inputs map[string]any
	inputText := strings.TrimSpace(a.msg.Value)
	if inputText != "" {
		inputs = make(map[string]any)
		for _, line := range strings.Split(inputText, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if kv := strings.SplitN(line, "=", 2); len(kv) == 2 {
				inputs[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
		if len(inputs) == 0 {
			inputs = nil
		}
	}
	owner, repo := p.gh.owner, p.gh.repo
	ghClient := p.gh.client
	ctx := a.ctx
	return p, func() tea.Msg {
		err := ghClient.DispatchWorkflow(ctx, owner, repo, workflowID, ref, inputs)
		return workflowDispatchResultMsg{workflowName: workflowName, err: err}
	}
}

func (p *Panel) handlePRMergeStrategy(a modalArgs) (panels.Panel, tea.Cmd) {
	// User selected a merge strategy from the picker.
	// pendingName format: "number:headBranch:title"
	parts := strings.SplitN(a.name, ":", 3)
	if len(parts) < 3 {
		return p, nil
	}
	prNumber, _ := strconv.Atoi(parts[0])
	headBranch := parts[1]
	prTitle := parts[2]
	if prNumber == 0 {
		return p, nil
	}

	strategy := a.msg.Value // "merge", "squash", or "rebase"

	// Store merge details for the confirmation step.
	p.pending = opPRMergeConfirm
	p.pendingName = fmt.Sprintf("%d:%s:%s", prNumber, strategy, headBranch)

	label := mergeStrategyLabel(strategy)
	confirmMsg := fmt.Sprintf("Merge PR #%d %q using %s?", prNumber, prTitle, label)
	return p, notify.ShowConfirm("Confirm Merge", confirmMsg)
}

func (p *Panel) handlePRMergeConfirm(a modalArgs) (panels.Panel, tea.Cmd) {
	// User confirmed the merge. Execute it.
	// pendingName format: "number:strategy:headBranch"
	parts := strings.SplitN(a.name, ":", 3)
	if len(parts) < 3 {
		return p, nil
	}
	prNumber, _ := strconv.Atoi(parts[0])
	strategy := parts[1]
	headBranch := parts[2]
	if prNumber == 0 {
		return p, nil
	}
	return p, p.mergePRCmd(prNumber, strategy, headBranch)
}

func (p *Panel) handlePRDeleteBranchAfterMerge(a modalArgs) (panels.Panel, tea.Cmd) {
	// User confirmed post-merge branch deletion.
	branch := a.name
	if branch == "" {
		return p, nil
	}
	client := p.gh.client
	owner, repo := p.gh.owner, p.gh.repo
	g := a.git
	ctx := a.ctx
	return p, func() tea.Msg {
		remoteErr := client.DeleteBranch(ctx, owner, repo, branch)
		var localErr error
		if g != nil {
			localErr = g.BranchDelete(ctx, branch, false)
		}
		return prBranchDeleteResultMsg{
			branch:    branch,
			remoteErr: remoteErr,
			localErr:  localErr,
		}
	}
}

// handleIssuePRComment posts the composed comment body to the selected issue
// or PR. Empty bodies are rejected without hitting the API.
// pendingName format: "kind:number" (e.g. "issue:252" or "PR:100").
func (p *Panel) handleIssuePRComment(a modalArgs) (panels.Panel, tea.Cmd) {
	parts := strings.SplitN(a.name, ":", 2)
	if len(parts) < 2 {
		return p, nil
	}
	kind := parts[0]
	number, _ := strconv.Atoi(parts[1])
	if number == 0 {
		return p, nil
	}

	body := strings.TrimSpace(a.msg.Value)
	if body == "" {
		return p, func() tea.Msg {
			return notify.ShowToastMsg{
				Message: "Comment cannot be empty",
				Level:   notify.Warn,
			}
		}
	}

	return p, p.commentCmd(number, body, kind)
}

// handleIssueCreateTitle is step 1 of the new-issue flow. It validates the
// title and advances to the body step. An empty title is rejected with an
// inline message and the title prompt is re-shown so the flow stays recoverable.
func (p *Panel) handleIssueCreateTitle(a modalArgs) (panels.Panel, tea.Cmd) {
	title := strings.TrimSpace(a.msg.Value)
	if title == "" {
		p.pending = opIssueCreateTitle
		return p, tea.Batch(
			func() tea.Msg {
				return notify.ShowToastMsg{Message: "Issue title is required", Level: notify.Warn}
			},
			notify.ShowInput("New Issue — Title (required)", "Title cannot be empty"),
		)
	}
	p.gh.issueDraftTitle = title
	p.pending = opIssueCreateBody
	return p, notify.ShowInput("New Issue — Body", "Optional description (leave empty to skip)")
}

// handleIssueCreateBody is step 2 of the new-issue flow. It records the body
// and advances to the optional labels step.
func (p *Panel) handleIssueCreateBody(a modalArgs) (panels.Panel, tea.Cmd) {
	p.gh.issueDraftTitle = a.issueTitle
	p.gh.issueDraftBody = strings.TrimSpace(a.msg.Value)
	p.pending = opIssueCreateLabels
	return p, notify.ShowInput("New Issue — Labels", "comma,separated (optional)")
}

// handleIssueCreateLabels is step 3 of the new-issue flow. It parses the
// optional labels and creates the issue.
func (p *Panel) handleIssueCreateLabels(a modalArgs) (panels.Panel, tea.Cmd) {
	title := a.issueTitle
	if title == "" {
		return p, nil
	}
	return p, p.createIssueCmd(title, a.issueBody, parseIssueLabels(a.msg.Value))
}

func (p *Panel) handlePRRequestReviewers(a modalArgs) (panels.Panel, tea.Cmd) {
	// User submitted reviewer logins. Empty input cancels.
	reviewers := parseReviewerLogins(a.msg.Value)
	if len(reviewers) == 0 {
		return p, nil
	}
	prNumber, err := strconv.Atoi(a.name)
	if err != nil || prNumber == 0 {
		return p, nil
	}
	return p, p.requestReviewersCmd(prNumber, reviewers)
}
