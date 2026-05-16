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
		SchemaType:             SchemaObject,
		SchemaProperties:   props,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// emptySchema returns a JSON Schema object with no properties.
func emptySchema() map[string]any {
	return map[string]any{
		SchemaType:             SchemaObject,
		SchemaProperties:   map[string]any{},
	}
}

// stringProp returns a JSON Schema string property.
func stringProp(desc string) map[string]any {
	return map[string]any{SchemaType: SchemaString, SchemaDescription: desc}
}

// boolProp returns a JSON Schema boolean property.
func boolProp(desc string) map[string]any {
	return map[string]any{SchemaType: "boolean", SchemaDescription: desc}
}

// intProp returns a JSON Schema integer property.
func intProp(desc string) map[string]any {
	return map[string]any{SchemaType: "integer", SchemaDescription: desc}
}

// stringArrayProp returns a JSON Schema array-of-strings property.
func stringArrayProp(desc string) map[string]any {
	return map[string]any{
		SchemaType:              "array",
		SchemaDescription:   desc,
		SchemaItems:         map[string]any{SchemaType: SchemaString},
	}
}

// ---------------------------------------------------------------------------
// File operation tools
// ---------------------------------------------------------------------------

func (r *ToolRegistry) registerFileTools() {
	r.register(
		ToolFileRead,
		"Read the contents of a file",
		Safe,
		objectSchema(map[string]any{
			PropPath: stringProp("File path relative to repo root"),
		}, []string{PropPath}),
	)

	r.register(
		ToolFileWrite,
		"Write content to a file, creating it if it does not exist",
		Destructive,
		objectSchema(map[string]any{
			PropPath:    stringProp("File path relative to repo root"),
			PropContent: stringProp("Content to write to the file"),
		}, []string{PropPath, PropContent}),
	)

	r.register(
		ToolFileDelete,
		"Delete a file from the repository",
		Destructive,
		objectSchema(map[string]any{
			PropPath: stringProp("File path relative to repo root"),
		}, []string{PropPath}),
	)

	r.register(
		ToolFileRename,
		"Rename or move a file within the repository",
		Destructive,
		objectSchema(map[string]any{
			PropOldPath: stringProp("Current file path relative to repo root"),
			PropNewPath: stringProp("New file path relative to repo root"),
		}, []string{PropOldPath, PropNewPath}),
	)

	r.register(
		ToolFileList,
		"List files and directories at the given path",
		Safe,
		objectSchema(map[string]any{
			PropPath:      stringProp("Directory path relative to repo root"),
			PropRecursive: boolProp("List files recursively"),
		}, []string{PropPath}),
	)

	r.register(
		ToolFileMkdir,
		"Create a directory and any necessary parents",
		Safe,
		objectSchema(map[string]any{
			PropPath: stringProp("Directory path relative to repo root"),
		}, []string{PropPath}),
	)
}

// ---------------------------------------------------------------------------
// Git read tools (all Safe)
// ---------------------------------------------------------------------------

func (r *ToolRegistry) registerGitReadTools() {
	r.register(
		ToolGitStatus,
		"Returns the list of changed files with their git status codes",
		Safe,
		emptySchema(),
	)

	r.register(
		ToolGitDiff,
		"Returns diff output for changed files",
		Safe,
		objectSchema(map[string]any{
			PropPath:    stringProp("Limit diff to a specific file path"),
			PropStagged: boolProp("Compare staged changes against HEAD"),
		}, nil),
	)

	r.register(
		ToolGitLog,
		"Returns the commit log",
		Safe,
		objectSchema(map[string]any{
			PropCount: intProp("Maximum number of commits to return (default 10)"),
			PropPath:  stringProp("Filter commits by file path"),
		}, nil),
	)

	r.register(
		ToolGitBlame,
		"Returns per-line blame annotation for a file",
		Safe,
		objectSchema(map[string]any{
			PropPath: stringProp("File path to blame"),
		}, []string{PropPath}),
	)

	r.register(
		ToolGitBranchList,
		"Returns the list of local and remote branches",
		Safe,
		emptySchema(),
	)

	r.register(
		ToolGitStashList,
		"Returns the list of stash entries",
		Safe,
		emptySchema(),
	)

	r.register(
		ToolGitWorktreeList,
		"Returns the list of git worktrees",
		Safe,
		emptySchema(),
	)
}

// ---------------------------------------------------------------------------
// Git write tools
// ---------------------------------------------------------------------------

