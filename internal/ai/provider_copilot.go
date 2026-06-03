package ai

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/jongio/grut/internal/git"
)

// ---------------------------------------------------------------------------
// CopilotProvider
// ---------------------------------------------------------------------------
// CopilotProvider implements AIProvider using the official GitHub Copilot SDK
// (github.com/github/copilot-sdk/go). It manages a Copilot CLI client that
// is lazily started on first use and creates per-request sessions.
//
// Tool-calling note: The SDK manages tool execution internally via registered
// handlers. Since the AIProvider interface provides ToolDefinition without
// execution logic, tool definitions in CompletionRequest.Tools are not
// forwarded to the SDK. Callers relying on intermediate ToolCalls in the
// CompletionResponse should be aware this provider does not produce them.
type CopilotProvider struct {
	startErr  error // most recent startup error
	client    *copilot.Client
	model     string
	startMu   sync.Mutex // protects startup state and Close while startup is in progress
	starting  bool
	startDone chan struct{}
	started   bool // true after client.Start succeeds
}

const copilotStartTimeout = 30 * time.Second

// NewCopilotProvider creates a CopilotProvider backed by the Copilot SDK.
// If model is empty the SDK / server selects a default model.
// Auth is handled by the SDK: if GITHUB_TOKEN is set it is passed directly;
// otherwise the SDK falls back to gh CLI / stored OAuth tokens.
func NewCopilotProvider(model string) (*CopilotProvider, error) {
	opts := &copilot.ClientOptions{
		LogLevel: "error",
	}
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		opts.GitHubToken = tok
	}
	return &CopilotProvider{
		client: copilot.NewClient(opts),
		model:  model,
	}, nil
}

// Name returns "copilot".
func (p *CopilotProvider) Name() string { return providerCopilot }

// Available reports whether the Copilot SDK can authenticate. It lazily
// starts the underlying CLI client and queries auth status.
func (p *CopilotProvider) Available(ctx context.Context) (bool, error) {
	if err := p.ensureStarted(ctx); err != nil {
		return false, nil //nolint:nilerr // session creation error handled by caller
	}
	status, err := p.client.GetAuthStatus(ctx)
	if err != nil {
		slog.Debug("copilot: auth status check failed", "error", err)
		return false, nil
	}
	return status.IsAuthenticated, nil
}

// Complete sends a one-shot completion request via the Copilot SDK and
// blocks until the full response is ready.
func (p *CopilotProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	if err := p.ensureStarted(ctx); err != nil {
		return CompletionResponse{}, fmt.Errorf("copilot complete: %w", err)
	}
	if len(req.Tools) > 0 {
		slog.Debug("copilot: tool definitions in request are not forwarded to the SDK")
	}
	session, err := p.client.CreateSession(ctx, p.buildSessionConfig(req))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("copilot complete: create session: %w", err)
	}
	defer func() { _ = session.Disconnect() }()
	event, err := session.SendAndWait(ctx, copilot.MessageOptions{
		Prompt: p.buildPrompt(req),
	})
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("copilot complete: send: %w", err)
	}
	return eventToResponse(event), nil
}

