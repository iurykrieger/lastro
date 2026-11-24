/**
 * Before and after snapshots, and what a reviewer must still do before a proposal is proved.
 *
 * Every intent gets a before snapshot of each feature it touches, taken on the current code when the proposal is
 * extracted, and the same snapshot on the new code when it is proved (the after). A snapshot has one of four kinds:
 *
 *   screen    the images the driver captured while running a screen or route surface's scenarios
 *   contract  the request and response shapes an api or command surface reports (JSON Schema), and for every feature
 *             with a model, the model's inputs and outputs with their domains
 *   data      the structure of the data the feature reads or writes, as the driver reports it
 *   examples  example inputs (the scenarios' plus the model's whole space up to 1,000 points, or a fixed-seed sample
 *             of 1,000) with the before and after outputs side by side
 *
 * Comparisons are deterministic: the share of pixels changed for screens, a structural diff for contracts and data
 * (a removal or a type change is breaking), the changed rows for examples. An example whose output changed although
 * no approved clause applies to its input is an unexpected change: the proposal is not proved until reviewers accept
 * it. Pure: reading and writing the files is in drive.ts and the workspace.
 */
import { canonicalJson } from "./artifact.js";
import { enumerateSpace, rng, sampleSpace, spaceSize, type Domain } from "./spec/domain.js";
import { holds, LslError } from "./spec/expr.js";
import type { Env, Model } from "./types.js";
import type { Surface } from "./features.js";

export type SnapshotKind = "screen" | "contract" | "data" | "examples";
export type SnapshotSide = "before" | "after";

/** One snapshot file, as the proposal records it. */
export interface SnapshotRecord {
  /** The intent that changed the feature, or "repository" for a whole-repository snapshot. */
  intent: string;
  /** The feature (use case) id; absent on a repository snapshot. */
  feature?: string;
  kind: SnapshotKind;
  side: SnapshotSide;
  /** Relative to the workspace folder: docs/<proposal>/<feature>/<intent>/<side>/<file>. */
  path: string;
  sha256: string;
}

export interface Comparison {
  intent: string;
  feature?: string;
  kind: SnapshotKind;
  /** The file name both sides share. */
  name: string;
  /** screen: the share of pixels changed, 0..1. */
  pixelShare?: number;
  /** contract and data: every field or column added, removed or changed in type. */
  diff?: FieldChange[];
  breaking?: boolean;
  /** examples: the rows whose output changed. */
  changedRows?: ExampleRow[];
}

export interface FieldChange {
  path: string;
  change: "added" | "removed" | "type";
  before?: string;
  after?: string;
  breaking: boolean;
}

export interface ExampleRow {
  input: Env;
  before?: Env | { error: string };
  after?: Env | { error: string };
}

/** An example whose output changed although no approved clause applies to its input. */
export interface UnexpectedChange {
  id: string; // "U1"
  intent: string;
  feature?: string;
  input: Env;
  before?: Env | { error: string };
  after?: Env | { error: string };
  acceptedBy: string[];
}

/** A reviewer's look at an intent beyond proof for human judgment, after the proof run. */
export interface SignOff {
  intent: string;
  by: string;
  verdict: "signed-off" | "changes-requested";
  note?: string;
  /** Screenshots (snapshot paths) the sign-off makes the reference of their scenario step. */
  references?: string[];
}

/** The kinds of snapshot a feature gets, from its surfaces, its model and whether its driver reports data. */
export function snapshotKinds(f: { surfaces?: Surface[]; hasModel: boolean; hasData: boolean }): SnapshotKind[] {
  const kinds = new Set((f.surfaces ?? []).map((s) => s.kind));
  const out: SnapshotKind[] = [];
  if (kinds.has("screen") || kinds.has("route")) out.push("screen");
  if (kinds.has("api") || kinds.has("command") || f.hasModel) out.push("contract");
  if (f.hasData) out.push("data");
  if (f.hasModel) out.push("examples");
  return out;
}

/** The side a snapshot taken at this moment is on: extraction takes the before, the proof run the after. */
export const sideOf = (phase: "extract" | "prove"): SnapshotSide => (phase === "extract" ? "before" : "after");

