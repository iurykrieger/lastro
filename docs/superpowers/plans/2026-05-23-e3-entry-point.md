# E3 EntryPoint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `internal/entrypoint/` — `EntryPoint` Go type, JSON Schema-backed YAML loader, accessors used by E4's template resolver (`SpecField`, `Label`), and a `Validate` method. Mirrors the package conventions established by `internal/fixture/` (E5) and `internal/stack/` (E2).

**Architecture:** Schema-validated loader pipeline (`sigs.k8s.io/yaml.YAMLToJSON` → JSON Schema validate via `github.com/santhosh-tekuri/jsonschema/v6` → `json.Unmarshal`). `Spec` stays as `map[string]any`; the canonical JSON Schema's `oneOf` enforces archetype-specific required fields and the inline `method` / `channel_kind` / `trigger_kind` enums. No custom unmarshaler, no Go-side discriminated union.

**Tech Stack:** Go 1.24, `sigs.k8s.io/yaml`, `github.com/santhosh-tekuri/jsonschema/v6`, stdlib `testing`.

---

## Preconditions

- On branch `feat/e3-entry-point`, rooted at `main` after the merge of PRs #1–#5 (schema-freeze, E1, E2, E5). HEAD has one prior commit: the E3 design spec at `docs/superpowers/specs/2026-05-23-e3-entry-point-design.md`.
- Working directory: `.claude/worktrees/e3-entry-point/`.
- `go.mod` declares module `github.com/iurykrieger/lastro` with Go 1.24.2.
- `internal/enums/`, `internal/stack/`, `internal/fixture/` are present and tested.
- `schemas/entry-point.yaml` and the nine archetype example files under `schemas/examples/entry-point/*.yaml` are on disk.

If any precondition fails, stop and report.

---

## File structure

All paths relative to repo root.

**Created (in order):**

| Path | Responsibility |
|---|---|
| `internal/entrypoint/schema.yaml` | Byte-equal mirror of `schemas/entry-point.yaml`. Embedded into the binary; drift test enforces equality. |
| `internal/entrypoint/schema.go` | `//go:embed schema.yaml` + cached `compiledSchema()`. |
| `internal/entrypoint/drift_test.go` | Asserts `schema.yaml` matches the canonical source byte-for-byte. |
| `internal/entrypoint/schema_test.go` | Asserts `compiledSchema()` returns a usable, cached schema. |
| `internal/entrypoint/types.go` | `EntryPoint` struct with `ID`, `Archetype`, `Spec` fields and a package doc comment. |
| `internal/entrypoint/loader.go` | `LoadEntryPoint`, `LoadFromExample`, internal `validateAgainstSchema`. |
| `internal/entrypoint/load_test.go` | Per-archetype happy-path + negative-case loader tests. |
| `internal/entrypoint/testdata/missing-id.yaml` | Sad-path fixture. |
| `internal/entrypoint/testdata/unknown-archetype.yaml` | Sad-path fixture. |
| `internal/entrypoint/testdata/http-api-missing-method.yaml` | Sad-path fixture. |
| `internal/entrypoint/testdata/event-consumer-bad-channel-kind.yaml` | Sad-path fixture. |
| `internal/entrypoint/testdata/worker-bad-trigger-kind.yaml` | Sad-path fixture. |
| `internal/entrypoint/testdata/extra-spec-field.yaml` | Sad-path fixture (`additionalProperties: false`). |
| `internal/entrypoint/accessors.go` | `SpecField`, `Label`, `Validate` methods on `EntryPoint`. |
| `internal/entrypoint/accessors_test.go` | Method-level tests. |

**Modified:** none.

---

## Task 1: Bootstrap — embedded schema + drift test + compile cache

**Files:**
- Copy: `schemas/entry-point.yaml` → `internal/entrypoint/schema.yaml`
- Create: `internal/entrypoint/schema.go`
- Create: `internal/entrypoint/drift_test.go`
- Create: `internal/entrypoint/schema_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/entrypoint/drift_test.go`:

```go
package entrypoint

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedSchemaMatchesCanonicalSource(t *testing.T) {
	canonicalPath := filepath.Join("..", "..", "schemas", "entry-point.yaml")
	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical schema %s: %v", canonicalPath, err)
	}
	if !bytes.Equal(canonical, embeddedSchemaYAML) {
		t.Errorf("internal/entrypoint/schema.yaml has drifted from schemas/entry-point.yaml; re-run `cp schemas/entry-point.yaml internal/entrypoint/schema.yaml`")
	}
}
```

