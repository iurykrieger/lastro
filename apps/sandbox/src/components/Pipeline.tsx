import { useState } from "react";
import { ArrowRight, Bot, CheckCircle2, Copy, FileCode2, GitPullRequest, ListChecks, Loader2, ShieldCheck, User, Users } from "lucide-react";
import { agentFeedback, failuresOf, specGapsOf } from "@lastro/sdk";
import { Button } from "@/components/ui/button";
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
      detail: !ts
        ? "waits for the spec"
        : proved
          ? "every proof passes"
          : `${ts.clauses.filter((c) => c.verdict === "violated").length + ts.scenarios.filter((s) => s.status === "failed").length} obligation(s) fail · ${ts.summary.gap} intent gap(s)`,
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
      <ol aria-label="Pipeline gates" className="grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
        {stages.map((s, k) => (
          <li key={s.title} aria-label={s.title} className={cn("relative grid gap-0.5 rounded-lg border p-2.5", TONE[s.state])}>
            <span className="flex items-center gap-1.5 text-sm font-semibold">
              <s.icon className="h-4 w-4" />
              {s.title}
            </span>
            <span className="text-xs text-muted-foreground">{s.detail}</span>
            {k < stages.length - 1 && <ArrowRight className="absolute -right-2.5 top-1/2 z-10 hidden h-3.5 w-3.5 -translate-y-1/2 bg-background text-muted-foreground lg:block" />}
          </li>
        ))}
      </ol>
      <LastroFeedback l={l} />
    </div>
  );
}

/** What Lastro says right now, gate by gate: who has to act, and on what. */
function LastroFeedback({ l }: { l: Lastro }) {
  const [copied, setCopied] = useState(false);
  const ts = l.results.ts;
  const contradictions = ts?.decisions ?? [];
  const proof = ts && l.plan ? { failures: failuresOf(l.plan, ts), specGaps: specGapsOf(ts).filter((g) => !g.startsWith("Contradiction")) } : null;
  const brief = proof ? agentFeedback(proof) : "";

  let tone = "border-proved/50";
  let body: React.ReactNode;
  if (l.pending.length) {
    tone = "border-evidence/60";
    body = (
      <Item icon={<Users className="text-evidence" />} who="The team" title={`Review is not finished (${l.pending.length} item${l.pending.length > 1 ? "s" : ""})`}>
        <ul className="list-disc pl-4">
          {l.pending.slice(0, 6).map((p) => (
            <li key={p}>{p}</li>
          ))}
          {l.pending.length > 6 && <li>and {l.pending.length - 6} more</li>}
        </ul>
        <p className="text-muted-foreground">Decisions sit under the intents they affect, in the Intents tab.</p>
      </Item>
    );
  } else if (!l.plan) {
    tone = l.ai.error?.step === "derive" ? "border-fail/50" : "border-brass/60";
    body = (
      <Item icon={l.ai.busy === "derive" ? <Loader2 className="animate-spin text-brass" /> : <FileCode2 className="text-brass" />} who="Lastro" title={l.ai.busy === "derive" ? "Deriving the formal spec from the approved intents…" : "No formal spec yet"}>
        {l.ai.error?.step === "derive" && <p className="text-fail">{l.ai.error.message}</p>}
        {l.ai.available === false && <p className="text-muted-foreground">Deriving a spec for edited prose needs Claude.</p>}
      </Item>
    );
  } else if (contradictions.length) {
    tone = "border-fail/50";
    body = (
      <Item icon={<Users className="text-fail" />} who="The team" title={`The spec contradicts itself (${contradictions.length})`}>
        <ul className="list-disc pl-4">
          {contradictions.map((d) => (
            <li key={d.id}>{d.question}</li>
          ))}
        </ul>
        <div>
          <Button size="sm" variant="outline" className="h-7" onClick={() => l.returnDecisions(contradictions)}>
            Send back to the team as decisions
          </Button>
        </div>
      </Item>
    );
  } else if (proof && proof.failures.length) {
    tone = "border-fail/50";
    body = (
      <Item
        icon={<Bot className="text-fail" />}
        who="Whoever writes the code"
        title={`${proof.failures.length} proof obligation${proof.failures.length > 1 ? "s" : ""} fail`}
        action={
          <Button
            size="sm"
            variant="ghost"
            className="h-7"
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(brief);
                setCopied(true);
              } catch {
                setCopied(false);
              }
            }}
          >
            <Copy /> {copied ? "Copied" : "Copy for the agent"}
          </Button>
        }
      >
        <pre className="max-h-40 overflow-auto whitespace-pre-wrap rounded-md bg-muted p-2 font-mono text-[11px]">{brief}</pre>
        <p className="text-muted-foreground">
          The same text <code className="font-mono">lastro feedback</code> and <code className="font-mono">agentFeedback()</code> return. Any coding agent or person fixes the code; the proofs decide.
        </p>
      </Item>
    );
  } else if (proof && proof.specGaps.length) {
    tone = "border-evidence/60";
    body = (
      <Item icon={<Users className="text-evidence" />} who="The team" title="The code satisfies every clause, but the spec has gaps">
        <ul className="list-disc pl-4">
          {proof.specGaps.map((g) => (
            <li key={g}>{g}</li>
          ))}
        </ul>
      </Item>
    );
  } else if (ts && !ts.ok) {
    tone = "border-fail/50";
    body = (
      <Item icon={<ShieldCheck className="text-fail" />} who="The team" title="A trusted piece does not hold">
        <p>A production trace breaks an assumption the proof relies on. See the trusted pieces in the Contrast panel.</p>
      </Item>
    );
  } else if (ts) {
    body = (
      <Item icon={<CheckCircle2 className="text-proved" />} who="Lastro" title="Every gate passes">
        <p className="text-muted-foreground">Proved code and a receipt for the pull request (Contrast → Receipt).</p>
      </Item>
    );
  } else body = <Item icon={<Loader2 className="animate-spin" />} who="Lastro" title="Proving…" />;

  return <section aria-label="Lastro feedback" aria-live="polite" className={cn("rounded-lg border-2 bg-card p-3", tone)}>{body}</section>;
}

function Item({ icon, who, title, action, children }: { icon: React.ReactNode; who: string; title: string; action?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <div className="grid gap-1.5 text-xs [&_svg]:h-4 [&_svg]:w-4">
      <div className="flex flex-wrap items-center gap-2">
        {icon}
        <span className="text-sm font-semibold">{title}</span>
        <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">next: {who}</span>
        {action && <span className="ml-auto">{action}</span>}
      </div>
      {children}
    </div>
  );
}
