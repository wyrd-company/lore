package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wyrd-company/lore/internal/rendering"
	"github.com/wyrd-company/lore/internal/synchronization"
)

func Notes(directory string, options Options) (synchronization.Manifest, error) {
	return notes(directory, options, nil)
}

func WatchNotes(directory string, options Options, watch WatchOptions) (synchronization.Manifest, error) {
	return notes(directory, options, &watch)
}

func notes(directory string, options Options, watch *WatchOptions) (synchronization.Manifest, error) {
	manifest := newManifest(options, "note")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return manifest, fmt.Errorf("read notes directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		if watch != nil && watch.ShouldSkip(path) {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return manifest, fmt.Errorf("read note %q: %w", path, err)
		}
		rendered, err := rendering.Markdown(source)
		if err != nil {
			if watch != nil {
				recordParseFailure(&manifest, path, err)
				continue
			}
			return manifest, fmt.Errorf("render note %q: %w", path, err)
		}
		identity := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		title, _ := rendered.FrontMatter["title"].(string)
		if title == "" && len(rendered.Headings) > 0 {
			title = rendered.Headings[0].Text
		}
		if title == "" {
			title = titleFromPath(path)
		}
		tags := stringSlice(rendered.FrontMatter["tags"])
		document, err := makeDocument(identity, title, source, "markdown", rendered, rendered.FrontMatter, map[string]any{
			"path": path, "filename": entry.Name(),
		}, tags)
		if err != nil {
			if watch != nil {
				recordParseFailure(&manifest, path, err)
				continue
			}
			return manifest, err
		}
		document.Terms = normalizeTaxonomyValues(stringSlice(rendered.FrontMatter["terms"]))
		manifest.Documents = append(manifest.Documents, document)
		for _, related := range relatedNoteIdentities(rendered.FrontMatter["relatedTo"]) {
			if related != identity {
				manifest.Relationships = append(manifest.Relationships, synchronization.Relationship{
					SourceIdentity: identity, TargetIdentity: related, Type: "note-related-to",
				})
			}
		}
	}
	return manifest, manifest.Validate()
}

func relatedNoteIdentities(value any) []string {
	var result []string
	for _, item := range anySlice(value) {
		switch typed := item.(type) {
		case string:
			result = append(result, strings.TrimSpace(typed))
		case map[string]any:
			if id, ok := typed["id"].(string); ok {
				result = append(result, strings.TrimSpace(id))
			}
		}
	}
	sort.Strings(result)
	return compactStrings(result)
}

func anySlice(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	return nil
}

func compactStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if value != "" && (len(result) == 0 || result[len(result)-1] != value) {
			result = append(result, value)
		}
	}
	return result
}
