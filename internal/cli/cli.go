package cli

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/wyrd-company/lore/internal/projects"
	"github.com/wyrd-company/lore/internal/synchronization"
	"github.com/wyrd-company/lore/internal/version"
	"github.com/wyrd-company/lore/internal/watcher"
)

type Runner struct {
	Out    io.Writer
	ErrOut io.Writer
}

type reportedError struct {
	err error
}

func (e reportedError) Error() string {
	return e.err.Error()
}

func (e reportedError) Unwrap() error {
	return e.err
}

func IsReportedError(err error) bool {
	var reported reportedError
	return errors.As(err, &reported)
}

func New(out, errOut io.Writer) *Runner {
	return &Runner{Out: out, ErrOut: errOut}
}

func (r *Runner) Run(ctx context.Context, args []string) error {
	var selection configSelection
	var err error
	args, selection, err = extractGlobalConfig(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		r.usage(r.Out)
		return nil
	}
	if isHelp(args[0]) || args[0] == "help" {
		r.usage(r.Out)
		return nil
	}
	switch args[0] {
	case "version", "--version", "-version":
		if helpRequested(args[1:]) {
			r.commandUsage(r.Out, "version")
			return nil
		}
		_, err := fmt.Fprintf(r.Out, "lore %s\n", version.Value)
		return err
	case "migrate":
		if helpRequested(args[1:]) {
			r.commandUsage(r.Out, "migrate")
			return nil
		}
		cfg := config.FromEnvironment()
		if err := cfg.ValidateDatabase(); err != nil {
			return err
		}
		return database.Migrate(ctx, cfg.DatabaseURL)
	case "upload", "sync":
		return r.upload(ctx, args[1:], selection)
	case "watch":
		return r.watch(ctx, args[1:], selection)
	case "annotations", "annotation":
		return r.annotations(ctx, args[1:], selection)
	case "projects", "project":
		return r.projects(ctx, args[1:], selection)
	case "config":
		if helpRequested(args[1:]) {
			r.commandUsage(r.Out, "config")
			return nil
		}
		return r.showConfig(args[1:], selection)
	case "briefings", "briefing":
		return r.briefings(args[1:])
	case "search":
		return r.search(ctx, args[1:], selection)
	default:
		return r.usageError(fmt.Errorf("unknown command %q", args[0]), "")
	}
}

func (r *Runner) projects(ctx context.Context, args []string, inherited configSelection) error {
	if len(args) == 0 {
		return r.usageError(fmt.Errorf("projects command is required"), "projects")
	}
	if isHelp(args[0]) {
		r.commandUsage(r.Out, "projects")
		return nil
	}
	if args[0] != "create" {
		return r.usageError(fmt.Errorf("unknown projects command %q", args[0]), "projects")
	}
	if helpRequested(args[1:]) {
		r.commandUsage(r.Out, "projects create")
		return nil
	}
	selection, err := selectionFromArgs(args[1:], inherited)
	if err != nil {
		return err
	}
	resolved, err := resolveClientConfig(selection)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("projects create", flag.ContinueOnError)
	flags.SetOutput(r.ErrOut)
	slug := flags.String("slug", "", "project slug")
	name := flags.String("name", "", "project display name")
	server := flags.String("server", resolved.ServerURL.Value, "Lore server base URL")
	token := flags.String("token", resolved.AdminToken.Value, "Lore admin token")
	_ = flags.String("config", selection.Path, "Lore client credential configuration YAML")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *token == "" {
		return resolved.missingCredential("Lore admin token", "--token", "LORE_ADMIN_TOKEN", "admin-token")
	}
	api, err := client.New(*server, *token)
	if err != nil {
		return err
	}
	project, err := api.CreateProject(ctx, projects.CreateRequest{Slug: *slug, Name: *name})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(r.Out, "%s: %s\n", project.Slug, project.Name)
	return err
}

