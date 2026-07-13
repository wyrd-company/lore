---
name: lore-briefing-author
description: Author trusted HTML briefing bodies that conform to Lore's .lore-prose contract.
---

# Lore briefing authoring

Produce one self-contained HTML body or body fragment for insertion inside a
`.lore-prose` container.

- Use semantic HTML: one `h1`, ordered `h2`–`h6` sections, paragraphs, lists,
  blockquotes, tables, `pre`/`code`, figures, and captions.
- Give headings and annotatable structural elements stable, descriptive `id`
  attributes. Do not derive IDs from presentation or position.
- Do not include authored CSS, external stylesheets, scripts, or head resources.
  Lore supplies `site.css`; head content is ignored during ingestion.
- Use data URLs for images and inline SVG for diagrams. Mermaid must be
  pre-rendered to inline SVG.
- Keep tables semantic with `thead` and `tbody`. Use language classes such as
  `language-go` on code blocks when known.
- Return valid trusted HTML only. Do not wrap the result in a Markdown fence.

Before handoff, verify the fragment reads correctly when wrapped exactly as:

```html
<article class="lore-prose">
  <!-- briefing body -->
</article>
```
