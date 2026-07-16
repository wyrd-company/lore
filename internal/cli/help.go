/*
---
relationships:

	implements: system

---
*/
package cli

import (
	"fmt"
	"io"
	"strings"
)

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

	"annotations": `Export project annotations or reply to an existing annotation.

Usage:
  lore annotations <export|reply> [flags]

Aliases:
  lore annotation export

Commands:
  export  Write a lossless JSON snapshot or incremental export
  reply   Add an attributed reply to an annotation thread

Run 'lore help annotations <command>' for command-specific usage.`,

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

	"annotations reply": `Add an attributed reply to one annotation thread.

Usage:
  lore annotations reply [--project <slug>] --body <text> --attributed-username <name> <annotation-uuid>

Flags:
  --project <slug>              Project containing the annotation; defaults to LORE_PROJECT
  --body <text>                 Reply body
  --attributed-username <name>  Person or agent responsible for the reply
  --server <url>                Lore server; LORE_SERVER_URL or client configuration
  --config <path>               Client credential YAML

The created reply is written as indented JSON.

Example:
  lore annotations reply --project refinery --body "Addressed in the current design." --attributed-username Bob 550e8400-e29b-41d4-a716-446655440000`,

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

Supported adapters are tasks, notes, briefing, repository, and conversations.
A malformed source file is recorded as a watcher issue and skipped while healthy
siblings continue synchronizing. Fix the file, open Watcher issues in the web
interface, and choose Retry to clear its quarantine. The next event or rescan
attempts that path again; merely editing a quarantined path does not clear it.

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
