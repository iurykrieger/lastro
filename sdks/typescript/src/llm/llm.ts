/**
 * Transport-free LLM steps: prompts, JSON schemas, and the deterministic
 * checks that turn an LLM answer into Lastro objects.
 *
 * Plug in any transport with an `AskJson` function (the Anthropic API, a
 * browser-side Claude, a test double). Nothing an LLM returns is trusted as
 * is: quotes are anchored by createArtifact(), plans are parsed and validated
 * as LSL, and generated code only matters through the proofs it passes.
 */
import { approvedIntents, createArtifact, type DecisionDraft, type IntentDraft } from "../artifact.js";
import { show, validateExpr, type Expr } from "../spec/expr.js";
import type { IntentArtifact, Model, ProofPlan } from "../types.js";
import { BEYOND_PROOF_REASONS } from "../types.js";
import type { IntentExtractor, ProofDeriver } from "./interfaces.js";

/** Ask an LLM for one JSON value matching `schema`. */
export type AskJson = (request: { step: "extract" | "derive" | "gap-review"; system: string; user: string; schema: Record<string, unknown> }) => Promise<unknown>;

// ---------------------------------------------------------------------------
// Extraction
// ---------------------------------------------------------------------------

export const EXTRACT_SYSTEM = `You turn a business rule or change request, written as prose, into a list of reviewable intents.

Rules:
- One intent = one checkable promise. Split compound sentences ("only X and always Y" is two intents).
- "quote" must be copied verbatim from the prose (it anchors the intent map). Never paraphrase the quote.
- "statement" restates the promise precisely in the prose's language.
- kind: behavior (must happen), invariant (must never happen), preserve (existing behavior to keep), non-functional.
- provability: "formal" when it is a pure input→output fact about the program's decision logic; "empirical" when it can only be checked by running tests; "beyond-proof" when it falls where proof stops.
- beyondProofReason (required for beyond-proof): external-code (libraries the verifier cannot see), world-facts (database, clock), termination-timing (whether/how fast it finishes), human-judgment (UX, clarity), unstated.
- When the prose is ambiguous, DO NOT guess. Add a decision: one question, 2-4 options each with its consequence, and the index of the option you recommend. Prefer questions the author has not considered (e.g. "must the right password ALWAYS get a token, or only must a wrong one be refused?").
- notPromised: things a reader might assume but the prose does not promise (timing, rate limits...).`;

export const EXTRACT_SCHEMA = {
  type: "object",
  additionalProperties: false,
  required: ["intents", "decisions", "notPromised"],
  properties: {
    intents: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["statement", "kind", "provability", "beyondProofReason", "quote"],
        properties: {
          statement: { type: "string" },
          kind: { type: "string", enum: ["behavior", "invariant", "preserve", "non-functional"] },
          provability: { type: "string", enum: ["formal", "empirical", "beyond-proof"] },
          beyondProofReason: { type: "string", enum: ["none", ...BEYOND_PROOF_REASONS] },
          quote: { type: "string" },
        },
      },
    },
    decisions: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["intentIndexes", "question", "options", "recommended"],
        properties: {
          intentIndexes: { type: "array", items: { type: "integer" } },
          question: { type: "string" },
          options: {
            type: "array",
            items: {
              type: "object",
              additionalProperties: false,
              required: ["label", "consequence"],
              properties: { label: { type: "string" }, consequence: { type: "string" } },
            },
          },
          recommended: { type: "integer" },
        },
      },
    },
    notPromised: { type: "array", items: { type: "string" } },
  },
};

export interface ExtractOut {
  intents: (Omit<IntentDraft, "beyondProofReason"> & { beyondProofReason: string })[];
  decisions: { intentIndexes: number[]; question: string; options: { label: string; consequence: string }[]; recommended: number }[];
  notPromised: string[];
}

// ---------------------------------------------------------------------------
// Derivation
// ---------------------------------------------------------------------------

export const DERIVE_SYSTEM = `You derive proof obligations from APPROVED intents, in LSL (Lastro Spec Language).

LSL expressions are JSON:
  {"var":"in.<input>"} | {"var":"out.<output>"} | {"const": true|1|"admin"}
  {"op": and|or|not|implies|iff|eq|neq|lt|le|gt|ge|add|sub|mul|ite|in, "args":[...]}
  {"call":"<def>", "args":[...]}
Only use the variables and defs of the given model. Each expression is returned as a JSON STRING.

Obligations:
- clauses: formal if-then promises "for every input: requires ⇒ ensures". One sentence of intent may need several clauses. Prefer "both ways" clauses (e.g. the right password ALWAYS gets a token, AND a wrong one NEVER does), since a one-sided clause lets a program that refuses everyone pass.
- Never weaken a clause to make it provable. If an intent cannot be expressed in the model, put it in beyondProof.
- assumptions: trusted pieces (external code, world facts) the proof relies on, each with youAccept / ifWrong and the scenarios that test it.
- scenarios: Given/When/Then examples with a concrete input (every model input) and an LSL "expect".
- beyondProof: intents (or parts) where proof stops, with the reason.
- Every approved intent must be covered by at least one obligation or a beyondProof entry. Never reference rejected intents.`;

