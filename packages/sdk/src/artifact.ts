/**
 * Intent artifact: creation, human review, and freezing.
 *
 * Extraction is non-deterministic. Review is where a human catches
 * "the AI misunderstood". freeze() pins the reviewed artifact with a content
 * hash; everything downstream is deterministic and bound to that hash.
 */
import { sha256Hex } from "./sha256.js";
import type { DecisionCard, Intent, IntentArtifact, IntentKind, Provability, BeyondProofReason } from "./types.js";
import { BEYOND_PROOF_REASONS } from "./types.js";

export interface IntentDraft {
  statement: string;
  kind: IntentKind;
  provability: Provability;
  beyondProofReason?: BeyondProofReason;
  /** Verbatim excerpt from the prose that this intent comes from. */
  quote: string;
}

export interface DecisionDraft {
  intentIds: string[];
  question: string;
  options: { label: string; consequence: string }[];
  recommended: number;
}

export class ArtifactError extends Error {
  constructor(public readonly problems: string[]) {
    super(`Invalid intent artifact:\n - ${problems.join("\n - ")}`);
  }
}

/** Build an artifact from extractor output. Quotes are anchored to the prose (intent map). */
export function createArtifact(
  prose: string,
  drafts: IntentDraft[],
  decisions: DecisionDraft[] = [],
  notPromised: string[] = [],
): IntentArtifact {
  const problems: string[] = [];
  const intents: Intent[] = drafts.map((d, i) => {
    const id = `I${i + 1}`;
    const start = locate(prose, d.quote);
    if (start < 0) problems.push(`${id}: quote not found in prose: "${d.quote}"`);
    if (d.provability === "beyond-proof" && !d.beyondProofReason)
      problems.push(`${id}: beyond-proof intent needs a beyondProofReason`);
    if (d.beyondProofReason && !BEYOND_PROOF_REASONS.includes(d.beyondProofReason))
      problems.push(`${id}: unknown beyondProofReason '${d.beyondProofReason}'`);
    return {
      id,
      statement: d.statement,
      kind: d.kind,
      provability: d.provability,
      ...(d.beyondProofReason ? { beyondProofReason: d.beyondProofReason } : {}),
      source: { quote: d.quote, start: Math.max(start, 0), end: Math.max(start, 0) + d.quote.length },
      status: "proposed",
    };
  });
  const ids = new Set(intents.map((x) => x.id));
  const cards: DecisionCard[] = decisions.map((d, i) => {
    const id = `D${i + 1}`;
    if (d.options.length < 2) problems.push(`${id}: a decision needs at least two options`);
    if (d.recommended < 0 || d.recommended >= d.options.length) problems.push(`${id}: recommended out of range`);
    for (const ref of d.intentIds) if (!ids.has(ref)) problems.push(`${id}: unknown intent ${ref}`);
    return { id, ...d, origin: "extraction" };
  });
  if (problems.length) throw new ArtifactError(problems);
  return { lastro: "intent-artifact/v0", prose, intents, decisions: cards, notPromised };
}

/** Whitespace-insensitive search, so a quote with reflowed lines still anchors. */
function locate(prose: string, quote: string): number {
  const exact = prose.indexOf(quote);
  if (exact >= 0) return exact;
  const norm = (s: string) => s.replace(/\s+/g, " ").trim().toLowerCase();
  const q = norm(quote);
  if (!q) return -1;
  // Fallback: scan for a normalized match starting at each word boundary.
  for (let i = 0; i < prose.length; i++) {
    if (i > 0 && !/\s/.test(prose[i - 1]!)) continue;
    if (norm(prose.slice(i, i + quote.length * 2)).startsWith(q)) return i;
  }
  return -1;
}

// ---------------------------------------------------------------------------
// Review
// ---------------------------------------------------------------------------

export class Review {
  private a: IntentArtifact;

  constructor(artifact: IntentArtifact) {
    if (artifact.hash) throw new Error("Artifact is frozen; review a copy created with thaw()");
    this.a = structuredClone(artifact);
  }

  private intent(id: string): Intent {
    const it = this.a.intents.find((x) => x.id === id);
    if (!it) throw new Error(`Unknown intent ${id}`);
    return it;
  }

