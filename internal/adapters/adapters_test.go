package adapters

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/wyrd-company/lore/internal/synchronization"
)

func fixtureOptions(instance string) Options {
	return Options{Project: "lore", SourceInstance: instance, Boundary: synchronization.BoundaryComplete}
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
