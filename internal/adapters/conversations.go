package adapters

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wyrd-company/lore/internal/rendering"
	"github.com/wyrd-company/lore/internal/synchronization"
)

type conversationData struct {
	Provider   string
	SessionID  string
	AgentID    string
	CWD        string
	Repository string
	Branch     string
	Title      string
	Messages   []rendering.Message
	Warnings   []string
}

type ConversationScan struct {
	Manifests []synchronization.Manifest
	Skipped   int
	Warnings  []string
}

func Conversations(provider, directory, sourceInstance string, mappings ProjectMappings, fallbackProject string) (ConversationScan, error) {
	files, err := conversationFiles(directory)
	if err != nil {
		return ConversationScan{}, err
	}
	byProject := make(map[string][]synchronization.Document)
	result := ConversationScan{}
	for _, path := range files {
		source, err := os.ReadFile(path)
		if err != nil {
			return result, fmt.Errorf("read conversation %q: %w", path, err)
		}
		var conversation conversationData
		switch provider {
		case "claude":
			conversation, err = parseClaude(source, path)
		case "codex":
			conversation, err = parseCodex(source, path)
		default:
			return result, fmt.Errorf("unsupported conversation provider %q", provider)
		}
		if err != nil {
			return result, fmt.Errorf("parse conversation %q: %w", path, err)
		}
		result.Warnings = append(result.Warnings, conversation.Warnings...)
		if len(conversation.Messages) == 0 || conversation.SessionID == "" {
			continue
		}
		if conversation.Repository == "" && conversation.CWD != "" {
			conversation.Repository, conversation.Branch, _ = deriveGit(conversation.CWD)
		}
		project := mappings.Resolve(SessionEvidence{
			SessionID: conversation.SessionID, CWD: conversation.CWD, Repository: conversation.Repository,
		}, fallbackProject)
		if project == "" && provider == "claude" {
			project = mappings.ResolveClaudeProjectDirectory(path)
		}
		if project == "" {
			result.Skipped++
			continue
		}
		rendered, err := rendering.Conversation(conversation.Messages)
		if err != nil {
			return result, err
		}
		metadata := map[string]any{
			"provider": provider, "sessionId": conversation.SessionID, "workingDirectory": conversation.CWD,
			"title": conversation.Title, "messages": conversation.Messages,
		}
		if conversation.AgentID != "" {
			metadata["agentId"] = conversation.AgentID
		}
		provenance := map[string]any{
			"path": path, "provider": provider, "repository": conversation.Repository, "branch": conversation.Branch,
		}
		normalizedSource, err := json.Marshal(conversation.Messages)
		if err != nil {
			return result, fmt.Errorf("encode normalized conversation: %w", err)
		}
		document, err := makeDocument(provider+":"+conversation.SessionID, conversation.Title, normalizedSource, "conversation", rendered, metadata, provenance, nil)
		if err != nil {
			return result, err
		}
		byProject[project] = append(byProject[project], document)
	}
	projects := mappings.Projects(fallbackProject)
	knownProjects := make(map[string]struct{}, len(projects))
	for _, project := range projects {
		knownProjects[project] = struct{}{}
	}
	for project := range byProject {
		if _, exists := knownProjects[project]; !exists {
			projects = append(projects, project)
		}
	}
	sort.Strings(projects)
	for _, project := range projects {
		documents := byProject[project]
		sort.Slice(documents, func(i, j int) bool { return documents[i].Identity < documents[j].Identity })
		manifest := synchronization.Manifest{
			Project: project, SourceInstance: sourceInstance, SourceType: "conversation",
			Boundary: synchronization.BoundaryComplete, Documents: documents,
		}
		metadata, _ := json.Marshal(map[string]string{"provider": provider})
		manifest.Metadata = metadata
		if err := manifest.Validate(); err != nil {
			return result, err
		}
		result.Manifests = append(result.Manifests, manifest)
	}
	return result, nil
}

func conversationFiles(directory string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Type().IsRegular() && strings.EqualFold(filepath.Ext(path), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func scanJSONLines(source []byte, consume func(int, []byte) string) ([]string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(source))
	scanner.Buffer(make([]byte, 64*1024), 32*1024*1024)
	var warnings []string
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if warning := consume(lineNumber, line); warning != "" {
			warnings = append(warnings, warning)
		}
	}
	return warnings, scanner.Err()
}

func conversationWarning(path string, line int, message string) string {
	return fmt.Sprintf("%s:%d: %s; record skipped", path, line, message)
}

func conversationTitle(messages []rendering.Message, fallback string) string {
	for _, message := range messages {
		if message.Role != "user" || message.Thinking {
			continue
		}
		title := strings.Join(strings.Fields(message.Markdown), " ")
		if len(title) > 100 {
			title = strings.TrimSpace(title[:100]) + "…"
		}
		if title != "" {
			return title
		}
	}
	return fallback
}

func isBookkeepingMessage(value string) bool {
	trimmed := strings.TrimSpace(value)
	prefixes := []string{"<system-reminder>", "<local-command-caveat>", "<command-name>", "<local-command-stdout>", "<task-notification>"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}
