# B1 — Composed Runtime (Fixture Binder + Per-Use-Case Aggregator)

> Source plan: [`plan.md`](plan.md) §6.1 (Fixture Binder), §6.3 (Verdict aggregation)
>
> Supersedes pre-rebase B1 + B2. Phase A delivered the template resolver, sensor DAG resolver, signal stream parser, per-sensor aggregator/rollup, heal-hint synthesis, and two-scope policy resolution — all the "primitives" the old B1 was scoped around. This chunk owns what remains: fixture binding and per-use-case aggregation.

## Branching (mandatory)

```bash
git fetch origin
git checkout -b feat/b1-composed-runtime origin/main
```

## Parallelism

- **Can run in parallel with:** B4 (detection + generation — totally separate code path).
- **Must run after:** Phase A (✓).
- **Blocks:** B2 (executor calls the fixture binder per step), B3 (heal loop calls the per-use-case aggregator for re-validation), B5 (`/validate-use-case` invokes the per-use-case aggregator).

## What Phase A already delivered (consume, do not rebuild)

| Need | Existing package | Key API |
|---|---|---|
| `${{ }}` template interpolation | `internal/usecase/template` | `(*Resolver).Resolve(segs)`, `ResolveValue` |
| Sensor DAG topological sort | `internal/sensor` | `ResolveExecutionOrder(sensors)` |
| Per-sensor terminal `AggregateSignal` | `internal/aggregate` | `Rollup(RollupInput) (AggregateSignal, error)` |
| Heal-hint synthesis (stream + observational) | `internal/aggregate` | `synthesize*HealHint(...)` invoked by `Rollup` |
| JSONL signal streaming | `internal/signal` | `ParseSignals(io.Reader) iter.Seq2[Signal, error]` |
| Two-scope policy resolution | `internal/policy` | `Resolve(global, local *ValidationPolicy) *EffectivePolicy` |

If any of those need extension, do it in the owning package, not here.

## Scope

In:
- **Fixture binder** — given a sensor step's `uses: [<Fixture.id>]`, resolve to concrete payloads and expose to the step. Uses `internal/usecase/template.Resolver` to interpolate any template tokens *inside* a fixture payload. Exposes payloads as env vars (small) + file mounts (large/binary).
- **Per-use-case aggregator** — given the set of `AggregateSignal`s belonging to one use case (already terminal, already with heal hints), apply the resolved `EffectivePolicy`, compute the use-case verdict + weighted confidence per plan §6.3.

Out:
- The per-sensor rollup (already in `internal/aggregate`).
- Heal-hint generation (already in `internal/aggregate/synthesize.go`).
- Stream parsing (already in `internal/signal`).
- The executor that *invokes* fixture binder per step (B2).
- Heal-loop orchestration that *consumes* the use-case verdict (B3).

## Package layout (proposed)

```
internal/runtime/
├── fixturebinder/        # depends on usecase/template + fixture
└── aggregator/
    └── usecase/          # depends on aggregate + policy + usecase
```

`internal/runtime/` does not exist yet; create it here.

## Inputs / Outputs

| Function | Input | Output |
|---|---|---|
| `fixturebinder.Bind` | `Sensor.steps[i].uses`, owning `UseCase` + `[]Fixture`, `template.Resolver` | `StepBinding{Env map[string]string, Files map[string]string}` |
| `aggregator.UseCase` | `UseCase`, `[]aggregate.AggregateSignal`, `*policy.EffectivePolicy` | `UseCaseVerdict{Verdict, Confidence, ObligatorySatisfied, FailingAngles []enums.ValidationAngle, AggregatedHint *aggregate.HealHint}` |

## Open questions for `/brainstorming`

1. **Fixture exposure surface.** Env vars only, files only, or both? Recommendation: both — small payloads as `HARNESS_FIXTURE_<ID>` env, large/binary as file paths via `HARNESS_FIXTURE_<ID>_PATH`.
2. **Template-inside-fixture semantics.** Plan §2 lets use case text reference fixtures via `${{ fixtures.<id> }}`. Can a fixture payload itself contain template tokens referencing other fixtures? Recommendation: no — keep the resolver one-pass and acyclic. Flag any cycle.
3. **Verdict weighting.** Plan §6.3 says confidence is weighted `(nature: computational=1.0, inferential=aggregate.confidence)`. Confirm: inferential signals' weight = their own confidence (so low-confidence signals contribute less to the use-case verdict). Add a test asserting this.
4. **Inconclusive floor.** Plan §10.3 (default `0.7`). The per-use-case aggregator should honor it from the `EffectivePolicy`. Check whether `policy.EffectivePolicy` already carries this field (the E9 work added two-scope resolution; the floor may or may not be exposed yet). If not, extend `internal/policy` here.
5. **Failure-first vs all-results aggregation.** Short-circuit on first obligatory `fail`, or evaluate every sensor? Recommendation: always evaluate — the heal loop (B3) needs the full failure surface.
6. **AggregatedHint shape.** Each per-sensor `AggregateSignal` already carries a `HealHint`. When multiple sensors in one use case fail, should the per-use-case verdict surface a *list* of hints or a *consolidated* hint? Recommendation: list — let B3's heal loop iterate; consolidation would lose locus precision.

## Deliverable acceptance

- `fixturebinder.Bind` resolves a step with 2 fixtures, surfaces them via env + file, errors cleanly on missing fixture id and on template-in-fixture cycle.
- `aggregator.UseCase` correctly applies a fixture policy: 3 obligatory angles all pass → use case pass; one obligatory fails → use case fail; only optional fails → use case pass with `FailingAngles` populated.
- Golden test: fixed use case + policy + 3 mocked `AggregateSignal`s → byte-identical `UseCaseVerdict` JSON across runs (determinism).
- Tests run with `-race` clean.
