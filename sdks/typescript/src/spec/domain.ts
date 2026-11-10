/**
 * Finite and infinite value domains for model inputs/outputs.
 *
 * A model whose inputs are all finite (and small enough) can be verified by
 * exhaustive enumeration, which is a complete proof over the declared domain.
 */
import type { Env, Value } from "../types.js";

export type Domain =
  | { type: "bool" }
  | { type: "enum"; values: (string | number)[] }
  | { type: "int"; min?: number; max?: number }
  | { type: "string"; examples?: string[] };

export const D = {
  bool: (): Domain => ({ type: "bool" }),
  enum: (...values: (string | number)[]): Domain => ({ type: "enum", values }),
  int: (min?: number, max?: number): Domain => ({ type: "int", min, max }),
  string: (...examples: string[]): Domain => ({ type: "string", examples }),
};

export function domainOf(d: Domain): Domain {
  return d;
}

export function domainSize(d: Domain): number {
  switch (d.type) {
    case "bool":
      return 2;
    case "enum":
      return d.values.length;
    case "int":
      return d.min !== undefined && d.max !== undefined ? Math.max(0, d.max - d.min + 1) : Infinity;
    case "string":
      return Infinity;
  }
}

export function spaceSize(table: Record<string, Domain>): number {
  return Object.values(table).reduce((n, d) => n * domainSize(d), 1);
}

export function* enumerateDomain(d: Domain): Generator<Value> {
  switch (d.type) {
    case "bool":
      yield false;
      yield true;
      return;
    case "enum":
      yield* d.values;
      return;
    case "int":
      if (d.min === undefined || d.max === undefined) throw new Error("cannot enumerate unbounded int");
      for (let i = d.min; i <= d.max; i++) yield i;
      return;
    case "string":
      throw new Error("cannot enumerate string domain");
  }
}

/** Cartesian product of a record of finite domains, in a deterministic order. */
export function* enumerateSpace(table: Record<string, Domain>): Generator<Env> {
  const keys = Object.keys(table).sort();
  const values = keys.map((k) => [...enumerateDomain(table[k]!)]);
  if (values.some((v) => v.length === 0)) return;
  const idx = keys.map(() => 0);
  while (true) {
    const env: Env = {};
    keys.forEach((k, i) => (env[k] = values[i]![idx[i]!]!));
    yield env;
    let i = keys.length - 1;
    while (i >= 0) {
      idx[i]!++;
      if (idx[i]! < values[i]!.length) break;
      idx[i] = 0;
      i--;
    }
    if (i < 0) return;
  }
}

/** Deterministic PRNG (mulberry32). Same seed, same samples, same verdict. */
export function rng(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export function sampleDomain(d: Domain, next: () => number): Value {
  const pick = <T>(xs: T[]) => xs[Math.floor(next() * xs.length)]!;
  switch (d.type) {
    case "bool":
      return next() < 0.5;
    case "enum":
      return pick(d.values);
    case "int": {
      const min = d.min ?? -1_000_000;
      const max = d.max ?? 1_000_000;
      // Bias toward boundaries, where bugs live.
      const r = next();
      if (r < 0.1) return min;
      if (r < 0.2) return max;
      if (r < 0.3 && min <= 0 && max >= 0) return 0;
      return min + Math.floor(next() * (max - min + 1));
    }
    case "string": {
      const ex = d.examples ?? [];
      if (ex.length && next() < 0.7) return pick(ex);
      const len = Math.floor(next() * 12);
      return Array.from({ length: len }, () => String.fromCharCode(97 + Math.floor(next() * 26))).join("");
    }
  }
}

export function sampleSpace(table: Record<string, Domain>, next: () => number): Env {
  const env: Env = {};
  for (const k of Object.keys(table).sort()) env[k] = sampleDomain(table[k]!, next);
  return env;
}

/** Alternative values for a field, used to plant mutants (one bug per mutant). */
export function mutationsOf(v: Value, d: Domain): Value[] {
  switch (d.type) {
    case "bool":
      return [!v];
    case "enum":
      return d.values.filter((x) => x !== v);
    case "int": {
      const n = v as number;
      return [n + 1, n - 1, 0].filter((x, i, a) => x !== n && a.indexOf(x) === i);
    }
    case "string":
      return [String(v) + "_mut", ""].filter((x) => x !== v);
  }
}

export function inDomain(v: unknown, d: Domain): boolean {
  switch (d.type) {
    case "bool":
      return typeof v === "boolean";
    case "enum":
      return d.values.includes(v as string | number);
    case "int":
      return (
        typeof v === "number" &&
        Number.isInteger(v) &&
        (d.min === undefined || v >= d.min) &&
        (d.max === undefined || v <= d.max)
      );
    case "string":
      return typeof v === "string";
  }
}
