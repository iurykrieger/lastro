/**
 * Use cases, proposals, intents and proofs (proposal/v1).
 *
 *   prose ─▶ PROPOSAL ─extract─▶ INTENTS (add / modify / remove one use case) ─▶ review ─▶ approve
 *         ─derive─▶ PROOFS (pending) ─▶ code (anyone) ─▶ prove (+ every use case's proofs) ─▶ apply
 *
 * Everything here is pure and deterministic: no file system, no LLM. The LLM
 * answers come in through `proposalFromExtraction` and `proofsFromDerivation`
 * (see llm/proposal.ts), which check them before anything is kept. The proofs
 * run on the existing engine: each (model, subject) group becomes a ProofPlan
 * over a synthetic approved artifact, so verdicts are the engine's verdicts.
 */
import { canonicalJson, hashOf } from "./artifact.js";
import { show, validateExpr, type Expr } from "./spec/expr.js";
import { sha256Hex } from "./sha256.js";
import type { RustProver } from "./rust.js";
import { tabulate } from "./rust.js";
import { checkConsistency, verify, type VerifyOptions } from "./verify/engine.js";
import type { Grounds, ProposalOrigin, ProposalSetup } from "./bootstrap.js";
import {
  BEYOND_PROOF_REASONS,
  type BeyondProofReason,
  type ClauseVerdict,
  type DecisionOption,
  type Env,
  type Evidence,
  type IntentArtifact,
  type Model,
  type ProofPlan,
  type Provability,
  type SourceSpan,
  type Subject,
  type Value,
} from "./types.js";

// ---------------------------------------------------------------------------
// Entities
// ---------------------------------------------------------------------------

export type ChangeKind = "add" | "modify" | "remove";
export const CHANGE_KINDS: readonly ChangeKind[] = ["add", "modify", "remove"];

/** How proposals are reviewed, from the `review` block of `.lastro/config.json`. */
export interface ProposalPolicy {
  /** Distinct reviewers who must approve each intent. Default 1. */
  approvals: number;
  /** Whether the person who submitted the prose counts as a reviewer. Default true. */
  submitterCanApprove: boolean;
}

export const DEFAULT_POLICY: ProposalPolicy = { approvals: 1, submitterCanApprove: true };

export type ProofCheck =
  | { kind: "clause"; requires?: Expr; ensures: Expr; assumptions: string[] }
  | { kind: "scenario"; input: Record<string, Value>; expect: Expr };

/** A deterministic check of one intent. After it passes, it belongs to its use case as a regression test. */
export interface ProofEntry {
  id: string; // "P-007.PF-05"
  slug: string;
  name: string;
  description: string;
  intent: string; // "P-007.I-03"
  useCase: string; // "UC-012"
  model: string;
  subject: string;
  check: ProofCheck;
  /** The superseded proofs this one takes the place of. */
  replaces?: string[];
  status: "pending" | "passed" | "failed";
  verdict?: ClauseVerdict;
  evidence?: Evidence;
}

/** One thing the application does, as the code implements it, with the proofs that keep it working. */
export interface UseCase {
  id: string; // "UC-012"
  slug: string;
  name: string;
  description: string;
  proofs: ProofEntry[];
  /** Applied proposals that added or changed it. */
  history: string[];
}

export interface ProofReplacement {
  proof: string;
  reason: string;
}

export type ProposalIntentStatus = "proposed" | "approved" | "amended" | "rejected";

export interface ProposalIntent {
  id: string; // "P-007.I-03"
  slug: string;
  name: string;
  description: string;
  change: ChangeKind;
  useCase: string;
  provability: Provability;
  beyondProofReason?: BeyondProofReason;
  source: SourceSpan;
  replaces: ProofReplacement[];
  status: ProposalIntentStatus;
  approvals: string[];
  reviewNote?: string;
  originalDescription?: string;
  /** Bootstrap: the files, tests and commits the behavior was read from. */
  grounds?: Grounds;
}

export interface UseCaseChange {
  useCase: string;
  slug: string;
  change: ChangeKind;
  name: string;
  /** modify/remove: copied from the use case by the SDK, never by the LLM. */
  before?: string;
  /** add/modify: the complete new description, every intent combined. */
  after?: string;
  intents: string[];
  /** Set when an intent of this use case was amended or rejected: `after` must be rewritten. */
  stale?: boolean;
}

export interface ProposalDecision {
  id: string; // "D1"
  slug: string;
  name: string;
  origin: "extraction" | "derivation" | "consistency-check";
  question: string;
  options: DecisionOption[];
  recommended: number;
  intents: string[];
  chosen?: number;
  answeredBy?: string;
}

export interface ThreadComment {
  by: string;
  text: string;
}

export interface Thread {
  id: string; // "T1"
  /** An intent id, a proof id, or "prose". */
  on: string;
  by: string;
  comments: ThreadComment[];
  resolved: boolean;
  resolvedBy?: string;
}

export type ProposalStatus = "open" | "changes-requested" | "approved" | "proving" | "proved" | "applied" | "closed";

/** One proof's outcome in the last run. */
export interface ProofOutcome {
  proof: string;
  useCase: string;
  intent: string;
  /** "proposal": derived for this proposal; "regression": a use case's proof; "superseded": replaced by this proposal. */
  role: "proposal" | "regression" | "superseded";
  verdict: ClauseVerdict;
  passed: boolean;
  counterexample?: { input: Env; output: Env | null };
  note?: string;
}

export interface ProposalRun {
  ok: boolean;
  outcomes: ProofOutcome[];
  /** Use-case proofs (not superseded) that failed. */
  regressions: string[];
  /** Superseded proofs that still pass: the behavior they guarded may not have changed. */
  replacedStillPass: string[];
  /** Use cases that would have no proofs after apply. Reported, never blocking. */
  unproved: string[];
  enginesAgree?: boolean;
}

export interface Proposal {
  lastro: "proposal/v1";
  id: string; // "P-007"
  slug: string;
  name: string;
  description: string;
  status: ProposalStatus;
  submittedBy: string;
  prose: string;
  baseline: string;
  intents: ProposalIntent[];
  useCases: UseCaseChange[];
  decisions: ProposalDecision[];
  threads: Thread[];
  notPromised: string[];
  policy: ProposalPolicy;
  proofs: ProofEntry[];
  supersedes: string[];
  lastRun?: ProposalRun;
  hash?: string;
  /** Where the proposal came from; a bootstrap proposal was read from the repository (see bootstrap.ts). */
  origin?: ProposalOrigin;
  /** Models and subjects the proposal adds to `.lastro/config.json` at apply, reviewed with its intents. */
  setup?: ProposalSetup;
}

export class ProposalError extends Error {
  constructor(public readonly problems: string[]) {
    super(`Invalid proposal:\n - ${problems.join("\n - ")}`);
  }
}

