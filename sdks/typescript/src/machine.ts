/**
 * State-machine proofs.
 *
 * A machine model has shared state variables, per-actor (local) variables, an initial state and events with finite
 * inputs. Actors are copies of the same process: their events interleave in every possible order, which is how races
 * between them are found. The code under proof is a step function from a state and an event, raised by an actor, to
 * the next state (or null when the event is not enabled there).
 *
 * In LSL, a step sees `in.<shared>`, `in.<local>` (the acting actor's own copy), `in.actor`, `in.event` and
 * `in.<event input>`, and the next state as `out.<shared>` and `out.<local>`. An invariant or a reachability
 * condition sees the whole state: `in.<shared>` and `in.<local>_<actor>` (actors numbered from 1).
 *
 * Exploration is breadth first, in one canonical order (actors ascending, events by name, inputs in domain order), so
 * every engine visits the same states and finds the same shortest counterexample. A space of at most `limit`
 * reachable states (100,000 by default) is visited completely and proves; a larger one is explored by random walks
 * with a fixed seed and only evidences.
 */
import { canonicalJson } from "./artifact.js";
import { enumerateSpace, inDomain, rng, spaceSize, type Domain } from "./spec/domain.js";
import { evaluate, holds, LslError, validateExpr, type Expr } from "./spec/expr.js";
import { boundedPath, canonicalBounded, inductionScript, SmtUnsupported, stepClauseScript, type SmtSolver } from "./smt.js";
import type { ClauseVerdict, Env, Evidence, Model, Value } from "./types.js";

export interface MachineModel {
  name: string;
  kind: "machine";
  /** Shared state variables. */
  state: Record<string, Domain>;
  /** Per-actor variables: every actor has its own copy. */
  local?: Record<string, Domain>;
  /** Copies of the process. Default 1. */
  actors?: number;
  /** Initial values: every shared variable and every local variable (the same for each actor). */
  initial: Env;
  /** Event name -> its inputs (finite domains; {} for none). */
  events: Record<string, Record<string, Domain>>;
  /**
   * The transitions in LSL, so a space too large to visit can be proved by SMT: per event, the guard (`when`, over
   * in.* as a step sees it; always enabled when left out) and the next value of each variable it changes.
   */
  transitions?: Record<string, { when?: Expr; next: Record<string, Expr> }>;
}

export type MachineClause =
  | { id: string; kind: "invariant"; description?: string; holds: Expr }
  | { id: string; kind: "step"; description?: string; requires?: Expr; ensures: Expr }
  | { id: string; kind: "reach"; description?: string; condition: Expr };

/** The code under proof: null when the event is not enabled in that state. */
export type MachineStep = (input: Env) => Env | null | undefined | Promise<Env | null | undefined>;

export interface MachineEvent {
  actor: number;
  event: string;
  input: Env;
}

export type MachineMethod = "bfs" | "smt-induction" | "random-walk";

export interface MachineClauseResult {
  clauseId: string;
  kind: MachineClause["kind"];
  verdict: ClauseVerdict;
  evidence: Evidence | "counterexample";
  method: MachineMethod;
  /** States visited (bfs) or steps taken (random walks). */
  checked: number;
  /** A violation (or, for a reach clause, the witness): the events from the initial state, each with its actor. */
  path?: MachineEvent[];
  /** The state the path ends in. */
  state?: Env;
  /** For random walks: the rule-of-three upper bound on the failure rate per step. */
  failureRateUpperBound95?: number;
  note?: string;
}

export interface MachineReport {
  model: string;
  /** "bfs" when every reachable state was visited. */
  method: MachineMethod;
  states: number;
  transitions: number;
  complete: boolean;
  clauses: MachineClauseResult[];
  /** Problems with the model or the step function's outputs. */
  errors: string[];
}

export interface MachineOptions {
  /** Max reachable states visited breadth first. Default 100,000. */
  limit?: number;
  seed?: number;
  walks?: number;
  depth?: number;
  /** Proves a space too large to visit from the model's LSL transitions (see smt.ts and solver.ts). */
  smt?: SmtSolver;
  /** Max state/event pairs on which the code is checked against the model's transitions. Default: `limit`. */
  conformanceLimit?: number;
  /** Max steps of the bounded search for a violation of the model. Default 12. */
  boundedDepth?: number;
}