/** Where a snapshot file lives, relative to the workspace folder (.lastro/). */
export function snapshotPath(proposal: string, side: SnapshotSide, file: string, at?: { feature: string; intent: string }): string {
  const safe = (s: string) => s.replace(/[^A-Za-z0-9._-]/g, "_");
  return at ? `docs/${proposal}/${safe(at.feature)}/${safe(at.intent.replace(`${proposal}.`, ""))}/${side}/${safe(file)}` : `docs/${proposal}/repository/${side}/${safe(file)}`;
}

// ---------------------------------------------------------------------------
// Contracts and data: structural diff
// ---------------------------------------------------------------------------

type Schema = { type?: string | string[]; properties?: Record<string, Schema>; items?: Schema; enum?: unknown[]; minimum?: number; maximum?: number };

/** A JSON Schema flattened to field paths and their types ("body.items[].id" -> "integer"). */
export function flattenSchema(s: Schema, at = "", out: Record<string, string> = {}): Record<string, string> {
  const type = Array.isArray(s.type) ? [...s.type].sort().join("|") : s.type ?? (s.properties ? "object" : s.enum ? "enum" : "any");
  if (at) out[at] = s.enum ? `${type}${canonicalJson([...s.enum].map(String).sort())}` : type;
  for (const k of Object.keys(s.properties ?? {}).sort()) flattenSchema(s.properties![k]!, at ? `${at}.${k}` : k, out);
  if (s.items) flattenSchema(s.items, `${at}[]`, out);
  return out;
}

/** Tables and columns flattened the same way ("orders.total" -> "numeric"). */
export function flattenTables(t: Record<string, Record<string, string>>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const table of Object.keys(t).sort()) {
    out[table] = "table";
    for (const col of Object.keys(t[table]!).sort()) out[`${table}.${col}`] = t[table]![col]!;
  }
  return out;
}

/** Every field added, removed or changed in type, in path order; a removal or a type change is breaking. */
export function structuralDiff(before: Record<string, string>, after: Record<string, string>): FieldChange[] {
  const paths = [...new Set([...Object.keys(before), ...Object.keys(after)])].sort();
  const out: FieldChange[] = [];
  for (const p of paths) {
    const b = before[p];
    const a = after[p];
    if (b === a) continue;
    if (b === undefined) out.push({ path: p, change: "added", after: a, breaking: false });
    else if (a === undefined) out.push({ path: p, change: "removed", before: b, breaking: true });
    else out.push({ path: p, change: "type", before: b, after: a, breaking: true });
  }
  return out;
}

/** The JSON Schema of a value, inferred structurally (used for responses an HTTP driver observed). */
export function inferSchema(v: unknown): Schema {
  if (v === null) return { type: "null" };
  if (Array.isArray(v)) {
    const items = v.map(inferSchema);
    return items.length ? { type: "array", items: mergeSchemas(items) } : { type: "array" };
  }
  if (typeof v === "object") {
    const props: Record<string, Schema> = {};
    for (const k of Object.keys(v as object).sort()) props[k] = inferSchema((v as Record<string, unknown>)[k]);
    return { type: "object", properties: props };
  }
  if (typeof v === "number") return { type: Number.isInteger(v) ? "integer" : "number" };
  return { type: typeof v };
}

function mergeSchemas(xs: Schema[]): Schema {
  const types = [...new Set(xs.flatMap((x) => (Array.isArray(x.type) ? x.type : [x.type ?? "any"])))].sort();
  const props: Record<string, Schema[]> = {};
  const items: Schema[] = [];
  for (const x of xs) {
    for (const [k, s] of Object.entries(x.properties ?? {})) (props[k] ??= []).push(s);
    if (x.items) items.push(x.items);
  }
  return {
    type: types.length === 1 ? types[0] : types,
    ...(Object.keys(props).length ? { properties: Object.fromEntries(Object.keys(props).sort().map((k) => [k, mergeSchemas(props[k]!)])) } : {}),
    ...(items.length ? { items: mergeSchemas(items) } : {}),
  };
}

