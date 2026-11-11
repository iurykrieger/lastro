# Proposal: use cases, proposals, intents and proofs

Status: **decided, not implemented yet**. The decisions are listed at the end; the sections below already
reflect them.

## The entities

Lastro works with four entities. Each one has an `id`, a `slug`, a `name` and a `description`, plus its own fields.

| Entity | What it is | Where it lives |
|---|---|---|
| **Use case** | One thing the application does, described in full as the code implements it, with the proofs that keep it working. | `.lastro/usecases/<id>-<slug>.json` |
| **Proposal** | A change to the repository: the prose someone wrote and everything derived from it. | `.lastro/proposals/<id>-<slug>.json` |
| **Intent** | One non-deterministic reading of the prose, in natural language: the addition, removal or change of one use case. It is approved, rejected or amended. | inside its proposal |
| **Proof** | A deterministic check that an intent holds in practice. Once it passes, it belongs to the use case as a regression test. | inside its proposal, then inside its use case |

An application is its list of use cases. Every change to the repository starts as a proposal.

```
                      ┌──────────── .lastro/usecases/ (the application, with its proofs) ───────────┐
                      │                                                                            │
prose ─▶ PROPOSAL ─ extract ─▶ INTENTS (add / modify / remove a use case) ─▶ review                │
                                                                              │                    │
                                                approved or amended intents   ▼                    │
                                                                    derive ─▶ PROOFS (pending)      │
                                                                              │                    │
                                              code (any agent or person) ─▶ prove ◀── regression ──┘
                                                                              │   (every use case's proofs)
                                                       all proofs pass        ▼
                                                                 apply ─▶ proofs join their USE CASES,
                                                                          descriptions synced, proposal applied
```

## Ids, slugs and files

- Use cases are numbered `UC-001`, `UC-002`, ... and proposals `P-001`, `P-002`, ...: the next free number in the
  folder. A number is never reused, even after a use case is removed.
- Intents and proofs are numbered inside their proposal, so their ids carry their origin: `P-007.I-03` is the
  third intent of proposal 7 and `P-007.PF-05` the fifth proof derived for it. A proof keeps its id after it
  moves to its use case, so every regression test points back to the proposal and the intent it came from.
- Slugs are kebab-case and stable; the file name is `<id>-<slug>.json` (`UC-012-derive-proof-plan.json`). A
  rename changes `name`, never the slug or the id.

## Use case

```ts
interface UseCase {
  id: string;                  // "UC-012"
  slug: string;                // "derive-proof-plan"
  name: string;
  description: string;         // the complete current behavior, as the code implements it
  proofs: Proof[];             // the proofs that passed, kept as regression tests (status "passed")
  history: string[];           // ids of the applied proposals that added or changed it
}
```

The use cases are the baseline. Their descriptions reflect the code, and their proofs show it: a proof only
joins a use case after it passed against the code.

## Proposal

```ts
interface Proposal {
  lastro: "proposal/v1";
  id: string;                  // "P-007"
  slug: string;
  name: string;
  description: string;         // a one-paragraph summary of the change
  status: "open" | "changes-requested" | "approved" | "proving" | "proved" | "applied" | "closed";
  submittedBy: string;         // who wrote the prose; the intents' author is the extractor (the LLM)
  prose: string;
  baseline: string;            // sha256 over the canonical use cases (without proofs) the prose was read against
  intents: Intent[];
  useCases: UseCaseChange[];   // the resulting state of each touched use case
  decisions: DecisionCard[];
  threads: Thread[];           // review comments
  notPromised: string[];
  policy: ReviewPolicy;        // the policy it was reviewed under
  proofs: Proof[];             // derived from the approved intents; pending until they pass
  supersedes: string[];        // ids of existing proofs that the change retires (reviewed with the proofs)
  hash?: string;               // set by approval
}

interface UseCaseChange {
  useCase: string;             // existing id for modify/remove; a newly reserved id for add
  slug: string;
  change: "add" | "modify" | "remove";
  name: string;
  before?: string;             // modify/remove: copied from the use case by the SDK, never by the LLM
  after?: string;              // add/modify: the complete new description, all its intents combined
  intents: string[];
}
```

## Intent

```ts
interface Intent {
  id: string;                  // "P-007.I-03"
  slug: string;
  name: string;
  description: string;         // the change, in natural language
  change: "add" | "modify" | "remove";
  useCase: string;             // the use case it changes (reserved id for an add)
  provability: Provability;    // formal | empirical | beyond-proof
  beyondProofReason?: BeyondProofReason;
  source: SourceSpan;          // the quote in the prose that motivates it
  status: "proposed" | "approved" | "amended" | "rejected";
  approvals: string[];
  reviewNote?: string;
  originalDescription?: string; // kept on amend
}
```

