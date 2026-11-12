// The decision cores of the intent → spec → proof pipeline (UC-003 to UC-028) as subjects Lastro can prove: each
// export takes a model's inputs, runs the real SDK (dist/, so build it first with `npm run build -w lastro`) on a
// small artifact, plan and code under proof, and reports what the SDK decided. The models are in .lastro/models/; the
// proofs in .lastro/proof/ were derived from the use cases' descriptions (`lastro derive --usecases`).
import { readFileSync } from "node:fs";
import * as Lastro from "../dist/src/index.js";

const PROSE = "The code returns x. The code may call a library. Nothing else is promised here.";
const QUOTE = "The code returns x.";
const draft = (statement = "The code returns its input.", quote = QUOTE) => ({ statement, kind: "behavior", provability: "formal", quote });
const toy = (max, extra = {}) => ({ name: "toy", inputs: { x: { type: "int", min: 0, max } }, outputs: { y: { type: "int", min: 0, max } }, ...extra });
const SMALL = 9;
const LARGE = 200_000;
const yIsX = { op: "eq", args: [{ var: "out.y" }, { var: "in.x" }] };
const refusal = (f) => {
  try {
    f();
    return false;
  } catch {
    return true;
  }
};

/** An approved artifact with `n` intents (all approved unless listed in `rejected`). */
function approvedArtifact({ n = 1, rejected = [], decisions = [], notPromised = [] } = {}) {
  let r = Lastro.review(Lastro.createArtifact(PROSE, Array.from({ length: n }, (_, k) => draft(`Intent ${k + 1}.`)), decisions, notPromised));
  for (const id of rejected) r = r.reject(id, "not wanted");
  r = r.approveAll();
  for (const d of decisions.map((_, k) => `D${k + 1}`)) r = r.answer(d, 0);
  return Lastro.approve(r.done());
}

const plan = (artifact, model, parts = {}) => ({
  lastro: "proof-plan/v0", artifactHash: artifact.hash, model, clauses: [], assumptions: [], scenarios: [], beyondProof: [], ...parts,
});
const clause = (id, intentIds, ensures, extra = {}) => ({ id, intentIds, description: `${id} holds`, ensures, ...extra });

// UC-003: change kinds that do not fit together, and decisions the extractor raises.
export function changeKinds({ first, second, options, recommended }) {
  const prose = "Change the checkout. Remove the checkout. Ask about it.";
  const proofs = [{ id: "P-000.PF-01", slug: "p", name: "p", description: "p", intent: "P-000.I-01", useCase: "UC-001", model: "toy", subject: "toy", check: { kind: "clause", ensures: { const: true }, assumptions: [] }, status: "passed" }];
  const useCases = [{ id: "UC-001", slug: "checkout", name: "Checkout", description: "The checkout.", proofs, history: [] }];
  const intent = (slug, change, quote) => ({ slug, name: slug, description: `${slug}.`, change, useCase: "checkout", provability: "formal", beyondProofReason: "none", quote, replaces: [] });
  const intents = [intent("first-change", first, first === "modify" ? "Change the checkout." : "Remove the checkout.")];
  if (second !== "none") intents.push(intent("second-change", second, second === "modify" ? "Change the checkout." : "Remove the checkout."));
  const decisions = options > 0 ? [{ slug: "ask", name: "Ask", question: "Which one?", options: Array.from({ length: options }, (_, k) => ({ label: `o${k}`, consequence: `c${k}` })), recommended, intents: ["first-change"] }] : [];
  const out = {
    name: "Change", slug: "change", description: "A change.", notPromised: [], decisions, intents,
    useCases: [{ slug: "checkout", change: first, name: "Checkout", after: first === "remove" ? "" : "The new checkout." }],
  };
  let p;
  try {
    p = Lastro.proposalFromExtraction({ id: "P-001", prose, submittedBy: "ana", useCases, policy: { approvals: 1, submitterCanApprove: true } }, out);
  } catch {
    return { refused: true, mixDecision: false, llmDecision: false };
  }
  return {
    refused: false,
    mixDecision: p.decisions.some((d) => d.slug.startsWith("change-kind-") && d.origin === "extraction"),
    llmDecision: p.decisions.some((d) => d.slug === "ask" && d.origin === "extraction"),
  };
}

