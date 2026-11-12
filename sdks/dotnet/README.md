# Lastro for .NET

The .NET SDK for Lastro: prose → reviewed intents → derived proofs → a deterministic check of the code
against them. It is a port of the TypeScript SDK in [`sdks/typescript`](../typescript), not a wrapper around it, and the
two agree byte for byte where it matters:

- the same wire format (camelCase JSON, kebab-case enums), so a `.lastro/` workspace written by one CLI is
  read and continued by the other;
- the same canonical JSON and SHA-256 hashes, so an artifact approved by one SDK is intact for the other and a
  plan has the same hash in both;
- the same verdicts, receipts and seeded sampling (mulberry32), checked by the test suite against the
  TypeScript engine and the Rust prover;
- the same LLM prompts and schemas, so a `--llm handoff` request has the same id in both CLIs.

Lastro's only AI steps are intent extraction and proof derivation. It never writes the implementation: any
coding agent or person does, and `ProveAsync` judges it.

| Project | What it is |
|---|---|
| `src/Lastro` | The SDK (`Lastro` package). No dependencies beyond the .NET 8 base library. |
| `src/Lastro.Claude` | Claude transport for the AI steps, over the official Anthropic C# SDK (`Lastro.Claude` package). |
| `src/Lastro.Cli` | The `lastro` command line, as a .NET tool. Same commands and workspace as the TypeScript CLI. |
| `tests/Lastro.Tests` | xUnit tests, including the cross-checks with the TypeScript SDK and the Rust prover. |

## Library

```csharp
using Lastro;
using Lastro.Claude;

var lastro = new LastroClient(new LastroOptions
{
    Model = model,                        // inputs, outputs and defs of the decision core
    Ask = ClaudeAsk.Create(),             // or any AskJson transport, or a custom Extractor/Deriver
    RequiredApprovals = 2,                // your team's policy; default 1
    Rust = RustProver.Locate(),           // optional independent prover (lastro-prove)
});

var intents = await lastro.PlanAsync(prose);                                  // AI: extract reviewable intents
intents = lastro.Review(intents, "ana").ApproveAll().Amend("I5", "...").Done();
intents = lastro.Review(intents, "bruno").ApproveAll().Done();
lastro.Pending(intents);                                                      // [] once the policy is met

var spec = await lastro.SpecAsync(intents);            // AI: derive clauses; then validated, checked for contradictions
if (!spec.Ok) intents = lastro.ReturnDecisions(spec);  // contradictions come back as decisions;
                                                       // each answer becomes an intent to approve

var proof = await lastro.ProveAsync(spec, input => DecideLogin(input)); // every input, plus the Rust prover if given
proof.Ok;                                              // gate a merge on it
proof.Receipt;                                         // Markdown for the pull request
Feedback.AgentFeedback(proof);                         // failures with counterexamples, for the coding agent
```

Proposals (the flow the CLI runs) are in `Proposals`, with the same rules and hashes as the TypeScript SDK:

```csharp
var p = await Proposals.ExtractAsync(ask, new ProposeInput("P-001", prose, "ana", useCases, new ProposalPolicy()));
p = Proposals.Review(p, "ana").Answer("D1", 1).ApproveAll().Done();
p = Proposals.Approve(p, useCases);                                      // bound to the use cases' baseline
p = await Proposals.DeriveAsync(ask, p, useCases, new ProofSetup(models, ["shipping_fee"]));
var (proved, run) = await Proposals.ProveAsync(p, useCases, new RunOptions(subjects, models, RustProver.Locate()));
var applied = Proposals.Apply(proved, useCases);                         // the use cases after the change
```

The building blocks are public too: `Artifacts` (create, review, approve, carry a review over), `Plans`
(validation), `Engine` (verify, explain a run, consistency), `Lsl` and `L` (the spec language), `D` and
`Domains`, `Receipt`, `LlmPrompts` and `RustProver`.

## CLI

```sh
dotnet tool install --global --add-source ./nupkg Lastro.Cli   # after `dotnet pack`
lastro init --model model.json --subject ./bin/MyApp.dll#MyApp.Shipping.Fee --approvals 1
lastro propose prose.md --as ana             # a proposal (P-001) and its intents, against .lastro/usecases/
lastro review P-001 --as ana approve-all     # or amend / reject / answer / comment / reply / resolve / rewrite / recombine
lastro derive P-001                          # the proofs of the approved intents
lastro prove P-001                           # its proofs plus every use case's (regression); appends to runs.jsonl
lastro feedback P-001
lastro apply P-001                           # attach the proofs to their use cases
lastro regress                               # every use case's proofs alone
```

Every command prints one JSON object and keeps state in `.lastro/`. Exit codes: 0 ok, 1 not ok, 2 error,
3 an LLM answer is needed. With `ANTHROPIC_API_KEY` set the AI steps run on Claude; otherwise (`--llm handoff`)
the CLI writes the request to `.lastro/llm/`, exits 3, and the calling agent answers with its own model.

`prove` and `regress` load a public static method from an assembly (`<assembly.dll> --export Namespace.Type.Method`,
or the subject recorded by `init --subject`). The method takes one argument, either `Lastro.Env` or a
dictionary of string to object (integers arrive as `long`), and returns the same kinds, optionally as a
`Task`. The assembly does not need to reference Lastro.

## Development

```sh
cargo build --release -p lastro-core          # lastro-prove, for the Rust cross-check
npm install && npx tsc -p sdks/typescript        # the TypeScript SDK, for the TS cross-check
dotnet test sdks/dotnet
```

Without node or `lastro-prove`, the cross-check tests are skipped. CI sets `LASTRO_REQUIRE_CROSSCHECK=1`,
which turns a skip into a failure.

The prompts in `src/Lastro/LlmPrompts.cs` follow `sdks/typescript/src/llm/llm.ts`, and `src/Lastro/ProposalPrompts.g.cs` is
generated from `sdks/typescript/src/llm/proposal.ts` (`npm run build -w lastro && node sdks/dotnet/tools/gen-prompts.mjs`).
When you change a prompt or schema in one SDK, change it in the other: the handoff tests fail until they match.
