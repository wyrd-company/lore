---
relationships:
  references:
    - lore
    - system
---

# Lore

Lore is a multi-project knowledge portal for browsing, searching, and annotating
project tasks, notes, briefings, repository documents, and agent conversations.
Local files remain authoritative; Lore stores a synchronized, project-scoped
projection and annotations against immutable revisions.

The product concept is documented in
[`docs/concepts/lore.yml`](docs/concepts/lore.yml). The implementation design is
documented in [`docs/technical-designs/system.yml`](docs/technical-designs/system.yml).

## Components

- `lore-server`: HTTP API and embedded React application.
- `lore`: source upload and watch client, briefing authoring resources, and
  explicit migration entry point.
- PostgreSQL with pgvector: projects, source instances, documents, immutable
  revisions, chunks, task relationships, tags, annotations, and 1024-dimension
  `voyage/voyage-4` embeddings.
- Docker Compose: local PostgreSQL and server deployment.

The HTTP API has three project-scoped boundaries:

- `GET /api/projects/{project}/browse` and `/search`
- `/api/projects/{project}/annotations`
- `POST /api/projects/{project}/synchronizations`

Synchronization provides source-instance ownership, complete versus partial
manifests, immutable revisions, content-hash idempotency, retention, and
complete-manifest deletion isolation. New revisions are chunked and
keyword-indexed transactionally; embeddings are completed asynchronously.
Annotation reads and browser-attributed mutations are available to every project
viewer. Ingest and administrative routes require their respective bearer tokens.

## Quickstart

Requirements are Go 1.25.9, Node.js 22, Docker, and Task.

Copy `.env.example` to `.env`, supply a real `AI_GATEWAY_API_KEY`, and replace
the ingest and admin tokens outside local development. `DATABASE_URL` defaults
to the Compose PostgreSQL service; `PUBLIC_BASE_URL` is the browser-visible
origin. `LORE_WATCH_CONFIG`, `LORE_CLIENT_CONFIG`, and `LORE_SOURCE_ROOT`
configure the optional watcher container.

Build and start the application. Compose waits for PostgreSQL to become healthy,
runs migrations in a one-shot init container using the Lore image, and starts
the server only after the migration container succeeds:

```bash
cp .env.example .env
# Edit .env before continuing.
docker compose up -d --build --wait
```

The application is available at <http://localhost:8080>. Migrations remain an
explicit operation and are not run by server startup; Compose performs that
operation through the `migrate` init service. Outside Compose they can be
applied with either binary:

```bash
DATABASE_URL='postgres://lore:lore@localhost:5432/lore?sslmode=disable' lore-server migrate
DATABASE_URL='postgres://lore:lore@localhost:5432/lore?sslmode=disable' lore migrate
```

Run the validated workflows through Task:

```bash
task lint
task test   # starts pgvector and runs integration tests against real PostgreSQL
task build
task ci
```

For source development, the equivalent Task workflow is:

```bash
task migrate
task dev
```

Create the client credential file, then create the first project through the
admin-protected bootstrap path before its first synchronization:

```bash
mkdir -p ~/.config/lore
cat > ~/.config/lore/config.yml <<'YAML'
server: http://localhost:8080
ingest-token: local-ingest-token
admin-token: local-admin-token
YAML
chmod 600 ~/.config/lore/config.yml
lore config
lore projects create --slug lore --name "Lore"
```

The command is idempotent by slug. The equivalent endpoint is
`POST /api/projects` with the administrative bearer token and a JSON body
containing `slug` and `name`.

The published image is `ghcr.io/wyrd-company/lore`. It contains both
`lore-server` and `lore`; its default entry point starts the server.

### Environment configuration