// UC-003, UC-008: answering a decision card; a contradiction's answer becomes an intent to review.
export function decisionAnswer({ origin, known, option, again }) {
  const a = Lastro.createArtifact(PROSE, [draft()], [{ intentIds: ["I1"], question: "Which one?", options: [{ label: "a", consequence: "A." }, { label: "b", consequence: "B." }], recommended: 0 }]);
  a.decisions[0].origin = origin;
  let r = Lastro.review(a, "ana").approveAll();
  try {
    r = r.answer(known ? "D1" : "D9", option);
    if (again) r = r.approveAll().answer("D1", option);
  } catch {
    return { throws: true, chosen: -1, linked: 0, status: "none", approvals: 0 };
  }
  const done = r.done();
  const linked = done.intents.filter((i) => i.decisionId === "D1");
  return {
    throws: false,
    chosen: done.decisions[0].chosen ?? -1,
    linked: linked.length,
    status: linked[0]?.status ?? "none",
    approvals: linked[0]?.approvals?.length ?? 0,
  };
}

// UC-004, UC-006: what is not promised, from the extractor and from reviewers.
export function notPromised({ extracted, appended }) {
  const base = extracted ? ["timing"] : [];
  let r = Lastro.review(Lastro.createArtifact(PROSE, [draft()], [], base)).approveAll();
  if (appended) r = r.notPromised("rate limits");
  const a = Lastro.approve(r.done());
  const without = Lastro.approve(Lastro.review(Lastro.createArtifact(PROSE, [draft()])).approveAll().done());
  const items = a.notPromised;
  const model = toy(SMALL);
  const p = plan(a, model, { clauses: [clause("C1", ["I1"], yIsX)] });
  return Lastro.verify(a, p, { subject: ({ x }) => ({ y: x }) }).then((report) => {
    const receipt = Lastro.renderReceipt(report, p);
    const prompt = Lastro.deriveUserPrompt(a, model);
    return {
      count: items.length,
      inHash: a.hash !== without.hash,
      inDerive: items.every((x) => prompt.includes(x)),
      inReceipt: items.every((x) => receipt.includes(x)) && (items.length === 0 || /Not promised/i.test(receipt)),
    };
  });
}

// UC-006: an intent added by a reviewer.
export function reviewerAdd({ existing, inProse }) {
  const a = Lastro.createArtifact(PROSE, Array.from({ length: existing }, (_, k) => draft(`Intent ${k + 1}.`)));
  const d = Lastro.review(a).add(draft("Also this.", inProse ? "The code may call a library." : "Something the prose never says.")).done();
  const it = d.intents.at(-1);
  return { id: Number(it.id.slice(1)), status: it.status, notInProse: /not in prose/.test(it.reviewNote ?? ""), byReviewer: /added by (a )?reviewer/.test(it.reviewNote ?? "") };
}

// UC-011: domains and the plan's demand for outputs.
export function domainKinds({ kind, hasOutputs }) {
  const d = { bool: { type: "bool" }, enum: { type: "enum", values: ["a", "b", "c"] }, "int-bounded": { type: "int", min: 0, max: 4 }, "int-unbounded": { type: "int", min: 0 }, string: { type: "string", examples: ["a"] } }[kind];
  const size = Lastro.domainSize(d);
  const model = { name: "m", inputs: { v: d }, outputs: hasOutputs ? { y: { type: "bool" } } : {} };
  const a = approvedArtifact();
  const rejected = refusal(() => Lastro.validatePlan(a, plan(a, model, { clauses: [clause("C1", ["I1"], { const: true })] })));
  let order = true;
  if (Number.isFinite(size)) {
    const one = [...Lastro.enumerateSpace({ b: d, a: { type: "bool" } })];
    const two = [...Lastro.enumerateSpace({ a: { type: "bool" }, b: d })];
    order = JSON.stringify(one) === JSON.stringify(two);
  }
  return { finite: Number.isFinite(size), size: Number.isFinite(size) ? size : 0, sortedOrder: order, rejected };
}

