/**
 * Proofs derived straight from the established use cases (`lastro derive --usecases`).
 *
 * A use case's description was already reviewed: it is the behavior the code has today. These proofs need no
 * proposal and no intents. They are numbered inside their use case (`UC-003.PF-01`), start pending, and become
 * regression tests once they pass against the code (`lastro prove --usecases`). A failing one goes to the user:
 * the code is the reference, so either the description is wrong (a proposal fixes it) or the code has a bug.
 */
import { checkFromAnswer, pad, passedProofs, ProposalError, runProofs, type ProofEntry, type ProofOutcome, type ProofSetup, type RunOptions, type UseCase } from "./proposal.js";

const SLUG = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const UC_ID = /^UC-\d+$/;

/** The derive step's answer for use cases: proofs per use case, and the use cases the models cannot express. */
export interface UseCaseDeriveOut {
  proofs: {
    useCase: string;
    slug: string;
    name: string;
    description: string;
    model: string;
    subject: string;
    kind: "clause" | "scenario";
    requires: string;
    ensures: string;
    assumptions: string[];
    inputJson: string;
    expect: string;
  }[];
  unprovable: { useCase: string; reason: string }[];
}

export interface UseCaseDerivation {
  /** The target use cases with their new pending proofs (their earlier pending or failed ones are replaced). */
  useCases: UseCase[];
  /** The new proofs, in order. */
  derived: ProofEntry[];
  /** Target use cases the models cannot express, with the reason. */
  unprovable: { useCase: string; reason: string }[];
}

/** Printed with every failing use-case proof: it never goes back to the code. */
export const USECASE_FEEDBACK_NOTE =
  "These proofs come from the use cases' own descriptions, and the code is the reference: a failing one means the description is wrong (change it with a proposal) or the code has a bug the user decides about. Do not change the code to make it pass.";

/**
 * The use cases `derive --usecases` works on: without ids, every use case with no passed proof; with ids, those
 * use cases, refusing any that already has passed proofs (its proofs change only through a proposal).
 */
export function useCaseTargets(useCases: UseCase[], ids?: string[]): UseCase[] {
  if (!ids?.length) return useCases.filter((u) => !passedProofs(u).length);
  const problems: string[] = [];
  const out: UseCase[] = [];
  for (const id of ids) {
    const u = useCases.find((x) => x.id === id || x.slug === id);
    if (!u) problems.push(`no use case ${id}`);
    else if (passedProofs(u).length) problems.push(`${u.id} already has proofs; they change only through a proposal`);
    else if (!out.includes(u)) out.push(u);
  }
  if (problems.length) throw new ProposalError(problems);
  return out;
}

/** The next proof number inside a use case: after every `UC-nnn.PF-nn` id it has had. */
function nextNumber(u: UseCase): number {
  const nums = u.proofs.map((x) => x.id.match(new RegExp(`^${u.id}\\.PF-(\\d+)$`))?.[1]).filter((x): x is string => !!x).map(Number);
  return (nums.length ? Math.max(...nums) : 0) + 1;
}

/**
 * Check the derive step's answer for `targets` and attach the pending proofs. Every target gets at least one proof
 * or is listed as unprovable; a proof or an unprovable entry for any other use case is refused, with every problem
 * listed. A target's earlier pending or failed proofs are replaced; numbering continues after them.
 */
