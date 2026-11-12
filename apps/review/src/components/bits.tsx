import type { ChangeKind, ProposalIntentStatus, ProposalStatus } from "@lastro/sdk";
import { Badge, type BadgeProps } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

const STATUS: Record<ProposalStatus, { label: string; variant: BadgeProps["variant"] }> = {
  open: { label: "In review", variant: "evidence" },
  "changes-requested": { label: "Changes requested", variant: "fail" },
  approved: { label: "Approved", variant: "proved" },
  proving: { label: "Proving", variant: "trusted" },
  proved: { label: "Proved", variant: "proved" },
  applied: { label: "Applied", variant: "beyond" },
  closed: { label: "Closed", variant: "beyond" },
};

export function ProposalStatusBadge({ status }: { status: ProposalStatus }) {
  const s = STATUS[status] ?? { label: status, variant: "outline" as const };
  return (
    <Badge variant={s.variant} data-testid="proposal-status">
      {s.label}
    </Badge>
  );
}

const INTENT: Record<ProposalIntentStatus, { label: string; variant: BadgeProps["variant"] }> = {
  proposed: { label: "To review", variant: "evidence" },
  approved: { label: "Approved", variant: "proved" },
  amended: { label: "Amended", variant: "trusted" },
  rejected: { label: "Rejected", variant: "fail" },
};

export function IntentStatusBadge({ status }: { status: ProposalIntentStatus }) {
  const s = INTENT[status];
  return (
    <Badge variant={s.variant} data-testid="intent-status">
      {s.label}
    </Badge>
  );
}

const CHANGE: Record<ChangeKind, { sign: string; cls: string }> = {
  add: { sign: "+", cls: "bg-proved-soft text-proved" },
  modify: { sign: "~", cls: "bg-evidence-soft text-evidence" },
  remove: { sign: "−", cls: "bg-fail-soft text-fail" },
};

export function ChangeBadge({ change, compact, className }: { change: ChangeKind; compact?: boolean; className?: string }) {
  const c = CHANGE[change];
  return (
    <span
      title={change}
      className={cn("inline-flex shrink-0 items-center justify-center gap-1 rounded-md px-1.5 py-0.5 font-mono text-[11px] font-semibold uppercase", compact && "w-5 px-0", c.cls, className)}
    >
      <span aria-hidden>{c.sign}</span>
      {compact ? <span className="sr-only">{change}</span> : change}
    </span>
  );
}

/** A person's initials in a circle, with their name for screen readers and on hover. */
export function Person({ name, muted, className }: { name: string; muted?: boolean; className?: string }) {
  const initials = name
    .split(/[\s._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0]!.toUpperCase())
    .join("");
  let hue = 0;
  for (const ch of name) hue = (hue * 31 + ch.charCodeAt(0)) % 360;
  return (
    <span
      title={name}
      className={cn("inline-flex h-6 w-6 shrink-0 select-none items-center justify-center rounded-full text-[10px] font-semibold text-white ring-2 ring-background", muted && "opacity-50", className)}
      style={{ backgroundColor: `hsl(${hue} 45% 42%)` }}
    >
      <span aria-hidden>{initials || "?"}</span>
      <span className="sr-only">{name}</span>
    </span>
  );
}

export function Empty({ children }: { children: React.ReactNode }) {
  return <p className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">{children}</p>;
}
