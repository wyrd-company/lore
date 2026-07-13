package adapters

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wyrd-company/lore/internal/rendering"
)

type codexRecord struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type codexPayload struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	Role      string `json:"role"`
	Git       struct {
		RepositoryURL string `json:"repository_url"`
		Branch        string `json:"branch"`
	} `json:"git"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Summary []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"summary"`
}

func parseCodex(source []byte, path string) (conversationData, error) {
	conversation := conversationData{Provider: "codex"}
	sequence := 0
	warnings, err := scanJSONLines(source, func(lineNumber int, line []byte) string {
		var record codexRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return conversationWarning(path, lineNumber, "malformed JSON: "+err.Error())
		}
		var payload codexPayload
		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			return conversationWarning(path, lineNumber, "malformed Codex payload: "+err.Error())
		}
		switch record.Type {
		case "session_meta":
			conversation.SessionID = payload.ID
			if conversation.SessionID == "" {
				conversation.SessionID = payload.SessionID
			}
			conversation.CWD = payload.CWD
			conversation.Repository = payload.Git.RepositoryURL
			conversation.Branch = payload.Git.Branch
		case "response_item":
			switch payload.Type {
			case "message":
				if payload.Role != "user" && payload.Role != "assistant" {
					return ""
				}
				for _, block := range payload.Content {
					if block.Type != "input_text" && block.Type != "output_text" || block.Text == "" || isBookkeepingMessage(block.Text) {
						continue
					}
					conversation.Messages = append(conversation.Messages, rendering.Message{
						ID: stableMessageID(payload.ID, sequence), Role: payload.Role, Markdown: block.Text,
					})
					sequence++
				}
			case "reasoning":
				for _, summary := range payload.Summary {
					if summary.Text == "" {
						continue
					}
					conversation.Messages = append(conversation.Messages, rendering.Message{
						ID: stableMessageID(payload.ID, sequence), Role: "assistant", Markdown: summary.Text, Thinking: true,
					})
					sequence++
				}
			default:
				if !knownCodexResponseType(payload.Type) {
					return conversationWarning(path, lineNumber, fmt.Sprintf("unknown Codex response type %q", payload.Type))
				}
			}
		default:
			if !knownCodexRecordType(record.Type) {
				return conversationWarning(path, lineNumber, fmt.Sprintf("unknown Codex record type %q", record.Type))
			}
		}
		return ""
	})
	if err != nil {
		return conversation, fmt.Errorf("parse Codex JSONL: %w", err)
	}
	conversation.Warnings = warnings
	if conversation.SessionID == "" {
		conversation.SessionID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	conversation.Title = conversationTitle(conversation.Messages, "Codex conversation "+conversation.SessionID)
	return conversation, nil
}

func knownCodexRecordType(recordType string) bool {
	switch recordType {
	case "event_msg", "inter_agent_communication_metadata", "turn_context", "world_state":
		return true
	default:
		return false
	}
}

func knownCodexResponseType(responseType string) bool {
	switch responseType {
	case "agent_message", "custom_tool_call", "custom_tool_call_output", "function_call", "function_call_output",
		"tool_search_call", "tool_search_output", "web_search_call":
		return true
	default:
		return false
	}
}
