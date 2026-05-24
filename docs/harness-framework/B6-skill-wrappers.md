# B6 — Skill Wrappers (Execution + Heal)

> Source plan: [`plan.md`](plan.md) §5 (Skills table rows 4–8)

The five slash commands that wrap the runtime: `/run-sensor`, `/start-sensor`, `/stop-sensor`, `/validate-use-case`, `/heal`. Each is a thin skill definition + `scripts/` calling into the Go runtime built in B2–B4.

## Branching (mandatory)

Before starting any work on this chunk:

```bash
git fetch origin
git checkout -b feat/b6-skill-wrappers origin/main
```

## Parallelism

- **Can run in parallel with:** B4 (heal loop runtime — but `/heal` sub-skill cannot integrate until B4 lands; the other four skills are unblocked), B7 (CLI is a sibling surface), B5 (different code path).
- **Must run after:** B2 (per-use-case aggregator for `/validate-use-case`), B3 (lifecycle for `/run-sensor`, `/start-sensor`, `/stop-sensor`), B4 (heal loop for `/heal`).
- **Blocks:** B8 (examples + dogfood).

### Implementation order within this chunk

The five skills don't need to land together. Recommended sub-order if multiple contributors:

1. `/run-sensor`, `/start-sensor`, `/stop-sensor` — unblocked as soon as B3 merges. Smallest skills; nearly 1:1 wrappers over `lifecycle.RunSensor` / `StartSensor` / `StopSensor`.
2. `/validate-use-case` — unblocked after B2 + B3; orchestrates a topo-sorted run of all sensors for one use case, then per-use-case aggregator.
3. `/heal` — unblocked after B4; thinnest LLM-interaction skill, satisfies the `LLMClient` interface B4 defined.

## Scope

In:
- `skills/run-sensor/` — slash command + scripts. Synchronous; blocks until terminal `AggregateSignal`.
- `skills/start-sensor/` — spawns the watcher via lifecycle; returns the `Handle` (printed as a stable id).
- `skills/stop-sensor/` — terminates the watcher; emits terminal `AggregateSignal`.
- `skills/validate-use-case/` — orchestrates: resolve sensors for the use case → topological run via lifecycle → per-use-case aggregator → emit `UseCaseVerdict`. This skill is the heal loop's re-validation target.
- `skills/heal/` — slash command + scripts. Consumes a failing `Signal`/`AggregateSignal` id, builds the prompt, calls the LLM, hands the proposed `EditPlan` back to `healLoop.Run`.

Out:
- Runtime logic (B2–B4).
- CLI surfaces (B7) — the same Go entry points may be wrapped by both, but the skill scripts here should call the Go runtime directly, not shell out to the CLI.
- Long-term handle storage (B3 owns the persistence format; this chunk reads/writes it).

## Inputs / Outputs

Match plan §5 exactly:

| Skill | Input | Output |
|---|---|---|
| `/run-sensor` | `Sensor.id` | `Signal`s + terminal `AggregateSignal` (blocking) |
| `/start-sensor` | `Sensor.id` | sensor handle |
| `/stop-sensor` | handle | terminal `AggregateSignal` with completeness |
| `/validate-use-case` | `UseCase.id` | aggregated signals + `UseCaseVerdict` |
| `/heal` | failing `Signal`/`AggregateSignal` id | `EditPlan` (proposal — applied by `healLoop`) |

## Dependencies

- B2, B3, B4.
- Phase A entity types.
- The skill files themselves are ≤ 200 lines (CLAUDE.md rule 4) — any procedural detail goes into the skill's `scripts/`, with logic shared across two+ skills promoted to `lib/`.

## Open questions for `/brainstorming`

1. **Skill-script transport.** Scripts call into Go via (a) a single shared binary with a hidden subcommand surface, (b) per-skill `go run` invocations, (c) one binary per skill compiled in `scripts/`? Recommendation: (a) — same Go binary as the CLI (B7), but exposed via internal-only flags. Avoids duplicating the runtime wire-up.
2. **`/validate-use-case` parallelism.** Run independent sensor branches in parallel (where the DAG allows), or strictly serial? Recommendation: parallel — matches B3's design.
3. **Handle serialization for `/stop-sensor`.** Handle as opaque opaque string or structured JSON? Recommendation: opaque short id; the lookup goes through `.harness/handles/`.
4. **`/heal` skill = LLMClient impl.** Confirm the skill scripts satisfy B4's `LLMClient` interface and that prompt text lives in the skill file (so prompt edits don't require recompile). Recommendation: yes.
5. **Failure surface.** When `/run-sensor` returns an `AggregateSignal` with `verdict=fail`, does the skill exit non-zero? Recommendation: yes — composability with shell pipelines and CI.

## Deliverable acceptance

- `/run-sensor <id>` on a passing assertion sensor exits 0, prints the streamed signals + terminal `AggregateSignal` JSON to stdout.
- `/run-sensor <id>` on a failing assertion sensor exits non-zero, terminal `AggregateSignal.heal_hint` is non-empty.
- `/start-sensor <id>` returns a handle; subsequent `/stop-sensor <handle>` emits an `AggregateSignal` whose `completeness` field reflects whether expected observations were seen.
- `/validate-use-case <id>` orchestrates a 3-sensor use case (mix of assertion + observational), produces a `UseCaseVerdict` matching the per-use-case aggregator's output.
- `/heal` against a known-failing signal in a sample repo produces an `EditPlan`, applies it via `healLoop`, re-validates, and exits with the resulting verdict.
