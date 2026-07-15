import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import "../../design/site.css";
import "./app.css";
import { App } from "./app";
import { AnnotationsPage } from "./annotations";
import { BriefingsPage, OverviewPage, RepositoryIndexPage, SourceIndexPage } from "./browse";
import { DocumentPage } from "./document";
import { SearchPage } from "./search";
import { TasksPage } from "./tasks";
import { TermIndexPage, TermPage } from "./terms";
import { WatcherIssuesPage } from "./watcher-issues";

createRoot(document.getElementById("root")!).render(
  <StrictMode><BrowserRouter><Routes>
    <Route path="/:project?" element={<App />}>
      <Route index element={<OverviewPage />} />
      <Route path="search" element={<SearchPage />} />
      <Route path="annotations" element={<AnnotationsPage />} />
      <Route path="watcher-issues" element={<WatcherIssuesPage />} />
      <Route path="tasks" element={<TasksPage />} />
      <Route path="tasks/:taskId" element={<DocumentPage section="tasks" />} />
      <Route path="notes" element={<SourceIndexPage section="notes" />} />
      <Route path="notes/:id" element={<DocumentPage section="notes" />} />
      <Route path="terms" element={<TermIndexPage />} />
      <Route path="terms/:termName" element={<TermPage />} />
      <Route path="briefings" element={<BriefingsPage />} />
      <Route path="briefings/:id" element={<DocumentPage section="briefings" />} />
      <Route path="repo" element={<RepositoryIndexPage />} />
      <Route path="repo/:repo/:branch/*" element={<DocumentPage section="repo" />} />
      <Route path="conversations" element={<SourceIndexPage section="conversations" />} />
      <Route path="conversations/:id" element={<DocumentPage section="conversations" />} />
      <Route path="*" element={<Navigate to=".." replace />} />
    </Route>
  </Routes></BrowserRouter></StrictMode>,
);
