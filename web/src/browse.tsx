import { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { PageError, PageLoading, useProject } from "./app";
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
    <div className="overview-grid">{cards.map(([label, path, type, count]) => <Link className="lore-card lore-card--pad lore-card--interactive overview-card" to={`/${project}/${path}`} key={path}><span className="lore-source-badge" data-type={sourceBadgeType(type)}>{sourceLabel(type)}</span><strong className="overview-card__count">{count}</strong><span className="lore-muted">{label}</span></Link>)}</div>
    <section className="repo-group"><div className="lore-page-head__row"><h2>Recently synchronized</h2><Link to={`/${project}/search`} className="lore-btn lore-btn--ghost lore-btn--sm section-action">Search archive →</Link></div>
      {recent.length ? <DocumentList documents={recent} project={project} /> : <EmptyState section="this project" />}
    </section>
  </div>;
}

type SourceSection = "notes" | "briefings" | "conversations";
const listConfig: Record<SourceSection, { title: string; hint: string; type: SourceType }> = {
  notes: { title: "Notes", hint: "Upload a Mnemonic notes directory with lore upload notes.", type: "note" },
  briefings: { title: "Briefings", hint: "Upload trusted HTML with lore upload briefing.", type: "briefing" },
  conversations: { title: "Conversations", hint: "Upload Claude or Codex sessions with lore upload conversations.", type: "conversation" },
};

export function SourceIndexPage({ section }: { section: SourceSection }) {
  const { project = "" } = useParams();
  const { browse, loading, error, reload } = useProject();
  const [filter, setFilter] = useState("");
  const config = listConfig[section];
  const documents: DocumentSummary[] | undefined = browse?.[section];
  const filtered = useMemo(() => (documents ?? []).filter((document) => {
    const matchesText = `${document.title} ${document.tags.join(" ")}`.toLowerCase().includes(filter.toLowerCase());
    return matchesText;
  }), [documents, filter]);
  if (loading) return <PageLoading />;
  if (error || !browse) return <PageError message={error ?? "Section unavailable."} retry={reload} />;
  return <div className="l-page">
    <div className="lore-page-head"><span className="page-kicker">{sourceLabel(config.type)}</span><h1 className="lore-page-head__title">{config.title}</h1><p className="lore-muted">{documents?.length ?? 0} documents in {browse.project.name}</p></div>
    <div className="section-tools"><input className="lore-input" value={filter} onChange={(event) => setFilter(event.target.value)} placeholder={`Filter ${config.title.toLowerCase()}…`} aria-label={`Filter ${config.title.toLowerCase()}`} /></div>
    {filtered.length ? <DocumentList documents={filtered} project={project} /> : <EmptyState section={config.title.toLowerCase()} hint={config.hint} />}
  </div>;
}

export function RepositoryIndexPage() {
  const { project = "" } = useParams();
  const { browse, loading, error, reload } = useProject();
  if (loading) return <PageLoading />;
  if (error || !browse) return <PageError message={error ?? "Repository archive unavailable."} retry={reload} />;
  return <div className="l-page"><div className="lore-page-head"><span className="page-kicker">Source archive</span><h1 className="lore-page-head__title">Repository documents</h1><p className="lore-muted">Grouped by repository and branch.</p></div>
    {browse.repositories.length ? browse.repositories.map((group) => <section className="repo-group" key={`${group.repository}@${group.branch}`}><div className="repo-group__head"><h2>{group.repository}</h2><span className="lore-chip">⑂ {group.branch}</span></div><DocumentList documents={group.documents} project={project} /></section>) : <EmptyState section="repository documents" hint="Upload files or a directory with lore upload repository." />}
  </div>;
}

function DocumentList({ documents, project }: { documents: DocumentSummary[]; project: string }) {
  return <div className="lore-list">{documents.map((document) => <Link className="lore-row" to={documentHref(project, document)} key={document.id}>
    <span className="lore-source-badge" data-type={sourceBadgeType(document.sourceType)}>{sourceLabel(document.sourceType)}</span>
    <span><span className="lore-row__title">{document.title}</span><span className="lore-row__meta"><span>{document.sourceInstance}</span>{jsonString(document.metadata.status) && <span>{jsonString(document.metadata.status)}</span>}<span>{new Date(document.updatedAt).toLocaleDateString()}</span></span></span>
    <span className="row-tail">{document.tags.slice(0, 3).map((tag) => <span className="lore-chip lore-chip--tag" key={tag}>{tag}</span>)}<span aria-hidden="true">›</span></span>
  </Link>)}</div>;
}

export function EmptyState({ section, hint }: { section: string; hint?: string }) {
  return <div className="lore-empty"><div className="lore-empty__icon" aria-hidden="true">◇</div><div className="lore-empty__title">No {section} yet</div><div className="lore-empty__hint">{hint ?? "Synchronize a source with the Lore CLI to begin this archive."}</div></div>;
}