export class MachineError extends Error {}

export const isMachine = (m: unknown): m is MachineModel => typeof m === "object" && m !== null && (m as { kind?: unknown }).kind === "machine";

const actorsOf = (m: MachineModel) => Math.max(1, m.actors ?? 1);
const sortedKeys = (o: object) => Object.keys(o).sort();

/** Problems with a machine model: the state is finite, the initial state is in it and the events have finite inputs. */
export function machineProblems(m: MachineModel): string[] {
  const errs: string[] = [];
  const finite = (table: Record<string, Domain>, at: string) => {
    for (const [k, d] of Object.entries(table)) if (!Number.isFinite(spaceSize({ [k]: d }))) errs.push(`${at}.${k}: the domain must be finite`);
  };
  finite(m.state ?? {}, "state");
  finite(m.local ?? {}, "local");
  for (const [e, inputs] of Object.entries(m.events ?? {})) finite(inputs, `events.${e}`);
  if (!Object.keys(m.events ?? {}).length) errs.push("events: a machine needs at least one event");
  for (const [k, d] of Object.entries({ ...(m.state ?? {}), ...(m.local ?? {}) }))
    if (!inDomain(m.initial?.[k], d)) errs.push(`initial.${k}: ${JSON.stringify(m.initial?.[k])} is not in its domain`);
  const actors = m.actors ?? 1;
  if (!Number.isInteger(actors) || actors < 1) errs.push("actors: must be a positive integer");
  for (const k of Object.keys(m.local ?? {})) if (k in (m.state ?? {})) errs.push(`local.${k}: also a shared variable`);
  if (m.transitions) {
    const sm = stepModel(m);
    for (const e of Object.keys(m.events ?? {})) if (!m.transitions[e]) errs.push(`transitions.${e}: every event needs a transition`);
    for (const [e, t] of Object.entries(m.transitions)) {
      if (!(e in (m.events ?? {}))) errs.push(`transitions.${e}: no such event`);
      if (t.when) errs.push(...validateExpr(t.when, sm).map((x) => `transitions.${e}.when: ${x}`));
      for (const [k, x] of Object.entries(t.next ?? {})) {
        if (!(k in (m.state ?? {})) && !(k in (m.local ?? {}))) errs.push(`transitions.${e}.next.${k}: no such variable`);
        errs.push(...validateExpr(x, sm).map((y) => `transitions.${e}.next.${k}: ${y}`));
      }
    }
  }
  return errs;
}

/** The model a step clause is written against (in.* = state, actor, event and inputs; out.* = next state). */
export function stepModel(m: MachineModel): Model {
  const inputs: Record<string, Domain> = { ...m.state, ...(m.local ?? {}), actor: { type: "int", min: 1, max: actorsOf(m) }, event: { type: "enum", values: sortedKeys(m.events) } };
  for (const ev of Object.values(m.events)) Object.assign(inputs, ev);
  return { name: m.name, inputs, outputs: { ...m.state, ...(m.local ?? {}) } };
}

/** The model an invariant or a reachability condition is written against (in.* = the whole state). */
export function stateModel(m: MachineModel): Model {
  const inputs: Record<string, Domain> = { ...m.state };
  for (let k = 1; k <= actorsOf(m); k++) for (const [v, d] of Object.entries(m.local ?? {})) inputs[`${v}_${k}`] = d;
  return { name: m.name, inputs, outputs: {} };
}

export function machineClauseProblems(m: MachineModel, c: MachineClause): string[] {
  if (c.kind === "step") return [...(c.requires ? validateExpr(c.requires, stepModel(m)) : []), ...validateExpr(c.ensures, stepModel(m))];
  return validateExpr(c.kind === "invariant" ? c.holds : c.condition, stateModel(m));
}

function initialState(m: MachineModel): Env {
  const s: Env = {};
  for (const k of sortedKeys(m.state)) s[k] = m.initial[k]!;
  for (let a = 1; a <= actorsOf(m); a++) for (const v of sortedKeys(m.local ?? {})) s[`${v}_${a}`] = m.initial[v]!;
  return s;
}

