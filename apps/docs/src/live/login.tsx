/**
 * Interactive examples on the store login (sdks/typescript/examples/login): run one request, plant a bug, change
 * the review, and replay a production trace. Every verdict is computed live by the SDK and the Rust prover.
 */
import { useMemo, useState } from "react";
import { ArrowRight, CircleDot, CircleSlash, CircleCheck } from "lucide-react";
import { agentFeedback, explainRun, failuresOf, specGapsOf, type Env } from "@lastro/sdk";
import { BUGS, STEPS, makeLogin, type Step } from "@lastro/example-login";
import { Badge } from "@/components/ui/badge";
import { Code } from "@/components/doc";
import { cn } from "@/lib/utils";
import { ClauseRows, Choice, Engines, Outcome, Playground, Select, Toggle, json } from "./kit";
import { IntentVerdictBadge } from "./verdicts";
import { AI_I5, DEFAULT_REVIEW, FIXED_I5, PRODUCTION_TRACE, loginSpec, prove, useAsync, type LoginReview } from "./runtime";

const IMPLEMENTATIONS = [{ value: "none", label: "The reference implementation" }, ...BUGS.map((b) => ({ value: b.id, label: `Bug: ${b.label}` }))];

// ---------------------------------------------------------------------------
// Run one login
// ---------------------------------------------------------------------------

type LoginInput = { sends: string; accountFound: string; password: string; infra: string };

const PRESETS: { label: string; input: LoginInput }[] = [
  { label: "Customer signs in", input: { sends: "email", accountFound: "true", password: "right", infra: "ok" } },
  { label: "Admin signs in", input: { sends: "username", accountFound: "true", password: "right", infra: "ok" } },
  { label: "Wrong password", input: { sends: "email", accountFound: "true", password: "wrong", infra: "ok" } },
  { label: "Both fields sent", input: { sends: "both", accountFound: "true", password: "right", infra: "ok" } },
  { label: "Database down", input: { sends: "email", accountFound: "true", password: "right", infra: "db-fails" } },
];

const STATUS = {
  holds: { label: "holds", className: "bg-proved-soft text-proved" },
  fails: { label: "fails", className: "bg-fail-soft text-fail" },
  "if-part-false": { label: "does not apply", className: "bg-muted text-muted-foreground" },
  error: { label: "error", className: "bg-fail-soft text-fail" },
} as const;

export function LoginRunner() {
  const [input, setInput] = useState<LoginInput>(PRESETS[0]!.input);
  const [bug, setBug] = useState("none");
  const { plan } = useMemo(() => loginSpec(), []);
  const env: Env = { ...input, accountFound: input.accountFound === "true" };
  const steps: Step[] = [];
  const output = makeLogin(bug === "none" ? undefined : bug)(env, steps);
  const checks = explainRun(plan, env, output);
  const set = (k: keyof LoginInput) => (v: string) => setInput((s) => ({ ...s, [k]: v }));
  const applies = checks.filter((c) => c.status !== "if-part-false").length;

  return (
    <Playground
      id="login-runner"
      title="Run one login"
      hint="Pick a request. The implementation runs, then every clause of the spec is checked against this one input and output."
    >
      <div className="flex flex-wrap gap-1.5" aria-label="Presets" role="group">
        {PRESETS.map((p) => (
          <button
            key={p.label}
            type="button"
            onClick={() => setInput(p.input)}
            className={cn(
              "rounded-full border px-3 py-1 text-xs transition-colors hover:bg-accent",
              JSON.stringify(p.input) === JSON.stringify(input) && "border-foreground/40 bg-accent font-medium",
            )}
          >
            {p.label}
          </button>
        ))}
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Choice label="sends" value={input.sends} onChange={set("sends")} options={["email", "username", "both", "neither"].map((v) => ({ value: v, label: v }))} />
        <Choice label="accountFound" value={input.accountFound} onChange={set("accountFound")} options={["true", "false"].map((v) => ({ value: v, label: v }))} />
        <Choice label="password" value={input.password} onChange={set("password")} options={["right", "wrong", "bad-hash"].map((v) => ({ value: v, label: v }))} />
        <Choice label="infra" value={input.infra} onChange={set("infra")} options={["ok", "db-fails", "signer-fails"].map((v) => ({ value: v, label: v }))} />
      </div>
      <Select label="Implementation" value={bug} onChange={setBug} options={IMPLEMENTATIONS} />

      <div className="grid gap-4 md:grid-cols-[1fr_1.4fr]">
        <div>
          <div className="mb-1.5 text-xs font-medium text-muted-foreground">Request flow</div>
          <ol className="space-y-1.5" aria-label="Request flow">
            {STEPS.map((s) => {
              const hit = steps.find((x) => x.name === s.name);
              const Icon = !hit ? CircleDot : hit.status === "stop" ? CircleSlash : CircleCheck;
              return (
                <li key={s.name} data-step={hit ? hit.status : "skipped"} className={cn("flex items-start gap-2 text-sm", !hit && "text-muted-foreground/60")}>
                  <Icon className={cn("mt-0.5 h-4 w-4 shrink-0", hit?.status === "ok" && "text-proved", hit?.status === "stop" && "text-evidence")} aria-hidden />
                  <span>
                    {s.name} <span className="font-mono text-[11px] text-muted-foreground">{s.fn}</span>
                  </span>
                </li>
              );
            })}
          </ol>
          <div className="mt-3 flex flex-wrap items-center gap-2 rounded-lg border bg-muted/40 px-3 py-2 font-mono text-xs" data-testid="login-output">
            <ArrowRight className="h-3.5 w-3.5 text-muted-foreground" aria-hidden />
            status {String(output.status)} · token {String(output.token)} · role {String(output.role)}
          </div>
        </div>
        <div>
          <div className="mb-1.5 text-xs font-medium text-muted-foreground">
            The spec on this run: {applies} of {checks.length} clauses apply
          </div>
          <ul className="divide-y rounded-lg border" aria-label="Clause checks">
            {plan.clauses.map((c) => {
              const s = STATUS[checks.find((x) => x.clauseId === c.id)!.status];
              return (
                <li key={c.id} data-clause={c.id} data-status={s.label} className="flex items-start gap-2 px-3 py-2 text-sm">
                  <span className="mt-0.5 font-mono text-xs font-semibold">{c.id}</span>
                  <span className="min-w-0 flex-1 leading-5">{c.description}</span>
                  <span className={cn("shrink-0 rounded px-1.5 py-0.5 text-[11px] font-medium", s.className)}>{s.label}</span>
                </li>
              );
            })}
          </ul>
        </div>
      </div>
    </Playground>
  );
}

