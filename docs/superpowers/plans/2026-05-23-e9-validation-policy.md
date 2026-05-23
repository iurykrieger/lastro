# E9 ValidationPolicy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `internal/policy/` Go package: a pure library that loads, validates, resolves, and serializes `ValidationPolicy` YAML files into an `EffectivePolicy` consumed by sensor generation and runtime gating.

**Architecture:** Two scopes (`global`, `local`) merge with per-`(archetype, angle)` override granularity. Loader pipeline mirrors the established pattern in E2/E3/E5: `sigs.k8s.io/yaml` YAML→JSON normalize → `santhosh-tekuri/jsonschema/v6` schema validate → `json.Unmarshal` → in-Go semantic validators (applicable-angle matrix, disjoint lists). `Resolve` is a pure function; serialization is deterministic via alphabetical sorting and `sigs.k8s.io/yaml.Marshal`.

**Tech Stack:** Go 1.24, `sigs.k8s.io/yaml`, `github.com/santhosh-tekuri/jsonschema/v6`, `github.com/iurykrieger/lastro/internal/enums` (E1), `github.com/iurykrieger/lastro/schemas` (embedded FS).

**Spec:** [`docs/superpowers/specs/2026-05-23-e9-validation-policy-design.md`](../specs/2026-05-23-e9-validation-policy-design.md)

---

## File Structure

**Created:**
- `internal/policy/types.go` — `Scope`, `ValidationPolicy`, `ArchetypeBlock`, `EffectivePolicy`, `AngleStatus`, `SupportedSchemaVersion`. Public API surface; no logic.
- `internal/policy/schema.go` — JSON Schema compilation against `schemas.FS`. Mirrors `internal/entrypoint/schema.go`.
- `internal/policy/loader.go` — `Load(io.Reader) (*ValidationPolicy, error)`. Phases: YAML→JSON, schema validate, unmarshal, semantic validate (matrix + disjoint).
- `internal/policy/resolve.go` — `Resolve(global, local *ValidationPolicy) *EffectivePolicy`. Pure function.
- `internal/policy/lookup.go` — `AnglesFor`, `Status` methods on `*EffectivePolicy`.
- `internal/policy/serialize.go` — `MarshalYAML` method on `*EffectivePolicy`.
- `internal/policy/schema_test.go`, `loader_test.go`, `resolve_test.go`, `lookup_test.go`, `serialize_test.go` — sibling tests.
- `internal/policy/testdata/*.yaml` — negative fixtures (one per rejected loader case).

**Modified:**
- `schemas/validation-policy.yaml` — frozen-schema amendment: `scope` enum becomes `[global, local]`; `inherits_from` removed.
- `schemas/examples/validation-policy/` — rename `repo.yaml` to `local.yaml`, delete `org.yaml`, update scope strings, drop `inherits_from`.
- `schemas/README.md` — drop the `ValidationPolicy.inherits_from` row from §4.

---

## Task 1: Amend the frozen schema (gate-level change)

The two-scope decision requires editing the schema-freeze gate's output. Land this first so every subsequent task is reading from the corrected schema.

**Files:**
- Modify: `schemas/validation-policy.yaml`

- [ ] **Step 1: Edit the schema**

Replace `schemas/validation-policy.yaml` with the amended version:

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/validation-policy.yaml"
title: ValidationPolicy
description: |
  Per-archetype declaration of which validation angles are obligatory,
  optional, or disabled. Two scopes compose with per-(archetype, angle)
  override granularity: local > global. Multi-level upstream composition
  is delegated to callers (pre-merge into a single global).

type: object
required: [schema_version, scope, per_archetype]
additionalProperties: false

properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  scope:
    type: string
    enum: [global, local]
  per_archetype:
    type: object
    additionalProperties:
      type: object
      required: [obligatory_angles, optional_angles, disabled_angles]
      additionalProperties: false
      properties:
        obligatory_angles: { $ref: "#/$defs/AngleList" }
        optional_angles:   { $ref: "#/$defs/AngleList" }
        disabled_angles:   { $ref: "#/$defs/AngleList" }
    propertyNames:
      enum: [http-api, event-consumer, event-producer, cli, sdk, library, worker, batch-job, static-site]

$defs:
  AngleList:
    type: array
    items:
      type: string
      enum: [security, build, code-structure, unit-test, e2e-test,
             contracts, logs, metrics, database, performance]
```

Changes from current: `scope` enum drops `org`/`repo`, gains `local`; `inherits_from` property removed; `$defs.Id` removed (was only referenced by `inherits_from`).

- [ ] **Step 2: Run the existing schema sanity test**

Run: `go test ./schemas/...`
Expected: PASS (the existing test only checks that key files exist in the embed; it does not introspect their content).

- [ ] **Step 3: Commit**

```bash
git add schemas/validation-policy.yaml
git commit -m "schema-freeze(e9): amend ValidationPolicy to two-scope model

