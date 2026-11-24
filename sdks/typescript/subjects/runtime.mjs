// The decision cores of P-006 (feature map, state machines, drivers, snapshots, sign-offs) as subjects Lastro can
// prove: each export takes a model's inputs and runs the real SDK (dist/, so build it first with
// `npm run build -w lastro`) to produce the model's outputs. The models are in .lastro/models/.
import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync, copyFileSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { deflateSync } from "node:zlib";
import * as Lastro from "../dist/src/index.js";
import * as Drive from "../dist/src/drive.js";
import { takeSnapshots } from "../dist/src/evidence.js";
import { openWorkspace } from "../dist/src/workspace.js";
import { commandSolver } from "../dist/src/solver.js";

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const PROSE = "Add the shipping rule. Change the checkout. Remove the coupons.";
const refused = (f) => {
  try {
    return [false, f()];
  } catch (e) {
    if (e instanceof Error) return [true, e];
    throw e;
  }
};
const useCase = (id, slug, extra = {}) => ({ id, slug, name: slug, description: `What ${slug} does.`, proofs: [], history: [], ...extra });
const surface = (k, kind = "route") => ({ kind, name: `place ${k}`, locator: `/p${k}` });
const POLICY = { approvals: 1, submitterCanApprove: true };

// ---------------------------------------------------------------------------
// Feature map (I-01 .. I-06)
// ---------------------------------------------------------------------------

// I-01: a feature file may carry surfaces and dependsOn; both are optional.
export function featureSurfaces({ kind, hasName, hasLocator, surfaces, hasDependsOn }) {
  const list = Array.from({ length: surfaces }, (_, k) => ({ kind, ...(hasName ? { name: "the chat list" } : {}), ...(hasLocator ? { locator: `/chats/${k}` } : {}) }));
  const f = useCase("UC-001", "open-chat", { ...(surfaces ? { surfaces: list } : {}), ...(hasDependsOn ? { dependsOn: ["UC-002"] } : {}) });
  const [rejected, map] = refused(() => Lastro.featureMap([f, useCase("UC-002", "sign-in")]));
  return { accepted: !rejected, reachable: !rejected && map.features[0].reachable };
}

// I-02: an add takes the proposed surfaces; a modify keeps the existing ones and only adds or removes what it names.
export function extractSurfaces({ change, existing, adds, removes, proposedOnAdd }) {
  const cases = [useCase("UC-001", "checkout", existing ? { surfaces: Array.from({ length: existing }, (_, k) => surface(`e${k}`)) } : {})];
  const isAdd = change === "add";
  const links = isAdd
    ? { addSurfaces: Array.from({ length: proposedOnAdd }, (_, k) => surface(`n${k}`)), removeSurfaces: [], addDependsOn: [], removeDependsOn: [] }
    : { addSurfaces: Array.from({ length: adds }, (_, k) => surface(`n${k}`)), removeSurfaces: Array.from({ length: removes }, (_, k) => `/pe${k}`), addDependsOn: [], removeDependsOn: [] };
  const slug = isAdd ? "new-rule" : "checkout";
  const out = {
    name: "Change", slug: "change", description: "A change.", decisions: [], notPromised: [],
    intents: [{ slug: "the-change", name: "The change", description: "It changes.", change, useCase: slug, provability: "formal", beyondProofReason: "none", quote: "Change the checkout.", replaces: [] }],
    useCases: [{ slug, change, name: "Checkout", after: "The new checkout.", links }],
  };
  const [rejected, p] = refused(() => Lastro.proposalFromExtraction({ id: "P-001", prose: PROSE, submittedBy: "ana", useCases: cases, policy: POLICY }, out));
  return { accepted: !rejected, after: rejected ? 0 : (p.useCases[0].surfaces ?? []).length };
}

// I-03, I-04, I-05: the map, its errors and the affected features (A, B, C: ab = A depends on B).
export function featureMap({ ab, bc, ca, aUnknown, bRemoved, changed, aSurface }) {
  const ids = { a: "UC-001", b: "UC-002", c: "UC-003" };
  const features = [
    useCase("UC-001", "a", { dependsOn: [...(ab ? ["UC-002"] : []), ...(aUnknown ? ["UC-099"] : [])], ...(aSurface ? { surfaces: [surface("a")] } : {}) }),
    useCase("UC-002", "b", { dependsOn: bc ? ["UC-003"] : [] }),
    useCase("UC-003", "c", { dependsOn: ca ? ["UC-001"] : [] }),
  ];
  const removed = bRemoved ? ["UC-002"] : [];
  const [rejected, r] = refused(() => Lastro.featureMap(features, removed));
  const again = refused(() => Lastro.featureMap([...features].reverse(), removed));
  const same = rejected ? again[0] && again[1].message === r.message : !again[0] && Lastro.renderFeatureMap(again[1]) === Lastro.renderFeatureMap(r);
  if (rejected) return { ok: false, cycle: /dependency cycle/.test(r.message), named: Math.min(3, r.cycle?.length ?? 0), affectedA: false, affectedB: false, affectedC: false, aReachable: false, same };
  const affected = changed === "none" ? [] : Lastro.affectedBy(r, [ids[changed]]);
  return {
    ok: true, cycle: false, named: 0, same,
    affectedA: affected.includes("UC-001"), affectedB: affected.includes("UC-002"), affectedC: affected.includes("UC-003"),
    aReachable: r.features.find((f) => f.id === "UC-001").reachable,
  };
}

// I-06: bootstrap --surfaces proposes for features without surfaces, only adds, and is reviewed like any proposal.
export function bootstrapSurfaces({ hasSurfaces, touchesDescription, addsSurface }) {
  const cases = [useCase("UC-001", "checkout", hasSurfaces ? { surfaces: [surface("x")] } : {}), useCase("UC-002", "coupons")];
  const digest = { head: null, hash: "sha256:x", scope: [], files: [{ path: "src/coupons.ts", role: "source", lines: 1, symbols: [] }], tests: [], commits: [], manifests: [], areas: [], totals: {}, truncated: {} };
  const out = { features: [{ slug: "coupons", surfaces: addsSurface ? [surface("c")] : [], dependsOn: [], files: ["src/coupons.ts"], ...(touchesDescription ? { description: "Coupons do something else." } : {}) }] };
  const [rejected, p] = refused(() => Lastro.proposalFromSurfaces({ id: "P-002", submittedBy: "ana", useCases: cases, policy: POLICY, digest }, out));
  return {
    proposed: Lastro.featuresWithoutSurfaces(cases).some((u) => u.id === "UC-001"),
    accepted: !rejected,
    descriptionKept: rejected || p.useCases.every((c) => c.after === c.before),
    reviewed: rejected || (p.status === "open" && !p.hash),
  };
}

