# B6 — CLI (`cmd/harness`)

> Source plan: [`plan.md`](plan.md) §8 (Repo layout), §9 Phase 6 (DX & ergonomics)

The user-facing `harness` binary: `harness detect`, `harness validate`, `harness heal`. Same underlying Go entry points as the skills (B5), different surface. Sibling to the existing `cmd/validate-schemas/` utility.

## Branching (mandatory)

```bash
git fetch origin
git checkout -b feat/b6-cli origin/main
```

## Parallelism

- **Can run in parallel with:** B3 (heal loop — `harness heal` integrates once B3 lands; `detect` and `validate` are unblocked), B5 (sibling surface).
- **Must run after:** B1, B2, B4 (`harness detect` invokes B4 skills' validators; `harness validate` invokes B2's lifecycle; both use B1's aggregator for output). `harness heal` additionally needs B3.
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
  - `harness detect [--cache]` — invokes the LLM via B4 skills' Go-validator entry points; writes `.harness/`.
  - `harness validate [--use-case <id>|--all]` — invokes `internal/lifecycle` + `internal/runtime/aggregator/usecase`; pretty-prints results; non-zero on any fail.
  - `harness heal [--signal <id>|--all-failing]` — invokes `internal/runtime/healloop` on one or all failing signals.
- Flags: `--output {json,text}`, `--quiet`, `--policy <path>`, `--max-heal-iterations <n>`.
- Structured logging via a mature library (`slog` stdlib, or `zerolog`).
- Detection result caching (plan §9 Phase 6): caches the *LLM-produced YAML* under `.harness/cache/`, keyed by content hash of the source tree relevant to each skill.

Out:
- Skill surface (B5).
- Runtime logic (B1–B3) and detection validators (B4) — imports and composes only.
- Web UI / dashboard.

## Inputs / Outputs

| Subcommand | Input | Output | Exit |
|---|---|---|---|
| `harness detect` | repo root, optional `--cache` | `.harness/stack-manifest.yaml`, `use-cases/*.yaml`, `fixtures/*.yaml`, `sensors/*.yaml` | 0 success / 1 detection failure |
| `harness validate` | `--use-case <id>` or `--all` | per-use-case `UseCaseVerdict` (text or JSON) | 0 pass / 1 fail / 2 inconclusive |
| `harness heal` | `--signal <id>` or `--all-failing` | applied edits + re-validation verdict | 0 healed / 1 exhausted / 2 abandoned |

## Dependencies

- B1, B2, B4 always; B3 for `harness heal`.
- Phase A entity packages.
- Cobra (or equivalent) for subcommands; `slog` for logging.

## Open questions for `/brainstorming`

1. **CLI framework choice.** Cobra (heavyweight, de facto) vs urfave/cli/v3 (lighter) vs stdlib `flag`. Recommendation: Cobra.
2. **Output schema.** JSON output for `validate` — match `UseCaseVerdict` exactly or wrap with run metadata (timing, sensor count)? Recommendation: wrap. CI tooling will want timing.
3. **Caching scope.** Cache stack-manifest only (slow + stable) by default; opt-in for use-cases + fixtures.
4. **`harness validate --all` orchestration.** Run use cases in parallel bounded by `GOMAXPROCS`; sensors within a use case follow the DAG (matches B2).
5. **Exit code semantics.** Standardize 0/1/2 across subcommands; document in `--help`.
6. **Skill ↔ CLI parity.** Define the integration test that asserts the CLI and the equivalent skill produce byte-identical JSON for the same input.

## Deliverable acceptance

- `harness --help` and `harness <subcommand> --help` produce sensible output.
- `harness detect` on a sample repo writes a valid `stack-manifest.yaml` + use cases + fixtures, exits 0.
- `harness validate --all` on a passing sample exits 0; on a deliberately-broken sample exits 1 with failure summary listing the failing sensors.
- `harness heal --all-failing` on the broken sample applies the LLM-proposed fix, re-validates, exits 0.
- `harness validate --output json` produces parseable JSON matching a documented schema.
- Cross-surface integration test: same inputs → CLI subcommand and `/`-skill produce identical JSON.