const STR_ARR = { type: "array", items: { type: "string" } };
export const DERIVE_SCHEMA = {
  type: "object",
  additionalProperties: false,
  required: ["clauses", "assumptions", "scenarios", "beyondProof"],
  properties: {
    clauses: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["id", "intentIds", "description", "requires", "ensures", "assumes"],
        properties: {
          id: { type: "string" },
          intentIds: STR_ARR,
          description: { type: "string" },
          requires: { type: "string", description: "LSL JSON string, or empty string for no if-part" },
          ensures: { type: "string", description: "LSL JSON string" },
          assumes: STR_ARR,
        },
      },
    },
    assumptions: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["id", "intentIds", "piece", "contract", "youAccept", "ifWrong", "checkedBy"],
        properties: {
          id: { type: "string" },
          intentIds: STR_ARR,
          piece: { type: "string" },
          contract: { type: "string" },
          youAccept: { type: "string" },
          ifWrong: { type: "string" },
          checkedBy: STR_ARR,
        },
      },
    },
    scenarios: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["id", "intentIds", "given", "when", "then", "inputJson", "expect"],
        properties: {
          id: { type: "string" },
          intentIds: STR_ARR,
          given: { type: "string" },
          when: { type: "string" },
          then: { type: "string" },
          inputJson: { type: "string", description: "JSON object with a value for every model input" },
          expect: { type: "string", description: "LSL JSON string over in.* and out.*" },
        },
      },
    },
    beyondProof: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["intentId", "reason", "note"],
        properties: {
          intentId: { type: "string" },
          reason: { type: "string", enum: [...BEYOND_PROOF_REASONS] },
          note: { type: "string" },
        },
      },
    },
  },
};

export interface DeriveOut {
  clauses: { id: string; intentIds: string[]; description: string; requires: string; ensures: string; assumes: string[] }[];
  assumptions: ProofPlan["assumptions"];
  scenarios: { id: string; intentIds: string[]; given: string; when: string; then: string; inputJson: string; expect: string }[];
  beyondProof: ProofPlan["beyondProof"];
}


// ---------------------------------------------------------------------------
// Converters (deterministic)
// ---------------------------------------------------------------------------

export function extractUserPrompt(prose: string, context?: string): string {
  return `${context ? `<context>\n${context}\n</context>\n\n` : ""}<prose>\n${prose}\n</prose>\n\nIntents are numbered from 0 in the order you list them; decisions reference them by index.`;
}

export function artifactFromExtraction(prose: string, out: ExtractOut): IntentArtifact {
  const drafts: IntentDraft[] = out.intents.map((i) => ({
    statement: i.statement,
    kind: i.kind,
    provability: i.provability,
    quote: i.quote,
    ...(i.beyondProofReason && i.beyondProofReason !== "none" ? { beyondProofReason: i.beyondProofReason as IntentDraft["beyondProofReason"] } : {}),
  }));
  const decisions: DecisionDraft[] = (out.decisions ?? []).map((d) => ({
    intentIds: d.intentIndexes.map((n) => `I${n + 1}`),
    question: d.question,
    options: d.options,
    recommended: d.recommended,
  }));
  return createArtifact(prose, drafts, decisions, out.notPromised ?? []);
}

export function deriveUserPrompt(artifact: IntentArtifact, model: Model): string {
  if (!artifact.hash) throw new Error("freeze() the artifact before deriving proofs");
  const intents = approvedIntents(artifact).map((i) => ({ id: i.id, statement: i.statement, kind: i.kind, provability: i.provability, beyondProofReason: i.beyondProofReason }));
  const decisions = artifact.decisions.map((d) => ({ question: d.question, answer: d.options[d.chosen ?? d.recommended]?.label }));
  return JSON.stringify({ model, approvedIntents: intents, decisions, notPromised: artifact.notPromised }, null, 2);
}

