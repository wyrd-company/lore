import { Link, useParams } from "react-router-dom";
import { PageError, PageLoading, useProject } from "./app";
import { DocumentPage } from "./document";
import { displayTaxonomyName } from "./utils";

export function TermIndexPage() {
  const { project = "" } = useParams();
  const { browse, loading, error, reload } = useProject();
  if (loading) return <PageLoading />;
  if (error || !browse) return <PageError message={error ?? "Term collection unavailable."} retry={reload} />;
  const definitions = browse.terms.filter((term) => term.defined);
  const missing = browse.terms.filter((term) => !term.defined);
  return <div className="l-page">
    <div className="lore-page-head"><span className="page-kicker">Shared language</span><h1 className="lore-page-head__title">Terms</h1><p className="lore-muted">{definitions.length} defined · {missing.length} missing definitions</p></div>
    {definitions.length > 0 && <TermList project={project} terms={definitions} />}
    {missing.length > 0 && <section className="repo-group"><h2>Missing definitions</h2><p className="lore-muted">Referenced terms without an uploaded <span className="lore-mono">/term</span> schema document.</p><TermList project={project} terms={missing} /></section>}
    {!browse.terms.length && <div className="lore-empty"><div className="lore-empty__title">No terms yet</div><div className="lore-empty__hint">Upload repository YAML documents with a terms property or a $schema ending in /term.</div></div>}
  </div>;
}

function TermList({ project, terms }: { project: string; terms: NonNullable<ReturnType<typeof useProject>["browse"]>["terms"] }) {
  return <div className="lore-list">{terms.map((term) => <Link className="lore-row" to={`/${project}/terms/${encodeURIComponent(term.name)}`} key={term.name}>
    <span className={`term-mark ${term.defined ? "is-defined" : "is-missing"}`} aria-hidden="true">T</span>
    <span><span className="lore-row__title">{term.title}</span><span className="lore-row__meta">{term.name}<span>{term.referenceCount} references</span></span></span>
    <span aria-hidden="true">›</span>
  </Link>)}</div>;
}

export function TermPage() {
  const { project = "", termName = "" } = useParams();
  const { browse, loading, error, reload } = useProject();
  if (loading) return <PageLoading />;
  if (error || !browse) return <PageError message={error ?? "Term unavailable."} retry={reload} />;
  const term = browse.terms.find((candidate) => candidate.name === termName);
  if (term?.defined && term.definitionDocumentId) return <DocumentPage section="terms" />;
  const title = term?.title ?? displayTaxonomyName(termName);
  return <div className="l-page"><nav className="lore-crumbs" aria-label="Breadcrumb"><Link to={`/${project}`}>{browse.project.name}</Link><span className="lore-crumbs__sep">/</span><Link to={`/${project}/terms`}>Terms</Link><span className="lore-crumbs__sep">/</span><span>{title}</span></nav>
    <div className="lore-page-head"><span className="page-kicker">Missing term</span><h1 className="lore-page-head__title">{title}</h1><p className="lore-muted lore-mono">{termName}</p></div>
    <div className="lore-empty"><div className="lore-empty__icon" aria-hidden="true">T</div><div className="lore-empty__title">No definition has been uploaded</div><div className="lore-empty__hint">Add a repository YAML document whose filename is <span className="lore-mono">{termName}.yml</span> and whose $schema path ends in <span className="lore-mono">/term</span>.</div></div>
  </div>;
}
