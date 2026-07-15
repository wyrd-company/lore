package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wyrd-company/lore/internal/browse"
	"github.com/wyrd-company/lore/internal/database"
	"github.com/wyrd-company/lore/internal/ingestfailures"
	"github.com/wyrd-company/lore/internal/synchronization"
)

func TestProjectBootstrapAndSynchronizationHTTPBoundaryWithPostgres(t *testing.T) {
	pool := integrationPool(t)
	server := httptest.NewServer(New(pool, "ingest-secret", "admin-secret"))
	t.Cleanup(server.Close)

	assertRequest(t, http.MethodPost, server.URL+"/api/projects", "", map[string]string{"slug": "lore", "name": "Lore"}, http.StatusUnauthorized)
	assertRequest(t, http.MethodPost, server.URL+"/api/projects", "admin-secret", map[string]string{"slug": "Bad Slug", "name": "Lore"}, http.StatusUnprocessableEntity)
	assertRequest(t, http.MethodPost, server.URL+"/api/projects", "admin-secret", map[string]string{"slug": "lore", "name": "Lore"}, http.StatusCreated)
	assertRequest(t, http.MethodPost, server.URL+"/api/projects", "admin-secret", map[string]string{"slug": "lore", "name": "Ignored"}, http.StatusOK)
	assertRequest(t, http.MethodPost, server.URL+"/api/projects", "admin-secret", map[string]string{"slug": "other", "name": "Other"}, http.StatusCreated)

	manifest := synchronization.Manifest{
		Project: "lore", SourceInstance: "notes", SourceType: "note", Boundary: synchronization.BoundaryComplete,
		Documents: []synchronization.Document{{
			Identity: "welcome", Title: "Welcome", ContentHash: strings.Repeat("a", 64),
			NormalizedText: "Welcome to Lore", RenderedContent: "<p>Welcome to Lore</p>", Renderer: "markdown",
		}},
	}
	endpoint := server.URL + "/api/projects/lore/synchronizations"
	assertRequest(t, http.MethodPost, endpoint, "", manifest, http.StatusUnauthorized)
	assertRequest(t, http.MethodPost, server.URL+"/api/projects/missing/synchronizations", "ingest-secret", manifest, http.StatusNotFound)
	mismatch := manifest
	mismatch.Project = "other"
	assertRequest(t, http.MethodPost, endpoint, "ingest-secret", mismatch, http.StatusBadRequest)
	invalid := manifest
	invalid.Documents = append([]synchronization.Document(nil), manifest.Documents...)
	invalid.Documents[0].ContentHash = "invalid"
	assertRequest(t, http.MethodPost, endpoint, "ingest-secret", invalid, http.StatusUnprocessableEntity)
	assertRawRequest(t, endpoint, "ingest-secret", []byte(`{"project":`), http.StatusBadRequest)
	assertRequest(t, http.MethodPost, endpoint, "ingest-secret", manifest, http.StatusOK)

	other := manifest
	other.Documents = append([]synchronization.Document(nil), manifest.Documents...)
	other.Project = "other"
	other.Documents[0].ContentHash = strings.Repeat("b", 64)
	assertRequest(t, http.MethodPost, server.URL+"/api/projects/other/synchronizations", "ingest-secret", other, http.StatusOK)
	var loreCount, otherCount int
	if err := pool.QueryRow(context.Background(), `SELECT
		count(*) FILTER (WHERE p.slug = 'lore'), count(*) FILTER (WHERE p.slug = 'other')
		FROM documents d JOIN projects p ON p.id = d.project_id WHERE d.source_identity = 'welcome'`).Scan(&loreCount, &otherCount); err != nil {
		t.Fatal(err)
	}
	if loreCount != 1 || otherCount != 1 {
		t.Fatalf("cross-project document counts = lore:%d other:%d", loreCount, otherCount)
	}

	broken := synchronization.Manifest{
		Project: "lore", SourceInstance: "tasks", SourceType: "task", Boundary: synchronization.BoundaryPartial,
		Documents: []synchronization.Document{{
			Identity: "rollback", Title: "Rollback", ContentHash: strings.Repeat("c", 64),
			NormalizedText: "rollback", RenderedContent: "<p>rollback</p>", Renderer: "markdown",
		}},
		Relationships: []synchronization.Relationship{{SourceIdentity: "rollback", TargetIdentity: "missing", Type: "task-depends-on"}},
	}
	assertRequest(t, http.MethodPost, endpoint, "ingest-secret", broken, http.StatusInternalServerError)
	var rolledBack int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM documents WHERE source_identity = 'rollback'`).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("transaction rollback count = %d, err = %v", rolledBack, err)
	}
}

func TestIngestionFailureAndBriefingSettingHTTPBoundariesWithPostgres(t *testing.T) {
	pool := integrationPool(t)
	server := httptest.NewServer(New(pool, "ingest-secret", "admin-secret"))
	t.Cleanup(server.Close)
	doJSON(t, http.MethodPost, server.URL+"/api/projects", "admin-secret", map[string]string{"slug": "lore", "name": "Lore"}, http.StatusCreated, nil)

	note := synchronization.Document{
		Identity: "welcome", Title: "Welcome", ContentHash: strings.Repeat("a", 64), NormalizedText: "Welcome",
		RenderedContent: "<p>Welcome</p>", Renderer: "markdown", Provenance: json.RawMessage(`{"path":"/sources/welcome.md"}`),
	}
	manifest := synchronization.Manifest{
		Project: "lore", SourceInstance: "notes", SourceType: "note", Boundary: synchronization.BoundaryComplete,
		Documents: []synchronization.Document{note}, Failures: []synchronization.ParseFailure{{Path: "/sources/broken.md", Message: "invalid YAML"}},
	}
	doJSON(t, http.MethodPost, server.URL+"/api/projects/lore/synchronizations", "ingest-secret", manifest, http.StatusOK, nil)
	var failureListing struct {
		Failures []ingestfailures.Record `json:"failures"`
	}
	doJSON(t, http.MethodGet, server.URL+"/api/projects/lore/ingestion-failures?sourceType=note&sourceInstance=notes", "", nil, http.StatusOK, &failureListing)
	if len(failureListing.Failures) != 1 || failureListing.Failures[0].Path != "/sources/broken.md" {
		t.Fatalf("ingestion failures = %#v", failureListing.Failures)
	}
	var listing browse.BrowseResponse
	doJSON(t, http.MethodGet, server.URL+"/api/projects/lore/browse", "", nil, http.StatusOK, &listing)
	if listing.IngestionFailureCount != 1 {
		t.Fatalf("browse ingestion failure count = %d", listing.IngestionFailureCount)
	}
	doJSON(t, http.MethodDelete, server.URL+"/api/projects/lore/ingestion-failures/"+failureListing.Failures[0].ID.String(), "", nil, http.StatusNoContent, nil)
	doJSON(t, http.MethodGet, server.URL+"/api/projects/lore/ingestion-failures", "", nil, http.StatusOK, &failureListing)
	if len(failureListing.Failures) != 0 {
		t.Fatalf("removed ingestion failures = %#v", failureListing.Failures)
	}

	briefingManifest := synchronization.Manifest{
		Project: "lore", SourceInstance: "briefings", SourceType: "briefing", Boundary: synchronization.BoundaryComplete,
		Documents: []synchronization.Document{
			{Identity: "architecture.html", Title: "Architecture", ContentHash: strings.Repeat("b", 64), NormalizedText: "Architecture", RenderedContent: "<h1>Architecture</h1>", Renderer: "briefing"},
			{Identity: "operations.html", Title: "Operations", ContentHash: strings.Repeat("c", 64), NormalizedText: "Operations", RenderedContent: "<h1>Operations</h1>", Renderer: "briefing"},
		},
	}
	doJSON(t, http.MethodPost, server.URL+"/api/projects/lore/synchronizations", "ingest-secret", briefingManifest, http.StatusOK, nil)
	doJSON(t, http.MethodGet, server.URL+"/api/projects/lore/browse", "", nil, http.StatusOK, &listing)
	architecture, operations := listing.Briefings[0], listing.Briefings[1]
	doJSON(t, http.MethodPatch, server.URL+"/api/projects/lore/briefings/"+architecture.ID.String(), "", map[string]any{"category": "Foundations", "home": true}, http.StatusOK, nil)
	doJSON(t, http.MethodPatch, server.URL+"/api/projects/lore/briefings/"+operations.ID.String(), "", map[string]any{"category": "Operations", "home": true}, http.StatusOK, nil)
	doJSON(t, http.MethodPatch, server.URL+"/api/projects/lore/briefings/"+noteID(t, pool).String(), "", map[string]any{"home": true}, http.StatusNotFound, nil)
	doJSON(t, http.MethodGet, server.URL+"/api/projects/lore/browse", "", nil, http.StatusOK, &listing)
	if listing.Briefings[0].BriefingHome || listing.Briefings[0].BriefingCategory != "Foundations" || !listing.Briefings[1].BriefingHome || listing.Briefings[1].BriefingCategory != "Operations" {
		t.Fatalf("briefing settings = %#v", listing.Briefings)
	}
}

func noteID(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT id FROM documents WHERE source_type = 'note'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func integrationPool(t *testing.T) *pgxpool.Pool {
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

func assertRequest(t *testing.T, method, endpoint, token string, body any, want int) []byte {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return assertRawRequest(t, endpoint, token, encoded, want)
}

func assertRawRequest(t *testing.T, endpoint, token string, body []byte, want int) []byte {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	contents, _ := io.ReadAll(response.Body)
	if response.StatusCode != want {
		t.Fatalf("POST %s returned %s, want %d: %s", endpoint, response.Status, want, contents)
	}
	return contents
}