An intent is what the LLM understood from the prose, so it is not deterministic: two extractions of the same prose
can differ. That is why intents are reviewed and proofs are not written from the prose directly. Each intent changes
exactly one use case, and several intents may change the same one; the SDK combines them into that use case's
`after`. Deterministic checks on every extraction:

- Quotes are anchored in the prose, as today.
- A `modify` or `remove` names an existing use case; an `add` reserves a new id and a new kebab-case slug.
- Every touched use case has exactly one `UseCaseChange`, with `after` for add and modify, listing at least one intent.
- Change kinds on one use case fit together (no `add` or `remove` mixed with a `modify`, no `remove` with anything),
  and two intents that contradict each other on one use case come back as a decision.

`kind` goes away: `preserve` is what every untouched use case already is, and `behavior`, `invariant` and
`non-functional` move into the use case's description.

## Proof

```ts
interface Proof {
  id: string;                  // "P-007.PF-05"
  slug: string;
  name: string;
  description: string;         // what it checks, in plain words
  intent: string;              // "P-007.I-03"
  useCase: string;             // "UC-012"
  model: string;               // the model it is written against (declared in .lastro/config.json)
  subject: string;             // how to run the code (declared in .lastro/config.json)
  check:
    | { kind: "clause"; requires: Expr; ensures: Expr; assumptions: string[] }
    | { kind: "scenario"; input: Record<string, Value>; expect: Expr };
  status: "pending" | "passed" | "failed";
  verdict?: Verdict;           // proved, proved-under-assumptions, evidenced, violated, ...
  evidence?: Evidence;
}
```

A proof is a deterministic check of one intent: a clause in LSL, proved over the finite domain or evidenced by
seeded sampling, or a concrete scenario. It names its model and its subject so it can run on its own, later, as a
regression test. Proofs are derived only from approved or amended intents.

- A `remove` intent always gets a proof that the old behavior no longer happens, written from the use case's
  `before`. When the model cannot express it, derivation returns a decision for the reviewer, and the proposal
  cannot be applied while it is open.
- A `modify` may make some of the use case's existing proofs wrong on purpose. Derivation lists them in
  `supersedes`, and reviewers see them next to the new proofs. A superseded proof is skipped by the regression run
  of this proposal and removed at apply. Any other existing proof that fails is a regression.

## The flow

1. **Propose.** `propose(prose, { submittedBy })` creates `P-<n>` in `.lastro/proposals/` and extracts its intents
   against the current use cases (AI step "extract"). Status `open`.
2. **Review.** Reviewers approve, amend or reject each intent, comment in threads, and answer decisions.
   Rejecting or amending an intent marks its use case's `after` stale; it is recombined from the remaining intents
   (part of the extract step, through the same LLM or handoff) and needs review again.
3. **Approve.** The proposal is approved when every intent is approved, amended or rejected, and every decision and
   thread is resolved, under the policy in `.lastro/config.json`. The approval hash binds the baseline: if the use
   cases changed since extraction, the approval is stale and the prose must be extracted again. Status `approved`.
4. **Derive.** `derive(proposal)` writes pending proofs for the approved and amended intents only (AI step
   "derive"), plus `supersedes`. The approved intents and the resulting use cases are the brief for whoever
   implements the change; there is no separate brief. Status `proving`.
5. **Implement.** Any agent or person writes the code. Lastro never does.
6. **Prove.** `prove(proposal)` runs the proposal's proofs and the proofs of every use case (except the superseded
   ones) against the code. Failures go back to the implementer through `feedback`; a failing use-case proof is a
   regression. When everything passes, status `proved`.
7. **Apply.** `apply(proposal)` runs only when every proof passes. It writes each `UseCaseChange` (adds the file,
   replaces the description and name, or deletes the file of a removed use case), moves each proof to its use case
   as a regression test, removes the superseded proofs, appends the proposal id to `history`, and records the new
   baseline hash in the receipt. Status `applied`.

Changing an approved intent (amend, reject, a new extraction) withdraws the approval, so derive and apply refuse
until it is approved again.

## Reviewing intents like a pull request

Intents replace code as what a developer reviews. Reviewing a proposal should feel like reviewing a pull request:
the prose is the description, each touched use case is a changed file, and each intent is a change in it that gets
resolved like a review comment.

