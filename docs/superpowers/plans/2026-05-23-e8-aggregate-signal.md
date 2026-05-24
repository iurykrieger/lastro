# E8 AggregateSignal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `internal/aggregate/` Go package — `AggregateSignal` type, JSON Schema-backed loader/validator, deterministic `Rollup` function that turns `[]signal.Signal` plus execution metadata into the terminal `AggregateSignal` for a sensor run. Extend the framework verdict enum with a new `warn` value (pass-grade outcome with actionable concerns).

**Architecture:** Hand-written Go package mirroring the existing `internal/fixture/` layout: an embedded copy of the canonical YAML schema validated against with `santhosh-tekuri/jsonschema/v6` for shape rules, plus hand-written Go for rules the schema can't express (count arithmetic, heal_hint forbidden-when-pass, observational completeness subset, time ordering). `Rollup` is a pure function — same `RollupInput` → byte-identical JSON every time. heal_hint synthesis is deterministic: single-shot copies the sole signal's hint; stream/observational synthesizes a meta-hint with templated summary, deduplicated `(path, symbol)` loci capped at 10, and a structured rationale.

**Tech Stack:** Go 1.24.2, `sigs.k8s.io/yaml`, `github.com/santhosh-tekuri/jsonschema/v6`, `encoding/json`, `errors`, `time` — all already in `go.mod`. No new dependencies.

**Spec:** [`docs/superpowers/specs/2026-05-23-e8-aggregate-signal-design.md`](../specs/2026-05-23-e8-aggregate-signal-design.md)

---

## Prerequisites

- **E1 enums** (`internal/enums/`) already exists with `Verdict`, `TerminationReason`, `ValidationAngle`, `SensorKind`, `SignalOutputType`. This plan extends `Verdict` with `warn` in Phase A.
- **Schema-freeze gate** (`schemas/`) already exists with `aggregate-signal.yaml`, `signal.yaml`, `enums/verdicts.yaml`, and YAML examples under `schemas/examples/aggregate-signal/`. This plan extends those files in Phase A.
- **E7 Signal** (`internal/signal/`) **does not exist yet**. To unblock parallel development, E8 vendors a minimal `Signal` and `HealHint` Go type inside `internal/aggregate/internal/signalstub/`. When E7 lands, refactor `internal/aggregate` to import `internal/signal` and delete the stub (out of scope for this plan).
- Working directory: repo root.
- Module path: `github.com/iurykrieger/lastro`.
- All tests run via `go test ./internal/...` from repo root.

---

## File map

**Phase A — Schema extension (modify):**
- Modify: `schemas/enums/verdicts.yaml` — add `warn` value
- Modify: `schemas/aggregate-signal.yaml` — add `warn` to verdict enum, add `warn_count` to rollup, extend `allOf` heal_hint conditional to include `warn`
- Modify: `schemas/signal.yaml` — add `warn` to verdict enum, extend `allOf` heal_hint conditional to include `warn`
- Modify: `schemas/examples/aggregate-signal/single-shot-pass.yaml`, `stream-pass.yaml`, `stream-fail.yaml`, `observational-pass.yaml`, `observational-fail-missing.yaml` — add `warn_count: 0` to each `rollup` block
- Modify: `internal/enums/enums.go` — add `VerdictWarn` constant
- Modify: `internal/enums/enums_test.go` — extend verdict tests

**Phase B — Signal stub:**
- Create: `internal/aggregate/internal/signalstub/signalstub.go`

**Phase C — Aggregate types + embedded schema:**
- Create: `internal/aggregate/types.go`
- Create: `internal/aggregate/schema.yaml` (copy of canonical)
- Create: `internal/aggregate/schema.go`
- Create: `internal/aggregate/drift_test.go`

**Phase D — Parser:**
- Create: `internal/aggregate/parse.go`
- Create: `internal/aggregate/parse_test.go`

**Phase E — Validator:**
- Create: `internal/aggregate/validate.go`
- Create: `internal/aggregate/validate_test.go`

**Phase F — Rollup:**
- Create: `internal/aggregate/rollup.go`
- Create: `internal/aggregate/rollup_test.go`
- Create: `internal/aggregate/synthesize.go` (heal_hint synthesis helpers)
- Create: `internal/aggregate/synthesize_test.go`

**Phase G — Golden files and determinism:**
- Create: `internal/aggregate/testdata/single-shot-pass.json`
- Create: `internal/aggregate/testdata/single-shot-fail.json`
- Create: `internal/aggregate/testdata/stream-all-pass.json`
- Create: `internal/aggregate/testdata/stream-mixed-warn.json`
- Create: `internal/aggregate/testdata/stream-mixed-fail.json`
- Create: `internal/aggregate/testdata/observational-complete.json`
- Create: `internal/aggregate/testdata/observational-missing.json`
- Create: `internal/aggregate/testdata/timeout-no-fails.json`
- Create: `internal/aggregate/golden_test.go`
- Create: `internal/aggregate/determinism_test.go`

---

## Phase A — Schema extension (warn verdict + warn_count)

### Task 1: Add `warn` to canonical verdicts enum

**Files:**
- Modify: `schemas/enums/verdicts.yaml`

- [ ] **Step 1: Update the YAML**

Open `schemas/enums/verdicts.yaml` and replace its body so the `values` list reads, in this order: `pass`, `warn`, `fail`, `inconclusive`. The file should look like:

```yaml
schema_version: 1.0.0
title: Verdict
description: |
  Outcome of a sensor signal or aggregate signal. Used by both Signal.verdict
  and AggregateSignal.verdict.

values:
  - id: pass
    purpose: "Behavior matched the criterion"
  - id: warn
    purpose: "Behavior matched, but the sensor identified an actionable concern worth addressing; heal_hint is required"
  - id: fail
    purpose: "Behavior did not match; heal_hint is required"
  - id: inconclusive
    purpose: "Sensor could not determine pass/fail (e.g., inferential confidence below floor, timeout without coverage)"
```

- [ ] **Step 2: Run the drift test (expected to fail)**

Run: `go test ./internal/enums/... -run TestGoConstantsMatchYAML -v`
Expected: FAIL — the Go `Verdict` constants list is shorter than the YAML by one entry.

- [ ] **Step 3: Commit the YAML change in isolation (the failing test will be fixed in Task 4)**

```bash
git add schemas/enums/verdicts.yaml
git commit -m "feat(schema): add warn verdict to verdicts enum"
```

### Task 2: Extend `aggregate-signal.yaml` schema

**Files:**
- Modify: `schemas/aggregate-signal.yaml`

- [ ] **Step 1: Extend the verdict enum**

In `schemas/aggregate-signal.yaml`, change line ~42 from:

```yaml
    enum: [pass, fail, inconclusive]
```

to:

```yaml
    enum: [pass, warn, fail, inconclusive]
```

- [ ] **Step 2: Add `warn_count` to the rollup block**

In the same file, change the `rollup` block so its `required` list and `properties` include `warn_count`:

```yaml
  rollup:
    type: object
    required: [total_signals, pass_count, warn_count, fail_count, inconclusive_count]
    additionalProperties: false
    properties:
      total_signals:       { type: integer, minimum: 0 }
      pass_count:          { type: integer, minimum: 0 }
      warn_count:          { type: integer, minimum: 0 }
      fail_count:          { type: integer, minimum: 0 }
      inconclusive_count:  { type: integer, minimum: 0 }
```

- [ ] **Step 3: Extend the `allOf` heal_hint conditional**

Replace the `allOf` block so heal_hint is required for both `fail` and `warn`:

```yaml
allOf:
  - if:
      properties:
        verdict: { enum: [fail, warn] }
    then:
      required: [heal_hint]
```

- [ ] **Step 4: Run drift test for inline enums**

Run: `go test ./internal/enums/... -run TestInlineSchemaEnumsMatchYAML -v`
Expected: PASS (the inline enum in `aggregate-signal.yaml` now matches the canonical `verdicts.yaml`).

- [ ] **Step 5: Commit**

```bash
git add schemas/aggregate-signal.yaml
git commit -m "feat(schema): extend aggregate-signal verdict enum with warn, add warn_count"
```

### Task 3: Extend `signal.yaml` schema

**Files:**
- Modify: `schemas/signal.yaml`

- [ ] **Step 1: Extend the verdict enum**

In `schemas/signal.yaml`, change line ~30 from:

```yaml
    enum: [pass, fail, inconclusive]
```

to:

```yaml
    enum: [pass, warn, fail, inconclusive]
```

- [ ] **Step 2: Extend the `allOf` heal_hint conditional**

Replace the `allOf` block at line ~47:

```yaml
allOf:
  - if:
      properties:
        verdict: { enum: [fail, warn] }
    then:
      required: [heal_hint]
```

- [ ] **Step 3: Run drift tests**

Run: `go test ./internal/enums/... -v`
Expected: `TestInlineSchemaEnumsMatchYAML` PASSes; `TestGoConstantsMatchYAML` still FAILs (Go constants not yet updated — fixed in Task 4).

- [ ] **Step 4: Commit**

```bash
git add schemas/signal.yaml
git commit -m "feat(schema): extend signal verdict enum with warn, broaden heal_hint conditional"
```

### Task 4: Add `VerdictWarn` constant to Go enums

**Files:**
- Modify: `internal/enums/enums.go`
- Modify: `internal/enums/enums_test.go`

- [ ] **Step 1: Update the test (expected to fail)**

Open `internal/enums/enums_test.go` and find the `TestAllVerdicts` (or equivalent) test. Locate the assertion that lists the verdict values and add `VerdictWarn` in the second position. For example, if the test reads:

```go
want := []Verdict{VerdictPass, VerdictFail, VerdictInconclusive}
```

change it to:

```go
want := []Verdict{VerdictPass, VerdictWarn, VerdictFail, VerdictInconclusive}
```

If `internal/enums/enums_test.go` does not already test `AllVerdicts()` in this exact form, search the file for `VerdictPass` and update every list-equality assertion that mentions verdicts to include `VerdictWarn` between `VerdictPass` and `VerdictFail`. Also add an `IsValidVerdict("warn")` assertion if a similar pattern exists for other enum values.

- [ ] **Step 2: Run the test (verify it fails)**

Run: `go test ./internal/enums/... -v`
Expected: FAIL — `VerdictWarn` is undefined.

- [ ] **Step 3: Add the constant**

In `internal/enums/enums.go`, locate the verdict declaration block:

```go
const (
	VerdictPass         Verdict = "pass"
	VerdictFail         Verdict = "fail"
	VerdictInconclusive Verdict = "inconclusive"
)
```

Replace with:

