import { countedApprovals, type Proposal, type ProposalIntent, type ProofEntry, type UseCase } from "@lastro/sdk";

export const need = (p: Proposal) => Math.max(1, p.policy.approvals);

/** Whether this reviewer may approve under the proposal's policy (the SDK refuses otherwise). */
export function mayApprove(p: Proposal, reviewer: string): boolean {
  return !!reviewer && (p.policy.submitterCanApprove || reviewer !== p.submittedBy);
}

/** An intent is settled once it is rejected, or approved or amended with enough counted approvals. */
export function settled(p: Proposal, it: ProposalIntent): boolean {
  return it.status === "rejected" || countedApprovals(p, it).length >= need(p);
}

/** The intent still waits for this reviewer's approval. */
export function waitsFor(p: Proposal, it: ProposalIntent, reviewer: string): boolean {
  return !settled(p, it) && mayApprove(p, reviewer) && !it.approvals.includes(reviewer);
}

export const reviewOpen = (p: Proposal) => p.status === "open" || p.status === "changes-requested";

/** Every proof in the workspace by id: regression proofs of use cases, then the proposal's own. */
export function proofIndex(useCases: UseCase[], proposals: Proposal[] = []): Map<string, ProofEntry> {
  const m = new Map<string, ProofEntry>();
  for (const u of useCases) for (const pf of u.proofs) m.set(pf.id, pf);
  for (const p of proposals) for (const pf of p.proofs) if (!m.has(pf.id)) m.set(pf.id, pf);
  return m;
}

/** The short id of an intent inside its proposal: "P-001.I-07" → "I-07". */
export const shortId = (id: string) => id.replace(/^P-\d+\./, "");

export function counts(p: Proposal) {
  const c = { proposed: 0, approved: 0, amended: 0, rejected: 0, settled: 0 };
  for (const it of p.intents) {
    c[it.status]++;
    if (settled(p, it)) c.settled++;
  }
  return c;
}
