# Lastro use cases

Lastro checks itself with its own pipeline. This file is the prose it starts from: each section below is one
decision core of the framework, written as a team would write a plan. `lastro plan USE-CASES.md --section <title>`
extracts the intents of one section, people review them, and only then are proofs derived and checked against
the SDK's own code. See [`.lastro/README.md`](.lastro/README.md).

Edit this file like any other plan: when a section changes, its intents are extracted again, the review of
intents that did not change is kept, and only what changed needs a new look.

## Review gate

Before any proof is derived, people review the intents. The team sets how many distinct reviewers must approve
each intent when it creates the SDK, and the default is one. An intent reaches the spec only when it has that many
approvals from different people; the same person approving twice counts once. When someone amends an intent, the
approvals others gave to the old wording no longer count: the person who amended it counts as one approval, and
everyone else must approve the new wording again. A rejected intent never reaches the spec, and it does not block
the others. An unanswered decision blocks the spec. The reviewed intents can be frozen only when nothing is
pending, and the freeze is all or nothing.

## Clause verdicts

Every proof result says how much it can be trusted. A clause is proved only when every input of a finite declared
domain was checked and none broke it; that evidence is formal. When the domain is too large or cannot be
enumerated, a clause that holds on every sample is evidenced, never proved, and its evidence is empirical. One
counterexample makes the clause violated. A clause whose if-part no input satisfies is vacuous, because it proves
nothing. A clause that rejects none of the planted bugs is weak, because it rules out no mistake. A recorded
production run that breaks a clause makes it violated, even when it was proved on the model.

## Re-extraction keeps the review

People edit the prose after the first review. When the prose is extracted again, an intent with the same
statement keeps its verdict and its approvals. If a reviewer amended an intent and the AI proposes its original
reading again for the same words, the amendment is applied again. Everything else comes back as proposed, so only
what the edit changed needs a new look. A decision asked again with the same question keeps its answer.
