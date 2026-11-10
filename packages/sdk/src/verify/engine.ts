/**
 * The verification engine: contrasts a ProofPlan with the program's final
 * output (a decision function and/or recorded traces).
 *
 * - Finite input space  -> exhaustive enumeration  -> `proved` (formal).
 * - Otherwise           -> seeded sampling         -> `evidenced` (empirical, rule of three).
 * - Every clause must kill at least one planted mutant, or it is `weak`.
 * - Clauses whose if-part never holds are `vacuous`.
 * - Jointly unsatisfiable clauses become decision cards (spec contradiction).
 */
import { approvedIntents, hashOf } from "../artifact.js";
import { checkPlan, PlanError } from "../plan.js";
import { evaluate, holds, LslError } from "../spec/expr.js";
import {
  enumerateDomain,
  enumerateSpace,
  inDomain,
  domainSize,
  rng,
  sampleSpace,
  spaceSize,
} from "../spec/domain.js";
import type {
  Clause,
  ClauseResult,
  DecisionCard,
  Env,
  IntentArtifact,
  IntentResult,
  IntentVerdict,
  Model,
  ProofPlan,
  ScenarioResult,
  Subject,
  Trace,
  Value,
  VerificationReport,
} from "../types.js";

export interface VerifyOptions {
  subject?: Subject;
  traces?: Trace[];
  /** Max input-space size enumerated exhaustively. */
  exhaustiveLimit?: number;
  samples?: number;
  seed?: number;
  mutation?: boolean;
  /** Max output-space size for the consistency check. */
  consistencyLimit?: number;
}

interface Run {
  input: Env;
  output: Env;
}

// ---------------------------------------------------------------------------
// Single-run explanation (used by UIs: "3 of 8 clauses apply, all hold")
// ---------------------------------------------------------------------------

export type ClauseStatus = "if-part-false" | "holds" | "fails" | "error";

export interface ClauseExplanation {
  clauseId: string;
  status: ClauseStatus;
  error?: string;
}

export function explainRun(plan: ProofPlan, input: Env, output: Env): ClauseExplanation[] {
  return plan.clauses.map((c) => {
    try {
      const ctx = { input, output, defs: plan.model.defs };
      if (c.requires && !holds(c.requires, ctx)) return { clauseId: c.id, status: "if-part-false" };
      return { clauseId: c.id, status: holds(c.ensures, ctx) ? "holds" : "fails" };
    } catch (e) {
      return { clauseId: c.id, status: "error", error: (e as Error).message };
    }
  });
}

// ---------------------------------------------------------------------------
// Spec consistency (no implementation needed)
// ---------------------------------------------------------------------------

/**
 * For every input, is there at least one output satisfying all applicable
 * clauses? If not, the spec contradicts itself and the team must decide.
 */
export function checkConsistency(plan: ProofPlan, opts: VerifyOptions = {}): DecisionCard[] {
  const limit = opts.consistencyLimit ?? 4096;
  if (!plan.clauses.length || spaceSize(plan.model.outputs) > limit) return [];
  const outputs = [...enumerateSpace(plan.model.outputs)];
  const inputs = inputSet(plan.model, opts).inputs;
  const cards: DecisionCard[] = [];
  const seen = new Set<string>();
  for (const input of inputs) {
    // Clauses that can apply to this input (their if-part may mention outputs).
    const applicable = plan.clauses.filter((c) => !c.requires || outputs.some((o) => safeHolds(c.requires!, input, o, plan)));
    const ok = outputs.some((output) =>
      plan.clauses.every((c) => {
        const ctx = { input, output, defs: plan.model.defs };
        return !c.requires || !holds(c.requires, ctx) ? true : holds(c.ensures, ctx);
      }),
    );
    if (ok) continue;
    const key = applicable.map((c) => c.id).join(",");
    if (seen.has(key)) continue;
    seen.add(key);
    cards.push({
      id: `X${cards.length + 1}`,
      intentIds: [...new Set(applicable.flatMap((c) => c.intentIds))],
      question: `Clauses ${key} cannot all hold for input ${JSON.stringify(input)}. Which one should give way?`,
      options: applicable.map((c) => ({ label: `Relax ${c.id}`, consequence: c.description })),
      recommended: 0,
      origin: "consistency-check",
    });
  }
  return cards;
}