func (r *Runner) upload(ctx context.Context, args []string, inherited configSelection) error {
	if len(args) == 0 {
		return r.usageError(fmt.Errorf("upload source type is required"), "upload")
	}
	if isHelp(args[0]) {
		r.commandUsage(r.Out, "upload")
		return nil
	}
	adapter := args[0]
	if !validUploadAdapter(adapter) {
		return r.usageError(fmt.Errorf("unknown upload source type %q", adapter), "upload")
	}
	if helpRequested(args[1:]) {
		r.commandUsage(r.Out, "upload "+adapter)
		return nil
	}
	selection, err := selectionFromArgs(args[1:], inherited)
	if err != nil {
		return err
	}
	resolved, err := resolveClientConfig(selection)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("upload "+adapter, flag.ContinueOnError)
	flags.SetOutput(r.ErrOut)
	project := flags.String("project", os.Getenv("LORE_PROJECT"), "Lore project slug")
	sourceInstance := flags.String("source-instance", "", "stable source instance name")
	server := flags.String("server", resolved.ServerURL.Value, "Lore server base URL")
	token := flags.String("token", resolved.IngestToken.Value, "Lore ingest token")
	_ = flags.String("config", selection.Path, "Lore client credential configuration YAML")
	complete := flags.Bool("complete", false, "treat the scan as the complete source projection")
	title := flags.String("title", "", "briefing title override")
	repository := flags.String("repository", "", "repository identity override")
	branch := flags.String("branch", "", "repository branch override")
	provider := flags.String("provider", "", "conversation provider (claude or codex)")
	mapping := flags.String("mapping", "", "conversation project mapping YAML")
	fallback := flags.String("fallback-project", os.Getenv("LORE_PROJECT"), "opt-in conversation project fallback")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	paths := flags.Args()
	if len(paths) == 0 {
		return fmt.Errorf("upload %s requires a source path", adapter)
	}
	if adapter != "conversations" && *project == "" {
		return fmt.Errorf("--project or LORE_PROJECT is required")
	}
	if *token == "" {
		return resolved.missingCredential("Lore ingest token", "--token", "LORE_INGEST_TOKEN", "ingest-token")
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
	manifests, skipped, warnings, err := source.Build(boundary)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(r.ErrOut, "warning: %s\n", warning)
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
		return r.usageError(fmt.Errorf("briefings command is required"), "briefings")
	}
	if isHelp(args[0]) {
		r.commandUsage(r.Out, "briefings")
		return nil
	}
	if helpRequested(args[1:]) {
		r.commandUsage(r.Out, "briefings "+args[0])
		return nil
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
		return r.usageError(fmt.Errorf("unknown briefings command %q", args[0]), "briefings")
	}
}

func (r *Runner) annotations(ctx context.Context, args []string, inherited configSelection) error {
	if len(args) == 0 {
		return r.usageError(fmt.Errorf("annotations command is required"), "annotations")
	}
	if isHelp(args[0]) {
		r.commandUsage(r.Out, "annotations")
		return nil
	}
	if args[0] != "export" {
		return r.usageError(fmt.Errorf("unknown annotations command %q", args[0]), "annotations")
	}
	if helpRequested(args[1:]) {
		r.commandUsage(r.Out, "annotations export")
		return nil
	}
	selection, err := selectionFromArgs(args[1:], inherited)
	if err != nil {
		return err
	}
	resolved, err := resolveClientConfig(selection)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("annotations export", flag.ContinueOnError)
	flags.SetOutput(r.ErrOut)
	project := flags.String("project", os.Getenv("LORE_PROJECT"), "exactly one Lore project slug")
	server := flags.String("server", resolved.ServerURL.Value, "Lore server base URL")
	_ = flags.String("config", selection.Path, "Lore client credential configuration YAML")
	after := flags.Int64("after", 0, "incremental update cursor; zero exports a complete snapshot")
	output := flags.String("output", "-", "output path, or - for standard output")
	format := flags.String("format", "json", "export format")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *project == "" {
		return fmt.Errorf("--project or LORE_PROJECT is required")
	}
	if *after < 0 {
		return fmt.Errorf("--after must be non-negative")
	}
	if *format != "json" {
		return fmt.Errorf("unsupported annotation export format %q", *format)
	}
	api, err := client.NewViewer(*server)
	if err != nil {
		return err
	}
	export, err := api.ExportAnnotations(ctx, *project, *after)
	if err != nil {
		return err
	}
	writer := r.Out
	var file *os.File
	if *output != "-" {
		file, err = os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return fmt.Errorf("open annotation export: %w", err)
		}
		defer file.Close()
		writer = file
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(export); err != nil {
		return fmt.Errorf("write annotation export: %w", err)
	}
	return nil
}

