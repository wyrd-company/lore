package watcher

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wyrd-company/lore/internal/client"
	"github.com/wyrd-company/lore/internal/ingest"
	"github.com/wyrd-company/lore/internal/ingestfailures"
	"github.com/wyrd-company/lore/internal/synchronization"
)

func TestWatcherSkipsRecordedFailureUntilItIsRemoved(t *testing.T) {
	directory := t.TempDir()
	goodPath := filepath.Join(directory, "good.md")
	brokenPath := filepath.Join(directory, "broken.md")
	if err := os.WriteFile(goodPath, []byte("---\ntitle: Good\n---\nGood.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(brokenPath, []byte("---\ntitle: [broken\n---\nBroken.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var failures []ingestfailures.Record
	var manifests []synchronization.Manifest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			_ = json.NewEncoder(writer).Encode(map[string]any{"failures": failures})
			return
		}
		var manifest synchronization.Manifest
		if err := json.NewDecoder(request.Body).Decode(&manifest); err != nil {
			t.Errorf("decode manifest: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		manifests = append(manifests, manifest)
		for _, failure := range manifest.Failures {
			failures = []ingestfailures.Record{{Path: failure.Path, Message: failure.Message}}
		}
		_ = json.NewEncoder(writer).Encode(synchronization.Result{Failed: len(manifest.Failures)})
	}))
	t.Cleanup(server.Close)
	api, err := client.New(server.URL, "ingest-secret")
	if err != nil {
		t.Fatal(err)
	}
	watcher := New(Config{}, api, &bytes.Buffer{})
	watcher.retryAttempts = 1
	source := ingest.Source{Project: "lore", SourceInstance: "notes", Adapter: "notes", Path: directory}
	if !watcher.synchronizeWithRetry(context.Background(), source, synchronization.BoundaryComplete) {
		t.Fatal("initial watcher synchronization failed")
	}
	if len(manifests[0].Documents) != 1 || len(manifests[0].Failures) != 1 {
		t.Fatalf("initial watcher manifest = %#v", manifests[0])
	}
	if err := os.WriteFile(brokenPath, []byte("---\ntitle: Fixed\n---\nFixed.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !watcher.synchronizeWithRetry(context.Background(), source, synchronization.BoundaryComplete) {
		t.Fatal("skipped watcher synchronization failed")
	}
	if len(manifests[1].Documents) != 1 || len(manifests[1].Failures) != 0 {
		t.Fatalf("recorded failure was retried before removal: %#v", manifests[1])
	}
	failures = nil
	if !watcher.synchronizeWithRetry(context.Background(), source, synchronization.BoundaryComplete) {
		t.Fatal("retried watcher synchronization failed")
	}
	if len(manifests[2].Documents) != 2 || len(manifests[2].Failures) != 0 {
		t.Fatalf("removed failure was not retried: %#v", manifests[2])
	}
}

func TestEnqueueCoalescesToLatestRequestWithoutWeakeningCompleteBoundary(t *testing.T) {
	tests := []struct {
		name     string
		pending  synchronization.Boundary
		incoming synchronization.Boundary
		want     synchronization.Boundary
	}{
		{"partial is replaced by complete", synchronization.BoundaryPartial, synchronization.BoundaryComplete, synchronization.BoundaryComplete},
		{"complete is not weakened by partial", synchronization.BoundaryComplete, synchronization.BoundaryPartial, synchronization.BoundaryComplete},
		{"partial remains partial", synchronization.BoundaryPartial, synchronization.BoundaryPartial, synchronization.BoundaryPartial},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			jobs := make(chan syncRequest, 1)
			jobs <- syncRequest{boundary: test.pending}
			enqueue(jobs, syncRequest{boundary: test.incoming})
			if got := (<-jobs).boundary; got != test.want {
				t.Fatalf("boundary = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWorkerRequeuesAfterRetryBudgetIsExhausted(t *testing.T) {
	var log bytes.Buffer
	w := New(Config{}, nil, &log)
	w.retryAttempts = 1
	w.retryInitial = time.Millisecond
	w.retryMaximum = time.Millisecond
	w.requeueDelay = time.Millisecond
	jobs := make(chan syncRequest, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.runWorker(ctx, ingest.Source{SourceInstance: "broken", Adapter: "unknown"}, jobs)
	}()
	enqueue(jobs, syncRequest{boundary: synchronization.BoundaryPartial})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		w.logMu.Lock()
		attempts := strings.Count(log.String(), "synchronization attempt 1 failed")
		w.logMu.Unlock()
		if attempts >= 2 {
			cancel()
			<-done
			return
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	t.Fatalf("worker did not retry after exhausting its budget; log: %s", log.String())
}
