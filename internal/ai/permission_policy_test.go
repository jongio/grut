package ai

import (
	"testing"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// isSafePermission
// ---------------------------------------------------------------------------

func TestIsSafePermission_ReadKind(t *testing.T) {
	assert.True(t, isSafePermission(copilot.PermissionRequestRead{
		Intention: "read config",
		Path:      "/etc/config",
	}))
}

func TestIsSafePermission_MemoryKind(t *testing.T) {
	assert.True(t, isSafePermission(copilot.PermissionRequestMemory{
		Fact: "test fact",
	}))
}

func TestIsSafePermission_MCPReadOnly(t *testing.T) {
	assert.True(t, isSafePermission(copilot.PermissionRequestMCP{
		ReadOnly:   true,
		ToolName:   "test-tool",
		ServerName: "test-server",
	}))
}

func TestIsSafePermission_MCPNotReadOnly(t *testing.T) {
	assert.False(t, isSafePermission(copilot.PermissionRequestMCP{
		ReadOnly:   false,
		ToolName:   "test-tool",
		ServerName: "test-server",
	}))
}

func TestIsSafePermission_MCPDefaultReadOnly(t *testing.T) {
	assert.False(t, isSafePermission(copilot.PermissionRequestMCP{
		ToolName:   "test-tool",
		ServerName: "test-server",
	}))
}

func TestIsSafePermission_WriteKind(t *testing.T) {
	assert.False(t, isSafePermission(copilot.PermissionRequestWrite{
		Intention: "write file",
		FileName:  "/tmp/out",
	}))
}

func TestIsSafePermission_ShellKind(t *testing.T) {
	assert.False(t, isSafePermission(copilot.PermissionRequestShell{
		FullCommandText: "rm -rf /",
		Intention:       "delete everything",
	}))
}

func TestIsSafePermission_URLKind(t *testing.T) {
	assert.False(t, isSafePermission(copilot.PermissionRequestURL{
		URL: "https://example.com",
	}))
}

func TestIsSafePermission_CustomToolKind(t *testing.T) {
	assert.False(t, isSafePermission(copilot.PermissionRequestCustomTool{
		ToolName:        "dangerous-tool",
		ToolDescription: "does bad things",
	}))
}

// ---------------------------------------------------------------------------
// policyPermissionHandler — safe requests
// ---------------------------------------------------------------------------

func TestPolicyHandler_ApprovesRead(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequestRead{Intention: "read", Path: "/tmp"},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.IsType(t, &rpc.PermissionDecisionApproveOnce{}, result)
}

func TestPolicyHandler_ApprovesMemory(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequestMemory{Fact: "test"},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.IsType(t, &rpc.PermissionDecisionApproveOnce{}, result)
}

func TestPolicyHandler_ApprovesReadOnlyMCP(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequestMCP{
			ReadOnly:   true,
			ToolName:   "reader",
			ServerName: "srv",
		},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.IsType(t, &rpc.PermissionDecisionApproveOnce{}, result)
}

// ---------------------------------------------------------------------------
// policyPermissionHandler — dangerous requests
// ---------------------------------------------------------------------------

func TestPolicyHandler_DeniesWrite(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequestWrite{Intention: "write", FileName: "/tmp/x"},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.IsType(t, &rpc.PermissionDecisionUserNotAvailable{}, result)
}

func TestPolicyHandler_DeniesShell(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequestShell{
			FullCommandText: "rm -rf /",
			Intention:       "destroy",
		},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.IsType(t, &rpc.PermissionDecisionUserNotAvailable{}, result)
}

func TestPolicyHandler_DeniesURL(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequestURL{URL: "https://evil.example.com"},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.IsType(t, &rpc.PermissionDecisionUserNotAvailable{}, result)
}

func TestPolicyHandler_DeniesCustomTool(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequestCustomTool{
			ToolName:        "dangerous-tool",
			ToolDescription: "bad",
		},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.IsType(t, &rpc.PermissionDecisionUserNotAvailable{}, result)
}

func TestPolicyHandler_DeniesMCPWithWriteAccess(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequestMCP{
			ReadOnly:   false,
			ToolName:   "writer",
			ServerName: "srv",
		},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.IsType(t, &rpc.PermissionDecisionUserNotAvailable{}, result)
}

func TestPolicyHandler_DeniesMCPWithDefaultReadOnly(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequestMCP{ToolName: "x", ServerName: "srv"},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.IsType(t, &rpc.PermissionDecisionUserNotAvailable{}, result)
}

// ---------------------------------------------------------------------------
// policyPermissionHandler — unknown kinds default to denied
// ---------------------------------------------------------------------------

func TestPolicyHandler_DeniesUnknownKind(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.RawPermissionRequest{Discriminator: "some-future-kind"},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.IsType(t, &rpc.PermissionDecisionUserNotAvailable{}, result)
}

// ---------------------------------------------------------------------------
// buildSessionConfig integration — verify handler is wired
// ---------------------------------------------------------------------------

func TestBuildSessionConfig_UsesPermissionPolicy(t *testing.T) {
	p := &CopilotProvider{model: "gpt-4o"}
	cfg := p.buildSessionConfig(CompletionRequest{})

	require.NotNil(t, cfg.OnPermissionRequest)

	// Invoke the handler with a dangerous request — must be denied.
	result, err := cfg.OnPermissionRequest(
		copilot.PermissionRequestShell{FullCommandText: "rm -rf /", Intention: "bad"},
		copilot.PermissionInvocation{SessionID: "test"},
	)
	require.NoError(t, err)
	assert.IsType(t, &rpc.PermissionDecisionUserNotAvailable{}, result)

	// Invoke with a safe request — must be approved.
	result, err = cfg.OnPermissionRequest(
		copilot.PermissionRequestRead{Intention: "read", Path: "/tmp"},
		copilot.PermissionInvocation{SessionID: "test"},
	)
	require.NoError(t, err)
	assert.IsType(t, &rpc.PermissionDecisionApproveOnce{}, result)
}
