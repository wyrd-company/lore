---
relationships:
  references:
    - lore
    - system
---

# Lore — UI Design Specification

Implementable UI design for the Lore React web application: a calm, dense,
readable knowledge portal for browsing, searching, and annotating project
tasks, notes, briefings, repository documents, and agent conversations.

## Design direction

Lore reads like an **editorial archive**: warm paper in light, ink in dark,
a humanist serif (`Fraunces`) for titles and rendered-document headings, a
humanist sans (`Public Sans`) for the interface, and a calm monospace
(`JetBrains Mono`) for code and identifiers. Verdigris is the single accent;
ochre is reserved for annotations and highlights so "someone marked this"
always reads the same way across every renderer. The feel is a quiet reading
room, not a dashboard — generous reading measure, hairline rules instead of
heavy boxes, soft paper shadows, restrained motion.

All visual tokens live in `site.css` as CSS custom properties. That same file
is the **briefing contract stylesheet**: uploaded trusted HTML briefing bodies
are dropped into `.lore-prose` and must look finished with no CSS of their own.
Every rendered document type (Markdown, YAML, briefing, conversation, task)
therefore shares the `.lore-prose` typography so the whole portal reads as one
publication.

Fonts are declared as stacks with strong system fallbacks (`Iowan Old Style` /
Georgia for display; system-humanist for body; `ui-monospace` for code) so
briefings render correctly on an offline local deployment even if the webfonts
are absent.

---

## 1. Information architecture & navigation

### 1.1 Global frame (`.l-app`)

A CSS grid with four regions:

```
┌──────────┬─────────────────────────────────────┐
│  brand   │  header (project select · search)   │  56px row
├──────────┼─────────────────────────────────────┤
│ sidebar  │  main (routed page)                 │  1fr row
│ 264px    │                                     │
└──────────┴─────────────────────────────────────┘
```

- **Brand** (`.l-brand`): mark + "Lore" wordmark, top-left, above the sidebar.
- **Header** (`.l-header`, sticky, `z-header`): project selector at left, a
  spacer, global search, then theme toggle and attribution-name control at
  right. Frosted (`backdrop-filter`) so content scrolls softly beneath it.
- **Sidebar** (`.l-sidebar`): project-scoped navigation. Collapses to an
  off-canvas drawer under 900px (`data-open` toggled by a header hamburger).
- **Main** (`.l-main`): the routed page, centered in `.l-page` (max 1080px;
  `.l-page--reading` narrows to 880px for single-document reading).

### 1.2 Project selector (`.lore-project-select`)

Lives in the header, always visible, because **every** route is project-scoped
(the concept's `project-scoped-content`; the design's `project-boundary`). It
is a button showing a color dot + project name + caret; clicking opens a
`.lore-popover` list of projects with a type-ahead filter. Selecting a project
navigates to that project's overview and swaps the entire sidebar/route tree.
The active project is the first path segment: `/{project}/…`.

Switching projects never carries filters, search text, or annotation state
across the boundary — the server rejects cross-project data and the UI mirrors
that hard wall.

### 1.3 Per-project sidebar sections

`.l-nav-section` groups with `.l-nav-section__label` eyebrows. Each
`.l-nav-item` shows an icon, label, and an optional right-aligned count
(`__count`). Active route uses `aria-current="page"` (accent inset bar).

| Section        | Route                         | Source badge type |
| -------------- | ----------------------------- | ----------------- |
| Overview       | `/{project}`                  | —                 |
| Search         | `/{project}/search`           | —                 |
| Tasks          | `/{project}/tasks`            | `task`            |
| Notes          | `/{project}/notes`            | `note`            |
| Briefings      | `/{project}/briefings`        | `briefing`        |
| Repository     | `/{project}/repo`             | `repo`            |
| Conversations  | `/{project}/conversations`    | `conversation`    |

Source-type identity is carried everywhere by `.lore-source-badge[data-type]`
with one fixed color per type (task=blue, note=verdigris, briefing=ochre,
repo=neutral, conversation=green). Same colors in the sidebar icons, list rows,
and search result headers so a user learns the palette once.