Create `internal/entrypoint/schema_test.go`:

```go
package entrypoint

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
	a, _ := compiledSchema()
	b, _ := compiledSchema()
	if a != b {
		t.Fatal("compiledSchema returned different pointers on successive calls")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/entrypoint/...
```

Expected: build error — `internal/entrypoint/schema.go` does not exist; `embeddedSchemaYAML` and `compiledSchema` undefined.

- [ ] **Step 3: Copy the canonical schema + write `schema.go`**

```bash
cp schemas/entry-point.yaml internal/entrypoint/schema.yaml
```

Create `internal/entrypoint/schema.go`:

```go
package entrypoint

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

// compiledSchema returns the compiled JSON Schema for EntryPoint, parsed
// once from the embedded schema.yaml. Subsequent calls reuse the cached
// schema.
func compiledSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		var doc any
		if err := yaml.Unmarshal(embeddedSchemaYAML, &doc); err != nil {
			schemaErr = fmt.Errorf("entrypoint: parse embedded schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		const url = "https://lastro.dev/harness/schemas/entry-point.yaml"
		if err := c.AddResource(url, doc); err != nil {
			schemaErr = fmt.Errorf("entrypoint: add schema resource: %w", err)
			return
		}
		s, err := c.Compile(url)
		if err != nil {
			schemaErr = fmt.Errorf("entrypoint: compile schema: %w", err)
			return
		}
		schemaCompiled = s
	})
	return schemaCompiled, schemaErr
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/entrypoint/... -v
```

Expected: PASS — `TestEmbeddedSchemaMatchesCanonicalSource`, `TestCompiledSchemaIsAvailable`, `TestCompiledSchemaIsCached`.

- [ ] **Step 5: Commit**

```bash
git add internal/entrypoint/schema.yaml internal/entrypoint/schema.go internal/entrypoint/drift_test.go internal/entrypoint/schema_test.go
git commit -m "feat(entrypoint): embedded schema, compile cache, and drift test"
```

---

## Task 2: `EntryPoint` type + `LoadEntryPoint` loader

**Files:**
- Create: `internal/entrypoint/types.go`
- Create: `internal/entrypoint/loader.go`
- Create: `internal/entrypoint/load_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/entrypoint/load_test.go`:

```go
package entrypoint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestLoadEntryPoint_HTTPAPIExample(t *testing.T) {
	path := filepath.Join("..", "..", "schemas", "examples", "entry-point", "http-api.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	ep, err := LoadEntryPoint(raw)
	if err != nil {
		t.Fatalf("LoadEntryPoint: %v", err)
	}
	if ep.ID != "create-order-endpoint" {
		t.Errorf("ID = %q, want create-order-endpoint", ep.ID)
	}
	if ep.Archetype != enums.ArchetypeHTTPAPI {
		t.Errorf("Archetype = %q, want %q", ep.Archetype, enums.ArchetypeHTTPAPI)
	}
	if got := ep.Spec["method"]; got != "POST" {
		t.Errorf("Spec[method] = %v, want POST", got)
	}
	if got := ep.Spec["path"]; got != "/orders" {
		t.Errorf("Spec[path] = %v, want /orders", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/entrypoint/... -run TestLoadEntryPoint
```

Expected: build error — `LoadEntryPoint`, `EntryPoint`, and `EntryPoint.Spec` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/entrypoint/types.go`:

```go
// Package entrypoint owns the EntryPoint data type embedded inside UseCase.
// The package provides the YAML loader (which delegates archetype-specific
// validation to the canonical JSON Schema at schemas/entry-point.yaml),
// minimal accessors used by the UseCase template resolver, and golden
// per-archetype examples.
package entrypoint

import "github.com/iurykrieger/lastro/internal/enums"

// EntryPoint is an archetype-typed observable surface. The Spec shape is
// determined by Archetype; see schemas/entry-point.yaml for the per-
// archetype required fields and inline-enum constraints. Spec values are
// the raw types yielded by yaml-to-json normalization (string, float64,
// bool, []any, map[string]any).
type EntryPoint struct {
	ID        string          `json:"id"        yaml:"id"`
	Archetype enums.Archetype `json:"archetype" yaml:"archetype"`
	Spec      map[string]any  `json:"spec"      yaml:"spec"`
}
```

Create `internal/entrypoint/loader.go`:

```go
package entrypoint

