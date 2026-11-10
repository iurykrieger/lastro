/**
 * Deterministic checks on a derived ProofPlan. The deriver may be an LLM; this
 * is what keeps its output honest before anything is verified.
 */
import { approvedIntents, isIntact } from "./artifact.js";
import { validateExpr } from "./spec/expr.js";
import { inDomain } from "./spec/domain.js";
import type { IntentArtifact, ProofPlan } from "./types.js";
import { BEYOND_PROOF_REASONS } from "./types.js";

export interface PlanCheck {
  errors: string[];
  /** Approved intents with no obligation at all. They become `gap` in the report. */
  uncovered: string[];
}

export class PlanError extends Error {
  constructor(public readonly errors: string[]) {
    super(`Invalid proof plan:\n - ${errors.join("\n - ")}`);
  }
}

export function checkPlan(artifact: IntentArtifact, plan: ProofPlan): PlanCheck {
  const errors: string[] = [];
  if (!artifact.hash || !isIntact(artifact)) errors.push("artifact is not frozen or was modified after freeze()");
  if (plan.artifactHash !== artifact.hash)
    errors.push(`plan was derived from ${plan.artifactHash}, artifact is ${artifact.hash}`);

  const approved = new Set(approvedIntents(artifact).map((i) => i.id));
  const known = new Set(artifact.intents.map((i) => i.id));
  const refIntents = (where: string, ids: string[]) => {
    if (!ids.length) errors.push(`${where}: must reference at least one intent`);
    for (const id of ids) {
      if (!known.has(id)) errors.push(`${where}: unknown intent ${id}`);
      else if (!approved.has(id)) errors.push(`${where}: intent ${id} was not approved (rejected intents must not be built)`);
    }
  };

  const seen = new Set<string>();
  const unique = (id: string) => {
    if (seen.has(id)) errors.push(`duplicate id ${id}`);
    seen.add(id);
  };

  const { model } = plan;
  if (!Object.keys(model.outputs).length) errors.push("model has no outputs");
  for (const [name, def] of Object.entries(model.defs ?? {}))
    errors.push(...validateExpr(def.body, model, def.params, `def ${name}`));

  const assumptionIds = new Set(plan.assumptions.map((a) => a.id));
  const scenarioIds = new Set(plan.scenarios.map((s) => s.id));

  for (const c of plan.clauses) {
    unique(c.id);
    refIntents(c.id, c.intentIds);
    if (c.requires) errors.push(...validateExpr(c.requires, model, [], `${c.id}.requires`));
    errors.push(...validateExpr(c.ensures, model, [], `${c.id}.ensures`));
    for (const t of c.assumes ?? []) if (!assumptionIds.has(t)) errors.push(`${c.id}: unknown assumption ${t}`);
  }
  for (const a of plan.assumptions) {
    unique(a.id);
    refIntents(a.id, a.intentIds);
    for (const s of a.checkedBy) if (!scenarioIds.has(s)) errors.push(`${a.id}: checkedBy unknown scenario ${s}`);
  }
  for (const s of plan.scenarios) {
    unique(s.id);
    refIntents(s.id, s.intentIds);
    errors.push(...validateExpr(s.expect, model, [], `${s.id}.expect`));
    for (const [k, d] of Object.entries(model.inputs)) {
      if (!(k in s.input)) errors.push(`${s.id}: input '${k}' missing`);
      else if (!inDomain(s.input[k], d)) errors.push(`${s.id}: input '${k}'=${JSON.stringify(s.input[k])} outside its domain`);
    }
  }
  for (const b of plan.beyondProof) {
    refIntents(`beyondProof(${b.intentId})`, [b.intentId]);
    if (!BEYOND_PROOF_REASONS.includes(b.reason)) errors.push(`beyondProof(${b.intentId}): unknown reason ${b.reason}`);
  }

  const covered = new Set<string>();
  for (const x of [...plan.clauses, ...plan.assumptions, ...plan.scenarios]) x.intentIds.forEach((i) => covered.add(i));
  plan.beyondProof.forEach((b) => covered.add(b.intentId));
  const uncovered = [...approved].filter((i) => !covered.has(i));

  return { errors, uncovered };
}

/** Throws on structural errors; returns uncovered intents (reported as gaps). */
export function validatePlan(artifact: IntentArtifact, plan: ProofPlan): string[] {
  const r = checkPlan(artifact, plan);
  if (r.errors.length) throw new PlanError(r.errors);
  return r.uncovered;
}
