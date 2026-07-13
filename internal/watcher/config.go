package watcher

import (
	"fmt"
	"os"
	"time"

	"github.com/wyrd-company/lore/internal/ingest"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Project        string
	Debounce       time.Duration
	RescanInterval time.Duration
	Sources        []ingest.Source
}

type rawConfig struct {
	Project        string    `yaml:"project"`
	Debounce       string    `yaml:"debounce"`
	RescanInterval string    `yaml:"rescan-interval"`
	Sources        yaml.Node `yaml:"sources"`
}

func LoadConfig(path string) (Config, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read watch config: %w", err)
	}
	var raw rawConfig
	if err := yaml.Unmarshal(contents, &raw); err != nil {
		return Config{}, fmt.Errorf("parse watch config: %w", err)
	}
	config := Config{Project: raw.Project, Debounce: 750 * time.Millisecond, RescanInterval: 15 * time.Minute}
	if raw.Debounce != "" {
		config.Debounce, err = time.ParseDuration(raw.Debounce)
		if err != nil {
			return Config{}, fmt.Errorf("parse debounce: %w", err)
		}
	}
	if raw.RescanInterval != "" {
		config.RescanInterval, err = time.ParseDuration(raw.RescanInterval)
		if err != nil {
			return Config{}, fmt.Errorf("parse rescan-interval: %w", err)
		}
	}
	if err := decodeSources(raw.Sources, &config); err != nil {
		return Config{}, err
	}
	if len(config.Sources) == 0 {
		return Config{}, fmt.Errorf("watch config requires at least one source")
	}
	for index := range config.Sources {
		source := &config.Sources[index]
		if source.Project == "" {
			source.Project = config.Project
		}
		if source.SourceInstance == "" {
			source.SourceInstance = source.Adapter
		}
		if source.Project == "" && source.Adapter != "conversations" {
			return Config{}, fmt.Errorf("source %d requires a project", index)
		}
		if len(source.WatchPaths()) == 0 {
			return Config{}, fmt.Errorf("source %d requires a path", index)
		}
	}
	return config, nil
}

func decodeSources(node yaml.Node, config *Config) error {
	if node.Kind == 0 {
		return nil
	}
	switch node.Kind {
	case yaml.SequenceNode:
		return node.Decode(&config.Sources)
	case yaml.MappingNode:
		for index := 0; index < len(node.Content); index += 2 {
			adapter := node.Content[index].Value
			value := node.Content[index+1]
			if value.Kind == yaml.ScalarNode {
				config.Sources = append(config.Sources, ingest.Source{
					Project: config.Project, SourceInstance: adapter, Adapter: adapter, Path: value.Value,
				})
				continue
			}
			var source ingest.Source
			if err := value.Decode(&source); err != nil {
				return fmt.Errorf("parse source %q: %w", adapter, err)
			}
			if source.Adapter == "" {
				source.Adapter = adapter
			}
			config.Sources = append(config.Sources, source)
		}
		return nil
	default:
		return fmt.Errorf("sources must be a list or mapping")
	}
}