// UC-012: LSL interpretation: arity, typing, unbound variables, depth.
export function lslEval({ op, arity, argType, unbound, depth }) {
  const model = { name: "m", inputs: { b: { type: "bool" }, n: { type: "int", min: 0, max: 3 }, s: { type: "string" } }, outputs: { y: { type: "bool" } } };
  const arg = unbound ? { var: "in.missing" } : { bool: { var: "in.b" }, int: { var: "in.n" }, string: { var: "in.s" } }[argType];
  let e = { op, args: Array.from({ length: arity }, () => arg) };
  // `depth` more levels of nesting around the expression.
  for (let k = 0; k < depth; k++) e = { op: "ite", args: [{ const: true }, e, { var: "in.b" }] };
  const ctx = { input: { b: true, n: 1, s: "a" }, output: { y: true } };
  const invalid = Lastro.validateExpr(e, model).length > 0;
  let error = false;
  try {
    Lastro.evaluate(e, ctx);
  } catch {
    error = true;
  }
  return { invalid, error };
}

// UC-014: the checks every plan passes before anything is verified.
export function planCheck({ approved, tampered, ref, duplicate, scenarioComplete, covered }) {
  let a = Lastro.review(Lastro.createArtifact(PROSE, [draft("One."), draft("Two."), draft("Three.")])).reject("I3", "not wanted").approveAll().done();
  a = approved ? Lastro.approve(a) : { ...a, hash: undefined };
  const artifactHash = a.hash ?? "sha256:none";
  if (tampered) a = { ...a, intents: a.intents.map((i, k) => (k === 0 ? { ...i, statement: "Changed." } : i)) };
  const refs = { "known-approved": ["I1"], unknown: ["I9"], rejected: ["I3"], none: [] }[ref];
  const model = toy(SMALL);
  const clauses = [clause("C1", refs, yIsX)];
  if (duplicate) clauses.push(clause("C1", ["I1"], yIsX));
  if (covered) clauses.push(clause("C2", ["I2"], yIsX));
  const scenarios = [{ id: "S1", intentIds: ["I1"], given: "g", when: "w", then: "t", input: scenarioComplete ? { x: 1 } : {}, expect: yIsX }];
  const c = Lastro.checkPlan(a, { ...plan(a, model, { clauses, scenarios }), artifactHash });
  return { errors: c.errors.length > 0, gap: c.uncovered.includes("I2") };
}

// UC-015: contradictions in the spec go back to the team as decision cards.
export async function contradictions({ a, b, c1Requires, twice }) {
  const model = { name: "m", inputs: { x: { type: "bool" } }, outputs: { y: { type: "bool" } } };
  // C2 applies when x holds; C1 always, or (c1Requires) only when x holds too. Either way they clash at x = true when a ≠ b.
  const c1 = clause("C1", ["I1"], { op: "eq", args: [{ var: "out.y" }, { const: a }] }, c1Requires ? { requires: { var: "in.x" } } : {});
  const c2 = clause("C2", ["I2"], { op: "eq", args: [{ var: "out.y" }, { const: b }] }, { requires: { var: "in.x" } });
  const deriver = new Lastro.StaticDeriver((art) => plan(art, model, { clauses: [c1, c2] }));
  const lastro = Lastro.createLastro({ model, deriver });
  const reviewed = Lastro.review(Lastro.createArtifact(PROSE, [draft("One."), draft("Two.")])).approveAll().done();
  const spec = await lastro.spec(reviewed);
  let back = lastro.returnDecisions(spec);
  if (twice) back = Lastro.raiseDecisions(back, spec.contradictions);
  const card = spec.contradictions[0];
  return {
    cards: spec.contradictions.length,
    ok: spec.ok,
    options: card?.options.length ?? 0,
    recommended: card?.recommended ?? -1,
    origin: card?.origin ?? "none",
    returned: back.decisions.length,
    reopened: !back.hash && Lastro.pendingReview(back).length > 0,
  };
}

