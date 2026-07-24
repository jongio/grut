package preview

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildExplainPromptFile(t *testing.T) {
	prompt := buildExplainPrompt(explainPromptFile, "internal/app.go", "package main\nfunc main() {}", "", "")

	assert.Contains(t, prompt, "Explain what this file does: internal/app.go")
	assert.Contains(t, prompt, "package main")
	assert.Contains(t, prompt, "func main() {}")
}

func TestBuildExplainPromptDiff(t *testing.T) {
	diff := "diff --git a/app.go b/app.go\n+func main() {}\n"
	prompt := buildExplainPrompt(explainPromptDiff, "app.go", "", diff, "")

	assert.Contains(t, prompt, "Explain this diff for app.go")
	assert.Contains(t, prompt, "```diff")
	assert.Contains(t, prompt, "+func main() {}")
}

func TestBuildExplainPromptCommit(t *testing.T) {
	prompt := buildExplainPrompt(explainPromptCommit, "", "", "", "abc1234")

	assert.Equal(t, "Explain this commit: abc1234", prompt)
}

func TestBuildExplainPromptTruncatesPayload(t *testing.T) {
	content := strings.Repeat("a", explainPayloadMaxRunes+1)
	prompt := buildExplainPrompt(explainPromptFile, "large.txt", content, "", "")

	assert.Contains(t, prompt, "[truncated]")
}
