# Lastro

**Lastro** (Portuguese for "backing", as in the reserves behind a currency) gives a plan written in prose a
deterministic backing: intents a human reviewed, proof obligations derived from them, and a reproducible check
of those obligations against what the program actually does.

It follows the ideas of GitHub Next's [PDD Kit](https://githubnext.com/projects/pdd-kit/) (Proof Driven
Development): review moves up from code to intent, and every result says plainly whether it is **proved**,
**proved under trusted assumptions**, **evidenced** to a stated confidence, or **beyond proof**.

```
prose ──extract──▶ intents ──review──▶ frozen intents (hash)
                                            │
                                     derive ▼
                         proof plan (LSL clauses, trusted pieces, scenarios, beyond-proof)
                                            │
code / production traces ──────── verify ───┤──▶ TypeScript engine
                                            └──▶ lastro-core (Rust, independent)
                                            ▼
                                        receipt
```

## What is in this repository

| Path | What it is |
|---|---|
| [`crates/lastro-core`](crates/lastro-core) | Rust prover. Enumerates the declared input space itself and only says "proved" when every point is covered. Also checks vacuity, mutants and contradictions. Ships as a CLI and as WASM. |
| [`packages/sdk`](packages/sdk) | TypeScript SDK (`lastro`): intent artifacts and review, LSL spec language (compiles to SMT-LIB2), plan validation, verification engine, receipts, Claude adapters, and the bridge to the Rust prover. |
| [`apps/sandbox`](apps/sandbox) | Interactive sandbox built with shadcn/ui: the prose and its intents, the generated code, and a contrast section that proves one against the other live, in both engines. |

## Quick start

Requires Rust (with the `wasm32-unknown-unknown` target) and Node 20+.

```sh
npm install
npm test            # cargo tests, WASM build, SDK tests (including the Rust cross-check)
npm run sandbox     # opens the sandbox on http://localhost:5173
```

The sandbox walks through a store login. Things to try:

- **Edit the business rule** in the prose panel. Claude re-extracts the intents (your earlier review is kept where
  nothing changed), derives new proof obligations, and they run against the current code, so you see which proofs
  break. **Regenerate code from the intents** then asks Claude to update the code, and the proofs decide whether it did.
  This needs Claude: it works in the sandbox published on claude.ai, or locally with `ANTHROPIC_API_KEY` set.
- **Plant a bug** in the code panel and watch the clause that catches it fail in both engines, with a counterexample you can run.
- **Approve the AI's wrong version of I5** in the prose panel: the plan changes, and the reference code now violates it.
- **Turn on the production trace**: the model still proves, but the trusted piece T2 (bcrypt) is broken by a real run.
- Open the **Receipt** tab for the Markdown that would go into a pull request.

## Using the SDK

```ts
import { createArtifact, review, freeze, validatePlan, verify, renderReceipt, loadRustProver, tabulate } from "lastro";
import { ClaudeIntentExtractor, ClaudeProofDeriver } from "lastro/claude"; // or LlmIntentExtractor(ask) over any transport

const draft = await new ClaudeIntentExtractor().extract(prose);            // 1. extract (AI)
const reviewed = review(draft).approve("I1").amend("I5", "...").answer("D1", 0).done(); // 2. review (human)
const frozen = freeze(reviewed);                                          //    hash
const plan = await new ClaudeProofDeriver().derive(frozen, model);        // 3. derive (AI)
validatePlan(frozen, plan);                                               //    deterministic gate
const report = await verify(frozen, plan, { subject: decideLogin });      // 4. verify
const rust = (await loadRustProver(wasmBytes)).prove(plan, await tabulate(plan.model, decideLogin)); // independent check
console.log(renderReceipt(report, plan, rust));                           // 5. receipt
```

See [`DESIGN.md`](DESIGN.md) for the semantics of each verdict and where proof stops.

## Status

v0. "Proved" means exhaustive checking over a finite declared domain. An SMT backend (via `toSmtLib`) and a
CLI/GitHub Action are the next steps. The Claude adapters are typed against the Anthropic SDK but have not
been exercised against the live API yet.
