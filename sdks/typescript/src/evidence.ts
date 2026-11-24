/**
 * Taking before and after snapshots on disk, and running driven scenarios and properties for a proof run (Node only).
 *
 * At extraction every intent gets a before snapshot of the feature it touches, on the current code; at the proof run
 * the same snapshot is taken on the new code as the after, compared with the before, and every example whose output
 * changed although no approved clause applies to its input becomes an unexpected change. Files go to
 * .lastro/docs/<proposal>/<feature>/<intent>/before|after/ (repository-wide ones to .lastro/docs/<proposal>/repository/)
 * and the proposal records the SHA-256 of each.
 */
import { execSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { canonicalJson } from "./artifact.js";
import { checkProperty, matches, openSession, runDrivenScenario, type DriverConfig, type Json } from "./drive.js";
import { isMachine, type MachineModel } from "./machine.js";
import { pixelShare } from "./png.js";
import { forwardIntents, type DriveRunner, type DrivenOutcome, type ProofCheck, type ProofEntry, type Proposal, type UseCase } from "./proposal.js";
import { sha256Hex } from "./sha256.js";
import {
  changedRows,
  exampleInputs,
  flattenSchema,
  flattenTables,
  inferSchema,
  modelContract,
  snapshotKinds,
  snapshotPath,
  structuralDiff,
  unexpectedChanges,
  type Comparison,
  type ExampleRow,
  type SnapshotRecord,
  type SnapshotSide,
  type UnexpectedChange,
} from "./snapshots.js";
import { holds, type Expr } from "./spec/expr.js";
import type { Env, Model, Subject } from "./types.js";
import type { Workspace } from "./workspace.js";

type Driven = ProofEntry & { check: Extract<ProofCheck, { kind: "driven" }> };
type Property = ProofEntry & { check: Extract<ProofCheck, { kind: "property" }> };

/** Runs driven scenarios and properties through the configured drivers; references are read from the workspace. */
export function driveRunner(ws: Workspace, drivers: DriverConfig | undefined, open = openSession): DriveRunner {
  return {
    async run(pr): Promise<DrivenOutcome> {
      if (pr.check.kind === "driven") {
        const c = pr.check;
        const r = await runDrivenScenario(
          {
            id: pr.id,
            kind: c.surface.kind,
            locator: c.surface.locator,
            steps: c.steps.map((s) => ({ input: s.input as Record<string, Json>, expect: s.expect as Record<string, Json>, ...(s.reference && existsSync(ws.file(s.reference)) ? { reference: readFileSync(ws.file(s.reference)) } : {}) })),
            ...(c.tolerance !== undefined ? { tolerance: c.tolerance } : {}),
          },
          drivers,
          (kind, locator, d) => open(kind, locator, d, ws.root),
        );
        const bad = r.steps.findIndex((x) => !x.passed);
        return {
          verdict: r.passed ? "evidenced" : "violated",
          passed: r.passed,
          ...(r.driverError ? { note: `driver error: ${r.driverError}` } : bad >= 0 ? { note: `step ${bad + 1}: ${r.steps[bad]!.problem}`, counterexample: { input: { step: bad + 1, ...(c.steps[bad]!.input as Env) }, output: (r.steps[bad]!.observed as Env) ?? null } } : {}),
        };
      }
      const c = (pr as Property).check;
      const r = await checkProperty(
        { id: pr.id, kind: c.surface.kind, locator: c.surface.locator, actions: c.actions as Record<string, Json>[], invariant: (obs) => safeHolds(c.invariant, obs) },
        drivers,
        (kind, locator, d) => open(kind, locator, d, ws.root),
      );
      return {
        verdict: r.verdict === "nondeterministic" ? "violated" : r.verdict,
        passed: r.verdict === "evidenced" || r.verdict === "proved",
        note: [
          r.verdict === "evidenced" ? `${r.sequences} sequences, ${r.actions} actions; failure rate below ${r.failureRateUpperBound95} (95%)` : r.verdict === "nondeterministic" ? "nondeterministic: the shortest failing prefix passed on replay" : undefined,
          r.driverError ? `driver error: ${r.driverError}` : undefined,
          r.note,
        ]
          .filter(Boolean)
          .join("; "),
        ...(r.prefix ? { counterexample: { input: { prefix: canonicalJson(r.prefix) }, output: null } } : {}),
      };
    },
  };
}

function safeHolds(e: Expr, observed: Record<string, Json>): boolean {
  try {
    return holds(e, { input: {}, output: observed as Env });
  } catch {
    return false;
  }
}

export interface SnapshotContext {
  ws: Workspace;
  models: Record<string, Model | MachineModel>;
  subjects: Record<string, Subject>;
  drivers?: DriverConfig;
  useCases: UseCase[];
  open?: typeof openSession;
}

interface Taken {
  record: SnapshotRecord;
  content: Uint8Array | string;
}

/** Take the snapshots of one side for every forward intent (at extraction: every intent) and record them. */
export async function takeSnapshots(p: Proposal, side: SnapshotSide, ctx: SnapshotContext): Promise<Proposal> {
  const intents = side === "before" ? p.intents : forwardIntents(p);
  const taken: Taken[] = [];
  const write = (path: string, content: Uint8Array | string, rec: Omit<SnapshotRecord, "path" | "sha256">) => {
    const bytes = typeof content === "string" ? new TextEncoder().encode(content) : content;
    taken.push({ record: { ...rec, path, sha256: sha256Hex(bytes) }, content });
  };
  const open = ctx.open ?? openSession;
  for (const it of intents) {
    const change = p.useCases.find((c) => c.useCase === it.useCase);
    const u = ctx.useCases.find((x) => x.id === it.useCase);
    if (side === "before" && !u) continue; // a new feature has no before
    if (side === "after" && change?.change === "remove") continue; // a removed one has no after
    const proofs = [...(u?.proofs ?? []), ...p.proofs.filter((x) => x.useCase === it.useCase)];
    const surfaces = (side === "after" ? change?.surfaces : u?.surfaces) ?? u?.surfaces ?? change?.surfaces ?? [];
    const io = [...new Set(proofs.filter((x) => x.check.kind === "clause" || x.check.kind === "scenario").map((x) => `${x.model}\u0000${x.subject}`))]
      .map((k) => k.split("\u0000") as [string, string])
      .filter(([m, s]) => ctx.models[m] && !isMachine(ctx.models[m]) && ctx.subjects[s]);
    const kinds = snapshotKinds({ surfaces, hasModel: io.length > 0, hasData: false });
    const at = { feature: it.useCase, intent: it.id };
    const rec = (kind: SnapshotRecord["kind"]) => ({ intent: it.id, feature: it.useCase, kind, side });
    const driven = proofs.filter((x): x is Driven => x.check.kind === "driven");
    if (kinds.includes("screen"))
      for (const pr of driven.filter((x) => x.check.surface.kind === "screen" || x.check.surface.kind === "route")) {
        const r = await runDrivenScenario({ id: pr.id, kind: pr.check.surface.kind, locator: pr.check.surface.locator, steps: pr.check.steps.map((s) => ({ input: s.input as Record<string, Json>, expect: {} })) }, ctx.drivers, (k, l, d) => open(k, l, d, ctx.ws.root));
        r.steps.forEach((s, i) => s.screenshot && write(snapshotPath(p.id, side, `${pr.slug}-${i + 1}.png`, at), s.screenshot, rec("screen")));
      }
    if (kinds.includes("contract")) {
      for (const pr of driven.filter((x) => x.check.surface.kind === "api" || x.check.surface.kind === "command")) {
        const r = await runDrivenScenario({ id: pr.id, kind: pr.check.surface.kind, locator: pr.check.surface.locator, steps: pr.check.steps.map((s) => ({ input: s.input as Record<string, Json>, expect: {} })) }, ctx.drivers, (k, l, d) => open(k, l, d, ctx.ws.root));
        if (r.driverError) continue;
        const schema = inferSchema(r.steps.map((s) => s.observed ?? null));
        write(snapshotPath(p.id, side, `contract-${pr.slug}.json`, at), json({ surface: pr.check.surface, schema, fields: flattenSchema(schema) }), rec("contract"));
      }
      for (const [m] of io) write(snapshotPath(p.id, side, `model-${m}.json`, at), json({ model: m, fields: modelContract(ctx.models[m] as Model) }), rec("contract"));
    }
    if (kinds.includes("examples"))
      for (const [m, s] of io) {
        const model = ctx.models[m] as Model;
        const scen = proofs.filter((x) => x.model === m && x.check.kind === "scenario").map((x) => (x.check as { input: Env }).input);
        const { inputs, wholeSpace } = exampleInputs(model, scen);
        const rows: ExampleRow[] = [];
        for (const input of inputs) {
          try {
            rows.push({ input, [side]: await ctx.subjects[s]!({ ...input }) });
          } catch (e) {
            rows.push({ input, [side]: { error: (e as Error).message } });
          }
        }
        write(snapshotPath(p.id, side, `examples-${m}.json`, at), json({ model: m, subject: s, wholeSpace, rows }), rec("examples"));
      }
  }
  if (ctx.drivers?.data) {
    try {
      const out = execSync(ctx.drivers.data.command, { cwd: ctx.drivers.data.cwd ?? ctx.ws.root, encoding: "utf8", timeout: ctx.drivers.timeoutMs ?? 30_000 });
      const data = JSON.parse(out) as { tables?: Record<string, Record<string, string>> };
      const fields = data.tables ? flattenTables(data.tables) : flattenSchema(data as Parameters<typeof flattenSchema>[0]);
      write(snapshotPath(p.id, side, "data.json", undefined), json({ data, fields }), { intent: "repository", kind: "data", side });
    } catch {
      /* the data command failed: no data snapshot this time */
    }
  }
  for (const t of taken) {
    const path = ctx.ws.file(t.record.path);
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, t.content);
  }
  const next = structuredClone(p);
  const keep = (next.snapshots ?? []).filter((x) => x.side !== side);
  next.snapshots = [...keep, ...taken.map((t) => t.record)].sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0));
  if (side === "after") Object.assign(next, compareSnapshots(next, ctx));
  return next;
}

