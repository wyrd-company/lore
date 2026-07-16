import { useEffect, useMemo, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { PageError, useProject } from "./app";
import { FilterGroup, FilterPanel } from "./filters";
import { orderedTaskLanes, taskPriority, taskStatus, taskStatusKey, type TaskLane } from "./task-board";
import type { DocumentSummary } from "./types";
import { documentHref } from "./utils";

type Facet = "status" | "tag" | "priority";
type TaskView = "board" | "list";

export function TasksPage() {
  const { project = "" } = useParams();
  const { browse, loading, error, reload } = useProject();
  const [params, setParams] = useSearchParams();
  const narrow = useNarrowScreen();
  const tasks = browse?.tasks ?? [];
  const lanes = useMemo(() => orderedTaskLanes([
    ...(browse?.taskStatuses ?? []),
    ...tasks.map(taskStatus),
  ]), [browse?.taskStatuses, tasks]);
  const selected = (facet: Facet) => params.getAll(facet);
  const viewParam = params.get("view");
  const view: TaskView = viewParam === "board" || viewParam === "list" ? viewParam : narrow ? "list" : "board";
  const filtered = useMemo(() => tasks.filter((task) => {
    const statuses = selected("status");
    const tags = selected("tag");
    const priorities = selected("priority");
    return (!statuses.length || statuses.includes(taskStatusKey(taskStatus(task))))
      && (!tags.length || tags.every((tag) => task.tags.includes(tag)))
      && (!priorities.length || priorities.includes(taskPriority(task)));
  }), [tasks, params]);

  const toggleFacet = (facet: Facet, value: string) => {
    const next = new URLSearchParams(params);
    const values = next.getAll(facet);
    next.delete(facet);
    for (const candidate of values.includes(value) ? values.filter((item) => item !== value) : [...values, value]) next.append(facet, candidate);
    setParams(next, { replace: true });
  };
  const setView = (nextView: TaskView) => {
    const next = new URLSearchParams(params);
    next.set("view", nextView);
    setParams(next, { replace: true });
  };

  if (loading) return <TasksLoading />;
  if (error || !browse) return <PageError message={error ?? "Tasks board unavailable."} retry={reload} />;
  return <div className="l-page">
    <div className="lore-page-head"><span className="page-kicker">Task</span><h1 className="lore-page-head__title">Tasks</h1><p className="lore-muted">{tasks.length} documents in {browse.project.name}</p></div>
    {tasks.length === 0 ? <TasksEmpty /> : <>
      <TasksToolbar tasks={tasks} lanes={lanes} params={params} view={view} onFacet={toggleFacet} onView={setView} />
      {view === "board"
        ? <TaskBoard tasks={filtered} lanes={lanes} project={project} />
        : <TaskList tasks={filtered} project={project} />}
    </>}
  </div>;
}

function TasksToolbar({ tasks, lanes, params, view, onFacet, onView }: {
  tasks: DocumentSummary[];
  lanes: TaskLane[];
  params: URLSearchParams;
  view: TaskView;
  onFacet: (facet: Facet, value: string) => void;
  onView: (view: TaskView) => void;
}) {
  const tags = [...new Set(tasks.flatMap((task) => task.tags))].sort((left, right) => left.localeCompare(right));
  const priorities = [...new Set(tasks.map(taskPriority))].sort((left, right) => priorityRank(left) - priorityRank(right) || left.localeCompare(right));
  const facet = (kind: Facet, value: string, label: string, count: number) => <button className="lore-facet" aria-pressed={params.getAll(kind).includes(value)} onClick={() => onFacet(kind, value)} key={`${kind}:${value}`}>{label}<span className="lore-facet__count">{count}</span></button>;
  const activeCount = (["status", "priority", "tag"] as Facet[]).reduce((count, kind) => count + params.getAll(kind).length, 0);
  return <div className="lore-tasks-toolbar">
    <FilterPanel activeCount={activeCount}>
      <FilterGroup title="Status">{lanes.map((lane) => facet("status", lane.key, lane.label, tasks.filter((task) => taskStatusKey(taskStatus(task)) === lane.key).length))}</FilterGroup>
      {priorities.length > 0 && <FilterGroup title="Priority">{priorities.map((priority) => facet("priority", priority, titleCase(priority), tasks.filter((task) => taskPriority(task) === priority).length))}</FilterGroup>}
      {tags.length > 0 && <FilterGroup title="Tag">{tags.map((tag) => facet("tag", tag, tag, tasks.filter((task) => task.tags.includes(tag)).length))}</FilterGroup>}
    </FilterPanel>
    <div className="lore-segmented lore-tasks-toolbar__view" role="group" aria-label="Tasks view">
      <button aria-pressed={view === "board"} onClick={() => onView("board")}>Board</button>
      <button aria-pressed={view === "list"} onClick={() => onView("list")}>List</button>
    </div>
  </div>;
}

function TaskBoard({ tasks, lanes, project }: {
  tasks: DocumentSummary[];
  lanes: TaskLane[];
  project: string;
}) {
  return <div className="lore-board-scroll" tabIndex={0} aria-label="Tasks board">
    <div className="lore-board" style={{ gridTemplateColumns: `repeat(${lanes.length}, minmax(0, 1fr))` }}>{lanes.map((lane) => {
      const laneTasks = tasks.filter((task) => taskStatusKey(taskStatus(task)) === lane.key);
      const head = <><span className="lore-col__swatch" aria-hidden="true" /><span className="lore-col__label">{lane.label}</span><span className="lore-col__count">{laneTasks.length}</span></>;
      return <section className="lore-col" data-status={lane.key} aria-label={`${lane.label}: ${laneTasks.length} tasks`} key={lane.key}>
        <div className="lore-col__head">{head}</div>
        <div className="lore-col__body">{laneTasks.length ? laneTasks.map((task) => <TaskCard task={task} project={project} key={task.id} />) : <div className="lore-col__empty">Nothing here</div>}</div>
      </section>;
    })}</div>
  </div>;
}

function TaskCard({ task, project }: { task: DocumentSummary; project: string }) {
  const priority = taskPriority(task);
  const visibleTags = task.tags.slice(0, 3);
  const hiddenTags = task.tags.length - visibleTags.length;
  const hasMeta = task.dependencyCount > 0 || task.dependentCount > 0 || task.openAnnotationCount > 0;
  return <Link className="lore-task-card" to={documentHref(project, task)}>
    <div className="lore-task-card__title">{task.title}</div>
    <div className="lore-task-card__sub"><span className="lore-task-card__id">#{task.sourceIdentity}</span>{(priority === "high" || priority === "critical") && <span className="lore-task-card__prio" data-prio={priority} title={`${titleCase(priority)} priority`} aria-label={`${titleCase(priority)} priority`} />}</div>
    {task.tags.length > 0 && <div className="lore-task-card__tags">{visibleTags.map((tag) => <span className="lore-chip lore-chip--tag" key={tag}>{tag}</span>)}{hiddenTags > 0 && <span className="lore-chip lore-chip--tag">+{hiddenTags}</span>}</div>}
    {hasMeta && <div className="lore-task-card__meta">
      {task.dependencyCount > 0 && <span className="lore-task-card__dep" title="Depends on"><span aria-hidden="true">↳</span>{task.dependencyCount}</span>}
      {task.dependentCount > 0 && <span className="lore-task-card__dep" title="Blocks / dependents"><span aria-hidden="true">↱</span>{task.dependentCount}</span>}
      {task.openAnnotationCount > 0 && <span className="lore-task-card__anno" title="Open annotations">{task.openAnnotationCount}</span>}
    </div>}
  </Link>;
}

function TaskList({ tasks, project }: { tasks: DocumentSummary[]; project: string }) {
  if (!tasks.length) return <div className="lore-empty"><div className="lore-empty__title">No matching tasks</div><div className="lore-empty__hint">Adjust the status, priority, or tag filters to see more of the board.</div></div>;
  return <div className="lore-list">{tasks.map((task) => <Link className="lore-row task-list-row" to={documentHref(project, task)} key={task.id}>
    <span className="lore-task-card__id">#{task.sourceIdentity}</span>
    <span><span className="lore-row__title">{task.title}</span><span className="lore-row__meta"><span className="lore-status" data-state={taskStatusKey(taskStatus(task))}>{taskStatus(task)}</span></span></span>
    <span className="row-tail">{task.tags.slice(0, 3).map((tag) => <span className="lore-chip lore-chip--tag" key={tag}>{tag}</span>)}<span aria-hidden="true">›</span></span>
  </Link>)}</div>;
}

function TasksLoading() {
  return <div className="l-page"><div className="lore-skel lore-skel--title" /><div className="lore-board-scroll"><div className="lore-board" style={{ gridTemplateColumns: "repeat(3, minmax(0, 1fr))" }}>{[0, 1, 2].map((column) => <div className="lore-col" key={column}><div className="lore-col__head"><span className="lore-skel lore-skel--line" /></div><div className="lore-col__body"><div className="lore-skel lore-skel--block" /><div className="lore-skel lore-skel--block" /></div></div>)}</div></div></div>;
}

function TasksEmpty() {
  return <div className="lore-empty"><div className="lore-empty__icon" aria-hidden="true">◇</div><div className="lore-empty__title">No tasks yet</div><div className="lore-empty__hint">Run <span className="lore-mono">lore watch</span> or upload a kanban-md board with <span className="lore-mono">lore upload tasks</span>. This board is a read-only view of that source.</div></div>;
}

function useNarrowScreen(): boolean {
  const query = "(max-width: 640px)";
  const [narrow, setNarrow] = useState(() => typeof matchMedia === "function" && matchMedia(query).matches);
  useEffect(() => {
    if (typeof matchMedia !== "function") return;
    const media = matchMedia(query);
    const changed = () => setNarrow(media.matches);
    changed();
    media.addEventListener("change", changed);
    return () => media.removeEventListener("change", changed);
  }, []);
  return narrow;
}

function priorityRank(priority: string): number {
  const rank: Record<string, number> = { critical: 0, high: 1, medium: 2, low: 3 };
  return rank[priority] ?? 4;
}

function titleCase(value: string): string {
  return value.split("-").map((part) => part ? part[0].toUpperCase() + part.slice(1) : part).join(" ");
}