| Variable | Used by | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | server, migrations | PostgreSQL connection string |
| `AI_GATEWAY_API_KEY` | server | Vercel AI Gateway embeddings; keyword search remains available without it |
| `LORE_INGEST_TOKEN` | server, CLI, watcher | Bearer token for synchronization |
| `LORE_ADMIN_TOKEN` | server, CLI | Bearer token for project bootstrap and revision cleanup |
| `PUBLIC_BASE_URL` | server, CLI fallback | Browser-visible Lore origin |
| `LORE_LISTEN_ADDRESS` | server | HTTP bind address; defaults to `:8080` |
| `LORE_SERVER_URL` | CLI, watcher | Server URL; overrides `PUBLIC_BASE_URL`; a value without a scheme uses HTTP |
| `LORE_CONFIG` | CLI, watcher | Explicit client credential configuration path |
| `LORE_PROJECT` | CLI | Optional project flag/fallback value |
| `LORE_CLIENT_CONFIG` | Compose watcher | Host credential file mounted read-only into the watcher |

### CLI credential configuration

The `lore` client reads this YAML shape:

```yaml
server: https://lore.example.net
ingest-token: replace-with-ingest-token
admin-token: replace-with-admin-token
```

`LORE_PROJECT` is deliberately not a file setting because it varies by
workspace. Command-line values resolve in this order: explicit flags,
environment variables, then the credential file. `LORE_SERVER_URL` is the
primary server environment variable; `PUBLIC_BASE_URL` remains its fallback.
Server values honor an explicit `http://` or `https://` scheme. A value without
a scheme, such as `lore:8080`, resolves to `http://lore:8080` for local and
Docker-network deployments where TLS terminates outside Lore.

The credential file is selected in this order:

1. `--config <path>`.
2. `LORE_CONFIG`.
3. `$XDG_CONFIG_HOME/lore/config.yml`, or `~/.config/lore/config.yml` when
   `XDG_CONFIG_HOME` is unset.
4. `/etc/lore/config.yml`.

The two default locations are searched in order, allowing `/etc` to fill keys
missing from the user file. A `--config` or `LORE_CONFIG` selection searches
only that path. Missing and partial files are allowed so flags and environment
variables remain usable. Malformed or unreadable files fail with their exact
path; a command missing a required token reports the credential and every
location it checked.

Run `lore config` to print the resolved server and each value's source. When
Lore supplies the HTTP scheme, this output includes `scheme omitted; assumed
http://`. Tokens are always printed as `<redacted>`. For a read-only container
mount, either use `LORE_CONFIG=/etc/lore/config.yml` or rely on that default
path:

```bash
docker run --rm \
  --entrypoint /usr/local/bin/lore \
  -v "$HOME/.config/lore/config.yml:/etc/lore/config.yml:ro" \
  ghcr.io/wyrd-company/lore:latest config
```

### Compose deployment with watchers

The image also contains the `lore` watcher executable. Keep `lore-watch.yml`
for source mappings and save a separate credential file as
`~/.config/lore/watcher.yml`. Container paths in the watch file live beneath
`/sources`; the credential server URL must be reachable from the watcher
container:

```yaml
# lore-client.yml
server: lore-server:8080
ingest-token: local-ingest-token
admin-token: local-admin-token
```

Set `LORE_SOURCE_ROOT` to the sources' common host directory, then start the
optional Compose profile:

```bash
docker compose up -d --build --wait
LORE_WATCH_CONFIG=./lore-watch.yml \
LORE_CLIENT_CONFIG=$HOME/.config/lore/watcher.yml \
LORE_SOURCE_ROOT=/workspaces \
  docker compose --profile watch up -d --build lore-watcher
```

Compose mounts both configuration files and the source tree read-only. The
watcher's `--config /config/lore-watch.yml` remains the source-layout option;
its credentials come from `LORE_CONFIG=/config/lore-client.yml`.

Run additional watcher services from the same image when sources require
different mounts or credentials. Mount source directories read-only and give
each projection a stable `source-instance`. Cloudflare Tunnel and Cloudflare
Zero Trust exposure are intentionally out of scope; place the deployment behind
the network boundary appropriate to the installation.

### Existing PostgreSQL

Lore can use an existing PostgreSQL 17 database instead of the bundled Compose
service. Create a dedicated login role and database as a PostgreSQL
administrator, then install `pgcrypto` and `vector` in that database as a
superuser:

```sql
CREATE ROLE lore LOGIN PASSWORD 'replace-with-a-secret';
CREATE DATABASE lore OWNER lore;
\connect lore
CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
```

