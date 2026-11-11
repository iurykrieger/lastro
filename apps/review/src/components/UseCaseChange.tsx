import { useMemo, useState } from "react";
import { AlertTriangle, Pencil, Sparkles } from "lucide-react";
import type { ProofEntry, Proposal, UseCaseChange } from "@lastro/sdk";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { ChangeBadge } from "@/components/bits";
import { IntentCard } from "@/components/IntentCard";
import { Threads } from "@/components/Threads";
import { wordDiff } from "@/lib/diff";
import { href } from "@/lib/router";
import { reviewOpen } from "@/lib/review";
import { useWorkspace } from "@/lib/workspace";
import { cn } from "@/lib/utils";

type View = "diff" | "before" | "after";

/** A use case's before and after, word by word: what a changed file is in a pull request. */
export function DescriptionDiff({ before, after, view = "diff" }: { before?: string; after?: string; view?: View }) {
  const parts = useMemo(() => wordDiff(before ?? "", after ?? ""), [before, after]);
  if (view === "before") return <p className="text-sm leading-relaxed">{before ?? <em className="text-muted-foreground">New use case: no description before.</em>}</p>;
  if (view === "after") return <p className="text-sm leading-relaxed">{after ?? <em className="text-muted-foreground">Removed: no description after.</em>}</p>;
  return (
    <p className="text-sm leading-relaxed" data-testid="diff">
      {parts.map((d, k) =>
        d.kind === "same" ? (
          <span key={k}>{d.text}</span>
        ) : d.kind === "added" ? (
          <ins key={k} className="rounded-sm bg-proved-soft text-proved no-underline">
            {d.text}
          </ins>
        ) : (
          <del key={k} className="rounded-sm bg-fail-soft text-fail">
            {d.text}
          </del>
        ),
      )}
    </p>
  );
}

