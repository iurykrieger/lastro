import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import {
  ProposalError,
  applyProposal,
  approveProposal,
  baselineOf,
  carryProposalReview,
  deriveProofs,
  extractProposal,
  isProposalApproved,
  loadRustProver,
  pendingProposal,
  proposalFeedback,
  proofsFromDerivation,
  proposalFromExtraction,
  reservedUseCaseIds,
  proveProposal,
  regress,
  renderProposalReceipt,
  reviewProposal,
  type AskJson,
  type DeriveV1Out,
  type Model,
  type Proposal,
  type ProposeOut,
  type Subject,
  type UseCase,
} from "../src/index.js";

const MODEL: Model = {
  name: "shipping_fee",
  inputs: { subtotal: { type: "int", min: 0, max: 200 }, member: { type: "bool" } },
  outputs: { fee: { type: "enum", values: [0, 15] } },
};
const models = { shipping_fee: MODEL };
const setup = { models, subjects: ["shipping"] };
const policy = { approvals: 1, submitterCanApprove: true };

const fee = (threshold: number): Subject => ({ subtotal, member }) => ({ fee: member || (subtotal as number) >= threshold ? 0 : 15 });
const offByOne: Subject = ({ subtotal, member }) => ({ fee: member || (subtotal as number) > 100 ? 0 : 15 });

const j = (e: unknown) => JSON.stringify(e);
const GE = (n: number) => j({ op: "ge", args: [{ var: "in.subtotal" }, { const: n }] });
const LT_NONMEMBER = (n: number) => j({ op: "and", args: [{ op: "not", args: [{ var: "in.member" }] }, { op: "lt", args: [{ var: "in.subtotal" }, { const: n }] }] });
const FREE = j({ op: "eq", args: [{ var: "out.fee" }, { const: 0 }] });
const PAYS = j({ op: "eq", args: [{ var: "out.fee" }, { const: 15 }] });

const PROSE_1 = "Orders of 100 or more ship free. Members always ship free. Every other order pays a flat shipping fee of 15.";

const EXTRACT_1: ProposeOut = {
  name: "Shipping fee",
  slug: "shipping-fee",
  description: "Charge shipping at checkout.",
  intents: [
    { slug: "free-over-100", name: "Free over 100", description: "Orders of 100 or more ship free.", change: "add", useCase: "charge-shipping", provability: "formal", beyondProofReason: "none", quote: "Orders of 100 or more ship free.", replaces: [] },
    { slug: "members-free", name: "Members ship free", description: "Members always ship free.", change: "add", useCase: "charge-shipping", provability: "formal", beyondProofReason: "none", quote: "Members always ship free.", replaces: [] },
    { slug: "flat-fee", name: "Flat fee", description: "Every other order pays 15.", change: "add", useCase: "charge-shipping", provability: "formal", beyondProofReason: "none", quote: "Every other order pays a flat shipping fee of 15.", replaces: [] },
  ],
  useCases: [{ slug: "charge-shipping", change: "add", name: "Charge shipping", after: "Orders of 100 or more and members ship free; every other order pays 15." }],
  decisions: [],
  notPromised: ["How fast the fee renders."],
};

const DERIVE_1: DeriveV1Out = {
  proofs: [
    { intent: "P-001.I-01", slug: "free-at-100", name: "Free at 100", description: "A subtotal of 100 or more ships free.", model: "shipping_fee", subject: "shipping", kind: "clause", requires: GE(100), ensures: FREE, assumptions: [], inputJson: "", expect: "", replaces: [] },
    { intent: "P-001.I-02", slug: "members-free", name: "Members free", description: "Members always ship free.", model: "shipping_fee", subject: "shipping", kind: "clause", requires: j({ var: "in.member" }), ensures: FREE, assumptions: [], inputJson: "", expect: "", replaces: [] },
    { intent: "P-001.I-03", slug: "pays-15", name: "Pays 15", description: "A non-member under 100 pays 15.", model: "shipping_fee", subject: "shipping", kind: "clause", requires: LT_NONMEMBER(100), ensures: PAYS, assumptions: [], inputJson: "", expect: "", replaces: [] },
    { intent: "P-001.I-03", slug: "guest-40", name: "A guest with 40", description: "A guest with a 40 cart pays 15.", model: "shipping_fee", subject: "shipping", kind: "scenario", requires: "", ensures: "", assumptions: [], inputJson: j({ subtotal: 40, member: false }), expect: PAYS, replaces: [] },
  ],
  decisions: [],
};

