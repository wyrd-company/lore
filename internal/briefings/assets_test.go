package briefings

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	designassets "github.com/wyrd-company/lore/design"
)

func TestEmbeddedBriefingResourcesMatchContract(t *testing.T) {
	if !bytes.Equal(SiteCSS(), designassets.SiteCSS) {
		t.Fatal("embedded stylesheet does not match design/site.css")
	}
	digest := sha256.Sum256(designassets.SiteCSS)
	if CurrentContract().StylesheetSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal("contract stylesheet identity is stale")
	}
	if !strings.Contains(string(AuthoringSkill), `.lore-prose`) || !strings.Contains(string(AuthoringSkill), "inline SVG") {
		t.Fatal("authoring skill omits briefing contract requirements")
	}
}