// ---------------------------------------------------------------------------
// Ids, slugs, baseline
// ---------------------------------------------------------------------------

export const SLUG = /^[a-z0-9]+(-[a-z0-9]+)*$/;

export const pad = (n: number, width: number) => String(n).padStart(width, "0");

/** The next free number after every id in `ids` ("UC-001" style). Numbers are never reused. */
export function nextId(prefix: "UC" | "P", ids: string[]): string {
  const max = ids.reduce((m, id) => Math.max(m, Number(id.match(new RegExp(`^${prefix}-(\\d+)$`))?.[1] ?? 0)), 0);
  return `${prefix}-${pad(max + 1, 3)}`;
}

export const fileNameOf = (e: { id: string; slug: string }) => `${e.id}-${e.slug}.json`;

/** sha256 over the canonical use cases without their proofs: what the prose was read against. */
export function baselineOf(useCases: UseCase[]): string {
  const core = [...useCases].sort((a, b) => a.id.localeCompare(b.id)).map(({ id, slug, name, description }) => ({ id, slug, name, description }));
  return "sha256:" + sha256Hex(canonicalJson(core));
}

export function readPolicy(config: { review?: Partial<ProposalPolicy>; requiredApprovals?: number } | undefined): ProposalPolicy {
  return {
    approvals: Math.max(1, config?.review?.approvals ?? config?.requiredApprovals ?? DEFAULT_POLICY.approvals),
    submitterCanApprove: config?.review?.submitterCanApprove ?? DEFAULT_POLICY.submitterCanApprove,
  };
}

