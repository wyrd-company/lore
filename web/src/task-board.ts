import type { DocumentSummary } from "./types";
import { jsonString } from "./utils";

const lifecycle = ["backlog", "todo", "in-progress", "review", "done", "archived"] as const;
const lifecycleRank = new Map<string, number>(lifecycle.map((status, index) => [status, index]));
const synonyms: Record<string, string> = {
  archive: "archived",
  archived: "archived",
  backlog: "backlog",
  completed: "done",
  doing: "in-progress",
  done: "done",
  icebox: "backlog",
  "in-progress": "in-progress",
  "in-review": "review",
  qa: "review",
  review: "review",
  shipped: "done",
  "to-do": "todo",
  todo: "todo",
  triage: "backlog",
  wip: "in-progress",
};

export interface TaskLane {
  key: string;
  label: string;
  recognized: boolean;
}

export function taskStatusSlug(status: string): string {
  return status.trim().toLowerCase().replace(/[\s_]+/g, "-").replace(/-+/g, "-");
}

export function taskStatusKey(status: string): string {
  const slug = taskStatusSlug(status);
  return synonyms[slug] ?? slug;
}

export function taskStatusLabel(status: string): string {
  return taskStatusKey(status).split("-").map((part) => part ? part[0].toUpperCase() + part.slice(1) : part).join(" ");
}

export function taskStatus(document: DocumentSummary): string {
  return jsonString(document.metadata.status) ?? "backlog";
}

export function taskPriority(document: DocumentSummary): string {
  return taskStatusSlug(jsonString(document.metadata.priority) ?? "medium");
}

export function orderedTaskLanes(statuses: string[]): TaskLane[] {
  const lanes: Array<TaskLane & { anchorRank?: number; rank?: number }> = [];
  const seen = new Set<string>();
  let anchorRank: number | undefined;

  for (const status of statuses) {
    const key = taskStatusKey(status);
    const rank = lifecycleRank.get(key);
    if (rank !== undefined) anchorRank = rank;
    if (!key || seen.has(key)) continue;
    seen.add(key);
    if (rank !== undefined) {
      lanes.push({ key, label: taskStatusLabel(key), recognized: true, rank });
    } else {
      lanes.push({ key, label: status.trim() || taskStatusLabel(key), recognized: false, anchorRank });
    }
  }

  const custom = lanes.filter((lane) => !lane.recognized);
  const recognized = lanes.filter((lane) => lane.recognized).sort((left, right) => left.rank! - right.rank!);
  const result: TaskLane[] = [];
  while (custom.length && custom[0].anchorRank === undefined) result.push(custom.shift()!);
  for (const lane of recognized) {
    result.push(lane);
    while (custom.length && custom[0].anchorRank! <= lane.rank!) result.push(custom.shift()!);
  }
  result.push(...custom);
  return result.map(({ key, label, recognized: known }) => ({ key, label, recognized: known }));
}