- scope: [global, local] (drops org, repo)
- remove inherits_from (multi-tier composition is caller's job)"
```

---

## Task 2: Update example files and schema README

The frozen examples and the README's cross-reference catalog still describe the three-scope world.

**Files:**
- Modify: `schemas/examples/validation-policy/global.yaml`
- Delete: `schemas/examples/validation-policy/org.yaml`
- Rename: `schemas/examples/validation-policy/repo.yaml` → `schemas/examples/validation-policy/local.yaml`
- Modify: `schemas/README.md` — §4 row removal

- [ ] **Step 1: Confirm `global.yaml` is already valid under the new schema**

Read `schemas/examples/validation-policy/global.yaml`. Verify it already has `scope: global` and no `inherits_from`. No edit needed if so.

- [ ] **Step 2: Rename `repo.yaml` to `local.yaml` and rewrite contents**

```bash
git mv schemas/examples/validation-policy/repo.yaml schemas/examples/validation-policy/local.yaml
```

Overwrite the file contents:

```yaml
# ValidationPolicy example — local scope, narrows http-api away from
# performance and pulls logs from optional into obligatory.
schema_version: 1.0.0
scope: local
per_archetype:
  http-api:
    obligatory_angles: [build, security, unit-test, e2e-test, contracts, logs]
    optional_angles:   [metrics]
    disabled_angles:   [performance]
```

- [ ] **Step 3: Delete `org.yaml`**

```bash
git rm schemas/examples/validation-policy/org.yaml
```

Rationale: org-scope was collapsed into `global` by the design (§3.1). One global + one local example is sufficient to cover both scopes.

- [ ] **Step 4: Update `schemas/README.md` §4 cross-reference table**

In `schemas/README.md`, remove this row (currently the last row of the §4 table):

```
| `ValidationPolicy.inherits_from` | ValidationPolicy | 0..1 |
```

- [ ] **Step 5: Run schemas package tests**

Run: `go test ./schemas/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A schemas/examples/validation-policy schemas/README.md
git commit -m "schema-freeze(e9): collapse ValidationPolicy examples to global+local

- Drop org.yaml (org-scope folded into global)
- Rename repo.yaml to local.yaml with two-scope shape
- Remove inherits_from row from schemas/README cross-ref table"
```

---

## Task 3: Package skeleton and types

Establish the public type surface before any logic. Mirror the field-tag style from `internal/entrypoint/types.go` (both `json:` and `yaml:` tags for forward compatibility with both libraries).

**Files:**
- Create: `internal/policy/types.go`
- Create: `internal/policy/types_test.go`

- [ ] **Step 1: Write the failing test**

`internal/policy/types_test.go`:

```go
package policy

import (
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestSupportedSchemaVersionMatchesExample(t *testing.T) {
	if SupportedSchemaVersion != "1.0.0" {
		t.Errorf("SupportedSchemaVersion = %q, want %q", SupportedSchemaVersion, "1.0.0")
	}
}

func TestScopeConstants(t *testing.T) {
	if ScopeGlobal != "global" {
		t.Errorf("ScopeGlobal = %q, want global", ScopeGlobal)
	}
	if ScopeLocal != "local" {
		t.Errorf("ScopeLocal = %q, want local", ScopeLocal)
	}
}

func TestAngleStatusConstants(t *testing.T) {
	cases := map[AngleStatus]string{
		StatusObligatory: "obligatory",
		StatusOptional:   "optional",
		StatusDisabled:   "disabled",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("AngleStatus %q has underlying string %q, want %q", got, string(got), want)
		}
	}
}

func TestZeroValueAngleStatusIsUnset(t *testing.T) {
	var zero AngleStatus
	if zero != "" {
		t.Errorf("zero AngleStatus = %q, want empty string (unset sentinel)", zero)
	}
}

func TestValidationPolicyShape(t *testing.T) {
	// Compile-time-ish sanity: a zero ValidationPolicy must have a nil map
	// (no auto-init), and assignments to all exported fields must compile.
	var p ValidationPolicy
	if p.PerArchetype != nil {
		t.Errorf("zero ValidationPolicy.PerArchetype = %v, want nil", p.PerArchetype)
	}
	p.SchemaVersion = "1.0.0"
	p.Scope = ScopeGlobal
	p.PerArchetype = map[enums.Archetype]ArchetypeBlock{
		enums.ArchetypeHTTPAPI: {
			Obligatory: []enums.ValidationAngle{enums.AngleBuild},
			Optional:   []enums.ValidationAngle{enums.AngleLogs},
			Disabled:   []enums.ValidationAngle{},
		},
	}
	if got := p.PerArchetype[enums.ArchetypeHTTPAPI].Obligatory[0]; got != enums.AngleBuild {
		t.Errorf("round-trip Obligatory[0] = %q, want build", got)
	}
}

func TestEffectivePolicyShape(t *testing.T) {
	var p EffectivePolicy
	if p.PerArchetype != nil {
		t.Errorf("zero EffectivePolicy.PerArchetype = %v, want nil", p.PerArchetype)
	}
	if p.ResolvedFrom != nil {
		t.Errorf("zero EffectivePolicy.ResolvedFrom = %v, want nil", p.ResolvedFrom)
	}
	p.PerArchetype = map[enums.Archetype]map[enums.ValidationAngle]AngleStatus{
		enums.ArchetypeCLI: {enums.AngleBuild: StatusObligatory},
	}
	if got := p.PerArchetype[enums.ArchetypeCLI][enums.AngleBuild]; got != StatusObligatory {
		t.Errorf("round-trip status = %q, want obligatory", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/policy/... -run TestSupportedSchemaVersionMatchesExample`
Expected: build error — package does not exist yet.

- [ ] **Step 3: Create `internal/policy/types.go`**

```go
// Package policy loads, validates, resolves, and serializes ValidationPolicy
// YAML files. It is a pure library: no filesystem awareness beyond the
// io.Reader passed to Load, no environment lookups, no network. Multi-tier
// upstream composition (org → division → framework default) is delegated
// to callers, who pre-merge a chain into a single *ValidationPolicy before
// calling Resolve.
package policy

import "github.com/iurykrieger/lastro/internal/enums"

// SupportedSchemaVersion pins the schema_version that EffectivePolicy
// values declare. Source files whose schema_version differs are accepted
// only if they match (loader rule 1).
const SupportedSchemaVersion = "1.0.0"

// Scope is the closed two-value enum for a source ValidationPolicy.
// EffectivePolicy has no Scope — it is the merged result of one or both.
type Scope string

const (
	ScopeGlobal Scope = "global"
	ScopeLocal  Scope = "local"
)

// ValidationPolicy mirrors the on-disk YAML form. Human-authored. The
// canonical schema is schemas/validation-policy.yaml.
type ValidationPolicy struct {
	SchemaVersion string                                  `json:"schema_version" yaml:"schema_version"`
	Scope         Scope                                   `json:"scope"          yaml:"scope"`
	PerArchetype  map[enums.Archetype]ArchetypeBlock      `json:"per_archetype"  yaml:"per_archetype"`
}

// ArchetypeBlock is a per-archetype declaration of which angles are
// obligatory, optional, or disabled. Within a single block the three
// lists are pairwise disjoint (loader rule 6).
type ArchetypeBlock struct {
	Obligatory []enums.ValidationAngle `json:"obligatory_angles" yaml:"obligatory_angles"`
	Optional   []enums.ValidationAngle `json:"optional_angles"   yaml:"optional_angles"`
	Disabled   []enums.ValidationAngle `json:"disabled_angles"   yaml:"disabled_angles"`
}

// EffectivePolicy is the resolved view of one or both source scopes.
// Derived; not human-authored. Per-(archetype, angle) map form lets
// Resolve express overrides without restating the archetype block.
type EffectivePolicy struct {
	SchemaVersion string                                                       `json:"schema_version" yaml:"schema_version"`
	ResolvedFrom  []string                                                     `json:"resolved_from"  yaml:"resolved_from"`
	PerArchetype  map[enums.Archetype]map[enums.ValidationAngle]AngleStatus    `json:"-"              yaml:"-"`
}

// AngleStatus is one of obligatory / optional / disabled. The zero value
// ("") represents "unset / no opinion" — used internally by Resolve and
// returned by Status when a (archetype, angle) pair has no policy coverage.
type AngleStatus string

const (
	StatusObligatory AngleStatus = "obligatory"
	StatusOptional   AngleStatus = "optional"
	StatusDisabled   AngleStatus = "disabled"
)
```

Note: `EffectivePolicy.PerArchetype` carries `json:"-" yaml:"-"` because its serialization is custom (`MarshalYAML` in Task 11 rebuilds the three-list form). It is not directly marshalable as a map.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/policy/...`
Expected: PASS — all five tests green.

- [ ] **Step 5: Commit**

```bash
git add internal/policy/types.go internal/policy/types_test.go
git commit -m "feat(e9): policy package skeleton and types

- Scope (global, local), AngleStatus (obligatory, optional, disabled, unset)
- ValidationPolicy mirrors YAML form; EffectivePolicy uses per-angle map"
```

---

## Task 4: Compile the JSON Schema from `schemas.FS`

Use the entrypoint pattern: read the schema from the shared embed, compile once, cache via `sync.Once`.

**Files:**
- Create: `internal/policy/schema.go`
- Create: `internal/policy/schema_test.go`

- [ ] **Step 1: Write the failing test**

`internal/policy/schema_test.go`:

```go
package policy

import "testing"

func TestCompiledSchemaIsAvailable(t *testing.T) {
	s, err := compiledSchema()
	if err != nil {
		t.Fatalf("compiledSchema: %v", err)
	}
	if s == nil {
		t.Fatal("compiledSchema: returned nil schema")
	}
}

func TestCompiledSchemaIsCached(t *testing.T) {
	a, err := compiledSchema()
	if err != nil {
		t.Fatalf("compiledSchema (first call): %v", err)
	}
	b, err := compiledSchema()
	if err != nil {
		t.Fatalf("compiledSchema (second call): %v", err)
	}
	if a != b {
		t.Fatal("compiledSchema returned different pointers on successive calls")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/policy/... -run TestCompiledSchema`
Expected: build error — `compiledSchema` undefined.

- [ ] **Step 3: Create `internal/policy/schema.go`**

```go
package policy

import (
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/schemas"
)

var (
	schemaOnce     sync.Once
	schemaCompiled *jsonschema.Schema
	schemaErr      error
)

// compiledSchema returns the compiled JSON Schema for ValidationPolicy.
// It loads schemas/validation-policy.yaml via the centralized schemas
// package's embed.FS on the first call and caches the result.
func compiledSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		raw, err := schemas.FS.ReadFile("validation-policy.yaml")
		if err != nil {
			schemaErr = fmt.Errorf("policy: read schema from schemas.FS: %w", err)
			return
		}
		var doc any
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			schemaErr = fmt.Errorf("policy: parse schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		const url = "https://lastro.dev/harness/schemas/validation-policy.yaml"
		if err := c.AddResource(url, doc); err != nil {
			schemaErr = fmt.Errorf("policy: add schema resource: %w", err)
			return
		}
		s, err := c.Compile(url)
		if err != nil {
			schemaErr = fmt.Errorf("policy: compile schema: %w", err)
			return
		}
		schemaCompiled = s
	})
	return schemaCompiled, schemaErr
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/policy/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/policy/schema.go internal/policy/schema_test.go
git commit -m "feat(e9): compile ValidationPolicy schema from shared embed"
```

---

## Task 5: Loader happy path (YAML → JSON → schema → struct)

Implement `Load(io.Reader)` covering rules 1–4 and 7–8 (everything JSON Schema can express). Rules 5 (matrix) and 6 (disjoint) come in Tasks 6 and 7.

**Files:**
- Create: `internal/policy/loader.go`
- Create: `internal/policy/loader_test.go`

- [ ] **Step 1: Write the failing happy-path test**

`internal/policy/loader_test.go`:

```go
package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func loadExample(t *testing.T, name string) *ValidationPolicy {
	t.Helper()
	path := filepath.Join("..", "..", "schemas", "examples", "validation-policy", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	p, err := Load(f)
	if err != nil {
		t.Fatalf("Load(%s): %v", path, err)
	}
	return p
}

func TestLoad_GlobalExample(t *testing.T) {
	p := loadExample(t, "global.yaml")
	if p.SchemaVersion != "1.0.0" {
		t.Errorf("SchemaVersion = %q, want 1.0.0", p.SchemaVersion)
	}
	if p.Scope != ScopeGlobal {
		t.Errorf("Scope = %q, want global", p.Scope)
	}
	block, ok := p.PerArchetype[enums.ArchetypeHTTPAPI]
	if !ok {
		t.Fatal("PerArchetype[http-api] missing")
	}
	if len(block.Obligatory) != 5 {
		t.Errorf("http-api obligatory count = %d, want 5", len(block.Obligatory))
	}
}

func TestLoad_LocalExample(t *testing.T) {
	p := loadExample(t, "local.yaml")
	if p.Scope != ScopeLocal {
		t.Errorf("Scope = %q, want local", p.Scope)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/policy/... -run TestLoad_`
Expected: build error — `Load` undefined.

- [ ] **Step 3: Create `internal/policy/loader.go`**

```go
package policy

import (
	"encoding/json"
	"fmt"
	"io"

	"sigs.k8s.io/yaml"
)

// Load parses a single ValidationPolicy from a YAML stream. The pipeline
// is read → YAML→JSON normalize → JSON Schema validate → json.Unmarshal →
// semantic validation. Semantic checks (applicable-angle matrix, disjoint
// lists) live in validateSemantics.
func Load(r io.Reader) (*ValidationPolicy, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("policy: read: %w", err)
	}
	asJSON, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("policy: yaml-to-json: %w", err)
	}
	if err := validateAgainstSchema(asJSON); err != nil {
		return nil, fmt.Errorf("policy: schema validation: %w", err)
	}
	var p ValidationPolicy
	if err := json.Unmarshal(asJSON, &p); err != nil {
		return nil, fmt.Errorf("policy: deserialize: %w", err)
	}
	if p.SchemaVersion != SupportedSchemaVersion {
		return nil, fmt.Errorf("policy: schema_version %q not supported (want %q)", p.SchemaVersion, SupportedSchemaVersion)
	}
	if err := validateSemantics(&p); err != nil {
		return nil, fmt.Errorf("policy: semantic validation: %w", err)
	}
	return &p, nil
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
	return s.Validate(instance)
}

// validateSemantics enforces loader rules 5 (applicable-angle matrix) and
// 6 (disjoint lists). Filled in by Tasks 6 and 7.
func validateSemantics(p *ValidationPolicy) error {
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/policy/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/policy/loader.go internal/policy/loader_test.go
git commit -m "feat(e9): loader happy path with schema+version validation"
```

---

## Task 6: Loader rule 5 — applicable-angle matrix cross-validation

Reject any `(archetype, angle)` pair not in `enums.ApplicableAngles[archetype]`. The check runs across all three lists (obligatory, optional, disabled) for every archetype block.

**Files:**
- Modify: `internal/policy/loader.go`
- Modify: `internal/policy/loader_test.go`
- Create: `internal/policy/testdata/inapplicable-angle.yaml`

- [ ] **Step 1: Create the fixture**

`internal/policy/testdata/inapplicable-angle.yaml`:

```yaml
# e2e-test is not applicable to library per E1's applicable-angle matrix.
schema_version: 1.0.0
scope: global
per_archetype:
  library:
    obligatory_angles: [build, e2e-test]
    optional_angles:   []
    disabled_angles:   []
```

- [ ] **Step 2: Write the failing test**

Append to `internal/policy/loader_test.go`:

```go
func loadTestdata(t *testing.T, name string) error {
	t.Helper()
	path := filepath.Join("testdata", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	_, err = Load(f)
	return err
}

func TestLoad_RejectsInapplicableAngle(t *testing.T) {
	err := loadTestdata(t, "inapplicable-angle.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := strings.ToLower(err.Error())
	for _, want := range []string{"library", "e2e-test", "not applicable"} {
		if !strings.Contains(msg, strings.ToLower(want)) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}
```

Add `"strings"` to the imports at the top of `loader_test.go` (it joins the existing `"os"`, `"path/filepath"`, `"testing"`, `"github.com/iurykrieger/lastro/internal/enums"` imports).

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/policy/... -run TestLoad_RejectsInapplicableAngle`
Expected: FAIL — `validateSemantics` returns nil; the load succeeds.

- [ ] **Step 4: Implement the matrix check in `validateSemantics`**

Replace the placeholder body in `internal/policy/loader.go`:

```go
func validateSemantics(p *ValidationPolicy) error {
	var errs []error
	for arch, block := range p.PerArchetype {
		errs = append(errs, checkApplicableAngles(arch, block)...)
	}
	return errors.Join(errs...)
}

func checkApplicableAngles(arch enums.Archetype, block ArchetypeBlock) []error {
	var errs []error
	lists := []struct {
		name   string
		angles []enums.ValidationAngle
	}{
		{"obligatory_angles", block.Obligatory},
		{"optional_angles", block.Optional},
		{"disabled_angles", block.Disabled},
	}
	for _, l := range lists {
		for _, a := range l.angles {
			if !enums.Applies(arch, a) {
				errs = append(errs, fmt.Errorf("archetype %q list %s: angle %q is not applicable", arch, l.name, a))
			}
		}
	}
	return errs
}
```

Add imports to `loader.go`:

```go
import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/enums"
)
```

- [ ] **Step 5: Run tests to verify pass**

Run: `go test ./internal/policy/...`
Expected: PASS — all loader tests including the new one.

- [ ] **Step 6: Commit**

```bash
git add internal/policy/loader.go internal/policy/loader_test.go internal/policy/testdata/inapplicable-angle.yaml
git commit -m "feat(e9): loader rejects inapplicable (archetype, angle) pairs"
```

---

## Task 7: Loader rule 6 — disjoint lists within an archetype block

Reject if the same angle appears in two of `obligatory_angles`, `optional_angles`, `disabled_angles` within one archetype block. Also rule 7: reject duplicates within a single list.

**Files:**
- Modify: `internal/policy/loader.go`
- Modify: `internal/policy/loader_test.go`
- Create: `internal/policy/testdata/overlapping-lists.yaml`
- Create: `internal/policy/testdata/duplicate-in-list.yaml`

- [ ] **Step 1: Create the fixtures**

`internal/policy/testdata/overlapping-lists.yaml`:

```yaml
schema_version: 1.0.0
scope: global
per_archetype:
  http-api:
    obligatory_angles: [build, logs]
    optional_angles:   []
    disabled_angles:   [logs]
```

`internal/policy/testdata/duplicate-in-list.yaml`:

```yaml
schema_version: 1.0.0
scope: global
per_archetype:
  http-api:
    obligatory_angles: [build, build]
    optional_angles:   []
    disabled_angles:   []
```

- [ ] **Step 2: Write the failing tests**

Append to `internal/policy/loader_test.go`:

```go
func TestLoad_RejectsOverlappingLists(t *testing.T) {
	err := loadTestdata(t, "overlapping-lists.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := strings.ToLower(err.Error())
	for _, want := range []string{"http-api", "logs", "overlap"} {
		if !strings.Contains(msg, strings.ToLower(want)) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestLoad_RejectsDuplicateInList(t *testing.T) {
	err := loadTestdata(t, "duplicate-in-list.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := strings.ToLower(err.Error())
	for _, want := range []string{"http-api", "build", "duplicate"} {
		if !strings.Contains(msg, strings.ToLower(want)) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/policy/... -run "TestLoad_Rejects(Overlapping|DuplicateInList)"`
Expected: both FAIL — no overlap/duplicate check yet.

- [ ] **Step 4: Extend `validateSemantics`**

Add to `internal/policy/loader.go`:

```go
func validateSemantics(p *ValidationPolicy) error {
	var errs []error
	for arch, block := range p.PerArchetype {
		errs = append(errs, checkApplicableAngles(arch, block)...)
		errs = append(errs, checkDuplicates(arch, block)...)
		errs = append(errs, checkDisjoint(arch, block)...)
	}
	return errors.Join(errs...)
}

func checkDuplicates(arch enums.Archetype, block ArchetypeBlock) []error {
	var errs []error
	lists := []struct {
		name   string
		angles []enums.ValidationAngle
	}{
		{"obligatory_angles", block.Obligatory},
		{"optional_angles", block.Optional},
		{"disabled_angles", block.Disabled},
	}
	for _, l := range lists {
		seen := make(map[enums.ValidationAngle]struct{}, len(l.angles))
		for _, a := range l.angles {
			if _, dup := seen[a]; dup {
				errs = append(errs, fmt.Errorf("archetype %q list %s: duplicate angle %q", arch, l.name, a))
				continue
			}
			seen[a] = struct{}{}
		}
	}
	return errs
}

func checkDisjoint(arch enums.Archetype, block ArchetypeBlock) []error {
	var errs []error
	pairs := []struct {
		aName, bName string
		a, b         []enums.ValidationAngle
	}{
		{"obligatory_angles", "optional_angles", block.Obligatory, block.Optional},
		{"obligatory_angles", "disabled_angles", block.Obligatory, block.Disabled},
		{"optional_angles", "disabled_angles", block.Optional, block.Disabled},
	}
	for _, p := range pairs {
		in := make(map[enums.ValidationAngle]struct{}, len(p.a))
		for _, a := range p.a {
			in[a] = struct{}{}
		}
		for _, b := range p.b {
			if _, ok := in[b]; ok {
				errs = append(errs, fmt.Errorf("archetype %q lists %s and %s overlap on angle %q", arch, p.aName, p.bName, b))
			}
		}
	}
	return errs
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/policy/...`
Expected: PASS — all loader tests green.

- [ ] **Step 6: Commit**

```bash
git add internal/policy/loader.go internal/policy/loader_test.go internal/policy/testdata/overlapping-lists.yaml internal/policy/testdata/duplicate-in-list.yaml
git commit -m "feat(e9): loader rejects duplicate and overlapping angles per archetype"
```

---

## Task 8: Loader negative coverage — schema-level rejections

Round out the negative test surface for rules already enforced by JSON Schema (rules 1, 2, 3, 4, 8). One small fixture per case; each one asserts the error mentions the offending field so authors get actionable diagnostics.

**Files:**
- Modify: `internal/policy/loader_test.go`
- Create the following testdata files under `internal/policy/testdata/`:
  - `missing-schema-version.yaml`
  - `unsupported-schema-version.yaml`
  - `unknown-scope.yaml`
  - `unknown-archetype.yaml`
  - `unknown-angle.yaml`
  - `unknown-top-field.yaml`
  - `unknown-block-field.yaml`

- [ ] **Step 1: Create the fixtures**

`missing-schema-version.yaml`:

```yaml
scope: global
per_archetype:
  cli:
    obligatory_angles: [build]
    optional_angles:   []
    disabled_angles:   []
```

`unsupported-schema-version.yaml`:

```yaml
schema_version: 9.9.9
scope: global
per_archetype:
  cli:
    obligatory_angles: [build]
    optional_angles:   []
    disabled_angles:   []
```

`unknown-scope.yaml`:

```yaml
schema_version: 1.0.0
scope: effective
per_archetype:
  cli:
    obligatory_angles: [build]
    optional_angles:   []
    disabled_angles:   []
```

`unknown-archetype.yaml`:

```yaml
schema_version: 1.0.0
scope: global
per_archetype:
  frobnicator:
    obligatory_angles: [build]
    optional_angles:   []
    disabled_angles:   []
```

`unknown-angle.yaml`:

```yaml
schema_version: 1.0.0
scope: global
per_archetype:
  cli:
    obligatory_angles: [not-a-real-angle]
    optional_angles:   []
    disabled_angles:   []
```

`unknown-top-field.yaml`:

```yaml
schema_version: 1.0.0
scope: global
inherits_from: legacy-policy-id
per_archetype:
  cli:
    obligatory_angles: [build]
    optional_angles:   []
    disabled_angles:   []
```

`unknown-block-field.yaml`:

```yaml
schema_version: 1.0.0
scope: global
per_archetype:
  cli:
    obligatory_angles: [build]
    optional_angles:   []
    disabled_angles:   []
    obligatorY_angles: [logs]   # typo with capital Y
```

- [ ] **Step 2: Write the failing parameterized test**

Append to `internal/policy/loader_test.go`:

```go
func TestLoad_RejectsSchemaViolations(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		wantSub string // case-insensitive substring expected in the error
	}{
		{"missing schema_version", "missing-schema-version.yaml", "schema_version"},
		{"unsupported schema_version", "unsupported-schema-version.yaml", "schema_version"},
		{"unknown scope", "unknown-scope.yaml", "scope"},
		{"unknown archetype", "unknown-archetype.yaml", "frobnicator"},
		{"unknown angle", "unknown-angle.yaml", "not-a-real-angle"},
		{"unknown top-level field", "unknown-top-field.yaml", "inherits_from"},
		{"unknown block field", "unknown-block-field.yaml", "obligatorY_angles"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := loadTestdata(t, tc.file)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantSub)) {
				t.Errorf("error %q missing %q", err.Error(), tc.wantSub)
			}
		})
	}
}
```

- [ ] **Step 3: Run the tests to verify they pass**

Run: `go test ./internal/policy/... -run TestLoad_RejectsSchemaViolations`
Expected: PASS — the existing loader pipeline already enforces all of these via JSON Schema (rules 1–4, 8) and the version check (`p.SchemaVersion != SupportedSchemaVersion`, rule 1).

If any case fails:
- For `unsupported schema_version`: confirm the error path in `Load` runs before semantic validation; the assertion `p.SchemaVersion != SupportedSchemaVersion` returns the error tagged with the rejected value.
- For `unknown-top-field` (`inherits_from`): the amended schema in Task 1 removed the property; `additionalProperties: false` produces the rejection. Confirm Task 1 landed.

- [ ] **Step 4: Commit**

```bash
git add internal/policy/loader_test.go internal/policy/testdata/
git commit -m "test(e9): exhaustive negative coverage for loader schema rules"
```

---

## Task 9: Resolve — pure merge with per-angle override

Implement `Resolve(global, local *ValidationPolicy) *EffectivePolicy`. Iterates `enums.ApplicableAngles` to ensure the loop scope matches what the loader admits.

**Files:**
- Create: `internal/policy/resolve.go`
- Create: `internal/policy/resolve_test.go`

- [ ] **Step 1: Write the failing test suite**

`internal/policy/resolve_test.go`:

```go
package policy

import (
	"reflect"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

// fixtureGlobal returns a hand-built ValidationPolicy that mirrors the
// global example, used as a building block for the resolver tests.
func fixtureGlobal() *ValidationPolicy {
	return &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeGlobal,
		PerArchetype: map[enums.Archetype]ArchetypeBlock{
			enums.ArchetypeHTTPAPI: {
				Obligatory: []enums.ValidationAngle{enums.AngleBuild, enums.AngleSecurity},
				Optional:   []enums.ValidationAngle{enums.AnglePerformance},
				Disabled:   []enums.ValidationAngle{},
			},
		},
	}
}

func TestResolve_BothNil(t *testing.T) {
	got := Resolve(nil, nil)
	if got == nil {
		t.Fatal("Resolve(nil, nil) returned nil; want empty *EffectivePolicy")
	}
	if got.SchemaVersion != SupportedSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", got.SchemaVersion, SupportedSchemaVersion)
	}
	if len(got.ResolvedFrom) != 0 {
		t.Errorf("ResolvedFrom = %v, want []", got.ResolvedFrom)
	}
	if len(got.PerArchetype) != 0 {
		t.Errorf("PerArchetype size = %d, want 0", len(got.PerArchetype))
	}
}

func TestResolve_LocalNil(t *testing.T) {
	got := Resolve(fixtureGlobal(), nil)
	if !reflect.DeepEqual(got.ResolvedFrom, []string{"global"}) {
		t.Errorf("ResolvedFrom = %v, want [global]", got.ResolvedFrom)
	}
	if got.PerArchetype[enums.ArchetypeHTTPAPI][enums.AngleBuild] != StatusObligatory {
		t.Error("http-api build should be obligatory")
	}
	if got.PerArchetype[enums.ArchetypeHTTPAPI][enums.AnglePerformance] != StatusOptional {
		t.Error("http-api performance should be optional")
	}
}

func TestResolve_GlobalNil(t *testing.T) {
	local := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeLocal,
		PerArchetype: map[enums.Archetype]ArchetypeBlock{
			enums.ArchetypeCLI: {
				Obligatory: []enums.ValidationAngle{enums.AngleBuild},
				Optional:   []enums.ValidationAngle{},
				Disabled:   []enums.ValidationAngle{},
			},
		},
	}
	got := Resolve(nil, local)
	if !reflect.DeepEqual(got.ResolvedFrom, []string{"local"}) {
		t.Errorf("ResolvedFrom = %v, want [local]", got.ResolvedFrom)
	}
	if got.PerArchetype[enums.ArchetypeCLI][enums.AngleBuild] != StatusObligatory {
		t.Error("cli build should be obligatory")
	}
}

func TestResolve_PerAngleOverride(t *testing.T) {
	// global says performance is optional; local flips it to obligatory
	// without restating the archetype block. Local must win.
	local := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeLocal,
		PerArchetype: map[enums.Archetype]ArchetypeBlock{
			enums.ArchetypeHTTPAPI: {
				Obligatory: []enums.ValidationAngle{enums.AnglePerformance},
				Optional:   []enums.ValidationAngle{},
				Disabled:   []enums.ValidationAngle{},
			},
		},
	}
	got := Resolve(fixtureGlobal(), local)
	if got.PerArchetype[enums.ArchetypeHTTPAPI][enums.AnglePerformance] != StatusObligatory {
		t.Error("performance should be obligatory after local override")
	}
	// global's build entry must survive — local didn't mention it.
	if got.PerArchetype[enums.ArchetypeHTTPAPI][enums.AngleBuild] != StatusObligatory {
		t.Error("build should remain obligatory from global")
	}
	if !reflect.DeepEqual(got.ResolvedFrom, []string{"global", "local"}) {
		t.Errorf("ResolvedFrom = %v, want [global local]", got.ResolvedFrom)
	}
}

func TestResolve_LocalDisablesObligatory(t *testing.T) {
	local := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeLocal,
		PerArchetype: map[enums.Archetype]ArchetypeBlock{
			enums.ArchetypeHTTPAPI: {
				Obligatory: []enums.ValidationAngle{},
				Optional:   []enums.ValidationAngle{},
				Disabled:   []enums.ValidationAngle{enums.AngleBuild},
			},
		},
	}
	got := Resolve(fixtureGlobal(), local)
	if got.PerArchetype[enums.ArchetypeHTTPAPI][enums.AngleBuild] != StatusDisabled {
		t.Error("build should be disabled after local disables it")
	}
}

func TestResolve_LocalIntroducesNewArchetype(t *testing.T) {
	local := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeLocal,
		PerArchetype: map[enums.Archetype]ArchetypeBlock{
			enums.ArchetypeCLI: {
				Obligatory: []enums.ValidationAngle{enums.AngleBuild},
				Optional:   []enums.ValidationAngle{},
				Disabled:   []enums.ValidationAngle{},
			},
		},
	}
	got := Resolve(fixtureGlobal(), local)
	if got.PerArchetype[enums.ArchetypeCLI][enums.AngleBuild] != StatusObligatory {
		t.Error("cli build should appear in effective from local-only archetype")
	}
	// http-api from global is still present.
	if got.PerArchetype[enums.ArchetypeHTTPAPI][enums.AngleBuild] != StatusObligatory {
		t.Error("http-api build should remain from global")
	}
}

func TestResolve_GlobalDisablesLocalSilent(t *testing.T) {
	global := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeGlobal,
		PerArchetype: map[enums.Archetype]ArchetypeBlock{
			enums.ArchetypeCLI: {
				Obligatory: []enums.ValidationAngle{},
				Optional:   []enums.ValidationAngle{},
				Disabled:   []enums.ValidationAngle{enums.AngleE2ETest},
			},
		},
	}
	local := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeLocal,
		PerArchetype:  map[enums.Archetype]ArchetypeBlock{}, // silent on cli
	}
	got := Resolve(global, local)
	// cli + e2e-test is not in enums.ApplicableAngles[cli], so the resolver
	// must not record any status for that pair regardless of source intent.
	if _, present := got.PerArchetype[enums.ArchetypeCLI][enums.AngleE2ETest]; present {
		t.Error("cli e2e-test should be absent (not applicable per E1 matrix)")
	}
}
```

Note: the last case (`TestResolve_GlobalDisablesLocalSilent`) intentionally constructs a `ValidationPolicy` that would fail the loader (e2e-test not applicable to cli). It is exercising `Resolve` in isolation — `Resolve` trusts the loader has already vetted inputs, so it iterates only `enums.ApplicableAngles[arch]` and silently drops anything outside that set. This is the contract documented in spec §6.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/policy/... -run TestResolve_`
Expected: build error — `Resolve` undefined.