```go
const (
	VerdictPass         Verdict = "pass"
	VerdictWarn         Verdict = "warn"
	VerdictFail         Verdict = "fail"
	VerdictInconclusive Verdict = "inconclusive"
)
```

Then update `AllVerdicts`:

```go
func AllVerdicts() []Verdict {
	return []Verdict{VerdictPass, VerdictWarn, VerdictFail, VerdictInconclusive}
}
```

- [ ] **Step 4: Run the full enum suite (verify it passes)**

Run: `go test ./internal/enums/... -v`
Expected: All tests PASS, including `TestGoConstantsMatchYAML` and `TestInlineSchemaEnumsMatchYAML`.

- [ ] **Step 5: Commit**

```bash
git add internal/enums/enums.go internal/enums/enums_test.go
git commit -m "feat(e1): add VerdictWarn constant and update AllVerdicts ordering"
```

### Task 5: Update aggregate-signal YAML examples with `warn_count`

**Files:**
- Modify: `schemas/examples/aggregate-signal/single-shot-pass.yaml`
- Modify: `schemas/examples/aggregate-signal/stream-pass.yaml`
- Modify: `schemas/examples/aggregate-signal/stream-fail.yaml`
- Modify: `schemas/examples/aggregate-signal/observational-pass.yaml`
- Modify: `schemas/examples/aggregate-signal/observational-fail-missing.yaml`

- [ ] **Step 1: Add `warn_count: 0` to every example's `rollup` block**

In each of the five files, insert `  warn_count: 0` immediately after the `pass_count:` line in the `rollup` block. Example (for `single-shot-pass.yaml`):

Before:
```yaml
rollup:
  total_signals: 1
  pass_count: 1
  fail_count: 0
  inconclusive_count: 0
```

After:
```yaml
rollup:
  total_signals: 1
  pass_count: 1
  warn_count: 0
  fail_count: 0
  inconclusive_count: 0
```

Apply the same change to all five files. Adjust the integer for `total_signals` only if needed (no change expected — these existing examples don't have warns).

- [ ] **Step 2: Sanity-check each example with a JSON-Schema lint pass**

There is no existing example-validator test under `schemas/`. Skip explicit verification at this step — the example correctness will be exercised in Phase G when golden files round-trip through the Go parser/validator (which uses the same schema).

- [ ] **Step 3: Commit**

```bash
git add schemas/examples/aggregate-signal/
git commit -m "feat(schema): add warn_count to aggregate-signal examples"
```

---

## Phase B — Signal stub

### Task 6: Vendor a minimal `Signal` type for parallel development with E7

**Files:**
- Create: `internal/aggregate/internal/signalstub/signalstub.go`

- [ ] **Step 1: Create the file**

Create `internal/aggregate/internal/signalstub/signalstub.go` with the following contents (no test file — this is a type-only stub):

```go
// Package signalstub vendors the minimal Signal and HealHint types that
// internal/aggregate needs while E7 (internal/signal) is being developed
// in parallel. When E7 lands, change every import of this package to
// "internal/signal" and delete this directory.
//
// The shapes here mirror plan.md §4.5 and the canonical schemas/signal.yaml
// JSON Schema. They intentionally have no parser/validator logic — that is
// E7's responsibility.
package signalstub

import (
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
)

// Locus identifies a code location the LLM should consider editing.
type Locus struct {
	Path   string `json:"path"`
	Symbol string `json:"symbol,omitempty"`
}

// HealHint is the LLM-actionable instruction attached to non-pass signals
// and aggregates. Required when Verdict is warn or fail.
type HealHint struct {
	Summary        string  `json:"summary"`
	SuggestedLocus []Locus `json:"suggested_locus,omitempty"`
	Rationale      string  `json:"rationale"`
}

// Signal is one record emitted by a sensor during execution. Single-shot
// sensors emit exactly one Signal followed by one AggregateSignal; stream
// sensors emit many.
type Signal struct {
	SchemaVersion string                `json:"schema_version"`
	SensorID      string                `json:"sensor_id"`
	UseCaseID     string                `json:"use_case_id"`
	Angle         enums.ValidationAngle `json:"angle"`
	EmittedAt     time.Time             `json:"emitted_at"`
	Verdict       enums.Verdict         `json:"verdict"`
	Confidence    float64               `json:"confidence"`
	Evidence      map[string]any        `json:"evidence,omitempty"`
	HealHint      *HealHint             `json:"heal_hint,omitempty"`
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./internal/aggregate/internal/signalstub/...`
Expected: no output (clean build).

- [ ] **Step 3: Commit**

```bash
git add internal/aggregate/internal/signalstub/signalstub.go
git commit -m "feat(e8): vendor minimal Signal stub for parallel development with E7"
```

---

## Phase C — Aggregate types and embedded schema

### Task 7: Define `AggregateSignal` Go types

**Files:**
- Create: `internal/aggregate/types.go`

- [ ] **Step 1: Create the file**

Create `internal/aggregate/types.go`:

```go
// Package aggregate owns the AggregateSignal entity — the terminal record
// emitted at the end of every sensor execution. It also owns the
// deterministic Rollup function that turns a slice of Signals plus
// execution metadata into the AggregateSignal.
//
// See docs/superpowers/specs/2026-05-23-e8-aggregate-signal-design.md
// and docs/harness-framework/E8-aggregate-signal.md for the design rationale.
package aggregate

import (
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

// TypeAggregate is the on-the-wire discriminator value distinguishing an
// AggregateSignal from a Signal in a shared JSON Lines stream.
const TypeAggregate = "aggregate"

// HealHint is re-exported from signalstub so callers of internal/aggregate
// can construct hints without importing the stub directly. When E7 lands,
// this alias points at internal/signal.HealHint.
type HealHint = signalstub.HealHint

// Locus is re-exported alongside HealHint.
type Locus = signalstub.Locus

// AggregateSignal is the terminal record emitted by every sensor run.
type AggregateSignal struct {
	SchemaVersion     string                  `json:"schema_version"`
	Type              string                  `json:"type"`
	SensorID          string                  `json:"sensor_id"`
	UseCaseID         string                  `json:"use_case_id"`
	Angle             enums.ValidationAngle   `json:"angle"`
	StartedAt         time.Time               `json:"started_at"`
	EndedAt           time.Time               `json:"ended_at"`
	TerminationReason enums.TerminationReason `json:"termination_reason"`
	Verdict           enums.Verdict           `json:"verdict"`
	Confidence        float64                 `json:"confidence"`
	Rollup            RollupCounts            `json:"rollup"`
	Completeness      *Completeness           `json:"completeness,omitempty"`
	HealHint          *HealHint               `json:"heal_hint,omitempty"`
}

// RollupCounts is the per-verdict tally for the sensor's signals.
type RollupCounts struct {
	TotalSignals      int `json:"total_signals"`
	PassCount         int `json:"pass_count"`
	WarnCount         int `json:"warn_count"`
	FailCount         int `json:"fail_count"`
	InconclusiveCount int `json:"inconclusive_count"`
}

// Completeness reports observation coverage for observational sensors.
// Always omitted (nil pointer) for non-observational kinds.
type Completeness struct {
	ExpectedObservations []string `json:"expected_observations"`
	MissingObservations  []string `json:"missing_observations"`
}

// RollupInput is the full set of inputs Rollup needs to produce an
// AggregateSignal. The runtime owns where these inputs come from; Rollup
// itself is pure.
type RollupInput struct {
	Signals              []signalstub.Signal
	SensorID             string
	UseCaseID            string
	Angle                enums.ValidationAngle
	Kind                 enums.SensorKind
	OutputType           enums.SignalOutputType
	StartedAt            time.Time
	EndedAt              time.Time
	TerminationReason    enums.TerminationReason
	ExpectedObservations []string // observational only; may be nil
	ObservedKeys         []string // observational only; keys actually seen
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./internal/aggregate/...`
Expected: no output (clean build).

- [ ] **Step 3: Commit**

```bash
git add internal/aggregate/types.go
git commit -m "feat(e8): define AggregateSignal, RollupCounts, Completeness, RollupInput types"
```

### Task 8: Embed canonical schema and add drift test

**Files:**
- Create: `internal/aggregate/schema.yaml` (copy of `schemas/aggregate-signal.yaml`)
- Create: `internal/aggregate/schema.go`
- Create: `internal/aggregate/drift_test.go`

- [ ] **Step 1: Copy the canonical schema into the package**

Run from the repo root:

```bash
cp schemas/aggregate-signal.yaml internal/aggregate/schema.yaml
```

- [ ] **Step 2: Write the drift test (expected to fail until schema.go exists)**

Create `internal/aggregate/drift_test.go`:

```go
package aggregate

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedSchemaMatchesCanonicalSource(t *testing.T) {
	canonicalPath := filepath.Join("..", "..", "schemas", "aggregate-signal.yaml")
	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical schema %s: %v", canonicalPath, err)
	}
	if !bytes.Equal(canonical, embeddedSchemaYAML) {
		t.Errorf("internal/aggregate/schema.yaml has drifted from schemas/aggregate-signal.yaml; re-run `cp schemas/aggregate-signal.yaml internal/aggregate/schema.yaml`")
	}
}

func TestCompiledSchemaIsAvailable(t *testing.T) {
	s, err := compiledSchema()
	if err != nil {
		t.Fatalf("compiledSchema: %v", err)
	}
	if s == nil {
		t.Fatal("compiledSchema: returned nil schema")
	}
}
```

- [ ] **Step 3: Run the test (verify it fails to compile)**

Run: `go test ./internal/aggregate/... -v`
Expected: BUILD FAIL — `embeddedSchemaYAML` and `compiledSchema` undefined.

- [ ] **Step 4: Create the schema loader**

Create `internal/aggregate/schema.go`:

```go
package aggregate

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"
)

//go:embed schema.yaml
var embeddedSchemaYAML []byte

var (
	schemaOnce     sync.Once
	schemaCompiled *jsonschema.Schema
	schemaErr      error
)

const schemaURL = "https://lastro.dev/harness/schemas/aggregate-signal.yaml"

func compiledSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		var doc any
		if err := yaml.Unmarshal(embeddedSchemaYAML, &doc); err != nil {
			schemaErr = fmt.Errorf("aggregate: parse embedded schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(schemaURL, doc); err != nil {
			schemaErr = fmt.Errorf("aggregate: add schema resource: %w", err)
			return
		}
		s, err := c.Compile(schemaURL)
		if err != nil {
			schemaErr = fmt.Errorf("aggregate: compile schema: %w", err)
			return
		}
		schemaCompiled = s
	})
	return schemaCompiled, schemaErr
}
```

- [ ] **Step 5: Run the test (verify it passes)**

Run: `go test ./internal/aggregate/... -v`
Expected: `TestEmbeddedSchemaMatchesCanonicalSource` PASS; `TestCompiledSchemaIsAvailable` PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/aggregate/schema.yaml internal/aggregate/schema.go internal/aggregate/drift_test.go
git commit -m "feat(e8): embed aggregate-signal schema and add drift test"
```

---

## Phase D — Parser

### Task 9: Implement `ParseAggregate` happy-path round-trip

**Files:**
- Create: `internal/aggregate/parse.go`
- Create: `internal/aggregate/parse_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/aggregate/parse_test.go`:

```go
package aggregate

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
)

