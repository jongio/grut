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
	for _, p := range ext.Manifest.Permissions {
		if Permission(p) == perm {
			return true
		}
	}
	return false
}

// AllPermissions returns every valid permission value.
func AllPermissions() []Permission {
	out := make([]Permission, 0, len(allPermissions))
	for p := range allPermissions {
		out = append(out, p)
	}
	return out
}
