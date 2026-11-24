/**
 * SMT proofs over LSL models, for spaces too large to enumerate.
 *
 * The code under proof is opaque, so the solver never reasons about it. It proves the model: a function model whose
 * outputs are defined in LSL (`defines`), or a state machine whose transitions are written in LSL (`transitions`).
 * The code is then checked against that model on every state/event pair (or every input) when they number at most
 * the exhaustive limit, which makes the result a proof; otherwise on a fixed-seed sample, which only evidences.
 *
 * The scripts are plain SMT-LIB 2 (logic ALL: integers, booleans and strings), deterministic for a given model and
 * clause, so any solver can check them. This module is pure; `solver.ts` runs a solver as a process.
 */
import { canonicalJson } from "./artifact.js";
import type { Domain } from "./spec/domain.js";
import type { Expr } from "./spec/expr.js";
import type { Env, Model, Value } from "./types.js";

/** What a solver answers for a script that asserts the negation of a goal. */
export type SmtAnswer = "sat" | "unsat" | "unknown";

/** Runs one script; `values` are read back with (get-value) when the answer is sat. */
export type SmtSolver = (script: string, values: string[]) => Promise<{ answer: SmtAnswer; values: Record<string, Value> }>;

/** An LSL construct SMT cannot express (an unbounded string, a def call with non-boolean parameters). */
export class SmtUnsupported extends Error {}

/** A function model whose outputs the model itself defines in LSL, over in.*. */
export type DefinedModel = Model & { defines?: Record<string, Expr> };

const sym = (name: string) => `|${name}|`;
const int = (n: number) => (n < 0 ? `(- ${-n})` : String(n));

function constant(v: Value): string {
  if (typeof v === "boolean") return v ? "true" : "false";
  if (typeof v === "number") {
    if (!Number.isInteger(v)) throw new SmtUnsupported(`the number ${v} is not an integer`);
    return int(v);
  }
  if (typeof v === "string") return `"${v.replace(/"/g, '""')}"`;
  throw new SmtUnsupported(`the value ${JSON.stringify(v)} has no SMT form`);
}

/** A string enum is an SMT String, a numeric enum or an int an Int, a bool a Bool. */
function sortOf(d: Domain): string {
  if (d.type === "bool") return "Bool";
  if (d.type === "int") return "Int";
  if (d.type === "enum") return d.values.every((v) => typeof v === "number") ? "Int" : "String";
  throw new SmtUnsupported("a string domain cannot be proved by SMT");
}

/** The constraint that keeps a variable inside its domain. */
export { sym as smtSymbol };

export function domainConstraint(name: string, d: Domain): string | undefined {
  const s = sym(name);
  if (d.type === "bool") return undefined;
  if (d.type === "int") {
    const parts = [d.min !== undefined ? `(>= ${s} ${int(d.min)})` : "", d.max !== undefined ? `(<= ${s} ${int(d.max)})` : ""].filter(Boolean);
    return parts.length ? `(and ${parts.join(" ")})` : undefined;
  }
  if (d.type === "enum") return `(or ${d.values.map((v) => `(= ${s} ${constant(v)})`).join(" ")})`;
  throw new SmtUnsupported("a string domain cannot be proved by SMT");
}

/** A compiler from LSL to SMT terms; `rename` maps an LSL variable to its SMT term (undefined: unknown variable). */
export function smtTerm(e: Expr, rename: (v: string) => string | undefined, defs: Model["defs"] = {}): string {
  const go = (x: Expr): string => {
    if ("const" in x) return constant(x.const);
    if ("var" in x) {
      const term = rename(x.var);
      if (term === undefined) throw new SmtUnsupported(`unknown variable ${x.var}`);
      return term;
    }
    if ("call" in x) {
      const def = defs?.[x.call];
      if (!def) throw new SmtUnsupported(`unknown def ${x.call}`);
      // Inline the def: its parameters are bound to the arguments' terms.
      const args = new Map(def.params.map((p, i) => [p, go(x.args[i]!)]));
      return smtTerm(def.body, (v) => args.get(v) ?? rename(v), defs);
    }
    const a = x.args;
    switch (x.op) {
      case "eq":
        return `(= ${go(a[0]!)} ${go(a[1]!)})`;
      case "neq":
        return `(not (= ${go(a[0]!)} ${go(a[1]!)}))`;
      case "in":
        return `(or ${a.slice(1).map((y) => `(= ${go(a[0]!)} ${go(y)})`).join(" ")})`;
      case "implies":
        return `(=> ${go(a[0]!)} ${go(a[1]!)})`;
      case "iff":
        return `(= ${go(a[0]!)} ${go(a[1]!)})`;
      default: {
        const head = { and: "and", or: "or", not: "not", lt: "<", le: "<=", gt: ">", ge: ">=", add: "+", sub: "-", mul: "*", ite: "ite" }[x.op];
        return `(${head} ${a.map(go).join(" ")})`;
      }
    }
  };
  return go(e);
}

