# Core Sensors — Foundation (schema + enums + internal/sensor) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make repo-level "core" sensors expressible and loadable by introducing a `scope: core | use-case` discriminator on `Sensor`, a new `environment` validation angle, and the matching Go machinery in `internal/sensor` — with backward compatibility for every existing sensor.

**Architecture:** Schema-first (the schema-freeze gate is updated before any Go code). The discriminator `scope` governs whether `use_case_id` is required; `angle` stays required for both scopes. Core sensors live under `.harness/sensors/core/`, use-case sensors under `.harness/sensors/<usecase-id>/`. The loader tolerates both the new foldered layout and the legacy flat layout (half-migrated state). The `angle_not_applicable` grounding check applies only to `use-case` sensors.

**Tech Stack:** Go, `sigs.k8s.io/yaml`, `santhosh-tekuri/jsonschema` (draft 2020-12), YAML schemas under `schemas/`.

**Spec:** [`docs/superpowers/specs/2026-05-29-core-sensors-design.md`](../specs/2026-05-29-core-sensors-design.md). This plan implements §5 (schema), §6 (internal/sensor + internal/enums), and decisions #1–#4, #9–#11. The `/create-core-sensors` skill (§7), the `validate-use-case` gather change (§8), the physical migration of on-disk flat files (§11 Q4), and the dogfood run are **follow-on plans** (see "Follow-on plans" at the end).

**Scope note:** This is a foundation slice — it delivers tested Go + schema machinery with no user-visible behavior change yet, mirroring how this project landed the E-series entity packages. It is independently testable: after it lands, a `scope: core` sensor loads, validates, stores, and persists; existing sensors are unaffected.

---

## File structure (what changes and why)

| File | Responsibility | Change |
|------|----------------|--------|
| `docs/harness-framework/00-schema-freeze.md` | The freeze gate that pins cross-entity field names | Record the `scope` field + `environment` angle |
| `schemas/enums/validation-angles.yaml` | Canonical angle enum | Append `environment` |
| `schemas/enums/sensor-scopes.yaml` | **New** canonical scope enum | Create with `core`, `use-case` |
| `schemas/sensor.yaml` | Sensor schema | Add `scope`; add `environment` to angle enum; make `use_case_id` conditional |
| `schemas/examples/sensor/core-run-dev.yaml` | **New** golden example | A `scope: core` sensor |
| `internal/enums/enums.go` | Typed enum constants | Add `AngleEnvironment`; add `SensorScope` type + helpers |
| `internal/enums/drift_test.go` | Guards Go↔YAML enum drift | Add `sensor-scopes` to both canonical lists |
| `internal/sensor/types.go` | `Sensor` struct | Add `Scope` field |
| `internal/sensor/loader.go` | Load + schema-validate + walk dir | Default `Scope`; walk one level of subfolders |
| `internal/sensor/validate.go` | Intrinsic (post-schema) invariants | Add scope↔use_case_id consistency check |
| `internal/sensor/store.go` | In-memory index | Add `byScope` index + `ForScope` accessor |
| `internal/sensor/persist.go` | Write path used by generation skills | Scope the angle check to use-case; write to per-scope folders; skip use-case/fixture checks for core |

---

## Task 1: Schema-freeze gate — record the change (docs only, no code)

Per `CLAUDE.md`, the freeze gate lands **before** any Go code so cross-entity field names don't drift across parallel work.

**Files:**
- Modify: `docs/harness-framework/00-schema-freeze.md`

- [ ] **Step 1: Read the current freeze doc to match its format**

Run: `sed -n '1,40p' docs/harness-framework/00-schema-freeze.md`
Note the section that lists per-entity frozen fields for `Sensor`.

- [ ] **Step 2: Add the change record**

In the Sensor section of `00-schema-freeze.md`, append a dated entry documenting:

```markdown
### 2026-05-29 — core sensors (issue #24)

- **`Sensor.scope`** (string enum `core | use-case`, default `use-case`): repo-level vs use-case-bound.
- **`Sensor.use_case_id`** is now **conditional**: required when `scope: use-case`, forbidden when `scope: core`.
- **`ValidationAngle`** gains an 11th value **`environment`** (boot/datastore preconditions). It is added to the
  angle enum and `schemas/sensor.yaml`'s inline `angle` enum, but **NOT** to `ValidationPolicy.AngleList`
  (environment is a DAG precondition, never policy-graded).
- File layout: core sensors under `.harness/sensors/core/`, use-case sensors under `.harness/sensors/<usecase-id>/`.
- Backward compatibility: a sensor that omits `scope` defaults to `use-case` and still requires `use_case_id`.
```

