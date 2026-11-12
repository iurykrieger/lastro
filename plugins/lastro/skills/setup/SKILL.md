---
name: setup
description: Set up Lastro in the current repository - draft the model of the decision core with the user, create the .lastro/ workspace and the plugin config that gates code edits and runs the proofs. Use when the user asks to start using Lastro here, or when a Lastro hook says the workspace is not set up.
---

# Set up Lastro in this repository

Lastro checks code against proofs derived from intents the user approved. The proofs are about a **decision
core**: a pure function from finite inputs to outputs (for example `shippingFee({ subtotal, member }) -> { fee }`).
Side effects stay outside it.

1. **Find the CLI.** Try `lastro --help`, then `npx lastro --help`, then `lastro-go`. Use the first that works as
   `cli` below. If none does, tell the user how to install it (`npm i -D lastro`, or build it from
   github.com/iurykrieger/lastro) and stop.
2. **Find the decision core** the user wants to change. Read the code. If it is tangled with I/O, say so and
   propose (do not perform) the extraction as a first change request.
3. **Draft the model** in `.lastro/model.json`: one entry per input and output, each with a finite domain
   (`{"type":"int","min":0,"max":200}`, `{"type":"bool"}`, `{"type":"enum","values":[...]}`). Show it to the user
   and ask them to confirm the names and the ranges. The ranges decide what "proved" covers.
4. **Create the workspace**: `<cli> init --model .lastro/model.json` (add `--approvals N` if the team wants more
   than one reviewer per intent, as a branch protection rule would, and `--submitter-cannot-approve` if whoever
   asked for a change must not approve its intents). The policy lands in `.lastro/config.json` under `review`.
5. **Write `.lastro/plugin.json`** with the user:

   ```json
   {
     "cli": "npx lastro",
     "build": "npm run build",
     "prove": "dist/shipping.js --export shippingFee",
     "enabled": true,
     "allow": ["docs/**", "**/*.md"],
     "maxIterations": 5
   }
   ```

   `prove` is everything after `lastro prove`: the built module and its export. `build` runs before each proof.
   `enabled: true` turns the hooks on: code edits wait until the proposal is approved and its proofs derived, the
   proofs run when the agent stops, and the board shows progress. `false` turns the plugin off. `allow` lists paths
   the gate never blocks.
6. Commit `.lastro/` with the change (except `.lastro/plugin/`, which holds local hook state: add it to
   `.gitignore`). Use cases, proposals and proofs travel with the code, the same way a pull request's review does.

If the repository already has code, record what it does today before the first change: use the `bootstrap` skill
(it can run before step 3, and its proposal brings the models and subjects it finds). Then use the `propose` skill
for the first change.
