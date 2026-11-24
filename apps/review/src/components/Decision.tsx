import { useState } from "react";
import { CircleHelp } from "lucide-react";
import type { Proposal, ProposalDecision } from "@lastro/sdk";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { useWorkspace } from "@/lib/workspace";
import { reviewOpen, shortId } from "@/lib/review";
import { cn } from "@/lib/utils";

/** A question the extraction (or derivation) could not settle from the prose. The team answers it. */
export function DecisionView({ p, d }: { p: Proposal; d: ProposalDecision }) {
  const { act, busy, reviewer } = useWorkspace();
  const [editing, setEditing] = useState(d.chosen === undefined);
  const [choice, setChoice] = useState<string>(String(d.chosen ?? d.recommended ?? 0));
  const answered = d.chosen !== undefined;
  const chosen = answered ? d.options[d.chosen!] : undefined;
  const name = `decision-${d.id}`;
  return (
    <div className={cn("rounded-lg border p-4", answered ? "bg-card" : "border-evidence/50 bg-evidence-soft/40")} data-testid="decision" data-decision={d.id}>
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <CircleHelp className="size-4 text-evidence" aria-hidden />
        <span className="font-mono">{d.id}</span>
        {d.name && <span className="font-medium text-foreground">{d.name}</span>}
        <span>· from {d.origin}</span>
        {d.intents.length > 0 && <span>· affects {d.intents.map(shortId).join(", ")}</span>}
        {answered ? <Badge variant="proved">Answered{d.answeredBy ? ` by ${d.answeredBy}` : ""}</Badge> : <Badge variant="evidence">Needs an answer</Badge>}
      </div>
      <p className="mt-2 font-medium">{d.question}</p>
      {answered && !editing ? (
        <div className="mt-2 flex flex-wrap items-center gap-3 text-sm">
          <span>
            → <strong>{chosen?.label}</strong>
            {chosen?.consequence ? <span className="text-muted-foreground">: {chosen.consequence}</span> : null}
          </span>
          {reviewOpen(p) && (
            <Button size="sm" variant="ghost" onClick={() => setEditing(true)} disabled={!reviewer}>
              Change answer
            </Button>
          )}
        </div>
      ) : (
        <form
          className="mt-3 grid gap-3"
          onSubmit={async (e) => {
            e.preventDefault();
            if (await act(p.id, { type: "answer", decision: d.id, option: Number(choice) })) setEditing(false);
          }}
        >
          <RadioGroup value={choice} onValueChange={setChoice} aria-label={d.question}>
            {d.options.map((o, k) => (
              <div key={k} className="flex items-start gap-2.5">
                <RadioGroupItem value={String(k)} id={`${name}-${k}`} className="mt-0.5" />
                <Label htmlFor={`${name}-${k}`} className="grid gap-0.5 font-normal leading-snug">
                  <span className="font-medium">
                    {o.label} {k === d.recommended && <Badge variant="outline" className="ml-1 align-middle">Recommended</Badge>}
                  </span>
                  {o.consequence && <span className="text-muted-foreground">{o.consequence}</span>}
                </Label>
              </div>
            ))}
          </RadioGroup>
          {reviewOpen(p) && (
            <div className="flex gap-2">
              <Button type="submit" size="sm" disabled={busy || !reviewer}>
                Answer
              </Button>
              {answered && (
                <Button type="button" size="sm" variant="ghost" onClick={() => setEditing(false)}>
                  Cancel
                </Button>
              )}
            </div>
          )}
        </form>
      )}
    </div>
  );
}
