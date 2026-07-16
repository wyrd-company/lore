import { useMemo, useState } from "react";
import { Link, Navigate, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { PageError, PageLoading, useProject } from "./app";
import { FilterGroup, FilterPanel } from "./filters";
import type { DocumentSummary, SourceType } from "./types";
import { documentHref, jsonString, sourceBadgeType, sourceLabel } from "./utils";

export function OverviewPage() {
  const { project = "" } = useParams();
  const { browse, loading, error, reload } = useProject();
  if (loading) return <PageLoading />;
  if (error || !browse) return <PageError message={error ?? "Project not found."} retry={reload} />;
  const cards = [
    ["Tasks", "tasks", "task", browse.tasks.length], ["Notes", "notes", "note", browse.notes.length],
    ["Briefings", "briefings", "briefing", browse.briefings.length], ["Repository documents", "repo", "repository", browse.repositories.reduce((sum, group) => sum + group.documents.length, 0)],
    ["Conversations", "conversations", "conversation", browse.conversations.length],
  ] as const;
  const recent = [...browse.tasks, ...browse.notes, ...browse.briefings, ...browse.repositories.flatMap((group) => group.documents), ...browse.conversations]
    .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)).slice(0, 7);
  return <div className="l-page">
    <span className="page-kicker">Project archive</span><h1 className="lore-page-head__title">{browse.project.name}</h1>
    <p className="lore-muted">{browse.project.documentCount} documents held across {browse.project.sourceCount} source instances.</p>
    <div className="overview-grid">{cards.map(([label, path, type, count]) => <Link className="lore-card lore-card--pad lore-card--interactive overview-card" to={`/${project}/${path}`} key={path}><span className="lore-source-badge" data-type={sourceBadgeType(type)}>{sourceLabel(type)}</span><strong className="overview-card__count">{count}</strong><span className="lore-muted">{label}</span></Link>)}
      <Link className="lore-card lore-card--pad lore-card--interactive overview-card" to={`/${project}/terms`}><span className="lore-source-badge">Term</span><strong className="overview-card__count">{browse.terms.filter((term) => term.defined).length}</strong><span className="lore-muted">Terms</span></Link>
    </div>
    <section className="repo-group"><div className="lore-page-head__row"><h2>Recently synchronized</h2><Link to={`/${project}/search`} className="lore-btn lore-btn--ghost lore-btn--sm section-action">Search archive →</Link></div>
      {recent.length ? <DocumentList documents={recent} project={project} /> : <EmptyState section="this project" />}
    </section>
  </div>;
}

type SourceSection = "notes" | "conversations";
const listConfig: Record<SourceSection, { title: string; hint: string; type: SourceType }> = {
  notes: { title: "Notes", hint: "Upload a Mnemonic notes directory with lore upload notes.", type: "note" },
  conversations: { title: "Conversations", hint: "Upload Claude or Codex sessions with lore upload conversations.", type: "conversation" },
};

export function SourceIndexPage({ section }: { section: SourceSection }) {
  const { project = "" } = useParams();
  const { browse, loading, error, reload } = useProject();
  const [filter, setFilter] = useState("");
  const [params, setParams] = useSearchParams();
  const config = listConfig[section];
  const documents: DocumentSummary[] | undefined = browse?.[section];
  const filtered = useMemo(() => (documents ?? []).filter((document) => {
    const matchesText = `${document.title} ${document.tags.join(" ")}`.toLowerCase().includes(filter.toLowerCase());
    if (section !== "notes") return matchesText;
    const selected = (key: string) => params.getAll(key);
    const role = jsonString(document.metadata.role) ?? "";
    const lifecycle = jsonString(document.metadata.lifecycle) ?? "";
    const projectName = jsonString(document.metadata.projectName) ?? "";
    return matchesText
      && (!selected("role").length || selected("role").includes(role))
      && (!selected("lifecycle").length || selected("lifecycle").includes(lifecycle))
      && (!selected("projectName").length || selected("projectName").includes(projectName))
      && (!selected("tag").length || selected("tag").every((tag) => document.tags.includes(tag)));
  }).sort((left, right) => compareNotes(left, right, params.get("sort") ?? "updatedAt")), [documents, filter, params, section]);
  const toggleNoteFacet = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    const values = next.getAll(key);
    next.delete(key);
    for (const candidate of values.includes(value) ? values.filter((item) => item !== value) : [...values, value]) next.append(key, candidate);
    setParams(next, { replace: true });
  };
  if (loading) return <PageLoading />;
  if (error || !browse) return <PageError message={error ?? "Section unavailable."} retry={reload} />;
  return <div className="l-page">
    <div className="lore-page-head"><span className="page-kicker">{sourceLabel(config.type)}</span><h1 className="lore-page-head__title">{config.title}</h1><p className="lore-muted">{documents?.length ?? 0} documents in {browse.project.name}</p></div>
    <FilterPanel activeCount={(filter ? 1 : 0) + ["role", "tag", "lifecycle", "projectName"].reduce((count, key) => count + params.getAll(key).length, 0)}>
      <FilterGroup title="Text"><input className="lore-input" value={filter} onChange={(event) => setFilter(event.target.value)} placeholder={`Filter ${config.title.toLowerCase()}…`} aria-label={`Filter ${config.title.toLowerCase()}`} /></FilterGroup>
      {section === "notes" && <FilterGroup title="Sort"><select className="lore-select" aria-label="Sort" value={params.get("sort") ?? "updatedAt"} onChange={(event) => { const next = new URLSearchParams(params); next.set("sort", event.target.value); setParams(next, { replace: true }); }}><option value="updatedAt">Updated</option><option value="createdAt">Created</option><option value="title">Title</option></select></FilterGroup>}
      {section === "notes" && documents && <NoteFacets documents={documents} params={params} toggle={toggleNoteFacet} />}
    </FilterPanel>
    {filtered.length ? <DocumentList documents={filtered} project={project} /> : <EmptyState section={config.title.toLowerCase()} hint={config.hint} />}
  </div>;
}

