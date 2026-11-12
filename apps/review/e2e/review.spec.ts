import { appendFileSync, existsSync, readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, type Browser, type Page } from "@playwright/test";
import { WORK_DIR, resetWorkspace } from "./fixture";

/** Fails the test on any uncaught error, and on console errors other than the API's expected 4xx answers. */
function watchErrors(page: Page) {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => {
    if (m.type() === "error" && !/status of 4\d\d/.test(m.text())) errors.push(m.text());
  });
  return errors;
}

async function reviewAs(page: Page, name: string) {
  const change = page.getByRole("button", { name: /Change reviewer/ });
  if (await change.isVisible()) await change.click();
  await page.getByLabel("Your name").fill(name);
  await page.getByRole("button", { name: "Review as" }).click();
  await expect(page.getByRole("button", { name: `Reviewing as ${name}. Change reviewer` })).toBeVisible();
}

const intent = (page: Page, id: string) => page.locator(`[data-intent="P-002.${id}"]`);
const proposalOnDisk = () => JSON.parse(readFileSync(join(WORK_DIR, "proposals", "P-002-raise-free-shipping-threshold.json"), "utf8"));

async function openChanges(page: Page, name: string) {
  await page.goto("#/proposals/P-002/changes");
  await reviewAs(page, name);
  await expect(intent(page, "I-01")).toBeVisible();
}

async function newReviewer(browser: Browser, name: string) {
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  await openChanges(page, name);
  return { ctx, page };
}

test.beforeEach(() => resetWorkspace());

test("proposals list, proposal and use cases render without errors or horizontal scroll", async ({ page }) => {
  const errors = watchErrors(page);
  for (const [url, heading] of [
    ["#/proposals", "Proposals"],
    ["#/proposals/P-002", "Raise the free shipping threshold"],
    ["#/proposals/P-002/changes", "Raise the free shipping threshold"],
    ["#/proposals/P-002/proofs", "Raise the free shipping threshold"],
    ["#/proposals/P-002/timeline", "Raise the free shipping threshold"],
    ["#/proposals/P-003/timeline", "Discount codes"],
    ["#/usecases", "Use cases"],
    ["#/usecases/UC-001", "Free shipping over a threshold"],
  ] as const) {
    await page.goto(url);
    await expect(page.getByRole("heading", { level: 1 })).toContainText(heading);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow, `${url} must not scroll sideways`).toBeLessThanOrEqual(0);
  }
  expect(errors).toEqual([]);
});

