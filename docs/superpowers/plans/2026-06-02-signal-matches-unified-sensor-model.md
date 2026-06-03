# Signal Matches — Unified Sensor Signal Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `expected_observations` with a unified `signal_matches` mechanism on all sensors — regex-driven, verdict-bearing signals with named-capture evidence and an `expected` completeness flag — plus richer composable core primitives.

**Architecture:** A `SignalMatch` (key, pattern, verdict, confidence, expected, heal_hint) declared on any sensor. The executor compiles each pattern to a Go RE2 regex, tests every stdout/stderr line, and synthesizes a schema-valid `Signal` per match (named captures → evidence). `expected` matcher keys feed the existing completeness/rollup path, widened to any sensor kind. Lib-awareness lives in the generator prompts; the harness stays a generic regex engine.

**Tech Stack:** Go 1.25, `regexp` (RE2), `sigs.k8s.io/yaml`, `santhosh-tekuri/jsonschema/v6`, testify-free table tests (existing style).

**Spec:** `docs/superpowers/specs/2026-06-02-signal-matches-unified-sensor-model.md`

**Branch:** work continues on `feat/detached-observational-watcher` (this reworks the `expected_observations` code added there).

---

## File Structure

| File | Responsibility | Change |
|------|----------------|--------|
| `schemas/sensor.yaml` | Sensor JSON Schema | Replace `expected_observations` with `signal_matches` |
| `internal/sensor/types.go` | `Sensor` struct | `ExpectedObservations []ObservationMatcher` → `SignalMatches []SignalMatch` + `MatchHealHint` |
| `internal/sensor/loader.go` | YAML→struct + validation | Reject `expected: true` on non-`pass` matchers |
| `internal/runtime/executor/signals.go` | per-line matching + signal synthesis | `observationMatcher`/`observationConfig` → `signalMatcher`/`signalConfig`; verdict/confidence/captures/heal_hint |
| `internal/runtime/executor/executor.go` | build config from sensor; compile-time validation | Build `signalMatcher`s with defaults + heal_hint fallback; probe-validate; expected keys → rollup |
| `internal/runtime/executor/{step,compose}.go` | thread config through steps | Rename field `Obs` type only (mechanical) |
| `internal/aggregate/rollup.go` | completeness | Widen `computeCompleteness` gate beyond observational |
| `internal/runtime/executor/signals_test.go` | pump tests | Update to new types + new captures/verdict test |
| charge-api `.harness/sensors/core/e2e-test.yaml` | e2e primitive | Add `headers` input + send in curl |
| charge-api `.harness/sensors/core/run-dev.yaml` | dev watcher | Migrate `expected_observations` → `signal_matches` |
| `skills/create-core-sensors/SKILL.md`, `skills/create-sensors/SKILL.md` | generation prompts | Document `signal_matches`, lib-aware regexes, demand-driven core extension |

---

## Task 1: Sensor schema + type for `signal_matches`

**Files:**
- Modify: `schemas/sensor.yaml` (the `expected_observations` block added previously)
- Modify: `internal/sensor/types.go` (Sensor struct + ObservationMatcher)
- Modify: `internal/sensor/loader.go` (add load-time validation)
- Test: `internal/sensor/loader_test.go`

- [ ] **Step 1: Write the failing loader test**

Add to `internal/sensor/loader_test.go`:

```go
func TestLoadSensor_SignalMatches(t *testing.T) {
	raw := []byte(`schema_version: 1.0.0
id: obs
scope: core
angle: environment
kind: observational
nature: computational
output_type: stream
uses: []
signal_matches:
  - {key: api-ready, pattern: "ready on :", expected: true}
  - {key: http-5xx, pattern: "status_code\":5\\d\\d", verdict: fail, heal_hint: {summary: "5xx", rationale: "server error"}}
steps:
  - {id: up, run: "docker compose up"}
