# E7 — Signal: Design Spec

> Source: [`docs/harness-framework/E7-signal.md`](../../harness-framework/E7-signal.md), [`plan.md`](../../harness-framework/plan.md) §4.5, §2.
> Status: drafted 2026-05-23, awaiting written-spec review.

## 1. Purpose

Deliver the `internal/signal/` Go package: the typed `Signal`, a streaming JSON Lines parser exposed as `iter.Seq2[Signal, error]`, an explicit `Validate(Signal) error` for Go-constructed signals, and a `WriteSignal` encoder used by the round-trip stability test.

The schema is already frozen at [`schemas/signal.yaml`](../../../schemas/signal.yaml). E7 does **not** change the schema; it produces the Go-side machinery that parses, validates, and encodes signals conforming to it.

## 2. Scope

In:
- `internal/signal/` package — types, parser, validator, encoder, tests.
- Streaming JSONL parser with per-line `(Signal, error)` yields and blank-line tolerance.
- Single source of truth for validation: the embedded JSON Schema, used both inside the parser and by the exported `Validate`.
- Round-trip helper sufficient to satisfy the parse → encode → parse stability test.
- Golden tests against in-tree fixtures under `internal/signal/testdata/` covering pass/fail/inconclusive, malformed lines, blank lines, empty stream, and the buffer-size boundary.

Out:
- Signal **emission** (sensors emit; runtime owns the writer — Phase B).
- Signal **aggregation** (E8 owns rollup and the `AggregateSignal` type).
- Signal **consumption** by the heal loop (Phase B).
- Log parsing, regex derivation, HTTP response capture, or any other source-format → Signal translation. Those live in sensors / runtime, not here. By the time bytes reach `internal/signal`, they are already JSON Lines.
- A YAML loader for individual signals. Signals are machine-emitted; the gate's `schemas/examples/signal/*.yaml` exercise schema validation via the schema-gate validator, not the parser.

## 3. Decisions captured from brainstorm

| # | Decision | Rationale |
|---|---|---|
| 1 | **Parser API is `iter.Seq2[Signal, error]`** (range-over-func, Go 1.23+). | Caller writes `for sig, err := range ParseSignals(r)`. No goroutines, no channel cleanup. Matches the shape the E7 doc itself sketched. |
| 2 | **Malformed line → yield `(zero Signal, error)`, continue.** Reader-level errors (I/O, line too long) yield once at end and terminate. | Per-line error visibility lets the caller decide abort vs. skip without losing surrounding good signals. |
| 3 | **Parser test fixtures live at `internal/signal/testdata/*.jsonl`** in the wire format the parser parses. The schema gate's YAML examples are unaffected. | Tests exercise real bytes; the gate's YAML examples test a different thing (schema-validation), not parser behavior. |
| 4 | **No `LoadSignal(path)` for YAML.** JSONL parser only. | Signals are machine-emitted, never human-authored at runtime. Adding a YAML loader duplicates surface for a consumer that doesn't yet exist. |
| 5 | **`Validate` is a thin wrapper around the compiled JSON Schema** — no parallel Go-side rule reimplementation. | Single source of truth; zero drift surface between the schema's `if/then` (heal_hint when fail) / enum / range constraints and Go code. |
| 6 | **`Evidence` is `map[string]any` with typed accessors** for the three schema-documented keys (`expected`, `actual`, `fixture_id`). | Schema is `additionalProperties: true`; sensor-specific keys are read via raw map access. No accessor proliferation until a real consumer demands it. |
| 7 | **`EmittedAt` decodes to `time.Time`.** | Schema is `format: date-time` (RFC3339). Decoding catches malformed timestamps at parse time; the cost is one extra invalid-timestamp error per malformed line, surfaced via the per-line `(Signal, error)` yield. |
| 8 | **Hard cap of 1 MiB per signal line** via `bufio.Scanner` buffer, surfaced as an explicit reader-level error if exceeded. | Signals are summary records, not log dumps. The cap is a constant in `parser.go`, easily bumped if profiling on real sensor output proves it too tight. |
| 9 | **Parser does not close the reader.** | Caller owns the reader's lifetime; matches `bufio.Scanner`'s contract and avoids surprising callers passing an `os.File` they want to inspect afterward. |

## 4. Package surface