/** Every event any actor can raise, in the canonical order. */
function eventsOf(m: MachineModel): MachineEvent[] {
  const out: MachineEvent[] = [];
  for (let actor = 1; actor <= actorsOf(m); actor++)
    for (const event of sortedKeys(m.events)) for (const input of enumerateSpace(m.events[event]!)) out.push({ actor, event, input });
  return out;
}

/** What the step function and a step clause see for one event in one state. */
function stepInput(m: MachineModel, s: Env, e: MachineEvent): Env {
  const x: Env = { actor: e.actor, event: e.event, ...e.input };
  for (const k of sortedKeys(m.state)) x[k] = s[k]!;
  for (const v of sortedKeys(m.local ?? {})) x[v] = s[`${v}_${e.actor}`]!;
  return x;
}

/** The next whole state from the step function's output, or a problem with it. */
function nextState(m: MachineModel, s: Env, e: MachineEvent, out: Env): Env | string {
  const n: Env = { ...s };
  for (const [k, d] of Object.entries(m.state)) {
    const v = k in out ? out[k]! : s[k]!;
    if (!inDomain(v, d)) return `${k}=${JSON.stringify(v)} is outside its domain after ${show(e)}`;
    n[k] = v;
  }
  for (const [k, d] of Object.entries(m.local ?? {})) {
    const own = `${k}_${e.actor}`;
    const v = k in out ? out[k]! : s[own]!;
    if (!inDomain(v, d)) return `${k}=${JSON.stringify(v)} is outside its domain after ${show(e)}`;
    n[own] = v;
  }
  return n;
}

/** The step output a step clause sees: every shared variable and the actor's own locals. */
function stepOutput(m: MachineModel, n: Env, actor: number): Env {
  const o: Env = {};
  for (const k of Object.keys(m.state)) o[k] = n[k]!;
  for (const k of Object.keys(m.local ?? {})) o[k] = n[`${k}_${actor}`]!;
  return o;
}

const show = (e: MachineEvent) => `${e.event}${Object.keys(e.input).length ? canonicalJson(e.input) : ""} by actor ${e.actor}`;
const key = (s: Env) => canonicalJson(s);

interface Node {
  state: Env;
  parent?: number;
  via?: MachineEvent;
}

function pathTo(nodes: Node[], i: number): MachineEvent[] {
  const path: MachineEvent[] = [];
  for (let k: number | undefined = i; k !== undefined && nodes[k]!.via; k = nodes[k]!.parent) path.unshift(nodes[k]!.via!);
  return path;
}

