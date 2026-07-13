package adapters

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wyrd-company/lore/internal/rendering"
	"github.com/wyrd-company/lore/internal/synchronization"
)

func Briefing(path, titleOverride string, options Options) (synchronization.Manifest, error) {
	manifest := newManifest(options, "briefing")
	source, err := os.ReadFile(path)
	if err != nil {
		return manifest, fmt.Errorf("read briefing: %w", err)
	}
	rendered, err := rendering.Briefing(source)
	if err != nil {
		return manifest, err
	}
	title := titleOverride
	if title == "" {
		title = titleFromPath(path)
	}
	identity := filepath.Base(path)
	metadata := map[string]any{
		"filename": identity, "headings": rendered.Headings, "elementIds": rendered.ElementIDs,
		"stylesheetContract": ".lore-prose",
	}
	document, err := makeDocument(identity, title, source, "briefing", rendered, metadata, map[string]any{
		"path": path, "filename": identity,
	}, nil)
	if err != nil {
		return manifest, err
	}
	manifest.Documents = []synchronization.Document{document}
	return manifest, manifest.Validate()
}
