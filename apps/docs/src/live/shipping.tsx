/**
 * Interactive examples on the quickstart's shipping fee: change the code and watch the proof, grow the input space
 * until "proved" becomes "evidenced", and take a clause apart.
 */
import { useMemo, useState, type ReactNode } from "react";
import { agentFeedback, failuresOf, specGapsOf, type Env, type Model } from "@lastro/sdk";
import { Code } from "@/components/doc";
import { cn } from "@/lib/utils";
import { ClauseRows, Choice, Engines, Outcome, Playground, Toggle, formal } from "./kit";
import { SHIPPING_MODEL, prove, shippingSpec, useAsync } from "./runtime";

// ---------------------------------------------------------------------------
// Fix the shipping fee
// ---------------------------------------------------------------------------

interface Impl {
  cmp: ">=" | ">";
  threshold: number;
  membersFree: boolean;
  fee: number;
}

const CORRECT: Impl = { cmp: ">=", threshold: 100, membersFree: true, fee: 15 };
const BUGGY: Impl = { cmp: ">", threshold: 100, membersFree: true, fee: 15 };

/** The implementation is built from the controls (no eval): the same function the code preview shows. */
function subject({ cmp, threshold, membersFree, fee }: Impl) {
  return ({ subtotal, member }: Env): Env => {
    const free = (membersFree && member === true) || (cmp === ">=" ? (subtotal as number) >= threshold : (subtotal as number) > threshold);
    return { fee: free ? 0 : fee };
  };
}

const source = ({ cmp, threshold, membersFree, fee }: Impl) =>
  `export default function shippingFee({ subtotal, member }) {
  if (${membersFree ? "member || " : ""}subtotal ${cmp} ${threshold}) return { fee: 0 };
  return { fee: ${fee} };
}`;

export function ShippingPlayground() {
  const [impl, setImpl] = useState<Impl>(BUGGY);
  const spec = useMemo(() => shippingSpec(), []);
  const { value: run, busy } = useAsync(JSON.stringify(impl), () => prove(spec.artifact, spec.plan, subject(impl)));
  const set = <K extends keyof Impl>(k: K, v: Impl[K]) => setImpl((s) => ({ ...s, [k]: v }));
  const feedback = run ? agentFeedback({ failures: failuresOf(spec.plan, run.ts), specGaps: specGapsOf(run.ts) }) : "";

  return (
    <Playground
      id="shipping"
      title="Fix the shipping fee"
      hint="This is the quickstart's spec. The code starts with the off-by-one bug; change it until every clause is proved over all 402 inputs."
    >
      <div className="flex flex-wrap gap-1.5" role="group" aria-label="Presets">
        {[
          { label: "shipping-buggy.mjs", impl: BUGGY },
          { label: "shipping.mjs", impl: CORRECT },
        ].map((p) => (
          <button
            key={p.label}
            type="button"
            onClick={() => setImpl(p.impl)}
            className={cn("rounded-full border px-3 py-1 font-mono text-xs transition-colors hover:bg-accent", JSON.stringify(p.impl) === JSON.stringify(impl) && "border-foreground/40 bg-accent")}
          >
            {p.label}
          </button>
        ))}
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <Choice
          label="Comparison"
          value={impl.cmp}
          onChange={(v) => set("cmp", v)}
          options={[
            { value: ">=", label: "subtotal >=" },
            { value: ">", label: "subtotal >" },
          ]}
        />
        <label className="block">
          <span className="mb-1.5 block text-xs font-medium text-muted-foreground">
            Threshold: <span className="font-mono text-foreground">{impl.threshold}</span>
          </span>
          <input
            type="range"
            min={90}
            max={110}
            value={impl.threshold}
            aria-label="Threshold"
            onChange={(e) => set("threshold", Number(e.target.value))}
            className="w-full accent-[hsl(var(--brass))]"
          />
        </label>
        <Choice
          label="Fee for everyone else"
          value={String(impl.fee)}
          onChange={(v) => set("fee", Number(v))}
          options={[
            { value: "15", label: "15" },
            { value: "0", label: "0" },
          ]}
        />
      </div>
      <Toggle id="members-free" label="Members ship free" checked={impl.membersFree} onChange={(v) => set("membersFree", v)} />
      <Code lang="js" title="shipping.mjs (generated from the controls)" code={source(impl)} className="my-0" />
      <Outcome run={run} />
      <Engines run={run} busy={busy} />
      {run && <ClauseRows clauses={spec.plan.clauses} results={run.ts.clauses} rust={run.rust?.clauses} />}
      {run && <Code lang="text" title="lastro feedback" code={feedback} wrap className="my-0" />}
      {run && <Code lang="markdown" title=".lastro/receipt.md" code={run.receipt} className="my-0" />}
    </Playground>
  );
}

// ---------------------------------------------------------------------------
// Proved vs evidenced
// ---------------------------------------------------------------------------

const SIZES = [200, 20_000, 49_999, 50_000, 1_000_000];

