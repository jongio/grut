package extension

// Permission represents a capability an extension can request.
type Permission string

// Valid extension permissions.
const (
	PermFileRead  Permission = "file_read"
	PermFileWrite Permission = "file_write"
	PermGitRead   Permission = "git_read"
	PermGitWrite  Permission = "git_write"
	PermNetwork   Permission = "network"
	PermProcess   Permission = "process"
	PermClipboard Permission = "clipboard"
	PermNotify    Permission = "notify"
)

// allPermissions is the canonical set of valid permissions.
var allPermissions = map[Permission]struct{}{
	PermFileRead:  {},
	PermFileWrite: {},
	PermGitRead:   {},
	PermGitWrite:  {},
	PermNetwork:   {},
	PermProcess:   {},
	PermClipboard: {},
	PermNotify:    {},
}

// ValidPermission reports whether p is a recognised permission name.
func ValidPermission(p string) bool {
	_, ok := allPermissions[Permission(p)]
	return ok
}

// CheckPermission reports whether ext has been granted perm.
func CheckPermission(ext *ExtensionInfo, perm Permission) bool {
	return ManifestHasPermission(&ext.Manifest, perm)
}

// ManifestHasPermission reports whether m declares perm in its permissions
// list. This is the manifest-level equivalent of CheckPermission and is used
// by runtimes that hold a *Manifest rather than a full *ExtensionInfo.
func ManifestHasPermission(m *Manifest, perm Permission) bool {
	for _, p := range m.Permissions {
		if Permission(p) == perm {
			return true
		}
	}
	return false
}

// ErrPermissionDenied is returned when an extension attempts an operation
// that requires a permission it has not declared.
type ErrPermissionDenied struct {
	Extension  string
	Permission Permission
	Operation  string
}

func (e *ErrPermissionDenied) Error() string {
	return "extension " + e.Extension + ": permission " + string(e.Permission) +
		" is required for " + e.Operation
}

// AllPermissions returns every valid permission value.
func AllPermissions() []Permission {
	out := make([]Permission, 0, len(allPermissions))
	for p := range allPermissions {
		out = append(out, p)
	}
	return out
}
