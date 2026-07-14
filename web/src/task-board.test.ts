import { describe, expect, it } from "vitest";
import { orderedTaskLanes, taskStatusKey, taskStatusSlug } from "./task-board";

describe("task board status vocabulary", () => {
  it("slugs status names and tolerates lifecycle synonyms", () => {
    expect(taskStatusSlug(" In_Progress ")).toBe("in-progress");
    for (const status of ["in progress", "in_progress", "doing", "wip"]) expect(taskStatusKey(status)).toBe("in-progress");
    for (const status of ["qa", "in-review"]) expect(taskStatusKey(status)).toBe("review");
    for (const status of ["completed", "shipped"]) expect(taskStatusKey(status)).toBe("done");
    for (const status of ["icebox", "triage"]) expect(taskStatusKey(status)).toBe("backlog");
  });

  it("sorts lifecycle lanes while keeping custom lanes with their source anchor", () => {
    const lanes = orderedTaskLanes(["done", "Ready for deploy", "backlog", "archived", "in progress"]);
    expect(lanes.map((lane) => lane.key)).toEqual(["backlog", "in-progress", "done", "ready-for-deploy", "archived"]);
    expect(lanes.find((lane) => lane.key === "ready-for-deploy")).toMatchObject({ label: "Ready for deploy", recognized: false });
  });

  it("keeps leading and adjacent custom lanes in source order", () => {
    const lanes = orderedTaskLanes(["Intake", "blocked", "done", "Deploy", "Verify", "todo"]);
    expect(lanes.map((lane) => lane.key)).toEqual(["intake", "blocked", "todo", "done", "deploy", "verify"]);
  });

  it("never reverses custom lanes when lifecycle lanes move around them", () => {
    const lanes = orderedTaskLanes(["done", "Deploy", "backlog", "Verify", "shipped", "Observe"]);
    expect(lanes.map((lane) => lane.key)).toEqual(["backlog", "done", "deploy", "verify", "observe"]);
  });
});
