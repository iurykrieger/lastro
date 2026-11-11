# Proposal: intents are changes to use cases

Status: **decided, not implemented yet**. The open questions were answered on 2026-11-11 (see Decisions);
the sections below already reflect the answers.

## The idea

Today an intent is a standalone promise read from the prose. The model this proposal moves to:

- **Use cases** describe what the application does now. The catalog (`.lastro/use-cases.json`: `slug`, `name`,
  `description`) is the baseline, with each description stating the complete current behavior. The catalog
  reflects the code.
- **Prose** is a change request against that baseline.
- **Intents** are the changes the prose implies. Each one **adds**, **modifies** or **removes** a use case, and
  several intents may touch the same use case. Reviewing intents means reviewing proposed changes to the catalog.
- **Proofs** belong to use cases. Each use case keeps its clauses, and every change re-proves the whole catalog.
- Once the code passes every proof, the change is **applied**: the catalog is synced to the state the approved
  intents describe, and becomes the new baseline. The catalog stays versioned with the code.

```
use-cases.json (baseline) ─┐
                           ├─ extract ─▶ intents: add / modify / remove ─▶ review ─▶ approved
prose (change request) ────┘                                                    │
                                                                         derive ▼
                                   clauses for the touched use cases (+ the kept clauses of the rest)
                                                                                │
                                                       code ──── prove ─────────┤ every clause of the catalog
                                                                                ▼
                                       apply ─▶ use-cases.json + specs (new baseline) + receipt
```

## Data model

```ts
interface UseCase { slug: string; name: string; description: string }

interface Intent {
  id: string;                         // "I1"
  change: "add" | "modify" | "remove";
  useCase: string;                    // slug: existing for modify/remove, new for add
  statement: string;                  // the one change this intent promises, in a sentence
  provability: Provability;           // unchanged: formal | empirical | beyond-proof
  beyondProofReason?: BeyondProofReason;
  source: SourceSpan;                 // unchanged: the quote that motivates the change
  status, approvals, reviewNote, ... // unchanged
}

/** The resulting state of one touched use case: what `apply` will write. */
interface UseCaseChange {
  useCase: string;
  change: "add" | "modify" | "remove";
  name?: string;                      // add, or a rename on modify
  before?: string;                    // modify/remove: copied from the catalog by the SDK, never by the LLM
  after?: string;                     // add/modify: the complete new description, all its intents combined
  intents: string[];                  // ids of the intents that produce this change
}

interface IntentArtifact {
  lastro: "intent-artifact/v1";
  baseline: string;                   // hash of the canonical catalog the prose was read against
  author: string;                     // who submitted the prose
  prose: string;
  intents: Intent[];
  useCases: UseCaseChange[];          // one entry per touched use case
  decisions: DecisionCard[];
  notPromised: string[];
  policy?: ReviewPolicy;
  hash?: string;
}
```

`kind` goes away: `preserve` is what every untouched use case already is, and `behavior`, `invariant` and
`non-functional` move into the use case's description.

### Several intents on one use case

Each promise in the prose is its own intent, so one use case can receive several `modify` intents (or one `add`
plus refinements). Extraction returns the intents and, for each touched use case, the combined `after`.
Deterministic checks on the result:

- Every intent's `useCase` has exactly one `UseCaseChange`, and every `UseCaseChange` lists at least one intent.
- All intents on one use case agree on the kind of change: an `add` or a `remove` cannot be mixed with a `modify`
  on the same slug, and `remove` cannot be combined with anything. A mix is returned as a decision.
- Two intents that contradict each other on the same use case are returned as a decision, as contradictions are today.
- When an intent on a use case is rejected or amended, that use case's `after` is stale: it is recombined from the
  remaining intents (an extraction step, through the same LLM or handoff) and the change request needs review again
  for that use case before it is approved.

## Approved intents

"Frozen" is gone as a user-facing term: the reviewed state is called **approved**. Approval is per intent, the
way comments are resolved on a pull request:

- Each intent ends **approved** (by the policy's number of reviewers), **amended** (the reviewer's new wording
  counts as their approval; everyone else approves it again) or **rejected**.
- The change request is approved when every intent is approved, amended or rejected, and every decision is answered.
- Only the intents that were not rejected go forward: they are sent to proof derivation and they are what the
  implementer builds. A rejected intent drops out of its use case's `after`.
- The approved intents are the brief for whoever implements the change (an agent or a person): there is no
  separate brief artifact. The implementer reads the approved intents and the resulting use cases (before and
  after) from the approved artifact; failing proofs keep coming back through `feedback`.
- Changing an approved intent (amend, reject, a new extraction) withdraws the approval of the change request,
  so derivation and `apply` refuse until it is approved again.

## Reviewing intents like a pull request

Intents replace code as what a developer reviews. Reviewing a change request should feel like reviewing a pull
request today: the prose is the description, each touched use case is a changed file, and each intent is a
change in it that gets resolved like a review comment.

| Pull request today | Change request in Lastro |
|---|---|
| Author opens a PR with a description | An author submits prose; extraction turns it into the change request's intents. The author is recorded on the artifact. |
| Diff per file | Diff per use case: before and after, with the intents that produce it. |
| Line comment, starting a thread | A comment on one intent (or on the prose), starting a thread. Threads are resolved by the author or the commenter. |
| Suggested change | An amended statement (the current `amend`), which recombines the use case's `after`. |
| Resolving every comment | Every intent ends approved, amended or rejected; only non-rejected intents go forward. |
| Author pushes new commits | The author edits the prose and extracts again. Intents that changed lose their approvals (stale review dismissed); untouched intents keep them. |
| Required approvals, no self-approval | The team's policy: N distinct reviewers per intent, never the author. |
| Unresolved conversations block the merge | Open threads, unresolved intents and unanswered decisions block approval. |
| Approved, then merged | Approved (proofs can be derived and the implementer works from the approved intents), then applied to the catalog once every proof passes. |
| PR status | `open`, `changes-requested`, `approved`, `applied`, `closed`. |

