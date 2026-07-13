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
	Metadata       json.RawMessage `json:"metadata,omitempty"`
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
	Chunks          []Chunk         `json:"chunks,omitempty"`
}

type Chunk struct {
	Ordinal            int             `json:"ordinal"`
	NormalizedText     string          `json:"normalizedText"`
	StructuralLocation json.RawMessage `json:"structuralLocation,omitempty"`
	TokenCount         *int            `json:"tokenCount,omitempty"`
}

type Result struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Deleted   int `json:"deleted"`
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
		for j, chunk := range document.Chunks {
			if chunk.Ordinal < 0 {
				return fmt.Errorf("documents[%d].chunks[%d].ordinal must not be negative", i, j)
			}
			if chunk.TokenCount != nil && *chunk.TokenCount < 0 {
				return fmt.Errorf("documents[%d].chunks[%d].tokenCount must not be negative", i, j)
			}
		}
	}
	return nil
}

func jsonOrEmpty(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}
