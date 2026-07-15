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

func (s Source) Build(boundary synchronization.Boundary) ([]synchronization.Manifest, int, []string, error) {
	return s.build(boundary, nil)
}

func (s Source) BuildForWatcher(boundary synchronization.Boundary, skipPaths map[string]struct{}) ([]synchronization.Manifest, int, []string, error) {
	watch := &adapters.WatchOptions{SkipPaths: skipPaths}
	return s.build(boundary, watch)
}

func (s Source) build(boundary synchronization.Boundary, watch *adapters.WatchOptions) ([]synchronization.Manifest, int, []string, error) {
	if s.SourceInstance == "" {
		return nil, 0, nil, fmt.Errorf("source-instance is required")
	}
	options := adapters.Options{Project: s.Project, SourceInstance: s.SourceInstance, Boundary: boundary}
	switch s.Adapter {
	case "tasks":
		var manifest synchronization.Manifest
		var err error
		if watch == nil {
			manifest, err = adapters.Tasks(s.Path, options)
		} else {
			manifest, err = adapters.WatchTasks(s.Path, options, *watch)
		}
		return one(manifest, err)
	case "notes":
		var manifest synchronization.Manifest
		var err error
		if watch == nil {
			manifest, err = adapters.Notes(s.Path, options)
		} else {
			manifest, err = adapters.WatchNotes(s.Path, options, *watch)
		}
		return one(manifest, err)
	case "briefing":
		var manifest synchronization.Manifest
		var err error
		if watch == nil {
			manifest, err = adapters.Briefing(s.Path, s.Title, options)
		} else {
			manifest, err = adapters.WatchBriefing(s.Path, s.Title, options, *watch)
		}
		return one(manifest, err)
	case "repository":
		paths := s.Paths
		if len(paths) == 0 && s.Path != "" {
			paths = []string{s.Path}
		}
		repositoryOptions := adapters.RepositoryOptions{Options: options, Repository: s.Repository, Branch: s.Branch}
		var manifest synchronization.Manifest
		var err error
		if watch == nil {
			manifest, err = adapters.Repository(paths, repositoryOptions)
		} else {
			manifest, err = adapters.WatchRepository(paths, repositoryOptions, *watch)
		}
		return one(manifest, err)
	case "conversations":
		mappings, err := adapters.LoadProjectMappings(s.Mapping)
		if err != nil {
			return nil, 0, nil, err
		}
		var scan adapters.ConversationScan
		if watch == nil {
			scan, err = adapters.Conversations(s.Provider, s.Path, s.SourceInstance, mappings, s.FallbackProject)
		} else {
			failureProject := s.Project
			if failureProject == "" {
				failureProject = s.FallbackProject
			}
			scan, err = adapters.WatchConversations(s.Provider, s.Path, s.SourceInstance, mappings, s.FallbackProject, failureProject, *watch)
		}
		if err != nil {
			return nil, 0, nil, err
		}
		for index := range scan.Manifests {
			scan.Manifests[index].Boundary = boundary
		}
		return scan.Manifests, scan.Skipped, scan.Warnings, nil
	default:
		return nil, 0, nil, fmt.Errorf("unsupported adapter %q", s.Adapter)
	}
}

func one(manifest synchronization.Manifest, err error) ([]synchronization.Manifest, int, []string, error) {
	if err != nil {
		return nil, 0, nil, err
	}
	return []synchronization.Manifest{manifest}, 0, nil, nil
}
