package adapters

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wyrd-company/lore/internal/rendering"
	"github.com/wyrd-company/lore/internal/synchronization"
)

type Options struct {
	Project        string
	SourceInstance string
	Boundary       synchronization.Boundary
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
	tags = normalizeTags(tags)
	hash := sha256.Sum256(source)
	return synchronization.Document{
		Identity: identity, Title: title, ContentHash: hex.EncodeToString(hash[:]), NormalizedText: rendered.Text,
		RenderedContent: rendered.HTML, Renderer: renderer, Metadata: metadataJSON, Provenance: provenanceJSON, Tags: tags,
	}, nil
}

func normalizeTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		if normalized := strings.TrimSpace(tag); normalized != "" {
			result = append(result, normalized)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
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
