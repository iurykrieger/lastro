// UI tests for the interactive examples. They run the real SDK and the Rust prover (WASM) in the browser,
// so these also check that the two engines load and agree.
import { expect, test, type Locator, type Page } from "@playwright/test";

const playground = (page: Page, id: string) => page.locator(`[data-playground="${id}"]`);
const clause = (pg: Locator, id: string) => pg.getByRole("list", { name: "Clauses" }).locator(`[data-clause="${id}"]`);
const pick = (pg: Locator, group: string, option: string) => pg.getByRole("radiogroup", { name: group, exact: true }).getByRole("radio", { name: option, exact: true }).click();

async function proved(pg: Locator) {
  await expect(pg.getByTestId("outcome")).toHaveText("Proved: every clause holds on every input");
}

test("fix the shipping fee: the buggy code fails C1, the fixed code is proved by both engines", async ({ page }) => {
  await page.goto("#/examples/shipping");
  const pg = playground(page, "shipping");
  await expect(pg.getByTestId("outcome")).toHaveText("Not proved: C1 fails");
  await expect(clause(pg, "C1")).toHaveAttribute("data-verdict", "violated");
  await expect(clause(pg, "C1").getByTestId("counterexample")).toContainText('{"member": false, "subtotal": 100} → {"fee": 15}');
  await expect(clause(pg, "C1")).toContainText("rust: violated");
  await expect(pg.getByTestId("engines")).toContainText("both engines agree on every clause");
  await expect(pg).toContainText('counterexample: {"member":false,"subtotal":100} returned {"fee":15}');

  await pick(pg, "Comparison", "subtotal >=");
  await proved(pg);
  await expect(pg.locator("figure").filter({ hasText: "shipping.mjs" })).toContainText("subtotal >= 100");
  await expect(pg).toContainText("Every proof passes.");
  await expect(pg).toContainText("Proved: 3 · Under assumptions: 0 · Evidenced: 0 · Beyond proof: 1");

  await pg.getByLabel("Members ship free").click();
  await expect(pg.getByTestId("outcome")).toHaveText("Not proved: C2 fails");

  await pg.getByRole("button", { name: "shipping.mjs", exact: true }).click();
  await proved(pg);
  await pick(pg, "Fee for everyone else", "0");
  await expect(pg.getByTestId("outcome")).toHaveText("Not proved: C3 fails");
});

test("the threshold slider moves the counterexample to the boundary", async ({ page }) => {
  await page.goto("#/examples/shipping");
  const pg = playground(page, "shipping");
  await pg.getByRole("button", { name: "shipping.mjs", exact: true }).click();
  await proved(pg);
  const slider = pg.getByRole("slider", { name: "Threshold" });
  await slider.focus();
  await page.keyboard.press("ArrowLeft");
  await expect(pg.getByTestId("outcome")).toHaveText("Not proved: C3 fails");
  await expect(clause(pg, "C3").getByTestId("counterexample")).toContainText('"subtotal": 99');
});

test("run one login: presets drive the request flow and the clause checks", async ({ page }) => {
  await page.goto("#/examples/login");
  const pg = playground(page, "login-runner");
  await expect(pg.getByTestId("login-output")).toHaveText(/status 200 · token true · role customer/);

  await pg.getByRole("button", { name: "Database down" }).click();
  await expect(pg.getByTestId("login-output")).toHaveText(/status 500 · token false · role none/);
  await expect(pg.locator('[data-step="stop"]')).toHaveText(/Await the lookup/);
  await expect(pg.locator('[data-clause="C7"]')).toHaveAttribute("data-status", "holds");
  await expect(pg.locator('[data-status="holds"], [data-status="fails"]')).toHaveCount(1);

  await pg.getByRole("button", { name: "Admin signs in" }).click();
  await pg.getByLabel("Implementation").selectOption("admin-everywhere");
  await expect(pg.getByTestId("login-output")).toHaveText(/role admin/);
  await pick(pg, "sends", "email");
  await expect(pg.locator('[data-clause="C10"]')).toHaveAttribute("data-status", "fails");
});

test("plant a bug: each bug is caught by its clause, with the Rust prover agreeing", async ({ page }) => {
  await page.goto("#/examples/bugs");
  const pg = playground(page, "bug-hunt");
  // Refuse everyone: only the both-ways clause catches it; the one-way clauses go vacuous.
  await expect(pg.getByTestId("outcome")).toHaveText("Not proved: C3 fails");
  await expect(clause(pg, "C2")).toHaveAttribute("data-verdict", "vacuous");
  await expect(pg.getByTestId("engines")).toContainText("both engines agree on every clause");

  await pg.getByLabel("Implementation").selectOption("db-as-401");
  await expect(pg.getByTestId("outcome")).toHaveText("Not proved: C7 fails");
  await expect(clause(pg, "C7").getByTestId("counterexample")).toContainText('"infra": "db-fails"');
  await expect(pg).toContainText("- C7 (I7): A database failure gets 500 and no token.");

  await pg.getByLabel("Implementation").selectOption("none");
  await proved(pg);
  await expect(pg).toContainText("Every proof passes.");
});

