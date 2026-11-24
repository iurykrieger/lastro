import { useEffect, useMemo, useState } from "react";
import { Activity, FileDiff, FlaskConical, MessagesSquare } from "lucide-react";
import type { Proposal } from "@lastro/sdk";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { ChangeBadge, Empty, ProposalStatusBadge } from "@/components/bits";
import { DecisionView } from "@/components/Decision";
import { Markdown } from "@/components/Markdown";
import { ReviewBox } from "@/components/ReviewBox";
import { ThreadView, Threads } from "@/components/Threads";
import { Timeline } from "@/components/Timeline";
import { UseCaseChangeView } from "@/components/UseCaseChange";
import { proofIndex, settled, waitsFor } from "@/lib/review";
import { href, type Route } from "@/lib/router";
import { useWorkspace } from "@/lib/workspace";
import { cn } from "@/lib/utils";

type Tab = "conversation" | "changes" | "proofs" | "timeline";
const TABS: Tab[] = ["changes", "proofs", "timeline"];
type Filter = "all" | "mine" | "open";

function Conversation({ p }: { p: Proposal }) {
  const decisions = p.decisions;
  const elsewhere = p.threads.filter((t) => t.on !== "prose");
  return (
    <div className="grid gap-6">
      {decisions.length > 0 && (
        <section className="grid gap-3" aria-labelledby="decisions">
          <h2 id="decisions" className="text-lg font-semibold">
            Decisions
          </h2>
          {decisions.map((d) => (
            <DecisionView key={d.id} p={p} d={d} />
          ))}
        </section>
      )}
      <section className="grid gap-3" aria-labelledby="prose">
        <h2 id="prose" className="text-lg font-semibold">
          Prose
        </h2>
        <div className="max-h-[70vh] overflow-y-auto rounded-xl border bg-card p-5" data-testid="prose">
          <Markdown text={p.prose} />
        </div>
        <Threads p={p} on="prose" startLabel="Comment on the prose" />
      </section>
      {elsewhere.length > 0 && (
        <section className="grid gap-3" aria-labelledby="threads">
          <h2 id="threads" className="text-lg font-semibold">
            Threads on intents and use cases
          </h2>
          {elsewhere.map((t) => (
            <ThreadView key={t.id} p={p} t={t} showTarget />
          ))}
        </section>
      )}
      {p.notPromised.length > 0 && (
        <section className="grid gap-2" aria-labelledby="not-promised">
          <h2 id="not-promised" className="text-lg font-semibold">
            Not promised
          </h2>
          <ul className="grid list-disc gap-1 pl-5 text-sm text-muted-foreground">
            {p.notPromised.map((n) => (
              <li key={n}>{n}</li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

function Changes({ p, focus }: { p: Proposal; focus?: string }) {
  const { data, reviewer } = useWorkspace();
  const [filter, setFilter] = useState<Filter>("all");
  const proofs = useMemo(() => proofIndex(data?.useCases ?? [], data?.proposals), [data]);
  const known = new Set((data?.useCases ?? []).map((u) => u.id));
  const keep = (id: string) => {
    const it = p.intents.find((i) => i.id === id)!;
    return filter === "all" || (filter === "mine" ? waitsFor(p, it, reviewer) : !settled(p, it));
  };
  const changes = p.useCases.filter((c) => filter === "all" || c.intents.some(keep));

  useEffect(() => {
    if (focus) document.getElementById(focus)?.scrollIntoView({ block: "start" });
  }, [focus]);

  return (
    <div className="grid gap-4 lg:grid-cols-[14rem_minmax(0,1fr)]">
      <nav aria-label="Use cases touched" className="lg:sticky lg:top-20 lg:max-h-[calc(100vh-6rem)] lg:self-start lg:overflow-y-auto">
        <ul className="grid gap-0.5 text-sm">
          {p.useCases.map((c) => {
            const its = c.intents.map((id) => p.intents.find((i) => i.id === id)!).filter(Boolean);
            const open = its.filter((i) => !settled(p, i)).length;
            return (
              <li key={c.useCase}>
                <a href={href(`proposals/${p.id}/changes?uc=${c.useCase}`)} className={cn("flex items-center gap-2 rounded-md px-2 py-1 hover:bg-accent", focus === c.useCase && "bg-accent")}>
                  <ChangeBadge change={c.change} compact />
                  <span className="min-w-0 flex-1 truncate" title={c.name}>
                    {c.name}
                  </span>
                  {open > 0 ? <span className="text-xs text-evidence">{open}</span> : <span className="text-xs text-proved">✓</span>}
                </a>
              </li>
            );
          })}
        </ul>
      </nav>
      <div className="grid min-w-0 gap-4">
        <div className="flex flex-wrap items-center gap-2">
          <ToggleGroup type="single" size="sm" variant="outline" value={filter} onValueChange={(v) => v && setFilter(v as Filter)} aria-label="Show intents">
            <ToggleGroupItem value="all">All intents</ToggleGroupItem>
            <ToggleGroupItem value="open">Not settled</ToggleGroupItem>
            <ToggleGroupItem value="mine">Waiting for me</ToggleGroupItem>
          </ToggleGroup>
        </div>
        {!changes.length && <Empty>Nothing here: every intent in this view is settled.</Empty>}
        {changes.map((c) => (
          <UseCaseChangeView key={c.useCase} p={p} c={c} proofs={proofs} exists={known.has(c.useCase)} filter={filter === "all" ? undefined : keep} />
        ))}
      </div>
    </div>
  );
}

function Proofs({ p }: { p: Proposal }) {
  return (
    <div className="grid gap-4">
      {p.supersedes.length > 0 && (
        <div className="rounded-xl border border-fail/40 bg-fail-soft/40 p-4 text-sm">
          <p className="font-medium text-fail">Supersedes {p.supersedes.length} existing proof(s)</p>
          <p className="mt-1 font-mono text-xs">{p.supersedes.join(", ")}</p>
        </div>
      )}
      {!p.proofs.length ? (
        <Empty>No proofs yet. Proofs are derived from the approved and amended intents once the proposal is approved (`lastro derive`).</Empty>
      ) : (
        <ul className="grid gap-3">
          {p.proofs.map((pf) => (
            <li key={pf.id} className="rounded-lg border bg-card p-4" data-testid="proof">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono text-xs text-muted-foreground">{pf.id}</span>
                <span className="font-semibold">{pf.name}</span>
                <Badge variant={pf.status === "passed" ? "proved" : pf.status === "failed" ? "fail" : "outline"}>{pf.verdict ?? pf.status}</Badge>
                <span className="ml-auto text-xs text-muted-foreground">
                  {pf.intent} → <a className="hover:underline" href={href(`usecases/${pf.useCase}`)}>{pf.useCase}</a>
                </span>
              </div>
              <p className="mt-1 text-sm">{pf.description}</p>
              {pf.replaces?.length ? <p className="mt-1 text-xs text-muted-foreground">Takes the place of {pf.replaces.join(", ")}</p> : null}
            </li>
          ))}
        </ul>
      )}
      {p.lastRun && (
        <div className="rounded-xl border bg-card p-4 text-sm">
          <p className="font-medium">Last run: {p.lastRun.ok ? "every proof passed" : `${p.lastRun.outcomes.filter((o) => !o.passed).length} failed`}</p>
          {p.lastRun.regressions.length > 0 && <p className="mt-1 text-fail">Regressions: {p.lastRun.regressions.join(", ")}</p>}
          {p.lastRun.unproved.length > 0 && <p className="mt-1 text-xs text-muted-foreground">Unproved use cases: {p.lastRun.unproved.join(", ")}</p>}
        </div>
      )}
    </div>
  );
}

export function ProposalPage({ route }: { route: Route }) {
  const { data, reviewer } = useWorkspace();
  const id = route.path[1]!;
  const p = data?.proposals.find((x) => x.id === id || x.slug === id);
  const tab: Tab = TABS.find((t) => t === route.path[2]) ?? "conversation";
  const runs = data?.runs?.[p?.id ?? ""] ?? [];
  const proofs = useMemo(() => proofIndex(data?.useCases ?? [], data?.proposals), [data]);
  useEffect(() => {
    if (p) document.title = `${p.id} ${p.name} · Lastro Review`;
  }, [p]);
  if (!p)
    return (
      <Empty>
        No proposal {id}. <a className="underline" href={href("proposals")}>See every proposal</a>.
      </Empty>
    );
  const mine = p.intents.filter((i) => waitsFor(p, i, reviewer)).length;
  const openDecisions = p.decisions.filter((d) => d.chosen === undefined).length;

  return (
    <div className="grid gap-5">
      <header className="grid gap-2">
        <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <a href={href("proposals")} className="hover:underline">
            Proposals
          </a>
          <span aria-hidden>/</span>
          <span className="font-mono">{p.id}</span>
        </div>
        <h1 className="flex flex-wrap items-center gap-3 text-2xl font-semibold">
          {p.name} <ProposalStatusBadge status={p.status} />
        </h1>
        <p className="max-w-3xl text-sm text-muted-foreground">{p.description}</p>
        <p className="text-xs text-muted-foreground">
          Submitted by <strong className="text-foreground">{p.submittedBy}</strong> · intents by the LLM · {p.intents.length} intents over {p.useCases.length} use cases
          {mine > 0 && <span className="ml-2 rounded-full bg-evidence-soft px-2 py-0.5 font-medium text-evidence">{mine} waiting for you</span>}
        </p>
      </header>
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_18rem]">
        <Tabs value={tab} onValueChange={(v) => (location.hash = href(`proposals/${p.id}${v === "conversation" ? "" : `/${v}`}`))} className="min-w-0">
          <TabsList className="h-auto flex-wrap">
            <TabsTrigger value="conversation">
              <MessagesSquare className="size-4" aria-hidden /> Conversation
              {openDecisions > 0 && <span className="rounded-full bg-evidence-soft px-1.5 text-[11px] text-evidence">{openDecisions}</span>}
            </TabsTrigger>
            <TabsTrigger value="changes">
              <FileDiff className="size-4" aria-hidden /> Use cases changed <span className="text-xs text-muted-foreground">{p.useCases.length}</span>
            </TabsTrigger>
            <TabsTrigger value="proofs">
              <FlaskConical className="size-4" aria-hidden /> Proofs <span className="text-xs text-muted-foreground">{p.proofs.length}</span>
            </TabsTrigger>
            <TabsTrigger value="timeline">
              <Activity className="size-4" aria-hidden /> Time-lapse <span className="text-xs text-muted-foreground">{runs.length}</span>
            </TabsTrigger>
          </TabsList>
          <TabsContent value="conversation">
            <Conversation p={p} />
          </TabsContent>
          <TabsContent value="changes">
            <Changes p={p} focus={route.query.get("uc") ?? undefined} />
          </TabsContent>
          <TabsContent value="proofs">
            <Proofs p={p} />
          </TabsContent>
          <TabsContent value="timeline">
            <Timeline key={p.id} p={p} runs={runs} proofs={proofs} />
          </TabsContent>
        </Tabs>
        <div className="xl:sticky xl:top-20 xl:self-start">
          <ReviewBox p={p} />
        </div>
      </div>
    </div>
  );
}
