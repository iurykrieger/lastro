/**
 * Claude-backed extractor and deriver.
 *
 * Both steps are non-deterministic by nature. Their output is never trusted as
 * is: the extractor's quotes are anchored against the prose by
 * createArtifact(), and the deriver's plan is checked by validatePlan()
 * (LSL expressions are parsed and validated; no code is evaluated).
 *
 * `@anthropic-ai/sdk` is an optional peer dependency, imported from the
 * `lastro/claude` entry point only.
 */
import Anthropic from "@anthropic-ai/sdk";
import { approvedIntents, createArtifact, type DecisionDraft, type IntentDraft } from "../artifact.js";
import { validateExpr, type Expr } from "../spec/expr.js";
import type { IntentArtifact, Model, ProofPlan } from "../types.js";
import { BEYOND_PROOF_REASONS } from "../types.js";
import type { IntentExtractor, ProofDeriver } from "./interfaces.js";

export interface ClaudeOptions {
  client?: Anthropic;
  model?: string;
  effort?: "low" | "medium" | "high" | "xhigh" | "max";
}

const DEFAULT_MODEL = "claude-opus-5-5";

async function askJson<T>(opts: ClaudeOptions, system: string, user: string, schema: Record<string, unknown>): Promise<T> {
  const client = opts.client ?? new Anthropic();
  const stream = client.beta.messages.stream({
    model: opts.model ?? DEFAULT_MODEL,
    max_tokens: 64000,
    betas: ["server-side-fallback-2026-07-01"],
    // On a policy decline, the API re-runs the request on a fallback model.
    fallbacks: "default",
    thinking: { type: "adaptive" },
    output_config: { effort: opts.effort ?? "high", format: { type: "json_schema", schema } },
    system,
    messages: [{ role: "user", content: user }],
  } as unknown as Parameters<typeof client.beta.messages.stream>[0]);
  const msg = await stream.finalMessage();
  if (msg.stop_reason === "refusal") throw new Error("Claude declined the request (stop_reason=refusal)");
  if (msg.stop_reason === "max_tokens") throw new Error("Claude output was truncated (max_tokens)");
  const text = msg.content.flatMap((b) => (b.type === "text" ? [b.text] : [])).join("");
  return JSON.parse(text) as T;
}

// ---------------------------------------------------------------------------
// Extraction
// ---------------------------------------------------------------------------

const EXTRACT_SYSTEM = `You turn a business rule or change request, written as prose, into a list of reviewable intents.

Rules:
- One intent = one checkable promise. Split compound sentences ("only X and always Y" is two intents).
- "quote" must be copied verbatim from the prose (it anchors the intent map). Never paraphrase the quote.
- "statement" restates the promise precisely in the prose's language.
- kind: behavior (must happen), invariant (must never happen), preserve (existing behavior to keep), non-functional.
- provability: "formal" when it is a pure input→output fact about the program's decision logic; "empirical" when it can only be checked by running tests; "beyond-proof" when it falls where proof stops.
- beyondProofReason (required for beyond-proof): external-code (libraries the verifier cannot see), world-facts (database, clock), termination-timing (whether/how fast it finishes), human-judgment (UX, clarity), unstated.
- When the prose is ambiguous, DO NOT guess. Add a decision: one question, 2-4 options each with its consequence, and the index of the option you recommend. Prefer questions the author has not considered (e.g. "must the right password ALWAYS get a token, or only must a wrong one be refused?").
- notPromised: things a reader might assume but the prose does not promise (timing, rate limits...).`;