const answers = (extract: unknown, derive: unknown): AskJson => async (req) => (req.step === "extract" ? structuredClone(extract) : structuredClone(derive));

async function appliedP1(): Promise<UseCase[]> {
  const ask = answers(EXTRACT_1, DERIVE_1);
  let p = await extractProposal(ask, { id: "P-001", prose: PROSE_1, submittedBy: "ana", useCases: [], policy });
  p = approveProposal(reviewProposal(p, "ana").approveAll().done(), []);
  p = await deriveProofs(ask, p, [], setup);
  p = (await proveProposal(p, [], { subjects: { shipping: fee(100) }, models })).proposal;
  return applyProposal(p, []).useCases;
}

test("propose: intents change use cases, the SDK assigns ids and copies before", async () => {
  const p = await extractProposal(answers(EXTRACT_1, DERIVE_1), { id: "P-001", prose: PROSE_1, submittedBy: "ana", useCases: [], policy });
  assert.equal(p.lastro, "proposal/v1");
  assert.equal(p.status, "open");
  assert.deepEqual(p.intents.map((i) => i.id), ["P-001.I-01", "P-001.I-02", "P-001.I-03"]);
  assert.deepEqual([...new Set(p.intents.map((i) => i.useCase))], ["UC-001"]);
  assert.deepEqual(p.useCases, [{ useCase: "UC-001", slug: "charge-shipping", change: "add", name: "Charge shipping", after: EXTRACT_1.useCases[0]!.after, intents: ["P-001.I-01", "P-001.I-02", "P-001.I-03"] }]);
  assert.equal(p.baseline, baselineOf([]));
  assert.deepEqual(p.intents[0]!.source, { quote: "Orders of 100 or more ship free.", start: 0, end: 32 });
});

test("propose: adds number after the use-case ids other open proposals reserved", async () => {
  const first = await extractProposal(answers(EXTRACT_1, DERIVE_1), { id: "P-001", prose: PROSE_1, submittedBy: "ana", useCases: [], policy });
  assert.deepEqual(reservedUseCaseIds([first]), ["UC-001"]);
  assert.deepEqual(reservedUseCaseIds([first], "P-001"), [], "a proposal does not reserve against itself");
  assert.deepEqual(reservedUseCaseIds([{ ...first, status: "applied" }]), []);
  const second = await extractProposal(answers(EXTRACT_1, DERIVE_1), { id: "P-002", prose: PROSE_1, submittedBy: "ana", useCases: [], policy, reserved: reservedUseCaseIds([first]) });
  assert.deepEqual(second.useCases.map((c) => c.useCase), ["UC-002"]);
});

