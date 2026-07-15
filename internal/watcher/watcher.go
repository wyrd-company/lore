package watcher

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/wyrd-company/lore/internal/client"
	"github.com/wyrd-company/lore/internal/ingest"
	"github.com/wyrd-company/lore/internal/ingestfailures"
	"github.com/wyrd-company/lore/internal/synchronization"
)

type Watcher struct {
	config        Config
	client        *client.Client
	log           io.Writer
	logMu         sync.Mutex
	retryAttempts int
	retryInitial  time.Duration
	retryMaximum  time.Duration
	requeueDelay  time.Duration
}

type syncRequest struct {
	boundary synchronization.Boundary
}

func New(config Config, api *client.Client, log io.Writer) *Watcher {
	return &Watcher{
		config: config, client: api, log: log, retryAttempts: 5,
		retryInitial: 250 * time.Millisecond, retryMaximum: 4 * time.Second, requeueDelay: 4 * time.Second,
	}
}

func (w *Watcher) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	filesystem, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create filesystem watcher: %w", err)
	}
	defer filesystem.Close()
	for _, source := range w.config.Sources {
		for _, path := range source.WatchPaths() {
			if err := addRecursive(filesystem, path); err != nil {
				return err
			}
		}
	}

	requests := make([]chan syncRequest, len(w.config.Sources))
	var workers sync.WaitGroup
	for index := range w.config.Sources {
		requests[index] = make(chan syncRequest, 1)
		workers.Add(1)
		go func(source ingest.Source, jobs chan syncRequest) {
			defer workers.Done()
			w.runWorker(ctx, source, jobs)
		}(w.config.Sources[index], requests[index])
	}
	defer func() {
		cancel()
		workers.Wait()
	}()
	for _, jobs := range requests {
		enqueue(jobs, syncRequest{boundary: synchronization.BoundaryComplete})
	}

	dirty := make(map[int]struct{})
	var debounce <-chan time.Time
	var debounceTimer *time.Timer
	rescan := time.NewTicker(w.config.RescanInterval)
	defer rescan.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-filesystem.Events:
			if !ok {
				return nil
			}
			if event.Op&fsnotify.Create != 0 {
				if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
					_ = addRecursive(filesystem, event.Name)
				}
			}
			for index, source := range w.config.Sources {
				if sourceContains(source, event.Name) {
					dirty[index] = struct{}{}
				}
			}
			if len(dirty) > 0 {
				if debounceTimer == nil {
					debounceTimer = time.NewTimer(w.config.Debounce)
				} else {
					if !debounceTimer.Stop() {
						select {
						case <-debounceTimer.C:
						default:
						}
					}
					debounceTimer.Reset(w.config.Debounce)
				}
				debounce = debounceTimer.C
			}
		case err, ok := <-filesystem.Errors:
			if ok {
				w.writeLog("watch error: %v\n", err)
			}
		case <-debounce:
			for index := range dirty {
				enqueue(requests[index], syncRequest{boundary: synchronization.BoundaryPartial})
			}
			dirty = make(map[int]struct{})
			debounce = nil
		case <-rescan.C:
			for _, jobs := range requests {
				enqueue(jobs, syncRequest{boundary: synchronization.BoundaryComplete})
			}
		}
	}
}

func (w *Watcher) runWorker(ctx context.Context, source ingest.Source, jobs chan syncRequest) {
	for {
		select {
		case <-ctx.Done():
			return
		case request := <-jobs:
			if w.synchronizeWithRetry(ctx, source, request.boundary) {
				continue
			}
			timer := time.NewTimer(w.requeueDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				enqueue(jobs, request)
			}
		}
	}
}

func (w *Watcher) synchronizeWithRetry(ctx context.Context, source ingest.Source, boundary synchronization.Boundary) bool {
	delay := w.retryInitial
	for attempt := 1; attempt <= w.retryAttempts; attempt++ {
		failureProject := source.Project
		if failureProject == "" && source.Adapter == "conversations" {
			failureProject = source.FallbackProject
		}
		skipPaths := make(map[string]struct{})
		var err error
		if failureProject != "" {
			var failures []ingestfailures.Record
			failures, err = w.client.IngestionFailures(ctx, failureProject, sourceType(source.Adapter), source.SourceInstance)
			for _, failure := range failures {
				skipPaths[filepath.Clean(failure.Path)] = struct{}{}
			}
		}
		var manifests []synchronization.Manifest
		var skipped int
		var warnings []string
		if err == nil {
			manifests, skipped, warnings, err = source.BuildForWatcher(boundary, skipPaths)
		}
		for _, warning := range warnings {
			w.writeLog("%s warning: %s\n", source.SourceInstance, warning)
		}
		if err == nil {
			for _, manifest := range manifests {
				var result synchronization.Result
				result, err = w.client.Synchronize(ctx, manifest)
				if err != nil {
					break
				}
				w.writeLog("%s/%s: %d created, %d updated, %d unchanged, %d deleted, %d failed\n",
					manifest.Project, manifest.SourceInstance, result.Created, result.Updated, result.Unchanged, result.Deleted, result.Failed)
			}
		}
		if err == nil {
			if skipped > 0 {
				w.writeLog("%s: skipped %d unassigned session(s)\n", source.SourceInstance, skipped)
			}
			return true
		}
		w.writeLog("%s synchronization attempt %d failed: %v\n", source.SourceInstance, attempt, err)
		if attempt == w.retryAttempts {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
		}
		delay = min(delay*2, w.retryMaximum)
	}
	return false
}

func sourceType(adapter string) string {
	switch adapter {
	case "tasks":
		return "task"
	case "notes":
		return "note"
	case "conversations":
		return "conversation"
	default:
		return adapter
	}
}

func (w *Watcher) writeLog(format string, arguments ...any) {
	w.logMu.Lock()
	defer w.logMu.Unlock()
	_, _ = fmt.Fprintf(w.log, format, arguments...)
}

func enqueue(jobs chan syncRequest, request syncRequest) {
	select {
	case jobs <- request:
	default:
		select {
		case pending := <-jobs:
			if pending.boundary == synchronization.BoundaryComplete {
				request = pending
			}
		default:
		}
		select {
		case jobs <- request:
		default:
		}
	}
}

func addRecursive(filesystem *fsnotify.Watcher, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("watch %q: %w", path, err)
	}
	if !info.IsDir() {
		return filesystem.Add(filepath.Dir(path))
	}
	return filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return filesystem.Add(current)
		}
		return nil
	})
}

func sourceContains(source ingest.Source, changed string) bool {
	changed, _ = filepath.Abs(changed)
	for _, root := range source.WatchPaths() {
		root, _ = filepath.Abs(root)
		if changed == root || strings.HasPrefix(changed, root+string(filepath.Separator)) ||
			strings.HasPrefix(root, changed+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
