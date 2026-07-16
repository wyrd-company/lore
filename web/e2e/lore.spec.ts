import { expect, type Page, test } from "@playwright/test";

test("validates the complete archive journey with real services", async ({ page }) => {
  await page.goto("/e2e-primary");
  await expect(page.getByRole("heading", { name: "Lore E2E archive" })).toBeVisible();
  await expect(page.getByText(/documents held across/)).toBeVisible();

  await page.getByRole("link", { name: /Tasks 2/ }).click();
  await expect(page.getByRole("button", { name: "Board", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator(".filter-panel")).not.toHaveAttribute("open", "");
  await page.locator(".filter-panel > summary").click();
  const filterGroupTops = await page.locator(".filter-panel .facet-group").evaluateAll((groups) => groups.map((group) => Math.round(group.getBoundingClientRect().top)));
  expect(new Set(filterGroupTops).size).toBe(filterGroupTops.length);
  await expect(page.locator(".lore-col__label")).toHaveText(["Backlog", "In Progress", "Done", "Ready for deploy", "Archived"]);
  const laneWidths = await page.locator(".lore-col").evaluateAll((lanes) => lanes.map((lane) => lane.getBoundingClientRect().width));
  expect(Math.max(...laneWidths) - Math.min(...laneWidths)).toBeLessThanOrEqual(1);

  const foundationCard = page.locator(".lore-task-card").filter({ hasText: "Build foundation" });
  await expect(foundationCard.locator('.lore-task-card__prio[data-prio="high"]')).toBeVisible();
  await expect(foundationCard.getByTitle("Blocks / dependents")).toContainText("1");
  await expect(foundationCard.getByTitle("Open annotations")).toHaveText("1");
  await expect(page.locator(".lore-task-card").filter({ hasText: "Build adapters" }).getByTitle("Depends on")).toContainText("1");

  await page.locator(".lore-facet").filter({ hasText: "architecture" }).click();
  await expect(page).toHaveURL(/tag=architecture/);
  await expect(page.locator('.lore-col[data-status="backlog"] .lore-task-card')).toHaveCount(0);
  await expect(page.locator('.lore-col[data-status="backlog"] .lore-col__empty')).toHaveText("Nothing here");
  await expect(foundationCard).toBeVisible();
  await page.locator(".lore-facet").filter({ hasText: "High" }).click();
  await expect(page).toHaveURL(/priority=high/);
  await page.locator(".lore-facet").filter({ hasText: "Done" }).click();
  await expect(page).toHaveURL(/status=done/);
  await page.getByRole("button", { name: "List", exact: true }).click();
  await expect(page).toHaveURL(/view=list/);
  await expect(page.locator(".task-list-row").filter({ hasText: "Build foundation" })).toBeVisible();
  await expect(page.locator(".task-list-row").filter({ hasText: "Build adapters" })).toHaveCount(0);

  await page.setViewportSize({ width: 600, height: 900 });
  await page.goto("/e2e-primary/tasks");
  await expect(page.getByRole("button", { name: "List", exact: true })).toHaveAttribute("aria-pressed", "true");
  await page.setViewportSize({ width: 1280, height: 900 });
  await expect(page.getByRole("button", { name: "Board", exact: true })).toHaveAttribute("aria-pressed", "true");
  await page.locator(".lore-task-card").filter({ hasText: "Build foundation" }).click();
  await expect(page.getByRole("heading", { name: "Build foundation", level: 1 })).toBeVisible();
  await expect(page.locator(".task-number")).toHaveText("#1");
  await expect(page.getByLabel("Revision", { exact: true })).toHaveCount(0);
  await page.getByRole("link", { name: /Build adapters/ }).click();
  await expect(page.getByRole("heading", { name: "Build adapters", level: 1 })).toBeVisible();
  await page.getByRole("link", { name: /Build foundation/ }).click();

  await page.getByRole("link", { name: /Notes 4/ }).click();
  await expect(page.locator(".lore-row").filter({ hasText: "Adapter finding" }).locator("time")).toHaveAttribute("datetime", "2026-07-04T00:00:00Z");
  await page.locator(".filter-panel > summary").click();
  await page.locator(".lore-facet").filter({ hasText: "summary" }).click();
  await expect(page).toHaveURL(/role=summary/);
  await page.locator(".lore-facet").filter({ hasText: /^lore/ }).first().click();
  await expect(page).toHaveURL(/tag=lore/);
  await page.locator(".lore-facet").filter({ hasText: "permanent" }).click();
  await expect(page).toHaveURL(/lifecycle=permanent/);
  await page.locator(".lore-facet").filter({ hasText: /^lore/ }).last().click();
  await expect(page).toHaveURL(/projectName=lore/);
  await page.getByLabel("Sort").selectOption("title");
  await expect(page).toHaveURL(/sort=title/);
  await openRow(page, "Adapter finding");
  await expect(page.getByRole("heading", { name: "Adapter finding", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Related notes" })).toBeVisible();
  await expect(page.getByRole("link", { name: /Related adapter context/ })).toBeVisible();
  const revision = page.getByLabel("Revision", { exact: true });
  await expect(revision).toBeVisible();
  const priorValue = await revision.locator("option").filter({ hasText: "annotations" }).getAttribute("value");
  expect(priorValue).toBeTruthy();
  await revision.selectOption(priorValue!);
  await expect(page.getByText(/Retained revision/)).toBeVisible();
  const prepared = page.locator(".lore-anno").filter({ hasText: "Prepared annotation on the prior revision" });
  await expect(prepared).toHaveAttribute("data-state", "open");
  await page.getByRole("textbox", { name: "Annotation attribution name" }).fill("Browser E2E");
  await prepared.getByLabel("Target revision").selectOption({ label: "Current" });
  await Promise.all([
    page.waitForResponse((response) => response.url().endsWith("/copy") && response.ok()),
    prepared.getByRole("button", { name: "Copy" }).click(),
  ]);
  await page.goto(page.url().split("?")[0]);
  const copied = page.locator(".lore-anno").filter({ hasText: "Prepared annotation on the prior revision" });
  await expect(copied).toContainText("copied from");
  await copied.getByLabel("Target revision").selectOption({ index: 1 });
  await Promise.all([
    page.waitForResponse((response) => response.url().endsWith("/move") && response.ok()),
    copied.getByRole("button", { name: "Move" }).click(),
  ]);
  await expect(copied).toHaveCount(0);

  const resolvedBody = `Browser resolved ${Date.now()}`;
  await createTextAnnotation(page, "provenance", resolvedBody);
  const resolved = page.locator(".lore-anno").filter({ hasText: resolvedBody });
  await resolved.getByRole("button", { name: "Resolve" }).click();
  await expect(resolved).toHaveAttribute("data-state", "resolved");
  const dismissedBody = `Browser dismissed ${Date.now()}`;
  await createTextAnnotation(page, "identity", dismissedBody);
  const dismissed = page.locator(".lore-anno").filter({ hasText: dismissedBody });
  await dismissed.getByRole("button", { name: "Dismiss" }).click();
  await expect(dismissed).toHaveAttribute("data-state", "dismissed");

  await page.getByRole("link", { name: /Annotations/ }).click();
  await expect(page).not.toHaveURL(/status=/);
  await expect(page.getByText(resolvedBody)).toHaveCount(0);
  await page.locator(".filter-panel > summary").click();
  await page.locator(".lore-facet").filter({ hasText: "resolved" }).click();
  await expect(page).toHaveURL(/status=resolved/);
  await expect(page.getByText(resolvedBody)).toBeVisible();
  await expect(page.getByText(dismissedBody)).toHaveCount(0);
  await page.locator(".lore-facet").filter({ hasText: "All" }).click();
  await page.locator(".annotation-index__item").filter({ hasText: "retained revision" }).first().getByRole("button", { name: /View annotation on Adapter finding/ }).click();
  await expect(page.locator(".annotation-revision-pane")).toHaveCount(2);
  await expect(page.getByText("Annotated version", { exact: true })).toBeVisible();
  await expect(page.getByText("Current version", { exact: true })).toBeVisible();
  await page.getByLabel("Reply", { exact: true }).fill("Browser reply on retained context");
  await page.getByRole("button", { name: "Reply", exact: true }).click();
  await expect(page.getByText("Browser reply on retained context")).toBeVisible();
  await Promise.all([
    page.waitForResponse((response) => response.request().method() === "PATCH" && response.url().includes("/annotations/") && response.ok()),
    page.getByRole("button", { name: "Resolve", exact: true }).click(),
  ]);
  await expect(page.locator(".annotation-index__item.is-active .lore-status")).toHaveText("resolved");

  await page.getByRole("link", { name: /Watcher issues/ }).click();
  await expect(page.locator(".watcher-issue")).toContainText("broken.md");
  await page.getByRole("button", { name: "Retry" }).click();
  await expect(page.getByText("No watcher issues")).toBeVisible();

  await page.getByRole("link", { name: /Briefings 2/ }).click();
  await openRow(page, "Architecture");
  await page.getByLabel("Briefing settings").getByRole("textbox").fill("Foundations");
  await page.getByRole("button", { name: "Save category" }).click();
  await expect(page.getByText("Category saved")).toBeVisible();
  await page.getByRole("button", { name: "Set as home" }).click();
  await expect(page.getByText("Home briefing set")).toBeVisible();
  await page.getByRole("button", { name: "List", exact: true }).click();
  await expect(page).toHaveURL(/briefings\?view=list/);
  await openRow(page, "Operations");
  await page.getByLabel("Briefing settings").getByRole("textbox").fill("Operations");
  await page.getByRole("button", { name: "Save category" }).click();
  await expect(page.getByText("Category saved")).toBeVisible();
  await page.goto("/e2e-primary/briefings");
  await expect(page).toHaveURL(/\/briefings\/[^?]+$/);
  await expect(page.getByRole("heading", { name: "Architecture", level: 1, exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Home", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("complementary", { name: "Other briefings" }).getByRole("link", { name: "Architecture" })).toHaveAttribute("aria-current", "page");
  await expect(page.locator(".brief-category").filter({ hasText: "Foundations" })).toHaveAttribute("open", "");
  await expect(page.locator(".brief-category").filter({ hasText: "Operations" })).not.toHaveAttribute("open", "");
  await expect(page.locator(".document-content #boundary")).toContainText("source files authoritative");
  const svgTaxonomyLink = page.locator('.document-content svg .taxonomy-link[data-taxonomy="architecture"]');
  await expect(svgTaxonomyLink).toBeVisible();
  await expect(svgTaxonomyLink).toHaveText("architecture");
  await expect(svgTaxonomyLink).toHaveAttribute("href", "/e2e-primary/search?q=architecture&tag=architecture");
  await expect(page.locator('.document-content svg text')).toHaveText("architecture foundation");
  await expect(svgTaxonomyLink).toHaveJSProperty("namespaceURI", "http://www.w3.org/2000/svg");

  await page.getByRole("link", { name: /Repository 3/ }).click();
  await expect(page.getByRole("heading", { name: "git@github.com:wyrd-company/lore-e2e-fixture.git" })).toBeVisible();
  await expect(page.getByText(/e2e\/real-services/)).toBeVisible();
  await page.locator(".filter-panel > summary").click();
  await page.locator(".lore-facet").filter({ hasText: /^term/ }).click();
  await expect(page).toHaveURL(/schema=term/);
  await expect(page.locator(".lore-row")).toHaveCount(1);
  await page.locator(".lore-facet").filter({ hasText: /^term/ }).click();
  await openRow(page, "Fixture repository");
  await expect(page.locator(".document-content")).toContainText("shared renderer");
  await page.getByRole("link", { name: /Repository 3/ }).click();
  await openRow(page, "project");
  await expect(page.locator(".document-content [data-yaml-path]").first()).toBeVisible();
  await expect(page.locator('.document-content .taxonomy-link--term[data-taxonomy="knowledge-portal"]')).toBeVisible();
  await expect(page.locator('.document-content .taxonomy-link--tag[data-taxonomy="architecture"]')).toBeVisible();
  await expect(page.locator(".terms-footer").getByRole("link", { name: "Knowledge Portal" })).toBeVisible();
  await page.getByRole("link", { name: /Terms 1/ }).click();
  await openRow(page, "Knowledge Portal");
  await expect(page.getByRole("heading", { name: "Knowledge Portal", level: 1 })).toBeVisible();
  await page.getByRole("link", { name: /Terms 1/ }).click();
  const missingTerms = page.getByRole("heading", { name: "Missing definitions" }).locator("..");
  await expect(missingTerms).toBeVisible();
  await missingTerms.locator('a[href="/e2e-primary/terms/knowledge"]').click();
  await expect(page.getByText("No definition has been uploaded")).toBeVisible();

  await page.getByRole("link", { name: /Conversations 2/ }).click();
  await openRow(page, "Implement Codex normalization");
  await expect(page.locator(".lore-msg[data-role=user]")).toContainText("Implement Codex normalization");
  await expect(page.locator(".lore-thinking")).not.toHaveAttribute("open", "");
  await page.getByRole("link", { name: /Conversations 2/ }).click();
  await openRow(page, "Build the adapter");
  await expect(page.locator(".lore-msg[data-role=assistant]").filter({ hasText: "implement it" })).toBeVisible();

  await page.goto("/e2e-primary/search?q=foundation%20architecture&sourceType=task&tag=architecture&datePreset=24h&createdFrom=2000-01-01T00%3A00%3A00.000Z");
  await expect(page.getByRole("button", { name: "Vector" })).toBeEnabled();
  await expect(page.getByRole("link", { name: "Build foundation", exact: true })).toBeVisible();
  await expect(page).toHaveURL(/sourceType=task/);
  await expect(page).toHaveURL(/tag=architecture/);
  await page.goto("/e2e-primary/search?q=shared%20renderer&sourceType=repository&repository=git%40github.com%3Awyrd-company%2Flore-e2e-fixture.git&branch=e2e%2Freal-services");
  await expect(page.getByRole("link", { name: "Fixture repository", exact: true })).toBeVisible();
  await expect(page).toHaveURL(/repository=/);
  await expect(page).toHaveURL(/branch=/);

  await page.goto("/e2e-primary/search?q=velvet-quasar-719");
  await expect(page.getByRole("link", { name: "Quasar isolation ledger", exact: true })).toHaveCount(0);
  await page.locator(".project-menu summary").click();
  await page.getByRole("button", { name: "Isolated archive" }).click();
  await expect(page).toHaveURL(/\/e2e-isolated$/);
  await expect(page.locator(".project-menu summary")).toContainText("Isolated archive");
  await expect(page.locator(".project-menu")).not.toHaveAttribute("open", "");
  const search = page.getByRole("textbox", { name: "Search this project" });
  await search.fill("velvet-quasar-719");
  await search.press("Enter");
  await expect(page.getByRole("link", { name: "Quasar isolation ledger", exact: true })).toBeVisible();
});

async function openRow(page: Page, title: string) {
  await page.locator(".lore-row").filter({ hasText: title }).click();
}

async function createTextAnnotation(page: Page, quote: string, body: string) {
  await page.locator(".document-content").evaluate((root, selected) => {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    let node: Node | null;
    while ((node = walker.nextNode())) {
      const index = node.textContent?.indexOf(selected) ?? -1;
      if (index >= 0) {
        const range = document.createRange();
        range.setStart(node, index);
        range.setEnd(node, index + selected.length);
        const selection = getSelection();
        selection?.removeAllRanges();
        selection?.addRange(range);
        return;
      }
    }
    throw new Error(`quote not found: ${selected}`);
  }, quote);
  await page.locator(".document-content").dispatchEvent("mouseup");
  await page.locator(".lore-anno-pop button").click();
  await page.getByLabel("Note").fill(body);
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.locator(".lore-anno").filter({ hasText: body })).toHaveAttribute("data-state", "open");
}
