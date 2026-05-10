package mcp

import (
	"os"
	"strings"
)

// safeAgentEnvAllowlist lists environment variable names (uppercased) that
// are safe to propagate to spawned agent subprocesses. Secrets, tokens, and
// credentials are intentionally excluded (CWE-526).
var safeAgentEnvAllowlist = map[string]struct{}{
	"PATH":               {},
	"HOME":               {},
	"TMPDIR":             {},
	"TMP":                {},
	"TEMP":               {},
	"LANG":               {},
	"LC_ALL":             {},
	"TERM":               {},
	"USER":               {},
	"USERNAME":           {},
	"SHELL":              {},
	"COMSPEC":            {},
	"SYSTEMROOT":         {},
	"WINDIR":             {},
	"USERPROFILE":        {},
	"LOCALAPPDATA":       {},
	"APPDATA":            {},
	"PROGRAMFILES":       {},
	"PROGRAMFILES(X86)":  {},
	"COMMONPROGRAMFILES": {},
	"PROGRAMDATA":        {},
}

// filterEnvForAgent returns a filtered copy of the current process
// environment containing only safe variables. Secrets and tokens are excluded
// to prevent credential leaks to untrusted agent subprocesses.
func filterEnvForAgent() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		name, _, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if _, allowed := safeAgentEnvAllowlist[strings.ToUpper(name)]; allowed {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
