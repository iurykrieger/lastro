/**
 * The two AI steps of proposal/v1, transport-free: prompts, JSON Schemas and
 * the calls that check the answers (see ../proposal.ts).
 *
 *   extract  prose + use cases (with their proofs) ─▶ intents, resulting use cases, decisions
 *            (also: recombine one use case's `after` from its remaining intents)
 *   derive   approved intents + models ─▶ pending proofs, or decisions
 */
import { show } from "../spec/expr.js";
import { BEYOND_PROOF_REASONS, type Model } from "../types.js";
import {
  forwardIntents,
  proofsFromDerivation,
  proposalFromExtraction,
  type DeriveV1Out,
  type ProofSetup,
  type Proposal,
  type ProposeInput,
  type ProposeOut,
  type UseCase,
} from "../proposal.js";
import type { AskJson } from "./llm.js";
import type { VerifyOptions } from "../verify/engine.js";

const STR = { type: "string" };
const STR_ARR = { type: "array", items: STR };
const OPTIONS = {
  type: "array",
  items: { type: "object", additionalProperties: false, required: ["label", "consequence"], properties: { label: STR, consequence: STR } },
};

// ---------------------------------------------------------------------------
// Extract
// ---------------------------------------------------------------------------

export const PROPOSE_SYSTEM = `You read a change request, written as prose, against an application's current use cases, and write it as reviewable intents.

The application is its list of use cases. Each has a slug, a name, a description of its complete current behavior, and the proofs that keep it working.

Rules:
- An intent is one change to exactly one use case: "add" a new use case, "modify" an existing one, or "remove" one. One intent = one checkable promise; several intents may change the same use case.
- "useCase" is a use-case slug: an existing slug for modify and remove, a new kebab-case slug for add.
- "slug" (kebab-case, unique in this answer), "name" and "description" describe the intent; the description states the change in plain language.
- "quote" is copied verbatim from the prose and anchors the intent. Never paraphrase it.
- provability: "formal" for a pure input-to-output fact about the program's decisions, "empirical" when only running tests can check it, "beyond-proof" where proof stops. beyondProofReason is "none" unless beyond-proof: external-code, world-facts, termination-timing, human-judgment or unstated.
- replaces: for a modify, list each existing proof of that use case whose guarantee the new behavior contradicts, with the reason in plain words. List only proofs of the same use case; leave it empty when nothing breaks. For an add or a remove, leave it empty (the SDK fills it for a remove).
- useCases: exactly one entry per touched use case, with the same change as its intents, its name, and "after": the complete new description with every intent combined (empty for a remove). Write the whole behavior, not a diff.
- When the prose is ambiguous, or two intents on one use case contradict each other, do not guess: add a decision (one question, 2-4 options each with its consequence, the recommended index, and the slugs of the intents it concerns).
- notPromised: what a reader might assume but the prose does not promise.
- name, slug and description at the top level describe the whole proposal.`;

export const PROPOSE_SCHEMA = {
  type: "object",
  additionalProperties: false,
  required: ["name", "slug", "description", "intents", "useCases", "decisions", "notPromised"],
  properties: {
    name: STR,
    slug: STR,
    description: STR,
    intents: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["slug", "name", "description", "change", "useCase", "provability", "beyondProofReason", "quote", "replaces"],
        properties: {
          slug: STR,
          name: STR,
          description: STR,
          change: { type: "string", enum: ["add", "modify", "remove"] },
          useCase: STR,
          provability: { type: "string", enum: ["formal", "empirical", "beyond-proof"] },
          beyondProofReason: { type: "string", enum: ["none", ...BEYOND_PROOF_REASONS] },
          quote: STR,
          replaces: {
            type: "array",
            items: { type: "object", additionalProperties: false, required: ["proof", "reason"], properties: { proof: STR, reason: STR } },
          },
        },
      },
    },
    useCases: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["slug", "change", "name", "after"],
        properties: { slug: STR, change: { type: "string", enum: ["add", "modify", "remove"] }, name: STR, after: STR },
      },
    },
    decisions: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["slug", "name", "question", "options", "recommended", "intents"],
        properties: { slug: STR, name: STR, question: STR, options: OPTIONS, recommended: { type: "integer" }, intents: STR_ARR },
      },
    },
    notPromised: STR_ARR,
  },
};

function proofView(u: UseCase) {
  return u.proofs.map((p) => ({
    id: p.id,
    name: p.name,
    description: p.description,
    formal: p.check.kind === "clause" ? `${p.check.requires ? `${show(p.check.requires)} => ` : ""}${show(p.check.ensures)}` : `${JSON.stringify(p.check.input)} => ${show(p.check.expect)}`,
  }));
}

export function proposeUserPrompt(prose: string, useCases: UseCase[]): string {
  const cases = useCases.map((u) => ({ slug: u.slug, name: u.name, description: u.description, ...(u.proofs.length ? { proofs: proofView(u) } : {}) }));
  return `<use-cases>\n${JSON.stringify(cases, null, 2)}\n</use-cases>\n\n<prose>\n${prose}\n</prose>`;
}

/** The extract step: prose against the use cases, checked into an open proposal. */
export async function extractProposal(ask: AskJson, input: ProposeInput): Promise<Proposal> {
  const out = (await ask({ step: "extract", system: PROPOSE_SYSTEM, user: proposeUserPrompt(input.prose, input.useCases), schema: PROPOSE_SCHEMA })) as ProposeOut;
  return proposalFromExtraction(input, out);
}

export const RECOMBINE_SYSTEM = `You rewrite one use case's description after its review changed.

You get the use case's current description (empty for a new one), the intents that still apply to it (approved, amended or still proposed; rejected ones are gone), and optionally the reviewer's instructions.
Write "after": the complete new description of the use case with every remaining intent combined and nothing from the rejected ones. Write the whole behavior, not a diff. Keep "name" unless the intents rename it.`;

