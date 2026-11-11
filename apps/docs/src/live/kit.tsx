/** Shared pieces of the interactive examples: the playground frame, controls, and clause rows. */
import { useState, type ReactNode } from "react";
import { CheckCircle2, Cpu, FlaskConical, Loader2, XCircle } from "lucide-react";
import { show, type Clause, type ClauseResult, type RustClauseProof } from "@lastro/sdk";
import { Badge } from "@/components/ui/badge";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";
import { ClauseVerdictBadge } from "./verdicts";
import type { Run } from "./runtime";

export function Playground({ id, title, hint, children }: { id: string; title: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <section aria-label={title} data-playground={id} className="my-8 overflow-hidden rounded-xl border bg-card shadow-sm">
      <header className="flex flex-wrap items-start justify-between gap-2 border-b bg-muted/40 px-4 py-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-semibold">
            <FlaskConical className="h-4 w-4 text-brass" aria-hidden />
            {title}
          </div>
          {hint && <p className="mt-1 text-xs leading-5 text-muted-foreground">{hint}</p>}
        </div>
        <Badge variant="outline" className="shrink-0 font-normal text-muted-foreground">
          Live: the real SDK, in your browser
        </Badge>
      </header>
      <div className="space-y-5 p-4">{children}</div>
    </section>
  );
}

export interface Option<T extends string> {
  value: T;
  label: ReactNode;
}

