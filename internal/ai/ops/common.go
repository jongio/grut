package ops

import "strings"

// stripCodeFences removes optional markdown code fences (```json ... ```)
// that some models wrap around their JSON output.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)

	// Handle ```json or ``` prefix.
	if strings.HasPrefix(s, "```") {
		// Remove first line (the opening fence).
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
	}

	// Remove trailing ```.
	s = strings.TrimSuffix(s, "```")

	return strings.TrimSpace(s)
}
