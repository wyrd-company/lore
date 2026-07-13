package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wyrd-company/lore/internal/synchronization"
)

func fixtureOptions(instance string) Options {
	return Options{Project: "lore", SourceInstance: instance, Boundary: synchronization.BoundaryComplete}
}

func TestTasksUsesKanbanDefaultsWhenTaskOmitsStatusAndPriority(t *testing.T) {
	board := t.TempDir()
	if err := os.Mkdir(filepath.Join(board, "cards"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := []byte("tasks_dir: cards\ndefaults:\n  status: ready\n  priority: high\n  class: standard\n")
	if err := os.WriteFile(filepath.Join(board, "config.yml"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	task := []byte("---\nid: 7\ntitle: Defaulted task\n---\n\nTask body.\n")
	if err := os.WriteFile(filepath.Join(board, "cards", "7-defaulted.md"), task, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := Tasks(board, fixtureOptions("tasks"))
	if err != nil {
		t.Fatalf("read task using board defaults: %v", err)
	}
	if len(manifest.Documents) != 1 {
		t.Fatalf("documents = %d", len(manifest.Documents))
	}
	metadata := string(manifest.Documents[0].Metadata)
	if !strings.Contains(metadata, `"status":"ready"`) || !strings.Contains(metadata, `"priority":"high"`) || !strings.Contains(metadata, `"class":"standard"`) {
		t.Fatalf("defaults missing from task metadata: %s", metadata)
	}
}

func TestTasksFallsBackToKanbanDefaultValues(t *testing.T) {
	board := t.TempDir()
	if err := os.Mkdir(filepath.Join(board, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(board, "config.yml"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	task := []byte("---\nid: 8\ntitle: Fallback task\n---\n")
	if err := os.WriteFile(filepath.Join(board, "tasks", "8-fallback.md"), task, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := Tasks(board, fixtureOptions("tasks"))
	if err != nil {
		t.Fatal(err)
	}
	metadata := string(manifest.Documents[0].Metadata)
	if !strings.Contains(metadata, `"status":"backlog"`) || !strings.Contains(metadata, `"priority":"medium"`) {
		t.Fatalf("fallback defaults missing: %s", metadata)
	}
}

func TestTasksReadsKanbanBoard(t *testing.T) {
	manifest, err := Tasks(filepath.Join("testdata", "kanban"), fixtureOptions("tasks"))
	if err != nil {
		t.Fatalf("read task board: %v", err)
	}
	if len(manifest.Documents) != 2 || len(manifest.Relationships) != 1 {
		t.Fatalf("unexpected task manifest: documents=%d relationships=%d", len(manifest.Documents), len(manifest.Relationships))
	}
	first := manifest.Documents[0]
	if first.Identity != "1" || first.Title != "Build foundation" || strings.Join(first.Tags, ",") != "architecture,lore" {
		t.Fatalf("unexpected first task: %#v", first)
	}
	if !strings.Contains(first.NormalizedText, "foundation") || !strings.Contains(first.RenderedContent, "lore-task-meta") {
		t.Fatalf("task content was not indexed/rendered: %#v", first)
	}
	relationship := manifest.Relationships[0]
	if relationship.SourceIdentity != "2" || relationship.TargetIdentity != "1" || relationship.Type != "task-depends-on" {
		t.Fatalf("unexpected dependency: %#v", relationship)
	}
}

func TestNotesReadsMnemonicMarkdown(t *testing.T) {
	manifest, err := Notes(filepath.Join("testdata", "notes"), fixtureOptions("notes"))
	if err != nil {
		t.Fatalf("read notes: %v", err)
	}
	if len(manifest.Documents) != 1 {
		t.Fatalf("documents = %d", len(manifest.Documents))
	}
	document := manifest.Documents[0]
	if document.Identity != "note-identity" || document.Title != "Adapter finding" || strings.Join(document.Tags, ",") != "adapters,lore" {
		t.Fatalf("unexpected note: %#v", document)
	}
	if strings.Contains(document.NormalizedText, "lifecycle") || !strings.Contains(document.RenderedContent, `<h1 id="adapter-finding">`) {
		t.Fatalf("front matter leaked or heading missing: %#v", document)
	}
}

func TestBriefingExtractsTrustedBody(t *testing.T) {
	manifest, err := Briefing(filepath.Join("testdata", "briefing", "architecture.html"), "", fixtureOptions("briefing"))
	if err != nil {
		t.Fatalf("read briefing: %v", err)
	}
	document := manifest.Documents[0]
	if document.Title != "Architecture" || strings.Contains(document.RenderedContent, "Ignored head") || strings.Contains(document.RenderedContent, "<style") {
		t.Fatalf("unexpected briefing: %#v", document)
	}
	if !strings.Contains(document.NormalizedText, "source files authoritative") || !strings.Contains(string(document.Metadata), `"architecture"`) {
		t.Fatalf("briefing index metadata missing: %#v", document)
	}
}

func TestRepositoryRendersMarkdownAndYAML(t *testing.T) {
	manifest, err := Repository([]string{filepath.Join("testdata", "repository")}, RepositoryOptions{
		Options: fixtureOptions("repository"), Repository: "wyrd-company/fixture", Branch: "main",
	})
	if err != nil {
		t.Fatalf("read repository: %v", err)
	}
	if len(manifest.Documents) != 2 {
		t.Fatalf("documents = %d", len(manifest.Documents))
	}
	renderers := map[string]bool{}
	for _, document := range manifest.Documents {
		renderers[document.Renderer] = true
		if !strings.HasPrefix(document.Identity, "wyrd-company/fixture@main:") {
			t.Fatalf("identity did not include repository and branch: %q", document.Identity)
		}
	}
	if !renderers["markdown"] || !renderers["yaml"] {
		t.Fatalf("renderers = %#v", renderers)
	}
}

func TestRepositoryIncludesDotDirectoriesExceptGitMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".github", "workflows", "ci.yml"), []byte("name: CI\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("private git metadata"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := collectFiles([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !strings.Contains(filepath.ToSlash(files[0]), "/.github/workflows/ci.yml") {
		t.Fatalf("collected files = %#v", files)
	}
}
