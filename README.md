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
| [`sdks/typescript`](sdks/typescript) | TypeScript SDK (`lastro`): intent artifacts and review, LSL spec language (compiles to SMT-LIB2), plan validation, verification engine, receipts, Claude adapters, and the bridge to the Rust prover. |
| [`sdks/dotnet`](sdks/dotnet) | .NET SDK and CLI. Same wire format, hashes, verdicts, receipts and prompts as the TypeScript SDK, so both CLIs work on one `.lastro/` workspace. |
| [`sdks/go`](sdks/go) | Go SDK and the `lastro-go` CLI. Same data model, hashes and `.lastro/` workspace as the TypeScript SDK; its engine is cross-checked against the TS engine and the Rust prover. |
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
  break. Fix the code by hand (or paste the "Feedback for the coding agent" into any agent) and watch them pass.
  This needs Claude: it works in the sandbox published on claude.ai, or locally with `ANTHROPIC_API_KEY` set.
- **Review as a team**: switch between Ana and Bruno and set the policy to two approvals per intent.
- **Plant a bug** in the code panel and watch the clause that catches it fail in both engines, with a counterexample you can run.
- **Approve the AI's wrong version of I5** in the prose panel: the plan changes, and the reference code now violates it.
- **Turn on the production trace**: the model still proves, but the trusted piece T2 (bcrypt) is broken by a real run.
- Open the **Receipt** tab for the Markdown that would go into a pull request.

## The interface

Lastro does two AI things: it extracts intents from prose, and it derives the proofs. Everything else is
deterministic. It never writes the implementation: any coding agent (Claude Code, Codex, ...) or a person does,
and Lastro judges the result. The same pipeline is available as a library and as a CLI, so it fits a script,
a CI job, an app or any agent harness.

```
You ──plan──▶ intents ──review (your policy)──▶ spec ──prove──▶ proof + receipt
                 ▲                                │               │
                 └──── contradictions: decisions ─┘               └── failures ──▶ whoever writes the code
```

### Library

```ts
import { createLastro, agentFeedback } from "lastro";
import { claudeAsk } from "lastro/claude";            // or any AskJson transport

const lastro = createLastro({ model, ask: claudeAsk(), requiredApprovals: 2 }); // your team's policy; default 1

let intents = await lastro.plan(prose);               // AI: extract reviewable intents
intents = lastro.review(intents, "ana").approveAll().amend("I5", "...").done();
intents = lastro.review(intents, "bruno").approveAll().done();
lastro.pending(intents);                              // [] once the policy is met

const spec = await lastro.spec(intents);              // AI: derive clauses; then validated, checked for contradictions
if (!spec.ok) intents = lastro.returnDecisions(spec); // contradictions come back as decisions;
                                                      // each answer becomes an intent to approve

const proof = await lastro.prove(spec, decideLogin);  // deterministic: every input, plus the Rust prover if given
proof.ok;                                             // gate a merge on it
proof.receipt;                                        // Markdown for the pull request
agentFeedback(proof);                                 // failures with counterexamples, for the coding agent
```

### CLI (any harness)

Every command prints one JSON object and keeps state in `.lastro/`. Exit codes: 0 ok, 1 not ok, 2 error,
3 an LLM answer is needed.

```sh
lastro init --model model.json --approvals 1
lastro plan prose.md                      # extract intents
lastro review --as ana approve-all        # approve | amend | reject | answer
lastro spec                               # derive the formal spec; contradictions become decisions
lastro prove ./decide-login.js            # proof.json + receipt.md; exit 1 with failures
lastro feedback                           # the failures as a brief for the coding agent
```

The AI steps run on the Anthropic API when `ANTHROPIC_API_KEY` is set. Otherwise (`--llm handoff`) the CLI
writes the request (instructions, data and JSON Schema) to `.lastro/llm/`, exits 3, and the calling agent
answers with its own model and runs the command again. No API key or harness-specific plugin is needed.

See [`DESIGN.md`](DESIGN.md) for the semantics of each verdict and where proof stops.

## Status

v0. "Proved" means exhaustive checking over a finite declared domain. An SMT backend (via `toSmtLib`) and a
CLI/GitHub Action are the next steps. The Claude adapters are typed against the Anthropic SDK but have not
been exercised against the live API yet.
