package adapters

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wyrd-company/lore/internal/rendering"
)

type claudeRecord struct {
	Type      string `json:"type"`
	UUID      string `json:"uuid"`
	SessionID string `json:"sessionId"`
	AgentID   string `json:"agentId"`
	CWD       string `json:"cwd"`
	GitBranch string `json:"gitBranch"`
	Message   struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type claudeBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
}

func parseClaude(source []byte, path string) (conversationData, error) {
	conversation := conversationData{Provider: "claude", Branch: ""}
	err := scanJSONLines(source, func(line []byte) error {
		var record claudeRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return err
		}
		if conversation.SessionID == "" {
			conversation.SessionID = record.SessionID
			if record.AgentID != "" {
				conversation.SessionID = record.AgentID
			}
		}
		if conversation.CWD == "" {
			conversation.CWD = record.CWD
		}
		if conversation.Branch == "" {
			conversation.Branch = record.GitBranch
		}
		if record.Type != "user" && record.Type != "assistant" || record.Message.Role != "user" && record.Message.Role != "assistant" {
			return nil
		}
		messages, err := claudeMessages(record)
		if err != nil {
			return err
		}
		conversation.Messages = append(conversation.Messages, messages...)
		return nil
	})
	if err != nil {
		return conversation, err
	}
	if conversation.SessionID == "" {
		conversation.SessionID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	conversation.Title = conversationTitle(conversation.Messages, "Claude conversation "+conversation.SessionID)
	return conversation, nil
}

func claudeMessages(record claudeRecord) ([]rendering.Message, error) {
	var text string
	if err := json.Unmarshal(record.Message.Content, &text); err == nil {
		if text == "" || isBookkeepingMessage(text) {
			return nil, nil
		}
		return []rendering.Message{{ID: stableMessageID(record.UUID, 0), Role: record.Message.Role, Markdown: text}}, nil
	}
	var blocks []claudeBlock
	if err := json.Unmarshal(record.Message.Content, &blocks); err != nil {
		return nil, fmt.Errorf("parse Claude message content: %w", err)
	}
	var messages []rendering.Message
	for index, block := range blocks {
		var value string
		thinking := false
		switch block.Type {
		case "text":
			value = block.Text
		case "thinking":
			value = block.Thinking
			thinking = true
		default:
			continue
		}
		if value == "" || isBookkeepingMessage(value) {
			continue
		}
		messages = append(messages, rendering.Message{
			ID: stableMessageID(record.UUID, index), Role: record.Message.Role, Markdown: value, Thinking: thinking,
		})
	}
	return messages, nil
}

func stableMessageID(id string, index int) string {
	if id == "" {
		id = "message"
	}
	return fmt.Sprintf("%s-%d", id, index)
}
