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
- `plugins/lastro`: one plugin for Claude Code and Codex (skills, hooks, progress board) that wraps the `lastro` CLI.
  Root marketplaces: `.claude-plugin/marketplace.json` (Claude Code) and `.agents/plugins/marketplace.json` (Codex).
  This repository enables both plugins for itself in `.claude/settings.json`; `.lastro/plugin.json` turns the plugin on
  (`"enabled": true`): code edits wait for an approved proposal, as for any user of the plugin.
- `plugins/lastro-board`: a Claude Code mod (pane, band, toasts) that draws the same board live from `.lastro/`.
- `apps/sandbox`: Vite + React + shadcn/ui sandbox (prose, code, contrast) running the real SDK and the WASM prover.
- `apps/review`: Vite + React + shadcn/ui web app to browse use cases, proofs and proposals and review intents together
  (approve, amend, reject, decisions, threads, after rewrites). A Vite plugin (`server/api.ts`) serves `.lastro/` (or `LASTRO_DIR`)
  through the TS SDK's `reviewProposal`; no review logic of its own. Playwright tests in `e2e/` run on a fixture workspace.
- `apps/docs`: Vite + React + shadcn/ui developer docs. Pages live in `src/pages`, the sidebar in `src/content/nav.ts`.
  Code samples come from `examples/shipping`, which `test/` runs through the real SDKs; the interactive examples in `src/live`
  run the real TS SDK and the WASM prover in the browser (build it with `npm run wasm` first); Playwright UI tests are in `e2e/`.

## Rules

- All repository content (code, comments, docs, commit messages) is in US English.
- Never `eval` spec expressions. LSL is a JSON AST; both engines interpret it.
- "Proved" means exhaustive over a finite declared domain. Anything sampled is "evidenced", never "proved".
- The TS engine and the Rust prover must agree verdict by verdict. When you change one, change the other
  and keep the cross-check test in `sdks/typescript/test/lastro.test.ts` green.
- The .NET SDK must match the TS SDK (hashes, verdicts, receipts, prompts and schemas). When you change semantics,
  a prompt or the CLI workspace in one, change the other; `sdks/dotnet/tests/Lastro.Tests/InteropTests.cs` checks it.
  Its proposal prompts are generated from the TS SDK: after changing one, run `node sdks/dotnet/tools/gen-prompts.mjs`.
- The Go SDK ports the TS engine and must produce identical reports and hashes (`sdks/go/crosscheck_test.go`).
  Its prompts are generated from the TS SDK: after changing a prompt or schema, run `go generate ./...` in `sdks/go`.
- Verification is deterministic: fixed seeds, canonical JSON, hashes over frozen artifacts.
- Lastro's only AI steps are intent extraction and proof derivation. Never add code generation to the SDK:
  implementations come from any agent or person, and Lastro judges them.
- Everything the framework generates or keeps (use cases, proposals, intents, proofs, receipts, LLM handoff
  files) lives in the `.lastro/` folder. Never write framework files elsewhere.
- Lastro's own use cases live in `.lastro/usecases/<id>-<slug>.json` (`id`, `slug`, `name`, `description`, `history`),
  each description stating the complete behavior as the source implements it; their proofs live one per file in
  `.lastro/proof/<proof-id>-<slug>.json`, and only the CLI writes them (`apply`, `derive --usecases`, `prove --usecases`). When a change adds, alters
  or removes behavior, update those files in the same change. Ids (`UC-001`...) are never reused; slugs are stable
  kebab-case. Changes are written as proposals in `.lastro/proposals/<id>-<slug>.json` (see
  `docs/proposals/intents-as-use-case-changes.md` for the four entities).
- Stay harness-agnostic: no dependency on a specific agent product. New surfaces wrap `createLastro`.
- When you change the CLI, an SDK's public API or a verdict, update `apps/docs` in the same change
  (`npm run test:examples -w @lastro/docs` catches drift in the quickstart and the library examples).
- UI changes (sandbox, docs, review) come with Playwright tests. Never commit screenshots.

## Commands

```sh
cargo test -p lastro-core && cargo clippy -p lastro-core -- -D warnings
npm install && npm run wasm          # builds sdks/typescript/wasm/lastro_core.wasm
npm test -w lastro                   # SDK tests, including the Rust cross-check
npm run sandbox                      # dev server for the sandbox
npm run docs                         # dev server for the docs (apps/docs)
npm run build -w lastro && npm run test:examples -w @lastro/docs   # docs examples through the real SDK
npm run test:e2e -w @lastro/docs     # docs UI tests (Playwright)
npm run review                       # the review app on this repository's .lastro/ (LASTRO_DIR=... for another)
npm run test:e2e -w @lastro/review   # review app UI tests (Playwright, on a fixture workspace)
npm test -w @lastro/plugin           # Claude Code / Codex plugin hooks, through the real CLI
claude plugin test plugins/lastro-board   # the Claude Code mod's tests
dotnet test sdks/dotnet          # .NET SDK tests (cross-checks need the TS build and lastro-prove)
cargo build --release -p lastro-core && (cd sdks/go && go test ./...)   # Go SDK, cross-checked with TS and Rust
npm run build:sandbox && npm run test:e2e -w @lastro/sandbox   # sandbox UI tests (Playwright)
node sdks/typescript/dist/src/cli.js prove   # Lastro's own proofs (.lastro/models, subjects in sdks/typescript/subjects and plugins/lastro/subjects)
```