export function BriefingsPage() {
  const { project = "" } = useParams();
  const { browse, loading, error, reload } = useProject();
  const [params, setParams] = useSearchParams();
  if (loading) return <PageLoading />;
  if (error || !browse) return <PageError message={error ?? "Briefings unavailable."} retry={reload} />;
  const home = browse.briefings.find((document) => document.briefingHome);
  const requestedView = params.get("view");
  if (home && requestedView !== "list") return <Navigate replace to={documentHref(project, home)} />;
  return <div className="l-page">
    <div className="lore-page-head">
      <div className="lore-page-head__row">
        <div><span className="page-kicker">Briefing</span><h1 className="lore-page-head__title">Briefings</h1></div>
        <BriefingViewToggle home={home} project={project} view="list" onList={() => { const next = new URLSearchParams(params); next.set("view", "list"); setParams(next, { replace: true }); }} />
      </div>
      <p className="lore-muted">{browse.briefings.length} documents in {browse.project.name}</p>
    </div>
    {browse.briefings.length ? <DocumentList documents={browse.briefings} project={project} /> : <EmptyState section="briefings" hint="Upload trusted HTML with lore upload briefing." />}
  </div>;
}

export function BriefingViewToggle({ home, project, view, onList }: { home?: DocumentSummary; project: string; view?: "home" | "list"; onList?: () => void }) {
  const navigate = useNavigate();
  return <div className="lore-segmented briefing-view-toggle" role="group" aria-label="Briefings view">
    <button type="button" aria-pressed={view === "home"} disabled={!home} title={home ? `Open ${home.title}` : "No home briefing is set"} onClick={() => home && navigate(documentHref(project, home))}>Home</button>
    <button type="button" aria-pressed={view === "list"} onClick={() => onList ? onList() : navigate(`/${project}/briefings?view=list`)}>List</button>
  </div>;
}

export function RepositoryIndexPage() {
  const { project = "" } = useParams();
  const { browse, loading, error, reload } = useProject();
  const [params, setParams] = useSearchParams();
  if (loading) return <PageLoading />;
  if (error || !browse) return <PageError message={error ?? "Repository archive unavailable."} retry={reload} />;
  const documents = browse.repositories.flatMap((group) => group.documents);
  const toggle = (key: string, value: string) => {
    const next = new URLSearchParams(params); const values = next.getAll(key); next.delete(key);
    for (const candidate of values.includes(value) ? values.filter((item) => item !== value) : [...values, value]) next.append(key, candidate);
    setParams(next, { replace: true });
  };
  const repositories = [...new Set(browse.repositories.map((group) => group.repository))].sort();
  const schemaTypes = [...new Set(documents.map((document) => jsonString(document.metadata.schemaType)).filter((value): value is string => Boolean(value)))].sort();
  const tags = [...new Set(documents.flatMap((document) => document.tags))].sort();
  const filteredGroups = browse.repositories.map((group) => ({ ...group, documents: group.documents.filter((document) =>
    (!params.getAll("repository").length || params.getAll("repository").includes(group.repository))
    && (!params.getAll("schema").length || params.getAll("schema").includes(jsonString(document.metadata.schemaType) ?? ""))
    && (!params.getAll("tag").length || params.getAll("tag").every((tag) => document.tags.includes(tag))),
  ) })).filter((group) => group.documents.length);
  return <div className="l-page"><div className="lore-page-head"><span className="page-kicker">Source archive</span><h1 className="lore-page-head__title">Repository documents</h1><p className="lore-muted">Grouped by repository and branch.</p></div>
    <FilterPanel activeCount={["repository", "schema", "tag"].reduce((count, key) => count + params.getAll(key).length, 0)}>
      {repositories.length > 1 && <FilterGroup title="Repository">{repositories.map((value) => facetButton("repository", value, value, documents.filter((document) => jsonString(document.metadata.repository) === value).length, params, toggle))}</FilterGroup>}
      {schemaTypes.length > 0 && <FilterGroup title="Schema">{schemaTypes.map((value) => facetButton("schema", value, value, documents.filter((document) => jsonString(document.metadata.schemaType) === value).length, params, toggle))}</FilterGroup>}
      {tags.length > 0 && <FilterGroup title="Tag">{tags.map((value) => facetButton("tag", value, value, documents.filter((document) => document.tags.includes(value)).length, params, toggle))}</FilterGroup>}
    </FilterPanel>
    {filteredGroups.length ? filteredGroups.map((group) => <section className="repo-group" key={`${group.repository}@${group.branch}`}><div className="repo-group__head"><h2>{group.repository}</h2><span className="lore-chip">⑂ {group.branch}</span></div><DocumentList documents={group.documents} project={project} /></section>) : <EmptyState section="matching repository documents" hint={browse.repositories.length ? "Adjust the repository, schema, or tag filters." : "Upload files or a directory with lore upload repository."} />}
  </div>;
}

