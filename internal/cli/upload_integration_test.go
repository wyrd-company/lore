package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wyrd-company/lore/internal/browse"
	"github.com/wyrd-company/lore/internal/database"
	"github.com/wyrd-company/lore/internal/httpapi"
)

func TestSourceUploadsThroughCLIAndServerWithPostgres(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; real PostgreSQL integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA %s`, schema)); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), fmt.Sprintf(`DROP SCHEMA %s CASCADE`, schema)) //nolint:errcheck

	testURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := testURL.Query()
	query.Set("search_path", schema+",public")
	testURL.RawQuery = query.Encode()
	if err := database.Migrate(ctx, testURL.String()); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, testURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, slug, name) VALUES ($1, 'lore', 'Lore'), ($2, 'other', 'Other')`, uuid.New(), uuid.New()); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(pool, "ingest-secret", "admin-secret"))
	defer server.Close()
	var output bytes.Buffer
	runner := New(&output, &output)
	fixtures := filepath.Join("..", "adapters", "testdata")
	uploads := [][]string{
		{"upload", "tasks", "--project", "lore", "--source-instance", "kanban", "--complete", "--server", server.URL, "--token", "ingest-secret", filepath.Join(fixtures, "kanban")},
		{"upload", "notes", "--project", "lore", "--source-instance", "mnemonic", "--complete", "--server", server.URL, "--token", "ingest-secret", filepath.Join(fixtures, "notes")},
		{"upload", "briefing", "--project", "lore", "--source-instance", "architecture", "--server", server.URL, "--token", "ingest-secret", filepath.Join(fixtures, "briefing", "architecture.html")},
		{"upload", "repository", "--project", "lore", "--source-instance", "fixture-repository", "--repository", "wyrd-company/fixture", "--branch", "main", "--server", server.URL, "--token", "ingest-secret", filepath.Join(fixtures, "repository")},
		{"upload", "conversations", "--source-instance", "codex", "--provider", "codex", "--server", server.URL, "--token", "ingest-secret", filepath.Join(fixtures, "conversations", "codex")},
	}
	for _, arguments := range uploads {
		if err := runner.Run(ctx, arguments); err != nil {
			t.Fatalf("CLI upload %s: %v\n%s", arguments[1], err, output.String())
		}
	}
	var documents, revisions, tags, relationships int
	row := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM documents WHERE deleted_at IS NULL),
		(SELECT count(*) FROM revisions WHERE rendered_content <> '' AND normalized_text <> ''),
		(SELECT count(*) FROM document_tags),
		(SELECT count(*) FROM relationships)`)
	if err := row.Scan(&documents, &revisions, &tags, &relationships); err != nil {
		t.Fatal(err)
	}
	if documents != 7 || revisions != 7 || tags == 0 || relationships != 1 {
		t.Fatalf("unexpected persistence: documents=%d revisions=%d tags=%d relationships=%d", documents, revisions, tags, relationships)
	}
	if !strings.Contains(output.String(), "2 created") {
		t.Fatalf("unexpected CLI output: %s", output.String())
	}
	rows, err := pool.Query(ctx, `SELECT source_type, count(*) FROM documents WHERE deleted_at IS NULL GROUP BY source_type`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var sourceType string
		var count int
		if err := rows.Scan(&sourceType, &count); err != nil {
			t.Fatal(err)
		}
		counts[sourceType] = count
	}
	want := map[string]int{"task": 2, "note": 1, "briefing": 1, "repository": 2, "conversation": 1}
	for sourceType, count := range want {
		if counts[sourceType] != count {
			t.Fatalf("%s documents = %d, want %d (all: %#v)", sourceType, counts[sourceType], count, counts)
		}
	}

	var projects struct {
		Projects []browse.ProjectSummary `json:"projects"`
	}
	getJSON(t, server.URL+"/api/projects", http.StatusOK, &projects)
	if len(projects.Projects) != 2 || projects.Projects[0].DocumentCount != 7 {
		t.Fatalf("project listing = %#v", projects.Projects)
	}
	var listing browse.BrowseResponse
	getJSON(t, server.URL+"/api/projects/lore/browse", http.StatusOK, &listing)
	if len(listing.Tasks) != 2 || len(listing.Notes) != 1 || len(listing.Briefings) != 1 ||
		len(listing.Repositories) != 1 || len(listing.Repositories[0].Documents) != 2 || len(listing.Conversations) != 1 {
		t.Fatalf("browse listing = %#v", listing)
	}
	var taskID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM documents WHERE source_type = 'task' AND source_identity = '2'`).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	var detail browse.DocumentDetail
	getJSON(t, server.URL+"/api/projects/lore/documents/"+taskID.String(), http.StatusOK, &detail)
	if detail.RenderedContent == "" || len(detail.Relationships) != 1 || detail.Relationships[0].Direction != "dependency" || len(detail.Revisions) != 1 {
		t.Fatalf("task detail = %#v", detail)
	}
	var dependencyID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM documents WHERE source_type = 'task' AND source_identity = '1'`).Scan(&dependencyID); err != nil {
		t.Fatal(err)
	}
	var dependency browse.DocumentDetail
	getJSON(t, server.URL+"/api/projects/lore/documents/"+dependencyID.String(), http.StatusOK, &dependency)
	if len(dependency.Relationships) != 1 || dependency.Relationships[0].Direction != "dependent" {
		t.Fatalf("task dependent relationship = %#v", dependency.Relationships)
	}
	var revisionsResponse struct {
		Revisions []browse.RevisionSummary `json:"revisions"`
	}
	getJSON(t, server.URL+"/api/projects/lore/documents/"+taskID.String()+"/revisions", http.StatusOK, &revisionsResponse)
	if len(revisionsResponse.Revisions) != 1 || !revisionsResponse.Revisions[0].Current {
		t.Fatalf("revision listing = %#v", revisionsResponse.Revisions)
	}
	getJSON(t, server.URL+"/api/projects/other/documents/"+taskID.String(), http.StatusNotFound, &map[string]any{})

	notesDirectory := t.TempDir()
	note, err := os.ReadFile(filepath.Join(fixtures, "notes", "note-identity.md"))
	if err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(notesDirectory, "note-identity.md")
	if err := os.WriteFile(notePath, note, 0o600); err != nil {
		t.Fatal(err)
	}
	watchConfig := filepath.Join(t.TempDir(), "watch.yml")
	configuration := fmt.Sprintf("debounce: 25ms\nrescan-interval: 1h\nsources:\n  - project: lore\n    source-instance: watched-notes\n    adapter: notes\n    path: %s\n", notesDirectory)
	if err := os.WriteFile(watchConfig, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	watchContext, stopWatch := context.WithCancel(ctx)
	watchResult := make(chan error, 1)
	go func() {
		watchResult <- runner.Run(watchContext, []string{"watch", "--config", watchConfig, "--server", server.URL, "--token", "ingest-secret"})
	}()
	waitForCount(t, ctx, pool, `SELECT count(*) FROM revisions r JOIN documents d ON d.id = r.document_id JOIN source_instances s ON s.id = d.source_instance_id WHERE s.external_key = 'watched-notes'`, 1)
	if err := os.WriteFile(notePath, append(note, []byte("\nWatcher update.\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForCount(t, ctx, pool, `SELECT count(*) FROM revisions r JOIN documents d ON d.current_revision_id = r.id JOIN source_instances s ON s.id = d.source_instance_id WHERE s.external_key = 'watched-notes' AND r.normalized_text LIKE '%Watcher update.%'`, 1)
	stopWatch()
	select {
	case err := <-watchResult:
		if err != nil {
			t.Fatalf("watch command: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watch command did not stop after context cancellation")
	}
}

func getJSON(t *testing.T, endpoint string, expectedStatus int, target any) {
	t.Helper()
	response, err := http.Get(endpoint) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		t.Fatalf("GET %s returned %s", endpoint, response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func waitForCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, expected int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err := pool.QueryRow(ctx, query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == expected {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("database count did not reach %d", expected)
}
