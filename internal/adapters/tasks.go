package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/wyrd-company/lore/internal/rendering"
	"github.com/wyrd-company/lore/internal/synchronization"
	"gopkg.in/yaml.v3"
)

type taskFrontMatter struct {
	ID          int      `yaml:"id" json:"id"`
	Title       string   `yaml:"title" json:"title"`
	Status      string   `yaml:"status" json:"status"`
	Priority    string   `yaml:"priority" json:"priority"`
	Created     string   `yaml:"created" json:"created"`
	Updated     string   `yaml:"updated" json:"updated"`
	Started     string   `yaml:"started,omitempty" json:"started,omitempty"`
	Completed   string   `yaml:"completed,omitempty" json:"completed,omitempty"`
	Assignee    string   `yaml:"assignee,omitempty" json:"assignee,omitempty"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Terms       []string `yaml:"terms,omitempty" json:"terms,omitempty"`
	Due         string   `yaml:"due,omitempty" json:"due,omitempty"`
	Estimate    string   `yaml:"estimate,omitempty" json:"estimate,omitempty"`
	Parent      int      `yaml:"parent,omitempty" json:"parent,omitempty"`
	DependsOn   []int    `yaml:"depends_on,omitempty" json:"dependsOn,omitempty"`
	Blocked     bool     `yaml:"blocked,omitempty" json:"blocked,omitempty"`
	BlockReason string   `yaml:"block_reason,omitempty" json:"blockReason,omitempty"`
	Class       string   `yaml:"class,omitempty" json:"class,omitempty"`
}

type taskBoardMetadata struct {
	Statuses []string `json:"statuses"`
}

func Tasks(boardPath string, options Options) (synchronization.Manifest, error) {
	return tasks(boardPath, options, nil)
}

func WatchTasks(boardPath string, options Options, watch WatchOptions) (synchronization.Manifest, error) {
	return tasks(boardPath, options, &watch)
}

func tasks(boardPath string, options Options, watch *WatchOptions) (synchronization.Manifest, error) {
	manifest := newManifest(options, "task")
	configPath := filepath.Join(boardPath, "config.yml")
	if watch != nil && watch.ShouldSkip(configPath) {
		manifest.Boundary = synchronization.BoundaryPartial
		return manifest, manifest.Validate()
	}
	config, err := loadTaskConfig(boardPath)
	if err != nil {
		if watch != nil {
			recordParseFailure(&manifest, configPath, err)
			manifest.Boundary = synchronization.BoundaryPartial
			return manifest, manifest.Validate()
		}
		return manifest, err
	}
	tasksDirectory := filepath.Join(boardPath, filepath.Clean(config.TasksDirectory))
	entries, err := os.ReadDir(tasksDirectory)
	if err != nil {
		return manifest, fmt.Errorf("read task directory: %w", err)
	}
	statuses := make([]string, 0, len(config.Statuses))
	seenStatuses := make(map[string]struct{}, len(config.Statuses))
	appendStatus := func(status string) {
		status = strings.TrimSpace(status)
		if status == "" {
			return
		}
		key := strings.ToLower(status)
		if _, exists := seenStatuses[key]; exists {
			return
		}
		seenStatuses[key] = struct{}{}
		statuses = append(statuses, status)
	}
	for _, status := range config.Statuses {
		appendStatus(status.Name)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(tasksDirectory, entry.Name())
		if watch != nil && watch.ShouldSkip(path) {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return manifest, fmt.Errorf("read task %q: %w", path, err)
		}
		frontMatter, body, err := parseTask(source)
		if err != nil {
			if watch != nil {
				recordParseFailure(&manifest, path, err)
				continue
			}
			return manifest, fmt.Errorf("parse task %q: %w", path, err)
		}
		if frontMatter.Status == "" {
			frontMatter.Status = config.Defaults.Status
		}
		if frontMatter.Priority == "" {
			frontMatter.Priority = config.Defaults.Priority
		}
		if frontMatter.Class == "" {
			frontMatter.Class = config.Defaults.Class
		}
		appendStatus(frontMatter.Status)
		identity := strconv.Itoa(frontMatter.ID)
		dependencies := make([]string, 0, len(frontMatter.DependsOn))
		for _, dependency := range frontMatter.DependsOn {
			target := strconv.Itoa(dependency)
			dependencies = append(dependencies, target)
		}
		rendered, err := rendering.Task(rendering.TaskModel{
			ID: identity, Title: frontMatter.Title, Status: frontMatter.Status, Priority: frontMatter.Priority,
			Assignee: frontMatter.Assignee, Body: string(body), Tags: frontMatter.Tags, DependsOn: dependencies,
		})
		if err != nil {
			if watch != nil {
				recordParseFailure(&manifest, path, err)
				continue
			}
			return manifest, fmt.Errorf("render task %q: %w", path, err)
		}
		document, err := makeDocument(identity, frontMatter.Title, source, "task", rendered, frontMatter, map[string]any{
			"path": path, "filename": entry.Name(),
		}, frontMatter.Tags)
		if err != nil {
			if watch != nil {
				recordParseFailure(&manifest, path, err)
				continue
			}
			return manifest, err
		}
		document.Terms = normalizeTaxonomyValues(frontMatter.Terms)
		manifest.Documents = append(manifest.Documents, document)
		for _, target := range dependencies {
			manifest.Relationships = append(manifest.Relationships, synchronization.Relationship{
				SourceIdentity: identity, TargetIdentity: target, Type: "task-depends-on",
			})
		}
	}
	manifest.Metadata, err = json.Marshal(taskBoardMetadata{Statuses: statuses})
	if err != nil {
		return manifest, fmt.Errorf("encode kanban metadata: %w", err)
	}
	return manifest, manifest.Validate()
}

type taskStatusConfig struct {
	Name string `yaml:"name"`
}

func (status *taskStatusConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&status.Name)
	}
	type plain taskStatusConfig
	return node.Decode((*plain)(status))
}

type taskBoardConfig struct {
	TasksDirectory string             `yaml:"tasks_dir"`
	Statuses       []taskStatusConfig `yaml:"statuses"`
	Defaults       struct {
		Status   string `yaml:"status"`
		Priority string `yaml:"priority"`
		Class    string `yaml:"class"`
	} `yaml:"defaults"`
}

func loadTaskConfig(boardPath string) (taskBoardConfig, error) {
	configPath := filepath.Join(boardPath, "config.yml")
	contents, err := os.ReadFile(configPath)
	if err != nil {
		return taskBoardConfig{}, fmt.Errorf("read kanban config: %w", err)
	}
	var config taskBoardConfig
	if err := yaml.Unmarshal(contents, &config); err != nil {
		return taskBoardConfig{}, fmt.Errorf("parse kanban config: %w", err)
	}
	if config.TasksDirectory == "" {
		config.TasksDirectory = "tasks"
	}
	if config.Defaults.Status == "" {
		config.Defaults.Status = "backlog"
	}
	if config.Defaults.Priority == "" {
		config.Defaults.Priority = "medium"
	}
	return config, nil
}

func parseTask(source []byte) (taskFrontMatter, []byte, error) {
	normalized := strings.ReplaceAll(string(source), "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return taskFrontMatter{}, nil, fmt.Errorf("task has no YAML front matter")
	}
	end := strings.Index(normalized[4:], "\n---\n")
	if end < 0 {
		return taskFrontMatter{}, nil, fmt.Errorf("task front matter is not terminated")
	}
	end += 4
	var frontMatter taskFrontMatter
	if err := yaml.Unmarshal([]byte(normalized[4:end]), &frontMatter); err != nil {
		return taskFrontMatter{}, nil, err
	}
	if frontMatter.ID <= 0 || frontMatter.Title == "" {
		return taskFrontMatter{}, nil, fmt.Errorf("task requires id and title")
	}
	return frontMatter, []byte(strings.TrimPrefix(normalized[end+5:], "\n")), nil
}
