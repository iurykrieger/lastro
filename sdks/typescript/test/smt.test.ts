import { test } from "node:test";
import assert from "node:assert/strict";
import { exploreMachine, verify, hashOf, type MachineModel, type Expr, type Env, type IntentArtifact, type ProofPlan } from "../src/index.js";
import { commandSolver, solverAvailable } from "../src/solver.js";

// The solver is a separate program: these tests run where `z3` is installed (CI installs it), and skip elsewhere
// unless LASTRO_REQUIRE_SMT is set.
const required = !!process.env.LASTRO_REQUIRE_SMT;
const available = await solverAvailable();
if (required && !available) throw new Error("LASTRO_REQUIRE_SMT is set but `z3 -in -smt2` does not run");
const skip = available ? false : "z3 is not installed";
const smt = commandSolver();

const v = (n: string): Expr => ({ var: n });
const c = (x: string | number | boolean): Expr => ({ const: x });
const op = (o: string, ...args: Expr[]): Expr => ({ op: o, args }) as Expr;

/** A shared counter that actors raise by reading it and then writing it back, plus one (the lost update). */
const counter = (atomic: boolean, actors = 3): MachineModel => ({
  name: "counter",
  kind: "machine",
  actors,
  state: { n: { type: "int", min: 0, max: 9 } },
  local: { pc: { type: "enum", values: ["idle", "read", "done"] }, t: { type: "int", min: 0, max: 9 } },
  initial: { n: 0, pc: "idle", t: 0 },
  events: { read: {}, write: {} },
  transitions: atomic
    ? { read: { when: op("eq", v("in.pc"), c("idle")), next: { n: op("add", v("in.n"), c(1)), pc: c("done") } }, write: { when: c(false), next: {} } }
    : {
        read: { when: op("eq", v("in.pc"), c("idle")), next: { t: v("in.n"), pc: c("read") } },
        write: { when: op("eq", v("in.pc"), c("read")), next: { n: op("add", v("in.t"), c(1)), pc: c("done") } },
      },
});
const code = (atomic: boolean) => (x: Env): Env | null => {
  if (x.event === "read" && x.pc === "idle") return atomic ? { n: (x.n as number) + 1, pc: "done" } : { t: x.n!, pc: "read" };
  if (!atomic && x.event === "write" && x.pc === "read") return { n: (x.t as number) + 1, pc: "done" };
  return null;
};
const finished = (k: number): Expr => op("ite", op("eq", v(`in.pc_${k}`), c("done")), c(1), c(0));
const invariant = { id: "C1", kind: "invariant" as const, holds: op("eq", v("in.n"), op("add", finished(1), finished(2), finished(3))) };

test("a space too large to visit is proved by SMT induction on the model, and the code is checked on every state/event pair", { skip }, async () => {
  // limit 4 states forces the SMT path; the 1,800 state/event pairs are all checked against the model.
  const r = await exploreMachine(counter(true), code(true), [invariant], { limit: 4, conformanceLimit: 100_000, smt });
  assert.equal(r.complete, false);
  assert.equal(r.clauses[0]!.verdict, "proved");
  assert.equal(r.clauses[0]!.method, "smt-induction");
  assert.equal(r.clauses[0]!.evidence, "formal");
  assert.equal(r.clauses[0]!.checked, 1800);
});

test("with too many pairs to check, a model proved by SMT only evidences the code", { skip }, async () => {
  const r = await exploreMachine(counter(true), code(true), [invariant], { limit: 4, conformanceLimit: 100, smt });
  assert.equal(r.clauses[0]!.verdict, "evidenced");
  assert.equal(r.clauses[0]!.method, "smt-induction");
});

test("a code that differs from the proved model is violated at the pair where they differ", { skip }, async () => {
  const buggy = (x: Env): Env | null => (x.event === "read" && x.pc === "idle" ? { n: Math.min(9, (x.n as number) + (x.n === 5 ? 2 : 1)), pc: "done" } : null);
  const r = await exploreMachine(counter(true), buggy, [invariant], { limit: 4, conformanceLimit: 100_000, smt });
  assert.equal(r.clauses[0]!.verdict, "violated");
  assert.match(r.clauses[0]!.note ?? "", /differs from it/);
});

test("a race comes back as the canonical shortest path, the one breadth-first search finds", { skip }, async () => {
  const small = await exploreMachine(counter(false), code(false), [invariant]);
  const large = await exploreMachine(counter(false), code(false), [invariant], { limit: 4, smt });
  assert.equal(large.clauses[0]!.verdict, "violated");
  assert.equal(large.clauses[0]!.method, "smt-induction");
  assert.deepEqual(large.clauses[0]!.path, small.clauses[0]!.path);
  assert.deepEqual(large.clauses[0]!.path!.map((e) => `${e.event}@${e.actor}`), ["read@1", "read@2", "write@1", "write@2"]);
});

test("a step clause is proved over every state of the declared domains", { skip }, async () => {
  const step = { id: "C2", kind: "step" as const, requires: op("eq", v("in.event"), c("write")), ensures: op("eq", v("out.pc"), c("done")) };
  const r = await exploreMachine(counter(false), code(false), [step], { limit: 4, conformanceLimit: 100_000, smt });
  assert.equal(r.clauses[0]!.verdict, "proved");
  assert.equal(r.clauses[0]!.method, "smt-induction");
});

test("a function over a large space: the solver proves its definition, the code is only evidenced against it", { skip }, async () => {
  const model = {
    name: "fee",
    inputs: { subtotal: { type: "int" as const, min: 0, max: 1_000_000 }, member: { type: "bool" as const } },
    outputs: { fee: { type: "int" as const, min: 0, max: 15 } },
    defines: { fee: op("ite", op("or", v("in.member"), op("ge", v("in.subtotal"), c(100))), c(0), c(15)) },
  };
  const a: IntentArtifact = {
    lastro: "intent-artifact/v0",
    prose: "",
    intents: [{ id: "C1", statement: "C1", kind: "behavior", provability: "formal", source: { quote: "", start: 0, end: 0 }, status: "approved" }],
    decisions: [],
    notPromised: [],
  } as IntentArtifact;
  const artifact = { ...a, hash: hashOf(a) };
  const plan = { artifactHash: artifact.hash, model, clauses: [{ id: "C1", intentIds: ["C1"], description: "free from 100", requires: op("ge", v("in.subtotal"), c(100)), ensures: op("eq", v("out.fee"), c(0)) }], scenarios: [], assumptions: [], beyondProof: [] } as unknown as ProofPlan;
  const good = (x: Env) => ({ fee: x.member || (x.subtotal as number) >= 100 ? 0 : 15 });
  const r = await verify(artifact, plan, { subject: good, smt });
  assert.equal(r.clauses[0]!.method, "smt");
  assert.equal(r.clauses[0]!.verdict, "evidenced");
  assert.match(r.clauses[0]!.note ?? "", /proved by SMT on the model's definition/);
});