- [ ] **Step 3: Create `internal/policy/resolve.go`**

```go
package policy

import "github.com/iurykrieger/lastro/internal/enums"

// policySource pairs a *ValidationPolicy with its scope name for
// ResolvedFrom tracking. Unexported; an implementation detail of Resolve.
type policySource struct {
	name string
	pol  *ValidationPolicy
}

// Resolve merges a global default and a local override into a single
// EffectivePolicy. Override granularity is per (archetype, angle): local
// wins for any angle it mentions; angles local omits inherit from global.
// Either argument may be nil. Both nil yields an empty *EffectivePolicy
// with ResolvedFrom = [].
//
// The resolver iterates only enums.ApplicableAngles[archetype]; any angle
// outside that matrix is ignored even if a source carries it. The loader
// already rejects inapplicable pairs at load time.
func Resolve(global, local *ValidationPolicy) *EffectivePolicy {
	var sources []policySource
	if global != nil {
		sources = append(sources, policySource{"global", global})
	}
	if local != nil {
		sources = append(sources, policySource{"local", local})
	}

	resolvedFrom := make([]string, 0, len(sources))
	for _, s := range sources {
		resolvedFrom = append(resolvedFrom, s.name)
	}

	eff := &EffectivePolicy{
		SchemaVersion: SupportedSchemaVersion,
		ResolvedFrom:  resolvedFrom,
		PerArchetype:  map[enums.Archetype]map[enums.ValidationAngle]AngleStatus{},
	}

	archetypes := unionArchetypes(sources)
	for _, arch := range archetypes {
		for _, angle := range enums.ApplicableAngles[arch] {
			status := AngleStatus("")
			for _, s := range sources {
				block, ok := s.pol.PerArchetype[arch]
				if !ok {
					continue
				}
				switch {
				case containsAngle(block.Obligatory, angle):
					status = StatusObligatory
				case containsAngle(block.Optional, angle):
					status = StatusOptional
				case containsAngle(block.Disabled, angle):
					status = StatusDisabled
					// absent from this source's block: leave status unchanged
				}
			}
			if status == "" {
				continue
			}
			if eff.PerArchetype[arch] == nil {
				eff.PerArchetype[arch] = map[enums.ValidationAngle]AngleStatus{}
			}
			eff.PerArchetype[arch][angle] = status
		}
	}
	return eff
}

func unionArchetypes(sources []policySource) []enums.Archetype {
	seen := map[enums.Archetype]struct{}{}
	for _, s := range sources {
		for arch := range s.pol.PerArchetype {
			seen[arch] = struct{}{}
		}
	}
	// Iterate enums.AllArchetypes() for deterministic order.
	out := make([]enums.Archetype, 0, len(seen))
	for _, arch := range enums.AllArchetypes() {
		if _, ok := seen[arch]; ok {
			out = append(out, arch)
		}
	}
	return out
}

func containsAngle(list []enums.ValidationAngle, target enums.ValidationAngle) bool {
	for _, a := range list {
		if a == target {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/policy/...`
