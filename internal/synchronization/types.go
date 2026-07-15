package synchronization

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

type Boundary string

const (
	BoundaryComplete Boundary = "complete"
	BoundaryPartial  Boundary = "partial"
)

type Manifest struct {
	Project        string          `json:"project"`
	SourceInstance string          `json:"sourceInstance"`
	SourceType     string          `json:"sourceType"`
	Boundary       Boundary        `json:"boundary"`
	Documents      []Document      `json:"documents"`
	Relationships  []Relationship  `json:"relationships,omitempty"`
	Failures       []ParseFailure  `json:"failures,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

type ParseFailure struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type Document struct {
	Identity        string          `json:"identity"`
	Title           string          `json:"title"`
	ContentHash     string          `json:"contentHash"`
	NormalizedText  string          `json:"normalizedText"`
	RenderedContent string          `json:"renderedContent"`
	Renderer        string          `json:"renderer"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
	Provenance      json.RawMessage `json:"provenance,omitempty"`
	Tags            []string        `json:"tags,omitempty"`
	Terms           []string        `json:"terms,omitempty"`
	DefinesTerm     string          `json:"definesTerm,omitempty"`
}

type Relationship struct {
	SourceIdentity string          `json:"sourceIdentity"`
	TargetIdentity string          `json:"targetIdentity"`
	Type           string          `json:"type"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

type Result struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Deleted   int `json:"deleted"`
	Failed    int `json:"failed"`
}

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var sourceTypes = map[string]struct{}{
	"task": {}, "note": {}, "briefing": {}, "repository": {}, "conversation": {},
}

func (m Manifest) Validate() error {
	if m.Project == "" {
		return errors.New("project is required")
	}
	if m.SourceInstance == "" {
		return errors.New("sourceInstance is required")
	}
	if _, ok := sourceTypes[m.SourceType]; !ok {
		return fmt.Errorf("unsupported sourceType %q", m.SourceType)
	}
	if m.Boundary != BoundaryComplete && m.Boundary != BoundaryPartial {
		return fmt.Errorf("boundary must be %q or %q", BoundaryComplete, BoundaryPartial)
	}
	if len(m.Metadata) > 0 && !json.Valid(m.Metadata) {
		return errors.New("metadata must be valid JSON")
	}
	if len(m.Relationships) > 0 && m.SourceType != "task" && m.SourceType != "note" {
		return errors.New("relationships are only supported for task and note manifests")
	}
	seen := make(map[string]struct{}, len(m.Documents))
	for i, document := range m.Documents {
		if document.Identity == "" {
			return fmt.Errorf("documents[%d].identity is required", i)
		}
		if _, exists := seen[document.Identity]; exists {
			return fmt.Errorf("duplicate document identity %q", document.Identity)
		}
		seen[document.Identity] = struct{}{}
		if !hashPattern.MatchString(document.ContentHash) {
			return fmt.Errorf("documents[%d].contentHash must be a lowercase SHA-256 hex digest", i)
		}
		if document.Renderer == "" {
			return fmt.Errorf("documents[%d].renderer is required", i)
		}
		if len(document.Metadata) > 0 && !json.Valid(document.Metadata) {
			return fmt.Errorf("documents[%d].metadata must be valid JSON", i)
		}
		if len(document.Provenance) > 0 && !json.Valid(document.Provenance) {
			return fmt.Errorf("documents[%d].provenance must be valid JSON", i)
		}
		tags := make(map[string]struct{}, len(document.Tags))
		for j, tag := range document.Tags {
			if tag == "" {
				return fmt.Errorf("documents[%d].tags[%d] must not be empty", i, j)
			}
			if _, exists := tags[tag]; exists {
				return fmt.Errorf("documents[%d] contains duplicate tag %q", i, tag)
			}
			tags[tag] = struct{}{}
		}
		terms := make(map[string]struct{}, len(document.Terms))
		for j, term := range document.Terms {
			if term == "" {
				return fmt.Errorf("documents[%d].terms[%d] must not be empty", i, j)
			}
			if _, exists := terms[term]; exists {
				return fmt.Errorf("documents[%d] contains duplicate term %q", i, term)
			}
			terms[term] = struct{}{}
		}
	}
	seenRelationships := make(map[string]struct{}, len(m.Relationships))
	for i, relationship := range m.Relationships {
		expectedType := "task-depends-on"
		if m.SourceType == "note" {
			expectedType = "note-related-to"
		}
		if relationship.Type != expectedType {
			return fmt.Errorf("relationships[%d] has unsupported type %q", i, relationship.Type)
		}
		if relationship.SourceIdentity == "" || relationship.TargetIdentity == "" {
			return fmt.Errorf("relationships[%d] requires sourceIdentity and targetIdentity", i)
		}
		if relationship.SourceIdentity == relationship.TargetIdentity {
			return fmt.Errorf("relationships[%d] cannot target its source", i)
		}
		if _, exists := seen[relationship.SourceIdentity]; !exists {
			return fmt.Errorf("relationship source %q is not present in the manifest", relationship.SourceIdentity)
		}
		key := relationship.SourceIdentity + "\x00" + relationship.Type + "\x00" + relationship.TargetIdentity
		if _, exists := seenRelationships[key]; exists {
			return fmt.Errorf("duplicate relationship %q -> %q", relationship.SourceIdentity, relationship.TargetIdentity)
		}
		seenRelationships[key] = struct{}{}
		if len(relationship.Metadata) > 0 && !json.Valid(relationship.Metadata) {
			return fmt.Errorf("relationships[%d].metadata must be valid JSON", i)
		}
	}
	seenFailures := make(map[string]struct{}, len(m.Failures))
	for i, failure := range m.Failures {
		if failure.Path == "" {
			return fmt.Errorf("failures[%d].path is required", i)
		}
		if failure.Message == "" {
			return fmt.Errorf("failures[%d].message is required", i)
		}
		if _, exists := seenFailures[failure.Path]; exists {
			return fmt.Errorf("duplicate failure path %q", failure.Path)
		}
		seenFailures[failure.Path] = struct{}{}
	}
	return nil
}

func jsonOrEmpty(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}