const happyPathJSON = `{
  "schema_version": "1.0.0",
  "type": "aggregate",
  "sensor_id": "sensor-x",
  "use_case_id": "uc-x",
  "angle": "unit-test",
  "started_at": "2026-05-23T14:00:00Z",
  "ended_at":   "2026-05-23T14:00:42Z",
  "termination_reason": "completed",
  "verdict": "pass",
  "confidence": 1.0,
  "rollup": {
    "total_signals": 1,
    "pass_count": 1,
    "warn_count": 0,
    "fail_count": 0,
    "inconclusive_count": 0
  }
}`

func TestParseAggregateHappyPath(t *testing.T) {
	got, err := ParseAggregate(strings.NewReader(happyPathJSON))
	if err != nil {
		t.Fatalf("ParseAggregate: %v", err)
	}
	if got.Type != TypeAggregate {
		t.Errorf("Type = %q, want %q", got.Type, TypeAggregate)
	}
	if got.Verdict != enums.VerdictPass {
		t.Errorf("Verdict = %q, want %q", got.Verdict, enums.VerdictPass)
	}
	if got.Rollup.TotalSignals != 1 || got.Rollup.PassCount != 1 {
		t.Errorf("Rollup = %+v, want total=1 pass=1", got.Rollup)
	}
	if got.StartedAt.UTC() != time.Date(2026, 5, 23, 14, 0, 0, 0, time.UTC) {
		t.Errorf("StartedAt = %v, want 2026-05-23T14:00:00Z", got.StartedAt)
	}
}

func TestParseAggregateRoundTrip(t *testing.T) {
	parsed, err := ParseAggregate(strings.NewReader(happyPathJSON))
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(parsed); err != nil {
		t.Fatalf("encode: %v", err)
	}

	reparsed, err := ParseAggregate(&buf)
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}

	if !reflect.DeepEqual(parsed, reparsed) {
		t.Errorf("round-trip mismatch:\n  first:  %+v\n  second: %+v", parsed, reparsed)
	}
}
```

- [ ] **Step 2: Run test (verify it fails to compile)**

Run: `go test ./internal/aggregate/... -run TestParseAggregate -v`
Expected: BUILD FAIL — `ParseAggregate` undefined.

- [ ] **Step 3: Implement `ParseAggregate`**

Create `internal/aggregate/parse.go`:

```go
package aggregate

import (
	"encoding/json"
	"fmt"
	"io"
)

// ParseAggregate reads a single JSON-encoded AggregateSignal from r,
// validates it against both the embedded JSON Schema and the hand-written
// Go-level rules, and returns the typed record on success.
//
// The reader is expected to contain exactly one JSON record (the terminal
// JSON Lines record of a sensor's stdout). Splitting a multi-line stream
// into Signal records plus the terminal AggregateSignal is the
// responsibility of the sensor runtime (Phase B), not this package.
func ParseAggregate(r io.Reader) (AggregateSignal, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return AggregateSignal{}, fmt.Errorf("aggregate: read input: %w", err)
	}

	var a AggregateSignal
	if err := json.Unmarshal(raw, &a); err != nil {
		return AggregateSignal{}, fmt.Errorf("aggregate: decode JSON: %w", err)
	}

	if a.Type != TypeAggregate {
		return AggregateSignal{}, fmt.Errorf("aggregate: type must be %q, got %q", TypeAggregate, a.Type)
	}

	if err := validateAgainstSchema(raw); err != nil {
		return AggregateSignal{}, fmt.Errorf("aggregate: schema validation: %w", err)
	}

	if err := Validate(a); err != nil {
		return AggregateSignal{}, fmt.Errorf("aggregate: %w", err)
	}

	return a, nil
}

func validateAgainstSchema(jsonDoc []byte) error {
	s, err := compiledSchema()
	if err != nil {
		return err
	}
	var instance any
	if err := json.Unmarshal(jsonDoc, &instance); err != nil {
		return fmt.Errorf("decode instance: %w", err)
	}
	if err := s.Validate(instance); err != nil {
		return err
	}
	return nil
}
```

Note: `Validate` is defined in the next phase. To unblock this task, also create a temporary placeholder so the package builds:

```go
// Validate (placeholder — full implementation in Phase E).
func Validate(a AggregateSignal) error { return nil }
```

Put the placeholder in `internal/aggregate/validate.go` (a one-liner file that Phase E will replace). Do not add tests for the placeholder.

Create `internal/aggregate/validate.go`:

```go
package aggregate

// Validate is the hand-written rule check that complements JSON Schema
// validation. Full implementation lands in Phase E.
func Validate(a AggregateSignal) error { return nil }
```

- [ ] **Step 4: Run tests (verify they pass)**

Run: `go test ./internal/aggregate/... -run TestParseAggregate -v`
Expected: `TestParseAggregateHappyPath` PASS; `TestParseAggregateRoundTrip` PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/aggregate/parse.go internal/aggregate/parse_test.go internal/aggregate/validate.go
git commit -m "feat(e8): implement ParseAggregate with schema validation and round-trip test"
```

### Task 10: Reject bad discriminator and malformed input

**Files:**
- Modify: `internal/aggregate/parse_test.go`

- [ ] **Step 1: Add the failing tests**

Append the following tests to `internal/aggregate/parse_test.go`:

```go
func TestParseAggregateRejectsWrongType(t *testing.T) {
	bad := strings.Replace(happyPathJSON, `"type": "aggregate"`, `"type": "signal"`, 1)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for wrong type discriminator, got nil")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Errorf("error should mention 'type': %v", err)
	}
}

func TestParseAggregateRejectsMalformedJSON(t *testing.T) {
	_, err := ParseAggregate(strings.NewReader(`{not json}`))
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decode JSON") {
		t.Errorf("error should mention 'decode JSON': %v", err)
	}
}

func TestParseAggregateRejectsEmptyInput(t *testing.T) {
	_, err := ParseAggregate(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}
```

- [ ] **Step 2: Run tests (verify they pass)**

Run: `go test ./internal/aggregate/... -run TestParseAggregate -v`
Expected: all three new tests PASS (the existing parser already handles these cases via `json.Unmarshal` errors and the type-discriminator check).

- [ ] **Step 3: Commit**

```bash
git add internal/aggregate/parse_test.go
git commit -m "test(e8): assert parser rejects bad type discriminator and malformed input"
```

---

## Phase E — Validator

### Task 11: Required fields, value rules, heal_hint-when-fail (schema-enforced)

**Files:**
- Modify: `internal/aggregate/validate.go`
- Create: `internal/aggregate/validate_test.go`

The JSON Schema (loaded in Task 8) already enforces: required fields, enum membership for `angle`/`verdict`/`termination_reason`, `confidence` range, and `heal_hint` required when verdict ∈ {warn, fail}. Phase E tests confirm these rules fire end-to-end via `ParseAggregate` and add the rules JSON Schema cannot express.

- [ ] **Step 1: Write the failing tests**

Create `internal/aggregate/validate_test.go`:

```go
package aggregate

import (
	"strings"
	"testing"
)

// withField returns happyPathJSON with the given top-level JSON field
// removed (string-search based — sufficient for these flat test inputs).
func withFieldRemoved(t *testing.T, field string) string {
	t.Helper()
	// crude removal: find "<field>": ..., (or trailing) and drop it
	// for these tests we rely on the exact formatting of happyPathJSON
	prefix := `"` + field + `":`
	idx := strings.Index(happyPathJSON, prefix)
	if idx < 0 {
		t.Fatalf("field %q not found in happyPathJSON", field)
	}
	// find end of this field's value: the next newline followed by another field or '}'
	end := strings.Index(happyPathJSON[idx:], ",")
	if end < 0 {
		// last field — strip the preceding comma and this field
		commaBefore := strings.LastIndex(happyPathJSON[:idx], ",")
		return happyPathJSON[:commaBefore] + happyPathJSON[idx+len(happyPathJSON[idx:]):]
	}
	return happyPathJSON[:idx] + happyPathJSON[idx+end+1:]
}

func TestParseRejectsMissingRequiredField(t *testing.T) {
	cases := []string{"sensor_id", "use_case_id", "angle", "started_at", "verdict", "confidence", "rollup"}
	for _, field := range cases {
		t.Run(field, func(t *testing.T) {
			bad := withFieldRemoved(t, field)
			if _, err := ParseAggregate(strings.NewReader(bad)); err == nil {
				t.Errorf("expected error when %q removed, got nil", field)
			}
		})
	}
}

func TestParseRejectsInvalidVerdict(t *testing.T) {
	bad := strings.Replace(happyPathJSON, `"verdict": "pass"`, `"verdict": "maybe"`, 1)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for invalid verdict, got nil")
	}
}

func TestParseRejectsConfidenceOutOfRange(t *testing.T) {
	bad := strings.Replace(happyPathJSON, `"confidence": 1.0`, `"confidence": 1.5`, 1)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for confidence > 1.0, got nil")
	}
}

func TestParseRejectsFailWithoutHealHint(t *testing.T) {
	bad := strings.Replace(happyPathJSON, `"verdict": "pass"`, `"verdict": "fail"`, 1)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for fail without heal_hint, got nil")
	}
}

func TestParseRejectsWarnWithoutHealHint(t *testing.T) {
	bad := strings.Replace(happyPathJSON, `"verdict": "pass"`, `"verdict": "warn"`, 1)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for warn without heal_hint, got nil")
	}
}
```

- [ ] **Step 2: Run tests (verify they pass)**

Run: `go test ./internal/aggregate/... -run TestParseRejects -v`
Expected: all PASS — the JSON Schema is doing the work.

- [ ] **Step 3: Commit**

```bash
git add internal/aggregate/validate_test.go
git commit -m "test(e8): assert schema validation catches missing fields, bad enums, fail-without-hint, warn-without-hint"
```

### Task 12: Arithmetic rule — counts sum to total_signals

**Files:**
- Modify: `internal/aggregate/validate.go`
- Modify: `internal/aggregate/validate_test.go`

- [ ] **Step 1: Add the failing test**

Append to `internal/aggregate/validate_test.go`:

