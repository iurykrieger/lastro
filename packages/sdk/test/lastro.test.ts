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
  freeze,
  hashOf,
  isIntact,
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
  return freeze(r.done());
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

test("freeze requires every intent reviewed and every decision answered", () => {
  assert.throws(() => freeze(extractLogin()), ArtifactError);
  assert.throws(() => freeze(review(extractLogin()).approveAll().done()), /D1 unanswered/);
  const f = reviewed();
  assert.ok(f.hash?.startsWith("sha256:"));
  assert.ok(isIntact(f));
  const tampered = structuredClone(f);
  tampered.intents[0]!.statement = "something else";
  assert.equal(isIntact(tampered), false);
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
  const a = freeze(review(extractLogin()).approveAll().reject("I5", "out of scope").answer("D1", 0).answer("D2", 0).done());
  const plan = deriveLoginPlan(reviewed());
  assert.throws(() => validatePlan(a, plan), PlanError);
  const own = deriveLoginPlan(a);
  assert.deepEqual(validatePlan(a, own), []);
});

function tinyPlan(clauses: ProofPlan["clauses"], inputs = { x: D.int(0, 9) }): { a: IntentArtifact; plan: ProofPlan } {
  const a = freeze(review(createArtifact("x doubles", [{ statement: "y = 2x", kind: "behavior", provability: "formal", quote: "x doubles" }])).approveAll().done());
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
