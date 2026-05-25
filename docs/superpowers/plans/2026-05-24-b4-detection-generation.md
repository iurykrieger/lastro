# B4 — Detection & Generation Skills Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship three LLM-driven Claude Code slash commands (`/detect-stack`, `/detect-use-cases`, `/create-sensors`) backed by per-entity Go `Persist` functions that validate against existing Phase A schemas and write atomically to `.harness/`.

**Architecture:** Repository becomes a Claude Code plugin. Each skill is `skills/<name>/SKILL.md` (prompt body, ≤200 lines) + `skills/<name>/scripts/main.go` (tiny wrapper, ~30–60 LOC). The wrappers call new `Persist` functions on the existing Phase A entity packages (`internal/stack`, `internal/fixture`, `internal/usecase`, `internal/sensor`). Each `Persist`: validates schema, validates cross-entity invariants against on-disk `.harness/`, patch-bumps `schema_version`, writes atomically via temp-file + rename. Structured errors go through a new shared `internal/persisterror` package and are JSON-encoded to stdout by the wrappers.

**Tech Stack:** Go 1.24.2, `sigs.k8s.io/yaml`, `github.com/santhosh-tekuri/jsonschema/v6`, embedded schemas via `schemas.FS`. Module path: `github.com/iurykrieger/lastro`.

**Branching (run before Task 1):**
```bash
git fetch origin
git checkout -b feat/b4-detection-generation origin/main
```

---

## File map

**New files (created in this plan):**
- `internal/persisterror/persisterror.go`
- `internal/persisterror/persisterror_test.go`
- `internal/stack/persist.go`
- `internal/stack/persist_test.go`
- `internal/fixture/persist.go`
- `internal/fixture/persist_test.go`
- `internal/usecase/persist.go`
- `internal/usecase/persist_test.go`
- `internal/sensor/persist.go`
- `internal/sensor/persist_test.go`
- `plugin.json` (Claude Code plugin manifest at repo root)
- `skills/detect-stack/SKILL.md`
- `skills/detect-stack/scripts/main.go`
- `skills/detect-stack/scripts/main_test.go`
- `skills/detect-use-cases/SKILL.md`
- `skills/detect-use-cases/scripts/main.go`
- `skills/detect-use-cases/scripts/main_test.go`
- `skills/create-sensors/SKILL.md`
- `skills/create-sensors/scripts/main.go`
- `skills/create-sensors/scripts/main_test.go`

**Existing files modified:**
- `schemas/stack-manifest.yaml` — add `applicable_angles` to schema
- `schemas/examples/stack-manifest/http-api.yaml` — add `applicable_angles` to example (so loader test still passes)
- `internal/stack/types.go` — add `ApplicableAngles []enums.ValidationAngle` field
- `internal/stack/validate.go` — relax `schema_version` strict-equality to `1.x.x` range; validate `applicable_angles` non-empty
- `internal/stack/load.go` — extract `LoadBytes(b []byte) (StackManifest, error)` helper from `Load(path)`
- `internal/usecase/loader.go` — relax `supportedSchemaVersion` strict-equality to `2.x.x` range
- `internal/fixture/loader.go` — extract `LoadFixtureBytes(b []byte) (Fixture, error)` if needed by `fixture.Persist`; relax schema-version handling if any
- `internal/sensor/loader.go` — extract `LoadSensorBytes(b []byte) (Sensor, error)` from `LoadSensor(path)`; relax schema-version handling if any

---

## Phase 0 — `internal/persisterror` foundation

The shared structured error type. Every `Persist` returns this; every skill script JSON-encodes it.

### Task 0.1: Create the persisterror package

**Files:**
- Create: `internal/persisterror/persisterror.go`

- [ ] **Step 1: Write the package file**

```go
// Package persisterror defines the structured error returned by every
// entity Persist function in this framework. Skill scripts type-assert
// these and JSON-encode them to stdout so the slash-command body's
// retry loop can read them.
package persisterror

import "fmt"

// Kind discriminates the failure mode so the retry prompt can switch on
// it. Every Persist returns one of these.
type Kind string

const (
	SchemaViolation      Kind = "schema_violation"
	FixtureBinding       Kind = "fixture_binding"
	Grounding            Kind = "grounding"
	TemplateResolution   Kind = "template_resolution"
	MissingRequiredField Kind = "missing_required_field"
	UnknownEnumValue     Kind = "unknown_enum_value"
	AngleNotApplicable   Kind = "angle_not_applicable"
	MissingDependency    Kind = "missing_dependency"
)

// Error is the structured error type. Skill scripts marshal it to JSON.
// Path is YAML JSONPath (best effort; may be empty when the upstream
// validator doesn't surface a path).
type Error struct {
	Kind       Kind           `json:"kind"`
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id,omitempty"`
	File       string         `json:"file,omitempty"`
	Path       string         `json:"path,omitempty"`
	Value      any            `json:"value,omitempty"`
	Expected   string         `json:"expected,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	Message    string         `json:"message"`
}

func (e *Error) Error() string {
	if e.EntityID != "" {
		return fmt.Sprintf("%s on %s %q: %s", e.Kind, e.EntityType, e.EntityID, e.Message)
	}
	return fmt.Sprintf("%s on %s: %s", e.Kind, e.EntityType, e.Message)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/persisterror/`
Expected: no output, exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/persisterror/persisterror.go
git commit -m "feat(persisterror): introduce shared structured error for Persist functions"
```

### Task 0.2: Test persisterror

**Files:**
- Create: `internal/persisterror/persisterror_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package persisterror

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestError_ImplementsError(t *testing.T) {
	var _ error = (*Error)(nil)
}

func TestError_MessageWithID(t *testing.T) {
	e := &Error{Kind: SchemaViolation, EntityType: "sensor", EntityID: "s1", Message: "bad"}
	got := e.Error()
	want := `schema_violation on sensor "s1": bad`
	if got != want {
		t.Fatalf("Error()=%q, want %q", got, want)
	}
}

func TestError_MessageWithoutID(t *testing.T) {
	e := &Error{Kind: SchemaViolation, EntityType: "stack-manifest", Message: "bad"}
	got := e.Error()
	want := "schema_violation on stack-manifest: bad"
	if got != want {
		t.Fatalf("Error()=%q, want %q", got, want)
	}
}