test("propose: bad references, kinds and replaced proofs reject the extraction", async () => {
  const useCases = await appliedP1();
  const base = { id: "P-002", prose: "Orders of 150 or more ship free.", submittedBy: "ana", useCases, policy };
  const intent = { slug: "x", name: "x", description: "x", provability: "formal" as const, beyondProofReason: "none", quote: "Orders of 150 or more ship free.", replaces: [] as { proof: string; reason: string }[] };
  const out = (o: Partial<ProposeOut>): ProposeOut => ({ name: "n", slug: "n", description: "", intents: [], useCases: [], decisions: [], notPromised: [], ...o });
  const problems = (o: ProposeOut) => {
    try {
      proposalFromExtraction(base, o);
      return [];
    } catch (e) {
      assert.ok(e instanceof ProposalError);
      return e.problems;
    }
  };
  assert.match(problems(out({ intents: [{ ...intent, change: "modify", useCase: "nope" }], useCases: [{ slug: "nope", change: "modify", name: "n", after: "a" }] })).join(), /modify names unknown use case "nope"/);
  assert.match(problems(out({ intents: [{ ...intent, change: "add", useCase: "charge-shipping" }], useCases: [{ slug: "charge-shipping", change: "add", name: "n", after: "a" }] })).join(), /add names existing use case UC-001/);
  assert.match(problems(out({ intents: [{ ...intent, change: "modify", useCase: "charge-shipping" }], useCases: [] })).join(), /expected exactly one use-case change, got 0/);
  assert.match(problems(out({ intents: [{ ...intent, change: "modify", useCase: "charge-shipping" }], useCases: [{ slug: "charge-shipping", change: "modify", name: "n", after: "" }] })).join(), /needs the complete after/);
  assert.match(problems(out({ intents: [{ ...intent, change: "modify", useCase: "charge-shipping", replaces: [{ proof: "P-009.PF-01", reason: "r" }] }], useCases: [{ slug: "charge-shipping", change: "modify", name: "n", after: "a" }] })).join(), /replaces unknown proof P-009.PF-01/);
  assert.match(problems(out({ intents: [{ ...intent, change: "modify", useCase: "charge-shipping", quote: "not there" }], useCases: [{ slug: "charge-shipping", change: "modify", name: "n", after: "a" }] })).join(), /quote not found/);
  // A remove replaces every proof of its use case, without asking the LLM.
  const removed = proposalFromExtraction(base, out({ intents: [{ ...intent, change: "remove", useCase: "charge-shipping" }], useCases: [{ slug: "charge-shipping", change: "remove", name: "n", after: "" }] }));
  assert.deepEqual(removed.intents[0]!.replaces.map((r) => r.proof), ["P-001.PF-01", "P-001.PF-02", "P-001.PF-03", "P-001.PF-04"]);
  assert.equal(removed.useCases[0]!.before, useCases[0]!.description);
  // Mixed kinds on one use case come back as a decision.
  const mixed = proposalFromExtraction(base, out({ intents: [{ ...intent, change: "modify", useCase: "charge-shipping" }, { ...intent, slug: "y", change: "remove", useCase: "charge-shipping" }], useCases: [{ slug: "charge-shipping", change: "modify", name: "n", after: "a" }] }));
  assert.equal(mixed.decisions[0]!.slug, "change-kind-charge-shipping");
});

test("review: per intent, threads and decisions block approval; the policy can exclude the submitter", async () => {
  let p = await extractProposal(answers(EXTRACT_1, DERIVE_1), { id: "P-001", prose: PROSE_1, submittedBy: "ana", useCases: [], policy: { approvals: 1, submitterCanApprove: false } });
  assert.throws(() => reviewProposal(p, "ana").approveAll(), /submitter can amend but not approve/);
  p = reviewProposal(p, "bia").approveAll().comment("P-001.I-02", "Gold members only?").done();
  assert.deepEqual(pendingProposal(p), ["T1 open thread on P-001.I-02"]);
  assert.throws(() => reviewProposal(p, "caio").resolve("T1"), /resolved by bia or ana/);
  p = reviewProposal(p, "ana").amend("P-001.I-02", "Every member ships free.").resolve("T1").done();
  assert.equal(p.status, "changes-requested");
  assert.deepEqual(pendingProposal(p), ["P-001.I-02 needs 1 more approval(s) from someone other than the submitter", "UC-001 after is stale: rewrite it or recombine it"]);
  p = reviewProposal(p, "ana").rewriteAfter("UC-001", "Orders of 100 or more and every member ship free; others pay 15.").done();
  assert.equal(p.intents[0]!.status, "proposed", "rewriting after puts the use case's intents back in review");
  p = reviewProposal(p, "bia").approveAll().done();
  assert.deepEqual(pendingProposal(p), []);
  p = approveProposal(p, []);
  assert.equal(p.status, "approved");
  assert.ok(isProposalApproved(p));
  // Any change after approval withdraws it.
  const again = reviewProposal(p, "bia").reject("P-001.I-03", "not now").done();
  assert.equal(isProposalApproved(again), false);
  assert.equal(again.status, "changes-requested");
});