func (r *ToolRegistry) registerGitWriteTools() {
	r.register(
		ToolGitStage,
		"Stage files for commit",
		Safe,
		objectSchema(map[string]any{
			PropPaths: stringArrayProp("File paths to stage"),
		}, []string{PropPaths}),
	)

	r.register(
		ToolGitUnstage,
		"Unstage files from the index",
		Safe,
		objectSchema(map[string]any{
			PropPaths: stringArrayProp("File paths to unstage"),
		}, []string{PropPaths}),
	)

	r.register(
		ToolGitCommit,
		"Create a commit with staged changes",
		Safe,
		objectSchema(map[string]any{
			PropMessage: stringProp("Commit message"),
		}, []string{PropMessage}),
	)

	r.register(
		ToolGitPush,
		"Push commits to a remote",
		Destructive,
		objectSchema(map[string]any{
			PropRemote: stringProp("Remote name (default origin)"),
			PropForce:  boolProp("Force push (overwrites remote history)"),
		}, nil),
	)

	r.register(
		ToolGitPull,
		"Pull changes from a remote",
		Safe,
		objectSchema(map[string]any{
			PropRemote: stringProp("Remote name (default origin)"),
		}, nil),
	)

	r.register(
		ToolGitFetch,
		"Fetch refs and objects from a remote",
		Safe,
		objectSchema(map[string]any{
			PropRemote: stringProp("Remote name"),
		}, nil),
	)

	r.register(
		ToolGitCheckout,
		"Checkout a branch, tag, or commit",
		Safe,
		objectSchema(map[string]any{
			PropRef: stringProp("Git ref to checkout"),
		}, []string{PropRef}),
	)

	r.register(
		ToolGitBranchCreate,
		"Create a new branch",
		Safe,
		objectSchema(map[string]any{
			PropName:       stringProp("Branch name"),
			PropStartPoint: stringProp("Base ref for the new branch (default HEAD)"),
		}, []string{PropName}),
	)

	r.register(
		ToolGitBranchDelete,
		"Delete a branch",
		Destructive,
		objectSchema(map[string]any{
			PropName:  stringProp("Branch name to delete"),
			PropForce: boolProp("Force delete even if not fully merged"),
		}, []string{PropName}),
	)

	r.register(
		ToolGitMerge,
		"Merge a branch into the current branch",
		Safe,
		objectSchema(map[string]any{
			PropBranch: stringProp("Branch to merge"),
		}, []string{PropBranch}),
	)

	r.register(
		ToolGitRebase,
		"Rebase the current branch onto another ref",
		Destructive,
		objectSchema(map[string]any{
			PropOnto: stringProp("Ref to rebase onto"),
		}, []string{PropOnto}),
	)

	r.register(
		ToolGitStashPush,
		"Stash the current working directory changes",
		Safe,
		objectSchema(map[string]any{
			PropMessage: stringProp("Stash message"),
		}, nil),
	)

	r.register(
		ToolGitStashPop,
		"Apply and remove the top stash entry",
		Safe,
		objectSchema(map[string]any{
			PropIndex: intProp("Stash index to pop (default 0)"),
		}, nil),
	)

	r.register(
		ToolGitReset,
		"Reset the current HEAD to a specified state",
		Destructive,
		objectSchema(map[string]any{
			PropRef:  stringProp("Git ref to reset to"),
			PropHard: boolProp("Discard all working tree changes (--hard)"),
		}, []string{PropRef}),
	)

	r.register(
		ToolGitTagCreate,
		"Create a new tag",
		Safe,
		objectSchema(map[string]any{
			PropName:    stringProp("Tag name"),
			PropRef:     stringProp("Ref to tag (default HEAD)"),
			PropMessage: stringProp("Annotation message (creates annotated tag if provided)"),
		}, []string{PropName}),
	)

	r.register(
		ToolGitTagDelete,
		"Delete a tag",
		Destructive,
		objectSchema(map[string]any{
			PropName: stringProp("Tag name to delete"),
		}, []string{PropName}),
	)

	r.register(
		ToolGitDiscard,
		"Discard working tree changes for specified files",
		Destructive,
		objectSchema(map[string]any{
			PropPaths: stringArrayProp("File paths to discard changes for"),
		}, []string{PropPaths}),
	)
}

// ---------------------------------------------------------------------------
// Navigation & search tools (all Safe)
// ---------------------------------------------------------------------------

func (r *ToolRegistry) registerNavSearchTools() {
	r.register(
		ToolNavigateTo,
		"Navigate to a file or directory in the repository",
		Safe,
		objectSchema(map[string]any{
			PropPath: stringProp("File or directory path relative to repo root"),
		}, []string{PropPath}),
	)

	r.register(
		ToolSearchFiles,
		"Search for files matching a glob pattern",
		Safe,
		objectSchema(map[string]any{
			PropPattern: stringProp("Glob pattern to match file names"),
			PropPath:    stringProp("Directory to search within (default repo root)"),
		}, []string{PropPattern}),
	)

	r.register(
		ToolSearchContent,
		"Search file contents for a regex pattern",
		Safe,
		objectSchema(map[string]any{
			PropPattern: stringProp("Regex pattern to search for in file contents"),
			PropPath:    stringProp("Directory to search within (default repo root)"),
		}, []string{PropPattern}),
	)

	r.register(
		ToolExplain,
		"Explain a git concept, command, or workflow",
		Safe,
		objectSchema(map[string]any{
			PropTopic: stringProp("The topic to explain"),
		}, []string{PropTopic}),
	)
}

// ---------------------------------------------------------------------------
// Bulk operation tools
// ---------------------------------------------------------------------------

func (r *ToolRegistry) registerBulkTools() {
	r.register(
		ToolBulkStage,
		"Stage files matching one or more glob patterns",
		Safe,
		objectSchema(map[string]any{
			PropPatterns: stringArrayProp("Glob patterns of files to stage"),
		}, []string{PropPatterns}),
	)

	r.register(
		ToolBulkDelete,
		"Delete multiple files from the repository",
		Destructive,
		objectSchema(map[string]any{
			PropPaths: stringArrayProp("File paths to delete"),
		}, []string{PropPaths}),
	)

	r.register(
		ToolBulkRename,
		"Rename multiple files in a single operation",
		Destructive,
		objectSchema(map[string]any{
			PropRenames: map[string]any{
				SchemaType:            "array",
				SchemaDescription: "List of rename operations to perform",
				SchemaItems: map[string]any{
					SchemaType:            SchemaObject,
					SchemaProperties: map[string]any{
						PropOld: stringProp("Current file path"),
						PropNew: stringProp("New file path"),
					},
					"required": []string{PropOld, PropNew},
				},
			},
		}, []string{PropRenames}),
	)
}
