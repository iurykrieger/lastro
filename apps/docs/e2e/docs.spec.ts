import { expect, test, type Page } from "@playwright/test";
import { PAGES } from "../src/content/nav";

const isMobile = (page: Page) => (page.viewportSize()?.width ?? 1440) < 1024;

/** Fails the test on any uncaught error or console error. */
function watchErrors(page: Page) {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => {
    if (m.type() === "error") errors.push(m.text());
  });
  return errors;
}

test.describe("every page", () => {
  for (const p of PAGES) {
    test(`${p.slug} renders without errors or horizontal scroll`, async ({ page }) => {
      const errors = watchErrors(page);
      await page.goto(`#/${p.slug}`);
      await expect(page.locator("main h1")).toBeVisible();
      await expect(page).toHaveTitle(`${p.title} · Lastro Docs`);
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
      expect(overflow, "the page must not scroll sideways").toBeLessThanOrEqual(0);
      expect(errors).toEqual([]);
    });
  }
});

test("the root opens the introduction", async ({ page }) => {
  await page.goto("./");
  await expect(page.getByRole("heading", { level: 1, name: "Lastro" })).toBeVisible();
});

test("an unknown route shows a not-found page with a way back", async ({ page }) => {
  await page.goto("#/nope/nothing");
  await expect(page.getByRole("heading", { level: 1, name: "Page not found" })).toBeVisible();
  await page.locator("main").getByRole("link", { name: "introduction" }).click();
  await expect(page).toHaveURL(/#\/introduction$/);
});

test("the sidebar navigates and marks the current page", async ({ page }) => {
  test.skip(isMobile(page), "the sidebar is a menu on small screens");
  await page.goto("#/introduction");
  const nav = page.getByRole("navigation", { name: "Documentation" });
  await nav.getByRole("link", { name: "LLM handoff" }).click();
  await expect(page).toHaveURL(/#\/cli\/llm$/);
  await expect(page.getByRole("heading", { level: 1, name: "LLM handoff" })).toBeVisible();
  await expect(nav.getByRole("link", { name: "LLM handoff" })).toHaveAttribute("aria-current", "page");
});

test("the mobile menu opens, navigates and closes", async ({ page }) => {
  test.skip(!isMobile(page), "desktop shows the sidebar");
  await page.goto("#/introduction");
  await page.getByRole("button", { name: "Open menu" }).click();
  const menu = page.getByTestId("mobile-nav");
  await expect(menu).toBeVisible();
  await menu.getByRole("link", { name: ".NET" }).click();
  await expect(page.getByRole("heading", { level: 1, name: ".NET SDK" })).toBeVisible();
  await expect(menu).toBeHidden();
});

test("previous and next links walk the docs in order", async ({ page }) => {
  await page.goto("#/introduction");
  const pager = page.getByRole("navigation", { name: "Pagination" });
  await pager.getByRole("link", { name: /Next/ }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Installation" })).toBeVisible();
  await pager.getByRole("link", { name: /Previous/ }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Lastro" })).toBeVisible();
});

test("search finds a page by keyword and opens it with Enter", async ({ page }) => {
  await page.goto("#/introduction");
  await expect(page.locator("main h1")).toBeVisible();
  await page.keyboard.press("Control+k");
  const dialog = page.getByRole("dialog", { name: "Search the docs" });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("textbox", { name: "Search pages" }).fill("exit 3");
  await expect(dialog.getByRole("option").first()).toContainText("LLM handoff");
  await page.keyboard.press("Enter");
  await expect(dialog).toBeHidden();
  await expect(page.getByRole("heading", { level: 1, name: "LLM handoff" })).toBeVisible();
});

test("search says so when nothing matches, and Escape closes it", async ({ page }) => {
  await page.goto("#/introduction");
  await page.getByRole("button", { name: "Search the docs" }).click();
  const dialog = page.getByRole("dialog", { name: "Search the docs" });
  await dialog.getByRole("textbox").fill("kubernetes helm");
  await expect(dialog).toContainText("No page matches");
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
});

test("the SDK tab choice is shared across pages and remembered", async ({ page }) => {
  await page.goto("#/concepts/review");
  await page.getByRole("tab", { name: "Go" }).first().click();
  await expect(page.getByRole("tabpanel").first()).toContainText("RequiredApprovals: 2");
  await page.goto("#/concepts/decisions");
  await expect(page.getByRole("tab", { name: "Go" }).first()).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("tabpanel").first()).toContainText("l.ReturnDecisions(spec)");
  await page.reload();
  await expect(page.getByRole("tab", { name: "Go" }).first()).toHaveAttribute("aria-selected", "true");
});

test("the theme toggle switches to dark and is remembered", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("#/introduction");
  await page.getByRole("button", { name: "Switch to dark theme" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  const bg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  expect(bg).not.toBe("rgb(247, 247, 248)");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.getByRole("button", { name: "Switch to light theme" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
});

test("copying a shell block drops the prompts", async ({ page, context, browserName }) => {
  test.skip(browserName !== "chromium", "clipboard permissions are Chromium-only");
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("#/quickstart");
  const block = page.locator("figure[data-code]").filter({ hasText: "lastro init --model model.json" }).first();
  await block.getByRole("button", { name: "Copy code" }).click();
  await expect(block.getByRole("button", { name: "Copied" })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("lastro init --model model.json");
});

test("the outline links to sections of the page", async ({ page }) => {
  test.skip(isMobile(page), "the outline is shown on wide screens");
  await page.goto("#/quickstart");
  const outline = page.getByRole("navigation", { name: "On this page" });
  await outline.getByRole("link", { name: "Prove the code" }).click();
  await expect(page).toHaveURL(/#\/quickstart~prove-the-code$/);
  await expect(page.locator("#prove-the-code")).toBeInViewport();
});

test("a deep link scrolls to its section", async ({ page }) => {
  await page.goto("#/cli/overview~exit-codes");
  await expect(page.locator("#exit-codes")).toBeInViewport();
});

test("the quickstart shows the real counterexample and receipt", async ({ page }) => {
  await page.goto("#/quickstart");
  await expect(page.getByText('"status": "not-proved"')).toBeVisible();
  await expect(page.getByText('"subtotal": 100').first()).toBeAttached();
  await expect(page.getByText("Engines agree: yes.")).toBeAttached();
  await expect(page.locator("figure[data-code]").filter({ has: page.locator("figcaption", { hasText: "UC-001-charge-shipping.json" }) })).toContainText('"history"');
});

test("the proposals page explains the four entities and links from the pipeline", async ({ page }) => {
  await page.goto("#/concepts/pipeline");
  await expect(page.getByRole("figure", { name: "The Lastro pipeline" })).toContainText("apply");
  await page.locator("main").getByRole("link", { name: "proposal", exact: true }).first().click();
  await expect(page.getByRole("heading", { level: 1, name: "Use cases and proposals" })).toBeVisible();
  for (const id of ["UC-001", "P-001", "P-001.I-01", "P-001.PF-01"]) await expect(page.locator("main table").first()).toContainText(id);
  await expect(page.getByText('"replaces": [{ "proof": "P-001.PF-01"')).toBeAttached();
});

test("the plugin page shows the progress board at each stage", async ({ page }) => {
  await page.goto("#/plugins/agents");
  await expect(page.getByRole("heading", { level: 1, name: "Claude Code and Codex" })).toBeVisible();
  const board = (title: string) => page.locator("figure[data-code]").filter({ has: page.locator("figcaption", { hasText: title }) });
  await expect(board("the board, review pending")).toContainText("? D1 Is the 100 threshold measured before or after discounts?");
  await expect(board("the board, implementing")).toContainText("○ PF-01 pending");
  const failed = board("the board, a proof failed");
  await expect(failed).toContainText("✗ PF-01 violated");
  await expect(failed).toContainText("Verification 3/4 passed, 1 failed");
  await expect(page.getByRole("heading", { level: 3, name: "You review, like a pull request" })).toBeVisible();
  await expect(page.getByRole("heading", { level: 3, name: "The agent proposes, before its first edit" })).toBeVisible();
  await expect(page.getByRole("heading", { level: 2, name: "The live board in Claude Code" })).toBeVisible();
  await expect(page.locator("figure[data-code]").filter({ hasText: "/plugin install lastro-board" })).toBeVisible();
});

test("the CLI page shows a streamed proof run, one event per line", async ({ page }) => {
  await page.goto("#/cli/overview~streaming-a-run");
  const block = page.locator("figure[data-code]").filter({ has: page.locator("figcaption", { hasText: "--stream" }) });
  await expect(block).toBeInViewport();
  await expect(block).toContainText('{"event":"start","proposal":"P-001"');
  await expect(block).toContainText('{"event":"proof","proof":"P-001.PF-01","role":"proposal","verdict":"violated"');
  await expect(block).toContainText('{"event":"result","status":"not-proved"');
});

test("search finds the plugin page", async ({ page }) => {
  await page.goto("#/introduction");
  await expect(page.locator("main h1")).toBeVisible();
  await page.keyboard.press("Control+k");
  const dialog = page.getByRole("dialog", { name: "Search the docs" });
  await dialog.getByRole("textbox", { name: "Search pages" }).fill("codex hooks");
  await expect(dialog.getByRole("option").first()).toContainText("Claude Code and Codex");
});
