import { CheckCircle2, CircleDashed, Lock } from "lucide-react";
import { countedApprovals, isProposalApproved, pendingProposal, type Proposal } from "@lastro/sdk";
import { Person } from "@/components/bits";
import { counts, need } from "@/lib/review";
import { cn } from "@/lib/utils";

function Row({ ok, children }: { ok: boolean; children: React.ReactNode }) {
  return (
    <li className="flex items-start gap-2">
      {ok ? <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-proved" aria-label="done" /> : <CircleDashed className="mt-0.5 size-4 shrink-0 text-evidence" aria-label="pending" />}
      <span>{children}</span>
    </li>
  );
}

/** The merge box: what still blocks the proposal's approval, under its policy, and who reviewed what. */
export function ReviewBox({ p }: { p: Proposal }) {
  const pending = pendingProposal(p);
  const c = counts(p);
  const approved = isProposalApproved(p);
  const openDecisions = p.decisions.filter((d) => d.chosen === undefined).length;
  const openThreads = p.threads.filter((t) => !t.resolved).length;
  const stale = p.useCases.filter((u) => u.stale && u.intents.some((i) => p.intents.find((x) => x.id === i)?.status !== "rejected")).length;
  const people = new Map<string, number>();
  for (const it of p.intents) for (const a of countedApprovals(p, it)) people.set(a, (people.get(a) ?? 0) + 1);

  return (
    <aside className="grid gap-4 rounded-xl border bg-card p-4 text-sm" aria-label="Review status" data-testid="review-box">
      <div className={cn("flex items-center gap-2 font-semibold", approved ? "text-proved" : "text-foreground")}>
        {approved ? <Lock className="size-4" aria-hidden /> : <CircleDashed className="size-4 text-evidence" aria-hidden />}
        {approved ? "Approved: the intents are the brief" : pending.length ? `${pending.length} item${pending.length > 1 ? "s" : ""} block approval` : "Ready to approve"}
      </div>
      <ul className="grid gap-1.5">
        <Row ok={c.settled === p.intents.length}>
          {c.settled} of {p.intents.length} intents settled
          <span className="block text-xs text-muted-foreground">
            {c.approved} approved · {c.amended} amended · {c.rejected} rejected · {c.proposed} to review
          </span>
        </Row>
        <Row ok={openDecisions === 0}>{openDecisions ? `${openDecisions} decision${openDecisions > 1 ? "s" : ""} to answer` : "Every decision answered"}</Row>
        <Row ok={openThreads === 0}>{openThreads ? `${openThreads} open thread${openThreads > 1 ? "s" : ""}` : "No open threads"}</Row>
        <Row ok={stale === 0}>{stale ? `${stale} use case${stale > 1 ? "s" : ""} with a stale after` : "Every after is current"}</Row>
      </ul>
      <div className="grid gap-1 border-t pt-3 text-xs text-muted-foreground">
        <span>
          Policy: <strong className="text-foreground">{need(p)}</strong> approval{need(p) > 1 ? "s" : ""} per intent; the submitter ({p.submittedBy}) {p.policy.submitterCanApprove ? "counts" : "does not count"} as a reviewer.
        </span>
        <span>Intents were written by the LLM; every person is a reviewer.</span>
      </div>
      {people.size > 0 && (
        <div className="grid gap-2 border-t pt-3">
          <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Reviewers</span>
          {[...people].map(([name, n]) => (
            <div key={name} className="flex items-center gap-2">
              <Person name={name} />
              <span className="flex-1">{name}</span>
              <span className="text-xs text-muted-foreground">{n} approved</span>
            </div>
          ))}
        </div>
      )}
      {pending.length > 0 && (
        <details className="border-t pt-3">
          <summary className="cursor-pointer text-xs font-medium uppercase tracking-wide text-muted-foreground">What blocks it</summary>
          <ul className="mt-2 grid max-h-64 gap-1 overflow-y-auto font-mono text-[11px] text-muted-foreground">
            {pending.map((x) => (
              <li key={x}>{x}</li>
            ))}
          </ul>
        </details>
      )}
      {approved && <p className="break-all border-t pt-3 font-mono text-[11px] text-muted-foreground">hash {p.hash}</p>}
    </aside>
  );
}
