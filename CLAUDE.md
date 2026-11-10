# Lastro: notes for agents

Lastro turns prose into reviewed intents, derives proof obligations from them, and checks those
obligations against what the code actually does. Read `DESIGN.md` before changing semantics.

## Layout

- `crates/lastro-core`: Rust prover. Independent second engine; builds as a CLI (`lastro-prove`) and as WASM.
- `packages/sdk`: TypeScript SDK (`lastro` on npm). Orchestrates extraction, review, plans, verification, receipts.
- `apps/sandbox`: Vite + React + shadcn/ui sandbox (prose, code, contrast) running the real SDK and the WASM prover.

## Rules

- All repository content (code, comments, docs, commit messages) is in US English.
- Never `eval` spec expressions. LSL is a JSON AST; both engines interpret it.
- "Proved" means exhaustive over a finite declared domain. Anything sampled is "evidenced", never "proved".
- The TS engine and the Rust prover must agree verdict by verdict. When you change one, change the other
  and keep the cross-check test in `packages/sdk/test/lastro.test.ts` green.
- Verification is deterministic: fixed seeds, canonical JSON, hashes over frozen artifacts.

## Commands

```sh
cargo test -p lastro-core && cargo clippy -p lastro-core -- -D warnings
npm install && npm run wasm          # builds packages/sdk/wasm/lastro_core.wasm
npm test -w lastro                   # SDK tests, including the Rust cross-check
npm run sandbox                      # dev server for the sandbox
```