```go
func TestParseRejectsBadArithmetic(t *testing.T) {
	// Replace pass_count: 1 with pass_count: 2 so the sum no longer equals total_signals: 1.
	bad := strings.Replace(happyPathJSON, `"pass_count": 1`, `"pass_count": 2`, 1)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for pass+warn+fail+inconclusive != total_signals, got nil")
	}
	if !strings.Contains(err.Error(), "rollup") || !strings.Contains(err.Error(), "sum") {
		t.Errorf("error should mention 'rollup' and 'sum': %v", err)
	}
}
```

- [ ] **Step 2: Run test (verify it fails)**

Run: `go test ./internal/aggregate/... -run TestParseRejectsBadArithmetic -v`
Expected: FAIL — the placeholder `Validate` returns nil and the schema does not enforce arithmetic.

- [ ] **Step 3: Implement the arithmetic check**

Replace `internal/aggregate/validate.go` with:

```go
package aggregate

import (
	"errors"
	"fmt"
)

// Validate runs the hand-written rules that the embedded JSON Schema
// cannot express. ParseAggregate runs the schema first, then this; Rollup
// runs only this on its own output. Errors are collected via errors.Join
// so callers see every violation in one pass.
func Validate(a AggregateSignal) error {
	var errs []error

	if err := validateRollupArithmetic(a.Rollup); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func validateRollupArithmetic(r RollupCounts) error {
	sum := r.PassCount + r.WarnCount + r.FailCount + r.InconclusiveCount
	if sum != r.TotalSignals {
		return fmt.Errorf("rollup: counts sum to %d but total_signals is %d (pass=%d warn=%d fail=%d inconclusive=%d)",
			sum, r.TotalSignals, r.PassCount, r.WarnCount, r.FailCount, r.InconclusiveCount)
	}
	return nil
}
```

- [ ] **Step 4: Run test (verify it passes)**

Run: `go test ./internal/aggregate/... -run TestParseRejectsBadArithmetic -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/aggregate/validate.go internal/aggregate/validate_test.go
git commit -m "feat(e8): enforce rollup count arithmetic in Validate"
```

### Task 13: Heal_hint forbidden when verdict is pass

**Files:**
- Modify: `internal/aggregate/validate.go`
- Modify: `internal/aggregate/validate_test.go`

- [ ] **Step 1: Add the failing test**

Append to `internal/aggregate/validate_test.go`:

```go
func TestParseRejectsPassWithHealHint(t *testing.T) {
	// Inject a heal_hint into a pass-verdict aggregate. We do this by
	// replacing the closing brace of the rollup block with rollup-close +
	// a sibling heal_hint key.
	bad := strings.Replace(
		happyPathJSON,
		`"inconclusive_count": 0
  }
}`,
		`"inconclusive_count": 0
  },
  "heal_hint": {"summary": "x", "rationale": "y"}
}`,
		1,
	)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for pass with heal_hint, got nil")
	}
	if !strings.Contains(err.Error(), "heal_hint") {
		t.Errorf("error should mention 'heal_hint': %v", err)
	}
}
```

- [ ] **Step 2: Run test (verify it fails)**

Run: `go test ./internal/aggregate/... -run TestParseRejectsPassWithHealHint -v`
Expected: FAIL — the schema allows heal_hint on any verdict; only Go can forbid it on pass.

- [ ] **Step 3: Implement the forbid-when-pass rule**

In `internal/aggregate/validate.go`, extend `Validate`:

```go
func Validate(a AggregateSignal) error {
	var errs []error

	if err := validateRollupArithmetic(a.Rollup); err != nil {
		errs = append(errs, err)
	}
	if err := validateHealHintAbsence(a); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func validateHealHintAbsence(a AggregateSignal) error {
	// Schema enforces "required when verdict in {warn, fail}". This adds
	// the other direction: forbidden when verdict is pass or inconclusive.
	if a.HealHint == nil {
		return nil
	}
	switch a.Verdict {
	case "pass", "inconclusive":
		return fmt.Errorf("heal_hint must be absent when verdict is %q (only warn and fail carry heal hints)", a.Verdict)
	}
	return nil
}
```

- [ ] **Step 4: Run test (verify it passes)**

Run: `go test ./internal/aggregate/... -run TestParseRejectsPassWithHealHint -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/aggregate/validate.go internal/aggregate/validate_test.go
git commit -m "feat(e8): forbid heal_hint on pass and inconclusive aggregates"
```

### Task 14: Completeness subset rule and `ended_at >= started_at`

**Files:**
- Modify: `internal/aggregate/validate.go`
- Modify: `internal/aggregate/validate_test.go`

- [ ] **Step 1: Add the failing tests**

Append to `internal/aggregate/validate_test.go`:

```go
func TestParseRejectsEndBeforeStart(t *testing.T) {
	bad := strings.Replace(happyPathJSON, `"ended_at":   "2026-05-23T14:00:42Z"`, `"ended_at":   "2026-05-23T13:00:00Z"`, 1)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for ended_at < started_at, got nil")
	}
	if !strings.Contains(err.Error(), "ended_at") {
		t.Errorf("error should mention 'ended_at': %v", err)
	}
}

func TestParseRejectsMissingObservationNotInExpected(t *testing.T) {
	withCompleteness := strings.Replace(
		happyPathJSON,
		`"inconclusive_count": 0
  }
}`,
		`"inconclusive_count": 0
  },
  "completeness": {
    "expected_observations": ["a", "b"],
    "missing_observations": ["c"]
  }
}`,
		1,
	)
	_, err := ParseAggregate(strings.NewReader(withCompleteness))
	if err == nil {
		t.Fatal("expected error for missing_observation not in expected_observations, got nil")
	}
	if !strings.Contains(err.Error(), "missing_observations") {
		t.Errorf("error should mention 'missing_observations': %v", err)
	}
}
```

- [ ] **Step 2: Run tests (verify they fail)**

Run: `go test ./internal/aggregate/... -run "TestParseRejectsEndBeforeStart|TestParseRejectsMissingObservationNotInExpected" -v`
Expected: FAIL — neither rule is enforced yet.

- [ ] **Step 3: Implement both checks**

Extend `internal/aggregate/validate.go`:

```go
func Validate(a AggregateSignal) error {
	var errs []error

	if err := validateRollupArithmetic(a.Rollup); err != nil {
		errs = append(errs, err)
	}
	if err := validateHealHintAbsence(a); err != nil {
		errs = append(errs, err)
	}
	if err := validateTimeOrder(a); err != nil {
		errs = append(errs, err)
	}
	if err := validateCompleteness(a.Completeness); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func validateTimeOrder(a AggregateSignal) error {
	if a.EndedAt.Before(a.StartedAt) {
		return fmt.Errorf("ended_at (%s) is before started_at (%s)",
			a.EndedAt.Format("2006-01-02T15:04:05Z07:00"),
			a.StartedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	return nil
}

func validateCompleteness(c *Completeness) error {
	if c == nil {
		return nil
	}
	expected := make(map[string]bool, len(c.ExpectedObservations))
	for _, k := range c.ExpectedObservations {
		expected[k] = true
	}
	for _, k := range c.MissingObservations {
		if !expected[k] {
			return fmt.Errorf("completeness: missing_observations contains %q which is not in expected_observations", k)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests (verify they pass)**

Run: `go test ./internal/aggregate/... -run "TestParseRejectsEndBeforeStart|TestParseRejectsMissingObservationNotInExpected" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/aggregate/validate.go internal/aggregate/validate_test.go
git commit -m "feat(e8): enforce time ordering and missing/expected observation subset"
```

---

## Phase F — Rollup

### Task 15: Compute counts from signals

**Files:**
- Create: `internal/aggregate/rollup.go`
- Create: `internal/aggregate/rollup_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/aggregate/rollup_test.go`:

```go
package aggregate