Expected: PASS — all six resolver tests plus all loader/types tests.

- [ ] **Step 5: Commit**

```bash
git add internal/policy/resolve.go internal/policy/resolve_test.go
git commit -m "feat(e9): Resolve merges global+local with per-angle override"
```

---

## Task 10: Lookup API — AnglesFor and Status

Sensor generation calls `AnglesFor`; reports and audit tooling call `Status`. Both must produce deterministic output.

**Files:**
- Create: `internal/policy/lookup.go`
- Create: `internal/policy/lookup_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/policy/lookup_test.go`:

```go
package policy

import (
	"reflect"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func effectiveFixture() *EffectivePolicy {
	return &EffectivePolicy{
		SchemaVersion: SupportedSchemaVersion,
		ResolvedFrom:  []string{"global", "local"},
		PerArchetype: map[enums.Archetype]map[enums.ValidationAngle]AngleStatus{
			enums.ArchetypeHTTPAPI: {
				enums.AngleBuild:       StatusObligatory,
				enums.AngleSecurity:    StatusObligatory,
				enums.AnglePerformance: StatusOptional,
				enums.AngleLogs:        StatusOptional,
				enums.AngleE2ETest:     StatusDisabled,
			},
		},
	}
}

func TestAnglesFor_ReturnsSortedObligatoryAndOptional(t *testing.T) {
	p := effectiveFixture()
	got, optional := p.AnglesFor(enums.ArchetypeHTTPAPI)
	wantObligatory := []enums.ValidationAngle{enums.AngleBuild, enums.AngleSecurity}
	wantOptional := []enums.ValidationAngle{enums.AngleLogs, enums.AnglePerformance}
	if !reflect.DeepEqual(got, wantObligatory) {
		t.Errorf("obligatory = %v, want %v", got, wantObligatory)
	}
	if !reflect.DeepEqual(optional, wantOptional) {
		t.Errorf("optional = %v, want %v", optional, wantOptional)
	}
}

func TestAnglesFor_ExcludesDisabledAndUnset(t *testing.T) {
	p := effectiveFixture()
	obligatory, optional := p.AnglesFor(enums.ArchetypeHTTPAPI)
	for _, a := range obligatory {
		if a == enums.AngleE2ETest {
			t.Error("disabled angle e2e-test must not appear in obligatory")
		}
	}
	for _, a := range optional {
		if a == enums.AngleE2ETest {
			t.Error("disabled angle e2e-test must not appear in optional")
		}
	}
}

func TestAnglesFor_ReturnsEmptyNotNilForUnknownArchetype(t *testing.T) {
	p := effectiveFixture()
	obligatory, optional := p.AnglesFor(enums.ArchetypeCLI)
	if obligatory == nil || optional == nil {
		t.Fatal("AnglesFor must return empty slices, not nil")
	}
	if len(obligatory) != 0 || len(optional) != 0 {
		t.Errorf("expected empty slices, got obligatory=%v optional=%v", obligatory, optional)
	}
}

func TestStatus_ReturnsConfiguredValues(t *testing.T) {
	p := effectiveFixture()
	cases := []struct {
		arch  enums.Archetype
		angle enums.ValidationAngle
		want  AngleStatus
	}{
		{enums.ArchetypeHTTPAPI, enums.AngleBuild, StatusObligatory},
		{enums.ArchetypeHTTPAPI, enums.AnglePerformance, StatusOptional},
		{enums.ArchetypeHTTPAPI, enums.AngleE2ETest, StatusDisabled},
		{enums.ArchetypeHTTPAPI, enums.AngleContracts, ""},  // unset
		{enums.ArchetypeCLI, enums.AngleBuild, ""},          // unconfigured archetype
	}
	for _, tc := range cases {
		got := p.Status(tc.arch, tc.angle)
		if got != tc.want {
			t.Errorf("Status(%s, %s) = %q, want %q", tc.arch, tc.angle, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/policy/... -run "TestAnglesFor_|TestStatus_"`