/** Parse and validate the deriver's answer. Throws with every problem found. */
export function planFromDerivation(artifact: IntentArtifact, model: Model, out: DeriveOut): ProofPlan {
  const parse = (where: string, s: string): Expr => {
    let e: unknown;
    try {
      e = typeof s === "string" ? JSON.parse(s) : s;
    } catch {
      throw new Error(`${where}: not valid JSON: ${s}`);
    }
    const errs = validateExpr(e, model, [], where);
    if (errs.length) throw new Error(errs.join("; "));
    return e as Expr;
  };
  return {
    lastro: "proof-plan/v0",
    artifactHash: artifact.hash!,
    model,
    clauses: out.clauses.map((c) => ({
      id: c.id,
      intentIds: c.intentIds,
      description: c.description,
      ...(c.requires && String(c.requires).trim() ? { requires: parse(`${c.id}.requires`, c.requires) } : {}),
      ensures: parse(`${c.id}.ensures`, c.ensures),
      ...(c.assumes?.length ? { assumes: c.assumes } : {}),
    })),
    assumptions: out.assumptions ?? [],
    scenarios: (out.scenarios ?? []).map((s) => ({
      id: s.id,
      intentIds: s.intentIds,
      given: s.given,
      when: s.when,
      then: s.then,
      input: (typeof s.inputJson === "string" ? JSON.parse(s.inputJson) : s.inputJson) as Record<string, unknown>,
      expect: parse(`${s.id}.expect`, s.expect),
    })),
    beyondProof: out.beyondProof ?? [],
  };
}

// ---------------------------------------------------------------------------
// Transport-agnostic steps
// ---------------------------------------------------------------------------

export class LlmIntentExtractor implements IntentExtractor {
  constructor(private readonly ask: AskJson) {}
  async extract(prose: string, context?: string): Promise<IntentArtifact> {
    const out = (await this.ask({ step: "extract", system: EXTRACT_SYSTEM, user: extractUserPrompt(prose, context), schema: EXTRACT_SCHEMA })) as ExtractOut;
    return artifactFromExtraction(prose, out);
  }
}

export class LlmProofDeriver implements ProofDeriver {
  constructor(private readonly ask: AskJson) {}
  async derive(artifact: IntentArtifact, model: Model): Promise<ProofPlan> {
    const out = (await this.ask({ step: "derive", system: DERIVE_SYSTEM, user: deriveUserPrompt(artifact, model), schema: DERIVE_SCHEMA })) as DeriveOut;
    return planFromDerivation(artifact, model, out);
  }
}

// ---------------------------------------------------------------------------
// LLM gap review (evidence, never proof)
// ---------------------------------------------------------------------------

export const GAP_REVIEW_SYSTEM = `You review a proof-driven change for gaps the proofs cannot see.

You get the approved intents, the formal clauses that were proved, the trusted pieces, what is beyond proof, and the code.
Report only real gaps, each tied to an intent:
- an intent whose meaning is only partly captured by its clauses (the proof passes but the intent could still be broken),
- code behavior that relies on a trusted piece or an input the model does not represent,
- an intent with no clause and no beyond-proof reason.
Do not restate what is already proved. An empty list is a fine answer.`;

export const GAP_REVIEW_SCHEMA = {
  type: "object",
  additionalProperties: false,
  required: ["gaps"],
  properties: {
    gaps: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["intentId", "gap", "suggestion", "severity"],
        properties: {
          intentId: { type: "string" },
          gap: { type: "string" },
          suggestion: { type: "string" },
          severity: { type: "string", enum: ["low", "medium", "high"] },
        },
      },
    },
  },
};

export interface Gap {
  intentId: string;
  gap: string;
  suggestion: string;
  severity: "low" | "medium" | "high";
}

export function gapReviewUserPrompt(artifact: IntentArtifact, plan: ProofPlan, source: string): string {
  return JSON.stringify(
    {
      approvedIntents: approvedIntents(artifact).map((i) => ({ id: i.id, statement: i.statement })),
      clauses: plan.clauses.map((c) => ({ id: c.id, intentIds: c.intentIds, description: c.description, formal: `${c.requires ? `${show(c.requires)} => ` : ""}${show(c.ensures)}` })),
      trustedPieces: plan.assumptions.map((a) => ({ id: a.id, piece: a.piece, contract: a.contract })),
      beyondProof: plan.beyondProof,
      code: source,
    },
    null,
    2,
  );
}

export class LlmGapReviewer {
  constructor(private readonly ask: AskJson) {}
  async review(artifact: IntentArtifact, plan: ProofPlan, source: string): Promise<Gap[]> {
    const out = (await this.ask({ step: "gap-review", system: GAP_REVIEW_SYSTEM, user: gapReviewUserPrompt(artifact, plan, source), schema: GAP_REVIEW_SCHEMA })) as { gaps?: Gap[] };
    return Array.isArray(out?.gaps) ? out.gaps : [];
  }
}
