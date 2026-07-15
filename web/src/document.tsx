import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { api } from "./api";
import { PageError, PageLoading, useAttribution, useProject } from "./app";
import { BriefingViewToggle } from "./browse";
import type { Annotation, DocumentDetail, DocumentSummary, Json, RevisionSummary } from "./types";
import { taskStatusKey } from "./task-board";
import { linkTaxonomyText } from "./taxonomy";
import { allDocuments, documentHref, jsonString, relativeTime, shortHash, sourceBadgeType, sourceLabel } from "./utils";

type DraftTarget = { selector: Json; selectedQuote?: string; quotePrefix?: string; quoteSuffix?: string; structuralLocation: Json };

export function DocumentPage({ section }: { section: "tasks" | "notes" | "terms" | "briefings" | "repo" | "conversations" }) {
  const { project = "", id, taskId, termName, repo, branch, "*": path } = useParams();
  const { browse, loading: browseLoading, reload: reloadBrowse } = useProject();
  const [searchParams, setSearchParams] = useSearchParams();
  const location = useLocation();
  const navigate = useNavigate();
  const [detail, setDetail] = useState<DocumentDetail>();
  const [annotations, setAnnotations] = useState<Annotation[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [reloadToken, setReloadToken] = useState(0);
  const [toast, setToast] = useState("");
  const [draftTarget, setDraftTarget] = useState<DraftTarget>();
  const document = useMemo(() => resolveDocument(browse, section, { id, taskId, termName, repo, branch, path }), [browse, section, id, taskId, termName, repo, branch, path]);

  useEffect(() => {
    if (!document) { if (!browseLoading && browse) { setError("Document not found in this project."); setLoading(false); } return; }
    let active = true; setLoading(true); setError(undefined);
    Promise.all([api.document(project, document.id), api.annotations(project, document.id)]).then(async ([current, records]) => {
      if (!active) return;
      let requestedHash = searchParams.get("rev");
      if (!requestedHash) {
        const deepLinked = records.find((item) => item.id === searchParams.get("anno"));
        requestedHash = current.revisions.find((item) => item.id === deepLinked?.revisionIdentity)?.contentHash ?? null;
      }
      if (requestedHash && requestedHash !== current.contentHash) {
        const revision = current.revisions.find((item) => item.contentHash === requestedHash || item.id === requestedHash);
        if (!revision) throw new Error("The requested retained revision is no longer available.");
        const prior = await api.revision(project, document.id, revision.id);
        current = { ...current, revisionId: prior.id, contentHash: prior.contentHash, normalizedText: prior.normalizedText, renderedContent: prior.renderedContent, renderer: prior.renderer, metadata: prior.metadata, provenance: prior.provenance };
      }
      if (active) { setDetail(current); setAnnotations(records); }
    }).catch((reason: Error) => active && setError(reason.message)).finally(() => active && setLoading(false));
    return () => { active = false; };
  }, [project, document, searchParams, reloadToken, browseLoading, browse]);

  const reload = useCallback(() => setReloadToken((value) => value + 1), []);
  if (loading || browseLoading) return <PageLoading />;
  if (error || !detail || !document) return <PageError message={error ?? "Document unavailable."} retry={reload} />;
  const metadata = detail.metadata ?? {};
  const retained = detail.revisions.filter((revision) => !revision.current && revision.annotationCount > 0);
  const isPrior = detail.revisionId !== document.revisionId;
  const title = detail.title;
  const titleAnnotation = annotations.find((item) => item.revisionIdentity === detail.revisionId && item.selector.field === "title");
  const copyLink = async () => { await navigator.clipboard.writeText(window.location.href); setToast("Link copied"); setTimeout(() => setToast(""), 1800); };
  const currentHash = detail.revisions.find((item) => item.current)?.contentHash ?? detail.contentHash;
  const switchRevision = (hash: string) => { const next = new URLSearchParams(searchParams); hash && hash !== currentHash ? next.set("rev", hash) : next.delete("rev"); next.delete("anno"); setSearchParams(next); };

  return <div className="l-page">
    <nav className="lore-crumbs" aria-label="Breadcrumb"><Link to={`/${project}`}>{browse?.project.name}</Link><span className="lore-crumbs__sep">/</span><Link to={`/${project}/${section}`}>{sectionLabel(section)}</Link><span className="lore-crumbs__sep">/</span><span>{title}</span></nav>
    <header className="lore-page-head"><div className="lore-page-head__row"><div><div className="lore-page-head__title-row"><span className="lore-source-badge" data-type={sourceBadgeType(detail.sourceType)}>{section === "terms" ? "Term" : sourceLabel(detail.sourceType)}</span>{detail.sourceType === "task" && <span className="task-number">#{detail.sourceIdentity}</span>}<h1 className="lore-page-head__title">{title}</h1>{detail.sourceType === "task" && <span className="lore-status" data-state={jsonString(metadata.status) ?? "todo"}>{jsonString(metadata.status) ?? "todo"}</span>}</div></div><div className="lore-page-head__actions">
      {detail.sourceType === "task" && <button className={`lore-btn lore-btn--ghost lore-btn--icon task-title-target lore-anno-target ${titleAnnotation?.id === searchParams.get("anno") ? "is-active" : ""}`} data-state={titleAnnotation?.status} aria-label="Annotate task title" onClick={() => setDraftTarget({ selector: { kind: "task-field", field: "title" }, structuralLocation: { taskField: "title" } })}>＋</button>}
      {detail.sourceType === "briefing" && <BriefingViewToggle home={browse?.briefings.find((item) => item.briefingHome)} project={project} view={document.briefingHome ? "home" : undefined} />}
      {retained.length > 0 && <label className="lore-revision"><span className="lore-revision__dot" /><span className="lore-visually-hidden">Revision</span><select aria-label="Revision" value={detail.contentHash} onChange={(event) => switchRevision(event.target.value)}><option value={currentHash}>Current</option>{retained.map((revision) => <option value={revision.contentHash} key={revision.id}>{shortHash(revision.contentHash)} · {new Date(revision.createdAt).toLocaleDateString()} · {revision.annotationCount} annotations</option>)}</select></label>}
      <button className="lore-btn lore-btn--secondary lore-btn--sm" onClick={copyLink}>Copy link</button>
    </div></div>
    <div className="lore-provenance"><span className="lore-provenance__item">{detail.sourceInstance}</span>{provenanceItems(detail).map((item) => <span className="lore-provenance__item" key={item}>{item}</span>)}<span className="lore-provenance__item lore-mono">{shortHash(detail.contentHash)}</span><span className="lore-provenance__item">Synced {relativeTime(detail.updatedAt)}</span></div>
    {detail.sourceType === "briefing" && <BriefingSettings project={project} document={document} reload={reloadBrowse} onMessage={(message) => { setToast(message); setTimeout(() => setToast(""), 1800); }} />}
    {isPrior && <p className="search-warning">◉ Retained revision. You are reading the immutable content that received these annotations.</p>}
    </header>
    {detail.sourceType === "task" && <TaskMetadata detail={detail} project={project} annotations={annotations.filter((item) => item.revisionIdentity === detail.revisionId)} activeId={searchParams.get("anno") ?? ""} onTarget={setDraftTarget} />}
    {detail.renderer === "markdown" && detail.sourceType !== "task" && Object.keys(detail.metadata).length > 0 && <FrontMatter metadata={detail.metadata} />}
    <div className={detail.sourceType === "briefing" ? "brief-detail-layout" : undefined}>
      {detail.sourceType === "briefing" && browse && <BriefSidebar project={project} current={detail.id} documents={browse.briefings} />}
      <div className="brief-detail-main"><div className="l-doc"><DocumentContent project={project} detail={detail} tags={browse?.tags ?? []} terms={browse?.terms ?? []} annotations={annotations} activeAnnotation={searchParams.get("anno") ?? ""} onTarget={setDraftTarget} />
        <aside className="lore-doc__rail"><AnnotationRail project={project} detail={detail} revisions={detail.revisions} annotations={annotations} activeId={searchParams.get("anno") ?? ""} onActive={(annotation) => { const next = new URLSearchParams(searchParams); next.set("anno", annotation.id); setSearchParams(next); }} onChanged={reload} draftTarget={draftTarget} clearDraft={() => setDraftTarget(undefined)} /></aside>
      </div></div>
    </div>
    {detail.sourceType === "task" && <TaskRelationships detail={detail} project={project} browseDocuments={browse ? allDocuments(browse) : []} />}
    {detail.sourceType === "note" && <NoteRelationships detail={detail} project={project} browseDocuments={browse ? allDocuments(browse) : []} />}
    {isPrior && annotations.filter((item) => item.revisionIdentity === detail.revisionId).every((item) => item.status !== "open") && <p className="lore-muted">This retained revision is eligible for administrative cleanup.</p>}
    {detail.terms.length > 0 && <TermsFooter project={project} terms={detail.terms} />}
    {toast && <div className="lore-toast copy-feedback" role="status">✓ {toast}</div>}
  </div>;
}

function resolveDocument(browse: ReturnType<typeof useProject>["browse"], section: string, params: { id?: string; taskId?: string; termName?: string; repo?: string; branch?: string; path?: string }): DocumentSummary | undefined {
  if (!browse) return undefined;
  if (section === "tasks") return browse.tasks.find((item) => item.sourceIdentity === params.taskId || item.id === params.taskId);
  if (section === "notes") return browse.notes.find((item) => item.id === params.id || item.sourceIdentity === params.id);
  if (section === "terms") {
    const definition = browse.terms.find((item) => item.name === params.termName)?.definitionDocumentId;
    return allDocuments(browse).find((item) => item.id === definition);
  }
  if (section === "briefings") return browse.briefings.find((item) => item.id === params.id || item.sourceIdentity === params.id);
  if (section === "conversations") return browse.conversations.find((item) => item.id === params.id || item.sourceIdentity === params.id);
  return browse.repositories.flatMap((group) => group.documents).find((item) => jsonString(item.metadata.repository) === params.repo && jsonString(item.metadata.branch) === params.branch && jsonString(item.metadata.path) === params.path);
}

function sectionLabel(section: string) { return section === "repo" ? "Repository" : section[0].toUpperCase() + section.slice(1); }

function provenanceItems(detail: DocumentDetail): string[] {
  const values = [jsonString(detail.provenance.path), jsonString(detail.metadata.repository), jsonString(detail.metadata.branch), jsonString(detail.metadata.provider), jsonString(detail.metadata.sessionId), jsonString(detail.metadata.workingDirectory), jsonString(detail.metadata.stylesheetContract)];
  return values.filter((value): value is string => Boolean(value));
}

function TaskMetadata({ detail, project, annotations, activeId, onTarget }: { detail: DocumentDetail; project: string; annotations: Annotation[]; activeId: string; onTarget: (target: DraftTarget) => void }) {
  const target = (field: string) => <button className="structural-target__add" aria-label={`Annotate task ${field}`} onClick={() => onTarget({ selector: { kind: "task-field", field }, structuralLocation: { taskField: field } })}>+</button>;
  const state = (field: string, value?: string) => { const annotation = annotations.find((item) => item.selector.field === field && (!value || item.selector.value === value)); return { className: `structural-target ${annotation ? "lore-anno-target" : ""} ${annotation?.id === activeId ? "is-active" : ""}`, "data-state": annotation?.status }; };
  return <div className="lore-task-meta task-meta-grid">
    <div {...state("identifier")}>{target("identifier")}<div className="lore-task-meta__k">Task identifier</div><div className="lore-task-meta__v lore-mono">{detail.sourceIdentity}</div></div>
    <div {...state("status")}>{target("status")}<div className="lore-task-meta__k">Status</div><div className="lore-task-meta__v"><span className="lore-status" data-state={taskStatusKey(jsonString(detail.metadata.status) ?? "todo")}>{jsonString(detail.metadata.status) ?? "todo"}</span></div></div>
    <div {...state("priority")}>{target("priority")}<div className="lore-task-meta__k">Priority</div><div className="lore-task-meta__v">{jsonString(detail.metadata.priority) ?? "—"}</div></div>
    <div className="structural-target"><div className="lore-task-meta__k">Tags</div><div className="lore-task-meta__v">{detail.tags.length ? detail.tags.map((tag) => { const annotation = annotations.find((item) => item.selector.field === "tag" && item.selector.value === tag); return <span className={`task-tag-target ${annotation ? "lore-anno-target" : ""} ${annotation?.id === activeId ? "is-active" : ""}`} data-state={annotation?.status} key={tag}><button className="tag-target__add" aria-label={`Annotate tag ${tag}`} onClick={() => onTarget({ selector: { kind: "task-field", field: "tag", value: tag }, structuralLocation: { taskField: "tag", value: tag } })}>＋</button><Link className="lore-chip lore-chip--tag" to={`/${project}/search?q=${encodeURIComponent(tag)}&sourceType=task&tag=${encodeURIComponent(tag)}`}>{tag}</Link></span>; }) : "—"}</div></div>
  </div>;
}

function FrontMatter({ metadata }: { metadata: Json }) { const entries = Object.entries(metadata).filter(([, value]) => typeof value === "string" || typeof value === "number" || Array.isArray(value)); return entries.length ? <dl className="lore-card lore-card--pad frontmatter">{entries.map(([key, value]) => <div key={key}><dt className="lore-task-meta__k">{key}</dt><dd className="lore-task-meta__v">{Array.isArray(value) ? value.join(", ") : String(value)}</dd></div>)}</dl> : null; }

function TaskRelationships({ detail, project, browseDocuments }: { detail: DocumentDetail; project: string; browseDocuments: DocumentSummary[] }) {
  const column = (direction: "dependency" | "dependent", title: string, arrow: string) => { const items = detail.relationships.filter((item) => item.direction === direction); return <div className="lore-deps__col"><h3>{title}</h3>{items.length ? items.map((item) => { const document = browseDocuments.find((candidate) => candidate.id === item.documentId); const status = jsonString(item.metadata.status) ?? "todo"; return document ? <Link className="lore-dep-link" to={documentHref(project, document)} key={item.documentId}><span className="lore-dep-link__arrow">{arrow}</span><span>{item.title}</span><span className="l-header__spacer" /><span className="lore-status" data-state={taskStatusKey(status)}>{status}</span></Link> : null; }) : <span className="empty-inline">None</span>}</div>; };
  return <section><h2>Task relationships</h2><div className="lore-deps">{column("dependency", "Depends on", "←")}{column("dependent", "Blocks / dependents", "→")}</div></section>;
}

function NoteRelationships({ detail, project, browseDocuments }: { detail: DocumentDetail; project: string; browseDocuments: DocumentSummary[] }) {
  const related = [...new Map(detail.relationships.filter((item) => item.type === "note-related-to").map((item) => browseDocuments.find((document) => document.id === item.documentId)).filter((document): document is DocumentSummary => Boolean(document)).map((document) => [document.id, document])).values()];
  if (!related.length) return null;
  return <section className="related-notes"><h2>Related notes</h2><DocumentLinks project={project} documents={related} /></section>;
}

function DocumentLinks({ project, documents }: { project: string; documents: DocumentSummary[] }) {
  return <div className="lore-list">{documents.map((document) => <Link className="lore-row" to={documentHref(project, document)} key={document.id}><span className="lore-source-badge" data-type={sourceBadgeType(document.sourceType)}>{sourceLabel(document.sourceType)}</span><span className="lore-row__title">{document.title}</span><span aria-hidden="true">›</span></Link>)}</div>;
}

function BriefingSettings({ project, document, reload, onMessage }: { project: string; document: DocumentSummary; reload: () => void; onMessage: (message: string) => void }) {
  const [category, setCategory] = useState(document.briefingCategory ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => setCategory(document.briefingCategory ?? ""), [document.id, document.briefingCategory]);
  const update = async (body: { category?: string; home?: boolean }, message: string) => {
    setSaving(true); setError("");
    try { await api.updateBriefing(project, document.id, body); reload(); onMessage(message); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Briefing setting update failed."); }
    finally { setSaving(false); }
  };
  const normalizedCategory = category.trim();
  const categoryChanged = normalizedCategory !== (document.briefingCategory ?? "");
  return <div className="briefing-settings" aria-label="Briefing settings">
    <label><span>Category</span><input className="lore-input" value={category} onChange={(event) => setCategory(event.target.value)} placeholder="Uncategorized" onKeyDown={(event) => { if (event.key === "Enter" && categoryChanged) update({ category: normalizedCategory }, "Category saved"); }} /></label>
    <button className="lore-btn lore-btn--secondary lore-btn--sm" disabled={saving || !categoryChanged} onClick={() => update({ category: normalizedCategory }, "Category saved")}>Save category</button>
    <button className="lore-btn lore-btn--ghost lore-btn--sm" aria-pressed={Boolean(document.briefingHome)} disabled={saving} onClick={() => update({ home: !document.briefingHome }, document.briefingHome ? "Home briefing cleared" : "Home briefing set")}>{document.briefingHome ? "Clear home" : "Set as home"}</button>
    {error && <span className="briefing-settings__error" role="alert">{error}</span>}
  </div>;
}

function BriefSidebar({ project, current, documents }: { project: string; current: string; documents: DocumentSummary[] }) {
  const grouped = documents.reduce((result, document) => {
    const category = document.briefingCategory?.trim() || "Uncategorized";
    result.set(category, [...(result.get(category) ?? []), document]);
    return result;
  }, new Map<string, DocumentSummary[]>());
  const groups = [...grouped]
    .sort(([left], [right]) => left.localeCompare(right));
  return <aside className="brief-sidebar" aria-label="Other briefings"><div className="task-facet-label">Briefings</div><nav>{groups.map(([category, categoryDocuments]) => <BriefCategory category={category} current={current} documents={categoryDocuments} project={project} key={category} />)}</nav></aside>;
}

function BriefCategory({ category, current, documents, project }: { category: string; current: string; documents: DocumentSummary[]; project: string }) {
  const containsCurrent = documents.some((document) => document.id === current);
  const [open, setOpen] = useState(containsCurrent);
  useEffect(() => { if (containsCurrent) setOpen(true); }, [containsCurrent]);
  return <details className="brief-category" open={open} onToggle={(event) => setOpen(event.currentTarget.open)}>
    <summary><span>{category}</span><span>{documents.length}</span></summary>
    <div>{[...documents].sort((left, right) => left.title.localeCompare(right.title)).map((document) => <Link className={document.id === current ? "is-active" : ""} aria-current={document.id === current ? "page" : undefined} to={documentHref(project, document)} key={document.id}>{document.briefingHome && <span className="brief-home-mark" aria-label="Home briefing">⌂</span>}{document.title}</Link>)}</div>
  </details>;
}

function TermsFooter({ project, terms }: { project: string; terms: DocumentDetail["terms"] }) {
  return <footer className="terms-footer"><h2>Terms</h2><div className="facet-stack">{terms.map((term) => <Link className={`lore-chip taxonomy-link taxonomy-link--term ${term.defined ? "" : "is-missing"}`} to={`/${project}/terms/${encodeURIComponent(term.name)}`} key={term.name}>{term.title}{!term.defined && <span aria-label="missing definition"> △</span>}</Link>)}</div></footer>;
}

function DocumentContent({ project, detail, tags, terms, annotations, activeAnnotation, onTarget }: { project: string; detail: DocumentDetail; tags: string[]; terms: NonNullable<ReturnType<typeof useProject>["browse"]>["terms"]; annotations: Annotation[]; activeAnnotation: string; onTarget: (target: DraftTarget) => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const [popover, setPopover] = useState<{ x: number; y: number; target: DraftTarget }>();
  const revisionAnnotations = annotations.filter((item) => item.revisionIdentity === detail.revisionId);
  useEffect(() => {
    const root = ref.current; if (!root) return;
    root.innerHTML = detail.renderedContent || `<div class="lore-empty"><div class="lore-empty__title">No rendered body</div><div class="lore-empty__hint">The source revision contains no displayable content.</div></div>`;
    enhanceRenderedContent(root, project, detail, tags, terms, revisionAnnotations, activeAnnotation, onTarget);
  }, [project, detail, tags, terms, revisionAnnotations, activeAnnotation]);
  const selected = (event: React.MouseEvent) => {
    const root = ref.current; const selection = getSelection(); if (!root || !selection || selection.isCollapsed || !selection.rangeCount || !root.contains(selection.anchorNode)) { setPopover(undefined); return; }
    const quote = selection.toString().trim(); if (!quote) return;
    const full = root.textContent ?? ""; const offset = full.indexOf(quote); const prefix = full.slice(Math.max(0, offset - 48), offset); const suffix = full.slice(offset + quote.length, offset + quote.length + 48);
    const rect = selection.getRangeAt(0).getBoundingClientRect(); const container = root.parentElement?.getBoundingClientRect() ?? { left: 0, top: 0 };
    setPopover({ x: rect.left - container.left, y: rect.top - container.top - 42, target: { selector: { kind: "text-quote", exact: quote, prefix, suffix }, selectedQuote: quote, quotePrefix: prefix, quoteSuffix: suffix, structuralLocation: structuralLocationFor(selection.anchorNode) } });
    event.stopPropagation();
  };
  return <article className="document-surface"><div ref={ref} className="document-content lore-prose" onMouseUp={selected} />{popover && <div className="lore-anno-pop" style={{ left: popover.x, top: popover.y }}><button onClick={() => { onTarget(popover.target); setPopover(undefined); getSelection()?.removeAllRanges(); }}>＋ Annotate</button></div>}</article>;
}

function enhanceRenderedContent(root: HTMLElement, project: string, detail: DocumentDetail, tags: string[], terms: NonNullable<ReturnType<typeof useProject>["browse"]>["terms"], annotations: Annotation[], activeId: string, open: (target: DraftTarget) => void) {
  root.querySelectorAll("table").forEach((table) => { if (!table.parentElement?.classList.contains("table-scroll")) { const wrapper = document.createElement("div"); wrapper.className = "table-scroll"; table.before(wrapper); wrapper.append(table); } });
  root.querySelectorAll(".lore-msg").forEach((message) => { const header = message.querySelector("header"); if (header) { header.className = "lore-msg__role"; const avatar = document.createElement("span"); avatar.className = "lore-msg__avatar"; avatar.textContent = (message.getAttribute("data-role") ?? "M")[0].toUpperCase(); message.prepend(avatar); } });
  root.querySelectorAll<HTMLElement>("h1[id],h2[id],h3[id],h4[id],h5[id],h6[id],[data-yaml-path],.lore-msg[id],.lore-thinking[id]").forEach((element) => {
    if (element.closest("[data-yaml-path]") && !element.hasAttribute("data-yaml-path") && !element.matches(".lore-msg,.lore-thinking")) return;
    element.classList.add("structural-target"); const button = document.createElement("button"); button.className = "structural-target__add"; button.type = "button"; button.title = "Annotate this section"; button.setAttribute("aria-label", `Annotate ${element.textContent?.trim().slice(0, 60) || "section"}`);
    button.onclick = (event) => { event.stopPropagation(); const selector = selectorFor(element, detail.sourceType); open({ selector, structuralLocation: selector }); };
    element.prepend(button);
  });
  for (const annotation of annotations) {
    const selector = annotation.selector; let target: HTMLElement | null = null;
    if (jsonString(selector.elementId)) target = root.querySelector(`#${CSS.escape(jsonString(selector.elementId)!)}`);
    else if (jsonString(selector.messageId)) target = root.querySelector(`#${CSS.escape(jsonString(selector.messageId)!)}`);
    else if (jsonString(selector.yamlPath)) target = root.querySelector(`[data-yaml-path="${CSS.escape(jsonString(selector.yamlPath)!)}"]`);
    else if (Array.isArray(selector.headingPath)) { const id = String(selector.headingPath.at(-1) ?? "").toLowerCase().replace(/[^a-z0-9]+/g, "-"); target = root.querySelector(`#${CSS.escape(id)}`); }
    if (!target && annotation.selectedQuote) target = wrapFirstQuote(root, annotation.selectedQuote);
    if (target) { target.classList.add("lore-anno-target"); target.dataset.state = annotation.status; target.dataset.annotationId = annotation.id; if (annotation.id === activeId) { target.classList.add("is-active"); setTimeout(() => target?.scrollIntoView({ block: "center", behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth" }), 0); } }
  }
  linkTaxonomyText(root, project, tags, terms);
}

function selectorFor(element: HTMLElement, sourceType: string): Json {
  if (element.dataset.yamlPath) return { kind: "yaml-property", yamlPath: element.dataset.yamlPath };
  if (element.matches(".lore-msg,.lore-thinking")) return { kind: "conversation-message", messageId: element.id };
  if (sourceType === "briefing") return { kind: "html-element", elementId: element.id };
  return { kind: sourceType === "task" ? "task-field" : "heading-path", headingPath: [element.textContent?.replace(/^\+/, "").trim() ?? element.id], elementId: element.id };
}

function structuralLocationFor(node: Node | null): Json { const element = node instanceof Element ? node : node?.parentElement; const target = element?.closest<HTMLElement>("[data-yaml-path], [id]"); if (!target) return {}; return target.dataset.yamlPath ? { yamlPath: target.dataset.yamlPath } : target.matches(".lore-msg,.lore-thinking") ? { messageId: target.id } : { elementId: target.id }; }

function wrapFirstQuote(root: HTMLElement, quote: string): HTMLElement | null { const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT); let node: Node | null; while ((node = walker.nextNode())) { const text = node.textContent ?? ""; const index = text.indexOf(quote); if (index >= 0 && node.parentElement && !node.parentElement.closest("button")) { const range = document.createRange(); range.setStart(node, index); range.setEnd(node, index + quote.length); const mark = document.createElement("mark"); try { range.surroundContents(mark); return mark; } catch { return node.parentElement; } } } return null; }

function AnnotationRail({ project, detail, revisions, annotations, activeId, onActive, onChanged, draftTarget, clearDraft }: { project: string; detail: DocumentDetail; revisions: RevisionSummary[]; annotations: Annotation[]; activeId: string; onActive: (annotation: Annotation) => void; onChanged: () => void; draftTarget?: DraftTarget; clearDraft: () => void }) {
  const [attribution, setAttribution] = useAttribution(); const [body, setBody] = useState(""); const [filter, setFilter] = useState("all"); const [saving, setSaving] = useState(false); const [error, setError] = useState("");
  const visible = annotations.filter((item) => item.revisionIdentity === detail.revisionId && (filter === "all" || item.status === filter)); const openCount = annotations.filter((item) => item.revisionIdentity === detail.revisionId && item.status === "open").length;
  const mutate = async (operation: () => Promise<unknown>) => { if (!attribution.trim()) { setError("Enter your attribution name before saving a change."); return; } setSaving(true); setError(""); try { await operation(); onChanged(); } catch (reason) { setError(reason instanceof Error ? reason.message : "Annotation operation failed."); } finally { setSaving(false); } };
  const create = () => draftTarget && mutate(async () => { await api.createAnnotation(project, { documentId: detail.id, revisionId: detail.revisionId, body, attributedUsername: attribution, originatingOperation: "web-create", selector: draftTarget.selector, selectedQuote: draftTarget.selectedQuote, quotePrefix: draftTarget.quotePrefix, quoteSuffix: draftTarget.quoteSuffix, structuralLocation: draftTarget.structuralLocation, originalContentHash: detail.contentHash }); setBody(""); clearDraft(); });
  return <><div className="lore-rail-head"><span className="lore-rail-head__title">Annotations</span><span className="lore-rail-head__count">{openCount} open</span><label className="rail-filter"><span className="lore-visually-hidden">Annotation state</span><select className="lore-select" value={filter} onChange={(event) => setFilter(event.target.value)}><option value="all">All</option><option value="open">Open</option><option value="resolved">Resolved</option><option value="dismissed">Dismissed</option></select></label></div>
    {error && <div className="lore-error" role="alert">{error}</div>}
    {draftTarget && <div className="lore-card lore-card--pad annotation-composer"><h3>New annotation</h3>{draftTarget.selectedQuote && <blockquote className="lore-anno__quote">{draftTarget.selectedQuote}</blockquote>}<label htmlFor="annotation-author">Attribution</label><input id="annotation-author" className="lore-input" value={attribution} onChange={(event) => setAttribution(event.target.value)} placeholder="Your name" /><label htmlFor="annotation-body">Note</label><textarea id="annotation-body" className="lore-textarea" value={body} onChange={(event) => setBody(event.target.value)} autoFocus /><div className="annotation-composer__actions"><button className="lore-btn lore-btn--ghost lore-btn--sm" onClick={clearDraft}>Cancel</button><button className="lore-btn lore-btn--primary lore-btn--sm" onClick={create} disabled={saving || !body.trim()}>{saving ? <span className="lore-spinner" /> : "Save"}</button></div></div>}
    {visible.map((annotation) => <AnnotationCard key={annotation.id} project={project} annotation={annotation} active={annotation.id === activeId} revisions={revisions} attribution={attribution} onActive={() => onActive(annotation)} mutate={mutate} />)}
    {!draftTarget && visible.length === 0 && <div className="lore-empty"><div className="lore-empty__title">No annotations</div><div className="lore-empty__hint">Select text or use a section’s + marker to leave one.</div></div>}</>;
}

function AnnotationCard({ project, annotation, active, revisions, attribution, onActive, mutate }: { project: string; annotation: Annotation; active: boolean; revisions: RevisionSummary[]; attribution: string; onActive: () => void; mutate: (operation: () => Promise<unknown>) => Promise<void> }) {
  const [targetRevision, setTargetRevision] = useState(""); const eligible = revisions.filter((item) => item.id !== annotation.revisionIdentity);
  const update = (status: Annotation["status"]) => mutate(() => api.updateAnnotation(project, annotation.id, { status, attributedUsername: attribution }));
  const retarget = (operation: "copy" | "move") => { const revision = eligible.find((item) => item.id === targetRevision); if (!revision) return; return mutate(() => api.retargetAnnotation(project, annotation.id, operation, { targetRevisionId: revision.id, attributedUsername: attribution, selector: annotation.selector, selectedQuote: annotation.selectedQuote, quotePrefix: annotation.quotePrefix, quoteSuffix: annotation.quoteSuffix, structuralLocation: annotation.structuralLocation, originalContentHash: revision.contentHash })); };
  return <article className={`lore-anno ${active ? "is-active" : ""}`} data-state={annotation.status} tabIndex={0} onKeyDown={(event) => { if (event.key === "Enter") onActive(); }} onClick={onActive}><div className="lore-anno__top"><span className="lore-status" data-state={annotation.status}>{annotation.status}</span><span className="lore-anno__author">{annotation.attributedUsername}</span><time className="lore-anno__time" dateTime={annotation.updatedAt}>{relativeTime(annotation.updatedAt)}</time></div>{annotation.selectedQuote && <blockquote className="lore-anno__quote">{annotation.selectedQuote}</blockquote>}<p className="lore-anno__body">{annotation.body}</p>{annotation.copiedFromAnnotationId && <div className="lore-anno__prov">↪ copied from {annotation.copiedFromAnnotationId.slice(0, 8)}</div>}{annotation.priorTarget && <div className="lore-anno__prov">↪ moved from an earlier revision</div>}<div className="lore-anno__actions" onClick={(event) => event.stopPropagation()}>{annotation.status === "open" ? <><button className="lore-btn lore-btn--ghost lore-btn--sm" onClick={() => update("resolved")}>Resolve</button><button className="lore-btn lore-btn--ghost lore-btn--sm" onClick={() => update("dismissed")}>Dismiss</button></> : <button className="lore-btn lore-btn--ghost lore-btn--sm" onClick={() => update("open")}>Reopen</button>}</div>{eligible.length > 0 && <div className="retarget-controls" onClick={(event) => event.stopPropagation()}><select className="lore-select" aria-label="Target revision" value={targetRevision} onChange={(event) => setTargetRevision(event.target.value)}><option value="">Revision…</option>{eligible.map((item) => <option value={item.id} key={item.id}>{item.current ? "Current" : shortHash(item.contentHash)}</option>)}</select><button className="lore-btn lore-btn--ghost lore-btn--sm" disabled={!targetRevision} onClick={() => retarget("copy")}>Copy</button><button className="lore-btn lore-btn--ghost lore-btn--sm" disabled={!targetRevision} onClick={() => retarget("move")}>Move</button></div>}</article>;
}
