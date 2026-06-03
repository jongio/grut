package ai

import (
	"fmt"
	"log/slog"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

// ---------------------------------------------------------------------------
// Policy-based permission handler for the Copilot SDK
// ---------------------------------------------------------------------------
// The SDK calls OnPermissionRequest before executing any tool action.
// Instead of blindly approving every request (ApproveAll), we classify
// each PermissionRequestKind into safe (read-only / no side-effects)
// vs dangerous (write / execute / network), then:
//
//   - Safe requests are auto-approved.
//   - Dangerous requests are denied by default. A future interactive
//     confirmation flow can upgrade them to approved.
//
// Every decision is logged at Debug level for auditability. Denied
// requests additionally log at Warn so operators can spot unexpected
// blocks.

// safePermissionKinds contains the request kinds that are considered
// read-only with no destructive side-effects. Only these are
// auto-approved.
var safePermissionKinds = map[copilot.PermissionRequestKind]struct{}{
	copilot.PermissionRequestKindRead:   {}, // file / directory reads
	copilot.PermissionRequestKindMemory: {}, // storing facts / conventions
}

// isSafePermission reports whether a permission request is classified
// as safe (read-only, no side-effects). MCP requests are safe only when
// the SDK marks them as ReadOnly.
func isSafePermission(req copilot.PermissionRequest) bool {
	if mcp, ok := req.(copilot.PermissionRequestMCP); ok {
		return mcp.ReadOnly
	}
	_, safe := safePermissionKinds[req.Kind()]
	return safe
}

// policyPermissionHandler evaluates a Copilot SDK permission request
// against the safety policy. Safe (read-only) requests are auto-approved;
// dangerous (write / shell / network) requests are denied by default.
//
// All decisions are logged for auditability.
func policyPermissionHandler(req copilot.PermissionRequest, inv copilot.PermissionInvocation) (rpc.PermissionDecision, error) {
	kind := req.Kind()
	attrs := []any{
		"kind", string(kind),
		"session_id", inv.SessionID,
	}

	// Extract optional context fields from concrete request types for logging.
	switch r := req.(type) {
	case copilot.PermissionRequestRead:
		attrs = append(attrs, "intention", r.Intention, "path", r.Path)
	case copilot.PermissionRequestShell:
		attrs = append(attrs, "intention", r.Intention, "command", r.FullCommandText)
	case copilot.PermissionRequestWrite:
		attrs = append(attrs, "intention", r.Intention, "file", r.FileName)
	case copilot.PermissionRequestMCP:
		attrs = append(attrs, "tool", r.ToolName, "server", r.ServerName)
	case copilot.PermissionRequestURL:
		attrs = append(attrs, "url", r.URL)
	case copilot.PermissionRequestCustomTool:
		attrs = append(attrs, "tool", r.ToolName)
	}

	if isSafePermission(req) {
		slog.Debug("copilot: permission auto-approved (safe)", attrs...)
		return &rpc.PermissionDecisionApproveOnce{}, nil
	}

	slog.Warn(fmt.Sprintf("copilot: permission denied (dangerous: %s)", kind), attrs...)
	return &rpc.PermissionDecisionUserNotAvailable{}, nil
}
