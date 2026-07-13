package indexing

import (
	"encoding/json"
	"strings"
)

const (
	maxChunkWords = 220
	overlapWords  = 40
)

type Chunk struct {
	Text       string
	Kind       string
	Location   map[string]any
	TokenCount int
}

type conversationMetadata struct {
	Messages []struct {
		ID       string `json:"id"`
		Role     string `json:"role"`
		Markdown string `json:"markdown"`
		Thinking bool   `json:"thinking"`
	} `json:"messages"`
}

func ChunkDocument(sourceType, normalizedText string, metadata json.RawMessage) []Chunk {
	if sourceType == "conversation" {
		var conversation conversationMetadata
		if json.Unmarshal(metadata, &conversation) == nil && len(conversation.Messages) > 0 {
			var chunks []Chunk
			for index, message := range conversation.Messages {
				kind := message.Role
				if message.Thinking {
					kind = "thinking"
				}
				if kind != "user" && kind != "assistant" && kind != "thinking" {
					kind = "body"
				}
				location := map[string]any{"messageId": message.ID, "messageIndex": index, "role": message.Role}
				chunks = append(chunks, chunkText(message.Markdown, kind, location)...)
			}
			return chunks
		}
	}
	return chunkText(normalizedText, "body", nil)
}

func chunkText(text, kind string, baseLocation map[string]any) []Chunk {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var chunks []Chunk
	for start := 0; start < len(words); {
		end := min(start+maxChunkWords, len(words))
		if end < len(words) {
			minimum := min(start+maxChunkWords*3/4, end)
			for candidate := end - 1; candidate >= minimum; candidate-- {
				if endsSentence(words[candidate]) {
					end = candidate + 1
					break
				}
			}
		}
		location := cloneLocation(baseLocation)
		location["startWord"] = start
		location["endWord"] = end
		chunks = append(chunks, Chunk{
			Text: strings.Join(words[start:end], " "), Kind: kind, Location: location, TokenCount: end - start,
		})
		if end == len(words) {
			break
		}
		start = max(end-overlapWords, start+1)
	}
	return chunks
}

func endsSentence(word string) bool {
	word = strings.TrimRight(word, `"')]}’”`)
	return strings.HasSuffix(word, ".") || strings.HasSuffix(word, "!") || strings.HasSuffix(word, "?")
}

func cloneLocation(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+2)
	for key, value := range source {
		result[key] = value
	}
	return result
}
