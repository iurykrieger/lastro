# E8 — AggregateSignal (Design)

> Source chunk: [`docs/harness-framework/E8-aggregate-signal.md`](../../harness-framework/E8-aggregate-signal.md)
> Plan reference: [`plan.md`](../../harness-framework/plan.md) §4.6, §6.2, §6.3
> Sequential gate consumed: [`docs/harness-framework/00-schema-freeze.md`](../../harness-framework/00-schema-freeze.md)
> Companion chunk (parallel): [`docs/harness-framework/E7-signal.md`](../../harness-framework/E7-signal.md)
> Brainstorm date: 2026-05-23

## 1. Purpose

An `AggregateSignal` is the terminal record of every sensor execution —
always exactly one, always last. This chunk owns:

- The `AggregateSignal` Go type and JSON shape.
- A loader (`ParseAggregate`) and validator (`Validate`).
- The deterministic `Rollup` function that turns a `[]signal.Signal`
  plus execution metadata into an `AggregateSignal`.

E8 does **not** own the runtime that invokes `Rollup` (Phase B), nor
per-use-case aggregation across multiple sensors (separate entity).

## 2. Cross-entity ripple (read first)

Brainstorming surfaced a new verdict value — `warn` — that does not
exist in the original plan. It is a "pass-grade outcome with concerns
worth addressing" and avoids overloading `fail` for non-blocking issues.
This ripples through three places that are not under E8's exclusive
ownership:

| Owner | Change |
|---|---|
| Schema-freeze gate | `schemas/enums/verdicts.yaml` adds `warn` to the value list |
| E1 enums (Go) | `Verdict` const set adds `VerdictWarn`; `AllVerdicts()`, `IsValidVerdict` follow automatically from the YAML |
| E7 Signal | `Signal.verdict` enum extended; `heal_hint` required-when rule extended from `verdict == fail` to `verdict ∈ {warn, fail}` |
| E8 AggregateSignal (this chunk) | Same enum + same heal_hint rule; `rollup` block gains a `warn_count` field |

**Coordination:** the schema-freeze gate and E1 must merge the extended
`Verdict` enum before E8 implementation lands. E7 and E8 can develop in
parallel: both consume the enum from `internal/enums`, and E8 imports
the `Signal` and `HealHint` types from `internal/signal` without
duplication.

## 3. Scope

**In:**

- Go package at `internal/aggregate/`:
  - `AggregateSignal`, `Rollup` (the input options struct is named
    `RollupInput` to avoid collision with the `Rollup` function),
    `RollupCounts`, `Completeness` structs.
  - `HealHint` re-exported as a type alias to `signal.HealHint`.
  - `ParseAggregate(r io.Reader) (AggregateSignal, error)`.
  - `Validate(a AggregateSignal) error`.
  - `Rollup(in RollupInput) (AggregateSignal, error)` — the deterministic
    rollup function.
- Golden JSON examples under `internal/aggregate/testdata/`.
- Table-driven unit tests for parser, validator, and rollup.

**Out:**

- The runtime that invokes `Rollup` at sensor termination (Phase B).
- Per-use-case aggregation across sensors (plan §6.3 — separate entity).
- Where `ExpectedObservations` originates inside the Sensor schema
  (plan §4.4 open question — for E8, it is a `RollupInput` field).
- Streaming or batch consumers downstream of the heal loop.

## 4. JSON Shape

The on-the-wire record is the last JSON Lines record a sensor writes.
The discriminator `"type": "aggregate"` distinguishes it from `Signal`
records sharing the same stream.

```json
{
  "schema_version": "1.0.0",
  "type": "aggregate",
  "sensor_id": "<Sensor.id>",
  "use_case_id": "<UseCase.id>",
  "angle": "unit-test",

  "started_at": "2026-05-23T14:00:00Z",
  "ended_at":   "2026-05-23T14:00:42Z",
  "termination_reason": "completed",

  "verdict":    "fail",
  "confidence": 0.92,

  "rollup": {
    "total_signals":      700,
    "pass_count":         628,
    "warn_count":           2,
    "fail_count":          70,
    "inconclusive_count":   0
  },

  "completeness": {
    "expected_observations": ["app-started", "log-emitted"],
    "missing_observations":  []
  },

  "heal_hint": {
    "summary": "70 of 700 unit tests failed",
    "suggested_locus": [
      { "path": "src/payments/charge.ts", "symbol": "Charge.compute" }
    ],
    "rationale": "see individual fail signals for per-test detail"
  }
}
```

Fields by visibility:

- **Always present:** `schema_version`, `type`, `sensor_id`,
  `use_case_id`, `angle`, `started_at`, `ended_at`,
  `termination_reason`, `verdict`, `confidence`, `rollup`.
