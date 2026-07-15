/*
---
relationships:
  references: system
---
*/
import { describe, expect, it } from "vitest";
import { displayUpdatedAt } from "./browse";
import type { DocumentSummary } from "./types";

function note(metadata: Record<string, unknown>): DocumentSummary {
  return {
    id: "note", sourceType: "note", sourceInstance: "mnemonic", sourceIdentity: "note", title: "Note",
    revisionId: "revision", metadata, tags: [], createdAt: "2026-07-01T00:00:00Z", updatedAt: "2026-07-15T00:00:00Z",
    chunkCount: 1, embeddedChunkCount: 1, dependencyCount: 0, dependentCount: 0, openAnnotationCount: 0,
  };
}

describe("displayUpdatedAt", () => {
  it("prefers the note's authored updatedAt value", () => {
    expect(displayUpdatedAt(note({ updatedAt: "2026-07-04T00:00:00Z" }))).toBe("2026-07-04T00:00:00Z");
  });

  it("falls back to the Lore synchronization timestamp", () => {
    expect(displayUpdatedAt(note({}))).toBe("2026-07-15T00:00:00Z");
  });
});
