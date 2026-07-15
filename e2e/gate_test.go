//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wyrd-company/lore/internal/annotations"
	"github.com/wyrd-company/lore/internal/browse"
	"github.com/wyrd-company/lore/internal/retrieval"
)

const (
	primaryProject   = "e2e-primary"
	isolatedProject  = "e2e-isolated"
	ingestToken      = "e2e-ingest"
	adminToken       = "e2e-admin"
	annotationAuthor = "E2E gate"
)

type gateState struct {
	DocumentID      uuid.UUID `json:"documentId"`
	OldRevisionID   uuid.UUID `json:"oldRevisionId"`
	CurrentRevision uuid.UUID `json:"currentRevisionId"`
	OldContentHash  string    `json:"oldContentHash"`
	SnapshotCursor  int64     `json:"snapshotCursor"`
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

func TestPrepareGate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pool := openPool(t, ctx)
	defer pool.Close()
	work := requiredEnv(t, "LORE_E2E_WORKDIR")
	fixtures := filepath.Join(work, "fixtures")
	architectureFixture := filepath.Join(fixtures, "briefing", "architecture.html")
	operationsFixture := filepath.Join(fixtures, "briefing", "operations.html")
	operationsBody := bytes.ReplaceAll(mustRead(t, architectureFixture), []byte("Architecture"), []byte("Operations"))
	operationsBody = bytes.ReplaceAll(operationsBody, []byte("architecture"), []byte("operations"))
	mustWrite(t, operationsFixture, operationsBody)

	runLore(t, ctx, "projects", "create", "--slug", primaryProject, "--name", "Lore E2E archive", "--server", baseURL(), "--token", adminToken)
	runLore(t, ctx, "projects", "create", "--slug", isolatedProject, "--name", "Isolated archive", "--server", baseURL(), "--token", adminToken)

	mapping := filepath.Join(work, "project-map.yml")
	mustWrite(t, mapping, []byte("sessions:\n  claude-session: e2e-primary\n  codex-session: e2e-primary\n"))
	repo := filepath.Join(fixtures, "repository")
	git(t, repo, "init", "-b", "e2e/real-services")
	git(t, repo, "config", "user.name", "Lore E2E")
	git(t, repo, "config", "user.email", "e2e@lore.invalid")
	git(t, repo, "remote", "add", "origin", "git@github.com:wyrd-company/lore-e2e-fixture.git")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "fixture")

	uploads := [][]string{
		{"upload", "tasks", "--project", primaryProject, "--source-instance", "kanban", "--complete", "--server", baseURL(), "--token", ingestToken, filepath.Join(fixtures, "kanban")},
		{"upload", "notes", "--project", primaryProject, "--source-instance", "mnemonic", "--complete", "--server", baseURL(), "--token", ingestToken, filepath.Join(fixtures, "notes")},
		{"upload", "briefing", "--project", primaryProject, "--source-instance", "architecture", "--complete", "--server", baseURL(), "--token", ingestToken, architectureFixture},
		{"upload", "briefing", "--project", primaryProject, "--source-instance", "operations", "--complete", "--server", baseURL(), "--token", ingestToken, operationsFixture},
		{"upload", "repository", "--project", primaryProject, "--source-instance", "git-fixture", "--complete", "--server", baseURL(), "--token", ingestToken, repo},
		{"upload", "conversations", "--source-instance", "claude", "--provider", "claude", "--mapping", mapping, "--complete", "--server", baseURL(), "--token", ingestToken, filepath.Join(fixtures, "conversations", "claude")},
		{"upload", "conversations", "--source-instance", "codex", "--provider", "codex", "--mapping", mapping, "--complete", "--server", baseURL(), "--token", ingestToken, filepath.Join(fixtures, "conversations", "codex")},
	}
	for _, args := range uploads {
		runLore(t, ctx, args...)
	}
	before := scalar[int](t, ctx, pool, `SELECT count(*) FROM revisions`)
	for _, args := range uploads {
		output := runLore(t, ctx, args...)
		if !strings.Contains(output, "unchanged") {
			t.Fatalf("unchanged upload did not report idempotency: %s", output)
		}
	}
	if after := scalar[int](t, ctx, pool, `SELECT count(*) FROM revisions`); after != before {
		t.Fatalf("unchanged re-upload created revisions: before=%d after=%d", before, after)
	}

	var listing browse.BrowseResponse
	getJSON(t, baseURL()+"/api/projects/"+primaryProject+"/browse", http.StatusOK, &listing)
	assertSourceCounts(t, listing)
	if got := strings.Join(listing.TaskStatuses, ","); got != "done,Ready for deploy,backlog,archived,in progress" {
		t.Fatalf("task status vocabulary = %q", got)
	}
	if len(listing.Repositories) != 1 || listing.Repositories[0].Repository != "git@github.com:wyrd-company/lore-e2e-fixture.git" || listing.Repositories[0].Branch != "e2e/real-services" {
		t.Fatalf("repository git derivation = %#v", listing.Repositories)
	}
	foundation := findDocument(t, listing.Tasks, "Build foundation")
	var foundationDetail browse.DocumentDetail
	getJSON(t, fmt.Sprintf("%s/api/projects/%s/documents/%s", baseURL(), primaryProject, foundation.ID), http.StatusOK, &foundationDetail)
	createAnnotation(t, foundationDetail, "Board-visible task annotation")
	getJSON(t, baseURL()+"/api/projects/"+primaryProject+"/browse", http.StatusOK, &listing)
	foundation = findDocument(t, listing.Tasks, "Build foundation")
	adapters := findDocument(t, listing.Tasks, "Build adapters")
	if foundation.DependencyCount != 0 || foundation.DependentCount != 1 || foundation.OpenAnnotationCount != 1 {
		t.Fatalf("foundation board counts = dependencies %d dependents %d annotations %d", foundation.DependencyCount, foundation.DependentCount, foundation.OpenAnnotationCount)
	}
	if adapters.DependencyCount != 1 || adapters.DependentCount != 0 || adapters.OpenAnnotationCount != 0 {
		t.Fatalf("adapters board counts = dependencies %d dependents %d annotations %d", adapters.DependencyCount, adapters.DependentCount, adapters.OpenAnnotationCount)
	}

	isolatedNotes := filepath.Join(work, "isolated-notes")
	mustMkdir(t, isolatedNotes)
	mustWrite(t, filepath.Join(isolatedNotes, "private.md"), []byte("---\ntitle: Quasar isolation ledger\ntags: [private]\n---\n# Quasar isolation ledger\n\nvelvet-quasar-719 belongs only to the isolated project.\n"))
	runLore(t, ctx, "upload", "notes", "--project", isolatedProject, "--source-instance", "isolated", "--complete", "--server", baseURL(), "--token", ingestToken, isolatedNotes)
	var isolatedListing browse.BrowseResponse
	getJSON(t, baseURL()+"/api/projects/"+isolatedProject+"/browse", http.StatusOK, &isolatedListing)
	if len(isolatedListing.Tasks) != 0 || len(isolatedListing.TaskStatuses) != 0 {
		t.Fatalf("task board data leaked across projects: %#v", isolatedListing)
	}

	manifestNotes := filepath.Join(work, "manifest-notes")
	mustMkdir(t, manifestNotes)
	mustWrite(t, filepath.Join(manifestNotes, "one.md"), []byte("---\ntitle: Manifest one\n---\nOne remains.\n"))
	mustWrite(t, filepath.Join(manifestNotes, "two.md"), []byte("---\ntitle: Manifest two\n---\nTwo is removed locally.\n"))
	runLore(t, ctx, "upload", "notes", "--project", primaryProject, "--source-instance", "manifest-semantics", "--complete", "--server", baseURL(), "--token", ingestToken, manifestNotes)
	mustRemove(t, filepath.Join(manifestNotes, "two.md"))
	runLore(t, ctx, "upload", "notes", "--project", primaryProject, "--source-instance", "manifest-semantics", "--server", baseURL(), "--token", ingestToken, manifestNotes)
	assertActiveSourceDocuments(t, ctx, pool, "manifest-semantics", 2)
	runLore(t, ctx, "upload", "notes", "--project", primaryProject, "--source-instance", "manifest-semantics", "--complete", "--server", baseURL(), "--token", ingestToken, manifestNotes)
	assertActiveSourceDocuments(t, ctx, pool, "manifest-semantics", 1)

	note := findDocument(t, listing.Notes, "Adapter finding")
	var old browse.DocumentDetail
	getJSON(t, fmt.Sprintf("%s/api/projects/%s/documents/%s", baseURL(), primaryProject, note.ID), http.StatusOK, &old)
	createAnnotation(t, old, "Prepared annotation on the prior revision")
	notePath := filepath.Join(fixtures, "notes", "note-identity.md")
	appendFile(t, notePath, "\nThis changed revision remains available while its annotation is open.\n")
	runLore(t, ctx, "upload", "notes", "--project", primaryProject, "--source-instance", "mnemonic", "--complete", "--server", baseURL(), "--token", ingestToken, filepath.Join(fixtures, "notes"))
	var current browse.DocumentDetail
	getJSON(t, fmt.Sprintf("%s/api/projects/%s/documents/%s", baseURL(), primaryProject, note.ID), http.StatusOK, &current)
	if len(current.Revisions) != 2 || current.RevisionID == old.RevisionID {
		t.Fatalf("annotated prior revision was not retained: %#v", current.Revisions)
	}

	briefing := findDocument(t, listing.Briefings, "Architecture")
	briefingPath := filepath.Join(fixtures, "briefing", "architecture.html")
	appendFile(t, briefingPath, "\n<!-- changed without annotation -->\n")
	runLore(t, ctx, "upload", "briefing", "--project", primaryProject, "--source-instance", "architecture", "--complete", "--server", baseURL(), "--token", ingestToken, briefingPath)
	var revisedBriefing browse.DocumentDetail
	getJSON(t, fmt.Sprintf("%s/api/projects/%s/documents/%s", baseURL(), primaryProject, briefing.ID), http.StatusOK, &revisedBriefing)
	if len(revisedBriefing.Revisions) != 1 {
		t.Fatalf("unannotated replaced revision retained: %#v", revisedBriefing.Revisions)
	}

	testWatch(t, ctx, work)
	testBriefingContract(t, ctx, work)
	waitForEmbeddings(t, ctx, pool)
	testHybridAndIsolation(t, ctx)

	snapshotPath := filepath.Join(work, "annotations-snapshot.json")
	runLore(t, ctx, "annotations", "export", "--project", primaryProject, "--server", baseURL(), "--output", snapshotPath)
	var snapshot annotations.Export
	readJSONFile(t, snapshotPath, &snapshot)
	if snapshot.Mode != "snapshot" || len(snapshot.Annotations) == 0 || snapshot.NextCursor == 0 {
		t.Fatalf("invalid annotation snapshot: %#v", snapshot)
	}
	state := gateState{DocumentID: note.ID, OldRevisionID: old.RevisionID, CurrentRevision: current.RevisionID, OldContentHash: old.ContentHash, SnapshotCursor: snapshot.NextCursor}
	writeJSONFile(t, requiredEnv(t, "LORE_E2E_STATE_PATH"), state)
}