// ---------------------------------------------------------------------------
// Formal coverage (I-08)
// ---------------------------------------------------------------------------

export function formalCoverage({ formal, evidenced, beyond }) {
  const intents = [];
  const proofs = [];
  const outcomes = [];
  const add = (n, kind) => {
    for (let k = 0; k < n; k++) {
      const id = `P-001.I-${String(intents.length + 1).padStart(2, "0")}`;
      intents.push({ id, slug: `i${intents.length + 1}`, name: id, description: id, change: "add", useCase: "UC-001", provability: kind === "beyond" ? "beyond-proof" : "formal", ...(kind === "beyond" ? { beyondProofReason: "human-judgment" } : {}), source: { quote: "", start: 0, end: 0 }, replaces: [], status: "approved", approvals: ["ana"] });
      if (kind === "beyond") continue;
      const pid = `P-001.PF-${String(proofs.length + 1).padStart(2, "0")}`;
      proofs.push({ id: pid, slug: `p${proofs.length + 1}`, name: pid, description: pid, intent: id, useCase: "UC-001", model: "toy", subject: "toy", check: { kind: "clause", ensures: { const: true }, assumptions: [] }, status: "passed" });
      outcomes.push({ proof: pid, useCase: "UC-001", intent: id, role: "proposal", verdict: kind === "formal" ? "proved" : "evidenced", passed: true, ...(kind === "formal" ? {} : { note: "sampled: more than 100,000 points" }) });
    }
  };
  add(formal, "formal");
  add(evidenced, "evidenced");
  add(beyond, "beyond");
  const p = { intents, proofs, lastRun: { ok: true, outcomes, regressions: [], replacedStillPass: [], unproved: [] } };
  const c = Lastro.formalCoverage(p, p.lastRun);
  return { countFormal: c.formal.length, countEvidenced: c.evidenced.length, countBeyond: c.beyond.length, reasons: [...c.evidenced, ...c.beyond].filter((x) => x.reason.trim()).length };
}

// ---------------------------------------------------------------------------
// State machines (I-09 .. I-16)
// ---------------------------------------------------------------------------

// I-09: a state-machine model is accepted when it is finite.
export function smModel({ stateFinite, initialInDomain, eventsFinite, actors }) {
  const m = {
    name: "door", kind: "machine",
    state: { open: { type: "bool" }, count: stateFinite ? { type: "int", min: 0, max: 3 } : { type: "int", min: 0 } },
    initial: { open: false, count: initialInDomain ? 0 : 7 },
    events: { toggle: {}, label: eventsFinite ? { color: { type: "enum", values: ["red", "blue"] } } : { text: { type: "string" } } },
    ...(actors ? { actors, local: { busy: { type: "bool" } } } : {}),
  };
  if (actors) m.initial.busy = false;
  return { accepted: Lastro.machineProblems(m).length === 0 };
}

const COUNTER = (actors) => ({
  name: "counter", kind: "machine", actors,
  state: { n: { type: "int", min: 0, max: 3 } },
  local: { pc: { type: "enum", values: ["idle", "read", "done"] }, t: { type: "int", min: 0, max: 3 } },
  initial: { n: 0, pc: "idle", t: 0 },
  events: { read: {}, write: {} },
});
const counterStep = (atomic) => (x) => {
  if (x.event === "read" && x.pc === "idle") return atomic ? { n: x.n + 1, pc: "done" } : { t: x.n, pc: "read" };
  if (x.event === "write" && x.pc === "read") return { n: x.t + 1, pc: "done" };
  return null;
};
/** How many actors have finished: the counter must equal it in every state. */
const doneCount = (actors) =>
  Array.from({ length: actors }, (_, k) => ({ op: "ite", args: [{ op: "eq", args: [{ var: `in.pc_${k + 1}` }, { const: "done" }] }, { const: 1 }, { const: 0 }] })).reduce((a, b) => ({ op: "add", args: [a, b] }));

// I-10: the actors' events interleave in every order, so the lost update between two of them is found.
export async function smInterleave({ actors, atomic }) {
  const inv = { id: "C1", kind: "invariant", holds: { op: "eq", args: [{ var: "in.n" }, doneCount(actors)] } };
  const r = (await Lastro.exploreMachine(COUNTER(actors), counterStep(atomic), [inv])).clauses[0];
  return { violated: r.verdict === "violated", cexActors: new Set((r.path ?? []).map((e) => e.actor)).size };
}

// I-15: the statechart has one region per actor, labelled transitions, forbidden states and the counterexample path.
export async function statechart({ actors, forbids, cex }) {
  const m = { name: "switch", kind: "machine", actors, state: {}, local: { on: { type: "bool" } }, initial: { on: false }, events: { flip: {} } };
  const step = (x) => ({ on: !x.on });
  const inv = { id: "C1", kind: "invariant", holds: forbids ? { op: "not", args: [{ var: "in.on_1" }] } : { const: true } };
  const path = cex ? [{ actor: 1, event: "flip", input: {} }] : [];
  const c = await Lastro.statechart(m, step, [inv], path);
  const ts = c.regions.flatMap((r) => r.transitions);
  return {
    regions: c.regions.length,
    marked: c.regions.some((r) => r.states.some((s) => s.forbidden)),
    pathDrawn: ts.some((t) => t.onPath),
    labelled: ts.length > 0 && ts.every((t) => t.event === "flip"),
  };
}

// ---------------------------------------------------------------------------
// Formal first, and SMT for spaces too large to enumerate (I-07, I-11 .. I-14)
// ---------------------------------------------------------------------------

const z3 = commandSolver();
/** A solver that cannot decide anything: what Lastro does when the real one gives up. */
const undecided = async () => ({ answer: "unknown", values: {} });