const VERIFY_CODE = {
  none: (max) => ({ x }) => ({ y: x }),
  wrong: (max) => ({ x }) => ({ y: x % 2 === 0 ? (x === max ? x - 1 : x + 1) : x }),
  throws: () => ({ x }) => {
    if (x % 2 === 0) throw new Error("boom");
    return { y: x };
  },
  "out-of-domain": () => ({ x }) => ({ y: x % 2 === 0 ? -1 : x }),
};

// UC-016 to UC-019: one clause checked exhaustively or by samples; counterexamples, vacuity and weakness.
export async function verifyClause({ large, bug, kind }) {
  const max = large ? LARGE : SMALL;
  const model = toy(max);
  const ensures = {
    normal: yIsX,
    vacuous: yIsX,
    weak: { op: "or", args: [yIsX, { op: "neq", args: [{ var: "out.y" }, { var: "in.x" }] }] },
    erroring: { op: "lt", args: [{ var: "out.y" }, { const: "a" }] },
  }[kind];
  const a = approvedArtifact();
  const p = plan(a, model, { clauses: [clause("C1", ["I1"], ensures, kind === "vacuous" ? { requires: { op: "lt", args: [{ var: "in.x" }, { const: 0 }] } } : {})] });
  const r = (await Lastro.verify(a, p, { subject: VERIFY_CODE[bug](max) })).clauses[0];
  return {
    verdict: r.verdict,
    evidence: r.evidence,
    method: r.method,
    counterexample: !!r.counterexample,
    noted: /invalid runs/.test(r.note ?? ""),
    errorNoted: /error|expects|number|boolean/i.test(r.note ?? ""),
    ruleOfThree: r.checked > 0 && r.failureRateUpperBound95 === 3 / r.checked,
    checkedAll: r.checked === max + 1,
  };
}

// UC-020: recorded production runs.
export async function traceCheck({ subject, trace }) {
  const model = toy(SMALL);
  const a = approvedArtifact();
  const p = plan(a, model, { clauses: [clause("C1", ["I1"], yIsX)] });
  const traces = trace === "none" ? [] : [1, 2, 3, 4, 5].map((x) => ({ input: { x }, output: { y: trace === "breaks" && x === 3 ? 4 : x }, ref: `run-${x}` }));
  const r = (await Lastro.verify(a, p, { ...(subject === "right" ? { subject: ({ x }) => ({ y: x }) } : {}), traces })).clauses[0];
  return {
    verdict: r.verdict,
    evidence: r.evidence,
    violatingTrace: r.traceViolation?.ref === "run-3",
    trustedNote: /trusted piece/i.test(r.note ?? ""),
    ruleOfThree: r.failureRateUpperBound95 === 3 / 5,
  };
}

// UC-021: Given/When/Then scenarios.
export async function scenarioRun({ subject }) {
  const model = toy(SMALL);
  const a = approvedArtifact();
  const p = plan(a, model, { scenarios: [{ id: "S1", intentIds: ["I1"], given: "x is 3", when: "it runs", then: "y is 3", input: { x: 3 }, expect: yIsX }] });
  const code = { none: undefined, right: ({ x }) => ({ y: x }), wrong: ({ x }) => ({ y: x + 1 }), throws: () => { throw new Error("boom"); } }[subject];
  const report = await Lastro.verify(a, p, code ? { subject: code } : {});
  const s = report.scenarios[0];
  return { status: s.status, output: !!s.output, error: !!s.error, empirical: report.intents[0].evidence.includes("empirical") };
}