const EXTRACT_SCHEMA = {
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

interface ExtractOut {
  intents: (Omit<IntentDraft, "beyondProofReason"> & { beyondProofReason: string })[];
  decisions: { intentIndexes: number[]; question: string; options: { label: string; consequence: string }[]; recommended: number }[];
  notPromised: string[];
}

export class ClaudeIntentExtractor implements IntentExtractor {
  constructor(private readonly opts: ClaudeOptions = {}) {}

  async extract(prose: string, context?: string): Promise<IntentArtifact> {
    const user = `${context ? `<context>\n${context}\n</context>\n\n` : ""}<prose>\n${prose}\n</prose>\n\nIntents are numbered from 0 in the order you list them; decisions reference them by index.`;
    const out = await askJson<ExtractOut>(this.opts, EXTRACT_SYSTEM, user, EXTRACT_SCHEMA);
    const drafts: IntentDraft[] = out.intents.map((i) => ({
      statement: i.statement,
      kind: i.kind,
      provability: i.provability,
      quote: i.quote,
      ...(i.beyondProofReason !== "none" ? { beyondProofReason: i.beyondProofReason as IntentDraft["beyondProofReason"] } : {}),
    }));
    const decisions: DecisionDraft[] = out.decisions.map((d) => ({
      intentIds: d.intentIndexes.map((n) => `I${n + 1}`),
      question: d.question,
      options: d.options,
      recommended: d.recommended,
    }));
    return createArtifact(prose, drafts, decisions, out.notPromised);
  }
}

// ---------------------------------------------------------------------------
// Derivation
// ---------------------------------------------------------------------------

const DERIVE_SYSTEM = `You derive proof obligations from APPROVED intents, in LSL (Lastro Spec Language).

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
const DERIVE_SCHEMA = {
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

interface DeriveOut {
  clauses: { id: string; intentIds: string[]; description: string; requires: string; ensures: string; assumes: string[] }[];
  assumptions: ProofPlan["assumptions"];
  scenarios: { id: string; intentIds: string[]; given: string; when: string; then: string; inputJson: string; expect: string }[];
  beyondProof: ProofPlan["beyondProof"];
}

export class ClaudeProofDeriver implements ProofDeriver {
  constructor(private readonly opts: ClaudeOptions = {}) {}

  async derive(artifact: IntentArtifact, model: Model): Promise<ProofPlan> {
    if (!artifact.hash) throw new Error("freeze() the artifact before deriving proofs");
    const intents = approvedIntents(artifact).map((i) => ({ id: i.id, statement: i.statement, kind: i.kind, provability: i.provability, beyondProofReason: i.beyondProofReason }));
    const decisions = artifact.decisions.map((d) => ({ question: d.question, answer: d.options[d.chosen ?? d.recommended]?.label }));
    const user = JSON.stringify({ model, approvedIntents: intents, decisions, notPromised: artifact.notPromised }, null, 2);
    const out = await askJson<DeriveOut>(this.opts, DERIVE_SYSTEM, user, DERIVE_SCHEMA);

    const parse = (where: string, s: string): Expr => {
      let e: unknown;
      try {
        e = JSON.parse(s);
      } catch {
        throw new Error(`${where}: not valid JSON: ${s}`);
      }
      const errs = validateExpr(e, model, [], where);
      if (errs.length) throw new Error(errs.join("; "));
      return e as Expr;
    };

    return {
      lastro: "proof-plan/v0",
      artifactHash: artifact.hash,
      model,
      clauses: out.clauses.map((c) => ({
        id: c.id,
        intentIds: c.intentIds,
        description: c.description,
        ...(c.requires.trim() ? { requires: parse(`${c.id}.requires`, c.requires) } : {}),
        ensures: parse(`${c.id}.ensures`, c.ensures),
        ...(c.assumes.length ? { assumes: c.assumes } : {}),
      })),
      assumptions: out.assumptions,
      scenarios: out.scenarios.map((s) => ({
        id: s.id,
        intentIds: s.intentIds,
        given: s.given,
        when: s.when,
        then: s.then,
        input: JSON.parse(s.inputJson) as Record<string, unknown>,
        expect: parse(`${s.id}.expect`, s.expect),
      })),
      beyondProof: out.beyondProof,
    };
  }
}
