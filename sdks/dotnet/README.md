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

The building blocks are public too: `Artifacts` (create, review, approve, carry a review over), `Plans`
(validation), `Engine` (verify, explain a run, consistency), `Lsl` and `L` (the spec language), `D` and
`Domains`, `Receipt`, `LlmPrompts` and `RustProver`.

## CLI

```sh
dotnet tool install --global --add-source ./nupkg Lastro.Cli   # after `dotnet pack`
lastro init --model model.json --approvals 1
lastro plan prose.md
lastro review --as ana approve-all
lastro spec
lastro prove ./bin/MyApp.dll --export MyApp.Login.Decide
lastro feedback
```

Every command prints one JSON object and keeps state in `.lastro/`. Exit codes: 0 ok, 1 not ok, 2 error,
3 an LLM answer is needed. With `ANTHROPIC_API_KEY` set the AI steps run on Claude; otherwise (`--llm handoff`)
the CLI writes the request to `.lastro/llm/`, exits 3, and the calling agent answers with its own model.

`prove` loads a public static method from an assembly. It takes one argument, either `Lastro.Env` or a
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

The prompts in `src/Lastro/LlmPrompts.cs` are generated from `sdks/typescript/src/llm/llm.ts`. When you change a
prompt or schema in one SDK, change it in the other: the handoff-id test fails until they match.
