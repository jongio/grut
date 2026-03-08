// Package chat implements the conversational AI chat box, including tool
// definitions that the model can invoke to interact with the repository.
package chat

import "github.com/jongio/grut/internal/ai"

// ToolSafety classifies a tool's risk level. Safe tools can execute
// immediately; Destructive tools require explicit user confirmation
// before execution.
type ToolSafety int

const (
	// Safe tools are read-only or low-risk and execute immediately.
	Safe ToolSafety = iota

	// Destructive tools modify or delete data and require user confirmation.
	Destructive
)

// ToolInfo combines a tool definition with its safety classification.
type ToolInfo struct {
	Definition ai.ToolDefinition
	Safety     ToolSafety
}

// ToolRegistry holds all available chat tools with their definitions
// and safety classifications.
type ToolRegistry struct {
	tools map[string]ToolInfo
}

// NewToolRegistry creates a registry with all built-in tools.
func NewToolRegistry() *ToolRegistry {
	r := &ToolRegistry{tools: make(map[string]ToolInfo)}
	r.registerFileTools()
	r.registerGitReadTools()
	r.registerGitWriteTools()
	r.registerNavSearchTools()
	r.registerBulkTools()
	r.registerGitHubTools()
	return r
}

// Get returns the tool info for a given name, or false if not found.
func (r *ToolRegistry) Get(name string) (ToolInfo, bool) {
	info, ok := r.tools[name]
	return info, ok
}

// Definitions returns all tool definitions for sending to the AI provider.
func (r *ToolRegistry) Definitions() []ai.ToolDefinition {
	defs := make([]ai.ToolDefinition, 0, len(r.tools))
	for _, info := range r.tools {
		defs = append(defs, info.Definition)
	}
	return defs
}

// IsSafe reports whether the named tool can be executed without
// confirmation. Returns false for unknown tools.
func (r *ToolRegistry) IsSafe(name string) bool {
	info, ok := r.tools[name]
	return ok && info.Safety == Safe
}

// register adds a tool to the registry.
func (r *ToolRegistry) register(name, description string, safety ToolSafety, params map[string]any) {
	r.tools[name] = ToolInfo{
		Definition: ai.ToolDefinition{
			Name:        name,
			Description: description,
			Parameters:  params,
		},
		Safety: safety,
	}
}

// ---------------------------------------------------------------------------
// JSON Schema helpers — keep tool registrations DRY.
// ---------------------------------------------------------------------------

// objectSchema builds a JSON Schema "object" with the given properties and
// required field list.
func objectSchema(props map[string]any, required []string) map[string]any {
	s := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// emptySchema returns a JSON Schema object with no properties.
func emptySchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

// stringProp returns a JSON Schema string property.
func stringProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// boolProp returns a JSON Schema boolean property.
func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

// intProp returns a JSON Schema integer property.
func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

// stringArrayProp returns a JSON Schema array-of-strings property.
func stringArrayProp(desc string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": desc,
		"items":       map[string]any{"type": "string"},
	}
}

// ---------------------------------------------------------------------------
// File operation tools
// ---------------------------------------------------------------------------

func (r *ToolRegistry) registerFileTools() {
	r.register("file_read",
		"Read the contents of a file",
		Safe,
		objectSchema(map[string]any{
			"path": stringProp("File path relative to repo root"),
		}, []string{"path"}),
	)

	r.register("file_write",
		"Write content to a file, creating it if it does not exist",
		Destructive,
		objectSchema(map[string]any{
			"path":    stringProp("File path relative to repo root"),
			"content": stringProp("Content to write to the file"),
		}, []string{"path", "content"}),
	)

	r.register("file_delete",
		"Delete a file from the repository",
		Destructive,
		objectSchema(map[string]any{
			"path": stringProp("File path relative to repo root"),
		}, []string{"path"}),
	)

	r.register("file_rename",
		"Rename or move a file within the repository",
		Destructive,
		objectSchema(map[string]any{
			"old_path": stringProp("Current file path relative to repo root"),
			"new_path": stringProp("New file path relative to repo root"),
		}, []string{"old_path", "new_path"}),
	)

	r.register("file_list",
		"List files and directories at the given path",
		Safe,
		objectSchema(map[string]any{
			"path":      stringProp("Directory path relative to repo root"),
			"recursive": boolProp("List files recursively"),
		}, []string{"path"}),
	)

	r.register("file_mkdir",
		"Create a directory and any necessary parents",
		Safe,
		objectSchema(map[string]any{
			"path": stringProp("Directory path relative to repo root"),
		}, []string{"path"}),
	)
}

