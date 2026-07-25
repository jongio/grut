// Modal result handlers for the gitinfo panel.
// Each handler corresponds to a pendingOp case extracted from handleModalResult.
package gitinfo

import (
	"context"
	"errors"
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

// errGitHubClientUnavailable is surfaced (as a failure toast) when a modal
// handler reaches a GitHub write with no client configured, instead of
// dereferencing a nil client and crashing the TUI.
var errGitHubClientUnavailable = errors.New("GitHub client unavailable")

// modalArgs holds the snapshot of pending-operation state captured before
// the switch in handleModalResult. Passing it explicitly keeps handler
// signatures uniform and avoids re-reading (already-cleared) panel fields.
type modalArgs struct {
	msg         notify.ModalResultMsg
	name        string
	pendingPath string
	issueTitle  string // captured new-issue draft title (opIssueCreate* flow)
	issueBody   string // captured new-issue draft body (opIssueCreate* flow)
	prCheckout  ghPRItem
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

func (p *Panel) handleBranchPull(a modalArgs, rebase bool) (panels.Panel, tea.Cmd) {
	branchName := a.name
	upstream := a.pendingPath
	remote, branch := splitBranchUpstream(upstream)
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic during branch pull", "branch", branchName, "panic", r)
			}
		}()
		err := g.Pull(ctx, git.PullOpts{Rebase: rebase, Remote: remote, Branch: branch})
		return opResultMsg{op: eventBranchPulled, name: branchName, err: err}
	}
}

func (p *Panel) handleBranchPush(a modalArgs) (panels.Panel, tea.Cmd) {
	branchName := a.name
	upstream := a.pendingPath
	remote, _ := splitBranchUpstream(upstream)
	setUpstream := false
	if remote == "" {
		remote = remoteOrigin
		setUpstream = true
	}
	g, ctx := a.git, a.ctx
	return p, func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic during branch push", "branch", branchName, "panic", r)
			}
		}()
		err := g.Push(ctx, git.PushOpts{Remote: remote, Branch: branchName, SetUpstream: setUpstream})
		return opResultMsg{op: eventBranchPushed, name: branchName, err: err}
	}
}

func splitBranchUpstream(upstream string) (string, string) {
	remote, branch, ok := strings.Cut(upstream, "/")
	if !ok {
		return "", ""
	}
	return remote, branch
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
		err := g.TagPush(ctx, remoteOrigin, tagName)
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
	return p, guardedGitHubCmd("gitinfo.getWorkflowInputs", func() tea.Msg {
		var wfInputs []ghclient.WorkflowInput
		inputsKnown := false
		if ghClient != nil && workflowPath != "" {
			fetched, err := ghClient.GetWorkflowInputs(ctx, owner, repo, workflowPath, ref)
			if err != nil {
				// Non-fatal: fall back to the free-form key=value composer.
				slog.Warn("github: fetch workflow inputs failed", "path", workflowPath, "err", err)
			} else {
				wfInputs = fetched
				inputsKnown = true
			}
		}
		return workflowInputsFetchedMsg{
			workflowID:   workflowID,
			workflowName: workflowName,
			ref:          ref,
			inputs:       wfInputs,
			inputsKnown:  inputsKnown,
		}
	})
}

// handleWorkflowDispatchInputs records the value entered (or picked) for the
// current workflow_dispatch input and advances to the next input, firing the
// dispatch once every input has been collected. An empty value keeps the
// workflow's declared default.
func (p *Panel) handleWorkflowDispatchInputs(a modalArgs) (panels.Panel, tea.Cmd) {
	d := &p.wfDispatch
	if d.idx >= len(d.inputs) {
		// Defensive: no input to record (stale state) — just fire.
		return p.fireWorkflowDispatch()
	}
	input := d.inputs[d.idx]
	if value := strings.TrimSpace(a.msg.Value); value != "" {
		if d.values == nil {
			d.values = make(map[string]any)
		}
		d.values[input.Name] = value
	}
	d.idx++
	return p.promptNextWorkflowInput()
}

