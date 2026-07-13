package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigSupportsDesignShorthandAndExpandedSources(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "watch.yml")
	contents := []byte(`project: refinery
debounce: 50ms
rescan-interval: 2m
sources:
  tasks: /sources/refinery/tasks
  notes:
    path: /sources/refinery/notes
    source-instance: mnemonic
`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.Debounce != 50*time.Millisecond || len(config.Sources) != 2 {
		t.Fatalf("unexpected config: %#v", config)
	}
	if config.Sources[0].Adapter != "tasks" || config.Sources[0].SourceInstance != "tasks" {
		t.Fatalf("unexpected shorthand source: %#v", config.Sources[0])
	}
	if config.Sources[1].Adapter != "notes" || config.Sources[1].SourceInstance != "mnemonic" {
		t.Fatalf("unexpected expanded source: %#v", config.Sources[1])
	}
}
