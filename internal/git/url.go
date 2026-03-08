package git

import "strings"

// RemoteToHTTPS converts a git remote URL (SSH, HTTPS, or ssh://) to an
// HTTPS URL suitable for opening in a browser. Returns "" for unrecognised
// formats or empty input.
func RemoteToHTTPS(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// SSH format: git@github.com:user/repo.git
	if strings.HasPrefix(raw, "git@") {
		raw = strings.TrimPrefix(raw, "git@")
		raw = strings.TrimSuffix(raw, ".git")
		if idx := strings.IndexByte(raw, ':'); idx >= 0 {
			raw = raw[:idx] + "/" + raw[idx+1:]
		}
		return "https://" + raw
	}
	// Already HTTPS or HTTP
	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") {
		return strings.TrimSuffix(raw, ".git")
	}
	// ssh://git@github.com/user/repo.git
	if strings.HasPrefix(raw, "ssh://") {
		raw = strings.TrimPrefix(raw, "ssh://")
		raw = strings.TrimPrefix(raw, "git@")
		raw = strings.TrimSuffix(raw, ".git")
		return "https://" + raw
	}
	return ""
}