// handleWorkflowDispatchRaw dispatches a workflow using free-form key=value
// inputs entered in the fallback composer (used when the workflow's declared
// inputs could not be read). pendingName format: "id:name:ref".
func (p *Panel) handleWorkflowDispatchRaw(a modalArgs) (panels.Panel, tea.Cmd) {
	var workflowID int64
	var workflowName, ref string
	parts := strings.SplitN(a.name, ":", 3)
	if len(parts) == 3 {
		workflowID, _ = strconv.ParseInt(parts[0], 10, 64)
		workflowName = parts[1]
		ref = parts[2]
	}
	return p, p.dispatchWorkflowCmd(workflowID, workflowName, ref, parseKeyValueInputs(a.msg.Value))
}

// parseKeyValueInputs parses free-form "key=value" lines into a dispatch input
// map. Blank lines and lines without an "=" are ignored. Returns nil when no
// valid pairs are present.
func parseKeyValueInputs(s string) map[string]any {
	text := strings.TrimSpace(s)
	if text == "" {
		return nil
	}
	inputs := make(map[string]any)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if kv := strings.SplitN(line, "=", 2); len(kv) == 2 {
			inputs[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	if len(inputs) == 0 {
		return nil
	}
	return inputs
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

// prCreateAbort resets the in-progress draft and shows a warning toast. It is
// used by the create-PR steps when validation fails.
func (p *Panel) prCreateAbort(message string) (panels.Panel, tea.Cmd) {
	p.prDraft = prCreateDraft{}
	return p, func() tea.Msg {
		return notify.ShowToastMsg{Message: message, Level: notify.Warn}
	}
}

// handlePRCreateHead captures the head branch, then prompts for the base.
func (p *Panel) handlePRCreateHead(a modalArgs) (panels.Panel, tea.Cmd) {
	head := strings.TrimSpace(a.msg.Value)
	if head == "" {
		return p.prCreateAbort("Cannot open PR: head branch is required")
	}
	p.prDraft.head = head
	p.pending = opPRCreateBase
	return p, notify.ShowInputWithValue("PR Base Branch", "base-branch", p.prDraft.base)
}

// handlePRCreateBase captures the base branch, guards head==base, then prompts
// for the title.
func (p *Panel) handlePRCreateBase(a modalArgs) (panels.Panel, tea.Cmd) {
	base := strings.TrimSpace(a.msg.Value)
	if base == "" {
		return p.prCreateAbort("Cannot open PR: base branch is required")
	}
	if base == p.prDraft.head {
		return p.prCreateAbort(fmt.Sprintf("Cannot open PR: head and base are both %q", base))
	}
	p.prDraft.base = base
	p.pending = opPRCreateTitle
	return p, notify.ShowInput("PR Title", "title")
}

// handlePRCreateTitle captures the required title, then prompts for the
// optional body.
func (p *Panel) handlePRCreateTitle(a modalArgs) (panels.Panel, tea.Cmd) {
	title := strings.TrimSpace(a.msg.Value)
	if title == "" {
		return p.prCreateAbort("Cannot open PR: title is required")
	}
	p.prDraft.title = title
	p.pending = opPRCreateBody
	return p, notify.ShowInput("PR Body", "(optional)")
}

// handlePRCreateBody captures the optional body and fires the create request.
func (p *Panel) handlePRCreateBody(a modalArgs) (panels.Panel, tea.Cmd) {
	body := strings.TrimSpace(a.msg.Value)
	head, base, title := p.prDraft.head, p.prDraft.base, p.prDraft.title
	p.prDraft = prCreateDraft{}
	if head == "" || base == "" || title == "" {
		// Defensive: state was lost somehow; abort quietly with a warning.
		return p.prCreateAbort("Cannot open PR: missing branch or title")
	}
	return p, p.createPRCmd(head, base, title, body)
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