test("approval binds the baseline", async () => {
  const p = reviewProposal(await extractProposal(answers(EXTRACT_1, DERIVE_1), { id: "P-001", prose: PROSE_1, submittedBy: "ana", useCases: [], policy }), "ana").approveAll().done();
  const other: UseCase[] = [{ id: "UC-001", slug: "x", name: "x", description: "x", proofs: [], history: [] }];
  assert.throws(() => approveProposal(p, other), /use cases changed since P-001 was extracted/);
});

test("derive, prove with counterexamples, apply: proofs join the use case", async () => {
  const ask = answers(EXTRACT_1, DERIVE_1);
  let p = await extractProposal(ask, { id: "P-001", prose: PROSE_1, submittedBy: "ana", useCases: [], policy });
  await assert.rejects(deriveProofs(ask, p, [], setup), /not approved/);
  p = approveProposal(reviewProposal(p, "ana").approveAll().done(), []);
  p = await deriveProofs(ask, p, [], setup);
  assert.equal(p.status, "proving");
  assert.deepEqual(p.proofs.map((x) => [x.id, x.useCase, x.check.kind, x.status]), [
    ["P-001.PF-01", "UC-001", "clause", "pending"],
    ["P-001.PF-02", "UC-001", "clause", "pending"],
    ["P-001.PF-03", "UC-001", "clause", "pending"],
    ["P-001.PF-04", "UC-001", "scenario", "pending"],
  ]);
  assert.throws(() => applyProposal(p, []), /has not been proved/);

  const wasm = new URL("../../wasm/lastro_core.wasm", import.meta.url);
  const rust = existsSync(wasm) ? await loadRustProver(readFileSync(wasm)) : undefined;
  const bad = await proveProposal(p, [], { subjects: { shipping: offByOne }, models, ...(rust ? { rust } : {}) });
  assert.equal(bad.run.ok, false);
  assert.deepEqual(bad.run.outcomes.filter((x) => !x.passed).map((x) => [x.proof, x.verdict, x.counterexample?.input]), [["P-001.PF-01", "violated", { member: false, subtotal: 100 }]]);
  if (rust) assert.equal(bad.run.enginesAgree, true);
  assert.match(proposalFeedback(bad.proposal, []), /P-001.PF-01 \(P-001.I-01\).*\n.*\n  counterexample: \{"member":false,"subtotal":100\} returned \{"fee":15\}/);
  assert.throws(() => applyProposal(bad.proposal, []), /last run failed: P-001.PF-01/);

  const good = await proveProposal(bad.proposal, [], { subjects: { shipping: fee(100) }, models });
  assert.equal(good.run.ok, true);
  assert.equal(good.proposal.status, "proved");
  assert.deepEqual(good.proposal.proofs.map((x) => x.verdict), ["proved", "proved", "proved", "evidenced"]);
  const applied = applyProposal(good.proposal, []);
  assert.equal(applied.proposal.status, "applied");
  assert.deepEqual(applied.written, ["UC-001"]);
  const [uc] = applied.useCases;
  assert.equal(uc!.description, EXTRACT_1.useCases[0]!.after);
  assert.deepEqual(uc!.history, ["P-001"]);
  assert.deepEqual(uc!.proofs.map((x) => x.id), ["P-001.PF-01", "P-001.PF-02", "P-001.PF-03", "P-001.PF-04"]);
  assert.equal(applied.baseline, baselineOf(applied.useCases));
  assert.match(renderProposalReceipt(good.proposal, []), /### ➕ UC-001 Charge shipping \(add\)/);
});

test("derive refuses proofs that are malformed, uncovered or replace what the intent does not", async () => {
  const ask = answers(EXTRACT_1, DERIVE_1);
  const p = approveProposal(reviewProposal(await extractProposal(ask, { id: "P-001", prose: PROSE_1, submittedBy: "ana", useCases: [], policy }), "ana").approveAll().done(), []);
  const bad = structuredClone(DERIVE_1);
  bad.proofs[0]!.model = "other";
  bad.proofs[1]!.ensures = j({ var: "out.nope" });
  bad.proofs[2]!.replaces = ["P-000.PF-01"];
  bad.proofs.pop();
  try {
    proofsFromDerivation(p, [], setup, bad);
    assert.fail("should throw");
  } catch (e) {
    assert.ok(e instanceof ProposalError);
    const all = e.problems.join("\n");
    assert.match(all, /PF-01: unknown model "other"/);
    assert.match(all, /out\.nope/);
    assert.match(all, /PF-03: replaces P-000.PF-01, which its intent does not list/);
    assert.match(all, /P-001.I-01: no proof derived/);
  }
});

test("a change that replaces proven behavior: supersedes, regression run, apply", async () => {
  const useCases = await appliedP1();
  const prose = "Orders of 150 or more ship free; below that, non-members pay 15.";
  const extract: ProposeOut = {
    name: "Raise the free-shipping threshold",
    slug: "raise-threshold",
    description: "Free shipping starts at 150.",
    intents: [
      { slug: "free-over-150", name: "Free over 150", description: "Orders of 150 or more ship free; non-members below 150 pay 15.", change: "modify", useCase: "charge-shipping", provability: "formal", beyondProofReason: "none", quote: "Orders of 150 or more ship free", replaces: [{ proof: "P-001.PF-01", reason: "orders of 100 to 149 now pay" }, { proof: "P-001.PF-03", reason: "the fee band now ends at 150" }] },
    ],
    useCases: [{ slug: "charge-shipping", change: "modify", name: "Charge shipping", after: "Orders of 150 or more and members ship free; every other order pays 15." }],
    decisions: [],
    notPromised: [],
  };
  const derive: DeriveV1Out = {
    proofs: [
      { intent: "P-002.I-01", slug: "free-at-150", name: "Free at 150", description: "A subtotal of 150 or more ships free.", model: "shipping_fee", subject: "shipping", kind: "clause", requires: GE(150), ensures: FREE, assumptions: [], inputJson: "", expect: "", replaces: ["P-001.PF-01"] },
      { intent: "P-002.I-01", slug: "pays-under-150", name: "Pays under 150", description: "A non-member under 150 pays 15.", model: "shipping_fee", subject: "shipping", kind: "clause", requires: LT_NONMEMBER(150), ensures: PAYS, assumptions: [], inputJson: "", expect: "", replaces: ["P-001.PF-03"] },
    ],
    decisions: [],
  };
  const ask = answers(extract, derive);
  let p = await extractProposal(ask, { id: "P-002", prose, submittedBy: "ana", useCases, policy });
  assert.equal(p.useCases[0]!.before, useCases[0]!.description, "before comes from the use case");
  p = approveProposal(reviewProposal(p, "ana").approveAll().done(), useCases);
  assert.deepEqual(p.supersedes, ["P-001.PF-01", "P-001.PF-03"]);
  p = await deriveProofs(ask, p, useCases, setup);

  // Old code: the new proofs fail, the superseded ones still pass and are noted.
  const old = await proveProposal(p, useCases, { subjects: { shipping: fee(100) }, models });
  assert.equal(old.run.ok, false);
  assert.deepEqual(old.run.replacedStillPass, ["P-001.PF-01", "P-001.PF-03"]);
  assert.deepEqual(old.run.regressions, []);
  assert.deepEqual(old.run.outcomes.filter((x) => x.role === "regression").map((x) => x.proof), ["P-001.PF-02", "P-001.PF-04"]);

  // Code that charges members breaks a proof nobody replaced: a regression.
  const charged: Subject = ({ subtotal }) => ({ fee: (subtotal as number) >= 150 ? 0 : 15 });
  const reg = await proveProposal(p, useCases, { subjects: { shipping: charged }, models });
  assert.deepEqual(reg.run.regressions, ["P-001.PF-02"]);
  assert.match(proposalFeedback(reg.proposal, useCases), /P-001.PF-02 \(regression in UC-001\)/);
  assert.match(renderProposalReceipt(reg.proposal, useCases), /❌ \*\*P-001.PF-02\*\* \(UC-001\)/);

  const good = await proveProposal(p, useCases, { subjects: { shipping: fee(150) }, models });
  assert.equal(good.run.ok, true);
  const receipt = renderProposalReceipt(good.proposal, useCases);
  assert.match(receipt, /replaces P-001.PF-01 \(A subtotal of 100 or more ships free.\): orders of 100 to 149 now pay; now guarded by P-002.PF-01/);
  const applied = applyProposal(good.proposal, useCases);
  assert.deepEqual(applied.useCases[0]!.proofs.map((x) => x.id), ["P-001.PF-02", "P-001.PF-04", "P-002.PF-01", "P-002.PF-02"]);
  assert.deepEqual(applied.useCases[0]!.history, ["P-001", "P-002"]);
  const after = await regress(applied.useCases, { subjects: { shipping: fee(150) }, models });
  assert.equal(after.ok, true);
});

test("derivation never retires a proof: an unlisted break sends the intent back to review", async () => {
  const useCases = await appliedP1();
  const prose = "Members pay shipping too.";
  const extract: ProposeOut = {
    name: "Members pay",
    slug: "members-pay",
    description: "",
    intents: [{ slug: "members-pay", name: "Members pay", description: "Members pay shipping like everyone else.", change: "modify", useCase: "charge-shipping", provability: "formal", beyondProofReason: "none", quote: "Members pay shipping too.", replaces: [] }],
    useCases: [{ slug: "charge-shipping", change: "modify", name: "Charge shipping", after: "Orders of 100 or more ship free; every other order pays 15." }],
    decisions: [],
    notPromised: [],
  };
  const derive: DeriveV1Out = {
    proofs: [],
    decisions: [{ slug: "members-free-breaks", name: "Members free breaks", question: "P-001.PF-02 guarantees members ship free, which this change breaks. Replace it?", options: [{ label: "Replace it", consequence: "P-001.PF-02 is retired." }, { label: "Keep it", consequence: "Reject the intent." }], recommended: 0, intents: ["P-002.I-01"], breaks: "P-001.PF-02" }],
  };
  const ask = answers(extract, derive);
  let p = approveProposal(reviewProposal(await extractProposal(ask, { id: "P-002", prose, submittedBy: "ana", useCases, policy }), "ana").approveAll().done(), useCases);
  p = await deriveProofs(ask, p, useCases, setup);
  assert.equal(p.status, "changes-requested");
  assert.equal(isProposalApproved(p), false);
  assert.deepEqual(p.intents[0]!.replaces.map((r) => r.proof), ["P-001.PF-02"]);
  assert.equal(p.intents[0]!.status, "proposed");
  assert.deepEqual(p.supersedes, []);
  assert.deepEqual(pendingProposal(p), ["P-002.I-01 not reviewed", "D1 unanswered: P-001.PF-02 guarantees members ship free, which this change breaks. Replace it?"]);
});

test("re-extraction carries the review of unchanged intents", async () => {
  const ask = answers(EXTRACT_1, DERIVE_1);
  const first = reviewProposal(await extractProposal(ask, { id: "P-001", prose: PROSE_1, submittedBy: "ana", useCases: [], policy }), "ana").approve("P-001.I-01").amend("P-001.I-02", "Every member ships free.").done();
  const edited = structuredClone(EXTRACT_1);
  edited.intents[2]!.description = "Every other order pays 15 at checkout.";
  const again: Proposal = carryProposalReview(first, await extractProposal(answers(edited, DERIVE_1), { id: "P-001", prose: PROSE_1, submittedBy: "ana", useCases: [], policy }));
  assert.deepEqual(again.intents.map((i) => [i.status, i.approvals]), [["approved", ["ana"]], ["amended", ["ana"]], ["proposed", []]]);
  assert.equal(again.intents[1]!.description, "Every member ships free.");
});

// ---------------------------------------------------------------------------
// CLI: the same flow from any harness, with the agent's own model (handoff)
// ---------------------------------------------------------------------------

import { mkdtempSync, mkdirSync, readdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { main as cli } from "../src/cli.js";

test("cli: propose → review → derive → prove → apply → regress, LLM steps by handoff", async () => {
  const dir = mkdtempSync(join(tmpdir(), "lastro-"));
  const ws = ["--dir", join(dir, ".lastro"), "--llm", "handoff"];
  writeFileSync(join(dir, "model.json"), JSON.stringify(MODEL));
  writeFileSync(join(dir, "prose.md"), `# Shipping\n\n${PROSE_1}\n`);
  writeFileSync(join(dir, "shipping.mjs"), "export default ({ subtotal, member }) => ({ fee: member || subtotal >= 100 ? 0 : 15 });\n");
  writeFileSync(join(dir, "buggy.mjs"), "export default ({ subtotal, member }) => ({ fee: member || subtotal > 100 ? 0 : 15 });\n");

  let r = await cli(["init", "--model", join(dir, "model.json"), "--subject", "shipping.mjs", "--subject-name", "shipping", ...ws]);
  assert.equal(r.code, 0);
  assert.deepEqual(r.body.review, { approvals: 1, submitterCanApprove: true });

  r = await cli(["propose", join(dir, "prose.md"), "--section", "Shipping", "--as", "ana", ...ws]);
  assert.equal(r.code, 3);
  assert.equal(r.body.step, "extract");
  writeFileSync(r.body.answer as string, JSON.stringify(EXTRACT_1));
  r = await cli(["propose", join(dir, "prose.md"), "--section", "Shipping", "--as", "ana", ...ws]);
  assert.equal(r.code, 1, "proposed, review pending");
  assert.equal((r.body.proposal as { id: string }).id, "P-001");
  assert.ok(readdirSync(join(dir, ".lastro", "proposals")).includes("P-001-shipping-fee.md"));

  assert.equal((await cli(["derive", "P-001", ...ws])).body.status, "review-pending");
  r = await cli(["review", "P-001", "--as", "ana", "comment", "I-02", "Gold members only?", ...ws]);
  assert.deepEqual(r.body.pending, ["P-001.I-01 not reviewed", "P-001.I-02 not reviewed", "P-001.I-03 not reviewed", "T1 open thread on P-001.I-02"]);
  await cli(["review", "1", "--as", "ana", "resolve", "T1", ...ws]);
  r = await cli(["review", "shipping-fee", "--as", "ana", "approve-all", ...ws]);
  assert.equal(r.code, 0);
  assert.equal((r.body.proposal as { status: string }).status, "approved");

  r = await cli(["derive", "P-001", ...ws]);
  assert.equal(r.code, 3);
  assert.equal(r.body.step, "derive");
  writeFileSync(r.body.answer as string, JSON.stringify(DERIVE_1));
  r = await cli(["derive", "P-001", ...ws]);
  assert.equal(r.code, 0, JSON.stringify(r.body));
  assert.equal((r.body.proofs as unknown[]).length, 4);

  r = await cli(["prove", "P-001", join(dir, "buggy.mjs"), ...ws]);
  assert.equal(r.code, 1);
  assert.deepEqual((r.body.failures as { proof: string }[]).map((f) => f.proof), ["P-001.PF-01"]);
  assert.match(r.body.feedback as string, /counterexample: \{"member":false,"subtotal":100\} returned \{"fee":15\}/);
  assert.equal((await cli(["apply", "P-001", ...ws])).code, 1, "apply refuses a failing run");

  const events: Record<string, unknown>[] = [];
  r = await cli(["prove", "P-001", "--stream", ...ws], (e) => events.push(e));
  assert.equal(r.code, 0, JSON.stringify(r.body));
  assert.deepEqual(events[0], { event: "start", proposal: "P-001", proofs: ["P-001.PF-01", "P-001.PF-02", "P-001.PF-03", "P-001.PF-04"] });
  assert.deepEqual(events.filter((e) => e.event === "proof").map((e) => e.proof), ["P-001.PF-01", "P-001.PF-02", "P-001.PF-03", "P-001.PF-04"]);
  assert.deepEqual(r.body.unproved, []);
  // Every prove run is kept, one JSON line each, outside the hashed proposal.
  const runs = readFileSync(join(dir, ".lastro", "proposals", "P-001-shipping-fee.runs.jsonl"), "utf8").trim().split("\n").map((l) => JSON.parse(l) as { run: number; at: string; ok: boolean; outcomes: unknown[] });
  assert.deepEqual(runs.map((x) => [x.run, x.ok, x.outcomes.length]), [[1, false, 4], [2, true, 4]]);
  assert.ok(!Number.isNaN(Date.parse(runs[0]!.at)));

  r = await cli(["apply", "P-001", ...ws]);
  assert.equal(r.code, 0, JSON.stringify(r.body));
  assert.deepEqual(r.body.written, ["UC-001"]);
  const uc = JSON.parse(readFileSync(join(dir, ".lastro", "usecases", "UC-001-charge-shipping.json"), "utf8")) as UseCase;
  assert.equal("proofs" in uc, false);
  const proofFiles = readdirSync(join(dir, ".lastro", "proof")).sort();
  assert.deepEqual(proofFiles.map((f) => f.match(/^P-\d+\.PF-\d+/)![0]), ["P-001.PF-01", "P-001.PF-02", "P-001.PF-03", "P-001.PF-04"]);
  assert.match(readFileSync(join(dir, ".lastro", "proposals", "P-001-shipping-fee.receipt.md"), "utf8"), /Status: \*\*applied\*\*/);

  assert.equal((await cli(["regress", ...ws])).code, 0);
  r = await cli(["regress", join(dir, "buggy.mjs"), ...ws]);
  assert.equal(r.code, 1);
  assert.deepEqual(r.body.regressions, ["P-001.PF-01"]);
  r = await cli(["status", ...ws]);
  assert.deepEqual(r.body.proposals, [{ id: "P-001", slug: "shipping-fee", name: "Shipping fee", status: "applied", pending: 0 }]);
});

test("cli: old workspaces and commands are refused", async () => {
  const dir = mkdtempSync(join(tmpdir(), "lastro-"));
  const ws = ["--dir", join(dir, ".lastro")];
  writeFileSync(join(dir, "model.json"), JSON.stringify(MODEL));
  await cli(["init", "--model", join(dir, "model.json"), ...ws]);
  assert.match(String((await cli(["plan", "prose.md", ...ws])).body.error), /was replaced: use `lastro propose`/);
  mkdirSync(join(dir, ".lastro"), { recursive: true });
  writeFileSync(join(dir, ".lastro", "intents.json"), "{}");
  const r = await cli(["status", ...ws]);
  assert.equal(r.code, 2);
  assert.match(String(r.body.error), /old workspace format.*run `lastro propose <prose-file>` again/);
});