/** Explore a machine against its clauses with the code's step function. */
export async function exploreMachine(m: MachineModel, step: MachineStep, clauses: MachineClause[], opts: MachineOptions = {}): Promise<MachineReport> {
  const errors = [...machineProblems(m), ...clauses.flatMap((c) => machineClauseProblems(m, c).map((p) => `${c.id}: ${p}`))];
  const report = (method: MachineMethod, states: number, transitions: number, complete: boolean, results: MachineClauseResult[]): MachineReport => ({
    model: m.name,
    method,
    states,
    transitions,
    complete,
    clauses: results,
    errors,
  });
  if (errors.length) return report("bfs", 0, 0, false, clauses.map((c) => ({ clauseId: c.id, kind: c.kind, verdict: "not-run", evidence: "none", method: "bfs", checked: 0, note: "the model or the clause is invalid" })));

  const limit = opts.limit ?? 100_000;
  const events = eventsOf(m);
  const sm = stateModel(m);
  const ctxState = (s: Env) => ({ input: s, output: {}, defs: sm.defs });
  const results = new Map<string, MachineClauseResult>(clauses.map((c) => [c.id, { clauseId: c.id, kind: c.kind, verdict: "not-run", evidence: "none", method: "bfs", checked: 0 }]));
  const applied = new Map<string, number>();
  const fail = (c: MachineClause, note: string) => {
    const r = results.get(c.id)!;
    if (r.verdict === "not-run") Object.assign(r, { verdict: "violated", evidence: "none", note });
  };
  const safe = (c: MachineClause, f: () => boolean): boolean | undefined => {
    try {
      return f();
    } catch (e) {
      if (!(e instanceof LslError)) throw e;
      fail(c, `evaluation error: ${e.message}`);
      return undefined;
    }
  };
  /** Checks a new state; returns false when every clause is settled and exploration can stop early. */
  const onState = (nodes: Node[], i: number, method: MachineMethod) => {
    const s = nodes[i]!.state;
    for (const c of clauses) {
      const r = results.get(c.id)!;
      if (r.verdict !== "not-run") continue;
      if (c.kind === "invariant" && safe(c, () => !holds(c.holds, ctxState(s))))
        Object.assign(r, { verdict: "violated", evidence: "counterexample", method, path: pathTo(nodes, i), state: s });
      if (c.kind === "reach" && safe(c, () => holds(c.condition, ctxState(s))))
        Object.assign(r, { verdict: "proved", evidence: "formal", method, path: pathTo(nodes, i), state: s, note: "reached: the path is a witness" });
    }
  };
  const onStep = (nodes: Node[], from: number, e: MachineEvent, next: Env, method: MachineMethod): boolean => {
    let bad = false;
    for (const c of clauses) {
      if (c.kind !== "step") continue;
      const r = results.get(c.id)!;
      if (r.verdict !== "not-run") continue;
      const ctx = { input: stepInput(m, nodes[from]!.state, e), output: stepOutput(m, next, e.actor), defs: undefined };
      const applies = safe(c, () => !c.requires || holds(c.requires, ctx));
      if (!applies) continue;
      applied.set(c.id, (applied.get(c.id) ?? 0) + 1);
      if (safe(c, () => !holds(c.ensures, ctx))) {
        Object.assign(r, { verdict: "violated", evidence: "counterexample", method, path: [...pathTo(nodes, from), e], state: next });
        bad = true;
      }
    }
    return bad;
  };
  const run = async (s: Env, e: MachineEvent): Promise<Env | null | string> => {
    try {
      const out = await step(stepInput(m, s, e));
      return out ? nextState(m, s, e, out) : null;
    } catch (err) {
      return `the step function threw on ${show(e)}: ${(err as Error).message}`;
    }
  };

  // Breadth first over every interleaving.
  const nodes: Node[] = [{ state: initialState(m) }];
  const seen = new Map<string, number>([[key(nodes[0]!.state), 0]]);
  onState(nodes, 0, "bfs");
  let transitions = 0;
  let complete = true;
  for (let i = 0; i < nodes.length; i++) {
    for (const e of events) {
      const n = await run(nodes[i]!.state, e);
      if (n === null) continue;
      if (typeof n === "string") {
        if (!errors.includes(n)) errors.push(n);
        continue;
      }
      transitions++;
      onStep(nodes, i, e, n, "bfs");
      const k = key(n);
      if (seen.has(k)) continue;
      if (nodes.length >= limit) {
        complete = false;
        break;
      }
      seen.set(k, nodes.length);
      nodes.push({ state: n, parent: i, via: e });
      onState(nodes, nodes.length - 1, "bfs");
    }
    if (!complete) break;
  }

  if (complete) {
    for (const c of clauses) {
      const r = results.get(c.id)!;
      r.checked = c.kind === "step" ? applied.get(c.id) ?? 0 : nodes.length;
      if (r.verdict !== "not-run") continue;
      if (c.kind === "reach") Object.assign(r, { verdict: "violated", evidence: "formal", note: `no reachable state satisfies the condition (${nodes.length} states visited)` });
      else if (c.kind === "step" && !applied.get(c.id)) Object.assign(r, { verdict: "vacuous", note: "no reachable step makes the if-part true: the clause proves nothing" });
      else Object.assign(r, { verdict: "proved", evidence: "formal" });
    }
    if (errors.length) for (const r of results.values()) if (r.verdict === "proved") Object.assign(r, { verdict: "violated", evidence: "none", note: errors[0] });
    return report("bfs", nodes.length, transitions, true, [...results.values()]);
  }

  // Too many states. If the model writes its transitions in LSL and a solver is at hand, prove the model by SMT and
  // check the code against it on every state/event pair; what the solver cannot decide goes to random walks.
  if (opts.smt && m.transitions) await proveBySmt(m, step, clauses, results, opts, limit);

  // Too many states: random walks with a fixed seed. A clause that holds is evidenced, never proved; a reach witness
  // still proves the state reachable.
  const next = rng(opts.seed ?? 42);
  const walks = opts.walks ?? 200;
  const depth = opts.depth ?? 50;
  let steps = 0;
  for (let w = 0; w < walks; w++) {
    const walk: Node[] = [{ state: initialState(m) }];
    for (let d = 0; d < depth; d++) {
      const here = walk[walk.length - 1]!.state;
      const enabled: [MachineEvent, Env][] = [];
      for (const e of events) {
        const n = await run(here, e);
        if (n && typeof n !== "string") enabled.push([e, n]);
      }
      if (!enabled.length) break;
      const [e, n] = enabled[Math.floor(next() * enabled.length)]!;
      steps++;
      onStep(walk, walk.length - 1, e, n, "random-walk");
      walk.push({ state: n, parent: walk.length - 1, via: e });
      onState(walk, walk.length - 1, "random-walk");
    }
  }
  for (const c of clauses) {
    const r = results.get(c.id)!;
    if (r.verdict !== "not-run") continue;
    const smtNote = r.note;
    r.method = "random-walk";
    r.checked = steps;
    if (c.kind === "reach") Object.assign(r, { verdict: "evidenced", evidence: "none", note: "not reached by the walks; the space is too large to show it unreachable" });
    else if (c.kind === "step" && !applied.get(c.id)) Object.assign(r, { verdict: "vacuous", note: "no step of the walks makes the if-part true" });
    else Object.assign(r, { verdict: "evidenced", evidence: "empirical", failureRateUpperBound95: Math.min(1, 3 / Math.max(1, c.kind === "step" ? applied.get(c.id)! : steps)), note: [`more than ${limit} reachable states: explored by ${walks} random walks of up to ${depth} steps (seed ${opts.seed ?? 42})`, smtNote].filter(Boolean).join("; ") });
  }
  return report("random-walk", nodes.length, transitions, false, [...results.values()]);
}

