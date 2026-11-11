// The decision cores of proposal/v1 as subjects Lastro can prove: each export takes a model's inputs and runs the real
// SDK (dist/, so build it first with `npm run build -w lastro`) to produce the model's outputs. The models are in
// .lastro/models/; the proofs of P-001 name them and these subjects.
import * as Lastro from "../dist/src/index.js";

const PROSE = "Add the shipping rule. Change the checkout. Remove the coupons.";
const proofOf = (id, useCase, ensures = { op: "eq", args: [{ var: "out.y" }, { const: true }] }) => ({
  id, slug: id.toLowerCase().replace(/[^a-z0-9]+/g, "-"), name: id, description: `${id} holds`, intent: "P-000.I-01", useCase,
  model: "toy", subject: "toy", check: { kind: "clause", ensures, assumptions: [] }, status: "passed",
});
const useCase = (id, slug, proofs = []) => ({ id, slug, name: slug, description: `What ${slug} does.`, proofs, history: [] });
const intentOut = (o) => ({
  slug: o.slug ?? "the-change", name: "The change", description: o.description ?? "The change happens.", change: o.change ?? "add",
  useCase: o.useCase ?? "new-rule", provability: "formal", beyondProofReason: "none", quote: o.quote ?? "Add the shipping rule.", replaces: o.replaces ?? [],
});
const extraction = (intents, useCases) => ({ name: "Change", slug: "change", description: "A change.", intents, useCases, decisions: [], notPromised: [] });
const extract = (useCases, out, policy = { approvals: 1, submitterCanApprove: true }, submittedBy = "ana") =>
  Lastro.proposalFromExtraction({ id: "P-001", prose: PROSE, submittedBy, useCases, policy }, out);
const refused = (f) => {
  try {
    return [false, f()];
  } catch (e) {
    if (e instanceof Lastro.ProposalError || e instanceof Error) return [true, undefined];
    throw e;
  }
};
/** One add proposal, fully approved by `by`, with no decisions or threads. */
const approvedAdd = (useCases = [], by = "ana") => {
  const p = Lastro.reviewProposal(extract(useCases, extraction([intentOut({})], [{ slug: "new-rule", change: "add", name: "New rule", after: "The new rule." }])), by).approveAll().done();
  return Lastro.approveProposal(p, useCases);
};

// I-03: ids take the next free number after every id in use, including the ones other open proposals reserved.
export function nextUseCaseId({ a, b, reserved, hasReserved }) {
  const pad = (n) => `UC-${String(n).padStart(3, "0")}`;
  const ids = [a, b].filter((n) => n > 0).map(pad);
  const p = extract([], extraction([intentOut({})], [{ slug: "new-rule", change: "add", name: "New rule", after: "The new rule." }]));
  const withIds = Lastro.proposalFromExtraction(
    { id: "P-001", prose: PROSE, submittedBy: "ana", useCases: ids.map((id, k) => useCase(id, `case-${k}`)), policy: p.policy, reserved: hasReserved && reserved > 0 ? [pad(reserved)] : [] },
    extraction([intentOut({})], [{ slug: "new-rule", change: "add", name: "New rule", after: "The new rule." }]),
  );
  return { next: Number(withIds.useCases[0].useCase.slice(3)) };
}

// I-04: slugs are kebab-case.
export function slugCheck({ slug }) {
  const [rejected] = refused(() => extract([], extraction([intentOut({ slug })], [{ slug: "new-rule", change: "add", name: "New rule", after: "The new rule." }])));
  return { accepted: !rejected };
}