export function proofsFromUseCases(useCases: UseCase[], targets: string[], setup: ProofSetup, out: UseCaseDeriveOut): UseCaseDerivation {
  const problems: string[] = [];
  const wanted = new Set(targets);
  const byId = new Map(useCases.filter((u) => wanted.has(u.id)).map((u) => [u.id, u]));
  for (const t of targets) if (!byId.has(t)) problems.push(`no use case ${t}`);
  const next = new Map([...byId.values()].map((u) => [u.id, nextNumber(u)]));
  const slugs = new Map<string, Set<string>>();
  const derived: ProofEntry[] = [];
  for (const d of out.proofs ?? []) {
    const u = byId.get(d.useCase);
    if (!u) {
      problems.push(`proof "${d.slug}": ${d.useCase} is not one of the use cases to prove (${targets.join(", ") || "none"})`);
      continue;
    }
    const pid = `${u.id}.PF-${pad(next.get(u.id)!, 2)}`;
    next.set(u.id, next.get(u.id)! + 1);
    if (!SLUG.test(d.slug ?? "")) problems.push(`${pid}: slug "${d.slug}" is not kebab-case`);
    const seen = slugs.get(u.id) ?? new Set<string>();
    if (seen.has(d.slug)) problems.push(`${pid}: duplicate proof slug "${d.slug}"`);
    seen.add(d.slug);
    slugs.set(u.id, seen);
    const model = setup.models[d.model];
    if (!model) problems.push(`${pid}: unknown model "${d.model}" (declared: ${Object.keys(setup.models).join(", ") || "none"})`);
    if (!setup.subjects.includes(d.subject)) problems.push(`${pid}: unknown subject "${d.subject}" (declared: ${setup.subjects.join(", ") || "none"})`);
    if (!model) continue;
    const check = checkFromAnswer(pid, d, model, problems);
    if (check) derived.push({ id: pid, slug: d.slug, name: d.name, description: d.description, useCase: u.id, model: d.model, subject: d.subject, check, status: "pending" });
  }
  const unprovable: { useCase: string; reason: string }[] = [];
  for (const x of out.unprovable ?? []) {
    if (!byId.has(x.useCase)) problems.push(`unprovable ${x.useCase}: not one of the use cases to prove`);
    else if (!String(x.reason ?? "").trim()) problems.push(`unprovable ${x.useCase}: needs a reason`);
    else unprovable.push({ useCase: x.useCase, reason: x.reason });
  }
  for (const t of byId.keys()) {
    const has = (out.proofs ?? []).some((d) => d.useCase === t);
    const listed = unprovable.some((x) => x.useCase === t);
    if (!has && !listed) problems.push(`${t}: no proof derived and not listed as unprovable`);
    if (has && listed) problems.push(`${t}: has proofs and is also listed as unprovable`);
  }
  if (problems.length) throw new ProposalError(problems);
  const updated = [...byId.values()].map((u) => {
    const v = structuredClone(u);
    const mine = derived.filter((x) => x.useCase === u.id);
    if (mine.length || unprovable.some((x) => x.useCase === u.id)) v.proofs = [...passedProofs(v), ...structuredClone(mine)];
    return v;
  });
  return { useCases: updated, derived, unprovable };
}

export interface UseCaseRun {
  ok: boolean;
  outcomes: ProofOutcome[];
  /** Use-case proofs (pending or failed before the run) that failed: they go to the user, never to the code. */
  failed: string[];
  /** Passed proofs that failed: regressions. */
  regressions: string[];
  /** Use cases that still have no passed proof. */
  unproved: string[];
  enginesAgree?: boolean;
  /** Set when a use-case proof failed. */
  note?: string;
}

/**
 * Run every use case's pending and failed proofs (role "usecase") together with every passed proof (role
 * "regression"). A use-case proof that passes becomes passed, and from then on is a regression test; one that fails
 * becomes failed. Returns the use cases whose proofs changed.
 */
export async function proveUseCases(useCases: UseCase[], o: RunOptions): Promise<{ useCases: UseCase[]; run: UseCaseRun }> {
  const proofs = useCases.flatMap((u) => u.proofs.map((proof) => ({ proof, role: proof.status === "passed" ? ("regression" as const) : ("usecase" as const) })));
  const { outcomes, enginesAgree } = await runProofs(proofs, o);
  const byProof = new Map(outcomes.map((x) => [x.proof, x]));
  const changed: UseCase[] = [];
  for (const u of useCases) {
    if (!u.proofs.some((x) => x.status !== "passed")) continue;
    const v = structuredClone(u);
    for (const pr of v.proofs) {
      if (pr.status === "passed") continue;
      const r = byProof.get(pr.id);
      if (!r) continue;
      pr.status = r.passed ? "passed" : "failed";
      pr.verdict = r.verdict;
      pr.evidence = r.verdict === "proved" ? "formal" : r.verdict === "not-run" ? "none" : "empirical";
    }
    changed.push(v);
  }
  const failed = outcomes.filter((x) => x.role === "usecase" && !x.passed).map((x) => x.proof);
  const regressions = outcomes.filter((x) => x.role === "regression" && !x.passed).map((x) => x.proof);
  const after = useCases.map((u) => changed.find((c) => c.id === u.id) ?? u);
  const run: UseCaseRun = {
    ok: !failed.length && !regressions.length && enginesAgree !== false,
    outcomes,
    failed,
    regressions,
    unproved: after.filter((u) => !passedProofs(u).length).map((u) => u.id),
    ...(enginesAgree !== undefined ? { enginesAgree } : {}),
    ...(failed.length ? { note: USECASE_FEEDBACK_NOTE } : {}),
  };
  return { useCases: changed, run };
}

/** True when `ids` is a comma-separated list of use case ids (`UC-003,UC-016`). */
export const isUseCaseList = (s: string | undefined): s is string => !!s && s.split(",").every((x) => UC_ID.test(x.trim()));
