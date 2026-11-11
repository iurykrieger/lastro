import { test } from "node:test";
import assert from "node:assert/strict";
import {
  ArtifactError,
  D,
  L,
  PlanError,
  checkConsistency,
  createArtifact,
  explainRun,
  approve,
  hashOf,
  isApproved,
  review,
  toSmtLib,
  validateExpr,
  validatePlan,
  verify,
  type IntentArtifact,
  type ProofPlan,
} from "../src/index.js";
import { sha256Hex } from "../src/sha256.js";
import { existsSync, readFileSync } from "node:fs";
import { loadRustProver, tabulate } from "../src/index.js";
import { BUGS, FIXED_I5, LOGIN_SOURCE, MODEL, compileLogin, deriveLoginPlan, extractLogin, makeLogin, plantBug } from "../examples/login/login.js";

function reviewed(opts: { d1?: number; fixI5?: boolean } = {}): IntentArtifact {
  let r = review(extractLogin()).approveAll().answer("D1", opts.d1 ?? 0).answer("D2", 0);
  if (opts.fixI5 !== false) r = r.amend("I5", FIXED_I5);
  return approve(r.done());
}

test("sha256 matches the standard test vectors", () => {
  assert.equal(sha256Hex("abc"), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
  assert.equal(sha256Hex(""), "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
});

test("quotes must anchor to the prose (intent map)", () => {
  assert.throws(
    () => createArtifact("only the right password", [{ statement: "x", kind: "behavior", provability: "formal", quote: "something not in the prose" }]),
    ArtifactError,
  );
  const a = extractLogin();
  const i1 = a.intents[0]!;
  assert.equal(a.prose.slice(i1.source.start, i1.source.end), i1.source.quote);
});

test("approve requires every intent reviewed and every decision answered", () => {
  assert.throws(() => approve(extractLogin()), ArtifactError);
  assert.throws(() => approve(review(extractLogin()).approveAll().done()), /D1 unanswered/);
  const f = reviewed();
  assert.ok(f.hash?.startsWith("sha256:"));
  assert.ok(isApproved(f));
  const tampered = structuredClone(f);
  tampered.intents[0]!.statement = "something else";
  assert.equal(isApproved(tampered), false);
});

test("the correct login is proved; beyond-proof intents are reported, not hidden", async () => {
  const a = reviewed();
  const plan = deriveLoginPlan(a);
  assert.deepEqual(validatePlan(a, plan), []);
  const report = await verify(a, plan, { subject: makeLogin() });
  assert.equal(report.ok, true);
  assert.ok(report.clauses.every((c) => c.verdict === "proved" && c.method === "exhaustive"));
  assert.equal(report.summary["beyond-proof"], 2);
  assert.equal(report.intents.find((i) => i.intentId === "I1")!.verdict, "proved-under-assumptions");
  assert.ok(report.scenarios.every((s) => s.status === "passed"));
});

test("every planted bug is rejected with a counterexample", async () => {
  const a = reviewed();
  const plan = deriveLoginPlan(a);
  for (const bug of BUGS) {
    const report = await verify(a, plan, { subject: makeLogin(bug.id) });
    assert.equal(report.ok, false, `bug ${bug.id} should be caught`);
    const failed = report.clauses.filter((c) => c.verdict === "violated");
    assert.ok(failed.length > 0, bug.id);
    assert.ok(failed.every((c) => c.counterexample), bug.id);
  }
});

test("one-way spec: no formal clause fails on 'refuse everyone', but vacuity and scenarios expose it", async () => {
  const oneWay = reviewed({ d1: 1 });
  const r1 = await verify(oneWay, deriveLoginPlan(oneWay), { subject: makeLogin("refuse-everyone") });
  assert.equal(r1.clauses.filter((c) => c.verdict === "violated").length, 0, "the proof alone accepts a login that refuses everyone");
  assert.ok(r1.clauses.some((c) => c.verdict === "vacuous"), "clauses guarded by 'a token was issued' hold vacuously");
  assert.equal(r1.ok, false);
  const bothWays = reviewed({ d1: 0 });
  const r2 = await verify(bothWays, deriveLoginPlan(bothWays), { subject: makeLogin("refuse-everyone") });
  assert.ok(r2.clauses.some((c) => c.verdict === "violated"), "the both-ways clause rejects it formally");
});

test("an intent the AI misread and nobody fixed shows up as violated", async () => {
  const a = reviewed({ fixI5: false });
  const report = await verify(a, deriveLoginPlan(a), { subject: makeLogin() });
  assert.equal(report.intents.find((i) => i.intentId === "I5")!.verdict, "violated");
});

test("rejected intents cannot be built, and the plan is bound to the artifact hash", () => {
  const a = approve(review(extractLogin()).approveAll().reject("I5", "out of scope").answer("D1", 0).answer("D2", 0).done());
  const plan = deriveLoginPlan(reviewed());
  assert.throws(() => validatePlan(a, plan), PlanError);
  const own = deriveLoginPlan(a);
  assert.deepEqual(validatePlan(a, own), []);
});

function tinyPlan(clauses: ProofPlan["clauses"], inputs = { x: D.int(0, 9) }): { a: IntentArtifact; plan: ProofPlan } {
  const a = approve(review(createArtifact("x doubles", [{ statement: "y = 2x", kind: "behavior", provability: "formal", quote: "x doubles" }])).approveAll().done());
  return {
    a,
    plan: { lastro: "proof-plan/v0", artifactHash: a.hash!, model: { name: "double", inputs, outputs: { y: D.int(-1000, 10_000_000) } }, clauses, assumptions: [], scenarios: [], beyondProof: [] },
  };
}

test("weak (tautological) and vacuous clauses are flagged", async () => {
  const { a, plan } = tinyPlan([
    { id: "C1", intentIds: ["I1"], description: "tautology", ensures: L.or(L.ge(L.out("y"), 0), L.lt(L.out("y"), 0)) },
    { id: "C2", intentIds: ["I1"], description: "never applies", requires: L.gt(L.in("x"), 100), ensures: L.eq(L.out("y"), 0) },
    { id: "C3", intentIds: ["I1"], description: "y = 2x", ensures: L.eq(L.out("y"), { op: "mul", args: [L.c(2), L.in("x")] }) },
  ]);
  const r = await verify(a, plan, { subject: (i) => ({ y: 2 * (i.x as number) }) });
  assert.equal(r.clauses[0]!.verdict, "weak");
  assert.equal(r.clauses[1]!.verdict, "vacuous");
  assert.equal(r.clauses[2]!.verdict, "proved");
});

test("large domains fall back to seeded sampling with a quantified bound", async () => {
  const { a, plan } = tinyPlan(
    [{ id: "C1", intentIds: ["I1"], description: "y = 2x", ensures: L.eq(L.out("y"), { op: "mul", args: [L.c(2), L.in("x")] }) }],
    { x: D.int(0, 1_000_000) },
  );
  const r = await verify(a, plan, { subject: (i) => ({ y: 2 * (i.x as number) }), samples: 3000 });
  assert.equal(r.clauses[0]!.verdict, "evidenced");
  assert.equal(r.clauses[0]!.failureRateUpperBound95, 0.001);
  const again = await verify(a, plan, { subject: (i) => ({ y: 2 * (i.x as number) }), samples: 3000 });
  assert.equal(hashOf(again.clauses), hashOf(r.clauses), "same seed, same verdict");
});

test("a production trace can contradict a model proof (broken trusted piece)", async () => {
  const a = reviewed();
  const plan = deriveLoginPlan(a);
  const report = await verify(a, plan, {
    subject: makeLogin(),
    traces: [{ ref: "req-123", input: { sends: "email", accountFound: true, password: "wrong", infra: "ok" }, output: { status: 200, token: true, role: "customer" } }],
  });
  const c5 = report.clauses.find((c) => c.traceViolation);
  assert.ok(c5);
  assert.match(c5!.note ?? "", /trusted piece/);
});

test("contradictory clauses come back as a decision card", () => {
  const { plan } = tinyPlan([
    { id: "C1", intentIds: ["I1"], description: "y is 1 when x=3", requires: L.eq(L.in("x"), 3), ensures: L.eq(L.out("y"), 1) },
    { id: "C2", intentIds: ["I1"], description: "y is 2 when x=3", requires: L.eq(L.in("x"), 3), ensures: L.eq(L.out("y"), 2) },
  ]);
  plan.model.outputs = { y: D.int(0, 5) };
  const cards = checkConsistency(plan);
  assert.equal(cards.length, 1);
  assert.deepEqual(cards[0]!.options.map((o) => o.label), ["Relax C1", "Relax C2"]);
});

test("explainRun classifies each clause for a single run", () => {
  const plan = deriveLoginPlan(reviewed());
  const input = { sends: "email", accountFound: true, password: "right", infra: "ok" };
  const ex = explainRun(plan, input, makeLogin()(input));
  assert.ok(ex.every((e) => e.status !== "fails"));
  assert.ok(ex.some((e) => e.status === "holds") && ex.some((e) => e.status === "if-part-false"));
});

test("LSL validation rejects unknown variables; SMT-LIB export is well-formed", () => {
  assert.ok(validateExpr(L.eq(L.in("nope"), 1), MODEL).length > 0);
  assert.deepEqual(validateExpr(L.eq(L.in("sends"), "email"), MODEL), []);
  const plan = deriveLoginPlan(reviewed());
  const smt = toSmtLib(plan.model, plan.clauses[0]!);
  assert.match(smt, /\(assert \(not /);
  assert.match(smt, /\(check-sat\)/);
  assert.equal((smt.match(/\(/g) ?? []).length, (smt.match(/\)/g) ?? []).length);
});

test("bug patches apply cleanly and compiled source matches the reference", () => {
  for (const b of BUGS) assert.notEqual(plantBug(LOGIN_SOURCE, b.id), LOGIN_SOURCE, b.id);
  const ref = compileLogin(LOGIN_SOURCE);
  const steps: { name: string }[] = [];
  const out = ref({ sends: "both", accountFound: true, password: "right", infra: "ok" }, steps as never);
  assert.deepEqual(out, { status: 200, token: true, role: "customer" });
  assert.equal(steps.length, 5);
});

const WASM = new URL("../../wasm/lastro_core.wasm", import.meta.url);

test("Rust prover (WASM) agrees with the TypeScript engine on every bug", { skip: !existsSync(WASM) && "run `npm run wasm` first" }, async () => {
  const prover = await loadRustProver(readFileSync(WASM));
  const a = reviewed();
  const plan = deriveLoginPlan(a);
  for (const bug of [undefined, ...BUGS.map((b) => b.id)]) {
    const subject = makeLogin(bug);
    const ts = await verify(a, plan, { subject });
    const rs = prover.prove(plan, await tabulate(plan.model, subject));
    assert.equal(rs.coverage.complete, true);
    assert.equal(rs.coverage.space, 72);
    for (const c of ts.clauses) assert.equal(rs.clauses.find((x) => x.clauseId === c.clauseId)!.verdict, c.verdict, `${bug ?? "reference"} ${c.clauseId}`);
    assert.equal(rs.ok, ts.clauses.every((c) => c.verdict === "proved"), bug ?? "reference");
  }
});

test("Rust prover refuses to call a partial table a proof", { skip: !existsSync(WASM) && "run `npm run wasm` first" }, async () => {
  const prover = await loadRustProver(readFileSync(WASM));
  const plan = deriveLoginPlan(reviewed());
  const table = (await tabulate(plan.model, makeLogin())).slice(0, 50);
  const rs = prover.prove(plan, table);
  assert.equal(rs.coverage.complete, false);
  assert.equal(rs.coverage.missing, 22);
  assert.ok(rs.clauses.every((c) => c.verdict !== "proved"));
});

// ---------------------------------------------------------------------------
// LLM steps over a fake transport, and re-extraction
// ---------------------------------------------------------------------------

import { LlmIntentExtractor, LlmProofDeriver, carryReview, type AskJson } from "../src/index.js";

function reviewedLogin(): IntentArtifact {
  const a = extractLogin();
  for (const it of a.intents) it.status = "approved";
  const i5 = a.intents.find((i) => i.id === "I5")!;
  i5.originalStatement = i5.statement;
  i5.statement = FIXED_I5;
  i5.status = "amended";
  for (const d of a.decisions) d.chosen = d.recommended;
  return a;
}

test("carryReview keeps verdicts for unchanged intents and reapplies amendments", () => {
  const before = reviewedLogin();
  const prose = before.prose.replace("a database failure gets 500", "a database failure gets 503");
  const fresh = extractLogin(prose.replace("gets 503", "gets 500")); // same wording, re-extracted
  const next = carryReview(before, fresh);
  assert.ok(next.intents.every((i) => i.status !== "proposed"), "nothing changed, nothing to review");
  const i5 = next.intents.find((i) => i.id === "I5")!;
  assert.equal(i5.status, "amended");
  assert.equal(i5.statement, FIXED_I5);
  assert.deepEqual(next.decisions.map((d) => d.chosen), [0, 0]);

  fresh.intents[6]!.statement = "A database failure gets 503 and no token.";
  const changed = carryReview(before, fresh);
  assert.deepEqual(changed.intents.filter((i) => i.status === "proposed").map((i) => i.id), ["I7"]);
});

test("LLM steps turn JSON answers into checked Lastro objects", async () => {
  const approved = approve(reviewedLogin());
  const reference = deriveLoginPlan(approved);
  const answers: Record<string, unknown> = {
    extract: {
      intents: [{ statement: "Only the right password gets a token.", kind: "invariant", provability: "formal", beyondProofReason: "none", quote: "only the right password gets a token" }],
      decisions: [],
      notPromised: [],
    },
    derive: {
      clauses: reference.clauses.map((c) => ({ ...c, requires: c.requires ? JSON.stringify(c.requires) : "", ensures: JSON.stringify(c.ensures), assumes: c.assumes ?? [] })),
      assumptions: reference.assumptions,
      scenarios: reference.scenarios.map((s) => ({ ...s, inputJson: JSON.stringify(s.input), expect: JSON.stringify(s.expect) })),
      beyondProof: reference.beyondProof,
    },
  };
  const ask: AskJson = async ({ step }) => answers[step];

  const extracted = await new LlmIntentExtractor(ask).extract(approved.prose);
  assert.equal(extracted.intents[0]!.source.quote, "only the right password gets a token");

  const plan = await new LlmProofDeriver(ask).derive(approved, MODEL);
  validatePlan(approved, plan);
  const report = await verify(approved, plan, { subject: makeLogin() });
  assert.equal(report.ok, true);

  // A derivation that references a variable outside the model is refused.
  (answers.derive as { clauses: { ensures: string }[] }).clauses[0]!.ensures = JSON.stringify({ var: "out.cookie" });
  await assert.rejects(new LlmProofDeriver(ask).derive(approved, MODEL), /cookie/);
});

// ---------------------------------------------------------------------------
// The pipeline facade: two reviewers, contradictions as decisions, agent loop
// ---------------------------------------------------------------------------

import { agentFeedback, createLastro, raiseDecisions } from "../src/index.js";

function loginPipeline(requiredApprovals?: number) {
  return createLastro({
    model: MODEL,
    extractor: { extract: async (prose: string) => extractLogin(prose) },
    deriver: { derive: async (a: IntentArtifact) => deriveLoginPlan(a) },
    ...(requiredApprovals ? { requiredApprovals } : {}),
  });
}

test("pipeline: one approval by default", async () => {
  const lastro = loginPipeline();
  const a = lastro.review(await lastro.plan(extractLogin().prose), "ana").approveAll().amend("I5", FIXED_I5).answer("D1", 0).answer("D2", 0).done();
  assert.deepEqual(lastro.pending(a), []);
});

test("pipeline: a policy of two approvals needs two distinct reviewers before the spec", async () => {
  const lastro = loginPipeline(2);
  let a = await lastro.plan(extractLogin().prose);
  a = lastro.review(a, "ana").approveAll().amend("I5", FIXED_I5).answer("D1", 0).answer("D2", 0).done();
  assert.ok(lastro.pending(a).some((p) => /I1 needs 1 more approval/.test(p)));
  await assert.rejects(lastro.spec(a));
  // Ana's amendment resets I5: Bruno must approve the new wording too.
  a = lastro.review(a, "ana").approveAll().done();
  assert.ok(lastro.pending(a).some((p) => /I5 needs 1 more/.test(p)));
  a = lastro.review(a, "bruno").approveAll().done();
  assert.deepEqual(lastro.pending(a), []);
  const spec = await lastro.spec(a);
  assert.equal(spec.ok, true);
  const proof = await lastro.prove(spec, makeLogin());
  assert.equal(proof.ok, true);
  assert.deepEqual(proof.failures, []);
  assert.match(proof.receipt, /login_flow/);
});

test("pipeline: contradictions go back to the team as unanswered decisions", () => {
  const a = approve(reviewedLogin());
  const raised = raiseDecisions(a, [{ id: "X1", intentIds: ["I5", "I7"], question: "C5 and C7 cannot both hold. Which gives way?", options: [{ label: "Relax C5", consequence: "" }, { label: "Relax C7", consequence: "" }], recommended: 0, origin: "consistency-check" }]);
  assert.equal(raised.hash, undefined);
  assert.equal(raised.decisions.at(-1)!.id, "D3");
  assert.throws(() => approve(raised), /D3 unanswered/);
});

test("pipeline: answering a contradiction adds an intent the team must approve", () => {
  const raised = raiseDecisions(approve(reviewedLogin()), [{ id: "X1", intentIds: ["I5", "I7"], question: "C5 and C7 cannot both hold for both+db-fails. Which gives way?", options: [{ label: "Relax C5", consequence: "the database error wins" }, { label: "Relax C7", consequence: "the 400 wins" }], recommended: 0, origin: "consistency-check" }]);
  const answered = review(raised, "ana").answer("D3", 0).done();
  const it = answered.intents.at(-1)!;
  assert.equal(it.decisionId, "D3");
  assert.equal(it.status, "proposed");
  assert.match(it.statement, /Relax C5 \(the database error wins\)/);
  assert.throws(() => approve(answered), /I10 not reviewed/);
  const approved = review(answered, "bruno").approve(it.id).done();
  assert.ok(approve(approved).hash);
});

test("pipeline: failures are structured feedback for whatever writes the code", async () => {
  const lastro = loginPipeline();
  const spec = await lastro.spec(review(reviewedLogin(), "ana").approveAll().done());
  const proof = await lastro.prove(spec, makeLogin("skip-password"));
  assert.equal(proof.ok, false);
  const f = proof.failures.find((x) => x.kind === "clause")!;
  assert.equal(f.input.password, "wrong");
  assert.match(agentFeedback(proof), /counterexample: .*"password":"wrong"/);
  assert.equal(agentFeedback(await lastro.prove(spec, makeLogin())), "Every proof passes.");
});

// ---------------------------------------------------------------------------
// CLI: the same pipeline from any harness, with the agent's own model (handoff)
// ---------------------------------------------------------------------------

import { mkdtempSync, readdirSync, writeFileSync as writeFile, readFileSync as readFile } from "node:fs";
import { tmpdir } from "node:os";
import { join as joinPath } from "node:path";
import { fileURLToPath as toPath } from "node:url";
import { main as cli, section } from "../src/cli.js";

test("cli: plan → review → spec → prove, answering LLM steps by handoff", async () => {
  const dir = mkdtempSync(joinPath(tmpdir(), "lastro-"));
  const ws = ["--dir", joinPath(dir, ".lastro"), "--llm", "handoff"];
  writeFile(joinPath(dir, "model.json"), JSON.stringify(MODEL));
  writeFile(joinPath(dir, "prose.md"), extractLogin().prose);
  const loginModule = joinPath(toPath(new URL(".", import.meta.url)), "..", "examples", "login", "login.js");
  writeFile(joinPath(dir, "impl.mjs"), `import { makeLogin } from ${JSON.stringify(loginModule)};\nexport default makeLogin();\nexport const buggy = makeLogin("skip-password");\n`);

  assert.equal((await cli(["init", "--model", joinPath(dir, "model.json"), ...ws])).code, 0);

  // The agent is asked to extract; it answers with the example's extraction.
  let r = await cli(["plan", joinPath(dir, "prose.md"), ...ws]);
  assert.equal(r.code, 3);
  const ex = extractLogin();
  writeFile(r.body.answer as string, JSON.stringify({
    intents: ex.intents.map((i) => ({ statement: i.statement, kind: i.kind, provability: i.provability, beyondProofReason: i.beyondProofReason ?? "none", quote: i.source.quote })),
    decisions: ex.decisions.map((d) => ({ intentIndexes: d.intentIds.map((x) => Number(x.slice(1)) - 1), question: d.question, options: d.options, recommended: d.recommended })),
    notPromised: ex.notPromised,
  }));
  r = await cli(["plan", joinPath(dir, "prose.md"), ...ws]);
  assert.equal(r.code, 1, "planned, review pending");

  assert.equal((await cli(["spec", ...ws])).body.status, "review-pending");
  await cli(["review", "--as", "ana", "approve-all", ...ws]);
  await cli(["review", "--as", "ana", "amend", "I5", FIXED_I5, ...ws]);
  await cli(["review", "--as", "ana", "answer", "D1", "0", ...ws]);
  await cli(["review", "--as", "ana", "answer", "D2", "0", ...ws]);
  assert.equal((await cli(["status", ...ws])).code, 0);

  r = await cli(["spec", ...ws]);
  assert.equal(r.code, 3);
  const approved = approve(JSON.parse(readFile(joinPath(dir, ".lastro", "intents.json"), "utf8")));
  const plan = deriveLoginPlan(approved);
  writeFile(r.body.answer as string, JSON.stringify({
    clauses: plan.clauses.map((c) => ({ ...c, requires: c.requires ? JSON.stringify(c.requires) : "", ensures: JSON.stringify(c.ensures), assumes: c.assumes ?? [] })),
    assumptions: plan.assumptions,
    scenarios: plan.scenarios.map((s) => ({ ...s, inputJson: JSON.stringify(s.input), expect: JSON.stringify(s.expect) })),
    beyondProof: plan.beyondProof,
  }));
  r = await cli(["spec", ...ws]);
  assert.equal(r.code, 0, JSON.stringify(r.body));

  r = await cli(["prove", joinPath(dir, "impl.mjs"), ...ws]);
  assert.equal(r.code, 0, JSON.stringify(r.body));
  r = await cli(["prove", joinPath(dir, "impl.mjs"), "--export", "buggy", ...ws]);
  assert.equal(r.code, 1);
  assert.match(r.body.feedback as string, /counterexample/);
  assert.equal((await cli(["feedback", ...ws])).code, 1);

  // --stream: start, then one event per clause and scenario (in plan order), then the same result as without it.
  const events: Record<string, unknown>[] = [];
  const streamed = await cli(["prove", "--stream", joinPath(dir, "impl.mjs"), "--export", "buggy", ...ws], (e) => events.push(e));
  assert.equal(streamed.code, 1);
  assert.deepEqual(streamed.body, r.body);
  const kinds = events.map((e) => e.event).filter((k) => k !== "progress");
  assert.deepEqual(kinds, ["start", ...plan.clauses.map(() => "clause"), ...plan.scenarios.map(() => "scenario"), ...(r.body.enginesAgree === null ? [] : ["crosscheck"])]);
  assert.deepEqual(events[0], { event: "start", model: plan.model.name, clauses: plan.clauses.map((c) => c.id), scenarios: plan.scenarios.map((x) => x.id) });
  const violated = events.filter((e) => e.event === "clause" && e.verdict === "violated");
  assert.deepEqual(violated.map((e) => e.clauseId), (r.body.failures as { kind: string; id: string }[]).filter((f) => f.kind === "clause").map((f) => f.id));
  assert.ok(violated.every((e) => e.counterexample), "a violated clause carries its counterexample");
  const runs = events.filter((e) => e.event === "progress").map((e) => e.runs as number);
  assert.deepEqual(runs, runs.map((_, i) => (i + 1) * 100), "a progress event every 100 runs of the code");
});

test("cli: --section reads one Markdown section, up to the next heading of the same level", () => {
  const md = "# Use cases\n\nIntro.\n\n## Review gate\n\nOne rule.\n\n### Detail\n\nStill in.\n\n## Clause verdicts\n\nOther rule.\n";
  assert.equal(section(md, "review gate").trim(), "One rule.\n\n### Detail\n\nStill in.");
  assert.equal(section(md, "Clause verdicts").trim(), "Other rule.");
  assert.throws(() => section(md, "Missing"));
});

test(".lastro/usecases and .lastro/proposals: Lastro's own use cases and proposals are well formed", () => {
  const lastro = joinPath(toPath(new URL(".", import.meta.url)), "..", "..", "..", "..", ".lastro");
  const kebab = /^[a-z0-9]+(-[a-z0-9]+)*$/;
  const ids = new Set<string>();
  const slugs = new Set<string>();
  const files = readdirSync(joinPath(lastro, "usecases")).sort();
  assert.ok(files.length > 0);
  for (const f of files) {
    const u = JSON.parse(readFile(joinPath(lastro, "usecases", f), "utf8")) as Record<string, unknown>;
    assert.deepEqual(Object.keys(u).sort(), ["description", "history", "id", "name", "proofs", "slug"], f);
    assert.match(String(u.id), /^UC-\d{3,}$/, `${f}: bad id`);
    assert.match(String(u.slug), kebab, `${f}: slug is not kebab-case`);
    assert.equal(f, `${u.id}-${u.slug}.json`, `${f}: file name must be <id>-<slug>.json`);
    assert.ok(!ids.has(u.id as string) && !slugs.has(u.slug as string), `${f}: duplicate id or slug`);
    ids.add(u.id as string);
    slugs.add(u.slug as string);
    assert.ok(String(u.name).trim() && String(u.description).trim(), `${f}: needs a name and a description`);
    assert.ok(Array.isArray(u.proofs) && Array.isArray(u.history), `${f}: proofs and history are lists`);
  }
  for (const f of readdirSync(joinPath(lastro, "proposals")).filter((f) => f.endsWith(".json"))) {
    const p = JSON.parse(readFile(joinPath(lastro, "proposals", f), "utf8")) as {
      id: string; slug: string; prose: string; useCases: { useCase: string; change: string }[];
      intents: { id: string; slug: string; name: string; description: string; useCase: string; change: string; source: { quote: string; start: number; end: number } }[];
    };
    assert.equal(f, `${p.id}-${p.slug}.json`, `${f}: file name must be <id>-<slug>.json`);
    for (const i of p.intents) {
      assert.ok(i.id.startsWith(`${p.id}.I-`) && kebab.test(i.slug) && i.name && i.description, `${f}: ${i.id} is incomplete`);
      assert.equal(p.prose.slice(i.source.start, i.source.end), i.source.quote, `${f}: ${i.id} quote is not anchored`);
      const change = p.useCases.find((c) => c.useCase === i.useCase);
      assert.ok(change && change.change === i.change, `${f}: ${i.id} has no matching use-case change`);
      assert.equal(ids.has(i.useCase), i.change !== "add", `${f}: ${i.id} ${i.change} must ${i.change === "add" ? "reserve a new" : "name an existing"} use case`);
    }
  }
});
