// Proofs in .lastro/proof/, and proofs derived straight from the established use cases (P-004).
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { ProposalError, proofsFromUseCases, proveUseCases, regress, useCaseTargets, type Model, type ProofEntry, type UseCase, type UseCaseDeriveOut } from "../src/index.js";
import type { Expr } from "../src/spec/expr.js";
import { openWorkspace } from "../src/workspace.js";
import { main as cli } from "../src/cli.js";

const MODEL: Model = {
  name: "shipping",
  inputs: { subtotal: { type: "int", min: 0, max: 200 }, member: { type: "bool" } },
  outputs: { fee: { type: "int", min: 0, max: 50 } },
};
const FREE: Expr = { op: "iff", args: [{ op: "eq", args: [{ var: "out.fee" }, { const: 0 }] }, { op: "or", args: [{ var: "in.member" }, { op: "ge", args: [{ var: "in.subtotal" }, { const: 100 }] }] }] };
const uc = (id: string, slug: string, proofs: ProofEntry[] = []): UseCase => ({ id, slug, name: slug, description: `What ${slug} does.`, proofs, history: [] });
const passed = (id: string, useCase: string): ProofEntry => ({ id, slug: "it-holds", name: "It holds", description: "It holds.", intent: "P-001.I-01", useCase, model: "shipping", subject: "shipping", check: { kind: "clause", ensures: FREE, assumptions: [] }, status: "passed" });
const answer = (useCase: string, slug = "free-shipping"): UseCaseDeriveOut["proofs"][number] => ({
  useCase, slug, name: "Free shipping", description: "Members and orders of 100 or more ship free.", model: "shipping", subject: "shipping",
  kind: "clause", requires: "", ensures: JSON.stringify(FREE), assumptions: [], inputJson: "", expect: "",
});
const setup = { models: { shipping: MODEL }, subjects: ["shipping"] };

test("use-case targets: every unproved use case by default; a named one with passed proofs is refused", () => {
  const cases = [uc("UC-001", "fee", [passed("P-001.PF-01", "UC-001")]), uc("UC-002", "coupons"), uc("UC-003", "refunds")];
  assert.deepEqual(useCaseTargets(cases).map((u) => u.id), ["UC-002", "UC-003"]);
  assert.deepEqual(useCaseTargets(cases, ["UC-003"]).map((u) => u.id), ["UC-003"]);
  assert.throws(() => useCaseTargets(cases, ["UC-001", "UC-009"]), (e: unknown) => e instanceof ProposalError && e.problems.length === 2 && /already has proofs/.test(e.problems[0]!));
});

test("derived proofs are numbered inside their use case; every target gets a proof or a reason, never both", () => {
  const pending: ProofEntry = { ...passed("UC-002.PF-01", "UC-002"), status: "failed" };
  delete pending.intent;
  const cases = [uc("UC-002", "coupons", [pending]), uc("UC-003", "refunds")];
  const d = proofsFromUseCases(cases, ["UC-002", "UC-003"], setup, { proofs: [answer("UC-002"), answer("UC-002", "member-ships-free")], unprovable: [{ useCase: "UC-003", reason: "no model expresses refunds" }] });
  assert.deepEqual(d.derived.map((x) => [x.id, x.status, "intent" in x]), [["UC-002.PF-02", "pending", false], ["UC-002.PF-03", "pending", false]]);
  assert.deepEqual(d.useCases.find((u) => u.id === "UC-002")!.proofs.map((x) => x.id), ["UC-002.PF-02", "UC-002.PF-03"]);
  assert.deepEqual(d.unprovable, [{ useCase: "UC-003", reason: "no model expresses refunds" }]);
  assert.throws(
    () => proofsFromUseCases(cases, ["UC-002", "UC-003"], setup, { proofs: [answer("UC-009"), { ...answer("UC-002"), model: "nope" }], unprovable: [{ useCase: "UC-002", reason: "" }] }),
    (e: unknown) => e instanceof ProposalError && e.problems.length >= 4,
  );
});

test("proving use-case proofs: passing ones become regression tests; failing ones go to the user with the note", async () => {
  const pending: ProofEntry = { ...passed("UC-001.PF-01", "UC-001"), status: "pending" };
  delete pending.intent;
  const cases = [uc("UC-001", "fee", [pending])];
  const buggy = ({ subtotal, member }: { subtotal: number; member: boolean }) => ({ fee: member || subtotal > 100 ? 0 : 15 });
  const right = ({ subtotal, member }: { subtotal: number; member: boolean }) => ({ fee: member || subtotal >= 100 ? 0 : 15 });
  const bad = await proveUseCases(cases, { subjects: { shipping: buggy as never }, models: { shipping: MODEL } });
  assert.equal(bad.run.ok, false);
  assert.deepEqual(bad.run.failed, ["UC-001.PF-01"]);
  assert.match(bad.run.note!, /code is the reference/);
  assert.equal(bad.useCases[0]!.proofs[0]!.status, "failed");
  assert.deepEqual(bad.run.unproved, ["UC-001"]);
  // A failed proof is not a regression test.
  assert.deepEqual((await regress(bad.useCases, { subjects: { shipping: buggy as never }, models: { shipping: MODEL } })).outcomes, []);
  const good = await proveUseCases(bad.useCases, { subjects: { shipping: right as never }, models: { shipping: MODEL } });
  assert.equal(good.run.ok, true);
  assert.equal(good.run.note, undefined);
  assert.deepEqual(good.useCases[0]!.proofs.map((x) => [x.status, x.verdict]), [["passed", "proved"]]);
  const r = await regress(good.useCases, { subjects: { shipping: buggy as never }, models: { shipping: MODEL } });
  assert.deepEqual(r.regressions, ["UC-001.PF-01"]);
});