// UC-022 to UC-024: the verdict of one intent, from what covers it.
export async function intentRollup({ coverage, scenario, assumption, beyond }) {
  const large = coverage === "evidenced";
  const max = large ? LARGE : SMALL;
  const model = toy(max);
  const a = approvedArtifact();
  const parts = { clauses: [], scenarios: [], assumptions: [], beyondProof: [] };
  if (coverage !== "none")
    parts.clauses.push(clause("C1", ["I1"], yIsX, { ...(coverage === "vacuous" ? { requires: { op: "lt", args: [{ var: "in.x" }, { const: 0 }] } } : {}), ...(assumption ? { assumes: ["T1"] } : {}) }));
  if (scenario !== "none")
    parts.scenarios.push({ id: "S1", intentIds: ["I1"], given: "x is 1", when: "it runs", then: "y is x", input: { x: 1 }, expect: scenario === "passed" ? yIsX : { op: "neq", args: [{ var: "out.y" }, { var: "in.x" }] } });
  if (assumption)
    parts.assumptions.push({ id: "T1", intentIds: ["I1"], piece: "library", contract: "it returns x", youAccept: "the library", ifWrong: "y is wrong", checkedBy: scenario !== "none" ? ["S1"] : [] });
  if (beyond) parts.beyondProof.push({ intentId: "I1", reason: "external-code", note: "the library" });
  const code = coverage === "violated" ? VERIFY_CODE.wrong(max) : VERIFY_CODE.none(max);
  const report = await Lastro.verify(a, plan(a, model, parts), { subject: code });
  const i = report.intents[0];
  return {
    verdict: i.verdict,
    confidence: i.confidence === 0 ? "zero" : i.confidence === 1 ? "one" : "partial",
    trusted: i.evidence.includes("trusted"),
    notes: i.notes.length > 0,
    reportOk: report.ok,
  };
}

// UC-027: failures and spec gaps handed to whoever writes the code.
export async function agentFeedback({ codeOk, gap }) {
  const model = toy(SMALL);
  const a = approvedArtifact({ n: 2 });
  const clauses = [clause("C1", ["I1"], yIsX)];
  if (gap === "none") clauses.push(clause("C2", ["I2"], { op: "ge", args: [{ var: "out.y" }, { var: "in.x" }] }));
  if (gap === "vacuous") clauses.push(clause("C2", ["I2"], yIsX, { requires: { op: "lt", args: [{ var: "in.x" }, { const: 0 }] } }));
  const deriver = new Lastro.StaticDeriver((art) => plan(art, model, { clauses }));
  const lastro = Lastro.createLastro({ model, deriver });
  const spec = await lastro.spec(Lastro.review(Lastro.createArtifact(PROSE, [draft("One."), draft("Two.")])).approveAll().done());
  const proof = await lastro.prove(spec, codeOk ? VERIFY_CODE.none(SMALL) : VERIFY_CODE.wrong(SMALL));
  const brief = Lastro.agentFeedback(proof);
  return {
    failures: proof.failures.length,
    brief: brief.startsWith("Every proof passes") ? "all-pass" : "fix",
    counterexample: proof.failures.length > 0 && proof.failures.every((f) => f.input && f.output && brief.includes("counterexample")),
    specGaps: proof.specGaps.length,
    listsGaps: proof.specGaps.length > 0 && brief.includes(proof.specGaps[0]),
  };
}

// UC-028: one run explained clause by clause.
export function explainRun({ applies, holds, erroring }) {
  const model = { name: "m", inputs: { r: { type: "bool" } }, outputs: { y: { type: "bool" }, s: { type: "string" } } };
  const a = approvedArtifact();
  const ensures = erroring ? { op: "lt", args: [{ var: "out.s" }, { const: 1 }] } : { op: "eq", args: [{ var: "out.y" }, { const: true }] };
  const p = plan(a, model, { clauses: [clause("C1", ["I1"], ensures, { requires: { var: "in.r" } })] });
  return { status: Lastro.explainRun(p, { r: applies }, { y: holds, s: "a" })[0].status };
}

