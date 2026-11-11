import { expect, test, type Page } from "@playwright/test";

const gates = (page: Page) => page.getByRole("list", { name: "Pipeline gates" });
const gate = (page: Page, name: string) => gates(page).getByRole("listitem", { name, exact: true });
const feedback = (page: Page) => page.getByRole("region", { name: "Lastro feedback" });
const intent = (page: Page, id: string) => page.getByRole("listitem", { name: `Intent ${id}`, exact: true });
const decision = (page: Page, id: string) => page.getByRole("group", { name: `Decision ${id}`, exact: true });

const pageErrors = new WeakMap<Page, string[]>();

test.beforeEach(async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(String(e)));
  page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
  await page.goto("/");
  await expect(feedback(page)).toBeVisible();
  pageErrors.set(page, errors);
});

test.afterEach(async ({ page }) => {
  expect(pageErrors.get(page)).toEqual([]);
});

test.describe("Lastro feedback at the top", () => {
  test("sits right under the pipeline gates", async ({ page }) => {
    const g = await gates(page).boundingBox();
    const f = await feedback(page).boundingBox();
    const prose = await page.getByRole("tab", { name: /Intents/ }).boundingBox();
    expect(g && f && prose).toBeTruthy();
    expect(f!.y).toBeGreaterThan(g!.y + g!.height - 1);
    expect(f!.y).toBeLessThan(prose!.y);
  });

  test("says every gate passes on the reviewed example", async ({ page }) => {
    await expect(feedback(page)).toContainText("Every gate passes");
    await expect(gate(page, "Prove")).toContainText("every proof passes");
    await expect(gate(page, "Pull request")).toContainText("proved code and receipt");
  });

  test("hands failing obligations to whoever writes the code, matching the Prove gate", async ({ page }) => {
    await page.getByRole("button", { name: /Skip the password check/ }).click();
    const fb = feedback(page);
    await expect(fb).toContainText(/(\d+) proof obligations? fail/);
    await expect(fb).toContainText("next: Whoever writes the code");
    await expect(fb).toContainText("counterexample:");
    await expect(fb.getByRole("button", { name: /Copy for the agent/ })).toBeVisible();

    const n = (await fb.innerText()).match(/(\d+) proof obligations? fail/)![1];
    await expect(gate(page, "Prove")).toContainText(`${n} obligation(s) fail`);
    await expect(gate(page, "Pull request")).toContainText("blocked until proofs pass");

    await page.getByRole("button", { name: /Reset code/ }).click();
    await expect(fb).toContainText("Every gate passes");
  });

  test("asks the team to finish the review on a fresh AI draft", async ({ page }) => {
    await page.getByRole("button", { name: /Start from the AI draft/ }).click();
    const fb = feedback(page);
    await expect(fb).toContainText(/Review is not finished \(\d+ items\)/);
    await expect(fb).toContainText("next: The team");
    await expect(fb).toContainText("I1 not reviewed");
    await expect(gate(page, "Formal spec")).toContainText("waits for the review");
  });
});

test.describe("Intents and decisions in one tab", () => {
  test.beforeEach(async ({ page }) => {
    await page.getByRole("button", { name: /Start from the AI draft/ }).click();
    await page.getByRole("tab", { name: /Intents/ }).click();
  });

  test("there is no separate Decisions tab; the Intents tab counts decisions", async ({ page }) => {
    await expect(page.getByRole("tab", { name: /^Decisions/ })).toHaveCount(0);
    await expect(page.getByRole("tab", { name: /Intents \(\d+\) · \d+ decisions?/ })).toBeVisible();
  });

  test("each decision sits under the first related intent still waiting for approval", async ({ page }) => {
    const d1 = decision(page, "D1");
    await expect(d1).toHaveCount(1);
    await expect(d1).toContainText("affects I1, I2");
    await expect(intent(page, "I1").getByRole("group", { name: "Decision D1" })).toBeVisible();

    // Approving I1 moves the open decision to I2, the next related intent not yet approved.
    await intent(page, "I1").getByRole("button", { name: /^Approve as/ }).click();
    await expect(intent(page, "I1")).toContainText(/approved/);
    await expect(intent(page, "I1").getByRole("group", { name: "Decision D1" })).toHaveCount(0);
    await expect(intent(page, "I2").getByRole("group", { name: "Decision D1" })).toBeVisible();
    await expect(decision(page, "D1")).toHaveCount(1);
  });

  test("an answered decision collapses to its answer once its intents are approved", async ({ page }) => {
    const d1 = decision(page, "D1");
    await d1.getByRole("radio").first().click();
    await expect(d1.getByRole("radio").first()).toBeChecked();
    await intent(page, "I1").getByRole("button", { name: /^Approve as/ }).click();
    await intent(page, "I2").getByRole("button", { name: /^Approve as/ }).click();

    const settled = decision(page, "D1");
    await expect(settled).toContainText("Decided:");
    await expect(settled.getByRole("radio")).toHaveCount(0);
    await settled.getByRole("button", { name: "Change" }).click();
    await expect(settled.getByRole("radio")).not.toHaveCount(0);
  });

  test("approving everything clears the review gate", async ({ page }) => {
    await page.getByRole("button", { name: /Approve everything as/ }).click();
    await expect(feedback(page)).not.toContainText("Review is not finished");
    await expect(gate(page, "Plan")).toContainText("intents approved");
  });
});
