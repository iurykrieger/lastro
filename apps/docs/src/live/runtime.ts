/**
 * The engine behind the interactive examples: the real TypeScript SDK and the Rust prover (WASM), running in the
 * reader's browser. Nothing here is a mock: every verdict on the page is computed by the same code the CLI runs.
 */
import { useEffect, useState } from "react";
import {
  artifactFromExtraction,
  freeze,
  loadRustProver,
  planFromDerivation,
  renderReceipt,
  review,
  tabulate,
  verify,
  type DeriveOut,
  type ExtractOut,
  type IntentArtifact,
  type Model,
  type ProofPlan,
  type RustProofReport,
  type RustProver,
  type Subject,
  type Trace,
  type VerificationReport,
} from "@lastro/sdk";
import { AI_I5, FIXED_I5, deriveLoginPlan, extractLogin } from "@lastro/example-login";
import wasmUrl from "@lastro/wasm/lastro_core.wasm?url";
import shippingProse from "../../examples/shipping/prose.md?raw";
import shippingModel from "../../examples/shipping/model.json";
import extractAnswer from "../../examples/shipping/answers/extract.json";
import deriveAnswer from "../../examples/shipping/answers/derive.json";

// ---------------------------------------------------------------------------
// The Rust prover, loaded once per page.
// ---------------------------------------------------------------------------

let prover: Promise<RustProver> | null = null;
export function rustProver(): Promise<RustProver> {
  prover ??= fetch(wasmUrl)
    .then((r) => {
      if (!r.ok) throw new Error(`could not load the Rust prover (${r.status})`);
      return r.arrayBuffer();
    })
    .then((b) => loadRustProver(b));
  return prover;
}

export interface Run {
  ts: VerificationReport;
  rust: RustProofReport | null;
  /** Both engines gave every clause the same verdict (null without the Rust prover). */
  agree: boolean | null;
  receipt: string;
  ms: number;
}

/** Verify in the TypeScript engine, then (when the space is small enough to tabulate) in the Rust prover. */
export async function prove(artifact: IntentArtifact, plan: ProofPlan, subject: Subject, opts: { traces?: Trace[]; exhaustiveLimit?: number; rust?: boolean } = {}): Promise<Run> {
  const t0 = performance.now();
  const ts = await verify(artifact, plan, { subject, traces: opts.traces ?? [], ...(opts.exhaustiveLimit ? { exhaustiveLimit: opts.exhaustiveLimit } : {}) });
  let rust: RustProofReport | null = null;
  if (opts.rust !== false) {
    try {
      rust = (await rustProver()).prove(plan, await tabulate(plan.model, subject));
    } catch {
      rust = null; // no WASM (or an infinite space): the TS verdicts still stand
    }
  }
  const agree = rust ? ts.clauses.every((c) => rust!.clauses.find((r) => r.clauseId === c.clauseId)?.verdict === c.verdict) : null;
  return { ts, rust, agree, receipt: renderReceipt(ts, plan, rust ?? undefined), ms: performance.now() - t0 };
}

/** Run an async computation whenever its key changes; keeps the last result while recomputing. */
export function useAsync<T>(key: string, run: () => Promise<T>): { value: T | null; error: string | null; busy: boolean } {
  const [state, setState] = useState<{ value: T | null; error: string | null; busy: boolean }>({ value: null, error: null, busy: true });
  useEffect(() => {
    let live = true;
    setState((s) => ({ ...s, busy: true }));
    run().then(
      (value) => live && setState({ value, error: null, busy: false }),
      (e: unknown) => live && setState({ value: null, error: (e as Error).message, busy: false }),
    );
    return () => {
      live = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  return state;
}

// ---------------------------------------------------------------------------
// The login example (sdks/typescript/examples/login), reviewed in different ways.
// ---------------------------------------------------------------------------

export interface LoginReview {
  /** Approve the AI's reading of I5 ("invalid") or the reviewer's amendment ("the email wins"). */
  i5: "ai" | "fixed";
  /** D1: must the right password ALWAYS get a token (0), or only must a wrong one be refused (1)? */
  d1: number;
  /** D2: a hash bcrypt cannot read answers 500 (0) or 401 (1). */
  d2: number;
  /** Intents the reviewer rejects. */
  rejected: string[];
}

export const DEFAULT_REVIEW: LoginReview = { i5: "fixed", d1: 0, d2: 0, rejected: [] };

export function loginArtifact(r: LoginReview = DEFAULT_REVIEW): IntentArtifact {
  let rv = review(extractLogin(), "ana");
  for (const id of r.rejected) rv = rv.reject(id, "out of scope");
  rv = rv.approveAll().answer("D1", r.d1).answer("D2", r.d2);
  if (r.i5 === "fixed" && !r.rejected.includes("I5")) rv = rv.amend("I5", FIXED_I5, {}, "the AI read 'invalid'; the prose says the email wins");
  return freeze(rv.done());
}

export function loginSpec(r: LoginReview = DEFAULT_REVIEW): { artifact: IntentArtifact; plan: ProofPlan } {
  const artifact = loginArtifact(r);
  return { artifact, plan: deriveLoginPlan(artifact) };
}

export { AI_I5, FIXED_I5 };

// ---------------------------------------------------------------------------
// The shipping example (apps/docs/examples/shipping): the quickstart's answers, replayed.
// ---------------------------------------------------------------------------

export const SHIPPING_PROSE = shippingProse.trim();

export function shippingSpec(model: Model = shippingModel as Model): { artifact: IntentArtifact; plan: ProofPlan } {
  const extracted = artifactFromExtraction(SHIPPING_PROSE, extractAnswer as ExtractOut);
  const artifact = freeze(review(extracted, "ana").answer("D1", 1).approveAll().done());
  return { artifact, plan: planFromDerivation(artifact, model, deriveAnswer as DeriveOut) };
}

export const SHIPPING_MODEL = shippingModel as Model;

/** A recorded production request that contradicts the proof: the trusted bcrypt piece said "wrong", yet a token left. */
export const PRODUCTION_TRACE: Trace = {
  ref: "prod req 7f3a",
  input: { sends: "email", accountFound: true, password: "wrong", infra: "ok" },
  output: { status: 200, token: true, role: "customer" },
};
