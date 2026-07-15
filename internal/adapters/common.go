package adapters

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/wyrd-company/lore/internal/rendering"
	"github.com/wyrd-company/lore/internal/synchronization"
)

type Options struct {
	Project        string
	SourceInstance string
	Boundary       synchronization.Boundary
}

type WatchOptions struct {
	SkipPaths map[string]struct{}
}

func (options WatchOptions) ShouldSkip(path string) bool {
	_, skipped := options.SkipPaths[canonicalPath(path)]
	return skipped
}

func recordParseFailure(manifest *synchronization.Manifest, path string, err error) {
	manifest.Failures = append(manifest.Failures, synchronization.ParseFailure{Path: canonicalPath(path), Message: err.Error()})
}

func canonicalPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err == nil {
		return filepath.Clean(absolute)
	}
	return filepath.Clean(path)
}

func newManifest(options Options, sourceType string) synchronization.Manifest {
	boundary := options.Boundary
	if boundary == "" {
		boundary = synchronization.BoundaryComplete
	}
	return synchronization.Manifest{
		Project: options.Project, SourceInstance: options.SourceInstance, SourceType: sourceType, Boundary: boundary,
	}
}

func makeDocument(identity, title string, source []byte, renderer string, rendered rendering.Result, metadata, provenance any, tags []string) (synchronization.Document, error) {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return synchronization.Document{}, fmt.Errorf("encode metadata: %w", err)
	}
	provenanceJSON, err := json.Marshal(provenance)
	if err != nil {
		return synchronization.Document{}, fmt.Errorf("encode provenance: %w", err)
	}
	tags = normalizeTaxonomyValues(tags)
	hash := sha256.Sum256(source)
	return synchronization.Document{
		Identity: identity, Title: title, ContentHash: hex.EncodeToString(hash[:]), NormalizedText: rendered.Text,
		RenderedContent: rendered.HTML, Renderer: renderer, Metadata: metadataJSON, Provenance: provenanceJSON, Tags: tags,
	}, nil
}

func normalizeTaxonomyValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := normalizeTaxonomyValue(value); normalized != "" {
			result = append(result, normalized)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func normalizeTaxonomyValue(value string) string {
	var normalized strings.Builder
	separator := false
	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case unicode.IsSpace(character) || character == '_' || character == '-':
			separator = normalized.Len() > 0
		default:
			if separator {
				normalized.WriteByte('-')
				separator = false
			}
			normalized.WriteRune(character)
		}
	}
	return strings.Trim(normalized.String(), "-")
}

func titleFromPath(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	if name == "" {
		return "Untitled"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	case string:
		return []string{typed}
	default:
		return nil
	}
}