- [ ] **Step 3: Commit**

```bash
git add docs/harness-framework/00-schema-freeze.md
git commit -m "docs(schema-freeze): record core sensor scope + environment angle (#24)"
```

---

## Task 2: Add the `environment` angle (enum YAML + Go constant)

**Files:**
- Modify: `schemas/enums/validation-angles.yaml`
- Modify: `internal/enums/enums.go:8-31`
- Test: `internal/enums/drift_test.go` (existing `TestGoConstantsMatchYAML` covers this — no new test code, the change must keep it green)

- [ ] **Step 1: Run the drift test to confirm it is currently green**

Run: `go test ./internal/enums/ -run TestGoConstantsMatchYAML -v`
Expected: PASS (`validation-angles` subtest passes with 10 angles).

- [ ] **Step 2: Append `environment` to the canonical YAML**

In `schemas/enums/validation-angles.yaml`, after the `performance` entry:

```yaml
  - id: environment
    purpose: "Repo-level environment preconditions: app boot, datastore reachability (core sensors only)"
```

- [ ] **Step 3: Run the drift test to verify it now FAILS**

Run: `go test ./internal/enums/ -run TestGoConstantsMatchYAML -v`
Expected: FAIL — `drift in validation-angles: yaml has 11, go has 10`.

- [ ] **Step 4: Add the Go constant and include it in `AllAngles()`**

In `internal/enums/enums.go`, add to the angle `const` block (line 21, after `AnglePerformance`):

```go
	AnglePerformance   ValidationAngle = "performance"
	AngleEnvironment   ValidationAngle = "environment"
```

And update `AllAngles()` (line 25-31):

```go
func AllAngles() []ValidationAngle {
	return []ValidationAngle{
		AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
		AngleE2ETest, AngleContracts, AngleLogs, AngleMetrics,
		AngleDatabase, AnglePerformance, AngleEnvironment,
	}
}
```

Also update the type doc comment on line 8 (`one of the ten facets` → `one of the eleven facets`).

- [ ] **Step 5: Run the drift test to verify it passes**

Run: `go test ./internal/enums/ -run TestGoConstantsMatchYAML -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add schemas/enums/validation-angles.yaml internal/enums/enums.go
git commit -m "feat(enums): add environment validation angle (#24)"
```

---

## Task 3: New `SensorScope` enum (YAML + Go + drift wiring)

**Files:**
- Create: `schemas/enums/sensor-scopes.yaml`
- Modify: `internal/enums/enums.go` (add type + helpers)
- Modify: `internal/enums/drift_test.go:77-86` and `:149-153`
- Test: existing `TestGoConstantsMatchYAML` / `TestInlineSchemaEnumsMatchYAML` (extended via the two list edits)

- [ ] **Step 1: Create the canonical scope enum YAML**

Create `schemas/enums/sensor-scopes.yaml`:

```yaml
schema_version: 1.0.0
title: SensorScope
description: |
  Whether a sensor is bound to a single use case or is a repo-level
  precondition shared across use cases. Core sensors carry no use_case_id
  and form the roots of the depends_on DAG.

values:
  - id: core
    purpose: "Repo-level, use-case-agnostic. No use_case_id. DAG root."
  - id: use-case
    purpose: "Bound to one use case via use_case_id. Default when scope is omitted."
```

- [ ] **Step 2: Add `sensor-scopes` to both drift lists and run to verify it FAILS**

In `internal/enums/drift_test.go`, add to the `TestGoConstantsMatchYAML` cases (after the `stack-kinds` line at 85):

```go
		{"stack-kinds", stringify(AllStackKinds())},
		{"sensor-scopes", stringify(AllSensorScopes())},
```

And add to the `enumNames` slice in `TestInlineSchemaEnumsMatchYAML` (line 149-153):

```go
	enumNames := []string{
		"validation-angles", "archetypes", "sensor-kinds", "sensor-natures",
		"signal-output-types", "fixture-roles", "verdicts", "termination-reasons",
		"stack-kinds", "sensor-scopes",
	}
```

Run: `go test ./internal/enums/ -v`
Expected: FAIL to compile — `undefined: AllSensorScopes`.

- [ ] **Step 3: Add the Go type + helpers**

In `internal/enums/enums.go`, after the `StackKind` block (line 251):

