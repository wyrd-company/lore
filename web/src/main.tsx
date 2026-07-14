import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import "../../design/site.css";
import "./app.css";
import { App } from "./app";
import { OverviewPage, RepositoryIndexPage, SourceIndexPage } from "./browse";
import { DocumentPage } from "./document";
import { SearchPage } from "./search";
import { TasksPage } from "./tasks";

createRoot(document.getElementById("root")!).render(
  <StrictMode><BrowserRouter><Routes>
    <Route path="/:project?" element={<App />}>
      <Route index element={<OverviewPage />} />
      <Route path="search" element={<SearchPage />} />
      <Route path="tasks" element={<TasksPage />} />
      <Route path="tasks/:taskId" element={<DocumentPage section="tasks" />} />
      <Route path="notes" element={<SourceIndexPage section="notes" />} />
      <Route path="notes/:id" element={<DocumentPage section="notes" />} />
      <Route path="briefings" element={<SourceIndexPage section="briefings" />} />
      <Route path="briefings/:id" element={<DocumentPage section="briefings" />} />
      <Route path="repo" element={<RepositoryIndexPage />} />
      <Route path="repo/:repo/:branch/*" element={<DocumentPage section="repo" />} />
      <Route path="conversations" element={<SourceIndexPage section="conversations" />} />
      <Route path="conversations/:id" element={<DocumentPage section="conversations" />} />
      <Route path="*" element={<Navigate to=".." replace />} />
    </Route>
  </Routes></BrowserRouter></StrictMode>,
);