Expected: build error — `AnglesFor` and `Status` undefined.

- [ ] **Step 3: Create `internal/policy/lookup.go`**

```go
package policy

import (
	"sort"

	"github.com/iurykrieger/lastro/internal/enums"
)

// AnglesFor returns the obligatory and optional angles for the given
// archetype, sorted alphabetically by underlying string for deterministic
// output. Disabled and unset angles are both excluded — sensor generation
// treats them identically (no sensor generated). The returned slices are
// always non-nil; an unconfigured archetype yields two empty slices.
func (p *EffectivePolicy) AnglesFor(a enums.Archetype) (obligatory, optional []enums.ValidationAngle) {
	obligatory = []enums.ValidationAngle{}
	optional = []enums.ValidationAngle{}
	if p == nil {
		return
	}
	block, ok := p.PerArchetype[a]
	if !ok {
		return
	}
	for angle, status := range block {
		switch status {
		case StatusObligatory:
			obligatory = append(obligatory, angle)
		case StatusOptional:
			optional = append(optional, angle)
		}
	}
	sort.Slice(obligatory, func(i, j int) bool { return obligatory[i] < obligatory[j] })
	sort.Slice(optional, func(i, j int) bool { return optional[i] < optional[j] })
	return
}

// Status returns the AngleStatus configured for the given (archetype,
// angle) pair, or "" (the unset sentinel) if no scope mentioned it.
// Lets callers distinguish "disabled by policy" from "no policy coverage".
func (p *EffectivePolicy) Status(a enums.Archetype, angle enums.ValidationAngle) AngleStatus {
	if p == nil {
		return ""
	}
	block, ok := p.PerArchetype[a]
	if !ok {
		return ""
	}
	return block[angle]
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/policy/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/policy/lookup.go internal/policy/lookup_test.go
git commit -m "feat(e9): AnglesFor and Status lookups on EffectivePolicy"
```

