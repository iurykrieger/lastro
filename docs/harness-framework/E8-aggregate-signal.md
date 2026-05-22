# E8 — AggregateSignal

> Source plan: [`plan.md`](plan.md) §4.6 (AggregateSignal schema), §6.3 (Verdict aggregation)

An `AggregateSignal` is the terminal record emitted by every sensor execution — always exactly one, always last. For single-shot sensors it mirrors the lone signal; for stream sensors it rolls up counts and verdict; for observational sensors it reports observation completeness. This chunk owns the schema, the type, and the rollup function that turns a `[]Signal` into an `AggregateSignal`.

## Scope

In:
- The `AggregateSignal` schema and Go type.
- Loader/parser (it's also JSON, last record in a JSON Lines stream).
- Validator: required fields, valid `termination_reason`, `rollup` consistency (counts sum correctly), `completeness` required for observational sensors.
- **The rollup function:** `Rollup(signals []Signal, kind SensorKind, ...) AggregateSignal`.
  - Single-shot: `total_signals == 1`, aggregate verdict = sole signal's verdict.
  - Stream assertion: `verdict = pass` iff `fail_count == 0 && inconclusive_count == 0`.
  - Observational: `verdict = fail` if `missing_observations` is non-empty.
- Tests.

Out:
- Per-use-case aggregation across multiple sensors (that's the runtime's per-use-case aggregator — Phase B).
- The runtime that *invokes* rollup at sensor termination (Phase B).

## Schema (from plan §4.6)

Key fields:
- `schema_version`, `type: "aggregate"`, `sensor_id`, `use_case_id`, `angle`
- `started_at`, `ended_at`, `termination_reason`: `completed | stopped | timeout | error`
- `verdict`, `confidence`
- `rollup: {total_signals, pass_count, fail_count, inconclusive_count}`
- `completeness: {expected_observations, missing_observations}` — observational only
- `heal_hint` — required when `verdict == fail`

## Inputs / Outputs

- **Input:** an array of `Signal` (E7) + execution metadata (`started_at`, `ended_at`, `kind`, `expected_observations`).
- **Output:** `internal/aggregate/` Go package — types, parser, validator, `Rollup()` function.

## Dependencies

- E1 (enums) — `ValidationAngle`, `SensorKind`.
- E7 (Signal) — for the input type to `Rollup`.
- Schema-freeze gate.

## Open questions for `/brainstorming`

1. **Rollup determinism.** Plan rules:
   - Single-shot: aggregate.verdict = sole signal's verdict
   - Stream assertion: pass iff zero failures AND zero inconclusives
   - Observational: fail if `missing_observations` non-empty
   - **What about timeout?** Plan §6.2 says observational with timeout → `inconclusive`. Confirm: `inconclusive` overrides `pass` if termination was timeout and observation coverage is unclear. Where exactly is that decision made — in Rollup or in the runtime?
2. **Confidence aggregation.** Plan §6.3 gives a formula at the *per-use-case* layer. What about per-sensor (E8's responsibility)? Recommendation: weighted average of signal confidences; if computational and stream, all 1.0 → aggregate is 1.0.
3. **`heal_hint` synthesis.** When the aggregate verdict is `fail`, where does the `heal_hint` come from? Options: (a) carry over the first failing signal's heal_hint, (b) synthesize a meta-hint summarizing N failures, (c) require the runtime to provide one. Recommendation: (b) for stream sensors (e.g., "70 of 700 unit tests failed; see individual signals for detail"), (a) for single-shot.
4. **Validator for `rollup` arithmetic.** Should the load-time validator enforce `pass_count + fail_count + inconclusive_count == total_signals`? Recommendation: yes — corrupted aggregates should not parse.
5. **Observational `completeness` source.** Where does `expected_observations` come from? It's not in the Sensor schema (plan §4.4) but it's needed at execution time. Open question for sensor execution design (Phase B); for E8, treat it as an input parameter to `Rollup`.

## Deliverable acceptance

- `internal/aggregate/` parses golden examples for each rollup mode.
- `Rollup` tested against:
  - Single-shot pass + fail + inconclusive
  - Stream assertion with all-pass, mixed, all-fail
  - Observational with full coverage, partial coverage, timeout
- Negative tests: bad arithmetic, missing required fields, fail-without-heal_hint.
