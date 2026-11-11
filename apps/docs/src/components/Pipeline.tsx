import { ArrowRight, Bot, Cog, User } from "lucide-react";
import { cn } from "@/lib/utils";

/** The four stages, who runs each, and what it hands to the next. */
const STAGES = [
  { name: "plan", who: "AI", out: "intents", note: "extract reviewable intents from prose" },
  { name: "review", who: "People", out: "approved intents", note: "approve, amend, reject, answer decisions" },
  { name: "spec", who: "AI", out: "formal spec", note: "derive LSL clauses, then validate and check consistency" },
  { name: "prove", who: "Deterministic", out: "proof + receipt", note: "check the code in two engines" },
] as const;

const ICON = { AI: Bot, People: User, Deterministic: Cog };
const TONE = { AI: "text-evidence bg-evidence-soft", People: "text-trusted bg-trusted-soft", Deterministic: "text-proved bg-proved-soft" };

export function Pipeline({ className }: { className?: string }) {
  return (
    <figure className={cn("my-8", className)} aria-label="The Lastro pipeline">
      <ol className="grid gap-2 sm:grid-cols-4">
        {STAGES.map((s, i) => {
          const Icon = ICON[s.who];
          return (
            <li key={s.name} className="relative flex flex-col rounded-lg border bg-card p-3">
              <span className={cn("mb-2 inline-flex w-fit items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium", TONE[s.who])}>
                <Icon className="h-3 w-3" /> {s.who}
              </span>
              <span className="font-mono text-sm font-semibold">{s.name}</span>
              <span className="mt-1 text-xs leading-snug text-muted-foreground">{s.note}</span>
              <span className="mt-auto pt-3 text-xs">
                → <span className="font-medium">{s.out}</span>
              </span>
              {i < STAGES.length - 1 && (
                <ArrowRight aria-hidden className="absolute -right-[13px] top-1/2 z-10 hidden h-4 w-4 -translate-y-1/2 rounded-full bg-background text-muted-foreground sm:block" />
              )}
            </li>
          );
        })}
      </ol>
      <figcaption className="mt-3 grid gap-1 text-xs text-muted-foreground sm:grid-cols-2">
        <span>
          <span className="font-medium text-foreground">Contradictions</span> found by <span className="font-mono">spec</span> return to review as decisions.
        </span>
        <span>
          <span className="font-medium text-foreground">Failures</span> found by <span className="font-mono">prove</span> go to whoever writes the code. Lastro never
          writes it.
        </span>
      </figcaption>
    </figure>
  );
}
