package context

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCountTokens_Empty(t *testing.T) {
	assert.Equal(t, 0, CountTokens(""))
}

func TestCountTokens_SingleWord(t *testing.T) {
	tokens := CountTokens("hello")
	assert.Greater(t, tokens, 0)
	assert.Equal(t, 2, tokens) // ceil(1 * 1.3) = 2
}

func TestCountTokens_KnownSentence(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog"
	tokens := CountTokens(text) // 9 words → ceil(9*1.3) = 12
	assert.Equal(t, 12, tokens)
}

func TestCountTokens_MultiLine(t *testing.T) {
	text := "line one\nline two\nline three"
	tokens := CountTokens(text) // 6 words → ceil(6*1.3) = 8
	assert.Equal(t, 8, tokens)
}

func TestCountTokens_LargeText(t *testing.T) {
	// Build a large string of repeated words.
	words := make([]string, 1000)
	for i := range words {
		words[i] = "word"
	}
	text := strings.Join(words, " ")
	tokens := CountTokens(text) // 1000 words → ceil(1000*1.3) = 1300
	assert.Equal(t, 1300, tokens)
}

func TestCountTokens_WhitespaceOnly(t *testing.T) {
	// strings.Fields on whitespace-only returns empty slice.
	assert.Equal(t, 0, CountTokens("   \t\n  "))
}
