package synchronization

import (
	"strings"
	"testing"
)

func TestManifestValidate(t *testing.T) {
	valid := Manifest{
		Project: "lore", SourceInstance: "notes", SourceType: "note", Boundary: BoundaryComplete,
		Documents: []Document{{Identity: "one", ContentHash: strings.Repeat("a", 64), Renderer: "markdown"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"missing project", func(m *Manifest) { m.Project = "" }},
		{"unknown source", func(m *Manifest) { m.SourceType = "other" }},
		{"invalid boundary", func(m *Manifest) { m.Boundary = "all" }},
		{"invalid hash", func(m *Manifest) { m.Documents[0].ContentHash = "nope" }},
		{"duplicate identity", func(m *Manifest) { m.Documents = append(m.Documents, m.Documents[0]) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			candidate.Documents = append([]Document(nil), valid.Documents...)
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
