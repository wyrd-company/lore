package adapters

import (
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
	Due         string   `yaml:"due,omitempty" json:"due,omitempty"`
	Estimate    string   `yaml:"estimate,omitempty" json:"estimate,omitempty"`
	Parent      int      `yaml:"parent,omitempty" json:"parent,omitempty"`
	DependsOn   []int    `yaml:"depends_on,omitempty" json:"dependsOn,omitempty"`
	Blocked     bool     `yaml:"blocked,omitempty" json:"blocked,omitempty"`
	BlockReason string   `yaml:"block_reason,omitempty" json:"blockReason,omitempty"`
	Class       string   `yaml:"class,omitempty" json:"class,omitempty"`
}

func Tasks(boardPath string, options Options) (synchronization.Manifest, error) {
	manifest := newManifest(options, "task")
	tasksDirectory, err := taskDirectory(boardPath)
	if err != nil {
		return manifest, err
	}
	entries, err := os.ReadDir(tasksDirectory)
	if err != nil {
		return manifest, fmt.Errorf("read task directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(tasksDirectory, entry.Name())
		source, err := os.ReadFile(path)
		if err != nil {
			return manifest, fmt.Errorf("read task %q: %w", path, err)
		}
		frontMatter, body, err := parseTask(source)
		if err != nil {
			return manifest, fmt.Errorf("parse task %q: %w", path, err)
		}
		identity := strconv.Itoa(frontMatter.ID)
		dependencies := make([]string, 0, len(frontMatter.DependsOn))
		for _, dependency := range frontMatter.DependsOn {
			target := strconv.Itoa(dependency)
			dependencies = append(dependencies, target)
			manifest.Relationships = append(manifest.Relationships, synchronization.Relationship{
				SourceIdentity: identity, TargetIdentity: target, Type: "task-depends-on",
			})
		}
		rendered, err := rendering.Task(rendering.TaskModel{
			ID: identity, Title: frontMatter.Title, Status: frontMatter.Status, Priority: frontMatter.Priority,
			Assignee: frontMatter.Assignee, Body: string(body), Tags: frontMatter.Tags, DependsOn: dependencies,
		})
		if err != nil {
			return manifest, fmt.Errorf("render task %q: %w", path, err)
		}
		document, err := makeDocument(identity, frontMatter.Title, source, "task", rendered, frontMatter, map[string]any{
			"path": path, "filename": entry.Name(),
		}, frontMatter.Tags)
		if err != nil {
			return manifest, err
		}
		manifest.Documents = append(manifest.Documents, document)
	}
	return manifest, manifest.Validate()
}

func taskDirectory(boardPath string) (string, error) {
	configPath := filepath.Join(boardPath, "config.yml")
	contents, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read kanban config: %w", err)
	}
	var config struct {
		TasksDirectory string `yaml:"tasks_dir"`
	}
	if err := yaml.Unmarshal(contents, &config); err != nil {
		return "", fmt.Errorf("parse kanban config: %w", err)
	}
	if config.TasksDirectory == "" {
		config.TasksDirectory = "tasks"
	}
	return filepath.Join(boardPath, filepath.Clean(config.TasksDirectory)), nil
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
	if frontMatter.ID <= 0 || frontMatter.Title == "" || frontMatter.Status == "" || frontMatter.Priority == "" {
		return taskFrontMatter{}, nil, fmt.Errorf("task requires id, title, status, and priority")
	}
	return frontMatter, []byte(strings.TrimPrefix(normalized[end+5:], "\n")), nil
}