`)
	s, err := LoadSensorBytes(raw)
	if err != nil {
		t.Fatalf("LoadSensorBytes: %v", err)
	}
	if len(s.SignalMatches) != 2 {
		t.Fatalf("SignalMatches = %d, want 2", len(s.SignalMatches))
	}
	if s.SignalMatches[0].Key != "api-ready" || !s.SignalMatches[0].Expected {
		t.Errorf("matcher[0] = %+v", s.SignalMatches[0])
	}
	if s.SignalMatches[1].Verdict != "fail" || s.SignalMatches[1].HealHint == nil ||
		s.SignalMatches[1].HealHint.Summary != "5xx" {
		t.Errorf("matcher[1] = %+v", s.SignalMatches[1])
	}
}

func TestLoadSensor_RejectsExpectedOnNonPass(t *testing.T) {
	raw := []byte(`schema_version: 1.0.0
id: obs
scope: core
angle: environment
kind: observational
nature: computational
output_type: stream
uses: []
signal_matches:
  - {key: bad, pattern: "x", verdict: fail, expected: true, heal_hint: {summary: a, rationale: b}}
steps:
  - {id: up, run: "x"}
`)
	_, err := LoadSensorBytes(raw)
	if err == nil {
		t.Fatal("expected error for expected:true on non-pass matcher")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/sensor/ -run 'SignalMatches|RejectsExpectedOnNonPass' -v`
Expected: FAIL — `s.SignalMatches` undefined (compile error).

- [ ] **Step 3: Replace the schema block**

In `schemas/sensor.yaml`, replace the `expected_observations:` property block (added previously, right after the `outputs:` block) with:

```yaml
  signal_matches:
    type: array
    description: |
      Regex matchers (Go RE2) applied to every stdout/stderr line of any sensor.
      Each match synthesizes a Signal carrying `verdict` and the named capture
      groups as evidence. `expected: true` (pass matchers only) feeds completeness.
    items:
      type: object
      required: [key, pattern]
      additionalProperties: false
      properties:
        key:        { $ref: "#/$defs/Id" }
        pattern:    { type: string, minLength: 1 }
        verdict:    { type: string, enum: [pass, warn, fail, inconclusive] }
        confidence: { type: number, minimum: 0, maximum: 1 }
        expected:   { type: boolean }
        heal_hint:
          type: object
          required: [summary, rationale]
          additionalProperties: false
          properties:
            summary:   { type: string, minLength: 1 }
            rationale: { type: string, minLength: 1 }
```

- [ ] **Step 4: Replace the struct + add MatchHealHint**

In `internal/sensor/types.go`, replace the `ExpectedObservations` field and the `ObservationMatcher` type with:

```go
	// SignalMatches declares regex matchers applied to every stdout/stderr line.
	// Each match synthesizes a Signal (see internal/runtime/executor). Valid on
	// both assertion and observational sensors.
	SignalMatches []SignalMatch `json:"signal_matches,omitempty"`
	Steps         []Step        `json:"steps"`
}

// SignalMatch maps a regex over a sensor's output lines to a synthesized Signal.
// Verdict defaults to pass and Confidence to 1 when their pointers are nil
// (distinguishing "unset" from the zero value). Expected is only valid on pass
// matchers (enforced at load). HealHint is required-by-effect for fail/warn.
type SignalMatch struct {
	Key        string         `json:"key"`
	Pattern    string         `json:"pattern"`
	Verdict    string         `json:"verdict,omitempty"`
	Confidence *float64       `json:"confidence,omitempty"`
	Expected   bool           `json:"expected,omitempty"`
	HealHint   *MatchHealHint `json:"heal_hint,omitempty"`
}

// MatchHealHint is the heal-hint a fail/warn matcher attaches to its signal.
// Kept local to avoid a sensor→signal package dependency; the executor maps it
// to signal.HealHint.
type MatchHealHint struct {
	Summary   string `json:"summary"`
	Rationale string `json:"rationale"`
}
```

(Delete the old `ObservationMatcher` struct entirely.)

- [ ] **Step 5: Add load-time validation**

In `internal/sensor/loader.go`, find `LoadSensorBytes` and add, after the schema validation succeeds and before returning `s`:

