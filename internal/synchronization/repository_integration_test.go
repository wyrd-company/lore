package synchronization

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wyrd-company/lore/internal/browse"
	"github.com/wyrd-company/lore/internal/database"
)

func TestRepositoryApplyWithPostgres(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; real PostgreSQL integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect admin pool: %v", err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA %s`, schema)); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	defer admin.Exec(context.Background(), fmt.Sprintf(`DROP SCHEMA %s CASCADE`, schema)) //nolint:errcheck

	testURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	query := testURL.Query()
	query.Set("search_path", schema+",public")
	testURL.RawQuery = query.Encode()
	if err := database.Migrate(ctx, testURL.String()); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	pool, err := pgxpool.New(ctx, testURL.String())
	if err != nil {
		t.Fatalf("connect test pool: %v", err)
	}
	defer pool.Close()
	projectID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, slug, name) VALUES ($1, 'lore', 'Lore')`, projectID); err != nil {
		t.Fatalf("create project: %v", err)
	}
	repository := NewRepository(pool)

	manifest := Manifest{
		Project: "lore", SourceInstance: "tasks", SourceType: "task", Boundary: BoundaryComplete,
		Documents: []Document{
			documentFixture("one", "a"),
			documentFixture("two", "b"),
		},
		Relationships: []Relationship{{SourceIdentity: "two", TargetIdentity: "one", Type: "task-depends-on"}},
	}
	manifest.Documents[0].Tags = []string{"architecture", "lore"}
	manifest.Documents[0].Terms = []string{"knowledge-portal", "missing-term"}
	manifest.Documents[1].DefinesTerm = "knowledge-portal"
	result, err := repository.Apply(ctx, projectID, manifest)
	if err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	if result.Created != 2 || result.Updated != 0 || result.Deleted != 0 {
		t.Fatalf("unexpected initial result: %+v", result)
	}
	var tagLinks, relationships int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM document_tags`).Scan(&tagLinks); err != nil || tagLinks != 2 {
		t.Fatalf("tag links = %d, err = %v", tagLinks, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM relationships`).Scan(&relationships); err != nil || relationships != 1 {
		t.Fatalf("relationships = %d, err = %v", relationships, err)
	}
	var terms, termLinks, termDefinitions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM terms`).Scan(&terms); err != nil || terms != 2 {
		t.Fatalf("terms = %d, err = %v", terms, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM document_terms`).Scan(&termLinks); err != nil || termLinks != 2 {
		t.Fatalf("term links = %d, err = %v", termLinks, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM term_definitions`).Scan(&termDefinitions); err != nil || termDefinitions != 1 {
		t.Fatalf("term definitions = %d, err = %v", termDefinitions, err)
	}
	browser := browse.NewRepository(pool)
	listing, err := browser.Browse(ctx, projectID)
	if err != nil {
		t.Fatalf("browse taxonomy: %v", err)
	}
	if len(listing.Terms) != 2 || listing.Terms[0].Name != "knowledge-portal" || !listing.Terms[0].Defined || listing.Terms[1].Name != "missing-term" || listing.Terms[1].Defined {
		t.Fatalf("term collection = %#v", listing.Terms)
	}
	var firstID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM documents WHERE source_identity = 'one'`).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	detail, err := browser.Document(ctx, projectID, firstID)
	if err != nil || len(detail.Terms) != 2 || !detail.Terms[0].Defined || detail.Terms[1].Defined {
		t.Fatalf("document terms = %#v, err = %v", detail.Terms, err)
	}
	var chunks, embeddingJobs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chunks`).Scan(&chunks); err != nil || chunks != 2 {
		t.Fatalf("chunks = %d, err = %v", chunks, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM embedding_jobs`).Scan(&embeddingJobs); err != nil || embeddingJobs != 2 {
		t.Fatalf("embedding jobs = %d, err = %v", embeddingJobs, err)
	}

	result, err = repository.Apply(ctx, projectID, manifest)
	if err != nil {
		t.Fatalf("idempotent apply: %v", err)
	}
	if result.Unchanged != 2 {
		t.Fatalf("expected two unchanged documents, got %+v", result)
	}
	var revisions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM revisions`).Scan(&revisions); err != nil || revisions != 2 {
		t.Fatalf("idempotency revision count = %d, err = %v", revisions, err)
	}

	partial := manifest
	partial.Boundary = BoundaryPartial
	partial.Documents = []Document{documentFixture("one", "c")}
	partial.Relationships = nil
	result, err = repository.Apply(ctx, projectID, partial)
	if err != nil {
		t.Fatalf("partial apply: %v", err)
	}
	if result.Updated != 1 || result.Deleted != 0 {
		t.Fatalf("unexpected partial result: %+v", result)
	}
	assertCurrentDocuments(t, ctx, pool, 2)
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM relationships`).Scan(&relationships); err != nil || relationships != 1 {
		t.Fatalf("partial sync changed sibling relationships: count=%d err=%v", relationships, err)
	}

	complete := partial
	complete.Boundary = BoundaryComplete
	result, err = repository.Apply(ctx, projectID, complete)
	if err != nil {
		t.Fatalf("complete apply: %v", err)
	}
	if result.Deleted != 1 {
		t.Fatalf("expected one deletion, got %+v", result)
	}
	assertCurrentDocuments(t, ctx, pool, 1)

	otherSource := Manifest{
		Project: "lore", SourceInstance: "other-notes", SourceType: "note", Boundary: BoundaryComplete,
		Documents: []Document{documentFixture("other", "d")},
	}
	if _, err := repository.Apply(ctx, projectID, otherSource); err != nil {
		t.Fatalf("apply other source: %v", err)
	}
	emptyOriginal := complete
	emptyOriginal.Documents = nil
	if _, err := repository.Apply(ctx, projectID, emptyOriginal); err != nil {
		t.Fatalf("empty complete apply: %v", err)
	}
	assertCurrentDocuments(t, ctx, pool, 1)

	notes := Manifest{
		Project: "lore", SourceInstance: "related-notes", SourceType: "note", Boundary: BoundaryComplete,
		Documents: []Document{documentFixture("note-one", "1"), documentFixture("note-two", "2")},
		Relationships: []Relationship{
			{SourceIdentity: "note-one", TargetIdentity: "note-two", Type: "note-related-to"},
			{SourceIdentity: "note-one", TargetIdentity: "external-note", Type: "note-related-to"},
		},
	}
	if _, err := repository.Apply(ctx, projectID, notes); err != nil {
		t.Fatalf("apply related notes: %v", err)
	}
	var noteID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM documents WHERE source_identity = 'note-one'`).Scan(&noteID); err != nil {
		t.Fatal(err)
	}
	noteDetail, err := browser.Document(ctx, projectID, noteID)
	if err != nil || len(noteDetail.Relationships) != 1 || noteDetail.Relationships[0].Direction != "related" || noteDetail.Relationships[0].SourceIdentity != "note-two" {
		t.Fatalf("note relationships = %#v, err = %v", noteDetail.Relationships, err)
	}

	broken := Manifest{
		Project: "lore", SourceInstance: "broken", SourceType: "task", Boundary: BoundaryPartial,
		Documents:     []Document{{Identity: "rollback", Title: "Rollback", ContentHash: strings.Repeat("e", 64), Renderer: "markdown", NormalizedText: "rollback"}},
		Relationships: []Relationship{{SourceIdentity: "rollback", TargetIdentity: "missing", Type: "task-depends-on"}},
	}
	if _, err := repository.Apply(ctx, projectID, broken); err == nil {
		t.Fatal("expected duplicate chunk transaction to fail")
	}
	var rolledBack int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM documents WHERE source_identity = 'rollback'`).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("transaction did not roll back: documents=%d err=%v", rolledBack, err)
	}
}

func documentFixture(identity, hashCharacter string) Document {
	return Document{
		Identity: identity, Title: strings.ToUpper(identity), ContentHash: strings.Repeat(hashCharacter, 64),
		NormalizedText: identity, RenderedContent: "<p>" + identity + "</p>", Renderer: "markdown",
	}
}

func assertCurrentDocuments(t *testing.T, ctx context.Context, pool *pgxpool.Pool, expected int) {
	t.Helper()
	var actual int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM documents WHERE deleted_at IS NULL`).Scan(&actual); err != nil {
		t.Fatalf("count current documents: %v", err)
	}
	if actual != expected {
		t.Fatalf("current documents = %d, want %d", actual, expected)
	}
}
