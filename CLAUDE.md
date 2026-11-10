# Lastro: notes for agents

Lastro turns prose into reviewed intents, derives proof obligations from them, and checks those
obligations against what the code actually does. Read `DESIGN.md` before changing semantics.

## Layout

- `crates/lastro-core`: Rust prover. Independent second engine; builds as a CLI (`lastro-prove`) and as WASM.
- `packages/sdk`: TypeScript SDK (`lastro` on npm). Orchestrates extraction, review, plans, verification, receipts.
- `packages/dotnet`: .NET SDK (`Lastro`, `Lastro.Claude`, `Lastro.Cli`). A port of the TS SDK with the same wire format,
  hashes, verdicts, receipts and prompts; the CLIs share one `.lastro/` workspace.
- `apps/sandbox`: Vite + React + shadcn/ui sandbox (prose, code, contrast) running the real SDK and the WASM prover.

## Rules

- All repository content (code, comments, docs, commit messages) is in US English.
- Never `eval` spec expressions. LSL is a JSON AST; both engines interpret it.
- "Proved" means exhaustive over a finite declared domain. Anything sampled is "evidenced", never "proved".
- The TS engine and the Rust prover must agree verdict by verdict. When you change one, change the other
  and keep the cross-check test in `packages/sdk/test/lastro.test.ts` green.
- The .NET SDK must match the TS SDK (hashes, verdicts, receipts, prompts and schemas). When you change semantics,
  a prompt or the CLI workspace in one, change the other; `packages/dotnet/tests/Lastro.Tests/InteropTests.cs` checks it.
- Verification is deterministic: fixed seeds, canonical JSON, hashes over frozen artifacts.
- Lastro's only AI steps are intent extraction and proof derivation. Never add code generation to the SDK:
  implementations come from any agent or person, and Lastro judges them.
- Stay harness-agnostic: no dependency on a specific agent product. New surfaces wrap `createLastro`.

## Commands

```sh
cargo test -p lastro-core && cargo clippy -p lastro-core -- -D warnings
npm install && npm run wasm          # builds packages/sdk/wasm/lastro_core.wasm
npm test -w lastro                   # SDK tests, including the Rust cross-check
npm run sandbox                      # dev server for the sandbox
dotnet test packages/dotnet          # .NET SDK tests (cross-checks need the TS build and lastro-prove)
```