let prover;
const rustProver = async () => (prover ??= await Lastro.loadRustProver(readFileSync(new URL("../wasm/lastro_core.wasm", import.meta.url))));

// UC-025: the Rust prover re-checks every clause from the table of the code's runs.
export async function crosscheck({ table, bug }) {
  const model = toy(SMALL);
  const a = approvedArtifact();
  const p = plan(a, model, { clauses: [clause("C1", ["I1"], yIsX)] });
  const code = bug ? VERIFY_CODE.wrong(SMALL) : VERIFY_CODE.none(SMALL);
  // The table the prover gets: the code's runs, with one row dropped, repeated, contradicted or out of domain.
  const alter = (rows) => {
    if (table === "missing-row") return rows.slice(1);
    if (table === "repeated-row") return [...rows, rows[1]];
    if (table === "conflicting-row") return [...rows, { input: rows[1].input, output: { y: rows[1].output.y === 0 ? 1 : 0 } }];
    if (table === "out-of-domain-row") return [...rows.slice(1), { input: rows[0].input, output: { y: -1 } }];
    return rows;
  };
  const real = await rustProver();
  let rust;
  const lastro = Lastro.createLastro({ model, deriver: new Lastro.StaticDeriver(() => p), rust: { prove: (pl, rows) => (rust = real.prove(pl, alter(rows))) } });
  const proof = await lastro.prove({ artifact: a, plan: p, contradictions: [], ok: true }, code);
  return {
    complete: rust.coverage.complete,
    proved: rust.clauses[0].verdict === "proved",
    rustOk: rust.ok,
    digest: /^[0-9a-f]{64}$/.test(rust.tableDigest.replace(/^sha256:/, "")),
    agree: proof.enginesAgree === true,
    ok: proof.ok,
    sdkOk: proof.report.ok,
  };
}

// UC-026: what the proposal's receipt shows.
export function receiptSections({ notPromised, regression, stillPass, violated }) {
  const prose = "Change the checkout.";
  const old = { id: "P-000.PF-01", slug: "old", name: "old", description: "The old checkout holds.", intent: "P-000.I-01", useCase: "UC-001", model: "toy", subject: "toy", check: { kind: "clause", ensures: yIsX, assumptions: [] }, status: "passed" };
  const useCases = [{ id: "UC-001", slug: "checkout", name: "Checkout", description: "The old checkout.", proofs: [old], history: [] }];
  const out = {
    name: "Change", slug: "change", description: "A change.", decisions: [], notPromised: notPromised ? ["speed"] : [],
    intents: [{ slug: "new-checkout", name: "New checkout", description: "The checkout changes.", change: "modify", useCase: "checkout", provability: "formal", beyondProofReason: "none", quote: prose, replaces: [{ proof: "P-000.PF-01", reason: "it changes" }] }],
    useCases: [{ slug: "checkout", change: "modify", name: "Checkout", after: "The new checkout." }],
  };
  let p = Lastro.proposalFromExtraction({ id: "P-001", prose, submittedBy: "ana", useCases, policy: { approvals: 1, submitterCanApprove: true } }, out);
  p = Lastro.approveProposal(Lastro.reviewProposal(p, "ana").approveAll().done(), useCases);
  p.proofs = [{ ...old, id: "P-001.PF-01", slug: "new", description: "The new checkout holds.", intent: "P-001.I-01", replaces: ["P-000.PF-01"], status: "pending" }];
  const cx = { input: { x: 2 }, output: { y: 3 } };
  p.lastRun = {
    ok: !violated && !regression,
    outcomes: [
      { proof: "P-001.PF-01", role: "proposal", useCase: "UC-001", intent: "P-001.I-01", verdict: violated ? "violated" : "proved", passed: !violated, ...(violated ? { counterexample: cx } : {}) },
      { proof: "P-009.PF-01", role: "regression", useCase: "UC-002", verdict: regression ? "violated" : "proved", passed: !regression, ...(regression ? { counterexample: cx } : {}) },
      { proof: "P-000.PF-01", role: "superseded", useCase: "UC-001", verdict: stillPass ? "proved" : "violated", passed: stillPass },
    ],
    regressions: regression ? ["P-009.PF-01"] : [],
    replacedStillPass: stillPass ? ["P-000.PF-01"] : [],
    unproved: [],
  };
  const r = Lastro.renderProposalReceipt(p, useCases);
  return {
    showsDiff: r.includes("Before: The old checkout.") && r.includes("After: The new checkout."),
    showsReplaced: /replaces P-000\.PF-01.*now guarded by P-001\.PF-01/.test(r),
    showsRegression: r.includes("P-009.PF-01"),
    showsStillPass: /P-000\.PF-01\*\* is superseded but still passes/.test(r),
    showsCounterexample: r.includes(JSON.stringify(cx.input)),
    showsNotPromised: /## Not promised[\s\S]*speed/.test(r),
  };
}