func TestError_JSONRoundTrip(t *testing.T) {
	in := &Error{
		Kind:       FixtureBinding,
		EntityType: "use-case",
		EntityID:   "uc1",
		File:       "/tmp/uc.yaml",
		Path:       "fixture_ids[1]",
		Value:      "fx_missing",
		Expected:   "id present in .harness/fixtures/",
		Details:    map[string]any{"missing_fixture_ids": []any{"fx_missing"}},
		Message:    "fixture not found",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Error
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Kind != in.Kind || out.EntityType != in.EntityType ||
		out.EntityID != in.EntityID || out.File != in.File ||
		out.Path != in.Path || out.Expected != in.Expected ||
		out.Message != in.Message {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", out, in)
	}
}

func TestError_ErrorsAs(t *testing.T) {
	var wrapped error = &Error{Kind: Grounding, EntityType: "sensor", Message: "x"}
	var pe *Error
	if !errors.As(wrapped, &pe) {
		t.Fatal("errors.As failed to extract *Error")
	}
	if pe.Kind != Grounding {
		t.Fatalf("Kind=%q, want %q", pe.Kind, Grounding)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/persisterror/ -v`
Expected: all four tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/persisterror/persisterror_test.go
git commit -m "test(persisterror): cover Error formatting, JSON round-trip, errors.As"
```

---

## Phase 1 — Relax schema-version checks on Phase A loaders

Persist patch-bumps `schema_version` on every re-emit, so loaders must accept any `<major>.x.x` value, not just the original constant. Each entity package has a different strictness today; relax each in place.

### Task 1.1: Add semver helper in persisterror (used by all relaxation tasks)

Actually keep the version comparison local to each entity package — it's a 5-line check and keeping it skill-local avoids creating a shared semver module for one use. Skip this task.

### Task 1.2: Relax `internal/stack` schema_version

**Files:**
- Modify: `internal/stack/validate.go:21-23` (component check) and `internal/stack/validate.go:76-79` (manifest check)

Current behavior: both reject anything that is not exactly `"1.0.0"`.

- [ ] **Step 1: Write a failing test**

Add to a new test file `internal/stack/version_test.go`:

```go
package stack

import (
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestStackManifest_Validate_AcceptsPatchBumpedVersion(t *testing.T) {
	m := StackManifest{
		SchemaVersion: "1.0.7",
		Archetype:     enums.ArchetypeHTTPAPI,
		Components: []StackComponent{{
			SchemaVersion:     "1.0.7",
			ID:                "express",
			Kind:              enums.StackKindLibrary,
			Name:              "express",
			Version:           "4.18.0",
			Capabilities:      []string{"http-routing"},
			DetectionEvidence: []EvidenceRef{{File: "package.json", Path: ".dependencies.express"}},
		}},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate rejected patch-bumped version: %v", err)
	}
}

func TestStackManifest_Validate_RejectsDifferentMajor(t *testing.T) {
	m := StackManifest{
		SchemaVersion: "2.0.0",
		Archetype:     enums.ArchetypeHTTPAPI,
		Components: []StackComponent{{
			SchemaVersion:     "2.0.0",
			ID:                "express",
			Kind:              enums.StackKindLibrary,
			Name:              "express",
			Version:           "4.18.0",
			Capabilities:      []string{"http-routing"},
			DetectionEvidence: []EvidenceRef{{File: "package.json", Path: ".dependencies.express"}},
		}},
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("Validate accepted incompatible major version")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("error should mention schema_version, got: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/stack/ -run Version -v`
Expected: `TestStackManifest_Validate_AcceptsPatchBumpedVersion` FAILs (current code rejects "1.0.7" with `got "1.0.7", want "1.0.0"`).

- [ ] **Step 3: Implement the relaxation**

Edit `internal/stack/validate.go`. Replace the component-level check at lines 21–23:

```go
	if !schemaVersionCompatible(c.SchemaVersion) {
		problems = append(problems,
			fmt.Sprintf("schema_version: got %q, want major.minor.patch matching %s.x", c.SchemaVersion, supportedMajorMinor))
	}
```

And the manifest-level check at lines 76–79:

```go
	if !schemaVersionCompatible(m.SchemaVersion) {
		problems = append(problems,
			fmt.Sprintf("schema_version: got %q, want major.minor.patch matching %s.x", m.SchemaVersion, supportedMajorMinor))
	}
```

Add the helper at the bottom of `validate.go`:

```go
// supportedMajorMinor is the "major.minor" prefix this loader accepts.
// Persist patch-bumps schema_version on every re-emit, so loaders must
// tolerate any patch within the supported major.minor.
const supportedMajorMinor = "1.0"

func schemaVersionCompatible(v string) bool {
	return strings.HasPrefix(v, supportedMajorMinor+".")
}
```

`SchemaVersion = "1.0.0"` in `types.go` stays — it's still the initial value Persist will write for brand-new entities; only the read-side strictness loosens.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/stack/ -run Version -v`
Expected: both new tests PASS.

- [ ] **Step 5: Run the full stack test suite to check for regressions**

Run: `go test ./internal/stack/ -v`
Expected: all tests PASS (previously-passing version tests still see "1.0.0" which still matches "1.0.x").

- [ ] **Step 6: Commit**

```bash
git add internal/stack/validate.go internal/stack/version_test.go
git commit -m "feat(stack): accept patch-bumped schema_version within 1.0.x"
```

### Task 1.3: Relax `internal/usecase` schema_version

**Files:**
- Modify: `internal/usecase/loader.go:15` (constant) and `internal/usecase/loader.go:26-31` (check)

Current behavior: rejects anything that is not exactly `"2.0.0"` with `Code: "USECASE_SCHEMA_VERSION_UNSUPPORTED"`.

- [ ] **Step 1: Write a failing test**

Add to a new test file `internal/usecase/version_test.go`:

```go
package usecase

import (
	"testing"

	"github.com/iurykrieger/lastro/internal/fixture"
)

func TestLoad_AcceptsPatchBumpedVersion(t *testing.T) {
	// Re-use the existing golden example but bump schema_version.
	raw, err := os.ReadFile("../../schemas/examples/use-case/http-api.yaml")
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	bumped := bytes.Replace(raw, []byte("schema_version: 2.0.0"), []byte("schema_version: 2.0.5"), 1)

	store, err := fixturestore.FromExamples(t)
	if err != nil {
		t.Fatalf("fixture store: %v", err)
	}
	if _, err := Load(bumped, store); err != nil {
		t.Fatalf("Load rejected 2.0.5: %v", err)
	}
}

func TestLoad_RejectsDifferentMajor(t *testing.T) {
	raw, err := os.ReadFile("../../schemas/examples/use-case/http-api.yaml")
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	bumped := bytes.Replace(raw, []byte("schema_version: 2.0.0"), []byte("schema_version: 3.0.0"), 1)

	if _, err := Load(bumped, nil); err == nil {
		t.Fatal("Load accepted incompatible major version 3.0.0")
	}
}
```

Add the imports `"bytes"`, `"os"` at the top, plus `fixturestore "github.com/iurykrieger/lastro/internal/usecase/internal/fixturestub"` (verify the actual sub-package's import path with `go doc` if needed — adjust per current repo).

If `fixturestore.FromExamples(t)` doesn't exist, replace with whatever helper the existing usecase tests use to construct a `fixture.FixtureStore` (look at `internal/usecase/loader_test.go` for the pattern). If no helper exists, build an empty store with `fixture.NewStore()` and accept that the `Validate` step may fail for non-version reasons — narrow the test to the schema_version error specifically by inspecting the returned `*ValidationError.Code`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/usecase/ -run Version -v`
Expected: first test FAILS (currently rejects 2.0.5); second test passes incidentally (3.0.0 is rejected today).

- [ ] **Step 3: Implement the relaxation**

Edit `internal/usecase/loader.go`. Replace lines 13-15:

```go
// supportedMajorMinor is the "major.minor" prefix this loader accepts.
// Persist patch-bumps schema_version on every re-emit; the loader
// tolerates any patch in the supported major.minor.
const supportedMajorMinor = "2.0"
```

Replace lines 26-31:

```go
	if !strings.HasPrefix(uc.SchemaVersion, supportedMajorMinor+".") {
		return nil, &ValidationError{
			Code:    "USECASE_SCHEMA_VERSION_UNSUPPORTED",
			Message: "schema_version " + uc.SchemaVersion + " is not supported (want " + supportedMajorMinor + ".x)",
		}
	}
```

Add `"strings"` to the imports.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/usecase/ -run Version -v`
Expected: both tests PASS.

- [ ] **Step 5: Run the full usecase test suite to check for regressions**

Run: `go test ./internal/usecase/...`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/usecase/loader.go internal/usecase/version_test.go
git commit -m "feat(usecase): accept patch-bumped schema_version within 2.0.x"
```

### Task 1.4: Audit fixture and sensor schema_version handling

**Files:**
- Inspect: `internal/fixture/loader.go`, `internal/fixture/types.go`, `internal/sensor/loader.go`, `internal/sensor/types.go`

- [ ] **Step 1: Grep for strict version checks**

Run:
```bash
grep -nE 'SchemaVersion|schema_version|supportedSchemaVersion|supportedMajorMinor' internal/fixture/*.go internal/sensor/*.go
```

For each strict-equality check found (e.g., `if x.SchemaVersion != "1.0.0"`), apply the same relaxation pattern as Task 1.2: introduce a local `supportedMajorMinor` constant + `HasPrefix` check, add a `version_test.go` with the two tests (accepts patch-bumped; rejects different major).

- [ ] **Step 2: Repeat Task 1.2's steps 1-6 for each entity package that had a strict check**

If neither package has a strict version check (schema validates via JSON Schema pattern only and Go has no equality test), skip — the version is already free-form. Note in commit message which packages needed relaxation and which didn't.

- [ ] **Step 3: Final regression check across all loaders**

Run: `go test ./internal/...`
Expected: all PASS.

- [ ] **Step 4: Commit (only if changes were made)**

```bash
git add internal/fixture/ internal/sensor/
git commit -m "feat(loaders): accept patch-bumped schema_version within supported major.minor"
```

---

## Phase 2 — `applicable_angles` on stack-manifest

Add the derived field that `stack.Persist` will inject and `/create-sensors` will read.

### Task 2.1: Extend stack-manifest schema YAML

**Files:**
- Modify: `schemas/stack-manifest.yaml`

- [ ] **Step 1: Read current schema to find the right insertion point**

Run: `Read schemas/stack-manifest.yaml fully (it's ~50 lines).`

- [ ] **Step 2: Add the field**

Insert under `properties:` (alphabetical position is between `archetype` and `components`):

```yaml
  applicable_angles:
    type: array
    minItems: 1
    items:
      type: string
    description: |
      Derived field. Populated by stack.Persist from the manifest's
      archetype via internal/enums.ApplicableAngles[archetype]. The
      canonical source of truth is schemas/enums/archetypes.yaml's
      per-value applicable_angles list. Never authored by the LLM.
```

Also add `applicable_angles` to the `required:` list (manifest is invalid without it once written).

- [ ] **Step 3: Commit (schema-only change first; the loader still works because it ignores unknown fields)**

Wait — before committing, also update the example so the existing loader test doesn't fail.

### Task 2.2: Update the stack-manifest example

**Files:**
- Modify: `schemas/examples/stack-manifest/http-api.yaml`

- [ ] **Step 1: Read the example and confirm its archetype**

Run: `Read schemas/examples/stack-manifest/http-api.yaml fully.` Confirm `archetype: http-api`.

- [ ] **Step 2: Add applicable_angles matching internal/enums.ApplicableAngles[ArchetypeHTTPAPI]**

The list (from `internal/enums/archetype_angles.go:7-11`):
```yaml
applicable_angles:
  - security
  - build
  - code-structure
  - unit-test
  - e2e-test
  - contracts
  - logs
  - metrics
  - database
  - performance
```

Insert under the existing `archetype:` line.

- [ ] **Step 3: Run the existing stack loader tests to confirm the example still loads**

Run: `go test ./internal/stack/ -v`
Expected: tests still PASS (the loader doesn't yet know about the field, but schema validation accepts it because we added it to properties; struct unmarshal silently ignores unknown fields).

- [ ] **Step 4: Commit (schema + example together)**

```bash
git add schemas/stack-manifest.yaml schemas/examples/stack-manifest/http-api.yaml
git commit -m "feat(schemas): add applicable_angles to stack-manifest schema and example"
```

### Task 2.3: Add `ApplicableAngles` field to the Go type

**Files:**
- Modify: `internal/stack/types.go:42-50`

- [ ] **Step 1: Write a failing test**

Append to `internal/stack/load_test.go` (or wherever the existing manifest-loading tests live — grep for `TestLoad` in `internal/stack/` to confirm):

```go
func TestLoad_PopulatesApplicableAngles(t *testing.T) {
	m, err := Load("../../schemas/examples/stack-manifest/http-api.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.ApplicableAngles) == 0 {
		t.Fatal("ApplicableAngles is empty after Load")
	}
	wantFirst := enums.AngleSecurity
	if m.ApplicableAngles[0] != wantFirst {
		t.Fatalf("ApplicableAngles[0]=%q, want %q", m.ApplicableAngles[0], wantFirst)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/stack/ -run TestLoad_PopulatesApplicableAngles -v`
Expected: FAIL with `len(m.ApplicableAngles) == 0` (field doesn't exist yet, unmarshals to nil).

- [ ] **Step 3: Add the field**

Edit `internal/stack/types.go`. Replace the `StackManifest` struct:

```go
// StackManifest is the full detected manifest for a repository: the
// archetype, applicable validation angles (derived from archetype), plus
// the ordered list of detected StackComponents.
type StackManifest struct {
	SchemaVersion    string                  `json:"schema_version"    yaml:"schema_version"`
	Archetype        enums.Archetype         `json:"archetype"         yaml:"archetype"`
	ApplicableAngles []enums.ValidationAngle `json:"applicable_angles" yaml:"applicable_angles"`
	Components       []StackComponent        `json:"components"        yaml:"components"`

	byID map[string]StackComponent `json:"-" yaml:"-"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/stack/ -run TestLoad_PopulatesApplicableAngles -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/stack/types.go internal/stack/load_test.go
git commit -m "feat(stack): expose ApplicableAngles field on StackManifest"
```

### Task 2.4: Validate that `applicable_angles` matches the archetype's canonical list

**Files:**
- Modify: `internal/stack/validate.go` (extend `StackManifest.Validate`)

- [ ] **Step 1: Write a failing test**

Append to `internal/stack/version_test.go` (or whichever validate test file):

```go
func TestStackManifest_Validate_RejectsWrongApplicableAngles(t *testing.T) {
	m := StackManifest{
		SchemaVersion:    "1.0.0",
		Archetype:        enums.ArchetypeHTTPAPI,
		ApplicableAngles: []enums.ValidationAngle{enums.AngleSecurity}, // missing the rest
		Components: []StackComponent{{
			SchemaVersion:     "1.0.0",
			ID:                "express",
			Kind:              enums.StackKindLibrary,
			Name:              "express",
			Version:           "4.18.0",
			Capabilities:      []string{"http-routing"},
			DetectionEvidence: []EvidenceRef{{File: "package.json", Path: ".dependencies.express"}},
		}},
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("Validate accepted ApplicableAngles that don't match enums.ApplicableAngles[archetype]")
	}
	if !strings.Contains(err.Error(), "applicable_angles") {
		t.Fatalf("error should mention applicable_angles, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/stack/ -run RejectsWrongApplicableAngles -v`
Expected: FAIL — Validate currently has no `applicable_angles` check.

- [ ] **Step 3: Add the validation**

Edit `internal/stack/validate.go`. Inside `StackManifest.Validate`, after the existing `components` check (around the section that loops over components), add:

```go
	// applicable_angles must match the canonical archetype × angle matrix
	// in internal/enums. Persist is the only legitimate writer of this
	// field, so any mismatch is a programmer/loader bug, not user input.
	if m.Archetype != "" && enums.IsValidArchetype(string(m.Archetype)) {
		want := enums.ApplicableAngles[m.Archetype]
		if !angleSetEqual(m.ApplicableAngles, want) {
			problems = append(problems,
				fmt.Sprintf("applicable_angles: got %v, want %v (canonical list for archetype %q)",
					m.ApplicableAngles, want, m.Archetype))
		}
	}
```

Add the helper at the bottom of the file:

```go
func angleSetEqual(a, b []enums.ValidationAngle) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[enums.ValidationAngle]bool, len(b))
	for _, v := range b {
		seen[v] = true
	}
	for _, v := range a {
		if !seen[v] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/stack/ -run RejectsWrongApplicableAngles -v`
Expected: PASS.

- [ ] **Step 5: Run full stack tests**

Run: `go test ./internal/stack/`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/stack/validate.go internal/stack/version_test.go
git commit -m "feat(stack): validate applicable_angles matches archetype's canonical list"
```

---

## Phase 3 — `stack.LoadBytes` and `stack.Persist`

### Task 3.1: Extract `LoadBytes` from `Load(path)`

**Files:**
- Modify: `internal/stack/load.go`

- [ ] **Step 1: Write a failing test**

Append to `internal/stack/load_test.go`:

```go
func TestLoadBytes_RoundTripsExample(t *testing.T) {
	b, err := os.ReadFile("../../schemas/examples/stack-manifest/http-api.yaml")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	m, err := LoadBytes(b)
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if m.Archetype != enums.ArchetypeHTTPAPI {
		t.Fatalf("Archetype=%q, want http-api", m.Archetype)
	}
}
```

Add `"os"` import if not present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/stack/ -run TestLoadBytes -v`
Expected: FAIL (LoadBytes not defined).

- [ ] **Step 3: Refactor `Load` to use `LoadBytes`**

Edit `internal/stack/load.go`. Replace `Load` with:

```go
// Load reads, JSON-Schema-validates, unmarshals, programmatically
// validates, and indexes a stack manifest YAML file. It is the canonical
// entrypoint for file-based callers; LoadBytes is the entrypoint for
// in-memory callers (e.g., Persist's pre-write validation).
func Load(path string) (StackManifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return StackManifest{}, fmt.Errorf("read %s: %w", path, err)
	}
	m, err := LoadBytes(b)
	if err != nil {
		return StackManifest{}, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// LoadBytes validates and unmarshals an in-memory stack manifest. Same
// pipeline as Load minus the os.ReadFile step.
func LoadBytes(b []byte) (StackManifest, error) {
	manifestSch, _, err := compileSchemas()
	if err != nil {
		return StackManifest{}, err
	}
	if err := validateAgainstSchema(b, manifestSch); err != nil {
		return StackManifest{}, err
	}
	var m StackManifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return StackManifest{}, fmt.Errorf("unmarshal: %w", err)
	}
	if err := m.Validate(); err != nil {
		return StackManifest{}, err
	}
	if err := m.buildIndex(); err != nil {
		return StackManifest{}, err
	}
	return m, nil
}
```

- [ ] **Step 4: Run tests to verify both pass**

Run: `go test ./internal/stack/`
Expected: all PASS (Load still works because it delegates; LoadBytes works directly).

- [ ] **Step 5: Commit**

```bash
git add internal/stack/load.go internal/stack/load_test.go
git commit -m "refactor(stack): extract LoadBytes for in-memory callers"
```

### Task 3.2: Add `stack.Persist` — happy path (new file)

**Files:**
- Create: `internal/stack/persist.go`
- Create: `internal/stack/persist_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/stack/persist_test.go`:

```go
package stack

import (
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestPersist_NewFile_WritesEnrichedManifest(t *testing.T) {
	dir := t.TempDir()
	in := []byte(`schema_version: 1.0.0
archetype: http-api
components:
  - schema_version: 1.0.0
    id: express
    kind: library
    name: express
    version: 4.18.0
    capabilities: [http-routing]
    detection_evidence:
      - file: package.json
        path: .dependencies.express
`)
	if err := Persist(in, dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "stack-manifest.yaml"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	var m StackManifest
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal written: %v", err)
	}
	if m.Archetype != enums.ArchetypeHTTPAPI {
		t.Fatalf("Archetype=%q", m.Archetype)
	}
	wantAngles := enums.ApplicableAngles[enums.ArchetypeHTTPAPI]
	if !angleSetEqual(m.ApplicableAngles, wantAngles) {
		t.Fatalf("ApplicableAngles=%v, want %v", m.ApplicableAngles, wantAngles)
	}
	if m.SchemaVersion != "1.0.0" {
		t.Fatalf("SchemaVersion=%q, want 1.0.0 (no prior file to bump from)", m.SchemaVersion)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/stack/ -run TestPersist_NewFile -v`
Expected: FAIL (Persist undefined).

- [ ] **Step 3: Implement `Persist`**

Create `internal/stack/persist.go`:

```go
package stack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/persisterror"
)

const manifestFilename = "stack-manifest.yaml"

// Persist validates an LLM-emitted stack-manifest YAML, enriches it with
// applicable_angles derived from the archetype, patch-bumps its
// schema_version against any prior on-disk version, and atomically writes
// it to <harnessDir>/stack-manifest.yaml. Returns a *persisterror.Error
// on validation failure; nothing is written when an error is returned.
func Persist(content []byte, harnessDir string) error {
	m, err := LoadBytes(content)
	if err != nil {
		return wrapLoadError(err)
	}

	// Enrichment: stamp ApplicableAngles from the archetype's canonical
	// list. We do this *before* Validate so that Validate's
	// applicable_angles check passes regardless of what (if anything) the
	// LLM emitted in that field.
	m.ApplicableAngles = enums.ApplicableAngles[m.Archetype]

	// Schema-version bump.
	targetPath := filepath.Join(harnessDir, manifestFilename)
	bumped, err := bumpSchemaVersion(targetPath, m.SchemaVersion)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "stack-manifest",
			Message:    fmt.Sprintf("schema_version bump: %v", err),
		}
	}
	m.SchemaVersion = bumped

	// Marshal back to YAML.
	out, err := yaml.Marshal(m)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "stack-manifest",
			Message:    fmt.Sprintf("marshal: %v", err),
		}
	}

	// Atomic write.
	if err := atomicWrite(targetPath, out); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "stack-manifest",
			Message:    fmt.Sprintf("write %s: %v", targetPath, err),
		}
	}
	return nil
}

// wrapLoadError converts a Load/LoadBytes error to a *persisterror.Error.
// LoadBytes errors are flat strings today; for B4 we lump them as
// SchemaViolation. Future passes can introspect for finer Kinds.
func wrapLoadError(err error) error {
	var pe *persisterror.Error
	if errors.As(err, &pe) {
		return pe
	}
	return &persisterror.Error{
		Kind:       persisterror.SchemaViolation,
		EntityType: "stack-manifest",
		Message:    err.Error(),
	}
}

// bumpSchemaVersion reads the existing target file (if any), parses its
// schema_version, and returns the patch-incremented value. If the target
// doesn't exist, returns input unchanged (the LLM-supplied initial
// version).
func bumpSchemaVersion(targetPath, input string) (string, error) {
	existing, err := os.ReadFile(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		return input, nil
	}
	if err != nil {
		return "", err
	}
	var head struct {
		SchemaVersion string `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(existing, &head); err != nil {
		return "", fmt.Errorf("parse existing schema_version: %w", err)
	}
	if head.SchemaVersion == "" {
		return input, nil
	}
	bumped, err := bumpPatch(head.SchemaVersion)
	if err != nil {
		return "", err
	}
	return bumped, nil
}

// bumpPatch increments the patch component of a semver string. Major
// and minor are preserved verbatim.
func bumpPatch(v string) (string, error) {
	var maj, min, patch int
	n, err := fmt.Sscanf(v, "%d.%d.%d", &maj, &min, &patch)
	if err != nil || n != 3 {
		return "", fmt.Errorf("not a semver: %q", v)
	}
	return fmt.Sprintf("%d.%d.%d", maj, min, patch+1), nil
}

// atomicWrite writes content to <targetPath>.tmp then renames over the
// target. Ensures any reader sees either the prior content or the new
// content, never a partial file.
func atomicWrite(targetPath string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	tmp := targetPath + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, targetPath)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/stack/ -run TestPersist_NewFile -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/stack/persist.go internal/stack/persist_test.go
git commit -m "feat(stack): Persist new-file path — validate, enrich, atomic write"
```

### Task 3.3: `stack.Persist` — schema-version bump on existing file

- [ ] **Step 1: Append the test**

In `internal/stack/persist_test.go`:

```go
func TestPersist_ExistingFile_BumpsSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	prior := []byte(`schema_version: 1.0.7
archetype: http-api
applicable_angles: [security, build, code-structure, unit-test, e2e-test, contracts, logs, metrics, database, performance]
components:
  - schema_version: 1.0.0
    id: express
    kind: library
    name: express
    version: 4.18.0
    capabilities: [http-routing]
    detection_evidence:
      - file: package.json
        path: .dependencies.express
`)
	if err := os.WriteFile(filepath.Join(dir, "stack-manifest.yaml"), prior, 0o644); err != nil {
		t.Fatal(err)
	}
	in := []byte(`schema_version: 1.0.0
archetype: http-api
components:
  - schema_version: 1.0.0
    id: express
    kind: library
    name: express
    version: 4.18.0
    capabilities: [http-routing]
    detection_evidence:
      - file: package.json
        path: .dependencies.express
`)
	if err := Persist(in, dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "stack-manifest.yaml"))
	var m StackManifest
	_ = yaml.Unmarshal(out, &m)
	if m.SchemaVersion != "1.0.8" {
		t.Fatalf("SchemaVersion=%q, want 1.0.8 (prior 1.0.7 + 1)", m.SchemaVersion)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/stack/ -run TestPersist_ExistingFile -v`
Expected: PASS (Persist already implements the bump in Task 3.2).

- [ ] **Step 3: Commit**

```bash
git add internal/stack/persist_test.go
git commit -m "test(stack): Persist bumps schema_version on existing target"
```

### Task 3.4: `stack.Persist` — rejects bad archetype

- [ ] **Step 1: Append the test**

```go
func TestPersist_RejectsBadArchetype(t *testing.T) {
	dir := t.TempDir()
	in := []byte(`schema_version: 1.0.0
archetype: not-a-real-archetype
components:
  - schema_version: 1.0.0
    id: x
    kind: library
    name: x
    version: 1.0.0
    capabilities: [c]
    detection_evidence: [{file: f, path: p}]
`)
	err := Persist(in, dir)
	if err == nil {
		t.Fatal("Persist accepted unknown archetype")
	}
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("error is not *persisterror.Error: %T", err)
	}
	// The exact Kind depends on whether the schema or programmatic check
	// fires first; both are acceptable signals of a bad archetype.
	if pe.Kind != persisterror.SchemaViolation && pe.Kind != persisterror.UnknownEnumValue {
		t.Fatalf("Kind=%q, want SchemaViolation or UnknownEnumValue", pe.Kind)
	}
	if _, err := os.Stat(filepath.Join(dir, "stack-manifest.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".harness/ written to despite validation failure: stat=%v", err)
	}
}
```

Add `"errors"` and `"github.com/iurykrieger/lastro/internal/persisterror"` imports if not yet present in the test file.

- [ ] **Step 2: Run test**

Run: `go test ./internal/stack/ -run TestPersist_RejectsBadArchetype -v`
Expected: PASS (LoadBytes already enforces this via schema enum).

- [ ] **Step 3: Commit**

```bash
git add internal/stack/persist_test.go
git commit -m "test(stack): Persist rejects unknown archetype and writes nothing"
```

### Task 3.5: `stack.Persist` — atomic-write safety under simulated failure

- [ ] **Step 1: Append a test that pre-seeds the target, then forces a non-existent harnessDir on write**

The simpler form: pre-seed a target, then try to Persist into a `harnessDir` that points to a non-existent and unwritable parent. Confirm the seeded file is untouched. Easiest reproducible test: rely on `os.MkdirAll` failing when the parent is a regular file.

```go
func TestPersist_AtomicityOnWriteFailure(t *testing.T) {
	// Create a regular file at the path that MkdirAll would need as a parent dir.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	harnessDir := filepath.Join(blocker, ".harness") // child of a regular file → MkdirAll fails

	in := []byte(`schema_version: 1.0.0
archetype: http-api
components:
  - schema_version: 1.0.0
    id: express
    kind: library
    name: express
    version: 4.18.0
    capabilities: [http-routing]
    detection_evidence: [{file: package.json, path: .dependencies.express}]
`)
	err := Persist(in, harnessDir)
	if err == nil {
		t.Fatal("Persist succeeded against unwritable harnessDir")
	}
	if _, statErr := os.Stat(filepath.Join(harnessDir, "stack-manifest.yaml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("partial state left behind: stat=%v", statErr)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/stack/ -run TestPersist_Atomicity -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/stack/persist_test.go
git commit -m "test(stack): Persist leaves no partial state when write fails"
```

---

## Phase 4 — `fixture.Persist`

Fixture has no cross-entity check, so this is the simplest Persist after stack. Reuses the bump/write helpers from `internal/stack` — but those live in package `stack`, not exported. **Inline-copy the small helpers (`bumpSchemaVersion`, `bumpPatch`, `atomicWrite`) into `internal/fixture/persist.go`.** Per CLAUDE.md rule 3, promote to a shared package on the *third* caller, not the second.

### Task 4.1: Add `LoadFixtureBytes` to `internal/fixture`

**Files:**
- Modify: `internal/fixture/loader.go`

Mirror Task 3.1's pattern. The existing `LoadFixture(path)` does ReadFile then a chain of validate/unmarshal/parse-payload steps. Extract everything after ReadFile into `LoadFixtureBytes(b []byte) (Fixture, error)` and have `LoadFixture` delegate to it.

- [ ] **Step 1: Test**, **Step 2: Run (fail)**, **Step 3: Implement**, **Step 4: Run (pass)**, **Step 5: Commit** — same shape as Task 3.1.

Commit message: `refactor(fixture): extract LoadFixtureBytes for in-memory callers`.

### Task 4.2: Add `fixture.Persist` — happy path

**Files:**
- Create: `internal/fixture/persist.go`
- Create: `internal/fixture/persist_test.go`

- [ ] **Step 1: Write the failing test**

```go
package fixture

import (
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestPersist_NewFile_WritesUnderFixturesDir(t *testing.T) {
	dir := t.TempDir()
	// Borrow shape from schemas/examples/fixture/input.yaml.
	in := []byte(`schema_version: 1.0.0
id: fx_create_order_request
use_case_id: create-order
role: input
content_type: application/json
payload: |
  {"item":"book"}
binding:
  channel: http
  selector:
    method: POST
    path: /orders
source_refs:
  - path: src/handlers/orders.ts
    symbol: createOrder
`)
	if err := Persist(in, dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "fixtures", "fx_create_order_request.yaml"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	var fx Fixture
	if err := yaml.Unmarshal(out, &fx); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if fx.ID != "fx_create_order_request" {
		t.Fatalf("ID=%q", fx.ID)
	}
}
```

(Adjust the YAML if the actual `schemas/examples/fixture/input.yaml` shape differs — `Read` it first.)

- [ ] **Step 2: Run test to verify it fails** (`Persist` undefined).

- [ ] **Step 3: Implement** in `internal/fixture/persist.go`:

```go
package fixture

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/persisterror"
)

// Persist validates an LLM-emitted fixture YAML, patch-bumps its
// schema_version against any prior on-disk version, and atomically
// writes it to <harnessDir>/fixtures/<id>.yaml.
func Persist(content []byte, harnessDir string) error {
	fx, err := LoadFixtureBytes(content)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "fixture",
			Message:    err.Error(),
		}
	}

	targetPath := filepath.Join(harnessDir, "fixtures", fx.ID+".yaml")
	bumped, err := bumpSchemaVersion(targetPath, fx.SchemaVersion)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "fixture", EntityID: fx.ID,
			Message: fmt.Sprintf("schema_version bump: %v", err)}
	}
	fx.SchemaVersion = bumped

	out, err := yaml.Marshal(fx)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "fixture", EntityID: fx.ID,
			Message: fmt.Sprintf("marshal: %v", err)}
	}
	if err := atomicWrite(targetPath, out); err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "fixture", EntityID: fx.ID,
			Message: fmt.Sprintf("write %s: %v", targetPath, err)}
	}
	return nil
}

// --- inline-copied helpers (see Phase 4 header note) ---

func bumpSchemaVersion(targetPath, input string) (string, error) {
	existing, err := os.ReadFile(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		return input, nil
	}
	if err != nil {
		return "", err
	}
	var head struct {
		SchemaVersion string `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(existing, &head); err != nil {
		return "", fmt.Errorf("parse existing schema_version: %w", err)
	}
	if head.SchemaVersion == "" {
		return input, nil
	}
	return bumpPatch(head.SchemaVersion)
}

func bumpPatch(v string) (string, error) {
	var maj, min, patch int
	n, err := fmt.Sscanf(v, "%d.%d.%d", &maj, &min, &patch)
	if err != nil || n != 3 {
		return "", fmt.Errorf("not a semver: %q", v)
	}
	return fmt.Sprintf("%d.%d.%d", maj, min, patch+1), nil
}

func atomicWrite(targetPath string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	tmp := targetPath + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, targetPath)
}
```

- [ ] **Step 4: Run test** — PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fixture/persist.go internal/fixture/persist_test.go
git commit -m "feat(fixture): Persist new-file path — validate and atomic write"
```

### Task 4.3: `fixture.Persist` — bump existing + reject malformed payload

Same pattern as Tasks 3.3 + 3.4. Two short tests, both should pass against the implementation already written.

- [ ] **Step 1: Append `TestPersist_BumpsExisting` (mirror Task 3.3 with `fixtures/<id>.yaml` target).**
- [ ] **Step 2: Append `TestPersist_RejectsMalformedJSONPayload` — set `content_type: application/json` with `payload: '{ broken'`. Expect `*persisterror.Error` with `Kind: SchemaViolation`.**
- [ ] **Step 3: Run.** Expected: both PASS.
- [ ] **Step 4: Commit.**

---

## Phase 5 — `usecase.Persist`

UseCase has cross-entity invariants (fixture refs + template token resolution). Persist must load on-disk fixtures into a `fixture.Store` first.

### Task 5.1: Add `usecase.Persist` — happy path with fixtures already present

**Files:**
- Create: `internal/usecase/persist.go`
- Create: `internal/usecase/persist_test.go`

- [ ] **Step 1: Write the failing test**

```go
package usecase

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/fixture"
)

func TestPersist_NewFile_WithReferencedFixturesOnDisk(t *testing.T) {
	dir := t.TempDir()
	// Pre-seed a fixture on disk that the use case references.
	fxDir := filepath.Join(dir, "fixtures")
	if err := os.MkdirAll(fxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fxYAML := []byte(`schema_version: 1.0.0
id: fx_create_order_request
use_case_id: create-order
role: input
content_type: application/json
payload: |
  {"item":"book"}
binding:
  channel: http
  selector: {method: POST, path: /orders}
source_refs: [{path: src/handlers.ts, symbol: createOrder}]
`)
	if err := os.WriteFile(filepath.Join(fxDir, "fx_create_order_request.yaml"), fxYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	uc := []byte(`schema_version: 2.0.0
id: create-order
title: Create order
archetype_scope: [http-api]
entry_points:
  - id: create_order_endpoint
    archetype: http-api
    spec: {method: POST, path: /orders}
given:
  - "Request matching {{fixtures.fx_create_order_request}} is constructed"
when:
  - "Client invokes {{entry_points.create_order_endpoint}}"
then:
  - "Endpoint returns success"
fixture_ids: [fx_create_order_request]
`)
	if err := Persist(uc, dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "use-cases", "create-order.yaml")); err != nil {
		t.Fatalf("use-case not written: %v", err)
	}
	// Sanity: the fixture store path is the one Persist used.
	store, err := fixture.LoadDirectory(fxDir)
	if err != nil {
		t.Fatalf("LoadDirectory: %v", err)
	}
	if _, ok := store.LookupFixture("fx_create_order_request"); !ok {
		t.Fatal("test setup broken: fixture not in store")
	}
}
```

- [ ] **Step 2: Run test to verify it fails.**

- [ ] **Step 3: Implement**

```go
package usecase

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/persisterror"
)

// Persist validates an LLM-emitted use-case YAML against schemas + the
// on-disk fixtures under <harnessDir>/fixtures/, patch-bumps its
// schema_version against any prior version, and atomically writes it to
// <harnessDir>/use-cases/<id>.yaml.
func Persist(content []byte, harnessDir string) error {
	// Load on-disk fixtures so the use-case loader can resolve fixture refs.
	fxDir := filepath.Join(harnessDir, "fixtures")
	store, err := loadFixtureStoreOrEmpty(fxDir)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.FixtureBinding, EntityType: "use-case",
			Message: fmt.Sprintf("load fixtures: %v", err)}
	}

	uc, err := Load(content, store)
	if err != nil {
		return mapLoadError(uc, err)
	}

	targetPath := filepath.Join(harnessDir, "use-cases", uc.ID+".yaml")
	bumped, err := bumpSchemaVersion(targetPath, uc.SchemaVersion)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "use-case", EntityID: uc.ID,
			Message: fmt.Sprintf("schema_version bump: %v", err)}
	}
	uc.SchemaVersion = bumped

	out, err := yaml.Marshal(uc)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "use-case", EntityID: uc.ID,
			Message: fmt.Sprintf("marshal: %v", err)}
	}
	if err := atomicWrite(targetPath, out); err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "use-case", EntityID: uc.ID,
			Message: fmt.Sprintf("write %s: %v", targetPath, err)}
	}
	return nil
}

