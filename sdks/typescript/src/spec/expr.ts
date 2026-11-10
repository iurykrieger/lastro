/**
 * LSL — Lastro Spec Language.
 *
 * A tiny, JSON-serializable expression AST. It is safe to accept from an LLM
 * (no eval), evaluable in TS, and compilable to SMT-LIB2 for external provers.
 *
 * Variables are namespaced: `in.<name>` for model inputs, `out.<name>` for
 * outputs. Inside a def body, parameters are referenced by bare name.
 */
import type { Env, Model, Value } from "../types.js";
import { domainOf } from "./domain.js";

export type Op =
  | "and"
  | "or"
  | "not"
  | "implies"
  | "iff"
  | "eq"
  | "neq"
  | "lt"
  | "le"
  | "gt"
  | "ge"
  | "add"
  | "sub"
  | "mul"
  | "ite"
  | "in";

export type Expr =
  | { var: string }
  | { const: Value }
  | { op: Op; args: Expr[] }
  | { call: string; args: Expr[] };

const ARITY: Record<Op, [number, number]> = {
  and: [1, Infinity],
  or: [1, Infinity],
  not: [1, 1],
  implies: [2, 2],
  iff: [2, 2],
  eq: [2, 2],
  neq: [2, 2],
  lt: [2, 2],
  le: [2, 2],
  gt: [2, 2],
  ge: [2, 2],
  add: [2, Infinity],
  sub: [2, 2],
  mul: [2, Infinity],
  ite: [3, 3],
  in: [2, Infinity],
};

export const OPS = Object.keys(ARITY) as Op[];

// ---------------------------------------------------------------------------
// Builders (ergonomic authoring)
// ---------------------------------------------------------------------------

const lit = (v: Expr | Value): Expr => (typeof v === "object" ? v : { const: v });

export const L = {
  in: (name: string): Expr => ({ var: `in.${name}` }),
  out: (name: string): Expr => ({ var: `out.${name}` }),
  v: (name: string): Expr => ({ var: name }),
  c: (value: Value): Expr => ({ const: value }),
  and: (...args: Expr[]): Expr => ({ op: "and", args }),
  or: (...args: Expr[]): Expr => ({ op: "or", args }),
  not: (a: Expr): Expr => ({ op: "not", args: [a] }),
  implies: (a: Expr, b: Expr): Expr => ({ op: "implies", args: [a, b] }),
  iff: (a: Expr, b: Expr): Expr => ({ op: "iff", args: [a, b] }),
  eq: (a: Expr | Value, b: Expr | Value): Expr => ({ op: "eq", args: [lit(a), lit(b)] }),
  neq: (a: Expr | Value, b: Expr | Value): Expr => ({ op: "neq", args: [lit(a), lit(b)] }),
  lt: (a: Expr | Value, b: Expr | Value): Expr => ({ op: "lt", args: [lit(a), lit(b)] }),
  le: (a: Expr | Value, b: Expr | Value): Expr => ({ op: "le", args: [lit(a), lit(b)] }),
  gt: (a: Expr | Value, b: Expr | Value): Expr => ({ op: "gt", args: [lit(a), lit(b)] }),
  ge: (a: Expr | Value, b: Expr | Value): Expr => ({ op: "ge", args: [lit(a), lit(b)] }),
  ite: (c: Expr, a: Expr | Value, b: Expr | Value): Expr => ({ op: "ite", args: [c, lit(a), lit(b)] }),
  oneOf: (a: Expr, ...values: Value[]): Expr => ({ op: "in", args: [a, ...values.map((v) => lit(v))] }),
  call: (name: string, ...args: Expr[]): Expr => ({ call: name, args }),
};

// ---------------------------------------------------------------------------
// Evaluation
// ---------------------------------------------------------------------------

export class LslError extends Error {}

export interface EvalContext {
  input: Env;
  output: Env;
  defs?: Model["defs"];
}