The author is whoever submitted the prose. That person never counts as an approver of any intent in the change
request, including intents another reviewer amended.

Everything stays in the artifact in `.lastro/`, so the review works the same from the library, any of the CLIs
(`lastro review --as ana comment I3 "..."`, `approve I3`, `amend I3 "..."`, `reject I3`, `resolve`), an agent or
a UI such as the sandbox. Lastro does not depend on a code host; a GitHub integration could mirror threads later.

## Proofs per use case

Proofs are persistent and belong to use cases, not to change requests:

- Each use case keeps its clauses in `.lastro/specs/<slug>.json` (model, clauses, and the hash of the description
  they were derived from).
- A change request derives clauses only for the use cases it touches, from their `after` state. The clauses of
  every other use case are reused as they are.
- Verification runs the clauses of the whole catalog, so a change that breaks an untouched use case fails as a
  regression.
- A use case whose description changed without new clauses (its hash no longer matches) is reported as stale and
  blocks `apply`.

### Proving a removal

A `remove` always needs a clause showing the old behavior no longer happens. The deriver writes it from the
`before` description. When the model cannot express it, derivation returns a decision for the reviewer, as with
contradictions today; the change request cannot be applied while it is open. The removal clause belongs to the
change request: at `apply` the use case and its spec file are deleted.

## When to apply

`apply` runs only after every proof passes: the clauses of the touched use cases and those of the rest of the
catalog. The catalog reflects the code, and the proofs show the code does what the approved intents say, so only
then is it safe to sync the catalog to the new state. `apply` writes each `UseCaseChange` into
`.lastro/use-cases.json`, moves the derived clauses into `.lastro/specs/`, and records the new baseline hash in
the receipt.

## Where files live

Everything the framework generates or keeps lives in `.lastro/` (or the folder given with `--dir`):

```
.lastro/
  use-cases.json     the catalog: the baseline, versioned with the code
  specs/<slug>.json  the clauses of each use case, versioned with the code
  config.json        model and review policy
  intents.json       the current change request's intents and their review
  spec.json          the clauses derived for the current change request
  proof.json         the last verification
  receipt.md         the last receipt
  llm/               handoff requests and answers
```

`apply` rewrites `.lastro/use-cases.json` and `.lastro/specs/`. No framework file is written outside the
workspace folder.

## Pipeline changes

| Step | Today | Proposed |
|---|---|---|
| Extract | `plan(prose)` | `plan(prose, catalog, author)`. The prompt gets the catalog and must return intents and one `UseCaseChange` per touched use case. Deterministic checks: `modify`/`remove` name an existing slug; `add` uses a new kebab-case slug; `after` is present for add/modify; change kinds on one slug are compatible; quotes are anchored as today. `before` is filled from the catalog. |
| Review | approve / amend / reject statements | Same actions per intent, by reviewers other than the author. Rejecting or amending recombines the use case's `after`. |
| Approve | `approve()` (formerly `freeze()`) hashes the reviewed artifact | Every intent resolved and every decision answered. The hash also binds `baseline`: if the catalog changed since extraction, the approval is stale and the prose must be extracted again. |
| Derive | clauses from the approved intents | Clauses for the `after` state of each touched use case; a removal clause (or a decision) for each `remove`. |
| Prove | the change's clauses | The change's clauses plus the kept clauses of every untouched use case. |
| Apply | — | New `apply` (library and CLI): only when every proof passes; writes the catalog and the specs and records the new baseline hash in the receipt. |
| Re-extract | carry review by statement | Carry review by `(change, useCase, statement)`, ignoring case and spacing. |

The receipt gains a section with the catalog diff: each use case added, modified (before and after) or removed,
with the intents behind it, and a line for regressions in untouched use cases.

## Decisions

Answered by Iury on 2026-11-11:

1. **Several intents on one use case:** allowed. Each promise is its own intent; the SDK combines them into the
   use case's `after` and returns conflicts as decisions.
2. **Proving a removal:** always a clause showing the old behavior is gone; when the model cannot express it, a
   decision for the reviewer.
3. **When to apply:** only after every proof passes.
4. **Where proofs live:** per use case, persistent; every change re-proves the whole catalog.
5. **Approval scope:** per intent. The change request is approved when every intent is approved, amended or
   rejected and every decision is answered; only non-rejected intents go forward.
6. **Who is the author:** whoever submitted the prose, who never approves any intent of the change request.

## Cost

A semantic change in all three SDKs (TypeScript, .NET, Go): data model, extraction prompt and schema (and
`go generate`), the recombination step, artifact checks, review with authors, carry-over, persistent specs,
receipt and CLI (`plan --catalog`, `review`, `apply`), with the cross-check tests and the sandbox example updated
to the new format. The Rust prover does not change: it only sees models, clauses and tables.

Persistent specs need a one-time bootstrap: deriving and reviewing clauses for the 38 use cases already in the
catalog before the first change can be re-proved against them. Until a use case has a spec, verification reports
it as unproved rather than failing.