### 1.4 Routing map

```
/{project}                          Overview (recent + counts per source)
/{project}/search?q=&type=&…        Hybrid search results
/{project}/tasks                    Task index (filter by status/tag)
/{project}/tasks/{taskId}           Task page
/{project}/notes                    Note index
/{project}/notes/{noteId}           Markdown page
/{project}/briefings                Briefing index
/{project}/briefings/{id}           Briefing page
/{project}/repo                     Repo doc index (group by repo/branch)
/{project}/repo/{repo}/{branch}/*   Repository document page (md/yaml/text)
/{project}/conversations            Conversation index
/{project}/conversations/{id}       Conversation page
```

Every document route accepts `?rev={hash}` to view a retained prior revision
and `?anno={id}` to deep-link and auto-open an annotation.

---

## 2. Shared page anatomy

Every source page is built from the same parts so navigation, provenance, and
annotation behavior stay identical across renderers (design goal
`heterogeneous-rendering`):

1. **Breadcrumbs** (`.lore-crumbs`): `Project / Section / Title`.
2. **Page header** (`.lore-page-head`): serif title, action buttons
   (revision selector, copy link, export not shown here), and a
   **provenance strip** (`.lore-provenance`) showing source instance, source
   path/repo/branch, content hash (short, mono), and last-synced marker.
3. **Body**: a two-column `.l-doc` — rendered content (`.lore-prose` or a
   type-specific renderer) plus a sticky **annotation rail** (`.lore-doc__rail`)
   on the right. Under 1180px the rail drops below the content.
4. **Revision selector** (`.lore-revision`): only rendered when annotated prior
   revisions exist (see §6.4).

Loading uses skeletons in the same skeleton (`.lore-skel`); errors use
`.lore-error`; a document with no body uses `.lore-empty`.

---

## 3. Source renderers

### 3.1 Task page (`/{project}/tasks/{taskId}`)

- Header title = task title; a `.lore-status[data-state]` badge for task status
  (todo / doing / blocked / done) sits beside it.
- **Metadata grid** (`.lore-task-meta`): key/value tiles for task identifier,
  status, and tags (`.lore-chip--tag`). Tags link to
  `/{project}/search?type=task&tag=…`.