// CompleteStream sends a streaming completion request and returns a channel
// that delivers incremental chunks. The channel is closed when the response
// finishes or an error occurs.
func (p *CopilotProvider) CompleteStream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	if err := p.ensureStarted(ctx); err != nil {
		return nil, fmt.Errorf("copilot stream: %w", err)
	}
	if len(req.Tools) > 0 {
		slog.Debug("copilot: tool definitions in streaming request are not forwarded to the SDK")
	}
	cfg := p.buildSessionConfig(req)
	streaming := true
	cfg.Streaming = &streaming
	session, err := p.client.CreateSession(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("copilot stream: create session: %w", err)
	}
	ch := make(chan StreamChunk, 64)
	done := make(chan struct{})
	var (
		closeOnce sync.Once
		usageMu   sync.Mutex
		lastUsage *TokenUsage
	)
	// finish sends the terminal chunk, closes the channel, and tears down
	// the session. It is safe to call from multiple goroutines.
	// Uses select/default to prevent blocking on a full channel (CWE-404).
	finish := func(chunk StreamChunk) {
		closeOnce.Do(func() {
			select {
			case ch <- chunk:
			default:
				// Channel full — consumer stopped reading. Drop the
				// terminal chunk rather than deadlocking the SDK callback.
			}
			close(ch)
			close(done)
			// Disconnect with a timeout to prevent goroutine leak.
			// Fire-and-forget: if Disconnect hangs past the 5s deadline
			// we log and exit so the outer goroutine is not pinned.
			go func() {
				disconnDone := make(chan struct{})
				go func() {
					_ = session.Disconnect()
					close(disconnDone)
				}()
				timer := time.NewTimer(5 * time.Second)
				defer timer.Stop()
				select {
				case <-disconnDone:
				case <-timer.C:
					slog.Debug("copilot: session disconnect timed out after 5s; abandoning")
					// Inner goroutine will finish on its own.
				}
			}()
		})
	}
	session.On(func(event copilot.SessionEvent) {
		switch event.Type() { //nolint:exhaustive // only relevant cases handled
		case copilot.SessionEventTypeAssistantMessageDelta:
			if d, ok := event.Data.(*copilot.AssistantMessageDeltaData); ok && d.DeltaContent != "" {
				select {
				case ch <- StreamChunk{Delta: d.DeltaContent}:
				case <-ctx.Done():
				}
			}
		case copilot.SessionEventTypeAssistantUsage:
			if d, ok := event.Data.(*copilot.AssistantUsageData); ok {
				if u := extractUsage(d); u != nil {
					usageMu.Lock()
					lastUsage = u
					usageMu.Unlock()
				}
			}
		case copilot.SessionEventTypeSessionIdle:
			usageMu.Lock()
			u := lastUsage
			usageMu.Unlock()
			finish(StreamChunk{Done: true, TokensUsed: u})
		case copilot.SessionEventTypeSessionError:
			errMsg := "unknown error"
			if d, ok := event.Data.(*copilot.SessionErrorData); ok {
				errMsg = d.Message
			}
			finish(StreamChunk{Done: true, Err: fmt.Errorf("copilot stream: %s", errMsg)})
		}
	})
	prompt := p.buildPrompt(req)
	if _, err := session.Send(ctx, copilot.MessageOptions{Prompt: prompt}); err != nil {
		closeOnce.Do(func() {
			close(ch)
			close(done)
		})
		_ = session.Disconnect()
		return nil, fmt.Errorf("copilot stream: send: %w", err)
	}
	// Cancel-safety: if the caller's context is cancelled before the
	// stream completes naturally, emit a final error chunk.
	go func() {
		select {
		case <-ctx.Done():
			finish(StreamChunk{Done: true, Err: ctx.Err()})
		case <-done:
			// Stream completed normally; nothing to do.
		}
	}()
	return ch, nil
}

// Close stops the Copilot CLI client if it was started.
func (p *CopilotProvider) Close() error {
	for {
		p.startMu.Lock()
		if p.starting {
			done := p.startDone
			p.startMu.Unlock()
			<-done
			continue
		}
		defer p.startMu.Unlock()
		if p.started && p.startErr == nil {
			if err := p.client.Stop(); err != nil {
				return fmt.Errorf("copilot: stop client: %w", err)
			}
			p.started = false
		}
		return nil
	}
}

// ---------------------------------------------------------------------------
// Lazy startup
// ---------------------------------------------------------------------------
// ensureStarted lazily starts the Copilot CLI client on first use.
// Concurrent callers share one in-flight startup. Failed startup attempts are
// retryable because sync.Once would permanently cache transient errors.
func (p *CopilotProvider) ensureStarted(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		p.startMu.Lock()
		if p.started && p.startErr == nil {
			p.startMu.Unlock()
			return nil
		}
		if p.starting {
			done := p.startDone
			p.startMu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return fmt.Errorf("copilot: start client: %w", ctx.Err())
			}
		}
		p.starting = true
		p.startDone = make(chan struct{})
		done := p.startDone
		p.startMu.Unlock()

		startCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), copilotStartTimeout)
		err := p.client.Start(startCtx)
		cancel()
		if err != nil {
			err = fmt.Errorf("copilot: start client: %w", err)
		}

		p.startMu.Lock()
		p.startErr = err
		p.started = err == nil
		p.starting = false
		close(done)
		p.startMu.Unlock()
		return err
	}
}

// ---------------------------------------------------------------------------
// Session / prompt helpers
// ---------------------------------------------------------------------------
// buildSessionConfig creates a SessionConfig from a CompletionRequest.
func (p *CopilotProvider) buildSessionConfig(req CompletionRequest) *copilot.SessionConfig {
	cfg := &copilot.SessionConfig{
		Model:               p.model,
		OnPermissionRequest: policyPermissionHandler,
	}
	if req.SystemPrompt != "" {
		cfg.SystemMessage = &copilot.SystemMessageConfig{
			Mode:    "append",
			Content: req.SystemPrompt,
		}
	}
	return cfg
}