// UC-029: what the gap review sends to the LLM, and what it accepts back.
export async function gapReview({ assumption, beyond, answer }) {
  const model = toy(SMALL);
  const parts = { clauses: [clause("C1", ["I1"], yIsX)] };
  if (assumption) parts.assumptions = [{ id: "T1", intentIds: ["I1"], piece: "the-library", contract: "returns x", youAccept: "it", ifWrong: "y breaks", checkedBy: [] }];
  if (beyond) parts.beyondProof = [{ intentId: "I1", reason: "human-judgment", note: "readable-output" }];
  let request;
  const ask = async (req) => {
    request = req;
    return answer === "empty" ? { gaps: [] } : { gaps: [{ intentIds: ["I1"], kind: "partial-meaning", description: "y is not checked for negatives", suggestion: "add a clause", severity: "low" }] };
  };
  const lastro = Lastro.createLastro({ model, ask, deriver: new Lastro.StaticDeriver((art) => plan(art, model, parts)) });
  const spec = await lastro.spec(Lastro.review(Lastro.createArtifact(PROSE, [draft("The code returns its input.")])).approveAll().done());
  const gaps = await lastro.gapReview(spec, "export const f = (x) => x; // SOURCE");
  const user = request.user;
  return {
    step: request.step === "gap-review",
    intents: user.includes("The code returns its input."),
    clauses: user.includes(Lastro.show(yIsX)),
    pieces: !assumption || user.includes("the-library"),
    beyondProof: !beyond || user.includes("readable-output"),
    source: user.includes("// SOURCE"),
    gaps: gaps.length,
  };
}

// UC-034: answers from any transport are checked, never trusted.
export async function untrustedAnswers({ via, quote, lsl }) {
  const model = toy(SMALL);
  const extraction = { intents: [{ statement: "Returns x.", kind: "behavior", provability: "formal", quote: quote === "anchored" ? QUOTE : "A sentence the prose never had.", beyondProofReason: "none" }], decisions: [], notPromised: [] };
  const derivation = { clauses: [{ id: "C1", intentIds: ["I1"], description: "y is x", requires: "", ensures: JSON.stringify(lsl === "valid" ? yIsX : { op: "eq", args: [{ var: "out.z" }, { var: "in.x" }] }), assumes: [] }], assumptions: [], scenarios: [], beyondProof: [] };
  const opts = via === "ask"
    ? { model, ask: async (req) => (req.step === "extract" ? extraction : derivation) }
    : { model, extractor: new Lastro.StaticExtractor((prose) => Lastro.artifactFromExtraction(prose, extraction)), deriver: new Lastro.StaticDeriver((art, m) => Lastro.planFromDerivation(art, m, derivation)) };
  const lastro = Lastro.createLastro(opts);
  let artifact;
  try {
    artifact = await lastro.plan(PROSE);
  } catch {
    return { extracted: false, derived: false };
  }
  try {
    await lastro.spec(lastro.review(artifact, "ana").approveAll().done());
  } catch {
    return { extracted: true, derived: false };
  }
  return { extracted: true, derived: true };
}

export { refusal };