// ---------------------------------------------------------------------------
// Plant a bug
// ---------------------------------------------------------------------------

export function BugHunt() {
  const [bug, setBug] = useState(BUGS[0]!.id);
  const spec = useMemo(() => loginSpec(), []);
  const { value: run, busy } = useAsync(bug, () => prove(spec.artifact, spec.plan, makeLogin(bug === "none" ? undefined : bug)));
  const planted = BUGS.find((b) => b.id === bug);
  const feedback = run ? agentFeedback({ failures: failuresOf(spec.plan, run.ts), specGaps: specGapsOf(run.ts) }) : "";

  return (
    <Playground id="bug-hunt" title="Plant a bug" hint="Each bug is a small edit to the reference login. Both engines check every one of the 72 possible requests.">
      <Select label="Implementation" value={bug} onChange={setBug} options={IMPLEMENTATIONS} />
      {planted && (
        <div className="space-y-2">
          <p className="text-sm leading-6">
            <Badge variant="outline" className="mr-2 font-mono font-normal">
              {planted.where}
            </Badge>
            {planted.explain}
          </p>
          <Code
            lang="text"
            title="the edit"
            code={planted.patch.map(([from, to]) => `- ${from.trim().replace(/\n\s*/g, " ")}\n+ ${to.trim().replace(/\n\s*/g, " ")}`).join("\n")}
            className="my-0"
          />
        </div>
      )}
      <Outcome run={run} />
      <Engines run={run} busy={busy} />
      {run && <ClauseRows clauses={spec.plan.clauses} results={run.ts.clauses} rust={run.rust?.clauses} />}
      {run && <Code lang="text" title="agentFeedback(proof): what the coding agent gets back" code={feedback} wrap className="my-0" />}
    </Playground>
  );
}

// ---------------------------------------------------------------------------
// The review changes the proof
// ---------------------------------------------------------------------------