---

## Task 11: MarshalYAML — deterministic audit dump

Emit YAML with `resolved_from:` (no `scope:`), archetypes alphabetical, angles alphabetical, empty lists as `[]`.

**Files:**
- Create: `internal/policy/serialize.go`
- Create: `internal/policy/serialize_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/policy/serialize_test.go`:

```go
package policy

import (
	"bytes"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestMarshalYAML_ContainsResolvedFromAndNoScope(t *testing.T) {
	p := effectiveFixture()
	out, err := p.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	if !bytes.Contains(out, []byte("resolved_from:")) {
		t.Errorf("output missing resolved_from:\n%s", out)
	}
	if bytes.Contains(out, []byte("scope:")) {
		t.Errorf("output must not contain scope: but did:\n%s", out)
	}
}

func TestMarshalYAML_IsDeterministic(t *testing.T) {
	p := effectiveFixture()
	a, err := p.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML a: %v", err)
	}
	b, err := p.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("MarshalYAML is non-deterministic.\nA:\n%s\nB:\n%s", a, b)
	}
}

func TestMarshalYAML_SortsAnglesAndArchetypes(t *testing.T) {
	p := &EffectivePolicy{
		SchemaVersion: SupportedSchemaVersion,
		ResolvedFrom:  []string{"global", "local"},
		PerArchetype: map[enums.Archetype]map[enums.ValidationAngle]AngleStatus{
			enums.ArchetypeHTTPAPI: {
				enums.AngleSecurity: StatusObligatory,
				enums.AngleBuild:    StatusObligatory,
			},
			enums.ArchetypeCLI: {
				enums.AngleBuild: StatusObligatory,
			},
		},
	}
	out, err := p.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	s := string(out)
	// "cli" must appear before "http-api" (alphabetical archetype order).
	cliIdx := strings.Index(s, "cli:")
	httpIdx := strings.Index(s, "http-api:")
	if cliIdx < 0 || httpIdx < 0 || cliIdx >= httpIdx {
		t.Errorf("archetypes not alphabetical:\n%s", s)
	}
	// Within http-api obligatory, "build" must appear before "security".
	httpBlock := s[httpIdx:]
	buildIdx := strings.Index(httpBlock, "build")
	secIdx := strings.Index(httpBlock, "security")
	if buildIdx < 0 || secIdx < 0 || buildIdx >= secIdx {
		t.Errorf("angles in http-api obligatory not alphabetical:\n%s", httpBlock)
	}
}

func TestMarshalYAML_EmptyListsRenderedAsBrackets(t *testing.T) {
	p := &EffectivePolicy{
		SchemaVersion: SupportedSchemaVersion,
		ResolvedFrom:  []string{"global"},
		PerArchetype: map[enums.Archetype]map[enums.ValidationAngle]AngleStatus{
			enums.ArchetypeCLI: {
				enums.AngleBuild: StatusObligatory,
			},
		},
	}
	out, err := p.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	s := string(out)
	// The cli block has no optional and no disabled — they should be []
	// not omitted, so downstream readers see explicit emptiness.
	if !strings.Contains(s, "optional_angles: []") {
		t.Errorf("optional_angles: [] missing:\n%s", s)
	}
	if !strings.Contains(s, "disabled_angles: []") {
		t.Errorf("disabled_angles: [] missing:\n%s", s)
	}
}

func TestMarshalYAML_RoundTripIntoLoadFails(t *testing.T) {
	p := effectiveFixture()
	out, err := p.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	_, err = Load(bytes.NewReader(out))
	if err == nil {
		t.Fatal("Load(MarshalYAML(effective)) succeeded; should have failed (effective dumps are not re-ingestable)")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/policy/... -run TestMarshalYAML_`