// buildPrompt assembles the user-facing prompt from GitContext, conversation
// messages, and the single-turn user prompt. For multi-turn conversations
// the message history is serialised as labelled text because the SDK's
// Send method accepts a single prompt string.
func (p *CopilotProvider) buildPrompt(req CompletionRequest) string {
	var parts []string
	if gc := serializeGitContext(req.GitContext); gc != "" {
		parts = append(parts, gc)
	}
	for _, m := range req.Messages {
		if m.Content != "" {
			parts = append(parts, fmt.Sprintf("[%s]: %s", m.Role, m.Content))
		}
	}
	if req.UserPrompt != "" {
		parts = append(parts, req.UserPrompt)
	}
	return strings.Join(parts, "\n\n")
}

// ---------------------------------------------------------------------------
// Response conversion
// ---------------------------------------------------------------------------
// eventToResponse converts a SDK SessionEvent into a CompletionResponse.
func eventToResponse(event *copilot.SessionEvent) CompletionResponse {
	resp := CompletionResponse{
		FinishReason: finishReasonStop,
		Metadata:     map[string]string{"provider": "copilot-sdk"},
	}
	if event == nil {
		return resp
	}
	if d, ok := event.Data.(*copilot.AssistantMessageData); ok {
		resp.Content = d.Content
	}
	if d, ok := event.Data.(*copilot.AssistantUsageData); ok {
		if u := extractUsage(d); u != nil {
			resp.TokensUsed = *u
		}
	}
	return resp
}

// extractUsage extracts token usage from an AssistantUsageData payload.
// Returns nil if neither input nor output token counts are present.
func extractUsage(data *copilot.AssistantUsageData) *TokenUsage {
	if data == nil || (data.InputTokens == nil && data.OutputTokens == nil) {
		return nil
	}
	u := &TokenUsage{}
	if data.InputTokens != nil {
		u.InputTokens = int(*data.InputTokens)
	}
	if data.OutputTokens != nil {
		u.OutputTokens = int(*data.OutputTokens)
	}
	return u
}

// ---------------------------------------------------------------------------
// GitContext serialisation
// ---------------------------------------------------------------------------
// serializeGitContext renders structured repository state into a plain-text
// block suitable for inclusion as model context. All git-sourced content
// (branch names, commit subjects, diff lines) is wrapped in boundary
// markers to mitigate prompt injection from malicious repository content.
func serializeGitContext(gc GitContext) string {
	var b strings.Builder
	if gc.RepoRoot != "" {
		fmt.Fprintf(&b, "Repository: %s\n", QuoteUntrusted(SanitizeFilePath(gc.RepoRoot)))
	}
	if gc.CurrentBranch != "" {
		fmt.Fprintf(&b, "Current Branch: %s\n", QuoteUntrusted(SanitizeBranchName(gc.CurrentBranch)))
	}
	if gc.TargetBranch != "" {
		fmt.Fprintf(&b, "Target Branch: %s\n", QuoteUntrusted(SanitizeBranchName(gc.TargetBranch)))
	}
	if len(gc.Status) > 0 {
		b.WriteString("\nFile Status:\n")
		for _, s := range gc.Status {
			fmt.Fprintf(&b, "  %c%c %s\n", byte(s.StagedStatus), byte(s.WorktreeStatus), QuoteUntrusted(SanitizeFilePath(s.Path)))
		}
	}
	if len(gc.Log) > 0 {
		b.WriteString("\nRecent Commits:\n")
		for _, c := range gc.Log {
			fmt.Fprintf(&b, "  %s %s\n", QuoteUntrusted(stripControlChars(c.ShortHash)), QuoteUntrusted(SanitizeCommitMessage(c.Subject)))
		}
	}
	if len(gc.Diffs) > 0 {
		var diffBuf strings.Builder
		for _, d := range gc.Diffs {
			fmt.Fprintf(&diffBuf, "--- %s\n", SanitizeFilePath(d.Path))
			for _, h := range d.Hunks {
				fmt.Fprintf(&diffBuf, "%s\n", SanitizeCommitMessage(h.Header))
				for _, l := range h.Lines {
					var prefix byte
					switch l.Type {
					case git.DiffLineAdded:
						prefix = '+'
					case git.DiffLineRemoved:
						prefix = '-'
					default:
						prefix = ' '
					}
					// Cap individual diff line length to prevent huge
					// binary blobs from bloating AI context.
					const maxDiffLineLen = 10000
					content := l.Content
					if len(content) > maxDiffLineLen {
						content = content[:maxDiffLineLen] + "… [truncated]"
					}
					fmt.Fprintf(&diffBuf, "%c%s\n", prefix, SanitizeCommitMessage(content))
				}
			}
		}
		b.WriteString("\nDiffs:\n")
		b.WriteString(SanitizeExternalContent(diffBuf.String()))
	}
	return strings.TrimSpace(b.String())
}