// ---------------------------------------------------------------------------
// SMT: the model's LSL transitions, and the code checked against them
// ---------------------------------------------------------------------------

/** The model's own step function, from its LSL transitions: null when the event's guard is false. */
export function modelStep(m: MachineModel): MachineStep {
  return (x) => {
    const t = m.transitions?.[String(x.event)];
    if (!t) return null;
    const ctx = { input: x, output: {}, defs: undefined };
    if (t.when && !holds(t.when, ctx)) return null;
    const out: Env = {};
    for (const [k, e] of Object.entries(t.next)) out[k] = evaluate(e, ctx);
    return out;
  };
}

export interface Conformance {
  /** Every state/event pair in the declared domains was checked. */
  complete: boolean;
  checked: number;
  /** The first pair where the code and the model disagree. */
  mismatch?: { input: Env; code: Env | null; model: Env | null };
}

/**
 * Check the code's step against the model's on every pair of a state (in the declared domains) and an event raised by
 * an actor, or on a fixed-seed sample of them when there are more than `limit`. Two steps agree when both are
 * disabled, or both give the same next value for every variable.
 */
export async function conformance(m: MachineModel, step: MachineStep, limit = 100_000, seed = 42): Promise<Conformance> {
  const vars: Record<string, Domain> = { ...m.state, ...(m.local ?? {}) };
  const model = modelStep(m);
  const perEvent = sortedKeys(m.events).map((e) => [e, spaceSize(m.events[e]!)] as const);
  const states = spaceSize(vars);
  const total = states * actorsOf(m) * perEvent.reduce((n, [, k]) => n + k, 0);
  const same = (x: Env, a: Env | null | undefined, b: Env | null | undefined) =>
    (!a && !b) || (!!a && !!b && Object.keys(vars).every((k) => canonicalJson((k in a ? a[k] : x[k]) ?? null) === canonicalJson((k in b ? b[k] : x[k]) ?? null)));
  const check = async (x: Env): Promise<Conformance["mismatch"]> => {
    let code: Env | null | undefined;
    try {
      code = await step({ ...x });
    } catch (e) {
      return { input: x, code: { error: (e as Error).message }, model: model(x) as Env | null };
    }
    const mine = model(x) as Env | null;
    return same(x, code, mine) ? undefined : { input: x, code: code ?? null, model: mine };
  };
  if (total <= limit) {
    let checked = 0;
    for (let actor = 1; actor <= actorsOf(m); actor++)
      for (const event of sortedKeys(m.events))
        for (const input of enumerateSpace(m.events[event]!))
          for (const st of enumerateSpace(vars)) {
            checked++;
            const bad = await check({ ...st, actor, event, ...input });
            if (bad) return { complete: true, checked, mismatch: bad };
          }
    return { complete: true, checked };
  }
  const next = rng(seed);
  const n = 2000;
  for (let i = 0; i < n; i++) {
    const actor = 1 + Math.floor(next() * actorsOf(m));
    const event = sortedKeys(m.events)[Math.floor(next() * perEvent.length)]!;
    const bad = await check({ ...sampleOf(vars, next), actor, event, ...sampleOf(m.events[event]!, next) });
    if (bad) return { complete: false, checked: i + 1, mismatch: bad };
  }
  return { complete: false, checked: n };
}

