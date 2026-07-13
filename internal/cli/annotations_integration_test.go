package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wyrd-company/lore/internal/annotations"
	"github.com/wyrd-company/lore/internal/database"
	"github.com/wyrd-company/lore/internal/httpapi"
	"github.com/wyrd-company/lore/internal/synchronization"
)

func TestAnnotationSnapshotAndIncrementalExportThroughCLIWithPostgres(t *testing.T) {
	pool := annotationExportPool(t)
	ctx := context.Background()
	projectID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, slug, name) VALUES ($1, 'lore', 'Lore')`, projectID); err != nil {
		t.Fatal(err)
	}
	manifest := synchronization.Manifest{
		Project: "lore", SourceInstance: "notes", SourceType: "note", Boundary: synchronization.BoundaryPartial,
		Documents: []synchronization.Document{{
			Identity: "export-note", Title: "Export note", ContentHash: strings.Repeat("a", 64),
			NormalizedText: "Export me", RenderedContent: "<p>Export me</p>", Renderer: "markdown",
			Provenance: json.RawMessage(`{"path":"notes/export.md"}`),
		}},
	}
	if _, err := synchronization.NewRepository(pool).Apply(ctx, projectID, manifest); err != nil {
		t.Fatal(err)
	}
	var documentID, revisionID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id, current_revision_id FROM documents WHERE project_id = $1`, projectID).Scan(&documentID, &revisionID); err != nil {
		t.Fatal(err)
	}
	record, err := annotations.NewRepository(pool).Create(ctx, projectID, annotations.CreateRequest{
		DocumentID: documentID, RevisionID: revisionID, Body: "Export this annotation", AttributedUsername: "alice",
		OriginatingOperation: "selection", Selector: json.RawMessage(`{"kind":"heading-path","path":["Export"]}`),
		OriginalContentHash: strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(pool, "ingest-secret", "admin-secret"))
	t.Cleanup(server.Close)

	var output bytes.Buffer
	runner := New(&output, &output)
	if err := runner.Run(ctx, []string{"annotations", "export", "--project", "lore", "--server", server.URL}); err != nil {
		t.Fatal(err)
	}
	var snapshot annotations.Export
	if err := json.Unmarshal(output.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v: %s", err, output.String())
	}
	if snapshot.Mode != "snapshot" || snapshot.Project != "lore" || len(snapshot.Annotations) != 1 || snapshot.Annotations[0].ID != record.ID {
		t.Fatalf("snapshot = %#v", snapshot)
	}

	status := "resolved"
	if _, err := annotations.NewRepository(pool).Update(ctx, projectID, record.ID, annotations.UpdateRequest{Status: &status, AttributedUsername: "bob"}); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := runner.Run(ctx, []string{"annotations", "export", "--project", "lore", "--server", server.URL, "--after", fmt.Sprint(snapshot.NextCursor)}); err != nil {
		t.Fatal(err)
	}
	var incremental annotations.Export
	if err := json.Unmarshal(output.Bytes(), &incremental); err != nil {
		t.Fatalf("decode incremental: %v: %s", err, output.String())
	}
	if incremental.Mode != "incremental" || incremental.AfterCursor != snapshot.NextCursor || len(incremental.Annotations) != 1 || incremental.Annotations[0].Status != "resolved" {
		t.Fatalf("incremental = %#v", incremental)
	}
}

func annotationExportPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; real PostgreSQL integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA %s`, schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), fmt.Sprintf(`DROP SCHEMA %s CASCADE`, schema)) })
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema+",public")
	parsed.RawQuery = query.Encode()
	if err := database.Migrate(ctx, parsed.String()); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
