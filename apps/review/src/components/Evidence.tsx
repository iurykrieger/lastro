import { useMemo, useState } from "react";
import { Check, GitBranch, Image as ImageIcon, MessageSquareWarning, Workflow } from "lucide-react";
import { dueOf, intentEvidence, judgmentIntents, type IntentEvidence, type Proposal, type ProposalIntent, type SnapshotRecord } from "@lastro/sdk";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Person } from "@/components/bits";
import { useWorkspace } from "@/lib/workspace";
import { parseStatechart } from "@/lib/statechart";
import { cn } from "@/lib/utils";

/** A snapshot file, served from the workspace's docs/ folder. */
export const fileUrl = (path: string) => `/api/files?path=${encodeURIComponent(path)}`;
const fileName = (path: string) => path.slice(path.lastIndexOf("/") + 1);
const show = (v: unknown) => (v === undefined ? "—" : JSON.stringify(v));

/**
 * The evidence for one intent: its place in the feature map, the statechart of its state-machine model, the before
 * snapshots during review and, after the proof run, the after snapshots with their comparison, every proof's verdict
 * with its counterexample, the unexpected changes to accept and the sign-off an intent beyond proof waits for.
 */
export function Evidence({ p, it }: { p: Proposal; it: ProposalIntent }) {
  const { data } = useWorkspace();
  const e = useMemo<IntentEvidence | undefined>(() => {
    try {
      return intentEvidence(p, it.id, data?.useCases ?? []);
    } catch {
      return undefined;
    }
  }, [p, it.id, data?.useCases]);
  if (!e || it.status === "rejected") return null;
  const unexpected = (p.unexpected ?? []).filter((u) => u.intent === it.id);
  const judgment = judgmentIntents(p).includes(it.id) && !!p.lastRun;
  const empty = !e.map && !e.mapError && !e.statecharts.length && !e.before.length && !e.after.length && !e.verdicts.length && !unexpected.length && !judgment;
  if (empty) return null;
  return (
    <section className="mt-3 grid gap-3 rounded-md border bg-muted/30 p-3" data-testid="evidence" aria-label={`Evidence for ${it.id}`}>
      <FeaturePlace e={e} />
      {e.statecharts.map((s) => (
        <Statechart key={s.model} model={s.model} mermaid={s.mermaid} />
      ))}
      <Snapshots e={e} />
      {e.verdicts.length > 0 && <Verdicts e={e} />}
      {unexpected.length > 0 && <Unexpected p={p} changes={unexpected} />}
      {judgment && <SignOff p={p} it={it} after={e.after} />}
    </section>
  );
}

function Heading({ icon: Icon, children }: { icon: typeof Check; children: React.ReactNode }) {
  return (
    <h5 className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
      <Icon className="size-3.5" aria-hidden /> {children}
    </h5>
  );
}

function FeaturePlace({ e }: { e: IntentEvidence }) {
  if (!e.map) return e.mapError ? <p className="text-xs text-fail" data-testid="evidence-map">Feature map: {e.mapError}</p> : null;
  const { entry, affected } = e.map;
  return (
    <div className="grid gap-1 text-sm" data-testid="evidence-map">
      <Heading icon={GitBranch}>Feature map</Heading>
      <p>
        <span className="font-mono text-xs">{entry.id}</span> <span className="font-medium">{entry.name}</span>
        {!entry.reachable && <Badge variant="evidence" className="ml-2">not reachable by a driver</Badge>}
      </p>
      {entry.surfaces.length > 0 && (
        <ul className="flex flex-wrap gap-1.5 text-xs">
          {entry.surfaces.map((s) => (
            <li key={`${s.kind}:${s.locator}`} className="rounded border bg-card px-1.5 py-0.5">
              {s.kind} <span className="font-medium">{s.name}</span> <span className="font-mono text-muted-foreground">{s.locator}</span>
            </li>
          ))}
        </ul>
      )}
      <p className="text-xs text-muted-foreground">
        Depends on: {entry.dependsOn.length ? entry.dependsOn.join(", ") : "nothing"} · Needed by: {entry.dependents.length ? entry.dependents.join(", ") : "nothing"}
      </p>
      {affected.length > 0 && (
        <p className="text-xs" data-testid="affected">
          Affected by this change: <span className="font-medium">{affected.join(", ")}</span> (their proofs and snapshots run again)
        </p>
      )}
    </div>
  );
}