function sampleOf(table: Record<string, Domain>, next: () => number): Env {
  const out: Env = {};
  for (const k of sortedKeys(table)) {
    const values = [...enumerateSpace({ [k]: table[k]! })].map((x) => x[k]!);
    out[k] = values[Math.floor(next() * values.length)]!;
  }
  return out;
}

async function proveBySmt(m: MachineModel, step: MachineStep, clauses: MachineClause[], results: Map<string, MachineClauseResult>, opts: MachineOptions, limit: number) {
  const solve = opts.smt!;
  let conf: Conformance | undefined;
  const conforms = async () => (conf ??= await conformance(m, step, opts.conformanceLimit ?? limit, opts.seed ?? 42));
  for (const c of clauses) {
    const r = results.get(c.id)!;
    if (r.verdict !== "not-run" || c.kind === "reach") continue;
    let proved = false;
    try {
      if (c.kind === "invariant") {
        const s0 = initialState(m);
        if (!holds(c.holds, { input: s0, output: {}, defs: undefined })) {
          Object.assign(r, { verdict: "violated", evidence: "counterexample", method: "smt-induction", path: [], state: s0, note: "the initial state breaks the invariant" });
          continue;
        }
        const ind = await solve(inductionScript(m, c.holds), []);
        if (ind.answer === "unsat") proved = true;
        else {
          // Not inductive (or undecided): look for a real violation of the model, shortest first.
          for (let d = 1; d <= (opts.boundedDepth ?? 12) && !proved; d++) {
            const values = await canonicalBounded(m, c.holds, d, solve);
            if (!values) continue;
            const path = boundedPath(m, d, values);
            const end = await replay(m, step, path);
            const broken = typeof end !== "string" && !holds(c.holds, { input: end, output: {}, defs: undefined });
            Object.assign(r, broken
              ? { verdict: "violated", evidence: "counterexample", method: "smt-induction", path, state: end, note: `the model breaks the invariant in ${d} step(s), and so does the code` }
              : { note: `the model breaks the invariant in ${d} step(s), but the code does not follow that path${typeof end === "string" ? ` (${end})` : ""}` });
            break;
          }
          if (r.verdict === "not-run" && !r.note) r.note = ind.answer === "unknown" ? "solver-unknown: the solver could not decide the induction" : "solver-unknown: the invariant is not inductive and no violation was found within the bound";
          continue;
        }
      } else {
        let all = true;
        for (let a = 1; a <= actorsOf(m) && all; a++) all = (await solve(stepClauseScript(m, c, a), [])).answer === "unsat";
        if (all) proved = true;
        else {
          r.note = "solver-unknown: the step clause does not hold over every state of the declared domains";
          continue;
        }
      }
    } catch (e) {
      if (!(e instanceof SmtUnsupported) && !(e instanceof LslError)) throw e;
      r.note = `not-expressible: ${e.message}`;
      continue;
    }
    if (!proved) continue;
    const k = await conforms();
    if (k.mismatch)
      Object.assign(r, { verdict: "violated", evidence: "counterexample", method: "smt-induction", checked: k.checked, state: k.mismatch.input, note: `the model is proved by SMT, but the code differs from it at ${canonicalJson(k.mismatch.input)}: code ${canonicalJson(k.mismatch.code)}, model ${canonicalJson(k.mismatch.model)}` });
    else if (k.complete) Object.assign(r, { verdict: "proved", evidence: "formal", method: "smt-induction", checked: k.checked, note: `proved by SMT induction on the model; the code matches the model on all ${k.checked} state/event pairs` });
    else Object.assign(r, { verdict: "evidenced", evidence: "empirical", method: "smt-induction", checked: k.checked, failureRateUpperBound95: Math.min(1, 3 / k.checked), note: `proved by SMT induction on the model; the code matches it on ${k.checked} sampled state/event pairs (too many to check them all)` });
  }
}

