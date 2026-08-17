package chat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/ai"
)

var chatBenchmarkSink string

func BenchmarkRenderModalMessageHistory(b *testing.B) {
	for _, count := range []int{10, 100, 200} {
		b.Run(fmt.Sprintf("%d_messages", count), func(b *testing.B) {
			m := newViewTestModel(b)
			m.renderMD = true
			m.messages = benchmarkMessages(count)
			m.RenderModalContent(100, 40)

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				chatBenchmarkSink = m.RenderModalContent(100, 40)
			}
		})
	}
}

func BenchmarkStreamingChunks(b *testing.B) {
	for _, count := range []int{10, 100, 200} {
		b.Run(fmt.Sprintf("%d_chunks", count), func(b *testing.B) {
			m := newViewTestModel(b)
			m.streaming = true
			m.expanded = true
			chunks := benchmarkStreamChunks(count)

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				m.resetStream()
				for _, chunk := range chunks {
					m, _ = m.Update(StreamChunkMsg{Chunk: ai.StreamChunk{Delta: chunk}})
					chatBenchmarkSink = m.renderStreaming()
				}
			}
		})
	}
}

func benchmarkMessages(count int) []ai.ChatMessage {
	messages := make([]ai.ChatMessage, count)
	for i := range messages {
		role := RoleUser
		content := fmt.Sprintf("Question %d about repository state and the files changed.", i)
		if i%2 == 1 {
			role = RoleAssistant
			content = fmt.Sprintf(
				"## Response %d\n\nThe repository has **several** changes.\n\n%s",
				i,
				strings.Repeat("- cached markdown line\n", 3),
			)
		}
		messages[i] = ai.ChatMessage{Role: role, Content: content}
	}
	return messages
}

func benchmarkStreamChunks(count int) []string {
	chunks := make([]string, count)
	for i := range chunks {
		if i%8 == 7 {
			chunks[i] = "completes the current streamed sentence.\n"
		} else {
			chunks[i] = "streamed words "
		}
	}
	return chunks
}
