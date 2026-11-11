# Lastro: notes for agents

Lastro turns prose into reviewed intents, derives proof obligations from them, and checks those
obligations against what the code actually does. Read `DESIGN.md` before changing semantics.

## Layout

Every SDK lives in `sdks/<language>`; a new language port goes there too.

- `crates/lastro-core`: Rust prover. Independent second engine; builds as a CLI (`lastro-prove`) and as WASM.
- `sdks/typescript`: TypeScript SDK (`lastro` on npm). Orchestrates extraction, review, plans, verification, receipts.
- `sdks/dotnet`: .NET SDK (`Lastro`, `Lastro.Claude`, `Lastro.Cli`). A port of the TS SDK with the same wire format,
  hashes, verdicts, receipts and prompts; the CLIs share one `.lastro/` workspace.
- `sdks/go`: Go SDK and `lastro-go` CLI. Same data model, hashes and `.lastro/` workspace as the TS SDK.
- `apps/sandbox`: Vite + React + shadcn/ui sandbox (prose, code, contrast) running the real SDK and the WASM prover.

## Rules

- All repository content (code, comments, docs, commit messages) is in US English.
- Never `eval` spec expressions. LSL is a JSON AST; both engines interpret it.
- "Proved" means exhaustive over a finite declared domain. Anything sampled is "evidenced", never "proved".
- The TS engine and the Rust prover must agree verdict by verdict. When you change one, change the other
  and keep the cross-check test in `sdks/typescript/test/lastro.test.ts` green.
- The .NET SDK must match the TS SDK (hashes, verdicts, receipts, prompts and schemas). When you change semantics,
  a prompt or the CLI workspace in one, change the other; `sdks/dotnet/tests/Lastro.Tests/InteropTests.cs` checks it.
- The Go SDK ports the TS engine and must produce identical reports and hashes (`sdks/go/crosscheck_test.go`).
  Its prompts are generated from the TS SDK: after changing a prompt or schema, run `go generate ./...` in `sdks/go`.
- Verification is deterministic: fixed seeds, canonical JSON, hashes over frozen artifacts.
- Lastro's only AI steps are intent extraction and proof derivation. Never add code generation to the SDK:
  implementations come from any agent or person, and Lastro judges them.
- Everything the framework generates or keeps (the use-case catalog, intents, specs, proofs, receipts, LLM handoff
  files) lives in the `.lastro/` folder. Never write framework files elsewhere.
- `.lastro/use-cases.json` is the catalog of Lastro's current use cases (`slug`, `name`, `description`), each description
  stating the complete behavior as the source implements it. When a change adds, alters or removes behavior,
  update the catalog in the same change; slugs are stable kebab-case ids.
- Stay harness-agnostic: no dependency on a specific agent product. New surfaces wrap `createLastro`.

## Commands

```sh
cargo test -p lastro-core && cargo clippy -p lastro-core -- -D warnings
npm install && npm run wasm          # builds sdks/typescript/wasm/lastro_core.wasm
npm test -w lastro                   # SDK tests, including the Rust cross-check
npm run sandbox                      # dev server for the sandbox
dotnet test sdks/dotnet          # .NET SDK tests (cross-checks need the TS build and lastro-prove)
cargo build --release -p lastro-core && (cd sdks/go && go test ./...)   # Go SDK, cross-checked with TS and Rust
```