```go
// SensorScope is whether a sensor is repo-level (core) or bound to a use case.
type SensorScope string

const (
	ScopeCore    SensorScope = "core"
	ScopeUseCase SensorScope = "use-case"
)

// AllSensorScopes returns every SensorScope in canonical (YAML) order.
func AllSensorScopes() []SensorScope {
	return []SensorScope{ScopeCore, ScopeUseCase}
}

// IsValidSensorScope reports whether s is one of the canonical SensorScope values.
func IsValidSensorScope(s string) bool {
	for _, v := range AllSensorScopes() {
		if string(v) == s {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the drift test to verify it passes**

Run: `go test ./internal/enums/ -v`
Expected: PASS. (Note: `TestInlineSchemaEnumsMatchYAML` will only stay green once `sensor.yaml` carries an inline `scope` enum equal to `{core, use-case}` — that lands in Task 4. If running this task in isolation before Task 4, the `sensor-scopes` referenced check fails; run Tasks 3 and 4 together, then test.)

- [ ] **Step 5: Commit**

```bash
git add schemas/enums/sensor-scopes.yaml internal/enums/enums.go internal/enums/drift_test.go
git commit -m "feat(enums): add SensorScope enum (core|use-case) (#24)"
```

---

## Task 4: Update `schemas/sensor.yaml` (scope, environment, conditional use_case_id)

**Files:**
- Modify: `schemas/sensor.yaml`
- Test: `internal/enums/drift_test.go` (`TestInlineSchemaEnumsMatchYAML`), `internal/sensor` schema tests

- [ ] **Step 1: Add `scope` to `required`-independent properties + the inline enums**

In `schemas/sensor.yaml`, change the top-level `required` (line 11) to drop `use_case_id`:

```yaml
required: [schema_version, id, angle, kind, nature, output_type, uses, steps]
```

Add the `scope` property (inside `properties:`, before `use_case_id`):

```yaml
  scope:
    type: string
    description: "Canonical source: schemas/enums/sensor-scopes.yaml"
    enum: [core, use-case]
    default: use-case
```

Add `environment` to the inline `angle` enum (line 25-26):

```yaml
  angle:
    type: string
    description: "Canonical source: schemas/enums/validation-angles.yaml"
    enum: [security, build, code-structure, unit-test, e2e-test,
           contracts, logs, metrics, database, performance, environment]
```

- [ ] **Step 2: Add the conditional `use_case_id` rules**

Append an `allOf` block at the top level of `schemas/sensor.yaml` (sibling to `properties`, after `$defs` or before it — placement is free in JSON Schema):

```yaml
allOf:
  - if:
      anyOf:
        - not: { required: [scope] }
        - properties: { scope: { const: use-case } }
    then:
      required: [use_case_id]
  - if:
      properties: { scope: { const: core } }
      required: [scope]
    then:
      not: { required: [use_case_id] }