func loadFixtureStoreOrEmpty(fxDir string) (fixture.FixtureStore, error) {
	if _, err := os.Stat(fxDir); errors.Is(err, os.ErrNotExist) {
		s, _ := fixture.NewStore()
		return s, nil
	}
	return fixture.LoadDirectory(fxDir)
}

// mapLoadError translates the usecase.Load error into a *persisterror.Error.
// usecase.Load returns *ValidationError with a Code; we map known codes to
// fine-grained Kinds and fall back to SchemaViolation.
func mapLoadError(uc *UseCase, err error) error {
	var ve *ValidationError
	id := ""
	if uc != nil {
		id = uc.ID
	}
	if errors.As(err, &ve) {
		switch ve.Code {
		case "USECASE_FIXTURE_REF_UNKNOWN", "USECASE_FIXTURE_REF_NOT_OWNED":
			return &persisterror.Error{Kind: persisterror.FixtureBinding, EntityType: "use-case", EntityID: id,
				Message: ve.Message}
		case "USECASE_TEMPLATE_PARSE_ERROR", "USECASE_TEMPLATE_RESOLVE_ERROR":
			return &persisterror.Error{Kind: persisterror.TemplateResolution, EntityType: "use-case", EntityID: id,
				Message: ve.Message}
		default:
			return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "use-case", EntityID: id,
				Message: ve.Message}
		}
	}
	return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "use-case", EntityID: id,
		Message: err.Error()}
}

