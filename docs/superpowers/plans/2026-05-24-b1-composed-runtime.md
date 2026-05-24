# B1 — Composed Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement B1 — the fixture binder, per-use-case aggregator, and `EffectivePolicy.InferentialFloor` extension — per [the B1 spec](../specs/2026-05-24-b1-composed-runtime-design.md).

**Architecture:** Two new leaf packages under `internal/runtime/` (`fixturebinder` and `aggregator/usecase`), plus one new field on `policy.EffectivePolicy`. No existing Phase A package imports them. Each function is pure (deterministic, no hidden state) except for `fixturebinder.Bind`'s filesystem writes.

**Tech Stack:** Go 1.22+, `sigs.k8s.io/yaml`, `github.com/santhosh-tekuri/jsonschema/v6` (already in go.mod). No new third-party dependencies.

**Spec reference shorthand:** §N below refers to sections of [`2026-05-24-b1-composed-runtime-design.md`](../specs/2026-05-24-b1-composed-runtime-design.md).

**Branching:** Work on `feat/b1-composed-runtime` branched from `origin/main` (per §14).

---

## File map

**Modify (Phase 1 — Policy extension):**
- `schemas/validation-policy.yaml` — add `inferential_floor` property.
- `internal/policy/types.go` — add `InferentialFloor *float64` to `ValidationPolicy`, add `InferentialFloor float64` to `EffectivePolicy`, add `DefaultInferentialFloor` constant.
- `internal/policy/resolve.go` — populate `EffectivePolicy.InferentialFloor` (local wins; default 0.7).
- `internal/policy/serialize.go` — emit `inferential_floor` in `MarshalYAML`.
- `internal/policy/loader_test.go` — add range + nullable cases.
- `internal/policy/resolve_test.go` — add floor resolution cases.
- `internal/policy/serialize_test.go` — add floor serialization case.
- `schemas/examples/validation-policy/local.yaml` — add a `inferential_floor` example.

**Create (Phase 2 — Fixture binder):**
- `internal/runtime/fixturebinder/types.go` — `StepBinding`, `BindError`.
- `internal/runtime/fixturebinder/extensions.go` — content-type → extension.
- `internal/runtime/fixturebinder/normalize.go` — fixture-id → env-var name.
- `internal/runtime/fixturebinder/binder.go` — `Binder` struct + `Bind` method.
- `internal/runtime/fixturebinder/binder_test.go` — tests.

**Create (Phase 3 — Per-use-case aggregator):**
- `internal/runtime/aggregator/usecase/types.go` — `UseCaseVerdict`, `AngleHint`.
- `internal/runtime/aggregator/usecase/aggregator.go` — `UseCase` function.
- `internal/runtime/aggregator/usecase/aggregator_test.go` — tests.
- `internal/runtime/aggregator/usecase/testdata/golden_verdict.json` — golden fixture.

---

# Phase 1 — Policy extension

## Task 1: Add `inferential_floor` to schema + Go types

**Files:**
- Modify: `schemas/validation-policy.yaml`
- Modify: `internal/policy/types.go`

- [ ] **Step 1: Write the failing test**

Edit `internal/policy/types_test.go` and add:

```go
func TestDefaultInferentialFloorIsSeventy(t *testing.T) {
	if DefaultInferentialFloor != 0.7 {
		t.Errorf("DefaultInferentialFloor = %v, want 0.7", DefaultInferentialFloor)
	}
}

func TestValidationPolicyInferentialFloorIsNullable(t *testing.T) {
	var p ValidationPolicy
	if p.InferentialFloor != nil {
		t.Errorf("zero ValidationPolicy.InferentialFloor = %v, want nil", *p.InferentialFloor)
	}
	v := 0.85
	p.InferentialFloor = &v
	if p.InferentialFloor == nil || *p.InferentialFloor != 0.85 {
		t.Errorf("ValidationPolicy.InferentialFloor round-trip broken")
	}
}

func TestEffectivePolicyHasFloorField(t *testing.T) {
	e := EffectivePolicy{InferentialFloor: 0.42}
	if e.InferentialFloor != 0.42 {
		t.Errorf("EffectivePolicy.InferentialFloor = %v, want 0.42", e.InferentialFloor)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/policy/ -run 'InferentialFloor|HasFloorField|DefaultInferentialFloor' -v`
Expected: FAIL with "undeclared name: DefaultInferentialFloor" and "unknown field InferentialFloor".

- [ ] **Step 3: Add fields + constant to `internal/policy/types.go`**

In the `const` block near `SupportedSchemaVersion`, add:

```go
// DefaultInferentialFloor is applied by Resolve when no scope sets
// inferential_floor. Plan §10.3 default.
const DefaultInferentialFloor = 0.7
```

In `type ValidationPolicy struct { ... }`, add the new field after `PerArchetype`:

```go
// InferentialFloor is the minimum confidence below which an inferential
// sensor's verdict is treated as inconclusive by the use-case aggregator.
// Nullable so callers can distinguish "field omitted" (nil) from "explicitly
// 0.0" (non-nil pointer to 0.0). Resolve fills in DefaultInferentialFloor
// only when every source scope leaves it nil.
InferentialFloor *float64 `json:"inferential_floor,omitempty" yaml:"inferential_floor,omitempty"`
```

In `type EffectivePolicy struct { ... }`, add the new field after `PerArchetype`:

```go
// InferentialFloor is the resolved floor — always populated post-Resolve.
InferentialFloor float64 `json:"inferential_floor" yaml:"inferential_floor"`
```

- [ ] **Step 4: Update the schema at `schemas/validation-policy.yaml`**

Inside the `properties:` block (after `per_archetype:`), add:

```yaml
  inferential_floor:
    type: number
    minimum: 0.0
    maximum: 1.0
    description: |
      Minimum confidence below which an inferential sensor's verdict is
      treated as inconclusive when computing the use-case verdict.
      Defaults to 0.7 when omitted at every scope. Computational sensors
      are unaffected.
```

The field is **optional** — do NOT add it to the `required:` list at the top of the schema.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/policy/ -run 'InferentialFloor|HasFloorField|DefaultInferentialFloor' -v`
Expected: PASS for all three.

- [ ] **Step 6: Run the full policy package to make sure nothing else broke**

Run: `go test ./internal/policy/...`
Expected: PASS (existing tests unchanged because the new field is optional and nullable).

- [ ] **Step 7: Commit**

```bash
git add schemas/validation-policy.yaml internal/policy/types.go internal/policy/types_test.go
git commit -m "feat(policy): add nullable InferentialFloor field to ValidationPolicy and EffectivePolicy

Source-level *float64 distinguishes 'omitted' from 'explicit 0.0';
EffectivePolicy carries the resolved float64 always populated post-Resolve.
DefaultInferentialFloor = 0.7 per plan §10.3."
```

---

## Task 2: Loader rejects out-of-range floor; accepts nil and valid values

**Files:**
- Create: `internal/policy/testdata/floor-out-of-range.yaml`
- Create: `internal/policy/testdata/floor-explicit.yaml`
- Modify: `internal/policy/loader_test.go`

- [ ] **Step 1: Write failing tests in `internal/policy/loader_test.go`**

Append:

```go
func TestLoad_AcceptsExplicitFloor(t *testing.T) {
	p, err := loadValid(t, "floor-explicit.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.InferentialFloor == nil {
		t.Fatal("InferentialFloor should be non-nil when YAML sets it")
	}
	if *p.InferentialFloor != 0.85 {
		t.Errorf("*InferentialFloor = %v, want 0.85", *p.InferentialFloor)
	}
}

func TestLoad_OmittedFloorRemainsNil(t *testing.T) {
	p := loadExample(t, "global.yaml")
	if p.InferentialFloor != nil {
		t.Errorf("global.yaml has no inferential_floor; InferentialFloor = %v, want nil", *p.InferentialFloor)
	}
}

func TestLoad_RejectsOutOfRangeFloor(t *testing.T) {
	err := loadTestdata(t, "floor-out-of-range.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := strings.ToLower(err.Error())
	for _, want := range []string{"inferential_floor", "maximum"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}
```

Also add a helper next to `loadTestdata`:

```go
func loadValid(t *testing.T, name string) (*ValidationPolicy, error) {
	t.Helper()
	path := filepath.Join("testdata", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	return Load(f)
}
```

- [ ] **Step 2: Create `internal/policy/testdata/floor-explicit.yaml`**

```yaml
schema_version: "1.0.0"
scope: local
inferential_floor: 0.85
per_archetype:
  http-api:
    obligatory_angles: [build]
    optional_angles: []
    disabled_angles: []
```

- [ ] **Step 3: Create `internal/policy/testdata/floor-out-of-range.yaml`**

```yaml
schema_version: "1.0.0"
scope: local
inferential_floor: 1.5
per_archetype:
  http-api:
    obligatory_angles: [build]
    optional_angles: []
    disabled_angles: []
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/policy/ -run 'Floor' -v`
Expected: PASS for all three.

- [ ] **Step 5: Run the full policy package**

Run: `go test ./internal/policy/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/policy/loader_test.go internal/policy/testdata/floor-explicit.yaml internal/policy/testdata/floor-out-of-range.yaml
git commit -m "test(policy): loader accepts explicit and omitted inferential_floor; rejects out-of-range"
```

---

## Task 3: `Resolve` populates `EffectivePolicy.InferentialFloor` (local wins, default 0.7)

**Files:**
- Modify: `internal/policy/resolve.go`
- Modify: `internal/policy/resolve_test.go`

- [ ] **Step 1: Write failing tests in `internal/policy/resolve_test.go`**

Append:

```go
func floatPtr(v float64) *float64 { return &v }

func TestResolve_FloorBothNilUsesDefault(t *testing.T) {
	g := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeGlobal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}}
	l := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeLocal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}}
	eff := Resolve(g, l)
	if eff.InferentialFloor != DefaultInferentialFloor {
		t.Errorf("InferentialFloor = %v, want %v", eff.InferentialFloor, DefaultInferentialFloor)
	}
}

func TestResolve_FloorGlobalSet_LocalNil(t *testing.T) {
	g := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeGlobal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}, InferentialFloor: floatPtr(0.5)}
	l := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeLocal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}}
	eff := Resolve(g, l)
	if eff.InferentialFloor != 0.5 {
		t.Errorf("InferentialFloor = %v, want 0.5", eff.InferentialFloor)
	}
}

