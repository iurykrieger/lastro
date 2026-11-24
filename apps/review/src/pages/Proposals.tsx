import { MessageSquare } from "lucide-react";
import { pendingProposal } from "@lastro/sdk";
import { Empty, ProposalStatusBadge } from "@/components/bits";
import { counts, waitsFor } from "@/lib/review";
import { href } from "@/lib/router";
import { useWorkspace } from "@/lib/workspace";

export function ProposalsPage() {
  const { data, reviewer } = useWorkspace();
  const proposals = [...(data?.proposals ?? [])].reverse();
  return (
    <div className="grid gap-4">
      <div>
        <h1 className="text-2xl font-semibold">Proposals</h1>
        <p className="text-sm text-muted-foreground">Every change starts as prose. Lastro reads it into intents, and you review the intents instead of the code.</p>
      </div>
      {!proposals.length && (
        <Empty>
          No proposals in <code className="font-mono">{data?.dir}</code> yet. Run <code className="font-mono">lastro propose &lt;prose-file&gt;</code> to start one.
        </Empty>
      )}
      <ul className="grid divide-y rounded-xl border bg-card">
        {proposals.map((p) => {
          const c = counts(p);
          const mine = p.intents.filter((i) => waitsFor(p, i, reviewer)).length;
          const pending = pendingProposal(p).length;
          const threads = p.threads.filter((t) => !t.resolved).length;
          return (
            <li key={p.id} data-testid="proposal-row">
              <a href={href(`proposals/${p.id}`)} className="grid gap-1 px-4 py-3 hover:bg-accent/50">
                <span className="flex flex-wrap items-center gap-2">
                  <span className="font-mono text-xs text-muted-foreground">{p.id}</span>
                  <span className="font-semibold">{p.name}</span>
                  <ProposalStatusBadge status={p.status} />
                  {mine > 0 && <span className="rounded-full bg-evidence-soft px-2 py-0.5 text-xs font-medium text-evidence">{mine} waiting for you</span>}
                </span>
                <span className="line-clamp-2 text-sm text-muted-foreground">{p.description}</span>
                <span className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                  <span>submitted by {p.submittedBy}</span>
                  <span>
                    {c.settled}/{p.intents.length} intents settled
                  </span>
                  <span>{p.useCases.length} use cases touched</span>
                  {pending > 0 && <span>{pending} blocking</span>}
                  {threads > 0 && (
                    <span className="inline-flex items-center gap-1">
                      <MessageSquare className="size-3" aria-hidden /> {threads} open
                    </span>
                  )}
                </span>
              </a>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