- **Conditional:**
  - `completeness` — present iff sensor is observational
    (caller signals this through `RollupInput.Kind`; `Rollup` omits
    the block for non-observational kinds).
  - `heal_hint` — required iff `verdict ∈ {warn, fail}`; forbidden
    otherwise.

## 5. Go types

```go
package aggregate

import (
    "time"

    "github.com/iurykrieger/lastro/internal/enums"
    "github.com/iurykrieger/lastro/internal/signal"
)

type AggregateSignal struct {
    SchemaVersion     string                 `json:"schema_version"`
    Type              string                 `json:"type"` // always "aggregate"
    SensorID          string                 `json:"sensor_id"`
    UseCaseID         string                 `json:"use_case_id"`
    Angle             enums.ValidationAngle  `json:"angle"`
    StartedAt         time.Time              `json:"started_at"`
    EndedAt           time.Time              `json:"ended_at"`
    TerminationReason enums.TerminationReason `json:"termination_reason"`
    Verdict           enums.Verdict          `json:"verdict"`
    Confidence        float64                `json:"confidence"`
    Rollup            RollupCounts           `json:"rollup"`
    Completeness      *Completeness          `json:"completeness,omitempty"`
    HealHint          *HealHint              `json:"heal_hint,omitempty"`
}

type RollupCounts struct {
    TotalSignals      int `json:"total_signals"`
    PassCount         int `json:"pass_count"`
    WarnCount         int `json:"warn_count"`
    FailCount         int `json:"fail_count"`
    InconclusiveCount int `json:"inconclusive_count"`
}

type Completeness struct {
    ExpectedObservations []string `json:"expected_observations"`
    MissingObservations  []string `json:"missing_observations"`
}

// HealHint mirrors signal.HealHint exactly; aliased to avoid duplication.
type HealHint = signal.HealHint

type RollupInput struct {
    Signals              []signal.Signal
    SensorID             string
    UseCaseID            string
    Angle                enums.ValidationAngle
    Kind                 enums.SensorKind         // assertion | observational
    OutputType           enums.SignalOutputType   // single-shot | stream
    StartedAt            time.Time
    EndedAt              time.Time
    TerminationReason    enums.TerminationReason  // completed | stopped | timeout | error
    ExpectedObservations []string                 // observational only; may be nil
    ObservedKeys         []string                 // observational only; keys actually seen
}
```

## 6. Rollup logic

### 6.1 Verdict decision tree

Executed top-to-bottom; first match wins.

1. **Observational + missing_observations non-empty** → `fail`.
   (Computed first because it overrides everything: a sensor that
   failed to do its job cannot report any other verdict.)
2. **TerminationReason == `error`** →
   - any `signal.verdict == fail` → `fail`
   - else → `inconclusive`
3. **TerminationReason == `timeout`** → same rule as `error`.
4. **OutputType == `single-shot`** (clean termination) →
   sole signal's verdict, mirrored.
5. **Stream / observational with clean termination:**
   - any `fail` → `fail`
   - else any `warn` → `warn`
   - else any `inconclusive` → `inconclusive`
   - else → `pass`

   Severity ordering rationale (`fail > warn > inconclusive > pass`):
   `warn` represents a *determined* outcome (the sensor decided something
   is off but non-blocking), while `inconclusive` represents *missing
   information*. We rank a known minor issue above unknown signal-by-
   signal noise, so an aggregate with one warn and one inconclusive
   reports `warn` — the actionable verdict — rather than burying it
   behind an unactionable `inconclusive`.

### 6.2 Counts

```
total_signals      = len(Signals)
pass_count         = count(Signals where verdict == pass)
warn_count         = count(Signals where verdict == warn)
fail_count         = count(Signals where verdict == fail)
inconclusive_count = count(Signals where verdict == inconclusive)
```

Invariant: `pass + warn + fail + inconclusive == total`. The validator
enforces it.

### 6.3 Confidence

- Normal case: arithmetic mean of `signal.Confidence` across all
  signals.
- `Signals` empty (legitimate for observational sensors that observed
  nothing):
  - verdict `pass` → confidence `1.0`
  - verdict `fail` (from missing_observations) → confidence `1.0`
    (we *know* observations are missing — high certainty about the
    failure, not the underlying behavior)
  - verdict `inconclusive` → confidence `0.0`

Rationale: avoids divide-by-zero, and avoids over-stating confidence
for a sensor that produced no signals.

### 6.4 Completeness