export function SamplingPlayground() {
  const [max, setMax] = useState(200);
  const [buggy, setBuggy] = useState(false);
  const model: Model = useMemo(() => ({ ...SHIPPING_MODEL, inputs: { ...SHIPPING_MODEL.inputs, subtotal: { type: "int", min: 0, max } } }), [max]);
  const spec = useMemo(() => shippingSpec(model), [model]);
  const space = (max + 1) * 2;
  const { value: run, busy } = useAsync(JSON.stringify([max, buggy]), () => prove(spec.artifact, spec.plan, subject(buggy ? BUGGY : CORRECT), { rust: space <= 100_000 }));

  return (
    <Playground
      id="sampling"
      title="When proof stops"
      hint="Proof is exhaustive: every input of a finite domain. Grow the subtotal's domain past 100,000 inputs and Lastro can only sample, so the verdict drops to evidenced."
    >
      <Choice label="subtotal is an int from 0 to" value={String(max)} onChange={(v) => setMax(Number(v))} options={SIZES.map((s) => ({ value: String(s), label: s.toLocaleString("en-US") }))} />
      <Toggle id="sampling-buggy" label={<>Plant the off-by-one bug (<span className="font-mono text-xs">subtotal &gt; 100</span>)</>} checked={buggy} onChange={setBuggy} />
      <p className="font-mono text-xs text-muted-foreground" data-testid="space">
        input space: {space.toLocaleString("en-US")} inputs ({space <= 100_000 ? "enumerated" : "sampled: 2,000 inputs, seed 42"})
      </p>
      <Outcome run={run} />
      <Engines run={run} busy={busy} />
      {run && (
        <ClauseRows
          clauses={spec.plan.clauses}
          results={run.ts.clauses}
          rust={run.rust?.clauses}
          extra={(c) => {
            const r = run.ts.clauses.find((x) => x.clauseId === c.id);
            if (!r) return null;
            return (
              <p className="mt-1 font-mono text-[11px] text-muted-foreground">
                {r.method}, {r.checked.toLocaleString("en-US")} inputs checked
                {r.failureRateUpperBound95 !== undefined && <> · failure rate ≤ {(r.failureRateUpperBound95 * 100).toFixed(2)}% at 95% (rule of three: 3/{r.checked})</>}
              </p>
            );
          }}
        />
      )}
      {run && buggy && space > 100_000 && (
        <p className="text-sm leading-6" data-testid="missed-bug">
          The bug is still there: only <span className="font-mono">{"{ member: false, subtotal: 100 }"}</span> triggers it, and 2,000 samples out of{" "}
          {space.toLocaleString("en-US")} did not hit it. That is why Lastro never calls a sampled clause proved.
        </p>
      )}
    </Playground>
  );
}

// ---------------------------------------------------------------------------
// Anatomy of a clause
// ---------------------------------------------------------------------------

const PARTS: { key: string; label: string; explain: ReactNode }[] = [
  { key: "id", label: "id", explain: "A stable name. Failures, receipts and the agent's feedback point at it." },
  { key: "intentIds", label: "intentIds", explain: "The approved intents this clause proves. A plan may only cite approved intents, and every approved intent needs a clause or a reason it is beyond proof." },
  { key: "description", label: "description", explain: "The clause in plain words, for the reviewer and the receipt. Nothing checks it: the LSL below is what runs." },
  { key: "requires", label: "requires", explain: "The if-part: an LSL expression over in.* (and out.*). Inputs where it is false do not count. If no input makes it true, the clause is vacuous." },
  { key: "ensures", label: "ensures", explain: "What must hold on every input where requires holds. One input where it is false is a counterexample, and the clause is violated." },
  { key: "assumes", label: "assumes", explain: "Trusted pieces (T1, T2…) the clause leans on. A clause that assumes something is proved under assumptions, never plain proved." },
];

export function ClauseAnatomy() {
  const [part, setPart] = useState("requires");
  const spec = useMemo(() => shippingSpec(), []);
  const c = spec.plan.clauses[0]!;
  const value: Record<string, string> = {
    id: JSON.stringify(c.id),
    intentIds: JSON.stringify(c.intentIds),
    description: JSON.stringify(c.description),
    requires: JSON.stringify(c.requires),
    ensures: JSON.stringify(c.ensures),
    assumes: JSON.stringify(c.assumes ?? []),
  };
  const current = PARTS.find((p) => p.key === part)!;

  return (
    <Playground id="anatomy" title="Anatomy of a clause" hint="C1 of the quickstart's plan, as the deriver returned it. Click a field.">
      <div className="overflow-x-auto rounded-lg border bg-muted/40 p-3 font-mono text-[12.5px] leading-7">
        <div>{"{"}</div>
        {PARTS.map((p) => (
          <div key={p.key} className="pl-4">
            <button
              type="button"
              aria-pressed={part === p.key}
              onClick={() => setPart(p.key)}
              className={cn("rounded px-1 text-left transition-colors hover:bg-accent", part === p.key && "bg-brass/15 ring-1 ring-brass/50")}
            >
              <span className="tok-keyword">"{p.label}"</span>: <span className="break-all">{value[p.key]}</span>
            </button>
          </div>
        ))}
        <div>{"}"}</div>
      </div>
      <div className="rounded-lg border-l-2 border-brass bg-muted/30 px-3 py-2 text-sm leading-6" data-testid="anatomy-explain">
        <span className="font-mono text-xs font-semibold">{current.label}</span>: {current.explain}
      </div>
      <p className="text-sm text-muted-foreground">
        Read as math: <span className="font-mono text-foreground">{formal(c)}</span>
      </p>
    </Playground>
  );
}