- **Description**: rendered through `.lore-prose` (tasks may carry Markdown).
- **Dependencies & dependents** (`.lore-deps`, two columns):
  - Left column "Depends on" lists `.lore-dep-link` rows (title + status dot +
    inward arrow) to each dependency task page.
  - Right column "Blocks / dependents" lists tasks that depend on this one.
  - Empty column shows an inline muted "None" rather than a full empty state.
  - Dependency edges are navigation only; they carry no search weight and there
    is **no task mutation UI** anywhere (design: "provides no task mutation
    operations").
- Annotation targets: task fields (title, description text, individual tag,
  status) via structural selectors; description body also supports text-quote
  selection.

### 3.2 Markdown page (notes & repo `.md`)

- Body rendered into `.lore-prose` with GFM: tables (wrapped in
  `.table-scroll` for horizontal overflow), task lists, footnotes,
  syntax-highlighted code (map the highlighter to `.tok-*` classes), stable
  heading ids (`scroll-margin-top` set), and front-matter surfaced as a small
  metadata card above the body.
- A right-rail **table of contents** may reuse `.l-nav-item` styling from the
  heading tree (optional; annotation rail takes priority when annotations
  exist — TOC and rail can share the rail via tabs if both are present).
- Annotation targets: heading paths (structural) and text-quote selection.

### 3.3 Briefing page (briefings)

- The stored body HTML is inserted verbatim into a `.lore-prose` container
  inside the normal app shell — **no iframe** (content is trusted). `site.css`
  alone styles it (the contract). Images are data URLs, diagrams inline SVG /
  pre-rendered Mermaid; `.lore-prose figure.diagram` frames them.
- Title comes from the upload filename unless one was supplied.
- Provenance strip notes filename and the stylesheet-contract identity.
- Annotation targets: HTML element ids and heading paths (structural) plus
  text-quote selection.
- Because briefing bodies are the stylesheet's second consumer, the whole
  §5 prose contract must render them beautifully with zero briefing-authored CSS.

### 3.4 Structured YAML page (repo `.yml`)

- Rendered by the structural renderer into `.lore-prose` semantics: mapping
  keys become headings (nesting depth → `h1`…`h6`, capped at `h6`), scalars
  become paragraphs, string arrays become lists, mixed/numeric arrays become
  lists preserving scalar form, nested mappings become nested sections, arrays
  of mappings become repeated sections.
- Optional `.lore-struct` treatment (left rule + mono `.lore-struct__key`) for
  a more data-shaped read on deeply nested docs; default is prose headings so
  YAML docs read like documents, matching the archive tone.
- Annotation targets: YAML property paths (structural), e.g.
  `proposed-design.content-model`.

### 3.5 Conversation page (conversations)

- `.lore-convo` column of `.lore-msg` rows, each `data-role="user" | "assistant"`.
- Avatar chip (`__avatar`) color-coded by role; a `__role` eyebrow label; body
  (`.lore-msg__body`) rendered as Markdown via `.lore-prose`.
- **Thinking blocks** render as `.lore-thinking` `<details>` disclosures,
  **collapsed by default**, dashed border, "Thinking" summary with a rotating
  caret. Only assistant messages have them.
- System/developer instructions, tool calls, and tool results are absent (the
  ingestion normalization already strips them; the UI never shows them).
- Provenance strip: provider, session id (mono), working directory, title.
- Annotation targets: conversation message ids (structural) and text-quote
  selection inside a message body. Thinking regions are annotatable only when
  expanded.

---

## 4. Hybrid search UX (`/{project}/search`)

Implements the design's `search-indexing`: keyword + vector candidates fused by
reciprocal rank fusion, grouped by parent document, filtered by facets.

### 4.1 Layout

```
┌─────────────────────────────────────────────────────────┐
│ .lore-search  [ query …………………  ⌘K ]     (rank: fused ▾) │
├──────────────┬──────────────────────────────────────────┤
│ Facet rail   │  Results (.lore-results)                 │
│ (filters)    │   ▸ grouped chunks under parent docs     │
└──────────────┴──────────────────────────────────────────┘
```

- Search box `.lore-search` (also the global header search; ⌘K focuses it).
- A `.lore-segmented` control offers rank inspection (Fused / Keyword / Vector)
  for debugging relevance; default **Fused**.

### 4.2 Filters (facets)

A left rail (or a `.lore-facets` bar on narrow widths) of toggle
`.lore-facet[aria-pressed]` chips with counts, grouped:

- **Source type**: task, note, briefing, repo, conversation (multi-select).
- **Repository** and **Branch**: shown only when repo docs are in the result
  set; branch is dependent on the chosen repository.
- **Tags**: exact structured tag values (task/note tags), type-ahead when long.
- **Dates**: last-synced / created range (presets: 24h, 7d, 30d, custom).

Active filters also appear as removable `.lore-chip--removable` above results so
they can be cleared without reopening the rail. Filters serialize to the query
string so a search is shareable.

### 4.3 Results — grouped chunks under parent documents

Each parent document is one `.lore-result` card:

- `.lore-result__head`: source badge + document title (links to the doc) +
  fused score (`__score`, mono, subtle) at right.
- Under it, up to N matching `.lore-chunk` rows: a mono structural location
  (`__loc`, e.g. `§ content-model` or `heading path`), then a snippet
  (`__snippet`) with query terms wrapped in `<mark>`. Clicking a chunk deep-links
  to that location in the document (`#anchor`).
- If more chunks matched, a `.lore-chunk__more` "＋3 more passages" expander.

Empty query → recent documents + tag cloud. Zero results → `.lore-empty` with
a hint to broaden filters. In-flight → three skeleton result cards.

---

## 5. Prose / briefing contract (typography)

`.lore-prose` is the shared reading surface for briefings and every rendered
document. The contract (fully specified in `site.css` §5) guarantees, with no
author CSS:

- **Headings** in `--font-display`, tight leading; `h2` gets a hairline
  underline for section rhythm; `h5`/`h6` become small-caps eyebrows.
- **Body** at `--text-md` / `--leading-relaxed`, capped at `--measure` (72ch)
  for a comfortable line; first paragraph reads as a lead.
- **Lists** with custom verdigris bullet dots, tabular ordered markers, GFM
  task-list checkboxes, and definition lists.
- **Blockquotes** as tinted accent panels; **code** inline (bordered chip) and
  block (framed, horizontal-scroll, `.tok-*` syntax colors); **kbd** as keycaps.
- **Tables** full-width, zebra + hover, sticky-friendly headers, tabular
  numerals, wrapped in an `overflow-x` scroller so wide tables never break the
  page.
- **Figures / images / inline SVG diagrams** framed on a surface panel with
  italic centered captions; images capped to `max-width:100%`.
- **Marks** (search highlight) and **footnotes** styled to match.

This is the single most load-bearing block: it must look finished on a raw
briefing body and identical to the app's own rendered content.

---

## 6. Annotation UX

Implements `annotation-model` and `revision-retention`.

### 6.1 Creating annotations

Two entry paths, both producing a target selector:

1. **Text selection** → on `mouseup`/`selectionchange` over rendered content a
   `.lore-anno-pop` popover appears anchored to the selection with an
   "Annotate" action. Captures the text-quote selector, the selected quotation,
   and surrounding context.
2. **Structural target** → hovering an annotatable structural element (task
   field, YAML property, HTML element id, heading, conversation message) reveals
   a small margin "＋" affordance; clicking targets that stable selector.

Both open a composer (inline in the rail, or `.lore-modal` on narrow screens):
attribution name (prefilled from localStorage, editable), body `.lore-textarea`,
and Save/Cancel. Creation records originating operation, target selector,
selected quote, structural location, and the revision's original content hash.

### 6.2 Attribution name (localStorage)

A header control (`.lore-attribution`) stores the viewer's display name in
`localStorage` and sends it with every annotation mutation. Framed as
attribution, **not** authentication (matches `access-and-credentials`). First
mutation with an empty name prompts inline for one.

### 6.3 States: open / resolved / dismissed

- In-content highlights: `.lore-anno-target[data-state]` — ochre underline for
  `open`, green for `resolved`, dotted grey for `dismissed`. Active/selected
  annotation gets a focus ring (`.is-active`).
- Rail cards: `.lore-anno[data-state]` with a colored left border, status badge
  (`.lore-status[data-state]`), author, relative time, a clamped quote, and
  body. Resolved/dismissed cards dim (opacity). Hovering/selecting reveals
  `__actions` (Resolve, Dismiss, Reopen, Copy to revision, Move to revision).
- The rail header offers a state filter (All / Open / Resolved / Dismissed) and
  the open count. Clicking a rail card scrolls to and rings its in-content
  target and vice-versa.

### 6.4 Revision dropdown & cross-revision moves

- The current revision is default. When prior revisions are retained (because
  they still hold open annotations), the `.lore-revision` control appears in the
  page header (ochre, to signal "you're looking at retained history" when a past
  revision is chosen). It lists: **Current** plus each retained revision with its
  short hash, date, and open-annotation count.
- Viewing a prior revision renders that immutable content with its annotations
  in place, read as-authored.
- Each annotation's `__actions` include **Copy to revision** and
  **Move to revision** (a `.lore-popover` of eligible revisions):
  - *Copy* creates a new annotation on the target revision whose provenance
    references the source annotation (`__prov` line shows "copied from …").
  - *Move* re-points the active target to the chosen revision while retaining
    the original revision + selector in provenance ("moved from …").
- A retained revision whose annotations are all resolved/dismissed shows a muted
  "eligible for cleanup" note; cleanup itself is an explicit admin/CLI operation,
  not a viewer action.

---

## 7. Empty / loading / error states

| State    | Component      | Usage                                                        |
| -------- | -------------- | ------------------------------------------------------------ |
| Empty    | `.lore-empty`  | No documents in a section, no search results, no annotations |
| Loading  | `.lore-skel`   | Skeleton lines/blocks matching final layout                  |
| Loading  | `.lore-spinner`| Inline within buttons / small async actions                  |
| Error    | `.lore-error`  | Failed fetch, sync/embedding error, revision not found       |

Guidance: empty states are calm and instructive (icon + serif title + one-line
hint pointing at the CLI upload/watch path that would populate it). Errors are
specific and recoverable (message + retry). Skeletons mirror the real layout to
avoid layout shift.

---

## 8. Component inventory

**Frame:** `l-app`, `l-brand`, `l-header`, `l-sidebar`, `l-main`, `l-page`,
`l-doc`, `l-doc__rail`, `l-nav-section`, `l-nav-item`.

**Navigation & chrome:** `lore-project-select`, `lore-crumbs`,
`lore-page-head`, `lore-provenance`, theme toggle, `lore-attribution`.

**Controls:** `lore-btn` (primary/secondary/ghost/danger/sm/icon),
`lore-input`, `lore-textarea`, `lore-select`, `lore-search`, `lore-facet`,
`lore-facets`, `lore-segmented`, `lore-chip` (+ tag / removable),
`lore-source-badge`, `lore-status`.

**Surfaces:** `lore-card`, `lore-list` / `lore-row`, `lore-popover`,
`lore-menu-item`, `lore-modal` (+ backdrop), `lore-toast`.

**Search:** `lore-results`, `lore-result`, `lore-chunk`.

**Task:** `lore-task-meta`, `lore-deps`, `lore-dep-link`.

**Conversation:** `lore-convo`, `lore-msg`, `lore-thinking`.

**Structured:** `lore-struct`.

**Annotation:** `lore-anno-target`, `lore-anno-pop`, `lore-anno`,
`lore-revision`.

**State:** `lore-empty`, `lore-error`, `lore-skel`, `lore-spinner`.

**Prose contract:** `lore-prose` (+ `--wide`) and all bare semantic HTML within.

---

## 9. Design tokens (summary)

Full values in `site.css` and `tokens.md`. Highlights:

- **Type:** display `Fraunces`(→Iowan/Georgia), body `Public Sans`(→system),
  mono `JetBrains Mono`(→ui-monospace). Scale `--text-2xs`(11px)…`--text-4xl`(47px);
  reading measure 72ch.
- **Spacing:** 4px base, `--space-1`…`--space-9`.
- **Radii:** 3 / 5 / 8 / 12px + pill. **Borders:** hairline 1px, thick 2px.
- **Color:** warm-paper light / ink dark surfaces; verdigris `--accent`; ochre
  `--gold` for annotations; fixed per-source-type badge colors; annotation
  state colors (open=ochre, resolved=green, dismissed=grey) reused for tasks.
- **Elevation:** three soft paper shadows. **Motion:** 120/200/320ms with two
  eased curves; all motion respects `prefers-reduced-motion`.

---

## 10. Theming & accessibility

- Light is the default (`:root`); dark via `@media (prefers-color-scheme: dark)`
  **and** an explicit `:root[data-theme="dark"]`/`[data-theme="light"]` override
  so the header theme toggle wins in both directions. `color-scheme: light dark`
  set so form controls follow.
- Focus: visible `:focus-visible` ring on the accent; inputs get an accent ring
  + soft halo. Interactive targets are ≥28px tall.
- Color is never the sole signal: source and status also carry text labels /
  icons / shapes (badge glyph dots, dashed vs solid annotation underlines).
- Contrast: body/paper and body/ink pairings target WCAG AA; muted text is used
  only for secondary content.
- All disclosures (`lore-thinking`), popovers, and modals are keyboard operable;
  the thinking summary is a real `<summary>`.

---

## 11. Files

- `design/site.css` — application + briefing-contract stylesheet (tokens,
  themes, prose contract, app shell, all components).
- `design/design-spec.md` — this document.
- `design/tokens.md` — token quick reference for the implementing engineer.
