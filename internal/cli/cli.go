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
	if isHelp(args[0]) {
		r.usage(r.Out)
		return nil
	}
	if args[0] == "help" {
		return r.help(args[1:])
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

func (r *Runner) help(args []string) error {
	if len(args) == 0 || helpRequested(args) {
		r.usage(r.Out)
		return nil
	}
	command := canonicalHelpCommand(strings.Join(args, " "))
	if _, ok := helpText[command]; !ok {
		return r.usageError(fmt.Errorf("unknown help topic %q", strings.Join(args, " ")), "")
	}
	r.commandUsage(r.Out, command)
	return nil
}

func (r *Runner) commandUsage(writer io.Writer, command string) {
	command = canonicalHelpCommand(command)
	text, ok := helpText[command]
	if !ok {
		text = helpText[""]
	}
	_, _ = io.WriteString(writer, strings.TrimSpace(text)+"\n")
}

func canonicalHelpCommand(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ""
	}
	switch parts[0] {
	case "project":
		parts[0] = "projects"
	case "annotation":
		parts[0] = "annotations"
	case "briefing":
		parts[0] = "briefings"
	case "sync":
		parts[0] = "upload"
	}
	return strings.Join(parts, " ")
}

var helpText = map[string]string{
	"": `Lore command-line client

Usage:
  lore [--config <credentials.yml>] <command> [flags]
  lore help <command> [subcommand]

Commands:
  config       Show resolved client configuration
  projects     Create projects (alias: project)
  upload       Synchronize local source material (alias: sync)
  watch        Watch and synchronize configured sources
  annotations  Export annotations (alias: annotation)
  briefings    Read or write briefing resources (alias: briefing)
  search       Search project knowledge
  migrate      Apply database migrations
  version      Print the Lore version
  help         Show help for a command

Project selection:
  Commands that operate on one project accept --project <slug> or LORE_PROJECT.
  The flag wins when both are set. The project is intentionally not stored in
  the credential file because it normally varies by workspace.

Client configuration:
  Resolution order is flags > environment > credential file > defaults.
  LORE_CONFIG selects a credential file. LORE_SERVER_URL selects the server;
  PUBLIC_BASE_URL is its fallback. LORE_INGEST_TOKEN and LORE_ADMIN_TOKEN supply
  credentials. Run 'lore config' to see resolved sources without exposing tokens.

Help:
  lore help <command> [subcommand]
  lore <command> [subcommand] --help`,

	"config": `Show the resolved client configuration and where each value came from.

Usage:
  lore [--config <credentials.yml>] config [flags]

Flags:
  --server <url>          Override the Lore server URL
  --ingest-token <token> Override the synchronization token
  --admin-token <token>  Override the project-administration token
  --config <path>        Select one credential YAML file

Credential YAML:
  server: https://lore.example.net
  ingest-token: replace-with-ingest-token
  admin-token: replace-with-admin-token

Resolution order:
  flags > environment > credential file > defaults

  Environment: LORE_SERVER_URL (then PUBLIC_BASE_URL), LORE_INGEST_TOKEN,
  LORE_ADMIN_TOKEN. Credential selection: --config, LORE_CONFIG,
  $XDG_CONFIG_HOME/lore/config.yml (or ~/.config/lore/config.yml), then
  /etc/lore/config.yml. A server without a scheme is treated as HTTP.

Tokens are always redacted in this command's output.

Examples:
  lore config
  lore --config ./credentials.yml config
  LORE_SERVER_URL=lore:8080 lore config`,

	"projects": `Create and administer Lore projects.

Usage:
  lore projects create [flags]

Aliases:
  lore project create

Run 'lore help projects create' for required credentials, flags, and examples.`,

	"projects create": `Create a project. Creation is idempotent by slug.

Usage:
  lore projects create --slug <slug> --name <name> [flags]

Flags:
  --slug <slug>      Stable URL-safe project identifier
  --name <name>      Human-readable project name
  --server <url>     Lore server; LORE_SERVER_URL or client configuration
  --token <token>    Admin token; LORE_ADMIN_TOKEN or config admin-token
  --config <path>    Client credential YAML

Example:
  lore projects create --slug refinery --name "Refinery"

Use 'lore config' to diagnose server and admin-token resolution.`,

	"upload": `Synchronize authoritative local source material into Lore.

Usage:
  lore upload <tasks|notes|briefing|repository|conversations> [flags] <path...>

Shared flags:
  --project <slug>          Project; defaults to LORE_PROJECT (not used to route conversations)
  --source-instance <name>  Required stable identity for this source projection
  --complete                Treat this scan as the authoritative projection
  --server <url>            Lore server; LORE_SERVER_URL or client configuration
  --token <token>           Ingest token; LORE_INGEST_TOKEN or config ingest-token
  --config <path>           Client credential YAML

Without --complete, an upload is partial and cannot delete sibling documents.
With --complete, documents missing from the same project and source instance are
marked deleted. Reuse the same --source-instance for subsequent scans.

Run 'lore help upload <type>' for source-specific flags and examples.`,

	"upload tasks": `Upload a kanban-md task directory.

Usage:
  lore upload tasks [shared flags] <directory>

Required:
  --source-instance <name>
  --project <slug>, or LORE_PROJECT
  --token <token>, LORE_INGEST_TOKEN, or config ingest-token

Use --complete when the directory is the authoritative projection for this
source instance; omitted tasks are then removed from Lore.

Example:
  LORE_PROJECT=refinery lore upload tasks --source-instance kanban --complete ./tasks`,

	"upload notes": `Upload a directory of Markdown notes.

Usage:
  lore upload notes [shared flags] <directory>

Required:
  --source-instance <name>
  --project <slug>, or LORE_PROJECT
  --token <token>, LORE_INGEST_TOKEN, or config ingest-token

Use --complete when the directory is the authoritative projection for this
source instance; omitted notes are then removed from Lore.

Example:
  LORE_PROJECT=refinery lore upload notes --source-instance mnemonic --complete /memory/.mnemonic/notes`,

	"upload briefing": `Upload one trusted HTML briefing body.

Usage:
  lore upload briefing [shared flags] [--title <title>] <file.html>

Flags:
  --title <title>  Override the title derived from the filename

Required:
  --source-instance <name>
  --project <slug>, or LORE_PROJECT
  --token <token>, LORE_INGEST_TOKEN, or config ingest-token

The body must follow the embedded briefing contract. Scripts and head resources
are ignored; diagrams, including Mermaid, must already be inline SVG.

Example:
  lore upload briefing --project refinery --source-instance primer --title "Architecture Primer" ./primer.html`,

	"upload repository": `Upload UTF-8 repository documents from one or more files or directories.

Usage:
  lore upload repository [shared flags] [repository flags] <path...>

Flags:
  --repository <identity>  Override the repository derived from Git
  --branch <name>          Override the branch derived from Git

Required:
  --source-instance <name>
  --project <slug>, or LORE_PROJECT
  --token <token>, LORE_INGEST_TOKEN, or config ingest-token

Repository uploads are the only upload type that accepts multiple source paths.

Example:
  lore upload repository --project lore --source-instance repository docs README.md`,

	"upload conversations": `Upload Claude or Codex JSONL sessions, routing each session to a project.

Usage:
  lore upload conversations [shared flags] --provider <claude|codex> [flags] <directory>

Flags:
  --provider <name>          Required session format: claude or codex
  --mapping <path>           YAML project-routing rules
  --fallback-project <slug>  Fallback project, defaulting to LORE_PROJECT

Mapping resolution checks exact session IDs, longest working-directory prefixes,
and repositories. The fallback is used only when the mapping file contains
allowProjectFallback: true. Otherwise unassigned sessions are skipped and counted.

Example mapping:
  paths:
    - prefix: /workspaces/tools/lore
      project: lore
  repositories:
    git@github.com:wyrd-company/lore.git: lore
  allowProjectFallback: false

Example:
  lore upload conversations --source-instance codex --provider codex --complete --mapping projects.yml ~/.codex/sessions`,

	"annotations": `Export project annotations for backup, processing, or incremental synchronization.

Usage:
  lore annotations export [flags]

Aliases:
  lore annotation export

Annotations are created and managed in the Lore web interface. The CLI currently
provides a lossless JSON export; it does not create, resolve, or import annotations.

Run 'lore help annotations export' for snapshot and cursor semantics.`,

	"annotations export": `Export exactly one project's annotations as lore.annotations/v1 JSON.

Usage:
  lore annotations export [--project <slug>] [--after <cursor>] [--output <path>]

Annotations are created and managed in the Lore web interface. This command is
read-only and does not require an ingest or admin token.

Flags:
  --project <slug>  Project to export; defaults to LORE_PROJECT
  --after <cursor>  Export events after this cursor; zero produces a complete snapshot
  --output <path>   Write to this file; - writes to standard output (default -)
  --format json     Export format; json is currently the only supported value
  --server <url>    Lore server; LORE_SERVER_URL or client configuration
  --config <path>   Client credential YAML

The JSON envelope includes nextCursor. Save it and pass it to --after for the
next incremental export. Output files are created with owner-only permissions.

Examples:
  LORE_PROJECT=refinery lore annotations export --output annotations.json
  lore annotations export --project refinery --after 12345 --output changes.json`,

	"briefings": `Inspect or write the briefing authoring resources embedded in this Lore version.

Usage:
  lore briefings <show-css|show-skill|write-css|write-skill|contract>

Commands:
  show-css          Print the exact briefing contract stylesheet
  show-skill        Print the agent briefing-authoring instructions
  write-css         Write the stylesheet to a path
  write-skill       Write the authoring skill to a path
  contract          Print the machine-readable contract as JSON

Aliases:
  lore briefing ...

Run 'lore help briefings <command>' for command-specific usage.`,

	"briefings show-css": `Print the exact site.css embedded in this Lore version to standard output.

Usage:
  lore briefings show-css

Example:
  lore briefings show-css > site.css`,

	"briefings show-skill": `Print the embedded agent instructions for authoring compatible briefings.

Usage:
  lore briefings show-skill

Example:
  lore briefings show-skill`,

	"briefings write-css": `Write the exact embedded briefing stylesheet to a new file.

Usage:
  lore briefings write-css <path>

An existing file at the path is replaced.

Example:
  lore briefings write-css ./site.css`,

	"briefings write-skill": `Write the embedded briefing-authoring skill to a new file.

Usage:
  lore briefings write-skill <path>

An existing file at the path is replaced.

Example:
  lore briefings write-skill ./SKILL.md`,

	"briefings contract": `Print the machine-readable briefing contract, including stylesheet identity and authoring constraints.

Usage:
  lore briefings contract [--format json]

Flags:
  --format json  Output format; json is currently the only supported value

Example:
  lore briefings contract --format json`,

	"search": `Search one Lore project's indexed knowledge and write the complete response as indented JSON.

Usage:
  lore search [--project <slug>] [filters] <query...>

Project and connection:
  --project <slug>        Project to search; defaults to LORE_PROJECT
  --server <url>          Lore server; LORE_SERVER_URL or client configuration
  --config <path>         Client credential YAML

Filters:
  --source-type <type>    Source types such as task, note, briefing, repository, conversation
  --repository <name>    Repository identity
  --branch <name>        Repository branch
  --tag <tag>            Normalized tag
  --created-from <time>  RFC3339 inclusive lower bound
  --created-to <time>    RFC3339 inclusive upper bound
  --limit <count>        Maximum documents (default 20)

List filters are repeatable or comma-separated. The JSON response includes query
modes, warnings, document scores, matching chunks, snippets, and locations.

Examples:
  LORE_PROJECT=refinery lore search --tag architecture "knowledge graph"
  lore search --project lore --source-type note,repository --repository wyrd-company/lore --branch main --limit 20 "search and retrieval"`,

	"watch": `Continuously synchronize configured sources.

Usage:
  lore [--config <credentials.yml>] watch --config <watch.yml> [flags]

The two config files have different purposes:
  credentials.yml  Server and tokens. Select it before 'watch' or with LORE_CONFIG.
  watch.yml        Source paths and identities. Select it with watch's --config flag.

Flags:
  --config <watch.yml>  Watch configuration (default lore-watch.yml)
  --server <url>        Lore server; LORE_SERVER_URL or credential configuration
  --token <token>       Ingest token; LORE_INGEST_TOKEN or config ingest-token

The watcher performs a complete scan at startup, debounces filesystem events,
and periodically performs another complete scan. Each source needs a stable
source-instance; concise mappings default it to the adapter name.

Example:
  lore --config credentials.yml watch --config watch.yml

Minimal watch.yml:
  project: refinery
  debounce: 750ms
  rescan-interval: 15m
  sources:
    tasks: /sources/refinery/tasks
    notes: /sources/refinery/notes`,

	"migrate": `Apply Lore database migrations to DATABASE_URL.

Usage:
  lore migrate

Migrations are an explicit deployment operation. They do not run automatically
when lore-server starts.

Example:
  DATABASE_URL='postgres://lore:secret@localhost:5432/lore?sslmode=disable' lore migrate`,

	"version": `Print the Lore CLI version.

Usage:
  lore version

Aliases:
  lore --version
  lore -version`,
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
