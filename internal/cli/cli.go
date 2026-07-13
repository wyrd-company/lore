package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wyrd-company/lore/internal/briefings"
	"github.com/wyrd-company/lore/internal/client"
	"github.com/wyrd-company/lore/internal/config"
	"github.com/wyrd-company/lore/internal/database"
	"github.com/wyrd-company/lore/internal/ingest"
	"github.com/wyrd-company/lore/internal/synchronization"
	"github.com/wyrd-company/lore/internal/version"
	"github.com/wyrd-company/lore/internal/watcher"
)

type Runner struct {
	Out    io.Writer
	ErrOut io.Writer
}

func New(out, errOut io.Writer) *Runner {
	return &Runner{Out: out, ErrOut: errOut}
}

func (r *Runner) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		r.usage()
		return nil
	}
	switch args[0] {
	case "version", "--version", "-version":
		_, err := fmt.Fprintf(r.Out, "lore %s\n", version.Value)
		return err
	case "migrate":
		cfg := config.FromEnvironment()
		if err := cfg.ValidateDatabase(); err != nil {
			return err
		}
		return database.Migrate(ctx, cfg.DatabaseURL)
	case "upload", "sync":
		return r.upload(ctx, args[1:])
	case "watch":
		return r.watch(ctx, args[1:])
	case "annotations":
		return r.annotations(args[1:])
	case "briefings":
		return r.briefings(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func (r *Runner) upload(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("upload source type is required (tasks, notes, briefing, repository, or conversations)")
	}
	adapter := args[0]
	flags := flag.NewFlagSet("upload "+adapter, flag.ContinueOnError)
	flags.SetOutput(r.ErrOut)
	project := flags.String("project", os.Getenv("PROJECT"), "Lore project slug")
	sourceInstance := flags.String("source-instance", "", "stable source instance name")
	server := flags.String("server", serverFromEnvironment(), "Lore server base URL")
	token := flags.String("token", os.Getenv("LORE_INGEST_TOKEN"), "Lore ingest token")
	complete := flags.Bool("complete", false, "treat the scan as the complete source projection")
	title := flags.String("title", "", "briefing title override")
	repository := flags.String("repository", "", "repository identity override")
	branch := flags.String("branch", "", "repository branch override")
	provider := flags.String("provider", "", "conversation provider (claude or codex)")
	mapping := flags.String("mapping", "", "conversation project mapping YAML")
	fallback := flags.String("fallback-project", os.Getenv("PROJECT"), "opt-in conversation project fallback")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	paths := flags.Args()
	if len(paths) == 0 {
		return fmt.Errorf("upload %s requires a source path", adapter)
	}
	if adapter != "conversations" && *project == "" {
		return fmt.Errorf("--project or PROJECT is required")
	}
	boundary := synchronization.BoundaryPartial
	if *complete {
		boundary = synchronization.BoundaryComplete
	}
	source := ingest.Source{
		Project: *project, SourceInstance: *sourceInstance, Adapter: adapter, Path: paths[0], Paths: paths,
		Title: *title, Repository: *repository, Branch: *branch, Provider: *provider,
		Mapping: *mapping, FallbackProject: *fallback,
	}
	if adapter != "repository" && len(paths) != 1 {
		return fmt.Errorf("upload %s accepts exactly one source path", adapter)
	}
	manifests, skipped, err := source.Build(boundary)
	if err != nil {
		return err
	}
	if len(manifests) == 0 {
		_, _ = fmt.Fprintf(r.Out, "No assigned documents to upload (skipped sessions: %d).\n", skipped)
		return nil
	}
	api, err := client.New(*server, *token)
	if err != nil {
		return err
	}
	for _, manifest := range manifests {
		result, err := api.Synchronize(ctx, manifest)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(r.Out, "%s/%s: %d created, %d updated, %d unchanged, %d deleted\n",
			manifest.Project, manifest.SourceInstance, result.Created, result.Updated, result.Unchanged, result.Deleted)
	}
	if skipped > 0 {
		_, _ = fmt.Fprintf(r.Out, "Skipped %d unassigned conversation session(s).\n", skipped)
	}
	return nil
}

func (r *Runner) briefings(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("briefings command is required")
	}
	switch args[0] {
	case "show-css":
		_, err := r.Out.Write(briefings.SiteCSS())
		return err
	case "show-skill":
		_, err := r.Out.Write(briefings.AuthoringSkill)
		return err
	case "write-css", "write-skill":
		if len(args) != 2 {
			return fmt.Errorf("usage: lore briefings %s <path>", args[0])
		}
		contents := briefings.SiteCSS()
		if args[0] == "write-skill" {
			contents = briefings.AuthoringSkill
		}
		return briefings.WriteFile(args[1], contents)
	case "contract":
		flags := flag.NewFlagSet("briefings contract", flag.ContinueOnError)
		flags.SetOutput(r.ErrOut)
		format := flags.String("format", "json", "output format")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *format != "json" {
			return fmt.Errorf("unsupported contract format %q", *format)
		}
		return briefings.WriteContract(r.Out)
	default:
		return fmt.Errorf("unknown briefings command %q", args[0])
	}
}

func (r *Runner) annotations(args []string) error {
	if len(args) == 1 && args[0] == "export" {
		return fmt.Errorf("annotation export is planned for milestone 4")
	}
	return fmt.Errorf("usage: lore annotations export")
}

func (r *Runner) watch(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("watch", flag.ContinueOnError)
	flags.SetOutput(r.ErrOut)
	configPath := flags.String("config", "lore-watch.yml", "watch configuration YAML")
	server := flags.String("server", serverFromEnvironment(), "Lore server base URL")
	token := flags.String("token", os.Getenv("LORE_INGEST_TOKEN"), "Lore ingest token")
	if err := flags.Parse(args); err != nil {
		return err
	}
	watchConfig, err := watcher.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	api, err := client.New(*server, *token)
	if err != nil {
		return err
	}
	return watcher.New(watchConfig, api, r.Out).Run(ctx)
}

func (r *Runner) usage() {
	_, _ = io.WriteString(r.Out, strings.TrimSpace(`Lore command-line client
usage:
  lore upload <tasks|notes|briefing|repository|conversations> [flags] <path...>
  lore watch --config <path>
  lore annotations export
  lore briefings <show-css|show-skill|write-css|write-skill|contract>
  lore migrate
  lore version`)+"\n")
}

func serverFromEnvironment() string {
	if value := os.Getenv("LORE_SERVER_URL"); value != "" {
		return value
	}
	if value := os.Getenv("PUBLIC_BASE_URL"); value != "" {
		return value
	}
	return "http://localhost:8080"
}
