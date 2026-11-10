import { useEffect, useMemo, useState } from "react";
import {
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
import { AI_I5, FIXED_I5, LOGIN_SOURCE, compileLogin, deriveLoginPlan, extractLogin, plantBug, type LoginFn } from "@lastro/example-login";
import wasmDataUrl from "@lastro/wasm/lastro_core.wasm?inline";

export { AI_I5, FIXED_I5, LOGIN_SOURCE };

/** The example as a reviewer would leave it: everything approved, I5 corrected. */
export function reviewedArtifact(): IntentArtifact {
  const a = extractLogin();
  for (const it of a.intents) it.status = "approved";
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

export function useLastro() {
  const [artifact, setArtifact] = useState<IntentArtifact>(reviewedArtifact);
  const [source, setSource] = useState(LOGIN_SOURCE);
  const [bug, setBug] = useState<string | null>(null);
  const [withTrace, setWithTrace] = useState(false);
  const [debounced, setDebounced] = useState(source);
  const [results, setResults] = useState<Results>({ ts: null, rust: null, error: null, ms: 0 });
  const [rustStatus, setRustStatus] = useState<"loading" | "ready" | "failed">("loading");

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
  const plan: ProofPlan | null = useMemo(() => (frozen ? deriveLoginPlan(frozen) : null), [frozen]);

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

  return {
    artifact,
    setArtifact,
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
      setBug(id);
      const s = plantBug(LOGIN_SOURCE, id ?? undefined);
      setSource(s);
      setDebounced(s);
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
