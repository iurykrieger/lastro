# B2 — Composed Runtime

> Source plan: [`plan.md`](plan.md) §6.1 (Fixture Binder, Aggregator), §6.3 (Verdict aggregation), §4.6 (AggregateSignal)

The mid-layer runtime: fixture binding, per-sensor aggregation, per-use-case aggregation. Each composes one or more primitives from B1.

## Branching (mandatory)

Before starting any work on this chunk:

```bash
git fetch origin
git checkout -b feat/b2-composed-runtime origin/main
```

If B1 hasn't merged yet, branch off `main` anyway and rebase once B1 lands. Do not branch off `feat/b1-*` — your tree will inherit unreviewed code.

## Parallelism

- **Can run in parallel with:** B5 (detection + generation).
- **Must run after:** B1 (imports `internal/runtime/template`, `internal/runtime/signalCollector`, `internal/policy`).
- **Blocks:** B3 (executor invokes fixture binder + per-sensor aggregator), B4 (heal loop calls per-use-case aggregator for re-validation), B6 (`/validate-use-case` invokes per-use-case aggregator).

## Scope

In:
- `internal/runtime/fixtureBinder/` — given a sensor step's `uses: [<Fixture.id>]`, resolve to concrete payloads, expose to the step (env vars + optional file mounts). Uses B1's template package to interpolate payload-internal references.
- `internal/runtime/aggregator/sensor/` — consume the `Signal` stream from B1's signal collector, emit exactly one terminal `AggregateSignal` per sensor run. Handles both `output_type: single-shot` and `stream`, both `kind: assertion` and `kind: observational`.
- `internal/runtime/aggregator/usecase/` — consume the set of `AggregateSignal`s belonging to one use case, apply the `ValidationPolicy` resolution from B1, compute the use-case verdict + weighted confidence per plan §6.3.

Out:
- Step execution itself (B3).
- Long-lived watcher lifecycle (B3 — observational sensors are spawned by lifecycle, but emit signals consumed by per-sensor aggregator here).
- Heal hint consumption (B4).

## Inputs / Outputs

| Package | Input | Output |
|---|---|---|
| `fixtureBinder` | `Sensor.steps[i].uses`, owning `UseCase` + `[]Fixture` | `StepBinding{Env map[string]string, Files map[string]string}` |
| `aggregator/sensor` | `Sensor`, `<-chan Signal`, completion event | one `AggregateSignal` |
| `aggregator/usecase` | `UseCase`, `[]AggregateSignal`, `policy.Resolved` | `UseCaseVerdict{Verdict, Confidence, ObligatorySatisfied, FailingAngles []ValidationAngle}` |

## Dependencies

- B1 (template, signalCollector, policy).
- Phase A: `internal/sensor`, `internal/fixture`, `internal/usecase`, `internal/signal`, `internal/aggregate` (the type — this chunk adds the rollup logic; check current state).

**Coordination note for parallel work:** if B1 hasn't merged, develop against the contract (interfaces) listed in B1's "Inputs / Outputs" table and stub. Integration test once B1 is in.

## Open questions for `/brainstorming`

1. **Fixture exposure surface.** Env vars only, files only, or both? Recommendation: both — small payloads as `HARNESS_FIXTURE_<ID>` env, large/binary as file paths via `HARNESS_FIXTURE_<ID>_PATH`. Implementer chooses threshold.
2. **Aggregator completeness for observational sensors.** Plan §6.2 says observational `verdict` distinguishes `pass` / `fail` (missing expected observations) / `inconclusive` (timeout). What's the "expected observations" source? Recommendation: derived from sensor metadata (currently underspecified — flag as a Phase B refinement to schema E6).
3. **Confidence weighting.** Plan §6.3: `weighted average … weighted by (nature: computational=1.0, inferential=aggregate.confidence)`. Reread that — does it mean inferential signals' weight = their own confidence (so low-confidence signals contribute less)? Recommendation: yes; specify in the doc and add a test asserting this behavior.
4. **Inconclusive floor.** Plan open question §10.3 (default `0.7`). Per-use-case aggregator should honor this from the resolved policy. Confirm policy schema (E9) carries this field; if not, propose extension.
5. **Failure-first vs all-results aggregation.** Per-use-case: short-circuit on first obligatory `fail`, or always evaluate every sensor for full reporting? Recommendation: always evaluate — the heal loop needs the full failure surface to propose multi-file fixes.

## Deliverable acceptance

- `fixtureBinder` resolves a step with 2 fixtures, surfaces them via env + file, errors cleanly on missing fixture id.
- `aggregator/sensor` produces a deterministic terminal `AggregateSignal` from a 10-signal stream; `verdict` correctly reflects `any fail → fail`, all pass → pass.
- `aggregator/usecase` correctly applies policy: 3 obligatory angles all pass → use case pass; one obligatory fails → use case fail; only optional fails → use case pass with `FailingAngles` populated.
- Golden test exercises the full chain with a fixed use case + policy + 3 sensors; asserts byte-identical `UseCaseVerdict` JSON across runs (determinism).