test("workspace: one file per proof; an old use case file keeps its proofs until it is written", () => {
  const dir = join(mkdtempSync(join(tmpdir(), "lastro-proofdir-")), ".lastro");
  mkdirSync(join(dir, "usecases"), { recursive: true });
  writeFileSync(join(dir, "usecases", "UC-001-fee.json"), JSON.stringify(uc("UC-001", "fee", [passed("P-001.PF-01", "UC-001")])));
  const ws = openWorkspace(dir);
  assert.deepEqual(ws.useCases()[0]!.proofs.map((x) => x.id), ["P-001.PF-01"]);
  assert.equal(existsSync(join(dir, "proof")), false);
  ws.writeUseCase(ws.useCases()[0]!);
  assert.deepEqual(Object.keys(JSON.parse(readFileSync(join(dir, "usecases", "UC-001-fee.json"), "utf8"))), ["id", "slug", "name", "description", "history"]);
  assert.deepEqual(readdirSync(join(dir, "proof")), ["P-001.PF-01-it-holds.json"]);
  assert.deepEqual(ws.useCases()[0]!.proofs.map((x) => x.id), ["P-001.PF-01"]);
  // Dropping a proof deletes its file; removing the use case deletes the rest.
  ws.writeUseCase({ ...ws.useCases()[0]!, proofs: [passed("P-002.PF-01", "UC-001")] });
  assert.deepEqual(readdirSync(join(dir, "proof")), ["P-002.PF-01-it-holds.json"]);
  ws.removeUseCase({ id: "UC-001", slug: "fee" });
  assert.deepEqual(readdirSync(join(dir, "proof")), []);
});

test("cli: derive --usecases → prove --usecases → regress, the LLM step by handoff", async () => {
  const root = mkdtempSync(join(tmpdir(), "lastro-ucproofs-"));
  const dir = join(root, ".lastro");
  const ws = ["--dir", dir, "--llm", "handoff"];
  writeFileSync(join(root, "model.json"), JSON.stringify(MODEL));
  writeFileSync(join(root, "shipping.mjs"), "export default ({ subtotal, member }) => ({ fee: member || subtotal >= 100 ? 0 : 15 });\n");
  writeFileSync(join(root, "buggy.mjs"), "export default ({ subtotal, member }) => ({ fee: member || subtotal > 100 ? 0 : 15 });\n");
  assert.equal((await cli(["init", "--model", join(root, "model.json"), "--subject", "./buggy.mjs", ...ws])).code, 0);
  mkdirSync(join(dir, "usecases"), { recursive: true });
  for (const u of [uc("UC-001", "charge-shipping"), uc("UC-002", "print-labels")]) writeFileSync(join(dir, "usecases", `${u.id}-${u.slug}.json`), JSON.stringify({ ...u, proofs: undefined }));

  let r = await cli(["derive", "--usecases", ...ws]);
  assert.equal(r.code, 3, JSON.stringify(r.body));
  const request = JSON.parse(readFileSync(r.body.request as string, "utf8")) as { user: string };
  assert.deepEqual((JSON.parse(request.user) as { useCasesToProve: { id: string }[] }).useCasesToProve.map((x) => x.id), ["UC-001", "UC-002"]);
  writeFileSync(r.body.answer as string, JSON.stringify({ proofs: [answer("UC-001")], unprovable: [{ useCase: "UC-002", reason: "printing is external code" }] }));
  r = await cli(["derive", "--usecases", ...ws]);
  assert.equal(r.code, 0, JSON.stringify(r.body));
  assert.deepEqual(r.body.unprovable, [{ useCase: "UC-002", reason: "printing is external code" }]);
  assert.deepEqual(readdirSync(join(dir, "proof")), ["UC-001.PF-01-free-shipping.json"]);
  assert.deepEqual((await cli(["status", ...ws])).body.pendingProofs, ["UC-001.PF-01"]);

  // The code under proof is buggy: the proof fails and goes to the user, with the note.
  r = await cli(["prove", "--usecases", ...ws]);
  assert.equal(r.code, 1);
  assert.deepEqual(r.body.failed, ["UC-001.PF-01"]);
  assert.match(String(r.body.note), /Do not change the code/);
  // The right code, named on the command line: the proof passes and becomes a regression test.
  r = await cli(["prove", "--usecases", join(root, "shipping.mjs"), ...ws]);
  assert.equal(r.code, 0, JSON.stringify(r.body));
  assert.deepEqual(r.body.passed, ["UC-001.PF-01"]);
  assert.deepEqual(r.body.unproved, ["UC-002"]);
  r = await cli(["status", ...ws]);
  assert.equal(r.body.proofs, 1);
  assert.equal(r.body.pendingProofs, undefined);
  r = await cli(["regress", ...ws]);
  assert.equal(r.code, 1);
  assert.deepEqual(r.body.regressions, ["UC-001.PF-01"]);
  // Proven now: its proofs change only through a proposal.
  r = await cli(["derive", "--usecases", "UC-001", ...ws]);
  assert.equal(r.code, 1);
  assert.match(JSON.stringify(r.body.problems), /UC-001 already has proofs/);
});