func TestResolve_FloorLocalSet_GlobalNil(t *testing.T) {
	g := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeGlobal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}}
	l := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeLocal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}, InferentialFloor: floatPtr(0.9)}
	eff := Resolve(g, l)
	if eff.InferentialFloor != 0.9 {
		t.Errorf("InferentialFloor = %v, want 0.9", eff.InferentialFloor)
	}
}

func TestResolve_FloorBothSet_LocalWins(t *testing.T) {
	g := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeGlobal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}, InferentialFloor: floatPtr(0.5)}
	l := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeLocal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}, InferentialFloor: floatPtr(0.9)}
	eff := Resolve(g, l)
	if eff.InferentialFloor != 0.9 {
		t.Errorf("InferentialFloor = %v, want 0.9 (local wins)", eff.InferentialFloor)
	}
}

func TestResolve_NilNil_FloorIsDefault(t *testing.T) {
	eff := Resolve(nil, nil)
	if eff.InferentialFloor != DefaultInferentialFloor {
		t.Errorf("Resolve(nil,nil).InferentialFloor = %v, want %v", eff.InferentialFloor, DefaultInferentialFloor)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/policy/ -run 'Floor' -v`
Expected: FAIL (all `Resolve_Floor*` tests + `NilNil_FloorIsDefault`) because `Resolve` doesn't populate the field yet.

- [ ] **Step 3: Update `internal/policy/resolve.go`**

After the `eff := &EffectivePolicy{ ... }` block, before the archetypes loop, add:

```go
	eff.InferentialFloor = resolveFloor(sources)
```

And at the bottom of the file, add the helper:

```go
// resolveFloor walks sources in order (global, local) and returns the last
// non-nil InferentialFloor encountered. Falls back to DefaultInferentialFloor
// when every source leaves the field nil.
func resolveFloor(sources []policySource) float64 {
	out := DefaultInferentialFloor
	for _, s := range sources {
		if s.pol.InferentialFloor != nil {
			out = *s.pol.InferentialFloor
		}
	}
	return out
}
```

Then update the `Resolve(nil, nil)` early-return branch in `Resolve` (the `eff := &EffectivePolicy{ ... }` initializer) so that when `sources` is empty, `InferentialFloor` is still `DefaultInferentialFloor`. Replace the initializer:

```go
	eff := &EffectivePolicy{
		SchemaVersion:    SupportedSchemaVersion,
		ResolvedFrom:     resolvedFrom,
		PerArchetype:     map[enums.Archetype]map[enums.ValidationAngle]AngleStatus{},
		InferentialFloor: resolveFloor(sources),
	}
```

(Drop the standalone `eff.InferentialFloor = resolveFloor(sources)` line — fold it into the struct literal.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/policy/ -run 'Floor|NilNil_Floor' -v`
Expected: PASS.

- [ ] **Step 5: Run the full policy package**

Run: `go test ./internal/policy/...`
Expected: PASS (existing tests unaffected; only `TestResolve_BothNil` would now have `InferentialFloor == 0.7`, but that test doesn't assert on it).

- [ ] **Step 6: Commit**

```bash
git add internal/policy/resolve.go internal/policy/resolve_test.go
git commit -m "feat(policy): Resolve populates InferentialFloor (local wins, default 0.7)"
```

---

## Task 4: `MarshalYAML` emits `inferential_floor` for audit dumps

**Files:**
- Modify: `internal/policy/serialize.go`
- Modify: `internal/policy/serialize_test.go`

- [ ] **Step 1: Write the failing test in `internal/policy/serialize_test.go`**

Append:

```go
func TestMarshalYAML_EmitsInferentialFloor(t *testing.T) {
	eff := &EffectivePolicy{
		SchemaVersion:    SupportedSchemaVersion,
		ResolvedFrom:     []string{"global"},
		PerArchetype:     map[enums.Archetype]map[enums.ValidationAngle]AngleStatus{},
		InferentialFloor: 0.65,
	}
	raw, err := eff.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "inferential_floor: 0.65") {
		t.Errorf("output missing 'inferential_floor: 0.65':\n%s", got)
	}
}

func TestMarshalYAML_FloorDeterministic(t *testing.T) {
	eff := &EffectivePolicy{
		SchemaVersion:    SupportedSchemaVersion,
		ResolvedFrom:     []string{"global", "local"},
		PerArchetype:     map[enums.Archetype]map[enums.ValidationAngle]AngleStatus{},
		InferentialFloor: 0.7,
	}
	first, err := eff.MarshalYAML()
	if err != nil {
		t.Fatalf("first MarshalYAML: %v", err)
	}
	for i := 0; i < 10; i++ {
		again, err := eff.MarshalYAML()
		if err != nil {
			t.Fatalf("iteration %d MarshalYAML: %v", i, err)
		}
		if !bytes.Equal(first, again) {
			t.Fatalf("MarshalYAML output drifted between calls\nfirst:\n%s\niter %d:\n%s", string(first), i, string(again))
		}
	}
}
```

Add the import `"bytes"` and `"strings"` to the test file if not already present.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/policy/ -run 'MarshalYAML_(Emits|FloorDeterministic)' -v`
Expected: FAIL (output does not contain `inferential_floor`).

- [ ] **Step 3: Update `internal/policy/serialize.go`**

Inside `MarshalYAML`, update the `type docOut struct { ... }` to include the floor:

```go
		type docOut struct {
			SchemaVersion    string              `json:"schema_version"    yaml:"schema_version"`
			ResolvedFrom     []string            `json:"resolved_from"     yaml:"resolved_from"`
			InferentialFloor float64             `json:"inferential_floor" yaml:"inferential_floor"`
			PerArchetype     map[string]blockOut `json:"per_archetype"     yaml:"per_archetype"`
		}
```

And update the `doc := docOut{...}` initializer:

```go
		doc := docOut{
			SchemaVersion:    p.SchemaVersion,
			ResolvedFrom:     append([]string{}, p.ResolvedFrom...),
			InferentialFloor: p.InferentialFloor,
			PerArchetype:     map[string]blockOut{},
		}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/policy/ -run 'MarshalYAML_(Emits|FloorDeterministic)' -v`
Expected: PASS.

- [ ] **Step 5: Run the full policy package**

Run: `go test ./internal/policy/...`
Expected: PASS (existing `MarshalYAML` tests may need updates if they assert exact bytes — check and fix if needed).

If `TestMarshalYAML_*` golden tests fail because of the new field, update the golden expectations to include `inferential_floor: 0.7` (default) in the right position.

- [ ] **Step 6: Commit**

```bash
git add internal/policy/serialize.go internal/policy/serialize_test.go
git commit -m "feat(policy): MarshalYAML emits inferential_floor in audit dumps"
```

---

# Phase 2 — Fixture binder

## Task 5: Package scaffold with `StepBinding` and `BindError` types + empty-uses behavior

**Files:**
- Create: `internal/runtime/fixturebinder/types.go`
- Create: `internal/runtime/fixturebinder/binder.go`
- Create: `internal/runtime/fixturebinder/binder_test.go`

- [ ] **Step 1: Write failing tests in `internal/runtime/fixturebinder/binder_test.go`**

```go
package fixturebinder

import (
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)

func TestBindError_ImplementsError(t *testing.T) {
	e := &BindError{Code: "fixture-not-found", FixtureID: "login", UseCaseID: "uc-login"}
	msg := e.Error()
	for _, want := range []string{"fixturebinder", "fixture-not-found", "login", "uc-login"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, missing %q", msg, want)
		}
	}
}

func TestBindError_UnwrapReturnsCause(t *testing.T) {
	cause := &errStub{msg: "disk full"}
	e := &BindError{Code: "write-failed", Cause: cause}
	if e.Unwrap() != cause {
		t.Errorf("Unwrap() = %v, want %v", e.Unwrap(), cause)
	}
}

func TestBind_EmptyUsesReturnsEmptyBinding(t *testing.T) {
	b := &Binder{ScratchDir: t.TempDir()}
	step := sensor.Step{ID: "step-1", Run: "true", Uses: nil}
	uc := &usecase.UseCase{ID: "uc-login", FixtureIDs: []string{"login-basic"}}
	// Empty step.Uses never queries the store; nil is safe.
	binding, err := b.Bind(step, uc, nil)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if binding.Env == nil || len(binding.Env) != 0 {
		t.Errorf("Env = %v, want empty non-nil map", binding.Env)
	}
	if binding.Files == nil || len(binding.Files) != 0 {
		t.Errorf("Files = %v, want empty non-nil map", binding.Files)
	}
	if binding.BoundIDs == nil || len(binding.BoundIDs) != 0 {
		t.Errorf("BoundIDs = %v, want empty non-nil slice", binding.BoundIDs)
	}
}

type errStub struct{ msg string }

func (e *errStub) Error() string { return e.msg }
```

(Empty `step.Uses` never reaches the store lookup. Passing `nil` keeps the scaffold test independent of the `fixture.Store` constructor's exact signature.)

- [ ] **Step 2: Run tests to verify they fail with a compile error**

Run: `go test ./internal/runtime/fixturebinder/...`
Expected: FAIL with "no Go files" or "undefined: Binder, BindError, StepBinding".

- [ ] **Step 3: Create `internal/runtime/fixturebinder/types.go`**

```go
// Package fixturebinder writes a sensor step's fixture payloads to disk
// and returns the environment variables a step command needs to read them.
// See docs/superpowers/specs/2026-05-24-b1-composed-runtime-design.md §5.
package fixturebinder

// StepBinding is the resolved per-step view a sensor step's executor consumes.
type StepBinding struct {
	// Env maps HARNESS_FIXTURE_<NORMALIZED_ID> -> absolute file path.
	Env map[string]string
	// Files maps fixture id -> absolute file path. For diagnostics/tests.
	Files map[string]string
	// BoundIDs is the canonical-ordered (ascending) list of bound fixture ids.
	BoundIDs []string
}

// BindError reports a failure during fixture binding.
type BindError struct {
	Code      string // "fixture-not-found" | "fixture-not-owned" | "write-failed"
	FixtureID string
	UseCaseID string
	Cause     error // non-nil only for "write-failed"
}

func (e *BindError) Error() string {
	if e.Cause != nil {
		return "fixturebinder: " + e.Code + ": fixture=" + e.FixtureID + " use_case=" + e.UseCaseID + ": " + e.Cause.Error()
	}
	return "fixturebinder: " + e.Code + ": fixture=" + e.FixtureID + " use_case=" + e.UseCaseID
}

func (e *BindError) Unwrap() error { return e.Cause }
```

- [ ] **Step 4: Create `internal/runtime/fixturebinder/binder.go`**

```go
package fixturebinder

import (
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// Binder writes fixture payloads to disk under ScratchDir. The caller owns
// ScratchDir's lifecycle (mkdir + cleanup); the binder neither creates the
// directory nor removes files when Bind returns.
type Binder struct {
	// ScratchDir is the absolute path under which payload files are written.
	// Must exist when Bind is called.
	ScratchDir string
}

// Bind resolves a step's `uses:` fixture ids against the use case's owned
// fixtures, writes each payload to ScratchDir, and returns a StepBinding.
// See spec §5 for the full behavior contract.
func (b *Binder) Bind(step sensor.Step, owningUseCase *usecase.UseCase, store fixture.FixtureStore) (StepBinding, error) {
	binding := StepBinding{
		Env:      map[string]string{},
		Files:    map[string]string{},
		BoundIDs: []string{},
	}
	if len(step.Uses) == 0 {
		return binding, nil
	}
	// Real binding logic implemented in subsequent tasks.
	return binding, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/runtime/fixturebinder/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/runtime/fixturebinder/
git commit -m "feat(runtime): scaffold fixturebinder package with StepBinding and BindError types"
```

---

## Task 6: Content-type → extension mapping

**Files:**
- Create: `internal/runtime/fixturebinder/extensions.go`
- Create: `internal/runtime/fixturebinder/extensions_test.go`

- [ ] **Step 1: Write failing tests in `internal/runtime/fixturebinder/extensions_test.go`**

```go
package fixturebinder

import "testing"

func TestExtensionFor(t *testing.T) {
	cases := []struct {
		contentType string
		want        string
	}{
		{"application/json", ".json"},
		{"application/vnd.api+json", ".json"},
		{"application/yaml", ".yaml"},
		{"text/yaml", ".yaml"},
		{"application/x-yaml", ".yaml"},
		{"application/xml", ".xml"},
		{"text/xml", ".xml"},
		{"application/atom+xml", ".xml"},
		{"application/octet-stream", ".bin"},
		{"image/png", ".bin"},
		{"", ".bin"},
		{"text/plain", ".bin"},
	}
	for _, c := range cases {
		got := extensionFor(c.contentType)
		if got != c.want {
			t.Errorf("extensionFor(%q) = %q, want %q", c.contentType, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/runtime/fixturebinder/ -run TestExtensionFor -v`
Expected: FAIL with "undefined: extensionFor".

- [ ] **Step 3: Create `internal/runtime/fixturebinder/extensions.go`**

```go
package fixturebinder

import "strings"

// extensionFor maps a fixture content_type to the file extension used when
// writing its payload under ScratchDir. Mirrors fixture.payload.go's
// structured-content detection.
func extensionFor(contentType string) string {
	switch contentType {
	case "application/json":
		return ".json"
	case "application/yaml", "text/yaml", "application/x-yaml":
		return ".yaml"
	case "application/xml", "text/xml":
		return ".xml"
	}
	if strings.HasSuffix(contentType, "+json") {
		return ".json"
	}
	if strings.HasSuffix(contentType, "+xml") {
		return ".xml"
	}
	return ".bin"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runtime/fixturebinder/ -run TestExtensionFor -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/fixturebinder/extensions.go internal/runtime/fixturebinder/extensions_test.go
git commit -m "feat(runtime/fixturebinder): map content_type to file extension"
```

---

## Task 7: Fixture-id → env-var name normalization

**Files:**
- Create: `internal/runtime/fixturebinder/normalize.go`
- Create: `internal/runtime/fixturebinder/normalize_test.go`

- [ ] **Step 1: Write failing tests in `internal/runtime/fixturebinder/normalize_test.go`**

```go
package fixturebinder

import "testing"

func TestNormalizeEnvName(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"login", "HARNESS_FIXTURE_LOGIN"},
		{"login-basic", "HARNESS_FIXTURE_LOGIN_BASIC"},
		{"a", "HARNESS_FIXTURE_A"},
		{"x1-y2-z3", "HARNESS_FIXTURE_X1_Y2_Z3"},
		{"abc-def-ghi", "HARNESS_FIXTURE_ABC_DEF_GHI"},
	}
	for _, c := range cases {
		got := normalizeEnvName(c.id)
		if got != c.want {
			t.Errorf("normalizeEnvName(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/runtime/fixturebinder/ -run TestNormalizeEnvName -v`
Expected: FAIL with "undefined: normalizeEnvName".

- [ ] **Step 3: Create `internal/runtime/fixturebinder/normalize.go`**

```go
package fixturebinder

import "strings"

// normalizeEnvName converts a fixture id (regex ^[a-z][a-z0-9-]*$, enforced
// by the E5 schema) to the POSIX env-var name HARNESS_FIXTURE_<UPPER_UNDER>.
// Uppercase + hyphens-to-underscores; the input regex guarantees no other
// transformation is needed.
func normalizeEnvName(fixtureID string) string {
	return "HARNESS_FIXTURE_" + strings.ToUpper(strings.ReplaceAll(fixtureID, "-", "_"))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runtime/fixturebinder/ -run TestNormalizeEnvName -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/fixturebinder/normalize.go internal/runtime/fixturebinder/normalize_test.go
git commit -m "feat(runtime/fixturebinder): normalize fixture id to env-var name"
```

---

## Task 8: `Binder.Bind` happy path — writes payloads, sets env vars, deterministic BoundIDs

**Files:**
- Modify: `internal/runtime/fixturebinder/binder.go`
- Modify: `internal/runtime/fixturebinder/binder_test.go`

- [ ] **Step 1: Write failing tests in `internal/runtime/fixturebinder/binder_test.go`**

Append the following helpers and tests:

```go
func makeFixture(t *testing.T, id, useCaseID, contentType string, payload []byte) fixture.Fixture {
	t.Helper()
	return fixture.Fixture{
		SchemaVersion: "1.0.0",
		ID:            id,
		UseCaseID:     useCaseID,
		Role:          fixture.RoleInput,
		ContentType:   contentType,
		Payload:       payload,
	}
}

type stubStore struct {
	byID map[string]fixture.Fixture
}

func (s *stubStore) LookupFixture(id string) (fixture.Fixture, bool) {
	f, ok := s.byID[id]
	return f, ok
}
func (s *stubStore) FixturesForUseCase(string) []fixture.Fixture { return nil }
func (s *stubStore) All() []fixture.Fixture                       { return nil }

func newStubStore(t *testing.T, fs ...fixture.Fixture) *stubStore {
	t.Helper()
	m := make(map[string]fixture.Fixture, len(fs))
	for _, f := range fs {
		m[f.ID] = f
	}
	return &stubStore{byID: m}
}

func TestBind_HappyPath_JSONAndBinary(t *testing.T) {
	scratch := t.TempDir()
	b := &Binder{ScratchDir: scratch}

	jsonFix := makeFixture(t, "login-basic", "uc-login", "application/json", []byte(`{"user":"alice"}`))
	binFix := makeFixture(t, "avatar-png", "uc-login", "image/png", []byte{0x89, 0x50, 0x4E, 0x47})

	store := newStubStore(t, jsonFix, binFix)
	uc := &usecase.UseCase{ID: "uc-login", FixtureIDs: []string{"login-basic", "avatar-png"}}
	step := sensor.Step{ID: "s1", Run: "true", Uses: []string{"avatar-png", "login-basic"}} // intentionally reversed

	binding, err := b.Bind(step, uc, store)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	// BoundIDs sorted ascending.
	wantBound := []string{"avatar-png", "login-basic"}
	if !reflect.DeepEqual(binding.BoundIDs, wantBound) {
		t.Errorf("BoundIDs = %v, want %v", binding.BoundIDs, wantBound)
	}

	// Env names canonical.
	wantEnv := map[string]string{
		"HARNESS_FIXTURE_LOGIN_BASIC": filepath.Join(scratch, "login-basic.json"),
		"HARNESS_FIXTURE_AVATAR_PNG":  filepath.Join(scratch, "avatar-png.bin"),
	}
	if !reflect.DeepEqual(binding.Env, wantEnv) {
		t.Errorf("Env = %v, want %v", binding.Env, wantEnv)
	}

	// Files map.
	wantFiles := map[string]string{
		"login-basic": filepath.Join(scratch, "login-basic.json"),
		"avatar-png":  filepath.Join(scratch, "avatar-png.bin"),
	}
	if !reflect.DeepEqual(binding.Files, wantFiles) {
		t.Errorf("Files = %v, want %v", binding.Files, wantFiles)
	}

	// File contents byte-equal to payloads.
	got, err := os.ReadFile(binding.Files["login-basic"])
	if err != nil {
		t.Fatalf("read login-basic: %v", err)
	}
	if !bytes.Equal(got, jsonFix.Payload) {
		t.Errorf("login-basic file contents = %q, want %q", string(got), string(jsonFix.Payload))
	}

	got, err = os.ReadFile(binding.Files["avatar-png"])
	if err != nil {
		t.Fatalf("read avatar-png: %v", err)
	}
	if !bytes.Equal(got, binFix.Payload) {
		t.Errorf("avatar-png file contents = %v, want %v", got, binFix.Payload)
	}
}

func TestBind_BoundIDsDeterministicAcrossCalls(t *testing.T) {
	b := &Binder{ScratchDir: t.TempDir()}
	fxA := makeFixture(t, "alpha", "uc", "application/json", []byte(`{}`))
	fxB := makeFixture(t, "bravo", "uc", "application/json", []byte(`{}`))
	fxC := makeFixture(t, "charlie", "uc", "application/json", []byte(`{}`))
	store := newStubStore(t, fxA, fxB, fxC)
	uc := &usecase.UseCase{ID: "uc", FixtureIDs: []string{"alpha", "bravo", "charlie"}}
	step := sensor.Step{Uses: []string{"charlie", "alpha", "bravo"}}

	first, err := b.Bind(step, uc, store)
	if err != nil {
		t.Fatalf("Bind first: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	if !reflect.DeepEqual(first.BoundIDs, want) {
		t.Errorf("first BoundIDs = %v, want %v", first.BoundIDs, want)
	}
	for i := 0; i < 5; i++ {
		again, err := b.Bind(step, uc, store)
		if err != nil {
			t.Fatalf("Bind iter %d: %v", i, err)
		}
		if !reflect.DeepEqual(again.BoundIDs, want) {
			t.Errorf("iter %d BoundIDs = %v, want %v", i, again.BoundIDs, want)
		}
	}
}
```

Add the required imports at the top of the test file:

```go
import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)
```

(Remove the existing `fixture.NewStore()` call from `TestBind_EmptyUsesReturnsEmptyBinding` — replace with `newStubStore(t)`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/runtime/fixturebinder/ -run 'TestBind_HappyPath|TestBind_BoundIDsDeterministic' -v`
Expected: FAIL — binding has empty maps because the implementation in Task 5 is a no-op for non-empty `Uses`.

- [ ] **Step 3: Implement `Bind` happy path in `internal/runtime/fixturebinder/binder.go`**

Replace the existing `Bind` function:

```go
package fixturebinder

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// Binder writes fixture payloads to disk under ScratchDir. The caller owns
// ScratchDir's lifecycle (mkdir + cleanup); the binder neither creates the
// directory nor removes files when Bind returns.
type Binder struct {
	ScratchDir string
}

// Bind resolves a step's `uses:` fixture ids against the use case's owned
// fixtures, writes each payload to ScratchDir, and returns a StepBinding.
// See spec §5 for the full behavior contract.
func (b *Binder) Bind(step sensor.Step, owningUseCase *usecase.UseCase, store fixture.FixtureStore) (StepBinding, error) {
	binding := StepBinding{
		Env:      map[string]string{},
		Files:    map[string]string{},
		BoundIDs: []string{},
	}
	if len(step.Uses) == 0 {
		return binding, nil
	}

	owned := make(map[string]struct{}, len(owningUseCase.FixtureIDs))
	for _, id := range owningUseCase.FixtureIDs {
		owned[id] = struct{}{}
	}

	// Sort step.Uses to get deterministic BoundIDs and file-write order.
	ids := append([]string(nil), step.Uses...)
	sort.Strings(ids)

	for _, id := range ids {
		if _, ok := owned[id]; !ok {
			return StepBinding{}, &BindError{
				Code: "fixture-not-owned", FixtureID: id, UseCaseID: owningUseCase.ID,
			}
		}
		fx, ok := store.LookupFixture(id)
		if !ok {
			return StepBinding{}, &BindError{
				Code: "fixture-not-found", FixtureID: id, UseCaseID: owningUseCase.ID,
			}
		}
		path := filepath.Join(b.ScratchDir, fx.ID+extensionFor(fx.ContentType))
		if err := os.WriteFile(path, fx.Payload, 0o600); err != nil {
			return StepBinding{}, &BindError{
				Code: "write-failed", FixtureID: id, UseCaseID: owningUseCase.ID, Cause: err,
			}
		}
		binding.Env[normalizeEnvName(fx.ID)] = path
		binding.Files[fx.ID] = path
		binding.BoundIDs = append(binding.BoundIDs, fx.ID)
	}
	return binding, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runtime/fixturebinder/ -run 'TestBind_HappyPath|TestBind_BoundIDsDeterministic|TestBind_EmptyUses' -v`
Expected: PASS for all three.

- [ ] **Step 5: Run the full fixturebinder package**

Run: `go test ./internal/runtime/fixturebinder/...`
Expected: PASS for all (4 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/runtime/fixturebinder/
git commit -m "feat(runtime/fixturebinder): Bind writes payloads, sets env vars, sorts BoundIDs"
```

---

## Task 9: `Bind` error cases — not-found, not-owned, write-failed

**Files:**
- Modify: `internal/runtime/fixturebinder/binder_test.go`

- [ ] **Step 1: Write failing tests in `internal/runtime/fixturebinder/binder_test.go`**

Append:

```go
func TestBind_FixtureNotFound(t *testing.T) {
	b := &Binder{ScratchDir: t.TempDir()}
	uc := &usecase.UseCase{ID: "uc-login", FixtureIDs: []string{"missing"}}
	step := sensor.Step{Uses: []string{"missing"}}
	store := newStubStore(t)

	_, err := b.Bind(step, uc, store)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	be, ok := err.(*BindError)
	if !ok {
		t.Fatalf("err = %T, want *BindError", err)
	}
	if be.Code != "fixture-not-found" {
		t.Errorf("Code = %q, want fixture-not-found", be.Code)
	}
	if be.FixtureID != "missing" {
		t.Errorf("FixtureID = %q, want missing", be.FixtureID)
	}
}

func TestBind_FixtureNotOwned(t *testing.T) {
	b := &Binder{ScratchDir: t.TempDir()}
	uc := &usecase.UseCase{ID: "uc-login", FixtureIDs: []string{"login-basic"}}
	step := sensor.Step{Uses: []string{"foreign-fixture"}}
	foreignFx := makeFixture(t, "foreign-fixture", "uc-other", "application/json", []byte(`{}`))
	store := newStubStore(t, foreignFx)

	_, err := b.Bind(step, uc, store)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	be, ok := err.(*BindError)
	if !ok {
		t.Fatalf("err = %T, want *BindError", err)
	}
	if be.Code != "fixture-not-owned" {
		t.Errorf("Code = %q, want fixture-not-owned", be.Code)
	}
	if be.FixtureID != "foreign-fixture" {
		t.Errorf("FixtureID = %q, want foreign-fixture", be.FixtureID)
	}
	if be.UseCaseID != "uc-login" {
		t.Errorf("UseCaseID = %q, want uc-login", be.UseCaseID)
	}
}

func TestBind_WriteFailed(t *testing.T) {
	// Point ScratchDir at a nonexistent + un-creatable path so os.WriteFile fails.
	b := &Binder{ScratchDir: "/dev/null/cannot-write-here"}
	fx := makeFixture(t, "x", "uc", "application/json", []byte(`{}`))
	uc := &usecase.UseCase{ID: "uc", FixtureIDs: []string{"x"}}
	step := sensor.Step{Uses: []string{"x"}}
	store := newStubStore(t, fx)

	_, err := b.Bind(step, uc, store)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	be, ok := err.(*BindError)
	if !ok {
		t.Fatalf("err = %T, want *BindError", err)
	}
	if be.Code != "write-failed" {
		t.Errorf("Code = %q, want write-failed", be.Code)
	}
	if be.Cause == nil {
		t.Error("Cause = nil, want underlying error")
	}
	if !strings.Contains(be.Error(), "write-failed") {
		t.Errorf("Error() = %q, missing 'write-failed'", be.Error())
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

The implementation from Task 8 already returns the right errors. Run:

Run: `go test ./internal/runtime/fixturebinder/ -run 'TestBind_(FixtureNotFound|FixtureNotOwned|WriteFailed)' -v`
Expected: PASS for all three.

If `TestBind_WriteFailed` flakes on a non-Linux platform where `/dev/null/...` resolves differently, swap to a path the test can guarantee is unwritable, e.g. create a file and use it as the parent:

```go
scratch := t.TempDir()
parent := filepath.Join(scratch, "not-a-dir")
if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
	t.Fatalf("setup: %v", err)
}
b := &Binder{ScratchDir: parent}
```

(WriteFile under a path whose parent is a regular file fails on every Unix and on Windows.)

- [ ] **Step 3: Run the full fixturebinder package once more**

Run: `go test ./internal/runtime/fixturebinder/...`
Expected: PASS for all 7 tests.

- [ ] **Step 4: Commit**

```bash
git add internal/runtime/fixturebinder/binder_test.go
git commit -m "test(runtime/fixturebinder): cover fixture-not-found, fixture-not-owned, write-failed paths"
```

---

# Phase 3 — Per-use-case aggregator

## Task 10: Package scaffold with `UseCaseVerdict` and `AngleHint` types

**Files:**
- Create: `internal/runtime/aggregator/usecase/types.go`
- Create: `internal/runtime/aggregator/usecase/aggregator.go`
- Create: `internal/runtime/aggregator/usecase/aggregator_test.go`

- [ ] **Step 1: Write the failing test in `internal/runtime/aggregator/usecase/aggregator_test.go`**

```go
package aggregator

import (
	"testing"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
)

func TestAngleHint_HoldsAngleVerdictHint(t *testing.T) {
	h := AngleHint{
		Angle:   enums.AngleBuild,
		Verdict: enums.VerdictFail,
		Hint:    aggregate.HealHint{},
	}
	if h.Angle != enums.AngleBuild {
		t.Errorf("Angle = %q, want build", h.Angle)
	}
	if h.Verdict != enums.VerdictFail {
		t.Errorf("Verdict = %q, want fail", h.Verdict)
	}
}

func TestUseCaseVerdict_ZeroValueShape(t *testing.T) {
	var v UseCaseVerdict
	if v.UseCaseID != "" || v.Verdict != "" || v.Confidence != 0.0 ||
		v.ObligatorySatisfied != false ||
		v.EvaluatedAngles != nil || v.FailingAngles != nil ||
		v.WarningAngles != nil || v.HealHints != nil {
		t.Errorf("zero UseCaseVerdict has non-zero fields: %+v", v)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail with compile errors**

Run: `go test ./internal/runtime/aggregator/usecase/...`
Expected: FAIL with "no Go files" or "undefined: AngleHint, UseCaseVerdict".

- [ ] **Step 3: Create `internal/runtime/aggregator/usecase/types.go`**

```go
// Package aggregator computes the use-case verdict (plan §6.3) from the
// AggregateSignals emitted by every sensor that validated one use case.
// See docs/superpowers/specs/2026-05-24-b1-composed-runtime-design.md §6.
package aggregator

import (
	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
)

// AngleHint pairs a non-pass verdict with its angle and heal hint.
// Used to surface warn and fail signals from one use case in one slice,
// preserving locus precision (no consolidation).
type AngleHint struct {
	Angle   enums.ValidationAngle
	Verdict enums.Verdict // always either warn or fail
	Hint    aggregate.HealHint
}

// UseCaseVerdict is the terminal output of aggregator.UseCase.
type UseCaseVerdict struct {
	UseCaseID           string
	Archetype           enums.Archetype
	Verdict             enums.Verdict // pass | fail | inconclusive (warn lives at signal level only)
	Confidence          float64       // weighted average, [0.0, 1.0]
	ObligatorySatisfied bool          // true iff every obligatory effective verdict in {pass, warn}
	EvaluatedAngles     []enums.ValidationAngle
	FailingAngles       []enums.ValidationAngle // post-floor verdict == fail (canonical order)
	WarningAngles       []enums.ValidationAngle // post-floor verdict == warn (canonical order)
	HealHints           []AngleHint             // one per fail + warn, canonical order
}
```

- [ ] **Step 4: Create `internal/runtime/aggregator/usecase/aggregator.go`**

```go
package aggregator

import (
	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/policy"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// UseCase computes the verdict for one use case under one archetype context.
// Real implementation comes in subsequent tasks.
func UseCase(
	uc *usecase.UseCase,
	archetype enums.Archetype,
	signals []aggregate.AggregateSignal,
	sensors []sensor.Sensor,
	pol *policy.EffectivePolicy,
) (UseCaseVerdict, error) {
	return UseCaseVerdict{
		UseCaseID: uc.ID,
		Archetype: archetype,
		Verdict:   enums.VerdictInconclusive,
		EvaluatedAngles: []enums.ValidationAngle{},
		FailingAngles:   []enums.ValidationAngle{},
		WarningAngles:   []enums.ValidationAngle{},
		HealHints:       []AngleHint{},
	}, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/runtime/aggregator/usecase/...`
Expected: PASS (2 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/runtime/aggregator/usecase/
git commit -m "feat(runtime/aggregator): scaffold per-use-case aggregator package with UseCaseVerdict and AngleHint"
```

---

## Task 11: Input validation — archetype-not-in-scope, signal-foreign-use-case, duplicate-angle-signal, missing-obligatory-signal

**Files:**
- Modify: `internal/runtime/aggregator/usecase/aggregator.go`
- Modify: `internal/runtime/aggregator/usecase/aggregator_test.go`

- [ ] **Step 1: Write failing tests in `internal/runtime/aggregator/usecase/aggregator_test.go`**

Append:

```go
import (
	"strings"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/policy"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)
```

(Merge the new imports with the existing ones; remove duplicates.)

Then add these tests:

```go
func makeUseCase(id string, archetypes ...enums.Archetype) *usecase.UseCase {
	return &usecase.UseCase{ID: id, ArchetypeScope: archetypes}
}

func makeSignal(useCaseID, sensorID string, angle enums.ValidationAngle, verdict enums.Verdict, confidence float64) aggregate.AggregateSignal {
	sig := aggregate.AggregateSignal{
		SchemaVersion:     "1.0.0",
		Type:              aggregate.TypeAggregate,
		SensorID:          sensorID,
		UseCaseID:         useCaseID,
		Angle:             angle,
		StartedAt:         time.Unix(1, 0),
		EndedAt:           time.Unix(2, 0),
		TerminationReason: enums.TerminationCompleted,
		Verdict:           verdict,
		Confidence:        confidence,
	}
	if verdict == enums.VerdictFail || verdict == enums.VerdictWarn {
		sig.HealHint = &aggregate.HealHint{Summary: "synthetic"}
	}
	return sig
}

func makeSensor(id string, useCaseID string, angle enums.ValidationAngle, nature enums.SensorNature) sensor.Sensor {
	return sensor.Sensor{
		SchemaVersion: "1.0.0",
		ID:            id, UseCaseID: useCaseID, Angle: angle,
		Kind: enums.KindAssertion, Nature: nature, OutputType: enums.OutputSingleShot,
	}
}

func makeEffectivePolicy(arch enums.Archetype, obligatory []enums.ValidationAngle, optional []enums.ValidationAngle, floor float64) *policy.EffectivePolicy {
	statuses := map[enums.ValidationAngle]policy.AngleStatus{}
	for _, a := range obligatory {
		statuses[a] = policy.StatusObligatory
	}
	for _, a := range optional {
		statuses[a] = policy.StatusOptional
	}
	return &policy.EffectivePolicy{
		SchemaVersion:    policy.SupportedSchemaVersion,
		ResolvedFrom:     []string{"global"},
		PerArchetype:     map[enums.Archetype]map[enums.ValidationAngle]policy.AngleStatus{arch: statuses},
		InferentialFloor: floor,
	}
}

func TestUseCase_ArchetypeNotInScopeReturnsError(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeCLI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI, []enums.ValidationAngle{enums.AngleBuild}, nil, 0.7)
	_, err := UseCase(uc, enums.ArchetypeHTTPAPI, nil, nil, pol)
	if err == nil || !strings.Contains(err.Error(), "archetype-not-in-scope") {
		t.Errorf("err = %v, want archetype-not-in-scope", err)
	}
}

func TestUseCase_ForeignSignalReturnsError(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI, []enums.ValidationAngle{enums.AngleBuild}, nil, 0.7)
	sig := makeSignal("uc-other", "s1", enums.AngleBuild, enums.VerdictPass, 1.0)
	sensors := []sensor.Sensor{makeSensor("s1", "uc-other", enums.AngleBuild, enums.NatureComputational)}
	_, err := UseCase(uc, enums.ArchetypeHTTPAPI, []aggregate.AggregateSignal{sig}, sensors, pol)
	if err == nil || !strings.Contains(err.Error(), "signal-foreign-use-case") {
		t.Errorf("err = %v, want signal-foreign-use-case", err)
	}
}

func TestUseCase_DuplicateAngleReturnsError(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI, []enums.ValidationAngle{enums.AngleBuild}, nil, 0.7)
	sig1 := makeSignal("uc-login", "s1", enums.AngleBuild, enums.VerdictPass, 1.0)
	sig2 := makeSignal("uc-login", "s2", enums.AngleBuild, enums.VerdictPass, 1.0)
	sensors := []sensor.Sensor{
		makeSensor("s1", "uc-login", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s2", "uc-login", enums.AngleBuild, enums.NatureComputational),
	}
	_, err := UseCase(uc, enums.ArchetypeHTTPAPI, []aggregate.AggregateSignal{sig1, sig2}, sensors, pol)
	if err == nil || !strings.Contains(err.Error(), "duplicate-angle-signal") {
		t.Errorf("err = %v, want duplicate-angle-signal", err)
	}
}

func TestUseCase_MissingObligatorySignalReturnsError(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI, []enums.ValidationAngle{enums.AngleBuild, enums.AngleUnitTest}, nil, 0.7)
	// Only one signal — unit-test obligatory is missing.
	sig := makeSignal("uc-login", "s1", enums.AngleBuild, enums.VerdictPass, 1.0)
	sensors := []sensor.Sensor{makeSensor("s1", "uc-login", enums.AngleBuild, enums.NatureComputational)}
	_, err := UseCase(uc, enums.ArchetypeHTTPAPI, []aggregate.AggregateSignal{sig}, sensors, pol)
	if err == nil || !strings.Contains(err.Error(), "missing-obligatory-signal") {
		t.Errorf("err = %v, want missing-obligatory-signal", err)
	}
}
```

(Confirmed against `internal/enums/enums.go` at plan-write time: `enums.TerminationCompleted`, `enums.OutputSingleShot`, `enums.KindAssertion`, `enums.NatureComputational`, `enums.NatureInferential`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/runtime/aggregator/usecase/ -run 'NotInScope|Foreign|Duplicate|MissingObligatory' -v`
Expected: FAIL (the stub from Task 10 returns no errors).

- [ ] **Step 3: Implement validation in `internal/runtime/aggregator/usecase/aggregator.go`**

Replace the existing `UseCase` function:

```go
package aggregator

import (
	"fmt"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/policy"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// UseCase computes the verdict for one use case under one archetype context.
// See spec §6 for the algorithm.
func UseCase(
	uc *usecase.UseCase,
	archetype enums.Archetype,
	signals []aggregate.AggregateSignal,
	sensors []sensor.Sensor,
	pol *policy.EffectivePolicy,
) (UseCaseVerdict, error) {
	if !archetypeInScope(uc, archetype) {
		return UseCaseVerdict{}, fmt.Errorf("aggregator: archetype-not-in-scope: %q is not in use case %q archetype_scope", archetype, uc.ID)
	}
	for _, s := range signals {
		if s.UseCaseID != uc.ID {
			return UseCaseVerdict{}, fmt.Errorf("aggregator: signal-foreign-use-case: signal use_case_id=%q does not match %q", s.UseCaseID, uc.ID)
		}
	}
	seen := make(map[enums.ValidationAngle]struct{}, len(signals))
	signalByAngle := make(map[enums.ValidationAngle]aggregate.AggregateSignal, len(signals))
	for _, s := range signals {
		if _, dup := seen[s.Angle]; dup {
			return UseCaseVerdict{}, fmt.Errorf("aggregator: duplicate-angle-signal: angle %q has more than one AggregateSignal", s.Angle)
		}
		seen[s.Angle] = struct{}{}
		signalByAngle[s.Angle] = s
	}

	statuses := pol.PerArchetype[archetype]
	for angle, status := range statuses {
		if status != policy.StatusObligatory {
			continue
		}
		if _, ok := signalByAngle[angle]; !ok {
			return UseCaseVerdict{}, fmt.Errorf("aggregator: missing-obligatory-signal: angle %q has no AggregateSignal", angle)
		}
	}

	// Verdict + confidence computation arrives in Task 12.
	return UseCaseVerdict{
		UseCaseID:       uc.ID,
		Archetype:       archetype,
		Verdict:         enums.VerdictInconclusive,
		EvaluatedAngles: []enums.ValidationAngle{},
		FailingAngles:   []enums.ValidationAngle{},
		WarningAngles:   []enums.ValidationAngle{},
		HealHints:       []AngleHint{},
	}, nil
}

func archetypeInScope(uc *usecase.UseCase, arch enums.Archetype) bool {
	for _, a := range uc.ArchetypeScope {
		if a == arch {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runtime/aggregator/usecase/ -run 'NotInScope|Foreign|Duplicate|MissingObligatory' -v`
Expected: PASS for all four.

- [ ] **Step 5: Run the full aggregator package**

Run: `go test ./internal/runtime/aggregator/usecase/...`
Expected: PASS for all 6 tests.

- [ ] **Step 6: Commit**

```bash
git add internal/runtime/aggregator/usecase/
git commit -m "feat(runtime/aggregator): validate archetype scope, signal ownership, angle uniqueness, obligatory coverage"
```

---

## Task 12: Verdict computation with floor demotion, warn handling, fail handling

**Files:**
- Modify: `internal/runtime/aggregator/usecase/aggregator.go`
- Modify: `internal/runtime/aggregator/usecase/aggregator_test.go`

- [ ] **Step 1: Write failing tests in `internal/runtime/aggregator/usecase/aggregator_test.go`**

Append:

```go
func TestUseCase_AllObligatoryPass(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild, enums.AngleUnitTest, enums.AngleE2ETest},
		nil, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-build", enums.AngleBuild, enums.VerdictPass, 1.0),
		makeSignal("uc-login", "s-unit", enums.AngleUnitTest, enums.VerdictPass, 1.0),
		makeSignal("uc-login", "s-e2e", enums.AngleE2ETest, enums.VerdictPass, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-build", "uc-login", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s-unit", "uc-login", enums.AngleUnitTest, enums.NatureComputational),
		makeSensor("s-e2e", "uc-login", enums.AngleE2ETest, enums.NatureComputational),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Verdict != enums.VerdictPass {
		t.Errorf("Verdict = %q, want pass", v.Verdict)
	}
	if !v.ObligatorySatisfied {
		t.Error("ObligatorySatisfied = false, want true")
	}
	if len(v.FailingAngles) != 0 || len(v.WarningAngles) != 0 || len(v.HealHints) != 0 {
		t.Errorf("unexpected non-pass surface: failing=%v warning=%v hints=%d", v.FailingAngles, v.WarningAngles, len(v.HealHints))
	}
}

func TestUseCase_OneObligatoryFail(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild, enums.AngleUnitTest}, nil, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-build", enums.AngleBuild, enums.VerdictPass, 1.0),
		makeSignal("uc-login", "s-unit", enums.AngleUnitTest, enums.VerdictFail, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-build", "uc-login", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s-unit", "uc-login", enums.AngleUnitTest, enums.NatureComputational),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Verdict != enums.VerdictFail {
		t.Errorf("Verdict = %q, want fail", v.Verdict)
	}
	if v.ObligatorySatisfied {
		t.Error("ObligatorySatisfied = true, want false")
	}
	if len(v.FailingAngles) != 1 || v.FailingAngles[0] != enums.AngleUnitTest {
		t.Errorf("FailingAngles = %v, want [unit-test]", v.FailingAngles)
	}
	if len(v.HealHints) != 1 || v.HealHints[0].Angle != enums.AngleUnitTest || v.HealHints[0].Verdict != enums.VerdictFail {
		t.Errorf("HealHints = %+v, want one entry for unit-test:fail", v.HealHints)
	}
}

func TestUseCase_OnlyOptionalFails_VerdictStaysPass(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild},
		[]enums.ValidationAngle{enums.AngleSecurity}, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-build", enums.AngleBuild, enums.VerdictPass, 1.0),
		makeSignal("uc-login", "s-sec", enums.AngleSecurity, enums.VerdictFail, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-build", "uc-login", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s-sec", "uc-login", enums.AngleSecurity, enums.NatureComputational),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Verdict != enums.VerdictPass {
		t.Errorf("Verdict = %q, want pass", v.Verdict)
	}
	if !v.ObligatorySatisfied {
		t.Error("ObligatorySatisfied = false, want true")
	}
	if len(v.FailingAngles) != 1 || v.FailingAngles[0] != enums.AngleSecurity {
		t.Errorf("FailingAngles = %v, want [security]", v.FailingAngles)
	}
}

func TestUseCase_ObligatoryWarn_StaysPass(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild}, nil, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-build", enums.AngleBuild, enums.VerdictWarn, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-build", "uc-login", enums.AngleBuild, enums.NatureComputational),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Verdict != enums.VerdictPass {
		t.Errorf("Verdict = %q, want pass", v.Verdict)
	}
	if !v.ObligatorySatisfied {
		t.Error("ObligatorySatisfied = false, want true (warn is pass-grade)")
	}
	if len(v.WarningAngles) != 1 || v.WarningAngles[0] != enums.AngleBuild {
		t.Errorf("WarningAngles = %v, want [build]", v.WarningAngles)
	}
	if len(v.HealHints) != 1 || v.HealHints[0].Verdict != enums.VerdictWarn {
		t.Errorf("HealHints = %+v, want one entry for warn", v.HealHints)
	}
}

func TestUseCase_ObligatoryWarnAndFail_VerdictIsFail(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild, enums.AngleUnitTest}, nil, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-build", enums.AngleBuild, enums.VerdictWarn, 1.0),
		makeSignal("uc-login", "s-unit", enums.AngleUnitTest, enums.VerdictFail, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-build", "uc-login", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s-unit", "uc-login", enums.AngleUnitTest, enums.NatureComputational),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	// Any obligatory fail wins over warn.
	if v.Verdict != enums.VerdictFail {
		t.Errorf("Verdict = %q, want fail", v.Verdict)
	}
	if v.ObligatorySatisfied {
		t.Error("ObligatorySatisfied = true, want false")
	}
	if len(v.FailingAngles) != 1 || v.FailingAngles[0] != enums.AngleUnitTest {
		t.Errorf("FailingAngles = %v, want [unit-test]", v.FailingAngles)
	}
	if len(v.WarningAngles) != 1 || v.WarningAngles[0] != enums.AngleBuild {
		t.Errorf("WarningAngles = %v, want [build]", v.WarningAngles)
	}
	if len(v.HealHints) != 2 {
		t.Fatalf("HealHints = %d, want 2", len(v.HealHints))
	}
	// Canonical angle order: build (1) before unit-test (3).
	if v.HealHints[0].Angle != enums.AngleBuild || v.HealHints[0].Verdict != enums.VerdictWarn {
		t.Errorf("HealHints[0] = %+v, want build:warn", v.HealHints[0])
	}
	if v.HealHints[1].Angle != enums.AngleUnitTest || v.HealHints[1].Verdict != enums.VerdictFail {
		t.Errorf("HealHints[1] = %+v, want unit-test:fail", v.HealHints[1])
	}
}

func TestUseCase_InferentialFailBelowFloor_Demotes(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleE2ETest}, nil, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-e2e", enums.AngleE2ETest, enums.VerdictFail, 0.5),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-e2e", "uc-login", enums.AngleE2ETest, enums.NatureInferential),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Verdict != enums.VerdictInconclusive {
		t.Errorf("Verdict = %q, want inconclusive (demoted)", v.Verdict)
	}
	if len(v.FailingAngles) != 0 {
		t.Errorf("FailingAngles = %v, want [] (demoted)", v.FailingAngles)
	}
	if len(v.HealHints) != 0 {
		t.Errorf("HealHints = %d, want 0 (demoted)", len(v.HealHints))
	}
}

func TestUseCase_InferentialWarnBelowFloor_Demotes(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleE2ETest}, nil, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-e2e", enums.AngleE2ETest, enums.VerdictWarn, 0.6),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-e2e", "uc-login", enums.AngleE2ETest, enums.NatureInferential),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Verdict != enums.VerdictInconclusive {
		t.Errorf("Verdict = %q, want inconclusive (demoted)", v.Verdict)
	}
	if len(v.WarningAngles) != 0 {
		t.Errorf("WarningAngles = %v, want [] (demoted)", v.WarningAngles)
	}
}

func TestUseCase_CanonicalAngleOrder(t *testing.T) {
	// Submit signals in non-canonical order; verify output sorted per enums.AllAngles().
	uc := makeUseCase("uc-multi", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild, enums.AngleSecurity, enums.AngleUnitTest},
		nil, 0.7)
	// Reverse order intentionally.
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-multi", "s-unit", enums.AngleUnitTest, enums.VerdictFail, 1.0),
		makeSignal("uc-multi", "s-sec", enums.AngleSecurity, enums.VerdictFail, 1.0),
		makeSignal("uc-multi", "s-build", enums.AngleBuild, enums.VerdictFail, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-unit", "uc-multi", enums.AngleUnitTest, enums.NatureComputational),
		makeSensor("s-sec", "uc-multi", enums.AngleSecurity, enums.NatureComputational),
		makeSensor("s-build", "uc-multi", enums.AngleBuild, enums.NatureComputational),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	// enums.AllAngles() order is: security, build, code-structure, unit-test, ...
	want := []enums.ValidationAngle{enums.AngleSecurity, enums.AngleBuild, enums.AngleUnitTest}
	if !reflect.DeepEqual(v.FailingAngles, want) {
		t.Errorf("FailingAngles = %v, want %v (canonical order)", v.FailingAngles, want)
	}
	for i, h := range v.HealHints {
		if h.Angle != want[i] {
			t.Errorf("HealHints[%d].Angle = %q, want %q", i, h.Angle, want[i])
		}
	}
}
```

Add `"reflect"` to the test imports if not already there.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/runtime/aggregator/usecase/ -v`
Expected: FAIL — the implementation still returns the inconclusive stub from Task 11.

- [ ] **Step 3: Implement verdict computation in `internal/runtime/aggregator/usecase/aggregator.go`**

Replace the post-validation portion of `UseCase` with the full algorithm. Final shape of `UseCase`:

```go
func UseCase(
	uc *usecase.UseCase,
	archetype enums.Archetype,
	signals []aggregate.AggregateSignal,
	sensors []sensor.Sensor,
	pol *policy.EffectivePolicy,
) (UseCaseVerdict, error) {
	if !archetypeInScope(uc, archetype) {
		return UseCaseVerdict{}, fmt.Errorf("aggregator: archetype-not-in-scope: %q is not in use case %q archetype_scope", archetype, uc.ID)
	}
	for _, s := range signals {
		if s.UseCaseID != uc.ID {
			return UseCaseVerdict{}, fmt.Errorf("aggregator: signal-foreign-use-case: signal use_case_id=%q does not match %q", s.UseCaseID, uc.ID)
		}
	}
	seen := make(map[enums.ValidationAngle]struct{}, len(signals))
	signalByAngle := make(map[enums.ValidationAngle]aggregate.AggregateSignal, len(signals))
	for _, s := range signals {
		if _, dup := seen[s.Angle]; dup {
			return UseCaseVerdict{}, fmt.Errorf("aggregator: duplicate-angle-signal: angle %q has more than one AggregateSignal", s.Angle)
		}
		seen[s.Angle] = struct{}{}
		signalByAngle[s.Angle] = s
	}

	statuses := pol.PerArchetype[archetype]
	for angle, status := range statuses {
		if status != policy.StatusObligatory {
			continue
		}
		if _, ok := signalByAngle[angle]; !ok {
			return UseCaseVerdict{}, fmt.Errorf("aggregator: missing-obligatory-signal: angle %q has no AggregateSignal", angle)
		}
	}

	natureBySensorID := make(map[string]enums.SensorNature, len(sensors))
	for _, s := range sensors {
		natureBySensorID[s.ID] = s.Nature
	}

	v := UseCaseVerdict{
		UseCaseID:       uc.ID,
		Archetype:       archetype,
		EvaluatedAngles: []enums.ValidationAngle{},
		FailingAngles:   []enums.ValidationAngle{},
		WarningAngles:   []enums.ValidationAngle{},
		HealHints:       []AngleHint{},
	}

	anyObligatoryFail := false
	allObligatoryPassGrade := true
	var weightSum, weightedSum float64

	for _, angle := range enums.AllAngles() {
		status, hasStatus := statuses[angle]
		if !hasStatus || status == policy.StatusDisabled {
			continue
		}
		sig, hasSig := signalByAngle[angle]
		if !hasSig {
			// status == optional with no signal: skip.
			continue
		}

		nature := natureBySensorID[sig.SensorID]
		effective := effectiveVerdict(sig, nature, pol.InferentialFloor)

		v.EvaluatedAngles = append(v.EvaluatedAngles, angle)

		// Confidence: raw signal.Confidence, weight depends on nature.
		var weight float64
		if nature == enums.NatureComputational {
			weight = 1.0
		} else {
			weight = sig.Confidence
		}
		weightSum += weight
		weightedSum += weight * sig.Confidence

		switch effective {
		case enums.VerdictFail:
			if sig.HealHint == nil {
				return UseCaseVerdict{}, fmt.Errorf("aggregator: signal angle=%q verdict=fail but heal_hint is nil (E8 invariant violated)", angle)
			}
			v.FailingAngles = append(v.FailingAngles, angle)
			v.HealHints = append(v.HealHints, AngleHint{Angle: angle, Verdict: enums.VerdictFail, Hint: *sig.HealHint})
			if status == policy.StatusObligatory {
				anyObligatoryFail = true
			}
		case enums.VerdictWarn:
			if sig.HealHint == nil {
				return UseCaseVerdict{}, fmt.Errorf("aggregator: signal angle=%q verdict=warn but heal_hint is nil (E8 invariant violated)", angle)
			}
			v.WarningAngles = append(v.WarningAngles, angle)
			v.HealHints = append(v.HealHints, AngleHint{Angle: angle, Verdict: enums.VerdictWarn, Hint: *sig.HealHint})
		case enums.VerdictInconclusive:
			if status == policy.StatusObligatory {
				allObligatoryPassGrade = false
			}
		case enums.VerdictPass:
			// pass-grade; nothing to surface.
		}
	}

	switch {
	case anyObligatoryFail:
		v.Verdict = enums.VerdictFail
	case allObligatoryPassGrade:
		v.Verdict = enums.VerdictPass
	default:
		v.Verdict = enums.VerdictInconclusive
	}
	v.ObligatorySatisfied = v.Verdict == enums.VerdictPass

	if weightSum > 0 {
		v.Confidence = weightedSum / weightSum
	}

	return v, nil
}

func effectiveVerdict(sig aggregate.AggregateSignal, nature enums.SensorNature, floor float64) enums.Verdict {
	if nature == enums.NatureInferential && sig.Confidence < floor {
		return enums.VerdictInconclusive
	}
	return sig.Verdict
}
```

Note: `anyObligatoryFail` covers the "any obligatory fail → fail" rule. `allObligatoryPassGrade` is true iff every obligatory signal's effective verdict is pass or warn (i.e., none is inconclusive or fail). This matches the truth table:

| obligatory situation | anyFail | allPassGrade | Verdict |
|---|---|---|---|
| all pass | false | true | pass |
| all warn | false | true | pass |
| mixed pass/warn | false | true | pass |
| any fail | true | * | fail |
| any inconclusive (no fails) | false | false | inconclusive |

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runtime/aggregator/usecase/ -v`
Expected: PASS for all tests so far.

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/aggregator/usecase/
git commit -m "feat(runtime/aggregator): implement verdict computation with floor demotion and warn handling"
```

---

## Task 13: Weighted confidence — computational + inferential mix, including floor-demoted signals

**Files:**
- Modify: `internal/runtime/aggregator/usecase/aggregator_test.go`

- [ ] **Step 1: Write failing tests in `internal/runtime/aggregator/usecase/aggregator_test.go`**

The implementation in Task 12 already computes confidence; this task locks the math down with hand-calculated assertions.

Append:

```go
func almostEqual(a, b, tol float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}

func TestUseCase_Confidence_AllComputationalPass(t *testing.T) {
	uc := makeUseCase("uc", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild, enums.AngleUnitTest}, nil, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc", "s1", enums.AngleBuild, enums.VerdictPass, 1.0),
		makeSignal("uc", "s2", enums.AngleUnitTest, enums.VerdictPass, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s1", "uc", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s2", "uc", enums.AngleUnitTest, enums.NatureComputational),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	// weights: 1.0, 1.0. weighted sum: 1*1 + 1*1 = 2.0. total weight: 2.0. avg: 1.0.
	if !almostEqual(v.Confidence, 1.0, 1e-9) {
		t.Errorf("Confidence = %v, want 1.0", v.Confidence)
	}
}

func TestUseCase_Confidence_MixedComputationalInferential(t *testing.T) {
	// Worked example from spec §6.3 (second variant, fail not floor-demoted):
	// build (comp, pass, 1.0): weight 1.0, value 1.0
	// unit-test (comp, pass, 1.0): weight 1.0, value 1.0
	// e2e-test (inf, fail, 0.95): weight 0.95, value 0.95
	// security (inf, pass, 0.9): weight 0.9, value 0.9
	// weight sum = 3.85, weighted sum = 1 + 1 + 0.9025 + 0.81 = 3.7125
	// confidence ≈ 0.9642857...
	uc := makeUseCase("uc", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild, enums.AngleUnitTest, enums.AngleE2ETest},
		[]enums.ValidationAngle{enums.AngleSecurity}, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc", "s-build", enums.AngleBuild, enums.VerdictPass, 1.0),
		makeSignal("uc", "s-unit", enums.AngleUnitTest, enums.VerdictPass, 1.0),
		makeSignal("uc", "s-e2e", enums.AngleE2ETest, enums.VerdictFail, 0.95),
		makeSignal("uc", "s-sec", enums.AngleSecurity, enums.VerdictPass, 0.9),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-build", "uc", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s-unit", "uc", enums.AngleUnitTest, enums.NatureComputational),
		makeSensor("s-e2e", "uc", enums.AngleE2ETest, enums.NatureInferential),
		makeSensor("s-sec", "uc", enums.AngleSecurity, enums.NatureInferential),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	want := 3.7125 / 3.85
	if !almostEqual(v.Confidence, want, 1e-9) {
		t.Errorf("Confidence = %v, want %v", v.Confidence, want)
	}
	if v.Verdict != enums.VerdictFail {
		t.Errorf("Verdict = %q, want fail (e2e-test confidence 0.95 >= floor 0.7)", v.Verdict)
	}
}

func TestUseCase_Confidence_FloorDemotedSignalStillContributes(t *testing.T) {
	// Worked example from spec §6.3 (first variant, fail floor-demoted):
	// build (comp, pass, 1.0): weight 1.0
	// unit-test (comp, pass, 1.0): weight 1.0
	// e2e-test (inf, fail, 0.5): weight 0.5, demoted to inconclusive for verdict; still contributes confidence
	// security (inf, pass, 0.9): weight 0.9
	// weight sum = 3.4, weighted sum = 1 + 1 + 0.25 + 0.81 = 3.06
	// confidence ≈ 0.9
	uc := makeUseCase("uc", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild, enums.AngleUnitTest, enums.AngleE2ETest},
		[]enums.ValidationAngle{enums.AngleSecurity}, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc", "s-build", enums.AngleBuild, enums.VerdictPass, 1.0),
		makeSignal("uc", "s-unit", enums.AngleUnitTest, enums.VerdictPass, 1.0),
		makeSignal("uc", "s-e2e", enums.AngleE2ETest, enums.VerdictFail, 0.5),
		makeSignal("uc", "s-sec", enums.AngleSecurity, enums.VerdictPass, 0.9),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-build", "uc", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s-unit", "uc", enums.AngleUnitTest, enums.NatureComputational),
		makeSensor("s-e2e", "uc", enums.AngleE2ETest, enums.NatureInferential),
		makeSensor("s-sec", "uc", enums.AngleSecurity, enums.NatureInferential),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	want := 3.06 / 3.4
	if !almostEqual(v.Confidence, want, 1e-9) {
		t.Errorf("Confidence = %v, want %v", v.Confidence, want)
	}
	if v.Verdict != enums.VerdictInconclusive {
		t.Errorf("Verdict = %q, want inconclusive (e2e-test demoted)", v.Verdict)
	}
	if len(v.FailingAngles) != 0 {
		t.Errorf("FailingAngles = %v, want [] (demoted)", v.FailingAngles)
	}
}

func TestUseCase_Confidence_ZeroWhenNoSignals(t *testing.T) {
	uc := makeUseCase("uc", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI, nil, nil, 0.7) // no obligatory, no optional
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, nil, nil, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Confidence != 0.0 {
		t.Errorf("Confidence = %v, want 0.0", v.Confidence)
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./internal/runtime/aggregator/usecase/ -run 'Confidence' -v`
Expected: PASS for all four.

If any fail, the most likely cause is the iteration not catching a corner case — debug by printing `v.EvaluatedAngles`, `weightSum`, `weightedSum` before comparing.

- [ ] **Step 3: Run the full aggregator package**

Run: `go test ./internal/runtime/aggregator/usecase/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/runtime/aggregator/usecase/aggregator_test.go
git commit -m "test(runtime/aggregator): lock weighted confidence math with hand-calculated cases"
```

---

## Task 14: Reject `verdict=fail` signal with `nil` heal_hint (E8 invariant)

**Files:**
- Modify: `internal/runtime/aggregator/usecase/aggregator_test.go`

- [ ] **Step 1: Write failing test in `internal/runtime/aggregator/usecase/aggregator_test.go`**

```go
func TestUseCase_FailSignalWithNilHealHintReturnsError(t *testing.T) {
	uc := makeUseCase("uc", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild}, nil, 0.7)
	// Hand-construct a fail signal WITHOUT a HealHint (bypassing makeSignal's auto-fill).
	sig := aggregate.AggregateSignal{
		SchemaVersion: "1.0.0", Type: aggregate.TypeAggregate,
		SensorID: "s1", UseCaseID: "uc",
		Angle:             enums.AngleBuild,
		StartedAt:         time.Unix(1, 0),
		EndedAt:           time.Unix(2, 0),
		TerminationReason: enums.TerminationCompleted,
		Verdict:           enums.VerdictFail,
		Confidence:        1.0,
		HealHint:          nil,
	}
	sensors := []sensor.Sensor{makeSensor("s1", "uc", enums.AngleBuild, enums.NatureComputational)}
	_, err := UseCase(uc, enums.ArchetypeHTTPAPI, []aggregate.AggregateSignal{sig}, sensors, pol)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"heal_hint", "build", "fail"} {
		if !strings.Contains(msg, want) {
			t.Errorf("err = %q, missing %q", msg, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

The Task 12 implementation already returns this error. Run:

Run: `go test ./internal/runtime/aggregator/usecase/ -run TestUseCase_FailSignalWithNilHealHint -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/runtime/aggregator/usecase/aggregator_test.go
git commit -m "test(runtime/aggregator): reject fail signal with nil heal_hint per E8 invariant"
```

---

## Task 15: Golden determinism test — fixed inputs → byte-identical `UseCaseVerdict` JSON

**Files:**
- Create: `internal/runtime/aggregator/usecase/testdata/golden_verdict.json`
- Modify: `internal/runtime/aggregator/usecase/aggregator_test.go`

- [ ] **Step 1: Write the failing test in `internal/runtime/aggregator/usecase/aggregator_test.go`**

```go
func TestUseCase_GoldenDeterminism(t *testing.T) {
	uc := makeUseCase("uc-checkout", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild, enums.AngleUnitTest, enums.AngleE2ETest},
		[]enums.ValidationAngle{enums.AngleSecurity}, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-checkout", "s-build", enums.AngleBuild, enums.VerdictPass, 1.0),
		makeSignal("uc-checkout", "s-unit", enums.AngleUnitTest, enums.VerdictPass, 1.0),
		makeSignal("uc-checkout", "s-e2e", enums.AngleE2ETest, enums.VerdictFail, 0.95),
		makeSignal("uc-checkout", "s-sec", enums.AngleSecurity, enums.VerdictWarn, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-build", "uc-checkout", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s-unit", "uc-checkout", enums.AngleUnitTest, enums.NatureComputational),
		makeSensor("s-e2e", "uc-checkout", enums.AngleE2ETest, enums.NatureInferential),
		makeSensor("s-sec", "uc-checkout", enums.AngleSecurity, enums.NatureComputational),
	}

	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	gotJSON, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}

	goldenPath := filepath.Join("testdata", "golden_verdict.json")
	wantJSON, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(gotJSON, bytes.TrimRight(wantJSON, "\n")) {
		t.Errorf("golden mismatch\ngot:\n%s\nwant:\n%s", string(gotJSON), string(wantJSON))
	}

	// Re-run 5 times to verify determinism within a process.
	for i := 0; i < 5; i++ {
		again, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
		if err != nil {
			t.Fatalf("re-run %d: %v", i, err)
		}
		againJSON, err := json.MarshalIndent(again, "", "  ")
		if err != nil {
			t.Fatalf("re-run %d marshal: %v", i, err)
		}
		if !bytes.Equal(gotJSON, againJSON) {
			t.Fatalf("re-run %d JSON drifted", i)
		}
	}
}
```

Add the imports `"bytes"`, `"encoding/json"`, `"os"`, `"path/filepath"` if not already present.

- [ ] **Step 2: Run the test to capture initial output**

Run: `go test ./internal/runtime/aggregator/usecase/ -run TestUseCase_GoldenDeterminism -v`
Expected: FAIL with "read golden: ... no such file or directory" — capture the printed `got:` block.

Run the test once with `-v` and copy the `got:` JSON output. Save it to `internal/runtime/aggregator/usecase/testdata/golden_verdict.json`.

Hand-verify the JSON matches expectations from the inputs:
- `UseCaseID: "uc-checkout"`, `Archetype: "http-api"`
- `Verdict: "fail"` (e2e-test fails, confidence 0.95 ≥ floor 0.7)
- `Confidence ≈ 0.9642857` (from Task 13's calculation, plus security warn @ 1.0 weight 1.0 ⇒ recomputed below)
- Actually with security at confidence 1.0 (warn, computational): weight 1.0, value 1.0 → weighted sum 3.7125 + 1.0 = 4.7125 over total weight 3.85 + 1.0 = 4.85 → ≈ 0.97164948...
- `EvaluatedAngles: ["security", "build", "unit-test", "e2e-test"]` (canonical enums.AllAngles() order with active angles only)
- `FailingAngles: ["e2e-test"]`
- `WarningAngles: ["security"]`
- `HealHints: [{angle: "security", verdict: "warn", ...}, {angle: "e2e-test", verdict: "fail", ...}]` (canonical angle order: security < e2e-test in AllAngles())

The first `go test` run prints the actual computed JSON — use that verbatim for the golden. Do NOT hand-write it.

- [ ] **Step 3: Run the test to verify it passes**

Run: `go test ./internal/runtime/aggregator/usecase/ -run TestUseCase_GoldenDeterminism -v`
Expected: PASS.

- [ ] **Step 4: Run the full aggregator package**

Run: `go test ./internal/runtime/aggregator/usecase/...`
Expected: PASS for all tests.

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/aggregator/usecase/aggregator_test.go internal/runtime/aggregator/usecase/testdata/golden_verdict.json
git commit -m "test(runtime/aggregator): golden determinism test on fixed use case + signals + policy"
```

---

# Phase 4 — Integration verification

## Task 16: `-race` clean across new + extended packages

**Files:** (none — verification only)

- [ ] **Step 1: Run `go test -race` on the new + extended packages**

Run:
```bash
go test -race ./internal/runtime/... ./internal/policy/...
```
Expected: PASS, no `DATA RACE` reports.

- [ ] **Step 2: If a race is reported, debug as a normal bug**

The aggregator and binder are pure functions modulo file writes; races should not be expected. If one shows up, it's a real bug — fix it and add a test that exposes it.

- [ ] **Step 3: Run `go vet`**

Run:
```bash
go vet ./internal/runtime/... ./internal/policy/...
```
Expected: silent (no findings).

- [ ] **Step 4: Run the full test suite to confirm nothing else broke**

Run:
```bash
go test ./...
```
Expected: PASS.

- [ ] **Step 5: Commit any cleanup if needed (otherwise skip)**

If steps 1–4 surfaced fixes, commit them. Otherwise nothing to commit — proceed to Task 17.

---

## Task 17: Add `inferential_floor` to example local policy + drift check

**Files:**
- Modify: `schemas/examples/validation-policy/local.yaml`

- [ ] **Step 1: Inspect the current example**

```bash
cat schemas/examples/validation-policy/local.yaml
```

- [ ] **Step 2: Add `inferential_floor: 0.8` near the top of `schemas/examples/validation-policy/local.yaml`**

After the existing `scope: local` line, add:

```yaml
inferential_floor: 0.8
```

(Pick `0.8` to distinguish from the default `0.7` so any future test that loads this example can assert on the field.)

- [ ] **Step 3: Verify the loader still accepts the example**

Run: `go test ./internal/policy/ -run TestLoad_LocalExample -v`
Expected: PASS.

- [ ] **Step 4: Verify schema drift still clean**

If there is a `drift_test.go` under `internal/policy/`, run it. Otherwise:

Run: `go test ./internal/policy/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add schemas/examples/validation-policy/local.yaml
git commit -m "docs(schemas): add inferential_floor example to local validation-policy"
```

---

## Task 18: Final spec self-check + push branch

**Files:** (none — verification + push only)

- [ ] **Step 1: Re-read [spec §12](../specs/2026-05-24-b1-composed-runtime-design.md#12-acceptance-criteria) — acceptance criteria**

Walk through each criterion and confirm a test exists:

- `fixturebinder.Bind` 2 fixtures (json + binary), env + file, errors on not-found and not-owned → Tasks 8, 9 ✓
- `aggregator.UseCase` 3 obligatory all pass → Task 12 ✓
- `aggregator.UseCase` one obligatory fails → Task 12 ✓
- `aggregator.UseCase` only optional fails → Task 12 ✓
- Floor demotes fail-with-low-confidence at use-case level → Task 12 ✓
- Golden test byte-identical → Task 15 ✓
- `policy.Resolve` honors `InferentialFloor` overrides + defaults → Task 3 ✓
- `-race` passes → Task 16 ✓

If any criterion has no matching test, add it before moving on.

- [ ] **Step 2: Push the branch**

```bash
git push -u origin feat/b1-composed-runtime
```

- [ ] **Step 3: Open a draft PR (do not request review yet)**

```bash
gh pr create --draft --title "feat: B1 composed runtime (fixture binder + per-use-case aggregator + InferentialFloor)" --body "$(cat <<'EOF'
## Summary

Implements [B1 — Composed Runtime](../docs/superpowers/specs/2026-05-24-b1-composed-runtime-design.md).

- New package `internal/runtime/fixturebinder/` — writes fixture payloads to a step-owned scratch dir, exposes paths via `HARNESS_FIXTURE_<NORMALIZED_ID>` env vars
- New package `internal/runtime/aggregator/usecase/` — computes the use-case verdict per plan §6.3, applying the inferential confidence floor
- `internal/policy/EffectivePolicy.InferentialFloor` — new resolved field, default 0.7
- Schema: `schemas/validation-policy.yaml` gains optional `inferential_floor` property

## Test plan

- [ ] `go test ./internal/runtime/... ./internal/policy/...` passes
- [ ] `go test -race ./internal/runtime/... ./internal/policy/...` passes
- [ ] `go vet ./...` is silent
- [ ] Golden test `TestUseCase_GoldenDeterminism` confirms byte-identical JSON across runs

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 4: Done — move branch to "ready for review" only after the human owner reviews locally**

The PR is intentionally a draft so the owner can re-pull and verify locally before requesting review.
