/**
 * Intent artifact: creation, human review, and approval.
 *
 * Extraction is non-deterministic. Review is where a human catches
 * "the AI misunderstood". approve() pins the reviewed artifact with a content
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

  /** `reviewer` names who is reviewing; it is required when the artifact's policy asks for several approvals. */
  constructor(
    artifact: IntentArtifact,
    private readonly reviewer?: string,
  ) {
    if (artifact.hash) throw new Error("Artifact is approved; review a copy created with reopen()");
    this.a = structuredClone(artifact);
  }

  private sign(it: Intent, reset = false): void {
    if (!this.reviewer) return;
    const prev = reset ? [] : (it.approvals ?? []);
    it.approvals = prev.includes(this.reviewer) ? prev : [...prev, this.reviewer];
  }

  private intent(id: string): Intent {
    const it = this.a.intents.find((x) => x.id === id);
    if (!it) throw new Error(`Unknown intent ${id}`);
    return it;
  }

  approve(...ids: string[]): this {
    for (const id of ids) {
      const it = this.intent(id);
      if (it.status !== "amended") it.status = "approved";
      this.sign(it);
    }
    return this;
  }

  /** Approve every intent this reviewer has not signed yet (rejected ones stay rejected). */
  approveAll(): this {
    for (const it of this.a.intents) if (it.status !== "rejected") this.approve(it.id);
    return this;
  }

  /** The AI got it wrong and the intent should not be built at all. */
  reject(id: string, note: string): this {
    const it = this.intent(id);
    it.status = "rejected";
    it.reviewNote = this.reviewer ? `${note} (${this.reviewer})` : note;
    delete it.approvals;
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
    // A new wording needs fresh approvals from everyone else.
    this.sign(it, true);
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
    // A decision raised after extraction (a contradiction in the spec) changes what is promised:
    // its answer becomes an intent, and the team approves it like any other.
    if (d.origin !== "extraction") {
      const o = d.options[option]!;
      const statement = `${d.question} Decided: ${o.label}${o.consequence ? ` (${o.consequence})` : ""}.`;
      const existing = this.a.intents.find((x) => x.decisionId === d.id);
      const it: Intent = existing ?? {
        id: `I${this.a.intents.length + 1}`,
        statement,
        kind: "behavior",
        provability: "formal",
        source: { quote: "", start: 0, end: 0 },
        status: "proposed",
        decisionId: d.id,
      };
      it.statement = statement;
      it.status = "proposed";
      it.reviewNote = `answer to decision ${d.id}`;
      delete it.approvals;
      delete it.originalStatement;
      if (!existing) this.a.intents.push(it);
    }
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

export const review = (artifact: IntentArtifact, reviewer?: string) => new Review(artifact, reviewer);

export function requiredApprovals(a: IntentArtifact): number {
  return Math.max(1, a.policy?.requiredApprovals ?? 1);
}

/** Approvals an intent has collected (legacy single-reviewer approvals count as one). */
export function approvalCount(it: Intent): number {
  if (it.status !== "approved" && it.status !== "amended") return 0;
  return it.approvals?.length ?? 1;
}

/** Problems that block approval: every intent reviewed by enough people and every decision answered. */
export function pendingReview(a: IntentArtifact): string[] {
  const p: string[] = [];
  const need = requiredApprovals(a);
  for (const it of a.intents) {
    if (it.status === "proposed" && !it.approvals?.length) p.push(`${it.id} not reviewed`);
    else if (it.status !== "rejected" && approvalCount(it) < need) p.push(`${it.id} needs ${need - approvalCount(it)} more approval(s)`);
  }
  for (const d of a.decisions) if (d.chosen === undefined) p.push(`${d.id} unanswered: ${d.question}`);
  return p;
}

export function approve(a: IntentArtifact): IntentArtifact {
  const pending = pendingReview(a);
  if (pending.length) throw new ArtifactError(pending);
  const copy = structuredClone(a);
  delete copy.hash;
  return { ...copy, hash: hashOf(copy) };
}

export function reopen(a: IntentArtifact): IntentArtifact {
  const copy = structuredClone(a);
  delete copy.hash;
  return copy;
}

/** Re-computes the hash; true when the artifact was not modified after approve(). */
export function isApproved(a: IntentArtifact): boolean {
  if (!a.hash) return false;
  const copy = structuredClone(a);
  delete copy.hash;
  return hashOf(copy) === a.hash;
}

export function approvedIntents(a: IntentArtifact): Intent[] {
  const need = requiredApprovals(a);
  return a.intents.filter((x) => approvalCount(x) >= need);
}

/**
 * Send spec problems back to the team: contradictions found by the
 * consistency check (or any decision raised after derivation) become
 * unanswered decision cards on a reopened copy, so the artifact cannot be
 * approved again until someone answers them.
 */
export function raiseDecisions(a: IntentArtifact, cards: DecisionCard[]): IntentArtifact {
  const out = reopen(a);
  const known = new Set(out.decisions.map((d) => d.question));
  for (const c of cards) {
    if (known.has(c.question)) continue;
    out.decisions.push({ ...structuredClone(c), id: `D${out.decisions.length + 1}`, chosen: undefined });
  }
  return out;
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

// ---------------------------------------------------------------------------
// Re-extraction
// ---------------------------------------------------------------------------

/**
 * Carry a previous review over to a fresh extraction of edited prose.
 *
 * An intent whose statement matches one the reviewer already approved or
 * rejected keeps that verdict. If the reviewer had amended it and the AI
 * proposes the same original reading of the same quote, the amendment is reapplied.
 * Everything else stays `proposed`, so only what the edit changed needs a
 * new look. Decisions with the same question keep their answer.
 */
export function carryReview(previous: IntentArtifact, next: IntentArtifact): IntentArtifact {
  const out = structuredClone(next);
  const norm = (s: string) => s.replace(/\s+/g, " ").trim().toLowerCase();
  const reviewed = previous.intents.filter((i) => i.status !== "proposed");
  for (const it of out.intents) {
    const same = reviewed.find((p) => norm(p.statement) === norm(it.statement));
    // Reapply an amendment only when the AI repeats its old reading of the SAME words.
    const amended = reviewed.find(
      (p) => p.status === "amended" && p.originalStatement && norm(p.originalStatement) === norm(it.statement) && norm(p.source.quote) === norm(it.source.quote),
    );
    if (same) {
      it.status = same.status;
      if (same.approvals) it.approvals = [...same.approvals];
      if (same.reviewNote) it.reviewNote = same.reviewNote;
      if (same.originalStatement) it.originalStatement = same.originalStatement;
    } else if (amended) {
      it.originalStatement = it.statement;
      it.statement = amended.statement;
      it.status = "amended";
      if (amended.approvals) it.approvals = [...amended.approvals];
      if (amended.reviewNote) it.reviewNote = amended.reviewNote;
    }
  }
  for (const d of out.decisions) {
    const prev = previous.decisions.find((p) => norm(p.question) === norm(d.question));
    if (prev?.chosen !== undefined && prev.chosen < d.options.length) d.chosen = prev.chosen;
  }
  if (previous.policy) out.policy = { ...previous.policy };
  return out;
}