export function evaluate(e: Expr, ctx: EvalContext, locals: Env = {}, depth = 0): Value {
  if (depth > 64) throw new LslError("LSL: max recursion depth exceeded (recursive def?)");
  if ("const" in e) return e.const;
  if ("var" in e) return lookup(e.var, ctx, locals);
  if ("call" in e) {
    const def = ctx.defs?.[e.call];
    if (!def) throw new LslError(`LSL: unknown def '${e.call}'`);
    if (def.params.length !== e.args.length)
      throw new LslError(`LSL: def '${e.call}' expects ${def.params.length} args, got ${e.args.length}`);
    const bound: Env = {};
    def.params.forEach((p, i) => (bound[p] = evaluate(e.args[i]!, ctx, locals, depth + 1)));
    return evaluate(def.body, ctx, bound, depth + 1);
  }
  const ev = (x: Expr) => evaluate(x, ctx, locals, depth + 1);
  const a = e.args;
  switch (e.op) {
    case "and":
      return a.every((x) => bool(ev(x)));
    case "or":
      return a.some((x) => bool(ev(x)));
    case "not":
      return !bool(ev(a[0]!));
    case "implies":
      return !bool(ev(a[0]!)) || bool(ev(a[1]!));
    case "iff":
      return bool(ev(a[0]!)) === bool(ev(a[1]!));
    case "eq":
      return ev(a[0]!) === ev(a[1]!);
    case "neq":
      return ev(a[0]!) !== ev(a[1]!);
    case "lt":
      return num(ev(a[0]!)) < num(ev(a[1]!));
    case "le":
      return num(ev(a[0]!)) <= num(ev(a[1]!));
    case "gt":
      return num(ev(a[0]!)) > num(ev(a[1]!));
    case "ge":
      return num(ev(a[0]!)) >= num(ev(a[1]!));
    case "add":
      return a.reduce((s, x) => s + num(ev(x)), 0);
    case "sub":
      return num(ev(a[0]!)) - num(ev(a[1]!));
    case "mul":
      return a.reduce((s, x) => s * num(ev(x)), 1);
    case "ite":
      return bool(ev(a[0]!)) ? ev(a[1]!) : ev(a[2]!);
    case "in": {
      const v = ev(a[0]!);
      return a.slice(1).some((x) => ev(x) === v);
    }
  }
}

export function holds(e: Expr, ctx: EvalContext): boolean {
  return bool(evaluate(e, ctx));
}

function lookup(name: string, ctx: EvalContext, locals: Env): Value {
  if (name in locals) return locals[name]!;
  const [ns, field] = splitVar(name);
  const src = ns === "in" ? ctx.input : ns === "out" ? ctx.output : undefined;
  if (!src || field === undefined || !(field in src)) throw new LslError(`LSL: unbound variable '${name}'`);
  return src[field]!;
}

function bool(v: Value): boolean {
  if (typeof v !== "boolean") throw new LslError(`LSL: expected boolean, got ${JSON.stringify(v)}`);
  return v;
}

function num(v: Value): number {
  if (typeof v !== "number") throw new LslError(`LSL: expected number, got ${JSON.stringify(v)}`);
  return v;
}

function splitVar(name: string): [string, string | undefined] {
  const i = name.indexOf(".");
  return i < 0 ? [name, undefined] : [name.slice(0, i), name.slice(i + 1)];
}

// ---------------------------------------------------------------------------
// Static validation
// ---------------------------------------------------------------------------

/** Returns a list of problems; empty means the expression is well-formed for the model. */
export function validateExpr(e: unknown, model: Model, locals: string[] = [], path = "expr"): string[] {
  const errs: string[] = [];
  if (typeof e !== "object" || e === null) return [`${path}: not an object`];
  const o = e as Record<string, unknown>;
  if ("const" in o) {
    if (!["boolean", "number", "string"].includes(typeof o.const)) errs.push(`${path}: bad const`);
  } else if ("var" in o) {
    const name = String(o.var);
    if (!locals.includes(name)) {
      const [ns, field] = splitVar(name);
      const table = ns === "in" ? model.inputs : ns === "out" ? model.outputs : undefined;
      if (!table || field === undefined || !(field in table))
        errs.push(`${path}: unknown variable '${name}' (use in.<input> or out.<output>)`);
    }
  } else if ("call" in o) {
    const def = model.defs?.[String(o.call)];
    if (!def) errs.push(`${path}: unknown def '${String(o.call)}'`);
    const args = Array.isArray(o.args) ? o.args : [];
    if (def && def.params.length !== args.length) errs.push(`${path}: def '${String(o.call)}' arity mismatch`);
    args.forEach((x, i) => errs.push(...validateExpr(x, model, locals, `${path}.args[${i}]`)));
  } else if ("op" in o) {
    const op = o.op as Op;
    const args = Array.isArray(o.args) ? o.args : null;
    if (!(op in ARITY)) errs.push(`${path}: unknown op '${String(op)}'`);
    else if (!args) errs.push(`${path}: missing args`);
    else {
      const [min, max] = ARITY[op];
      if (args.length < min || args.length > max) errs.push(`${path}: op '${op}' arity ${args.length}`);
      args.forEach((x, i) => errs.push(...validateExpr(x, model, locals, `${path}.args[${i}]`)));
    }
  } else errs.push(`${path}: expected one of var/const/op/call`);
  return errs;
}

export function freeVars(e: Expr, out = new Set<string>()): Set<string> {
  if ("var" in e) out.add(e.var);
  else if ("op" in e || "call" in e) e.args.forEach((x) => freeVars(x, out));
  return out;
}

// ---------------------------------------------------------------------------
// Pretty printing
// ---------------------------------------------------------------------------

const INFIX: Partial<Record<Op, string>> = {
  and: "∧",
  or: "∨",
  implies: "⇒",
  iff: "⇔",
  eq: "=",
  neq: "≠",
  lt: "<",
  le: "≤",
  gt: ">",
  ge: "≥",
  add: "+",
  sub: "-",
  mul: "·",
};

