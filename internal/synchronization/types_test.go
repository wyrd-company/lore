package synchronization

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestRetryablePostgresErrors(t *testing.T) {
	for _, code := range []string{"40001", "40P01"} {
		if !IsRetryable(fmt.Errorf("commit: %w", &pgconn.PgError{Code: code})) {
			t.Fatalf("PostgreSQL error %s should be retryable", code)
		}
	}
	if IsRetryable(&pgconn.PgError{Code: "23505"}) {
		t.Fatal("unique violation must not be classified as retryable")
	}
}

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
		{"duplicate term", func(m *Manifest) { m.Documents[0].Terms = []string{"term", "term"} }},
		{"missing failure path", func(m *Manifest) { m.Failures = []ParseFailure{{Message: "invalid YAML"}} }},
		{"missing failure message", func(m *Manifest) { m.Failures = []ParseFailure{{Path: "/sources/broken.md"}} }},
		{"duplicate failure path", func(m *Manifest) {
			m.Failures = []ParseFailure{{Path: "/sources/broken.md", Message: "invalid YAML"}, {Path: "/sources/broken.md", Message: "still invalid"}}
		}},
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
