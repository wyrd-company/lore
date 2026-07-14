package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultServerURL = "http://localhost:8080"

type configSelection struct {
	Path     string
	Explicit bool
}

type resolvedValue struct {
	Value  string
	Source string
}

type configLookup struct {
	Path   string
	Loaded bool
}

type clientConfig struct {
	ServerURL   resolvedValue
	IngestToken resolvedValue
	AdminToken  resolvedValue
	Lookups     []configLookup
}

type clientConfigFile struct {
	ServerURL   string `yaml:"server"`
	IngestToken string `yaml:"ingest-token"`
	AdminToken  string `yaml:"admin-token"`
}

func resolveClientConfig(selection configSelection) (clientConfig, error) {
	config := clientConfig{}
	for _, path := range configPaths(selection) {
		path = expandHome(path)
		contents, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			config.Lookups = append(config.Lookups, configLookup{Path: path})
			continue
		}
		if err != nil {
			return clientConfig{}, fmt.Errorf("read Lore client config %s: %w", path, err)
		}
		config.Lookups = append(config.Lookups, configLookup{Path: path, Loaded: true})
		file, err := decodeClientConfig(contents)
		if err != nil {
			return clientConfig{}, fmt.Errorf("parse Lore client config %s: %w", path, err)
		}
		setIfEmpty(&config.ServerURL, file.ServerURL, "config "+path)
		setIfEmpty(&config.IngestToken, file.IngestToken, "config "+path)
		setIfEmpty(&config.AdminToken, file.AdminToken, "config "+path)
	}

	if value := os.Getenv("LORE_SERVER_URL"); value != "" {
		config.ServerURL = resolvedValue{Value: value, Source: "environment LORE_SERVER_URL"}
	} else if value := os.Getenv("PUBLIC_BASE_URL"); value != "" {
		config.ServerURL = resolvedValue{Value: value, Source: "environment PUBLIC_BASE_URL"}
	}
	setFromEnvironment(&config.IngestToken, "LORE_INGEST_TOKEN")
	setFromEnvironment(&config.AdminToken, "LORE_ADMIN_TOKEN")
	if config.ServerURL.Value == "" {
		config.ServerURL = resolvedValue{Value: defaultServerURL, Source: "default"}
	}
	return config, nil
}

func decodeClientConfig(contents []byte) (clientConfigFile, error) {
	var file clientConfigFile
	if len(bytes.TrimSpace(contents)) == 0 {
		return file, nil
	}
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return clientConfigFile{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return clientConfigFile{}, fmt.Errorf("multiple YAML documents are not supported")
		}
		return clientConfigFile{}, err
	}
	return file, nil
}

func configPaths(selection configSelection) []string {
	if selection.Explicit {
		return []string{selection.Path}
	}
	if path := os.Getenv("LORE_CONFIG"); path != "" {
		return []string{path}
	}
	paths := make([]string, 0, 2)
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "lore", "config.yml"))
	} else if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".config", "lore", "config.yml"))
	}
	return append(paths, "/etc/lore/config.yml")
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func setIfEmpty(target *resolvedValue, value, source string) {
	if target.Value == "" && value != "" {
		*target = resolvedValue{Value: value, Source: source}
	}
}

func setFromEnvironment(target *resolvedValue, name string) {
	if value := os.Getenv(name); value != "" {
		*target = resolvedValue{Value: value, Source: "environment " + name}
	}
}

func selectionFromArgs(args []string, inherited configSelection) (configSelection, error) {
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--config" {
			if index+1 >= len(args) {
				return configSelection{}, fmt.Errorf("--config requires a path")
			}
			if args[index+1] == "" {
				return configSelection{}, fmt.Errorf("--config path must not be empty")
			}
			if inherited.Explicit {
				return configSelection{}, fmt.Errorf("Lore client --config was specified more than once")
			}
			return configSelection{Path: args[index+1], Explicit: true}, nil
		}
		if strings.HasPrefix(argument, "--config=") {
			path := strings.TrimPrefix(argument, "--config=")
			if path == "" {
				return configSelection{}, fmt.Errorf("--config path must not be empty")
			}
			if inherited.Explicit {
				return configSelection{}, fmt.Errorf("Lore client --config was specified more than once")
			}
			return configSelection{Path: path, Explicit: true}, nil
		}
	}
	return inherited, nil
}

