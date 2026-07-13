package briefings

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	designassets "github.com/wyrd-company/lore/design"
)

//go:embed authoring-skill.md
var AuthoringSkill []byte

func SiteCSS() []byte {
	return append([]byte(nil), designassets.SiteCSS...)
}

type Contract struct {
	Version           string   `json:"version"`
	ContainerClass    string   `json:"containerClass"`
	StylesheetSHA256  string   `json:"stylesheetSha256"`
	Payload           string   `json:"payload"`
	Ignored           []string `json:"ignored"`
	Images            string   `json:"images"`
	Diagrams          string   `json:"diagrams"`
	StableTargets     []string `json:"stableTargets"`
	ExternalResources bool     `json:"externalResources"`
	AuthorStyles      bool     `json:"authorStyles"`
}

func CurrentContract() Contract {
	digest := sha256.Sum256(designassets.SiteCSS)
	return Contract{
		Version: "1", ContainerClass: "lore-prose", StylesheetSHA256: hex.EncodeToString(digest[:]),
		Payload: "HTML body content or a body fragment", Ignored: []string{"head content", "scripts", "external head resources"},
		Images: "data URLs", Diagrams: "inline SVG, including pre-rendered Mermaid",
		StableTargets: []string{"element ids", "heading ids"}, ExternalResources: false, AuthorStyles: false,
	}
}

func WriteContract(output io.Writer) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(CurrentContract())
}

func WriteFile(path string, contents []byte) error {
	if path == "" {
		return fmt.Errorf("output path is required")
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}