// ---------------------------------------------------------------------------
// Git read tools (all Safe)
// ---------------------------------------------------------------------------

func (r *ToolRegistry) registerGitReadTools() {
	r.register("git_status",
		"Returns the list of changed files with their git status codes",
		Safe,
		emptySchema(),
	)

	r.register("git_diff",
		"Returns diff output for changed files",
		Safe,
		objectSchema(map[string]any{
			"path":   stringProp("Limit diff to a specific file path"),
			"staged": boolProp("Compare staged changes against HEAD"),
		}, nil),
	)

	r.register("git_log",
		"Returns the commit log",
		Safe,
		objectSchema(map[string]any{
			"count": intProp("Maximum number of commits to return (default 10)"),
			"path":  stringProp("Filter commits by file path"),
		}, nil),
	)

	r.register("git_blame",
		"Returns per-line blame annotation for a file",
		Safe,
		objectSchema(map[string]any{
			"path": stringProp("File path to blame"),
		}, []string{"path"}),
	)

	r.register("git_branch_list",
		"Returns the list of local and remote branches",
		Safe,
		emptySchema(),
	)

	r.register("git_stash_list",
		"Returns the list of stash entries",
		Safe,
		emptySchema(),
	)

	r.register("git_worktree_list",
		"Returns the list of git worktrees",
		Safe,
		emptySchema(),
	)
}

// ---------------------------------------------------------------------------
// Git write tools
// ---------------------------------------------------------------------------

func (r *ToolRegistry) registerGitWriteTools() {
	r.register("git_stage",
		"Stage files for commit",
		Safe,
		objectSchema(map[string]any{
			"paths": stringArrayProp("File paths to stage"),
		}, []string{"paths"}),
	)

	r.register("git_unstage",
		"Unstage files from the index",
		Safe,
		objectSchema(map[string]any{
			"paths": stringArrayProp("File paths to unstage"),
		}, []string{"paths"}),
	)

	r.register("git_commit",
		"Create a commit with staged changes",
		Safe,
		objectSchema(map[string]any{
			"message": stringProp("Commit message"),
		}, []string{"message"}),
	)

	r.register("git_push",
		"Push commits to a remote",
		Destructive,
		objectSchema(map[string]any{
			"remote": stringProp("Remote name (default origin)"),
			"force":  boolProp("Force push (overwrites remote history)"),
		}, nil),
	)

	r.register("git_pull",
		"Pull changes from a remote",
		Safe,
		objectSchema(map[string]any{
			"remote": stringProp("Remote name (default origin)"),
		}, nil),
	)

	r.register("git_fetch",
		"Fetch refs and objects from a remote",
		Safe,
		objectSchema(map[string]any{
			"remote": stringProp("Remote name"),
		}, nil),
	)

	r.register("git_checkout",
		"Checkout a branch, tag, or commit",
		Safe,
		objectSchema(map[string]any{
			"ref": stringProp("Git ref to checkout"),
		}, []string{"ref"}),
	)

	r.register("git_branch_create",
		"Create a new branch",
		Safe,
		objectSchema(map[string]any{
			"name":        stringProp("Branch name"),
			"start_point": stringProp("Base ref for the new branch (default HEAD)"),
		}, []string{"name"}),
	)

	r.register("git_branch_delete",
		"Delete a branch",
		Destructive,
		objectSchema(map[string]any{
			"name":  stringProp("Branch name to delete"),
			"force": boolProp("Force delete even if not fully merged"),
		}, []string{"name"}),
	)

	r.register("git_merge",
		"Merge a branch into the current branch",
		Safe,
		objectSchema(map[string]any{
			"branch": stringProp("Branch to merge"),
		}, []string{"branch"}),
	)

	r.register("git_rebase",
		"Rebase the current branch onto another ref",
		Destructive,
		objectSchema(map[string]any{
			"onto": stringProp("Ref to rebase onto"),
		}, []string{"onto"}),
	)

	r.register("git_stash_push",
		"Stash the current working directory changes",
		Safe,
		objectSchema(map[string]any{
			"message": stringProp("Stash message"),
		}, nil),
	)

	r.register("git_stash_pop",
		"Apply and remove the top stash entry",
		Safe,
		objectSchema(map[string]any{
			"index": intProp("Stash index to pop (default 0)"),
		}, nil),
	)

	r.register("git_reset",
		"Reset the current HEAD to a specified state",
		Destructive,
		objectSchema(map[string]any{
			"ref":  stringProp("Git ref to reset to"),
			"hard": boolProp("Discard all working tree changes (--hard)"),
		}, []string{"ref"}),
	)

	r.register("git_tag_create",
		"Create a new tag",
		Safe,
		objectSchema(map[string]any{
			"name":    stringProp("Tag name"),
			"ref":     stringProp("Ref to tag (default HEAD)"),
			"message": stringProp("Annotation message (creates annotated tag if provided)"),
		}, []string{"name"}),
	)

	r.register("git_tag_delete",
		"Delete a tag",
		Destructive,
		objectSchema(map[string]any{
			"name": stringProp("Tag name to delete"),
		}, []string{"name"}),
	)

	r.register("git_discard",
		"Discard working tree changes for specified files",
		Destructive,
		objectSchema(map[string]any{
			"paths": stringArrayProp("File paths to discard changes for"),
		}, []string{"paths"}),
	)
}