func (r *Runner) watch(ctx context.Context, args []string, selection configSelection) error {
	if helpRequested(args) {
		r.commandUsage(r.Out, "watch")
		return nil
	}
	resolved, err := resolveClientConfig(selection)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("watch", flag.ContinueOnError)
	flags.SetOutput(r.ErrOut)
	configPath := flags.String("config", "lore-watch.yml", "watch configuration YAML")
	server := flags.String("server", resolved.ServerURL.Value, "Lore server base URL")
	token := flags.String("token", resolved.IngestToken.Value, "Lore ingest token")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *token == "" {
		return resolved.missingCredential("Lore ingest token", "--token", "LORE_INGEST_TOKEN", "ingest-token")
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

func (r *Runner) usage(writer io.Writer) {
	r.commandUsage(writer, "")
}

func (r *Runner) commandUsage(writer io.Writer, command string) {
	usage := map[string]string{
		"": `Lore command-line client

Usage:
  lore [--config <credentials.yml>] <command> [flags]

Commands:
  config       Show resolved client configuration
  projects     Create projects (alias: project)
  upload       Synchronize local source material
  watch        Watch and synchronize configured sources
  annotations  Export annotations (alias: annotation)
  briefings    Read or write briefing resources (alias: briefing)
  search       Search project knowledge
  migrate      Apply database migrations
  version      Print the Lore version
  help         Show this usage`,
		"config": `Usage:
  lore [--config <credentials.yml>] config [--server <url>] [--ingest-token <token>] [--admin-token <token>]`,
		"projects": `Usage:
  lore projects create --slug <slug> --name <name> [--server <url>] [--token <token>]`,
		"projects create": `Usage:
  lore projects create --slug <slug> --name <name> [--server <url>] [--token <token>]`,
		"upload": `Usage:
  lore upload <tasks|notes|briefing|repository|conversations> [flags] <path...>`,
		"annotations": `Usage:
  lore annotations export --project <project> [--after <cursor>] [--output <path>]`,
		"annotations export": `Usage:
  lore annotations export --project <project> [--after <cursor>] [--output <path>]`,
		"briefings": `Usage:
  lore briefings <show-css|show-skill|write-css|write-skill|contract>`,
		"search": `Usage:
  lore search --project <project> [filters] <query...>

Filters:
  --source-type <type>   Repeatable or comma-separated source types
  --repository <name>   Repeatable or comma-separated repositories
  --branch <name>       Repeatable or comma-separated branches
  --tag <tag>           Repeatable or comma-separated normalized tags
  --created-from <time> RFC3339 inclusive lower bound
  --created-to <time>   RFC3339 inclusive upper bound
  --limit <count>       Maximum documents (default 20)`,
		"watch": `Usage:
  lore [--config <credentials.yml>] watch --config <watch.yml> [--server <url>] [--token <token>]`,
		"migrate": `Usage:
  lore migrate`,
		"version": `Usage:
  lore version`,
	}
	if strings.HasPrefix(command, "upload ") {
		usage[command] = "Usage:\n  lore " + command + " [flags] <path...>"
	}
	if strings.HasPrefix(command, "briefings ") {
		usage[command] = "Usage:\n  lore " + command + " [flags]"
	}
	text, ok := usage[command]
	if !ok {
		text = usage[""]
	}
	_, _ = io.WriteString(writer, strings.TrimSpace(text)+"\n")
}

func (r *Runner) usageError(err error, command string) error {
	_, _ = fmt.Fprintf(r.ErrOut, "error: %v\n\n", err)
	r.commandUsage(r.ErrOut, command)
	return reportedError{err: err}
}

func isHelp(argument string) bool {
	return argument == "--help" || argument == "-h"
}

func helpRequested(args []string) bool {
	for _, argument := range args {
		if isHelp(argument) {
			return true
		}
	}
	return false
}

func validUploadAdapter(adapter string) bool {
	switch adapter {
	case "tasks", "notes", "briefing", "repository", "conversations":
		return true
	default:
		return false
	}
}
