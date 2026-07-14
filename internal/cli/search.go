package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wyrd-company/lore/internal/client"
	"github.com/wyrd-company/lore/internal/retrieval"
)

func (r *Runner) search(ctx context.Context, args []string, inherited configSelection) error {
	if helpRequested(args) {
		r.commandUsage(r.Out, "search")
		return nil
	}
	selection, err := selectionFromArgs(args, inherited)
	if err != nil {
		return err
	}
	resolved, err := resolveClientConfig(selection)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("search", flag.ContinueOnError)
	flags.SetOutput(r.ErrOut)
	project := flags.String("project", os.Getenv("LORE_PROJECT"), "Lore project slug")
	server := flags.String("server", resolved.ServerURL.Value, "Lore server base URL")
	_ = flags.String("config", selection.Path, "Lore client credential configuration YAML")
	createdFrom := flags.String("created-from", "", "RFC3339 inclusive lower bound")
	createdTo := flags.String("created-to", "", "RFC3339 inclusive upper bound")
	limit := flags.Int("limit", 20, "maximum result documents")
	var sourceTypes, repositories, branches, tags stringListFlag
	flags.Var(&sourceTypes, "source-type", "source type filter; repeatable or comma-separated")
	flags.Var(&repositories, "repository", "repository filter; repeatable or comma-separated")
	flags.Var(&branches, "branch", "branch filter; repeatable or comma-separated")
	flags.Var(&tags, "tag", "normalized tag filter; repeatable or comma-separated")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *project == "" {
		return fmt.Errorf("--project or LORE_PROJECT is required")
	}
	query := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if query == "" {
		return fmt.Errorf("search query is required")
	}
	if *limit <= 0 {
		return fmt.Errorf("--limit must be a positive integer")
	}
	from, err := parseSearchTime("--created-from", *createdFrom)
	if err != nil {
		return err
	}
	to, err := parseSearchTime("--created-to", *createdTo)
	if err != nil {
		return err
	}
	api, err := client.NewViewer(*server)
	if err != nil {
		return err
	}
	response, err := api.Search(ctx, *project, retrieval.Request{
		Query: query,
		Filters: retrieval.Filters{
			SourceTypes: sourceTypes, Repositories: repositories, Branches: branches, Tags: tags,
			CreatedFrom: from, CreatedTo: to,
		},
		Limit: *limit,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(r.Out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(response); err != nil {
		return fmt.Errorf("write search response: %w", err)
	}
	return nil
}

type stringListFlag []string

func (values *stringListFlag) String() string {
	return strings.Join(*values, ",")
}

func (values *stringListFlag) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			*values = append(*values, item)
		}
	}
	return nil
}

func parseSearchTime(name, value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("%s must be RFC3339", name)
	}
	return &parsed, nil
}