/** Run the code along a path from the initial state; the state it ends in, or what went wrong. */
async function replay(m: MachineModel, step: MachineStep, path: MachineEvent[]): Promise<Env | string> {
  let s = initialState(m);
  for (const e of path) {
    let out: Env | null | undefined;
    try {
      out = await step(stepInput(m, s, e));
    } catch (err) {
      return `the step function threw on ${show(e)}: ${(err as Error).message}`;
    }
    if (!out) return `the code does not enable ${show(e)}`;
    const n = nextState(m, s, e, out);
    if (typeof n === "string") return n;
    s = n;
  }
  return s;
}

// ---------------------------------------------------------------------------
// Statechart
// ---------------------------------------------------------------------------

/** A step that answers from a recorded table ({input, output|null} rows), as the Rust prover and the other SDKs take it. */
export function tableStep(rows: { input: Env; output: Env | null }[]): MachineStep {
  const by = new Map(rows.map((r) => [key(r.input), r.output] as const));
  return (input) => {
    const k = key(input);
    if (!by.has(k)) throw new Error(`the table has no row for ${k}`);
    return by.get(k) ?? null;
  };
}

export interface StatechartRegion {
  /** "shared" or "actor <k>". */
  name: string;
  states: { id: string; label: string; initial: boolean; forbidden: boolean }[];
  transitions: { from: string; to: string; event: string; onPath: boolean }[];
}

export interface Statechart {
  model: string;
  regions: StatechartRegion[];
}

/**
 * The machine as the user sees it: one region for the shared state and one per actor (its own variables), each state
 * a valuation of that region's variables, each transition labelled with the event that causes it. States that appear
 * in a state an invariant forbids are marked, and the counterexample path, when there is one, is flagged.
 */
