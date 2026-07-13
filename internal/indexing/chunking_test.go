package indexing

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestChunkDocumentIsBoundedAndOverlapping(t *testing.T) {
	t.Parallel()
	words := make([]string, 500)
	for index := range words {
		words[index] = fmt.Sprintf("word-%d", index)
	}
	chunks := ChunkDocument("note", strings.Join(words, " "), nil)
	if len(chunks) != 3 {
		t.Fatalf("chunks = %d, want 3", len(chunks))
	}
	for _, chunk := range chunks {
		if chunk.TokenCount > maxChunkWords {
			t.Fatalf("chunk has %d words", chunk.TokenCount)
		}
	}
	if !strings.Contains(chunks[1].Text, "word-180") || !strings.Contains(chunks[0].Text, "word-219") {
		t.Fatalf("expected overlap between first two chunks")
	}
	if chunks[0].Location["startWord"] != 0 || chunks[0].Location["endWord"] != 220 {
		t.Fatalf("unexpected location: %#v", chunks[0].Location)
	}
}

func TestConversationChunksRetainMessageLocationAndThinkingKind(t *testing.T) {
	t.Parallel()
	metadata := json.RawMessage(`{"messages":[{"id":"user-1","role":"user","markdown":"Ask a question"},{"id":"thought-1","role":"assistant","markdown":"Private reasoning","thinking":true}]}`)
	chunks := ChunkDocument("conversation", "flattened text", metadata)
	if len(chunks) != 2 || chunks[0].Kind != "user" || chunks[1].Kind != "thinking" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
	if chunks[1].Location["messageId"] != "thought-1" || chunks[1].Location["role"] != "assistant" {
		t.Fatalf("thinking location = %#v", chunks[1].Location)
	}
}