function safeHolds(e: Clause["ensures"], input: Env, output: Env, plan: ProofPlan): boolean {
  try {
    return holds(e, { input, output, defs: plan.model.defs });
  } catch {
    return true;
  }
}

// ---------------------------------------------------------------------------
// Main entry point
// ---------------------------------------------------------------------------

function inputSet(model: Model, opts: VerifyOptions): { inputs: Env[]; exhaustive: boolean } {
  const limit = opts.exhaustiveLimit ?? 100_000;
  if (spaceSize(model.inputs) <= limit) return { inputs: [...enumerateSpace(model.inputs)], exhaustive: true };
  const next = rng(opts.seed ?? 42);
  const n = opts.samples ?? 2000;
  return { inputs: Array.from({ length: n }, () => sampleSpace(model.inputs, next)), exhaustive: false };
}

export async function verify(artifact: IntentArtifact, plan: ProofPlan, opts: VerifyOptions = {}): Promise<VerificationReport> {
  const check = checkPlan(artifact, plan);
  if (check.errors.length) throw new PlanError(check.errors);
  const { model } = plan;
  const defs = model.defs;

  // 1. Run the program over the input set.
  let runs: Run[] = [];
  let exhaustive = false;
  const runErrors: string[] = [];
  if (opts.subject) {
    const set = inputSet(model, opts);
    exhaustive = set.exhaustive;
    for (const input of set.inputs) {
      try {
        const output = await opts.subject({ ...input });
        for (const [k, d] of Object.entries(model.outputs))
          if (!inDomain(output[k], d)) runErrors.push(`output '${k}'=${JSON.stringify(output[k])} outside its domain for ${JSON.stringify(input)}`);
        runs.push({ input, output });
      } catch (e) {
        runErrors.push(`subject threw for ${JSON.stringify(input)}: ${(e as Error).message}`);
      }
    }
    if (runErrors.length) runs = runs.filter((r) => Object.entries(model.outputs).every(([k, d]) => inDomain(r.output[k], d)));
  }

  // 2. Clauses.
  const mutants = opts.mutation === false ? [] : outputMutants(model);
  const clauses: ClauseResult[] = plan.clauses.map((c) => checkClause(c, runs, exhaustive, mutants, opts.traces ?? [], model));
  if (runErrors.length)
    for (const r of clauses) r.note = [r.note, `${runErrors.length} invalid runs (e.g. ${runErrors[0]})`].filter(Boolean).join("; ");

  // 3. Scenarios (Given/When/Then).
  const scenarios: ScenarioResult[] = [];
  for (const s of plan.scenarios) {
    if (!opts.subject) {
      scenarios.push({ scenarioId: s.id, status: "failed", error: "no subject to run" });
      continue;
    }
    try {
      const input = s.input as Env;
      const output = await opts.subject({ ...input });
      const ok = holds(s.expect, { input, output, defs });
      scenarios.push({ scenarioId: s.id, status: ok ? "passed" : "failed", output });
    } catch (e) {
      scenarios.push({ scenarioId: s.id, status: "failed", error: (e as Error).message });
    }
  }

  // 4. Spec consistency.
  const decisions = checkConsistency(plan, opts);

  // 5. Roll up per intent.
  const intents = rollup(artifact, plan, clauses, scenarios, check.uncovered);
  const summary = { proved: 0, "proved-under-assumptions": 0, evidenced: 0, violated: 0, "beyond-proof": 0, gap: 0 } as Record<IntentVerdict, number>;
  intents.forEach((i) => summary[i.verdict]++);

  return {
    lastro: "verification-report/v0",
    artifactHash: artifact.hash!,
    planHash: hashOf(plan),
    model: model.name,
    clauses,
    scenarios,
    intents,
    assumptions: plan.assumptions,
    beyondProof: plan.beyondProof,
    notPromised: artifact.notPromised,
    decisions,
    summary,
    ok: summary.violated === 0 && summary.gap === 0 && decisions.length === 0,
  };
}