export async function statechart(m: MachineModel, step: MachineStep, clauses: MachineClause[] = [], path: MachineEvent[] = [], opts: { limit?: number } = {}): Promise<Statechart> {
  const actors = actorsOf(m);
  const regions: { name: string; vars: (k: number) => string[]; actor?: number }[] = [];
  if (Object.keys(m.state).length) regions.push({ name: "shared", vars: () => sortedKeys(m.state) });
  if (Object.keys(m.local ?? {}).length) for (let a = 1; a <= actors; a++) regions.push({ name: `actor ${a}`, actor: a, vars: (k) => sortedKeys(m.local!).map((v) => `${v}_${k}`) });
  const label = (s: Env, vars: string[]) => vars.map((v) => `${v.replace(/_\d+$/, "")}=${String(s[v])}`).join(", ") || "(no state)";
  const out: StatechartRegion[] = regions.map((r) => ({ name: r.name, states: [], transitions: [] }));
  const invariants = clauses.filter((c): c is Extract<MachineClause, { kind: "invariant" }> => c.kind === "invariant");
  const sm = stateModel(m);
  const forbidden = (s: Env) => invariants.some((c) => {
    try {
      return !holds(c.holds, { input: s, output: {}, defs: sm.defs });
    } catch {
      return false;
    }
  });
  const addState = (r: number, s: Env, initial: boolean) => {
    const vars = regions[r]!.vars(regions[r]!.actor ?? 0);
    const id = `s${canonicalJson(vars.map((v) => s[v]))}`;
    const st = out[r]!.states.find((x) => x.id === id);
    if (!st) out[r]!.states.push({ id, label: label(s, vars), initial, forbidden: forbidden(s) });
    else if (forbidden(s)) st.forbidden = true;
    return id;
  };
  const events = eventsOf(m);
  const limit = opts.limit ?? 100_000;
  const queue: Env[] = [initialState(m)];
  const seen = new Set([key(queue[0]!)]);
  regions.forEach((_, r) => addState(r, queue[0]!, true));
  // Walk the counterexample path first so its edges can be flagged.
  const pathEdges = new Set<string>();
  {
    let s = queue[0]!;
    for (const e of path) {
      const out1 = await step(stepInput(m, s, e));
      if (!out1) break;
      const n = nextState(m, s, e, out1);
      if (typeof n === "string") break;
      regions.forEach((r, k) => {
        if (r.actor !== undefined && r.actor !== e.actor) return;
        pathEdges.add(`${k}|${addState(k, s, false)}|${addState(k, n, false)}|${e.event}`);
      });
      s = n;
    }
  }
  for (let i = 0; i < queue.length && queue.length <= limit; i++) {
    const s = queue[i]!;
    for (const e of events) {
      let o: Env | null | undefined;
      try {
        o = await step(stepInput(m, s, e));
      } catch {
        continue;
      }
      if (!o) continue;
      const n = nextState(m, s, e, o);
      if (typeof n === "string") continue;
      regions.forEach((r, k) => {
        if (r.actor !== undefined && r.actor !== e.actor) return;
        const from = addState(k, s, false);
        const to = addState(k, n, false);
        if (from === to && r.actor === undefined && sortedKeys(m.state).every((v) => s[v] === n[v]) && regions.length > 1) return;
        const t = out[k]!.transitions;
        if (!t.some((x) => x.from === from && x.to === to && x.event === e.event)) t.push({ from, to, event: e.event, onPath: pathEdges.has(`${k}|${from}|${to}|${e.event}`) });
      });
      const kk = key(n);
      if (!seen.has(kk)) {
        seen.add(kk);
        queue.push(n);
      }
    }
  }
  for (const r of out) {
    r.states.sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
    // Ordinal, never locale-dependent: every engine draws the same chart.
    const k = (t: { from: string; to: string; event: string }) => canonicalJson([t.from, t.to, t.event]);
    r.transitions.sort((a, b) => (k(a) < k(b) ? -1 : k(a) > k(b) ? 1 : 0));
  }
  return { model: m.name, regions: out };
}

/** The statechart in Mermaid (stateDiagram-v2), one concurrent region per actor. Forbidden states are styled. */
export function mermaidStatechart(c: Statechart): string {
  const ids = new Map<string, string>();
  const id = (region: number, s: string) => {
    const k = `${region}|${s}`;
    if (!ids.has(k)) ids.set(k, `r${region}_${ids.size}`);
    return ids.get(k)!;
  };
  const lines = ["stateDiagram-v2", `  state "${c.model}" as machine {`];
  c.regions.forEach((r, k) => {
    if (k > 0) lines.push("    --");
    lines.push(`    %% ${r.name}`);
    for (const s of r.states) {
      lines.push(`    state "${s.label.replace(/"/g, "'")}" as ${id(k, s.id)}`);
      if (s.initial) lines.push(`    [*] --> ${id(k, s.id)}`);
    }
    for (const t of r.transitions) lines.push(`    ${id(k, t.from)} --> ${id(k, t.to)} : ${t.event}${t.onPath ? " (counterexample)" : ""}`);
  });
  lines.push("  }");
  const bad = c.regions.flatMap((r, k) => r.states.filter((s) => s.forbidden).map((s) => id(k, s.id)));
  if (bad.length) lines.push("  classDef forbidden fill:#fdd,stroke:#c00", `  class ${bad.join(",")} forbidden`);
  return lines.join("\n") + "\n";
}

export type { Value };
