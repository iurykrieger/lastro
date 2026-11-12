import { useEffect, useMemo, useState } from "react";
import { Search } from "lucide-react";
import type { Proposal, UseCase } from "@lastro/sdk";
import { Badge } from "@/components/ui/badge";
import { ChangeBadge, Empty, ProposalStatusBadge } from "@/components/bits";
import { DescriptionDiff } from "@/components/UseCaseChange";
import { href, type Route } from "@/lib/router";
import { reviewOpen } from "@/lib/review";
import { useWorkspace } from "@/lib/workspace";

/** Proposals that are not applied or closed and touch this use case. */
function pendingFor(id: string, proposals: Proposal[]) {
  return proposals.filter((p) => p.status !== "applied" && p.status !== "closed").flatMap((p) => p.useCases.filter((c) => c.useCase === id).map((c) => ({ p, c })));
}

export function UseCasesPage() {
  const { data } = useWorkspace();
  const [q, setQ] = useState("");
  const all = data?.useCases ?? [];
  const proposals = data?.proposals ?? [];
  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    return s ? all.filter((u) => `${u.id} ${u.slug} ${u.name} ${u.description}`.toLowerCase().includes(s)) : all;
  }, [all, q]);
  const proofs = all.reduce((n, u) => n + u.proofs.length, 0);
  const unproved = all.filter((u) => !u.proofs.length).length;
  useEffect(() => {
    document.title = "Use cases · Lastro Review";
  }, []);

  return (
    <div className="grid gap-4">
      <div>
        <h1 className="text-2xl font-semibold">Use cases</h1>
        <p className="text-sm text-muted-foreground">
          The application as it is today: {all.length} use cases, {proofs} regression proof{proofs === 1 ? "" : "s"}, {unproved} without proofs yet.
        </p>
      </div>
      <label className="relative max-w-md">
        <span className="sr-only">Filter use cases</span>
        <Search className="pointer-events-none absolute left-2.5 top-2.5 size-4 text-muted-foreground" aria-hidden />
        <input
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Filter by id, name or behavior"
          className="h-9 w-full rounded-md border border-input bg-card pl-8 pr-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        />
      </label>
      {!shown.length && <Empty>No use case matches “{q}”.</Empty>}
      <ul className="grid divide-y rounded-xl border bg-card">
        {shown.map((u) => {
          const pending = pendingFor(u.id, proposals);
          return (
            <li key={u.id} data-testid="usecase-row">
              <a href={href(`usecases/${u.id}`)} className="grid gap-1 px-4 py-3 hover:bg-accent/50">
                <span className="flex flex-wrap items-center gap-2">
                  <span className="font-mono text-xs text-muted-foreground">{u.id}</span>
                  <span className="font-semibold">{u.name}</span>
                  <Badge variant={u.proofs.length ? "proved" : "outline"}>{u.proofs.length ? `${u.proofs.length} proof${u.proofs.length > 1 ? "s" : ""}` : "unproved"}</Badge>
                  {pending.map(({ p, c }) => (
                    <span key={p.id} className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                      <ChangeBadge change={c.change} /> in {p.id}
                    </span>
                  ))}
                </span>
                <span className="line-clamp-2 text-sm text-muted-foreground">{u.description}</span>
              </a>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function ProofItem({ pf }: { pf: UseCase["proofs"][number] }) {
  return (
    <li className="rounded-lg border bg-card p-4" data-testid="proof">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-xs text-muted-foreground">{pf.id}</span>
        <span className="font-semibold">{pf.name}</span>
        <Badge variant={pf.status === "passed" ? "proved" : pf.status === "failed" ? "fail" : "outline"}>{pf.verdict ?? pf.status}</Badge>
        <span className="ml-auto text-xs text-muted-foreground">from {pf.intent}</span>
      </div>
      <p className="mt-1 text-sm">{pf.description}</p>
      <details className="mt-2">
        <summary className="cursor-pointer text-xs text-muted-foreground">Check ({pf.check.kind}, model {pf.model}, subject {pf.subject})</summary>
        <pre className="mt-2 overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs">{JSON.stringify(pf.check, null, 2)}</pre>
      </details>
    </li>
  );
}

export function UseCasePage({ route }: { route: Route }) {
  const { data } = useWorkspace();
  const id = route.path[1]!;
  const u = data?.useCases.find((x) => x.id === id || x.slug === id);
  useEffect(() => {
    if (u) document.title = `${u.id} ${u.name} · Lastro Review`;
  }, [u]);
  if (!u)
    return (
      <Empty>
        No use case {id}. <a className="underline" href={href("usecases")}>See every use case</a>.
      </Empty>
    );
  const pending = pendingFor(u.id, data?.proposals ?? []);
  return (
    <div className="grid max-w-4xl gap-6">
      <header className="grid gap-2">
        <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <a href={href("usecases")} className="hover:underline">
            Use cases
          </a>
          <span aria-hidden>/</span>
          <span className="font-mono">{u.id}</span>
        </div>
        <h1 className="text-2xl font-semibold">{u.name}</h1>
        <p className="font-mono text-xs text-muted-foreground">{u.slug}</p>
      </header>
      <section className="grid gap-2" aria-labelledby="behavior">
        <h2 id="behavior" className="text-lg font-semibold">
          Behavior
        </h2>
        <p className="rounded-xl border bg-card p-4 text-sm leading-relaxed">{u.description}</p>
      </section>
      {pending.length > 0 && (
        <section className="grid gap-3" aria-labelledby="pending">
          <h2 id="pending" className="text-lg font-semibold">
            Proposed changes
          </h2>
          {pending.map(({ p, c }) => (
            <div key={p.id} className="grid gap-2 rounded-xl border bg-card p-4">
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <ChangeBadge change={c.change} />
                <a className="font-medium hover:underline" href={href(`proposals/${p.id}/changes?uc=${c.useCase}`)}>
                  {p.id} {p.name}
                </a>
                <ProposalStatusBadge status={p.status} />
                {reviewOpen(p) && <span className="text-xs text-muted-foreground">{c.intents.length} intent(s) in review</span>}
              </div>
              <DescriptionDiff before={c.change === "add" ? undefined : c.before ?? u.description} after={c.change === "remove" ? undefined : c.after} />
            </div>
          ))}
        </section>
      )}
      <section className="grid gap-3" aria-labelledby="proofs">
        <h2 id="proofs" className="text-lg font-semibold">
          Regression proofs
        </h2>
        {u.proofs.length ? (
          <ul className="grid gap-3">
            {u.proofs.map((pf) => (
              <ProofItem key={pf.id} pf={pf} />
            ))}
          </ul>
        ) : (
          <Empty>No proofs yet. A use case gains proofs when a proposal that changes it is proved and applied; until then it is reported as unproved.</Empty>
        )}
      </section>
      <section className="grid gap-2" aria-labelledby="history">
        <h2 id="history" className="text-lg font-semibold">
          History
        </h2>
        {u.history.length ? (
          <ul className="grid gap-1 text-sm">
            {u.history.map((h) => {
              const p = data?.proposals.find((x) => x.id === h);
              return (
                <li key={h}>
                  <a className="hover:underline" href={href(`proposals/${h}`)}>
                    <span className="font-mono text-xs text-muted-foreground">{h}</span> {p?.name ?? ""}
                  </a>
                </li>
              );
            })}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">No applied proposal has changed it yet.</p>
        )}
      </section>
    </div>
  );
}
