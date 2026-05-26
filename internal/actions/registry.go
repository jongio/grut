// Package actions defines the action registry for configurable double-click
// actions. Each item type in the UI maps to a default action and a set of
// alternatives that the user may choose via first-use prompts or settings.
package actions

// ItemType identifies a clickable item category in the UI.
type ItemType string

const (
	ItemLocalBranch  ItemType = "local_branch"
	ItemRemoteBranch ItemType = "remote_branch"
	ItemWorktree     ItemType = "worktree"
	ItemRemote       ItemType = "remote"
	ItemStashEntry   ItemType = "stash_entry"
	ItemIssue        ItemType = "issue"
	ItemPR           ItemType = "pr"
	ItemActionRun    ItemType = "action_run"
	ItemWorkflow     ItemType = "workflow"
	ItemRelease      ItemType = "release"
	ItemFile         ItemType = "file"
	ItemDirectory    ItemType = "directory"
	ItemCommit       ItemType = "commit"
	ItemStatusFile   ItemType = "status_file"
	ItemLogCommit    ItemType = "log_commit"
	ItemStash        ItemType = "stash"
	ItemConflictFile ItemType = "conflict_file"
	ItemReviewFile   ItemType = "review_file"
	ItemContextFile  ItemType = "context_file"
	ItemBookmark     ItemType = "bookmark"
	ItemExtension    ItemType = "extension"
	ItemAgent        ItemType = "agent"
	ItemSetting      ItemType = "setting"
	ItemTag          ItemType = "tag"
)

// ActionID identifies a specific action that can be performed on an item.
type ActionID string

const (
	ActionCheckout        ActionID = "checkout"
	ActionOpenTerminal    ActionID = "open_terminal"
	ActionOpenInBrowser   ActionID = "open_in_browser"
	ActionOpenInEditor    ActionID = "open_in_editor"
	ActionPromptAction    ActionID = "prompt_action"
	ActionApply           ActionID = "apply"
	ActionPop             ActionID = "pop"
	ActionDrop            ActionID = "drop"
	ActionExpandCollapse  ActionID = "expand_collapse"
	ActionShowDetail      ActionID = "show_detail"
	ActionExpandDiff      ActionID = "expand_diff"
	ActionStageUnstage    ActionID = "stage_unstage"
	ActionOpenDiff        ActionID = "open_diff"
	ActionResolveOurs     ActionID = "resolve_ours"
	ActionResolveTheirs   ActionID = "resolve_theirs"
	ActionPreview         ActionID = "preview"
	ActionJump            ActionID = "jump"
	ActionToggleDetails   ActionID = "toggle_details"
	ActionEnableDisable   ActionID = "enable_disable"
	ActionToggleOutput    ActionID = "toggle_output"
	ActionCycleValue      ActionID = "cycle_value"
	ActionCopyName        ActionID = "copy_name"
	ActionCopyURL         ActionID = "copy_url"
	ActionCopyNumber      ActionID = "copy_number"
	ActionCopyHash        ActionID = "copy_hash"
	ActionCopyPath        ActionID = "copy_path"
	ActionStage           ActionID = "stage"
	ActionApprove         ActionID = "approve"
	ActionRemove          ActionID = "remove"
	ActionDelete          ActionID = "delete"
	ActionRerun           ActionID = "rerun"
	ActionDispatch        ActionID = "dispatch"
	ActionDownloadAssets  ActionID = "download_assets"
	ActionCheckoutBranch  ActionID = "checkout_branch"
	ActionMergePR         ActionID = "merge_pr"
	ActionOpenInDefaultApp ActionID = "open_in_default_app"
	ActionShowContextMenu  ActionID = "context_menu"
	ActionPush             ActionID = "push"
	ActionChangeDirectory  ActionID = "change_directory"
)

// ItemActions defines the default action and alternatives for an item type.
type ItemActions struct {
	Default      ActionID
	RightClick   ActionID // default right-click action (usually ActionShowContextMenu)
	Description  string   // human-readable prompt text for the default action
	Alternatives []ActionID
}

