package ingest

import (
	"fmt"

	"github.com/wyrd-company/lore/internal/adapters"
	"github.com/wyrd-company/lore/internal/synchronization"
)

// Source describes one local source projection. It is shared by the one-shot CLI
// and the watcher so both paths produce exactly the same manifests.
type Source struct {
	Project         string   `yaml:"project"`
	SourceInstance  string   `yaml:"source-instance"`
	Adapter         string   `yaml:"adapter"`
	Path            string   `yaml:"path"`
	Paths           []string `yaml:"paths,omitempty"`
	Title           string   `yaml:"title,omitempty"`
	Repository      string   `yaml:"repository,omitempty"`
	Branch          string   `yaml:"branch,omitempty"`
	Provider        string   `yaml:"provider,omitempty"`
	Mapping         string   `yaml:"mapping,omitempty"`
	FallbackProject string   `yaml:"fallback-project,omitempty"`
}

func (s Source) WatchPaths() []string {
	if len(s.Paths) > 0 {
		return append([]string(nil), s.Paths...)
	}
	if s.Path != "" {
		return []string{s.Path}
	}
	return nil
}

func (s Source) Build(boundary synchronization.Boundary) ([]synchronization.Manifest, int, error) {
	if s.SourceInstance == "" {
		return nil, 0, fmt.Errorf("source-instance is required")
	}
	options := adapters.Options{Project: s.Project, SourceInstance: s.SourceInstance, Boundary: boundary}
	switch s.Adapter {
	case "tasks":
		manifest, err := adapters.Tasks(s.Path, options)
		return one(manifest, err)
	case "notes":
		manifest, err := adapters.Notes(s.Path, options)
		return one(manifest, err)
	case "briefing":
		manifest, err := adapters.Briefing(s.Path, s.Title, options)
		return one(manifest, err)
	case "repository":
		paths := s.Paths
		if len(paths) == 0 && s.Path != "" {
			paths = []string{s.Path}
		}
		manifest, err := adapters.Repository(paths, adapters.RepositoryOptions{
			Options: options, Repository: s.Repository, Branch: s.Branch,
		})
		return one(manifest, err)
	case "conversations":
		mappings, err := adapters.LoadProjectMappings(s.Mapping)
		if err != nil {
			return nil, 0, err
		}
		scan, err := adapters.Conversations(s.Provider, s.Path, s.SourceInstance, mappings, s.FallbackProject)
		if err != nil {
			return nil, 0, err
		}
		for index := range scan.Manifests {
			scan.Manifests[index].Boundary = boundary
		}
		return scan.Manifests, scan.Skipped, nil
	default:
		return nil, 0, fmt.Errorf("unsupported adapter %q", s.Adapter)
	}
}

func one(manifest synchronization.Manifest, err error) ([]synchronization.Manifest, int, error) {
	if err != nil {
		return nil, 0, err
	}
	return []synchronization.Manifest{manifest}, 0, nil
}
