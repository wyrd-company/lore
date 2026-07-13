---
relationships:
  references:
    - lore
    - system
---

# Lore

Lore is a multi-project knowledge portal for browsing, searching, and annotating
project tasks, notes, briefings, repository documents, and agent conversations.

The product concept is documented in
[`docs/concepts/lore.yml`](docs/concepts/lore.yml). The implementation design is
documented in [`docs/technical-designs/system.yml`](docs/technical-designs/system.yml).

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

## Provided:

- `.env` with a valid AI_GATEWAY_API_KEY for Vercel AI Gateway for local testing.
- AI_GATEWAY_API_KEY is available in the repo actions secrets.
- FORMULAE_PUBLISH_KEY is available in the repo actions secrets.

## Rules of Engagement

- You must make any decision necessary to achieve the definition of done.
- No GitHub PRs.
- you may use kanban-md if you choose, but you must initialize a new kanban board in the lore directory, .gitignored.