// I-07: exhaustive when the space is small; SMT on the model's definition when it is large; sampling, with the
// reason, when the solver cannot decide or the model cannot express the clause.
export async function formalFirst({ space, smt, holds }) {
  const v = (n) => ({ var: n });
  const k = (x) => ({ const: x });
  const op = (o, ...args) => ({ op: o, args });
  const max = space === "small" ? 999 : 1_000_000;
  // The clause: from 100 on, shipping is free. A broken fee charges 15 at exactly one subtotal, which sampling misses.
  const bad = space === "small" ? 500 : 777_777;
  const fee = (s) => (s < 100 || (!holds && s === bad) ? 15 : 0);
  const model = {
    name: "fee",
    inputs: { subtotal: { type: "int", min: 0, max } },
    outputs: { fee: { type: "int", min: 0, max: 15 } },
    ...(smt === "inexpressible" ? {} : { defines: { fee: op("ite", op("or", op("lt", v("in.subtotal"), k(100)), ...(holds ? [] : [op("eq", v("in.subtotal"), k(bad))])), k(15), k(0)) } }),
  };
  const a = { lastro: "intent-artifact/v0", prose: "", intents: [{ id: "C1", statement: "C1", kind: "behavior", provability: "formal", source: { quote: "", start: 0, end: 0 }, status: "approved" }], decisions: [], notPromised: [] };
  const artifact = { ...a, hash: Lastro.hashOf(a) };
  const plan = { artifactHash: artifact.hash, model, clauses: [{ id: "C1", intentIds: ["C1"], description: "free from 100", requires: op("ge", v("in.subtotal"), k(100)), ensures: op("eq", v("out.fee"), k(0)) }], scenarios: [], assumptions: [], beyondProof: [] };
  const r = (await Lastro.verify(artifact, plan, { subject: (x) => ({ fee: fee(x.subtotal) }), smt: smt === "unknown" ? undecided : z3, mutation: false })).clauses[0];
  const reason = /solver-unknown/.test(r.note ?? "") ? "solver-unknown" : /not-expressible/.test(r.note ?? "") ? "not-expressible" : r.method === "sampling" ? "too-large" : "none";
  return { method: { exhaustive: "exhaustive", smt: "smt", sampling: "sampled" }[r.method] ?? "sampled", verdict: r.verdict, reason };
}

/** Two actors and a counter: atomic when the clause should hold, the lost update otherwise. */
function counterModel(atomic) {
  const v = (n) => ({ var: n });
  const k = (x) => ({ const: x });
  const op = (o, ...args) => ({ op: o, args });
  return {
    name: "counter", kind: "machine", actors: 2,
    state: { n: { type: "int", min: 0, max: 9 } },
    local: { pc: { type: "enum", values: ["idle", "read", "done"] }, t: { type: "int", min: 0, max: 9 } },
    initial: { n: 0, pc: "idle", t: 0 },
    events: { read: {}, write: {} },
    transitions: atomic
      ? { read: { when: op("eq", v("in.pc"), k("idle")), next: { n: op("add", v("in.n"), k(1)), pc: k("done") } }, write: { when: k(false), next: {} } }
      : { read: { when: op("eq", v("in.pc"), k("idle")), next: { t: v("in.n"), pc: k("read") } }, write: { when: op("eq", v("in.pc"), k("read")), next: { n: op("add", v("in.t"), k(1)), pc: k("done") } } },
  };
}

// I-11 .. I-14: each kind of clause, over a space visited breadth first or (large) proved by SMT, else walked.
export async function smClauses({ clause, holds, space, smt }) {
  const v = (n) => ({ var: n });
  const k = (x) => ({ const: x });
  const op = (o, ...args) => ({ op: o, args });
  const m = counterModel(holds);
  const step = (x) => {
    if (x.event === "read" && x.pc === "idle") return holds ? { n: x.n + 1, pc: "done" } : { t: x.n, pc: "read" };
    if (!holds && x.event === "write" && x.pc === "read") return { n: x.t + 1, pc: "done" };
    return null;
  };
  const finished = (a) => op("ite", op("eq", v(`in.pc_${a}`), k("done")), k(1), k(0));
  const c =
    clause === "invariant"
      ? { id: "C1", kind: "invariant", holds: op("eq", v("in.n"), op("add", finished(1), finished(2))) }
      : clause === "step"
        ? { id: "C1", kind: "step", requires: op("eq", v("in.event"), k("read")), ensures: op("eq", v("out.n"), op("add", v("in.n"), k(1))) }
        : { id: "C1", kind: "reach", condition: op("eq", v("in.n"), k(holds ? 2 : 9)) };
  // "large": more reachable states than the limit allows, so the exploration cannot finish.
  const opts = space === "small" ? {} : { limit: 2, conformanceLimit: 100_000, smt: smt === "proves" ? z3 : undecided };
  const report = await Lastro.exploreMachine(m, step, [c], opts);
  const r = report.clauses[0];
  // The shortest violations, found independently: the lost update needs both reads before both writes; a read that
  // does not raise the counter breaks the step clause at once.
  const shortest = r.path ? r.path.length === (clause === "invariant" ? 4 : 1) : false;
  return {
    verdict: r.verdict,
    evidence: r.evidence === "none" ? "empirical" : r.evidence,
    method: r.method,
    states: Math.min(100_000, report.states),
    shortest,
    actorsNamed: !!r.path && r.path.length > 0 && r.path.every((e) => Number.isInteger(e.actor) && e.actor >= 1),
  };
}

// ---------------------------------------------------------------------------
// Drivers and driven scenarios (I-17 .. I-20, I-38)
// ---------------------------------------------------------------------------

// I-17: the protocol each surface is driven over.
export function driverProtocol({ target, config }) {
  const kind = { web: "screen", electron: "screen", android: "screen", ios: "screen", api: "api", command: "command" }[target];
  const standard = {
    web: { screen: { webdriver: "http://localhost:4444", capabilities: { browserName: "chrome" } } },
    electron: { screen: { webdriver: "http://localhost:9515", capabilities: { "goog:chromeOptions": { binary: "/app/electron" } } } },
    android: { screen: { appium: "http://localhost:4723", capabilities: { platformName: "Android" } } },
    ios: { screen: { appium: "http://localhost:4723", capabilities: { platformName: "iOS" } } },
    api: { api: { baseUrl: "http://localhost:3000" } },
    command: { command: { process: true } },
  }[target];
  const drivers = config === "standard" ? standard : config === "jsonlines" ? { [kind]: { jsonlines: "node driver.mjs" } } : undefined;
  return { protocol: Drive.protocolFor(kind, drivers) };
}

