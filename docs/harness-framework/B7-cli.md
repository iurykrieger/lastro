# B7 — CLI (`cmd/harness`)

> Source plan: [`plan.md`](plan.md) §8 (Repo layout), §9 Phase 6 (DX & ergonomics)

The user-facing `harness` binary: `harness detect`, `harness validate`, `harness heal`. Same underlying Go entry points as the skills (B6), different surface.

## Branching (mandatory)

Before starting any work on this chunk:

```bash
git fetch origin
git checkout -b feat/b7-cli origin/main
```

## Parallelism

- **Can run in parallel with:** B4 (heal loop runtime — `harness heal` cannot integrate until B4 lands, but `detect` and `validate` are unblocked), B6 (sibling surface — both wrap the same Go code).
- **Must run after:** B2, B3, B5 (`harness detect` invokes B5's detection; `harness validate` invokes B3's lifecycle; both need B2's aggregator for output formatting). `harness heal` additionally needs B4.
- **Blocks:** B8 (examples + dogfood scripts).

## Scope

In:
- `cmd/harness/` — `main` package, subcommand dispatch via a mature CLI library (per CLAUDE.md rule 5 — prefer mature Go libs; e.g. `github.com/spf13/cobra` or `github.com/urfave/cli/v3`).
- Subcommands:
  - `harness detect [--cache]` — runs `/detect-stack` + `/detect-use-cases` flow; writes outputs to repo.
  - `harness validate [--use-case <id>|--all]` — runs `/validate-use-case` flow for one or all use cases; pretty-prints results, exits non-zero on any fail.
  - `harness heal [--signal <id>|--all-failing]` — runs the heal loop on one or all failing signals.
- Optional flags: `--output {json,text}`, `--quiet`, `--policy <path>`, `--max-heal-iterations <n>`.
- Structured logging via a mature library (`zerolog`, `slog`, etc. — per CLAUDE.md rule 5).
- Detection result caching (plan §9 Phase 6): on-disk cache keyed by repo content hash; `--cache` opt-in or default with `--no-cache` override.

Out:
- The skill surface (B6).
- Runtime logic (B2–B4) and detection logic (B5) — this chunk imports and composes, doesn't reimplement.
- Web UI, dashboard, anything beyond text/JSON output.

## Inputs / Outputs

| Subcommand | Input | Output | Exit code |
|---|---|---|---|
| `harness detect` | repo root, optional `--cache` | `stack-manifest.yaml`, `use-cases/*.yaml`, `fixtures/*.yaml` written; summary on stdout | 0 on success, 1 on detection failure |
| `harness validate` | `--use-case <id>` or `--all` | per-use-case verdict on stdout (text or JSON) | 0 if all pass, 1 if any fail, 2 if inconclusive (no obligatory verdict) |
| `harness heal` | `--signal <id>` or `--all-failing` | applied edits + re-validation result | 0 healed, 1 exhausted, 2 abandoned |

## Dependencies

- B2, B3, B5 always; B4 for `harness heal`.
- Phase A entity packages.
- A mature CLI framework (Cobra recommended for subcommand depth + auto-generated help).

## Open questions for `/brainstorming`

1. **CLI framework choice.** Cobra (de facto standard, heavyweight) vs urfave/cli/v3 (lighter, modern) vs stdlib `flag`. Recommendation: Cobra — subcommand structure + completion + help generation outweighs the dep weight.
2. **Output schema.** JSON output for `validate` — match `UseCaseVerdict` exactly, or wrap with run metadata (timing, sensor count)? Recommendation: wrap. Tooling will want timing.
3. **Caching scope.** Cache stack-manifest only, or use-cases + fixtures too? Recommendation: stack-manifest by default (slow + stable); use-case re-detection is cheap and changes with code edits.
4. **`harness validate --all` orchestration.** Run use cases in parallel? Recommendation: yes, bounded by `GOMAXPROCS`; sensors within a use case follow the DAG.
5. **Exit code semantics.** Standardize across subcommands (0=success, 1=expected failure, 2=infrastructure error) or per-subcommand? Recommendation: standardize; document in `--help`.
6. **Skill ↔ CLI parity.** Confirm B6's skills and this CLI invoke the same Go entry points — define the integration test that asserts they produce byte-identical JSON output for the same input.

## Deliverable acceptance

- `harness --help` and `harness <subcommand> --help` produce sensible output.
- `harness detect` on a sample repo writes a valid `stack-manifest.yaml` + use cases + fixtures, exits 0.
- `harness validate --all` on a passing sample repo exits 0; on a deliberately-broken sample exits 1 with failure summary listing the failing sensors.
- `harness heal --all-failing` on the broken sample applies the LLM-proposed fix, re-validates, exits 0.
- `harness validate --output json` produces parseable JSON matching a documented schema.
- Cross-surface integration test: same inputs → CLI subcommand and skill produce identical JSON.
