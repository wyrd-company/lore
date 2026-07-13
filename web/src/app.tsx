import { createContext, useContext, useEffect, useMemo, useState } from "react";
import { Link, NavLink, Outlet, useLocation, useNavigate, useParams } from "react-router-dom";
import { api } from "./api";
import type { BrowseResponse, ProjectSummary } from "./types";

interface ProjectContextValue {
  projects: ProjectSummary[];
  browse?: BrowseResponse;
  loading: boolean;
  error?: string;
  reload: () => void;
}

const ProjectContext = createContext<ProjectContextValue>({ projects: [], loading: true, reload: () => undefined });
export const useProject = () => useContext(ProjectContext);

const navItems = [
  ["Overview", "", "⌂", undefined], ["Search", "search", "⌕", undefined],
  ["Tasks", "tasks", "□", "task"], ["Notes", "notes", "◇", "note"],
  ["Briefings", "briefings", "▱", "briefing"], ["Repository", "repo", "⌘", "repository"],
  ["Conversations", "conversations", "◌", "conversation"],
] as const;

function useStoredValue(key: string, initial: string) {
  const [value, setValue] = useState(() => localStorage.getItem(key) ?? initial);
  const update = (next: string) => { setValue(next); localStorage.setItem(key, next); };
  return [value, update] as const;
}

export function useAttribution() { return useStoredValue("lore.attribution", ""); }

export function App() {
  const { project = "" } = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const [projects, setProjects] = useState<ProjectSummary[]>([]);
  const [browse, setBrowse] = useState<BrowseResponse>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [reloadToken, setReloadToken] = useState(0);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [projectFilter, setProjectFilter] = useState("");
  const [attribution, setAttribution] = useAttribution();
  const [theme, setTheme] = useStoredValue("lore.theme", "system");

  useEffect(() => {
    document.documentElement.removeAttribute("data-theme");
    if (theme !== "system") document.documentElement.dataset.theme = theme;
  }, [theme]);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setError(undefined); setBrowse(undefined);
    api.projects().then(async (items) => {
      if (controller.signal.aborted) return;
      setProjects(items);
      if (!project && items.length) {
        navigate(`/${items[0].slug}`, { replace: true });
        return;
      }
      if (project) setBrowse(await api.browse(project));
    }).catch((reason: Error) => !controller.signal.aborted && setError(reason.message)).finally(() => !controller.signal.aborted && setLoading(false));
    return () => controller.abort();
  }, [project, navigate, reloadToken]);

  useEffect(() => setSidebarOpen(false), [location.pathname]);
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault(); document.querySelector<HTMLInputElement>("[data-global-search]")?.focus();
      }
    };
    addEventListener("keydown", keydown); return () => removeEventListener("keydown", keydown);
  }, []);

  const filteredProjects = useMemo(() => projects.filter((item) => item.name.toLowerCase().includes(projectFilter.toLowerCase()) || item.slug.includes(projectFilter.toLowerCase())), [projects, projectFilter]);
  const counts = browse?.project.sourceTypeCounts ?? {};
  const context = { projects, browse, loading, error, reload: () => setReloadToken((value) => value + 1) };

  const searchSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const query = String(data.get("q") ?? "").trim();
    if (project && query) navigate(`/${project}/search?q=${encodeURIComponent(query)}`);
  };

  return <ProjectContext.Provider value={context}>
    <div className="l-app" data-sidebar-open={sidebarOpen || undefined}>
      <Link className="l-brand" to={project ? `/${project}` : "/"} aria-label="Lore home">
        <span className="l-brand__mark" aria-hidden="true">L</span><span className="l-brand__name">Lore</span>
      </Link>
      <header className="l-header">
        <button className="lore-btn lore-btn--ghost lore-btn--icon l-mobile-menu" onClick={() => setSidebarOpen(!sidebarOpen)} aria-expanded={sidebarOpen} aria-label="Toggle navigation">☰</button>
        <details className="project-menu">
          <summary className="lore-project-select"><span className="lore-project-select__dot" />{browse?.project.name ?? (loading ? "Loading…" : "Select project")}<span className="lore-project-select__caret">⌄</span></summary>
          <div className="lore-popover project-menu__popover">
            <label className="lore-visually-hidden" htmlFor="project-filter">Filter projects</label>
            <input id="project-filter" className="lore-input" placeholder="Filter projects…" value={projectFilter} onChange={(event) => setProjectFilter(event.target.value)} />
            {filteredProjects.map((item) => <button className="lore-menu-item" key={item.id} onClick={() => navigate(`/${item.slug}`)}><span className="lore-project-select__dot" />{item.name}</button>)}
          </div>
        </details>
        <span className="l-header__spacer" />
        <form className="lore-search l-global-search" onSubmit={searchSubmit} role="search">
          <span aria-hidden="true">⌕</span><input data-global-search name="q" aria-label="Search this project" placeholder="Search this project" disabled={!project} /><kbd>⌘K</kbd>
        </form>
        <button className="lore-btn lore-btn--ghost lore-btn--icon" aria-label={`Theme: ${theme}`} title={`Theme: ${theme}`} onClick={() => setTheme(theme === "system" ? "dark" : theme === "dark" ? "light" : "system")}>{theme === "dark" ? "☾" : theme === "light" ? "☀" : "◐"}</button>
        <label className="lore-attribution"><span>Writing as</span><input className="lore-input" aria-label="Annotation attribution name" placeholder="Your name" value={attribution} onChange={(event) => setAttribution(event.target.value)} /></label>
      </header>
      <aside className="l-sidebar" aria-label="Project knowledge">
        <nav>
          <div className="l-nav-section"><div className="l-nav-section__label">Project archive</div>
            {navItems.slice(0, 2).map(([label, path, icon]) => <NavLink end={path === ""} className="l-nav-item" key={label} to={`/${project}${path ? `/${path}` : ""}`}><span className="l-nav-item__icon">{icon}</span>{label}</NavLink>)}
          </div>
          <div className="l-nav-section"><div className="l-nav-section__label">Sources</div>
            {navItems.slice(2).map(([label, path, icon, type]) => <NavLink className="l-nav-item" key={label} to={`/${project}/${path}`}><span className="l-nav-item__icon">{icon}</span>{label}<span className="l-nav-item__count">{counts[type!] ?? 0}</span></NavLink>)}
          </div>
        </nav>
      </aside>
      <main className="l-main"><Outlet /></main>
    </div>
  </ProjectContext.Provider>;
}

export function PageLoading() {
  return <div className="l-page"><div className="lore-skel lore-skel--title" /><div className="lore-skel lore-skel--line" /><div className="lore-skel lore-skel--block" /></div>;
}

export function PageError({ message, retry }: { message: string; retry?: () => void }) {
  return <div className="l-page"><div className="lore-error" role="alert"><span className="lore-error__icon">!</span><div><strong>Something interrupted the archive.</strong><p>{message}</p>{retry && <button className="lore-btn lore-btn--secondary lore-btn--sm" onClick={retry}>Try again</button>}</div></div></div>;
}
