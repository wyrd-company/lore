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
- `lore`: command-line client and explicit migration entry point.
- PostgreSQL with pgvector: projects, source instances, documents, immutable
  revisions, chunks, task relationships, tags, annotations, and 1024-dimension
  `voyage/voyage-4` embeddings.
- Docker Compose: local PostgreSQL and server deployment.

The HTTP API has three project-scoped boundaries:

- `GET /api/projects/{project}/browse` and `/search`
- `/api/projects/{project}/annotations`
- `POST /api/projects/{project}/synchronizations`

Browse and annotation handlers are API shells. Synchronization implements the
transactional manifest core: source-instance ownership, complete versus partial
manifests, immutable revisions, content-hash idempotency, and complete-manifest
deletion isolation. Ingest and administrative routes require their respective
bearer tokens.

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
      "provenance": {},
      "chunks": [
        {"ordinal": 0, "normalizedText": "Searchable text", "structuralLocation": {}}
      ]
    }
  ]
}
```

A `complete` manifest marks documents absent from that same source instance as
deleted. A `partial` manifest never deletes siblings. Matching current content
hashes do not create revisions or chunks.

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