// I-18: no SDK depends on a browser or a device, and the docs show each driver setup.
const BROWSERS = /puppeteer|playwright|selenium|webdriverio|chromedp|appium|Selenium\.WebDriver|Microsoft\.Playwright/i;
export function shippedDrivers({ sdk, setup }) {
  const manifest = { typescript: "sdks/typescript/package.json", go: "sdks/go/go.mod", dotnet: "sdks/dotnet/src/Lastro/Lastro.csproj" }[sdk];
  const deps = (() => {
    const text = readFileSync(join(ROOT, manifest), "utf8");
    if (sdk !== "typescript") return text;
    const pkg = JSON.parse(text);
    return JSON.stringify({ ...pkg.dependencies, ...pkg.peerDependencies, ...pkg.optionalDependencies });
  })();
  const page = join(ROOT, "apps/docs/src/pages/runtime.tsx");
  const heading = { web: "data-setup=\"web\"", appium: "data-setup=\"appium\"", http: "data-setup=\"http\"", jsonlines: "data-setup=\"jsonlines\"" }[setup];
  return { bundlesBrowser: BROWSERS.test(deps), documented: existsSync(page) && readFileSync(page, "utf8").includes(heading) };
}

/** A PNG of `width` x 1 pixels, the first `changed` of them red and the rest white. */
function png(width, changed) {
  const crcTable = Array.from({ length: 256 }, (_, n) => {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    return c >>> 0;
  });
  const crc = (buf) => {
    let c = 0xffffffff;
    for (const b of buf) c = crcTable[(c ^ b) & 0xff] ^ (c >>> 8);
    return (c ^ 0xffffffff) >>> 0;
  };
  const chunk = (type, data) => {
    const len = Buffer.alloc(4);
    len.writeUInt32BE(data.length);
    const td = Buffer.concat([Buffer.from(type, "latin1"), data]);
    const c = Buffer.alloc(4);
    c.writeUInt32BE(crc(td));
    return Buffer.concat([len, td, c]);
  };
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(width, 0);
  ihdr.writeUInt32BE(1, 4);
  ihdr[8] = 8;
  ihdr[9] = 2;
  const row = Buffer.alloc(1 + width * 3, 255);
  row[0] = 0;
  for (let k = 0; k < changed; k++) row.set([255, 0, 0], 1 + k * 3);
  return new Uint8Array(Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk("IHDR", ihdr), chunk("IDAT", deflateSync(row)), chunk("IEND", Buffer.alloc(0))]));
}

const FAIL_DRIVER = "node -e \"process.exit(1)\"";
const HANG_DRIVER = "node -e \"process.stdin.resume(); setInterval(() => {}, 1000)\"";

// I-19, I-20: a driven step passes when its output matches and its screenshot stays within tolerance; a driver that
// fails to start or times out fails the scenario with its error.
export async function drivenScenario({ step, matches, pixelShare, tolerance, hasReference, driver }) {
  const kind = { screen: "screen", api: "api", function: "command" }[step];
  const scenario = {
    id: "S1", kind, locator: "/chats", tolerance: tolerance / 10,
    steps: [{ input: { click: "#send" }, expect: { text: "sent" }, ...(hasReference ? { reference: png(10, 0) } : {}) }],
  };
  if (driver !== "ok") {
    const drivers = { [kind]: { jsonlines: driver === "timeout" ? HANG_DRIVER : FAIL_DRIVER }, timeoutMs: 300 };
    const r = await Drive.runDrivenScenario(scenario, drivers);
    return { passed: r.passed, driverError: !!r.driverError, evidence: r.evidence };
  }
  const open = async () => ({
    act: async () => ({ text: matches ? "sent" : "lost" }),
    screenshot: async () => png(10, pixelShare),
    close: async () => undefined,
  });
  const r = await Drive.runDrivenScenario(scenario, {}, open);
  return { passed: r.passed, driverError: !!r.driverError, evidence: r.evidence };
}

const drivenProof = (reference) => ({
  id: "P-001.PF-01", slug: "send-a-message", name: "Send", description: "Send", intent: "P-001.I-01", useCase: "UC-001", model: "", subject: "",
  check: { kind: "driven", surface: { kind: "screen", locator: "/chats" }, steps: [{ input: { click: "#send" }, expect: { text: "sent" }, ...(reference ? { reference } : {}) }] },
  status: "passed",
});

// I-38: a signed-off after screenshot becomes that step's reference.
export function referenceScreenshot({ signedOff, hadReference }) {
  const after = "docs/P-001/UC-001/I-01/after/send-a-message-1.png";
  const out = {
    name: "Chat", slug: "chat", description: "Chat.", decisions: [], notPromised: [],
    intents: [{ slug: "send", name: "Send", description: "Send a message.", change: "add", useCase: "chat", provability: "empirical", beyondProofReason: "none", quote: "Add the shipping rule.", replaces: [] }],
    useCases: [{ slug: "chat", change: "add", name: "Chat", after: "Chat.", links: { addSurfaces: [], removeSurfaces: [], addDependsOn: [], removeDependsOn: [] } }],
  };
  let p = Lastro.proposalFromExtraction({ id: "P-001", prose: PROSE, submittedBy: "ana", useCases: [], policy: POLICY }, out);
  p = Lastro.approveProposal(Lastro.reviewProposal(p, "ana").approveAll().done(), []);
  p.proofs = [drivenProof(hadReference ? "docs/P-001/UC-001/I-01/reference/old-1.png" : undefined)];
  p.lastRun = { ok: true, outcomes: [], regressions: [], replacedStillPass: [], unproved: [] };
  p.snapshots = [{ intent: "P-001.I-01", feature: "UC-001", kind: "screen", side: "after", path: after, sha256: "x" }];
  if (signedOff) p = Lastro.reviewProposal(p, "bruno").reference(after).done();
  return { referenceIsAfter: p.proofs[0].check.steps[0].reference === after.replace("/after/", "/reference/") };
}

// ---------------------------------------------------------------------------
// Properties over action sequences (I-21 .. I-23)
// ---------------------------------------------------------------------------