```

Note: the `if` clauses guard explicitly on scope **presence**. JSON Schema `properties` is vacuously true when the key is absent, so a naive `properties: {scope: {const: use-case}}` would ALSO match a scope-less sensor — and the second branch's `properties: {scope: {const: core}}` would match it too, firing both contradictory `then`s and breaking every existing (scope-less, `use_case_id`-bearing) sensor. The `anyOf` + `required: [scope]` guards make branch 1 fire when scope is absent-or-use-case and branch 2 fire only when scope is explicitly `core`.

- [ ] **Step 3: Run the enum drift test to verify it stays green**

Run: `go test ./internal/enums/ -run 'TestInlineSchemaEnumsMatchYAML|TestGoConstantsMatchYAML' -v`
Expected: PASS — `sensor.yaml`'s `angle` enum now equals the 11-value canonical (marks `validation-angles` referenced); its `scope` enum equals `{core, use-case}` (marks `sensor-scopes` referenced); `validation-policy.yaml`'s `AngleList` (10 values) is a strict subset and is skipped silently.

- [ ] **Step 4: Commit**

```bash
git add schemas/sensor.yaml
git commit -m "feat(schema): sensor scope discriminator + conditional use_case_id (#24)"
```

---

## Task 5: Golden example — a `scope: core` sensor

**Files:**
- Create: `schemas/examples/sensor/core-run-dev.yaml`
- Test: whatever test validates `schemas/examples/sensor/*.yaml` against the schema (find it in Step 1)

- [ ] **Step 1: Find the example-validation test**

Run: `grep -rn "examples/sensor" --include=*.go .`
Expected: a test that loads every `schemas/examples/sensor/*.yaml` and asserts it validates. Note its path.

- [ ] **Step 2: Create the core example**

Create `schemas/examples/sensor/core-run-dev.yaml`:

```yaml
schema_version: 1.0.0
id: run-dev
scope: core
angle: environment
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: boot
    run: "make dev"
  - id: wait-ready
    run: "curl --fail --retry 30 --retry-delay 1 http://localhost:8080/health_check/ready"
```

(Note: `uses: []` and the `make dev` command are illustrative for the *example*; correct command grounding is the follow-on generation plan's concern, not the schema example's.)

- [ ] **Step 3: Run the example-validation test**

Run: `go test ./... -run Example` (or the specific test found in Step 1)
Expected: PASS — the core example validates: no `use_case_id`, `scope: core`, `angle: environment`.

- [ ] **Step 4: Commit**

```bash
git add schemas/examples/sensor/core-run-dev.yaml
git commit -m "test(schema): golden core-scope sensor example (#24)"
```

---

## Task 6: `Sensor.Scope` field + loader default

**Files:**
- Modify: `internal/sensor/types.go:15-26`
- Modify: `internal/sensor/loader.go` (`LoadSensorBytes`)
- Test: `internal/sensor/loader_test.go` (add cases)

- [ ] **Step 1: Write the failing test**

Add to `internal/sensor/loader_test.go`:

```go
func TestLoadSensor_DefaultsScopeToUseCase(t *testing.T) {
	yaml := []byte(`schema_version: 1.0.0
id: order-create-build
use_case_id: order-create
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: compile
    run: "go build ./..."
`)
	s, err := LoadSensorBytes(yaml)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Scope != enums.ScopeUseCase {
		t.Errorf("scope: got %q, want %q (default)", s.Scope, enums.ScopeUseCase)
	}
}

func TestLoadSensor_CoreScopeNoUseCase(t *testing.T) {
	yaml := []byte(`schema_version: 1.0.0
id: run-dev
scope: core
angle: environment
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: boot
    run: "make dev"
`)
	s, err := LoadSensorBytes(yaml)
	if err != nil {
		t.Fatalf("load core: %v", err)
	}
	if s.Scope != enums.ScopeCore {
		t.Errorf("scope: got %q, want core", s.Scope)
	}
	if s.UseCaseID != "" {
		t.Errorf("use_case_id: got %q, want empty for core", s.UseCaseID)
	}
}
```

Add the `enums` import to the test file if not present.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/sensor/ -run TestLoadSensor_ -v`
Expected: FAIL to compile — `s.Scope undefined`.

- [ ] **Step 3: Add the `Scope` field**

In `internal/sensor/types.go`, add to the `Sensor` struct (after `ID`, before `UseCaseID`):

```go
	ID            string                 `json:"id"`
	Scope         enums.SensorScope      `json:"scope,omitempty"`
	UseCaseID     string                 `json:"use_case_id,omitempty"`
```

(Change `use_case_id` tag to add `,omitempty` so core sensors round-trip without an empty key.)

- [ ] **Step 4: Default the scope in `LoadSensorBytes`**

In `internal/sensor/loader.go`, in `LoadSensorBytes`, after `json.Unmarshal` (line 45) and before `validateIntrinsic`:

```go
	var s Sensor
	if err := json.Unmarshal(asJSON, &s); err != nil {
		return Sensor{}, fmt.Errorf("deserialize: %w", err)
	}

	if s.Scope == "" {
		s.Scope = enums.ScopeUseCase
	}

	if err := validateIntrinsic(s); err != nil {
```

Add `"github.com/iurykrieger/lastro/internal/enums"` to loader.go imports if absent.

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/sensor/ -run TestLoadSensor_ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/sensor/types.go internal/sensor/loader.go internal/sensor/loader_test.go
git commit -m "feat(sensor): add Scope field, default to use-case on load (#24)"
```

---

## Task 7: Intrinsic scope↔use_case_id consistency check

The schema if/then already enforces this for the YAML path, but `NewStore` accepts in-memory `Sensor` values that bypass the schema. Add a belt-and-suspenders intrinsic check.

**Files:**
- Modify: `internal/sensor/validate.go`
- Test: `internal/sensor/validate_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/sensor/validate_test.go`:

```go
func TestValidateIntrinsic_CoreScopeForbidsUseCaseID(t *testing.T) {
	s := Sensor{
		ID: "run-dev", Scope: enums.ScopeCore, UseCaseID: "order-create",
		Angle: enums.AngleEnvironment, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Steps: []Step{{ID: "boot", Run: "make dev"}},
	}
	err := validateIntrinsic(s)
	if err == nil || !strings.Contains(err.Error(), "core") {
		t.Fatalf("expected core-scope use_case_id error, got %v", err)
	}
}

func TestValidateIntrinsic_UseCaseScopeRequiresUseCaseID(t *testing.T) {
	s := Sensor{
		ID: "x-build", Scope: enums.ScopeUseCase, UseCaseID: "",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Steps: []Step{{ID: "c", Run: "go build ./..."}},
	}
	err := validateIntrinsic(s)
	if err == nil || !strings.Contains(err.Error(), "use_case_id") {
		t.Fatalf("expected missing use_case_id error, got %v", err)
	}
}
```

Ensure `strings` and `enums` are imported in the test file.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/sensor/ -run TestValidateIntrinsic_ -v`
Expected: FAIL — no error returned (check not implemented).

- [ ] **Step 3: Implement the check**

In `internal/sensor/validate.go`, add a call inside `validateIntrinsic` (after `checkNoSelfDependency`):

```go
	if err := checkScopeConsistency(s); err != nil {
		errs = append(errs, err)
	}
```

And add the function and the `enums` import:

```go
func checkScopeConsistency(s Sensor) error {
	switch s.Scope {
	case enums.ScopeCore:
		if s.UseCaseID != "" {
			return fmt.Errorf("scope=core forbids use_case_id (got %q)", s.UseCaseID)
		}
	case enums.ScopeUseCase, "": // "" treated as use-case (loader defaults it)
		if s.UseCaseID == "" {
			return fmt.Errorf("scope=use-case requires use_case_id")
		}
	}
	return nil
}
```

Add `"github.com/iurykrieger/lastro/internal/enums"` to validate.go imports.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/sensor/ -v`
Expected: PASS (all sensor tests, including existing ones, stay green — existing use-case sensors have a `use_case_id`).

- [ ] **Step 5: Commit**

```bash
git add internal/sensor/validate.go internal/sensor/validate_test.go
git commit -m "feat(sensor): intrinsic scope<->use_case_id consistency check (#24)"
```

---

## Task 8: `Store.byScope` index + `ForScope` accessor

**Files:**
- Modify: `internal/sensor/store.go`
- Test: `internal/sensor/store_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/sensor/store_test.go`:

```go
func TestStore_ForScope(t *testing.T) {
	core := Sensor{ID: "run-dev", Scope: enums.ScopeCore, Angle: enums.AngleEnvironment,
		Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Steps: []Step{{ID: "b", Run: "make dev"}}}
	uc := Sensor{ID: "oc-build", Scope: enums.ScopeUseCase, UseCaseID: "order-create", Angle: enums.AngleBuild,
		Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Steps: []Step{{ID: "c", Run: "go build ./..."}}}
	st, err := NewStore(core, uc)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	got := st.ForScope(enums.ScopeCore)
	if len(got) != 1 || got[0].ID != "run-dev" {
		t.Fatalf("ForScope(core): got %+v", got)
	}
	if len(st.ForScope(enums.ScopeUseCase)) != 1 {
		t.Fatalf("ForScope(use-case): want 1")
	}
}

func TestStore_DuplicateIDRejected(t *testing.T) {
	a := Sensor{ID: "dup", Scope: enums.ScopeCore, Angle: enums.AngleBuild,
		Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Steps: []Step{{ID: "s", Run: "x"}}}
	if _, err := NewStore(a, a); err == nil {
		t.Fatal("expected duplicate-id error")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/sensor/ -run TestStore_ForScope -v`
Expected: FAIL to compile — `st.ForScope undefined`.

- [ ] **Step 3: Implement `byScope` + `ForScope`**

In `internal/sensor/store.go`, add the field to the struct:

```go
type Store struct {
	byID         map[string]Sensor
	byUseCase    map[string][]string // useCaseID -> sorted []ID
	byScope      map[enums.SensorScope][]string
	allSortedIDs []string
}
```

In `NewStore`, initialize and populate it:

```go
	s := &Store{
		byID:      make(map[string]Sensor, len(sensors)),
		byUseCase: make(map[string][]string),
		byScope:   make(map[enums.SensorScope][]string),
	}
	for _, sn := range sensors {
		if _, exists := s.byID[sn.ID]; exists {
			return nil, fmt.Errorf("sensor: duplicate id %q", sn.ID)
		}
		scope := sn.Scope
		if scope == "" {
			scope = enums.ScopeUseCase
		}
		s.byID[sn.ID] = sn
		s.byUseCase[sn.UseCaseID] = append(s.byUseCase[sn.UseCaseID], sn.ID)
		s.byScope[scope] = append(s.byScope[scope], sn.ID)
		s.allSortedIDs = append(s.allSortedIDs, sn.ID)
	}
	for uc := range s.byUseCase {
		sort.Strings(s.byUseCase[uc])
	}
	for sc := range s.byScope {
		sort.Strings(s.byScope[sc])
	}
	sort.Strings(s.allSortedIDs)
	return s, nil
```

Add the accessor:

```go
// ForScope returns all sensors with the given scope, sorted by id
// ascending. An absent scope on a stored sensor counts as use-case.
func (s *Store) ForScope(scope enums.SensorScope) []Sensor {
	ids := s.byScope[scope]
	out := make([]Sensor, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.byID[id])
	}
	return out
}
```

Add `"github.com/iurykrieger/lastro/internal/enums"` to store.go imports.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/sensor/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sensor/store.go internal/sensor/store_test.go
git commit -m "feat(sensor): byScope index + ForScope accessor (#24)"
```

---

## Task 9: Loader walks one level of subfolders (foldered + legacy flat)

**Files:**
- Modify: `internal/sensor/loader.go` (`LoadDirectory`)
- Test: `internal/sensor/loader_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/sensor/loader_test.go`:

```go
func TestLoadDirectory_FolderedAndFlat(t *testing.T) {
	dir := t.TempDir()
	// legacy flat use-case sensor
	writeSensor(t, filepath.Join(dir, "oc-build.yaml"), "oc-build", "use-case", "order-create", "build")
	// foldered use-case sensor
	if err := os.MkdirAll(filepath.Join(dir, "order-create"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSensor(t, filepath.Join(dir, "order-create", "oc-e2e.yaml"), "oc-e2e", "use-case", "order-create", "e2e-test")
	// foldered core sensor
	if err := os.MkdirAll(filepath.Join(dir, "core"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSensor(t, filepath.Join(dir, "core", "run-dev.yaml"), "run-dev", "core", "", "environment")

	st, err := LoadDirectory(dir)
	if err != nil {
		t.Fatalf("load dir: %v", err)
	}
	if len(st.All()) != 3 {
		t.Fatalf("want 3 sensors, got %d", len(st.All()))
	}
	if len(st.ForScope(enums.ScopeCore)) != 1 {
		t.Fatalf("want 1 core sensor")
	}
}

// writeSensor writes a minimal valid sensor YAML to path.
func writeSensor(t *testing.T, path, id, scope, useCase, angle string) {
	t.Helper()
	var ucLine string
	if useCase != "" {
		ucLine = "use_case_id: " + useCase + "\n"
	}
	body := "schema_version: 1.0.0\nid: " + id + "\nscope: " + scope + "\n" + ucLine +
		"angle: " + angle + "\nkind: assertion\nnature: computational\noutput_type: single-shot\nuses: []\nsteps:\n  - id: s\n    run: \"echo ok\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

Ensure `os`, `path/filepath`, and `enums` are imported in the test file.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/sensor/ -run TestLoadDirectory_FolderedAndFlat -v`
Expected: FAIL — only 1 sensor loaded (the flat one); subfolders are skipped by the current non-recursive walk.

- [ ] **Step 3: Rewrite `LoadDirectory` to descend one level**

Replace the entry-collection loop in `internal/sensor/loader.go` `LoadDirectory` (lines 61-86) so it gathers `*.yaml`/`*.yml` from the top directory **and** from each immediate subdirectory:

```go
func LoadDirectory(path string) (*Store, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("sensor dir %s: read: %w", path, err)
	}

	type loaded struct {
		s    Sensor
		from string
	}
	var sensors []loaded

	collect := func(dir string, files []os.DirEntry) error {
		for _, e := range files {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
				continue
			}
			full := filepath.Join(dir, name)
			sn, err := LoadSensor(full)
			if err != nil {
				return err // already path-decorated by LoadSensor
			}
			sensors = append(sensors, loaded{s: sn, from: full})
		}
		return nil
	}

	if err := collect(path, entries); err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(path, e.Name())
		subEntries, err := os.ReadDir(sub)
		if err != nil {
			return nil, fmt.Errorf("sensor subdir %s: read: %w", sub, err)
		}
		if err := collect(sub, subEntries); err != nil {
			return nil, err
		}
	}

	// Build the store. Catch duplicate ids here so the error can name both files.
	seen := make(map[string]string, len(sensors))
	bare := make([]Sensor, 0, len(sensors))
	for _, l := range sensors {
		if prior, dup := seen[l.s.ID]; dup {
			return nil, fmt.Errorf("sensor: duplicate id %q in %s and %s", l.s.ID, prior, l.from)
		}
		seen[l.s.ID] = l.from
		bare = append(bare, l.s)
	}

	return NewStore(bare...)
}
```

Also update the doc comment on `LoadDirectory` (lines 54-60) to say it walks the directory plus one level of immediate subdirectories (`<usecase-id>/`, `core/`), tolerating the legacy flat layout.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/sensor/ -v`
Expected: PASS (all sensor tests).

- [ ] **Step 5: Commit**

```bash
git add internal/sensor/loader.go internal/sensor/loader_test.go
git commit -m "feat(sensor): loader walks per-usecase and core subfolders (#24)"
```

---

## Task 10: `Persist` — scope the angle check, write per-scope, skip use-case checks for core

**Files:**
- Modify: `internal/sensor/persist.go`
- Test: `internal/sensor/persist_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/sensor/persist_test.go` (mirror the setup of the existing persist tests — they build a temp `harnessDir` with a `stack-manifest.yaml`). A core sensor must persist to `sensors/core/<id>.yaml` **without** requiring a use-case file, and its `environment` angle must be accepted even though it is not in `applicable_angles`:

```go
func TestPersist_CoreSensorWritesToCoreFolder(t *testing.T) {
	dir := newHarnessDirWithManifest(t) // existing helper that writes stack-manifest.yaml with applicable_angles
	content := []byte(`schema_version: 1.0.0
id: run-dev
scope: core
angle: environment
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: boot
    run: "make dev"
`)
	if err := Persist(content, dir); err != nil {
		t.Fatalf("persist core: %v", err)
	}
	got := filepath.Join(dir, "sensors", "core", "run-dev.yaml")
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("expected core sensor at %s: %v", got, err)
	}
}

func TestPersist_UseCaseSensorWritesToUseCaseFolder(t *testing.T) {
	dir := newHarnessDirWithManifestAndUseCase(t, "order-create") // existing-style helper
	content := []byte(`schema_version: 1.0.0
id: order-create-build
scope: use-case
use_case_id: order-create
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: compile
    run: "go build ./..."
`)
	if err := Persist(content, dir); err != nil {
		t.Fatalf("persist use-case: %v", err)
	}
	got := filepath.Join(dir, "sensors", "order-create", "order-create-build.yaml")
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("expected use-case sensor at %s: %v", got, err)
	}
}
```

If the named helpers don't exist verbatim, inline the temp-dir setup the existing persist tests already use (read `persist_test.go` first and reuse its pattern). The two assertions that matter: core writes to `sensors/core/`, use-case writes to `sensors/<use_case_id>/`, and the core sensor with `environment` angle is accepted.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/sensor/ -run TestPersist_ -v`
Expected: FAIL — core sensor rejected with `angle_not_applicable` (environment not in applicable_angles) and/or written to the flat `sensors/` path, and the use-case sensor written flat rather than under `order-create/`.

- [ ] **Step 3: Branch `Persist` on scope**

In `internal/sensor/persist.go`, after the `LoadSensorBytes` + `ValidateAgainstStack` block (line 53), gate checks (b)–(d) on scope, and compute a per-scope target path. Replace lines 55-106 with:

```go
	// (b)-(d) are use-case-scoped. Core sensors are repo-level: no angle
	// applicability check (they legitimately carry environment and any
	// angle), no use-case existence check, no fixture binding.
	if s.Scope != enums.ScopeCore {
		// (b) Angle must be in manifest's applicable_angles.
		if !angleInList(s.Angle, manifest.ApplicableAngles) {
			return &persisterror.Error{
				Kind:       persisterror.AngleNotApplicable,
				EntityType: "sensor",
				EntityID:   s.ID,
				Message: fmt.Sprintf("angle %q is not in stack-manifest.applicable_angles %v",
					s.Angle, manifest.ApplicableAngles),
			}
		}

		// (c) Use-case must exist on disk.
		ucPath := filepath.Join(harnessDir, "use-cases", s.UseCaseID+".yaml")
		if _, err := os.Stat(ucPath); errors.Is(err, os.ErrNotExist) {
			return &persisterror.Error{
				Kind:       persisterror.MissingDependency,
				EntityType: "sensor",
				EntityID:   s.ID,
				Message:    fmt.Sprintf("use-case %q not found at %s", s.UseCaseID, ucPath),
			}
		}

		// (d) Step uses ⊆ fixtures with matching use_case_id.
		store, err := loadFixtureStoreOrEmpty(filepath.Join(harnessDir, "fixtures"))
		if err != nil {
			return &persisterror.Error{
				Kind:       persisterror.FixtureBinding,
				EntityType: "sensor",
				EntityID:   s.ID,
				Message:    fmt.Sprintf("load fixtures: %v", err),
			}
		}
		if err := ValidateAgainstFixtures(s, fixtureOwner{store: store, useCaseID: s.UseCaseID}); err != nil {
			return &persisterror.Error{
				Kind:       persisterror.FixtureBinding,
				EntityType: "sensor",
				EntityID:   s.ID,
				Message:    err.Error(),
			}
		}
	}

	// Per-scope target directory: core -> sensors/core/, use-case -> sensors/<use_case_id>/.
	subdir := s.UseCaseID
	if s.Scope == enums.ScopeCore {
		subdir = "core"
	}
	sensorsDir := filepath.Join(harnessDir, "sensors", subdir)

	// Map-based bump (preserves original field order/content faithfully).
	var raw map[string]any
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "sensor",
			EntityID:   s.ID,
			Message:    fmt.Sprintf("re-parse for bump: %v", err),
		}
	}
	targetPath := filepath.Join(sensorsDir, s.ID+".yaml")
```

Then keep the existing bump + marshal + write block, but ensure the target directory exists before `AtomicWrite`. Check whether `persisthelp.AtomicWrite` creates parent dirs (read `internal/persisthelp`); if it does not, add before the write:

```go
	if err := os.MkdirAll(sensorsDir, 0o755); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "sensor",
			EntityID:   s.ID,
			Message:    fmt.Sprintf("mkdir %s: %v", sensorsDir, err),
		}
	}
```

(The `BumpSchemaVersion(targetPath, ...)` call already uses `targetPath`; update it to the new per-scope `targetPath`.)

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/sensor/ -v`
Expected: PASS, including the existing persist tests. ⚠️ If existing persist tests assert the old flat path `sensors/<id>.yaml`, they will now fail because use-case sensors move to `sensors/<use_case_id>/<id>.yaml`. Update those assertions in the same commit — this is the intended layout change (spec §4 decision #10).

- [ ] **Step 5: Run the full module test suite**

Run: `go test ./...`
Expected: PASS. If `cmd/harness` or `skills/*/scripts` tests assert the flat sensor path, they belong to the follow-on gather/migration plan; if any break here, note them and fix the assertion to the new path (do not change gather logic in this plan).

- [ ] **Step 6: Commit**

```bash
git add internal/sensor/persist.go internal/sensor/persist_test.go
git commit -m "feat(sensor): persist core sensors to sensors/core, use-case to per-uc folder (#24)"
```

---

## Self-review (completed by plan author)

- **Spec coverage:** §5 (schema) → Tasks 1,4. §6 internal/enums → Tasks 2,3. §6 internal/sensor (Scope field, conditional validators, slug uniqueness, loader subfolders, store byScope, persist applicability scoping + per-scope write) → Tasks 6,7,8,9,10. Decision #11 (environment not in ApplicableAngles; check scoped to use-case) → Task 10. Backward compat → Tasks 4,6,7. Golden example → Task 5. **Deferred to follow-on plans (intentional, stated in header):** §7 generation, §8 gather, §11 Q4 physical migration of the two on-disk flat files, dogfood.
- **Placeholder scan:** none — every code step shows complete code; helper-name fallbacks (Task 10) explicitly instruct reading the existing test file and reusing its pattern, with the concrete assertions spelled out.
- **Type consistency:** `enums.SensorScope`, `ScopeCore`, `ScopeUseCase`, `AngleEnvironment`, `Store.byScope`, `Store.ForScope`, `Sensor.Scope` are used consistently across Tasks 3,6,7,8,9,10.

---

## Follow-on plans (not in this plan)

1. **Generation** — `/create-core-sensors` skill (emit one core sensor per applicable angle + `environment` primitives, wire the intra-core DAG) and `/create-sensors` `depends_on` wiring resolved by angle (spec §7).
2. **Gather + migration + dogfood** — the two `validate-use-case` gather sites (`skills/validate-use-case/scripts/main.go`, `cmd/harness/usecase_runner.go`) switch to the use-case + core `depends_on`-closure gather (spec §8); physically migrate the two on-disk flat dogfood sensors into per-use-case folders with a half-migrated-state loader test (spec §11 Q4); run the dogfood chain (spec §10).

After this Foundation plan lands and its tests are green, ask whether to write plan (1).
