---
name: propose
description: Turn a requested change into Lastro intents the user reviews like a pull request - write the proposal as prose, extract intents, ask the user to answer every decision and approve, amend or reject every intent, then derive the proofs. Use before writing code for any change in behavior in a repository that has .lastro/plugin.json.
---

# Propose a change through Lastro

In this repository **the intents are the code review**. You are the author of the proposal; the user is the
reviewer. They review intents the way they would review a pull request, before any code exists. You never approve,
answer or amend on their behalf. Code edits stay blocked until the review is done and the proofs are derived.

`<cli>` is the `cli` in `.lastro/plugin.json` (default `lastro`). Every command prints one JSON object.

## 1. Write the proposal

Write the change as plain prose in `.lastro/proposals/<slug>.md`: what the behavior must be after the change, in
the user's words plus what you would implement. Behavior only, no code, no file names. One proposal per change.

## 2. Extract the intents

Run `<cli> plan .lastro/proposals/<slug>.md`.

- **Exit 3 (`needs-llm`)**: the extraction is yours. Read the `request` file: act as its `system`, read its `user`,
  and write ONLY a JSON value matching its `schema` to the `answer` path. Run the same command again.
- **Exit 1 or 0**: intents and decisions come back. The board shows them.

## 3. The review (the user's, like a pull request)

Present the proposal as a pull request for review: a one-line summary, then every intent with its id, statement and
the quote it came from. Then:

- **Decisions first.** Each open decision is a review question. Ask it with its options and the recommended one
  first (Claude Code: the `AskUserQuestion` tool, one question per decision; Codex: ask in the chat and wait).
  Record the answer: `<cli> review --as <reviewer> answer <D> <option-index>`.
- **Then each intent**: Approve, Request changes, or Reject. Ask the user per intent (Claude Code: `AskUserQuestion`
  with those three options; several intents may go in one call). Record exactly what they chose:
  - Approve: `<cli> review --as <reviewer> approve I1 I2`
  - Request changes: ask what should change, then `<cli> review --as <reviewer> amend <I> <new statement>`
  - Reject: `<cli> review --as <reviewer> reject <I> <their reason>`
  - Only when the user explicitly says to approve everything: `<cli> review --as <reviewer> approve-all`.
- `<reviewer>` is the user's name (ask once). Never `claude`, `codex` or another agent name: an author cannot
  approve their own proposal, and the plugin refuses it. In Claude Code each review command also shows a permission
  prompt: that click is the user's approval.
- If the policy needs more approvals than this user can give (`requiredApprovals` > 1), the proposal waits for the
  other reviewers, like a pull request waiting for a second approval. Commit `.lastro/` and tell the user who still
  has to review. Do not continue.
- New decisions or intents can appear after an answer: repeat until `<cli> status` says `"ready": true`.

## 4. Derive the proofs

Run `<cli> spec` (exit 3 means the derivation is yours: answer it like the extraction). If it reports
`contradictions`, the new decisions go back to the user (step 3). When it succeeds, the proofs are in
`.lastro/spec.json`, in this repository.

## 5. Implement

Implement from the approved intents in `.lastro/intents.json`; there is no other brief. Then end your turn: the
Stop hook builds, proves, and sends the failures back if any proof fails. Use the `verify` skill to prove on
demand.
