# Flexible Core Sensors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Core parameterized primitives ship with a validated per-angle baseline input floor and a grade-and-emit contract, so use-case sensors can compose them to validate any journey variation (success / failure / alternative).

**Architecture:** Baseline input specs are new YAML files under `schemas/core-inputs/`, embedded via `schemas.FS` and loaded by `internal/sensor`. Three new validations enforce the contract: a floor check + an input-faithfulness check inside `sensor.Persist` (core scope only, ordered after step resolvability), and a `with:`-key check in `cmd/lastro`'s `persistCreateSensors`. Three new `persisterror.Kind` values surface them to skills as exit-2 JSON.

**Tech Stack:** Go 1.x, `sigs.k8s.io/yaml`, `github.com/santhosh-tekuri/jsonschema/v6` (both already in go.mod). Module path: `github.com/iurykrieger/lastro`.

**Spec:** `docs/superpowers/specs/2026-06-11-flexible-core-sensors-design.md`

**Two deliberate refinements to the spec (apply as written here):**
1. `performance` gains a `base_url` baseline input and `metrics` gains a `metrics_url` baseline input — the spec's own problem statement calls out hardcoded `localhost:8080`, and those two angles target a URL just like `e2e-test` does.
2. `timeout` (e2e-test, database) is a plain seconds string (`"10"`) rather than a Go duration, because it feeds `curl --max-time` / DB client flags directly. `within` (logs, metrics) and `duration` (performance) stay Go durations (`"45s"`) as they feed `observe_window`-style logic.

**Verification command (run from repo root):** `go test ./...` — every task must end green.

---

### Task 1: New persisterror kinds

**Files:**
- Modify: `internal/persisterror/persisterror.go:13-24`
- Test: `internal/persisterror/persisterror_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/persisterror/persisterror_test.go`:

```go
func TestNewKinds_StringValues(t *testing.T) {
	cases := map[Kind]string{
		IncompleteInputSurface: "incomplete_input_surface",
		UnknownWithKey:         "unknown_with_key",
		UnreferencedInput:      "unreferenced_input",
	}
	for kind, want := range cases {
		if string(kind) != want {
			t.Errorf("kind %v = %q, want %q", kind, string(kind), want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/persisterror/ -run TestNewKinds -v`
Expected: FAIL — `undefined: IncompleteInputSurface` (compile error).

- [ ] **Step 3: Add the constants**

In `internal/persisterror/persisterror.go`, extend the `const` block (after `UnknownBranchRef Kind = "unknown_branch_ref"`):

```go
	IncompleteInputSurface Kind = "incomplete_input_surface"
	UnknownWithKey         Kind = "unknown_with_key"
	UnreferencedInput      Kind = "unreferenced_input"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/persisterror/ -v`
Expected: PASS (all tests).

- [ ] **Step 5: Commit**

```bash
git add internal/persisterror/
git commit -m "feat: add persisterror kinds for core-sensor input contracts"
```

---

### Task 2: Baseline spec files, meta-schema, embed

**Files:**
- Create: `schemas/core-input-baseline.yaml`
- Create: `schemas/core-inputs/e2e-test.yaml`, `schemas/core-inputs/database.yaml`, `schemas/core-inputs/performance.yaml`, `schemas/core-inputs/logs.yaml`, `schemas/core-inputs/metrics.yaml`
- Modify: `schemas/schemas.go:12` (embed directive)
- Modify: `cmd/validate-schemas/main.go` (register + validate baselines)
- Test: `schemas/schemas_test.go`