export const RECOMBINE_SCHEMA = {
  type: "object",
  additionalProperties: false,
  required: ["name", "after"],
  properties: { name: STR, after: STR },
};

export function recombineUserPrompt(p: Proposal, useCase: string, instructions?: string): string {
  const c = p.useCases.find((x) => x.useCase === useCase || x.slug === useCase);
  if (!c) throw new Error(`${useCase} is not touched by ${p.id}`);
  const intents = p.intents.filter((i) => c.intents.includes(i.id) && i.status !== "rejected").map((i) => ({ id: i.id, change: i.change, description: i.description }));
  return JSON.stringify({ useCase: c.useCase, name: c.name, before: c.before ?? "", intents, ...(instructions ? { instructions } : {}) }, null, 2);
}

/** Part of the extract step: the LLM recombines a stale `after`. Pass the result to ProposalReview.rewriteAfter. */
export async function recombineAfter(ask: AskJson, p: Proposal, useCase: string, instructions?: string): Promise<{ name: string; after: string }> {
  return (await ask({ step: "extract", system: RECOMBINE_SYSTEM, user: recombineUserPrompt(p, useCase, instructions), schema: RECOMBINE_SCHEMA })) as { name: string; after: string };
}

// ---------------------------------------------------------------------------
// Derive
// ---------------------------------------------------------------------------

export const DERIVE_PROOFS_SYSTEM = `You derive proofs for the APPROVED intents of a proposal, in LSL (Lastro Spec Language).

LSL expressions are JSON:
  {"var":"in.<input>"} | {"var":"out.<output>"} | {"const": true|1|"admin"}
  {"op": and|or|not|implies|iff|eq|neq|lt|le|gt|ge|add|sub|mul|ite|in, "args":[...]}
  {"call":"<def>", "args":[...]}
Only use the variables and defs of the model the proof names. Each expression is returned as a JSON STRING.

Each proof checks one intent and names its model and subject (from the ones declared):
- kind "clause": a formal if-then promise "for every input: requires => ensures" (requires is "" when there is no if-part). Prefer both directions (the right password ALWAYS gets a token AND a wrong one NEVER does). inputJson and expect are "".
- kind "scenario": a concrete input (inputJson, a JSON object with every model input) and an LSL "expect" over in.* and out.*. requires and ensures are "".
- Never weaken a proof to make it pass. An intent that is beyond proof needs no proof.
- A "remove" intent always gets a proof that the old behavior (the use case's "before") no longer happens. When the model cannot express it, return a decision instead.
- When a new proof takes the place of a proof its intent replaces, list that proof in "replaces". Never list a proof the intent does not replace.
- If the approved behavior breaks an existing proof of the use case that no intent replaces, do not drop it: return a decision with "breaks" set to that proof's id and the intents concerned. Otherwise "breaks" is "".
- Every approved intent that is not beyond proof needs at least one proof or a decision. Slugs are kebab-case and unique.`;

export const DERIVE_PROOFS_SCHEMA = {
  type: "object",
  additionalProperties: false,
  required: ["proofs", "decisions"],
  properties: {
    proofs: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["intent", "slug", "name", "description", "model", "subject", "kind", "requires", "ensures", "assumptions", "inputJson", "expect", "replaces"],
        properties: {
          intent: STR,
          slug: STR,
          name: STR,
          description: STR,
          model: STR,
          subject: STR,
          kind: { type: "string", enum: ["clause", "scenario"] },
          requires: STR,
          ensures: STR,
          assumptions: STR_ARR,
          inputJson: STR,
          expect: STR,
          replaces: STR_ARR,
        },
      },
    },
    decisions: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        required: ["slug", "name", "question", "options", "recommended", "intents", "breaks"],
        properties: { slug: STR, name: STR, question: STR, options: OPTIONS, recommended: { type: "integer" }, intents: STR_ARR, breaks: STR },
      },
    },
  },
};

export function deriveProofsUserPrompt(p: Proposal, useCases: UseCase[], models: Record<string, Model>, subjects: string[]): string {
  const touched = new Set(forwardIntents(p).map((i) => i.useCase));
  return JSON.stringify(
    {
      models,
      subjects,
      approvedIntents: forwardIntents(p).map((i) => ({ id: i.id, change: i.change, useCase: i.useCase, description: i.description, provability: i.provability, ...(i.beyondProofReason ? { beyondProofReason: i.beyondProofReason } : {}), replaces: i.replaces })),
      useCases: p.useCases
        .filter((c) => touched.has(c.useCase))
        .map((c) => ({ id: c.useCase, change: c.change, name: c.name, before: c.before ?? "", after: c.after ?? "", proofs: proofView(useCases.find((u) => u.id === c.useCase) ?? { proofs: [] } as unknown as UseCase) })),
      decisions: p.decisions.map((d) => ({ question: d.question, answer: d.options[d.chosen ?? d.recommended]?.label })),
      notPromised: p.notPromised,
    },
    null,
    2,
  );
}

/** The derive step: pending proofs for the approved intents, or decisions that send the proposal back to review. */
export async function deriveProofs(ask: AskJson, p: Proposal, useCases: UseCase[], setup: ProofSetup, opts: VerifyOptions = {}): Promise<Proposal> {
  const out = (await ask({ step: "derive", system: DERIVE_PROOFS_SYSTEM, user: deriveProofsUserPrompt(p, useCases, setup.models, setup.subjects), schema: DERIVE_PROOFS_SCHEMA })) as DeriveV1Out;
  return proofsFromDerivation(p, useCases, setup, out, opts);
}
