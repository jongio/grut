package cmd

import "strings"

func textFilterMatches(filter string, fields ...string) bool {
	query := strings.TrimSpace(strings.ToLower(filter))
	if query == "" {
		return true
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}