// I-08, I-10, I-12, I-13: what extraction accepts, and what the SDK fills in itself.
export function extractionRules({ change, known, replaces }) {
  const proofs = [proofOf("P-000.PF-01", "UC-001"), proofOf("P-000.PF-02", "UC-001")];
  const cases = [useCase("UC-001", "checkout", proofs), useCase("UC-002", "coupons", [proofOf("P-000.PF-03", "UC-002")])];
  const target = known ? "checkout" : "unknown-case";
  const proof = { none: null, own: "P-000.PF-01", other: "P-000.PF-03", unknown: "P-000.PF-99" }[replaces];
  const out = extraction(
    [intentOut({ change, useCase: target, replaces: proof ? [{ proof, reason: "it changes" }] : [] })],
    [{ slug: target, change, name: "Checkout", after: change === "remove" ? "" : "The new checkout." }],
  );
  const [rejected, p] = refused(() => extract(cases, out));
  const it = p?.intents[0];
  const c = p?.useCases[0];
  return {
    accepted: !rejected,
    replacesAll: !!it && it.replaces.length === 2 && it.replaces.every((r) => proofs.some((x) => x.id === r.proof)),
    beforeFromUseCase: !!c && c.before === cases[0].description,
  };
}

// I-15: amending or rejecting an intent makes its use case's after stale.
export function staleAfter({ action, sibling }) {
  const intents = [intentOut({ slug: "first", description: "First part." })];
  if (sibling) intents.push(intentOut({ slug: "second", description: "Second part.", quote: "Change the checkout." }));
  let r = Lastro.reviewProposal(extract([], extraction(intents, [{ slug: "new-rule", change: "add", name: "New rule", after: "The new rule." }])), "ana");
  if (action === "approve") r = r.approve("I-01");
  else if (action === "amend") r = r.amend("I-01", "First part, amended.");
  else r = r.reject("I-01", "not now");
  return { stale: r.done().useCases[0].stale === true };
}

// I-17: a thread is resolved by whoever started it or by the submitter.
export function threadResolve({ by }) {
  const p = Lastro.reviewProposal(extract([], extraction([intentOut({})], [{ slug: "new-rule", change: "add", name: "New rule", after: "The new rule." }])), "bruno")
    .comment("I-01", "Why?").done();
  const who = { commenter: "bruno", submitter: "ana", other: "carla" }[by];
  const [rejected, q] = refused(() => Lastro.reviewProposal(p, who).resolve("T1").done());
  return { resolved: !rejected && q.threads[0].resolved === true };
}

// I-18, I-21, I-23: which approvals count and when a proposal is ready to approve.
export function approvalPolicy({ required, submitterCanApprove, submitterSigned, others, status, answered, threadOpen }) {
  const out = extraction([intentOut({})], [{ slug: "new-rule", change: "add", name: "New rule", after: "The new rule." }]);
  out.decisions = [{ slug: "which", name: "Which", question: "Which one?", options: [{ label: "a", consequence: "a" }, { label: "b", consequence: "b" }], recommended: 0, intents: [] }];
  const p = extract([], out, { approvals: required, submitterCanApprove });
  const it = p.intents[0];
  it.status = status;
  it.approvals = [...(submitterSigned ? ["ana"] : []), ...["bruno", "carla", "davi"].slice(0, others)];
  if (status === "amended") it.originalDescription = it.description;
  if (answered) Object.assign(p.decisions[0], { chosen: 0, answeredBy: "bruno" });
  if (threadOpen) p.threads.push({ id: "T1", on: it.id, by: "bruno", comments: [{ by: "bruno", text: "?" }], resolved: false });
  return { counted: Lastro.countedApprovals(p, it).length, ready: Lastro.pendingProposal(p).length === 0 };
}

// I-20: the policy read from .lastro/config.json, with its defaults and the legacy key.
export function configPolicy({ hasReview, reviewApprovals, hasLegacy, legacy, hasFlag, flag }) {
  const review = hasReview ? { approvals: reviewApprovals, ...(hasFlag ? { submitterCanApprove: flag } : {}) } : hasFlag ? { submitterCanApprove: flag } : undefined;
  const p = Lastro.readPolicy({ ...(review ? { review } : {}), ...(hasLegacy ? { requiredApprovals: legacy } : {}) });
  return { approvals: p.approvals, submitterCanApprove: p.submitterCanApprove };
}