function declare(lines: string[], name: string, d: Domain) {
  lines.push(`(declare-const ${sym(name)} ${sortOf(d)})`);
  const c = domainConstraint(name, d);
  if (c) lines.push(`(assert ${c})`);
}

/** Parse a (get-value) answer: ((|in.x| 3) (|in.y| "a") (|in.z| true)). */
export function parseValues(text: string): Record<string, Value> {
  const out: Record<string, Value> = {};
  const re = /\(\|([^|]+)\|\s+(\(-\s*\d+\)|-?\d+|"(?:[^"]|"")*"|true|false)\)/g;
  for (let m = re.exec(text); m; m = re.exec(text)) {
    const raw = m[2]!;
    out[m[1]!] = raw === "true" ? true : raw === "false" ? false : raw.startsWith('"') ? raw.slice(1, -1).replace(/""/g, '"') : raw.startsWith("(") ? -Number(raw.replace(/[^\d]/g, "")) : Number(raw);
  }
  return out;
}

// ---------------------------------------------------------------------------
// Function models: the clause over the model's own definition
// ---------------------------------------------------------------------------

/**
 * The script that refutes `requires ⇒ ensures` over a model whose outputs are all defined in LSL. unsat: the model
 * satisfies the clause for every input in the declared domains; sat: the in.* values read back break it.
 */
export function definedClauseScript(model: DefinedModel, clause: { requires?: Expr; ensures: Expr }): { script: string; values: string[] } {
  const defines = model.defines ?? {};
  const missing = Object.keys(model.outputs).filter((k) => !defines[k]);
  if (missing.length) throw new SmtUnsupported(`the model does not define ${missing.map((k) => `out.${k}`).join(", ")} in LSL`);
  const lines = ["(set-logic ALL)"];
  for (const k of Object.keys(model.inputs).sort()) declare(lines, `in.${k}`, model.inputs[k]!);
  const inputs = (v: string) => (v.startsWith("in.") && v.slice(3) in model.inputs ? sym(v) : undefined);
  const outputs = new Map(Object.keys(model.outputs).map((k) => [`out.${k}`, smtTerm(defines[k]!, inputs, model.defs)]));
  const rename = (v: string) => inputs(v) ?? outputs.get(v);
  const goal = clause.requires ? `(=> ${smtTerm(clause.requires, rename, model.defs)} ${smtTerm(clause.ensures, rename, model.defs)})` : smtTerm(clause.ensures, rename, model.defs);
  lines.push(`(assert (not ${goal}))`, "(check-sat)");
  return { script: lines.join("\n"), values: Object.keys(model.inputs).sort().map((k) => `in.${k}`) };
}

/** The model's own outputs for an input, from its LSL definitions (used to check the code against it). */
export function definedOutputs(model: DefinedModel, input: Env, evaluate: (e: Expr, env: { input: Env; output: Env; defs: Model["defs"] }) => Value): Env {
  const out: Env = {};
  for (const [k, e] of Object.entries(model.defines ?? {})) out[k] = evaluate(e, { input, output: {}, defs: model.defs });
  return out;
}

export const sameOutputs = (a: Env, b: Env, keys: string[]) => keys.every((k) => canonicalJson(a[k] ?? null) === canonicalJson(b[k] ?? null));

// ---------------------------------------------------------------------------
// State machines: induction and bounded search over the LSL transitions
// ---------------------------------------------------------------------------

/** The parts of a machine model the SMT encoding needs (see machine.ts). */
export interface SmtMachine {
  name: string;
  state: Record<string, Domain>;
  local?: Record<string, Domain>;
  actors?: number;
  initial: Env;
  events: Record<string, Record<string, Domain>>;
  /** Event -> its guard (default: always enabled) and the next value of each variable it changes, over in.* as a step sees it. */
  transitions?: Record<string, { when?: Expr; next: Record<string, Expr> }>;
}

const keys = (o: object) => Object.keys(o).sort();
const actorCount = (m: SmtMachine) => Math.max(1, m.actors ?? 1);

/** The whole-state variables: shared ones, then each actor's locals as <name>_<actor>. */
function wholeState(m: SmtMachine): [string, Domain][] {
  const out: [string, Domain][] = keys(m.state).map((k) => [k, m.state[k]!]);
  for (let a = 1; a <= actorCount(m); a++) for (const v of keys(m.local ?? {})) out.push([`${v}_${a}`, m.local![v]!]);
  return out;
}

