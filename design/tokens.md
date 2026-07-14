# Lore — Design Tokens

Quick reference for the implementing engineer. All tokens are CSS custom
properties defined in [`site.css`](./site.css). Light values are the default on
`:root`; dark values come from `@media (prefers-color-scheme: dark)` and the
explicit `:root[data-theme="dark"]` override. Toggle themes by stamping
`data-theme="light" | "dark"` on the root element; omit it to follow the OS.

Consume tokens with `var(--token)`. Do not hard-code hex, px, or font names in
component code — add or adjust a token instead.

## Typography

| Token | Value / purpose |
| --- | --- |
| `--font-display` | Fraunces → Iowan Old Style / Palatino / Georgia (titles, prose headings) |
| `--font-body` | Public Sans → system humanist sans (interface + prose body) |
| `--font-mono` | JetBrains Mono → ui-monospace (code, ids, hashes, numbers) |
| `--text-2xs` … `--text-4xl` | 11, 12, 13, 15(base), 16(prose), 19, 23, 29, 36, 47 px |
| `--leading-tight/snug/normal/relaxed` | 1.22 / 1.40 / 1.60 / 1.72 |
| `--tracking-tight/wide/caps` | -0.011em / 0.02em / 0.08em (eyebrows) |

## Spacing (4px base) & shape

| Token | Value |
| --- | --- |
| `--space-1` … `--space-9` | 4, 8, 12, 16, 24, 32, 48, 64, 96 px |
| `--radius-xs/sm/md/lg/pill` | 3 / 5 / 8 / 12 / 999 px |
| `--border-hair` / `--border-thick` | 1px / 2px |
| `--shadow-1/2/3` | soft paper elevations (low → high) |

## Motion & layering

| Token | Value |
| --- | --- |
| `--dur-fast/med/slow` | 120 / 200 / 320 ms |
| `--ease-standard` | cubic-bezier(0.2, 0.6, 0.2, 1) |
| `--ease-emphasized` | cubic-bezier(0.2, 0.8, 0.2, 1) |
| `--z-header/sidebar/popover/drawer/modal/toast` | 100 / 90 / 400 / 500 / 600 / 700 |

All transitions/animations collapse under `prefers-reduced-motion: reduce`.

## Color — semantic roles

Values differ per theme; use the role, not the hex.

### Surfaces & text
| Token | Role |
| --- | --- |
| `--bg-canvas` | app backdrop (warm paper / ink) |
| `--bg-surface`, `--bg-surface-2`, `--bg-surface-3` | ascending panel elevations |
| `--bg-inverse` | inverted surface (toasts, selection popover) |
| `--text-strong` | headings / emphasis |
| `--text-body` | body copy |
| `--text-muted` | secondary / meta |
| `--text-faint` | placeholder / disabled |
| `--text-on-accent`, `--text-inverse` | text on accent / on inverse surfaces |
| `--border-subtle/default/strong` | hairline rules → strong dividers |

### Accent & secondary
| Token | Role |
| --- | --- |
| `--accent`, `--accent-hover`, `--accent-active` | verdigris primary (buttons, links, active nav) |
| `--accent-soft`, `--accent-soft-border`, `--accent-text` | tinted fills / accent text on paper |
| `--gold`, `--gold-soft`, `--gold-soft-border`, `--gold-text` | ochre — annotations, retained revisions, highlights |

### Status (shared by annotations and tasks)
| Token | Role |
| --- | --- |
| `--status-open` / `--status-open-soft` | open annotation (ochre) |
| `--status-resolved` / `--status-resolved-soft` | resolved annotation / done task (green) |
| `--status-dismissed` / `--status-dismissed-soft` | dismissed annotation (grey) |
| `--task-backlog/todo/in-progress/review/blocked/done/archived` (+ `-soft`) | data-driven kanban lane hues, keyed by `[data-status]` on board columns, status badges, and card priority ticks; `--task-doing` aliases `in-progress` |
| `--task-status-fallback` (+ `-soft`) | hue for unrecognized custom board statuses |
| `--danger` / `--danger-soft` | destructive / error |
| `--info` / `--info-soft` | informational / task source |

### Annotation & search overlays
| Token | Role |
| --- | --- |
| `--anno-open` / `--anno-open-underline` | open highlight fill + underline |
| `--anno-resolved` / `--anno-resolved-underline` | resolved highlight |
| `--anno-dismissed` | dismissed highlight (dotted) |
| `--anno-active-ring` | selected-annotation focus ring |
| `--mark-bg` / `--mark-text` | search-term `<mark>` highlight |

### Code & syntax
| Token | Role |
| --- | --- |
| `--code-bg`, `--code-border`, `--code-text`, `--kbd-bg` | code block / inline code / keycap |
| `--syn-comment/keyword/string/number/function/type/punctuation/variable` | map your highlighter's tokens to these; also aliased to `.hljs-*` |

### Source-type badge colors
Fixed per type, applied via `.lore-source-badge[data-type]`:
`task`=info-blue, `note`=verdigris, `briefing`=ochre, `repo`=neutral,
`conversation`=green. Reuse the same hues for sidebar icons and result headers.

### Focus & selection
`--focus-ring`, `--focus-ring-offset`, `--selection-bg`, `--selection-text`.

## Usage rules

1. One accent (verdigris). Ochre means "annotation / retained history" only.
2. Source type and status must carry a text/shape signal in addition to color.
3. Prose (`.lore-prose`) is the briefing contract — never rely on briefing CSS;
   test any change against a bare semantic-HTML briefing body.
4. Prefer hairline borders + soft shadows over heavy boxes to keep the calm feel.
