import { useEffect, useMemo, useRef, useState } from "react";
import {
  LlmIntentExtractor,
  LlmProofDeriver,
  carryReview,
  createLastro,
  raiseDecisions,
  review,
  type DecisionCard,
  type Gap,
  freeze,
  loadRustProver,
  pendingReview,
  tabulate,
  validatePlan,
  verify,
  type Env,
  type IntentArtifact,
  type ProofPlan,
  type RustProofReport,
  type RustProver,
  type Trace,
  type VerificationReport,
} from "@lastro/sdk";
import { AI_I5, FIXED_I5, LOGIN_SOURCE, MODEL, PROSE, compileLogin, deriveLoginPlan, extractLogin, plantBug, type LoginFn } from "@lastro/example-login";
import { detectAi, type Ai } from "./ai";
import wasmDataUrl from "@lastro/wasm/lastro_core.wasm?inline";

export { AI_I5, FIXED_I5, LOGIN_SOURCE, PROSE };

export const REVIEWERS = ["Ana", "Bruno"] as const;

/** The example as the team left it: Ana and Bruno approved everything, I5 corrected. */
export function reviewedArtifact(requiredApprovals = 1): IntentArtifact {
  const a = extractLogin();
  a.policy = { requiredApprovals };
  for (const it of a.intents) {
    it.status = "approved";
    it.approvals = [...REVIEWERS];
  }
  const i5 = a.intents.find((i) => i.id === "I5")!;
  i5.originalStatement = i5.statement;
  i5.statement = FIXED_I5;
  i5.status = "amended";
  i5.reviewNote = "The AI read 'invalid'; the prose says the email wins.";
  for (const d of a.decisions) d.chosen = d.recommended;
  return a;
}

export const PRODUCTION_TRACE: Trace = {
  ref: "prod req 7f3a",
  input: { sends: "email", accountFound: true, password: "wrong", infra: "ok" },
  output: { status: 200, token: true, role: "customer" },
};

let proverPromise: Promise<RustProver> | null = null;
function rustProver(): Promise<RustProver> {
  proverPromise ??= (async () => {
    const b64 = wasmDataUrl.slice(wasmDataUrl.indexOf(",") + 1);
    const bin = atob(b64);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    return loadRustProver(bytes);
  })();
  return proverPromise;
}

export interface Results {
  ts: VerificationReport | null;
  rust: RustProofReport | null;
  error: string | null;
  ms: number;
}

export type AiStep = "extract" | "derive" | "gap-review";
export interface AiState {
  /** Claude is available in this view (null while detecting). */
  available: boolean | null;
  kind: Ai["kind"] | null;
  busy: AiStep | null;
  error: { step: AiStep; message: string } | null;
}

