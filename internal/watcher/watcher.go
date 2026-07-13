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
	"github.com/wyrd-company/lore/internal/synchronization"
)

type Watcher struct {
	config Config
	client *client.Client
	log    io.Writer
}

type syncRequest struct {
	boundary synchronization.Boundary
}

func New(config Config, api *client.Client, log io.Writer) *Watcher {
	return &Watcher{config: config, client: api, log: log}
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
		go func(source ingest.Source, jobs <-chan syncRequest) {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case request := <-jobs:
					w.synchronizeWithRetry(ctx, source, request.boundary)
				}
			}
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
				_, _ = fmt.Fprintf(w.log, "watch error: %v\n", err)
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

func (w *Watcher) synchronizeWithRetry(ctx context.Context, source ingest.Source, boundary synchronization.Boundary) {
	delay := 250 * time.Millisecond
	for attempt := 1; attempt <= 5; attempt++ {
		manifests, skipped, err := source.Build(boundary)
		if err == nil {
			for _, manifest := range manifests {
				var result synchronization.Result
				result, err = w.client.Synchronize(ctx, manifest)
				if err != nil {
					break
				}
				_, _ = fmt.Fprintf(w.log, "%s/%s: %d created, %d updated, %d unchanged, %d deleted\n",
					manifest.Project, manifest.SourceInstance, result.Created, result.Updated, result.Unchanged, result.Deleted)
			}
		}
		if err == nil {
			if skipped > 0 {
				_, _ = fmt.Fprintf(w.log, "%s: skipped %d unassigned session(s)\n", source.SourceInstance, skipped)
			}
			return
		}
		_, _ = fmt.Fprintf(w.log, "%s synchronization attempt %d failed: %v\n", source.SourceInstance, attempt, err)
		if attempt == 5 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, 4*time.Second)
	}
}

func enqueue(jobs chan syncRequest, request syncRequest) {
	select {
	case jobs <- request:
	default:
		if request.boundary == synchronization.BoundaryComplete {
			select {
			case <-jobs:
			default:
			}
			select {
			case jobs <- request:
			default:
			}
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