Alternatively, grant the Lore database role sufficient privileges to install
both extensions. Pre-creating them is preferred because many managed providers
reserve extension management for an administrative role.

Set the deployment connection string to the external host, for example:

```bash
DATABASE_URL='postgres://lore:replace-with-a-secret@postgres.example.net:5432/lore?sslmode=require'
```

Remove or ignore the bundled `postgres` service and remove its direct
`depends_on` entries from `migrate` and `lore-server`. Keep the
`lore-server` dependency on `migrate` with
`condition: service_completed_successfully`; the init container then applies
migrations to `DATABASE_URL` and continues to gate server startup. This replaces
the former manual `lore-server migrate` deployment step.

## CLI usage

```text
lore --help
lore -h
lore help
lore [--config <credentials.yml>] config
lore projects create --slug <slug> --name <name>
lore upload <tasks|notes|briefing|repository|conversations> [flags] <path...>
lore [--config <credentials.yml>] watch --config <watch.yml>
lore annotations export --project <project> [--after <cursor>] [--output <path>]
lore briefings <show-css|show-skill|write-css|write-skill|contract>
lore migrate
lore version
```

Every command and command group accepts `--help` or `-h`. The singular command
aliases `project`, `annotation`, and `briefing` are equivalent to `projects`,
`annotations`, and `briefings` respectively.

## Synchronization manifest

The ingest endpoint accepts a manifest shaped as follows:

```json
{
  "project": "refinery",
  "sourceInstance": "mnemonic-notes",
  "sourceType": "note",
  "boundary": "complete",
  "documents": [
    {
      "identity": "note-identity",
      "title": "Note title",
      "contentHash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
      "normalizedText": "Searchable text",
      "renderedContent": "<p>Rendered content</p>",
      "renderer": "markdown",
      "metadata": {},
      "provenance": {}
    }
  ]
}
```

A `complete` manifest marks documents absent from that same source instance as
deleted. A `partial` manifest never deletes siblings. Matching current content
hashes do not create revisions or chunks.

## Search indexing and retrieval

Lore divides each new revision into bounded 220-word chunks with a 40-word
overlap. Chunk locations retain word offsets; conversation chunks additionally
retain provider message identifiers, message order, role, and thinking status.
The server constructs chunks itself rather than accepting client-generated
index data.

PostgreSQL stores a weighted `tsvector` for each chunk:

- document titles have the strongest weight;
- task and note tags share that strongest weight;
- ordinary content, including conversation user and assistant messages, uses
  normal body weight;
- assistant thinking and source metadata use a lower weight.

Exact tags, source type, repository, branch, and dates remain structured
filters. Search first resolves the project, independently selects keyword and
vector candidates inside that project, combines chunk ranks using reciprocal
rank fusion, and groups matching chunks under their documents.

Embeddings use `voyage/voyage-4` through Vercel AI Gateway at exactly 1,024
dimensions. Synchronization commits chunks and durable `embedding_jobs` rows
without calling the gateway. The server worker batches jobs, retries transient
failures with bounded backoff, and stores one pgvector row per chunk. A gateway
outage therefore leaves keyword search available and the failed vectors queued
for later backfill. Server startup also chunks current revisions created by an
older Lore version before starting the worker.

## Browse and search API

The read API is public within Lore's network boundary and always scopes document
retrieval before accessing content:

```text
GET /api/projects
GET /api/projects/{project}/browse
GET /api/projects/{project}/documents/{document-uuid}
GET /api/projects/{project}/documents/{document-uuid}/revisions
GET /api/projects/{project}/documents/{document-uuid}/revisions/{revision-uuid}
GET /api/projects/{project}/search?q=...
```

The browse response includes source instances, type counts, the unified project
tag store, terms, tasks, their source-board status order, notes, briefings,
repository documents grouped by
repository and branch, conversations, and per-document embedding coverage. Task
summaries also include dependency, dependent, and open-annotation counts for the
read-only board. Document detail includes current rendered content, normalized
text, metadata, provenance, tags, referenced terms, revision history, task
dependencies and dependents in both directions, and note relationships.