Expected: build error — `MarshalYAML` undefined.

- [ ] **Step 3: Create `internal/policy/serialize.go`**

```go
package policy

import (
	"fmt"
	"sort"

	"sigs.k8s.io/yaml"
)

// MarshalYAML renders the EffectivePolicy as audit-friendly YAML. Output
// is deterministic: archetypes sorted alphabetically, angles sorted
// alphabetically within each list, empty lists emitted as []. No scope:
// field; resolved_from: is the discriminator that distinguishes a
// resolved view from a source policy.
//
// The output intentionally cannot be loaded back through Load. Source
// policies are human-authored; effective dumps are derived artifacts.
func (p *EffectivePolicy) MarshalYAML() ([]byte, error) {
	type blockOut struct {
		Obligatory []string `json:"obligatory_angles" yaml:"obligatory_angles"`
		Optional   []string `json:"optional_angles"   yaml:"optional_angles"`
		Disabled   []string `json:"disabled_angles"   yaml:"disabled_angles"`
	}
	type docOut struct {
		SchemaVersion string              `json:"schema_version" yaml:"schema_version"`
		ResolvedFrom  []string            `json:"resolved_from"  yaml:"resolved_from"`
		PerArchetype  map[string]blockOut `json:"per_archetype"  yaml:"per_archetype"`
	}

	doc := docOut{
		SchemaVersion: p.SchemaVersion,
		ResolvedFrom:  append([]string{}, p.ResolvedFrom...),
		PerArchetype:  map[string]blockOut{},
	}

	for arch, block := range p.PerArchetype {
		out := blockOut{
			Obligatory: []string{},
			Optional:   []string{},
			Disabled:   []string{},
		}
		for angle, status := range block {
			switch status {
			case StatusObligatory:
				out.Obligatory = append(out.Obligatory, string(angle))
			case StatusOptional:
				out.Optional = append(out.Optional, string(angle))
			case StatusDisabled:
				out.Disabled = append(out.Disabled, string(angle))
			}
		}
		sort.Strings(out.Obligatory)
		sort.Strings(out.Optional)
		sort.Strings(out.Disabled)
		doc.PerArchetype[string(arch)] = out
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("policy: marshal effective: %w", err)
	}
	return raw, nil
}
```