// The application is a counter that "inc" raises; the property is that it never passes 3. A faulty application
// overshoots once it reaches 2 (always, or only the first time when it is flaky).
function counterApp(faulty, flaky) {
  let runs = 0;
  return async () => {
    runs++;
    let n = 0;
    const misbehaves = faulty && (!flaky || runs === 1);
    return {
      act: async (a) => {
        if (a.op === "inc") n = n >= 3 ? 3 : n + 1 + (misbehaves && n === 2 ? 5 : 0);
        if (a.op === "reset") n = 0;
        return { n };
      },
      screenshot: async () => undefined,
      close: async () => undefined,
    };
  };
}
const ACTIONS = [{ op: "inc" }, { op: "reset" }];

export async function actionProperty({ hasModel, modelHolds, sequencesHold, replayFails }) {
  const open = counterApp(!sequencesHold, !replayFails);
  const base = { id: "PR1", kind: "screen", locator: "/counter", sequences: 20, length: 8, seed: 7 };
  const sameSequences = JSON.stringify(Drive.actionSequences(ACTIONS, 20, 8, 7)) === JSON.stringify(Drive.actionSequences(ACTIONS, 20, 8, 7));
  let r;
  if (hasModel) {
    const model = { name: "counter", kind: "machine", state: { n: { type: "int", min: 0, max: 9 } }, initial: { n: 0 }, events: { inc: {}, reset: {} } };
    // A model that holds caps at 3; one that does not lets the counter reach 4.
    const step = (x) => (x.event === "reset" ? { n: 0 } : { n: Math.min(modelHolds ? 3 : 4, x.n + 1) });
    r = await Drive.checkModelledProperty(
      { ...base, model, step, invariant: { id: "C1", kind: "invariant", holds: { op: "le", args: [{ var: "in.n" }, { const: 3 }] } }, actions: [{ action: ACTIONS[0], event: "inc" }, { action: ACTIONS[1], event: "reset" }] },
      {},
      open,
    );
  } else r = await Drive.checkProperty({ ...base, actions: ACTIONS, invariant: (o) => o.n <= 3 }, {}, open);
  return { verdict: r.verdict, evidence: r.evidence, bound: r.failureRateUpperBound95 !== undefined, sameSequences };
}

// I-23: a sequence that first fails at action k is cut to its first k actions.
export async function shrinkSequence({ length, failAt }) {
  const open = async () => {
    let k = 0;
    return { act: async () => ({ ok: ++k < failAt }), screenshot: async () => undefined, close: async () => undefined };
  };
  const r = await Drive.checkProperty({ id: "PR1", kind: "screen", locator: "/x", actions: [{ op: "tap" }], invariant: (o) => o.ok, sequences: 1, length }, {}, open);
  return { prefix: r.prefix ? r.prefix.length : 0 };
}

// ---------------------------------------------------------------------------
// Snapshots, unexpected changes and sign-offs (I-24 .. I-35, I-37)
// ---------------------------------------------------------------------------

// I-24 .. I-27: the kinds of snapshot a feature gets and the side each moment takes.
export function snapshotKinds({ surface: kind, hasModel, hasData, phase }) {
  const kinds = Lastro.snapshotKinds({ surfaces: kind === "none" ? [] : [{ kind, name: "it", locator: "/it" }], hasModel, hasData });
  return { screen: kinds.includes("screen"), contract: kinds.includes("contract"), data: kinds.includes("data"), examples: kinds.includes("examples"), side: Lastro.sideOf(phase) };
}

// I-28: the structural diff of a contract or a data structure.
export function structuralDiff({ before, after }) {
  const flat = (t) => (t === "absent" ? {} : { "body.total": t });
  const d = Lastro.structuralDiff(flat(before), flat(after));
  return { change: d.length ? d[0].change : "none", breaking: d.some((x) => x.breaking) };
}

// I-29, I-30: the examples, the changed rows and the unexpected changes, which block until accepted.
export function examplesCompare({ points, changed, clauseApplies, acceptances, required }) {
  const model = { name: "fee", inputs: { x: { type: "int", min: 0, max: points === "small" ? 999 : 1000 } }, outputs: { y: { type: "int" } } };
  const { inputs, wholeSpace } = Lastro.exampleInputs(model);
  const row = { input: inputs[5], before: { y: 1 }, after: { y: changed ? 2 : 1 } };
  const rows = [{ input: inputs[0], before: { y: 0 }, after: { y: 0 } }, row];
  const listed = Lastro.changedRows(rows).some((r) => r.input === row.input);
  const clause = clauseApplies ? { requires: { op: "eq", args: [{ var: "in.x" }, { const: row.input.x }] } } : { requires: { op: "lt", args: [{ var: "in.x" }, { const: 0 }] } };
  const unexpected = Lastro.unexpectedChanges(rows, [clause], "P-001.I-01");
  unexpected.forEach((u) => (u.acceptedBy = ["ana", "bruno", "carla"].slice(0, acceptances)));
  const due = Lastro.reviewDue([], [], unexpected, required);
  return { wholeSpace, listed, unexpected: unexpected.length > 0, blocks: due.unexpected.length > 0 };
}