func extractGlobalConfig(args []string) ([]string, configSelection, error) {
	if len(args) == 0 {
		return args, configSelection{}, nil
	}
	if args[0] == "--config" {
		if len(args) < 2 {
			return nil, configSelection{}, fmt.Errorf("--config requires a path")
		}
		if args[1] == "" {
			return nil, configSelection{}, fmt.Errorf("--config path must not be empty")
		}
		return args[2:], configSelection{Path: args[1], Explicit: true}, nil
	}
	if strings.HasPrefix(args[0], "--config=") {
		path := strings.TrimPrefix(args[0], "--config=")
		if path == "" {
			return nil, configSelection{}, fmt.Errorf("--config path must not be empty")
		}
		return args[1:], configSelection{Path: path, Explicit: true}, nil
	}
	return args, configSelection{}, nil
}

func (config clientConfig) missingCredential(name, flagName, environmentName, fileKey string) error {
	looked := []string{flagName, environmentName}
	for _, lookup := range config.Lookups {
		status := "missing"
		if lookup.Loaded {
			status = "loaded"
		}
		looked = append(looked, fmt.Sprintf("%s in %s (%s)", fileKey, lookup.Path, status))
	}
	return fmt.Errorf("%s is required; looked in %s", name, strings.Join(looked, ", "))
}

func (r *Runner) showConfig(args []string, inherited configSelection) error {
	selection, err := selectionFromArgs(args, inherited)
	if err != nil {
		return err
	}
	resolved, err := resolveClientConfig(selection)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("config", flag.ContinueOnError)
	flags.SetOutput(r.ErrOut)
	server := flags.String("server", resolved.ServerURL.Value, "Lore server base URL")
	ingestToken := flags.String("ingest-token", resolved.IngestToken.Value, "Lore ingest token")
	adminToken := flags.String("admin-token", resolved.AdminToken.Value, "Lore admin token")
	_ = flags.String("config", selection.Path, "Lore client credential configuration YAML")
	if err := flags.Parse(args); err != nil {
		return err
	}
	flags.Visit(func(flagValue *flag.Flag) {
		switch flagValue.Name {
		case "server":
			resolved.ServerURL = resolvedValue{Value: *server, Source: "flag --server"}
		case "ingest-token":
			resolved.IngestToken = resolvedValue{Value: *ingestToken, Source: "flag --ingest-token"}
		case "admin-token":
			resolved.AdminToken = resolvedValue{Value: *adminToken, Source: "flag --admin-token"}
		}
	})
	if len(resolved.Lookups) == 0 {
		_, _ = fmt.Fprintln(r.Out, "config-files: none")
	} else {
		_, _ = fmt.Fprintln(r.Out, "config-files:")
		for _, lookup := range resolved.Lookups {
			status := "missing"
			if lookup.Loaded {
				status = "loaded"
			}
			_, _ = fmt.Fprintf(r.Out, "  - %s (%s)\n", lookup.Path, status)
		}
	}
	_, _ = fmt.Fprintf(r.Out, "server: %s (%s)\n", valueOrUnset(resolved.ServerURL.Value), sourceOrUnset(resolved.ServerURL.Source))
	_, _ = fmt.Fprintf(r.Out, "ingest-token: %s (%s)\n", redacted(resolved.IngestToken.Value), sourceOrUnset(resolved.IngestToken.Source))
	_, err = fmt.Fprintf(r.Out, "admin-token: %s (%s)\n", redacted(resolved.AdminToken.Value), sourceOrUnset(resolved.AdminToken.Source))
	return err
}

func redacted(value string) string {
	if value == "" {
		return "<unset>"
	}
	return "<redacted>"
}

func valueOrUnset(value string) string {
	if value == "" {
		return "<unset>"
	}
	return value
}

func sourceOrUnset(source string) string {
	if source == "" {
		return "unset"
	}
	return source
}