  approve(...ids: string[]): this {
    for (const id of ids) this.intent(id).status = "approved";
    return this;
  }

  approveAll(): this {
    for (const it of this.a.intents) if (it.status === "proposed") it.status = "approved";
    return this;
  }

  /** The AI got it wrong and the intent should not be built at all. */
  reject(id: string, note: string): this {
    const it = this.intent(id);
    it.status = "rejected";
    it.reviewNote = note;
    return this;
  }

  /** The AI got it partially wrong: rewrite the statement. Amended intents count as approved. */
  amend(id: string, statement: string, patch: Partial<Pick<Intent, "kind" | "provability" | "beyondProofReason">> = {}, note = "amended by reviewer"): this {
    const it = this.intent(id);
    it.originalStatement ??= it.statement;
    it.statement = statement;
    Object.assign(it, patch);
    it.status = "amended";
    it.reviewNote = note;
    return this;
  }

  /** Add an intent the extractor missed (the "unstated" requirement made explicit). */
  add(draft: IntentDraft): this {
    const start = locate(this.a.prose, draft.quote);
    const id = `I${this.a.intents.length + 1}`;
    this.a.intents.push({
      id,
      statement: draft.statement,
      kind: draft.kind,
      provability: draft.provability,
      ...(draft.beyondProofReason ? { beyondProofReason: draft.beyondProofReason } : {}),
      source: { quote: draft.quote, start: Math.max(start, 0), end: Math.max(start, 0) + draft.quote.length },
      status: "approved",
      reviewNote: start < 0 ? "added by reviewer (not in prose)" : "added by reviewer",
    });
    return this;
  }

  answer(decisionId: string, option: number): this {
    const d = this.a.decisions.find((x) => x.id === decisionId);
    if (!d) throw new Error(`Unknown decision ${decisionId}`);
    if (option < 0 || option >= d.options.length) throw new Error(`Option ${option} out of range for ${decisionId}`);
    d.chosen = option;
    return this;
  }

  notPromised(...items: string[]): this {
    this.a.notPromised.push(...items);
    return this;
  }

  done(): IntentArtifact {
    return structuredClone(this.a);
  }
}

export const review = (artifact: IntentArtifact) => new Review(artifact);

/** Problems that block freezing: everything must be reviewed and every decision answered. */
export function pendingReview(a: IntentArtifact): string[] {
  const p: string[] = [];
  for (const it of a.intents) if (it.status === "proposed") p.push(`${it.id} not reviewed`);
  for (const d of a.decisions) if (d.chosen === undefined) p.push(`${d.id} unanswered: ${d.question}`);
  return p;
}

export function freeze(a: IntentArtifact): IntentArtifact {
  const pending = pendingReview(a);
  if (pending.length) throw new ArtifactError(pending);
  const copy = structuredClone(a);
  delete copy.hash;
  return { ...copy, hash: hashOf(copy) };
}

export function thaw(a: IntentArtifact): IntentArtifact {
  const copy = structuredClone(a);
  delete copy.hash;
  return copy;
}

/** Re-computes the hash; true when the artifact was not modified after freeze(). */
export function isIntact(a: IntentArtifact): boolean {
  if (!a.hash) return false;
  const copy = structuredClone(a);
  delete copy.hash;
  return hashOf(copy) === a.hash;
}

export function approvedIntents(a: IntentArtifact): Intent[] {
  return a.intents.filter((x) => x.status === "approved" || x.status === "amended");
}

// ---------------------------------------------------------------------------
// Canonical hashing
// ---------------------------------------------------------------------------

export function canonicalJson(v: unknown): string {
  if (v === null || typeof v !== "object") return JSON.stringify(v);
  if (Array.isArray(v)) return `[${v.map(canonicalJson).join(",")}]`;
  const o = v as Record<string, unknown>;
  return `{${Object.keys(o)
    .filter((k) => o[k] !== undefined)
    .sort()
    .map((k) => `${JSON.stringify(k)}:${canonicalJson(o[k])}`)
    .join(",")}}`;
}

export function hashOf(v: unknown): string {
  return "sha256:" + sha256Hex(canonicalJson(v));
}
