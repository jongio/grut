package layout

// Position names used by PreviewPosition string conversion.
const (
	positionTop    = "top"
	positionBottom = "bottom"
	positionLeft   = "left"
)

// Panel slot names used in layout tree definitions and the panel registry.
const (
	slotPreview   = "preview"
	slotFiletree  = "filetree"
	slotGitinfo   = "gitinfo"
	slotGithub    = "github"
	slotCommits   = "commits"
	slotGitstatus = "gitstatus"
	slotTerminal  = "terminal"
	slotAgents    = "agents"
	slotContext   = "context"
	slotReview    = "review"
	slotStatus    = "status"
)

// Layout preset names used in preset definitions and the tab bar.
const (
	layoutExplorer = "explorer"
	layoutGit      = "git"
	layoutReview   = "review"
	layoutAgent    = "agent"
	layoutFull     = "full"
)