/** The (actor, event) choices of one step, in the canonical order. */
export function machineChoices(m: SmtMachine): { actor: number; event: string }[] {
  const out: { actor: number; event: string }[] = [];
  for (let actor = 1; actor <= actorCount(m); actor++) for (const event of keys(m.events)) out.push({ actor, event });
  return out;
}

function declareState(lines: string[], m: SmtMachine, prefix: string) {
  for (const [k, d] of wholeState(m)) declare(lines, `${prefix}.${k}`, d);
}

/** An invariant (or reach condition) over the state named by `prefix`. */
function stateTerm(m: SmtMachine, e: Expr, prefix: string): string {
  const names = new Set(wholeState(m).map(([k]) => k));
  return smtTerm(e, (v) => (v.startsWith("in.") && names.has(v.slice(3)) ? sym(`${prefix}.${v.slice(3)}`) : undefined));
}

/** How a step (guard, next values, step clause) sees in.* for actor `a` raising `event` from state `from`. */
function stepRename(m: SmtMachine, from: string, a: number, event: string, tag: string) {
  return (v: string): string | undefined => {
    if (!v.startsWith("in.")) return undefined;
    const x = v.slice(3);
    if (x in m.state) return sym(`${from}.${x}`);
    if (m.local && x in m.local) return sym(`${from}.${x}_${a}`);
    if (x === "actor") return int(a);
    if (x === "event") return constant(event);
    if (x in (m.events[event] ?? {})) return sym(`${tag}.${event}.${x}`);
    return undefined;
  };
}

/**
 * One step from `from` to `to`: some actor raises some enabled event. `tag` names the step's event inputs and its
 * choice (an Int indexing machineChoices), so a bounded search can read back which event happened.
 */
function transition(lines: string[], m: SmtMachine, from: string, to: string, tag: string): string {
  const t = m.transitions ?? {};
  for (const e of keys(m.events)) for (const i of keys(m.events[e]!)) declare(lines, `${tag}.${e}.${i}`, m.events[e]![i]!);
  lines.push(`(declare-const ${sym(`${tag}.choice`)} Int)`);
  const options = machineChoices(m).map(({ actor, event }, idx) => {
    const tr = t[event];
    if (!tr) throw new SmtUnsupported(`event ${event} has no LSL transition`);
    const rn = stepRename(m, from, actor, event, tag);
    const parts = [`(= ${sym(`${tag}.choice`)} ${idx})`];
    if (tr.when) parts.push(smtTerm(tr.when, rn));
    for (const k of keys(m.state)) parts.push(`(= ${sym(`${to}.${k}`)} ${tr.next[k] ? smtTerm(tr.next[k]!, rn) : sym(`${from}.${k}`)})`);
    for (let b = 1; b <= actorCount(m); b++)
      for (const v of keys(m.local ?? {})) parts.push(`(= ${sym(`${to}.${v}_${b}`)} ${b === actor && tr.next[v] ? smtTerm(tr.next[v]!, rn) : sym(`${from}.${v}_${b}`)})`);
    return `(and ${parts.join(" ")})`;
  });
  return `(or ${options.join(" ")})`;
}

/** Induction step: a state where the invariant holds never steps to one where it does not. unsat proves it (with the initial check). */
export function inductionScript(m: SmtMachine, invariant: Expr): string {
  const lines = ["(set-logic ALL)"];
  declareState(lines, m, "s");
  declareState(lines, m, "n");
  const tr = transition(lines, m, "s", "n", "t");
  lines.push(`(assert ${stateTerm(m, invariant, "s")})`, `(assert ${tr})`, `(assert (not ${stateTerm(m, invariant, "n")}))`, "(check-sat)");
  return lines.join("\n");
}

/**
 * Bounded search: `depth` steps from the initial state, the invariant holding before the last state and broken in it.
 * Run with depth 1, 2, ... the first sat is a shortest violation; the values to read back name each step's event.
 */