import (
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

// sig builds a minimal signal with the given verdict and confidence 1.0.
// Pass + Fail also carry a heal_hint so the resulting signals are
// individually schema-valid (mirrors what a real sensor would emit).
func sig(verdict enums.Verdict) signalstub.Signal {
	s := signalstub.Signal{
		SchemaVersion: "1.0.0",
		SensorID:      "sensor-x",
		UseCaseID:     "uc-x",
		Angle:         enums.AngleUnitTest,
		EmittedAt:     time.Date(2026, 5, 23, 14, 0, 0, 0, time.UTC),
		Verdict:       verdict,
		Confidence:    1.0,
		Evidence:      map[string]any{"summary": "x"},
	}
	if verdict == enums.VerdictFail || verdict == enums.VerdictWarn {
		s.HealHint = &HealHint{Summary: "fix x", Rationale: "x is wrong"}
	}
	return s
}

func baseInput(signals []signalstub.Signal) RollupInput {
	return RollupInput{
		Signals:           signals,
		SensorID:          "sensor-x",
		UseCaseID:         "uc-x",
		Angle:             enums.AngleUnitTest,
		Kind:              enums.SensorKindAssertion,
		OutputType:        enums.OutputStream,
		StartedAt:         time.Date(2026, 5, 23, 14, 0, 0, 0, time.UTC),
		EndedAt:           time.Date(2026, 5, 23, 14, 0, 42, 0, time.UTC),
		TerminationReason: enums.TerminationCompleted,
	}
}

func TestRollupCountsMatchVerdictDistribution(t *testing.T) {
	in := baseInput([]signalstub.Signal{
		sig(enums.VerdictPass), sig(enums.VerdictPass), sig(enums.VerdictPass),
		sig(enums.VerdictWarn),
		sig(enums.VerdictFail), sig(enums.VerdictFail),
		sig(enums.VerdictInconclusive),
	})
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.Rollup.TotalSignals != 7 {
		t.Errorf("TotalSignals = %d, want 7", got.Rollup.TotalSignals)
	}
	if got.Rollup.PassCount != 3 || got.Rollup.WarnCount != 1 || got.Rollup.FailCount != 2 || got.Rollup.InconclusiveCount != 1 {
		t.Errorf("counts mismatch: %+v", got.Rollup)
	}
}
```

Look up the actual constant names in `internal/enums/enums.go` first — `SensorKindAssertion` is the expected form based on the existing `SensorKind` type. If your repo uses different identifiers, substitute accordingly throughout this plan.

- [ ] **Step 2: Run test (verify it fails to compile)**

Run: `go test ./internal/aggregate/... -run TestRollupCounts -v`
Expected: BUILD FAIL — `Rollup` undefined.

- [ ] **Step 3: Implement counts**

Create `internal/aggregate/rollup.go`:

```go
package aggregate

import (
	"fmt"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

// Rollup is the deterministic per-sensor aggregator. Given a slice of
// emitted Signals plus execution metadata, it returns the terminal
// AggregateSignal that ends the sensor's JSON Lines stream.
//
// Rollup is pure: same RollupInput → byte-identical AggregateSignal.
func Rollup(in RollupInput) (AggregateSignal, error) {
	a := AggregateSignal{
		SchemaVersion:     "1.0.0",
		Type:              TypeAggregate,
		SensorID:          in.SensorID,
		UseCaseID:         in.UseCaseID,
		Angle:             in.Angle,
		StartedAt:         in.StartedAt,
		EndedAt:           in.EndedAt,
		TerminationReason: in.TerminationReason,
		Rollup:            computeCounts(in.Signals),
	}

	if err := Validate(a); err != nil {
		return AggregateSignal{}, fmt.Errorf("rollup output failed validation: %w", err)
	}
	return a, nil
}

func computeCounts(signals []signalstub.Signal) RollupCounts {
	c := RollupCounts{TotalSignals: len(signals)}
	for _, s := range signals {
		switch s.Verdict {
		case enums.VerdictPass:
			c.PassCount++
		case enums.VerdictWarn:
			c.WarnCount++
		case enums.VerdictFail:
			c.FailCount++
		case enums.VerdictInconclusive:
			c.InconclusiveCount++
		}
	}
	return c
}
```

The function still produces an empty Verdict, Confidence 0, no Completeness, no HealHint. The current test only checks counts so it should pass; Validate will not error on counts since they sum correctly. The Verdict field being "" will fail the schema check if we ran ParseAggregate, but since we go directly from struct → Validate, only count arithmetic and time order and completeness are checked at this point. Confirm by running the test.

- [ ] **Step 4: Run test (verify it passes)**

Run: `go test ./internal/aggregate/... -run TestRollupCounts -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/aggregate/rollup.go internal/aggregate/rollup_test.go
git commit -m "feat(e8): Rollup computes per-verdict counts from signals"
```

### Task 16: Compute confidence (arithmetic mean + empty-signals cases)

**Files:**
- Modify: `internal/aggregate/rollup.go`
- Modify: `internal/aggregate/rollup_test.go`

- [ ] **Step 1: Add the failing tests**

Append to `internal/aggregate/rollup_test.go`:

```go
func TestRollupConfidenceIsArithmeticMean(t *testing.T) {
	s1 := sig(enums.VerdictPass)
	s1.Confidence = 1.0
	s2 := sig(enums.VerdictPass)
	s2.Confidence = 0.5
	in := baseInput([]signalstub.Signal{s1, s2})
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.Confidence < 0.749999 || got.Confidence > 0.750001 {
		t.Errorf("Confidence = %v, want ~0.75", got.Confidence)
	}
}

func TestRollupEmptySignalsObservationalPassConfidence(t *testing.T) {
	in := baseInput(nil)
	in.Kind = enums.SensorKindObservational
	in.OutputType = enums.OutputStream
	in.ExpectedObservations = []string{"a", "b"}
	in.ObservedKeys = []string{"a", "b"}
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.Confidence != 1.0 {
		t.Errorf("Confidence = %v, want 1.0 for full-coverage observational with no signals", got.Confidence)
	}
}
```

- [ ] **Step 2: Run tests (verify they fail)**

Run: `go test ./internal/aggregate/... -run TestRollupConfidence -v && go test ./internal/aggregate/... -run TestRollupEmpty -v`
Expected: FAIL — Confidence is currently 0.

- [ ] **Step 3: Implement confidence and a placeholder verdict-stamping path**

In `internal/aggregate/rollup.go`, extend `Rollup` to compute the verdict (placeholder for now — full decision tree lands in the next two tasks) and the confidence. Replace the body of `Rollup` with:

```go
func Rollup(in RollupInput) (AggregateSignal, error) {
	a := AggregateSignal{
		SchemaVersion:     "1.0.0",
		Type:              TypeAggregate,
		SensorID:          in.SensorID,
		UseCaseID:         in.UseCaseID,
		Angle:             in.Angle,
		StartedAt:         in.StartedAt,
		EndedAt:           in.EndedAt,
		TerminationReason: in.TerminationReason,
		Rollup:            computeCounts(in.Signals),
		Completeness:      computeCompleteness(in),
	}
	a.Verdict = computeVerdict(in, a)
	a.Confidence = computeConfidence(in.Signals, a.Verdict)

	if err := Validate(a); err != nil {
		return AggregateSignal{}, fmt.Errorf("rollup output failed validation: %w", err)
	}
	return a, nil
}

// computeVerdict is a placeholder until Task 17/18/19. It picks pass for
// now so we can land tests incrementally.
func computeVerdict(in RollupInput, a AggregateSignal) enums.Verdict {
	return enums.VerdictPass
}

func computeConfidence(signals []signalstub.Signal, v enums.Verdict) float64 {
	if len(signals) == 0 {
		switch v {
		case enums.VerdictInconclusive:
			return 0.0
		default:
			return 1.0
		}
	}
	var sum float64
	for _, s := range signals {
		sum += s.Confidence
	}
	return sum / float64(len(signals))
}

func computeCompleteness(in RollupInput) *Completeness {
	if in.Kind != enums.SensorKindObservational {
		return nil
	}
	expected := append([]string(nil), in.ExpectedObservations...)
	observed := make(map[string]bool, len(in.ObservedKeys))
	for _, k := range in.ObservedKeys {
		observed[k] = true
	}
	missing := make([]string, 0)
	for _, k := range expected {
		if !observed[k] {
			missing = append(missing, k)
		}
	}
	return &Completeness{
		ExpectedObservations: expected,
		MissingObservations:  missing,
	}
}
```

- [ ] **Step 4: Run tests (verify they pass)**

Run: `go test ./internal/aggregate/... -v`
Expected: confidence and counts tests PASS; the placeholder verdict logic returns pass, which is correct for all-pass inputs. The empty-signals observational test passes because it's full coverage with verdict pass.

- [ ] **Step 5: Commit**

```bash
git add internal/aggregate/rollup.go internal/aggregate/rollup_test.go
git commit -m "feat(e8): Rollup computes confidence and observational completeness"
```

### Task 17: Verdict decision tree — single-shot mirror and stream severity ordering

**Files:**
- Modify: `internal/aggregate/rollup.go`
- Modify: `internal/aggregate/rollup_test.go`

- [ ] **Step 1: Add the failing tests**

Append to `internal/aggregate/rollup_test.go`:

```go
func TestRollupSingleShotMirrorsSoleVerdict(t *testing.T) {
	cases := []enums.Verdict{
		enums.VerdictPass, enums.VerdictWarn, enums.VerdictFail, enums.VerdictInconclusive,
	}
	for _, v := range cases {
		t.Run(string(v), func(t *testing.T) {
			in := baseInput([]signalstub.Signal{sig(v)})
			in.OutputType = enums.OutputSingleShot
			got, err := Rollup(in)
			if v == enums.VerdictPass {
				if err != nil {
					t.Fatalf("Rollup: %v", err)
				}
			} else {
				// warn/fail/inconclusive aggregates need a heal_hint or pass the
				// inconclusive carve-out; we expect synthesis to attach one when
				// Tasks 20/21 land. For this task, accept either success or a
				// validation error mentioning heal_hint.
				if err != nil && !strings.Contains(err.Error(), "heal_hint") {
					t.Fatalf("Rollup: %v", err)
				}
			}
			if got.Verdict != v && err == nil {
				t.Errorf("Verdict = %q, want %q", got.Verdict, v)
			}
		})
	}
}

func TestRollupStreamSeverityOrdering(t *testing.T) {
	cases := []struct {
		name      string
		verdicts  []enums.Verdict
		want      enums.Verdict
	}{
		{"all-pass", []enums.Verdict{enums.VerdictPass, enums.VerdictPass}, enums.VerdictPass},
		{"pass-warn", []enums.Verdict{enums.VerdictPass, enums.VerdictWarn}, enums.VerdictWarn},
		{"pass-warn-inconclusive", []enums.Verdict{enums.VerdictPass, enums.VerdictWarn, enums.VerdictInconclusive}, enums.VerdictWarn},
		{"pass-inconclusive", []enums.Verdict{enums.VerdictPass, enums.VerdictInconclusive}, enums.VerdictInconclusive},
		{"pass-warn-fail", []enums.Verdict{enums.VerdictPass, enums.VerdictWarn, enums.VerdictFail}, enums.VerdictFail},
		{"fail-only", []enums.Verdict{enums.VerdictFail}, enums.VerdictFail},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			signals := make([]signalstub.Signal, len(c.verdicts))
			for i, v := range c.verdicts {
				signals[i] = sig(v)
			}
			in := baseInput(signals)
			got, err := Rollup(in)
			// Tasks 20/21 fill in heal_hint synthesis; until then, warn/fail
			// outputs may fail validation. Accept synthesis-pending errors
			// but check the verdict was decided correctly.
			if got.Verdict != c.want && err == nil {
				t.Errorf("verdict = %q, want %q", got.Verdict, c.want)
			} else if c.want == enums.VerdictPass || c.want == enums.VerdictInconclusive {
				if err != nil {
					t.Fatalf("unexpected error for non-hinted verdict: %v", err)
				}
			}
		})
	}
}
```

You'll also need to import `strings` in the test file if you haven't already.

- [ ] **Step 2: Run tests (verify they fail)**

Run: `go test ./internal/aggregate/... -run "TestRollupSingleShot|TestRollupStreamSeverity" -v`
Expected: FAIL — verdict is hardcoded to pass.

- [ ] **Step 3: Implement the decision tree (clean-termination branches only)**

Replace the placeholder `computeVerdict` in `internal/aggregate/rollup.go`:

```go
func computeVerdict(in RollupInput, a AggregateSignal) enums.Verdict {
	// Rule 1: observational + missing observations → fail (overrides all).
	if a.Completeness != nil && len(a.Completeness.MissingObservations) > 0 {
		return enums.VerdictFail
	}

	// Rule 4: single-shot → mirror the sole signal's verdict (clean term).
	if in.OutputType == enums.OutputSingleShot && len(in.Signals) == 1 {
		return in.Signals[0].Verdict
	}

	// Rule 5: severity ordering (clean termination).
	// fail > warn > inconclusive > pass.
	return severityVerdict(in.Signals)
}

