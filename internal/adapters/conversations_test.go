package adapters

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectResolutionPrecedence(t *testing.T) {
	mappings := ProjectMappings{
		Sessions: map[string]string{"session": "explicit"},
		Paths: []PathMapping{
			{Prefix: "/workspaces", Project: "broad"},
			{Prefix: "/workspaces/tools/lore", Project: "path"},
		},
		Repositories:         map[string]string{"git@example.test:group/repo.git": "mapped-repo"},
		AllowProjectFallback: true,
	}
	tests := []struct {
		name     string
		evidence SessionEvidence
		fallback string
		want     string
	}{
		{"explicit", SessionEvidence{SessionID: "session", CWD: "/workspaces/tools/lore"}, "fallback", "explicit"},
		{"longest path", SessionEvidence{CWD: "/workspaces/tools/lore/internal"}, "fallback", "path"},
		{"mapped repository", SessionEvidence{Repository: "git@example.test:group/repo.git"}, "fallback", "mapped-repo"},
		{"derived repository", SessionEvidence{Repository: "git@github.com:wyrd-company/lore.git"}, "fallback", "lore"},
		{"fallback", SessionEvidence{}, "fallback", "fallback"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := mappings.Resolve(test.evidence, test.fallback); got != test.want {
				t.Fatalf("Resolve() = %q, want %q", got, test.want)
			}
		})
	}
	mappings.AllowProjectFallback = false
	if got := mappings.Resolve(SessionEvidence{}, "fallback"); got != "" {
		t.Fatalf("opt-out fallback resolved %q", got)
	}
	if got := mappings.ResolveClaudeProjectDirectory("/home/vscode/.claude/projects/-workspaces-tools-lore/session.jsonl"); got != "path" {
		t.Fatalf("Claude project directory resolved to %q", got)
	}
}

func TestClaudeConversationExcludesToolsAndBookkeeping(t *testing.T) {
	scan, err := Conversations("claude", filepath.Join("testdata", "conversations", "claude"), "claude-sessions", ProjectMappings{
		Sessions: map[string]string{"claude-session": "lore"},
	}, "")
	if err != nil {
		t.Fatalf("scan Claude conversations: %v", err)
	}
	if len(scan.Manifests) != 1 || len(scan.Manifests[0].Documents) != 1 || scan.Skipped != 0 {
		t.Fatalf("unexpected scan: %#v", scan)
	}
	document := scan.Manifests[0].Documents[0]
	for _, excluded := range []string{"private tool output", "system-reminder", "secret"} {
		if strings.Contains(document.NormalizedText, excluded) || strings.Contains(document.RenderedContent, excluded) || strings.Contains(string(document.Metadata), excluded) {
			t.Errorf("Claude document contains excluded %q", excluded)
		}
	}
	if !strings.Contains(document.NormalizedText, "Thinking: Inspect the source format.") || !strings.Contains(document.RenderedContent, "lore-thinking") {
		t.Fatalf("Claude thinking not retained: %#v", document)
	}
}

func TestCodexConversationUsesGitProjectAndExcludesPrivateRecords(t *testing.T) {
	scan, err := Conversations("codex", filepath.Join("testdata", "conversations", "codex"), "codex-sessions", ProjectMappings{}, "")
	if err != nil {
		t.Fatalf("scan Codex conversations: %v", err)
	}
	if len(scan.Manifests) != 1 || scan.Manifests[0].Project != "lore" {
		t.Fatalf("Codex git project was not resolved: %#v", scan)
	}
	document := scan.Manifests[0].Documents[0]
	for _, excluded := range []string{"Private developer instruction", "private", "not-indexed"} {
		if strings.Contains(document.NormalizedText, excluded) || strings.Contains(document.RenderedContent, excluded) || strings.Contains(string(document.Metadata), excluded) {
			t.Errorf("Codex document contains excluded %q", excluded)
		}
	}
	if !strings.Contains(document.NormalizedText, "Implement Codex normalization") || !strings.Contains(document.NormalizedText, "Thinking: Identify message records") {
		t.Fatalf("expected messages missing: %#v", document)
	}
}

func TestUnassignedConversationIsSkipped(t *testing.T) {
	scan, err := Conversations("claude", filepath.Join("testdata", "conversations", "unassigned"), "sessions", ProjectMappings{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if scan.Skipped != 1 || len(scan.Manifests) != 0 {
		t.Fatalf("unassigned scan = %#v", scan)
	}
}
