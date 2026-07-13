package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProjectMappings struct {
	Sessions             map[string]string `yaml:"sessions" json:"sessions"`
	Paths                []PathMapping     `yaml:"paths" json:"paths"`
	Repositories         map[string]string `yaml:"repositories" json:"repositories"`
	AllowProjectFallback bool              `yaml:"allowProjectFallback" json:"allowProjectFallback"`
}

type PathMapping struct {
	Prefix  string `yaml:"prefix" json:"prefix"`
	Project string `yaml:"project" json:"project"`
}

type SessionEvidence struct {
	SessionID  string
	CWD        string
	Repository string
}

func LoadProjectMappings(path string) (ProjectMappings, error) {
	if path == "" {
		return ProjectMappings{}, nil
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return ProjectMappings{}, fmt.Errorf("read project mapping: %w", err)
	}
	var mappings ProjectMappings
	if err := yaml.Unmarshal(contents, &mappings); err != nil {
		return ProjectMappings{}, fmt.Errorf("parse project mapping: %w", err)
	}
	return mappings, nil
}

func (m ProjectMappings) Resolve(evidence SessionEvidence, fallback string) string {
	if project := m.Sessions[evidence.SessionID]; project != "" {
		return project
	}
	paths := append([]PathMapping(nil), m.Paths...)
	sort.SliceStable(paths, func(i, j int) bool {
		return len(filepath.Clean(paths[i].Prefix)) > len(filepath.Clean(paths[j].Prefix))
	})
	for _, mapping := range paths {
		if mapping.Project != "" && withinPath(evidence.CWD, mapping.Prefix) {
			return mapping.Project
		}
	}
	if evidence.Repository != "" {
		if project := m.Repositories[evidence.Repository]; project != "" {
			return project
		}
		if project := repositoryProject(evidence.Repository); project != "" {
			return project
		}
	}
	if m.AllowProjectFallback {
		return fallback
	}
	return ""
}

func (m ProjectMappings) ResolveClaudeProjectDirectory(sessionPath string) string {
	marker := string(filepath.Separator) + "projects" + string(filepath.Separator)
	index := strings.Index(sessionPath, marker)
	if index < 0 {
		return ""
	}
	directory := strings.SplitN(sessionPath[index+len(marker):], string(filepath.Separator), 2)[0]
	type candidate struct {
		encoded string
		project string
	}
	var candidates []candidate
	for _, mapping := range m.Paths {
		encoded := strings.ReplaceAll(filepath.Clean(mapping.Prefix), string(filepath.Separator), "-")
		candidates = append(candidates, candidate{encoded: encoded, project: mapping.Project})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return len(candidates[i].encoded) > len(candidates[j].encoded) })
	for _, mapping := range candidates {
		if mapping.project != "" && directory == mapping.encoded {
			return mapping.project
		}
	}
	return ""
}

// Projects returns the explicitly configured project universe. Complete
// conversation scans use it to emit empty manifests when a project's final
// session disappears from the source directory.
func (m ProjectMappings) Projects(fallback string) []string {
	seen := make(map[string]struct{})
	for _, project := range m.Sessions {
		if project != "" {
			seen[project] = struct{}{}
		}
	}
	for _, mapping := range m.Paths {
		if mapping.Project != "" {
			seen[mapping.Project] = struct{}{}
		}
	}
	for _, project := range m.Repositories {
		if project != "" {
			seen[project] = struct{}{}
		}
	}
	if m.AllowProjectFallback && fallback != "" {
		seen[fallback] = struct{}{}
	}
	projects := make([]string, 0, len(seen))
	for project := range seen {
		projects = append(projects, project)
	}
	sort.Strings(projects)
	return projects
}

func withinPath(path, prefix string) bool {
	if path == "" || prefix == "" {
		return false
	}
	path = filepath.Clean(path)
	prefix = filepath.Clean(prefix)
	relative, err := filepath.Rel(prefix, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func repositoryProject(repository string) string {
	repository = strings.TrimSuffix(strings.TrimSpace(repository), ".git")
	if separator := strings.LastIndexAny(repository, "/:"); separator >= 0 {
		repository = repository[separator+1:]
	}
	repository = strings.ToLower(repository)
	var slug strings.Builder
	lastDash := false
	for _, character := range repository {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			slug.WriteRune(character)
			lastDash = false
		} else if slug.Len() > 0 && !lastDash {
			slug.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(slug.String(), "-")
}
