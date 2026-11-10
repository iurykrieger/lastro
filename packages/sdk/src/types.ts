/**
 * Core data model for Lastro.
 *
 * prose -> IntentArtifact (reviewed, frozen) -> ProofPlan (LSL) -> VerificationReport -> Receipt
 */
import type { Expr } from "./spec/expr.js";
import type { Domain } from "./spec/domain.js";

// ---------------------------------------------------------------------------
// Intents
// ---------------------------------------------------------------------------

/** The five limits of formal proof, after PDD Kit's "where proof stops". */
export type BeyondProofReason =
  | "external-code" // code the verifier cannot see (bcrypt, sqlx, jwt libs...)
  | "world-facts" // database contents, the clock, the network
  | "termination-timing" // whether / how fast something finishes
  | "human-judgment" // "the error message should be helpful"
  | "unstated"; // requirements nobody wrote down

export const BEYOND_PROOF_REASONS: readonly BeyondProofReason[] = [
  "external-code",
  "world-facts",
  "termination-timing",
  "human-judgment",
  "unstated",
];

/** How the extractor proposes the intent can be checked. */
export type Provability = "formal" | "empirical" | "beyond-proof";

export type IntentKind =
  | "behavior" // what must happen
  | "invariant" // what must never happen
  | "preserve" // existing behavior that must keep working
  | "non-functional"; // performance, UX, etc.

export type IntentStatus = "proposed" | "approved" | "rejected" | "amended";

/** Where in the prose an intent came from (the "intent map"). */
export interface SourceSpan {
  quote: string;
  start: number;
  end: number;
}

export interface Intent {
  id: string; // "I1"
  statement: string; // normalized, plain language
  kind: IntentKind;
  provability: Provability;
  beyondProofReason?: BeyondProofReason;
  source: SourceSpan;
  status: IntentStatus;
  /** Reviewer note, required when amended or rejected. */
  reviewNote?: string;
  /** The original statement, kept when a reviewer amends it. */
  originalStatement?: string;
  /** Reviewers who approved the current statement. Absent on single-reviewer artifacts. */
  approvals?: string[];
  /** Set when the intent records the answer to a decision (for example a contradiction in the spec). */
  decisionId?: string;
}

export interface DecisionOption {
  label: string;
  consequence: string;
}

/** One question, options with consequences, one recommended (PDD Kit decision card). */
export interface DecisionCard {
  id: string; // "D1"
  intentIds: string[];
  question: string;
  options: DecisionOption[];
  recommended: number;
  chosen?: number;
  origin: "extraction" | "consistency-check" | "verification";
}

export interface IntentArtifact {
  lastro: "intent-artifact/v0";
  prose: string;
  intents: Intent[];
  decisions: DecisionCard[];
  /** Explicitly not promised (timing, rate limits...). */
  notPromised: string[];
  /** Review rules. Without one, a single approval is enough. */
  policy?: ReviewPolicy;
  /** Set by freeze(). */
  hash?: string;
}

export interface ReviewPolicy {
  /** Distinct reviewers who must approve each intent before it reaches the spec. */
  requiredApprovals: number;
}

// ---------------------------------------------------------------------------
// Proof plan (derived from approved intents)
// ---------------------------------------------------------------------------

/**
 * The vocabulary the proofs talk about. Inputs include facts provided by
 * trusted pieces (like uninterpreted functions in Verus); outputs are what the
 * program decides.
 */
export interface Model {
  name: string;
  inputs: Record<string, Domain>;
  outputs: Record<string, Domain>;
  /** Shared named definitions (e.g. `named_account`), referenced via {call}. */
  defs?: Record<string, { params: string[]; body: Expr; description?: string }>;
}

/** A formal obligation: for all inputs, requires(input) => ensures(input, output). */
export interface Clause {
  id: string; // "C1"
  intentIds: string[];
  description: string; // plain-English reading of the clause
  requires?: Expr;
  ensures: Expr;
  /** Trusted assumptions this clause relies on. */
  assumes?: string[];
}

/** A trusted piece: the boundary of the proof (PDD Kit trust card). */
export interface Assumption {
  id: string; // "T1"
  intentIds: string[];
  piece: string; // e.g. "checkPassword (src/auth.ts)"
  contract: string; // what is assumed about it
  youAccept: string;
  ifWrong: string;
  checkedBy: string[]; // scenario ids
}

/** An empirical obligation: Given/When/Then, checked by running the program. */
export interface Scenario {
  id: string; // "S1"
  intentIds: string[];
  given: string;
  when: string;
  then: string;
  /** Concrete input for the subject. */
  input: Record<string, unknown>;
  /** Expected property over {input, output}. */
  expect: Expr;
}

export interface BeyondProofItem {
  intentId: string;
  reason: BeyondProofReason;
  note: string;
}

export interface ProofPlan {
  lastro: "proof-plan/v0";
  artifactHash: string;
  model: Model;
  clauses: Clause[];
  assumptions: Assumption[];
  scenarios: Scenario[];
  beyondProof: BeyondProofItem[];
}

// ---------------------------------------------------------------------------
// Verification
// ---------------------------------------------------------------------------

export type Value = boolean | number | string;
export type Env = Record<string, Value>;

/** The final output of the program: a decision function over the model inputs. */
export type Subject = (input: Env) => Env | Promise<Env>;

/** A recorded real execution. */
export interface Trace {
  input: Env;
  output: Env;
  ref?: string;
}

export type Evidence = "formal" | "empirical" | "trusted" | "none";

export type ClauseVerdict = "proved" | "evidenced" | "violated" | "vacuous" | "weak" | "not-run";

export interface Counterexample {
  input: Env;
  output: Env;
}

export interface ClauseResult {
  clauseId: string;
  verdict: ClauseVerdict;
  evidence: Evidence;
  method: "exhaustive" | "sampling" | "traces" | "none";
  checked: number;
  /** For sampling: upper bound of failure rate at 95% (rule of three). */
  failureRateUpperBound95?: number;
  counterexample?: Counterexample;
  mutants?: { total: number; killed: number };
  tracesChecked?: number;
  traceViolation?: Trace;
  note?: string;
}

export interface ScenarioResult {
  scenarioId: string;
  status: "passed" | "failed";
  output?: Env;
  error?: string;
}

export type IntentVerdict =
  | "proved"
  | "proved-under-assumptions"
  | "evidenced"
  | "violated"
  | "beyond-proof"
  | "gap";

export interface IntentResult {
  intentId: string;
  statement: string;
  verdict: IntentVerdict;
  evidence: Evidence[];
  clauses: string[];
  assumptions: string[];
  scenarios: string[];
  /** 1 for proved; 1 - upper bound for evidenced; 0 for violated/gap. */
  confidence: number;
  notes: string[];
}

export interface VerificationReport {
  lastro: "verification-report/v0";
  artifactHash: string;
  planHash: string;
  model: string;
  clauses: ClauseResult[];
  scenarios: ScenarioResult[];
  intents: IntentResult[];
  assumptions: Assumption[];
  beyondProof: BeyondProofItem[];
  notPromised: string[];
  /** Contradictions in the spec, returned to the team as decisions. */
  decisions: DecisionCard[];
  summary: Record<IntentVerdict, number>;
  ok: boolean;
}
