package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wyrd-company/lore/internal/database"
	"github.com/wyrd-company/lore/internal/synchronization"
)

func TestKeywordSearchWeightsTagsAndIsolatesProjects(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; real PostgreSQL integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, cleanup := searchTestPool(t, ctx, databaseURL)
	defer cleanup()

	projectA, projectB := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, slug, name) VALUES ($1, 'project-a', 'Project A'), ($2, 'project-b', 'Project B')`, projectA, projectB); err != nil {
		t.Fatal(err)
	}
	sync := synchronization.NewRepository(pool)
	manifestA := synchronization.Manifest{
		Project: "project-a", SourceInstance: "tasks", SourceType: "task", Boundary: synchronization.BoundaryComplete,
		Documents: []synchronization.Document{
			searchDocument("tag-hit", "Tag hit", "ordinary content", []string{"zirconium"}, nil, "a"),
			searchDocument("body-hit", "Body hit", "zirconium appears in the body", nil, nil, "b"),
		},
	}
	if _, err := sync.Apply(ctx, projectA, manifestA); err != nil {
		t.Fatal(err)
	}
	manifestB := synchronization.Manifest{
		Project: "project-b", SourceInstance: "notes", SourceType: "note", Boundary: synchronization.BoundaryComplete,
		Documents: []synchronization.Document{searchDocument("private", "Other project", "zirconium private", nil, nil, "c")},
	}
	if _, err := sync.Apply(ctx, projectB, manifestB); err != nil {
		t.Fatal(err)
	}

	repository := NewRepository(pool)
	response, err := repository.Search(ctx, projectA, Request{Query: "zirconium", Limit: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 2 || response.Results[0].SourceIdentity != "tag-hit" {
		t.Fatalf("unexpected weighted results: %#v", response.Results)
	}
	for _, result := range response.Results {
		if result.Title == "Other project" {
			t.Fatal("project B content crossed the project A retrieval boundary")
		}
	}
	filtered, err := repository.Search(ctx, projectA, Request{
		Query: "zirconium", Filters: Filters{Tags: []string{"zirconium"}}, Limit: 10,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Results) != 1 || filtered.Results[0].SourceIdentity != "tag-hit" {
		t.Fatalf("tag filter results: %#v", filtered.Results)
	}
}

func searchDocument(identity, title, text string, tags []string, metadata any, hash string) synchronization.Document {
	metadataJSON, _ := json.Marshal(metadata)
	return synchronization.Document{
		Identity: identity, Title: title, ContentHash: strings.Repeat(hash, 64),
		NormalizedText: text, RenderedContent: "<p>" + text + "</p>", Renderer: "markdown",
		Metadata: metadataJSON, Provenance: json.RawMessage(`{}`), Tags: tags,
	}
}

func searchTestPool(t *testing.T, ctx context.Context, databaseURL string) (*pgxpool.Pool, func()) {
	t.Helper()
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA %s`, schema)); err != nil {
		t.Fatal(err)
	}
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
	cleanup := func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), fmt.Sprintf(`DROP SCHEMA %s CASCADE`, schema))
		admin.Close()
	}
	return pool, cleanup
}