export function boundedScript(m: SmtMachine, invariant: Expr, depth: number, fixed: Record<string, Value> = {}): { script: string; values: string[] } {
  const lines = ["(set-logic ALL)"];
  const values: string[] = [];
  for (let t = 0; t <= depth; t++) declareState(lines, m, `s${t}`);
  for (const [k] of wholeState(m)) {
    const base = k in m.state ? k : k.replace(/_\d+$/, "");
    lines.push(`(assert (= ${sym(`s0.${k}`)} ${constant(m.initial[base]!)}))`);
  }
  for (let t = 0; t < depth; t++) {
    lines.push(`(assert ${transition(lines, m, `s${t}`, `s${t + 1}`, `t${t}`)})`);
    values.push(`t${t}.choice`);
    for (const e of keys(m.events)) for (const i of keys(m.events[e]!)) values.push(`t${t}.${e}.${i}`);
  }
  for (let t = 0; t < depth; t++) lines.push(`(assert ${stateTerm(m, invariant, `s${t}`)})`);
  for (const k of keys(fixed)) lines.push(`(assert (= ${sym(k)} ${constant(fixed[k]!)}))`);
  lines.push(`(assert (not ${stateTerm(m, invariant, `s${depth}`)}))`, "(check-sat)");
  return { script: lines.join("\n"), values };
}

/** The events of a bounded counterexample, read back from the solver's values. */
export function boundedPath(m: SmtMachine, depth: number, values: Record<string, Value>): { actor: number; event: string; input: Env }[] {
  const choices = machineChoices(m);
  return Array.from({ length: depth }, (_, t) => {
    const c = choices[Number(values[`t${t}.choice`])]!;
    const input: Env = {};
    for (const i of keys(m.events[c.event]!)) input[i] = values[`t${t}.${c.event}.${i}`]!;
    return { actor: c.actor, event: c.event, input };
  });
}

/**
 * A step clause over every state in the declared domains (not only the reachable ones), for one actor: unsat proves
 * it for that actor. sat is no refutation, since the state found may be unreachable.
 */
export function stepClauseScript(m: SmtMachine, clause: { requires?: Expr; ensures: Expr }, actor: number): string {
  const lines = ["(set-logic ALL)"];
  declareState(lines, m, "s");
  declareState(lines, m, "n");
  const tr = transition(lines, m, "s", "n", "t");
  lines.push(`(assert ${tr})`, `(assert (= ${sym("t.choice")} (+ ${(actor - 1) * keys(m.events).length} ${sym("t.k")})))`);
  lines.splice(1, 0, `(declare-const ${sym("t.k")} Int)`, `(assert (and (>= ${sym("t.k")} 0) (< ${sym("t.k")} ${keys(m.events).length})))`);
  // in.event and in.<input> depend on which event the step raised: a chain of ite over the choice.
  const evs = keys(m.events);
  const pick = (f: (event: string) => string | undefined): string | undefined => {
    let term: string | undefined;
    for (let k = evs.length - 1; k >= 0; k--) {
      const here = f(evs[k]!);
      if (here === undefined) continue;
      term = term === undefined ? here : `(ite (= ${sym("t.k")} ${k}) ${here} ${term})`;
    }
    return term;
  };
  const rename = (v: string): string | undefined => {
    if (v.startsWith("out.")) {
      const x = v.slice(4);
      if (x in m.state) return sym(`n.${x}`);
      if (m.local && x in m.local) return sym(`n.${x}_${actor}`);
      return undefined;
    }
    return pick((event) => stepRename(m, "s", actor, event, "t")(v));
  };
  const goal = clause.requires ? `(=> ${smtTerm(clause.requires, rename)} ${smtTerm(clause.ensures, rename)})` : smtTerm(clause.ensures, rename);
  lines.push(`(assert (not ${goal}))`, "(check-sat)");
  return lines.join("\n");
}

/**
 * The canonical shortest violation at `depth`: each step's event chosen as early as possible in the canonical order
 * (actors ascending, events by name, inputs in domain order), the order breadth-first search uses, so every engine
 * reports the same path. One solver call per candidate, step by step.
 */
export async function canonicalBounded(m: SmtMachine, invariant: Expr, depth: number, solve: SmtSolver): Promise<Record<string, Value> | undefined> {
  const fixed: Record<string, Value> = {};
  const sat = async () => (await solve(boundedScript(m, invariant, depth, fixed).script, [])).answer === "sat";
  if (!(await sat())) return undefined;
  const choices = machineChoices(m);
  for (let t = 0; t < depth; t++) {
    for (let idx = 0; idx < choices.length; idx++) {
      fixed[`t${t}.choice`] = idx;
      if (await sat()) break;
    }
    const event = choices[Number(fixed[`t${t}.choice`])]!.event;
    for (const i of keys(m.events[event]!)) {
      const d = m.events[event]![i]!;
      const values = d.type === "bool" ? [false, true] : d.type === "enum" ? d.values : d.type === "int" ? Array.from({ length: d.max! - d.min! + 1 }, (_, k) => d.min! + k) : [];
      for (const v of values) {
        fixed[`t${t}.${event}.${i}`] = v;
        if (await sat()) break;
      }
    }
  }
  return fixed;
}
