// Where proofs live and how they come from the established use cases (P-004), as subjects Lastro can prove: each
// export takes a model's inputs and runs the real SDK and workspace (dist/, so build it first with
// `npm run build -w lastro`) on a throwaway .lastro/ folder. The models are in .lastro/models/.
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import * as Lastro from "../dist/src/index.js";
import { openWorkspace } from "../dist/src/workspace.js";

const TOY = { name: "toy", inputs: { x: { type: "bool" } }, outputs: { y: { type: "bool" } } };
const ensuresY = { op: "eq", args: [{ var: "out.y" }, { var: "in.x" }] };
const proofOf = (id, useCase, status = "passed", extra = {}) => ({
  id, slug: id.toLowerCase().replace(/[^a-z0-9]+/g, "-"), name: id, description: `${id} holds`, useCase,
  model: "toy", subject: "toy", check: { kind: "clause", ensures: ensuresY, assumptions: [] }, status, ...extra,
});
const useCase = (id, slug, proofs = []) => ({ id, slug, name: slug, description: `What ${slug} does.`, proofs, history: [] });
const FIELDS = ["id", "slug", "name", "description", "useCase", "model", "subject", "check", "status"];

function withWorkspace(f) {
  const root = mkdtempSync(join(tmpdir(), "lastro-subject-"));
  try {
    const dir = join(root, ".lastro");
    mkdirSync(join(dir, "usecases"), { recursive: true });
    writeFileSync(join(dir, "config.json"), JSON.stringify({ models: { toy: TOY }, subjects: {}, review: { approvals: 1, submitterCanApprove: true } }));
    return f(openWorkspace(dir), dir);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}
const proofDir = (dir) => (existsSync(join(dir, "proof")) ? readdirSync(join(dir, "proof")).filter((f) => f.endsWith(".json")) : []);
const readJson = (p) => JSON.parse(readFileSync(p, "utf8"));

// P-004 I-01, I-02, I-03, I-05: one file per proof; use case files without proofs; old files move their proofs on write.
export function proofFiles({ legacy, rewrite }) {
  return withWorkspace((ws, dir) => {
    const proof = proofOf("P-001.PF-01", "UC-001", "passed", { intent: "P-001.I-01", verdict: "proved", evidence: "formal" });
    const ucFile = join(dir, "usecases", "UC-001-checkout.json");
    if (legacy) writeFileSync(ucFile, JSON.stringify({ ...useCase("UC-001", "checkout"), proofs: [proof] }));
    else {
      // The shared layout, written by hand: any SDK that follows it is read the same way.
      writeFileSync(ucFile, JSON.stringify({ id: "UC-001", slug: "checkout", name: "checkout", description: "What checkout does.", history: [] }));
      mkdirSync(join(dir, "proof"), { recursive: true });
      writeFileSync(join(dir, "proof", "P-001.PF-01-p-001-pf-01.json"), JSON.stringify(proof));
    }
    if (rewrite) ws.writeUseCase(ws.useCases()[0]);
    const loaded = ws.useCases()[0].proofs.filter((x) => x.id === proof.id).length;
    const files = proofDir(dir);
    const first = files[0] ? readJson(join(dir, "proof", files[0])) : undefined;
    return {
      loaded,
      proofFiles: files.length,
      embedded: "proofs" in readJson(ucFile),
      fileComplete: !!first && FIELDS.every((k) => k in first),
    };
  });
}

// P-004 I-04: apply writes the new passing proof's file and deletes the superseded one's, as `lastro apply` does.
export function applyProofFiles({ supersede }) {
  return withWorkspace((ws, dir) => {
    ws.writeUseCase(useCase("UC-001", "checkout", [proofOf("P-000.PF-01", "UC-001", "passed", { intent: "P-000.I-01" })]));
    const out = {
      name: "Change", slug: "change", description: "A change.", decisions: [], notPromised: [],
      intents: [{ slug: "the-change", name: "The change", description: "It changes.", change: "modify", useCase: "checkout", provability: "formal", beyondProofReason: "none", quote: "Change the checkout.", replaces: supersede ? [{ proof: "P-000.PF-01", reason: "changed" }] : [] }],
      useCases: [{ slug: "checkout", change: "modify", name: "Checkout", after: "The new checkout." }],
    };
    const before = ws.useCases();
    let p = Lastro.proposalFromExtraction({ id: "P-001", prose: "Change the checkout.", submittedBy: "ana", useCases: before, policy: { approvals: 1, submitterCanApprove: true } }, out);
    p = Lastro.approveProposal(Lastro.reviewProposal(p, "ana").approveAll().done(), before);
    p.proofs = [{ ...proofOf("P-001.PF-01", "UC-001", "passed", { intent: p.intents[0].id }), ...(supersede ? { replaces: ["P-000.PF-01"] } : {}) }];
    p.lastRun = { ok: true, outcomes: [], regressions: [], replacedStillPass: [], unproved: [] };
    let applied;
    try {
      applied = Lastro.applyProposal(p, before);
    } catch {
      return { applied: false, newFile: false, oldFile: proofDir(dir).some((f) => f.startsWith("P-000.PF-01-")) };
    }
    for (const id of applied.written) ws.writeUseCase(applied.useCases.find((u) => u.id === id));
    const files = proofDir(dir);
    return { applied: true, newFile: files.some((f) => f.startsWith("P-001.PF-01-")), oldFile: files.some((f) => f.startsWith("P-000.PF-01-")) };
  });
}

// P-004 I-06..I-09: which use cases derive --usecases works on, how their proofs are numbered, and what it refuses.
export function usecaseDerive({ hasPassed, hasPending, explicit, listed, expressible }) {
  const proofs = [...(hasPassed ? [proofOf("P-000.PF-01", "UC-001")] : []), ...(hasPending ? [proofOf("UC-001.PF-01", "UC-001", "pending")] : [])];
  const cases = [useCase("UC-001", "checkout", proofs), useCase("UC-002", "coupons")];
  const ids = explicit ? (listed ? ["UC-001"] : ["UC-002"]) : undefined;
  let targets;
  try {
    targets = Lastro.useCaseTargets(cases, ids);
  } catch {
    return { refused: true, derived: false, unprovable: false, number: 0, needsProposal: false };
  }
  // The derive step's answer, as an LLM would give it: a proof per target, or (for UC-001) unprovable.
  const answer = {
    proofs: targets.filter((u) => u.id !== "UC-001" || expressible).map((u) => ({ useCase: u.id, slug: "it-holds", name: "It holds", description: "It holds.", model: "toy", subject: "toy", kind: "clause", requires: "", ensures: JSON.stringify(ensuresY), assumptions: [], inputJson: "", expect: "" })),
    unprovable: targets.filter((u) => u.id === "UC-001" && !expressible).map((u) => ({ useCase: u.id, reason: "no model expresses it" })),
  };
  const d = Lastro.proofsFromUseCases(cases, targets.map((u) => u.id), { models: { toy: TOY }, subjects: ["toy"] }, answer);
  const mine = d.derived.find((x) => x.useCase === "UC-001");
  return {
    refused: false,
    derived: !!mine,
    unprovable: d.unprovable.some((x) => x.useCase === "UC-001"),
    number: mine ? Number(mine.id.split(".PF-")[1]) : 0,
    // Proofs derived from use cases carry no intent and need no proposal.
    needsProposal: !!mine?.intent,
  };
}

// P-004 I-10, I-11: proving a use case's proof against code that does (codeOk) or does not do what it states.
export async function usecaseProve({ status, codeOk }) {
  const cases = [useCase("UC-001", "checkout", [proofOf("UC-001.PF-01", "UC-001", status)])];
  const code = ({ x }) => ({ y: codeOk ? x : !x });
  const run1 = await Lastro.proveUseCases(cases, { subjects: { toy: code }, models: { toy: TOY } });
  const after = cases.map((u) => run1.useCases.find((c) => c.id === u.id) ?? u);
  const proof = after[0].proofs[0];
  const regress = await Lastro.regress(after, { subjects: { toy: code }, models: { toy: TOY } });
  return {
    status: proof.status,
    regression: regress.outcomes.some((x) => x.proof === proof.id),
    feedback: "feedback" in run1.run,
    note: !!run1.run.note,
    ok: run1.run.ok,
  };
}