/** A labeled single choice (a row of toggles). */
export function Choice<T extends string>({ label, value, options, onChange, className }: { label: string; value: T; options: Option<T>[]; onChange: (v: T) => void; className?: string }) {
  return (
    <div className={cn("min-w-0", className)}>
      <div className="mb-1.5 text-xs font-medium text-muted-foreground">{label}</div>
      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        aria-label={label}
        value={value}
        onValueChange={(v) => v && onChange(v as T)}
        className="flex-wrap justify-start"
      >
        {options.map((o) => (
          <ToggleGroupItem key={o.value} value={o.value} className="h-8 px-2.5 font-mono text-xs data-[state=on]:border-foreground/40 data-[state=on]:bg-accent">
            {o.label}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  );
}

export function Toggle({ label, checked, onChange, id }: { label: ReactNode; checked: boolean; onChange: (v: boolean) => void; id: string }) {
  return (
    <label htmlFor={id} className="flex cursor-pointer items-center gap-2.5 text-sm">
      <Switch id={id} checked={checked} onCheckedChange={onChange} />
      <span>{label}</span>
    </label>
  );
}

export function Select<T extends string>({ label, value, options, onChange }: { label: string; value: T; options: { value: T; label: string }[]; onChange: (v: T) => void }) {
  return (
    <label className="block min-w-0">
      <span className="mb-1.5 block text-xs font-medium text-muted-foreground">{label}</span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value as T)}
        className="h-9 w-full rounded-md border bg-background px-2.5 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  );
}

export const formal = (c: Clause) => `${c.requires ? `${show(c.requires)}  ⇒  ` : ""}${show(c.ensures)}`;

export const json = (v: unknown) => JSON.stringify(v).replace(/,"/g, ', "').replace(/":/g, '": ');

/** One row per clause: what it says, its LSL, and each engine's verdict. */
export function ClauseRows({ clauses, results, rust, extra }: { clauses: Clause[]; results?: ClauseResult[]; rust?: RustClauseProof[] | null; extra?: (c: Clause) => ReactNode }) {
  return (
    <ul className="divide-y rounded-lg border" aria-label="Clauses">
      {clauses.map((c) => {
        const r = results?.find((x) => x.clauseId === c.id);
        const rr = rust?.find((x) => x.clauseId === c.id);
        return (
          <li key={c.id} data-clause={c.id} data-verdict={r?.verdict ?? ""} className="flex flex-col gap-2 p-3 sm:flex-row sm:items-start">
            <div className="min-w-0 flex-1">
              <div className="flex items-baseline gap-2 text-sm">
                <span className="font-mono text-xs font-semibold">{c.id}</span>
                <span className="font-mono text-[11px] text-muted-foreground">{c.intentIds.join(", ")}</span>
              </div>
              <p className="mt-0.5 text-sm leading-6">{c.description}</p>
              <p className="mt-1 break-words font-mono text-[11.5px] leading-5 text-muted-foreground">{formal(c)}</p>
              {r?.counterexample && (
                <p className="mt-1.5 break-words rounded bg-fail-soft px-2 py-1 font-mono text-[11.5px] leading-5 text-fail" data-testid="counterexample">
                  {r.method === "traces" ? "trace" : "counterexample"}: {json(r.counterexample.input)} → {json(r.counterexample.output)}
                </p>
              )}
              {r?.traceViolation && !r.counterexample && (
                <p className="mt-1.5 break-words rounded bg-fail-soft px-2 py-1 font-mono text-[11.5px] leading-5 text-fail" data-testid="counterexample">
                  {r.traceViolation.ref}: {json(r.traceViolation.input)} → {json(r.traceViolation.output)}
                </p>
              )}
              {extra?.(c)}
            </div>
            {results && (
              <div className="flex shrink-0 flex-wrap items-center gap-1.5 sm:flex-col sm:items-end">
                {r ? <ClauseVerdictBadge verdict={r.verdict} /> : null}
                {rr && (
                  <span className="font-mono text-[10.5px] text-muted-foreground" title="The Rust prover's verdict">
                    rust: {rr.verdict}
                  </span>
                )}
              </div>
            )}
          </li>
        );
      })}
    </ul>
  );
}

/** "Both engines agree", or why only one ran. */
export function Engines({ run, busy }: { run: Run | null; busy: boolean }) {
  if (!run)
    return (
      <p className="flex items-center gap-2 text-xs text-muted-foreground">
        <Loader2 className="h-3.5 w-3.5 animate-spin" /> Proving…
      </p>
    );
  return (
    <p data-testid="engines" className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
      <span className="inline-flex items-center gap-1.5">
        <Cpu className="h-3.5 w-3.5" />
        TypeScript engine
        {run.rust ? " + Rust prover (WASM)" : ""}
      </span>
      {run.agree === true && (
        <span className="inline-flex items-center gap-1 text-proved">
          <CheckCircle2 className="h-3.5 w-3.5" /> both engines agree on every clause
        </span>
      )}
      {run.agree === false && (
        <span className="inline-flex items-center gap-1 text-fail">
          <XCircle className="h-3.5 w-3.5" /> the engines disagree
        </span>
      )}
      {run.agree === null && <span>the Rust prover only runs on spaces of up to 100,000 inputs</span>}
      <span>{Math.max(1, Math.round(run.ms))} ms</span>
      {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" aria-label="Proving" />}
    </p>
  );
}

const TONE = { fail: "bg-fail-soft text-fail", evidence: "bg-evidence-soft text-evidence", proved: "bg-proved-soft text-proved" };

/** The headline: proved, or which clauses fail. */
export function Outcome({ run }: { run: Run | null }) {
  if (!run) return null;
  const failed = run.ts.clauses.filter((c) => c.verdict === "violated").map((c) => c.clauseId);
  const soft = run.ts.clauses.filter((c) => c.verdict === "vacuous" || c.verdict === "weak").map((c) => c.clauseId);
  const sampled = run.ts.clauses.some((c) => c.verdict === "evidenced");
  const [tone, text] = failed.length
    ? (["fail", `Not proved: ${failed.join(", ")} ${failed.length === 1 ? "fails" : "fail"}`] as const)
    : soft.length
      ? (["evidence", `No clause fails, but ${soft.join(", ")} ${soft.length === 1 ? "proves" : "prove"} nothing here`] as const)
      : sampled
        ? (["evidence", "Evidenced: every sampled input passes, but the space was not covered"] as const)
        : (["proved", "Proved: every clause holds on every input"] as const);
  return (
    <div data-testid="outcome" data-tone={tone} className={cn("rounded-lg px-3 py-2 text-sm font-medium", TONE[tone])}>
      {text}
    </div>
  );
}

/** A one-question check, answered in place. */
export function Quiz({ question, options, answer, explain }: { question: string; options: string[]; answer: number; explain: ReactNode }) {
  const [picked, setPicked] = useState<number | null>(null);
  return (
    <section aria-label="Check yourself" data-playground="quiz" className="my-8 rounded-xl border bg-card p-4 shadow-sm">
      <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Check yourself</div>
      <p className="mt-1 font-medium leading-6">{question}</p>
      <div className="mt-3 grid gap-2">
        {options.map((o, i) => (
          <button
            key={o}
            type="button"
            aria-pressed={picked === i}
            onClick={() => setPicked(i)}
            className={cn(
              "rounded-lg border px-3 py-2 text-left text-sm leading-6 transition-colors hover:bg-accent",
              picked !== null && i === answer && "border-proved/60 bg-proved-soft",
              picked === i && i !== answer && "border-fail/60 bg-fail-soft",
            )}
          >
            {o}
          </button>
        ))}
      </div>
      {picked !== null && (
        <p data-testid="quiz-result" className={cn("mt-3 text-sm leading-6", picked === answer ? "text-proved" : "text-fail")}>
          <strong>{picked === answer ? "Right." : "Not quite."}</strong> <span className="text-foreground">{explain}</span>
        </p>
      )}
    </section>
  );
}