// I-22: a proposal is judged by the policy it was extracted under, not by a later config.
export function recordedPolicy({ atExtraction, later, signatures }) {
  let p = extract([], extraction([intentOut({})], [{ slug: "new-rule", change: "add", name: "New rule", after: "The new rule." }]), { approvals: atExtraction, submitterCanApprove: true });
  for (const who of ["ana", "bruno", "carla"].slice(0, signatures)) p = Lastro.reviewProposal(p, who).approveAll().done();
  void later; // the config changes after extraction; nothing reads it again
  return { ready: Lastro.pendingProposal(p).length === 0 };
}

// I-19: the status that follows from a proposal's content.
export function proposalStatus({ approved, proofs, run, allPassed, stale, raised, terminal }) {
  let p = approved ? approvedAdd() : extract([], extraction([intentOut({})], [{ slug: "new-rule", change: "add", name: "New rule", after: "The new rule." }]));
  if (!approved) {
    if (stale) p.useCases[0].stale = true;
    if (raised) p.decisions.push({ id: "D1", slug: "raised", name: "Raised", origin: "derivation", question: "Which?", options: [{ label: "a", consequence: "a" }, { label: "b", consequence: "b" }], recommended: 0, intents: [] });
  }
  if (proofs) p.proofs = [{ ...proofOf("P-001.PF-01", "UC-001"), status: allPassed ? "passed" : "failed" }];
  if (run !== "none") p.lastRun = { ok: run === "ok", outcomes: [], regressions: [], replacedStillPass: [], unproved: [] };
  if (terminal !== "none") p.status = terminal;
  return { status: Lastro.withStatus(p).status };
}

// I-11, I-24: the baseline binds the approval to the use cases' text.
export function baselineBinding({ change }) {
  const cases = [useCase("UC-001", "checkout", [proofOf("P-000.PF-01", "UC-001")])];
  const reviewed = Lastro.reviewProposal(extract(cases, extraction([intentOut({})], [{ slug: "new-rule", change: "add", name: "New rule", after: "The new rule." }])), "ana").approveAll().done();
  const now = structuredClone(cases);
  if (change === "description") now[0].description = "Something else.";
  if (change === "name") now[0].name = "Renamed";
  if (change === "slug") now[0].slug = "check-out";
  if (change === "proofs") now[0].proofs = [];
  if (change === "added") now.push(useCase("UC-002", "coupons"));
  const [rejected] = refused(() => Lastro.approveProposal(reviewed, now));
  return { approves: !rejected, sameBaseline: Lastro.baselineOf(now) === reviewed.baseline };
}

// I-25: re-extraction keeps the review of an intent whose change, use case and description are the same.
export function carryReview({ sameChange, sameUseCase, wording, reviewed }) {
  const cases = [useCase("UC-001", "checkout"), useCase("UC-002", "coupons")];
  const first = extract(cases, extraction([intentOut({ change: "modify", useCase: "checkout", description: "The fee shows at checkout." })], [{ slug: "checkout", change: "modify", name: "Checkout", after: "New." }]));
  const previous = reviewed ? Lastro.reviewProposal(first, "bruno").approveAll().done() : first;
  const description = { same: "The fee shows at checkout.", case: "THE FEE shows at checkout.", spacing: "The  fee shows\nat checkout.", different: "The fee hides at checkout." }[wording];
  const ucSlug = sameUseCase ? "checkout" : "coupons";
  const change = sameChange ? "modify" : "remove";
  const next = extract(cases, extraction([intentOut({ change, useCase: ucSlug, description })], [{ slug: ucSlug, change, name: "Checkout", after: change === "remove" ? "" : "New." }]));
  const carried = Lastro.carryProposalReview(previous, next).intents[0];
  return { carried: carried.approvals.includes("bruno") };
}

