import { describe, expect, it } from "vitest";
import { taxonomyMatches } from "./taxonomy";

describe("taxonomy matching", () => {
  it("prefers the longest term title and links normalized names", () => {
    const matches = taxonomyMatches("A Knowledge Portal stores knowledge and lore.", ["lore", "knowledge"], [
      { name: "knowledge-portal", title: "Knowledge Portal", defined: true, referenceCount: 1 },
      { name: "knowledge", title: "Knowledge", defined: false, referenceCount: 2 },
    ]);
    expect(matches.map(({ kind, name }) => [kind, name])).toEqual([
      ["term", "knowledge-portal"],
      ["term", "knowledge"],
      ["tag", "lore"],
    ]);
  });

  it("does not match taxonomy names inside larger words", () => {
    expect(taxonomyMatches("exploration lore", ["lore"], [])).toEqual([
      { start: 12, end: 16, kind: "tag", name: "lore" },
    ]);
  });

  it("orders equal-length labels deterministically", () => {
    expect(taxonomyMatches("beta alpha", ["beta", "alpha"], [])).toEqual([
      { start: 0, end: 4, kind: "tag", name: "beta" },
      { start: 5, end: 10, kind: "tag", name: "alpha" },
    ]);
  });
});
