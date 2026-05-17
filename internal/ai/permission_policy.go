package ai

import (
	"fmt"
	"log/slog"

	copilot "github.com/github/copilot-sdk/go"
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

// permissionDenied is the result returned for dangerous permission requests.
var permissionDenied = copilot.PermissionRequestResult{
	Kind: copilot.PermissionRequestResultKindUserNotAvailable,
}

// permissionApproved is the result returned for safe permission requests.
var permissionApproved = copilot.PermissionRequestResult{
	Kind: copilot.PermissionRequestResultKindApproved,
}

// isSafePermission reports whether a permission request kind is classified
// as safe (read-only, no side-effects). MCP requests are safe only when
// the SDK marks them as ReadOnly.
func isSafePermission(req copilot.PermissionRequest) bool {
	if req.Kind == copilot.PermissionRequestKindMcp {
		return req.ReadOnly != nil && *req.ReadOnly
	}
	_, safe := safePermissionKinds[req.Kind]
	return safe
}

// policyPermissionHandler evaluates a Copilot SDK permission request
// against the safety policy. Safe (read-only) requests are auto-approved;
// dangerous (write / shell / network) requests are denied by default.
//
// All decisions are logged for auditability.
func policyPermissionHandler(req copilot.PermissionRequest, inv copilot.PermissionInvocation) (copilot.PermissionRequestResult, error) {
	attrs := []any{
		"kind", string(req.Kind),
		"session_id", inv.SessionID,
	}
	if req.Intention != nil {
		attrs = append(attrs, "intention", *req.Intention)
	}
	if req.FullCommandText != nil {
		attrs = append(attrs, "command", *req.FullCommandText)
	}
	if req.ToolName != nil {
		attrs = append(attrs, "tool", *req.ToolName)
	}
	if req.Path != nil {
		attrs = append(attrs, "path", *req.Path)
	}
	if req.FileName != nil {
		attrs = append(attrs, "file", *req.FileName)
	}
	if req.URL != nil {
		attrs = append(attrs, "url", *req.URL)
	}

	if isSafePermission(req) {
		slog.Debug("copilot: permission auto-approved (safe)", attrs...)
		return permissionApproved, nil
	}

	slog.Warn(fmt.Sprintf("copilot: permission denied (dangerous: %s)", req.Kind), attrs...)
	return permissionDenied, nil
}
