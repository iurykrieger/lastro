---
name: verify
description: Check the implementation against the proofs Lastro derived from the approved intents, show the progress board (passed, failed, pending) and fix the code until every proof passes. Use after implementing an approved Lastro proposal, or when the user asks whether the code satisfies the intents.
---

# Verify the implementation

Run `node "<plugin root>/scripts/lastro-plugin.mjs" prove` from the repository root (in Claude Code the plugin root
is `${CLAUDE_PLUGIN_ROOT}`; in Codex it is `$PLUGIN_ROOT`). It runs the `build` and `prove` commands from
`.lastro/plugin.json` and prints the board: one line per proof, `✓` proved, `≈` evidenced (sampled, not proved),
`✗` violated with its counterexample, `!` weak or vacuous, `○` not run. Without the plugin script, run
`<cli> prove <module> --export <name>` and `<cli> feedback`.

When a proof fails:

1. Read the counterexample: the input, what the code returned, and the clause it breaks.
2. Fix the code so the clause holds for every input, without breaking the others. Read the approved intent in
   `.lastro/intents.json` when the clause alone is not enough.
3. Never edit `.lastro/intents.json`, `spec.json` or `proof.json`, and never weaken a test or the model to get
   green. If you believe an intent or a proof is wrong, stop and tell the user: changing it is a new proposal
   (`propose` skill) that they review.
4. Prove again.

`≈ evidenced` is not `✓ proved`: say so when you report. Proofs marked beyond proof need the user's sign-off;
list them. When every proof passes, show the board and the receipt (`.lastro/receipt.md`): it goes in the pull
request description.