test("review it yourself: the AI's reading of I5 fails the correct code; the decisions change what passes", async ({ page }) => {
  await page.goto("#/examples/review");
  const pg = playground(page, "review");
  await proved(pg);
  const hash = await pg.getByTestId("artifact-hash").textContent();

  await pick(pg, "I5", "approve the AI's reading");
  await expect(pg.getByTestId("i5-statement")).toContainText("the request is invalid (400)");
  await expect(pg.getByTestId("outcome")).toHaveText("Not proved: C11 fails");
  await expect(pg.getByTestId("artifact-hash")).not.toHaveText(hash!);
  await pick(pg, "I5", "amend: the email wins");
  await proved(pg);

  await pg.getByLabel("Code under proof").selectOption("bad-hash-as-wrong");
  await expect(pg.getByTestId("outcome")).toHaveText("Not proved: C6 fails");
  await pick(pg, "D2: an unreadable hash answers", "401");
  await proved(pg);

  await pick(pg, "D2: an unreadable hash answers", "500, as today");
  await pg.getByLabel("Code under proof").selectOption("refuse-everyone");
  await expect(pg.getByTestId("outcome")).toHaveText("Not proved: C3 fails");
  await pick(pg, "D1: must the right password always get a token?", "only refuse the wrong one");
  await expect(pg.getByTestId("outcome")).toHaveText(/^No clause fails, but .* prove nothing here$/);
  await expect(pg.getByTestId("clause-count")).toHaveText("10");

  await pg.getByLabel("Code under proof").selectOption("db-as-401");
  await expect(pg.getByTestId("outcome")).toHaveText(/C\d+ fails/);
  await pg.getByLabel("Reject I7 (a database failure gets 500)").click();
  await proved(pg);
});

test("the quiz explains the right answer", async ({ page }) => {
  await page.goto("#/examples/review");
  const quiz = page.locator('[data-playground="quiz"]');
  await quiz.getByRole("button", { name: /request is invalid/ }).click();
  await expect(quiz.getByTestId("quiz-result")).toContainText("Not quite.");
  await quiz.getByRole("button", { name: /the email wins/ }).click();
  await expect(quiz.getByTestId("quiz-result")).toContainText("Right.");
});

test("trusted pieces: a production trace breaks a clause proved on the model", async ({ page }) => {
  await page.goto("#/examples/trust");
  const pg = playground(page, "trust");
  await expect(pg.locator("[data-assumption]")).toHaveCount(3);
  await expect(pg.locator('[data-intent="I1"]')).toHaveAttribute("data-verdict", "proved-under-assumptions");
  await expect(pg.locator('[data-intent="I3"]')).toHaveAttribute("data-verdict", "proved");
  await expect(pg.locator('[data-intent="I8"]')).toHaveAttribute("data-verdict", "beyond-proof");

  await pg.getByLabel(/Replay the production trace/).click();
  await expect(pg.locator('[data-intent="I1"]')).toHaveAttribute("data-verdict", "violated");
  await expect(pg.getByTestId("trace-note")).toContainText("C2 and C5");
  await expect(pg.getByTestId("outcome")).toHaveText("Not proved: C2, C5 fail");
});

test("proved vs evidenced: past 100,000 inputs the engine samples, and the off-by-one bug slips through", async ({ page }) => {
  await page.goto("#/concepts/verdicts");
  const pg = playground(page, "sampling");
  await proved(pg);
  await expect(pg.getByTestId("space")).toContainText("402 inputs (enumerated)");

  await pg.getByLabel("Plant the off-by-one bug").click();
  await expect(pg.getByTestId("outcome")).toHaveText("Not proved: C1 fails");

  await pick(pg, "subtotal is an int from 0 to", "1,000,000");
  await expect(pg.getByTestId("space")).toContainText("2,000,002 inputs (sampled");
  await expect(pg.getByTestId("outcome")).toHaveText("Evidenced: every sampled input passes, but the space was not covered");
  await expect(clause(pg, "C1")).toHaveAttribute("data-verdict", "evidenced");
  await expect(clause(pg, "C1")).toContainText("rule of three");
  await expect(pg.getByTestId("missed-bug")).toBeVisible();
  await expect(pg.getByTestId("engines")).toContainText("the Rust prover only runs on spaces of up to 100,000 inputs");

  await pick(pg, "subtotal is an int from 0 to", "49,999");
  await expect(pg.getByTestId("space")).toContainText("100,000 inputs (enumerated)");
  await expect(pg.getByTestId("outcome")).toHaveText("Not proved: C1 fails", { timeout: 20_000 });
});

test("anatomy of a clause explains the field you click", async ({ page }) => {
  await page.goto("#/concepts/spec");
  const pg = playground(page, "anatomy");
  await expect(pg.getByTestId("anatomy-explain")).toContainText("requires: The if-part");
  await pg.getByRole("button", { name: /"ensures"/ }).click();
  await expect(pg.getByRole("button", { name: /"ensures"/ })).toHaveAttribute("aria-pressed", "true");
  await expect(pg.getByTestId("anatomy-explain")).toContainText("counterexample");
  await expect(pg).toContainText("in.subtotal ≥ 100  ⇒  out.fee = 0");
});
