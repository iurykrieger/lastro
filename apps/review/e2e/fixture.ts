/**
 * A small `.lastro/` workspace for the UI tests: a shipping app with three use cases, one applied proposal
 * that gave UC-001 its regression proof, and one open proposal (P-002) under a two-reviewer policy where
 * the submitter (ana) does not count. Every test starts from a fresh copy.
 */
import { mkdirSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { L, approveProposal, baselineOf, withStatus, type ProofEntry, type ProofOutcome, type Proposal, type UseCase } from "../../../sdks/typescript/src/index.ts";
import type { ProofRunRecord } from "../src/lib/api-types.ts";
import { writeJson } from "../../../sdks/typescript/src/workspace.ts";

export const WORK_DIR = join(dirname(fileURLToPath(import.meta.url)), ".work", ".lastro");

const PROSE = `# Raise the free shipping threshold

Free shipping now starts at 150 instead of 100, because margins on small orders went negative.
Express shipping is never free, whatever the total.

The checkout shows an estimate of the shipping fee before the customer pays.

We have not decided whether the estimate rounds to whole units.`;

const quote = (q: string) => {
  const start = PROSE.indexOf(q);
  if (start < 0) throw new Error(`quote not in prose: ${q}`);
  return { quote: q, start, end: start + q.length };
};

export function useCases(): UseCase[] {
  return [
    {
      id: "UC-001",
      slug: "free-shipping",
      name: "Free shipping over a threshold",
      description: "Orders whose total is over 100 ship for free; any other order pays the flat fee of 9.90.",
      proofs: [
        {
          id: "P-001.PF-01",
          slug: "free-over-100",
          name: "Free over 100",
          description: "For every total over 100 the shipping fee is 0.",
          intent: "P-001.I-01",
          useCase: "UC-001",
          model: "shipping",
          subject: "shipping",
          check: { kind: "clause", requires: L.gt(L.in("total"), 100), ensures: L.eq(L.out("fee"), 0), assumptions: [] },
          status: "passed",
          verdict: "proved",
        },
      ],
      history: ["P-001"],
    },
    {
      id: "UC-002",
      slug: "checkout-total",
      name: "Checkout total",
      description: "The checkout total is the sum of the items plus the shipping fee.",
      proofs: [],
      history: [],
    },
  ];
}

export function proposals(): Proposal[] {
  const ucs = useCases();
  const applied: Proposal = {
    lastro: "proposal/v1",
    id: "P-001",
    slug: "free-shipping",
    name: "Free shipping",
    description: "Orders over 100 ship for free.",
    status: "applied",
    submittedBy: "ana",
    prose: "Orders over 100 ship for free.",
    baseline: "sha256:0",
    intents: [],
    useCases: [],
    decisions: [],
    threads: [],
    notPromised: [],
    policy: { approvals: 1, submitterCanApprove: true },
    proofs: [],
    supersedes: [],
  };
  const open: Proposal = {
    lastro: "proposal/v1",
    id: "P-002",
    slug: "raise-free-shipping-threshold",
    name: "Raise the free shipping threshold",
    description: "Free shipping starts at 150, express is never free, and the checkout shows a shipping estimate.",
    status: "open",
    submittedBy: "ana",
    prose: PROSE,
    baseline: baselineOf(ucs),
    intents: [
      {
        id: "P-002.I-01",
        slug: "threshold-150",
        name: "Threshold at 150",
        description: "Orders whose total is over 150 ship for free; orders up to 150 pay the flat fee.",
        change: "modify",
        useCase: "UC-001",
        provability: "formal",
        source: quote("Free shipping now starts at 150 instead of 100"),
        replaces: [{ proof: "P-001.PF-01", reason: "orders between 100 and 150 now pay the fee" }],
        status: "proposed",
        approvals: [],
      },
      {
        id: "P-002.I-02",
        slug: "express-never-free",
        name: "Express is never free",
        description: "Express shipping always pays its fee, whatever the order total.",
        change: "modify",
        useCase: "UC-001",
        provability: "formal",
        source: quote("Express shipping is never free, whatever the total."),
        replaces: [],
        status: "proposed",
        approvals: [],
      },
      {
        id: "P-002.I-03",
        slug: "shipping-estimate",
        name: "Shipping estimate at checkout",
        description: "Before payment, the checkout shows the shipping fee the order will pay.",
        change: "add",
        useCase: "UC-003",
        provability: "empirical",
        source: quote("The checkout shows an estimate of the shipping fee before the customer pays."),
        replaces: [],
        status: "proposed",
        approvals: [],
      },
    ],
    useCases: [
      {
        useCase: "UC-001",
        slug: "free-shipping",
        change: "modify",
        name: "Free shipping over a threshold",
        before: ucs[0]!.description,
        after: "Standard orders whose total is over 150 ship for free; any other standard order pays the flat fee of 9.90. Express shipping always pays its fee.",
        intents: ["P-002.I-01", "P-002.I-02"],
      },
      {
        useCase: "UC-003",
        slug: "shipping-estimate",
        change: "add",
        name: "Shipping estimate at checkout",
        after: "Before payment, the checkout shows the shipping fee the order will pay, computed by the same rule as the charge.",
        intents: ["P-002.I-03"],
      },
    ],
    decisions: [
      {
        id: "D1",
        slug: "estimate-rounding",
        name: "Estimate rounding",
        origin: "extraction",
        question: "Does the shipping estimate round to whole units?",
        options: [
          { label: "Show cents", consequence: "The estimate equals the fee that will be charged." },
          { label: "Round up", consequence: "The estimate may be higher than the charge by less than one unit." },
        ],
        recommended: 0,
        intents: ["P-002.I-03"],
      },
    ],
    threads: [{ id: "T1", on: "prose", by: "bruno", comments: [{ by: "bruno", text: "Is 150 before or after discounts?" }], resolved: false }],
    notPromised: ["Changing the flat fee of 9.90."],
    policy: { approvals: 2, submitterCanApprove: false },
    proofs: [],
    supersedes: [],
  };
  return [applied, open, proving()];
}

const DISCOUNT_PROSE = "A discount code takes 10% off the items. Expired codes are refused.";
const at = (prose: string, q: string) => ({ quote: q, start: prose.indexOf(q), end: prose.indexOf(q) + q.length });

function pf(n: number, intent: string, name: string, description: string, check: ProofEntry["check"]): ProofEntry {
  return { id: `P-003.PF-0${n}`, slug: name.toLowerCase().replace(/[^a-z0-9]+/g, "-"), name, description, intent, useCase: "UC-004", model: "checkout", subject: "checkout", check, status: "pending" };
}

/** P-003: approved, with derived proofs and five `lastro prove` runs while the code was written. */
export function proving(): Proposal {
  const ucs = useCases();
  const draft: Proposal = {
    lastro: "proposal/v1",
    id: "P-003",
    slug: "discount-codes",
    name: "Discount codes",
    description: "A discount code takes 10% off the items; expired codes are refused.",
    status: "open",
    submittedBy: "bruno",
    prose: DISCOUNT_PROSE,
    baseline: baselineOf(ucs),
    intents: [
      { id: "P-003.I-01", slug: "ten-percent-off", name: "Ten percent off", description: "A valid code takes 10% off the items, never the shipping fee.", change: "add", useCase: "UC-004", provability: "formal", source: at(DISCOUNT_PROSE, "A discount code takes 10% off the items."), replaces: [], status: "approved", approvals: ["ana"] },
      { id: "P-003.I-02", slug: "expired-refused", name: "Expired codes are refused", description: "An expired code changes nothing and is reported as expired.", change: "add", useCase: "UC-004", provability: "formal", source: at(DISCOUNT_PROSE, "Expired codes are refused."), replaces: [], status: "approved", approvals: ["ana"] },
    ],
    useCases: [{ useCase: "UC-004", slug: "discount-codes", change: "add", name: "Discount codes", after: "A valid discount code takes 10% off the items, never the shipping fee; an expired code changes nothing and is reported as expired.", intents: ["P-003.I-01", "P-003.I-02"] }],
    decisions: [],
    threads: [],
    notPromised: [],
    policy: { approvals: 1, submitterCanApprove: true },
    proofs: [],
    supersedes: [],
  };
  const p = approveProposal(draft, ucs);
  p.proofs = [
    pf(1, "P-003.I-01", "Ten percent off the items", "With a valid code, the discount is 10% of the items' total.", { kind: "clause", requires: L.eq(L.in("code"), "valid"), ensures: L.eq(L.out("discount"), L.v("tenPercent")), assumptions: [] }),
    pf(2, "P-003.I-01", "Shipping is not discounted", "With any code, the shipping fee is the same as without one.", { kind: "clause", ensures: L.eq(L.out("fee"), L.v("feeWithoutCode")), assumptions: [] }),
    pf(3, "P-003.I-02", "Expired codes change nothing", "With an expired code, the discount is 0 and the reason is expired.", { kind: "clause", requires: L.eq(L.in("code"), "expired"), ensures: L.and(L.eq(L.out("discount"), 0), L.eq(L.out("reason"), "expired")), assumptions: [] }),
  ];
  return withStatus(p);
}

const o = (proof: string, intent: string, verdict: ProofOutcome["verdict"], extra: Partial<ProofOutcome> = {}): ProofOutcome => ({
  proof,
  useCase: proof.startsWith("P-001") ? "UC-001" : "UC-004",
  intent,
  role: proof.startsWith("P-001") ? "regression" : "proposal",
  verdict,
  passed: verdict === "proved" || verdict === "evidenced",
  ...extra,
});

/** Five runs, 12 minutes apart: I-01 passes from run 3, a regression in run 3, I-02 from run 4. */
export function provingRuns(): ProofRunRecord[] {
  const V = "violated" as const;
  const P = "proved" as const;
  const table: [ProofOutcome["verdict"], ProofOutcome["verdict"], ProofOutcome["verdict"], ProofOutcome["verdict"]][] = [
    [V, V, V, P],
    [P, V, V, P],
    [P, P, V, V],
    [P, P, "evidenced", P],
    [P, P, P, P],
  ];
  return table.map(([a, b, c, reg], i) => {
    const outcomes = [
      o("P-003.PF-01", "P-003.I-01", a, a === V ? { counterexample: { input: { code: "valid", items: 80 }, output: { discount: 10 } } } : {}),
      o("P-003.PF-02", "P-003.I-01", b, b === V ? { counterexample: { input: { code: "valid", items: 80, shipping: 9.9 }, output: { fee: 8.91 } } } : {}),
      o("P-003.PF-03", "P-003.I-02", c, c === V ? { counterexample: { input: { code: "expired", items: 50 }, output: { discount: 5, reason: "ok" } } } : {}),
      o("P-001.PF-01", "P-001.I-01", reg, reg === V ? { counterexample: { input: { total: 120 }, output: { fee: 9.9 } } } : {}),
    ];
    const regressions = outcomes.filter((x) => x.role === "regression" && !x.passed).map((x) => x.proof);
    const ok = outcomes.every((x) => x.passed);
    return {
      run: i + 1,
      at: new Date(Date.UTC(2026, 10, 11, 14, i * 12)).toISOString(),
      commit: `${"abcdef0123456789".slice(i, i + 4)}${"0".repeat(36)}`,
      ...(i === 3 ? { dirty: true } : {}),
      ok,
      outcomes,
      regressions,
      replacedStillPass: [],
      unproved: [],
      enginesAgree: true,
    };
  });
}

/** Write a fresh copy of the workspace. Files are replaced in place, so the server's file watcher keeps working. */
export function resetWorkspace() {
  for (const sub of ["usecases", "proposals", "llm"]) {
    const d = join(WORK_DIR, sub);
    mkdirSync(d, { recursive: true });
    for (const f of readdirSync(d)) rmSync(join(d, f), { recursive: true, force: true });
  }
  for (const u of useCases()) writeJson(join(WORK_DIR, "usecases", `${u.id}-${u.slug}.json`), u);
  for (const p of proposals()) writeJson(join(WORK_DIR, "proposals", `${p.id}-${p.slug}.json`), p);
  writeFileSync(join(WORK_DIR, "proposals", "P-003-discount-codes.runs.jsonl"), provingRuns().map((r) => JSON.stringify(r)).join("\n") + "\n");
}
