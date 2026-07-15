/*
---
relationships:
  implements: system
---
*/
import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { api } from "./api";
import { PageError, PageLoading, useProject } from "./app";
import type { IngestionFailure } from "./types";
import { relativeTime, sourceBadgeType, sourceLabel } from "./utils";

export function WatcherIssuesPage() {
  const { project = "" } = useParams();
  const { reload: reloadProject } = useProject();
  const [failures, setFailures] = useState<IngestionFailure[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [removing, setRemoving] = useState("");
  useEffect(() => {
    let active = true;
    setLoading(true); setError("");
    api.ingestionFailures(project).then((records) => {
      if (active) setFailures(records);
    }).catch((reason: Error) => {
      if (active) setError(reason.message);
    }).finally(() => {
      if (active) setLoading(false);
    });
    return () => { active = false; };
  }, [project, reload]);
  const retry = async (failure: IngestionFailure) => {
    setRemoving(failure.id); setError("");
    try {
      await api.retryIngestionFailure(project, failure.id);
      setFailures((current) => current.filter((candidate) => candidate.id !== failure.id));
      reloadProject();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Could not clear the watcher issue.");
    } finally {
      setRemoving("");
    }
  };
  if (loading) return <PageLoading />;
  if (error && failures.length === 0) return <PageError message={error} retry={() => setReload((value) => value + 1)} />;
  return <div className="l-page">
    <div className="lore-page-head"><span className="page-kicker">Ingestion quarantine</span><h1 className="lore-page-head__title">Watcher issues</h1><p className="lore-muted">Malformed files are isolated so healthy source documents continue synchronizing.</p></div>
    {error && <div className="lore-error" role="alert">{error}</div>}
    {failures.length ? <div className="watcher-issue-list">{failures.map((failure) => <article className="lore-card lore-card--pad watcher-issue" key={failure.id}><div className="watcher-issue__head"><span className="lore-source-badge" data-type={sourceBadgeType(failure.sourceType)}>{sourceLabel(failure.sourceType)}</span><strong>{failure.sourceInstance}</strong><time dateTime={failure.updatedAt}>{relativeTime(failure.updatedAt)}</time></div><code>{failure.path}</code><p>{failure.message}</p><div className="watcher-issue__actions"><span className="lore-muted">Fix the file, then clear this issue. The watcher retries it on the next scan.</span><button className="lore-btn lore-btn--primary lore-btn--sm" disabled={removing === failure.id} onClick={() => retry(failure)}>{removing === failure.id ? <span className="lore-spinner" /> : "Retry"}</button></div></article>)}</div> : <div className="lore-empty"><div className="lore-empty__icon" aria-hidden="true">✓</div><div className="lore-empty__title">No watcher issues</div><div className="lore-empty__hint">Every configured source file is eligible for synchronization.</div></div>}
  </div>;
}
