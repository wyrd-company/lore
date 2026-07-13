package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

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
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, slug, name) VALUES ($1, 'lore', 'Lore')`, uuid.New()); err != nil {
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
}
