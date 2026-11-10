import { ArrowRight, FileCode2, GitPullRequest, ListChecks, ShieldCheck, User } from "lucide-react";
import { cn } from "@/lib/utils";
import type { Lastro } from "@/state";

type State = "done" | "active" | "blocked" | "waiting";

const TONE: Record<State, string> = {
  done: "border-proved/50 bg-proved-soft/60",
  active: "border-brass/60 bg-brass/10",
  blocked: "border-fail/50 bg-fail-soft/60",
  waiting: "border-dashed opacity-70",
};

/** The flow from the diagram: people review intent, machines check the code. */
export function Pipeline({ l }: { l: Lastro }) {
  const ts = l.results.ts;
  const contradictions = ts?.decisions.length ?? 0;
  const proved = !!ts?.ok && (!l.results.rust || l.results.rust.ok);
  const stages: { icon: typeof User; title: string; detail: string; state: State }[] = [
    { icon: User, title: "You", detail: l.artifact.prose === "" ? "write the rule" : "state intent in plain language", state: "done" },
    {
      icon: ListChecks,
      title: "Plan",
      detail: l.pending.length ? `${l.pending.length} pending · ${l.requiredApprovals} approval(s) each` : `${l.artifact.intents.filter((i) => i.status !== "rejected").length} intents approved`,
      state: l.pending.length ? "active" : "done",
    },
    {
      icon: FileCode2,
      title: "Formal spec",
      detail: !l.frozen ? "waits for the review" : l.ai.busy === "derive" ? "Claude is deriving…" : !l.plan ? "not derived" : contradictions ? `${contradictions} contradiction(s)` : `${l.plan.clauses.length} clauses`,
      state: !l.frozen ? "waiting" : !l.plan ? "active" : contradictions ? "blocked" : "done",
    },
    {
      icon: ShieldCheck,
      title: "Prove",
      detail: !ts ? "waits for the spec" : proved ? "every proof passes" : `${ts.summary.violated} violated · ${ts.summary.gap} gaps`,
      state: !ts ? "waiting" : proved ? "done" : "blocked",
    },
    { icon: GitPullRequest, title: "Pull request", detail: proved ? "proved code and receipt" : "blocked until proofs pass", state: proved ? "done" : "waiting" },
  ];
  return (
    <div className="grid gap-1.5">
      <div className="hidden grid-cols-5 gap-2 text-[11px] font-medium text-muted-foreground lg:grid">
        <span className="col-span-2 border-t pt-1">People review intent</span>
        <span className="col-span-3 border-t pt-1">Machines check the code</span>
      </div>
      <ol className="grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
        {stages.map((s, k) => (
          <li key={s.title} className={cn("relative grid gap-0.5 rounded-lg border p-2.5", TONE[s.state])}>
            <span className="flex items-center gap-1.5 text-sm font-semibold">
              <s.icon className="h-4 w-4" />
              {s.title}
            </span>
            <span className="text-xs text-muted-foreground">{s.detail}</span>
            {k < stages.length - 1 && <ArrowRight className="absolute -right-2.5 top-1/2 z-10 hidden h-3.5 w-3.5 -translate-y-1/2 bg-background text-muted-foreground lg:block" />}
          </li>
        ))}
      </ol>
      {contradictions > 0 && <p className="text-xs text-fail">Contradictions in the spec come back to the team as decisions (see the Contrast panel).</p>}
    </div>
  );
}
