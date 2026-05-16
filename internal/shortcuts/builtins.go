package shortcuts

// Builtins returns all built-in shortcut definitions.
func Builtins() []Shortcut {
	return []Shortcut{
		scShortcut(),
		scpShortcut(),
		amendShortcut(),
		wipShortcut(),
		undoShortcut(),
		unstageShortcut(),
		rbShortcut(),
		syncShortcut(),
		pullShortcut(),
		upShortcut(),
		nbShortcut(),
		doneShortcut(),
		cleanupShortcut(),
		reviewShortcut(),
		prShortcut(),
		shipShortcut(),
		squashShortcut(),
		tidyShortcut(),
		nukeShortcut(),
		discardShortcut(),
		saveShortcut(),
		fixupShortcut(),
		freshShortcut(),
		renameShortcut(),
	}
}

// sc: stage all + commit with AI-generated message.
func scShortcut() Shortcut {
	return Shortcut{
		Name:        "sc",
		Description: "Stage all changes and commit with an AI-generated message",
		Builtin:     true,
		Confirm:     true,
		Steps: []Step{
			{Op: OpStage, Params: map[string]string{paramPaths: "."}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpCommit, Params: map[string]string{paramAIMessage: "true"}, OnFail: OnFailStop, AIAssist: true},
		},
	}
}

// scp: stage all + commit with AI message + push.
func scpShortcut() Shortcut {
	return Shortcut{
		Name:        "scp",
		Description: "Stage all, commit with AI message, and push",
		Builtin:     true,
		Confirm:     true,
		Steps: []Step{
			{Op: OpStage, Params: map[string]string{paramPaths: "."}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpCommit, Params: map[string]string{paramAIMessage: "true"}, OnFail: OnFailStop, AIAssist: true},
			{Op: OpPush, Params: map[string]string{}, OnFail: OnFailStop, AIAssist: false},
		},
	}
}

// amend: stage all + amend previous commit.
func amendShortcut() Shortcut {
	return Shortcut{
		Name:        "amend",
		Description: "Stage all changes and amend the previous commit",
		Builtin:     true,
		Confirm:     true,
		Steps: []Step{
			{Op: OpStage, Params: map[string]string{paramPaths: "."}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpCommit, Params: map[string]string{paramAmend: "true"}, OnFail: OnFailStop, AIAssist: false},
		},
	}
}

// wip: stage all + WIP commit.
func wipShortcut() Shortcut {
	return Shortcut{
		Name:        actionWip,
		Description: "Stage all changes and create a WIP commit",
		Builtin:     true,
		Confirm:     false,
		Steps: []Step{
			{Op: OpStage, Params: map[string]string{paramPaths: "."}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpCommit, Params: map[string]string{paramMessage: "WIP"}, OnFail: OnFailStop, AIAssist: false},
		},
	}
}

// undo: soft-reset HEAD~1.
func undoShortcut() Shortcut {
	return Shortcut{
		Name:        "undo",
		Description: "Undo the last commit (soft reset HEAD~1)",
		Builtin:     true,
		Confirm:     true,
		Steps: []Step{
			{Op: OpReset, Params: map[string]string{"ref": refHeadTilde1, "mode": resetModeSoft}, OnFail: OnFailStop, AIAssist: false},
		},
	}
}

// unstage: reset HEAD (unstage all).
func unstageShortcut() Shortcut {
	return Shortcut{
		Name:        actionUnstage,
		Description: "Unstage all staged changes",
		Builtin:     true,
		Confirm:     false,
		Steps: []Step{
			{Op: OpReset, Params: map[string]string{"ref": refHead, "mode": resetModeMixed}, OnFail: OnFailStop, AIAssist: false},
		},
	}
}

// rb: fetch + rebase onto remote/branch.
func rbShortcut() Shortcut {
	return Shortcut{
		Name:        "rb",
		Description: "Fetch and rebase onto a remote branch",
		Builtin:     true,
		Confirm:     true,
		Args: []Arg{
			{Name: paramRemote, Default: refOrigin, Prompt: "Remote name"},
			{Name: paramBranch, Default: "main", Prompt: "Branch to rebase onto", Required: true},
		},
		Steps: []Step{
			{Op: OpFetch, Params: map[string]string{paramRemote: placeholderRemote}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpRebase, Params: map[string]string{paramOnto: "{{remote}}/{{branch}}"}, OnFail: OnFailAsk, AIAssist: true},
		},
	}
}

// sync: fetch all + rebase upstream/main.
func syncShortcut() Shortcut {
	return Shortcut{
		Name:        "sync",
		Description: "Fetch all remotes and rebase onto upstream/main",
		Builtin:     true,
		Confirm:     true,
		Steps: []Step{
			{Op: OpFetch, Params: map[string]string{paramAll: "true"}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpRebase, Params: map[string]string{paramOnto: "upstream/main"}, OnFail: OnFailAsk, AIAssist: true},
		},
	}
}

