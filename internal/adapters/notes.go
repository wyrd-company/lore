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
		source, err := os.ReadFile(path)
		if err != nil {
			return manifest, fmt.Errorf("read note %q: %w", path, err)
		}
		rendered, err := rendering.Markdown(source)
		if err != nil {
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
			return manifest, err
		}
		manifest.Documents = append(manifest.Documents, document)
	}
	return manifest, manifest.Validate()
}