export function useLastro() {
  const [artifact, setArtifact] = useState<IntentArtifact>(reviewedArtifact);
  const [source, setSource] = useState(LOGIN_SOURCE);
  const [bug, setBug] = useState<string | null>(null);
  const [withTrace, setWithTrace] = useState(false);
  const [debounced, setDebounced] = useState(source);
  const [results, setResults] = useState<Results>({ ts: null, rust: null, error: null, ms: 0 });
  const [rustStatus, setRustStatus] = useState<"loading" | "ready" | "failed">("loading");
  const aiRef = useRef<Ai | null>(null);
  const [ai, setAi] = useState<AiState>({ available: null, kind: null, busy: null, error: null });
  const [aiPlans, setAiPlans] = useState<Record<string, ProofPlan>>({});
  const [reviewer, setReviewer] = useState<string>(REVIEWERS[0]);
  const [requiredApprovals, setRequiredApprovals] = useState(1);
  const [gaps, setGaps] = useState<{ hash: string; source: string; gaps: Gap[] } | null>(null);
  const tried = useRef(new Set<string>());

  useEffect(() => {
    detectAi().then((found) => {
      aiRef.current = found;
      setAi((s) => ({ ...s, available: !!found, kind: found?.kind ?? null }));
    });
  }, []);

  useEffect(() => {
    const t = setTimeout(() => setDebounced(source), 250);
    return () => clearTimeout(t);
  }, [source]);

  useEffect(() => {
    rustProver().then(
      () => setRustStatus("ready"),
      () => setRustStatus("failed"),
    );
  }, []);

  const pending = useMemo(() => pendingReview(artifact), [artifact]);
  const frozen = useMemo(() => (pending.length ? null : freeze(artifact)), [artifact, pending]);
  /** The example's own prose uses the hand-written derivation; edited prose is derived by Claude. */
  const planOrigin: "reference" | "claude" = artifact.prose === PROSE ? "reference" : "claude";
  const plan: ProofPlan | null = useMemo(
    () => (frozen ? (frozen.prose === PROSE ? deriveLoginPlan(frozen) : (aiPlans[frozen.hash!] ?? null)) : null),
    [frozen, aiPlans],
  );

  const runAi = async <T,>(step: AiStep, work: (a: Ai) => Promise<T>): Promise<T | null> => {
    const a = aiRef.current;
    if (!a) {
      setAi((s) => ({ ...s, error: { step, message: "Claude is not available in this view." } }));
      return null;
    }
    setAi((s) => ({ ...s, busy: step, error: null }));
    try {
      const r = await work(a);
      setAi((s) => ({ ...s, busy: null }));
      return r;
    } catch (e) {
      setAi((s) => ({ ...s, busy: null, error: { step, message: (e as Error).message } }));
      return null;
    }
  };

  const deriveFor = async (f: IntentArtifact) => {
    const p = await runAi("derive", (a) => new LlmProofDeriver(a.ask).derive(f, MODEL));
    if (p) setAiPlans((m) => ({ ...m, [f.hash!]: p }));
  };

  // Reviewed intents for edited prose: derive the proof obligations once per frozen hash.
  useEffect(() => {
    if (!frozen || frozen.prose === PROSE || aiPlans[frozen.hash!] || ai.available !== true || ai.busy) return;
    if (tried.current.has(frozen.hash!)) return;
    tried.current.add(frozen.hash!);
    void deriveFor(frozen);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [frozen, aiPlans, ai.available, ai.busy]);

  const compiled = useMemo((): { fn: LoginFn | null; error: string | null } => {
    try {
      const fn = compileLogin(debounced);
      fn({ sends: "email", accountFound: true, password: "right", infra: "ok" });
      return { fn, error: null };
    } catch (e) {
      return { fn: null, error: (e as Error).message };
    }
  }, [debounced]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      if (!frozen || !plan || !compiled.fn) {
        setResults({ ts: null, rust: null, error: compiled.error, ms: 0 });
        return;
      }
      const t0 = performance.now();
      try {
        validatePlan(frozen, plan);
        const subject = (input: Env) => compiled.fn!(input);
        const ts = await verify(frozen, plan, { subject, traces: withTrace ? [PRODUCTION_TRACE] : [] });
        let rust: RustProofReport | null = null;
        if (rustStatus === "ready") rust = (await rustProver()).prove(plan, await tabulate(plan.model, subject));
        if (!cancelled) setResults({ ts, rust, error: null, ms: performance.now() - t0 });
      } catch (e) {
        if (!cancelled) setResults({ ts: null, rust: null, error: (e as Error).message, ms: 0 });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [frozen, plan, compiled, withTrace, rustStatus]);

  const pipeline = (a: Ai) => createLastro({ model: MODEL, ask: a.ask, requiredApprovals });
  const currentSpec = () => (frozen && plan ? { artifact: frozen, plan, contradictions: results.ts?.decisions ?? [], ok: !results.ts?.decisions.length } : null);

  return {
    artifact,
    setArtifact,
    planOrigin,
    ai,
    /** Re-extract intents from edited prose with Claude, keeping the review where nothing changed. */
    applyProse: async (prose: string) => {
      if (prose.trim() === PROSE) {
        setArtifact(reviewedArtifact(requiredApprovals));
        return true;
      }
      const next = await runAi("extract", (a) => new LlmIntentExtractor(a.ask).extract(prose.trim()));
      if (!next) return false;
      setArtifact((prev) => carryReview(prev, { ...next, policy: { requiredApprovals } }));
      return true;
    },
    retryDerive: () => {
      if (frozen) void deriveFor(frozen);
    },
    reviewer,
    setReviewer,
    requiredApprovals,
    /** The team's review policy: approvals each intent needs. */
    setRequiredApprovals: (n: number) => {
      setRequiredApprovals(n);
      setArtifact((prev) => ({ ...prev, policy: { requiredApprovals: n } }));
    },
    restoreExample: () => setArtifact(reviewedArtifact(requiredApprovals)),
    /** Review as the current reviewer (approve, amend, reject, answer). */
    reviewAs: (fn: (r: ReturnType<typeof review>) => void) =>
      setArtifact((prev) => {
        const r = review({ ...prev, policy: { requiredApprovals } }, reviewer);
        fn(r);
        return r.done();
      }),
    /** Contradictions in the spec go back to the team as decisions. */
    returnDecisions: (cards: DecisionCard[]) => setArtifact((prev) => raiseDecisions(prev, cards)),
    gaps: gaps && frozen && gaps.hash === frozen.hash && gaps.source === source ? gaps.gaps : null,
    /** LLM gap review: what the proofs cannot see. */
    runGapReview: async () => {
      const spec = currentSpec();
      if (!spec) return;
      const out = await runAi("gap-review", (a) => pipeline(a).gapReview(spec, source));
      if (out) setGaps({ hash: spec.artifact.hash!, source, gaps: out });
    },
    updateArtifact: (fn: (a: IntentArtifact) => void) =>
      setArtifact((prev) => {
        const next = structuredClone(prev);
        fn(next);
        return next;
      }),
    pending,
    frozen,
    plan,
    source,
    setSource: (s: string) => {
      setSource(s);
      setBug(null);
    },
    bug,
    plantBug: (id: string | null) => {
      const s = plantBug(LOGIN_SOURCE, id ?? undefined);
      setSource(s);
      setDebounced(s);
      setBug(id);
    },
    compiled,
    withTrace,
    setWithTrace,
    results,
    rustStatus,
    editing: source !== debounced,
  };
}

export type Lastro = ReturnType<typeof useLastro>;