Repository YAML documents and Markdown front matter contribute top-level `tags`
and `terms` values. Lore normalizes those values into project-scoped stores. The
last path segment of `$schema` becomes the repository document's schema type. A
repository document whose schema path ends in `/term` defines the term named by
the document's filename stem. Referenced terms without an uploaded definition
remain visible through a missing-term page.

The project tasks page renders the ingested kanban-md vocabulary as a read-only
board. Recognized lifecycle statuses and synonyms share canonical lanes; custom
statuses retain their source order and use a neutral hue. Status, priority, and
tag facets share the URL query string across Board and List views. Cards link to
task pages and expose priority, relationship, and open-annotation context without
drag-and-drop or mutation controls. Narrow screens default to List while the
Board view keeps fixed-width, independently scrolling columns inside its own
horizontal scroller.

The notes page provides URL-backed role, tag, lifecycle, and source-project
facets. Notes sort by their source `createdAt`, source `updatedAt`, or title, with
Lore synchronization timestamps as fallbacks. Mnemonic `relatedTo` entries link
between notes available in the same synchronized source.

Repository browsing provides URL-backed repository, schema type, and tag facets.
Briefing detail pages include contextual navigation to other project briefings.
On every task, note, briefing, repository, and conversation page, visible text
matching a normalized tag or term becomes a link. Referenced terms are also
listed at the bottom of the page, including links to missing definitions.

Search accepts repeatable or comma-separated filters:

```text
sourceType=task,note
repository=wyrd-company/lore
branch=main
tag=search
createdFrom=2026-01-01T00:00:00Z
createdTo=2026-12-31T23:59:59Z
limit=20
```

Results expose the fused document score and each matching chunk's snippet,
structural location, keyword/vector ranks, component scores, and fused score.
When query embedding is unavailable, the response includes a warning and returns
keyword-ranked results instead of failing the request.

## Annotations and revision retention

Annotations target an immutable revision and preserve the original content hash,
source provenance, stable selector, structural location, selected quote, and
surrounding quote context. Browser-supplied usernames are attribution rather than
authenticated identities. Every create, update, status transition, copy, move,
and cleanup records the supplied attribution in an immutable event stream.

```text
GET    /api/projects/{project}/annotations
POST   /api/projects/{project}/annotations
GET    /api/projects/{project}/annotations/{annotation-uuid}
PATCH  /api/projects/{project}/annotations/{annotation-uuid}
POST   /api/projects/{project}/annotations/{annotation-uuid}/copy
POST   /api/projects/{project}/annotations/{annotation-uuid}/move
GET    /api/projects/{project}/annotations/{annotation-uuid}/events
GET    /api/projects/{project}/annotations/export?after={cursor}
POST   /api/projects/{project}/admin/cleanup
```

Annotation listing filters accept `documentId`, `revisionId`, `status`, and an
incremental `after` cursor. Status is `open`, `resolved`, or `dismissed`.

Synchronization removes superseded revisions that have no annotations. Prior
revisions with annotations remain renderable, including through the revision
detail endpoint. A prior revision becomes cleanup-eligible only after none of its
annotations remain open. The admin-protected cleanup operation removes its
rendered revision and search data while retaining annotation tombstones with
revision identity, body, attribution, resolution metadata, selector, provenance,
and original hash.

Export exactly one project's complete annotation snapshot as JSON:

```bash
lore annotations export --project lore --output lore-annotations.json
```

Use the returned `nextCursor` for an incremental export:

```bash
lore annotations export --project lore --after 12345 --output lore-annotations-since-12345.json
```

The stable `lore.annotations/v1` envelope contains the project, mode, cursors,
document and revision identities, source provenance, selector, attribution,
status, body, resolution fields, tombstone state, and timestamps.

## Source uploads

Every upload names a stable project source instance. One-shot uploads are
partial by default, so they cannot delete sibling documents; add `--complete`
when the path is the authoritative projection of that source.