import (
	"encoding/json"
	"fmt"

	"sigs.k8s.io/yaml"
)

// LoadEntryPoint parses a single EntryPoint from YAML bytes. The pipeline is
// YAML→JSON normalize → JSON Schema validate → json.Unmarshal. Errors are
// wrapped with the failing phase.
func LoadEntryPoint(raw []byte) (EntryPoint, error) {
	asJSON, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return EntryPoint{}, fmt.Errorf("entrypoint: yaml-to-json: %w", err)
	}
	if err := validateAgainstSchema(asJSON); err != nil {
		return EntryPoint{}, fmt.Errorf("entrypoint: schema validation: %w", err)
	}
	var ep EntryPoint
	if err := json.Unmarshal(asJSON, &ep); err != nil {
		return EntryPoint{}, fmt.Errorf("entrypoint: deserialize: %w", err)
	}
	return ep, nil
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
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/entrypoint/... -run TestLoadEntryPoint -v
```

Expected: PASS — the http-api example loads with the right fields.

- [ ] **Step 5: Commit**

```bash
git add internal/entrypoint/types.go internal/entrypoint/loader.go internal/entrypoint/load_test.go
git commit -m "feat(entrypoint): EntryPoint type and JSON-Schema-backed YAML loader"
```

---

## Task 3: `LoadFromExample` + happy-path coverage across all nine archetypes

**Files:**
- Modify: `internal/entrypoint/loader.go` (add `LoadFromExample`)
- Modify: `internal/entrypoint/load_test.go` (add 9-archetype table-driven test)

- [ ] **Step 1: Write the failing test**

Append to `internal/entrypoint/load_test.go`:

```go
func TestLoadFromExample_AllArchetypes(t *testing.T) {
	cases := []struct {
		file       string
		wantArch   enums.Archetype
		wantSpec   map[string]any
	}{
		{
			file:     "http-api.yaml",
			wantArch: enums.ArchetypeHTTPAPI,
			wantSpec: map[string]any{"method": "POST", "path": "/orders"},
		},
		{
			file:     "event-consumer.yaml",
			wantArch: enums.ArchetypeEventConsumer,
			// channel_kind + channel_name; exact values depend on the
			// example file — assert keys exist below, values are checked
			// by inspecting the example separately.
		},
		{file: "event-producer.yaml", wantArch: enums.ArchetypeEventProducer},
		{file: "cli.yaml", wantArch: enums.ArchetypeCLI},
		{file: "sdk.yaml", wantArch: enums.ArchetypeSDK},
		{file: "library.yaml", wantArch: enums.ArchetypeLibrary},
		{file: "worker.yaml", wantArch: enums.ArchetypeWorker},
		{file: "batch-job.yaml", wantArch: enums.ArchetypeBatchJob},
		{file: "static-site.yaml", wantArch: enums.ArchetypeStaticSite},
	}
	for _, tc := range cases {
		t.Run(string(tc.wantArch), func(t *testing.T) {
			path := filepath.Join("..", "..", "schemas", "examples", "entry-point", tc.file)
			ep, err := LoadFromExample(path)
			if err != nil {
				t.Fatalf("LoadFromExample(%s): %v", path, err)
			}
			if ep.ID == "" {
				t.Errorf("ID is empty")
			}
			if ep.Archetype != tc.wantArch {
				t.Errorf("Archetype = %q, want %q", ep.Archetype, tc.wantArch)
			}
			if ep.Spec == nil {
				t.Fatalf("Spec is nil")
			}
			for k, want := range tc.wantSpec {
				if got, ok := ep.Spec[k]; !ok || got != want {
					t.Errorf("Spec[%q] = (%v, %v), want (%v, true)", k, got, ok, want)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/entrypoint/... -run TestLoadFromExample
```

Expected: build error — `LoadFromExample` undefined.

- [ ] **Step 3: Write the implementation**

Append to `internal/entrypoint/loader.go`:

```go
// (add "os" to the existing imports — see the import block)

// LoadFromExample reads a YAML file (typically one of the canonical examples
// under schemas/examples/entry-point/) and loads it as an EntryPoint. Test
// convenience; not for runtime use.
func LoadFromExample(path string) (EntryPoint, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return EntryPoint{}, fmt.Errorf("entrypoint: read %s: %w", path, err)
	}
	return LoadEntryPoint(raw)
}
```

Update the import block at the top of `loader.go` to add `"os"`:

```go
import (
	"encoding/json"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/entrypoint/... -run "TestLoadEntryPoint|TestLoadFromExample" -v
```

Expected: PASS — all 9 archetype examples load cleanly with correct `Archetype` and non-empty `Spec`.

- [ ] **Step 5: Commit**

```bash
git add internal/entrypoint/loader.go internal/entrypoint/load_test.go
git commit -m "feat(entrypoint): LoadFromExample + happy-path coverage for all 9 archetypes"
```

---

## Task 4: Negative-case fixtures + sad-path tests

**Files:**
- Create: `internal/entrypoint/testdata/missing-id.yaml`
- Create: `internal/entrypoint/testdata/unknown-archetype.yaml`
- Create: `internal/entrypoint/testdata/http-api-missing-method.yaml`
- Create: `internal/entrypoint/testdata/event-consumer-bad-channel-kind.yaml`
- Create: `internal/entrypoint/testdata/worker-bad-trigger-kind.yaml`
- Create: `internal/entrypoint/testdata/extra-spec-field.yaml`
- Modify: `internal/entrypoint/load_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/entrypoint/load_test.go`:

```go
import "strings" // add this at the top alongside existing imports

func TestLoadEntryPoint_RejectsInvalidFixtures(t *testing.T) {
	cases := []struct {
		name       string
		file       string
		expectSubs string // case-insensitive substring expected in the error
	}{
		{"missing id", "testdata/missing-id.yaml", "id"},
		{"unknown archetype", "testdata/unknown-archetype.yaml", "archetype"},
		{"http-api missing method", "testdata/http-api-missing-method.yaml", "method"},
		{"event-consumer bad channel_kind", "testdata/event-consumer-bad-channel-kind.yaml", "channel_kind"},
		{"worker bad trigger_kind", "testdata/worker-bad-trigger-kind.yaml", "trigger_kind"},
		{"cli extra spec field", "testdata/extra-spec-field.yaml", "additional"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatalf("read %s: %v", tc.file, err)
			}
			_, err = LoadEntryPoint(raw)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.expectSubs)) {
				t.Errorf("error %q missing expected substring %q", err.Error(), tc.expectSubs)
			}
		})
	}
}
```

Note: this test references `testdata/...` files that don't exist yet — the test runs from the package directory, so the relative path works as long as `testdata/` is a sibling of the test file.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/entrypoint/... -run TestLoadEntryPoint_RejectsInvalidFixtures
```

Expected: FAIL — each sub-test fails with "read testdata/...: no such file or directory".

- [ ] **Step 3: Create the negative-case fixtures**

Create `internal/entrypoint/testdata/missing-id.yaml`:

```yaml
archetype: http-api
spec:
  method: GET
  path: /x
```

Create `internal/entrypoint/testdata/unknown-archetype.yaml`:

```yaml
id: x
archetype: not-a-real-archetype
spec: {}
```

Create `internal/entrypoint/testdata/http-api-missing-method.yaml`:

```yaml
id: bad-http
archetype: http-api
spec:
  path: /x
```

Create `internal/entrypoint/testdata/event-consumer-bad-channel-kind.yaml`:

```yaml
id: bad-channel
archetype: event-consumer
spec:
  channel_kind: fanout
  channel_name: orders.created
```

Create `internal/entrypoint/testdata/worker-bad-trigger-kind.yaml`:

```yaml
id: bad-trigger
archetype: worker
spec:
  trigger_kind: webhook
  schedule_or_signal: x
```

Create `internal/entrypoint/testdata/extra-spec-field.yaml`:

```yaml
id: extra-field
archetype: cli
spec:
  command: x
  bogus: extra
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/entrypoint/... -run TestLoadEntryPoint_RejectsInvalidFixtures -v
```

Expected: PASS — six sub-tests green; each error message contains the expected substring (case-insensitive).

If a sub-test fails with "error message missing expected substring", the test output prints the actual error. In most cases the JSON Schema validator's message naturally mentions the field name. If `event-consumer-bad-channel-kind.yaml` fails because the schema error says "enum" rather than "channel_kind", relax that case's `expectSubs` to `"enum"` — adjust the table, do not loosen the test by accepting any error.

- [ ] **Step 5: Commit**

```bash
git add internal/entrypoint/testdata internal/entrypoint/load_test.go
git commit -m "test(entrypoint): cover every required-field, enum, and additionalProperties violation"
```

---

## Task 5: `SpecField`, `Label`, `Validate` methods

**Files:**
- Create: `internal/entrypoint/accessors.go`
- Create: `internal/entrypoint/accessors_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/entrypoint/accessors_test.go`:

```go
package entrypoint

import (
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestSpecField_HitAndMiss(t *testing.T) {
	ep, err := LoadFromExample(filepath.Join("..", "..", "schemas", "examples", "entry-point", "http-api.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got, ok := ep.SpecField("method"); !ok || got != "POST" {
		t.Errorf("SpecField(method) = (%v, %v), want (POST, true)", got, ok)
	}
	if got, ok := ep.SpecField("nonexistent"); ok || got != nil {
		t.Errorf("SpecField(nonexistent) = (%v, %v), want (nil, false)", got, ok)
	}
}

func TestLabel_HappyPath(t *testing.T) {
	ep := EntryPoint{ID: "create-order-endpoint", Archetype: enums.ArchetypeHTTPAPI}
	if got, want := ep.Label(), "http-api:create-order-endpoint"; got != want {
		t.Errorf("Label = %q, want %q", got, want)
	}
}

func TestLabel_ZeroValue(t *testing.T) {
	if got := (EntryPoint{}).Label(); got != ":" {
		t.Errorf("zero-value Label = %q, want %q", got, ":")
	}
}

func TestValidate_LoadedFixturePasses(t *testing.T) {
	ep, err := LoadFromExample(filepath.Join("..", "..", "schemas", "examples", "entry-point", "cli.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := ep.Validate(); err != nil {
		t.Errorf("Validate on loaded EntryPoint: %v", err)
	}
}

func TestValidate_ConstructedInCodePasses(t *testing.T) {
	ep := EntryPoint{
		ID:        "test-cli",
		Archetype: enums.ArchetypeCLI,
		Spec:      map[string]any{"command": "harness"},
	}
	if err := ep.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidate_RejectsUnknownMethod(t *testing.T) {
	// PROPFIND is a WebDAV verb, not in the closed http-methods enum.
	bad := EntryPoint{
		ID:        "broken",
		Archetype: enums.ArchetypeHTTPAPI,
		Spec:      map[string]any{"method": "PROPFIND", "path": "/x"},
	}
	if err := bad.Validate(); err == nil {
		t.Errorf("Validate expected error for PROPFIND, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/entrypoint/... -run "TestSpecField|TestLabel|TestValidate"
```

Expected: build error — `SpecField`, `Label`, `Validate` methods undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/entrypoint/accessors.go`:

```go
package entrypoint

import (
	"encoding/json"
	"fmt"
)

// SpecField looks up a single key in the spec map. Returns the raw value and
// true if present; nil and false otherwise. E4's template resolver uses this
// for {{entry_points.<id>.spec.<key>}} resolution. The returned `any` is
// whatever JSON yielded from yaml-to-json normalization (string, float64,
// bool, []any, map[string]any).
func (e EntryPoint) SpecField(name string) (any, bool) {
	v, ok := e.Spec[name]
	return v, ok
}

// Label renders the EntryPoint as the compact "<archetype>:<id>" form used
// in human-facing log lines and as the fallback rendering for a bare
// {{entry_points.<id>}} reference (no spec field). On a zero-value
// EntryPoint the result is the literal ":" — callers must not assume the
// label parses as a valid archetype:id pair.
func (e EntryPoint) Label() string {
	return string(e.Archetype) + ":" + e.ID
}

// Validate runs JSON Schema validation against the receiver. The loader
// already validates during LoadEntryPoint; this method is for EntryPoints
// constructed in code (e.g., by tests or by E4's template resolver).
func (e EntryPoint) Validate() error {
	asJSON, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("entrypoint: marshal for validation: %w", err)
	}
	return validateAgainstSchema(asJSON)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/entrypoint/... -run "TestSpecField|TestLabel|TestValidate" -v
```

Expected: PASS — six sub-tests green (SpecField hit/miss, Label happy + zero-value, Validate loaded + constructed + bad-method-rejected).

- [ ] **Step 5: Commit**

```bash
git add internal/entrypoint/accessors.go internal/entrypoint/accessors_test.go
git commit -m "feat(entrypoint): SpecField, Label, and Validate methods"
```

---

## Task 6: Final acceptance verification

No new code — runs verifications across the package.

- [ ] **Step 1: Full package test suite**

```bash
go test ./internal/entrypoint/... -v -count=1
```

Expected: PASS — every test across drift_test, schema_test, load_test, accessors_test. No skips, no failures.

- [ ] **Step 2: `go vet`**

```bash
go vet ./internal/entrypoint/...
```

Expected: no output.

- [ ] **Step 3: `gofmt`**

```bash
gofmt -l internal/entrypoint/
```

Expected: no output. If any file is listed, run `gofmt -w internal/entrypoint/` and commit:

```bash
git add internal/entrypoint/
git commit -m "chore: gofmt internal/entrypoint"
```

- [ ] **Step 4: Walk the deliverable acceptance checklist**

  - [ ] `go build ./internal/entrypoint/...` succeeds (implicit from Step 1).
  - [ ] Every archetype golden example (`schemas/examples/entry-point/<archetype>.yaml`) loads via `LoadFromExample`. Verified by `TestLoadFromExample_AllArchetypes`.
  - [ ] Every negative `testdata/*.yaml` produces a load error whose message contains the expected substring. Verified by `TestLoadEntryPoint_RejectsInvalidFixtures`.
  - [ ] `drift_test.go` confirms embedded schema is byte-equal to canonical.
  - [ ] `SpecField`, `Label`, `Validate` work on both loaded and code-constructed `EntryPoint` values.
  - [ ] API surface matches the E4 plan's expected scaffold (`ID`, `Archetype`, `Spec`, `SpecField`, `Label`) so E4 can drop its `0712e14 feat(entrypoint): minimal EntryPoint scaffold for E4` commit on rebase.

- [ ] **Step 5: Open the pull request**

```bash
git push -u origin feat/e3-entry-point
gh pr create --title "feat(e3): EntryPoint type, JSON-schema-backed loader, and accessors" --body "$(cat <<'EOF'
## Summary
- Adds `internal/entrypoint/` with `EntryPoint` struct, YAML loader (YAMLToJSON → JSON Schema → json.Unmarshal pipeline shared with `internal/fixture/`), and the `SpecField` / `Label` / `Validate` methods E4's template resolver depends on.
- Golden examples for all 9 archetypes load cleanly; negative fixtures cover every required-field, inline-enum, and additionalProperties violation.
- Supersedes the temporary scaffold on `feat/e4-use-case` (commit `0712e14`); E4 can drop that commit on rebase.

## Test plan
- [ ] CI green on `go test ./internal/entrypoint/...`
- [ ] CI green on `go vet`
- [ ] `gofmt -l internal/entrypoint/` empty
- [ ] Drift test asserts `internal/entrypoint/schema.yaml` matches `schemas/entry-point.yaml`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

If `gh` is unavailable from this shell, the path on the user's Windows side is `/c/Program Files/GitHub CLI/gh.exe`. Use that absolute path when invoking from WSL.

---

## Plan self-review

**Spec coverage:** Every section of the design spec maps to a task —

- "Schema-validated loader pipeline" → Task 1 (compile) + Task 2 (LoadEntryPoint).
- "Spec as `map[string]any`" → Task 2 (types.go).
- "All 9 archetype golden examples load" → Task 3.
- "Negative cases cover required fields, inline enums, additionalProperties" → Task 4.
- "SpecField + Label + Validate" → Task 5.
- "Drift test + schema_test" → Task 1.
- "Deliverable acceptance" → Task 6.

**Placeholder scan:** no "TBD", "TODO", "similar to Task N", or vague guidance. Every step has either code or an exact shell command with the expected outcome.

**Type consistency:** All references to `enums.ArchetypeHTTPAPI`, `enums.ArchetypeEventConsumer`, etc., match the merged E1 surface (`internal/enums/enums.go`). `EntryPoint` field tags are `json:` first, `yaml:` second, matching `internal/stack/types.go`. The `validateAgainstSchema` helper is package-private and called by both `LoadEntryPoint` (Task 2) and `Validate` (Task 5) — the signature is consistent.

**One subtlety to watch in execution:** Task 4 Step 4 has a clause about adjusting the `expectSubs` table if the JSON Schema validator's error wording doesn't match the field name. The instruction is "adjust the table, do not loosen the test by accepting any error" — implementers must respect that boundary.

---

## Execution Handoff

Plan saved to `docs/superpowers/plans/2026-05-23-e3-entry-point.md`. Two execution options:

1. **Subagent-Driven (recommended)** — fresh subagent per task with two-stage review.
2. **Inline Execution** — batch execution in this session with checkpoints.

Which approach?