Note: `sigs.k8s.io/yaml.Marshal` round-trips through JSON, so map keys are emitted in JSON encoding order (lexicographic for strings) — that gives alphabetical archetype ordering automatically. `enums.Archetype` values become `string` via `string(arch)` (defined-type conversion); the `enums` package itself is not imported by `serialize.go`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/policy/...`
Expected: PASS — all five MarshalYAML tests plus all previous tests.

If `TestMarshalYAML_RoundTripIntoLoadFails` is the only failure: that means our amended schema (Task 1) is accepting the effective dump as a valid source policy. The most likely cause is a missing `additionalProperties: false` on the top level, which would let `resolved_from` pass. Re-verify Task 1's schema body keeps `additionalProperties: false` at the document root.

- [ ] **Step 5: Commit**

```bash
git add internal/policy/serialize.go internal/policy/serialize_test.go
git commit -m "feat(e9): MarshalYAML emits deterministic effective-policy dump"
```

---

## Task 12: Whole-package green and lint

Final sweep to confirm the package builds cleanly, all tests pass, no unused imports, no vet warnings.

- [ ] **Step 1: Run the whole suite**

Run: `go test ./...`
Expected: PASS for the entire module.

- [ ] **Step 2: Run go vet**

Run: `go vet ./internal/policy/...`
Expected: clean.

- [ ] **Step 3: Confirm package boundary**

Run: `go list -deps ./internal/policy/... | grep -v '^github.com/iurykrieger/lastro' | sort -u`

Expected (modulo Go stdlib):
- `github.com/santhosh-tekuri/jsonschema/v6`
- `sigs.k8s.io/yaml`
- `gopkg.in/yaml.v2` (indirect via sigs.k8s.io/yaml — fine)

No filesystem-bound runtime dependencies. No `os.*` calls in `loader.go`, `resolve.go`, `lookup.go`, or `serialize.go` (only in tests, where `os.Open` is used to read example fixtures).

- [ ] **Step 4: Final commit (only if there were lint fixes)**

If steps 1–3 surfaced any cleanup, commit it as:

```bash
git add internal/policy/
git commit -m "chore(e9): lint and dependency sweep"
```

If nothing changed, skip this step — no empty commits.

---

## Spec coverage check

| Spec section | Implemented by |
|---|---|
| §3.1 Two scopes | Tasks 1, 3 |
| §3.2 Per-angle override | Task 9 |
| §3.3 Hard reject inapplicable | Task 6 |
| §3.4 `inherits_from` dropped | Tasks 1, 2, 8 |
| §3.5 Effective serialization marker | Task 11 |
| §4 Types | Task 3 |
| §5 Loader rules 1–8 | Tasks 5 (1, 3, 4, 8), 6 (5), 7 (6, 7), 8 (1–4, 8 coverage), and the `SchemaVersion != Supported` check in Task 5 (rule 1) |
| §6 Resolve algorithm | Task 9 |
| §7 Lookup API | Task 10 |
| §8 Effective serialization | Task 11 |
| §9 Testing strategy | Distributed across all tasks; whole-package sweep in Task 12 |
| §10 Dependencies | imports established in Tasks 3, 4, 5, 9 |
| §11 Schema amendment | Tasks 1, 2 |
| §12 `SchemaVersion` propagation | Resolved by `SupportedSchemaVersion` constant in Task 3, used in Task 9 |