- Observational: `MissingObservations = ExpectedObservations \ ObservedKeys`
  (set difference, preserving the order of `ExpectedObservations`).
  Resulting `Completeness` block is always emitted, even when empty.
- Non-observational: the `Completeness` block is omitted (`nil` pointer
  in Go → absent in JSON).

### 6.5 heal_hint synthesis

| Case | Strategy |
|---|---|
| `verdict == pass` | No `heal_hint`; pointer left nil. |
| `verdict ∈ {warn, fail}`, `OutputType == single-shot` | Carry over the sole signal's `HealHint` verbatim (deep copy). |
| `verdict ∈ {warn, fail}`, `OutputType == stream` or observational | Synthesize (see below). |

Synthesis for stream/observational:

- `Summary`:
  - fail verdict: `"<fail_count> of <total_signals> <angle> signals failed"`
  - warn verdict: `"<warn_count> warnings across <total_signals> <angle> signals"`
  - observational fail with missing observations:
    `"<angle> sensor missing <N> of <M> expected observations: <comma-separated keys, max 5, then ellipsis>"`
- `SuggestedLocus`: union of `SuggestedLocus` entries from all signals
  whose verdict matches the aggregate verdict (i.e., fail signals when
  aggregate is fail; warn signals when aggregate is warn). Dedup by
  `(Path, Symbol)` tuple, preserving first-seen order, capped at 10
  entries.
- `Rationale`: structured one-liner — for example
  `"see individual fail signals for per-record detail"`. Never raw
  logs. The detail lives in the per-signal records that preceded the
  aggregate on the same stream.

### 6.6 Termination × verdict reference table

| Kind | Termination | Has fail signal | Verdict |
|---|---|---|---|
| any | `completed` | yes | fail |
| any | `completed` | no, has warn | warn |
| any | `completed` | no, has inconclusive only | inconclusive |
| any | `completed` | no | pass |
| any | `stopped` | yes | fail |
| any | `stopped` | no | apply step-5 rules |
| any | `timeout` | yes | fail |
| any | `timeout` | no | inconclusive |
| any | `error` | yes | fail |
| any | `error` | no | inconclusive |
| observational | any | — | **fail** if `missing_observations` non-empty (overrides every row above) |

## 7. Validator

`Validate(a AggregateSignal) error` runs at three points:

1. After `ParseAggregate` decodes a JSON record (load-time correctness).
2. After `Rollup` builds an `AggregateSignal` (self-check; refuses to
   emit invalid output).
3. (Phase B) On records arriving over the wire from sensors.

### 7.1 Required-field rules

Required (error if missing or zero-value where zero is invalid):
`schema_version`, `type`, `sensor_id`, `use_case_id`, `angle`,
`started_at`, `ended_at`, `termination_reason`, `verdict`,
`confidence`, `rollup`.

### 7.2 Value rules

- `type == "aggregate"` (discriminator).
- `angle` ∈ `enums.AllValidationAngles()`.
- `termination_reason` ∈ `enums.AllTerminationReasons()`.
- `verdict` ∈ `enums.AllVerdicts()`.
- `0.0 ≤ confidence ≤ 1.0`.
- `ended_at ≥ started_at`.
- All counts ≥ 0.

### 7.3 Arithmetic rule

`pass_count + warn_count + fail_count + inconclusive_count == total_signals`.

### 7.4 Conditional rules

- `heal_hint` present iff `verdict ∈ {warn, fail}`. Both directions
  enforced — a `pass` aggregate carrying a `heal_hint` is an error
  (signals tooling that something is wrong with the producer).