// pull: fetch origin + rebase on current tracking branch.
func pullShortcut() Shortcut {
	return Shortcut{
		Name:        actionPull,
		Description: "Fetch from origin and rebase on tracking branch",
		Builtin:     true,
		Confirm:     false,
		Steps: []Step{
			{Op: OpFetch, Params: map[string]string{paramRemote: refOrigin}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpPull, Params: map[string]string{paramRebase: "true"}, OnFail: OnFailAsk, AIAssist: true},
		},
	}
}

// nb: create new branch from default branch.
func nbShortcut() Shortcut {
	return Shortcut{
		Name:        "nb",
		Description: "Create a new branch from the default branch",
		Builtin:     true,
		Confirm:     false,
		Args: []Arg{
			{Name: paramName, Prompt: "New branch name", Required: true},
			{Name: paramBase, Default: "main", Prompt: "Base branch"},
		},
		Steps: []Step{
			{Op: OpFetch, Params: map[string]string{paramRemote: refOrigin}, OnFail: OnFailContinue, AIAssist: false},
			{Op: OpBranch, Params: map[string]string{paramName: "{{name}}", paramBase: "origin/{{base}}"}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpCheckout, Params: map[string]string{"ref": "{{name}}"}, OnFail: OnFailStop, AIAssist: false},
		},
	}
}

// done: merge current branch to default + delete.
func doneShortcut() Shortcut {
	return Shortcut{
		Name:        "done",
		Description: "Merge current branch into default branch and delete it",
		Builtin:     true,
		Confirm:     true,
		Args: []Arg{
			{Name: paramTarget, Default: "main", Prompt: "Branch to merge into"},
		},
		Steps: []Step{
			{Op: OpCheckout, Params: map[string]string{"ref": placeholderTarget}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpMerge, Params: map[string]string{paramBranch: refPrevBranch}, OnFail: OnFailAsk, AIAssist: true},
			{Op: OpDelete, Params: map[string]string{paramBranch: refPrevBranch}, OnFail: OnFailContinue, AIAssist: false},
		},
	}
}

// cleanup: delete merged branches.
func cleanupShortcut() Shortcut {
	return Shortcut{
		Name:        "cleanup",
		Description: "Delete local branches that have been merged",
		Builtin:     true,
		Confirm:     true,
		Steps: []Step{
			{Op: OpFetch, Params: map[string]string{"prune": "true"}, OnFail: OnFailContinue, AIAssist: false},
			{Op: OpDelete, Params: map[string]string{paramMerged: "true"}, OnFail: OnFailContinue, AIAssist: false},
		},
	}
}

// up: quick update — fetch all + pull with rebase.
func upShortcut() Shortcut {
	return Shortcut{
		Name:        "up",
		Description: "Fetch all remotes and rebase current branch on its upstream",
		Builtin:     true,
		Confirm:     false,
		Steps: []Step{
			{Op: OpFetch, Params: map[string]string{paramAll: "true"}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpPull, Params: map[string]string{paramRebase: "true"}, OnFail: OnFailAsk, AIAssist: true},
		},
	}
}

// review: fetch + checkout a branch for review.
func reviewShortcut() Shortcut {
	return Shortcut{
		Name:        "review",
		Description: "Fetch and checkout a branch for code review",
		Builtin:     true,
		Confirm:     false,
		Args: []Arg{
			{Name: paramBranch, Prompt: "Branch to review", Required: true},
		},
		Steps: []Step{
			{Op: OpFetch, Params: map[string]string{paramRemote: refOrigin}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpCheckout, Params: map[string]string{"ref": "{{branch}}"}, OnFail: OnFailStop, AIAssist: false},
		},
	}
}

// pr: push current branch with --set-upstream for pull request.
func prShortcut() Shortcut {
	return Shortcut{
		Name:        "pr",
		Description: "Push current branch with set-upstream, ready for pull request",
		Builtin:     true,
		Confirm:     true,
		Args: []Arg{
			{Name: paramRemote, Default: refOrigin, Prompt: "Remote to push to"},
		},
		Steps: []Step{
			{Op: OpPush, Params: map[string]string{paramRemote: placeholderRemote, "set_upstream": "true"}, OnFail: OnFailStop, AIAssist: false},
		},
	}
}

// ship: merge current branch into target and push.
func shipShortcut() Shortcut {
	return Shortcut{
		Name:        "ship",
		Description: "Merge current branch into target branch and push",
		Builtin:     true,
		Confirm:     true,
		Args: []Arg{
			{Name: paramTarget, Default: "main", Prompt: "Branch to merge into"},
		},
		Steps: []Step{
			{Op: OpCheckout, Params: map[string]string{"ref": placeholderTarget}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpMerge, Params: map[string]string{paramBranch: refPrevBranch, "no_ff": "true"}, OnFail: OnFailAsk, AIAssist: true},
			{Op: OpPush, Params: map[string]string{}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpDelete, Params: map[string]string{paramBranch: refPrevBranch}, OnFail: OnFailContinue, AIAssist: false},
		},
	}
}