function NoteFacets({ documents, params, toggle }: { documents: DocumentSummary[]; params: URLSearchParams; toggle: (key: string, value: string) => void }) {
  const metadataValues = (key: string) => [...new Set(documents.map((document) => jsonString(document.metadata[key])).filter((value): value is string => Boolean(value)))].sort();
  const groups = [
    ["role", "Role", metadataValues("role")],
    ["tag", "Tag", [...new Set(documents.flatMap((document) => document.tags))].sort()],
    ["lifecycle", "Lifecycle", metadataValues("lifecycle")],
    ["projectName", "Project", metadataValues("projectName")],
  ] as const;
  return <>{groups.map(([key, label, values]) => values.length ? <FilterGroup title={label} key={key}>{values.map((value) => facetButton(key, value, value, documents.filter((document) => key === "tag" ? document.tags.includes(value) : jsonString(document.metadata[key]) === value).length, params, toggle))}</FilterGroup> : null)}</>;
}

function facetButton(key: string, value: string, label: string, count: number, params: URLSearchParams, toggle: (key: string, value: string) => void) {
  return <button className="lore-facet" aria-pressed={params.getAll(key).includes(value)} onClick={() => toggle(key, value)} key={`${key}:${value}`}>{label}<span className="lore-facet__count">{count}</span></button>;
}

function compareNotes(left: DocumentSummary, right: DocumentSummary, sort: string): number {
  if (sort === "title") return left.title.localeCompare(right.title);
  const key = sort === "createdAt" ? "createdAt" : "updatedAt";
  const leftValue = jsonString(left.metadata[key]) ?? left[key];
  const rightValue = jsonString(right.metadata[key]) ?? right[key];
  return rightValue.localeCompare(leftValue) || left.title.localeCompare(right.title);
}

function DocumentList({ documents, project }: { documents: DocumentSummary[]; project: string }) {
  return <div className="lore-list">{documents.map((document) => <Link className="lore-row" to={documentHref(project, document)} key={document.id}>
    <span className="lore-source-badge" data-type={sourceBadgeType(document.sourceType)}>{sourceLabel(document.sourceType)}</span>
    <span><span className="lore-row__title">{document.title}</span><span className="lore-row__meta"><span>{document.sourceInstance}</span>{jsonString(document.metadata.status) && <span>{jsonString(document.metadata.status)}</span>}<time dateTime={displayUpdatedAt(document)}>{new Date(displayUpdatedAt(document)).toLocaleDateString()}</time></span></span>
    <span className="row-tail">{document.tags.slice(0, 3).map((tag) => <span className="lore-chip lore-chip--tag" key={tag}>{tag}</span>)}<span aria-hidden="true">›</span></span>
  </Link>)}</div>;
}

export function displayUpdatedAt(document: DocumentSummary): string {
  return document.sourceType === "note" ? jsonString(document.metadata.updatedAt) ?? document.updatedAt : document.updatedAt;
}

export function EmptyState({ section, hint }: { section: string; hint?: string }) {
  return <div className="lore-empty"><div className="lore-empty__icon" aria-hidden="true">◇</div><div className="lore-empty__title">No {section} yet</div><div className="lore-empty__hint">{hint ?? "Synchronize a source with the Lore CLI to begin this archive."}</div></div>;
}