```go
	for _, m := range s.SignalMatches {
		if m.Expected && m.Verdict != "" && m.Verdict != "pass" {
			return Sensor{}, fmt.Errorf("sensor %q: signal_match %q: expected:true is only valid on pass matchers (got verdict %q)", s.ID, m.Key, m.Verdict)
		}
	}
```

(Confirm `fmt` is imported; it is used elsewhere in the file.)

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/sensor/ -run 'SignalMatches|RejectsExpectedOnNonPass' -v`
Expected: PASS (both).

- [ ] **Step 7: Commit**

```bash
git add schemas/sensor.yaml internal/sensor/types.go internal/sensor/loader.go internal/sensor/loader_test.go
git commit -m "feat(sensor): replace expected_observations with signal_matches schema+type"
```

---

## Task 2: Executor — verdict/captures/heal_hint synthesis + validation

**Files:**
- Modify: `internal/runtime/executor/signals.go` (types + synthesize + matchLine)
- Modify: `internal/runtime/executor/executor.go` (build matchers, defaults, probe-validate, expected keys)
- Modify: `internal/runtime/executor/step.go`, `compose.go` (rename `*observationConfig` → `*signalConfig`)
- Test: `internal/runtime/executor/signals_test.go`

- [ ] **Step 1: Write the failing synthesis test**

Replace `TestPumpStdout_RegexMatchSynthesizesObservationSignal` in `internal/runtime/executor/signals_test.go` with:

```go
func TestPumpStdout_SignalMatchSynthesis(t *testing.T) {
	stdout := strings.NewReader(`{"path":"/v1/charges","status_code":500}` + "\n" + `served /health ok` + "\n")

	dir := t.TempDir()
	rl, _ := newRawLog(dir+"/raw.log", fixedNow(t))
	defer rl.Close()
	jw, _ := newJSONLWriter(dir + "/signals.jsonl")
	defer jw.Close()

	cfg := &signalConfig{
		SchemaVersion: "1.0.0", SensorID: "s", UseCaseID: "", Angle: "environment",
		Now: fixedNow(t),
		Matchers: []signalMatcher{
			{Key: "http-5xx", Re: regexp.MustCompile(`"status_code":(?P<status>5\d\d)`),
				Verdict: "fail", Confidence: 1,
				HealHint: &signal.HealHint{Summary: "5xx", Rationale: "server error"}},
			{Key: "served", Re: regexp.MustCompile(`served (?P<path>\S+)`),
				Verdict: "pass", Confidence: 1},
		},
	}
	out, err := pumpStdout(stdout, 1, rl, jw, cfg)
	if err != nil {
		t.Fatalf("pumpStdout: %v", err)
	}
	if len(out.Signals) != 2 {
		t.Fatalf("Signals = %d, want 2", len(out.Signals))
	}
	// The 5xx signal is fail with a status capture; validate it against the schema.
	var failSig signal.Signal
	for _, s := range out.Signals {
		if s.Verdict == "fail" {
			failSig = s
		}
	}
	if failSig.Verdict != "fail" {
		t.Fatal("no fail signal emitted")
	}
	if got, _ := failSig.Evidence["status"].(string); got != "500" {
		t.Errorf("evidence.status = %q, want 500", got)
	}
	if failSig.HealHint == nil {
		t.Error("fail signal missing heal_hint")
	}
	b, _ := json.Marshal(failSig)
	if _, derr := signal.DecodeLine(b); derr != nil {
		t.Errorf("synthesized fail signal invalid: %v", derr)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/runtime/executor/ -run 'SignalMatchSynthesis'`
Expected: FAIL — `signalConfig`/`signalMatcher` undefined.

- [ ] **Step 3: Rework the types + synthesize + matchLine in signals.go**

In `internal/runtime/executor/signals.go`, replace the `observationMatcher`, `observationConfig`, `synthesize`, and `matchLine` definitions with:

```go
// signalMatcher is a compiled signal_match: a regex over output lines plus the
// verdict/confidence/heal_hint of the Signal emitted on a match.
type signalMatcher struct {
	Key        string
	Re         *regexp.Regexp
	Verdict    enums.Verdict
	Confidence float64
	HealHint   *signal.HealHint
}

// signalConfig carries what the pumps need to synthesize signals from matched
// lines. Non-nil only for sensors that declare signal_matches (or are passed
// expected keys for JSON-signal observation collection).
type signalConfig struct {
	SchemaVersion string
	SensorID      string
	UseCaseID     string
	Angle         enums.ValidationAngle
	Now           func() time.Time
	Matchers      []signalMatcher
}

func (o *signalConfig) synthesize(m signalMatcher, sub []string) signal.Signal {
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	ev := signal.Evidence{"observation_key": m.Key, "matched_line": sub[0]}
	for i, name := range m.Re.SubexpNames() {
		if i == 0 || name == "" {
			continue
		}
		ev[name] = sub[i]
	}
	return signal.Signal{
		SchemaVersion: o.SchemaVersion,
		SensorID:      o.SensorID,
		UseCaseID:     o.UseCaseID,
		Angle:         o.Angle,
		EmittedAt:     now(),
		Verdict:       m.Verdict,
		Confidence:    m.Confidence,
		Evidence:      ev,
		HealHint:      m.HealHint,
	}
}

// matchLine tests line against every matcher; each hit synthesizes a signal,
// tees it to signalsJSONL, and records it in out. Shared by stdout/stderr pumps.
func (o *signalConfig) matchLine(line []byte, signalsJSONL *jsonlWriter, out *pumpOutput) {
	for _, m := range o.Matchers {
		sub := m.Re.FindStringSubmatch(string(line))
		if sub == nil {
			continue
		}
		sig := o.synthesize(m, sub)
		if b, err := json.Marshal(sig); err == nil {
			_ = signalsJSONL.WriteLine(b)
			out.Signals = append(out.Signals, sig)
			out.ObservationKeys = append(out.ObservationKeys, m.Key)
		}
	}
}
```

Then update the two pump signatures: change every `obs *observationConfig` parameter to `obs *signalConfig` in `pumpStdout` and `pumpStderr`. (The `enums` import is already present; `signal`, `json`, `regexp`, `time` too.)

- [ ] **Step 4: Update step.go and compose.go field type**

In `internal/runtime/executor/step.go`, change the `stepArgs` field:
```go
	Obs         *signalConfig // non-nil only for sensors with signal_matches / expected keys
```
In `internal/runtime/executor/compose.go`, change the `topStepArgs` field:
```go
	Obs         *signalConfig
```
(The `Obs: a.Obs` wirings stay unchanged.)

- [ ] **Step 5: Build matchers with defaults + probe-validate in executor.go**

In `internal/runtime/executor/executor.go`, replace the existing matcher-building block (the `for _, eo := range s.ExpectedObservations` loop and the `obs` construction) with:

```go
	// Compile signal_matches into matchers, applying defaults (verdict=pass,
	// confidence=1) and synthesizing a heal_hint for fail/warn matchers that
	// omit one (the Signal schema requires it). Expected (pass-only) keys feed
	// completeness.
	matchers := make([]signalMatcher, 0, len(s.SignalMatches))
	for _, sm := range s.SignalMatches {
		re, cerr := regexp.Compile(sm.Pattern)
		if cerr != nil {
			return aggregate.AggregateSignal{}, fmt.Errorf("executor: signal_match %q: bad pattern %q: %w", sm.Key, sm.Pattern, cerr)
		}
		verdict := enums.Verdict(sm.Verdict)
		if verdict == "" {
			verdict = enums.VerdictPass
		}
		confidence := 1.0
		if sm.Confidence != nil {
			confidence = *sm.Confidence
		}
		var hh *signal.HealHint
		if sm.HealHint != nil {
			hh = &signal.HealHint{Summary: sm.HealHint.Summary, Rationale: sm.HealHint.Rationale}
		}
		if (verdict == enums.VerdictFail || verdict == enums.VerdictWarn) && hh == nil {
			hh = &signal.HealHint{
				Summary:   fmt.Sprintf("matched %s pattern %q", verdict, sm.Key),
				Rationale: fmt.Sprintf("a stdout/stderr line matched signal_match %q", sm.Key),
			}
		}
		matchers = append(matchers, signalMatcher{Key: sm.Key, Re: re, Verdict: verdict, Confidence: confidence, HealHint: hh})
		if sm.Expected {
			expectedObs = mergeKeys(expectedObs, []string{sm.Key})
		}
	}
	var obs *signalConfig
	if len(expectedObs) > 0 || len(matchers) > 0 {
		obs = &signalConfig{
			SchemaVersion: observationSignalSchemaVersion,
			SensorID:      s.ID,
			UseCaseID:     s.UseCaseID,
			Angle:         s.Angle,
			Now:           e.opts.Now,
			Matchers:      matchers,
		}
		// Probe-validate each matcher: a representative signal must satisfy the
		// Signal schema, so emitted signals are valid by construction.
		for _, m := range matchers {
			probe := obs.synthesize(m, []string{"<probe>"})
			b, mErr := jsonMarshalSignal(probe)
			if mErr != nil {
				return aggregate.AggregateSignal{}, mErr
			}
			if _, vErr := signal.DecodeLine(b); vErr != nil {
				return aggregate.AggregateSignal{}, fmt.Errorf("executor: signal_match %q produces an invalid signal: %w", m.Key, vErr)
			}
		}
	}
```

Add this small helper near the bottom of `executor.go` (next to `mergeKeys`):

```go
// jsonMarshalSignal marshals a signal for probe validation.
func jsonMarshalSignal(s signal.Signal) ([]byte, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("executor: marshal probe signal: %w", err)
	}
	return b, nil
}
```

Add `"encoding/json"` to `executor.go` imports if not present.

- [ ] **Step 6: Update the existing pump tests to the new types**

In `internal/runtime/executor/signals_test.go`:
- `TestPumpStdout_HappyPath`: change `&observationConfig{}` → `&signalConfig{}`.
- `TestPumpStdout_PlainTextIsNotAParseError` and `TestPumpStdout_BadJSONLineKeepsStreaming`: unchanged (they pass `nil`).
- Ensure imports include `regexp`, `encoding/json`, and `github.com/iurykrieger/lastro/internal/signal` (added earlier).

- [ ] **Step 7: Run executor tests**

Run: `go test ./internal/runtime/executor/ -v 2>&1 | tail -20`
Expected: PASS (all, including `SignalMatchSynthesis`).

- [ ] **Step 8: Commit**

```bash
git add internal/runtime/executor/
git commit -m "feat(executor): synthesize verdict-bearing signals from signal_matches with captures + probe validation"
```

---

## Task 3: Widen completeness beyond observational

**Files:**
- Modify: `internal/aggregate/rollup.go:101-104`
- Test: `internal/aggregate/rollup_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/aggregate/rollup_test.go`:

```go
func TestCompleteness_AppliesToAssertionWhenExpected(t *testing.T) {
	agg, err := Rollup(RollupInput{
		Signals:              nil,
		SensorID:             "s",
		Angle:                enums.AngleE2ETest,
		Kind:                 enums.KindAssertion,
		OutputType:           enums.OutputSingleShot,
		TerminationReason:    enums.TerminationCompleted,
		ExpectedObservations: []string{"created"},
		ObservedKeys:         nil, // "created" never observed
	})
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if agg.Completeness == nil || len(agg.Completeness.MissingObservations) != 1 {
		t.Fatalf("expected 1 missing observation for assertion sensor, got %+v", agg.Completeness)
	}
	if agg.Verdict != enums.VerdictFail {
		t.Errorf("verdict = %q, want fail (missing expected observation)", agg.Verdict)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/aggregate/ -run 'AppliesToAssertionWhenExpected' -v`
Expected: FAIL — `Completeness` is nil for assertion kind.

- [ ] **Step 3: Widen the gate**

In `internal/aggregate/rollup.go`, change `computeCompleteness`'s opening gate from:

```go
	if in.Kind != enums.KindObservational {
		return nil
	}
```

to:

```go
	// Completeness applies to observational sensors (always) and to any sensor
	// that declares expected observation keys (e.g. assertion sensors with
	// expected signal_matches).
	if in.Kind != enums.KindObservational && len(in.ExpectedObservations) == 0 {
		return nil
	}
```

- [ ] **Step 4: Run the test + the package**

Run: `go test ./internal/aggregate/ -v 2>&1 | tail -15`
Expected: PASS (new test + existing).

- [ ] **Step 5: Commit**

```bash
git add internal/aggregate/rollup.go internal/aggregate/rollup_test.go
git commit -m "feat(aggregate): apply completeness to any sensor with expected observations"
```

---

## Task 4: `headers` input on the e2e-test core primitive (charge-api)

**Files:**
- Modify: `/Users/iury.krieger/Workspace/stone/charge/charge-api/.harness/sensors/core/e2e-test.yaml`
- Validate with the create-core-sensors script (lastro).

- [ ] **Step 1: Add the headers input + use it in the curl**

Edit `e2e-test.yaml`: add to `inputs:`:

```yaml
  headers:
    default: ""
    required: false
    description: Extra curl header arguments, e.g. -H 'Authorization: Bearer X' -H 'Provider-Id: 123'
```

And change the request step's `args=` line to include `${{ inputs.headers }}`:

```yaml
    args="-o /tmp/e2e-body -s -w %{http_code} -X ${{ inputs.method }} ${{ inputs.headers }}"
```

- [ ] **Step 2: Validate the sensor against the schema**

Run (from lastro repo):
```bash
go run ./skills/create-core-sensors/scripts/ \
  --file /Users/iury.krieger/Workspace/stone/charge/charge-api/.harness/sensors/core/e2e-test.yaml \
  --harness-dir /Users/iury.krieger/Workspace/stone/charge/charge-api/.harness
```
Expected: exit 0 (valid).

- [ ] **Step 3: Verify a bound header reaches the request (manual smoke)**

Confirm the use-case sensor can bind it, e.g. add `headers: "-H 'X-Test: 1'"` under `with:` in a scratch copy and run `/run-sensor`; the request log should show the `X-Test` header. (No automated test — this is charge-api config; covered by DoD 5a via the executor's input-env build which is already unit-tested.)

- [ ] **Step 4: Commit (charge-api repo, branch chore/lastro-harness-dev-env)**

```bash
cd /Users/iury.krieger/Workspace/stone/charge/charge-api
git add .harness/sensors/core/e2e-test.yaml
git commit -m "feat(harness): add headers input to e2e-test primitive for use-case composition"
```

---

## Task 5: Migrate `run-dev.yaml` to `signal_matches` (charge-api)

**Files:**
- Modify: `/Users/iury.krieger/Workspace/stone/charge/charge-api/.harness/sensors/core/run-dev.yaml`

- [ ] **Step 1: Replace `expected_observations` with `signal_matches`**

Replace the `expected_observations:` block with:

```yaml
signal_matches:
- key: broker-ready
  pattern: 'broker .*Kafka Server started'
  expected: true
- key: schema-registry-ready
  pattern: 'schema-registry .*===> Running'
  expected: true
- key: dev-api-ready
  pattern: 'dev-api .*Running'
  expected: true
```

- [ ] **Step 2: Validate**

Run (from lastro):
```bash
go run ./skills/create-core-sensors/scripts/ \
  --file /Users/iury.krieger/Workspace/stone/charge/charge-api/.harness/sensors/core/run-dev.yaml \
  --harness-dir /Users/iury.krieger/Workspace/stone/charge/charge-api/.harness
```
Expected: exit 0.

- [ ] **Step 3: End-to-end check**

Rebuild start-sensor, `docker compose --profile dev down`, `/start-sensor run-dev`, wait, confirm `signals.jsonl` accumulates the 3 keys, then `/stop-sensor` → `verdict: pass`, `missing_observations: []`.

- [ ] **Step 4: Commit (charge-api repo)**

```bash
git add .harness/sensors/core/run-dev.yaml
git commit -m "refactor(harness): migrate run-dev to signal_matches"
```

---

## Task 6: Generator docs — lib-aware regexes + demand-driven core extension

**Files:**
- Modify: `skills/create-core-sensors/SKILL.md`
- Modify: `skills/create-sensors/SKILL.md`

- [ ] **Step 1: Update create-core-sensors SKILL.md**

In the "Command shape" / observations section (added previously for `expected_observations`), replace references to `expected_observations` with `signal_matches`, and add:

```markdown
### signal_matches (all sensors)

Every sensor MAY declare `signal_matches: [{ key, pattern, verdict?, confidence?, expected?, heal_hint? }]`.
Each regex (Go RE2 — no backreferences/lookaround) is tested against every stdout
and stderr line; a match emits a Signal with the matcher's `verdict` (default pass)
and named capture groups `(?P<name>…)` as evidence. `expected: true` (pass matchers
only) means the key must be observed at least once or the run is incomplete.

Derive patterns from the logging library in `stack-manifest.yaml`:
- Anchor on individual fields; do NOT rely on JSON key order or bridge fields with `.*`.
- Prefer one matcher per outcome (e.g. a pass matcher for 2xx, a fail matcher for 5xx).
- For fail/warn matchers, provide a `heal_hint: {summary, rationale}`.
```

- [ ] **Step 2: Update create-sensors SKILL.md**

Add a section describing demand-driven core extension:

```markdown
### Composing core primitives + demanding inputs

A use-case sensor composes a core primitive via `uses:` + `with:`. Bind the inputs the
use case needs (e.g. `headers` for an authenticated request). If the core primitive does
NOT expose a required input, ADD that input to the core sensor's YAML with a
backward-compatible `default`/`required: false`, then bind it — the core evolves to
satisfy the use case. Derive required auth/merchant headers and signal_matches regexes
from the use case's preconditions and the stack manifest's logging library.
```

- [ ] **Step 3: Commit**

```bash
cd /Users/iury.krieger/Workspace/iurykrieger/lastro
git add skills/create-core-sensors/SKILL.md skills/create-sensors/SKILL.md
git commit -m "docs(skills): document signal_matches, lib-aware regexes, demand-driven core extension"
```

---

## Task 7: Full verification

- [ ] **Step 1: Full suite + race**

```bash
cd /Users/iury.krieger/Workspace/iurykrieger/lastro
go build ./...
go test ./... -count=1
go test -race ./internal/runtime/executor/... ./internal/lifecycle/...
```
Expected: build OK; all tests pass; race clean.

- [ ] **Step 2: Confirm no lingering `expected_observations` / `ObservationMatcher`**

Run: `git grep -n "expected_observations\|ObservationMatcher\|observationConfig\|observationMatcher" -- '*.go' '*.yaml' ':!docs/'`
Expected: no matches (all renamed). Fix any stragglers.

- [ ] **Step 3: Commit any cleanup, then push both branches**

```bash
git push   # lastro feat/detached-observational-watcher
cd /Users/iury.krieger/Workspace/stone/charge/charge-api && git push  # chore/lastro-harness-dev-env
```

---

## Self-Review Notes

- **Spec coverage:** §1 schema/type → Task 1; §2 synthesis/validation → Task 2; §3 completeness+streams → Tasks 2–3; §4 core composition → Task 4; §5 generation docs → Task 6; §6 migration → Tasks 1,2,5. DoD 1→T1, 2→T2, 3→T2, 4→T3, 5a→T4, 5b→T6, 6→T5+T7.
- **Type consistency:** `SignalMatch`/`MatchHealHint` (sensor) ↔ `signalMatcher`/`signalConfig`/`signal.HealHint` (executor) consistent across tasks; `synthesize(m, sub)` signature used identically in test and impl.
- **Known follow-up:** `expected` param keys passed via `StartSensor`/`RunWatcher` (lifecycle tests) continue to build `obs` for JSON-signal observation collection — preserved by the `len(expectedObs) > 0 || len(matchers) > 0` guard in Task 2 Step 5.
