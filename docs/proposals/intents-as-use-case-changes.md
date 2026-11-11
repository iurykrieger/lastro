# Proposal: intents are changes to use cases

Status: **draft for review**. Nothing here is implemented yet.

## The idea

Today an intent is a standalone promise read from the prose. The model this proposal moves to:

- **Use cases** describe what the application does now. The catalog (`.lastro/use-cases.json`: `slug`, `name`,
  `description`) is the baseline, with each description stating the complete current behavior.
- **Prose** is a change request against that baseline.
- **Intents** are the changes the prose implies. Each one **adds**, **modifies** or **removes** exactly one use
  case. Reviewing intents means reviewing proposed changes to the catalog.
- Once the code is proved against the approved changes, the changes are **applied** to the catalog, which
  becomes the new baseline. The catalog stays versioned with the code.

```
use-cases.json (baseline) ─┐
                           ├─ extract ─▶ intents: add / modify / remove ─▶ review ─▶ approved
prose (change request) ────┘                                                    │
                                                                         derive ▼
                                                proof plan for the use cases after the change
                                                                                │
                                                       code ──── prove ─────────┤
                                                                                ▼
                                                     apply ─▶ use-cases.json (new baseline) + receipt
```

## Data model

```ts
interface UseCase { slug: string; name: string; description: string }

interface Intent {
  id: string;                         // "I1"
  change: "add" | "modify" | "remove";
  useCase: string;                    // slug: existing for modify/remove, new for add
  name?: string;                      // add, or a rename on modify
  before?: string;                    // modify/remove: copied from the catalog by the SDK, never by the LLM
  after?: string;                     // add/modify: the complete new description of the use case
  provability: Provability;           // unchanged: formal | empirical | beyond-proof
  beyondProofReason?: BeyondProofReason;
  source: SourceSpan;                 // unchanged: the quote that motivates the change
  status, approvals, reviewNote, ... // unchanged
}

interface IntentArtifact {
  lastro: "intent-artifact/v1";
  baseline: string;                   // hash of the canonical catalog the prose was read against
  prose: string;
  intents: Intent[];
  decisions: DecisionCard[];
  notPromised: string[];
  policy?: ReviewPolicy;
  hash?: string;
}
```

`kind` goes away: `preserve` is what every untouched use case already is, and `behavior`, `invariant` and
`non-functional` move into the use case's description.

## Approved intents

"Frozen" is gone as a user-facing term: the reviewed state is called **approved**. An intent is approved when it
meets the team's review policy, and the change request is approved when every intent is approved or rejected
and every decision is answered. Approval is what gates everything downstream:

- Only approved intents are sent to proof derivation. Proposed, amended-but-unapproved and rejected intents never are.
- The approved intents are the brief for whoever implements the change (an agent or a person): there is no
  separate brief artifact. The implementer reads the approved intents (use case, before and after) from the
  approved artifact; failing proofs keep coming back through `feedback`.
- Changing an approved intent (amend, reject, a new extraction) withdraws the approval of the change request,
  so derivation and `apply` refuse until it is approved again.

## Reviewing intents like a pull request

Intents replace code as what a developer reviews. Reviewing a change request should feel like reviewing a pull
request today: the prose is the description, each intent is a changed file, and the review happens per line.

| Pull request today | Change request in Lastro |
|---|---|
| Author opens a PR with a description | An author submits prose; extraction turns it into the change request's intents. The author is recorded on the artifact. |
| Diff per file | Diff per use case: before and after, for each intent. |
| Line comment, starting a thread | A comment on one intent (or on the prose), starting a thread. Threads are resolved by the author or the commenter. |
| Suggested change | A suggested `after` (the current `amend`), which the author accepts or not. |
| Review verdict: approve, request changes, comment | The same three, per reviewer, on the whole change request, each with optional per-intent comments. Rejecting one intent becomes a comment that requests changes on it. |
| Author pushes new commits | The author edits the prose and extracts again, or accepts a suggestion. Intents that changed lose their approvals (stale review dismissed); untouched intents keep them. |
| Required approvals, no self-approval | The team's policy: N distinct reviewers, never the author. |
| Unresolved conversations block the merge | Open threads and unanswered decisions block approval. |
| Approved, then merged | Approved (proofs can be derived and the implementer works from the approved intents), then applied to the catalog once the proof is ok. |
| PR status | `open`, `changes-requested`, `approved`, `applied`, `closed`. |