- `completeness` present → both sub-fields present;
  `MissingObservations ⊆ ExpectedObservations` (subset check on the
  raw string slices, preserving the spec's set semantics).

### 7.5 Error reporting

Each validation error names the offending field (e.g.,
`"aggregate: heal_hint required when verdict == warn"`). No stack
traces or raw input dumps — the error is meant to be readable in test
output and CI logs.

## 8. Parser

`ParseAggregate(r io.Reader) (AggregateSignal, error)` reads a single
JSON record (the **last** record of a JSON Lines stream — the
sensor's runtime is responsible for slicing the right record). It:

1. Decodes the JSON into `AggregateSignal`.
2. Rejects if `type != "aggregate"` (the discriminator check is the
   parser's responsibility; the validator inherits it).
3. Calls `Validate`.
4. Returns the typed record on success.

For a real sensor stream, a higher-level reader (Phase B) splits the
stream into per-line `signal.Signal` records plus one terminal
`AggregateSignal`. E8 only owns the single-record parser; the
splitter lives with the runtime.

## 9. Test plan

### 9.1 Layout

```
internal/aggregate/
  rollup_test.go    — table-driven Rollup tests
  parse_test.go     — round-trip parse/encode
  validate_test.go  — negative validation cases
  testdata/
    aggregate-single-shot-pass.json
    aggregate-single-shot-fail.json
    aggregate-stream-all-pass.json
    aggregate-stream-mixed-warn.json
    aggregate-stream-mixed-fail.json
    aggregate-observational-complete.json
    aggregate-observational-missing.json
    aggregate-timeout-no-fails.json
    aggregate-bad-arithmetic.json
    aggregate-fail-without-heal-hint.json
    aggregate-warn-without-heal-hint.json
    aggregate-pass-with-heal-hint.json
```

### 9.2 Rollup tests (AAA, table-driven)

For each row: `(name, RollupInput, want AggregateSignal, wantErr bool)`.

- Single-shot: each of {pass, warn, fail, inconclusive} → aggregate
  mirrors.
- Stream assertion, `completed`:
  - all-pass → pass
  - pass + warn → warn
  - pass + fail → fail
  - pass + inconclusive → inconclusive
  - all-fail → fail
- Stream assertion, `timeout`:
  - pass + pass → inconclusive
  - pass + fail → fail (fail wins)
- Stream assertion, `error`: same matrix as timeout.
- Observational, `completed`:
  - full coverage, no signals → pass, confidence 1.0
  - full coverage, mixed signals → apply step-5 rules
  - one missing observation → fail (overrides even all-pass signals)
- Observational, `timeout`:
  - full coverage → pass (coverage IS clear despite timeout)
  - partial coverage → fail (missing non-empty overrides)
- Confidence:
  - mixed-confidence signals → arithmetic mean within 1e-9 tolerance
  - empty signals + pass verdict → 1.0
  - empty signals + inconclusive → 0.0

### 9.3 heal_hint tests

- Single-shot fail: aggregate.HealHint deep-equals the sole signal's
  HealHint.
- Stream fail (70 of 700): summary contains `"70 of 700"`;
  suggested_locus deduplicated by (path, symbol); length ≤ 10.
- Stream warn (no fails, 3 warns): summary references warnings; loci
  drawn only from warn-signals (no fail-signal loci leak in).
- Observational fail, 2 missing keys: summary lists the missing keys;
  if ≤ 5 keys, all are listed; if > 5, first 5 plus an ellipsis.

### 9.4 Parser tests

- Round-trip on every golden file:
  `parse → re-encode → parse → reflect.DeepEqual`.
- A JSON record without `"type": "aggregate"` is rejected with an
  error mentioning the discriminator.

### 9.5 Validator negative tests

Each row asserts a specific error message substring.

- Bad arithmetic (counts don't sum to `total_signals`).
- Missing each required field, one at a time (table-driven).
- `verdict: fail`, `heal_hint` absent.
- `verdict: warn`, `heal_hint` absent.
- `verdict: pass`, `heal_hint` present.
- Confidence out of range (`-0.1`, `1.5`).
- `ended_at < started_at`.
- Unknown `termination_reason`.
- `missing_observations` contains a key not in `expected_observations`.

### 9.6 Determinism test

A property test runs `Rollup` 100× on the same input and asserts
byte-identical JSON output. Catches any incidental non-determinism
(map iteration order in synthesis, for instance).

## 10. Deliverable acceptance

- `internal/aggregate/` compiles with no external dependencies beyond
  the standard library + `internal/enums` + `internal/signal`.
- All tests in §9 pass.
- All twelve golden files in `testdata/` round-trip losslessly.
- Negative tests fail with field-naming errors.
- `Rollup` determinism property test passes 100/100 runs.
- No duplicated definition of `Signal` or `HealHint` types (verified
  by import inspection — only `internal/signal` defines them).

## 11. Out of scope (reaffirmed)

- Runtime invocation of `Rollup`.
- Multi-sensor per-use-case aggregation.
- Stream splitter that separates `Signal` records from the terminal
  `AggregateSignal` on a sensor's stdout.
- Sensor schema field for `expected_observations` (plan §4.4 open
  question, defers to sensor execution design in Phase B).

## 12. Open questions deferred to Phase B

- **Where `ExpectedObservations` lives in the Sensor schema.** Plan
  §4.4 does not declare it. For E8, it is an input to `Rollup`; the
  runtime owns how to source it.
- **Whether `OutputType` belongs in `RollupInput` or can be inferred
  from `Kind`.** Currently kept explicit because a future sensor
  could be observational + single-shot; the explicit field future-
  proofs `Rollup` without speculative scope creep.
- **Locus cap of 10.** Picked as a round number; revisit once we have
  real heal-loop telemetry.