// Registry maps each item type to its available actions.
var Registry = map[ItemType]ItemActions{
	ItemLocalBranch:  {Default: ActionCheckout, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionCopyName, ActionOpenInBrowser}, Description: "switch to this branch"},
	ItemRemoteBranch: {Default: ActionCheckout, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionCopyName}, Description: "check out this remote branch locally"},
	ItemWorktree:     {Default: ActionChangeDirectory, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionOpenTerminal, ActionCopyPath}, Description: "change to this worktree's directory"},
	ItemRemote:       {Default: ActionOpenInBrowser, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionCopyURL}, Description: "open this remote in your browser"},
	ItemStashEntry:   {Default: ActionPromptAction, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionApply, ActionPop, ActionDrop}, Description: "choose a stash action (apply/pop/drop)"},
	ItemIssue:        {Default: ActionOpenInBrowser, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionCopyURL, ActionCopyNumber}, Description: "open this issue in your browser"},
	ItemPR:           {Default: ActionOpenInBrowser, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionMergePR, ActionCopyURL, ActionCopyNumber, ActionCheckoutBranch}, Description: "open this pull request in your browser"},
	ItemActionRun:    {Default: ActionOpenInBrowser, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionRerun, ActionCopyURL}, Description: "open this workflow run in your browser"},
	ItemWorkflow:     {Default: ActionDispatch, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionOpenInBrowser, ActionCopyURL}, Description: "dispatch this workflow"},
	ItemRelease:      {Default: ActionOpenInBrowser, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionDownloadAssets, ActionCopyURL, ActionCopyName}, Description: "open this release in your browser"},
	ItemFile:         {Default: ActionOpenInDefaultApp, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionOpenInEditor, ActionCopyPath, ActionStage, ActionPreview}, Description: "open this file in its default app"},
	ItemDirectory:    {Default: ActionExpandCollapse, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionCopyPath}, Description: "expand or collapse this directory"},
	ItemCommit:       {Default: ActionShowDetail, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionCopyHash, ActionOpenInBrowser}, Description: "show details for this commit"},
	ItemStatusFile:   {Default: ActionExpandDiff, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionStageUnstage, ActionCopyPath}, Description: "expand the inline diff for this file"},
	ItemLogCommit:    {Default: ActionShowDetail, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionCopyHash}, Description: "show details for this commit"},
	ItemStash:        {Default: ActionApply, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionPop, ActionDrop, ActionShowDetail}, Description: "apply this stash entry"},
	ItemConflictFile: {Default: ActionOpenDiff, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionResolveOurs, ActionResolveTheirs}, Description: "open the diff for this conflicted file"},
	ItemReviewFile:   {Default: ActionExpandDiff, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionApprove, ActionCopyPath}, Description: "expand the diff for this file"},
	ItemContextFile:  {Default: ActionPreview, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionRemove, ActionCopyPath}, Description: "preview this file"},
	ItemBookmark:     {Default: ActionJump, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionDelete, ActionCopyPath}, Description: "jump to this bookmark"},
	ItemExtension:    {Default: ActionToggleDetails, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionEnableDisable}, Description: "show details for this extension"},
	ItemAgent:        {Default: ActionToggleOutput, RightClick: ActionShowContextMenu, Alternatives: nil, Description: "expand output for this agent"},
	ItemSetting:      {Default: ActionCycleValue, RightClick: ActionShowContextMenu, Alternatives: nil, Description: "cycle this setting value"},
	ItemTag:          {Default: ActionCheckout, RightClick: ActionShowContextMenu, Alternatives: []ActionID{ActionPush, ActionDelete, ActionCopyName, ActionCopyHash}, Description: "checkout this tag (detached HEAD)"},
}

// allItemTypes lists every item type in display order for deterministic iteration.
var allItemTypes = []ItemType{
	ItemLocalBranch, ItemRemoteBranch, ItemWorktree, ItemRemote,
	ItemStashEntry, ItemIssue, ItemPR, ItemActionRun, ItemWorkflow, ItemRelease, ItemTag,
	ItemFile, ItemDirectory, ItemCommit, ItemStatusFile,
	ItemLogCommit, ItemStash, ItemConflictFile, ItemReviewFile,
	ItemContextFile, ItemBookmark, ItemExtension, ItemAgent, ItemSetting,
}

// actionLabels maps each action ID to a human-readable label.
var actionLabels = map[ActionID]string{
	ActionCheckout:        "checkout",
	ActionOpenTerminal:    "open terminal",
	ActionOpenInBrowser:   "open in browser",
	ActionOpenInDefaultApp: "open in default app",
	ActionOpenInEditor:     "open in editor",
	ActionPromptAction:    "prompt action",
	ActionApply:           "apply",
	ActionPop:             "pop",
	ActionDrop:            "drop",
	ActionExpandCollapse:  "expand/collapse",
	ActionShowDetail:      "show detail",
	ActionExpandDiff:      "expand diff",
	ActionStageUnstage:    "stage/unstage",
	ActionOpenDiff:        "open diff",
	ActionResolveOurs:     "resolve ours",
	ActionResolveTheirs:   "resolve theirs",
	ActionPreview:         "preview",
	ActionJump:            "jump",
	ActionToggleDetails:   "toggle details",
	ActionEnableDisable:   "enable/disable",
	ActionToggleOutput:    "toggle output",
	ActionCycleValue:      "cycle value",
	ActionCopyName:        "copy name",
	ActionCopyURL:         "copy URL",
	ActionCopyNumber:      "copy number",
	ActionCopyHash:        "copy hash",
	ActionCopyPath:        "copy path",
	ActionStage:           "stage",
	ActionApprove:         "approve",
	ActionRemove:          "remove",
	ActionDelete:          "delete",
	ActionRerun:           "rerun",
	ActionDispatch:        "dispatch",
	ActionDownloadAssets:  "download assets",
	ActionCheckoutBranch:  "checkout branch",
	ActionMergePR:         "merge PR",
	ActionShowContextMenu: "context menu",
	ActionPush:            "push",
	ActionChangeDirectory: "change directory",
}

