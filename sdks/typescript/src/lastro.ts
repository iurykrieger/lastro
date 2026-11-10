/**
 * The Lastro pipeline behind one small interface.
 *
 *   You ──plan──▶ intents ──review (policy)──▶ spec ──prove──▶ proof + receipt
 *                    ▲                          │                │
 *                    └─ contradictions: decisions┘                └─ failures ──▶ whoever writes the code
 *
 * Lastro's only AI steps are extracting intents from prose and deriving the
 * proofs. It never writes the implementation: any coding agent or person
 * does, and `prove()` judges it. Every stage takes and returns plain JSON-able
 * objects, so the same calls work in a script, a CI job, an app or an agent
 * harness.
 */
import { carryReview, freeze, pendingReview, raiseDecisions, Review, thaw } from "./artifact.js";
import { validatePlan } from "./plan.js";
import { renderReceipt } from "./receipt.js";
import { show } from "./spec/expr.js";
import type { RustProofReport, RustProver } from "./rust.js";
import { tabulate } from "./rust.js";
import type { DecisionCard, IntentArtifact, Model, ProofPlan, Subject, Trace, VerificationReport } from "./types.js";
import { checkConsistency, verify, type VerifyOptions } from "./verify/engine.js";
import type { IntentExtractor, ProofDeriver } from "./llm/interfaces.js";
import { LlmGapReviewer, LlmIntentExtractor, LlmProofDeriver, type AskJson, type Gap } from "./llm/llm.js";

export interface LastroOptions {
  /** The vocabulary of the proofs: inputs, outputs and helper definitions. */
  model: Model;
  /** One LLM transport for every AI step. Each step can also be replaced below. */
  ask?: AskJson;
  extractor?: IntentExtractor;
  deriver?: ProofDeriver;
  /** Distinct reviewers who must approve each intent (your team's policy). Default 1. */
  requiredApprovals?: number;
  /** Independent Rust prover (see loadRustProver). Recommended in CI. */
  rust?: RustProver;
  verify?: Omit<VerifyOptions, "subject" | "traces">;
}

/** Stage 2: the formal spec derived from a frozen, fully reviewed artifact. */
export interface Spec {
  artifact: IntentArtifact;
  plan: ProofPlan;
  /** Clauses that cannot all hold together. Send them back with `returnDecisions`. */
  contradictions: DecisionCard[];
  ok: boolean;
}

/** Stage 3: the code checked against the spec. */
export interface Proof {
  ok: boolean;
  report: VerificationReport;
  rust?: RustProofReport;
  /** Both engines gave every clause the same verdict (undefined without `rust`). */
  enginesAgree?: boolean;
  /** Code problems, with counterexamples: what the coding agent (or person) must fix. See agentFeedback(). */
  failures: ProofFailure[];
  /** Spec problems (vacuous or weak clauses, intents with no obligation): what the team must fix. */
  specGaps: string[];
  receipt: string;
}

/** One clause or scenario the code broke, with the input that shows it. Plain data for any agent. */
export interface ProofFailure {
  kind: "clause" | "scenario";
  id: string;
  intentIds: string[];
  description: string;
  /** The formal statement, in LSL notation. */
  formal: string;
  input: Record<string, unknown>;
  output: Record<string, unknown> | null;
}

export function failuresOf(plan: ProofPlan, report: VerificationReport): ProofFailure[] {
  const out: ProofFailure[] = [];
  for (const r of report.clauses) {
    if (r.verdict !== "violated" || !r.counterexample) continue;
    const c = plan.clauses.find((x) => x.id === r.clauseId)!;
    out.push({
      kind: "clause",
      id: c.id,
      intentIds: c.intentIds,
      description: c.description,
      formal: `${c.requires ? `requires ${show(c.requires)}; ` : ""}ensures ${show(c.ensures)}`,
      input: r.counterexample.input,
      output: r.counterexample.output,
    });
  }
  for (const s of report.scenarios) {
    if (s.status !== "failed") continue;
    const sc = plan.scenarios.find((x) => x.id === s.scenarioId)!;
    out.push({ kind: "scenario", id: sc.id, intentIds: sc.intentIds, description: `given ${sc.given}, when ${sc.when}, then ${sc.then}`, formal: `expect ${show(sc.expect)}`, input: sc.input, output: s.output ?? null });
  }
  return out;
}