func TestBuiltServerServesEmbeddedSPA(t *testing.T) {
	for _, route := range []string{"/", "/regression/deep/spa-route"} {
		response, err := httpClient.Get(baseURL() + route)
		if err != nil {
			t.Fatalf("GET embedded SPA route %q: %v", route, err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatalf("read embedded SPA route %q: %v", route, readErr)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("embedded SPA route %q returned %s: %s", route, response.Status, body)
		}
		if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
			t.Fatalf("embedded SPA route %q content type = %q, want text/html", route, contentType)
		}
		if !bytes.Contains(body, []byte(`<div id="root"></div>`)) {
			t.Fatalf("embedded SPA route %q did not return the Vite application shell", route)
		}
	}
}

func TestFinalizeGate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := openPool(t, ctx)
	defer pool.Close()
	var state gateState
	readJSONFile(t, requiredEnv(t, "LORE_E2E_STATE_PATH"), &state)
	var listing struct {
		Annotations []annotations.Record `json:"annotations"`
	}
	getJSON(t, fmt.Sprintf("%s/api/projects/%s/annotations?revisionId=%s", baseURL(), primaryProject, state.OldRevisionID), http.StatusOK, &listing)
	if len(listing.Annotations) < 2 {
		t.Fatalf("browser did not exercise copy/move on prior revision: %#v", listing.Annotations)
	}
	for index, record := range listing.Annotations {
		status := "resolved"
		if index%2 == 1 {
			status = "dismissed"
		}
		requestJSON(t, http.MethodPatch, fmt.Sprintf("%s/api/projects/%s/annotations/%s", baseURL(), primaryProject, record.ID), "", map[string]any{"status": status, "attributedUsername": annotationAuthor}, http.StatusOK, nil)
	}
	var cleaned annotations.CleanupResult
	requestJSON(t, http.MethodPost, baseURL()+"/api/projects/"+primaryProject+"/admin/cleanup", adminToken, map[string]any{"revisionId": state.OldRevisionID, "attributedUsername": annotationAuthor}, http.StatusOK, &cleaned)
	if cleaned.RevisionsRemoved != 1 || cleaned.AnnotationsTombstoned < 2 {
		t.Fatalf("cleanup result = %#v", cleaned)
	}
	requestJSON(t, http.MethodGet, fmt.Sprintf("%s/api/projects/%s/documents/%s/revisions/%s", baseURL(), primaryProject, state.DocumentID, state.OldRevisionID), "", nil, http.StatusNotFound, nil)
	listing = struct {
		Annotations []annotations.Record `json:"annotations"`
	}{}
	getJSON(t, fmt.Sprintf("%s/api/projects/%s/annotations?revisionId=%s", baseURL(), primaryProject, state.OldRevisionID), http.StatusOK, &listing)
	for _, record := range listing.Annotations {
		if record.RevisionID != nil || record.TombstonedAt == nil || record.OriginalContentHash != state.OldContentHash {
			t.Fatalf("annotation tombstone lost immutable context: %#v", record)
		}
	}

	incrementalPath := filepath.Join(requiredEnv(t, "LORE_E2E_WORKDIR"), "annotations-incremental.json")
	runLore(t, ctx, "annotations", "export", "--project", primaryProject, "--server", baseURL(), "--after", fmt.Sprint(state.SnapshotCursor), "--output", incrementalPath)
	var incremental annotations.Export
	readJSONFile(t, incrementalPath, &incremental)
	if incremental.Mode != "incremental" || incremental.AfterCursor != state.SnapshotCursor || len(incremental.Annotations) == 0 {
		t.Fatalf("invalid incremental export: %#v", incremental)
	}
	tombstone := false
	for _, record := range incremental.Annotations {
		tombstone = tombstone || record.TombstonedAt != nil
	}
	if !tombstone {
		t.Fatal("incremental export did not contain cleanup tombstones")
	}
	if count := scalar[int](t, ctx, pool, `SELECT count(*) FROM revisions WHERE id = $1`, state.CurrentRevision); count != 1 {
		t.Fatal("cleanup removed the current revision")
	}
}

