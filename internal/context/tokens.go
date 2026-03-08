// Package context provides context-building utilities for AI chat workflows.
// It supports manual file selection, token counting, and structured export
// of repository files for pasting into AI conversations.
package context

import (
	"math"
	"strings"
)

// CountTokens estimates the number of tokens in text using a word-based
// approximation. The multiplier (1.3) accounts for sub-word tokenisation
// used by models like GPT-4 (cl100k_base encoding). This is intentionally
// a simple heuristic — exact counts are not required for context budgeting.
func CountTokens(text string) int {
	if text == "" {
		return 0
	}
	words := len(strings.Fields(text))
	return int(math.Ceil(float64(words) * 1.3))
}