// I-28: supersedes is the union of replaces over the approved and amended intents.
export function supersedes({ first, second }) {
  const cases = [useCase("UC-001", "checkout", [proofOf("P-000.PF-01", "UC-001"), proofOf("P-000.PF-02", "UC-001")])];
  const p = extract(cases, extraction(
    [
      intentOut({ slug: "one", change: "modify", useCase: "checkout", description: "One.", replaces: [{ proof: "P-000.PF-01", reason: "r" }] }),
      intentOut({ slug: "two", change: "modify", useCase: "checkout", description: "Two.", quote: "Change the checkout.", replaces: [{ proof: "P-000.PF-02", reason: "r" }] }),
    ],
    [{ slug: "checkout", change: "modify", name: "Checkout", after: "New." }],
  ));
  [first, second].forEach((s, k) => Object.assign(p.intents[k], { status: s, approvals: s === "approved" || s === "amended" ? ["ana"] : [] }));
  const sup = Lastro.supersedesOf(p);
  return { first: sup.includes("P-000.PF-01"), second: sup.includes("P-000.PF-02") };
}

// I-30, I-31, I-32, I-33: what a proving run and a regression run report.
const TOY = { name: "toy", inputs: { x: { type: "bool" } }, outputs: { y: { type: "bool" } } };
const holds = (passes) => ({ op: "eq", args: [{ var: "out.y" }, { const: passes }] });
export async function proveRun({ proposalPasses, regressionPasses, supersededPasses, emptyUseCase }) {
  const cases = [useCase("UC-001", "checkout", [proofOf("P-000.PF-01", "UC-001", holds(regressionPasses)), proofOf("P-000.PF-02", "UC-001", holds(supersededPasses))])];
  if (emptyUseCase) cases.push(useCase("UC-002", "coupons"));
  let p = extract(cases, extraction(
    [intentOut({ change: "modify", useCase: "checkout", description: "Checkout changes.", replaces: [{ proof: "P-000.PF-02", reason: "changed" }] })],
    [{ slug: "checkout", change: "modify", name: "Checkout", after: "New." }],
  ));
  p = Lastro.approveProposal(Lastro.reviewProposal(p, "ana").approveAll().done(), cases);
  p.proofs = [{ ...proofOf("P-001.PF-01", "UC-001", holds(proposalPasses)), intent: p.intents[0].id, status: "pending" }];
  const o = { subjects: { toy: () => ({ y: true }) }, models: { toy: TOY } };
  const { run } = await Lastro.proveProposal(p, cases, o);
  const alone = await Lastro.regress(cases, o);
  return {
    ok: run.ok,
    regression: run.regressions.length > 0,
    stillPassNoted: run.replacedStillPass.includes("P-000.PF-02"),
    unproved: run.unproved.includes("UC-002"),
    regressOk: alone.ok,
  };
}

// I-34, I-35, I-36: when apply goes through and what it does to the use cases.
export async function applyRules({ approved, sameBaseline, run, change }) {
  const cases = [useCase("UC-001", "checkout", [proofOf("P-000.PF-01", "UC-001"), proofOf("P-000.PF-02", "UC-001")])];
  const isAdd = change === "add";
  let p = extract(cases, extraction(
    [intentOut({ change, useCase: isAdd ? "new-rule" : "checkout", description: "It changes.", replaces: change === "modify" ? [{ proof: "P-000.PF-02", reason: "changed" }] : [] })],
    [{ slug: isAdd ? "new-rule" : "checkout", change, name: "Changed", after: change === "remove" ? "" : "New." }],
  ));
  p = Lastro.reviewProposal(p, "ana").approveAll().done();
  if (approved) p = Lastro.approveProposal(p, cases);
  const target = isAdd ? p.useCases[0].useCase : "UC-001";
  p.proofs = [{ ...proofOf("P-001.PF-01", target), intent: p.intents[0].id, status: run === "passed" ? "passed" : run === "failed" ? "failed" : "pending" }];
  if (run !== "none") p.lastRun = { ok: run === "passed", outcomes: [], regressions: [], replacedStillPass: [], unproved: [] };
  const now = sameBaseline ? cases : [...cases, useCase("UC-009", "later")];
  const [rejected, applied] = refused(() => Lastro.applyProposal(p, now));
  const after = applied?.useCases.find((u) => u.id === target);
  return {
    applied: !rejected,
    proofs: after ? after.proofs.length : 0,
    history: !!after && after.history.includes("P-001"),
    removed: !!applied && applied.removed.includes("UC-001"),
  };
}
