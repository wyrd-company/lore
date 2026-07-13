package watcher

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wyrd-company/lore/internal/ingest"
	"github.com/wyrd-company/lore/internal/synchronization"
)

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