// ---------------------------------------------------------------------------
// Navigation & search tools (all Safe)
// ---------------------------------------------------------------------------

func (r *ToolRegistry) registerNavSearchTools() {
	r.register("navigate_to",
		"Navigate to a file or directory in the repository",
		Safe,
		objectSchema(map[string]any{
			"path": stringProp("File or directory path relative to repo root"),
		}, []string{"path"}),
	)

	r.register("search_files",
		"Search for files matching a glob pattern",
		Safe,
		objectSchema(map[string]any{
			"pattern": stringProp("Glob pattern to match file names"),
			"path":    stringProp("Directory to search within (default repo root)"),
		}, []string{"pattern"}),
	)

	r.register("search_content",
		"Search file contents for a regex pattern",
		Safe,
		objectSchema(map[string]any{
			"pattern": stringProp("Regex pattern to search for in file contents"),
			"path":    stringProp("Directory to search within (default repo root)"),
		}, []string{"pattern"}),
	)

	r.register("explain",
		"Explain a git concept, command, or workflow",
		Safe,
		objectSchema(map[string]any{
			"topic": stringProp("The topic to explain"),
		}, []string{"topic"}),
	)
}

// ---------------------------------------------------------------------------
// Bulk operation tools
// ---------------------------------------------------------------------------

func (r *ToolRegistry) registerBulkTools() {
	r.register("bulk_stage",
		"Stage files matching one or more glob patterns",
		Safe,
		objectSchema(map[string]any{
			"patterns": stringArrayProp("Glob patterns of files to stage"),
		}, []string{"patterns"}),
	)

	r.register("bulk_delete",
		"Delete multiple files from the repository",
		Destructive,
		objectSchema(map[string]any{
			"paths": stringArrayProp("File paths to delete"),
		}, []string{"paths"}),
	)

	r.register("bulk_rename",
		"Rename multiple files in a single operation",
		Destructive,
		objectSchema(map[string]any{
			"renames": map[string]any{
				"type":        "array",
				"description": "List of rename operations to perform",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"old": stringProp("Current file path"),
						"new": stringProp("New file path"),
					},
					"required": []string{"old", "new"},
				},
			},
		}, []string{"renames"}),
	)
}