/** Whitespace- and case-insensitive search, so a quote with reflowed lines still anchors. */
export function anchor(prose: string, quote: string): SourceSpan | undefined {
  const exact = prose.indexOf(quote);
  if (quote && exact >= 0) return { quote, start: exact, end: exact + quote.length };
  const words = quote.trim().split(/\s+/).filter(Boolean);
  if (!words.length) return undefined;
  const re = new RegExp(words.map((w) => w.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")).join("\\s+"), "i");
  const m = re.exec(prose);
  return m ? { quote: m[0], start: m.index, end: m.index + m[0].length } : undefined;
}

// ---------------------------------------------------------------------------
// Extraction: the LLM's answer, checked
// ---------------------------------------------------------------------------

/** What the extract step returns. Use cases are named by slug; the SDK assigns ids. */
export interface ProposeOut {
  name: string;
  slug: string;
  description: string;
  intents: {
    slug: string;
    name: string;
    description: string;
    change: ChangeKind;
    useCase: string; // a use-case slug: existing for modify/remove, new for add
    provability: Provability;
    beyondProofReason: string; // "none" unless beyond-proof
    quote: string;
    replaces: ProofReplacement[];
  }[];
  useCases: { slug: string; change: ChangeKind; name: string; after: string }[];
  decisions: { slug: string; name: string; question: string; options: DecisionOption[]; recommended: number; intents: string[] }[];
  notPromised: string[];
}

export interface ProposeInput {
  id: string;
  prose: string;
  submittedBy: string;
  useCases: UseCase[];
  policy: ProposalPolicy;
  /** Use-case ids other open proposals already reserved for their adds; new adds number after them too. */
  reserved?: string[];
}

/** Use-case ids reserved by the adds of proposals that are not applied or closed (excluding `except`). */
export function reservedUseCaseIds(proposals: Proposal[], except?: string): string[] {
  return proposals.filter((p) => p.id !== except && p.status !== "applied" && p.status !== "closed").flatMap((p) => p.useCases.filter((c) => c.change === "add").map((c) => c.useCase));
}

/** Build a proposal from the extract step's answer. Throws ProposalError with every problem found. */
export function proposalFromExtraction(input: ProposeInput, out: ProposeOut): Proposal {
  const { id, prose, useCases } = input;
  const problems: string[] = [];
  const bySlug = new Map(useCases.map((u) => [u.slug, u]));
  const reserved = new Map<string, string>();
  let next = Number(nextId("UC", [...useCases.map((u) => u.id), ...(input.reserved ?? [])]).slice(3));
  const ucId = (slug: string, change: ChangeKind): string | undefined => {
    const existing = bySlug.get(slug);
    if (existing) return existing.id;
    if (change !== "add") return undefined;
    if (!reserved.has(slug)) reserved.set(slug, `UC-${pad(next++, 3)}`);
    return reserved.get(slug);
  };

  if (!SLUG.test(out.slug ?? "")) problems.push(`proposal slug "${out.slug}" is not kebab-case`);
  const seen = new Set<string>();
  const intents: ProposalIntent[] = (out.intents ?? []).map((d, n) => {
    const iid = `${id}.I-${pad(n + 1, 2)}`;
    if (!SLUG.test(d.slug)) problems.push(`${iid}: slug "${d.slug}" is not kebab-case`);
    if (seen.has(d.slug)) problems.push(`${iid}: duplicate intent slug "${d.slug}"`);
    seen.add(d.slug);
    if (!CHANGE_KINDS.includes(d.change)) problems.push(`${iid}: unknown change "${d.change}"`);
    const existing = bySlug.get(d.useCase);
    if (d.change === "add" && existing) problems.push(`${iid}: add names existing use case ${existing.id} (${d.useCase}); an add reserves a new slug`);
    if (d.change !== "add" && !existing) problems.push(`${iid}: ${d.change} names unknown use case "${d.useCase}"`);
    if (d.change === "add" && !SLUG.test(d.useCase)) problems.push(`${iid}: new use-case slug "${d.useCase}" is not kebab-case`);
    const source = anchor(prose, d.quote);
    if (!source) problems.push(`${iid}: quote not found in prose: "${d.quote}"`);
    const reason = d.beyondProofReason && d.beyondProofReason !== "none" ? (d.beyondProofReason as BeyondProofReason) : undefined;
    if (d.provability === "beyond-proof" && !reason) problems.push(`${iid}: beyond-proof intent needs a beyondProofReason`);
    if (reason && !BEYOND_PROOF_REASONS.includes(reason)) problems.push(`${iid}: unknown beyondProofReason "${reason}"`);
    const useCase = ucId(d.useCase, d.change) ?? d.useCase;
    let replaces = d.replaces ?? [];
    if (d.change === "remove" && existing) replaces = existing.proofs.map((p) => ({ proof: p.id, reason: "the use case is removed" }));
    else if (d.change === "add" && replaces.length) problems.push(`${iid}: an add cannot replace proofs`);
    else
      for (const r of replaces) {
        const owner = useCases.find((u) => u.proofs.some((p) => p.id === r.proof));
        if (!owner) problems.push(`${iid}: replaces unknown proof ${r.proof}`);
        else if (owner.id !== useCase) problems.push(`${iid}: replaces ${r.proof}, which belongs to ${owner.id}, not ${useCase}`);
      }
    return {
      id: iid,
      slug: d.slug,
      name: d.name,
      description: d.description,
      change: d.change,
      useCase,
      provability: d.provability,
      ...(reason ? { beyondProofReason: reason } : {}),
      source: source ?? { quote: d.quote, start: 0, end: 0 },
      replaces,
      status: "proposed" as const,
      approvals: [],
    };
  });

  // Exactly one UseCaseChange per touched use case; kinds must fit together.
  const decisions: ProposalDecision[] = [];
  const changes: UseCaseChange[] = [];
  const touched = [...new Set(intents.map((i) => i.useCase))];
  for (const uc of touched) {
    const its = intents.filter((i) => i.useCase === uc);
    const slug = [...reserved].find(([, v]) => v === uc)?.[0] ?? useCases.find((u) => u.id === uc)?.slug ?? uc;
    const entries = (out.useCases ?? []).filter((c) => c.slug === slug);
    if (entries.length !== 1) {
      problems.push(`${uc} (${slug}): expected exactly one use-case change, got ${entries.length}`);
      continue;
    }
    const entry = entries[0]!;
    const kinds = [...new Set(its.map((i) => i.change))];
    if (kinds.length > 1) {
      decisions.push({
        id: "",
        slug: `change-kind-${slug}`,
        name: `How ${slug} changes`,
        origin: "extraction",
        question: `The intents on ${uc} (${slug}) disagree on whether it is added, modified or removed. Which one is meant?`,
        options: kinds.map((k) => ({ label: k, consequence: `${uc} is ${k === "add" ? "added" : k === "modify" ? "modified" : "removed"}; reject the intents that say otherwise.` })),
        recommended: 0,
        intents: its.map((i) => i.id),
      });
    } else if (entry.change !== kinds[0]) problems.push(`${uc} (${slug}): the use-case change says ${entry.change} but its intents say ${kinds[0]}`);
    const existing = useCases.find((u) => u.id === uc);
    if (entry.change !== "remove" && !entry.after?.trim()) problems.push(`${uc} (${slug}): ${entry.change} needs the complete after description`);
    changes.push({
      useCase: uc,
      slug,
      change: entry.change,
      name: entry.name || existing?.name || slug,
      ...(existing && entry.change !== "add" ? { before: existing.description } : {}),
      ...(entry.change !== "remove" ? { after: entry.after } : {}),
      intents: its.map((i) => i.id),
    });
  }
  for (const c of out.useCases ?? []) if (!changes.some((x) => x.slug === c.slug) && !problems.some((p) => p.includes(`(${c.slug})`))) problems.push(`use-case change "${c.slug}" has no intent`);

  const intentBySlug = new Map(intents.map((i) => [i.slug, i.id]));
  for (const d of out.decisions ?? []) {
    const dslug = d.slug || `decision-${decisions.length + 1}`;
    if (d.options.length < 2) problems.push(`decision "${dslug}" needs at least two options`);
    if (d.recommended < 0 || d.recommended >= d.options.length) problems.push(`decision "${dslug}": recommended out of range`);
    const refs = d.intents.map((s) => intentBySlug.get(s) ?? (intents.some((i) => i.id === s) ? s : undefined));
    d.intents.forEach((s, k) => refs[k] === undefined && problems.push(`decision "${dslug}": unknown intent "${s}"`));
    decisions.unshift({ id: "", slug: dslug, name: d.name || d.question, origin: "extraction", question: d.question, options: d.options, recommended: d.recommended, intents: refs.filter((x): x is string => !!x) });
  }
  if (!intents.length) problems.push("the extraction found no intent");
  if (problems.length) throw new ProposalError(problems);
  decisions.forEach((d, k) => (d.id = `D${k + 1}`));

  const p: Proposal = {
    lastro: "proposal/v1",
    id,
    slug: out.slug,
    name: out.name,
    description: out.description,
    status: "open",
    submittedBy: input.submittedBy,
    prose,
    baseline: baselineOf(useCases),
    intents,
    useCases: changes,
    decisions,
    threads: [],
    notPromised: out.notPromised ?? [],
    policy: { ...input.policy },
    proofs: [],
    supersedes: [],
  };
  return p;
}

/**
 * Carry a previous review over to a fresh extraction of edited prose, like a pull
 * request keeps the approvals of untouched files. An intent with the same change,
 * use case and description (ignoring case and spacing) keeps its status and approvals;
 * an amended one is reapplied when the LLM repeats its original reading of the same
 * quote. Everything else needs review. Decisions with the same question keep their
 * answer, and threads carry over.
 */
export function carryProposalReview(previous: Proposal, next: Proposal): Proposal {
  const out = structuredClone(next);
  const norm = (s: string) => s.replace(/\s+/g, " ").trim().toLowerCase();
  const key = (i: ProposalIntent, d: string) => `${i.change}|${i.useCase}|${norm(d)}`;
  const reviewed = previous.intents.filter((i) => i.status !== "proposed" || i.approvals.length);
  for (const it of out.intents) {
    const same = reviewed.find((p) => key(p, p.description) === key(it, it.description));
    const amended = reviewed.find((p) => p.status === "amended" && p.originalDescription && key(p, p.originalDescription) === key(it, it.description) && norm(p.source.quote) === norm(it.source.quote));
    const from = same ?? amended;
    if (!from) continue;
    if (amended && !same) {
      it.originalDescription = it.description;
      it.description = amended.description;
      it.replaces = structuredClone(amended.replaces);
    } else if (same?.originalDescription) it.originalDescription = same.originalDescription;
    it.status = from.status;
    it.approvals = [...from.approvals];
    if (from.reviewNote) it.reviewNote = from.reviewNote;
  }
  for (const d of out.decisions) {
    const prev = previous.decisions.find((p) => norm(p.question) === norm(d.question));
    if (prev?.chosen !== undefined && prev.chosen < d.options.length) {
      d.chosen = prev.chosen;
      if (prev.answeredBy) d.answeredBy = prev.answeredBy;
    }
  }
  out.threads = structuredClone(previous.threads);
  out.policy = { ...previous.policy };
  out.submittedBy = previous.submittedBy;
  return withStatus(out);
}

// ---------------------------------------------------------------------------
// Review
// ---------------------------------------------------------------------------

/** Approvals that count under the proposal's policy. */
export function countedApprovals(p: Proposal, it: ProposalIntent): string[] {
  if (it.status !== "approved" && it.status !== "amended") return [];
  return it.approvals.filter((a) => p.policy.submitterCanApprove || a !== p.submittedBy);
}

/** Intents that go forward to derivation and implementation. */
export function forwardIntents(p: Proposal): ProposalIntent[] {
  return p.intents.filter((i) => i.status === "approved" || i.status === "amended");
}

/** Use-case changes that still apply (at least one intent not rejected). */
export function liveChanges(p: Proposal): UseCaseChange[] {
  const live = new Set(forwardIntents(p).map((i) => i.id));
  return p.useCases.filter((c) => c.intents.some((i) => live.has(i)));
}

/** What blocks approval: unreviewed intents, missing approvals, stale use cases, open decisions and threads. */
export function pendingProposal(p: Proposal): string[] {
  const out: string[] = [];
  const need = Math.max(1, p.policy.approvals);
  for (const it of p.intents) {
    if (it.status === "rejected") continue;
    const n = countedApprovals(p, it).length;
    if (it.status === "proposed" && !it.approvals.length) out.push(`${it.id} not reviewed`);
    else if (n < need) out.push(`${it.id} needs ${need - n} more approval(s)${!p.policy.submitterCanApprove ? " from someone other than the submitter" : ""}`);
  }
  for (const c of p.useCases) if (c.stale && c.intents.some((i) => p.intents.find((x) => x.id === i)?.status !== "rejected")) out.push(`${c.useCase} after is stale: rewrite it or recombine it`);
  for (const d of p.decisions) if (d.chosen === undefined) out.push(`${d.id} unanswered: ${d.question}`);
  for (const t of p.threads) if (!t.resolved) out.push(`${t.id} open thread on ${t.on}`);
  return out;
}

/** The part of a proposal that approval binds. */
function reviewCore(p: Proposal) {
  const { id, slug, name, description, submittedBy, prose, baseline, intents, useCases, decisions, threads, notPromised, policy, supersedes } = p;
  return { lastro: p.lastro, id, slug, name, description, submittedBy, prose, baseline, intents, useCases, decisions, threads, notPromised, policy, supersedes, ...(p.origin ? { origin: p.origin } : {}), ...(p.setup ? { setup: p.setup } : {}) };
}

export function supersedesOf(p: Proposal): string[] {
  return [...new Set(forwardIntents(p).flatMap((i) => i.replaces.map((r) => r.proof)))].sort();
}

/** True when the reviewed part has not changed since approval. */
export function isProposalApproved(p: Proposal): boolean {
  return !!p.hash && hashOf(reviewCore(p)) === p.hash;
}

/** The status that follows from the proposal's content (applied and closed are kept). */
export function withStatus(p: Proposal): Proposal {
  if (p.status === "applied" || p.status === "closed") return p;
  if (isProposalApproved(p)) {
    p.status = !p.proofs.length ? "approved" : p.lastRun?.ok && p.proofs.every((x) => x.status === "passed") ? "proved" : "proving";
  } else {
    delete p.hash;
    const stale = p.useCases.some((c) => c.stale);
    const raised = p.decisions.some((d) => d.origin !== "extraction" && d.chosen === undefined);
    p.status = stale || raised ? "changes-requested" : "open";
  }
  return p;
}

export class ProposalReview {
  private p: Proposal;

  constructor(
    proposal: Proposal,
    private readonly reviewer: string,
  ) {
    if (proposal.status === "applied" || proposal.status === "closed") throw new Error(`${proposal.id} is ${proposal.status}`);
    if (!reviewer) throw new Error("review needs the reviewer's name");
    this.p = structuredClone(proposal);
  }

  private intent(id: string): ProposalIntent {
    const it = this.p.intents.find((x) => x.id === id || x.id === `${this.p.id}.${id}` || x.slug === id);
    if (!it) throw new Error(`Unknown intent ${id}`);
    return it;
  }

  private change(useCase: string): UseCaseChange {
    const c = this.p.useCases.find((x) => x.useCase === useCase || x.slug === useCase);
    if (!c) throw new Error(`${useCase} is not touched by ${this.p.id}`);
    return c;
  }

  /** Approving an intent accepts the replacement of the proofs it lists. */
  approve(...ids: string[]): this {
    if (this.reviewer === this.p.submittedBy && !this.p.policy.submitterCanApprove)
      throw new Error(`${this.reviewer} submitted ${this.p.id}; under this policy the submitter can amend but not approve`);
    for (const id of ids) {
      const it = this.intent(id);
      if (it.status === "rejected") throw new Error(`${it.id} is rejected; amend it to bring it back`);
      if (it.status === "proposed") it.status = "approved";
      if (!it.approvals.includes(this.reviewer)) it.approvals.push(this.reviewer);
    }
    return this;
  }

  /** Approve every intent that is not rejected. */
  approveAll(): this {
    return this.approve(...this.p.intents.filter((i) => i.status !== "rejected").map((i) => i.id));
  }

  /** Rewrite an intent. It needs approval again; its use case's after becomes stale. */
  amend(id: string, description: string, patch: { replaces?: ProofReplacement[]; provability?: Provability; beyondProofReason?: BeyondProofReason } = {}, note = "amended by reviewer"): this {
    const it = this.intent(id);
    if (patch.replaces) {
      const uc = this.p.useCases.find((c) => c.useCase === it.useCase);
      if (!uc) throw new Error(`${it.useCase} is not touched by ${this.p.id}`);
    }
    it.originalDescription ??= it.description;
    it.description = description;
    if (patch.replaces) it.replaces = patch.replaces;
    if (patch.provability) it.provability = patch.provability;
    if (patch.beyondProofReason) it.beyondProofReason = patch.beyondProofReason;
    it.status = "amended";
    it.reviewNote = `${note} (${this.reviewer})`;
    it.approvals = [this.reviewer];
    this.change(it.useCase).stale = true;
    return this;
  }

  /** The intent should not be built. Its use case's after becomes stale. */
  reject(id: string, note: string): this {
    const it = this.intent(id);
    it.status = "rejected";
    it.reviewNote = `${note} (${this.reviewer})`;
    it.approvals = [];
    const c = this.change(it.useCase);
    c.stale = c.intents.some((i) => this.p.intents.find((x) => x.id === i)?.status !== "rejected");
    return this;
  }

  /** Write a use case's after by hand (or pass the LLM's recombination). Its intents need review again. */
  rewriteAfter(useCase: string, after: string, name?: string): this {
    const c = this.change(useCase);
    if (c.change === "remove") throw new Error(`${c.useCase} is removed; it has no after`);
    c.after = after;
    if (name) c.name = name;
    delete c.stale;
    for (const i of c.intents) {
      const it = this.intent(i);
      if (it.status === "rejected") continue;
      it.approvals = [];
      if (it.status === "approved") it.status = "proposed";
    }
    return this;
  }

  answer(decisionId: string, option: number): this {
    const d = this.p.decisions.find((x) => x.id === decisionId || x.slug === decisionId);
    if (!d) throw new Error(`Unknown decision ${decisionId}`);
    if (!Number.isInteger(option) || option < 0 || option >= d.options.length) throw new Error(`Option ${option} out of range for ${d.id}`);
    d.chosen = option;
    d.answeredBy = this.reviewer;
    return this;
  }

  /** Start a thread on an intent, a proof, a use case or the prose. */
  comment(on: string, text: string): this {
    const full = `${this.p.id}.${on}`;
    const target = on === "prose" ? on : (this.p.intents.find((i) => i.id === on || i.id === full || i.slug === on)?.id ?? this.p.proofs.find((x) => x.id === on || x.id === full || x.slug === on)?.id ?? this.p.useCases.find((c) => c.useCase === on || c.slug === on)?.useCase);
    if (!target) throw new Error(`Nothing called ${on} in ${this.p.id}`);
    this.p.threads.push({ id: `T${this.p.threads.length + 1}`, on: target, by: this.reviewer, comments: [{ by: this.reviewer, text }], resolved: false });
    return this;
  }

  reply(threadId: string, text: string): this {
    this.thread(threadId).comments.push({ by: this.reviewer, text });
    return this;
  }

  /** Threads are resolved by whoever started them or by the submitter. */
  resolve(threadId: string): this {
    const t = this.thread(threadId);
    if (this.reviewer !== t.by && this.reviewer !== this.p.submittedBy) throw new Error(`${t.id} can be resolved by ${t.by} or ${this.p.submittedBy}`);
    t.resolved = true;
    t.resolvedBy = this.reviewer;
    return this;
  }

  private thread(id: string): Thread {
    const t = this.p.threads.find((x) => x.id === id);
    if (!t) throw new Error(`Unknown thread ${id}`);
    return t;
  }

  done(): Proposal {
    const out = structuredClone(this.p);
    out.supersedes = supersedesOf(out);
    // Any change after approval withdraws it: derived proofs and runs go with it.
    if (out.hash && !isProposalApproved(out)) {
      delete out.hash;
      out.proofs = [];
      delete out.lastRun;
    }
    return withStatus(out);
  }
}

export const reviewProposal = (p: Proposal, reviewer: string) => new ProposalReview(p, reviewer);

/**
 * Approve a fully reviewed proposal. The hash binds the baseline: if the use
 * cases changed since extraction, the approval is refused and the prose must be
 * extracted again.
 */
export function approveProposal(p: Proposal, useCases: UseCase[]): Proposal {
  if (isProposalApproved(p)) return p;
  const pending = pendingProposal(p);
  if (baselineOf(useCases) !== p.baseline) pending.push(`the use cases changed since ${p.id} was extracted: run propose again`);
  if (pending.length) throw new ProposalError(pending);
  const out = structuredClone(p);
  delete out.hash;
  out.supersedes = supersedesOf(out);
  out.proofs = [];
  delete out.lastRun;
  out.hash = hashOf(reviewCore(out));
  return withStatus(out);
}

// ---------------------------------------------------------------------------
// Derivation: the LLM's proofs, checked
// ---------------------------------------------------------------------------

export interface DeriveV1Out {
  proofs: {
    intent: string;
    slug: string;
    name: string;
    description: string;
    model: string;
    subject: string;
    kind: "clause" | "scenario";
    requires: string; // LSL JSON string, "" for none (clause)
    ensures: string; // LSL JSON string (clause)
    assumptions: string[];
    inputJson: string; // scenario
    expect: string; // scenario
    replaces: string[];
  }[];
  decisions: { slug: string; name: string; question: string; options: DecisionOption[]; recommended: number; intents: string[]; breaks: string }[];
}

export interface ProofSetup {
  models: Record<string, Model>;
  /** Subject names declared in `.lastro/config.json`. */
  subjects: string[];
}

/**
 * Check the derive step's answer and attach the pending proofs. When derivation
 * returns decisions (a removal the model cannot express, an existing proof the
 * approved behavior breaks but no intent replaces), the proposal goes back to
 * review: a broken proof is added to its intent's `replaces` and that intent
 * needs approval again. Derivation never retires a proof by itself.
 */
export function proofsFromDerivation(p: Proposal, useCases: UseCase[], setup: ProofSetup, out: DeriveV1Out, opts: VerifyOptions = {}): Proposal {
  if (!isProposalApproved(p)) throw new ProposalError([`${p.id} is not approved`]);
  const problems: string[] = [];
  const forward = forwardIntents(p);
  const byId = new Map(forward.map((i) => [i.id, i]));
  const slugs = new Set<string>();
  const proofs: ProofEntry[] = [];
  (out.proofs ?? []).forEach((d, n) => {
    const pid = `${p.id}.PF-${pad(n + 1, 2)}`;
    const it = byId.get(d.intent);
    if (!it) problems.push(`${pid}: intent ${d.intent} is not an approved or amended intent of ${p.id}`);
    if (!SLUG.test(d.slug ?? "")) problems.push(`${pid}: slug "${d.slug}" is not kebab-case`);
    if (slugs.has(d.slug)) problems.push(`${pid}: duplicate proof slug "${d.slug}"`);
    slugs.add(d.slug);
    const model = setup.models[d.model];
    if (!model) problems.push(`${pid}: unknown model "${d.model}" (declared: ${Object.keys(setup.models).join(", ") || "none"})`);
    if (!setup.subjects.includes(d.subject)) problems.push(`${pid}: unknown subject "${d.subject}" (declared: ${setup.subjects.join(", ") || "none"})`);
    for (const r of d.replaces ?? []) if (!it?.replaces.some((x) => x.proof === r)) problems.push(`${pid}: replaces ${r}, which its intent does not list`);
    if (!model) return;
    const parse = (where: string, s: string): Expr | undefined => {
      let e: unknown;
      try {
        e = typeof s === "string" ? JSON.parse(s) : s;
      } catch {
        problems.push(`${pid}.${where}: not valid JSON: ${s}`);
        return undefined;
      }
      const errs = validateExpr(e, model, [], `${pid}.${where}`);
      if (errs.length) {
        problems.push(...errs);
        return undefined;
      }
      return e as Expr;
    };
    let check: ProofCheck | undefined;
    if (d.kind === "clause") {
      const requires = d.requires && String(d.requires).trim() ? parse("requires", d.requires) : undefined;
      const ensures = parse("ensures", d.ensures);
      if (ensures) check = { kind: "clause", ...(requires ? { requires } : {}), ensures, assumptions: d.assumptions ?? [] };
    } else if (d.kind === "scenario") {
      let input: Record<string, Value> | undefined;
      try {
        input = (typeof d.inputJson === "string" ? JSON.parse(d.inputJson) : d.inputJson) as Record<string, Value>;
      } catch {
        problems.push(`${pid}.input: not valid JSON`);
      }
      for (const k of Object.keys(model.inputs)) if (input && !(k in input)) problems.push(`${pid}.input: missing model input ${k}`);
      const expect = parse("expect", d.expect);
      if (input && expect) check = { kind: "scenario", input, expect };
    } else problems.push(`${pid}: unknown kind "${d.kind}"`);
    if (!check || !it) return;
    proofs.push({
      id: pid,
      slug: d.slug,
      name: d.name,
      description: d.description,
      intent: it.id,
      useCase: it.useCase,
      model: d.model,
      subject: d.subject,
      check,
      ...(d.replaces?.length ? { replaces: [...d.replaces] } : {}),
      status: "pending",
    });
  });

  const decided = (out.decisions ?? []).map((d) => ({ ...d, intents: d.intents.filter((x) => byId.has(x)) }));
  for (const d of out.decisions ?? []) {
    for (const x of d.intents) if (!byId.has(x)) problems.push(`decision "${d.slug}": unknown or rejected intent ${x}`);
    if (d.options.length < 2) problems.push(`decision "${d.slug}" needs at least two options`);
    if (d.breaks) {
      const owner = useCases.find((u) => u.proofs.some((x) => x.id === d.breaks));
      if (!owner) problems.push(`decision "${d.slug}": breaks unknown proof ${d.breaks}`);
    }
  }
  // Coverage: every forward intent has a proof, unless it is beyond proof or a decision covers it.
  for (const it of forward) {
    if (proofs.some((x) => x.intent === it.id) || decided.some((d) => d.intents.includes(it.id))) continue;
    if (it.change === "remove") problems.push(`${it.id}: a remove needs a proof that the old behavior is gone, or a decision`);
    else if (it.provability !== "beyond-proof") problems.push(`${it.id}: no proof derived`);
  }
  if (problems.length) throw new ProposalError(problems);

  // Contradictions between the new clauses of one model come back as decisions.
  const contradictions: ProposalDecision[] = [];
  for (const [name, plan] of plansByGroup(proofs, setup.models, (x) => x.model)) {
    for (const c of checkConsistency(plan, opts))
      contradictions.push({ id: "", slug: `contradiction-${name}-${contradictions.length + 1}`, name: `Contradiction in ${name}`, origin: "consistency-check", question: c.question, options: c.options, recommended: c.recommended, intents: c.intentIds });
  }

  const next = structuredClone(p);
  const raised = [
    ...decided.map((d) => ({ id: "", slug: d.slug, name: d.name, origin: "derivation" as const, question: d.question, options: d.options, recommended: d.recommended, intents: d.intents, breaks: d.breaks })),
    ...contradictions.map((c) => ({ ...c, breaks: "" })),
  ];
  if (!raised.length) {
    next.proofs = proofs;
    delete next.lastRun;
    return withStatus(next);
  }
  // Back to review: the approval is withdrawn and no proof is kept.
  for (const d of raised) {
    if (next.decisions.some((x) => x.question === d.question)) continue;
    const { breaks, ...card } = d;
    next.decisions.push({ ...card, id: `D${next.decisions.length + 1}` });
    if (breaks)
      for (const iid of d.intents) {
        const it = next.intents.find((x) => x.id === iid)!;
        if (!it.replaces.some((r) => r.proof === breaks)) it.replaces.push({ proof: breaks, reason: d.question });
        it.status = "proposed";
        it.approvals = [];
      }
  }
  delete next.hash;
  next.proofs = [];
  delete next.lastRun;
  next.supersedes = supersedesOf(next);
  return withStatus(next);
}

// ---------------------------------------------------------------------------
// Proving: the proposal's proofs plus every use case's (regression)
// ---------------------------------------------------------------------------

function plansByGroup(proofs: ProofEntry[], models: Record<string, Model>, key: (p: ProofEntry) => string): Map<string, ProofPlan> {
  const out = new Map<string, ProofPlan>();
  for (const pr of proofs) {
    const k = key(pr);
    const model = models[pr.model];
    if (!model) continue;
    let plan = out.get(k);
    if (!plan) out.set(k, (plan = { lastro: "proof-plan/v0", artifactHash: "", model, clauses: [], assumptions: [], scenarios: [], beyondProof: [] }));
    if (pr.check.kind === "clause")
      plan.clauses.push({ id: pr.id, intentIds: [pr.id], description: pr.description, ...(pr.check.requires ? { requires: pr.check.requires } : {}), ensures: pr.check.ensures });
    else plan.scenarios.push({ id: pr.id, intentIds: [pr.id], given: pr.description, when: "the subject runs", then: show(pr.check.expect), input: pr.check.input, expect: pr.check.expect });
  }
  return out;
}

/** A synthetic approved artifact so the engine can run proofs that are not tied to a v0 artifact. */
function artifactFor(plan: ProofPlan): IntentArtifact {
  const ids = [...plan.clauses.map((c) => c.id), ...plan.scenarios.map((s) => s.id)];
  const a: IntentArtifact = {
    lastro: "intent-artifact/v0",
    prose: "",
    intents: ids.map((id) => ({ id, statement: id, kind: "behavior", provability: "formal", source: { quote: "", start: 0, end: 0 }, status: "approved" })),
    decisions: [],
    notPromised: [],
  };
  return { ...a, hash: hashOf(a) };
}

export interface RunOptions {
  /** The code under proof, by subject name. */
  subjects: Record<string, Subject>;
  models: Record<string, Model>;
  rust?: RustProver;
  verify?: Omit<VerifyOptions, "subject" | "traces">;
}

/** Run proofs grouped by (model, subject) on the engine (and the Rust prover, if given). */
export async function runProofs(proofs: { proof: ProofEntry; role: ProofOutcome["role"] }[], o: RunOptions): Promise<{ outcomes: ProofOutcome[]; enginesAgree?: boolean }> {
  const outcomes: ProofOutcome[] = [];
  let agree: boolean | undefined;
  const role = new Map(proofs.map((x) => [x.proof.id, x.role]));
  const all = proofs.map((x) => x.proof);
  const missing = (pr: ProofEntry, note: string): ProofOutcome => ({ proof: pr.id, useCase: pr.useCase, intent: pr.intent, role: role.get(pr.id)!, verdict: "not-run", passed: false, note });
  for (const pr of all) {
    if (!o.models[pr.model]) outcomes.push(missing(pr, `model ${pr.model} is not declared`));
    else if (!o.subjects[pr.subject]) outcomes.push(missing(pr, `subject ${pr.subject} is not available`));
  }
  const runnable = all.filter((pr) => o.models[pr.model] && o.subjects[pr.subject]);
  for (const [key, plan] of plansByGroup(runnable, o.models, (x) => `${x.model}\u0000${x.subject}`)) {
    const subject = o.subjects[key.split("\u0000")[1]!]!;
    const artifact = artifactFor(plan);
    plan.artifactHash = artifact.hash!;
    const report = await verify(artifact, plan, { ...o.verify, subject });
    let rust: ReturnType<RustProver["prove"]> | undefined;
    if (o.rust && plan.clauses.length) {
      rust = o.rust.prove(plan, await tabulate(plan.model, subject));
      const same = report.clauses.every((c) => rust!.clauses.find((r) => r.clauseId === c.clauseId)?.verdict === c.verdict);
      agree = (agree ?? true) && same;
    }
    for (const r of report.clauses) {
      const pr = runnable.find((x) => x.id === r.clauseId)!;
      outcomes.push({
        proof: pr.id,
        useCase: pr.useCase,
        intent: pr.intent,
        role: role.get(pr.id)!,
        verdict: r.verdict,
        passed: r.verdict === "proved" || r.verdict === "evidenced",
        ...(r.counterexample ? { counterexample: r.counterexample } : {}),
        ...(r.note ? { note: r.note } : {}),
      });
    }
    for (const r of report.scenarios) {
      const pr = runnable.find((x) => x.id === r.scenarioId)!;
      const input = (pr.check as { input: Env }).input;
      outcomes.push({
        proof: pr.id,
        useCase: pr.useCase,
        intent: pr.intent,
        role: role.get(pr.id)!,
        verdict: r.status === "passed" ? "evidenced" : "violated",
        passed: r.status === "passed",
        ...(r.status === "failed" ? { counterexample: { input, output: r.output ?? null } } : {}),
        ...(r.error ? { note: r.error } : {}),
      });
    }
  }
  const order = new Map(all.map((x, i) => [x.id, i]));
  outcomes.sort((a, b) => order.get(a.proof)! - order.get(b.proof)!);
  return { outcomes, ...(agree !== undefined ? { enginesAgree: agree } : {}) };
}

/** The use cases as they would be after apply (proofs not yet attached). */
function resultingUseCases(p: Proposal, useCases: UseCase[]): { id: string; proofs: number }[] {
  const sup = new Set(p.supersedes);
  const changes = liveChanges(p);
  const out: { id: string; proofs: number }[] = [];
  for (const u of useCases) {
    const c = changes.find((x) => x.useCase === u.id);
    if (c?.change === "remove") continue;
    out.push({ id: u.id, proofs: u.proofs.filter((x) => !sup.has(x.id)).length + p.proofs.filter((x) => x.useCase === u.id).length });
  }
  for (const c of changes) if (c.change === "add") out.push({ id: c.useCase, proofs: p.proofs.filter((x) => x.useCase === c.useCase).length });
  return out;
}

/**
 * Run the proposal's proofs and every use case's proofs except the superseded
 * ones (a failing one is a regression). Superseded proofs run too, only to note
 * the ones that still pass. Returns the proposal with each proof's verdict and
 * the run.
 */
export async function proveProposal(p: Proposal, useCases: UseCase[], o: RunOptions): Promise<{ proposal: Proposal; run: ProposalRun }> {
  if (!isProposalApproved(p)) throw new ProposalError([`${p.id} is not approved`]);
  if (!p.proofs.length && forwardIntents(p).some((i) => i.provability !== "beyond-proof")) throw new ProposalError([`${p.id} has no proofs: run derive first`]);
  const sup = new Set(p.supersedes);
  const removed = new Set(liveChanges(p).filter((c) => c.change === "remove").map((c) => c.useCase));
  const existing = useCases.flatMap((u) => u.proofs);
  const set = [
    ...p.proofs.map((proof) => ({ proof, role: "proposal" as const })),
    ...existing.filter((x) => !sup.has(x.id) && !removed.has(x.useCase)).map((proof) => ({ proof, role: "regression" as const })),
    ...existing.filter((x) => sup.has(x.id)).map((proof) => ({ proof, role: "superseded" as const })),
  ];
  const { outcomes, enginesAgree } = await runProofs(set, o);
  const regressions = outcomes.filter((x) => x.role === "regression" && !x.passed).map((x) => x.proof);
  const replacedStillPass = outcomes.filter((x) => x.role === "superseded" && x.passed).map((x) => x.proof);
  const unproved = resultingUseCases(p, useCases).filter((u) => !u.proofs).map((u) => u.id);
  const own = outcomes.filter((x) => x.role === "proposal");
  const ok = own.every((x) => x.passed) && !regressions.length && enginesAgree !== false;
  const run: ProposalRun = { ok, outcomes, regressions, replacedStillPass, unproved, ...(enginesAgree !== undefined ? { enginesAgree } : {}) };
  const next = structuredClone(p);
  for (const pr of next.proofs) {
    const r = own.find((x) => x.proof === pr.id);
    if (!r) continue;
    pr.status = r.passed ? "passed" : "failed";
    pr.verdict = r.verdict;
    pr.evidence = r.verdict === "proved" ? "formal" : r.verdict === "not-run" ? "none" : "empirical";
  }
  next.lastRun = run;
  return { proposal: withStatus(next), run };
}

/** Run every use case's proofs alone (`lastro regress`). */
export async function regress(useCases: UseCase[], o: RunOptions): Promise<ProposalRun> {
  const { outcomes, enginesAgree } = await runProofs(useCases.flatMap((u) => u.proofs.map((proof) => ({ proof, role: "regression" as const }))), o);
  const regressions = outcomes.filter((x) => !x.passed).map((x) => x.proof);
  return { ok: !regressions.length && enginesAgree !== false, outcomes, regressions, replacedStillPass: [], unproved: useCases.filter((u) => !u.proofs.length).map((u) => u.id), ...(enginesAgree !== undefined ? { enginesAgree } : {}) };
}

// ---------------------------------------------------------------------------
// Apply
// ---------------------------------------------------------------------------

export interface Applied {
  proposal: Proposal;
  /** The whole catalog after the change. */
  useCases: UseCase[];
  /** Use cases whose files must be written, and ids whose files must be deleted. */
  written: string[];
  removed: string[];
  baseline: string;
  /** Models and subjects to add to `.lastro/config.json` (a bootstrap's reviewed setup). */
  setup?: ProposalSetup;
}

/**
 * Attach the proofs and sync the use cases. Refuses unless the proposal is
 * approved, its baseline still matches and its last run passed every proof,
 * new and regression.
 */
export function applyProposal(p: Proposal, useCases: UseCase[]): Applied {
  const problems: string[] = [];
  if (!isProposalApproved(p)) problems.push(`${p.id} is not approved`);
  if (baselineOf(useCases) !== p.baseline) problems.push(`the use cases changed since ${p.id} was extracted: run propose again`);
  if (!p.lastRun) problems.push(`${p.id} has not been proved: run prove first`);
  else if (!p.lastRun.ok) problems.push(`${p.id}'s last run failed: ${[...p.lastRun.outcomes.filter((x) => x.role === "proposal" && !x.passed).map((x) => x.proof), ...p.lastRun.regressions].join(", ")}`);
  for (const pr of p.proofs) if (pr.status !== "passed") problems.push(`${pr.id} is ${pr.status}`);
  if (problems.length) throw new ProposalError(problems);

  const sup = new Set(p.supersedes);
  const out: UseCase[] = structuredClone(useCases);
  const written: string[] = [];
  const removed: string[] = [];
  const attach = (u: UseCase) => {
    u.proofs = [...u.proofs.filter((x) => !sup.has(x.id)), ...p.proofs.filter((x) => x.useCase === u.id).map((x) => structuredClone(x))];
    if (!u.history.includes(p.id)) u.history.push(p.id);
    written.push(u.id);
  };
  for (const c of liveChanges(p)) {
    const i = out.findIndex((u) => u.id === c.useCase);
    if (c.change === "remove") {
      if (i >= 0) out.splice(i, 1);
      removed.push(c.useCase);
    } else if (c.change === "add") {
      const u: UseCase = { id: c.useCase, slug: c.slug, name: c.name, description: c.after!, proofs: [], history: [] };
      attach(u);
      out.push(u);
    } else {
      const u = out[i]!;
      u.name = c.name;
      u.description = c.after!;
      attach(u);
    }
  }
  out.sort((a, b) => a.id.localeCompare(b.id));
  const next = structuredClone(p);
  next.status = "applied";
  return { proposal: next, useCases: out, written, removed, baseline: baselineOf(out), ...(p.setup ? { setup: structuredClone(p.setup) } : {}) };
}

// ---------------------------------------------------------------------------
// Feedback and receipt
// ---------------------------------------------------------------------------

function formalOf(pr: ProofEntry): string {
  return pr.check.kind === "clause" ? `${pr.check.requires ? `requires ${show(pr.check.requires)}; ` : ""}ensures ${show(pr.check.ensures)}` : `input ${JSON.stringify(pr.check.input)}; expect ${show(pr.check.expect)}`;
}

/** The last run's failures as a brief for whoever writes the code. */
export function proposalFeedback(p: Proposal, useCases: UseCase[]): string {
  const run = p.lastRun;
  if (!run) return `${p.id} has not been proved yet.`;
  const all = [...p.proofs, ...useCases.flatMap((u) => u.proofs)];
  const failing = run.outcomes.filter((x) => x.role !== "superseded" && !x.passed);
  if (!failing.length) return "Every proof passes." + (run.unproved.length ? ` Use cases with no proofs yet: ${run.unproved.join(", ")}.` : "");
  return [
    "The implementation breaks these proofs. Fix the code so each holds, without breaking the others:",
    ...failing.map((f) => {
      const pr = all.find((x) => x.id === f.proof);
      const what = f.role === "regression" ? `regression in ${f.useCase}` : `${f.intent}`;
      return `- ${f.proof} (${what}): ${pr?.description ?? ""}\n  ${pr ? formalOf(pr) : ""}${f.counterexample ? `\n  counterexample: ${JSON.stringify(f.counterexample.input)} returned ${JSON.stringify(f.counterexample.output)}` : f.note ? `\n  ${f.note}` : ""}`;
    }),
  ].join("\n");
}

const VERDICT_MARK: Record<string, string> = { proved: "✅ proved", evidenced: "🟡 evidenced", violated: "❌ violated", vacuous: "⚠️ vacuous", weak: "⚠️ weak", "not-run": "⏸ not run" };

/** Markdown receipt: the use-case diff, proofs gained and replaced, regressions. */
export function renderProposalReceipt(p: Proposal, useCases: UseCase[]): string {
  const L: string[] = [`# Receipt: ${p.id} ${p.name}`, "", `Status: **${p.status}** · submitted by ${p.submittedBy} · baseline \`${p.baseline}\`${p.hash ? ` · approval \`${p.hash}\`` : ""}`, "", p.description, ""];
  const run = p.lastRun;
  const all = [...p.proofs, ...useCases.flatMap((u) => u.proofs)];
  L.push("## Use cases", "");
  for (const c of p.useCases) {
    const its = p.intents.filter((i) => c.intents.includes(i.id));
    const live = its.filter((i) => i.status !== "rejected");
    L.push(`### ${c.change === "add" ? "➕" : c.change === "remove" ? "➖" : "✏️"} ${c.useCase} ${c.name} (${c.change}${live.length ? "" : ", every intent rejected"})`, "");
    if (c.before) L.push(`- Before: ${c.before}`);
    if (c.after && c.change !== "remove") L.push(`- After: ${c.after}`);
    for (const i of its) {
      L.push(`- **${i.id}** \`${i.status}\` ${i.description}`);
      for (const r of i.replaces) {
        const old = all.find((x) => x.id === r.proof);
        const by = p.proofs.filter((x) => x.replaces?.includes(r.proof)).map((x) => x.id);
        L.push(`  - replaces ${r.proof}${old ? ` (${old.description})` : ""}: ${r.reason}${by.length ? `; now guarded by ${by.join(", ")}` : ""}`);
      }
    }
    const gained = p.proofs.filter((x) => x.useCase === c.useCase);
    if (gained.length) L.push(`- Gains: ${gained.map((x) => x.id).join(", ")}`);
    L.push("");
  }
  if (p.proofs.length) {
    L.push("## Proofs", "");
    for (const pr of p.proofs) {
      const r = run?.outcomes.find((x) => x.proof === pr.id);
      L.push(`- **${pr.id}** ${r ? VERDICT_MARK[r.verdict] : pr.status} — ${pr.description} (${pr.intent}, ${pr.model}/${pr.subject})`, `  - \`${formalOf(pr)}\``);
      if (r?.counterexample) L.push(`  - Counterexample: \`${JSON.stringify(r.counterexample.input)}\` → \`${JSON.stringify(r.counterexample.output)}\``);
      if (r?.note) L.push(`  - Note: ${r.note}`);
    }
    L.push("");
  }
  if (run) {
    const reg = run.outcomes.filter((x) => x.role === "regression");
    L.push("## Regression", "", `${reg.length} use-case proof(s) ran; ${run.regressions.length} failed.${run.enginesAgree !== undefined ? ` Engines agree: ${run.enginesAgree ? "yes" : "NO"}.` : ""}`, "");
    for (const id of run.regressions) {
      const r = run.outcomes.find((x) => x.proof === id)!;
      L.push(`- ❌ **${id}** (${r.useCase})${r.counterexample ? `: \`${JSON.stringify(r.counterexample.input)}\` → \`${JSON.stringify(r.counterexample.output)}\`` : r.note ? `: ${r.note}` : ""}`);
    }
    for (const id of run.replacedStillPass) L.push(`- ⚠️ **${id}** is superseded but still passes: the behavior it guarded may not have changed.`);
    if (run.unproved.length) L.push(`- Unproved use cases (no proofs yet): ${run.unproved.join(", ")}`);
    L.push("");
  }
  if (p.decisions.length) {
    L.push("## Decisions", "");
    for (const d of p.decisions) L.push(`- **${d.id}** ${d.question} → ${d.chosen !== undefined ? `${d.options[d.chosen]!.label}${d.answeredBy ? ` (${d.answeredBy})` : ""}` : "unanswered"}`);
    L.push("");
  }
  if (p.notPromised.length) L.push("## Not promised", "", ...p.notPromised.map((n) => `- ${n}`), "");
  return L.join("\n");
}
