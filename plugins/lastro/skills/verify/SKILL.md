---
name: verify
description: Check the implementation against the proofs Lastro derived from the approved intents, plus every use case's proofs (regressions), show the progress board (passed, failed, pending) and fix the code until every proof passes. Use after implementing an approved Lastro proposal, or when the user asks whether the code satisfies the intents.
---

# Verify the implementation

Run `node "<plugin root>/scripts/lastro-plugin.mjs" prove` from the repository root (in Claude Code the plugin root
is `${CLAUDE_PLUGIN_ROOT}`; in Codex it is `$PLUGIN_ROOT`). It runs the `build` and `prove` commands from
`.lastro/plugin.json` on the current proposal and prints the board: one line per proof, `✓` proved, `≈` evidenced
(sampled, not proved), `✗` violated with its counterexample, `!` weak or vacuous, `○` not run, then the regressions.
Without the plugin script, run `<cli> prove <P> <module> --export <name>` and `<cli> feedback <P>`.

When a proof fails:

1. Read the counterexample: the input, what the code returned, and the proof it breaks. A failing use-case proof is
   a regression: behavior that must keep working, even if the change did not mean to touch it.
2. Fix the code so the proof holds for every input, without breaking the others. Read the approved intent and the
   use case's new description in `.lastro/proposals/<P>-<slug>.json` when the proof alone is not enough.
3. Never edit the files in `.lastro/proposals/` (except the prose `.md`) or `.lastro/usecases/`, and never weaken a
   test or the model to get green. If you believe an intent or a proof is wrong, stop and tell the user: changing
   it is a review of the proposal (`propose` skill) that they make.
4. Prove again.

`≈ evidenced` is not `✓ proved`: say so when you report. Intents beyond proof need the user's sign-off; list them.
A replaced proof that still passes means the behavior it guarded may not have changed: mention it. When every proof
passes, show the board and the receipt (`.lastro/proposals/<P>-<slug>.receipt.md`): it goes in the pull request
description, and the user applies the proposal with `<cli> apply <P>`.