```bash
lore upload tasks --project refinery --source-instance kanban --complete /workspaces/kanban
lore upload notes --project refinery --source-instance mnemonic --complete /memory/.mnemonic/notes
lore upload briefing --project refinery --source-instance weekly --title "Weekly briefing" briefing.html
lore upload repository --project lore --source-instance repository docs README.md
```

Repository uploads derive the repository and branch from Git when possible;
`--repository` and `--branch` provide explicit overrides. Markdown, YAML, and
other UTF-8 text files use their respective shared renderers.

Conversation uploads scan Claude or Codex JSONL session directories and upload
only sessions that resolve to a project. Use `--complete` for an authoritative
directory scan so sessions removed from disk are removed from that source
instance; omit it only for an intentionally partial upload:

```bash
lore upload conversations --source-instance claude --provider claude --complete \
  --mapping lore-projects.yml ~/.claude/projects
lore upload conversations --source-instance codex --provider codex --complete \
  --mapping lore-projects.yml ~/.codex/sessions
```

The mapping file supports exact session IDs, longest working-directory prefix
matches, repository mappings, and an explicitly enabled `LORE_PROJECT` fallback:

```yaml
sessions:
  session-uuid: lore
paths:
  - prefix: /workspaces/tools/lore
    project: lore
repositories:
  git@github.com:wyrd-company/lore.git: lore
allowProjectFallback: false
```

Malformed or unknown JSONL records are skipped with filename-and-line warnings;
the remaining session is still uploaded. Unassigned sessions are reported and
are not uploaded. Normalized conversation
documents retain user, assistant, and collapsed assistant-thinking messages;
instructions, tool traffic, and provider bookkeeping are excluded.

## Continuous synchronization

`lore watch` performs a complete scan at startup, debounces filesystem events,
and periodically runs another complete scan to recover missed events. Each
source retries independently with bounded exponential backoff.

The concise design layout is supported directly:

```yaml
project: refinery
debounce: 750ms
rescan-interval: 15m
sources:
  tasks: /sources/refinery/tasks
  notes: /sources/refinery/notes
```

Expanded source entries can set every adapter input explicitly:

```yaml
sources:
  - project: refinery
    source-instance: mnemonic-notes
    adapter: notes
    path: /sources/refinery/notes
  - source-instance: codex-sessions
    adapter: conversations
    provider: codex
    path: /sources/codex
    mapping: /config/lore-projects.yml
```

Run it with:

```bash
lore watch --config lore-watch.yml
# With an explicit credential file, place it before the command so the watch
# file keeps its established --config spelling:
lore --config ~/.config/lore/config.yml watch --config lore-watch.yml
```

## Briefing authoring contract

The CLI embeds the exact release-aligned application stylesheet and a concise
agent authoring skill for trusted `.lore-prose` HTML briefings:

```bash
lore briefings show-css
lore briefings show-skill
lore briefings write-css ./site.css
lore briefings write-skill ./lore-briefing-skill.md
lore briefings contract --format json
```

The contract output includes the stylesheet SHA-256 identity, body/fragment
payload rules, stable annotation targets, and self-contained image and diagram
conventions.

## Release conventions

Release tags are bare Semantic Versioning values such as `0.1.0`; they never
have a leading `v`. A tag must match `VERSION` and point to `main`. The `cd`
workflow creates checksummed Linux and macOS CLI archives, a GitHub Release, a
multi-architecture GHCR image, and the `wyrd-company/tools` Homebrew formula via
the `FORMULAE_PUBLISH_KEY` deploy key.

## Definition of Done

- All design elements have been implemented completely
- All implementation has been tested and validated e2e without any mocks, stubs, or fakes - using real services. Tests/validation is automated, not by hand.
- CI/CD has been setup and follows our conventions (ci.yml, cd.yml, tag triggered publish)
- The CLI has been published to the wyrd-company tap
- The container images have been published to GHCR.
- Instructions for deploying the system have been provided.
- This readme is updated
- Atomic commits. Work has been pushed.

### Out of scope

- Cloudflare configuration

## Rules of Engagement

- Make any decision necessary to achieve the definition of done.
- Do not use GitHub pull requests.
