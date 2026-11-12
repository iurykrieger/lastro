---
name: bootstrap
description: Bring an existing repository into Lastro - read its code, tests and commit history, propose the use cases it implements today as a bootstrap proposal the user reviews like a pull request, derive proofs and check them against the code as it is, then apply them as the baseline. Use when the user wants to start Lastro on a repository that already has code (not an empty one), when `.lastro/usecases/` is empty or incomplete, or when they ask what the application does today.
---

# Bootstrap the use cases of an existing repository

A repository with thousands of commits already has behavior. Before anyone proposes a change, Lastro needs the
**use cases** the code implements today, each with **proofs** that pass against that code. A bootstrap reads the
repository (files, tests, commit history) and writes them as one **bootstrap proposal**: intents that add a use case
(or fix the description of one that has no proofs yet), each grounded in the files, tests and commits it was read
from. The user reviews it like a pull request. Nothing becomes a use case until the user approved it, its proofs
passed against the current code, and the proposal was applied.

You are the author of the reading; the user is the reviewer. You never approve, amend or answer for them.

`<cli>` is the `cli` in `.lastro/plugin.json` (default `lastro`). Every command prints one JSON object.

## 1. Size the repository

Run `<cli> scan`. It reads the repository without an LLM and prints how many files, tests and commits it found, the
manifests (what the repo ships), its **areas** and `suggestedScopes`. Show the user the areas.

- A small repository is one bootstrap.
- A large one (`truncated` is true, or many areas) goes **one area at a time**: one bootstrap proposal per area,
  each small enough to review in one sitting. Ask the user which area to start with, usually the one with the core
  decisions (pricing, permissions, validation) and the most tests.
- `--max-commits N` (default 400) and `--since <date>` bound the history; `--scope a,b` bounds the files.

## 2. Survey the code

Run `<cli> bootstrap --as <user> [--scope <area>]` (`--as` names who asked for it: the submitter).

- **Exit 3 (`needs-llm`)**: the survey is yours. Read the `request` file: act as its `system`, read its `user` (the
  recorded use cases, the digest of files, tests and commits, and source excerpts). You may open any file of the
  digest to read more. Write ONLY a JSON value matching its `schema` to the `answer` path, then run the same command
  again. Describe what the code **does**, not what it should do; when code, tests and commits disagree, write a
  decision instead of picking. Ground every behavior in files, tests or commits of the digest: the CLI refuses
  anything else and lists every problem, so fix the answer and run again.
- **Exit 1 or 0**: the proposal `<P>` is created as `.lastro/proposals/<P>-<slug>.json`, and its report (one section per
  use case, one line per behavior with its grounds) as `<P>-<slug>.md`.

## 3. The review (the user's, like a pull request)

Follow step 3 of the `propose` skill: decisions first, then each intent (Approve, Request changes, Reject), threads,
and rewriting a use case after an amend or a reject. For a bootstrap, also show each intent's grounds and
confidence (from the report): the user checks the reading against files and commits they know. A "low" confidence
intent deserves a question, not a default approval. Rejecting a behavior that the code really has is fine: it stays
unrecorded, and the proposal says what it does not cover.

The proposal may bring **models and subjects** (`setup` in the JSON): the vocabulary of the proofs and the function
each one runs. Show them with the intents; they join `.lastro/config.json` only when the proposal is applied.

## 4. Derive and prove against today's code

Run `<cli> derive <P>` (exit 3: answer it like the survey), then `<cli> prove <P>` (or with the module from
`.lastro/plugin.json`'s `prove`, after its `build`).

The code is the reference here. **Never change the code to make a bootstrap proof pass.** A failing proof means:

- the reading is wrong: tell the user, and on their word amend the intent (`<cli> review <P> --as <user> amend ...`),
  derive again and prove again; or
- the code has a bug: tell the user. They reject the intent (the bug is not recorded as behavior) or keep the
  reading and open a normal proposal that fixes the code, after this one is applied.

## 5. Apply

When `<cli> prove <P>` passes, run `<cli> apply <P>`: the use cases are written to `.lastro/usecases/`, their proofs
to `.lastro/proof/` as regression tests, and the proposal's models and subjects join `.lastro/config.json`. Commit
`.lastro/`. Bootstrap the next area, or switch to the `propose` skill for the first change.

Use cases that are already written but have no proofs (`<cli> status` lists them under `unproved`) need no proposal:
run `<cli> derive --usecases` (answer the handoff from the use cases' descriptions only), then
`<cli> prove --usecases`. A use case no declared model expresses comes back as `unprovable`; tell the user. A failing
proof goes to the user with the note, never back to the code.
