import { expect, test } from "@playwright/test";

test("browses every source, searches, and annotates a real revision", async ({ page }) => {
  await page.goto("/web-e2e");
  await expect(page.getByRole("heading", { name: "Web interface smoke" })).toBeVisible();
  await expect(page.getByText("7 documents held across 5 source instances.")).toBeVisible();

  await page.getByRole("link", { name: /Tasks 2/ }).click();
  await page.locator(".lore-row").filter({ hasText: "Build foundation" }).click();
  await expect(page.getByRole("heading", { name: "Build foundation", level: 1 })).toBeVisible();
  await expect(page.getByRole("link", { name: /Build adapters/ })).toBeVisible();

  await page.getByRole("link", { name: /Notes 1/ }).click();
  await expect(page).toHaveURL(/\/web-e2e\/notes$/);
  await page.locator(".lore-row").filter({ hasText: "Adapter finding" }).click();
  await expect(page.getByRole("heading", { name: "Adapter finding", level: 1, exact: true })).toBeVisible();
  await expect(page.locator(".document-content")).toContainText("provenance");

  await page.getByRole("link", { name: /Briefings 1/ }).click();
  await expect(page).toHaveURL(/\/web-e2e\/briefings$/);
  await page.locator(".lore-row").filter({ hasText: "Architecture" }).click();
  await expect(page.locator(".document-content #boundary")).toContainText("source files authoritative");

  await page.getByRole("link", { name: /Repository 2/ }).click();
  await expect(page).toHaveURL(/\/web-e2e\/repo$/);
  await page.locator(".lore-row").filter({ hasText: "Fixture repository" }).click();
  await expect(page.locator(".document-content")).toContainText("shared renderer");

  await page.getByRole("link", { name: /Conversations 1/ }).click();
  await expect(page).toHaveURL(/\/web-e2e\/conversations$/);
  await page.locator(".lore-row").filter({ hasText: "Implement Codex normalization" }).click();
  await expect(page.locator(".lore-msg[data-role=user]")).toContainText("Implement Codex normalization");
  await expect(page.locator(".lore-thinking")).not.toHaveAttribute("open", "");

  const search = page.getByRole("textbox", { name: "Search this project" });
  await search.fill("foundation");
  await search.press("Enter");
  await expect(page).toHaveURL(/\/web-e2e\/search\?q=foundation/);
  await expect(page.getByRole("link", { name: "Build foundation", exact: true })).toBeVisible();

  await page.getByRole("link", { name: /Notes 1/ }).click();
  await expect(page).toHaveURL(/\/web-e2e\/notes$/);
  await page.locator(".lore-row").filter({ hasText: "Adapter finding" }).click();
  await page.getByRole("textbox", { name: "Annotation attribution name" }).fill("Browser smoke");
  await page.locator(".document-content").evaluate((root) => {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    let node: Node | null;
    while ((node = walker.nextNode())) {
      const index = node.textContent?.indexOf("provenance") ?? -1;
      if (index >= 0) {
        const range = document.createRange();
        range.setStart(node, index); range.setEnd(node, index + "provenance".length);
        const selection = getSelection(); selection?.removeAllRanges(); selection?.addRange(range);
        break;
      }
    }
  });
  await page.locator(".document-content").dispatchEvent("mouseup");
  await page.locator(".lore-anno-pop button").click();
  const annotationBody = `E2E annotation ${Date.now()}`;
  await page.getByLabel("Note").fill(annotationBody);
  await page.getByRole("button", { name: "Save" }).click();
  const card = page.locator(".lore-anno").filter({ hasText: annotationBody });
  await expect(card).toHaveAttribute("data-state", "open");
  await card.getByRole("button", { name: "Resolve" }).click();
  await expect(card).toHaveAttribute("data-state", "resolved");
});