// squash: squash-merge current branch into target with AI commit message.
func squashShortcut() Shortcut {
	return Shortcut{
		Name:        "squash",
		Description: "Squash-merge current branch into target with AI commit message",
		Builtin:     true,
		Confirm:     true,
		Args: []Arg{
			{Name: paramTarget, Default: "main", Prompt: "Branch to squash into"},
		},
		Steps: []Step{
			{Op: OpCheckout, Params: map[string]string{"ref": placeholderTarget}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpMerge, Params: map[string]string{paramBranch: refPrevBranch, paramSquash: "true"}, OnFail: OnFailAsk, AIAssist: true},
			{Op: OpCommit, Params: map[string]string{paramAIMessage: "true"}, OnFail: OnFailStop, AIAssist: true},
			{Op: OpDelete, Params: map[string]string{paramBranch: refPrevBranch}, OnFail: OnFailContinue, AIAssist: false},
		},
	}
}

// tidy: fetch with prune + clean up merged branches.
func tidyShortcut() Shortcut {
	return Shortcut{
		Name:        "tidy",
		Description: "Prune stale remote refs and delete merged local branches",
		Builtin:     true,
		Confirm:     true,
		Steps: []Step{
			{Op: OpFetch, Params: map[string]string{paramAll: "true", "prune": "true"}, OnFail: OnFailContinue, AIAssist: false},
			{Op: OpDelete, Params: map[string]string{paramMerged: "true"}, OnFail: OnFailContinue, AIAssist: false},
		},
	}
}

// nuke: hard reset to HEAD, discarding all uncommitted changes.
func nukeShortcut() Shortcut {
	return Shortcut{
		Name:        "nuke",
		Description: "Discard ALL uncommitted changes (hard reset to HEAD)",
		Builtin:     true,
		Confirm:     true,
		Steps: []Step{
			{Op: OpReset, Params: map[string]string{"ref": refHead, "mode": resetModeHard}, OnFail: OnFailStop, AIAssist: false},
		},
	}
}

// discard: discard working tree changes by checking out HEAD.
func discardShortcut() Shortcut {
	return Shortcut{
		Name:        "discard",
		Description: "Discard all working tree changes (keep staged)",
		Builtin:     true,
		Confirm:     true,
		Steps: []Step{
			{Op: OpCheckout, Params: map[string]string{"ref": "."}, OnFail: OnFailStop, AIAssist: false},
		},
	}
}

// save: stash with AI-generated descriptive name.
func saveShortcut() Shortcut {
	return Shortcut{
		Name:        "save",
		Description: "Stash all changes with an AI-generated descriptive name",
		Builtin:     true,
		Confirm:     false,
		Steps: []Step{
			{Op: OpStash, Params: map[string]string{paramAIMessage: "true"}, OnFail: OnFailStop, AIAssist: true},
		},
	}
}

// fixup: stage all + fixup last commit.
func fixupShortcut() Shortcut {
	return Shortcut{
		Name:        "fixup",
		Description: "Stage all changes and fixup the last commit",
		Builtin:     true,
		Confirm:     true,
		Steps: []Step{
			{Op: OpStage, Params: map[string]string{paramPaths: "."}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpCommit, Params: map[string]string{paramFixup: refHead}, OnFail: OnFailStop, AIAssist: false},
		},
	}
}

// fresh: stash current work, update default branch, return and restore.
func freshShortcut() Shortcut {
	return Shortcut{
		Name:        "fresh",
		Description: "Stash work, pull latest default branch, rebase, and restore stash",
		Builtin:     true,
		Confirm:     true,
		Args: []Arg{
			{Name: paramBase, Default: "main", Prompt: "Default branch to sync from"},
		},
		Steps: []Step{
			{Op: OpStash, Params: map[string]string{}, OnFail: OnFailContinue, AIAssist: false},
			{Op: OpCheckout, Params: map[string]string{"ref": "{{base}}"}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpPull, Params: map[string]string{paramRebase: "true"}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpCheckout, Params: map[string]string{"ref": "-"}, OnFail: OnFailStop, AIAssist: false},
			{Op: OpRebase, Params: map[string]string{paramOnto: "{{base}}"}, OnFail: OnFailAsk, AIAssist: true},
			{Op: OpStashPop, Params: map[string]string{}, OnFail: OnFailContinue, AIAssist: false},
		},
	}
}

// rename: rename the current branch.
func renameShortcut() Shortcut {
	return Shortcut{
		Name:        "rename",
		Description: "Rename the current branch",
		Builtin:     true,
		Confirm:     true,
		Args: []Arg{
			{Name: paramNewName, Prompt: "New branch name", Required: true},
		},
		Steps: []Step{
			{Op: OpBranchRename, Params: map[string]string{paramNewName: "{{new_name}}"}, OnFail: OnFailStop, AIAssist: false},
		},
	}
}
