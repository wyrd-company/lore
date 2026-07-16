import { useEffect, useMemo, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { api } from "./api";
import { EmptyState } from "./browse";
import { PageError, useProject } from "./app";
import { FilterGroup, FilterPanel } from "./filters";
import type { SearchResponse, SourceType } from "./types";
import { documentHref, jsonString, parseDatePreset, sourceBadgeType, sourceLabel } from "./utils";

const sourceTypes: SourceType[] = ["task", "note", "briefing", "repository", "conversation"];

export function SearchPage() {
  const { project = "" } = useParams();
  const { browse } = useProject();
  const [params, setParams] = useSearchParams();
  const [response, setResponse] = useState<SearchResponse>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();
  const [rankMode, setRankMode] = useState<"fused" | "keyword" | "vector">("fused");
  const query = params.get("q") ?? "";
  const selectedTypes = params.getAll("sourceType");

  useEffect(() => {
    if (!query.trim()) { setResponse(undefined); setLoading(false); return; }
    const controller = new AbortController(); setLoading(true); setError(undefined);
    api.search(project, params).then(setResponse).catch((reason: Error) => !controller.signal.aborted && setError(reason.message)).finally(() => !controller.signal.aborted && setLoading(false));
    return () => controller.abort();
  }, [project, params, query]);

  const repositories = useMemo(() => browse?.repositories.map((item) => item.repository).filter((value, index, all) => all.indexOf(value) === index) ?? [], [browse]);
  const branches = useMemo(() => browse?.repositories.filter((item) => !params.getAll("repository").length || params.getAll("repository").includes(item.repository)).map((item) => item.branch).filter((value, index, all) => all.indexOf(value) === index) ?? [], [browse, params]);
  const toggle = (key: string, value: string) => {
    const next = new URLSearchParams(params); const values = next.getAll(key); next.delete(key);
    for (const item of values.includes(value) ? values.filter((item) => item !== value) : [...values, value]) next.append(key, item);
    setParams(next);
  };
  const setSingle = (key: string, value: string) => { const next = new URLSearchParams(params); value ? next.set(key, value) : next.delete(key); setParams(next); };
  const submit = (event: React.FormEvent<HTMLFormElement>) => { event.preventDefault(); const data = new FormData(event.currentTarget); setSingle("q", String(data.get("q") ?? "").trim()); };
  const active = [...selectedTypes.map((value) => ["sourceType", value]), ...params.getAll("repository").map((value) => ["repository", value]), ...params.getAll("branch").map((value) => ["branch", value]), ...params.getAll("tag").map((value) => ["tag", value]), ...(params.has("createdFrom") ? [["createdFrom", params.get("datePreset") ?? "Created date"]] : [])] as string[][];
  const rankedResults = useMemo(() => [...(response?.results ?? [])].sort((left, right) => rankMode === "fused" ? right.score - left.score : bestRank(left, rankMode) - bestRank(right, rankMode)), [response, rankMode]);

  return <div className="l-page"><div className="lore-page-head"><span className="page-kicker">Hybrid retrieval</span><h1 className="lore-page-head__title">Search the archive</h1></div>
    <form className="lore-search" onSubmit={submit} role="search"><span aria-hidden="true">⌕</span><input name="q" aria-label="Search query" defaultValue={query} key={query} placeholder="Ask the archive…" autoFocus /><kbd>↵</kbd></form>
    <FilterPanel activeCount={active.length}>
      <FilterGroup title="Source type">{sourceTypes.map((type) => <button className="lore-facet" aria-pressed={selectedTypes.includes(type)} onClick={() => toggle("sourceType", type)} key={type}>{sourceLabel(type)}</button>)}</FilterGroup>
      {repositories.length > 0 && <FilterGroup title="Repository">{repositories.map((repo) => <button className="lore-facet" aria-pressed={params.getAll("repository").includes(repo)} onClick={() => toggle("repository", repo)} key={repo}>{repo}</button>)}</FilterGroup>}
      {branches.length > 0 && <FilterGroup title="Branch">{branches.map((branch) => <button className="lore-facet" aria-pressed={params.getAll("branch").includes(branch)} onClick={() => toggle("branch", branch)} key={branch}>{branch}</button>)}</FilterGroup>}
      {(browse?.tags.length ?? 0) > 0 && <FilterGroup title="Tags">{browse!.tags.map((tag) => <button className="lore-facet" aria-pressed={params.getAll("tag").includes(tag)} onClick={() => toggle("tag", tag)} key={tag}>{tag}</button>)}</FilterGroup>}
      <FilterGroup title="Created"><select className="lore-select" aria-label="Created date" value={params.has("createdFrom") ? params.get("datePreset") ?? "custom" : ""} onChange={(event) => { const next = new URLSearchParams(params); next.delete("createdFrom"); next.delete("datePreset"); const from = parseDatePreset(event.target.value); if (from) { next.set("createdFrom", from); next.set("datePreset", event.target.value); } setParams(next); }}><option value="">Any time</option><option value="24h">Last 24 hours</option><option value="7d">Last 7 days</option><option value="30d">Last 30 days</option></select></FilterGroup>
    </FilterPanel><section aria-live="polite">
      {active.length > 0 && <div className="active-filters">{active.map(([key, value]) => <button className="lore-chip lore-chip--removable" key={`${key}-${value}`} onClick={() => key === "createdFrom" ? clearCreated(params, setParams) : toggle(key, value)}>{value} <span className="lore-chip__x">×</span></button>)}</div>}
      {error && <PageError message={error} />}
      {loading && <SearchSkeleton />}
      {!query && <RecentAndTags project={project} />}
      {!loading && query && response && <>{response.warnings?.map((warning) => <div className="search-warning" key={warning}>△ {warning}</div>)}<div className="lore-segmented" aria-label="Ranking mode"><button aria-pressed={rankMode === "fused"} onClick={() => setRankMode("fused")}>Fused</button><button aria-pressed={rankMode === "keyword"} onClick={() => setRankMode("keyword")} disabled={!response.modes.keyword}>Keyword</button><button aria-pressed={rankMode === "vector"} onClick={() => setRankMode("vector")} disabled={!response.modes.vector}>Vector</button></div><p className="lore-muted">{response.results.length} documents matched “{query}”</p>{response.results.length ? <div className="lore-results">{rankedResults.map((result) => <article className="lore-result" key={result.id}><div className="lore-result__head"><span className="lore-source-badge" data-type={sourceBadgeType(result.sourceType)}>{sourceLabel(result.sourceType)}</span><Link className="lore-result__title" to={documentHref(project, result)}>{result.title}</Link><span className="lore-result__score">{rankLabel(result, rankMode)}</span></div>{result.matchedChunks.map((chunk) => <Link className="lore-chunk snippet-link" to={`${documentHref(project, result)}${locationHash(chunk.structuralLocation)}`} key={chunk.id}><span className="lore-chunk__loc">{locationLabel(chunk.structuralLocation, chunk.kind)}</span><span className="lore-chunk__snippet" dangerouslySetInnerHTML={{ __html: safeHeadline(chunk.snippet) }} /></Link>)}</article>)}</div> : <EmptyState section="matching passages" hint="Try broader words or remove a filter." />}</>}
    </section>
  </div>;
}

function SearchSkeleton() { return <div className="lore-results">{[1, 2, 3].map((item) => <div className="lore-result" key={item}><div className="lore-skel lore-skel--line" /><div className="lore-skel lore-skel--block" /></div>)}</div>; }
function RecentAndTags({ project }: { project: string }) { const { browse } = useProject(); const recent = [...(browse?.tasks ?? []), ...(browse?.notes ?? []), ...(browse?.briefings ?? []), ...(browse?.conversations ?? []), ...(browse?.repositories.flatMap((group) => group.documents) ?? [])].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)).slice(0, 5); return <><h2>Recent documents</h2><div className="lore-results">{recent.map((document) => <article className="lore-result" key={document.id}><div className="lore-result__head"><span className="lore-source-badge" data-type={sourceBadgeType(document.sourceType)}>{sourceLabel(document.sourceType)}</span><Link className="lore-result__title" to={documentHref(project, document)}>{document.title}</Link></div></article>)}</div>{browse?.tags.length ? <><h2>Tags</h2><div className="facet-stack">{browse.tags.map((tag) => <Link className="lore-chip lore-chip--tag" to={`/${project}/search?q=${encodeURIComponent(tag)}&tag=${encodeURIComponent(tag)}`} key={tag}>{tag}</Link>)}</div></> : null}</>; }
function locationLabel(location: Record<string, unknown>, fallback: string): string { const heading = location.headingPath; if (Array.isArray(heading)) return `§ ${heading.join(" › ")}`; return jsonString(location.yamlPath) ?? jsonString(location.messageId) ?? fallback; }
function locationHash(location: Record<string, unknown>): string { const heading = location.headingPath; const value = jsonString(location.elementId) ?? jsonString(location.messageId) ?? (Array.isArray(heading) ? String(heading.at(-1)).toLowerCase().replace(/[^a-z0-9]+/g, "-") : ""); return value ? `#${encodeURIComponent(value)}` : ""; }
export function safeHeadline(value: string): string { return value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;").replaceAll("'", "&#39;").replaceAll("&lt;mark&gt;", "<mark>").replaceAll("&lt;/mark&gt;", "</mark>"); }
function bestRank(result: SearchResponse["results"][number], mode: "keyword" | "vector"): number { const ranks = result.matchedChunks.map((chunk) => mode === "keyword" ? chunk.keywordRank : chunk.vectorRank).filter((rank): rank is number => rank !== undefined); return ranks.length ? Math.min(...ranks) : Number.MAX_SAFE_INTEGER; }
function rankLabel(result: SearchResponse["results"][number], mode: "fused" | "keyword" | "vector"): string { return mode === "fused" ? result.score.toFixed(4) : bestRank(result, mode) === Number.MAX_SAFE_INTEGER ? "—" : `#${bestRank(result, mode)}`; }
function clearCreated(params: URLSearchParams, setParams: ReturnType<typeof useSearchParams>[1]) { const next = new URLSearchParams(params); next.delete("createdFrom"); next.delete("datePreset"); setParams(next); }