const json = (v: unknown) => JSON.stringify(v, null, 2) + "\n";
const nameOf = (path: string) => path.slice(path.lastIndexOf("/") + 1);

/** Compare every after snapshot with its before; collect the unexpected example changes (keeping acceptances). */
export function compareSnapshots(p: Proposal, ctx: Pick<SnapshotContext, "ws">): Pick<Proposal, "comparisons" | "unexpected"> {
  const comparisons: Comparison[] = [];
  const unexpected: UnexpectedChange[] = [];
  const read = (rec: SnapshotRecord) => readFileSync(ctx.ws.file(rec.path));
  for (const after of (p.snapshots ?? []).filter((x) => x.side === "after")) {
    const before = (p.snapshots ?? []).find((x) => x.side === "before" && x.intent === after.intent && x.feature === after.feature && x.kind === after.kind && nameOf(x.path) === nameOf(after.path));
    if (!before) continue;
    const base = { intent: after.intent, ...(after.feature ? { feature: after.feature } : {}), kind: after.kind, name: nameOf(after.path) };
    if (after.kind === "screen") comparisons.push({ ...base, pixelShare: pixelShare(read(before), read(after)) });
    else if (after.kind === "contract" || after.kind === "data") {
      const diff = structuralDiff((JSON.parse(read(before).toString()) as { fields: Record<string, string> }).fields, (JSON.parse(read(after).toString()) as { fields: Record<string, string> }).fields);
      comparisons.push({ ...base, diff, breaking: diff.some((d) => d.breaking) });
    } else {
      const b = JSON.parse(read(before).toString()) as { model: string; rows: ExampleRow[] };
      const a = JSON.parse(read(after).toString()) as { rows: ExampleRow[] };
      const byInput = new Map(b.rows.map((r) => [canonicalJson(r.input), r.before]));
      const rows = a.rows.filter((r) => byInput.has(canonicalJson(r.input))).map((r) => ({ input: r.input, before: byInput.get(canonicalJson(r.input)), after: r.after }));
      const changed = changedRows(rows);
      comparisons.push({ ...base, changedRows: changed });
      const clauses = p.proofs
        .filter((x) => x.model === b.model && (x.check.kind === "clause" || x.check.kind === "scenario"))
        .map((x) => (x.check.kind === "clause" ? { ...(x.check.requires ? { requires: x.check.requires } : {}) } : { requires: equalsInput((x.check as { input: Env }).input) }));
      unexpected.push(...unexpectedChanges(changed, clauses, after.intent, after.feature, unexpected.length + 1));
    }
  }
  // An unexpected change already accepted (same intent and input) keeps its acceptances.
  for (const u of unexpected) {
    const old = (p.unexpected ?? []).find((x) => x.intent === u.intent && canonicalJson(x.input) === canonicalJson(u.input) && canonicalJson(x.after ?? null) === canonicalJson(u.after ?? null));
    if (old) u.acceptedBy = [...old.acceptedBy];
  }
  return { comparisons, unexpected };
}

function equalsInput(input: Env): Expr {
  const parts: Expr[] = Object.keys(input)
    .sort()
    .map((k) => ({ op: "eq", args: [{ var: `in.${k}` }, { const: input[k]! }] }));
  return parts.length === 1 ? parts[0]! : { op: "and", args: parts.length ? parts : [{ const: true }] };
}

export { matches };