```go
package signal

import (
    "io"
    "iter"
    "time"

    "github.com/iurykrieger/lastro/internal/enums"
)

type Signal struct {
    SchemaVersion string                `json:"schema_version"`
    SensorID      string                `json:"sensor_id"`
    UseCaseID     string                `json:"use_case_id"`
    Angle         enums.ValidationAngle `json:"angle"`
    EmittedAt     time.Time             `json:"emitted_at"`
    Verdict       enums.Verdict         `json:"verdict"`
    Confidence    float64               `json:"confidence"`
    Evidence      Evidence              `json:"evidence"`
    HealHint      *HealHint             `json:"heal_hint,omitempty"`
}

type Evidence map[string]any

func (e Evidence) Expected() (any, bool)
func (e Evidence) Actual() (any, bool)
func (e Evidence) FixtureID() (string, bool)

type HealHint struct {
    Summary        string  `json:"summary"`
    SuggestedLocus []Locus `json:"suggested_locus,omitempty"`
    Rationale      string  `json:"rationale"`
}

type Locus struct {
    Path   string `json:"path"`
    Symbol string `json:"symbol,omitempty"`
}

// ParseSignals streams a JSONL signal sequence. Blank lines are skipped.
// A malformed or schema-invalid line yields (Signal{}, error) and
// iteration continues. A reader-level failure (I/O, line exceeds
// maxSignalLineBytes) yields once at the end and terminates.
//
// The reader is not closed; the caller owns its lifetime.
func ParseSignals(r io.Reader) iter.Seq2[Signal, error]

// Validate checks a Signal against the canonical JSON Schema. Used by
// consumers (e.g., E8 tests) that construct Signals in Go rather than
// parsing them. The parser already validates each line it yields, so
// callers iterating ParseSignals do not need to call Validate.
func Validate(sig Signal) error

// WriteSignal encodes one Signal as a single JSON Lines record
// (terminated by a newline). Used for the round-trip stability test and
// by any future caller that needs to produce JSONL from typed Signals.
func WriteSignal(w io.Writer, sig Signal) error
```

`compiledSchema()` is an unexported `sync.Once`-cached helper shared by `ParseSignals` and `Validate`.

## 5. Parser pipeline

`ParseSignals(r)` returns an `iter.Seq2[Signal, error]`. Inside the returned function:

1. **Scan.** Wrap `r` in `bufio.NewScanner` with a starting buffer of 64 KiB and a max-token-size of `maxSignalLineBytes = 1 << 20` (1 MiB).
2. **Per line:**
   - Trim leading/trailing whitespace. If the line is empty, continue (blank-line tolerance).
   - Run the four sub-phases below.
   - `yield(sig, err)`. If `yield` returns false, exit (caller stopped).
3. **End-of-stream.** If `scanner.Err() != nil` (I/O failure, `bufio.ErrTooLong` on line overrun, etc.), `yield(Signal{}, fmt.Errorf("signal: scan: %w", err))` once and return.

Per-line sub-phases (collectively `decodeAndValidateLine([]byte) (Signal, error)`):

1. **JSON decode to `any`.** `json.Unmarshal(line, &instance)`. Failure → return `Signal{}, fmt.Errorf("signal: decode line: %w", err)`.
2. **Schema validation.** `compiledSchema().Validate(instance)`. Failure returns the raw `jsonschema/v6` error wrapped with `signal: schema:` context. This is where `heal_hint`-when-`fail`, `confidence` range, `verdict` / `angle` enum membership, required-field presence, and id-pattern checks fire.
3. **Typed decode.** `json.Unmarshal(line, &sig)` into a `Signal`. Failure (e.g., bad timestamp format that satisfied the schema but not `time.Time.UnmarshalJSON`) → return `Signal{}, fmt.Errorf("signal: decode typed: %w", err)`.
4. **Return** `(sig, nil)`.

The two-pass decode mirrors `internal/fixture`'s loader: decode-to-`any` for the schema validator, then a typed decode for the struct. Single-pass collapse is possible but not chased — same library, same precedent.

## 6. Validator scope

E7 validates **only** what the JSON Schema expresses:

