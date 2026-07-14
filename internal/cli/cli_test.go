package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/wyrd-company/lore/internal/briefings"
)

func TestBriefingResourceCommands(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	runner := New(&output, &output)
	if err := runner.Run(context.Background(), []string{"briefings", "show-css"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), briefings.SiteCSS()) {
		t.Fatal("show-css did not emit the embedded stylesheet exactly")
	}
	output.Reset()
	if err := runner.Run(context.Background(), []string{"briefings", "contract", "--format", "json"}); err != nil {
		t.Fatal(err)
	}
	var contract briefings.Contract
	if err := json.Unmarshal(output.Bytes(), &contract); err != nil {
		t.Fatal(err)
	}
	if contract.ContainerClass != "lore-prose" || contract.StylesheetSHA256 == "" {
		t.Fatalf("unexpected contract: %#v", contract)
	}
}

func TestSingularBriefingAlias(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := New(&output, &output).Run(context.Background(), []string{"briefing", "show-css"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), briefings.SiteCSS()) {
		t.Fatal("briefing alias did not emit the embedded stylesheet exactly")
	}
}
