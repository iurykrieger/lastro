# B5 — Skill Wrappers (Execution + Heal)

> Source plan: [`plan.md`](plan.md) §5 (Skills table rows 4–8)

The five slash commands that wrap the runtime: `/run-sensor`, `/start-sensor`, `/stop-sensor`, `/validate-use-case`, `/heal`. Each is a thin skill definition + `scripts/` calling the Go runtime built in B1–B3.

## Branching (mandatory)

```bash
git fetch origin
git checkout -b feat/b5-skill-wrappers origin/main
```

## Parallelism

- **Can run in parallel with:** B4 (different code path), B6 (CLI is a sibling surface).
- **Must run after:** B1 (per-use-case aggregator for `/validate-use-case`), B2 (lifecycle for `/run-sensor`, `/start-sensor`, `/stop-sensor`), B3 (heal loop for `/heal`).
- **Blocks:** B7 (examples + dogfood).

### Sub-ordering within the chunk

The five skills don't have to land together:

1. `/run-sensor`, `/start-sensor`, `/stop-sensor` — unblocked once B2 lands. Near-1:1 wrappers over `lifecycle.RunSensor` / `StartSensor` / `StopSensor`.
2. `/validate-use-case` — unblocked once B1 + B2 land. Orchestrates a topo-sorted run of all sensors for a use case, then per-use-case aggregator.
3. `/heal` — unblocked once B3 lands. Implements B3's `LLMClient` interface; prompt body lives in the skill file.

## Scope

In:
- `skills/run-sensor/`, `skills/start-sensor/`, `skills/stop-sensor/` — synchronous (run/stop) / asynchronous-spawn (start) wrappers over `internal/lifecycle`.
- `skills/validate-use-case/` — orchestrates: read use case + sensors from `.harness/` → topo-sort via `internal/sensor.ResolveExecutionOrder` → run via lifecycle → feed `[]AggregateSignal` into `internal/runtime/aggregator/usecase` → emit `UseCaseVerdict`.
- `skills/heal/` — consumes a failing `Signal`/`AggregateSignal` id, builds the prompt, calls the LLM, hands the proposed `EditPlan` to `internal/runtime/healloop.Run`.

Out:
- Runtime logic (B1–B3).
- CLI surface (B6) — sibling, not dependency.
- Long-term handle storage (B2 owns the format; this chunk reads/writes via lifecycle).

## Inputs / Outputs

Match plan §5:

| Skill | Input | Output |
|---|---|---|
| `/run-sensor` | `Sensor.id` | streamed `Signal`s + terminal `AggregateSignal` (blocking) |
| `/start-sensor` | `Sensor.id` | sensor handle |
| `/stop-sensor` | handle | terminal `AggregateSignal` with completeness |
| `/validate-use-case` | `UseCase.id` | aggregated signals + `UseCaseVerdict` |
| `/heal` | failing `Signal`/`AggregateSignal` id | `EditPlan` (applied by `healloop`) + final verdict |

## Dependencies

- B1, B2, B3.
- Phase A entity types (all 9).
- Skill files ≤ 200 lines (CLAUDE.md rule 4); procedural detail goes into `scripts/`, shared logic into `lib/`.

## Open questions for `/brainstorming`

1. **Skill-script transport.** Options: (a) shared `harness` binary (B6) with hidden internal subcommands, (b) per-skill `go run`, (c) per-skill compiled binary in `scripts/`. Recommendation: (a) once B6 lands; (b) interim. Avoids duplicate runtime wire-up.
2. **`/validate-use-case` parallelism.** Run independent sensor branches concurrently where the DAG allows, or strictly serial? Recommendation: parallel — matches B2's design.
3. **Handle format for `/stop-sensor`.** Opaque short id (read via lifecycle's sidecar lookup) vs structured JSON. Recommendation: opaque short id.
4. **`/heal` = `LLMClient` impl.** Confirm scripts implement B3's `LLMClient` interface and that prompt text lives in the skill body (so prompt edits don't require recompile).
5. **Failure surface.** When `/run-sensor` returns `AggregateSignal.verdict=fail`, does the skill exit non-zero? Recommendation: yes — composability with shell pipelines and CI.

## Deliverable acceptance

- `/run-sensor <id>` on a passing assertion sensor exits 0, prints streamed signals + terminal `AggregateSignal` JSON to stdout.
- `/run-sensor <id>` on a failing assertion sensor exits non-zero; `AggregateSignal.heal_hint` is non-empty (populated by `internal/aggregate/synthesize.go`).
- `/start-sensor <id>` returns a handle; subsequent `/stop-sensor <handle>` emits an `AggregateSignal` whose `completeness` reflects whether expected observations were seen.
- `/validate-use-case <id>` orchestrates a 3-sensor use case (mix of assertion + observational); produces a `UseCaseVerdict` matching the per-use-case aggregator's output.
- `/heal` against a known-failing signal in a sample repo produces an `EditPlan`, applies it via `healloop`, re-validates, and exits with the resulting verdict.
