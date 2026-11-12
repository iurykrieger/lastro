---
name: propose
description: Turn a requested change into a Lastro proposal the user reviews like a pull request - write the prose, extract intents that add, change or remove use cases, ask the user to answer every decision and approve, amend or reject every intent (seeing the proofs each one replaces), then derive the proofs. Use before the first source edit of any change, in any mode (plan mode, auto mode, a quick fix), in a repository that has .lastro/plugin.json, and whenever a Lastro hook denies an edit.
---

# Propose a change through Lastro

In this repository **the intents are the code review**. The application is its list of use cases
(`.lastro/usecases/`), each with the proofs that keep it working (`.lastro/proof/`). Every change is a **proposal**: prose whose
**intents** add, change or remove use cases. You write the proposal; the LLM that extracts the intents is their
author; the user is the reviewer. They review intents the way they would review a pull request, before any code
exists. You never approve, answer or amend on their behalf. Code edits stay blocked until the proposal is approved
and its proofs are derived.

`<cli>` is the `cli` in `.lastro/plugin.json` (default `lastro`). Every command prints one JSON object. `<P>` is the
proposal id the CLI gives (`P-007`; `7` works too).

This applies to every change, however it was planned: plan mode, auto mode, an informal plan in the chat, or none.
The first source edit is the trigger; the plugin denies it until this is done. When you left Claude Code's plan
mode, the plan is already saved as `.lastro/proposals/<id>-<slug>.md`: rewrite it there as behavior.

## 1. Write the prose

Write the change as plain prose in `.lastro/proposals/<slug>.md`: what the behavior must be after the change, in
the user's words plus what you would implement. Behavior only, no code, no file names. One proposal per change.

## 2. Extract the intents

Run `<cli> propose .lastro/proposals/<slug>.md --as <user>` (`--as` names who asked for the change: the submitter).

- **Exit 3 (`needs-llm`)**: the extraction is yours. Read the `request` file: act as its `system`, read its `user`
  (the prose and the current use cases with their proofs), and write ONLY a JSON value matching its `schema` to the
  `answer` path. Run the same command again.
- **Exit 1 or 0**: the proposal `<P>` is created as `.lastro/proposals/<P>-<slug>.json` (the prose moves to
  `<P>-<slug>.md`). Its intents, the use cases they touch and its decisions come back.

To change the prose later, edit `.lastro/proposals/<P>-<slug>.md` and run `<cli> propose` on it again: intents that
did not change keep their review.

## 3. The review (the user's, like a pull request)

Present the proposal as a pull request for review. After each `lastro` command the plugin hands you the review as
Markdown (also printed by `node "${CLAUDE_PLUGIN_ROOT}/scripts/lastro-plugin.mjs" review`): show it to the user exactly
as written. It puts every decision and every intent on its own list item, grouped by the use case it changes, with the
quote it came from and, marked ⚠, each existing proof the intent replaces on purpose. Never retype it as bare lines,
which chat apps join into one paragraph. Then:

- **Decisions first.** Each open decision is a review question. Ask it with its options and the recommended one
  first (Claude Code: the `AskUserQuestion` tool, one question per decision; Codex: ask in the chat and wait).
  Record the answer: `<cli> review <P> --as <reviewer> answer <D> <option-index>`.
- **Then each intent**: Approve, Request changes, or Reject. Ask the user per intent (Claude Code: `AskUserQuestion`
  with those three options; several intents may go in one call). Say what approving accepts: the proofs it replaces
  stop guarding the old behavior. Record exactly what they chose:
  - Approve: `<cli> review <P> --as <reviewer> approve I-01 I-02`
  - Request changes: ask what should change, then `<cli> review <P> --as <reviewer> amend <I> <new description>`
  - Reject: `<cli> review <P> --as <reviewer> reject <I> <their reason>`
  - Only when the user explicitly says to approve everything: `<cli> review <P> --as <reviewer> approve-all`.
- **Rewrite the use case after an amend or a reject.** Its description is then stale. Ask the user whether to write
  it themselves (`<cli> review <P> --as <reviewer> rewrite <UC> <description>`) or have it recombined
  (`<cli> review <P> --as <reviewer> recombine <UC> [instructions]`, exit 3 means you answer it). Either way that use
  case's intents need the user's review again.
- **Threads.** A user's comment on an intent goes in a thread: `<cli> review <P> --as <reviewer> comment <I> <text>`;
  `reply <T> <text>` and `resolve <T>` (by whoever opened it or the submitter). Open threads block approval.
- `<reviewer>` is the user's name (ask once). Never `claude`, `codex` or another agent name: an author cannot
  approve their own proposal, and the plugin refuses it. In Claude Code each review command also shows a permission
  prompt: that click is the user's approval.
- If the policy needs more approvals than this user can give, or the submitter's approval does not count
  (`.lastro/config.json` `review`), the proposal waits for the other reviewers, like a pull request waiting for a
  second approval. Commit `.lastro/` and tell the user who still has to review. Do not continue.
- New decisions can appear after an answer: repeat until `<cli> status <P>` says `"ready": true`. The proposal is
  then approved.

## 4. Derive the proofs

Run `<cli> derive <P>` (exit 3 means the derivation is yours: answer it like the extraction). If it comes back
`back-to-review` (a contradiction, a removal it cannot express, or an existing proof the change breaks but no intent
replaces), the new decisions and reopened intents go back to the user (step 3). When it succeeds, the proposal's
proofs are pending in `.lastro/proposals/<P>-<slug>.json`.

## 5. Implement

Implement from the approved intents and the resulting use-case descriptions in the proposal; there is no other
brief. Then end your turn: the Stop hook builds, proves the proposal together with every use case's proofs
(regressions), and sends the failures back if any proof fails. Use the `verify` skill to prove on demand. When every
proof passes, the user applies the proposal with `<cli> apply <P>`: the proofs join their use cases.