export function show(e: Expr): string {
  if ("const" in e) return JSON.stringify(e.const);
  if ("var" in e) return e.var;
  if ("call" in e) return `${e.call}(${e.args.map(show).join(", ")})`;
  const a = e.args;
  if (e.op === "not") return `¬${wrap(a[0]!)}`;
  if (e.op === "ite") return `if ${show(a[0]!)} then ${show(a[1]!)} else ${show(a[2]!)}`;
  if (e.op === "in") return `${show(a[0]!)} ∈ {${a.slice(1).map(show).join(", ")}}`;
  return a.map(wrap).join(` ${INFIX[e.op]} `);
}

function wrap(e: Expr): string {
  return "op" in e && e.op !== "not" ? `(${show(e)})` : show(e);
}

// ---------------------------------------------------------------------------
// SMT-LIB2 export (bridge to Z3 / cvc5 / Rust backends)
// ---------------------------------------------------------------------------

/**
 * Compile `requires ⇒ ensures` into an SMT-LIB2 script that asserts its
 * negation. `unsat` from the solver means the clause is proved for every
 * input/output pair allowed by the declared domains *and* by `implementation`
 * (an optional LSL expression describing the program's input→output relation).
 */
export function toSmtLib(model: Model, clause: { requires?: Expr; ensures: Expr }, implementation?: Expr): string {
  const lines: string[] = ["(set-logic ALL)"];
  const enums = new Map<string, string[]>();
  const sym = (name: string) => `|${name}|`;
  const declare = (prefix: "in" | "out", table: Model["inputs"]) => {
    for (const [field, d] of Object.entries(table)) {
      const name = `${prefix}.${field}`;
      const dom = domainOf(d);
      if (dom.type === "bool") lines.push(`(declare-const ${sym(name)} Bool)`);
      else if (dom.type === "int") {
        lines.push(`(declare-const ${sym(name)} Int)`);
        if (dom.min !== undefined) lines.push(`(assert (>= ${sym(name)} ${smtInt(dom.min)}))`);
        if (dom.max !== undefined) lines.push(`(assert (<= ${sym(name)} ${smtInt(dom.max)}))`);
      } else if (dom.type === "enum") {
        enums.set(name, dom.values.map(String));
        lines.push(`(declare-const ${sym(name)} Int)`);
        lines.push(`(assert (and (>= ${sym(name)} 0) (< ${sym(name)} ${dom.values.length})))`);
      } else lines.push(`(declare-const ${sym(name)} String)`);
    }
  };
  declare("in", model.inputs);
  declare("out", model.outputs);

  for (const [name, def] of Object.entries(model.defs ?? {})) {
    // Def params are untyped in v0; we only export defs whose params are Bool.
    const params = def.params.map((p) => `(${sym(p)} Bool)`).join(" ");
    lines.push(`(define-fun ${sym(name)} (${params}) Bool ${smt(def.body)})`);
  }

  function enumIndex(varExpr: Expr, other: Expr): [string, string] | null {
    if ("var" in varExpr && enums.has(varExpr.var) && "const" in other) {
      const idx = enums.get(varExpr.var)!.indexOf(String(other.const));
      return [sym(varExpr.var), String(idx)];
    }
    return null;
  }

  function smt(e: Expr): string {
    if ("const" in e) {
      if (typeof e.const === "boolean") return e.const ? "true" : "false";
      if (typeof e.const === "number") return smtInt(e.const);
      return JSON.stringify(e.const);
    }
    if ("var" in e) return sym(e.var);
    if ("call" in e) return `(${sym(e.call)} ${e.args.map(smt).join(" ")})`;
    const a = e.args;
    if (e.op === "eq" || e.op === "neq") {
      const pair = enumIndex(a[0]!, a[1]!) ?? enumIndex(a[1]!, a[0]!);
      const body = pair ? `(= ${pair[0]} ${pair[1]})` : `(= ${smt(a[0]!)} ${smt(a[1]!)})`;
      return e.op === "eq" ? body : `(not ${body})`;
    }
    if (e.op === "in") return `(or ${a.slice(1).map((x) => smt({ op: "eq", args: [a[0]!, x] })).join(" ")})`;
    const head: Record<Exclude<Op, "eq" | "neq" | "in">, string> = {
      and: "and",
      or: "or",
      not: "not",
      implies: "=>",
      iff: "=",
      lt: "<",
      le: "<=",
      gt: ">",
      ge: ">=",
      add: "+",
      sub: "-",
      mul: "*",
      ite: "ite",
    };
    return `(${head[e.op]} ${a.map(smt).join(" ")})`;
  }

  if (implementation) lines.push(`(assert ${smt(implementation)})`);
  const goal = clause.requires ? `(=> ${smt(clause.requires)} ${smt(clause.ensures)})` : smt(clause.ensures);
  lines.push(`(assert (not ${goal}))`, "(check-sat)", "(get-model)");
  return lines.join("\n");
}

function smtInt(n: number): string {
  return n < 0 ? `(- ${-n})` : String(n);
}