// --- inline-copied helpers (same as fixture/persist.go) ---

func bumpSchemaVersion(targetPath, input string) (string, error) { /* same as Task 4.2 */ }
func bumpPatch(v string) (string, error)                          { /* same */ }
func atomicWrite(targetPath string, content []byte) error          { /* same */ }
```

Copy the three helper bodies from `internal/fixture/persist.go` verbatim. (Yes, three callers now — but extracting to `lib/` is a separate decision we'll make at Phase 6 / sensor, the *fourth* caller. See note below.)

- [ ] **Step 4: Run test.** Expected: PASS.

- [ ] **Step 5: Commit.**

### Task 5.2: `usecase.Persist` — rejects missing fixture reference

- [ ] **Step 1: Append test** — same Persist call but `fixture_ids: [fx_missing]`. Assert `errors.As` to `*persisterror.Error`, assert `Kind == FixtureBinding`. Assert no `.harness/use-cases/<id>.yaml` was written.
- [ ] **Step 2: Run, expect PASS** (Load already enforces; mapping is in place).
- [ ] **Step 3: Commit.**

### Task 5.3: `usecase.Persist` — bumps existing file

- [ ] **Step 1: Append `TestPersist_BumpsExisting` (mirror Task 3.3).**
- [ ] **Step 2: Run, expect PASS.**
- [ ] **Step 3: Commit.**

### Task 5.4: Promote shared helpers to `internal/persisthelp` (third caller threshold reached)

Per CLAUDE.md rule 3, we now have three callers (`stack.Persist`, `fixture.Persist`, `usecase.Persist`) duplicating `bumpSchemaVersion`/`bumpPatch`/`atomicWrite`. Extract.

**Files:**
- Create: `internal/persisthelp/persisthelp.go`
- Create: `internal/persisthelp/persisthelp_test.go`
- Modify: `internal/stack/persist.go`, `internal/fixture/persist.go`, `internal/usecase/persist.go` to import and delegate.

- [ ] **Step 1: Write the new package**

```go
// Package persisthelp holds the small file-write primitives every entity
// Persist function uses: semver patch-bump against an on-disk target,
// and atomic write via temp-file + rename.
package persisthelp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