export function ReviewPlayground() {
  const [r, setR] = useState<LoginReview>(DEFAULT_REVIEW);
  const [bug, setBug] = useState("none");
  const spec = useMemo(() => loginSpec(r), [r]);
  const key = JSON.stringify([r, bug]);
  const { value: run, busy } = useAsync(key, () => prove(spec.artifact, spec.plan, makeLogin(bug === "none" ? undefined : bug)));
  const set = <K extends keyof LoginReview>(k: K, v: LoginReview[K]) => setR((s) => ({ ...s, [k]: v }));
  const toggleReject = (id: string, on: boolean) => set("rejected", on ? [...r.rejected, id].sort() : r.rejected.filter((x) => x !== id));

  return (
    <Playground
      id="review"
      title="Review it yourself"
      hint="The AI extracted nine intents and raised two decisions. What you approve, amend and answer decides which clauses exist, so it decides what code passes."
    >
      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-4">
          <div>
            <Choice
              label="I5"
              value={r.i5}
              onChange={(v) => set("i5", v)}
              options={[
                { value: "ai", label: "approve the AI's reading" },
                { value: "fixed", label: "amend: the email wins" },
              ]}
            />
            <p className="mt-2 text-xs leading-5 text-muted-foreground" data-testid="i5-statement">
              “{r.i5 === "ai" ? AI_I5 : FIXED_I5}”
            </p>
          </div>
          <Choice
            label="D1: must the right password always get a token?"
            value={String(r.d1)}
            onChange={(v) => set("d1", Number(v))}
            options={[
              { value: "0", label: "both ways" },
              { value: "1", label: "only refuse the wrong one" },
            ]}
          />
          <Choice
            label="D2: an unreadable hash answers"
            value={String(r.d2)}
            onChange={(v) => set("d2", Number(v))}
            options={[
              { value: "0", label: "500, as today" },
              { value: "1", label: "401" },
            ]}
          />
          <div className="space-y-2">
            <div className="text-xs font-medium text-muted-foreground">Reject an intent</div>
            <Toggle id="reject-i2" label="Reject I2 (the right password always gets a token)" checked={r.rejected.includes("I2")} onChange={(on) => toggleReject("I2", on)} />
            <Toggle id="reject-i7" label="Reject I7 (a database failure gets 500)" checked={r.rejected.includes("I7")} onChange={(on) => toggleReject("I7", on)} />
          </div>
        </div>
        <div className="space-y-3">
          <Select label="Code under proof" value={bug} onChange={setBug} options={IMPLEMENTATIONS} />
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 rounded-lg border bg-muted/40 px-3 py-2 font-mono text-xs">
            <dt className="text-muted-foreground">artifact</dt>
            <dd className="truncate" data-testid="artifact-hash">
              {spec.artifact.hash!.slice(0, 19)}…
            </dd>
            <dt className="text-muted-foreground">clauses</dt>
            <dd data-testid="clause-count">{spec.plan.clauses.length}</dd>
          </dl>
          <Outcome run={run} />
          <Engines run={run} busy={busy} />
        </div>
      </div>
      {run && <ClauseRows clauses={spec.plan.clauses} results={run.ts.clauses} rust={run.rust?.clauses} />}
    </Playground>
  );
}

// ---------------------------------------------------------------------------
// Trusted pieces and production traces
// ---------------------------------------------------------------------------

export function TrustPlayground() {
  const [trace, setTrace] = useState(false);
  const spec = useMemo(() => loginSpec(), []);
  const { value: run, busy } = useAsync(String(trace), () => prove(spec.artifact, spec.plan, makeLogin(), { traces: trace ? [PRODUCTION_TRACE] : [] }));

  return (
    <Playground
      id="trust"
      title="Proved, under what you trust"
      hint="The login calls three pieces it cannot see into. The proof holds if they keep their contracts; a production trace can show that one did not."
    >
      <div className="grid gap-3 md:grid-cols-3">
        {spec.plan.assumptions.map((a) => (
          <div key={a.id} data-assumption={a.id} className="rounded-lg border p-3 text-sm">
            <div className="flex items-center gap-2">
              <Badge variant="trusted" className="font-mono">
                {a.id}
              </Badge>
              <span className="font-mono text-xs">{a.piece}</span>
            </div>
            <p className="mt-2 leading-5">
              <span className="text-muted-foreground">You accept: </span>
              {a.youAccept}.
            </p>
            <p className="mt-1.5 leading-5">
              <span className="text-muted-foreground">If it is wrong: </span>
              {a.ifWrong}.
            </p>
          </div>
        ))}
      </div>
      <Toggle
        id="replay-trace"
        checked={trace}
        onChange={setTrace}
        label={
          <>
            Replay the production trace <span className="font-mono text-xs">{PRODUCTION_TRACE.ref}</span>:{" "}
            <span className="font-mono text-xs">{json(PRODUCTION_TRACE.input)} → {json(PRODUCTION_TRACE.output)}</span>
          </>
        }
      />
      <Outcome run={run} />
      <Engines run={run} busy={busy} />
      {run && (
        <ul className="divide-y rounded-lg border" aria-label="Intents">
          {run.ts.intents.map((i) => (
            <li key={i.intentId} data-intent={i.intentId} data-verdict={i.verdict} className="flex flex-col gap-1.5 p-3 text-sm sm:flex-row sm:items-start">
              <span className="w-8 shrink-0 font-mono text-xs font-semibold">{i.intentId}</span>
              <div className="min-w-0 flex-1 leading-6">
                {i.statement}
                {i.assumptions.length > 0 && <span className="ml-2 font-mono text-[11px] text-muted-foreground">trusts {i.assumptions.join(", ")}</span>}
              </div>
              <IntentVerdictBadge verdict={i.verdict} />
            </li>
          ))}
        </ul>
      )}
      {run?.ts.clauses.some((c) => c.traceViolation) && (
        <p className="text-sm leading-6" data-testid="trace-note">
          The code still passes every clause on the model, but the real system broke{" "}
          {run.ts.clauses
            .filter((c) => c.traceViolation)
            .map((c) => c.clauseId)
            .join(" and ")}
          : the trusted bcrypt check (T2) said “wrong” and a token left anyway. A trace can violate a proved clause, because the proof was only ever as good
          as the pieces it trusts.
        </p>
      )}
    </Playground>
  );
}
