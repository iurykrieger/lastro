# E7 — Signal

> Source plan: [`plan.md`](plan.md) §4.5 (Signal schema), §2 ("Signals are the contract with the LLM")

A `Signal` is one record emitted by a sensor during execution — a verdict, evidence, and (on failure) a `heal_hint` block. Signals are JSON Lines on stdout. This chunk owns the schema, the Go type, a JSON Lines parser, and the validator.

## Scope

In:
- The `Signal` schema and Go type.
- A JSON Lines parser: `func ParseSignals(r io.Reader) iter.Seq2[Signal, error]` (or channel-based equivalent).
- Validator: required fields, valid `verdict`, valid `angle`, confidence in [0.0, 1.0], `heal_hint` required iff `verdict == fail`.
- Tests.

Out:
- Signal *emission* (sensors emit; runtime owns the writer — Phase B).
- Signal *aggregation* (E8 — AggregateSignal owns rollup).
- Signal *consumption* by the heal loop (Phase B).

## Schema (from plan §4.5)

```json
{
  "schema_version": "1.0.0",
  "sensor_id": "...",
  "use_case_id": "...",
  "angle": "e2e-test",
  "emitted_at": "<iso8601>",
  "verdict": "pass | fail | inconclusive",
  "confidence": 0.0,
  "evidence": { "expected": "...", "actual": "...", "fixture_id": "..." },
  "heal_hint": {
    "summary": "<one-line actionable instruction>",
    "suggested_locus": [{"path": "src/...", "symbol": "..."}],
    "rationale": "<short, structured — not raw logs>"
  }
}
```

## Inputs / Outputs

- **Input:** a stream of JSON Lines (from a sensor's stdout or a test fixture).
- **Output:** `internal/signal/` Go package — types, streaming parser, validator.

## Dependencies

- E1 (enums) — `ValidationAngle`.
- Schema-freeze gate.

**Coordination note for parallel work:** E7 produces the typed `Signal` that E8 (AggregateSignal) consumes for rollup. E8 should depend on E7's type, not duplicate it. Both can develop in parallel since E8 can stub a `[]Signal` slice for testing rollup.

## Open questions for `/brainstorming`

1. **JSON or YAML.** Plan §4.5 example is JSON; signals are JSON Lines. Confirm: signal *files* (for testing) are JSON Lines, not YAML — different from every other schema in the framework. Recommendation: yes, JSON Lines for signals/aggregates. They're machine-emitted, not human-authored.
2. **Streaming vs batch.** Should the parser stream (one signal at a time, low memory) or batch (load all signals, then iterate)? Recommendation: stream, because some sensors emit hundreds of signals (e.g., unit-test runner — one per test).
3. **Malformed lines.** When one JSON Line in a stream is malformed, does the parser (a) skip and continue, (b) emit an error sentinel and continue, (c) abort? Recommendation: (b) — return `(Signal, error)` per line; downstream decides whether to abort.
4. **`heal_hint` required-when-fail enforcement.** Strict (load fails) or warning-only? Plan §4.5 says required. Recommendation: strict — the LLM contract depends on it.
5. **`evidence` schema flexibility.** Plan shows `{expected, actual, fixture_id}` but the angles differ wildly (e.g., a `logs` sensor's evidence isn't `expected/actual`). Should `evidence` be free-form `map[string]any`, or a discriminated union per angle? Recommendation: `map[string]any` with a few well-known keys documented — too early to lock per-angle shapes.
6. **`confidence` for computational sensors.** Plan §3.4: computational sensors are deterministic. Should they always emit `confidence: 1.0`, or omit the field? Recommendation: always `1.0` (explicit), so consumers don't branch on presence.

## Deliverable acceptance

- `internal/signal/` parses a fixture JSON Lines file containing pass/fail/inconclusive signals.
- Negative tests: missing `verdict`, fail without `heal_hint`, confidence out of range, unknown angle.
- Streaming test: parser yields signals as bytes arrive, doesn't load whole stream into memory.
- Round-trip: parse → re-encode → parse again is stable.