| Pull request today | Proposal in Lastro |
|---|---|
| Author opens a PR with a description | Someone submits prose; extraction turns it into intents. The extractor (the LLM) is the author of the intents, and the person who wrote the prose is recorded as the submitter. |
| Diff per file | Diff per use case: before and after, with the intents that produce it, and later the proofs it gains or loses. |
| Line comment, starting a thread | A comment on one intent, one proof or the prose, starting a thread. Threads are resolved by the commenter or the submitter. |
| Suggested change | An amended intent, which recombines the use case's `after`. |
| Resolving every comment | Every intent ends approved, amended or rejected; only non-rejected intents go forward. |
| Author pushes new commits | The prose is edited and extracted again. Intents that changed lose their approvals (stale review dismissed); untouched intents keep them. |
| Required approvals, branch protection | The policy in `.lastro/config.json`: N distinct reviewers per intent, and whether the submitter counts as one. |
| Unresolved conversations block the merge | Open threads, unresolved intents and unanswered decisions block approval. |
| CI must pass, then merge | Every proof, new and regression, must pass; then apply. |

The intents are what the LLM understood from the prose, so the LLM is their author and every person is a reviewer,
including whoever wrote the prose. By default the submitter can approve and amend the intents and go on to the
implementation, the way a pull request author can adjust everything and still merge when branch protection allows
it. A repository that wants a second pair of eyes sets `submitterCanApprove: false`.

## Configuration

`.lastro/config.json` holds what the proofs run against and how proposals are reviewed:

```json
{
  "models": { "login": { "...": "..." } },
  "subjects": { "login": { "module": "dist/login.js", "export": "login" } },
  "review": { "approvals": 2, "submitterCanApprove": true }
}
```

`approvals` defaults to 1 and `submitterCanApprove` to true. `lastro init --approvals N` writes it (today it writes
`requiredApprovals`, which becomes `review.approvals`; the old key is still read). A proposal records the policy it
was reviewed under, so a later change to the config never turns an approved proposal into an unapproved one or the
reverse.

## Where files live

Everything the framework generates or keeps lives in `.lastro/` (or the folder given with `--dir`):

```
.lastro/
  config.json                         models, subjects and the review policy
  usecases/<id>-<slug>.json           the application: one file per use case, with its regression proofs
  proposals/<id>-<slug>.json          one file per proposal: prose, intents, review, proofs
  proposals/<id>-<slug>.receipt.md    the proposal's last receipt
  llm/                                handoff requests and answers
```

`intents.json`, `spec.json` and `proof.json` go away: their content now lives in the proposal. `apply` rewrites
`.lastro/usecases/`. No framework file is written outside the workspace folder.

## CLI

```
lastro propose <prose-file> [--section <heading>] [--as <submitter>]   create a proposal and extract its intents
lastro status [<proposal>]                                             proposals, or one proposal's pending items
lastro review <proposal> --as <name> approve|amend|reject|answer|comment|resolve ...
lastro derive <proposal>                                               write its pending proofs
lastro prove <proposal> --subject ...                                  its proofs plus every use case's proofs
lastro feedback <proposal>                                             failures for the implementer
lastro apply <proposal>                                                attach the proofs and sync the use cases
lastro regress                                                         run every use case's proofs alone
```

The receipt gains the use-case diff (each use case added, modified with before and after, or removed, with its
intents), the proofs each use case gains or loses, and a line for each regression.

## Decisions

Answered by Iury on 2026-11-11:

1. **Several intents on one use case:** allowed; the SDK combines them into the use case's `after` and returns
   conflicts as decisions.
2. **Proving a removal:** always a proof that the old behavior is gone; when the model cannot express it, a decision.
3. **When to apply:** only after every proof passes.
4. **Where proofs live:** with their use case, as regression tests; every change re-runs all of them.
5. **Approval scope:** per intent; only non-rejected intents go forward.
6. **Who is the author:** the LLM that extracted the intents; the person who wrote the prose is a reviewer, and
   whether their approval counts is a per-repository policy (by default it does).
7. **The review policy** (number of reviewers and the submitter rule) lives in `.lastro/config.json`.
8. **The entities** are use case, proposal, intent and proof, each with id, slug, name and description; use cases
   live in `.lastro/usecases/<id>-<slug>.json` with their proofs, and every change starts as a proposal.

## Cost

A semantic change in all three SDKs (TypeScript, .NET, Go): the four entities and their files, the extraction
prompt and schema (and `go generate`), the recombination step, the review with threads and the submitter policy,
carry-over, derivation into proofs with `supersedes`, the regression run, apply, the receipt and the CLI, with the
cross-check tests and the sandbox example updated to the new format. The Rust prover does not change: it only sees
models, clauses and tables.

The use cases already in `.lastro/usecases/` start with no proofs. Until a use case has proofs, verification reports
it as unproved rather than failing; giving them proofs is a separate proposal.
