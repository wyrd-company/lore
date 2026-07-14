package adapters

import (
	"encoding/json"
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
	var board taskBoardMetadata
	if err := json.Unmarshal(manifest.Metadata, &board); err != nil {
		t.Fatalf("decode board metadata: %v", err)
	}
	if got := strings.Join(board.Statuses, ","); got != "done,Ready for deploy,backlog,archived,in progress" {
		t.Fatalf("board status order = %q", got)
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

func TestTasksAcceptsStringStatusConfigurationAndAppendsTaskStatuses(t *testing.T) {
	board := t.TempDir()
	if err := os.Mkdir(filepath.Join(board, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(board, "config.yml"), []byte("statuses: [done, backlog]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	task := []byte("---\nid: 9\ntitle: Custom lane task\nstatus: Ready for deploy\n---\n")
	if err := os.WriteFile(filepath.Join(board, "tasks", "9-custom.md"), task, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := Tasks(board, fixtureOptions("tasks"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata taskBoardMetadata
	if err := json.Unmarshal(manifest.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(metadata.Statuses, ","); got != "done,backlog,Ready for deploy" {
		t.Fatalf("board status order = %q", got)
	}
}

func TestNotesReadsMnemonicMarkdown(t *testing.T) {
	manifest, err := Notes(filepath.Join("testdata", "notes"), fixtureOptions("notes"))
	if err != nil {
		t.Fatalf("read notes: %v", err)
	}
	if len(manifest.Documents) != 2 {
		t.Fatalf("documents = %d", len(manifest.Documents))
	}
	document := manifest.Documents[0]
	if document.Identity != "note-identity" || document.Title != "Adapter finding" || strings.Join(document.Tags, ",") != "adapters,lore" {
		t.Fatalf("unexpected note: %#v", document)
	}
	if strings.Contains(document.NormalizedText, "lifecycle") || !strings.Contains(document.RenderedContent, `<h1 id="adapter-finding">`) {
		t.Fatalf("front matter leaked or heading missing: %#v", document)
	}
	if len(manifest.Relationships) != 1 || manifest.Relationships[0].TargetIdentity != "related-note" {
		t.Fatalf("note relationships = %#v", manifest.Relationships)
	}
}

func TestNotesExtractsTermsAndRelatedNotes(t *testing.T) {
	directory := t.TempDir()
	first := []byte("---\ntitle: First\nterms: [Knowledge Portal]\nrelatedTo:\n  - id: second\n    type: related-to\n---\nFirst note.\n")
	second := []byte("---\ntitle: Second\n---\nSecond note.\n")
	if err := os.WriteFile(filepath.Join(directory, "first.md"), first, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "second.md"), second, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := Notes(directory, fixtureOptions("notes"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(manifest.Documents[0].Terms, ","); got != "knowledge-portal" {
		t.Fatalf("terms = %q", got)
	}
	if len(manifest.Relationships) != 1 || manifest.Relationships[0].Type != "note-related-to" || manifest.Relationships[0].TargetIdentity != "second" {
		t.Fatalf("relationships = %#v", manifest.Relationships)
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
	if len(manifest.Documents) != 3 {
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
	var model synchronization.Document
	for _, document := range manifest.Documents {
		if strings.HasSuffix(document.Identity, "model.yml") {
			model = document
		}
	}
	if strings.Join(model.Tags, ",") != "architecture" || strings.Join(model.Terms, ",") != "knowledge-portal" || !strings.Contains(string(model.Metadata), `"schemaType":"model"`) {
		t.Fatalf("YAML taxonomy metadata missing: %#v", model)
	}
	if model.Title != "project" {
		t.Fatalf("YAML metadata replaced the content title: %q", model.Title)
	}
}

func TestRepositoryCollectsTermDefinitionsBySchemaAndFilename(t *testing.T) {
	root := t.TempDir()
	source := []byte("$schema: https://refinery.systems/ontology/term\ntitle: Knowledge Portal\ndescription: A shared archive.\ntags: [Domain Language]\nterms: [knowledge]\n")
	if err := os.WriteFile(filepath.Join(root, "knowledge-portal.yml"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := Repository([]string{root}, RepositoryOptions{Options: fixtureOptions("repository"), Repository: "wyrd-company/fixture", Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	document := manifest.Documents[0]
	if document.Title != "Knowledge Portal" || document.DefinesTerm != "knowledge-portal" || strings.Join(document.Terms, ",") != "knowledge" || strings.Join(document.Tags, ",") != "domain-language" {
		t.Fatalf("term document = %#v", document)
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
