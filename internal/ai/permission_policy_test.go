package ai

import (
	"testing"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// isSafePermission
// ---------------------------------------------------------------------------

func TestIsSafePermission_ReadKind(t *testing.T) {
	assert.True(t, isSafePermission(copilot.PermissionRequest{Kind: copilot.PermissionRequestKindRead}))
}

func TestIsSafePermission_MemoryKind(t *testing.T) {
	assert.True(t, isSafePermission(copilot.PermissionRequest{Kind: copilot.PermissionRequestKindMemory}))
}

func TestIsSafePermission_MCPReadOnly(t *testing.T) {
	ro := true
	assert.True(t, isSafePermission(copilot.PermissionRequest{
		Kind:     copilot.PermissionRequestKindMcp,
		ReadOnly: &ro,
	}))
}

func TestIsSafePermission_MCPNotReadOnly(t *testing.T) {
	ro := false
	assert.False(t, isSafePermission(copilot.PermissionRequest{
		Kind:     copilot.PermissionRequestKindMcp,
		ReadOnly: &ro,
	}))
}

func TestIsSafePermission_MCPNilReadOnly(t *testing.T) {
	assert.False(t, isSafePermission(copilot.PermissionRequest{Kind: copilot.PermissionRequestKindMcp}))
}

func TestIsSafePermission_WriteKind(t *testing.T) {
	assert.False(t, isSafePermission(copilot.PermissionRequest{Kind: copilot.PermissionRequestKindWrite}))
}

func TestIsSafePermission_ShellKind(t *testing.T) {
	assert.False(t, isSafePermission(copilot.PermissionRequest{Kind: copilot.PermissionRequestKindShell}))
}

func TestIsSafePermission_URLKind(t *testing.T) {
	assert.False(t, isSafePermission(copilot.PermissionRequest{Kind: copilot.PermissionRequestKindURL}))
}

func TestIsSafePermission_CustomToolKind(t *testing.T) {
	assert.False(t, isSafePermission(copilot.PermissionRequest{Kind: copilot.PermissionRequestKindCustomTool}))
}

// ---------------------------------------------------------------------------
// policyPermissionHandler — safe requests
// ---------------------------------------------------------------------------

func TestPolicyHandler_ApprovesRead(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequest{Kind: copilot.PermissionRequestKindRead},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindApproved, result.Kind)
}

func TestPolicyHandler_ApprovesMemory(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequest{Kind: copilot.PermissionRequestKindMemory},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindApproved, result.Kind)
}

func TestPolicyHandler_ApprovesReadOnlyMCP(t *testing.T) {
	ro := true
	result, err := policyPermissionHandler(
		copilot.PermissionRequest{
			Kind:     copilot.PermissionRequestKindMcp,
			ReadOnly: &ro,
		},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindApproved, result.Kind)
}

// ---------------------------------------------------------------------------
// policyPermissionHandler — dangerous requests
// ---------------------------------------------------------------------------

func TestPolicyHandler_DeniesWrite(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequest{Kind: copilot.PermissionRequestKindWrite},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindDeniedByRules, result.Kind)
}

func TestPolicyHandler_DeniesShell(t *testing.T) {
	cmd := "rm -rf /"
	result, err := policyPermissionHandler(
		copilot.PermissionRequest{
			Kind:            copilot.PermissionRequestKindShell,
			FullCommandText: &cmd,
		},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindDeniedByRules, result.Kind)
}

func TestPolicyHandler_DeniesURL(t *testing.T) {
	url := "https://evil.example.com"
	result, err := policyPermissionHandler(
		copilot.PermissionRequest{
			Kind: copilot.PermissionRequestKindURL,
			URL:  &url,
		},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindDeniedByRules, result.Kind)
}

func TestPolicyHandler_DeniesCustomTool(t *testing.T) {
	tool := "dangerous-tool"
	result, err := policyPermissionHandler(
		copilot.PermissionRequest{
			Kind:     copilot.PermissionRequestKindCustomTool,
			ToolName: &tool,
		},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindDeniedByRules, result.Kind)
}

func TestPolicyHandler_DeniesMCPWithWriteAccess(t *testing.T) {
	ro := false
	result, err := policyPermissionHandler(
		copilot.PermissionRequest{
			Kind:     copilot.PermissionRequestKindMcp,
			ReadOnly: &ro,
		},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindDeniedByRules, result.Kind)
}

func TestPolicyHandler_DeniesMCPWithNilReadOnly(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequest{Kind: copilot.PermissionRequestKindMcp},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindDeniedByRules, result.Kind)
}

// ---------------------------------------------------------------------------
// policyPermissionHandler — unknown kinds default to denied
// ---------------------------------------------------------------------------

func TestPolicyHandler_DeniesUnknownKind(t *testing.T) {
	result, err := policyPermissionHandler(
		copilot.PermissionRequest{Kind: "some-future-kind"},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindDeniedByRules, result.Kind)
}

// ---------------------------------------------------------------------------
// policyPermissionHandler — metadata passthrough (no panics)
// ---------------------------------------------------------------------------

func TestPolicyHandler_HandlesAllOptionalFields(t *testing.T) {
	intention := "read config"
	cmd := "cat /etc/hosts"
	tool := "file-reader"
	path := "/etc/hosts"
	file := "config.yaml"
	url := "https://example.com"

	// Safe request with all optional fields populated should not panic.
	result, err := policyPermissionHandler(
		copilot.PermissionRequest{
			Kind:            copilot.PermissionRequestKindRead,
			Intention:       &intention,
			FullCommandText: &cmd,
			ToolName:        &tool,
			Path:            &path,
			FileName:        &file,
			URL:             &url,
		},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindApproved, result.Kind)
}

func TestPolicyHandler_HandlesNilOptionalFields(t *testing.T) {
	// Dangerous request with all optional fields nil should not panic.
	result, err := policyPermissionHandler(
		copilot.PermissionRequest{Kind: copilot.PermissionRequestKindShell},
		copilot.PermissionInvocation{SessionID: "test-session"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindDeniedByRules, result.Kind)
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
		copilot.PermissionRequest{Kind: copilot.PermissionRequestKindShell},
		copilot.PermissionInvocation{SessionID: "test"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindDeniedByRules, result.Kind)

	// Invoke with a safe request — must be approved.
	result, err = cfg.OnPermissionRequest(
		copilot.PermissionRequest{Kind: copilot.PermissionRequestKindRead},
		copilot.PermissionInvocation{SessionID: "test"},
	)
	require.NoError(t, err)
	assert.Equal(t, copilot.PermissionRequestResultKindApproved, result.Kind)
}