// ---------------------------------------------------------------------------
// Clause checking
// ---------------------------------------------------------------------------

interface Mutant {
  label: string;
  apply: (o: Env) => Env;
}

/** "Plant one bug" on the output side: force a field to a constant, or negate a boolean. */
function outputMutants(model: Model): Mutant[] {
  const ms: Mutant[] = [];
  for (const [field, d] of Object.entries(model.outputs)) {
    if (d.type === "bool") ms.push({ label: `¬${field}`, apply: (o) => ({ ...o, [field]: !o[field] }) });
    if (domainSize(d) <= 16)
      for (const v of enumerateDomain(d)) ms.push({ label: `${field}:=${JSON.stringify(v)}`, apply: (o) => ({ ...o, [field]: v as Value }) });
    if (d.type === "int")
      for (const delta of [1, -1]) ms.push({ label: `${field}${delta > 0 ? "+" : "-"}1`, apply: (o) => ({ ...o, [field]: (o[field] as number) + delta }) });
  }
  return ms;
}

function checkClause(c: Clause, runs: Run[], exhaustive: boolean, mutants: Mutant[], traces: Trace[], model: Model): ClauseResult {
  const defs = model.defs;
  const applies = (r: Run) => !c.requires || holds(c.requires, { input: r.input, output: r.output, defs });
  const ok = (r: Run) => holds(c.ensures, { input: r.input, output: r.output, defs });

  const result: ClauseResult = { clauseId: c.id, verdict: "not-run", evidence: "none", method: "none", checked: 0 };
  try {
    if (runs.length) {
      const applicable = runs.filter(applies);
      result.checked = applicable.length;
      result.method = exhaustive ? "exhaustive" : "sampling";
      const bad = applicable.find((r) => !ok(r));
      if (bad) {
        result.verdict = "violated";
        result.evidence = exhaustive ? "formal" : "empirical";
        result.counterexample = { input: bad.input, output: bad.output };
      } else if (!applicable.length) {
        result.verdict = "vacuous";
        result.note = "no run makes the if-part true: the clause holds vacuously and proves nothing";
      } else if (exhaustive) {
        result.verdict = "proved";
        result.evidence = "formal";
      } else {
        result.verdict = "evidenced";
        result.evidence = "empirical";
        result.failureRateUpperBound95 = Math.min(1, 3 / applicable.length);
      }

      // Mutation check: the clause must reject at least one planted bug.
      if (mutants.length && (result.verdict === "proved" || result.verdict === "evidenced")) {
        let killed = 0;
        for (const m of mutants) {
          const caught = runs.some((r) => {
            const mr = { input: r.input, output: m.apply(r.output) };
            return applies(mr) && !ok(mr);
          });
          if (caught) killed++;
        }
        result.mutants = { total: mutants.length, killed };
        if (killed === 0) {
          result.verdict = "weak";
          result.note = "no planted mutant was rejected: the clause verifies but rules out no bug (vibe verification)";
        }
      }
    }

    // Production traces: the model proof can hold while reality disagrees.
    if (traces.length) {
      const tRuns = traces.map((t) => ({ input: t.input, output: t.output, ref: t.ref }));
      const applicable = tRuns.filter(applies);
      result.tracesChecked = applicable.length;
      const bad = tRuns.find((r) => applies(r) && !ok(r));
      if (bad) {
        const provedBefore = result.verdict === "proved";
        result.verdict = "violated";
        result.evidence = "empirical";
        result.traceViolation = bad;
        if (provedBefore)
          result.note = "proved on the model, but a real trace violates it: a trusted piece (assumption) does not hold";
      } else if (!runs.length && applicable.length) {
        result.method = "traces";
        result.checked = applicable.length;
        result.verdict = "evidenced";
        result.evidence = "empirical";
        result.failureRateUpperBound95 = Math.min(1, 3 / applicable.length);
      }
    }
  } catch (e) {
    if (!(e instanceof LslError)) throw e;
    result.verdict = "violated";
    result.note = `erro ao avaliar: ${e.message}`;
  }
  return result;
}

