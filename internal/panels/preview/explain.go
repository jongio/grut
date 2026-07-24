package preview

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

const explainPayloadMaxRunes = 60000

type explainPromptMode int

const (
	explainPromptFile explainPromptMode = iota
	explainPromptDiff
	explainPromptCommit
)

func buildExplainPrompt(mode explainPromptMode, path, content, diffText, commitRef string) string {
	switch mode {
	case explainPromptCommit:
		if commitRef == "" {
			return ""
		}
		return fmt.Sprintf("Explain this commit: %s", commitRef)
	case explainPromptDiff:
		diffText = trimExplainPayload(cleanExplainPayload(diffText))
		if diffText == "" {
			return ""
		}
		if path != "" {
			return fmt.Sprintf("Explain this diff for %s:\n\n```diff\n%s\n```", path, diffText)
		}
		return fmt.Sprintf("Explain this diff:\n\n```diff\n%s\n```", diffText)
	case explainPromptFile:
		content = trimExplainPayload(cleanExplainPayload(content))
		if path == "" || content == "" {
			return ""
		}
		return fmt.Sprintf("Explain what this file does: %s\n\n```\n%s\n```", path, content)
	default:
		return ""
	}
}

func cleanExplainPayload(s string) string {
	s = ansi.Strip(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

func trimExplainPayload(s string) string {
	if utf8.RuneCountInString(s) <= explainPayloadMaxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:explainPayloadMaxRunes]) + "\n\n[truncated]"
}