func testWatch(t *testing.T, ctx context.Context, work string) {
	dir := filepath.Join(work, "watch-notes")
	mustMkdir(t, dir)
	path := filepath.Join(dir, "watched.md")
	mustWrite(t, path, []byte("---\ntitle: Watched note\n---\nStartup projection.\n"))
	runWatchPhase(t, ctx, work, dir, "50ms", "1h", "fsnotify live change", 4*time.Second)
	brokenPath := filepath.Join(dir, "broken.md")
	mustWrite(t, brokenPath, []byte("---\ntitle: Retryable watcher note\n---\nThe corrected file remains quarantined until its issue is cleared.\n"))
	runWatchPhase(t, ctx, work, dir, "5s", "250ms", "periodic rescan recovery", 4*time.Second)
	if documentText("Retryable watcher note", "corrected file") {
		t.Fatal("watcher retried a quarantined file before its issue was cleared")
	}
	if !hasWatcherFailure("broken.md") {
		t.Fatal("watcher parse failure did not remain available for UI retry")
	}
}

func runWatchPhase(t *testing.T, ctx context.Context, work, dir, debounce, rescan, phrase string, deadline time.Duration) {
	config := filepath.Join(work, "watch-"+strings.ReplaceAll(debounce, ".", "_")+".yml")
	mustWrite(t, config, []byte(fmt.Sprintf("project: %s\ndebounce: %s\nrescan-interval: %s\nsources:\n  notes:\n    source-instance: watched\n    path: %s\n", primaryProject, debounce, rescan, dir)))
	logPath := config + ".log"
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, requiredEnv(t, "LORE_E2E_LORE_BIN"), "watch", "--config", config, "--server", baseURL(), "--token", ingestToken)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Signal(os.Interrupt); _ = cmd.Wait(); _ = logFile.Close() }()
	waitUntil(t, 5*time.Second, func() bool { return documentText("Watched note", "Startup projection") })
	appendFile(t, filepath.Join(dir, "watched.md"), "\n"+phrase+".\n")
	started := time.Now()
	waitUntil(t, deadline, func() bool { return documentText("Watched note", phrase) })
	if debounce == "5s" && time.Since(started) >= 5*time.Second {
		t.Fatal("watch update did not arrive through periodic rescan before debounce")
	}
	if debounce == "50ms" {
		mustWrite(t, filepath.Join(dir, "broken.md"), []byte("---\ntitle: [unterminated\n---\nMalformed front matter.\n"))
		waitUntil(t, 4*time.Second, func() bool { return hasWatcherFailure("broken.md") })
	}
}