test("the proposals list shows status and progress, and opens a proposal", async ({ page }) => {
  await page.goto("./");
  const rows = page.getByTestId("proposal-row");
  await expect(rows).toHaveCount(3);
  await expect(rows.first()).toContainText("P-003");
  await expect(rows.first()).toContainText("Proving");
  const p2 = rows.filter({ hasText: "P-002" });
  await expect(p2).toContainText("In review");
  await expect(p2).toContainText("0/3 intents settled");
  await expect(rows.filter({ hasText: "P-001" })).toContainText("Applied");
  await p2.getByRole("link").click();
  await expect(page).toHaveURL(/#\/proposals\/P-002$/);
  await expect(page.getByTestId("prose")).toContainText("Free shipping now starts at 150");
  await expect(page.getByTestId("decision")).toContainText("Does the shipping estimate round to whole units?");
  await expect(page.getByTestId("review-box")).toContainText("0 of 3 intents settled");
  await expect(page.getByTestId("review-box")).toContainText("2 approvals per intent; the submitter (ana) does not count");
});

test("use cases show their proofs, history and the changes proposed to them", async ({ page }) => {
  await page.goto("#/usecases");
  await expect(page.getByTestId("usecase-row")).toHaveCount(2);
  await page.getByLabel("Filter use cases").fill("checkout");
  await expect(page.getByTestId("usecase-row")).toHaveCount(1);
  await page.getByLabel("Filter use cases").fill("");
  await page.getByRole("link", { name: /UC-001/ }).click();
  await expect(page.getByTestId("proof")).toContainText("Free over 100");
  await expect(page.getByRole("region", { name: "Proposed changes" })).toContainText("P-002 Raise the free shipping threshold");
  await expect(page.getByTestId("diff").locator("ins").first()).toBeVisible();
  await page.getByRole("link", { name: "P-002 Raise the free shipping threshold" }).click();
  await expect(page).toHaveURL(/#\/proposals\/P-002\/changes\?uc=UC-001$/);
});

test("the changes tab shows each use case as a diff with its intents and the proofs they replace", async ({ page }) => {
  await page.goto("#/proposals/P-002/changes");
  const uc1 = page.locator('[data-usecase="UC-001"]');
  await expect(uc1.getByTestId("diff").locator("del")).toContainText(["100"]);
  await expect(uc1.getByTestId("diff").locator("ins").first()).toBeVisible();
  await uc1.getByRole("radio", { name: "Before" }).click();
  await expect(uc1).toContainText("Orders whose total is over 100 ship for free");
  await expect(intent(page, "I-01").getByTestId("replaces")).toContainText("Approving I-01 replaces 1 existing proof");
  await expect(intent(page, "I-01").getByTestId("replaces")).toContainText("Free over 100");
  await expect(page.locator('[data-usecase="UC-003"]')).toContainText("Shipping estimate at checkout");
});

test("an agent's name cannot review, and the submitter cannot approve under this policy", async ({ page }) => {
  await openChanges(page, "claude");
  await intent(page, "I-02").getByRole("button", { name: "Approve" }).click();
  await expect(page.getByTestId("flash")).toContainText("intents are reviewed by people");
  expect(proposalOnDisk().intents[1].approvals).toEqual([]);

  await reviewAs(page, "ana");
  await expect(intent(page, "I-02").getByRole("button", { name: "Approve" })).toBeDisabled();
});

test("two reviewers approve an intent, and each sees the other's approval live", async ({ page, browser }) => {
  await openChanges(page, "bruno");
  const carla = await newReviewer(browser, "carla");

  await intent(page, "I-02").getByRole("button", { name: "Approve" }).click();
  await expect(intent(page, "I-02").getByTestId("approvals")).toContainText("1 of 2 approvals");
  await expect(intent(page, "I-02").getByText("You approved")).toBeVisible();
  // Carla's page updates without a reload (server-sent events on file changes).
  await expect(intent(carla.page, "I-02").getByTestId("approvals")).toContainText("1 of 2 approvals");

  await intent(carla.page, "I-02").getByRole("button", { name: "Approve" }).click();
  await expect(intent(page, "I-02").getByTestId("approvals")).toContainText("2 of 2 approvals");
  expect(proposalOnDisk().intents[1]).toMatchObject({ status: "approved", approvals: ["bruno", "carla"] });
  await carla.ctx.close();
});

test("rejecting an intent makes the use case's after stale until a reviewer rewrites it", async ({ page }) => {
  await openChanges(page, "bruno");
  await intent(page, "I-01").getByRole("button", { name: "Approve" }).click();
  await expect(intent(page, "I-01").getByTestId("approvals")).toContainText("1 of 2");

  await intent(page, "I-02").getByRole("button", { name: "Reject" }).click();
  await intent(page, "I-02").getByLabel("Why should this not be built?").fill("Express pricing is out of scope.");
  await intent(page, "I-02").getByRole("button", { name: "Reject intent" }).click();
  await expect(intent(page, "I-02").getByTestId("intent-status")).toHaveText("Rejected");
  const uc1 = page.locator('[data-usecase="UC-001"]');
  await expect(uc1.getByTestId("stale")).toBeVisible();
  await expect(page.getByTestId("proposal-status")).toHaveText("Changes requested");

  await uc1.getByRole("button", { name: "Edit after" }).click();
  await uc1.getByLabel(/UC-001 after this proposal/).fill("Orders whose total is over 150 ship for free; any other order pays the flat fee of 9.90.");
  await uc1.getByRole("button", { name: "Save after" }).click();
  await expect(uc1.getByTestId("stale")).toHaveCount(0);
  // A new after needs review again: Bruno's approval of I-01 is withdrawn.
  await expect(intent(page, "I-01").getByTestId("intent-status")).toHaveText("To review");
  expect(proposalOnDisk().useCases[0].after).toContain("over 150");
});

test("amending an intent keeps the extracted wording and counts as the amender's approval", async ({ page }) => {
  await openChanges(page, "carla");
  await intent(page, "I-03").getByRole("button", { name: "Amend" }).click();
  await intent(page, "I-03").getByLabel(/New wording/).fill("Before payment, the checkout shows the exact shipping fee the order will pay.");
  await intent(page, "I-03").getByRole("button", { name: "Save amendment" }).click();
  await expect(intent(page, "I-03").getByTestId("intent-status")).toHaveText("Amended");
  await expect(intent(page, "I-03")).toContainText("Extracted as: Before payment, the checkout shows the shipping fee");
  await expect(intent(page, "I-03").getByTestId("approvals")).toContainText("1 of 2 approvals");
  await expect(page.locator('[data-usecase="UC-003"]').getByTestId("stale")).toBeVisible();
});

test("decisions are answered, and threads are resolved only by their author or the submitter", async ({ page }) => {
  await page.goto("#/proposals/P-002");
  await reviewAs(page, "carla");
  const d = page.getByTestId("decision");
  await d.getByRole("radio", { name: /Round up/ }).click();
  await d.getByRole("button", { name: "Answer" }).click();
  await expect(d).toContainText("Answered by carla");
  await expect(d).toContainText("→ Round up");
  expect(proposalOnDisk().decisions[0]).toMatchObject({ chosen: 1, answeredBy: "carla" });

  const t1 = page.locator('[data-thread="T1"]');
  await t1.getByLabel("Reply").fill("Before discounts.");
  await t1.getByRole("button", { name: "Reply" }).click();
  await expect(t1).toContainText("Before discounts.");
  await expect(t1).toContainText("Only bruno or ana can resolve this thread.");

  await page.getByRole("button", { name: "Comment on the prose" }).click();
  await page.getByLabel("Start thread").fill("Should the estimate include taxes?");
  await page.getByRole("button", { name: "Start thread" }).click();
  await expect(page.locator('[data-thread="T2"]')).toContainText("Should the estimate include taxes?");

  await reviewAs(page, "ana");
  await t1.getByRole("button", { name: "Resolve thread" }).click();
  await expect(t1).toContainText("Resolved by ana");
});

test("once every intent, decision and thread is settled, the proposal is approved", async ({ page, browser }) => {
  await page.goto("#/proposals/P-002");
  await reviewAs(page, "bruno");
  await page.getByTestId("decision").getByRole("button", { name: "Answer" }).click();
  await page.locator('[data-thread="T1"]').getByRole("button", { name: "Resolve thread" }).click();
  await page.getByRole("tab", { name: /Use cases changed/ }).click();
  for (const id of ["I-01", "I-02", "I-03"]) await intent(page, id).getByRole("button", { name: "Approve" }).click();
  await expect(page.getByTestId("review-box")).toContainText("3 items block approval");

  const carla = await newReviewer(browser, "carla");
  await carla.page.getByRole("radio", { name: "Waiting for me" }).click();
  for (const id of ["I-01", "I-02", "I-03"]) await intent(carla.page, id).getByRole("button", { name: "Approve" }).click();
  await expect(carla.page.getByText("Nothing here: every intent in this view is settled.")).toBeVisible();
  await expect(carla.page.getByTestId("flash")).toContainText("P-002 is approved");
  await expect(page.getByTestId("proposal-status")).toHaveText("Approved");
  await expect(page.getByTestId("review-box")).toContainText("Approved: the intents are the brief");
  expect(proposalOnDisk()).toMatchObject({ status: "approved", supersedes: ["P-001.PF-01"] });
  expect(proposalOnDisk().hash).toMatch(/^sha256:/);
  // An approved proposal is read-only here.
  await expect(intent(page, "I-01").getByRole("button", { name: "Amend" })).toHaveCount(0);
  await carla.ctx.close();
});

test("recombining without an API key writes a handoff request for an agent to answer", async ({ page }) => {
  await openChanges(page, "bruno");
  const uc1 = page.locator('[data-usecase="UC-001"]');
  await uc1.getByRole("button", { name: "Recombine with the LLM" }).click();
  await uc1.getByLabel("Instructions for the LLM (optional)").fill("Keep it to one sentence.");
  await uc1.getByRole("button", { name: "Recombine", exact: true }).click();
  await expect(page.getByTestId("flash")).toContainText("The LLM has not answered yet");
  const requests = readdirSync(join(WORK_DIR, "llm")).filter((f) => f.endsWith(".request.json"));
  expect(requests).toHaveLength(1);
  const req = JSON.parse(readFileSync(join(WORK_DIR, "llm", requests[0]!), "utf8"));
  expect(req.user).toContain("Keep it to one sentence.");
  expect(existsSync(join(WORK_DIR, "llm", requests[0]!.replace(".request.json", ".answer.json")))).toBe(false);
});

test.describe("proof time-lapse", () => {
  const cell = (page: Page, row: string, run: number) => page.getByRole("gridcell", { name: new RegExp(`^${row}, run ${run}:`) });

  test("shows every prove run as a Gantt of the intents, from failing to passing", async ({ page }) => {
    const errors = watchErrors(page);
    await page.goto("#/proposals/P-003/timeline");
    const t = page.getByTestId("timeline");
    await expect(t).toContainText("5 runs over 48 min");
    await expect(t).toContainText("All proofs passed first at run 4");
    await expect(t).toContainText("2 of 2 intents passing now");
    const i1 = page.locator('[data-group="P-003.I-01"]');
    await expect(i1.getByTestId("passing-since")).toHaveText("passes since run 3");
    await expect(page.locator('[data-group="P-003.I-02"]').getByTestId("passing-since")).toHaveText("passes since run 4");
    await expect(cell(page, "Ten percent off the items", 1)).toHaveAttribute("data-state", "fail");
    await expect(cell(page, "Ten percent off the items", 2)).toHaveAttribute("data-state", "pass");
    // The use case's existing proof broke in run 3: a regression.
    await expect(cell(page, "UC-001", 3)).toHaveAttribute("data-state", "fail");
    await expect(page.getByTestId("run-detail")).toContainText("Run 5");
    expect(errors).toEqual([]);
  });

  test("clicking a run and a proof shows what was tested, the counterexample and what changed", async ({ page }) => {
    await page.goto("#/proposals/P-003/timeline");
    await cell(page, "Expired codes change nothing", 2).click();
    const run = page.getByTestId("run-detail");
    await expect(run).toContainText("Run 2");
    await expect(run).toContainText("1/3 proposal proofs pass");
    await expect(run.getByTestId("flips")).toContainText("P-003.PF-01 now passes");
    const proof = page.getByTestId("proof-detail");
    await expect(proof).toContainText("Expired codes change nothing");
    await expect(proof).toContainText('in.code = "expired"');
    await expect(proof.getByTestId("counterexample")).toContainText('{"code":"expired","items":50}');

    await page.getByRole("columnheader", { name: /^Run 3 at/ }).click();
    await expect(run).toContainText("1 regression(s)");
    await expect(run.getByTestId("flips")).toContainText("P-001.PF-01 now fails (violated)");
    await proof.getByRole("button", { name: "Run 4: evidenced" }).click();
    await expect(run).toContainText("Run 4");
    await expect(run).toContainText("+ uncommitted changes");
  });

  test("plays the time-lapse from the first run to the last", async ({ page }) => {
    await page.goto("#/proposals/P-003/timeline");
    await page.getByRole("button", { name: "First run" }).click();
    await expect(page.getByTestId("run-detail")).toContainText("Run 1");
    await expect(cell(page, "Ten percent off the items", 5)).toHaveClass(/opacity-25/);
    await page.getByRole("button", { name: "Play the time-lapse" }).click();
    await expect(page.getByTestId("run-detail")).toContainText("Run 5", { timeout: 8000 });
    await expect(page.getByRole("button", { name: "Play the time-lapse" })).toBeVisible();
    await expect(cell(page, "Ten percent off the items", 5)).not.toHaveClass(/opacity-25/);
  });

  test("a new run appears live while the code is being written", async ({ page }) => {
    await page.goto("#/proposals/P-003/timeline");
    await expect(page.getByTestId("run-detail")).toContainText("Run 5");
    const file = join(WORK_DIR, "proposals", "P-003-discount-codes.runs.jsonl");
    const last = JSON.parse(readFileSync(file, "utf8").trim().split("\n").pop()!);
    appendFileSync(file, JSON.stringify({ ...last, run: 6, at: "2026-11-11T15:00:00.000Z" }) + "\n");
    await expect(page.getByTestId("run-detail")).toContainText("Run 6");
    await expect(page.getByTestId("timeline")).toContainText("6 runs");
  });

  test("a proposal without runs explains where they come from", async ({ page }) => {
    await page.goto("#/proposals/P-002/timeline");
    await expect(page.getByText("No proof runs yet.")).toBeVisible();
    await expect(page.getByText("P-002-raise-free-shipping-threshold.runs.jsonl")).toBeVisible();
  });
});