Note: baseline files are keyed by **angle** id (`database.yaml`, not `database-query.yaml` — `database-query` is the primitive's id, `database` is its angle).

- [ ] **Step 1: Write the failing test**

In `schemas/schemas_test.go`, extend the `wanted` slice in `TestFSContainsKeySchemas`:

```go
	wanted := []string{
		"stack-component.yaml",
		"stack-manifest.yaml",
		"enums/stack-kinds.yaml",
		"enums/archetypes.yaml",
		"core-input-baseline.yaml",
		"core-inputs/e2e-test.yaml",
		"core-inputs/database.yaml",
		"core-inputs/performance.yaml",
		"core-inputs/logs.yaml",
		"core-inputs/metrics.yaml",
	}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./schemas/ -v`
Expected: FAIL — `FS.ReadFile("core-input-baseline.yaml"): file does not exist` (and the five core-inputs files).

- [ ] **Step 3: Create the meta-schema**

Write `schemas/core-input-baseline.yaml`:

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/core-input-baseline.yaml"
title: CoreInputBaseline
description: |
  The baseline input floor for one parameterized core-sensor angle. A
  scope:core sensor whose angle has a baseline must declare at least these
  inputs (each with a default). The floor is a minimum, not a ceiling —
  primitives declare more inputs when a use case needs them.

type: object
required: [schema_version, angle, inputs]
additionalProperties: false

properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  angle:
    type: string
    description: "Canonical source: schemas/enums/validation-angles.yaml"
    enum: [security, build, code-structure, unit-test, e2e-test,
           contracts, logs, metrics, database, performance, environment]
  inputs:
    type: object
    minProperties: 1
    additionalProperties:
      type: object
      required: [description]
      additionalProperties: false
      properties:
        description:       { type: string, minLength: 1 }
        suggested_default: { type: string }
```

- [ ] **Step 4: Create the five baseline files**

Write `schemas/core-inputs/e2e-test.yaml`:

```yaml
schema_version: 1.0.0
angle: e2e-test
inputs:
  base_url:
    description: "Origin of the system under test (scheme://host:port, no trailing slash). Derive from the detected dev-server port."
    suggested_default: "http://localhost:8080"
  method:
    description: "HTTP method"
    suggested_default: "GET"
  path:
    description: "Request path, with leading slash"
    suggested_default: "/"
  query:
    description: "Query string including the leading '?', or empty for none"
    suggested_default: ""
  headers:
    description: "Newline-separated 'Key: value' header lines, or empty for none"
    suggested_default: ""
  body:
    description: "Path to the request-body file (a fixture payload path), or empty for no body"
    suggested_default: ""
  expect_status:
    description: "Expected response status: a class ('2xx', '4xx') or an exact code ('422'). Unmet expectation exits 1 with an 'expectation-unmet' line."
    suggested_default: "2xx"
  timeout:
    description: "Request timeout in seconds (plain number string, feeds curl --max-time)"
    suggested_default: "10"
```

Write `schemas/core-inputs/database.yaml`:

```yaml
schema_version: 1.0.0
angle: database
inputs:
  query:
    description: "The query to run, in the detected datastore's native dialect"
    suggested_default: "SELECT 1"
  params:
    description: "JSON-encoded array string of positional query parameters, e.g. '[\"42\", \"BRL\"]'"
    suggested_default: "[]"
  expect_rows:
    description: "Expected result rows: exact count ('0', '1') or lower bound ('>=1'). '0' asserts an ABSENT write (failure variations)."
    suggested_default: ">=1"
  timeout:
    description: "Query timeout in seconds (plain number string)"
    suggested_default: "10"
```

Write `schemas/core-inputs/performance.yaml`:

```yaml
schema_version: 1.0.0
angle: performance
inputs:
  base_url:
    description: "Origin of the system under test (scheme://host:port, no trailing slash)"
    suggested_default: "http://localhost:8080"
  method:
    description: "HTTP method for the load-tested request"
    suggested_default: "GET"
  path:
    description: "Request path under load, with leading slash"
    suggested_default: "/"
  headers:
    description: "Newline-separated 'Key: value' header lines, or empty"
    suggested_default: ""
  body:
    description: "Path to the request-body file, or empty for no body"
    suggested_default: ""
  duration:
    description: "Load duration as a Go duration string"
    suggested_default: "10s"
  rate:
    description: "Target request rate in requests/second (plain number string)"
    suggested_default: "5"
  p95_budget_ms:
    description: "p95 latency budget in milliseconds; measured p95 above this exits 1 with an 'expectation-unmet' line"
    suggested_default: "500"
```

Write `schemas/core-inputs/logs.yaml`:

```yaml
schema_version: 1.0.0
angle: logs
inputs:
  pattern:
    description: "Go RE2 regex that MUST appear in the watched stream within the window"
    suggested_default: ".+"
  anti_pattern:
    description: "Go RE2 regex that must NOT appear; empty disables the check"
    suggested_default: ""
  within:
    description: "Watch window as a Go duration string"
    suggested_default: "45s"
  service:
    description: "Id of the shared observational core service whose stream is watched"
    suggested_default: "run-dev"
```

Write `schemas/core-inputs/metrics.yaml`:

```yaml
schema_version: 1.0.0
angle: metrics
inputs:
  metrics_url:
    description: "Full URL of the metrics scrape endpoint"
    suggested_default: "http://localhost:8080/metrics"
  name:
    description: "Metric name to assert on; empty matches any metric line (smoke mode)"
    suggested_default: ""
  labels:
    description: "Newline-separated 'key=value' label filters, or empty"
    suggested_default: ""
  predicate:
    description: "Comparison over the scraped value, e.g. '>0', '>=1'; unmet predicate exits 1 with an 'expectation-unmet' line"
    suggested_default: ">=0"
  within:
    description: "How long to keep re-scraping before giving up, as a Go duration string"
    suggested_default: "45s"
```

- [ ] **Step 5: Extend the embed directive**

In `schemas/schemas.go`, change line 12:

```go
//go:embed *.yaml enums/*.yaml core-inputs/*.yaml
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./schemas/ -v`
Expected: PASS.

- [ ] **Step 7: Register baselines in validate-schemas**

In `cmd/validate-schemas/main.go`, append `"core-input-baseline"` to the `entities` slice (it has no `schemas/examples/core-input-baseline/` dir, so the example loop skips it). Then insert this block immediately after the enum-validation block (after line 103, before the `// Validate each example` comment):

```go
	// Validate each core-input baseline against core-input-baseline.yaml.
	if blSch, ok := schemas["core-input-baseline"]; ok {
		blFiles, globErr := filepath.Glob(filepath.Join("schemas", "core-inputs", "*.yaml"))
		if globErr != nil {
			errs = append(errs, fmt.Sprintf("glob core-inputs: %v", globErr))
		}
		sort.Strings(blFiles)
		for _, p := range blFiles {
			doc, err := loadYAMLAsAny(p)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			if err := blSch.Validate(doc); err != nil {
				errs = append(errs, fmt.Sprintf("FAIL %s: %v", p, err))
			} else {
				fmt.Printf("OK %s matches core-input-baseline.yaml\n", p)
			}
		}
	}
```

- [ ] **Step 8: Run validate-schemas**

Run: `go run ./cmd/validate-schemas`
Expected: exit 0; output includes `OK schemas/core-inputs/database.yaml matches core-input-baseline.yaml` (and the other four), ending with `All schemas, enums, and examples validated.`

- [ ] **Step 9: Commit**

```bash
git add schemas/ cmd/validate-schemas/
git commit -m "feat: define per-angle baseline input floors for core primitives"
```

---

### Task 3: Baseline loader (`LoadBaselines`)

**Files:**
- Create: `internal/sensor/baseline.go`
- Test: `internal/sensor/baseline_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/sensor/baseline_test.go`:

```go
package sensor

import (
	"encoding/json"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"
)

func TestLoadBaselines_AllFiveAngles(t *testing.T) {
	baselines, err := LoadBaselines()
	if err != nil {
		t.Fatalf("LoadBaselines: %v", err)
	}
	wantAngles := []enums.ValidationAngle{
		enums.AngleE2ETest, enums.AngleDatabase, enums.AnglePerformance,
		enums.AngleLogs, enums.AngleMetrics,
	}
	if len(baselines) != len(wantAngles) {
		t.Fatalf("got %d baselines, want %d: %v", len(baselines), len(wantAngles), baselines)
	}
	for _, a := range wantAngles {
		if _, ok := baselines[a]; !ok {
			t.Errorf("missing baseline for angle %q", a)
		}
	}
}

func TestLoadBaselines_E2EFloorContents(t *testing.T) {
	baselines, err := LoadBaselines()
	if err != nil {
		t.Fatalf("LoadBaselines: %v", err)
	}
	e2e := baselines[enums.AngleE2ETest]
	for _, name := range []string{
		"base_url", "method", "path", "query", "headers", "body",
		"expect_status", "timeout",
	} {
		spec, ok := e2e.Inputs[name]
		if !ok {
			t.Errorf("e2e-test baseline missing input %q", name)
			continue
		}
		if spec.Description == "" {
			t.Errorf("e2e-test baseline input %q has empty description", name)
		}
	}
}

func TestBaselineFiles_MatchMetaSchema(t *testing.T) {
	raw, err := schemas.FS.ReadFile("core-input-baseline.yaml")
	if err != nil {
		t.Fatalf("read meta-schema: %v", err)
	}
	asJSON, err := yaml.YAMLToJSON(raw)
	if err != nil {
		t.Fatalf("yaml->json meta-schema: %v", err)
	}
	var doc any
	if err := json.Unmarshal(asJSON, &doc); err != nil {
		t.Fatalf("unmarshal meta-schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	const url = "https://lastro.dev/harness/schemas/core-input-baseline.yaml"
	if err := c.AddResource(url, doc); err != nil {
		t.Fatalf("add resource: %v", err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		t.Fatalf("compile meta-schema: %v", err)
	}
	entries, err := schemas.FS.ReadDir("core-inputs")
	if err != nil {
		t.Fatalf("read core-inputs dir: %v", err)
	}
	for _, e := range entries {
		b, err := schemas.FS.ReadFile("core-inputs/" + e.Name())
		if err != nil {
			t.Errorf("read %s: %v", e.Name(), err)
			continue
		}
		j, err := yaml.YAMLToJSON(b)
		if err != nil {
			t.Errorf("yaml->json %s: %v", e.Name(), err)
			continue
		}
		var inst any
		if err := json.Unmarshal(j, &inst); err != nil {
			t.Errorf("unmarshal %s: %v", e.Name(), err)
			continue
		}
		if err := sch.Validate(inst); err != nil {
			t.Errorf("FAIL %s against meta-schema: %v", e.Name(), err)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sensor/ -run 'TestLoadBaselines|TestBaselineFiles' -v`
Expected: FAIL — `undefined: LoadBaselines` (compile error).

- [ ] **Step 3: Implement the loader**

Create `internal/sensor/baseline.go`:

```go
package sensor

import (
	"fmt"
	"strings"
	"sync"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/schemas"
)

// CoreInputBaseline is the baseline input floor for one parameterized
// core-sensor angle, loaded from the embedded schemas/core-inputs/<angle>.yaml.
// The floor is a minimum, not a ceiling: a core primitive must declare at
// least these inputs (each with a default) and may declare more.
type CoreInputBaseline struct {
	SchemaVersion string                       `json:"schema_version"`
	Angle         enums.ValidationAngle        `json:"angle"`
	Inputs        map[string]BaselineInputSpec `json:"inputs"`
}

// BaselineInputSpec describes one baseline input. SuggestedDefault is a
// hint for generation; the generating skill may override it with a
// manifest-derived value (e.g. base_url from the detected dev-server port).
type BaselineInputSpec struct {
	Description      string `json:"description"`
	SuggestedDefault string `json:"suggested_default,omitempty"`
}

var (
	baselineOnce sync.Once
	baselineMap  map[enums.ValidationAngle]CoreInputBaseline
	baselineErr  error
)

// LoadBaselines parses every embedded schemas/core-inputs/*.yaml into a
// map keyed by angle. Parsed once; subsequent calls reuse the cached map
// and the cached error.
func LoadBaselines() (map[enums.ValidationAngle]CoreInputBaseline, error) {
	baselineOnce.Do(func() {
		entries, err := schemas.FS.ReadDir("core-inputs")
		if err != nil {
			baselineErr = fmt.Errorf("read embedded core-inputs: %w", err)
			return
		}
		m := make(map[enums.ValidationAngle]CoreInputBaseline, len(entries))
		for _, e := range entries {
			raw, err := schemas.FS.ReadFile("core-inputs/" + e.Name())
			if err != nil {
				baselineErr = fmt.Errorf("read core-inputs/%s: %w", e.Name(), err)
				return
			}
			var bl CoreInputBaseline
			if err := yaml.Unmarshal(raw, &bl); err != nil {
				baselineErr = fmt.Errorf("parse core-inputs/%s: %w", e.Name(), err)
				return
			}
			if len(bl.Inputs) == 0 {
				baselineErr = fmt.Errorf("core-inputs/%s: declares no inputs", e.Name())
				return
			}
			if want := string(bl.Angle) + ".yaml"; want != e.Name() {
				baselineErr = fmt.Errorf("core-inputs/%s: angle %q does not match filename (want %s)",
					e.Name(), bl.Angle, want)
				return
			}
			for name, spec := range bl.Inputs {
				if strings.TrimSpace(spec.Description) == "" {
					baselineErr = fmt.Errorf("core-inputs/%s: input %q has empty description", e.Name(), name)
					return
				}
			}
			m[bl.Angle] = bl
		}
		baselineMap = m
	})
	return baselineMap, baselineErr
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sensor/ -run 'TestLoadBaselines|TestBaselineFiles' -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/sensor/baseline.go internal/sensor/baseline_test.go
git commit -m "feat: load embedded core-input baselines keyed by angle"
```

---

### Task 4: `ValidateBaselineInputs` (floor check)

**Files:**
- Modify: `internal/sensor/baseline.go`
- Test: `internal/sensor/baseline_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/sensor/baseline_test.go`:

```go
// fullE2EInputs returns an Inputs map satisfying the e2e-test floor.
func fullE2EInputs() map[string]InputSpec {
	names := []string{
		"base_url", "method", "path", "query", "headers", "body",
		"expect_status", "timeout",
	}
	m := make(map[string]InputSpec, len(names))
	for _, n := range names {
		m[n] = InputSpec{HasDefault: true}
	}
	return m
}

func mustBaselines(t *testing.T) map[enums.ValidationAngle]CoreInputBaseline {
	t.Helper()
	baselines, err := LoadBaselines()
	if err != nil {
		t.Fatalf("LoadBaselines: %v", err)
	}
	return baselines
}

func TestValidateBaselineInputs_FullFloorPasses(t *testing.T) {
	s := Sensor{ID: "e2e-test", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest, Inputs: fullE2EInputs()}
	if err := ValidateBaselineInputs(s, mustBaselines(t)); err != nil {
		t.Fatalf("full floor should pass: %v", err)
	}
}

func TestValidateBaselineInputs_ExtraInputsStillPass(t *testing.T) {
	in := fullE2EInputs()
	in["idempotency_key"] = InputSpec{HasDefault: true}
	s := Sensor{ID: "e2e-test", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest, Inputs: in}
	if err := ValidateBaselineInputs(s, mustBaselines(t)); err != nil {
		t.Fatalf("floor is a minimum, extras must pass: %v", err)
	}
}

func TestValidateBaselineInputs_MissingInputFails(t *testing.T) {
	in := fullE2EInputs()
	delete(in, "headers")
	s := Sensor{ID: "e2e-test", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest, Inputs: in}
	err := ValidateBaselineInputs(s, mustBaselines(t))
	if err == nil {
		t.Fatal("missing baseline input must fail")
	}
	if !strings.Contains(err.Error(), "headers") {
		t.Fatalf("error should name the missing input, got: %v", err)
	}
}

func TestValidateBaselineInputs_MissingDefaultFails(t *testing.T) {
	in := fullE2EInputs()
	in["body"] = InputSpec{HasDefault: false}
	s := Sensor{ID: "e2e-test", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest, Inputs: in}
	err := ValidateBaselineInputs(s, mustBaselines(t))
	if err == nil {
		t.Fatal("baseline input without default must fail")
	}
	if !strings.Contains(err.Error(), "body") {
		t.Fatalf("error should name the undefaulted input, got: %v", err)
	}
}

func TestValidateBaselineInputs_SkipsNonCoreAndUnknownAngles(t *testing.T) {
	useCase := Sensor{ID: "s-x", Scope: enums.ScopeUseCase, UseCaseID: "uc", Angle: enums.AngleE2ETest}
	if err := ValidateBaselineInputs(useCase, mustBaselines(t)); err != nil {
		t.Fatalf("use-case scope must be skipped: %v", err)
	}
	env := Sensor{ID: "run-dev", Scope: enums.ScopeCore, Angle: enums.AngleEnvironment}
	if err := ValidateBaselineInputs(env, mustBaselines(t)); err != nil {
		t.Fatalf("angle without a baseline must be skipped: %v", err)
	}
}
```

Also add `"strings"` to the test file's imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sensor/ -run TestValidateBaselineInputs -v`
Expected: FAIL — `undefined: ValidateBaselineInputs` (compile error).

- [ ] **Step 3: Implement the floor check**

Append to `internal/sensor/baseline.go` (add `"sort"` and `"errors"` to imports):

```go
// ValidateBaselineInputs enforces the per-angle input floor on core
// primitives: a scope=core sensor whose angle has a baseline must declare
// every baseline input, each carrying a default (the self-run smoke-test
// invariant). Non-core sensors and angles without a baseline pass
// trivially. The floor is a minimum — extra declared inputs are fine.
func ValidateBaselineInputs(s Sensor, baselines map[enums.ValidationAngle]CoreInputBaseline) error {
	if s.Scope != enums.ScopeCore {
		return nil
	}
	bl, ok := baselines[s.Angle]
	if !ok {
		return nil
	}
	var missing, undefaulted []string
	for name := range bl.Inputs {
		spec, declared := s.Inputs[name]
		if !declared {
			missing = append(missing, name)
			continue
		}
		if !spec.HasDefault {
			undefaulted = append(undefaulted, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(undefaulted)
	var errs []error
	if len(missing) > 0 {
		errs = append(errs, fmt.Errorf(
			"angle %q baseline input(s) not declared: %v — see schemas/core-inputs/%s.yaml; the floor is a minimum, declare them all with defaults",
			s.Angle, missing, s.Angle))
	}
	if len(undefaulted) > 0 {
		errs = append(errs, fmt.Errorf(
			"baseline input(s) missing a default: %v — every core input needs a default so the primitive self-runs",
			undefaulted))
	}
	return errors.Join(errs...)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sensor/ -run TestValidateBaselineInputs -v`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/sensor/
git commit -m "feat: enforce per-angle baseline input floor on core primitives"
```

---

### Task 5: `ValidateInputReferences` (faithfulness check)

**Files:**
- Modify: `internal/sensor/baseline.go`
- Test: `internal/sensor/baseline_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/sensor/baseline_test.go`:

```go
func TestValidateInputReferences_AllReferencedPasses(t *testing.T) {
	s := Sensor{
		ID: "e2e-test", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest,
		Inputs: map[string]InputSpec{
			"method": {HasDefault: true},
			"path":   {HasDefault: true},
		},
		Steps: []Step{{
			ID:  "request",
			Run: `curl -X "${{ inputs.method }}" "http://x${{inputs.path}}"`,
		}},
	}
	if err := ValidateInputReferences(s); err != nil {
		t.Fatalf("all inputs referenced (with and without inner spaces): %v", err)
	}
}

func TestValidateInputReferences_UnreferencedFails(t *testing.T) {
	s := Sensor{
		ID: "e2e-test", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest,
		Inputs: map[string]InputSpec{
			"method":        {HasDefault: true},
			"expect_status": {HasDefault: true},
		},
		Steps: []Step{{ID: "request", Run: `curl -X "${{ inputs.method }}" http://x/`}},
	}
	err := ValidateInputReferences(s)
	if err == nil {
		t.Fatal("declared-but-ignored input must fail")
	}
	if !strings.Contains(err.Error(), "expect_status") {
		t.Fatalf("error should name the unreferenced input, got: %v", err)
	}
}

func TestValidateInputReferences_WithValueCounts(t *testing.T) {
	s := Sensor{
		ID: "wrapper", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest,
		Inputs: map[string]InputSpec{"path": {HasDefault: true}},
		Steps: []Step{{
			ID:   "inner",
			Uses: "e2e-test",
			With: map[string]string{"path": "${{ inputs.path }}"},
		}},
	}
	if err := ValidateInputReferences(s); err != nil {
		t.Fatalf("a reference inside a with value counts: %v", err)
	}
}

func TestValidateInputReferences_SkipsNonCore(t *testing.T) {
	s := Sensor{
		ID: "s-x", Scope: enums.ScopeUseCase, UseCaseID: "uc", Angle: enums.AngleE2ETest,
		Inputs: map[string]InputSpec{"ghost": {}},
		Steps:  []Step{{ID: "a", Run: "echo hi"}},
	}
	if err := ValidateInputReferences(s); err != nil {
		t.Fatalf("non-core sensors are skipped: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sensor/ -run TestValidateInputReferences -v`
Expected: FAIL — `undefined: ValidateInputReferences` (compile error).

- [ ] **Step 3: Implement the faithfulness check**

Append to `internal/sensor/baseline.go` (add `"regexp"` to imports):

```go
// ValidateInputReferences enforces faithfulness on core primitives: every
// declared input must be referenced as ${{ inputs.<name> }} in at least one
// step's run script or with value. A declared-but-ignored input is a
// contract lie — consumers would bind it and silently change nothing.
func ValidateInputReferences(s Sensor) error {
	if s.Scope != enums.ScopeCore || len(s.Inputs) == 0 {
		return nil
	}
	var blob strings.Builder
	for _, st := range s.Steps {
		blob.WriteString(st.Run)
		blob.WriteByte('\n')
		for _, v := range st.With {
			blob.WriteString(v)
			blob.WriteByte('\n')
		}
	}
	text := blob.String()
	var unreferenced []string
	for name := range s.Inputs {
		re := regexp.MustCompile(`\$\{\{\s*inputs\.` + regexp.QuoteMeta(name) + `\s*\}\}`)
		if !re.MatchString(text) {
			unreferenced = append(unreferenced, name)
		}
	}
	sort.Strings(unreferenced)
	if len(unreferenced) > 0 {
		return fmt.Errorf(
			"declared input(s) never referenced as ${{ inputs.<name> }} in any step: %v — bind them in a run script or remove them",
			unreferenced)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sensor/ -run TestValidateInputReferences -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/sensor/
git commit -m "feat: reject declared-but-ignored inputs on core primitives"
```

---

### Task 6: Wire floor + faithfulness into `sensor.Persist`

The checks run **after** step resolvability (a2), so command errors keep surfacing first — `TestPersist_StepResolvability_AppliesToCoreSensors` (which uses a core `e2e-test`-angle sensor with no inputs) must keep passing unchanged. This task also updates the skill script's happy-path fixture, which otherwise goes red in the same commit.

**Files:**
- Modify: `internal/sensor/persist.go` (insert after the (a2) block, around line 72)
- Modify: `skills/create-core-sensors/scripts/main_test.go:84-103` (`happyCoreSensorYAML`)
- Test: `internal/sensor/persist_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/sensor/persist_test.go`:

```go
// fullCoreE2EYAML satisfies the e2e-test baseline floor and references
// every declared input. Commands (echo, curl, cat, tr, printf, mktemp)
// resolve on any POSIX dev machine.
const fullCoreE2EYAML = `schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: []
inputs:
  base_url:      { required: true,  default: "http://localhost:8080" }
  method:        { required: true,  default: GET }
  path:          { required: true,  default: / }
  query:         { required: false, default: "" }
  headers:       { required: false, default: "" }
  body:          { required: false, default: "" }
  expect_status: { required: true,  default: 2xx }
  timeout:       { required: false, default: "10" }
steps:
  - id: request
    run: |
      echo "${{ inputs.base_url }} ${{ inputs.method }} ${{ inputs.path }}"
      echo "${{ inputs.query }} ${{ inputs.headers }} ${{ inputs.body }}"
      echo "${{ inputs.expect_status }} ${{ inputs.timeout }}"
`

func TestPersist_CoreSensor_FullBaselinePasses(t *testing.T) {
	dir := seedHappyPath(t, t.TempDir())
	if err := Persist([]byte(fullCoreE2EYAML), dir); err != nil {
		t.Fatalf("full-floor core sensor must persist: %v", err)
	}
}

func TestPersist_CoreSensor_IncompleteInputSurface(t *testing.T) {
	dir := seedHappyPath(t, t.TempDir())
	content := []byte(`schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: []
inputs:
  method: { required: true, default: GET }
steps:
  - id: request
    run: 'echo "${{ inputs.method }}"'
`)
	err := Persist(content, dir)
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("want *persisterror.Error, got %v", err)
	}
	if pe.Kind != persisterror.IncompleteInputSurface {
		t.Fatalf("Kind=%q, want %q (err: %v)", pe.Kind, persisterror.IncompleteInputSurface, err)
	}
}

func TestPersist_CoreSensor_UnreferencedInput(t *testing.T) {
	dir := seedHappyPath(t, t.TempDir())
	// Full floor declared, but expect_status is never referenced in run.
	content := []byte(`schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: []
inputs:
  base_url:      { required: true,  default: "http://localhost:8080" }
  method:        { required: true,  default: GET }
  path:          { required: true,  default: / }
  query:         { required: false, default: "" }
  headers:       { required: false, default: "" }
  body:          { required: false, default: "" }
  expect_status: { required: true,  default: 2xx }
  timeout:       { required: false, default: "10" }
steps:
  - id: request
    run: |
      echo "${{ inputs.base_url }} ${{ inputs.method }} ${{ inputs.path }}"
      echo "${{ inputs.query }} ${{ inputs.headers }} ${{ inputs.body }}"
      echo "${{ inputs.timeout }}"
`)
	err := Persist(content, dir)
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("want *persisterror.Error, got %v", err)
	}
	if pe.Kind != persisterror.UnreferencedInput {
		t.Fatalf("Kind=%q, want %q (err: %v)", pe.Kind, persisterror.UnreferencedInput, err)
	}
}
```

(`persist_test.go` already imports `errors` and `persisterror`; add them if its existing imports lack them.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sensor/ -run 'TestPersist_CoreSensor_' -v`
Expected: `TestPersist_CoreSensor_FullBaselinePasses` PASSES (nothing enforces yet), the other two FAIL (Persist returns nil, not the expected kinds).

- [ ] **Step 3: Wire the checks into Persist**

In `internal/sensor/persist.go`, insert after the (a2) resolvability block (after line 72's closing `}`) and before the `// (b)-(d) are use-case-scoped` comment:

```go
	// (a3) Core primitives must satisfy the per-angle baseline input floor
	// and reference every input they declare. Ordered after (a2) so
	// unresolvable-command errors surface first.
	if s.Scope == enums.ScopeCore {
		baselines, blErr := LoadBaselines()
		if blErr != nil {
			return &persisterror.Error{
				Kind:       persisterror.SchemaViolation,
				EntityType: "sensor",
				EntityID:   s.ID,
				Message:    fmt.Sprintf("load core-input baselines: %v", blErr),
			}
		}
		if err := ValidateBaselineInputs(s, baselines); err != nil {
			return &persisterror.Error{
				Kind:       persisterror.IncompleteInputSurface,
				EntityType: "sensor",
				EntityID:   s.ID,
				Expected:   fmt.Sprintf("at least the inputs in schemas/core-inputs/%s.yaml, each with a default", s.Angle),
				Message:    err.Error(),
			}
		}
		if err := ValidateInputReferences(s); err != nil {
			return &persisterror.Error{
				Kind:       persisterror.UnreferencedInput,
				EntityType: "sensor",
				EntityID:   s.ID,
				Message:    err.Error(),
			}
		}
	}
```

Also update the function's doc comment list: after the `(a2)` line add:

```go
//	(a3) scope: core only — the sensor declares at least its angle's
//	    baseline input floor (IncompleteInputSurface) and references every
//	    declared input in its steps (UnreferencedInput).
```

- [ ] **Step 4: Update the skill script's happy-path fixture**

In `skills/create-core-sensors/scripts/main_test.go`, replace the whole `happyCoreSensorYAML` constant (lines 82–103) with:

```go
// happyCoreSensorYAML is a valid core sensor that passes all checks when
// the seeded harness is present: full e2e-test baseline floor, every
// input referenced, grade-and-emit shape (no curl --fail).
const happyCoreSensorYAML = `schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [curl]
depends_on: [run-dev]
inputs:
  base_url:      { required: true,  default: "http://localhost:8080" }
  method:        { required: true,  default: GET }
  path:          { required: true,  default: /health_check/ready }
  query:         { required: false, default: "" }
  headers:       { required: false, default: "" }
  body:          { required: false, default: "" }
  expect_status: { required: true,  default: 2xx }
  timeout:       { required: false, default: "10" }
outputs:
  status: { from: "${{ steps.request.outputs.status }}" }
  body:   { from: "${{ steps.request.outputs.body }}" }
signal_matches:
  - key: expectation-unmet
    pattern: "expectation-unmet expected=(?P<expected>\\S+) got=(?P<got>\\d+)"
    verdict: fail
    heal_hint: { summary: "Response status did not match the declared expectation", rationale: "The endpoint answered outside expect_status; inspect the handler or the expectation binding." }
steps:
  - id: request
    run: |
      hdr_file=$(mktemp)
      printf '%s\n' "${{ inputs.headers }}" > "$hdr_file"
      body_arg=""
      if [ -n "${{ inputs.body }}" ]; then body_arg="--data-binary @${{ inputs.body }}"; fi
      status=$(curl -sS -o /tmp/harness-e2e-body -w '%{http_code}' \
        --max-time "${{ inputs.timeout }}" -X "${{ inputs.method }}" \
        -H "@$hdr_file" $body_arg \
        "${{ inputs.base_url }}${{ inputs.path }}${{ inputs.query }}")
      body=$(cat /tmp/harness-e2e-body 2>/dev/null || true)
      printf 'status=%s\nbody=%s\n' "$status" "$body"
      printf 'status=%s\nbody=%s\n' "$status" "$body" >> "$HARNESS_OUTPUT"
      expect=$(printf '%s' "${{ inputs.expect_status }}" | tr x '?')
      case "$status" in
        $expect) ;;
        *) printf 'expectation-unmet expected=%s got=%s\n' "${{ inputs.expect_status }}" "$status"; exit 1 ;;
      esac
`
```

Then append a floor-violation test to the same file:

```go
func TestCreateCoreSensors_IncompleteInputSurface_ExitsTwoWithJSON(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	if err := os.MkdirAll(harness, 0o755); err != nil {
		t.Fatal(err)
	}
	seedHarness(t, harness)

	// e2e-test-angle core sensor declaring only a slice of the baseline floor.
	narrowYAML := `schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [curl]
inputs:
  method: { required: true, default: GET }
  path:   { required: true, default: / }
steps:
  - id: request
    run: 'curl -sS -X "${{ inputs.method }}" "http://localhost:8080${{ inputs.path }}"'
`
	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(narrowYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	sout, _, code := runScript(t, "--file", input, "--harness-dir", harness)
	if code != 2 {
		t.Fatalf("exit=%d, want 2; stdout=%q", code, sout)
	}
	var pe persisterror.Error
	if err := json.Unmarshal([]byte(sout), &pe); err != nil {
		t.Fatalf("stdout is not a persisterror.Error JSON: %v\nstdout=%q", err, sout)
	}
	if pe.Kind != persisterror.IncompleteInputSurface {
		t.Fatalf("Kind=%q, want %q", pe.Kind, persisterror.IncompleteInputSurface)
	}
}
```

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: PASS everywhere. Pay attention to:
- `TestPersist_CoreSensorWritesToCoreFolder` (environment angle — no baseline, skipped) still green.
- `TestPersist_StepResolvability_AppliesToCoreSensors` still green (resolvability fires before the floor).
- `skills/create-core-sensors/scripts` tests green with the new happy YAML.

- [ ] **Step 6: Commit**

```bash
git add internal/sensor/ skills/create-core-sensors/scripts/
git commit -m "feat: enforce baseline floor and input faithfulness at core-sensor persist"
```

---

### Task 7: `ValidateWithKeys` (compose-side check)

**Files:**
- Modify: `internal/sensor/compose.go`
- Test: `internal/sensor/compose_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/sensor/compose_test.go` (mirror the file's existing store-construction helpers; the shapes below work with `NewStore`):

```go
func TestValidateWithKeys_UndeclaredKeyFails(t *testing.T) {
	prim := Sensor{
		SchemaVersion: "1.0.0", ID: "e2e-test", Scope: enums.ScopeCore,
		Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Inputs: map[string]InputSpec{"method": {HasDefault: true}},
		Steps:  []Step{{ID: "request", Run: `echo "${{ inputs.method }}"`}},
	}
	consumer := Sensor{
		SchemaVersion: "1.0.0", ID: "s-uc-e2e", Scope: enums.ScopeUseCase,
		UseCaseID: "uc", Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Steps: []Step{{
			ID: "go", Uses: "e2e-test",
			With: map[string]string{"method": "POST", "idempotency_key": "abc"},
		}},
	}
	store, err := NewStore(prim, consumer)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	werr := ValidateWithKeys(consumer, store)
	if werr == nil {
		t.Fatal("undeclared with key must fail")
	}
	if !strings.Contains(werr.Error(), "idempotency_key") || !strings.Contains(werr.Error(), "e2e-test") {
		t.Fatalf("error should name the key and the primitive, got: %v", werr)
	}
}

func TestValidateWithKeys_DeclaredKeysPass(t *testing.T) {
	prim := Sensor{
		SchemaVersion: "1.0.0", ID: "e2e-test", Scope: enums.ScopeCore,
		Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Inputs: map[string]InputSpec{"method": {HasDefault: true}, "path": {HasDefault: true}},
		Steps:  []Step{{ID: "request", Run: `echo "${{ inputs.method }} ${{ inputs.path }}"`}},
	}
	consumer := Sensor{
		SchemaVersion: "1.0.0", ID: "s-uc-e2e", Scope: enums.ScopeUseCase,
		UseCaseID: "uc", Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Steps: []Step{{
			ID: "go", Uses: "e2e-test",
			With: map[string]string{"method": "POST", "path": "/v1/x"},
		}},
	}
	store, err := NewStore(prim, consumer)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if werr := ValidateWithKeys(consumer, store); werr != nil {
		t.Fatalf("declared keys must pass: %v", werr)
	}
}

func TestValidateWithKeys_UnknownTargetSkipped(t *testing.T) {
	// An unknown uses-target is ValidateComposition's finding, not this check's.
	consumer := Sensor{
		SchemaVersion: "1.0.0", ID: "s-uc-e2e", Scope: enums.ScopeUseCase,
		UseCaseID: "uc", Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Steps: []Step{{ID: "go", Uses: "ghost", With: map[string]string{"x": "y"}}},
	}
	store, err := NewStore(consumer)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if werr := ValidateWithKeys(consumer, store); werr != nil {
		t.Fatalf("unknown target must be skipped here: %v", werr)
	}
}
```

Add `"strings"` to the test file's imports if absent.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sensor/ -run TestValidateWithKeys -v`
Expected: FAIL — `undefined: ValidateWithKeys` (compile error).

- [ ] **Step 3: Implement the check**

Append to `internal/sensor/compose.go` (add `"sort"` to imports):

```go
// ValidateWithKeys checks every uses-step in s: each `with:` key must be a
// declared input of the composed core primitive. An undeclared key would
// silently bind nothing — the caller must evolve the core primitive first
// (add the input with a backward-compatible default, re-persist it), then
// retry. Unknown uses-targets are skipped here; ValidateComposition owns
// that finding.
func ValidateWithKeys(s Sensor, store *Store) error {
	var errs []error
	for _, st := range s.Steps {
		if st.Uses == "" {
			continue
		}
		prim, ok := store.LookupSensor(st.Uses)
		if !ok {
			continue
		}
		var unknown []string
		for key := range st.With {
			if _, declared := prim.Inputs[key]; !declared {
				unknown = append(unknown, key)
			}
		}
		if len(unknown) == 0 {
			continue
		}
		sort.Strings(unknown)
		declared := make([]string, 0, len(prim.Inputs))
		for name := range prim.Inputs {
			declared = append(declared, name)
		}
		sort.Strings(declared)
		errs = append(errs, fmt.Errorf(
			"step %q: with key(s) %v are not declared inputs of %q (declared: %v) — evolve the core primitive first: add the input with a default, re-persist it, then retry",
			st.ID, unknown, st.Uses, declared))
	}
	return errors.Join(errs...)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sensor/ -run TestValidateWithKeys -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/sensor/
git commit -m "feat: reject with-keys that are not declared inputs of the composed primitive"
```

---

### Task 8: Wire `unknown_with_key` into `persistCreateSensors` + end-to-end evolution-flow test

**Files:**
- Modify: `cmd/lastro/persist.go:124-131` (inside `persistCreateSensors`)
- Create: `cmd/lastro/persist_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/lastro/persist_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/persisterror"
)

const evoStackManifest = `schema_version: 1.0.0
archetype: http-api
applicable_angles: [e2e-test]
components:
  - schema_version: 1.0.0
    id: curl
    kind: tool
    name: curl
    version: "8.0"
    capabilities: [http-client]
    detection_evidence: [{file: Dockerfile, path: .}]
`

// evoCorePrimitive satisfies the e2e-test baseline floor (echo-based run
// keeps step resolvability trivial). extraRun lets the evolution step
// append a reference to a newly added input.
func evoCorePrimitive(extraInputs, extraRun string) string {
	return `schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [curl]
inputs:
  base_url:      { required: true,  default: "http://localhost:8080" }
  method:        { required: true,  default: GET }
  path:          { required: true,  default: / }
  query:         { required: false, default: "" }
  headers:       { required: false, default: "" }
  body:          { required: false, default: "" }
  expect_status: { required: true,  default: 2xx }
  timeout:       { required: false, default: "10" }
` + extraInputs + `steps:
  - id: request
    run: |
      echo "${{ inputs.base_url }} ${{ inputs.method }} ${{ inputs.path }}"
      echo "${{ inputs.query }} ${{ inputs.headers }} ${{ inputs.body }}"
      echo "${{ inputs.expect_status }} ${{ inputs.timeout }}"
` + extraRun
}

const evoUseCaseSensor = `schema_version: 1.0.0
id: s-uc-checkout-e2e-test
scope: use-case
use_case_id: uc-checkout
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: reject
    uses: e2e-test
    with:
      method: POST
      path: /v1/charges
      expect_status: "422"
      idempotency_key: abc123
`

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPersistCreateSensors_UnknownWithKey_ThenEvolutionFlow(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	writeFile(t, filepath.Join(harness, "stack-manifest.yaml"), evoStackManifest)
	writeFile(t, filepath.Join(harness, "use-cases", "uc-checkout.yaml"), "id: uc-checkout\n")

	// Seed the core primitive (without idempotency_key) via create-core-sensors.
	coreIn := filepath.Join(dir, "core.yaml")
	writeFile(t, coreIn, evoCorePrimitive("", ""))
	var out, errOut bytes.Buffer
	code := persistCreateCoreSensors(
		[]string{"create-core-sensors", "--file", coreIn, "--harness-dir", harness}, &out, &errOut)
	if code != 0 {
		t.Fatalf("seed core: exit=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}

	// 1. Binding an undeclared input fails with unknown_with_key.
	ucIn := filepath.Join(dir, "uc-sensor.yaml")
	writeFile(t, ucIn, evoUseCaseSensor)
	out.Reset()
	errOut.Reset()
	code = persistCreateSensors(
		[]string{"create-sensors", "--file", ucIn, "--harness-dir", harness}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit=%d, want 2; stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	var pe persisterror.Error
	if err := json.Unmarshal(out.Bytes(), &pe); err != nil {
		t.Fatalf("stdout is not persisterror JSON: %v\nstdout=%q", err, out.String())
	}
	if pe.Kind != persisterror.UnknownWithKey {
		t.Fatalf("Kind=%q, want %q", pe.Kind, persisterror.UnknownWithKey)
	}

	// 2. Evolve the core: add the input (with default) AND reference it.
	writeFile(t, coreIn, evoCorePrimitive(
		"  idempotency_key: { required: false, default: \"\" }\n",
		"      echo \"${{ inputs.idempotency_key }}\"\n"))
	out.Reset()
	errOut.Reset()
	code = persistCreateCoreSensors(
		[]string{"create-core-sensors", "--file", coreIn, "--harness-dir", harness}, &out, &errOut)
	if code != 0 {
		t.Fatalf("evolve core: exit=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}

	// 3. Retry the use-case sensor — now persists.
	out.Reset()
	errOut.Reset()
	code = persistCreateSensors(
		[]string{"create-sensors", "--file", ucIn, "--harness-dir", harness}, &out, &errOut)
	if code != 0 {
		t.Fatalf("retry: exit=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if _, err := os.Stat(filepath.Join(harness, "sensors", "uc-checkout", "s-uc-checkout-e2e-test.yaml")); err != nil {
		t.Fatalf("use-case sensor not written: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/lastro/ -run TestPersistCreateSensors_UnknownWithKey -v`
Expected: FAIL at step 1's assertion — exit is 0 (or the composition error path), not 2 with `unknown_with_key`, because nothing validates `with:` keys yet.

- [ ] **Step 3: Wire the check**

In `cmd/lastro/persist.go`, inside `persistCreateSensors`, immediately after the `sensor.ValidateComposition` block (after line 131's closing `}`):

```go
		if wkErr := sensor.ValidateWithKeys(s, store); wkErr != nil {
			pe := &persisterror.Error{
				Kind: persisterror.UnknownWithKey, EntityType: "sensor", EntityID: s.ID,
				Message: wkErr.Error(),
			}
			_ = json.NewEncoder(stdout).Encode(pe)
			return 2
		}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/lastro/ -v`
Expected: PASS, including the three-phase evolution-flow test.

- [ ] **Step 5: Commit**

```bash
git add cmd/lastro/
git commit -m "feat: surface unknown_with_key and validate the core-evolution flow end to end"
```

---

### Task 9: Rewrite the golden `core-e2e-primitive.yaml`

**Files:**
- Modify: `schemas/examples/sensor/core-e2e-primitive.yaml` (full rewrite)
- Verify-only: `schemas/examples/sensor/uc-consumer.yaml` (unchanged — its `body` and `expect_status` bindings become legal under the new primitive)

- [ ] **Step 1: Rewrite the golden example**

Replace the entire content of `schemas/examples/sensor/core-e2e-primitive.yaml` with:

```yaml
schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [curl]
depends_on: [run-dev]
inputs:
  base_url:      { required: true,  default: "http://localhost:8080" }
  method:        { required: true,  default: GET }
  path:          { required: true,  default: /health_check/ready }
  query:         { required: false, default: "" }
  headers:       { required: false, default: "" }
  body:          { required: false, default: "" }
  expect_status: { required: true,  default: 2xx }
  timeout:       { required: false, default: "10" }
outputs:
  status: { from: "${{ steps.request.outputs.status }}" }
  body:   { from: "${{ steps.request.outputs.body }}" }
signal_matches:
  - key: expectation-unmet
    pattern: "expectation-unmet expected=(?P<expected>\\S+) got=(?P<got>\\d+)"
    verdict: fail
    heal_hint:
      summary: "Response status did not match the declared expectation"
      rationale: "The endpoint answered outside expect_status; inspect the handler or the expectation binding."
steps:
  - id: request
    run: |
      hdr_file=$(mktemp)
      printf '%s\n' "${{ inputs.headers }}" > "$hdr_file"
      body_arg=""
      if [ -n "${{ inputs.body }}" ]; then body_arg="--data-binary @${{ inputs.body }}"; fi
      status=$(curl -sS -o /tmp/harness-e2e-body -w '%{http_code}' \
        --max-time "${{ inputs.timeout }}" -X "${{ inputs.method }}" \
        -H "@$hdr_file" $body_arg \
        "${{ inputs.base_url }}${{ inputs.path }}${{ inputs.query }}")
      body=$(cat /tmp/harness-e2e-body 2>/dev/null || true)
      printf 'status=%s\nbody=%s\n' "$status" "$body"
      printf 'status=%s\nbody=%s\n' "$status" "$body" >> "$HARNESS_OUTPUT"
      expect=$(printf '%s' "${{ inputs.expect_status }}" | tr x '?')
      case "$status" in
        $expect) ;;
        *) printf 'expectation-unmet expected=%s got=%s\n' "${{ inputs.expect_status }}" "$status"; exit 1 ;;
      esac
```

- [ ] **Step 2: Verify the example suite stays green**

Run: `go test ./internal/sensor/ ./schemas/ && go run ./cmd/validate-schemas`
Expected: PASS / exit 0 (`loader_test` loads every example file; `schema_test` and validate-schemas validate it against `sensor.yaml`).

- [ ] **Step 3: Commit**

```bash
git add schemas/examples/sensor/core-e2e-primitive.yaml
git commit -m "docs: golden e2e primitive models the full baseline floor and grade-and-emit contract"
```

---

### Task 10: Update `skills/create-core-sensors/SKILL.md`

**Files:**
- Modify: `skills/create-core-sensors/SKILL.md` (full rewrite, hard budget ≤ 200 lines)

- [ ] **Step 1: Rewrite the skill**

Replace the entire file with the content below (it keeps the frontmatter, environment-primitive rules, command-shape table, signal_matches guidance, validation command, and coverage check; it adds the baseline floor, the grade-and-emit contract, and the two new exit-2 kinds; the long inline e2e example is replaced by a pointer to the golden example plus the inputs block):

````markdown
---
name: create-core-sensors
description: Generate the repo-level core sensors (parameterized primitives + environment primitives) from the stack manifest. Run /detect-stack first. No argument.
---

# /create-core-sensors

You are generating the repo-level core sensor primitives for this repository.
Core sensors have `scope: core`, carry no `use_case_id`, and live under
`.harness/sensors/core/<id>.yaml`.

## Prerequisites

`.harness/stack-manifest.yaml` must exist. If it is absent, stop and tell the
user to run `/detect-stack` first.

## What to read first

Use the Read tool to load:

1. `.harness/stack-manifest.yaml` — note `archetype`, `applicable_angles`, and
   `components[*].id` (the only valid ids for sensor-level `uses:`).
2. `<plugin-root>/schemas/core-inputs/<angle>.yaml` for each parameterized
   angle you will emit — the **baseline input floor**: the inputs the
   primitive MUST declare, with descriptions and suggested defaults.

## Two categories of core sensor

### Parameterized primitives (angle-typed, composable)

One per applicable parameterized angle: `e2e-test`, `database` (primitive id
`database-query`), `performance`, `logs`, `metrics`. Use-case sensors compose
them via `uses:` + `with:` to validate any journey variation — success,
failure (expected rejections), alternative.

Rules:
- Declare `inputs:` covering **at least** the angle's baseline floor; declare
  more when this repo's surface needs them (auth headers, tenant ids) — never
  fewer. Every input carries a `default:` so the primitive self-runs as a
  smoke test. Derive defaults from the manifest (e.g. `base_url` from the
  detected dev-server port); otherwise use the baseline's `suggested_default`.
- Reference EVERY declared input as `${{ inputs.<name> }}` in step `run` — a
  declared-but-ignored input is rejected (`unreferenced_input`).
- Declare `outputs:` re-exporting normalized results; write step outputs to
  `$HARNESS_OUTPUT` as `name=value` lines.
- Top-level `uses:` contains only `StackComponent` ids from the manifest.
- `depends_on:` lists core sensors that must start first (e.g. `run-dev`).

**Grade-and-emit contract** (every parameterized primitive):

1. An unexpected response is *data*, not a transport error. Never
   `curl --fail` — a 422 must reach grading so failure-variation use cases
   can expect it.
2. Always emit normalized `key=value` lines to stdout (`status=`, `body=`,
   `rows=`, `p95_ms=`, `matched_line=`) and mirror them to `$HARNESS_OUTPUT`.
   Use-case sensors grade these lines with their own `signal_matches` — keep
   them stable, one fact per line.
3. Grade the expectation inputs (`expect_status`, `expect_rows`,
   `p95_budget_ms`, `pattern`/`anti_pattern`, `predicate`) inside the
   primitive: met → exit 0; unmet → print one line
   `expectation-unmet expected=<e> got=<g>` and exit 1, covered by a fail
   matcher with a generic `heal_hint`.

The canonical full shape — inputs, outputs, matcher, and the graded curl
run — is `schemas/examples/sensor/core-e2e-primitive.yaml`. Follow it
closely; the e2e-test floor is:

```yaml
inputs:
  base_url:      { required: true,  default: "http://localhost:8080" }  # from manifest
  method:        { required: true,  default: GET }
  path:          { required: true,  default: /health_check/ready }
  query:         { required: false, default: "" }
  headers:       { required: false, default: "" }   # newline-separated "K: v"
  body:          { required: false, default: "" }   # fixture payload path
  expect_status: { required: true,  default: 2xx }  # class (2xx/4xx) or exact (422)
  timeout:       { required: false, default: "10" } # seconds
```

### Environment primitives (lifecycle, no inputs)

Environment primitives set up or tear down runtime infrastructure
(`run-dev`, `datastore`). They take **no** `inputs:` and produce no
composable `outputs:`. Their steps contain only `run:` commands.

A `kind: observational` + `scope: core` environment primitive is a **shared
service**: the runtime starts exactly one instance, reference-counts it, and
keeps it alive while any sensor is attached. Other sensors attach to its live
signal stream (they do not spawn their own copy). Two consequences:

- **Readiness key is reserved.** The service manager blocks each attaching
  sensor until the service emits an observation whose `key` is exactly
  `ready`. Your readiness matcher **must** use `key: ready` (not `api-ready`
  or similar) or consumers will never unblock.
- **Emit a firehose matcher.** Add a catch-all matcher (e.g. `key: log-line`,
  `pattern: ".+"`, `verdict: pass`) so attaching sensors can grep each server
  log line via `matched_line`.

## Command shape MUST match `kind` + `output_type`

| kind | output_type | Command shape |
|------|-------------|---------------|
| `assertion` | `single-shot` | Runs, **exits**, verdict from exit code / parsed output. A detached or one-shot command (`docker compose up -d` + a `wait-ready` loop that `exit`s, a single `curl`, `go test`, …) is correct here. |
| `observational` | `stream` | A **long-running, foreground (non-detached) command** that blocks and streams its output for as long as the watcher lives. It must **not** detach or exit on its own. |

So a `run-dev` declared `kind: observational` + `output_type: stream` MUST run
the dev stack in the **foreground** — e.g. `docker compose --profile dev up`
(NO `-d`) as its single, blocking step. Never emit `up -d` + a `wait-ready`
loop for an observational/stream sensor: that is the assertion/single-shot
shape and contradicts the declared semantics.

### signal_matches (all sensors)

Every sensor MAY declare `signal_matches: [{ key, pattern, verdict?, confidence?, expected?, heal_hint? }]`.
Each regex (Go RE2 — no backreferences/lookahead/lookbehind) is tested against
every stdout/stderr line; a match emits a Signal with the matcher's `verdict`
(default pass) and named capture groups `(?P<name>…)` as evidence.
`expected: true` (pass matchers only) means the key must be observed at least
once or the run is incomplete (fail).

Derive patterns from the logging library in `stack-manifest.yaml`:
- Anchor on individual fields; do NOT rely on JSON key order or bridge fields with `.*`.
- Prefer one matcher per outcome (a pass matcher for 2xx, a fail matcher for 5xx).
- For fail/warn matchers, provide a `heal_hint: {summary, rationale}`.

Example shape (environment primitive, observational/stream):

```yaml
id: run-dev
scope: core
angle: environment
kind: observational
nature: computational
output_type: stream
uses: [docker-compose]
signal_matches:
  - { key: ready,         pattern: "api ready|listening on", verdict: pass, expected: true }   # reserved readiness key
  - { key: log-line,      pattern: ".+",                     verdict: pass }                    # firehose
  - { key: startup-error, pattern: "Error|error|fatal",      verdict: fail, heal_hint: { summary: "Service failed to start", rationale: "Check container logs for the failing service." } }
steps:
  - id: up
    run: |
      docker compose --profile dev up   # foreground, blocking — NO -d
```

## What to emit

For each applicable primitive (driven by `applicable_angles` and `archetype`),
emit one YAML file matching `schemas/sensor.yaml`: `schema_version: 1.0.0`,
canonical `id` (`run-dev`, `e2e-test`, `database-query`, …), `scope: core`,
no `use_case_id`, the `angle`/`kind`/`nature`/`output_type` for the category,
grounded `uses:`, and `signal_matches`.

## How to validate each sensor

> **Plugin users:** `<plugin-root>` is the directory two levels above this skill file.
> Typical path after marketplace install: `~/.claude/plugins/lastro-harness/`.

```bash
<plugin-root>/scripts/harness-tools.sh create-core-sensors --file /tmp/<sensor-id>.yaml --harness-dir .harness
```

## Exit code contract

| Exit | Meaning | Action |
|------|---------|--------|
| 0 | Success | Done with this write |
| 2 | Validation failure | Read JSON error from stdout; fix YAML; retry (cap 3) |
| 1 | Script-level error | Read stderr; surface to user; stop |

Common `kind` values on exit 2:

- `grounding` — top-level `uses:` contains a component id not in the stack manifest.
- `step_resolvability` — a run-step invokes a command not installed on this machine, or a
  `make` target missing from the repo Makefile. Switch to a stack-native tool or a
  self-bootstrapping form (`go run <module>@latest`, `npx --yes <pkg>`).
- `incomplete_input_surface` — the primitive misses part of its angle's baseline floor;
  declare every input from `schemas/core-inputs/<angle>.yaml`, each with a `default`.
- `unreferenced_input` — a declared input is never referenced as `${{ inputs.<name> }}`
  in any step; bind it in the run script or remove it.
- `missing_dependency` — the stack manifest is not on disk.
- `schema_violation` — a required field is missing or wrong shape.
- `unknown_enum_value` — `kind`, `nature`, `scope`, or `output_type` is invalid.
- `scope_violation` — the sensor's `scope` is not `core`, or `use_case_id` is set.

## Coverage check

After writing all sensors, list `.harness/sensors/core/` and confirm that each
expected primitive is present. Emit any missing primitive before finishing.
````

- [ ] **Step 2: Verify the line budget**

Run: `wc -l skills/create-core-sensors/SKILL.md`
Expected: ≤ 200.

- [ ] **Step 3: Commit**

```bash
git add skills/create-core-sensors/SKILL.md
git commit -m "docs: teach create-core-sensors the baseline floor and grade-and-emit contract"
```

---

### Task 11: Update `skills/create-sensors/SKILL.md` and `angles.md`

**Files:**
- Modify: `skills/create-sensors/SKILL.md:97-104` (composition section) and the exit-kind list (after line 134)
- Modify: `skills/create-sensors/angles.md` (e2e-test, logs, metrics, database, performance sections)

- [ ] **Step 1: Replace the composition section in SKILL.md**

Replace the section starting `### Composing core primitives + demanding inputs` (lines 97–104) with:

```markdown
### Composing core primitives + evolving their inputs

A use-case sensor composes a core primitive via `uses:` + `with:`. Bind the inputs
the use case needs — e.g. `headers` for an authenticated request, or
`expect_status: "422"` so a failure variation *expects* the rejection. Every
`with:` key MUST be a declared input of the composed primitive; an undeclared key
fails validation with `unknown_with_key`.

When the use case needs an input the primitive does not declare, evolve the core
FIRST (the baseline floor is a minimum, not a ceiling):

1. Edit `.harness/sensors/core/<primitive-id>.yaml`: add the input with a
   backward-compatible default (`required: false, default: ""`) AND reference it
   as `${{ inputs.<name> }}` in the run script (unreferenced inputs are rejected).
2. Re-persist the core via `harness-tools.sh create-core-sensors`.
3. Retry this use-case sensor.

Grade over the primitive's normalized output lines (`status=`, `body=`, `rows=`,
`p95_ms=`) with this sensor's `signal_matches`. Derive auth/merchant headers and
regexes from the use case's preconditions and the manifest's logging library.
```

- [ ] **Step 2: Add the new exit kind**

In the `Common kind values on exit 2` list (after the `fixture_binding` entry), insert:

```markdown
- `unknown_with_key` — a step's `with:` key is not a declared input of the composed
  core primitive. Evolve the core first (see "Composing core primitives"), then retry.
```

- [ ] **Step 3: Update angles.md per-angle sections**

In `skills/create-sensors/angles.md`:

(a) **e2e-test** — replace the `- **Grading:**` bullet (lines 71-75) with:

```markdown
- **Inputs:** the primitive's floor (`schemas/core-inputs/e2e-test.yaml`) is
  `base_url, method, path, query, headers, body, expect_status, timeout`. Bind
  `expect_status` to the variation's expectation — `"422"` for a failure use
  case — and `body` to a fixture path. The primitive grades the status itself
  (no `curl --fail`) and emits `status=` / `body=` lines.
- **Grading:** layer matchers over those normalized lines for the `then`
  clauses: an `expected: true` pass matcher on the expected status/body shape,
  fail matchers with `heal_hint`s for wrong status or missing fields. One
  sensor covers one use case; bind each variation to its own use case.
```

(b) **database** — in the `- **Shape:**` bullet (lines 155-158), after `the query and` ... `identifiers;` insert the floor sentence so the bullet reads:

```markdown
- **Shape:** `assertion` / `computational` / `single-shot`. Compose the
  `database-query` core primitive (floor: `query, params, expect_rows,
  timeout`) with the query and `${{ fixtures.<id> }}`-bound identifiers —
  bind `expect_rows: "0"` to assert an ABSENT write for failure variations;
  fall back to a stack CLI probe (`psql -c`, `aws dynamodb get-item`).
```

(c) **performance** — in its `- **Shape:**` bullet (lines 167-169), change `Compose the
  `performance` core primitive,` to:

```markdown
- **Shape:** `assertion` / `computational` / `single-shot`. Compose the
  `performance` core primitive (floor: `base_url, method, path, headers,
  body, duration, rate, p95_budget_ms`; it emits `p95_ms=` and grades the
  budget itself), or fall back to a load tool from the stack
  (`hey -n 100 <url>`, `k6 run script.js`); needs `depends_on: [run-dev]`.
```

(d) **logs** — append one sentence to the `- **Shape:**` bullet (after `…no observational core service exists.`):

```markdown
  A parameterized `logs` primitive (floor: `pattern, anti_pattern, within,
  service`) may also exist for one-shot grep-style assertions over the
  shared stream.
```

(e) **metrics** — replace the `- **Command:**` bullet (line 145) with:

```markdown
- **Command:** compose the `metrics` core primitive (floor: `metrics_url,
  name, labels, predicate, within`; it grades the predicate and emits the
  scraped value), or fall back to
  `curl -sS http://localhost:9090/metrics | grep <key>`.
```

- [ ] **Step 4: Verify budgets and commit**

Run: `wc -l skills/create-sensors/SKILL.md skills/create-sensors/angles.md`
Expected: SKILL.md ≤ 200 (angles.md is a reference doc, not budget-bound, but keep edits tight).

```bash
git add skills/create-sensors/
git commit -m "docs: replace prose-only core evolution with the validated unknown_with_key flow"
```

---

### Task 12: Final verification

- [ ] **Step 1: Full test suite**

Run: `go test ./...`
Expected: PASS, no failures anywhere.

- [ ] **Step 2: Schema + example validation**

Run: `go run ./cmd/validate-schemas`
Expected: exit 0, `All schemas, enums, and examples validated.`

- [ ] **Step 3: Spec cross-check**

Confirm each spec section maps to landed work:
- §1 baselines → Tasks 2–3; §2 grade-and-emit → Tasks 9–10; §3 validators →
  Tasks 1, 4–8; §4 skills/examples → Tasks 9–11; §5 testing → unit tests in
  Tasks 3–8 + evolution-flow test in Task 8.

- [ ] **Step 4: Commit any stragglers**

```bash
git status
git add -A docs/ && git commit -m "docs: flexible core sensors implementation plan" # only if the plan file itself is uncommitted
```