Everything stays in the artifact in `.lastro/`, so the review works the same from the library, any of the CLIs
(`lastro review --as ana comment I3 "..."`, `request-changes`, `approve`, `resolve`), an agent or a UI such as the
sandbox. Lastro does not depend on a code host; a GitHub integration could mirror threads later.

## Where files live

Everything the framework generates or keeps lives in `.lastro/` (or the folder given with `--dir`):

```
.lastro/
  use-cases.json     the catalog: the baseline, versioned with the code
  config.json        model and review policy
  intents.json       the current change request's intents and their review
  spec.json          the approved intents and the derived proof plan
  proof.json         the last verification
  receipt.md         the last receipt
  llm/               handoff requests and answers
```

`apply` rewrites `.lastro/use-cases.json`. No framework file is written outside the workspace folder.

## Pipeline changes

| Step | Today | Proposed |
|---|---|---|
| Extract | `plan(prose)` | `plan(prose, catalog)`. The prompt gets the catalog and must return changes. Deterministic checks: `modify`/`remove` name an existing slug; `add` uses a new kebab-case slug; `after` is present for add/modify; quotes are anchored as today. `before` is filled from the catalog. |
| Review | approve / amend / reject statements | Same actions on changes. `amend` rewrites `after` (or the slug of an add). |
| Approve | `approve()` (formerly `freeze()`) hashes the reviewed artifact | The hash also binds `baseline`: if the catalog changed since extraction, the approval is stale and the prose must be extracted again. |
| Derive | clauses from approved intents of a frozen artifact | Clauses from the **after** state of each touched use case. Unchanged use cases are out of scope for this change. |
| Prove | unchanged | unchanged |
| Apply | — | New `apply` (library and CLI): writes the approved changes into the catalog, only when the proof is ok, and records the new baseline hash in the receipt. |
| Re-extract | carry review by statement | Carry review by `(change, useCase, after)`, ignoring case and spacing. |

The receipt gains a section with the catalog diff: each use case added, modified (before and after) or removed.

## Open questions

1. **Several intents on one use case.** Allow a prose sentence to produce two `modify` intents on the same slug
   (each a separate promise), or require one intent per use case with the full new description?
   Recommendation: one intent per use case per change request, so `after` is always the full description.
2. **Proving a removal.** Is a `remove` beyond proof (it needs a human sign-off), or does the deriver try a
   clause showing the old behavior no longer happens? Recommendation: beyond proof by default; the deriver may
   add a clause when the model can express it.
3. **When to apply.** Only after the proof is ok (recommended), or right after approval?
5. **Approval scope.** On a pull request, an approval covers the whole PR. Should a reviewer's approval cover
   the whole change request (recommended, as on a PR), or keep today's per-intent approvals?
6. **Who is the author.** The person who submitted the prose (recommended), or whoever last edited an intent?
4. **Where proofs live.** Should each use case keep its clauses so the next change re-proves the whole catalog,
   or are proofs per change request as today? Recommendation: per change request in v1; a persistent spec per
   use case is a later step.

## Cost

A semantic change in all three SDKs (TypeScript, .NET, Go): data model, extraction prompt and schema (and
`go generate`), artifact checks, review, carry-over, receipt and CLI (`plan --catalog`, `apply`), with the
cross-check tests and the sandbox example updated to the new format. The Rust prover does not change: it only
sees models, clauses and tables.
