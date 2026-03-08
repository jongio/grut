package extension

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidPermission_AllKnown(t *testing.T) {
	known := []string{
		"file_read", "file_write",
		"git_read", "git_write",
		"network", "process",
		"clipboard", "notify",
	}
	for _, p := range known {
		t.Run(p, func(t *testing.T) {
			assert.True(t, ValidPermission(p), "expected %q to be valid", p)
		})
	}
}

func TestValidPermission_Unknown(t *testing.T) {
	unknown := []string{
		"admin", "root", "file_delete", "", "FILE_READ", "File_Read",
	}
	for _, p := range unknown {
		t.Run(p, func(t *testing.T) {
			assert.False(t, ValidPermission(p), "expected %q to be invalid", p)
		})
	}
}

func TestCheckPermission_HasPermission(t *testing.T) {
	ext := &ExtensionInfo{
		Manifest: Manifest{
			Permissions: []string{"file_read", "network"},
		},
	}
	assert.True(t, CheckPermission(ext, PermFileRead))
	assert.True(t, CheckPermission(ext, PermNetwork))
}

func TestCheckPermission_MissingPermission(t *testing.T) {
	ext := &ExtensionInfo{
		Manifest: Manifest{
			Permissions: []string{"file_read"},
		},
	}
	assert.False(t, CheckPermission(ext, PermGitWrite))
	assert.False(t, CheckPermission(ext, PermProcess))
}

func TestCheckPermission_NoPermissions(t *testing.T) {
	ext := &ExtensionInfo{
		Manifest: Manifest{},
	}
	assert.False(t, CheckPermission(ext, PermFileRead))
}

func TestAllPermissions_Complete(t *testing.T) {
	perms := AllPermissions()
	require.Len(t, perms, 8)

	// Verify every known permission is in the returned slice.
	set := make(map[Permission]struct{}, len(perms))
	for _, p := range perms {
		set[p] = struct{}{}
	}

	expected := []Permission{
		PermFileRead, PermFileWrite,
		PermGitRead, PermGitWrite,
		PermNetwork, PermProcess,
		PermClipboard, PermNotify,
	}
	for _, e := range expected {
		_, ok := set[e]
		assert.True(t, ok, "missing permission %q", e)
	}
}
