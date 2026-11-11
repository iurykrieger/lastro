import { useState } from "react";
import { Check, Pencil, Quote, ShieldAlert, X } from "lucide-react";
import { countedApprovals, type ProofEntry, type Proposal, type ProposalIntent } from "@lastro/sdk";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { ChangeBadge, IntentStatusBadge, Person } from "@/components/bits";
import { Threads } from "@/components/Threads";
import { useWorkspace } from "@/lib/workspace";
import { mayApprove, need, reviewOpen, settled, shortId } from "@/lib/review";
import { cn } from "@/lib/utils";

type Mode = "idle" | "amend" | "reject";

/**
 * One intent, reviewed like a comment on a pull request: approve it, amend its wording, or reject it.
 * Approving accepts the replacement of the proofs it lists.
 */
export function IntentCard({ p, it, proofs }: { p: Proposal; it: ProposalIntent; proofs: Map<string, ProofEntry> }) {
  const { act, busy, reviewer } = useWorkspace();
  const [mode, setMode] = useState<Mode>("idle");
  const [description, setDescription] = useState(it.description);
  const [note, setNote] = useState("");
  const counted = countedApprovals(p, it);
  const open = reviewOpen(p);
  const mine = it.approvals.includes(reviewer);
  const approvable = open && it.status !== "rejected" && mayApprove(p, reviewer) && !mine;
  const done = settled(p, it);

  const close = () => {
    setMode("idle");
    setNote("");
  };

  return (
    <article
      id={it.id}
      className={cn("scroll-mt-24 rounded-lg border bg-card p-4", it.status === "rejected" && "opacity-75", !done && "border-l-4 border-l-evidence")}
      data-testid="intent"
      data-intent={it.id}
      aria-labelledby={`${it.id}-name`}
    >
      <header className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-xs text-muted-foreground">{shortId(it.id)}</span>
        <h4 id={`${it.id}-name`} className="font-semibold">
          {it.name}
        </h4>
        <ChangeBadge change={it.change} />
        <Badge variant={it.provability === "formal" ? "proved" : it.provability === "empirical" ? "evidence" : "beyond"} title={it.beyondProofReason ?? undefined}>
          {it.provability}
        </Badge>
        <span className="ml-auto flex items-center gap-2">
          <IntentStatusBadge status={it.status} />
        </span>
      </header>

      <p className={cn("mt-2 text-sm leading-relaxed", it.status === "rejected" && "line-through decoration-muted-foreground/60")}>{it.description}</p>
      {it.originalDescription && it.originalDescription !== it.description && (
        <p className="mt-1 text-xs text-muted-foreground">
          <span className="font-medium">Extracted as:</span> {it.originalDescription}
        </p>
      )}
      {it.reviewNote && <p className="mt-1 text-xs text-muted-foreground">Note: {it.reviewNote}</p>}

      {it.source.quote && (
        <blockquote className="mt-3 flex gap-2 border-l-2 border-brass/60 pl-3 text-xs text-muted-foreground">
          <Quote className="mt-0.5 size-3 shrink-0" aria-hidden />
          <span className="whitespace-pre-wrap">{it.source.quote}</span>
        </blockquote>
      )}

      {it.replaces.length > 0 && (
        <div className="mt-3 grid gap-2 rounded-md border border-fail/40 bg-fail-soft/40 p-3 text-sm" data-testid="replaces">
          <p className="flex items-center gap-1.5 font-medium text-fail">
            <ShieldAlert className="size-4" aria-hidden /> Approving {shortId(it.id)} replaces {it.replaces.length} existing proof{it.replaces.length > 1 ? "s" : ""}
          </p>
          {it.replaces.map((r) => {
            const pf = proofs.get(r.proof);
            return (
              <div key={r.proof} className="grid gap-0.5">
                <span>
                  <span className="font-mono text-xs">{r.proof}</span> {pf ? <span className="font-medium">{pf.name}</span> : <span className="text-muted-foreground">(not found in the use cases)</span>}
                </span>
                {pf && <span className="text-xs text-muted-foreground">Guarantees today: {pf.description}</span>}
                <span className="text-xs">Why: {r.reason}</span>
              </div>
            );
          })}
        </div>
      )}

      <footer className="mt-3 flex flex-wrap items-center gap-3 border-t pt-3">
        <div className="flex items-center gap-2 text-xs text-muted-foreground" data-testid="approvals">
          <div className="flex -space-x-1.5">
            {it.approvals.map((a) => (
              <Person key={a} name={a} muted={!counted.includes(a)} />
            ))}
          </div>
          <span>
            {it.status === "rejected" ? "Rejected" : `${counted.length} of ${need(p)} approval${need(p) > 1 ? "s" : ""}`}
            {it.approvals.length > counted.length && " (the submitter's approval does not count)"}
          </span>
        </div>
        {open && mode === "idle" && (
          <div className="ml-auto flex flex-wrap gap-2">
            {mine && it.status !== "rejected" ? (
              <Badge variant="proved" className="h-8 px-3">
                <Check className="size-3.5" aria-hidden /> You approved
              </Badge>
            ) : (
              <Button
                size="sm"
                onClick={() => act(p.id, { type: "approve", intent: it.id })}
                disabled={busy || !approvable}
                title={!reviewer ? "Say who you are first" : !mayApprove(p, reviewer) ? "Under this policy the submitter can amend but not approve" : undefined}
              >
                <Check aria-hidden /> Approve
              </Button>
            )}
            <Button size="sm" variant="outline" onClick={() => (setDescription(it.description), setMode("amend"))} disabled={busy || !reviewer}>
              <Pencil aria-hidden /> {it.status === "rejected" ? "Amend to restore" : "Amend"}
            </Button>
            {it.status !== "rejected" && (
              <Button size="sm" variant="outline" className="text-fail hover:text-fail" onClick={() => setMode("reject")} disabled={busy || !reviewer}>
                <X aria-hidden /> Reject
              </Button>
            )}
          </div>
        )}
      </footer>

      {mode === "amend" && (
        <form
          className="mt-3 grid gap-2"
          onSubmit={async (e) => {
            e.preventDefault();
            if (await act(p.id, { type: "amend", intent: it.id, description, note: note || undefined })) close();
          }}
        >
          <label className="text-xs font-medium" htmlFor={`${it.id}-amend`}>
            New wording (your amendment counts as your approval; others review it again)
          </label>
          <Textarea id={`${it.id}-amend`} value={description} onChange={(e) => setDescription(e.target.value)} rows={4} />
          <Textarea aria-label="Why (optional)" placeholder="Why (optional)" value={note} onChange={(e) => setNote(e.target.value)} rows={1} />
          <div className="flex justify-end gap-2">
            <Button type="button" size="sm" variant="ghost" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" size="sm" disabled={busy || !description.trim() || description.trim() === it.description}>
              Save amendment
            </Button>
          </div>
        </form>
      )}

      {mode === "reject" && (
        <form
          className="mt-3 grid gap-2"
          onSubmit={async (e) => {
            e.preventDefault();
            if (await act(p.id, { type: "reject", intent: it.id, note })) close();
          }}
        >
          <label className="text-xs font-medium" htmlFor={`${it.id}-reject`}>
            Why should this not be built?
          </label>
          <Textarea id={`${it.id}-reject`} value={note} onChange={(e) => setNote(e.target.value)} rows={2} />
          <div className="flex justify-end gap-2">
            <Button type="button" size="sm" variant="ghost" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" size="sm" variant="destructive" disabled={busy || !note.trim()}>
              Reject intent
            </Button>
          </div>
        </form>
      )}

      <div className="mt-2">
        <Threads p={p} on={it.id} />
      </div>
    </article>
  );
}
