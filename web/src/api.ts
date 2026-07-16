import type { Annotation, BriefingSetting, BrowseResponse, DocumentDetail, IngestionFailure, ProjectSummary, RevisionDetail, SearchResponse } from "./types";

export class ApiError extends Error {
  constructor(message: string, readonly status: number) { super(message); }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  if (!response.ok) {
    const problem = await response.json().catch(() => ({})) as { detail?: string };
    throw new ApiError(problem.detail ?? `Request failed (${response.status})`, response.status);
  }
  return response.status === 204 ? undefined as T : response.json() as Promise<T>;
}

const projectPath = (project: string) => `/api/projects/${encodeURIComponent(project)}`;

export const api = {
  projects: async () => (await request<{ projects: ProjectSummary[] }>("/api/projects")).projects,
  browse: (project: string) => request<BrowseResponse>(`${projectPath(project)}/browse`),
  document: (project: string, id: string) => request<DocumentDetail>(`${projectPath(project)}/documents/${id}`),
  revision: (project: string, documentId: string, revisionId: string) => request<RevisionDetail>(`${projectPath(project)}/documents/${documentId}/revisions/${revisionId}`),
  search: (project: string, params: URLSearchParams) => request<SearchResponse>(`${projectPath(project)}/search?${params}`),
  annotations: async (project: string, documentId?: string) => {
    const query = new URLSearchParams();
    if (documentId) query.set("documentId", documentId);
    const suffix = query.size ? `?${query}` : "";
    return (await request<{ annotations: Annotation[] }>(`${projectPath(project)}/annotations${suffix}`)).annotations;
  },
  createAnnotation: (project: string, body: unknown) => request<Annotation>(`${projectPath(project)}/annotations`, { method: "POST", body: JSON.stringify(body) }),
  updateAnnotation: (project: string, id: string, body: unknown) => request<Annotation>(`${projectPath(project)}/annotations/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  replyToAnnotation: (project: string, id: string, body: { body: string; attributedUsername: string }) => request(`${projectPath(project)}/annotations/${id}/replies`, { method: "POST", body: JSON.stringify(body) }),
  retargetAnnotation: (project: string, id: string, operation: "copy" | "move", body: unknown) => request<Annotation>(`${projectPath(project)}/annotations/${id}/${operation}`, { method: "POST", body: JSON.stringify(body) }),
  ingestionFailures: async (project: string) => (await request<{ failures: IngestionFailure[] }>(`${projectPath(project)}/ingestion-failures`)).failures,
  retryIngestionFailure: (project: string, id: string) => request<void>(`${projectPath(project)}/ingestion-failures/${id}`, { method: "DELETE" }),
  updateBriefing: (project: string, id: string, body: { category?: string; home?: boolean }) => request<BriefingSetting>(`${projectPath(project)}/briefings/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
};
