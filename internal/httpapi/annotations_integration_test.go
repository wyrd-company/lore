package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	annotationmodel "github.com/wyrd-company/lore/internal/annotations"
	"github.com/wyrd-company/lore/internal/browse"
	"github.com/wyrd-company/lore/internal/synchronization"
)

func TestAnnotationLifecycleRetentionCleanupAndExportWithPostgres(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	projectID, otherProjectID := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, slug, name) VALUES ($1, 'lore', 'Lore'), ($2, 'other', 'Other')`, projectID, otherProjectID); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(pool, "ingest-secret", "admin-secret"))
	t.Cleanup(server.Close)

	manifest := annotationManifest("a")
	syncHTTP(t, server.URL, manifest)
	documentID, revisionOne := documentRevision(t, pool, projectID)
	doJSON(t, http.MethodPost, server.URL+"/api/projects/lore/annotations", "", annotationRequest(documentID, revisionOne, strings.Repeat("b", 64), "alice"), http.StatusConflict, nil)
	annotation := createAnnotationHTTP(t, server.URL, documentID, revisionOne, strings.Repeat("a", 64), "alice")
	if annotation.Status != "open" || annotation.AttributedUsername != "alice" || annotation.RevisionIdentity != revisionOne {
		t.Fatalf("created annotation = %#v", annotation)
	}

	var listing struct {
		Annotations []annotationmodel.Record `json:"annotations"`
	}
	doJSON(t, http.MethodGet, server.URL+"/api/projects/lore/annotations", "", nil, http.StatusOK, &listing)
	if len(listing.Annotations) != 1 {
		t.Fatalf("viewer annotation count = %d", len(listing.Annotations))
	}
	doJSON(t, http.MethodGet, server.URL+"/api/projects/other/annotations", "", nil, http.StatusOK, &listing)
	if len(listing.Annotations) != 0 {
		t.Fatalf("other project annotations = %#v", listing.Annotations)
	}
	doJSON(t, http.MethodGet, server.URL+"/api/projects/other/annotations/"+annotation.ID.String(), "", nil, http.StatusNotFound, nil)

	updatedBody := "Clarify this section"
	doJSON(t, http.MethodPatch, server.URL+"/api/projects/lore/annotations/"+annotation.ID.String(), "", map[string]any{
		"body": updatedBody, "attributedUsername": "bob",
	}, http.StatusOK, &annotation)
	if annotation.Body != updatedBody || annotation.UpdatedBy != "bob" || annotation.AttributedUsername != "alice" {
		t.Fatalf("updated annotation attribution = %#v", annotation)
	}

	manifest.Documents[0] = annotationDocument("b")
	syncHTTP(t, server.URL, manifest)
	_, revisionTwo := documentRevision(t, pool, projectID)
	assertRevisionCount(t, pool, documentID, 2)
	var retained browse.RevisionDetail
	doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/projects/lore/documents/%s/revisions/%s", server.URL, documentID, revisionOne), "", nil, http.StatusOK, &retained)
	if retained.RenderedContent == "" || retained.AnnotationCount != 1 {
		t.Fatalf("retained revision = %#v", retained)
	}

	target := map[string]any{
		"targetRevisionId": revisionTwo, "attributedUsername": "carol",
		"selector":      map[string]any{"kind": "heading-path", "path": []string{"Updated"}},
		"selectedQuote": "version b", "quotePrefix": "before", "quoteSuffix": "after",
		"structuralLocation":  map[string]any{"headingPath": []string{"Updated"}},
		"originalContentHash": strings.Repeat("b", 64),
	}
	var copied annotationmodel.Record
	doJSON(t, http.MethodPost, server.URL+"/api/projects/lore/annotations/"+annotation.ID.String()+"/copy", "", target, http.StatusCreated, &copied)
	if copied.CopiedFromAnnotation == nil || *copied.CopiedFromAnnotation != annotation.ID || copied.Status != "open" {
		t.Fatalf("copied annotation = %#v", copied)
	}
	doJSON(t, http.MethodPost, server.URL+"/api/projects/lore/annotations/"+annotation.ID.String()+"/move", "", target, http.StatusOK, &annotation)
	if annotation.RevisionIdentity != revisionTwo || len(annotation.PriorTarget) == 0 {
		t.Fatalf("moved annotation = %#v", annotation)
	}
	doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/projects/lore/documents/%s/revisions/%s", server.URL, documentID, revisionOne), "", nil, http.StatusNotFound, nil)

	var events struct {
		Events []annotationmodel.Event `json:"events"`
	}
	doJSON(t, http.MethodGet, server.URL+"/api/projects/lore/annotations/"+annotation.ID.String()+"/events", "", nil, http.StatusOK, &events)
	if len(events.Events) != 3 || events.Events[0].AttributedUsername != "alice" || events.Events[2].Operation != "move" {
		t.Fatalf("annotation events = %#v", events.Events)
	}

	manifest.Documents[0] = annotationDocument("c")
	syncHTTP(t, server.URL, manifest)
	_, revisionThree := documentRevision(t, pool, projectID)
	assertRevisionCount(t, pool, documentID, 2)
	resolveAnnotation(t, server.URL, annotation.ID, "resolved", "dana")
	resolveAnnotation(t, server.URL, copied.ID, "dismissed", "erin")
	doJSON(t, http.MethodPost, server.URL+"/api/projects/lore/admin/cleanup", "", map[string]any{
		"revisionId": revisionTwo, "attributedUsername": "admin",
	}, http.StatusUnauthorized, nil)
	var cleanup annotationmodel.CleanupResult
	doJSON(t, http.MethodPost, server.URL+"/api/projects/lore/admin/cleanup", "admin-secret", map[string]any{
		"revisionId": revisionTwo, "attributedUsername": "admin",
	}, http.StatusOK, &cleanup)
	if cleanup.RevisionsRemoved != 1 || cleanup.AnnotationsTombstoned != 2 {
		t.Fatalf("cleanup = %#v", cleanup)
	}
	assertRevisionCount(t, pool, documentID, 1)
	doJSON(t, http.MethodGet, server.URL+"/api/projects/lore/annotations?revisionId="+revisionTwo.String(), "", nil, http.StatusOK, &listing)
	if len(listing.Annotations) != 2 || listing.Annotations[0].RevisionID != nil || listing.Annotations[0].TombstonedAt == nil || listing.Annotations[0].ResolvedBy == nil {
		t.Fatalf("annotation tombstones = %#v", listing.Annotations)
	}

	var snapshot annotationmodel.Export
	doJSON(t, http.MethodGet, server.URL+"/api/projects/lore/annotations/export", "", nil, http.StatusOK, &snapshot)
	if snapshot.FormatVersion != "lore.annotations/v1" || snapshot.Mode != "snapshot" || len(snapshot.Annotations) != 2 || snapshot.NextCursor == 0 {
		t.Fatalf("snapshot export = %#v", snapshot)
	}

	manifest.Documents[0] = annotationDocument("d")
	syncHTTP(t, server.URL, manifest)
	_, revisionFour := documentRevision(t, pool, projectID)
	if revisionThree == revisionFour {
		t.Fatal("expected a new current revision")
	}
	assertRevisionCount(t, pool, documentID, 1)
	deletedAnnotation := createAnnotationHTTP(t, server.URL, documentID, revisionFour, strings.Repeat("d", 64), "frank")
	manifest.Documents = nil
	syncHTTP(t, server.URL, manifest)
	assertRevisionCount(t, pool, documentID, 1)
	resolveAnnotation(t, server.URL, deletedAnnotation.ID, "resolved", "frank")
	doJSON(t, http.MethodPost, server.URL+"/api/projects/lore/admin/cleanup", "admin-secret", map[string]any{
		"revisionId": revisionFour, "attributedUsername": "admin",
	}, http.StatusOK, &cleanup)
	assertRevisionCount(t, pool, documentID, 0)

	var incremental annotationmodel.Export
	doJSON(t, http.MethodGet, server.URL+"/api/projects/lore/annotations/export?after="+fmt.Sprint(snapshot.NextCursor), "", nil, http.StatusOK, &incremental)
	if incremental.Mode != "incremental" || incremental.AfterCursor != snapshot.NextCursor || len(incremental.Annotations) != 1 || incremental.Annotations[0].ID != deletedAnnotation.ID {
		t.Fatalf("incremental export = %#v", incremental)
	}
}

func annotationManifest(hashCharacter string) synchronization.Manifest {
	return synchronization.Manifest{
		Project: "lore", SourceInstance: "notes", SourceType: "note", Boundary: synchronization.BoundaryComplete,
		Documents: []synchronization.Document{annotationDocument(hashCharacter)},
	}
}

func annotationDocument(hashCharacter string) synchronization.Document {
	return synchronization.Document{
		Identity: "annotated-note", Title: "Annotated note", ContentHash: strings.Repeat(hashCharacter, 64),
		NormalizedText: "version " + hashCharacter, RenderedContent: "<h2 id=\"updated\">version " + hashCharacter + "</h2>",
		Renderer: "markdown", Provenance: json.RawMessage(`{"path":"notes/annotated.md"}`),
	}
}

func syncHTTP(t *testing.T, baseURL string, manifest synchronization.Manifest) {
	t.Helper()
	doJSON(t, http.MethodPost, baseURL+"/api/projects/lore/synchronizations", "ingest-secret", manifest, http.StatusOK, nil)
}

func documentRevision(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID) (uuid.UUID, uuid.UUID) {
	t.Helper()
	var documentID, revisionID uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT id, current_revision_id FROM documents WHERE project_id = $1 AND source_identity = 'annotated-note'`, projectID).
		Scan(&documentID, &revisionID); err != nil {
		t.Fatal(err)
	}
	return documentID, revisionID
}

