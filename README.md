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

Annotation handlers remain API shells. Browse and hybrid search are implemented,
and synchronization provides the
transactional manifest core: source-instance ownership, complete versus partial
manifests, immutable revisions, content-hash idempotency, and complete-manifest
deletion isolation. New revisions are chunked and keyword-indexed transactionally;
embeddings are completed asynchronously. Ingest and administrative routes require
their respective bearer tokens.

## Local development

Requirements are Go 1.25.9, Node.js 22, Docker, and Task.

The local `.env` supplies `AI_GATEWAY_API_KEY`. Compose also reads:

```text
DATABASE_URL
LORE_INGEST_TOKEN
LORE_ADMIN_TOKEN
PUBLIC_BASE_URL
```

Start PostgreSQL and apply migrations explicitly:

```bash
task database:up
task migrate
```

Run the server with the web application embedded:

```bash
task dev
```

The application is available at <http://localhost:8080>. Migrations are never
run implicitly by server startup. They can also be applied with either binary:

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

To deploy the complete local stack:

```bash
task migrate
docker compose up -d --build --wait
```

The published image is `ghcr.io/wyrd-company/lore`. It contains both
`lore-server` and `lore`; its default entry point starts the server.

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
GET /api/projects/{project}/search?q=...
```

The browse response includes source instances, type counts, tags, tasks, notes,
briefings, repository documents grouped by repository and branch, conversations,
and per-document embedding coverage. Document detail includes current rendered
content, normalized text, metadata, provenance, tags, revision history, and task
dependencies and dependents in both directions.

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

## Source uploads

Every upload names a stable project source instance. One-shot uploads are
partial by default, so they cannot delete sibling documents; add `--complete`
when the path is the authoritative projection of that source.

```bash
export LORE_SERVER_URL=http://localhost:8080
export LORE_INGEST_TOKEN=local-ingest-token

lore upload tasks --project refinery --source-instance kanban --complete /workspaces/kanban
lore upload notes --project refinery --source-instance mnemonic --complete /memory/.mnemonic/notes
lore upload briefing --project refinery --source-instance weekly --title "Weekly briefing" briefing.html
lore upload repository --project lore --source-instance repository docs README.md
```

Repository uploads derive the repository and branch from Git when possible;
`--repository` and `--branch` provide explicit overrides. Markdown, YAML, and
other UTF-8 text files use their respective shared renderers.

Conversation uploads scan Claude or Codex JSONL session directories and upload
only sessions that resolve to a project:

```bash
lore upload conversations --source-instance claude --provider claude \
  --mapping lore-projects.yml ~/.claude/projects
lore upload conversations --source-instance codex --provider codex \
  --mapping lore-projects.yml ~/.codex/sessions
```

The mapping file supports exact session IDs, longest working-directory prefix
matches, repository mappings, and an explicitly enabled `PROJECT` fallback:

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

Unassigned sessions are reported and are not uploaded. Normalized conversation
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
conventions. Annotation export is reserved at `lore annotations export` for
Milestone 4.

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