// I-31: snapshots land under .lastro/docs/<proposal>/<feature>/<intent>/<side>/ (or repository/), are not ignored by
// git, and the proposal records each file's SHA-256.
export async function snapshotFiles({ scope, side, file }) {
  const dir = mkdtempSync(join(tmpdir(), "lastro-snap-"));
  try {
    execFileSync("git", ["init", "-q", dir]);
    copyFileSync(join(ROOT, ".gitignore"), join(dir, ".gitignore"));
    const ws = openWorkspace(join(dir, ".lastro"));
    mkdirSync(ws.dir, { recursive: true });
    const u = useCase("UC-001", "chat", { surfaces: [{ kind: "screen", name: "chat", locator: "/chat" }, { kind: "api", name: "chat api", locator: "/api/chat" }] });
    u.proofs = [
      { ...drivenProof(), status: "passed", intent: undefined, useCase: "UC-001" },
      { ...drivenProof(), id: "UC-001.PF-02", slug: "chat-api", check: { kind: "driven", surface: { kind: "api", locator: "/api/chat" }, steps: [{ input: { method: "GET", path: "/api/chat" }, expect: {} }] }, intent: undefined },
    ];
    const p = {
      id: "P-001", intents: [{ id: "P-001.I-01", useCase: "UC-001", status: "approved" }], useCases: [{ useCase: "UC-001", change: "modify", intents: ["P-001.I-01"] }], proofs: [],
    };
    const open = async (kind) => ({ act: async () => (kind === "api" ? { status: 200, body: { id: 1 } } : { text: "ok" }), screenshot: async () => png(4, 1), close: async () => undefined });
    const drivers = scope === "repository" ? { data: { command: "node -e \"console.log(JSON.stringify({tables:{chats:{id:'int'}}}))\"" } } : undefined;
    const q = await takeSnapshots(p, side, { ws, models: {}, subjects: {}, useCases: [u], open, ...(drivers ? { drivers } : {}) });
    const ext = file === "png" ? ".png" : ".json";
    const recs = (q.snapshots ?? []).filter((s) => s.side === side && s.path.endsWith(ext) && (scope === "repository" ? s.intent === "repository" : s.intent !== "repository"));
    // The repository has no screen, so it has no PNG: nothing there can break the layout or the hashes.
    const expected = !(scope === "repository" && file === "png");
    if (!recs.length) return expected ? { layoutOk: false, ignored: true, hashRecorded: false, hashMatches: false } : { layoutOk: true, ignored: false, hashRecorded: true, hashMatches: true };
    const pattern = scope === "repository" ? new RegExp(`^docs/P-001/repository/${side}/[^/]+$`) : new RegExp(`^docs/P-001/UC-001/I-01/${side}/[^/]+$`);
    let ignored = false;
    for (const s of recs) {
      try {
        execFileSync("git", ["-C", dir, "check-ignore", "-q", join(".lastro", s.path)]);
        ignored = true;
      } catch {
        /* exit 1: not ignored */
      }
    }
    const sha = (path) => execFileSync("sha256sum", [path], { encoding: "utf8" }).split(" ")[0];
    return {
      layoutOk: recs.every((s) => pattern.test(s.path)),
      ignored,
      hashRecorded: recs.every((s) => /^[0-9a-f]{64}$/.test(s.sha256)),
      hashMatches: recs.every((s) => sha(ws.file(s.path)) === s.sha256),
    };
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

// I-33: after a passing run the proposal is proved only once the sign-offs and acceptances are in.
export function provedGate({ runOk, allPassed, judgment, signoffs, changesRequested, unexpected, acceptances, required }) {
  const out = {
    name: "Chat", slug: "chat", description: "Chat.", decisions: [], notPromised: [],
    intents: [
      { slug: "send", name: "Send", description: "Send a message.", change: "add", useCase: "chat", provability: "formal", beyondProofReason: "none", quote: "Add the shipping rule.", replaces: [] },
      ...(judgment ? [{ slug: "looks-right", name: "Looks right", description: "The message bubble looks right.", change: "add", useCase: "chat", provability: "beyond-proof", beyondProofReason: "human-judgment", quote: "Change the checkout.", replaces: [] }] : []),
    ],
    useCases: [{ slug: "chat", change: "add", name: "Chat", after: "Chat.", links: { addSurfaces: [], removeSurfaces: [], addDependsOn: [], removeDependsOn: [] } }],
  };
  let p = Lastro.proposalFromExtraction({ id: "P-001", prose: PROSE, submittedBy: "ana", useCases: [], policy: { approvals: required, submitterCanApprove: true } }, out);
  for (const who of ["ana", "bruno", "carla"].slice(0, required)) p = Lastro.reviewProposal(p, who).approveAll().done();
  p = Lastro.approveProposal(p, []);
  p.proofs = [{ ...drivenProof(), check: { kind: "clause", ensures: { const: true }, assumptions: [] }, status: allPassed ? "passed" : "failed" }];
  p.lastRun = { ok: runOk, outcomes: [], regressions: [], replacedStillPass: [], unproved: [] };
  if (unexpected) p.unexpected = [{ id: "U1", intent: "P-001.I-01", input: { x: 1 }, before: { y: 1 }, after: { y: 2 }, acceptedBy: [] }];
  let r = Lastro.reviewProposal(p, "ana");
  for (const who of ["ana", "bruno", "carla"].slice(0, acceptances)) if (unexpected) r = Lastro.reviewProposal(r.done(), who).accept("U1");
  p = r.done();
  if (judgment) {
    for (const who of ["ana", "bruno", "carla"].slice(0, signoffs)) p = Lastro.reviewProposal(p, who).signOff("I-02").done();
    if (changesRequested) p = Lastro.reviewProposal(p, "davi").requestChanges("I-02", "the bubble is cut").done();
  }
  return { status: Lastro.withStatus(p).status };
}

// I-34: what the review app shows for one intent.
export function reviewEvidence({ phase, kind, machine, violated }) {
  const p = {
    id: "P-001",
    intents: [{ id: "P-001.I-01", useCase: "UC-001", status: "approved" }],
    useCases: [{ useCase: "UC-001", slug: "chat", change: "modify", name: "Chat", intents: ["P-001.I-01"], surfaces: [{ kind: "route", name: "chat", locator: "/chat" }] }],
    proofs: [{ id: "P-001.PF-01", intent: "P-001.I-01", useCase: "UC-001", model: machine ? "door" : "toy", subject: "s", check: machine ? { kind: "machine", clause: "invariant", ensures: { const: true } } : { kind: "clause", ensures: { const: true }, assumptions: [] }, status: "pending" }],
    snapshots: [
      { intent: "P-001.I-01", feature: "UC-001", kind, side: "before", path: `docs/P-001/UC-001/I-01/before/x`, sha256: "a" },
      ...(phase === "proved" ? [{ intent: "P-001.I-01", feature: "UC-001", kind, side: "after", path: `docs/P-001/UC-001/I-01/after/x`, sha256: "b" }] : []),
    ],
    ...(phase === "proved" ? { comparisons: [{ intent: "P-001.I-01", feature: "UC-001", kind, name: "x" }] } : {}),
    // During review a run may already exist (a failed one, before the after snapshots are taken again).
    ...(phase === "proved" || violated
      ? {
          lastRun: { ok: !violated, outcomes: [{ proof: "P-001.PF-01", useCase: "UC-001", intent: "P-001.I-01", role: "proposal", verdict: violated ? "violated" : "proved", passed: !violated, ...(violated ? { counterexample: { input: { x: 1 }, output: { y: 2 } } } : {}) }], regressions: [], replacedStillPass: [], unproved: [] },
        }
      : {}),
    ...(machine ? { statecharts: { door: "stateDiagram-v2\n" } } : {}),
  };
  const e = Lastro.intentEvidence(p, "I-01", [useCase("UC-001", "chat")]);
  return {
    mapShown: !!e.map,
    statechart: e.statecharts.length > 0,
    before: e.before.length > 0,
    after: e.after.length > 0,
    comparison: e.comparisons[0]?.view ?? "none",
    counterexample: e.verdicts.some((v) => v.counterexample),
  };
}

// I-35: the receipt's evidence sections.
export function receiptEvidence({ affected, machine, snapshots, unexpected, signoff }) {
  const out = {
    name: "Chat", slug: "chat", description: "Chat.", decisions: [], notPromised: [],
    intents: [
      { slug: "send", name: "Send", description: "Send a message.", change: "modify", useCase: "chat", provability: "formal", beyondProofReason: "none", quote: "Add the shipping rule.", replaces: [] },
      ...(signoff ? [{ slug: "looks-right", name: "Looks right", description: "It looks right.", change: "modify", useCase: "chat", provability: "beyond-proof", beyondProofReason: "human-judgment", quote: "Change the checkout.", replaces: [] }] : []),
    ],
    useCases: [{ slug: "chat", change: "modify", name: "Chat", after: "Chat, changed.", links: { addSurfaces: [{ kind: "route", name: "chat", locator: "/chat" }], removeSurfaces: [], addDependsOn: [], removeDependsOn: [] } }],
  };
  const cases = [useCase("UC-001", "chat"), useCase("UC-002", "inbox", affected ? { dependsOn: ["UC-001"] } : {})];
  let p = Lastro.proposalFromExtraction({ id: "P-001", prose: PROSE, submittedBy: "ana", useCases: cases, policy: POLICY }, out);
  p = Lastro.approveProposal(Lastro.reviewProposal(p, "ana").approveAll().done(), cases);
  p.proofs = [{ ...drivenProof(), useCase: "UC-001", check: { kind: "clause", ensures: { const: true }, assumptions: [] } }];
  p.lastRun = { ok: true, outcomes: [{ proof: "P-001.PF-01", useCase: "UC-001", intent: "P-001.I-01", role: "proposal", verdict: "proved", passed: true }], regressions: [], replacedStillPass: [], unproved: [] };
  if (machine) p.statecharts = { door: "stateDiagram-v2\n  [*] --> closed\n" };
  if (snapshots) p.snapshots = [{ intent: "P-001.I-01", feature: "UC-001", kind: "examples", side: "after", path: "docs/P-001/UC-001/I-01/after/examples-fee.json", sha256: "ab".repeat(32) }];
  if (unexpected !== "none") p.unexpected = [{ id: "U1", intent: "P-001.I-01", input: { x: 1 }, before: { y: 1 }, after: { y: 2 }, acceptedBy: unexpected === "accepted" ? ["bruno"] : [] }];
  if (signoff) p = Lastro.reviewProposal(p, "bruno").signOff("I-02").done();
  const text = Lastro.renderProposalReceipt(p, cases);
  return {
    mapSection: /## Feature map[\s\S]*Affected: UC-002/.test(text),
    mermaid: /```mermaid\nstateDiagram-v2/.test(text),
    hashes: /`docs\/[^`]+` sha256:[0-9a-f]{64}/.test(text),
    acceptedBy: /accepted by bruno/.test(text),
    signedBy: /signed-off by bruno/.test(text),
  };
}

// ---------------------------------------------------------------------------
// Not implemented yet: these proofs fail until their part of P-006 lands.
// ---------------------------------------------------------------------------

const pending = (what) => () => {
  throw new Error(`not implemented yet: ${what}`);
};
// I-16, I-36: the Rust prover and the Go and .NET ports.
// The other engines, built once per run: the Rust prover (WASM), the Go CLI and the .NET CLI.
const ENGINE_DIR = mkdtempSync(join(tmpdir(), "lastro-engines-"));
let rustEngine;
const engines = {
  async rust(request) {
    rustEngine ??= await Lastro.loadRustProver(readFileSync(join(ROOT, "sdks", "typescript", "wasm", "lastro_core.wasm")));
    return rustEngine.proveMachine(request);
  },
  go(request) {
    const bin = join(ENGINE_DIR, "lastro-go");
    if (!existsSync(bin)) execFileSync("go", ["build", "-o", bin, "./cmd/lastro-go"], { cwd: join(ROOT, "sdks", "go"), stdio: "ignore" });
    return explore([bin], request);
  },
  dotnet(request) {
    const dll = join(ENGINE_DIR, "dotnet", "Lastro.Cli.dll");
    if (!existsSync(dll)) execFileSync("dotnet", ["build", join(ROOT, "sdks", "dotnet", "src", "Lastro.Cli"), "-c", "Release", "-o", join(ENGINE_DIR, "dotnet")], { stdio: "ignore" });
    return explore(["dotnet", dll], request);
  },
};
function explore([cmd, ...args], request) {
  const file = join(ENGINE_DIR, "request.json");
  writeFileSync(file, JSON.stringify(request));
  const body = JSON.parse(execFileSync(cmd === "dotnet" ? "dotnet" : cmd, [...args, "explore", file], { encoding: "utf8" }));
  return { ...body.report, mermaid: body.mermaid };
}

// The machines the engines are compared on: a correct counter, and one bug per kind of clause.
const CROSS_STEPS = {
  none: (x) => (x.event === "read" && x.pc === "idle" ? { n: x.n + 1, pc: "done" } : null),
  invariant: (x) => (x.event === "read" && x.pc === "idle" ? { n: x.n + 2, pc: "done" } : null),
  race: (x) => (x.event === "read" && x.pc === "idle" ? { t: x.n, pc: "read" } : x.event === "write" && x.pc === "read" ? { n: x.t + 1, pc: "done" } : null),
  step: (x) => (x.event === "read" && x.pc === "idle" ? { t: x.n, pc: "read" } : x.event === "write" && x.pc === "read" ? { n: x.n + 1, pc: "idle" } : null),
};
function crossCase(bug) {
  const v = (n) => ({ var: n });
  const k = (x) => ({ const: x });
  const op = (o, ...args) => ({ op: o, args });
  const model = { ...counterModel(true), actors: 2 };
  delete model.transitions;
  const done = (a) => op("ite", op("eq", v(`in.pc_${a}`), k("done")), k(1), k(0));
  const clauses = [
    { id: "C1", kind: "invariant", holds: op("eq", v("in.n"), op("add", done(1), done(2))) },
    { id: "C2", kind: "step", requires: op("eq", v("in.event"), k("write")), ensures: op("eq", v("out.pc"), k("done")) },
    { id: "C3", kind: "reach", condition: op("eq", v("in.n"), k(2)) },
  ];
  return { model, clauses, step: CROSS_STEPS[bug] };
}
async function tsExplore(model, clauses, code) {
  const table = [];
  const step = (x) => {
    const out = code(x);
    table.push({ input: { ...x }, output: out ?? null });
    return out;
  };
  const report = await Lastro.exploreMachine(model, step, clauses);
  const path = report.clauses.find((r) => r.verdict === "violated" && r.path)?.path ?? [];
  const chart = Lastro.mermaidStatechart(await Lastro.statechart(model, step, clauses, path));
  return { report, request: { machine: model, clauses, table, path }, mermaid: chart };
}

// I-16: the Rust prover, the Go SDK and the .NET SDK explore state machines exactly as the TS engine does.
export async function smCrosscheck({ bug, engine }) {
  const { model, clauses, step } = crossCase(bug);
  const ts = await tsExplore(model, clauses, step);
  const other = await engines[engine](ts.request);
  const pathOf = (r) => Lastro.canonicalJson(r?.path ?? null);
  return {
    sameVerdict: ts.report.clauses.every((c) => other.clauses.find((x) => x.clauseId === c.clauseId)?.verdict === c.verdict),
    sameCounterexample: ts.report.clauses.every((c) => pathOf(c) === pathOf(other.clauses.find((x) => x.clauseId === c.clauseId))),
    sameStates: ts.report.states === other.states && (other.missing ?? 0) === 0,
  };
}

// I-36: the Go and .NET SDKs give byte-identical artifacts. Each is compared by the SDK's own cross-check against the
// TS SDK (built from this checkout): the feature map and the structural diff by the map/snapshot helpers check, the
// statechart and the machine verdicts by exploring the same machines, the verdicts and the snapshot hashes by one
// workspace shared by both CLIs (same prove bodies, same receipts with the same SHA-256).
const PARITY_TESTS = {
  go: {
    map: "TestFeatureMapAndSnapshotsMatchTS",
    diff: "TestFeatureMapAndSnapshotsMatchTS",
    statechart: "TestMachinesAreExploredAsTheTSEngineDoes",
    verdict: "TestMachinesAreExploredAsTheTSEngineDoes|TestSharedWorkspaceWithTheTSCLI|TestTypeScriptCrossCheck",
    "snapshot-hash": "TestSharedWorkspaceWithTheTSCLI",
  },
  dotnet: {
    map: "FullyQualifiedName~FeatureMapAndSnapshotHelpersMatchTheTsSdk",
    diff: "FullyQualifiedName~FeatureMapAndSnapshotHelpersMatchTheTsSdk",
    statechart: "FullyQualifiedName~MachinesAreExploredAsTheTsEngineDoes",
    verdict: "FullyQualifiedName~MachinesAreExploredAsTheTsEngineDoes|FullyQualifiedName~OneWorkspaceSharedByTheDotNetAndTypeScriptClis|FullyQualifiedName~TypeScriptAndDotNetAgreeOnHashesVerdictsAndReceipts",
    "snapshot-hash": "FullyQualifiedName~OneWorkspaceSharedByTheDotNetAndTypeScriptClis",
  },
};
const parityRuns = new Map();
export function sdkParity({ sdk, artifact }) {
  const filter = PARITY_TESTS[sdk][artifact];
  const key = `${sdk}:${filter}`;
  if (!parityRuns.has(key)) parityRuns.set(key, runParity(sdk, filter));
  return { same: parityRuns.get(key) };
}
function runParity(sdk, filter) {
  try {
    if (sdk === "go") execFileSync("go", ["test", "-count=1", "-run", filter, "./..."], { cwd: join(ROOT, "sdks", "go"), stdio: "ignore" });
    else execFileSync("dotnet", ["test", join(ROOT, "sdks", "dotnet"), "--filter", filter], { stdio: "ignore", env: { ...process.env, LASTRO_REQUIRE_CROSSCHECK: "1" } });
    return true;
  } catch {
    return false;
  }
}
// I-37: the plugin's review screen.
export async function pluginReviewEvidence({ snapshots, unexpected, signoffDue }) {
  const { renderReview } = await import("../../../plugins/lastro/scripts/lib.mjs");
  const p = {
    id: "P-001", name: "Chat", hash: "sha256:x", policy: { approvals: 1, submitterCanApprove: true }, submittedBy: "ana", decisions: [], threads: [],
    intents: [
      { id: "P-001.I-01", description: "Send a message.", status: "approved", approvals: ["ana"], provability: "formal", source: { quote: "send" } },
      { id: "P-001.I-02", description: "It looks right.", status: "approved", approvals: ["ana"], provability: "beyond-proof", beyondProofReason: "human-judgment", source: { quote: "look" } },
    ],
    useCases: [{ useCase: "UC-001", slug: "chat", change: "modify", name: "Chat", intents: ["P-001.I-01", "P-001.I-02"] }],
    proofs: [],
    lastRun: { ok: true, outcomes: [], regressions: [], replacedStillPass: [], unproved: [] },
    ...(snapshots ? { snapshots: [{ intent: "P-001.I-02", feature: "UC-001", kind: "screen", side: "after", path: "docs/P-001/UC-001/I-02/after/chat-1.png", sha256: "a" }] } : {}),
    ...(unexpected ? { unexpected: [{ id: "U1", intent: "P-001.I-01", input: { x: 1 }, before: { y: 1 }, after: { y: 2 }, acceptedBy: [] }] } : {}),
    signoffs: signoffDue ? [] : [{ intent: "P-001.I-02", by: "bob", verdict: "signed-off" }],
  };
  const text = renderReview({ phase: "passed", proposal: p, useCases: [] });
  return {
    listsPaths: text.includes("docs/P-001/UC-001/I-02/after/chat-1.png"),
    listsUnexpected: /\*\*Unexpected changes\*\*[\s\S]*\*\*U1\*\*/.test(text),
    listsSignoffs: /\*\*Sign-offs due\*\*[\s\S]*\*\*I-02\*\*/.test(text),
  };
}
