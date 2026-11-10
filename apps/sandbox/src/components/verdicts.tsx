import { Badge, type BadgeProps } from "@/components/ui/badge";
import type { ClauseResult, IntentVerdict } from "@lastro/sdk";

type V = NonNullable<BadgeProps["variant"]>;

export const INTENT_VERDICT: Record<IntentVerdict, { label: string; variant: V }> = {
  proved: { label: "Proved", variant: "proved" },
  "proved-under-assumptions": { label: "Proved under assumptions", variant: "trusted" },
  evidenced: { label: "Evidenced", variant: "evidence" },
  violated: { label: "Violated", variant: "fail" },
  "beyond-proof": { label: "Beyond proof", variant: "beyond" },
  gap: { label: "Gap", variant: "fail" },
};

export const CLAUSE_VERDICT: Record<ClauseResult["verdict"], { label: string; variant: V; hint: string }> = {
  proved: { label: "Proved", variant: "proved", hint: "Holds on every input of the model and rejects at least one planted mutant." },
  evidenced: { label: "Evidenced", variant: "evidence", hint: "Holds on every checked input, but the inputs do not cover the whole space." },
  violated: { label: "Fails", variant: "fail", hint: "A counterexample exists." },
  vacuous: { label: "Vacuous", variant: "evidence", hint: "No run makes the if-part true: the clause proves nothing here." },
  weak: { label: "Weak", variant: "evidence", hint: "Holds, but rejects none of the planted mutants." },
  "not-run": { label: "Not run", variant: "beyond", hint: "No implementation to check." },
};

export function ClauseVerdictBadge({ verdict }: { verdict: ClauseResult["verdict"] }) {
  const v = CLAUSE_VERDICT[verdict];
  return (
    <Badge variant={v.variant} title={v.hint}>
      {v.label}
    </Badge>
  );
}

export function IntentVerdictBadge({ verdict }: { verdict: IntentVerdict }) {
  const v = INTENT_VERDICT[verdict];
  return <Badge variant={v.variant}>{v.label}</Badge>;
}

export const PROVABILITY: Record<string, { label: string; variant: V }> = {
  formal: { label: "formal proof", variant: "proved" },
  empirical: { label: "test", variant: "evidence" },
  "beyond-proof": { label: "beyond proof", variant: "beyond" },
};

export const REASON: Record<string, string> = {
  "external-code": "external code",
  "world-facts": "world facts",
  "termination-timing": "termination & timing",
  "human-judgment": "human judgment",
  unstated: "unstated",
};