- All required fields present (`schema_version`, `sensor_id`, `use_case_id`, `angle`, `emitted_at`, `verdict`, `confidence`, `evidence`).
- `id` pattern (`^[a-z][a-z0-9-]*$`) on `sensor_id` and `use_case_id`.
- `schema_version` is semver-shaped (`^\d+\.\d+\.\d+$`).
- `angle` ∈ canonical `ValidationAngle` enum.
- `verdict` ∈ `{pass, fail, inconclusive}`.
- `confidence` ∈ `[0, 1]`.
- `heal_hint` present iff `verdict == fail` (via the schema's `if/then`).
- `heal_hint.summary`, `heal_hint.rationale` non-empty.
- `heal_hint.suggested_locus[].path` non-empty.
- `evidence` is an object; `evidence.fixture_id` (if present) matches the id pattern.

E7 explicitly does **not** validate:

- Whether `SensorID` references a real sensor, or whether `UseCaseID` references a real use case. Runtime concern.
- Whether `Angle` is **applicable** to the use case's archetype. ValidationPolicy concern (E9).
- Whether `Confidence` clears a per-sensor floor for inferential sensors. Sensor emission / runtime concern.
- Whether the `evidence` content is semantically correct for its angle. Sensor concern.
- Any cross-signal invariant (rollup arithmetic, monotonic timestamps, etc.). E8 / runtime concern.

`Validate(Signal)` re-encodes the Signal via `json.Marshal`, decodes back into `any`, and runs the compiled schema's `Validate`. Same code path as the parser's phase 2, so behavior is identical.

**Timestamp shape note.** The schema declares `emitted_at` as `format: date-time`, but jsonschema/v6 treats `format` as advisory by default — we do not opt into format-assertion mode. Timestamp shape is therefore enforced by `time.Time.UnmarshalJSON` in phase 3 of the parser pipeline. `Validate(Signal)` cannot reach a malformed timestamp because the Go `time.Time` field requires a valid value before `Validate` is even called.

## 7. Tests

All under `internal/signal/`, following the repo's `_test.go`-sibling convention.

**`drift_test.go`:**
- `bytes.Equal(canonical, embeddedSchemaYAML)` where canonical is read from `../../schemas/signal.yaml`. Mirrors the fixture chunk's drift test, with the same `cp` hint on failure.
- `compiledSchema()` returns non-nil, no error.

**`types_test.go`:**
- `Evidence.Expected/Actual/FixtureID` accessors: present-and-correct-type, present-and-wrong-type, absent. The first returns `(value, true)`; the latter two return `(zero, false)`.
- `HealHint` and `Locus` zero-values marshal as expected (sanity).

**`parser_test.go`:**
- **Mixed stream (deliverable: "parses pass/fail/inconclusive"):** `testdata/mixed.jsonl` has one signal per verdict, in order. Iterate, collect; assert exactly three yields, every `err == nil`, fields match the file.
- **Malformed mid-stream:** `testdata/malformed-mid.jsonl` is `valid \n invalid-json \n valid`. Assert iteration yields three results: `(sig, nil)`, `(zero, err)`, `(sig, nil)` — surrounding signals parse cleanly, error mentions decode failure.
- **Schema-invalid mid-stream:** A line that decodes as JSON but fails schema validation (e.g., `verdict: "fail"` without `heal_hint`). Assert the bad line yields `(zero, err)` mentioning `heal_hint`; surrounding good lines yield cleanly.
- **Typed-decode-invalid mid-stream:** A line that passes JSON decode and schema validation but fails phase 3 — typically a malformed `emitted_at` timestamp that the schema's `format: date-time` does not enforce (format validation is opt-in in jsonschema/v6) but `time.Time.UnmarshalJSON` rejects. Assert the bad line yields `(zero, err)`; surrounding good lines yield cleanly.
- **Empty stream:** `testdata/empty.jsonl` is zero bytes. Iteration yields nothing, no error.
- **Blank lines:** `testdata/blank-lines.jsonl` interleaves valid signals with blank/whitespace-only lines. Assert blank lines are skipped silently (not yielded as errors), valid signals yield cleanly.
- **Big evidence (≤ 1 MiB):** `testdata/big-evidence.jsonl` is a single signal with an `evidence.actual` string sized to roughly 900 KiB. Assert the signal parses cleanly.
- **Line exceeds 1 MiB:** Build a 2 MiB synthetic line via `bytes.Repeat` and stream it. Assert iteration yields one error mentioning `bufio.ErrTooLong` (wrapped) and terminates.
- **Streaming behavior (deliverable: "yields as bytes arrive"):** Use `io.Pipe`. A writer goroutine writes one valid signal, sleeps 20 ms, writes another, closes. The reading test asserts the second yield arrives at least ~15 ms after the first — i.e., the parser is not buffering the whole stream.
- **Early caller stop:** Use `mixed.jsonl`. Caller returns false from `yield` after the first signal. Assert iteration stops; assert the underlying reader is not consumed past the first newline (use a counting reader to verify).

**`validator_test.go`** — explicit `Validate(Signal)` cases (deliverable: negative tests):
- **Missing `verdict`:** Construct a Signal with `Verdict: ""`. Assert `Validate` returns a schema error mentioning `verdict`.
- **Fail without `heal_hint`:** Construct a Signal with `Verdict: "fail"`, `HealHint: nil`. Assert error mentions `heal_hint`.
- **Confidence out of range:** `Confidence: 1.5`. Assert error mentions `confidence`.
- **Unknown angle:** `Angle: "not-a-real-angle"`. Assert error mentions `angle` enum.
- **Bad id pattern:** `SensorID: "Invalid_Id"`. Assert error mentions the id pattern.
- **Happy path:** A fully-populated valid Signal. Assert `Validate` returns nil.

**`encoder_test.go`** — round-trip (deliverable: "parse → re-encode → parse again is stable"):
- For each signal yielded by `ParseSignals(open(testdata/mixed.jsonl))`:
  - Encode via `WriteSignal` to a `bytes.Buffer`.
  - Re-parse the buffer with `ParseSignals`.
  - Assert exactly one signal yielded, no error.
  - Assert `reflect.DeepEqual(original, reparsed)`.
- **Note on "stable":** The contract is *semantic* equality of parsed Signals, not byte-equality of the encoded forms. Map iteration order and timestamp formatting may differ between input and output forms. Documented in the test header.

**Coverage floor.** No percentage gate, but every exported symbol must be exercised and every yielded-error path must be covered by an explicit assertion.

## 8. Dependencies

**New:** none. Everything required is already in `go.mod`.

**Existing (already in `go.mod`):**
- `github.com/santhosh-tekuri/jsonschema/v6` — JSON Schema validation (matches `internal/fixture`).
- `sigs.k8s.io/yaml` — only used at schema compile time to load the embedded `schema.yaml` into a JSON-shaped tree. The parser itself reads JSON, not YAML.

**Inter-package:**
- `internal/enums/` — E7 imports `enums.ValidationAngle` and `enums.Verdict` for typed fields. Both already exist (E1 has landed). If E1 weren't available, E7 would ship its own constants with identical string values; switching to the import later is a no-op.
- Schema-freeze gate — satisfied (`schemas/signal.yaml` is frozen).

## 9. Integration seam

**E8 (AggregateSignal):**
- Imports the `signal.Signal` type directly. Does not redefine it.
- `Rollup(signals []signal.Signal, kind enums.SensorKind, ...) aggregate.AggregateSignal` consumes a slice produced by callers (either parsed via `ParseSignals` + `slices.Collect` for tests, or constructed in-Go for unit tests of `Rollup` itself).
- E8 may call `signal.Validate` on each input signal before rollup if it wants belt-and-suspenders coverage; the parser already validated, so this is optional.

**Parallel-work guarantee:** E8 can develop entirely against a stubbed `[]Signal` slice while E7 is in flight. The Signal type's field set is frozen by the schema, not by E7's brainstorm, so the only churn risk is in helper-method names (`Evidence.Expected` etc.) — none of which `Rollup` needs.

**Phase B runtime (signal collector):**
- The collector reads sensor stdout, hands the `io.Reader` to `ParseSignals`, and consumes the `iter.Seq2` yield-by-yield.
- Per-line errors get logged and, depending on policy, may abort the sensor run or be tolerated. The `(Signal, error)` shape gives the runtime that choice.
- The reader-level final error (e.g., truncated stream from a crashed sensor) signals the runtime to treat the run as `termination_reason: error`.

## 10. Acceptance criteria

Mirror of the E7 doc's deliverable acceptance, made concrete:

- `internal/signal/` parses `testdata/mixed.jsonl` cleanly: three signals, one per verdict, no errors.
- Every negative-test case in §7's `validator_test.go` produces the expected error class.
- The streaming-behavior test (`io.Pipe` with sleeps) demonstrates that the parser yields per-arrival, not after EOF.
- The round-trip test passes via `reflect.DeepEqual` over the parsed forms.
- `drift_test.go` confirms `internal/signal/schema.yaml` is byte-equal to `schemas/signal.yaml`.
- A compile-time signature assertion lives in the package: `var _ func(io.Reader) iter.Seq2[Signal, error] = ParseSignals`. (Pure type-check; never invokes the function.)
- `go vet ./internal/signal/...` and `go test ./internal/signal/...` both pass.

## 11. Out of scope (deferred decisions)

- **Confidence-for-computational-sensors policy (E7 doc Q6).** Whether computational sensors always emit `1.0` or omit the field is a *sensor emission* policy, not a parser/validator concern. E7 accepts any value in `[0, 1]`, exactly as the schema specifies.
- **Discriminated-union evidence shape (E7 doc Q5).** The schema and the brainstorm both keep `evidence` as `map[string]any` with documented well-known keys. Promotion to per-angle typed shapes would require a schema change first.
- **Buffer-cap tuning.** The 1 MiB per-line cap is a guess based on "signals are summary records." If real sensor traffic shows summary records routinely approaching the cap, revisit — but the right fix is almost certainly "sensors should reference files instead of embedding blobs," not "bump the cap."
- **Round-trip *byte* stability.** Today's round-trip contract is semantic. If the runtime ever needs byte-stable signal storage (e.g., for content-hashing signals into a cache), a canonical-encoding helper would be added — but no consumer currently needs it.
- **Multi-document YAML loader.** Not in scope per Decision #4. If a future workflow wants to author signals as multi-document YAML for golden-file tests, an additive loader can live in a separate file without disturbing the JSONL parser.