/** A model's inputs and outputs with their domains, as a contract. */
export function modelContract(m: Model): Record<string, string> {
  const show = (d: Domain) =>
    d.type === "enum" ? `enum${canonicalJson([...d.values].map(String))}` : d.type === "int" ? `int[${d.min ?? ""}..${d.max ?? ""}]` : d.type;
  const out: Record<string, string> = {};
  for (const k of Object.keys(m.inputs).sort()) out[`in.${k}`] = show(m.inputs[k]!);
  for (const k of Object.keys(m.outputs).sort()) out[`out.${k}`] = show(m.outputs[k]!);
  return out;
}

// ---------------------------------------------------------------------------
// Examples
// ---------------------------------------------------------------------------

/** The whole space is used when it has at most this many points; otherwise a fixed-seed sample of this many. */
export const EXAMPLES_LIMIT = 1000;

/** Example inputs: the scenarios' inputs, then the model's whole space (up to 1,000 points) or a fixed-seed sample. */
export function exampleInputs(model: Model, scenarioInputs: Env[] = [], seed = 42): { inputs: Env[]; wholeSpace: boolean } {
  const seen = new Set<string>();
  const out: Env[] = [];
  const add = (e: Env) => {
    const k = canonicalJson(e);
    if (!seen.has(k)) {
      seen.add(k);
      out.push(e);
    }
  };
  scenarioInputs.forEach(add);
  const size = spaceSize(model.inputs);
  if (size <= EXAMPLES_LIMIT) {
    for (const e of enumerateSpace(model.inputs)) add(e);
    return { inputs: out, wholeSpace: true };
  }
  const next = rng(seed);
  for (let k = 0; k < EXAMPLES_LIMIT; k++) add(sampleSpace(model.inputs, next));
  return { inputs: out, wholeSpace: false };
}

/** The rows whose output changed between the before and the after code. */
export function changedRows(rows: ExampleRow[]): ExampleRow[] {
  return rows.filter((r) => canonicalJson(r.before ?? null) !== canonicalJson(r.after ?? null));
}

/**
 * The changed rows no approved clause explains: for each, no clause of the proposal's proofs on that model has an
 * if-part that holds for the input (a clause with no if-part applies to every input).
 */
export function unexpectedChanges(rows: ExampleRow[], clauses: { requires?: Parameters<typeof holds>[0] }[], intent: string, feature?: string, start = 1): UnexpectedChange[] {
  const out: UnexpectedChange[] = [];
  for (const r of changedRows(rows)) {
    const output = r.after && !("error" in r.after) ? r.after : {};
    const explained = clauses.some((c) => {
      if (!c.requires) return true;
      try {
        return holds(c.requires, { input: r.input, output: output as Env });
      } catch (e) {
        if (e instanceof LslError) return false;
        throw e;
      }
    });
    if (!explained) out.push({ id: `U${start + out.length}`, intent, ...(feature ? { feature } : {}), input: r.input, ...(r.before ? { before: r.before } : {}), ...(r.after ? { after: r.after } : {}), acceptedBy: [] });
  }
  return out;
}

// ---------------------------------------------------------------------------
// What still blocks "proved"
// ---------------------------------------------------------------------------

/**
 * Sign-offs due and unexpected changes not accepted yet. Each needs the policy's number of distinct reviewers.
 * `judgment` lists the intents beyond proof for human judgment.
 */
export function reviewDue(judgment: string[], signoffs: SignOff[], unexpected: UnexpectedChange[], required: number): { signoffs: string[]; unexpected: string[]; changesRequested: string[] } {
  const need = Math.max(1, required);
  const changesRequested = judgment.filter((i) => signoffs.some((s) => s.intent === i && s.verdict === "changes-requested"));
  const due = judgment.filter((i) => !changesRequested.includes(i) && new Set(signoffs.filter((s) => s.intent === i && s.verdict === "signed-off").map((s) => s.by)).size < need);
  const open = unexpected.filter((u) => new Set(u.acceptedBy).size < need).map((u) => u.id);
  return { signoffs: due, unexpected: open, changesRequested };
}
