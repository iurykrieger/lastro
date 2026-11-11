// Runs the TypeScript SDK on the login example and prints what the .NET SDK must reproduce exactly:
// artifact and plan hashes, verdicts for every planted bug, receipts and LLM handoff ids.
// Usage: node crosscheck.mjs <repo-root>   (needs `npx tsc -p sdks/typescript` first)
import { pathToFileURL } from "node:url";
import { join } from "node:path";

const root = process.argv[2];
const sdk = await import(pathToFileURL(join(root, "sdks/typescript/dist/src/index.js")).href);
const login = await import(pathToFileURL(join(root, "sdks/typescript/dist/examples/login/login.js")).href);
const { review, approve, verify, renderReceipt, sha256Hex, EXTRACT_SYSTEM, EXTRACT_SCHEMA, DERIVE_SYSTEM, DERIVE_SCHEMA, extractUserPrompt, deriveUserPrompt, checkConsistency } = {
  ...sdk,
  sha256Hex: (await import(pathToFileURL(join(root, "sdks/typescript/dist/src/sha256.js")).href)).sha256Hex,
};

const handoffId = (req) => `${req.step}-${sha256Hex(JSON.stringify(req)).slice(0, 12)}`;

function reviewed({ d1 = 0, fixI5 = true } = {}) {
  let r = review(login.extractLogin()).approveAll().answer("D1", d1).answer("D2", 0);
  if (fixI5) r = r.amend("I5", login.FIXED_I5);
  return approve(r.done());
}

const out = { cases: {} };
for (const [name, opts] of Object.entries({ reference: {}, oneWay: { d1: 1 }, misread: { fixI5: false } })) {
  const artifact = reviewed(opts);
  const plan = login.deriveLoginPlan(artifact);
  const bugs = {};
  for (const bug of [undefined, ...login.BUGS.map((b) => b.id)]) {
    const report = await verify(artifact, plan, { subject: login.makeLogin(bug) });
    bugs[bug ?? "none"] = {
      ok: report.ok,
      clauses: Object.fromEntries(report.clauses.map((c) => [c.clauseId, c.verdict])),
      intents: Object.fromEntries(report.intents.map((i) => [i.intentId, i.verdict])),
      summary: report.summary,
      receipt: renderReceipt(report, plan),
    };
  }
  out.cases[name] = { artifact, artifactHash: artifact.hash, planHash: sdk.hashOf(plan), bugs };
}

const approved = reviewed();
out.extractId = handoffId({ step: "extract", system: EXTRACT_SYSTEM, user: extractUserPrompt(login.PROSE), schema: EXTRACT_SCHEMA });
out.deriveId = handoffId({ step: "derive", system: DERIVE_SYSTEM, user: deriveUserPrompt(approved, login.MODEL), schema: DERIVE_SCHEMA });

// Sampling must be bit-identical too: same seed, same samples, same counts.
const big = approve(review(sdk.createArtifact("x doubles", [{ statement: "y = 2x", kind: "behavior", provability: "formal", quote: "x doubles" }])).approveAll().done());
const bigPlan = {
  lastro: "proof-plan/v0",
  artifactHash: big.hash,
  model: { name: "double", inputs: { x: sdk.D.int(0, 1_000_000), s: sdk.D.string("a", "b") }, outputs: { y: sdk.D.int(-1000, 10_000_000) } },
  clauses: [{ id: "C1", intentIds: ["I1"], description: "y = 2x", requires: sdk.L.gt(sdk.L.in("x"), 500_000), ensures: sdk.L.eq(sdk.L.out("y"), { op: "mul", args: [sdk.L.c(2), sdk.L.in("x")] }) }],
  assumptions: [],
  scenarios: [],
  beyondProof: [],
};
const sampled = await verify(big, bigPlan, { subject: (i) => ({ y: 2 * i.x }), samples: 500 });
out.sampling = { checked: sampled.clauses[0].checked, bound: sampled.clauses[0].failureRateUpperBound95, planHash: sdk.hashOf(bigPlan), artifactHash: big.hash };

const contradictionPlan = { ...bigPlan, model: { name: "c", inputs: { x: sdk.D.int(0, 9) }, outputs: { y: sdk.D.int(0, 5) } }, clauses: [
  { id: "C1", intentIds: ["I1"], description: "y is 1 when x=3", requires: sdk.L.eq(sdk.L.in("x"), 3), ensures: sdk.L.eq(sdk.L.out("y"), 1) },
  { id: "C2", intentIds: ["I1"], description: "y is 2 when x=3", requires: sdk.L.eq(sdk.L.in("x"), 3), ensures: sdk.L.eq(sdk.L.out("y"), 2) },
] };
out.contradictions = checkConsistency(contradictionPlan);

process.stdout.write(JSON.stringify(out));