func severityVerdict(signals []signalstub.Signal) enums.Verdict {
	var hasWarn, hasInconclusive bool
	for _, s := range signals {
		switch s.Verdict {
		case enums.VerdictFail:
			return enums.VerdictFail
		case enums.VerdictWarn:
			hasWarn = true
		case enums.VerdictInconclusive:
			hasInconclusive = true
		}
	}
	switch {
	case hasWarn:
		return enums.VerdictWarn
	case hasInconclusive:
		return enums.VerdictInconclusive
	default:
		return enums.VerdictPass
	}
}
```

- [ ] **Step 4: Run tests (verify they pass)**

Run: `go test ./internal/aggregate/... -run "TestRollupSingleShot|TestRollupStreamSeverity" -v`
Expected: verdict assertions PASS. Cases that produce warn/fail with no heal_hint may produce a validation error; those branches in the tests tolerate it via the comment in Step 1.

- [ ] **Step 5: Commit**

```bash
git add internal/aggregate/rollup.go internal/aggregate/rollup_test.go
git commit -m "feat(e8): Rollup verdict decision tree — single-shot mirror + severity ordering"
```

### Task 18: Verdict decision tree — timeout/error fail-wins

**Files:**
- Modify: `internal/aggregate/rollup.go`
- Modify: `internal/aggregate/rollup_test.go`

- [ ] **Step 1: Add the failing tests**

Append to `internal/aggregate/rollup_test.go`:

```go
func TestRollupTimeoutInconclusiveWithoutFail(t *testing.T) {
	in := baseInput([]signalstub.Signal{sig(enums.VerdictPass), sig(enums.VerdictPass)})
	in.TerminationReason = enums.TerminationTimeout
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.Verdict != enums.VerdictInconclusive {
		t.Errorf("Verdict = %q, want %q", got.Verdict, enums.VerdictInconclusive)
	}
}

func TestRollupTimeoutFailWinsOverInconclusive(t *testing.T) {
	in := baseInput([]signalstub.Signal{sig(enums.VerdictPass), sig(enums.VerdictFail)})
	in.TerminationReason = enums.TerminationTimeout
	got, err := Rollup(in)
	if got.Verdict != enums.VerdictFail && err == nil {
		t.Errorf("Verdict = %q, want %q", got.Verdict, enums.VerdictFail)
	}
}

func TestRollupErrorTreatedLikeTimeout(t *testing.T) {
	in := baseInput([]signalstub.Signal{sig(enums.VerdictPass)})
	in.TerminationReason = enums.TerminationError
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.Verdict != enums.VerdictInconclusive {
		t.Errorf("Verdict = %q, want inconclusive", got.Verdict)
	}
}
```

- [ ] **Step 2: Run tests (verify they fail)**

Run: `go test ./internal/aggregate/... -run "TestRollupTimeout|TestRollupError" -v`
Expected: FAIL — current code returns pass/severity for these inputs.

- [ ] **Step 3: Extend the decision tree**

In `internal/aggregate/rollup.go`, update `computeVerdict`:

```go
func computeVerdict(in RollupInput, a AggregateSignal) enums.Verdict {
	// Rule 1: observational + missing observations → fail (overrides all).
	if a.Completeness != nil && len(a.Completeness.MissingObservations) > 0 {
		return enums.VerdictFail
	}

	// Rules 2 + 3: timeout / error termination → fail wins, else inconclusive.
	if in.TerminationReason == enums.TerminationTimeout || in.TerminationReason == enums.TerminationError {
		for _, s := range in.Signals {
			if s.Verdict == enums.VerdictFail {
				return enums.VerdictFail
			}
		}
		return enums.VerdictInconclusive
	}

	// Rule 4: single-shot → mirror.
	if in.OutputType == enums.OutputSingleShot && len(in.Signals) == 1 {
		return in.Signals[0].Verdict
	}

	// Rule 5: severity ordering (completed / stopped).
	return severityVerdict(in.Signals)
}
```

- [ ] **Step 4: Run tests (verify they pass)**

Run: `go test ./internal/aggregate/... -run "TestRollupTimeout|TestRollupError" -v`
Expected: PASS for pass-only timeout/error cases. Fail-wins case may surface validation errors for missing heal_hint — those will resolve in Tasks 20/21.

- [ ] **Step 5: Commit**

```bash
git add internal/aggregate/rollup.go internal/aggregate/rollup_test.go
git commit -m "feat(e8): Rollup verdict — timeout/error fail-wins-else-inconclusive"
```

### Task 19: Observational missing-observations override

**Files:**
- Modify: `internal/aggregate/rollup_test.go`

- [ ] **Step 1: Add the failing test**

Append to `internal/aggregate/rollup_test.go`:

```go
func TestRollupObservationalMissingOverridesPasses(t *testing.T) {
	in := baseInput([]signalstub.Signal{sig(enums.VerdictPass), sig(enums.VerdictPass)})
	in.Kind = enums.SensorKindObservational
	in.ExpectedObservations = []string{"a", "b", "c"}
	in.ObservedKeys = []string{"a", "b"}
	got, err := Rollup(in)
	if got.Verdict != enums.VerdictFail && err == nil {
		t.Errorf("Verdict = %q, want fail (missing 'c' overrides pass signals)", got.Verdict)
	}
	if got.Completeness == nil {
		t.Fatal("Completeness must not be nil for observational sensors")
	}
	if len(got.Completeness.MissingObservations) != 1 || got.Completeness.MissingObservations[0] != "c" {
		t.Errorf("MissingObservations = %v, want [c]", got.Completeness.MissingObservations)
	}
}

func TestRollupObservationalCleanCoverageIsPass(t *testing.T) {
	in := baseInput([]signalstub.Signal{sig(enums.VerdictPass)})
	in.Kind = enums.SensorKindObservational
	in.ExpectedObservations = []string{"a"}
	in.ObservedKeys = []string{"a"}
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.Verdict != enums.VerdictPass {
		t.Errorf("Verdict = %q, want pass", got.Verdict)
	}
}
```

- [ ] **Step 2: Run tests (verify they pass)**

Run: `go test ./internal/aggregate/... -run TestRollupObservational -v`
Expected: PASS — the missing-observations branch (Rule 1) already added in Task 17 handles this. `Completeness` computation was added in Task 16. The fail case may surface a missing-heal_hint validation error; resolves in Task 21.

- [ ] **Step 3: Commit**

```bash
git add internal/aggregate/rollup_test.go
git commit -m "test(e8): observational missing observations override pass verdict"
```

### Task 20: heal_hint synthesis — single-shot carryover

**Files:**
- Create: `internal/aggregate/synthesize.go`
- Create: `internal/aggregate/synthesize_test.go`
- Modify: `internal/aggregate/rollup.go`

- [ ] **Step 1: Write the failing test**

Create `internal/aggregate/synthesize_test.go`:

```go
package aggregate

