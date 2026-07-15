/*
---
relationships:
  implements: system
---
*/
import { useEffect, useMemo, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { api } from "./api";
import { PageError, PageLoading, useProject } from "./app";
import type { Annotation } from "./types";
import { allDocuments, documentHref, relativeTime, sourceBadgeType, sourceLabel } from "./utils";

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
  const requestedStatus = params.get("status") ?? "all";
  const status: Status = statuses.includes(requestedStatus as Status) ? requestedStatus as Status : "all";

  useEffect(() => {
    let active = true;
    setLoading(true); setError("");
    api.annotations(project).then((records) => {
      if (active) setAnnotations(records);
    }).catch((reason: Error) => {
      if (active) setError(reason.message);
    }).finally(() => {
      if (active) setLoading(false);
    });
    return () => { active = false; };
  }, [project, reload]);

  const documents = useMemo(() => new Map((browse ? allDocuments(browse) : []).map((document) => [document.id, document])), [browse]);
  const visible = annotations.filter((annotation) => status === "all" || annotation.status === status);
  const chooseStatus = (next: Status) => {
    const query = new URLSearchParams(params);
    next === "all" ? query.delete("status") : query.set("status", next);
    setParams(query, { replace: true });
  };
  if (loading) return <PageLoading />;
  if (error) return <PageError message={error} retry={() => setReload((value) => value + 1)} />;
  return <div className="l-page">
    <div className="lore-page-head"><span className="page-kicker">Review ledger</span><h1 className="lore-page-head__title">Annotations</h1><p className="lore-muted">{annotations.length} annotations across this project.</p></div>
    <div className="lore-facets" aria-label="Annotation status filter">{statuses.map((candidate) => <button className="lore-facet" aria-pressed={status === candidate} onClick={() => chooseStatus(candidate)} key={candidate}>{candidate === "all" ? "All" : candidate}<span className="lore-facet__count">{candidate === "all" ? annotations.length : annotations.filter((annotation) => annotation.status === candidate).length}</span></button>)}</div>
    {visible.length ? <div className="annotation-index">{visible.map((annotation) => {
      const document = documents.get(annotation.documentId);
      const href = document ? `${documentHref(project, document)}?anno=${encodeURIComponent(annotation.id)}` : "";
      const content = <><div className="annotation-index__head"><span className="lore-source-badge" data-type={sourceBadgeType(annotation.sourceType)}>{sourceLabel(annotation.sourceType)}</span><span className="lore-status" data-state={annotation.status}>{annotation.status}</span><strong>{annotation.documentTitle}</strong><time dateTime={annotation.updatedAt}>{relativeTime(annotation.updatedAt)}</time></div>{annotation.selectedQuote && <blockquote className="lore-anno__quote">{annotation.selectedQuote}</blockquote>}<p>{annotation.body}</p><div className="annotation-index__meta">{annotation.attributedUsername} · {annotation.sourceInstance}{document ? " · Open annotation →" : " · Source document is no longer active"}</div></>;
      return document ? <Link className="lore-card lore-card--pad annotation-index__item" to={href} key={annotation.id}>{content}</Link> : <article className="lore-card lore-card--pad annotation-index__item" key={annotation.id}>{content}</article>;
    })}</div> : <div className="lore-empty"><div className="lore-empty__title">No {status === "all" ? "" : `${status} `}annotations</div><div className="lore-empty__hint">Annotations created on project documents will appear here.</div></div>}
  </div>;
}
