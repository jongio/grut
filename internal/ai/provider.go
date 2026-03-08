// Package ai defines the provider interface and shared types for the AI
// subsystem. Both structured git-ops (commit messages, conflict resolution)
// and the conversational chat box consume these types.
package ai

import (
	"context"

	"github.com/jongio/grut/internal/git"
)

// AIProvider is the interface every AI backend (OpenAI, Anthropic, Ollama, …)
// must implement. It covers one-shot completions, streaming completions, and
// lifecycle management.
type AIProvider interface {
	// Name returns a human-readable identifier for this provider (e.g. "openai").
	Name() string

	// Available reports whether the provider is configured and reachable.
	Available(ctx context.Context) (bool, error)

	// Complete sends a one-shot completion request and blocks until the full
	// response is ready.
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)

	// CompleteStream sends a completion request and returns a channel that
	// delivers incremental chunks. The channel is closed when the response is
	// finished or an error occurs.
	CompleteStream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)

	// Close releases any resources held by the provider (HTTP clients, etc.).
	Close() error
}

// CompletionRequest carries everything the provider needs to produce a
// response. Fields are selectively populated depending on the caller:
// git-ops fill GitContext; the chat box fills Messages.
type CompletionRequest struct {
	// Operation identifies the high-level task, e.g. "conflict_resolve",
	// "commit_message", or "chat".
	Operation string

	// SystemPrompt is the system-level instruction prepended to the
	// conversation.
	SystemPrompt string

	// GitContext supplies structured repository state for git-ops callers.
	GitContext GitContext

	// Messages carries a multi-turn conversation for the chat box.
	Messages []ChatMessage

	// UserPrompt is a single user turn, used by git-ops callers that don't
	// need multi-turn history.
	UserPrompt string

	// Tools lists function-calling tool definitions available to the model.
	Tools []ToolDefinition

	// MaxTokens caps the number of tokens the model may generate.
	MaxTokens int

	// Temperature controls output randomness (0 = deterministic, 1 = creative).
	Temperature float64
}

// CompletionResponse is the provider's reply to a CompletionRequest.
type CompletionResponse struct {
	// Content is the model-generated text.
	Content string

	// ToolCalls contains any function calls the model wants to invoke.
	ToolCalls []ToolCall

	// TokensUsed reports input/output token counts for the request.
	TokensUsed TokenUsage

	// FinishReason indicates why generation stopped:
	// "stop", "length", "tool_calls", or "error".
	FinishReason string

	// Metadata holds provider-specific key/value pairs (model name, request
	// ID, etc.).
	Metadata map[string]string
}

// ChatMessage represents a single turn in a multi-turn conversation.
type ChatMessage struct {
	// Role is one of "user", "assistant", "tool", or "system".
	Role string

	// Content is the textual body of the message.
	Content string

	// ToolCalls is populated when Role is "assistant" and the model invokes
	// one or more tools.
	ToolCalls []ToolCall

	// ToolID identifies the tool invocation this message responds to. Set
	// when Role is "tool".
	ToolID string
}

// ToolDefinition describes a function the model may call.
type ToolDefinition struct {
	// Name is the function name the model references in a ToolCall.
	Name string

	// Description explains what the tool does, helping the model decide when
	// to call it.
	Description string

	// Parameters is a JSON Schema object describing the function's arguments.
	Parameters map[string]any
}

// ToolCall represents a single function invocation requested by the model.
type ToolCall struct {
	// ID is a provider-assigned identifier used to correlate tool results
	// back to the call.
	ID string

	// Name is the function to invoke (must match a ToolDefinition.Name).
	Name string

	// Arguments are the parsed function arguments.
	Arguments map[string]any
}

// TokenUsage tracks how many tokens a request consumed.
type TokenUsage struct {
	// InputTokens is the number of prompt tokens sent.
	InputTokens int

	// OutputTokens is the number of tokens the model generated.
	OutputTokens int
}

// StreamChunk is a single incremental piece of a streaming response.
type StreamChunk struct {
	// Delta contains the new text fragment.
	Delta string

	// Done is true on the final chunk.
	Done bool

	// ToolCalls contains incremental tool-call data, if any.
	ToolCalls []ToolCall

	// TokensUsed is set only on the final chunk (Done == true).
	TokensUsed *TokenUsage

	// Err carries any error that terminated the stream.
	Err error
}

// GitContext provides structured repository state to AI-powered git
// operations. Callers populate only the fields relevant to the operation
// (e.g. Diffs for commit messages, Conflicts for merge resolution).
type GitContext struct {
	// RepoRoot is the absolute path to the repository root.
	RepoRoot string

	// CurrentBranch is the checked-out branch.
	CurrentBranch string

	// TargetBranch is the branch being merged into or compared against.
	TargetBranch string

	// Diffs contains file-level diff information (staged or unstaged).
	Diffs []git.FileDiff

	// Conflicts lists files with unresolved merge conflicts.
	Conflicts []ConflictFile

	// Log holds recent commit history for context.
	Log []git.Commit

	// FileContents maps file paths to their full text, used when the model
	// needs to see entire files rather than just diffs.
	FileContents map[string]string

	// Status lists the working tree and index status of tracked files.
	Status []git.FileStatus
}

// ConflictFile describes a single file with unresolved merge conflicts.
type ConflictFile struct {
	// Path is the repository-relative file path.
	Path string

	// OursContent is the full file content from the current branch.
	OursContent string

	// TheirsContent is the full file content from the incoming branch.
	TheirsContent string

	// BaseContent is the full file content from the common ancestor.
	BaseContent string

	// ConflictMarkers lists the individual conflict regions within the file.
	ConflictMarkers []ConflictRegion
}

// ConflictRegion marks a single conflict hunk inside a file.
type ConflictRegion struct {
	// StartLine is the 1-based line where the conflict begins.
	StartLine int

	// EndLine is the 1-based line where the conflict ends (inclusive).
	EndLine int

	// Ours is the text from the current branch within this region.
	Ours string

	// Theirs is the text from the incoming branch within this region.
	Theirs string

	// Base is the text from the common ancestor within this region, if
	// available (diff3 style).
	Base string
}