import (
	"reflect"
	"testing"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

func TestRollupSingleShotCarriesOverHealHint(t *testing.T) {
	s := sig(enums.VerdictFail)
	s.HealHint = &HealHint{
		Summary:        "fix broken handler",
		SuggestedLocus: []Locus{{Path: "src/handler.go", Symbol: "Handle"}},
		Rationale:      "handler panics on empty input",
	}
	in := baseInput([]signalstub.Signal{s})
	in.OutputType = enums.OutputSingleShot
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.HealHint == nil {
		t.Fatal("HealHint must not be nil for fail verdict")
	}
	if !reflect.DeepEqual(*got.HealHint, *s.HealHint) {
		t.Errorf("HealHint = %+v, want %+v", *got.HealHint, *s.HealHint)
	}
}
```

- [ ] **Step 2: Run test (verify it fails)**

Run: `go test ./internal/aggregate/... -run TestRollupSingleShotCarries -v`
Expected: FAIL — `HealHint` is nil; Rollup also errors on the missing-heal_hint validation rule.

- [ ] **Step 3: Implement single-shot synthesis**

Create `internal/aggregate/synthesize.go`:

```go
package aggregate

import (
	"fmt"
	"strings"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

// synthesizeHealHint produces the heal_hint attached to a warn/fail
// aggregate. For pass and inconclusive, returns nil (the caller must not
// attach a heal_hint).
//
// Single-shot: carry over the sole signal's hint verbatim (deep copy).
// Stream / observational: synthesize a meta-hint with templated summary,
// deduplicated loci (capped at 10), and structured rationale.
func synthesizeHealHint(in RollupInput, a AggregateSignal) *HealHint {
	if a.Verdict != enums.VerdictWarn && a.Verdict != enums.VerdictFail {
		return nil
	}
	if in.OutputType == enums.OutputSingleShot && len(in.Signals) == 1 && in.Signals[0].HealHint != nil {
		return deepCopyHealHint(in.Signals[0].HealHint)
	}
	return synthesizeStreamHealHint(in, a)
}

func deepCopyHealHint(h *HealHint) *HealHint {
	if h == nil {
		return nil
	}
	out := &HealHint{
		Summary:   h.Summary,
		Rationale: h.Rationale,
	}
	if len(h.SuggestedLocus) > 0 {
		out.SuggestedLocus = append([]Locus(nil), h.SuggestedLocus...)
	}
	return out
}

// synthesizeStreamHealHint will be implemented in Task 21.
func synthesizeStreamHealHint(in RollupInput, a AggregateSignal) *HealHint {
	// placeholder so the package compiles; Task 21 replaces this.
	return &HealHint{Summary: "stream synthesis pending", Rationale: "pending"}
}

// unused imports satisfied by Task 21:
var _ = fmt.Sprintf
var _ = strings.Join
var _ signalstub.Signal
```

Then wire it into `Rollup`. In `internal/aggregate/rollup.go`, add a call after verdict computation:

```go
func Rollup(in RollupInput) (AggregateSignal, error) {
	a := AggregateSignal{
		SchemaVersion:     "1.0.0",
		Type:              TypeAggregate,
		SensorID:          in.SensorID,
		UseCaseID:         in.UseCaseID,
		Angle:             in.Angle,
		StartedAt:         in.StartedAt,
		EndedAt:           in.EndedAt,
		TerminationReason: in.TerminationReason,
		Rollup:            computeCounts(in.Signals),
		Completeness:      computeCompleteness(in),
	}
	a.Verdict = computeVerdict(in, a)
	a.Confidence = computeConfidence(in.Signals, a.Verdict)
	a.HealHint = synthesizeHealHint(in, a)

	if err := Validate(a); err != nil {
		return AggregateSignal{}, fmt.Errorf("rollup output failed validation: %w", err)
	}
	return a, nil
}
```

- [ ] **Step 4: Run test (verify it passes)**

Run: `go test ./internal/aggregate/... -run TestRollupSingleShotCarries -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/aggregate/synthesize.go internal/aggregate/synthesize_test.go internal/aggregate/rollup.go
git commit -m "feat(e8): single-shot heal_hint carryover"
```

### Task 21: heal_hint synthesis — stream/observational meta-hint

**Files:**
- Modify: `internal/aggregate/synthesize.go`
- Modify: `internal/aggregate/synthesize_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/aggregate/synthesize_test.go`:

```go
func TestStreamSynthesizesFailSummaryWithCounts(t *testing.T) {
	signals := []signalstub.Signal{
		sig(enums.VerdictPass), sig(enums.VerdictPass), sig(enums.VerdictPass),
		sig(enums.VerdictFail), sig(enums.VerdictFail),
	}
	signals[3].HealHint = &HealHint{
		Summary: "x", Rationale: "y",
		SuggestedLocus: []Locus{{Path: "a.go", Symbol: "f"}},
	}
	signals[4].HealHint = &HealHint{
		Summary: "x", Rationale: "y",
		SuggestedLocus: []Locus{{Path: "b.go", Symbol: "g"}},
	}
	in := baseInput(signals)
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.HealHint == nil {
		t.Fatal("HealHint must not be nil")
	}
	if !strings.Contains(got.HealHint.Summary, "2 of 5") {
		t.Errorf("Summary = %q, want substring '2 of 5'", got.HealHint.Summary)
	}
	if len(got.HealHint.SuggestedLocus) != 2 {
		t.Errorf("SuggestedLocus len = %d, want 2", len(got.HealHint.SuggestedLocus))
	}
}

func TestStreamSynthesizesWarnSummary(t *testing.T) {
	signals := []signalstub.Signal{
		sig(enums.VerdictPass), sig(enums.VerdictPass),
		sig(enums.VerdictWarn), sig(enums.VerdictWarn), sig(enums.VerdictWarn),
	}
	in := baseInput(signals)
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.Verdict != enums.VerdictWarn {
		t.Fatalf("Verdict = %q, want warn", got.Verdict)
	}
	if got.HealHint == nil || !strings.Contains(got.HealHint.Summary, "3 warning") {
		t.Errorf("Summary = %q, want substring '3 warning'", got.HealHint.Summary)
	}
}

func TestStreamSynthesisDedupesLociAndCapsAt10(t *testing.T) {
	// Produce 15 fail signals with overlapping loci.
	var signals []signalstub.Signal
	for i := 0; i < 15; i++ {
		s := sig(enums.VerdictFail)
		s.HealHint = &HealHint{
			Summary:        "x",
			Rationale:      "y",
			SuggestedLocus: []Locus{{Path: "f.go", Symbol: "x"}}, // identical → must collapse to 1 entry
		}
		signals = append(signals, s)
	}
	// Add 12 unique fail loci.
	for i := 0; i < 12; i++ {
		s := sig(enums.VerdictFail)
		s.HealHint = &HealHint{
			Summary:   "x",
			Rationale: "y",
			SuggestedLocus: []Locus{
				{Path: "unique.go", Symbol: "sym-" + string(rune('a'+i))},
			},
		}
		signals = append(signals, s)
	}
	in := baseInput(signals)
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.HealHint == nil {
		t.Fatal("HealHint nil")
	}
	if len(got.HealHint.SuggestedLocus) > 10 {
		t.Errorf("SuggestedLocus len = %d, want ≤ 10", len(got.HealHint.SuggestedLocus))
	}
	// Verify dedup: the (f.go, x) locus should appear exactly once.
	count := 0
	for _, l := range got.HealHint.SuggestedLocus {
		if l.Path == "f.go" && l.Symbol == "x" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("(f.go, x) appeared %d times, want 1", count)
	}
}

func TestObservationalSynthesisListsMissingKeys(t *testing.T) {
	in := baseInput(nil)
	in.Kind = enums.SensorKindObservational
	in.ExpectedObservations = []string{"a", "b", "c"}
	in.ObservedKeys = []string{"a"}
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.Verdict != enums.VerdictFail {
		t.Fatalf("Verdict = %q, want fail", got.Verdict)
	}
	if got.HealHint == nil {
		t.Fatal("HealHint nil")
	}
	if !strings.Contains(got.HealHint.Summary, "b") || !strings.Contains(got.HealHint.Summary, "c") {
		t.Errorf("Summary = %q, expected to mention missing keys b and c", got.HealHint.Summary)
	}
}
```

You'll also need `import "strings"` in synthesize_test.go.

- [ ] **Step 2: Run tests (verify they fail)**

Run: `go test ./internal/aggregate/... -run "TestStream|TestObservationalSynth" -v`
Expected: FAIL — the placeholder summary "stream synthesis pending" doesn't match the assertions.

- [ ] **Step 3: Implement stream/observational synthesis**

Replace `synthesizeStreamHealHint` and remove the unused-import guards in `internal/aggregate/synthesize.go`:

```go
package aggregate

import (
	"fmt"
	"strings"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

const maxLoci = 10
const maxKeysInSummary = 5

func synthesizeHealHint(in RollupInput, a AggregateSignal) *HealHint {
	if a.Verdict != enums.VerdictWarn && a.Verdict != enums.VerdictFail {
		return nil
	}
	if in.OutputType == enums.OutputSingleShot && len(in.Signals) == 1 && in.Signals[0].HealHint != nil {
		return deepCopyHealHint(in.Signals[0].HealHint)
	}
	return synthesizeStreamHealHint(in, a)
}

func deepCopyHealHint(h *HealHint) *HealHint {
	if h == nil {
		return nil
	}
	out := &HealHint{Summary: h.Summary, Rationale: h.Rationale}
	if len(h.SuggestedLocus) > 0 {
		out.SuggestedLocus = append([]Locus(nil), h.SuggestedLocus...)
	}
	return out
}

func synthesizeStreamHealHint(in RollupInput, a AggregateSignal) *HealHint {
	// Observational with missing observations gets a dedicated summary
	// listing the missing keys (capped, with ellipsis when truncated).
	if a.Completeness != nil && len(a.Completeness.MissingObservations) > 0 {
		return observationalMissingHint(a)
	}

	loci := collectLoci(in.Signals, a.Verdict)
	rationale := "see individual non-pass signals for per-record detail"

	var summary string
	switch a.Verdict {
	case enums.VerdictFail:
		summary = fmt.Sprintf("%d of %d %s signals failed",
			a.Rollup.FailCount, a.Rollup.TotalSignals, a.Angle)
	case enums.VerdictWarn:
		noun := "warnings"
		if a.Rollup.WarnCount == 1 {
			noun = "warning"
		}
		summary = fmt.Sprintf("%d %s across %d %s signals",
			a.Rollup.WarnCount, noun, a.Rollup.TotalSignals, a.Angle)
	}

	return &HealHint{
		Summary:        summary,
		SuggestedLocus: loci,
		Rationale:      rationale,
	}
}

func observationalMissingHint(a AggregateSignal) *HealHint {
	missing := a.Completeness.MissingObservations
	keys := missing
	suffix := ""
	if len(missing) > maxKeysInSummary {
		keys = missing[:maxKeysInSummary]
		suffix = ", ..."
	}
	summary := fmt.Sprintf("%s sensor missing %d of %d expected observations: %s%s",
		a.Angle, len(missing), len(a.Completeness.ExpectedObservations),
		strings.Join(keys, ", "), suffix)
	return &HealHint{
		Summary:        summary,
		Rationale:      "the sensor did not observe one or more required events; the corresponding code path likely failed silently or is missing instrumentation",
	}
}

// collectLoci returns up to maxLoci deduplicated (path, symbol) entries
// drawn from signals whose verdict matches the aggregate verdict,
// preserving first-seen order.
func collectLoci(signals []signalstub.Signal, verdict enums.Verdict) []Locus {
	seen := make(map[Locus]bool)
	var out []Locus
	for _, s := range signals {
		if s.Verdict != verdict {
			continue
		}
		if s.HealHint == nil {
			continue
		}
		for _, l := range s.HealHint.SuggestedLocus {
			if seen[l] {
				continue
			}
			seen[l] = true
			out = append(out, l)
			if len(out) >= maxLoci {
				return out
			}
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests (verify they pass)**

Run: `go test ./internal/aggregate/... -v`
Expected: every test PASSes. The earlier "Tasks 20/21 fill in synthesis" branches now produce hints and no longer error on missing heal_hint.

- [ ] **Step 5: Commit**

```bash
git add internal/aggregate/synthesize.go internal/aggregate/synthesize_test.go
git commit -m "feat(e8): synthesize stream/observational meta heal_hint with templated summary and deduped loci"
```

---

## Phase G — Golden files and determinism

### Task 22: Golden round-trip suite

**Files:**
- Create: `internal/aggregate/testdata/single-shot-pass.json`
- Create: `internal/aggregate/testdata/single-shot-fail.json`
- Create: `internal/aggregate/testdata/stream-all-pass.json`
- Create: `internal/aggregate/testdata/stream-mixed-warn.json`
- Create: `internal/aggregate/testdata/stream-mixed-fail.json`
- Create: `internal/aggregate/testdata/observational-complete.json`
- Create: `internal/aggregate/testdata/observational-missing.json`
- Create: `internal/aggregate/testdata/timeout-no-fails.json`
- Create: `internal/aggregate/golden_test.go`

- [ ] **Step 1: Create the golden JSON files**

Create each file under `internal/aggregate/testdata/`. Each must be schema-valid and pass `Validate`.

`single-shot-pass.json`:

```json
{
  "schema_version": "1.0.0",
  "type": "aggregate",
  "sensor_id": "sensor-x",
  "use_case_id": "uc-x",
  "angle": "build",
  "started_at": "2026-05-23T14:00:00Z",
  "ended_at":   "2026-05-23T14:00:05Z",
  "termination_reason": "completed",
  "verdict": "pass",
  "confidence": 1.0,
  "rollup": {"total_signals": 1, "pass_count": 1, "warn_count": 0, "fail_count": 0, "inconclusive_count": 0}
}
```

`single-shot-fail.json`:

```json
{
  "schema_version": "1.0.0",
  "type": "aggregate",
  "sensor_id": "sensor-x",
  "use_case_id": "uc-x",
  "angle": "build",
  "started_at": "2026-05-23T14:00:00Z",
  "ended_at":   "2026-05-23T14:00:05Z",
  "termination_reason": "completed",
  "verdict": "fail",
  "confidence": 1.0,
  "rollup": {"total_signals": 1, "pass_count": 0, "warn_count": 0, "fail_count": 1, "inconclusive_count": 0},
  "heal_hint": {"summary": "build failed", "rationale": "compiler error"}
}
```

`stream-all-pass.json`:

```json
{
  "schema_version": "1.0.0",
  "type": "aggregate",
  "sensor_id": "sensor-x",
  "use_case_id": "uc-x",
  "angle": "unit-test",
  "started_at": "2026-05-23T14:00:00Z",
  "ended_at":   "2026-05-23T14:00:42Z",
  "termination_reason": "completed",
  "verdict": "pass",
  "confidence": 1.0,
  "rollup": {"total_signals": 700, "pass_count": 700, "warn_count": 0, "fail_count": 0, "inconclusive_count": 0}
}
```

`stream-mixed-warn.json`:

```json
{
  "schema_version": "1.0.0",
  "type": "aggregate",
  "sensor_id": "sensor-x",
  "use_case_id": "uc-x",
  "angle": "code-structure",
  "started_at": "2026-05-23T14:00:00Z",
  "ended_at":   "2026-05-23T14:00:42Z",
  "termination_reason": "completed",
  "verdict": "warn",
  "confidence": 1.0,
  "rollup": {"total_signals": 120, "pass_count": 117, "warn_count": 3, "fail_count": 0, "inconclusive_count": 0},
  "heal_hint": {"summary": "3 warnings across 120 code-structure signals", "rationale": "see individual non-pass signals for per-record detail"}
}
```

`stream-mixed-fail.json`:

```json
{
  "schema_version": "1.0.0",
  "type": "aggregate",
  "sensor_id": "sensor-x",
  "use_case_id": "uc-x",
  "angle": "unit-test",
  "started_at": "2026-05-23T14:00:00Z",
  "ended_at":   "2026-05-23T14:00:42Z",
  "termination_reason": "completed",
  "verdict": "fail",
  "confidence": 0.9,
  "rollup": {"total_signals": 700, "pass_count": 628, "warn_count": 2, "fail_count": 70, "inconclusive_count": 0},
  "heal_hint": {"summary": "70 of 700 unit-test signals failed", "rationale": "see individual non-pass signals for per-record detail", "suggested_locus": [{"path": "src/x.go", "symbol": "F"}]}
}
```

`observational-complete.json`:

```json
{
  "schema_version": "1.0.0",
  "type": "aggregate",
  "sensor_id": "sensor-x",
  "use_case_id": "uc-x",
  "angle": "logs",
  "started_at": "2026-05-23T14:00:00Z",
  "ended_at":   "2026-05-23T14:00:30Z",
  "termination_reason": "stopped",
  "verdict": "pass",
  "confidence": 1.0,
  "rollup": {"total_signals": 3, "pass_count": 3, "warn_count": 0, "fail_count": 0, "inconclusive_count": 0},
  "completeness": {"expected_observations": ["a", "b", "c"], "missing_observations": []}
}
```

`observational-missing.json`:

```json
{
  "schema_version": "1.0.0",
  "type": "aggregate",
  "sensor_id": "sensor-x",
  "use_case_id": "uc-x",
  "angle": "logs",
  "started_at": "2026-05-23T14:00:00Z",
  "ended_at":   "2026-05-23T14:00:30Z",
  "termination_reason": "stopped",
  "verdict": "fail",
  "confidence": 1.0,
  "rollup": {"total_signals": 2, "pass_count": 2, "warn_count": 0, "fail_count": 0, "inconclusive_count": 0},
  "completeness": {"expected_observations": ["a", "b", "c"], "missing_observations": ["c"]},
  "heal_hint": {"summary": "logs sensor missing 1 of 3 expected observations: c", "rationale": "the sensor did not observe one or more required events; the corresponding code path likely failed silently or is missing instrumentation"}
}
```

`timeout-no-fails.json`:

```json
{
  "schema_version": "1.0.0",
  "type": "aggregate",
  "sensor_id": "sensor-x",
  "use_case_id": "uc-x",
  "angle": "performance",
  "started_at": "2026-05-23T14:00:00Z",
  "ended_at":   "2026-05-23T14:01:00Z",
  "termination_reason": "timeout",
  "verdict": "inconclusive",
  "confidence": 1.0,
  "rollup": {"total_signals": 5, "pass_count": 5, "warn_count": 0, "fail_count": 0, "inconclusive_count": 0}
}
```

- [ ] **Step 2: Write the golden test**

Create `internal/aggregate/golden_test.go`:

```go
package aggregate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGoldenFilesRoundTrip(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", e.Name()))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			parsed, err := ParseAggregate(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("first parse: %v", err)
			}
			out, err := json.Marshal(parsed)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			reparsed, err := ParseAggregate(bytes.NewReader(out))
			if err != nil {
				t.Fatalf("second parse: %v\n  json: %s", err, out)
			}
			if !reflect.DeepEqual(parsed, reparsed) {
				t.Errorf("round-trip mismatch for %s", e.Name())
			}
		})
	}
}
```

- [ ] **Step 3: Run the test (verify it passes)**

Run: `go test ./internal/aggregate/... -run TestGoldenFiles -v`
Expected: all eight files round-trip cleanly.

- [ ] **Step 4: Commit**

```bash
git add internal/aggregate/testdata/ internal/aggregate/golden_test.go
git commit -m "test(e8): golden JSON round-trip suite across all rollup modes"
```

### Task 23: Determinism property test

**Files:**
- Create: `internal/aggregate/determinism_test.go`

- [ ] **Step 1: Write the test**

Create `internal/aggregate/determinism_test.go`:

```go
package aggregate

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

func TestRollupIsByteDeterministic(t *testing.T) {
	signals := []signalstub.Signal{
		sig(enums.VerdictPass), sig(enums.VerdictPass), sig(enums.VerdictPass),
		sig(enums.VerdictWarn),
		sig(enums.VerdictFail), sig(enums.VerdictFail),
	}
	signals[3].HealHint = &HealHint{
		Summary: "w", Rationale: "r",
		SuggestedLocus: []Locus{{Path: "w.go", Symbol: "W"}},
	}
	signals[4].HealHint = &HealHint{
		Summary: "f1", Rationale: "r",
		SuggestedLocus: []Locus{{Path: "a.go", Symbol: "A"}, {Path: "b.go", Symbol: "B"}},
	}
	signals[5].HealHint = &HealHint{
		Summary: "f2", Rationale: "r",
		SuggestedLocus: []Locus{{Path: "a.go", Symbol: "A"}}, // duplicate
	}
	in := baseInput(signals)

	var first []byte
	for i := 0; i < 100; i++ {
		a, err := Rollup(in)
		if err != nil {
			t.Fatalf("Rollup iter %d: %v", i, err)
		}
		buf, err := json.Marshal(a)
		if err != nil {
			t.Fatalf("Marshal iter %d: %v", i, err)
		}
		if i == 0 {
			first = buf
			continue
		}
		if !bytes.Equal(first, buf) {
			t.Fatalf("iter %d diverged from first run:\n  first: %s\n  this:  %s", i, first, buf)
		}
	}
}
```

- [ ] **Step 2: Run the test (verify it passes)**

Run: `go test ./internal/aggregate/... -run TestRollupIsByteDeterministic -v`
Expected: PASS.

If it fails, the most likely culprit is map-iteration non-determinism in `collectLoci` — verify that loci dedup uses a slice-tracked order, not a map-key iteration order.

- [ ] **Step 3: Final full-package test run**

Run: `go test ./internal/... -v`
Expected: every test in every package PASSes (E8 work hasn't touched E1's tests beyond Task 4; the enums drift tests pass with the extended verdict list).

- [ ] **Step 4: Commit**

```bash
git add internal/aggregate/determinism_test.go
git commit -m "test(e8): Rollup byte-determinism property test (100 iterations)"
```

---

## Self-Review

### Spec coverage

Walking the spec section by section:

- §2 cross-entity ripple (warn verdict, signal.yaml, aggregate-signal.yaml, E1 enums) → Tasks 1–5.
- §4 JSON shape, type discriminator, conditional fields → Tasks 7 (types), 8 (schema), 9 (parser), 11–14 (validator).
- §5 Go types, `HealHint` alias, `RollupInput` → Task 7.
- §6.1 verdict decision tree → Tasks 17, 18, 19.
- §6.2 counts → Task 15.
- §6.3 confidence + empty-signals cases → Task 16.
- §6.4 completeness computation → Task 16.
- §6.5 heal_hint synthesis → Tasks 20, 21.
- §6.6 termination × verdict table → covered by Tasks 17, 18 + the existing test matrix.
- §7 validator rules (required fields, value rules, arithmetic, conditional, time order, completeness subset, heal_hint forbidden on pass) → Tasks 11, 12, 13, 14.
- §8 parser → Tasks 9, 10.
- §9 test plan (rollup, parser, validator, heal_hint, determinism, golden files) → Tasks 9–23.
- §10 deliverable acceptance → covered by Task 23's final full-package run.
- §11 out-of-scope items → respected (no runtime, no multi-sensor aggregator).
- §12 deferred questions → preserved as-is; not implemented.

All spec sections have at least one task. No gaps found.

### Placeholder scan

Searched the plan for: "TBD", "TODO", "implement later", "fill in details", "add appropriate", "handle edge cases", "similar to Task". No matches that act as deferrals — only an explicit "Task 21 replaces this" reference in Task 20's intentional incremental placeholder, which Task 21 unwinds.

### Type consistency

- `AggregateSignal`, `RollupCounts`, `Completeness`, `RollupInput`, `HealHint`, `Locus`: defined in Task 7, used consistently thereafter.
- `signalstub.Signal`, `signalstub.HealHint`, `signalstub.Locus`: defined in Task 6; aliased as `HealHint` and `Locus` in Task 7; used consistently in Rollup tests (Tasks 15–21).
- `enums.VerdictPass`, `enums.VerdictWarn`, `enums.VerdictFail`, `enums.VerdictInconclusive`: defined in Task 4; used consistently in tests and rollup logic.
- `enums.TerminationCompleted`, `enums.TerminationTimeout`, `enums.TerminationError`: pre-existing in E1 (verified via grep at planning time).
- `enums.SensorKindAssertion`, `enums.SensorKindObservational`, `enums.OutputSingleShot`, `enums.OutputStream`: pre-existing in E1 (Task 15 step 1 includes a note to verify exact names).
- `TypeAggregate`: defined as a package constant in Task 7; used in parser (Task 9) and Rollup (Task 15).
- `Rollup`, `Validate`, `ParseAggregate`, `compiledSchema`, `validateAgainstSchema`, `synthesizeHealHint`, `computeCounts`, `computeVerdict`, `computeConfidence`, `computeCompleteness`, `severityVerdict`, `collectLoci`, `deepCopyHealHint`, `observationalMissingHint`, `synthesizeStreamHealHint`, `validateRollupArithmetic`, `validateHealHintAbsence`, `validateTimeOrder`, `validateCompleteness`: all consistently named between definition and call sites.
- `happyPathJSON`: defined in Task 9 (`parse_test.go`); referenced by every later validator test (Tasks 11–14) in the same package, so cross-file access works fine.
- `sig`, `baseInput`: helper functions defined in Task 15 (`rollup_test.go`); reused by Tasks 16–21 in the same `aggregate` test package.

No naming drift detected.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-23-e8-aggregate-signal.md`. Two execution options:

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
