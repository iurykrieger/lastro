# Shipping fee example

The example behind the docs' quickstart. `test/walkthrough.test.mjs` runs it through the real `lastro` CLI
(`--llm handoff`), answering the two AI steps with the files in `answers/`, so the commands and outputs shown
in the docs stay true.

| File | What it is |
|---|---|
| `prose.md` | The business rule, in prose |
| `model.json` | The vocabulary of the proofs: inputs, outputs and their domains |
| `answers/extract.json` | An answer to the `extract` handoff request (what a model would return) |
| `answers/derive.json` | An answer to the `derive` handoff request |
| `answers/bootstrap.json` | An answer to the `survey` handoff request of `lastro bootstrap` (the same rule, read from the code) |
| `shipping-buggy.mjs` | An implementation with an off-by-one bug |
| `shipping.mjs` | The fixed implementation |
| `go/` | The same decision core as a Go program that speaks JSON lines, for `lastro-go prove --cmd` |
| `library.ts` | The whole pipeline as TypeScript library calls (`node library.ts` with `ANTHROPIC_API_KEY`) |
| `go-library/` | The same in Go (`go run .`); `go test` replays `answers/` offline |
| `dotnet/` | The same in .NET (`dotnet run`); `dotnet run -- --replay` replays `answers/` offline |

All three produce the same intent and plan hashes and the same receipt as the CLI.
