/**
 * A small `.lastro/` workspace for the UI tests: a shipping app with three use cases, one applied proposal
 * that gave UC-001 its regression proof, and one open proposal (P-002) under a two-reviewer policy where
 * the submitter (ana) does not count. Every test starts from a fresh copy.
 */
import { mkdirSync, readdirSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { L, baselineOf, type Proposal, type UseCase } from "../../../sdks/typescript/src/index.ts";
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
  return [applied, open];
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
}