// BumpSchemaVersion reads targetPath's schema_version (if the file
// exists), patch-increments it, and returns the new value. If the file
// doesn't exist, returns input unchanged.
func BumpSchemaVersion(targetPath, input string) (string, error) {
	existing, err := os.ReadFile(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		return input, nil
	}
	if err != nil {
		return "", err
	}
	var head struct {
		SchemaVersion string `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(existing, &head); err != nil {
		return "", fmt.Errorf("parse existing schema_version: %w", err)
	}
	if head.SchemaVersion == "" {
		return input, nil
	}
	return BumpPatch(head.SchemaVersion)
}

func BumpPatch(v string) (string, error) {
	var maj, min, patch int
	n, err := fmt.Sscanf(v, "%d.%d.%d", &maj, &min, &patch)
	if err != nil || n != 3 {
		return "", fmt.Errorf("not a semver: %q", v)
	}
	return fmt.Sprintf("%d.%d.%d", maj, min, patch+1), nil
}

func AtomicWrite(targetPath string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	tmp := targetPath + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, targetPath)
}
```

- [ ] **Step 2: Tests**

```go
package persisthelp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBumpPatch(t *testing.T) {
	cases := map[string]string{"1.0.0": "1.0.1", "2.3.4": "2.3.5"}
	for in, want := range cases {
		got, err := BumpPatch(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s -> %s, want %s", in, got, want)
		}
	}
	if _, err := BumpPatch("not-semver"); err == nil {
		t.Fatal("BumpPatch accepted non-semver")
	}
}

func TestBumpSchemaVersion_FileMissingReturnsInput(t *testing.T) {
	got, err := BumpSchemaVersion(filepath.Join(t.TempDir(), "missing.yaml"), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.0.0" {
		t.Fatalf("got %q, want 1.0.0", got)
	}
}

func TestBumpSchemaVersion_FilePresentBumpsItsVersion(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.yaml")
	if err := os.WriteFile(target, []byte("schema_version: 1.0.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := BumpSchemaVersion(target, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.0.5" {
		t.Fatalf("got %q, want 1.0.5", got)
	}
}

func TestAtomicWrite_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "c.yaml")
	if err := AtomicWrite(target, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 3: Run tests** — `go test ./internal/persisthelp/ -v` → PASS.

- [ ] **Step 4: Delete the inline copies in stack/fixture/usecase Persist files; import `persisthelp` and call `persisthelp.BumpSchemaVersion`, etc.**

- [ ] **Step 5: Run all entity persist tests** — `go test ./internal/stack/ ./internal/fixture/ ./internal/usecase/ -v` → all still PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/persisthelp/ internal/stack/persist.go internal/fixture/persist.go internal/usecase/persist.go
git commit -m "refactor(persisthelp): extract semver bump + atomic write to shared package"
```

---

## Phase 6 — `sensor.Persist`

Most cross-entity checks: schema, grounding against stack-manifest, fixture-binding against fixtures whose `use_case_id` matches, angle membership in stack-manifest's `applicable_angles`, and use-case existence.

### Task 6.1: Add `LoadSensorBytes` to `internal/sensor`

Mirror Task 3.1's refactor pattern for `internal/sensor/loader.go`.

- [ ] Test, run (fail), implement, run (pass), commit.

Commit message: `refactor(sensor): extract LoadSensorBytes for in-memory callers`.

### Task 6.2: `sensor.Persist` — happy path with stack + fixture + use-case already on disk

**Files:**
- Create: `internal/sensor/persist.go`
- Create: `internal/sensor/persist_test.go`

- [ ] **Step 1: Write the failing test**

The test setup: pre-seed `.harness/` with stack-manifest, one fixture (`use_case_id: create-order`), and one use-case file. Then call `sensor.Persist` with a sensor whose `use_case_id: create-order`, `angle: e2e-test`, `uses: [express]`, and one step that uses the seeded fixture.

```go
package sensor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersist_NewFile_AllInvariantsSatisfied(t *testing.T) {
	dir := t.TempDir()

	// Stack manifest with applicable_angles including e2e-test.
	stackYAML := []byte(`schema_version: 1.0.0
archetype: http-api
applicable_angles: [security, build, code-structure, unit-test, e2e-test, contracts, logs, metrics, database, performance]
components:
  - schema_version: 1.0.0
    id: express
    kind: library
    name: express
    version: 4.18.0
    capabilities: [http-routing]
    detection_evidence: [{file: package.json, path: .dependencies.express}]
`)
	if err := os.WriteFile(filepath.Join(dir, "stack-manifest.yaml"), stackYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	// One fixture under use_case_id create-order.
	fxDir := filepath.Join(dir, "fixtures")
	_ = os.MkdirAll(fxDir, 0o755)
	if err := os.WriteFile(filepath.Join(fxDir, "fx_req.yaml"), []byte(`schema_version: 1.0.0
id: fx_req
use_case_id: create-order
role: input
content_type: application/json
payload: |
  {}
binding: {channel: http, selector: {method: POST, path: /orders}}
source_refs: [{path: src/x.ts, symbol: y}]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// One use case.
	ucDir := filepath.Join(dir, "use-cases")
	_ = os.MkdirAll(ucDir, 0o755)
	if err := os.WriteFile(filepath.Join(ucDir, "create-order.yaml"), []byte(`schema_version: 2.0.0
id: create-order
title: Create order
archetype_scope: [http-api]
entry_points: [{id: ep1, archetype: http-api, spec: {method: POST, path: /orders}}]
given: ["g"]
when: ["w"]
then: ["t"]
fixture_ids: [fx_req]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// The sensor under test.
	s := []byte(`schema_version: 1.0.0
id: s_create_order_e2e
use_case_id: create-order
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [express]
steps:
  - id: probe
    run: curl -X POST localhost:8080/orders
    uses: [fx_req]
`)
	if err := Persist(s, dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sensors", "s_create_order_e2e.yaml")); err != nil {
		t.Fatalf("sensor not written: %v", err)
	}
}
```

- [ ] **Step 2: Run, expect fail** (Persist undefined).

- [ ] **Step 3: Implement**

```go
package sensor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/persisterror"
	"github.com/iurykrieger/lastro/internal/persisthelp"
	"github.com/iurykrieger/lastro/internal/stack"
)

// Persist validates an LLM-emitted sensor YAML against schemas + the
// on-disk stack-manifest, fixtures, and use-cases under harnessDir,
// patch-bumps its schema_version against any prior on-disk version, and
// atomically writes it to <harnessDir>/sensors/<id>.yaml.
func Persist(content []byte, harnessDir string) error {
	s, err := LoadSensorBytes(content)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "sensor", Message: err.Error()}
	}

	// (a) Stack-manifest must exist; grounding via ValidateAgainstStack.
	manifest, err := loadStackOrErr(harnessDir, s.ID)
	if err != nil {
		return err
	}
	if err := ValidateAgainstStack(s, manifest); err != nil {
		return &persisterror.Error{Kind: persisterror.Grounding, EntityType: "sensor", EntityID: s.ID,
			Message: err.Error()}
	}

	// (d) sensor.angle ∈ manifest.applicable_angles.
	if !angleApplicable(s.Angle, manifest.ApplicableAngles) {
		return &persisterror.Error{Kind: persisterror.AngleNotApplicable, EntityType: "sensor", EntityID: s.ID,
			Message: fmt.Sprintf("angle %q is not in stack-manifest.applicable_angles %v", s.Angle, manifest.ApplicableAngles)}
	}

	// (b) Use-case must exist on disk under .harness/use-cases/<use_case_id>.yaml.
	ucPath := filepath.Join(harnessDir, "use-cases", s.UseCaseID+".yaml")
	if _, err := os.Stat(ucPath); errors.Is(err, os.ErrNotExist) {
		return &persisterror.Error{Kind: persisterror.MissingDependency, EntityType: "sensor", EntityID: s.ID,
			Message: fmt.Sprintf("use-case %q not found at %s", s.UseCaseID, ucPath)}
	}

	// (c) Step uses ⊆ fixtures with use_case_id == s.UseCaseID.
	store, err := loadFixtureStoreOrEmpty(filepath.Join(harnessDir, "fixtures"))
	if err != nil {
		return &persisterror.Error{Kind: persisterror.FixtureBinding, EntityType: "sensor", EntityID: s.ID,
			Message: fmt.Sprintf("load fixtures: %v", err)}
	}
	if err := ValidateAgainstFixtures(s, fixtureOwner{store, s.UseCaseID}); err != nil {
		return &persisterror.Error{Kind: persisterror.FixtureBinding, EntityType: "sensor", EntityID: s.ID,
			Message: err.Error()}
	}

	// Bump + write.
	targetPath := filepath.Join(harnessDir, "sensors", s.ID+".yaml")
	bumped, err := persisthelp.BumpSchemaVersion(targetPath, s.SchemaVersion)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "sensor", EntityID: s.ID,
			Message: fmt.Sprintf("schema_version bump: %v", err)}
	}
	s.SchemaVersion = bumped

	out, err := yaml.Marshal(s)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "sensor", EntityID: s.ID,
			Message: fmt.Sprintf("marshal: %v", err)}
	}
	if err := persisthelp.AtomicWrite(targetPath, out); err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "sensor", EntityID: s.ID,
			Message: fmt.Sprintf("write %s: %v", targetPath, err)}
	}
	return nil
}

func loadStackOrErr(harnessDir, sensorID string) (stack.StackManifest, error) {
	path := filepath.Join(harnessDir, "stack-manifest.yaml")
	m, err := stack.Load(path)
	if errors.Is(err, os.ErrNotExist) || (err != nil && os.IsNotExist(err)) {
		return stack.StackManifest{}, &persisterror.Error{Kind: persisterror.MissingDependency,
			EntityType: "sensor", EntityID: sensorID,
			Message: fmt.Sprintf("stack-manifest not found at %s", path)}
	}
	if err != nil {
		return stack.StackManifest{}, &persisterror.Error{Kind: persisterror.SchemaViolation,
			EntityType: "sensor", EntityID: sensorID,
			Message: fmt.Sprintf("load stack-manifest: %v", err)}
	}
	return m, nil
}

func loadFixtureStoreOrEmpty(fxDir string) (*fixture.Store, error) {
	if _, err := os.Stat(fxDir); errors.Is(err, os.ErrNotExist) {
		return fixture.NewStore()
	}
	return fixture.LoadDirectory(fxDir)
}

func angleApplicable(a string, list []string) bool {
	// `a` and `list` types may differ; cast both to string for comparison.
	for _, x := range list {
		if string(x) == string(a) {
			return true
		}
	}
	return false
}

// fixtureOwner adapts a fixture.Store + a fixed use-case id to satisfy
// the UseCaseFixtureOwnership interface that ValidateAgainstFixtures expects.
type fixtureOwner struct {
	store *fixture.Store
	useCaseID string
}

func (o fixtureOwner) OwnedFixtureIDs(useCaseID string) []string {
	if useCaseID != o.useCaseID {
		return nil
	}
	var ids []string
	for _, fx := range o.store.FixturesForUseCase(useCaseID) {
		ids = append(ids, fx.ID)
	}
	return ids
}
```

(Adjust `angleApplicable` if `s.Angle` and `manifest.ApplicableAngles` are already the same typed enum — drop the casts. Verify by reading `internal/sensor/types.go` and `internal/stack/types.go` for the exact field types.)

- [ ] **Step 4: Run test** — PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sensor/persist.go internal/sensor/persist_test.go
git commit -m "feat(sensor): Persist validates grounding, fixture binding, angle applicability"
```

### Task 6.3: `sensor.Persist` — each cross-entity invariant has its own failing test

For each Kind exercised by `sensor.Persist`, write a focused test:

- [ ] **Step 1: `TestPersist_Grounding_RejectsUnknownStackComponent`** — sensor has `uses: [unknown-lib]`; expect `Kind: Grounding`, no write.
- [ ] **Step 2: `TestPersist_AngleNotApplicable_RejectsAngleNotInManifest`** — sensor `angle: database` on a stack-manifest whose `applicable_angles` excludes `database` (use archetype `cli`); expect `Kind: AngleNotApplicable`.
- [ ] **Step 3: `TestPersist_MissingDependency_RejectsUnknownUseCase`** — sensor references `use_case_id: nonexistent`; expect `Kind: MissingDependency`.
- [ ] **Step 4: `TestPersist_FixtureBinding_RejectsStepUsingUnownedFixture`** — sensor's step `uses: [fx_other_uc]` where `fx_other_uc` exists but with `use_case_id` ≠ sensor's; expect `Kind: FixtureBinding`.

Each test: pre-seed `.harness/` with the appropriate state, call `Persist`, assert `errors.As` to `*persisterror.Error` with the expected `Kind`, assert no `.harness/sensors/<id>.yaml` was written.

- [ ] **Step 5: Run all** — `go test ./internal/sensor/ -run TestPersist -v` → PASS.
- [ ] **Step 6: Commit**

```bash
git add internal/sensor/persist_test.go
git commit -m "test(sensor): cover Grounding, AngleNotApplicable, MissingDependency, FixtureBinding"
```

### Task 6.4: `sensor.Persist` — schema-version bump on existing target

- [ ] Append `TestPersist_BumpsExisting` (same shape as Task 3.3), run, commit.

---

## Phase 7 — Skill scripts (Go wrappers)

Each skill has a tiny `scripts/main.go` that imports its entity package's `Persist`, reads a file, and prints structured errors.

### Task 7.1: `skills/detect-stack/scripts/main.go`

**Files:**
- Create: `skills/detect-stack/scripts/main.go`
- Create: `skills/detect-stack/scripts/main_test.go`

- [ ] **Step 1: Write the main.go**

```go
// Command detect-stack-script is invoked by the /detect-stack slash
// command. Reads an LLM-emitted stack-manifest YAML from --file and
// hands it to internal/stack.Persist. On success: exit 0, nothing on
// stdout. On validation failure: exit 2 with a JSON persisterror.Error
// on stdout. On script-level failure (bad args, missing file): exit 1.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/iurykrieger/lastro/internal/persisterror"
	"github.com/iurykrieger/lastro/internal/stack"
)

func main() {
	file := flag.String("file", "", "Path to the LLM-emitted stack-manifest YAML")
	harnessDir := flag.String("harness-dir", ".harness", "Target .harness directory")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "missing required --file")
		os.Exit(1)
	}
	content, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read input:", err)
		os.Exit(1)
	}
	if err := stack.Persist(content, *harnessDir); err != nil {
		var pe *persisterror.Error
		if errors.As(err, &pe) {
			_ = json.NewEncoder(os.Stdout).Encode(pe)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./skills/detect-stack/scripts/`
Expected: no output, exit 0.

- [ ] **Step 3: Write the test**

```go
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/persisterror"
)

// runScript builds and runs the script binary. Returns stdout, stderr, exit code.
func runScript(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
	var sout, serr strings.Builder
	cmd.Stdout = &sout
	cmd.Stderr = &serr
	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("exec: %v", err)
	}
	return sout.String(), serr.String(), code
}

func TestMain_HappyPath(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(`schema_version: 1.0.0
archetype: http-api
components:
  - schema_version: 1.0.0
    id: express
    kind: library
    name: express
    version: 4.18.0
    capabilities: [http-routing]
    detection_evidence: [{file: package.json, path: .dependencies.express}]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	sout, serr, code := runScript(t, "--file", input, "--harness-dir", harness)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, serr)
	}
	if sout != "" {
		t.Fatalf("expected empty stdout on success, got %q", sout)
	}
	if _, err := os.Stat(filepath.Join(harness, "stack-manifest.yaml")); err != nil {
		t.Fatalf("stack-manifest not written: %v", err)
	}
}

func TestMain_ValidationFailure_ExitsTwoWithJSON(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(`schema_version: 1.0.0
archetype: not-a-real-archetype
components: []
`), 0o644); err != nil {
		t.Fatal(err)
	}
	sout, _, code := runScript(t, "--file", input, "--harness-dir", filepath.Join(dir, ".harness"))
	if code != 2 {
		t.Fatalf("exit=%d, want 2", code)
	}
	var pe persisterror.Error
	if err := json.Unmarshal([]byte(sout), &pe); err != nil {
		t.Fatalf("stdout is not a persisterror.Error JSON: %v\nstdout=%q", err, sout)
	}
	if pe.EntityType != "stack-manifest" {
		t.Fatalf("EntityType=%q, want stack-manifest", pe.EntityType)
	}
}

func TestMain_MissingFlag_ExitsOne(t *testing.T) {
	_, serr, code := runScript(t)
	if code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}
	if !strings.Contains(serr, "--file") {
		t.Fatalf("stderr=%q should mention --file", serr)
	}
}
```

- [ ] **Step 4: Run tests** — `go test ./skills/detect-stack/scripts/ -v` → PASS.

- [ ] **Step 5: Commit**

```bash
git add skills/detect-stack/scripts/
git commit -m "feat(skills): detect-stack script — read YAML, call stack.Persist, structured errors"
```

### Task 7.2: `skills/detect-use-cases/scripts/main.go`

Same shape as Task 7.1 but dispatches on `--type fixture|use-case` to choose between `fixture.Persist` and `usecase.Persist`.

- [ ] **Step 1: Write main.go** with `--type`, `--file`, `--harness-dir` flags. Switch on `*type`:
  - `fixture` → `fixture.Persist`
  - `use-case` → `usecase.Persist`
  - other → exit 1 with stderr.

- [ ] **Step 2: Build** → green.

- [ ] **Step 3: Three tests** — happy path for `fixture`, happy path for `use-case` (pre-seed fixture first), validation failure for `use-case` (missing fixture ref → exit 2, `Kind: FixtureBinding`).

- [ ] **Step 4: Run.** PASS.

- [ ] **Step 5: Commit.**

### Task 7.3: `skills/create-sensors/scripts/main.go`

Single `--file` + `--harness-dir`, calls `sensor.Persist`. Same structure as Task 7.1.

- [ ] **Step 1: Write main.go.**
- [ ] **Step 2: Build.**
- [ ] **Step 3: Two tests** — happy path (pre-seed full `.harness/` with stack + fixture + use-case), validation failure (sensor with unknown stack-component → exit 2, `Kind: Grounding`).
- [ ] **Step 4: Run.**
- [ ] **Step 5: Commit.**

---

## Phase 8 — Plugin manifest + SKILL.md bodies

### Task 8.1: Plugin manifest

**Files:**
- Create: `plugin.json` (or `.claude-plugin/plugin.json` — pick whichever the host Claude Code version expects; default to `.claude-plugin/plugin.json` per the more recent convention)

- [ ] **Step 1: Write the manifest**

```json
{
  "name": "lastro-harness",
  "version": "0.1.0",
  "description": "Use-case-driven validation framework. Provides /detect-stack, /detect-use-cases, /create-sensors slash commands."
}
```

- [ ] **Step 2: Commit**

```bash
git add .claude-plugin/plugin.json
git commit -m "feat(plugin): introduce Claude Code plugin manifest for the harness framework"
```

### Task 8.2: `skills/detect-stack/SKILL.md`

**Files:**
- Create: `skills/detect-stack/SKILL.md`

- [ ] **Step 1: Write the file**

```markdown
---
name: detect-stack
description: Detect the project archetype and stack components for the current repository, writing the result to .harness/stack-manifest.yaml.
---

# /detect-stack

You are detecting the stack of the repository at the current working directory and producing a `stack-manifest.yaml`.

## What to inspect
- Dependency manifests: `go.mod`, `package.json`, `pyproject.toml`, `Cargo.toml`, `Gemfile`, `pom.xml`, etc.
- Directory structure for archetype hints: `cmd/<x>/main.go` → CLI, HTTP route handlers → http-api, queue listeners → event-consumer, etc.
- Framework conventions visible in source files.

## What to emit
A single YAML file matching `schemas/stack-manifest.yaml`:

- `schema_version: 1.0.0` (will be patch-bumped on re-emit by the script)
- `archetype`: one of the values in `schemas/enums/archetypes.yaml` — `http-api`, `event-consumer`, `event-producer`, `cli`, `sdk`, `library`, `worker`, `batch-job`, `static-site`.
- `components`: a list of `StackComponent` entries — each with `id`, `kind`, `name`, `version`, `capabilities`, `detection_evidence`. See `schemas/examples/stack-component/*.yaml` for shapes.

**DO NOT emit `applicable_angles`** — the script injects this from the archetype's canonical list.

## How to write

1. Use the Write tool to put your YAML at `/tmp/stack-manifest.yaml`.
2. Run:
   ```bash
   go run ./skills/detect-stack/scripts/ --file /tmp/stack-manifest.yaml --harness-dir .harness
   ```
3. If exit code is `0`, you're done. `.harness/stack-manifest.yaml` has been written with `applicable_angles` filled in.
4. If exit code is `2`, read the JSON error from stdout. It looks like:
   ```json
   {"kind":"schema_violation","entity_type":"stack-manifest","path":"archetype","value":"foo","expected":"one of http-api|cli|...","message":"..."}
   ```
   Edit `/tmp/stack-manifest.yaml` to address the error and re-run. Stop after 3 attempts and report the unresolved error to the user.
5. If exit code is `1`, a script-level error (bad args, unreadable file) — read stderr and report to the user.
```

- [ ] **Step 2: Check line count** — must be ≤200 lines (CLAUDE.md rule 4). This is ~40, well under.

- [ ] **Step 3: Commit**

```bash
git add skills/detect-stack/SKILL.md
git commit -m "feat(skills): detect-stack SKILL.md — LLM-facing prompt body"
```

### Task 8.3: `skills/detect-use-cases/SKILL.md`

- [ ] **Step 1: Write the file** with the same structural sections (`What to inspect` / `What to emit` / `How to write`). Critical additions:
  - **Write ordering rule**: for each use case identified, write all its fixtures first (one `harness-write fixture` call per fixture), then the use case. The use-case write fails with `Kind: FixtureBinding` if a referenced fixture isn't on disk yet.
  - **Atomicity warning**: each write is per-entity. If a use-case write fails after some fixtures already landed, either fix the issue and retry, or `rm` the partial fixture files before reporting failure.
  - **Per-call script invocation example** for both `--type fixture` and `--type use-case`.
  - **Retry cap**: 3 attempts per write.

- [ ] **Step 2: Check line count.** Keep under 200.

- [ ] **Step 3: Commit.**

### Task 8.4: `skills/create-sensors/SKILL.md`

- [ ] **Step 1: Write the file.** Critical additions:
  - **Invocation surface**: `/create-sensors <use-case-id>`. The slash-command takes one use-case id; loop externally if covering many use cases.
  - **Read step**: instruct the LLM to first read `.harness/stack-manifest.yaml` (get `applicable_angles`) and `.harness/use-cases/<use-case-id>.yaml` (get the use case + its `fixture_ids`).
  - **Emit rule**: one sensor per angle in `applicable_angles`. Each sensor's top-level `uses:` must reference only ids from `stack-manifest.components`. Each step's `uses:` must reference only fixture ids whose `use_case_id` matches `<use-case-id>`.
  - **Verification step**: after all writes, `ls .harness/sensors/` and confirm exactly one sensor per angle. If any are missing, emit and write them.
  - **Retry cap**: 3 attempts per write.

- [ ] **Step 2: Check line count.**

- [ ] **Step 3: Commit.**

---

## Phase 9 — Full-stack acceptance (manual run against a sample repo)

These steps validate the spec's §11 acceptance criteria end to end. They are not automated tests (those are covered Phase by Phase); they confirm the integration story holds in a real Claude Code session.

### Task 9.1: Pick a sample repo

- [ ] Use the existing `examples/http-api-sample/` if present, or pick any small Go HTTP API project. Document the choice in a one-line note appended to this plan.

### Task 9.2: Dogfood the three skills end to end

- [ ] **Step 1: Restart Claude Code** so the new plugin manifest is picked up; confirm `/detect-stack`, `/detect-use-cases`, `/create-sensors` appear in the slash-command list.

- [ ] **Step 2: Run `/detect-stack`** in the sample repo. Confirm `.harness/stack-manifest.yaml` exists with `archetype: http-api`, components ≥ 1, and `applicable_angles` matching `internal/enums.ApplicableAngles[ArchetypeHTTPAPI]`.

- [ ] **Step 3: Run `/detect-use-cases`** in the same repo. Confirm: for at least one entry point, paired `.harness/use-cases/<id>.yaml` + `.harness/fixtures/<id>.yaml` files exist. Open the use-case file and confirm every `{{ }}` token references a real fixture or entry-point id.

- [ ] **Step 4: Re-run `/detect-stack`** on the same repo. Confirm `.harness/stack-manifest.yaml`'s `schema_version` patch-incremented (from `1.0.0` → `1.0.1`).

- [ ] **Step 5: Run `/create-sensors <use-case-id>`** for one of the detected use cases. Confirm `.harness/sensors/` contains exactly one file per angle in the manifest's `applicable_angles`. Spot-check one sensor: its `uses:` only references components in the manifest; each step's `uses:` only references fixtures whose `use_case_id` matches.

- [ ] **Step 6: Inject a deliberate failure.** Manually edit `/tmp/sensor.yaml` to add an unknown stack component to `uses:`. Re-run the underlying script:
   ```bash
   go run ./skills/create-sensors/scripts/ --file /tmp/sensor.yaml --harness-dir .harness
   ```
   Confirm exit code `2`, JSON stdout with `"kind":"grounding"`, and `.harness/sensors/<bad-id>.yaml` was not created.

- [ ] **Step 7: Commit a short note**

```bash
git add docs/superpowers/plans/2026-05-24-b4-detection-generation.md  # this file, with the sample repo recorded in Task 9.1
git commit -m "docs(b4): record dogfood results against sample HTTP API"
```

---

## Self-review checklist (run after all phases above)

- [ ] Every spec §11 acceptance criterion has a test or a manual step covering it (Phase 9).
- [ ] No `internal/detect/` or `internal/sensors/` package was created (spec decision #1, PR-review-blocker).
- [ ] No `cmd/harness-write` binary (spec decision #4).
- [ ] All four `Persist` functions return `*persisterror.Error` (spec §4.3).
- [ ] Schema-version bumping works on re-emit (spec §6.2; covered Tasks 3.3, 4.3, 5.3, 6.4).
- [ ] `stack-manifest.yaml` has `applicable_angles` field populated by the script, not the LLM (spec §6.1; covered Tasks 2.1–2.4).
- [ ] Each skill is ≤200 lines (CLAUDE.md rule 4; check via `wc -l skills/*/SKILL.md`).
- [ ] Loaders accept patch-bumped versions (spec §6.2; covered Phase 1).
- [ ] All Phase A loader tests still pass (`go test ./internal/...`).

---

**Plan complete and saved.**
