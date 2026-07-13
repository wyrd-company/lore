import type { BrowseResponse, DocumentSummary, SourceType } from "./types";

export function jsonString(value: unknown): string | undefined {
  return typeof value === "string" && value ? value : undefined;
}

export function sourceLabel(type: SourceType | "repository"): string {
  return ({ task: "Task", note: "Note", briefing: "Briefing", repository: "Repository", conversation: "Conversation" })[type];
}

export function sourceBadgeType(type: SourceType): string { return type === "repository" ? "repo" : type; }

export function allDocuments(browse: BrowseResponse): DocumentSummary[] {
  return [...browse.tasks, ...browse.notes, ...browse.briefings, ...browse.repositories.flatMap((group) => group.documents), ...browse.conversations];
}

export function documentHref(project: string, document: Pick<DocumentSummary, "id" | "sourceType" | "sourceIdentity" | "metadata">): string {
  const root = `/${encodeURIComponent(project)}`;
  switch (document.sourceType) {
    case "task": return `${root}/tasks/${encodeURIComponent(document.sourceIdentity)}`;
    case "note": return `${root}/notes/${encodeURIComponent(document.id)}`;
    case "briefing": return `${root}/briefings/${encodeURIComponent(document.id)}`;
    case "conversation": return `${root}/conversations/${encodeURIComponent(document.id)}`;
    case "repository": {
      const repository = jsonString(document.metadata.repository) ?? "repository";
      const branch = jsonString(document.metadata.branch) ?? "unknown";
      const path = (jsonString(document.metadata.path) ?? document.sourceIdentity).split("/").map(encodeURIComponent).join("/");
      return `${root}/repo/${encodeURIComponent(repository)}/${encodeURIComponent(branch)}/${path}`;
    }
  }
}

export function shortHash(hash: string): string { return hash.slice(0, 10); }

export function relativeTime(value: string): string {
  const seconds = Math.round((Date.now() - new Date(value).getTime()) / 1000);
  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  if (Math.abs(seconds) < 60) return formatter.format(-seconds, "second");
  const minutes = Math.round(seconds / 60);
  if (Math.abs(minutes) < 60) return formatter.format(-minutes, "minute");
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return formatter.format(-hours, "hour");
  return formatter.format(-Math.round(hours / 24), "day");
}

export function parseDatePreset(value: string): string | undefined {
  const days = value === "24h" ? 1 : value === "7d" ? 7 : value === "30d" ? 30 : 0;
  if (!days) return undefined;
  return new Date(Date.now() - days * 86400000).toISOString();
}