/** Failures as a prompt-ready brief for whatever writes the code (a coding agent or a person). */
export function agentFeedback(proof: Pick<Proof, "failures" | "specGaps">): string {
  if (!proof.failures.length) return "Every proof passes." + (proof.specGaps.length ? ` Spec gaps for the team: ${proof.specGaps.join("; ")}` : "");
  return [
    "The implementation breaks these proof obligations. Fix the code so each holds for every input, without breaking the others:",
    ...proof.failures.map((f) => `- ${f.id} (${f.intentIds.join(", ")}): ${f.description}\n  ${f.formal}\n  counterexample: ${JSON.stringify(f.input)} returned ${JSON.stringify(f.output)}`),
  ].join("\n");
}

export function specGapsOf(report: VerificationReport): string[] {
  const out: string[] = [];
  for (const r of report.clauses) if (r.verdict === "vacuous" || r.verdict === "weak") out.push(`${r.clauseId} is ${r.verdict}: ${r.note ?? ""}`.trim());
  for (const i of report.intents) if (i.verdict === "gap") out.push(`${i.intentId} has no proof obligation`);
  for (const d of report.decisions) out.push(`Contradiction: ${d.question}`);
  return out;
}

export function createLastro(o: LastroOptions) {
  const need = <T>(x: T | undefined, what: string): T => {
    if (!x) throw new Error(`${what} needs an LLM: pass \`ask\` or a custom ${what.toLowerCase()} to createLastro()`);
    return x;
  };
  const extractor = o.extractor ?? (o.ask ? new LlmIntentExtractor(o.ask) : undefined);
  const deriver = o.deriver ?? (o.ask ? new LlmProofDeriver(o.ask) : undefined);
  const gapReviewer = o.ask ? new LlmGapReviewer(o.ask) : undefined;
  const policy = { requiredApprovals: o.requiredApprovals ?? 1 };

  const self = {
    /** 1. Plan: extract reviewable intents from prose. Pass the previous artifact to keep its review. */
    async plan(prose: string, previous?: IntentArtifact): Promise<IntentArtifact> {
      const fresh = await need(extractor, "Extractor").extract(prose);
      fresh.policy = { ...policy };
      return previous ? carryReview(previous, fresh) : fresh;
    },

    /** Review as one person. Chain approve/amend/reject/answer, then done(). */
    review(artifact: IntentArtifact, reviewer: string): Review {
      const a = artifact.hash ? thaw(artifact) : artifact;
      return new Review(a.policy ? a : { ...a, policy: { ...policy } }, reviewer);
    },

    /** What still blocks the spec: missing approvals and unanswered decisions. */
    pending(artifact: IntentArtifact): string[] {
      return pendingReview(artifact.policy ? artifact : { ...artifact, policy: { ...policy } });
    },

    /** 2. Formal spec: freeze the reviewed intents, derive clauses, validate, look for contradictions. */
    async spec(artifact: IntentArtifact): Promise<Spec> {
      const frozen = freeze(artifact.policy ? artifact : { ...artifact, policy: { ...policy } });
      const plan = await need(deriver, "Deriver").derive(frozen, o.model);
      validatePlan(frozen, plan);
      const contradictions = checkConsistency(plan, o.verify);
      return { artifact: frozen, plan, contradictions, ok: contradictions.length === 0 };
    },

    /** Contradictions go back to the team as decision cards; the artifact must be reviewed again. */
    returnDecisions(spec: Spec): IntentArtifact {
      return raiseDecisions(spec.artifact, spec.contradictions);
    },

    /** 3. Prove: check the code against every clause (and the Rust prover, if given). */
    async prove(spec: Spec, subject: Subject, opts: { traces?: Trace[] } = {}): Promise<Proof> {
      const report = await verify(spec.artifact, spec.plan, { ...o.verify, subject, traces: opts.traces ?? [] });
      let rust: RustProofReport | undefined;
      let enginesAgree: boolean | undefined;
      if (o.rust) {
        rust = o.rust.prove(spec.plan, await tabulate(spec.plan.model, subject));
        enginesAgree = report.clauses.every((c) => rust!.clauses.find((r) => r.clauseId === c.clauseId)?.verdict === c.verdict);
      }
      return {
        ok: report.ok && (rust ? rust.ok : true) && enginesAgree !== false,
        report,
        ...(rust ? { rust, enginesAgree } : {}),
        failures: failuresOf(spec.plan, report),
        specGaps: specGapsOf(report),
        receipt: renderReceipt(report, spec.plan, rust),
      };
    },

    /** LLM gap review: what the proofs cannot see. Evidence for humans, never proof. */
    async gapReview(spec: Spec, source: string): Promise<Gap[]> {
      return need(gapReviewer, "Gap reviewer").review(spec.artifact, spec.plan, source);
    },
  };
  return self;
}

export type Lastro = ReturnType<typeof createLastro>;
