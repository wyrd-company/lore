import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, NavLink, Outlet, Route, Routes } from "react-router-dom";
import "../../design/site.css";

const sections = ["Tasks", "Notes", "Briefings", "Repositories", "Conversations"] as const;

function Shell() {
  return (
    <div className="app-shell">
      <header className="site-header">
        <a className="brand" href="/">Lore</a>
        <label>
          <span className="visually-hidden">Project</span>
          <select aria-label="Project" defaultValue="">
            <option value="" disabled>Select a project</option>
          </select>
        </label>
      </header>
      <aside className="site-nav" aria-label="Project knowledge">
        <nav>
          {sections.map((section) => (
            <NavLink key={section} to={`/${section.toLowerCase()}`}>{section}</NavLink>
          ))}
        </nav>
      </aside>
      <main className="site-main"><Outlet /></main>
    </div>
  );
}

function EmptySection({ name }: { name: string }) {
  return <section><h1>{name}</h1><p>Select a project to browse its {name.toLowerCase()}.</p></section>;
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <Routes>
        <Route element={<Shell />}>
          <Route index element={<EmptySection name="Knowledge" />} />
          {sections.map((section) => (
            <Route key={section} path={section.toLowerCase()} element={<EmptySection name={section} />} />
          ))}
        </Route>
      </Routes>
    </BrowserRouter>
  </StrictMode>,
);