func testBriefingContract(t *testing.T, ctx context.Context, work string) {
	css := runLore(t, ctx, "briefings", "show-css")
	skill := runLore(t, ctx, "briefings", "show-skill")
	if !strings.Contains(css, ".lore-prose") || !strings.Contains(skill, "briefing") {
		t.Fatal("embedded briefing authoring assets are incomplete")
	}
	cssPath, skillPath := filepath.Join(work, "site.css"), filepath.Join(work, "SKILL.md")
	runLore(t, ctx, "briefings", "write-css", cssPath)
	runLore(t, ctx, "briefings", "write-skill", skillPath)
	if got := string(mustRead(t, cssPath)); got != css {
		t.Fatal("write-css differs from show-css")
	}
	if got := string(mustRead(t, skillPath)); got != skill {
		t.Fatal("write-skill differs from show-skill")
	}
	var contract map[string]any
	if err := json.Unmarshal([]byte(runLore(t, ctx, "briefings", "contract", "--format", "json")), &contract); err != nil || contract["containerClass"] != "lore-prose" {
		t.Fatalf("briefing contract: %v %#v", err, contract)
	}
}

func testHybridAndIsolation(t *testing.T, ctx context.Context) {
	var response retrieval.Response
	output := runLore(t, ctx, "search", "--project", primaryProject, "--server", baseURL(), "--limit", "5", "foundation architecture")
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("decode CLI search response: %v\n%s", err, output)
	}
	if !response.Modes.Keyword || !response.Modes.Vector || len(response.Results) == 0 {
		t.Fatalf("CLI hybrid modes/results = %#v", response)
	}
	both := false
	for _, result := range response.Results {
		for _, chunk := range result.MatchedChunks {
			both = both || chunk.KeywordRank != nil && chunk.VectorRank != nil
		}
	}
	if !both {
		t.Fatal("no chunk participated in both independent rankings")
	}
	query := url.Values{"q": {"velvet-quasar-719"}}
	getJSON(t, baseURL()+"/api/projects/"+primaryProject+"/search?"+query.Encode(), http.StatusOK, &response)
	for _, result := range response.Results {
		if result.Title == "Quasar isolation ledger" {
			t.Fatal("isolated project leaked into primary retrieval")
		}
	}
	getJSON(t, baseURL()+"/api/projects/"+isolatedProject+"/search?"+query.Encode(), http.StatusOK, &response)
	if len(response.Results) != 1 || response.Results[0].Title != "Quasar isolation ledger" {
		t.Fatalf("isolated search = %#v", response.Results)
	}
}