function Statechart({ model, mermaid }: { model: string; mermaid: string }) {
  const chart = useMemo(() => parseStatechart(mermaid), [mermaid]);
  return (
    <figure className="grid gap-1.5" data-testid="statechart" aria-label={`Statechart of ${model}`}>
      <Heading icon={Workflow}>Statechart · {model}</Heading>
      <div className="grid gap-2 sm:grid-cols-2">
        {chart.regions.map((r) => (
          <div key={r.name} className="rounded border bg-card p-2 text-xs">
            <p className="mb-1 font-medium">{r.name}</p>
            <ul className="mb-1.5 flex flex-wrap gap-1">
              {r.states.map((s) => (
                <li key={s.id} className={cn("rounded px-1.5 py-0.5 font-mono", s.forbidden ? "bg-fail-soft text-fail" : "bg-muted", s.initial && "ring-1 ring-brass")} title={s.forbidden ? "an invariant forbids this state" : s.initial ? "initial state" : undefined}>
                  {s.label}
                </li>
              ))}
            </ul>
            <ul className="grid gap-0.5">
              {r.transitions.map((t, i) => (
                <li key={i} className={cn("font-mono", t.counterexample && "font-semibold text-fail")} data-counterexample={t.counterexample || undefined}>
                  {t.from} → {t.to} <span className="text-muted-foreground">: {t.event}</span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
      <details className="text-xs">
        <summary className="cursor-pointer text-muted-foreground">Mermaid source</summary>
        <pre className="mt-1 overflow-x-auto rounded bg-card p-2">{mermaid}</pre>
      </details>
    </figure>
  );
}

function SnapshotLink({ s }: { s: SnapshotRecord }) {
  return (
    <a className="font-mono text-xs underline-offset-2 hover:underline" href={fileUrl(s.path)} target="_blank" rel="noreferrer" title={`sha256 ${s.sha256}`}>
      {s.side}/{fileName(s.path)}
    </a>
  );
}

function Snapshots({ e }: { e: IntentEvidence }) {
  if (!e.before.length && !e.after.length) return null;
  const compared = new Set(e.comparisons.map((c) => `${c.kind}:${c.name}`));
  const alone = [...e.before, ...e.after].filter((s) => !compared.has(`${s.kind}:${fileName(s.path)}`));
  return (
    <div className="grid gap-2" data-testid="snapshots">
      <Heading icon={ImageIcon}>{e.after.length ? "Before and after" : "Before (the after snapshots come with the proof run)"}</Heading>
      {e.comparisons.map((c) => {
        const before = e.before.find((s) => s.kind === c.kind && fileName(s.path) === c.name);
        const after = e.after.find((s) => s.kind === c.kind && fileName(s.path) === c.name);
        return (
          <div key={`${c.kind}:${c.name}`} className="grid gap-1.5 rounded border bg-card p-2 text-xs" data-testid="comparison" data-view={c.view}>
            <p className="flex flex-wrap items-center gap-2">
              <span className="font-mono">{c.name}</span>
              <Badge variant="outline">{c.kind}</Badge>
              {c.view === "images" && <span>{((c.pixelShare ?? 0) * 100).toFixed(2)}% of pixels changed</span>}
              {c.breaking && <Badge variant="fail">breaking</Badge>}
            </p>
            {c.view === "images" && before && after && (
              <div className="grid grid-cols-2 gap-2">
                {[before, after].map((s) => (
                  <figure key={s.side} className="grid gap-0.5">
                    <img src={fileUrl(s.path)} alt={`${s.side}: ${c.name}`} className="max-h-64 w-full rounded border object-contain" />
                    <figcaption className="text-muted-foreground">{s.side}</figcaption>
                  </figure>
                ))}
              </div>
            )}
            {c.view === "diff" &&
              (c.diff?.length ? (
                <table className="w-full text-left">
                  <thead className="text-muted-foreground">
                    <tr>
                      <th className="font-normal">Field</th>
                      <th className="font-normal">Change</th>
                      <th className="font-normal">Before</th>
                      <th className="font-normal">After</th>
                    </tr>
                  </thead>
                  <tbody>
                    {c.diff.map((d) => (
                      <tr key={d.path} className={cn(d.breaking && "text-fail")}>
                        <td className="font-mono">{d.path}</td>
                        <td>{d.change}{d.breaking ? " (breaking)" : ""}</td>
                        <td className="font-mono">{d.before ?? "—"}</td>
                        <td className="font-mono">{d.after ?? "—"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <p className="text-muted-foreground">No structural change.</p>
              ))}
            {c.view === "rows" &&
              (c.changedRows?.length ? (
                <table className="w-full text-left">
                  <thead className="text-muted-foreground">
                    <tr>
                      <th className="font-normal">Input</th>
                      <th className="font-normal">Before</th>
                      <th className="font-normal">After</th>
                    </tr>
                  </thead>
                  <tbody>
                    {c.changedRows.map((r, i) => (
                      <tr key={i} className="font-mono">
                        <td>{show(r.input)}</td>
                        <td>{show(r.before)}</td>
                        <td>{show(r.after)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <p className="text-muted-foreground">No example changed its output.</p>
              ))}
            <p className="flex gap-3">
              {before && <SnapshotLink s={before} />}
              {after && <SnapshotLink s={after} />}
            </p>
          </div>
        );
      })}
      {alone.length > 0 && (
        <ul className="grid gap-1">
          {alone.map((s) => (
            <li key={s.path} className="flex flex-wrap items-center gap-2 text-xs">
              <Badge variant="outline">{s.kind}</Badge>
              {s.kind === "screen" ? (
                <a href={fileUrl(s.path)} target="_blank" rel="noreferrer">
                  <img src={fileUrl(s.path)} alt={`${s.side}: ${fileName(s.path)}`} className="max-h-40 rounded border" />
                </a>
              ) : (
                <SnapshotLink s={s} />
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function Verdicts({ e }: { e: IntentEvidence }) {
  return (
    <div className="grid gap-1" data-testid="verdicts">
      <Heading icon={Check}>Proof run</Heading>
      <ul className="grid gap-1 text-xs">
        {e.verdicts.map((v) => (
          <li key={v.proof} className="grid gap-0.5">
            <span className="flex items-center gap-2">
              <span className="font-mono">{v.proof}</span>
              <Badge variant={v.passed ? (v.verdict === "proved" ? "proved" : "evidence") : "fail"}>{v.verdict}</Badge>
            </span>
            {v.counterexample && (
              <span className="font-mono text-fail" data-testid="counterexample">
                counterexample: {show(v.counterexample.input)} → {show(v.counterexample.output)}
              </span>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

function Unexpected({ p, changes }: { p: Proposal; changes: NonNullable<Proposal["unexpected"]> }) {
  const { act, busy, reviewer } = useWorkspace();
  const open = new Set(dueOf(p).unexpected);
  return (
    <div className="grid gap-1.5" data-testid="unexpected">
      <Heading icon={MessageSquareWarning}>Unexpected changes</Heading>
      <p className="text-xs text-muted-foreground">These outputs changed although no approved intent covers their input. The proposal is not proved until each is accepted.</p>
      {changes.map((u) => (
        <div key={u.id} className="flex flex-wrap items-center gap-2 rounded border border-evidence/40 bg-card p-2 text-xs" data-change={u.id}>
          <span className="font-mono font-medium">{u.id}</span>
          <span className="font-mono">
            {show(u.input)}: {show(u.before)} → {show(u.after)}
          </span>
          <span className="ml-auto flex items-center gap-1.5">
            {u.acceptedBy.map((a) => (
              <Person key={a} name={a} />
            ))}
            {open.has(u.id) ? (
              <Button size="sm" variant="outline" disabled={busy || !reviewer || u.acceptedBy.includes(reviewer)} onClick={() => act(p.id, { type: "accept", change: u.id })}>
                <Check aria-hidden /> Accept
              </Button>
            ) : (
              <Badge variant="proved">accepted</Badge>
            )}
          </span>
        </div>
      ))}
    </div>
  );
}

function SignOff({ p, it, after }: { p: Proposal; it: ProposalIntent; after: SnapshotRecord[] }) {
  const { act, busy, reviewer } = useWorkspace();
  const [asking, setAsking] = useState(false);
  const [note, setNote] = useState("");
  const mine = (p.signoffs ?? []).filter((s) => s.intent === it.id);
  const due = dueOf(p);
  const state = due.changesRequested.includes(it.id) ? "changes-requested" : due.signoffs.includes(it.id) ? "due" : "signed-off";
  const shots = after.filter((s) => s.kind === "screen").map((s) => s.path);
  return (
    <div className="grid gap-1.5" data-testid="signoff" data-state={state}>
      <Heading icon={Check}>Sign-off</Heading>
      <p className="text-xs text-muted-foreground">
        This intent is beyond proof for human judgment: look at its before and after, then sign it off or request changes.
        {shots.length > 0 && " Signing off makes the after screenshots the reference for the regressions that follow."}
      </p>
      {mine.map((s) => (
        <p key={s.by} className="flex items-center gap-2 text-xs">
          <Person name={s.by} /> {s.verdict === "signed-off" ? "signed off" : `requested changes: ${s.note ?? ""}`}
        </p>
      ))}
      {!asking ? (
        <div className="flex flex-wrap gap-2">
          <Button size="sm" disabled={busy || !reviewer} onClick={() => act(p.id, { type: "signoff", intent: it.id, references: shots })}>
            <Check aria-hidden /> Sign off
          </Button>
          <Button size="sm" variant="outline" disabled={busy || !reviewer} onClick={() => setAsking(true)}>
            Request changes
          </Button>
        </div>
      ) : (
        <form
          className="grid gap-2"
          onSubmit={async (ev) => {
            ev.preventDefault();
            if (await act(p.id, { type: "request-changes", intent: it.id, note })) {
              setAsking(false);
              setNote("");
            }
          }}
        >
          <Textarea aria-label="What should change?" placeholder="What should change?" value={note} onChange={(ev) => setNote(ev.target.value)} rows={2} />
          <div className="flex justify-end gap-2">
            <Button type="button" size="sm" variant="ghost" onClick={() => setAsking(false)}>
              Cancel
            </Button>
            <Button type="submit" size="sm" variant="destructive" disabled={busy || !note.trim()}>
              Request changes
            </Button>
          </div>
        </form>
      )}
    </div>
  );
}
