package httpapi

import (
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

	"github.com/wyrd-company/lore/internal/database"
	"github.com/wyrd-company/lore/internal/embedding"
	"github.com/wyrd-company/lore/internal/retrieval"
	"github.com/wyrd-company/lore/internal/synchronization"
)

func TestHybridSearchWithRealGatewayAndPostgres(t *testing.T) {
	apiKey := gatewayTestKey(t)
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; real PostgreSQL integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, cleanup := gatewayTestPool(t, ctx, databaseURL)
	defer cleanup()

	projectA, projectB := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, slug, name) VALUES ($1, 'project-a', 'Project A'), ($2, 'project-b', 'Project B')`, projectA, projectB); err != nil {
		t.Fatal(err)
	}
	sync := synchronization.NewRepository(pool)
	metadata := json.RawMessage(`{"repository":"wyrd-company/lore","branch":"main","path":"docs/search.md"}`)
	for _, fixture := range []struct {
		projectID uuid.UUID
		project   string
		identity  string
		hash      string
		title     string
	}{
		{projectA, "project-a", "search-a", "a", "Semantic search design"},
		{projectB, "project-b", "search-b", "b", "Project B private search design"},
	} {
		manifest := synchronization.Manifest{
			Project: fixture.project, SourceInstance: "repository", SourceType: "repository", Boundary: synchronization.BoundaryComplete,
			Documents: []synchronization.Document{{
				Identity: fixture.identity, Title: fixture.title, ContentHash: strings.Repeat(fixture.hash, 64),
				NormalizedText:  "Hybrid semantic retrieval combines keyword and vector candidates for project knowledge.",
				RenderedContent: "<p>Hybrid semantic retrieval combines keyword and vector candidates.</p>",
				Renderer:        "markdown", Metadata: metadata, Provenance: json.RawMessage(`{"path":"docs/search.md"}`), Tags: []string{"search"},
			}},
		}
		if _, err := sync.Apply(ctx, fixture.projectID, manifest); err != nil {
			t.Fatal(err)
		}
	}

	client, err := embedding.NewClient(apiKey)
	if err != nil {
		t.Fatal(err)
	}
	processed, err := embedding.NewWorker(pool, client).ProcessOnce(ctx)
	if err != nil {
		t.Fatalf("real Gateway embedding backfill: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed embeddings = %d, want 2", processed)
	}
	var embeddings, dimensions, queued int
	if err := pool.QueryRow(ctx, `SELECT count(*), min(dimensions), (SELECT count(*) FROM embedding_jobs) FROM embeddings`).Scan(&embeddings, &dimensions, &queued); err != nil {
		t.Fatal(err)
	}
	if embeddings != 2 || dimensions != 1024 || queued != 0 {
		t.Fatalf("embedding persistence: count=%d dimensions=%d queued=%d", embeddings, dimensions, queued)
	}

	server := httptest.NewServer(New(pool, "ingest", "admin", client))
	defer server.Close()
	query := url.Values{
		"q": {"finding conceptually related project knowledge"}, "sourceType": {"repository"},
		"repository": {"wyrd-company/lore"}, "branch": {"main"}, "tag": {"search"},
		"createdFrom": {time.Now().Add(-time.Hour).Format(time.RFC3339)},
		"createdTo":   {time.Now().Add(time.Hour).Format(time.RFC3339)},
	}
	response, err := http.Get(server.URL + "/api/projects/project-a/search?" + query.Encode()) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("search returned %s", response.Status)
	}
	var search retrieval.Response
	if err := json.NewDecoder(response.Body).Decode(&search); err != nil {
		t.Fatal(err)
	}
	if !search.Modes.Keyword || !search.Modes.Vector || len(search.Results) != 1 || search.Results[0].SourceIdentity != "search-a" {
		t.Fatalf("hybrid project-scoped response: %#v", search)
	}
	if len(search.Results[0].MatchedChunks) == 0 || search.Results[0].MatchedChunks[0].VectorRank == nil {
		t.Fatalf("vector rank evidence missing: %#v", search.Results[0])
	}
	if strings.Contains(string(search.Results[0].Metadata), "project-b") || search.Results[0].Title == "Project B private search design" {
		t.Fatal("project B content crossed the project A search boundary")
	}

	outageManifest := synchronization.Manifest{
		Project: "project-a", SourceInstance: "notes", SourceType: "note", Boundary: synchronization.BoundaryPartial,
		Documents: []synchronization.Document{{
			Identity: "queued-during-outage", Title: "Queued during outage", ContentHash: strings.Repeat("c", 64),
			NormalizedText: "Keyword indexing remains available.", RenderedContent: "<p>Keyword indexing remains available.</p>", Renderer: "markdown",
		}},
	}
	if _, err := sync.Apply(ctx, projectA, outageManifest); err != nil {
		t.Fatalf("synchronization failed before embedding: %v", err)
	}
	invalidClient, err := embedding.NewClient("invalid-gateway-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := embedding.NewWorker(pool, invalidClient).ProcessOnce(ctx); err == nil {
		t.Fatal("expected the real Gateway to reject the invalid credential")
	}
	var attempts int
	var lastError string
	if err := pool.QueryRow(ctx, `SELECT attempts, last_error FROM embedding_jobs`).Scan(&attempts, &lastError); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || lastError == "" {
		t.Fatalf("failed embedding was not retained for backfill: attempts=%d error=%q", attempts, lastError)
	}
	keywordOnly, err := retrieval.NewRepository(pool).Search(ctx, projectA, retrieval.Request{Query: "keyword indexing", Limit: 10}, nil)
	if err != nil || len(keywordOnly.Results) == 0 || keywordOnly.Results[0].SourceIdentity != "queued-during-outage" {
		t.Fatalf("keyword retrieval during embedding outage: response=%#v err=%v", keywordOnly, err)
	}
}

func gatewayTestKey(t *testing.T) string {
	t.Helper()
	if key := os.Getenv("AI_GATEWAY_API_KEY"); key != "" {
		return key
	}
	contents, err := os.ReadFile(filepath.Join("..", "..", ".env"))
	if err != nil {
		t.Skip("AI_GATEWAY_API_KEY is not available for the real Gateway integration test")
	}
	for _, line := range strings.Split(string(contents), "\n") {
		if key, found := strings.CutPrefix(line, "AI_GATEWAY_API_KEY="); found && strings.TrimSpace(key) != "" {
			return strings.Trim(strings.TrimSpace(key), `"'`)
		}
	}
	t.Skip("AI_GATEWAY_API_KEY is not available for the real Gateway integration test")
	return ""
}

func gatewayTestPool(t *testing.T, ctx context.Context, databaseURL string) (*pgxpool.Pool, func()) {
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
	values := testURL.Query()
	values.Set("search_path", schema+",public")
	testURL.RawQuery = values.Encode()
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
