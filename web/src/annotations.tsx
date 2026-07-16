/*
---
relationships:
  implements: system
---
*/
import { useEffect, useMemo, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { api } from "./api";
import { PageError, PageLoading, useAttribution, useProject } from "./app";
import { FilterGroup, FilterPanel } from "./filters";
import type { Annotation, DocumentDetail, RevisionDetail } from "./types";
import { allDocuments, relativeTime, sourceBadgeType, sourceLabel } from "./utils";

const statuses = ["all", "open", "resolved", "dismissed"] as const;
type Status = typeof statuses[number];

export function AnnotationsPage() {
  const { project = "" } = useParams();
  const { browse } = useProject();
  const [params, setParams] = useSearchParams();
  const [annotations, setAnnotations] = useState<Annotation[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const requestedStatus = params.get("status") ?? "open";
  const status: Status = statuses.includes(requestedStatus as Status) ? requestedStatus as Status : "open";
  const active = annotations.find((annotation) => annotation.id === params.get("anno"));

  useEffect(() => {
    let mounted = true;
    setLoading(true); setError("");
    api.annotations(project).then((records) => mounted && setAnnotations(records)).catch((reason: Error) => mounted && setError(reason.message)).finally(() => mounted && setLoading(false));
    return () => { mounted = false; };
  }, [project, reload]);

  const documents = useMemo(() => new Map((browse ? allDocuments(browse) : []).map((document) => [document.id, document])), [browse]);
  const visible = annotations.filter((annotation) => status === "all" || annotation.status === status);
  const chooseStatus = (next: Status) => { const query = new URLSearchParams(params); next === "open" ? query.delete("status") : query.set("status", next); setParams(query, { replace: true }); };
  const view = (annotation: Annotation) => { const query = new URLSearchParams(params); query.set("anno", annotation.id); setParams(query, { replace: true }); };
  if (loading) return <PageLoading />;
  if (error) return <PageError message={error} retry={() => setReload((value) => value + 1)} />;
  return <div className="l-page">
    <div className="lore-page-head"><span className="page-kicker">Review ledger</span><h1 className="lore-page-head__title">Annotations</h1><p className="lore-muted">{annotations.length} annotations across this project.</p></div>
    <FilterPanel activeCount={status === "open" ? 0 : 1}><FilterGroup title="Status">{statuses.map((candidate) => <button className="lore-facet" aria-pressed={status === candidate} onClick={() => chooseStatus(candidate)} key={candidate}>{candidate === "all" ? "All" : candidate}<span className="lore-facet__count">{candidate === "all" ? annotations.length : annotations.filter((annotation) => annotation.status === candidate).length}</span></button>)}</FilterGroup></FilterPanel>
    {active && <AnnotationViewer project={project} annotation={active} onChanged={() => setReload((value) => value + 1)} onClose={() => { const query = new URLSearchParams(params); query.delete("anno"); setParams(query, { replace: true }); }} />}
    {visible.length ? <div className="annotation-index">{visible.map((annotation) => {
      const document = documents.get(annotation.documentId);
      return <article className={`lore-card lore-card--pad annotation-index__item ${active?.id === annotation.id ? "is-active" : ""}`} key={annotation.id}>
        <button className="annotation-index__open" onClick={() => view(annotation)} disabled={!document} aria-label={`View annotation on ${annotation.documentTitle}`}>
          <div className="annotation-index__head"><span className="lore-source-badge" data-type={sourceBadgeType(annotation.sourceType)}>{sourceLabel(annotation.sourceType)}</span><span className="lore-status" data-state={annotation.status}>{annotation.status}</span><strong>{annotation.documentTitle}</strong><time dateTime={annotation.updatedAt}>{relativeTime(annotation.updatedAt)}</time></div>
          {annotation.selectedQuote && <blockquote className="lore-anno__quote">{annotation.selectedQuote}</blockquote>}<p>{annotation.body}</p>
          <div className="annotation-index__meta">{annotation.attributedUsername} · {annotation.sourceInstance}{document && document.revisionId !== annotation.revisionIdentity ? " · retained revision" : ""}{annotation.replies.length ? ` · ${annotation.replies.length} ${annotation.replies.length === 1 ? "reply" : "replies"}` : ""}{document ? " · View annotation →" : " · Source document is no longer active"}</div>
        </button>
      </article>;
    })}</div> : <div className="lore-empty"><div className="lore-empty__title">No {status === "all" ? "" : `${status} `}annotations</div><div className="lore-empty__hint">Annotations created on project documents will appear here.</div></div>}
  </div>;
}

function AnnotationViewer({ project, annotation, onChanged, onClose }: { project: string; annotation: Annotation; onChanged: () => void; onClose: () => void }) {
  const [current, setCurrent] = useState<DocumentDetail>();
  const [annotated, setAnnotated] = useState<RevisionDetail>();
  const [error, setError] = useState("");
  const [reply, setReply] = useState("");
  const [saving, setSaving] = useState(false);
  const [attribution] = useAttribution();
  useEffect(() => {
    let mounted = true; setCurrent(undefined); setAnnotated(undefined); setError("");
    api.document(project, annotation.documentId).then(async (document) => {
      const prior = document.revisionId === annotation.revisionIdentity ? undefined : await api.revision(project, annotation.documentId, annotation.revisionIdentity);
      if (mounted) { setCurrent(document); setAnnotated(prior); }
    }).catch((reason: Error) => mounted && setError(reason.message));
    return () => { mounted = false; };
  }, [project, annotation.documentId, annotation.revisionIdentity]);
  const submit = async () => {
    if (!attribution.trim()) { setError("Enter your attribution name before replying."); return; }
    setSaving(true); setError("");
    try { await api.replyToAnnotation(project, annotation.id, { body: reply, attributedUsername: attribution }); setReply(""); onChanged(); } catch (reason) { setError(reason instanceof Error ? reason.message : "Reply failed."); } finally { setSaving(false); }
  };
  return <section className="annotation-viewer" aria-label="Selected annotation">
    <header><div><span className="page-kicker">Selected annotation</span><h2>{annotation.documentTitle}</h2></div><button className="lore-btn lore-btn--ghost lore-btn--sm" onClick={onClose}>Close</button></header>
    <div className="annotation-thread"><div className="annotation-thread__root"><strong>{annotation.attributedUsername}</strong><p>{annotation.body}</p></div>{annotation.replies.map((item) => <div className="annotation-thread__reply" key={item.id}><div><strong>{item.attributedUsername}</strong><time dateTime={item.createdAt}>{relativeTime(item.createdAt)}</time></div><p>{item.body}</p></div>)}</div>
    <div className="annotation-reply-composer"><textarea className="lore-textarea" aria-label="Reply" value={reply} onChange={(event) => setReply(event.target.value)} placeholder="Reply to this annotation…" /><button className="lore-btn lore-btn--primary lore-btn--sm" disabled={saving || !reply.trim()} onClick={submit}>{saving ? "Replying…" : "Reply"}</button></div>
    {error && <div className="lore-error" role="alert">{error}</div>}
    {!current && !error && <div className="lore-skel lore-skel--block" />}
    {current && <div className={`annotation-revision-view ${annotated ? "is-comparison" : ""}`}>
      {annotated && <RevisionPane label="Annotated version" date={annotated.createdAt} html={annotated.renderedContent} />}
      <RevisionPane label={annotated ? "Current version" : "Annotated current version"} date={current.updatedAt} html={current.renderedContent} />
    </div>}
  </section>;
}

function RevisionPane({ label, date, html }: { label: string; date: string; html: string }) { return <article className="annotation-revision-pane"><header><strong>{label}</strong><time dateTime={date}>{new Date(date).toLocaleDateString()}</time></header><div className="lore-prose" dangerouslySetInnerHTML={{ __html: html }} /></article>; }
