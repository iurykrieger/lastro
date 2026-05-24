# B6 — CLI (`cmd/harness`)

> Source plan: [`plan.md`](plan.md) §8 (Repo layout), §9 Phase 6 (DX & ergonomics)

The user-facing `harness` binary: `harness validate`, `harness heal`, and (pending the open question below) optionally `harness detect`. Same underlying Go entry points as the skills (B5), different surface. Sibling to the existing `cmd/validate-schemas/` utility.

**Detection note:** detection is LLM-driven (B4). The CLI cannot do it deterministically. Whether `harness detect` exists at all is the first open question — see below.

## Branching (mandatory)

```bash
git fetch origin
git checkout -b feat/b6-cli origin/main
```

## Parallelism

- **Can run in parallel with:** B3 (heal loop — `harness heal` integrates once B3 lands; `validate` is unblocked earlier), B4 (only relevant if `harness detect` ends up being model (a) — see open questions), B5 (sibling surface).
- **Must run after:** B1, B2 (`harness validate` invokes B2's lifecycle + B1's aggregator). `harness heal` additionally needs B3. If `harness detect` exists, it additionally needs B4.
- **Blocks:** B7 (examples + dogfood scripts).

## What Phase A and current main already expose

| Need | Already present | Note |
|---|---|---|
| Schema validation utility | `cmd/validate-schemas/main.go` | Sibling binary; reuses `schemas/` embed pattern — copy the wire-up style |
| YAML→JSON + schema validation | `github.com/santhosh-tekuri/jsonschema/v6` + `sigs.k8s.io/yaml` | Same libraries; consistency matters |

Choose a mature CLI library per CLAUDE.md rule 5 (recommend `github.com/spf13/cobra`).

## Scope

In:
- `cmd/harness/main.go` — `main` package, subcommand dispatch.
- Subcommands:
  - `harness validate [--use-case <id>|--all]` — invokes `internal/lifecycle` + `internal/runtime/aggregator/usecase`; pretty-prints results; non-zero on any fail. Fully deterministic.
  - `harness heal [--signal <id>|--all-failing]` — invokes `internal/runtime/healloop`. Heal loop itself is deterministic loop control; the edit-proposal call to the LLM happens through the same `LLMClient` abstraction B3 defines.
  - `harness detect [--cache]` — **open question, see below.** Detection is LLM-driven (B4); the CLI cannot run it without either (a) embedding an LLM API client or (b) deferring to the Claude Code slash commands. Decide before implementing.
- Flags: `--output {json,text}`, `--quiet`, `--policy <path>`, `--max-heal-iterations <n>`.
- Structured logging via a mature library (`slog` stdlib, or `zerolog`).
- Detection result caching (plan §9 Phase 6) **iff `harness detect` exists**: caches the *LLM-produced YAML* under `.harness/cache/`, keyed by content hash of the source tree relevant to each skill.

Out:
- Skill surface (B5).
- Runtime logic (B1–B3) and B4 skill scripts — imports and composes only.
- Web UI / dashboard.
- Re-implementing detection inference in Go (B4's hard rule applies here too — if `harness detect` exists, it must drive an LLM, not infer in Go).

## Inputs / Outputs

| Subcommand | Input | Output | Exit |
|---|---|---|---|
| `harness validate` | `--use-case <id>` or `--all` | per-use-case `UseCaseVerdict` (text or JSON) | 0 pass / 1 fail / 2 inconclusive |
| `harness heal` | `--signal <id>` or `--all-failing` | applied edits + re-validation verdict | 0 healed / 1 exhausted / 2 abandoned |
| `harness detect` (if it exists) | repo root, optional `--cache` | `.harness/stack-manifest.yaml`, `use-cases/*.yaml`, `fixtures/*.yaml`, `sensors/*.yaml` | 0 success / 1 LLM-call or validation failure |

## Dependencies

- B1, B2 always; B3 for `harness heal`; B4 only if `harness detect` is implemented.
- Phase A entity packages.
- Cobra (or equivalent) for subcommands; `slog` for logging.
- *(Conditional on `harness detect` model (a))* Anthropic SDK + API key handling.

## Open questions for `/brainstorming`

1. **Does `harness detect` exist at all?** Three viable answers:
   - **(a) Yes, with embedded LLM client.** CLI uses the Anthropic API directly, loads the prompt body from `skills/<name>/skill.md`, drives the same validate-then-persist path as the slash command. Pro: works in CI without Claude Code. Con: CLI gains a hard dependency on an API key + the SDK.
   - **(b) Yes, as a thin shim that defers to the slash commands.** CLI prints "run `/detect-stack` in Claude Code" or similar. Effectively a stub. Pro: zero LLM coupling in Go. Con: doesn't actually do anything.
   - **(c) No, drop the subcommand.** Detection only happens via slash commands; CI uses `harness validate` (which reads pre-detected `.harness/`). Pro: cleanest separation. Con: deviates from plan §9 Phase 6 which lists `harness detect`.
   Recommendation: (a) for CI usability, but flag the SDK dependency and the API-key concern. Resolve in brainstorm.
2. **CLI framework choice.** Cobra (heavyweight, de facto) vs urfave/cli/v3 (lighter) vs stdlib `flag`. Recommendation: Cobra.
3. **Output schema.** JSON output for `validate` — match `UseCaseVerdict` exactly or wrap with run metadata (timing, sensor count)? Recommendation: wrap. CI tooling will want timing.
4. **Caching scope.** If `harness detect` exists: cache stack-manifest only (slow + stable) by default; opt-in for use-cases + fixtures.
5. **`harness validate --all` orchestration.** Run use cases in parallel bounded by `GOMAXPROCS`; sensors within a use case follow the DAG (matches B2).
6. **Exit code semantics.** Standardize 0/1/2 across subcommands; document in `--help`.
7. **Skill ↔ CLI parity.** Define the integration test that asserts the CLI and the equivalent skill produce byte-identical JSON for the same input (only applies to subcommands the CLI actually owns).

## Deliverable acceptance

- `harness --help` and `harness <subcommand> --help` produce sensible output.
- `harness validate --all` on a passing sample exits 0; on a deliberately-broken sample exits 1 with failure summary listing the failing sensors.
- `harness heal --all-failing` on the broken sample applies the LLM-proposed fix, re-validates, exits 0.
- `harness validate --output json` produces parseable JSON matching a documented schema.
- Cross-surface integration test: same inputs → CLI subcommand and equivalent `/`-skill produce identical JSON (for subcommands the CLI actually owns).
- *(If `harness detect` is implemented per model (a))* on a sample repo, it drives the LLM to produce a valid `stack-manifest.yaml` + use cases + fixtures, then exits 0. Validation errors from the embedded LLM call surface non-zero with the same structured error format B4's scripts use.
