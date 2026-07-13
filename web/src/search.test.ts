import { describe, expect, it } from "vitest";
import { safeHeadline } from "./search";

describe("safeHeadline", () => {
  it("retains only PostgreSQL headline marks", () => {
    expect(safeHeadline('A <mark>match</mark> <img src=x onerror="alert(1)">')).toBe(
      "A <mark>match</mark> &lt;img src=x onerror=&quot;alert(1)&quot;&gt;",
    );
  });

  it("does not promote encoded user content into markup", () => {
    expect(safeHeadline("&lt;mark&gt;authored&lt;/mark&gt;")).toBe(
      "&amp;lt;mark&amp;gt;authored&amp;lt;/mark&amp;gt;",
    );
  });
});