func waitForEmbeddings(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	waitUntil(t, 90*time.Second, func() bool {
		var jobs, chunks, vectors int
		if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM embedding_jobs), (SELECT count(*) FROM chunks), (SELECT count(*) FROM embeddings)`).Scan(&jobs, &chunks, &vectors); err != nil {
			return false
		}
		return jobs == 0 && chunks > 0 && vectors == chunks
	})
}

func assertSourceCounts(t *testing.T, listing browse.BrowseResponse) {
	t.Helper()
	if len(listing.Tasks) != 2 || len(listing.Notes) != 2 || len(listing.Briefings) != 2 || len(listing.Repositories) != 1 || len(listing.Repositories[0].Documents) != 3 || len(listing.Conversations) != 2 || len(listing.Terms) != 2 {
		t.Fatalf("real source browse counts = tasks %d notes %d briefings %d repository %d conversations %d", len(listing.Tasks), len(listing.Notes), len(listing.Briefings), len(listing.Repositories[0].Documents), len(listing.Conversations))
	}
}

func createAnnotation(t *testing.T, detail browse.DocumentDetail, body string) {
	t.Helper()
	var record annotations.Record
	requestJSON(t, http.MethodPost, baseURL()+"/api/projects/"+primaryProject+"/annotations", "", annotations.CreateRequest{DocumentID: detail.ID, RevisionID: detail.RevisionID, Body: body, AttributedUsername: annotationAuthor, OriginatingOperation: "e2e-prepare", Selector: json.RawMessage(`{"kind":"heading-path","headingPath":["Adapter finding"]}`), OriginalContentHash: detail.ContentHash}, http.StatusCreated, &record)
}

func documentText(title, phrase string) bool {
	var listing browse.BrowseResponse
	if status := getJSONStatus(baseURL()+"/api/projects/"+primaryProject+"/browse", &listing); status != http.StatusOK {
		return false
	}
	for _, summary := range listing.Notes {
		if summary.Title != title {
			continue
		}
		var detail browse.DocumentDetail
		return getJSONStatus(fmt.Sprintf("%s/api/projects/%s/documents/%s", baseURL(), primaryProject, summary.ID), &detail) == http.StatusOK && strings.Contains(detail.NormalizedText, phrase)
	}
	return false
}

func hasWatcherFailure(filename string) bool {
	var listing struct {
		Failures []struct {
			Path string `json:"path"`
		} `json:"failures"`
	}
	if status := getJSONStatus(baseURL()+"/api/projects/"+primaryProject+"/ingestion-failures", &listing); status != http.StatusOK {
		return false
	}
	for _, failure := range listing.Failures {
		if filepath.Base(failure.Path) == filename {
			return true
		}
	}
	return false
}

func findDocument(t *testing.T, documents []browse.DocumentSummary, title string) browse.DocumentSummary {
	t.Helper()
	for _, document := range documents {
		if document.Title == title {
			return document
		}
	}
	t.Fatalf("document %q not found", title)
	return browse.DocumentSummary{}
}
func assertActiveSourceDocuments(t *testing.T, ctx context.Context, pool *pgxpool.Pool, source string, want int) {
	t.Helper()
	got := scalar[int](t, ctx, pool, `SELECT count(*) FROM documents d JOIN source_instances s ON s.id=d.source_instance_id WHERE s.external_key=$1 AND d.deleted_at IS NULL`, source)
	if got != want {
		t.Fatalf("active documents for %s = %d, want %d", source, got, want)
	}
}
func scalar[T any](t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) T {
	t.Helper()
	var value T
	if err := pool.QueryRow(ctx, query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
func openPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(ctx, requiredEnv(t, "LORE_E2E_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func runLore(t *testing.T, ctx context.Context, args ...string) string {
	t.Helper()
	command := exec.CommandContext(ctx, requiredEnv(t, "LORE_E2E_LORE_BIN"), args...)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Run(); err != nil {
		t.Fatalf("lore %s: %v\n%s", strings.Join(args, " "), err, output.String())
	}
	logged := strings.TrimSpace(output.String())
	if len(logged) > 1000 {
		logged = logged[:1000] + "\n… output truncated by e2e harness"
	}
	t.Logf("lore %s\n%s", strings.Join(args, " "), logged)
	return output.String()
}
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}
func baseURL() string { return os.Getenv("LORE_E2E_BASE_URL") }
func requiredEnv(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Fatalf("%s is required", key)
	}
	return value
}

func requestJSON(t *testing.T, method, endpoint, token string, body any, want int, result any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
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
	response, err := httpClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != want {
		t.Fatalf("%s %s = %d, want %d: %s", method, endpoint, response.StatusCode, want, data)
	}
	if result != nil && len(data) > 0 {
		if err := json.Unmarshal(data, result); err != nil {
			t.Fatalf("decode %s: %v: %s", endpoint, err, data)
		}
	}
}
func getJSON(t *testing.T, endpoint string, want int, result any) {
	t.Helper()
	requestJSON(t, http.MethodGet, endpoint, "", nil, want, result)
}
func getJSONStatus(endpoint string, result any) int {
	response, err := httpClient.Get(endpoint)
	if err != nil {
		return 0
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		_ = json.NewDecoder(response.Body).Decode(result)
	}
	return response.StatusCode
}
func waitUntil(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
func appendFile(t *testing.T, path, text string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(text); err != nil {
		t.Fatal(err)
	}
}
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}
func mustRemove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, data)
}
func readJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	if err := json.Unmarshal(mustRead(t, path), value); err != nil {
		t.Fatal(err)
	}
}