export function UseCaseChangeView({ p, c, proofs, exists, filter }: { p: Proposal; c: UseCaseChange; proofs: Map<string, ProofEntry>; exists: boolean; filter?: (id: string) => boolean }) {
  const { act, busy, reviewer, data } = useWorkspace();
  const [view, setView] = useState<View>("diff");
  const [editing, setEditing] = useState(false);
  const [after, setAfter] = useState(c.after ?? "");
  const [asking, setAsking] = useState(false);
  const [instructions, setInstructions] = useState("");
  const intents = c.intents.map((id) => p.intents.find((i) => i.id === id)).filter((i): i is NonNullable<typeof i> => !!i);
  const shown = filter ? intents.filter((i) => filter(i.id)) : intents;
  const open = reviewOpen(p);
  const live = intents.some((i) => i.status !== "rejected");

  return (
    <section id={c.useCase} className="scroll-mt-24 rounded-xl border bg-background" data-testid="usecase-change" data-usecase={c.useCase} aria-labelledby={`${c.useCase}-title`}>
      <header className="sticky top-14 z-10 flex flex-wrap items-center gap-2 rounded-t-xl border-b bg-card/95 px-4 py-2.5 backdrop-blur">
        <ChangeBadge change={c.change} />
        <h3 id={`${c.useCase}-title`} className="flex flex-wrap items-baseline gap-x-2 font-semibold">
          {exists ? (
            <a className="font-mono text-sm text-muted-foreground hover:underline" href={href(`usecases/${c.useCase}`)}>
              {c.useCase}
            </a>
          ) : (
            <span className="font-mono text-sm text-muted-foreground">{c.useCase}</span>
          )}
          <span>{c.name}</span>
        </h3>
        <span className="font-mono text-xs text-muted-foreground">{c.slug}</span>
        <span className="ml-auto text-xs text-muted-foreground">
          {intents.length} intent{intents.length === 1 ? "" : "s"}
        </span>
      </header>

      <div className="grid gap-3 p-4">
        {c.stale && live && (
          <div className="flex flex-wrap items-center gap-2 rounded-md border border-evidence/50 bg-evidence-soft/50 px-3 py-2 text-sm" data-testid="stale">
            <AlertTriangle className="size-4 text-evidence" aria-hidden />
            <span className="flex-1">An intent here was amended or rejected, so the description after the change is out of date. Rewrite it, or ask the LLM to recombine it from the remaining intents.</span>
          </div>
        )}

        <div className="flex flex-wrap items-center gap-2">
          {c.change === "modify" && (
            <ToggleGroup type="single" size="sm" variant="outline" value={view} onValueChange={(v) => v && setView(v as View)} aria-label="Description view">
              <ToggleGroupItem value="diff">Diff</ToggleGroupItem>
              <ToggleGroupItem value="before">Before</ToggleGroupItem>
              <ToggleGroupItem value="after">After</ToggleGroupItem>
            </ToggleGroup>
          )}
          {open && c.change !== "remove" && !editing && (
            <div className="ml-auto flex flex-wrap gap-2">
              <Button size="sm" variant="outline" onClick={() => (setAfter(c.after ?? ""), setEditing(true), setAsking(false))} disabled={!reviewer || busy}>
                <Pencil aria-hidden /> Edit after
              </Button>
              <Button size="sm" variant="outline" onClick={() => (setAsking((a) => !a), setEditing(false))} disabled={!reviewer || busy} aria-expanded={asking}>
                <Sparkles aria-hidden /> Recombine with the LLM
              </Button>
            </div>
          )}
        </div>

        {editing ? (
          <form
            className="grid gap-2"
            onSubmit={async (e) => {
              e.preventDefault();
              if (await act(p.id, { type: "rewrite", useCase: c.useCase, after })) setEditing(false);
            }}
          >
            <label className="text-xs font-medium" htmlFor={`${c.useCase}-after`}>
              {c.useCase} after this proposal (its intents need review again)
            </label>
            <Textarea id={`${c.useCase}-after`} value={after} onChange={(e) => setAfter(e.target.value)} rows={8} />
            <div className="flex justify-end gap-2">
              <Button type="button" size="sm" variant="ghost" onClick={() => setEditing(false)}>
                Cancel
              </Button>
              <Button type="submit" size="sm" disabled={busy || !after.trim() || after.trim() === c.after}>
                Save after
              </Button>
            </div>
          </form>
        ) : (
          <div className={cn("rounded-md border bg-card p-3", c.stale && live && "opacity-80")}>
            <DescriptionDiff before={c.change === "add" ? undefined : c.before} after={c.change === "remove" ? undefined : c.after} view={c.change === "modify" ? view : "diff"} />
          </div>
        )}

        {asking && (
          <form
            className="grid gap-2 rounded-md border p-3"
            onSubmit={async (e) => {
              e.preventDefault();
              if (await act(p.id, { type: "recombine", useCase: c.useCase, instructions: instructions || undefined })) {
                setAsking(false);
                setInstructions("");
              }
            }}
          >
            <label className="text-xs font-medium" htmlFor={`${c.useCase}-instructions`}>
              Instructions for the LLM (optional)
            </label>
            <Textarea id={`${c.useCase}-instructions`} value={instructions} onChange={(e) => setInstructions(e.target.value)} rows={2} placeholder="e.g. keep the CLI flags out of it" />
            <p className="text-xs text-muted-foreground">
              {data?.llm === "anthropic"
                ? "Claude rewrites the description from the intents that are not rejected (part of the extract step)."
                : "No ANTHROPIC_API_KEY on the server: Lastro writes a handoff request in .lastro/llm/ for an agent to answer, then you recombine again."}
            </p>
            <div className="flex justify-end gap-2">
              <Button type="button" size="sm" variant="ghost" onClick={() => setAsking(false)}>
                Cancel
              </Button>
              <Button type="submit" size="sm" disabled={busy}>
                Recombine
              </Button>
            </div>
          </form>
        )}

        <Threads p={p} on={c.useCase} startLabel={`Comment on ${c.useCase}`} />

        <div className="grid gap-3">
          {shown.map((it) => (
            <IntentCard key={it.id} p={p} it={it} proofs={proofs} />
          ))}
        </div>
      </div>
    </section>
  );
}