// ---------------------------------------------------------------------------
// Intent roll-up
// ---------------------------------------------------------------------------

function rollup(
  artifact: IntentArtifact,
  plan: ProofPlan,
  clauses: ClauseResult[],
  scenarios: ScenarioResult[],
  uncovered: string[],
): IntentResult[] {
  return approvedIntents(artifact).map((intent) => {
    const id = intent.id;
    const cIds = plan.clauses.filter((c) => c.intentIds.includes(id)).map((c) => c.id);
    const cRes = clauses.filter((r) => cIds.includes(r.clauseId));
    const aIds = plan.assumptions.filter((a) => a.intentIds.includes(id)).map((a) => a.id);
    const assumedByClauses = plan.clauses.filter((c) => cIds.includes(c.id)).flatMap((c) => c.assumes ?? []);
    const allAssumptions = [...new Set([...aIds, ...assumedByClauses])];
    const sIds = plan.scenarios.filter((s) => s.intentIds.includes(id)).map((s) => s.id);
    const sRes = scenarios.filter((r) => sIds.includes(r.scenarioId));
    const beyond = plan.beyondProof.filter((b) => b.intentId === id);
    const notes: string[] = [];

    const base = { intentId: id, statement: intent.statement, clauses: cIds, assumptions: allAssumptions, scenarios: sIds, notes };
    const evidence = [...new Set(cRes.map((r) => r.evidence).filter((e) => e !== "none"))];
    if (allAssumptions.length) evidence.push("trusted");
    if (sRes.length && !evidence.includes("empirical")) evidence.push("empirical");

    if (uncovered.includes(id)) return { ...base, verdict: "gap", evidence: ["none"], confidence: 0, notes: ["approved intent with no derived obligation"] };

    const violated = cRes.filter((r) => r.verdict === "violated");
    const failedScenarios = sRes.filter((r) => r.status === "failed");
    if (violated.length || failedScenarios.length) {
      violated.forEach((r) => notes.push(`${r.clauseId} violated`));
      failedScenarios.forEach((r) => notes.push(`${r.scenarioId} failed${r.error ? `: ${r.error}` : ""}`));
      return { ...base, verdict: "violated", evidence, confidence: 0 };
    }

    cRes.filter((r) => r.verdict === "weak" || r.verdict === "vacuous").forEach((r) => notes.push(`${r.clauseId} ${r.verdict}: ${r.note}`));
    cRes.filter((r) => r.verdict === "not-run").forEach((r) => notes.push(`${r.clauseId} not run (no subject/traces)`));
    const useful = cRes.filter((r) => r.verdict === "proved" || r.verdict === "evidenced");

    if (beyond.length && !useful.length) {
      beyond.forEach((b) => notes.push(`beyond proof (${b.reason}): ${b.note}`));
      if (sRes.length) notes.push(`${sRes.length} scenario(s) passed as partial evidence`);
      return { ...base, verdict: "beyond-proof", evidence: evidence.length ? evidence : ["none"], confidence: 0 };
    }

    if (!useful.length) {
      if (sRes.length) {
        notes.push("examples only (scenarios), no sampling: confidence not quantified");
        return { ...base, verdict: "evidenced", evidence, confidence: 0 };
      }
      if (allAssumptions.length) {
        notes.push("backed by trusted pieces only");
        return { ...base, verdict: "evidenced", evidence, confidence: 0 };
      }
      return { ...base, verdict: "gap", evidence: ["none"], confidence: 0 };
    }

    if (useful.every((r) => r.verdict === "proved") && useful.length === cRes.length) {
      return { ...base, verdict: allAssumptions.length ? "proved-under-assumptions" : "proved", evidence, confidence: 1 };
    }
    const bound = Math.max(...useful.map((r) => r.failureRateUpperBound95 ?? 0));
    if (useful.length !== cRes.length) notes.push("not every clause carries a useful guarantee");
    return { ...base, verdict: "evidenced", evidence, confidence: 1 - bound };
  });
}

export { evaluate };