// itemLabels maps each item type to a human-readable label.
var itemLabels = map[ItemType]string{
	ItemLocalBranch:  "Branch",
	ItemRemoteBranch: "Remote Branch",
	ItemWorktree:     "Worktree",
	ItemRemote:       "Remote",
	ItemStashEntry:   "Stash Entry",
	ItemIssue:        "Issue",
	ItemPR:           "PR",
	ItemActionRun:    "Action Run",
	ItemWorkflow:     "Workflow",
	ItemRelease:      "Release",
	ItemFile:         "File",
	ItemDirectory:    "Directory",
	ItemCommit:       "Commit",
	ItemStatusFile:   "Status File",
	ItemLogCommit:    "Log Commit",
	ItemStash:        "Stash",
	ItemConflictFile: "Conflict File",
	ItemReviewFile:   "Review File",
	ItemContextFile:  "Context File",
	ItemBookmark:     "Bookmark",
	ItemExtension:    "Extension",
	ItemAgent:        "Agent",
	ItemSetting:      "Setting",
	ItemTag:          "Tag",
}

// DefaultAction returns the default double-click action for an item type.
// Returns an empty ActionID if the item type is not registered.
func DefaultAction(it ItemType) ActionID {
	if entry, ok := Registry[it]; ok {
		return entry.Default
	}
	return ""
}

// AllActions returns the default action followed by all alternatives for
// an item type. Returns nil if the item type is not registered.
func AllActions(it ItemType) []ActionID {
	entry, ok := Registry[it]
	if !ok {
		return nil
	}
	all := make([]ActionID, 0, 1+len(entry.Alternatives))
	all = append(all, entry.Default)
	all = append(all, entry.Alternatives...)
	return all
}

// Description returns the human-readable prompt description for an item
// type's default action. Returns an empty string for unknown item types.
func Description(it ItemType) string {
	if entry, ok := Registry[it]; ok {
		return entry.Description
	}
	return ""
}

// ActionLabel returns a human-readable label for an action ID.
// Falls back to the raw action ID string for unknown actions.
func ActionLabel(id ActionID) string {
	if label, ok := actionLabels[id]; ok {
		return label
	}
	return string(id)
}

// IsValidAction checks if the given action is valid for the item type.
func IsValidAction(it ItemType, action ActionID) bool {
	for _, a := range AllActions(it) {
		if a == action {
			return true
		}
	}
	return false
}

// ConfigurableItems returns all item types that have at least one
// alternative action, in a stable display order.
func ConfigurableItems() []ItemType {
	var result []ItemType
	for _, it := range allItemTypes {
		entry, ok := Registry[it]
		if ok && len(entry.Alternatives) > 0 {
			result = append(result, it)
		}
	}
	return result
}

// ItemLabel returns a human-readable label for an item type.
// Falls back to the raw item type string for unknown types.
func ItemLabel(it ItemType) string {
	if label, ok := itemLabels[it]; ok {
		return label
	}
	return string(it)
}

// AllItemTypes returns all registered item types in display order.
func AllItemTypes() []ItemType {
	out := make([]ItemType, len(allItemTypes))
	copy(out, allItemTypes)
	return out
}

// RightClickAction returns the default right-click action for an item type.
// Returns an empty ActionID if the item type is not registered.
func RightClickAction(it ItemType) ActionID {
	if entry, ok := Registry[it]; ok {
		return entry.RightClick
	}
	return ""
}

// AllRightClickActions returns the context menu action followed by all
// available actions for an item type (default + alternatives).
func AllRightClickActions(it ItemType) []ActionID {
	entry, ok := Registry[it]
	if !ok {
		return nil
	}
	all := make([]ActionID, 0, 2+len(entry.Alternatives))
	all = append(all, ActionShowContextMenu)
	all = append(all, entry.Default)
	all = append(all, entry.Alternatives...)
	return uniqueActions(all)
}

// uniqueActions deduplicates action IDs preserving order.
func uniqueActions(ids []ActionID) []ActionID {
	seen := make(map[ActionID]bool, len(ids))
	result := make([]ActionID, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}
