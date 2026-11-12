import { useEffect, useMemo, useState } from "react";
import { ArrowDown, ArrowUp, GitCommitHorizontal, Pause, Play, SkipBack, SkipForward } from "lucide-react";
import { show, type ProofEntry, type ProofOutcome, type Proposal } from "@lastro/sdk";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Empty } from "@/components/bits";
import type { ProofRunRecord } from "@/lib/api-types";
import { shortId } from "@/lib/review";
import { cn } from "@/lib/utils";

type State = "pass" | "fail" | "none";

interface Row {
  key: string;
  label: string;
  sub?: string;
  proofs: string[];
}

interface Group {
  key: string;
  title: string;
  hint?: string;
  rows: Row[];
  /** The group's own bar: an intent passes in a run when every one of its proofs passed. */
  bar: boolean;
}

const time = (iso: string) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
};
const dateTime = (iso: string) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString([], { dateStyle: "medium", timeStyle: "medium" });
};
const duration = (a: string, b: string) => {
  const ms = new Date(b).getTime() - new Date(a).getTime();
  if (!Number.isFinite(ms) || ms < 0) return "";
  const m = Math.round(ms / 60000);
  return m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${m % 60} min`;
};

const stateOf = (o: ProofOutcome | undefined): State => (!o ? "none" : o.passed ? "pass" : "fail");

function cellClass(o: ProofOutcome | undefined) {
  if (!o) return "bg-muted/40";
  if (!o.passed) return "bg-fail text-white";
  if (o.verdict === "proved") return "bg-proved text-white";
  if (o.verdict === "not-run") return "bg-beyond text-white";
  return "bg-evidence text-white";
}

function verdictVariant(o: ProofOutcome) {
  return !o.passed ? "fail" : o.verdict === "proved" ? "proved" : o.verdict === "not-run" ? "beyond" : "evidence";
}

/** The rows of the time-lapse: one group per intent (its proofs), then the use cases' regression proofs, then superseded ones. */
function groupsOf(p: Proposal, runs: ProofRunRecord[]): Group[] {
  const seen = new Map<string, ProofOutcome>();
  for (const r of runs) for (const o of r.outcomes) if (!seen.has(o.proof)) seen.set(o.proof, o);
  const all = [...seen.values()];
  const groups: Group[] = [];
  const intents = [...new Set([...p.intents.map((i) => i.id), ...all.flatMap((o) => (o.role === "proposal" && o.intent ? [o.intent] : []))])];
  for (const id of intents) {
    const proofs = all.filter((o) => o.role === "proposal" && o.intent === id).map((o) => o.proof);
    if (!proofs.length) continue;
    const it = p.intents.find((i) => i.id === id);
    groups.push({
      key: id,
      title: it ? `${shortId(id)} ${it.name}` : id,
      rows: proofs.map((pf) => ({ key: pf, label: p.proofs.find((x) => x.id === pf)?.name ?? pf, sub: pf, proofs: [pf] })),
      bar: true,
    });
  }
  const regression = all.filter((o) => o.role === "regression");
  if (regression.length) {
    const byUc = [...new Set(regression.map((o) => o.useCase))];
    groups.push({
      key: "regression",
      title: "Regression",
      hint: "proofs the use cases already had",
      rows: byUc.map((uc) => ({ key: `uc:${uc}`, label: uc, sub: `${regression.filter((o) => o.useCase === uc).length} proof(s)`, proofs: regression.filter((o) => o.useCase === uc).map((o) => o.proof) })),
      bar: false,
    });
  }
  const superseded = all.filter((o) => o.role === "superseded");
  if (superseded.length)
    groups.push({
      key: "superseded",
      title: "Superseded",
      hint: "replaced on purpose: expected to fail",
      rows: superseded.map((o) => ({ key: o.proof, label: o.proof, proofs: [o.proof] })),
      bar: false,
    });
  return groups;
}

function rowState(run: ProofRunRecord, proofs: string[]): State {
  const os = proofs.map((id) => run.outcomes.find((o) => o.proof === id));
  if (os.every((o) => !o)) return "none";
  return os.every((o) => o?.passed) ? "pass" : "fail";
}

/** Consecutive runs with the same state, as Gantt bars. */
function segments(states: State[]) {
  const out: { state: State; from: number; to: number }[] = [];
  states.forEach((s, i) => {
    const last = out[out.length - 1];
    if (last && last.state === s) last.to = i;
    else out.push({ state: s, from: i, to: i });
  });
  return out;
}

/** The first run from which a row passes in every later run (undefined if it does not pass in the last run). */
function passingSince(states: State[]): number | undefined {
  if (states[states.length - 1] !== "pass") return undefined;
  let i = states.length - 1;
  while (i > 0 && states[i - 1] === "pass") i--;
  return i;
}

function formalOf(pf: ProofEntry): string {
  try {
    return pf.check.kind === "clause"
      ? `${pf.check.requires ? `${show(pf.check.requires)}  ⇒  ` : ""}${show(pf.check.ensures)}`
      : `${JSON.stringify(pf.check.input)}  ⇒  ${show(pf.check.expect)}`;
  } catch {
    return JSON.stringify(pf.check);
  }
}

function ProofDetail({ id, runs, at, proofs, onPick }: { id: string; runs: ProofRunRecord[]; at: number; proofs: Map<string, ProofEntry>; onPick: (run: number) => void }) {
  const pf = proofs.get(id);
  const o = runs[at]!.outcomes.find((x) => x.proof === id);
  return (
    <div className="grid gap-3 rounded-lg border bg-card p-4" data-testid="proof-detail">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-xs text-muted-foreground">{id}</span>
        <span className="font-semibold">{pf?.name ?? id}</span>
        {o ? <Badge variant={verdictVariant(o)}>{o.verdict}</Badge> : <Badge variant="outline">not in this run</Badge>}
        {o && <span className="text-xs text-muted-foreground">{o.role === "proposal" ? `proves ${shortId(o.intent ?? "")}` : o.role === "regression" ? `regression of ${o.useCase}` : `superseded (${o.useCase})`}</span>}
      </div>
      {pf?.description && <p className="text-sm">{pf.description}</p>}
      {pf && (
        <div className="grid gap-1">
          <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            What it checks ({pf.check.kind}, model {pf.model}, subject {pf.subject})
          </span>
          <pre className="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs">{formalOf(pf)}</pre>
        </div>
      )}
      {o?.counterexample && (
        <div className="grid gap-1" data-testid="counterexample">
          <span className="text-xs font-medium uppercase tracking-wide text-fail">Counterexample in run {runs[at]!.run}</span>
          <pre className="overflow-x-auto rounded-md bg-fail-soft p-3 font-mono text-xs">
            {`input  ${JSON.stringify(o.counterexample.input)}\noutput ${JSON.stringify(o.counterexample.output)}`}
          </pre>
        </div>
      )}
      {o?.note && <p className="text-sm text-muted-foreground">{o.note}</p>}
      <div className="grid gap-1">
        <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Across runs</span>
        <div className="flex flex-wrap gap-1">
          {runs.map((r, i) => {
            const x = r.outcomes.find((y) => y.proof === id);
            return (
              <button
                key={r.run}
                type="button"
                onClick={() => onPick(i)}
                className={cn("h-6 min-w-6 rounded px-1 font-mono text-[10px]", cellClass(x), i === at && "ring-2 ring-ring ring-offset-1 ring-offset-background")}
                aria-label={`Run ${r.run}: ${x ? x.verdict : "not run"}`}
              >
                {r.run}
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}

/**
 * The proofs' time-lapse: every `lastro prove` run of the proposal, oldest to newest, one row per proof grouped by intent,
 * with a Gantt bar per intent (failing, then passing). Click a run or a cell to see what was tested and what came back.
 */
export function Timeline({ p, runs, proofs }: { p: Proposal; runs: ProofRunRecord[]; proofs: Map<string, ProofEntry> }) {
  const sorted = useMemo(() => [...runs].sort((a, b) => a.run - b.run), [runs]);
  const groups = useMemo(() => groupsOf(p, sorted), [p, sorted]);
  const [at, setAt] = useState(Math.max(0, sorted.length - 1));
  const [proof, setProof] = useState<string>();
  const [playing, setPlaying] = useState(false);
  const [follow, setFollow] = useState(true);

  // New runs arrive live while the agent iterates: stay on the latest unless someone picked a run.
  useEffect(() => {
    if (follow) setAt(Math.max(0, sorted.length - 1));
  }, [sorted.length, follow]);

  useEffect(() => {
    if (!playing) return;
    const t = setInterval(() => {
      setAt((i) => {
        if (i >= sorted.length - 1) {
          setPlaying(false);
          return i;
        }
        return i + 1;
      });
    }, 700);
    return () => clearInterval(t);
  }, [playing, sorted.length]);

  if (!sorted.length)
    return (
      <Empty>
        No proof runs yet. Each <code className="font-mono">lastro prove</code> on this proposal is recorded in{" "}
        <code className="font-mono">
          {p.id}-{p.slug}.runs.jsonl
        </code>
        , and shows up here as the implementation goes.
      </Empty>
    );

  const pick = (i: number, id?: string) => {
    setPlaying(false);
    setFollow(i === sorted.length - 1);
    setAt(i);
    if (id !== undefined) setProof(id);
  };
  const run = sorted[at]!;
  const prev = at > 0 ? sorted[at - 1] : undefined;
  const own = run.outcomes.filter((o) => o.role === "proposal");
  const flips = prev
    ? run.outcomes
        .map((o) => ({ o, before: prev.outcomes.find((x) => x.proof === o.proof) }))
        .filter(({ o, before }) => stateOf(before) !== stateOf(o))
    : [];
  const firstGreen = sorted.findIndex((r) => r.ok);
  const intentStates = groups.filter((g) => g.bar).map((g) => sorted.map((r) => rowState(r, g.rows.flatMap((x) => x.proofs))));
  const cols = `minmax(8rem, 16rem) repeat(${sorted.length}, minmax(1.75rem, 1fr))`;

  return (
    <div className="grid gap-4" data-testid="timeline">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
        <span>
          <strong>{sorted.length}</strong> run{sorted.length > 1 ? "s" : ""}
          {sorted.length > 1 && <span className="text-muted-foreground"> over {duration(sorted[0]!.at, sorted[sorted.length - 1]!.at)}</span>}
        </span>
        <span>
          {firstGreen >= 0 ? (
            <span className="text-proved">All proofs passed first at run {sorted[firstGreen]!.run}</span>
          ) : (
            <span className="text-fail">Not all proofs pass yet</span>
          )}
        </span>
        <span className="text-muted-foreground">
          {intentStates.filter((s) => s[s.length - 1] === "pass").length} of {intentStates.length} intents passing now
        </span>
        <div className="ml-auto flex items-center gap-1">
          <Button size="icon" variant="outline" className="size-8" onClick={() => pick(0)} aria-label="First run">
            <SkipBack />
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              if (playing) return setPlaying(false);
              setFollow(false);
              if (at >= sorted.length - 1) setAt(0);
              setPlaying(true);
            }}
            aria-label={playing ? "Pause the time-lapse" : "Play the time-lapse"}
          >
            {playing ? <Pause /> : <Play />} {playing ? "Pause" : "Play"}
          </Button>
          <Button size="icon" variant="outline" className="size-8" onClick={() => pick(sorted.length - 1)} aria-label="Latest run">
            <SkipForward />
          </Button>
        </div>
      </div>

      <label className="grid gap-1 text-xs text-muted-foreground">
        <span>
          Run {run.run} of {sorted.length} · {dateTime(run.at)}
        </span>
        <input type="range" min={0} max={sorted.length - 1} value={at} onChange={(e) => pick(Number(e.target.value))} aria-label="Run" className="w-full accent-[hsl(var(--brass))]" />
      </label>

      <div className="overflow-x-auto rounded-xl border bg-card" data-testid="gantt">
        <div className="grid min-w-max gap-y-0.5 py-3 pr-3 text-xs" style={{ gridTemplateColumns: cols }} role="grid" aria-label="Proof runs by intent">
          <div role="row" className="contents">
            <div role="columnheader" className="sticky left-0 z-10 bg-card pb-1 pl-3 font-medium text-muted-foreground">
              Intent · proof
            </div>
            {sorted.map((r, i) => (
              <button
                key={r.run}
                type="button"
                role="columnheader"
                onClick={() => pick(i)}
                className={cn("grid justify-items-center rounded pb-1 hover:bg-accent", i === at && "bg-accent font-semibold text-foreground")}
                aria-label={`Run ${r.run} at ${time(r.at)}: ${r.ok ? "all passed" : "failures"}`}
                aria-pressed={i === at}
              >
                <span className="font-mono">{r.run}</span>
                <span className={cn("size-1.5 rounded-full", r.ok ? "bg-proved" : "bg-fail")} aria-hidden />
                {(i === 0 || i === sorted.length - 1 || i === at) && <span className="whitespace-nowrap text-[10px] text-muted-foreground">{time(r.at)}</span>}
              </button>
            ))}
          </div>

          {groups.map((g) => {
            const states = sorted.map((r) => rowState(r, g.rows.flatMap((x) => x.proofs)));
            const since = passingSince(states);
            return (
              <div key={g.key} role="rowgroup" className="contents" data-testid="gantt-group" data-group={g.key}>
                <div role="row" className="contents">
                  <div role="rowheader" className="sticky left-0 z-10 mt-2 truncate bg-card pl-3 pr-2 font-semibold" title={g.title}>
                    {g.title}
                    {g.hint && <span className="ml-1 font-normal text-muted-foreground">({g.hint})</span>}
                  </div>
                  {g.bar ? (
                    <div className="relative mt-2" style={{ gridColumn: `2 / span ${sorted.length}` }}>
                      <div className="grid h-5" style={{ gridTemplateColumns: `repeat(${sorted.length}, minmax(0, 1fr))` }}>
                        {segments(states).map((s) => (
                          <div
                            key={s.from}
                            className={cn(
                              "mx-px h-5 rounded-full",
                              s.state === "pass" ? "bg-proved/80" : s.state === "fail" ? "bg-fail/70" : "bg-muted",
                              s.from > at && "opacity-25",
                            )}
                            style={{ gridColumn: `${s.from + 1} / ${s.to + 2}` }}
                            title={`${s.state === "pass" ? "passing" : s.state === "fail" ? "failing" : "not run"}: runs ${sorted[s.from]!.run}–${sorted[s.to]!.run}`}
                          />
                        ))}
                      </div>
                      {since !== undefined && (
                        <span className="pointer-events-none absolute inset-y-0 flex items-center pl-2 text-[10px] font-medium text-white" style={{ left: `${(since / sorted.length) * 100}%` }} data-testid="passing-since">
                          passes since run {sorted[since]!.run}
                        </span>
                      )}
                    </div>
                  ) : (
                    <div className="mt-2" style={{ gridColumn: `2 / span ${sorted.length}` }} />
                  )}
                </div>
                {g.rows.map((row) => (
                  <div key={row.key} role="row" className="contents">
                    <button
                      type="button"
                      role="rowheader"
                      onClick={() => pick(at, row.proofs[0])}
                      className={cn("sticky left-0 z-10 truncate bg-card pl-6 pr-2 text-left hover:underline", row.proofs.includes(proof ?? "") && "font-semibold")}
                      title={`${row.label}${row.sub ? ` (${row.sub})` : ""}`}
                    >
                      {row.label}
                      {row.sub && row.sub !== row.label && <span className="ml-1 font-mono text-[10px] text-muted-foreground">{row.sub}</span>}
                    </button>
                    {sorted.map((r, i) => {
                      const os = row.proofs.map((id) => r.outcomes.find((o) => o.proof === id));
                      const failing = os.find((o) => o && !o.passed);
                      const shown = failing ?? os.find(Boolean);
                      return (
                        <button
                          key={r.run}
                          type="button"
                          role="gridcell"
                          onClick={() => pick(i, failing?.proof ?? row.proofs[0])}
                          className={cn(
                            "mx-px h-5 rounded-sm transition-opacity",
                            cellClass(shown),
                            i > at && "opacity-25",
                            i === at && row.proofs.includes(proof ?? "") && "ring-2 ring-ring ring-offset-1 ring-offset-card",
                          )}
                          aria-label={`${row.label}, run ${r.run}: ${shown ? (failing ? `failed (${failing.verdict})` : shown.verdict) : "not run"}`}
                          data-state={stateOf(shown)}
                        />
                      );
                    })}
                  </div>
                ))}
              </div>
            );
          })}
        </div>
        <div className="flex flex-wrap gap-3 border-t px-3 py-2 text-[11px] text-muted-foreground">
          <span className="inline-flex items-center gap-1">
            <span className="size-2.5 rounded-sm bg-proved" /> proved
          </span>
          <span className="inline-flex items-center gap-1">
            <span className="size-2.5 rounded-sm bg-evidence" /> evidenced
          </span>
          <span className="inline-flex items-center gap-1">
            <span className="size-2.5 rounded-sm bg-fail" /> failed
          </span>
          <span className="inline-flex items-center gap-1">
            <span className="size-2.5 rounded-sm bg-muted" /> not in that run
          </span>
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="grid content-start gap-3 rounded-lg border bg-card p-4" data-testid="run-detail">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-semibold">Run {run.run}</span>
            <Badge variant={run.ok ? "proved" : "fail"}>{run.ok ? "all passed" : "not proved"}</Badge>
            <span className="text-xs text-muted-foreground">{dateTime(run.at)}</span>
          </div>
          <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
            {run.commit && (
              <span className="inline-flex items-center gap-1 font-mono">
                <GitCommitHorizontal className="size-3.5" aria-hidden />
                {run.commit.slice(0, 10)}
                {run.dirty && <span className="font-sans text-evidence">+ uncommitted changes</span>}
              </span>
            )}
            <span>
              {own.filter((o) => o.passed).length}/{own.length} proposal proofs pass
            </span>
            <span>{run.regressions.length ? <span className="text-fail">{run.regressions.length} regression(s)</span> : "no regressions"}</span>
            {run.enginesAgree !== undefined && <span>{run.enginesAgree ? "both engines agree" : <span className="text-fail">engines disagree</span>}</span>}
          </div>
          {prev && (
            <div className="grid gap-1 text-sm" data-testid="flips">
              <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Since run {prev.run}</span>
              {!flips.length && <span className="text-muted-foreground">No proof changed state.</span>}
              {flips.map(({ o }) => (
                <button key={o.proof} type="button" className="flex items-center gap-1.5 text-left hover:underline" onClick={() => setProof(o.proof)}>
                  {o.passed ? <ArrowUp className="size-3.5 text-proved" aria-hidden /> : <ArrowDown className="size-3.5 text-fail" aria-hidden />}
                  <span className="font-mono text-xs">{o.proof}</span> <span>{o.passed ? "now passes" : `now fails (${o.verdict})`}</span>
                </button>
              ))}
            </div>
          )}
          <div className="grid gap-1">
            <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Outcomes</span>
            <ul className="grid gap-0.5 text-sm">
              {[...run.outcomes]
                .sort((a, b) => Number(a.passed) - Number(b.passed))
                .map((o) => (
                  <li key={o.proof}>
                    <button type="button" onClick={() => setProof(o.proof)} className={cn("flex w-full items-center gap-2 rounded px-1 py-0.5 text-left hover:bg-accent", proof === o.proof && "bg-accent")}>
                      <span className={cn("size-2 shrink-0 rounded-full", cellClass(o))} aria-hidden />
                      <span className="shrink-0 whitespace-nowrap font-mono text-xs">{o.proof}</span>
                      <span className="truncate">{proofs.get(o.proof)?.name ?? ""}</span>
                      <span className="ml-auto text-xs text-muted-foreground">{o.verdict}</span>
                    </button>
                  </li>
                ))}
            </ul>
          </div>
        </div>
        {proof ? (
          <ProofDetail id={proof} runs={sorted} at={at} proofs={proofs} onPick={(i) => pick(i)} />
        ) : (
          <Empty>Click a cell, a proof or an outcome to see what it checks and what came back in this run.</Empty>
        )}
      </div>
    </div>
  );
}