func createAnnotationHTTP(t *testing.T, baseURL string, documentID, revisionID uuid.UUID, hash, username string) annotationmodel.Record {
	t.Helper()
	var record annotationmodel.Record
	doJSON(t, http.MethodPost, baseURL+"/api/projects/lore/annotations", "", annotationRequest(documentID, revisionID, hash, username), http.StatusCreated, &record)
	return record
}

func annotationRequest(documentID, revisionID uuid.UUID, hash, username string) map[string]any {
	return map[string]any{
		"documentId": documentID, "revisionId": revisionID, "body": "Review this text",
		"attributedUsername": username, "originatingOperation": "selection",
		"selector":      map[string]any{"kind": "text-quote", "headingPath": []string{"Introduction"}},
		"selectedQuote": "version", "quotePrefix": "before", "quoteSuffix": "after",
		"structuralLocation": map[string]any{"headingPath": []string{"Introduction"}}, "originalContentHash": hash,
	}
}

func resolveAnnotation(t *testing.T, baseURL string, annotationID uuid.UUID, status, username string) {
	t.Helper()
	var record annotationmodel.Record
	doJSON(t, http.MethodPatch, baseURL+"/api/projects/lore/annotations/"+annotationID.String(), "", map[string]any{
		"status": status, "attributedUsername": username,
	}, http.StatusOK, &record)
	if record.Status != status || record.ResolvedAt == nil || record.ResolvedBy == nil || *record.ResolvedBy != username {
		t.Fatalf("resolved annotation = %#v", record)
	}
}

func assertRevisionCount(t *testing.T, pool *pgxpool.Pool, documentID uuid.UUID, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM revisions WHERE document_id = $1`, documentID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("revision count = %d, want %d", count, want)
	}
}

func doJSON(t *testing.T, method, endpoint, token string, body any, want int, result any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
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
		t.Fatalf("%s %s returned %s, want %d: %s", method, endpoint, response.Status, want, contents)
	}
	if result != nil && len(contents) > 0 {
		if err := json.Unmarshal(contents, result); err != nil {
			t.Fatalf("decode %s: %v: %s", endpoint, err, contents)
		}
	}
}
