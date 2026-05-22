# E2 — StackComponent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `internal/stack/` — the Go package that owns `StackComponent` and `StackManifest` types, the YAML+JSON-Schema loader, the programmatic validator, and the accessors — along with the schema-level changes required to support it (`stack-component.yaml` refinement, new `stack-manifest.yaml`, new `stack-kinds` enum, refreshed examples).

**Architecture:** Two YAML JSON Schemas (`stack-component.yaml`, `stack-manifest.yaml`) under `schemas/` are the structural contract. Validation runs in two layers: JSON Schema at load time (structural correctness against the gate-frozen contract) plus programmatic Go validation for invariants JSON Schema can't express (id uniqueness across components, cross-field consistency). Schemas are embedded into the binary via a new `schemas/schemas.go` `embed.FS` so the Go loader has no runtime dependency on the repo working tree. `StackKind` joins `internal/enums/` as the 9th fixed enum following the existing one-file/multi-enum pattern. The package exposes minimal accessors (`ByID`, `HasCapability`, `ComponentsWithCapability`) plus a non-blocking `LintCapabilities` helper for the recognized-capabilities drift check.

**Tech Stack:** Go 1.24, `sigs.k8s.io/yaml` (YAML→JSON unmarshal), `github.com/santhosh-tekuri/jsonschema/v6` (JSON Schema 2020-12 validation). Both already in `go.mod`. Standard library `embed` for schema embedding.

**Source spec:** [`docs/superpowers/specs/2026-05-22-e2-stack-component-design.md`](../specs/2026-05-22-e2-stack-component-design.md)

**Branch note:** Plan is written assuming execution on `feat/e1-enums` (where the spec was committed) or on a branch cut from it. Tasks 5–7 extend E1; if E1 has already merged, rebase before Task 5.

**Implementation deviations from the spec (explicit):**
- **No `internal/stack/testdata/`.** Positive golden round-trip uses the existing `schemas/examples/stack-{component,manifest}/*.yaml` files (cleaner, no duplication, and ensures the package and the schema validator agree on the same fixtures). Negative cases use table-driven inline YAML strings inside `stack_test.go`.
- **One package test file** (`internal/stack/stack_test.go`) per the spec; sub-sections within it are grouped by helper region.

---

## File Structure

**Phase 1 — schema refinement:**
- Modify: `schemas/stack-component.yaml` (tighten required + change `detection_evidence` shape; update inline-enum description for `kind`)
- Modify: `schemas/examples/stack-component/{datastore,framework,library,protocol,runtime,tool}.yaml` (migrate evidence to structured shape)
- Create: `schemas/stack-manifest.yaml` (new JSON Schema for the container)
- Create: `schemas/examples/stack-manifest/http-api.yaml` (golden example)
- Modify: `cmd/validate-schemas/main.go` (register `stack-manifest`)

**Phase 2 — StackKind enum (E1 extension):**
- Create: `schemas/enums/stack-kinds.yaml`
- Modify: `cmd/validate-schemas/main.go` (register `stack-kinds`)
- Modify: `internal/enums/enums.go` (add `StackKind` type + constants + helpers; update package doc)
- Modify: `internal/enums/enums_test.go` (add `StackKind` tests)

**Phase 3 — schema embed package:**
- Create: `schemas/schemas.go` (`//go:embed` of all schemas + enums into an `embed.FS`)

**Phase 4 — `internal/stack/` package:**
- Create: `internal/stack/types.go` (`StackComponent`, `EvidenceRef`, `StackManifest`, `SchemaVersion` constant, `EvidenceRef.String()`)
- Create: `internal/stack/validate.go` (`StackComponent.Validate`, `StackManifest.Validate`, aggregated-error helper)
- Create: `internal/stack/load.go` (`Load`, `LoadComponent`, JSON Schema compiler with embedded schemas, `byID` build)
- Create: `internal/stack/accessors.go` (`ByID`, `HasCapability`, `ComponentsWithCapability`, `LintCapabilities`, `LintWarning`)
- Create: `internal/stack/stack_test.go` (one comprehensive test file covering all of the above)

---

## Task 1: Reshape `schemas/stack-component.yaml`

Tighten required fields and change `detection_evidence` items from plain strings to the structured `{file, path, value?}` object the design specifies. This is the contract change that all subsequent work depends on.

**Files:**
- Modify: `schemas/stack-component.yaml`

- [ ] **Step 1: Open the current schema and replace its contents**

Replace the entire file with this content:

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/stack-component.yaml"
title: StackComponent
description: |
  A detected library, runtime, framework, datastore, protocol, or tool present
  in the repository. Sensors may only reference StackComponents whose ids
  appear in the detected stack manifest (grounding invariant).

type: object
required: [schema_version, id, kind, name, version, capabilities, detection_evidence]
additionalProperties: false

properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  id:
    $ref: "#/$defs/Id"
  kind:
    type: string
    description: |
      StackComponent classification. Canonical source is
      schemas/enums/stack-kinds.yaml; this inline enum must stay in sync
      (drift contract).
    enum: [library, runtime, framework, datastore, protocol, tool]
  name:
    type: string
    minLength: 1
  version:
    type: string
    minLength: 1
  capabilities:
    type: array
    minItems: 1
    items: { type: string, minLength: 1 }
  detection_evidence:
    type: array
    minItems: 1
    items: { $ref: "#/$defs/Evidence" }

$defs:
  Id:
    type: string
    pattern: "^[a-z][a-z0-9-]*$"
    minLength: 1
    maxLength: 128
  Evidence:
    type: object
    required: [file, path]
    additionalProperties: false
    properties:
      file:
        type: string
        minLength: 1
      path:
        type: string
        minLength: 1
      value:
        type: string
```

- [ ] **Step 2: Run the schema validator to confirm the schema itself is still valid JSON Schema 2020-12**

Run from the repo root:

```bash
go run ./cmd/validate-schemas
```

Expected: line `OK schemas/stack-component.yaml is a valid JSON Schema 2020-12` appears. The example validation lines for `schemas/examples/stack-component/*.yaml` will **fail** (existing examples use the old string-form evidence). That is expected and is the trigger for Task 2.

- [ ] **Step 3: Commit**

```bash
git add schemas/stack-component.yaml
git commit -m "feat(e2): tighten stack-component schema and restructure detection_evidence

Make version/capabilities/detection_evidence required, add minItems
constraints, and reshape evidence items from plain strings to
{file, path, value?} objects. Mark the inline kind enum as
co-canonical with the soon-to-land stack-kinds.yaml.

Examples will be migrated in the next commit; validator will fail
between commits."
```

---

## Task 2: Migrate the 6 existing stack-component examples to structured evidence

Move every example to the new `detection_evidence` shape so the schema validator returns to green.

**Files:**
- Modify: `schemas/examples/stack-component/datastore.yaml`
- Modify: `schemas/examples/stack-component/framework.yaml`
- Modify: `schemas/examples/stack-component/library.yaml`
- Modify: `schemas/examples/stack-component/protocol.yaml`
- Modify: `schemas/examples/stack-component/runtime.yaml`
- Modify: `schemas/examples/stack-component/tool.yaml`

- [ ] **Step 1: Rewrite `datastore.yaml`**

```yaml
# StackComponent example — datastore kind (Postgres).
schema_version: 1.0.0
id: postgres
kind: datastore
name: postgres
version: 16.x
capabilities:
  - transactions
  - row-level-security
  - logical-replication
detection_evidence:
  - file: docker-compose.yaml
    path: services.db.image
    value: postgres:16
  - file: package.json
    path: dependencies.pg
    value: ^8.11.0
```

- [ ] **Step 2: Rewrite `framework.yaml`**

```yaml
# StackComponent example — framework kind (NestJS).
schema_version: 1.0.0
id: nestjs
kind: framework
name: "@nestjs/core"
version: 10.x
capabilities:
  - dependency-injection
  - http-routing
  - middleware
detection_evidence:
  - file: package.json
    path: dependencies.@nestjs/core
    value: ^10.0.0
```

- [ ] **Step 3: Rewrite `library.yaml`**

```yaml
# StackComponent example — a library kind (Express HTTP framework).
schema_version: 1.0.0
id: express
kind: library
name: express
version: 4.18.x
capabilities:
  - http-routing
  - middleware
  - json-body-parsing
detection_evidence:
  - file: package.json
    path: dependencies.express
    value: ^4.18.0
```

- [ ] **Step 4: Rewrite `protocol.yaml`**

```yaml
# StackComponent example — protocol kind (gRPC).
schema_version: 1.0.0
id: grpc
kind: protocol
name: grpc
version: "1.60"
capabilities:
  - streaming
  - http2-transport
  - protobuf-codec
detection_evidence:
  - file: package.json
    path: dependencies.@grpc/grpc-js
    value: ^1.60.0
```

- [ ] **Step 5: Rewrite `runtime.yaml`**

```yaml
# StackComponent example — runtime kind (Node).
schema_version: 1.0.0
id: node
kind: runtime
name: node
version: 20.x
capabilities:
  - event-loop
  - child-processes
  - native-tls
detection_evidence:
  - file: package.json
    path: engines.node
    value: ">=20"
```

- [ ] **Step 6: Rewrite `tool.yaml`**

The original second evidence (`.eslintrc.json`) was presence-only — no `:path` portion. Use the JSONPath root `$` as a sentinel meaning "the file's presence is the evidence" (it's just a non-empty string as far as the schema is concerned; convention documented inline by the surrounding examples).

```yaml
# StackComponent example — tool kind (ESLint).
schema_version: 1.0.0
id: eslint
kind: tool
name: eslint
version: 9.x
capabilities:
  - lint
  - autofix
detection_evidence:
  - file: package.json
    path: devDependencies.eslint
    value: ^9.0.0
  - file: .eslintrc.json
    path: $
```

- [ ] **Step 7: Run the schema validator and confirm all 6 examples pass**

Run from the repo root:

```bash
go run ./cmd/validate-schemas
```

Expected: 6 lines `OK schemas/examples/stack-component/<kind>.yaml passes stack-component.yaml` appear with no errors. The final line `All schemas, enums, and examples validated.` prints.

- [ ] **Step 8: Commit**

```bash
git add schemas/examples/stack-component/
git commit -m "feat(e2): migrate stack-component examples to structured evidence

Each detection_evidence string becomes {file, path[, value]}. Where
the original was presence-only (.eslintrc.json), path uses the
JSONPath root sentinel \$ to satisfy the now-required field. Validator
returns to green."
```

---

## Task 3: Add `schemas/stack-manifest.yaml` (new container schema)

The container that bundles `archetype` + an ordered list of `StackComponent`. Lives next to the existing entity schemas; the schema-freeze gate did not include it because it is not an "entity" in the gate's sense, but it is what `/detect-stack` will eventually produce.

**Files:**
- Create: `schemas/stack-manifest.yaml`

- [ ] **Step 1: Create the file**

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/stack-manifest.yaml"
title: StackManifest
description: |
  The full detected stack for a repository: its archetype plus the ordered
  list of detected StackComponents. Produced by /detect-stack and consumed
  by every other harness skill as the "toolbox" that grounds sensor
  generation.

type: object
required: [schema_version, archetype, components]
additionalProperties: false

properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  archetype:
    type: string
    description: |
      The repository's archetype. Canonical source is
      schemas/enums/archetypes.yaml; this inline enum stays in sync via the
      drift contract.
    enum:
      - http-api
      - event-consumer
      - event-producer
      - cli
      - sdk
      - library
      - worker
      - batch-job
      - static-site
  components:
    type: array
    minItems: 1
    items:
      $ref: "https://lastro.dev/harness/schemas/stack-component.yaml"
```

- [ ] **Step 2: Confirm the file is syntactically valid YAML**

Run from the repo root:

```bash
go run ./cmd/validate-schemas
```

Expected: existing schemas continue to validate. The validator does **not** yet know about `stack-manifest.yaml`; that is wired in Task 4. No new line about `stack-manifest.yaml` should appear, and no errors should appear. If the run errors out on YAML parsing, fix the indentation in the new file.

- [ ] **Step 3: Commit**

```bash
git add schemas/stack-manifest.yaml
git commit -m "feat(e2): add stack-manifest.yaml JSON Schema

New container schema bundling archetype + ordered components. References
the existing stack-component.yaml via canonical \$id URL. Not yet wired
into the validator entrypoint."
```

---

## Task 4: Register `stack-manifest` in the schema validator and add a golden example

Wire the new schema into `cmd/validate-schemas/main.go` and add the first example. Once this lands, the gate-validator covers it.

**Files:**
- Modify: `cmd/validate-schemas/main.go`
- Create: `schemas/examples/stack-manifest/http-api.yaml`

- [ ] **Step 1: Add `stack-manifest` to the entities list**

In `cmd/validate-schemas/main.go`, locate the `entities` declaration at the top of the file (it currently lists 8 entries). Append `"stack-manifest"`:

```go
var entities = []string{
    "stack-component", "entry-point", "use-case", "fixture",
    "sensor", "signal", "aggregate-signal", "validation-policy",
    "stack-manifest",
}
```

- [ ] **Step 2: Create the golden example directory and `http-api.yaml` example**

Path: `schemas/examples/stack-manifest/http-api.yaml`

```yaml
# StackManifest example — http-api archetype, two components.
schema_version: 1.0.0
archetype: http-api
components:
  - schema_version: 1.0.0
    id: nestjs
    kind: framework
    name: "@nestjs/core"
    version: 10.x
    capabilities:
      - dependency-injection
      - http-routing
      - middleware
    detection_evidence:
      - file: package.json
        path: dependencies.@nestjs/core
        value: ^10.0.0
  - schema_version: 1.0.0
    id: postgres
    kind: datastore
    name: postgres
    version: 16.x
    capabilities:
      - transactions
      - row-level-security
    detection_evidence:
      - file: docker-compose.yaml
        path: services.db.image
        value: postgres:16
```

- [ ] **Step 3: Run the validator and confirm the manifest schema + example both validate**

Run from the repo root:

```bash
go run ./cmd/validate-schemas
```

Expected output additions:
- `OK schemas/stack-manifest.yaml is a valid JSON Schema 2020-12`
- `OK schemas/examples/stack-manifest/http-api.yaml passes stack-manifest.yaml`
- Final line: `All schemas, enums, and examples validated.`

If the example fails with a `$ref` resolution error, double-check that `stack-component.yaml` is registered in `entities` (it should already be — it has been since the gate).

- [ ] **Step 4: Commit**

```bash
git add cmd/validate-schemas/main.go schemas/examples/stack-manifest/
git commit -m "feat(e2): register stack-manifest in validator and add golden example

Adds stack-manifest to the entities list in cmd/validate-schemas/main.go
and the first happy-path example (http-api archetype, two components).
Gate validator now covers the new schema end-to-end."
```

---

## Task 5: Add `schemas/enums/stack-kinds.yaml` (9th enum file)

Mirror the existing enum-file pattern (`schema_version`, `title`, `description`, `values: [{id, purpose, ...}]`). Each value needs at minimum `id` and `purpose` per the meta-schema.

**Files:**
- Create: `schemas/enums/stack-kinds.yaml`
- Modify: `cmd/validate-schemas/main.go`

- [ ] **Step 1: Create the enum file**

```yaml
schema_version: 1.0.0
title: StackKind
description: |
  Classification of a detected StackComponent. Closed enum; new values
  require a framework version bump. Values are kept in sync with the
  inline enum in schemas/stack-component.yaml via the drift contract.

values:
  - id: library
    purpose: "A reusable code module imported by application code (e.g., lodash, requests)"
  - id: runtime
    purpose: "The language runtime or virtual machine the application executes on (e.g., node, jvm, python)"
  - id: framework
    purpose: "An opinionated scaffold that inverts control of the application (e.g., nestjs, django, rails)"
  - id: datastore
    purpose: "Persistent storage the application reads or writes (e.g., postgres, redis, s3)"
  - id: protocol
    purpose: "A wire protocol or message format the application speaks (e.g., grpc, kafka, mqtt)"
  - id: tool
    purpose: "A build, packaging, or operational utility (e.g., docker, webpack, terraform)"
```

- [ ] **Step 2: Register the enum in the validator**

In `cmd/validate-schemas/main.go`, locate the `enums` declaration and append `"stack-kinds"`:

```go
var enums = []string{
    "validation-angles", "archetypes", "sensor-kinds", "sensor-natures",
    "signal-output-types", "fixture-roles", "verdicts", "termination-reasons",
    "stack-kinds",
}
```

- [ ] **Step 3: Run the validator and confirm the enum validates against `_meta.yaml`**

Run from the repo root:

```bash
go run ./cmd/validate-schemas
```

Expected: `OK schemas/enums/stack-kinds.yaml matches _meta.yaml` appears alongside the others. Final line: `All schemas, enums, and examples validated.`

- [ ] **Step 4: Commit**

```bash
git add schemas/enums/stack-kinds.yaml cmd/validate-schemas/main.go
git commit -m "feat(e1): add StackKind enum file (9th fixed enum)

New canonical source for the kind classification used by StackComponent.
Values mirror the inline enum already in stack-component.yaml; drift
contract keeps them in sync."
```

---

## Task 6: Add `StackKind` Go type, constants, and validators

Extend `internal/enums/enums.go` with the 9th enum, following the exact pattern of the eight already there. Update the package doc comment that currently says "eight fixed enums".

**Files:**
- Modify: `internal/enums/enums.go`
- Modify: `internal/enums/enums_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/enums/enums_test.go`:

```go
func TestAllStackKindsReturnsCanonicalOrder(t *testing.T) {
    got := AllStackKinds()
    want := []StackKind{
        StackKindLibrary, StackKindRuntime, StackKindFramework,
        StackKindDatastore, StackKindProtocol, StackKindTool,
    }
    if len(got) != len(want) {
        t.Fatalf("AllStackKinds length: got %d, want %d", len(got), len(want))
    }
    for i := range want {
        if got[i] != want[i] {
            t.Errorf("AllStackKinds[%d]: got %q, want %q", i, got[i], want[i])
        }
    }
}

func TestIsValidStackKindAcceptsCanonical(t *testing.T) {
    for _, v := range AllStackKinds() {
        if !IsValidStackKind(string(v)) {
            t.Errorf("IsValidStackKind(%q) = false; want true", v)
        }
    }
}

func TestIsValidStackKindRejectsUnknown(t *testing.T) {
    cases := []string{"", "database", "LIBRARY", " tool", "service"}
    for _, c := range cases {
        if IsValidStackKind(c) {
            t.Errorf("IsValidStackKind(%q) = true; want false", c)
        }
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/enums/...
```

Expected: FAIL with messages like `undefined: AllStackKinds`, `undefined: StackKind`, etc.

- [ ] **Step 3: Update the package doc and add the StackKind block to `enums.go`**

In `internal/enums/enums.go`:

(a) Update the package comment at the top:

```go
// Package enums provides typed constants and validators for the framework's
// nine fixed enums, plus the canonical archetype × angle matrix.
//
// The canonical source for every enum is YAML under schemas/enums/. Drift
// between this package and that source is caught by drift_test.go.
package enums
```

(b) Append the new enum at the end of the file, after the `FixtureRole` block:

```go
// StackKind is the classification of a detected StackComponent.
type StackKind string

const (
    StackKindLibrary   StackKind = "library"
    StackKindRuntime   StackKind = "runtime"
    StackKindFramework StackKind = "framework"
    StackKindDatastore StackKind = "datastore"
    StackKindProtocol  StackKind = "protocol"
    StackKindTool      StackKind = "tool"
)

// AllStackKinds returns every StackKind in canonical (YAML) order.
func AllStackKinds() []StackKind {
    return []StackKind{
        StackKindLibrary, StackKindRuntime, StackKindFramework,
        StackKindDatastore, StackKindProtocol, StackKindTool,
    }
}

// IsValidStackKind reports whether s is one of the canonical StackKind values.
func IsValidStackKind(s string) bool {
    for _, v := range AllStackKinds() {
        if string(v) == s {
            return true
        }
    }
    return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/enums/...
```

Expected: PASS. All previous enum tests still pass too.

- [ ] **Step 5: Commit**

```bash
git add internal/enums/enums.go internal/enums/enums_test.go
git commit -m "feat(e1): add StackKind Go enum + helpers

Mirrors the pattern of the existing eight enums. Updates package
doc to reflect the count. Sourced from schemas/enums/stack-kinds.yaml."
```

---

## Task 7: Add a drift test for `StackKind`

The existing enums package has a `drift_test.go` convention (referenced by the package doc comment as the mechanism that catches Go-vs-YAML drift). If that file exists, extend it. If it does not yet exist, skip this task — the next agent can add it as part of E1 wrap-up.

**Files:**
- Modify: `internal/enums/drift_test.go` (if it exists)

- [ ] **Step 1: Check whether `drift_test.go` exists**

```bash
ls internal/enums/
```

If `drift_test.go` is **not** listed, skip directly to Step 3 (the "no-op" commit), which simply records this task as intentionally deferred.

- [ ] **Step 2: If it exists, extend it for StackKind**

Read `internal/enums/drift_test.go` to learn its pattern, then add a parallel test case that loads `schemas/enums/stack-kinds.yaml`, extracts `values[*].id`, and asserts equality (in order) with `[]string{string(StackKindLibrary), string(StackKindRuntime), string(StackKindFramework), string(StackKindDatastore), string(StackKindProtocol), string(StackKindTool)}`. Mirror the helper functions and table-driven shape of the existing file exactly — do not reinvent.

Run: `go test ./internal/enums/...` — expected PASS.

Commit:

```bash
git add internal/enums/drift_test.go
git commit -m "test(e1): cover StackKind in enum drift test"
```

- [ ] **Step 3: If it does not exist, skip with no commit**

Move on to Task 8. The drift test will be added when E1's drift-test work lands; the test would reduce to a parallel copy of the existing pattern at that point, and there is nothing useful to add today.

---

## Task 8: Create `schemas/schemas.go` (embed package)

A tiny package that exposes an `embed.FS` containing every YAML schema and enum. Internal packages import it instead of fishing for files on disk. This decouples the package loaders from the runtime working tree.

**Files:**
- Create: `schemas/schemas.go`

- [ ] **Step 1: Create the file**

```go
// Package schemas embeds the canonical YAML JSON Schemas and enum files
// for every harness framework entity, so internal packages can validate
// inputs without depending on the repo working tree at runtime.
package schemas

import "embed"

// FS exposes every entity schema (e.g., stack-component.yaml,
// stack-manifest.yaml) and every enum file (under enums/).
// Examples and README are intentionally excluded.
//
//go:embed *.yaml enums/*.yaml
var FS embed.FS
```

- [ ] **Step 2: Confirm the package compiles and embedding picks up the expected files**

Run from the repo root:

```bash
go build ./schemas/...
```

Expected: no output (success). If embedding fails because no files match a pattern, double-check that both `schemas/*.yaml` and `schemas/enums/*.yaml` contain at least one file (they do — multiple of each exist).

- [ ] **Step 3: Write a tiny smoke test to confirm the manifest schema is reachable**

Create `schemas/schemas_test.go`:

```go
package schemas

import (
    "testing"
)

func TestFSContainsKeySchemas(t *testing.T) {
    wanted := []string{
        "stack-component.yaml",
        "stack-manifest.yaml",
        "enums/stack-kinds.yaml",
        "enums/archetypes.yaml",
    }
    for _, name := range wanted {
        b, err := FS.ReadFile(name)
        if err != nil {
            t.Errorf("FS.ReadFile(%q): %v", name, err)
            continue
        }
        if len(b) == 0 {
            t.Errorf("FS.ReadFile(%q): empty file", name)
        }
    }
}
```

- [ ] **Step 4: Run the test**

```bash
go test ./schemas/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add schemas/schemas.go schemas/schemas_test.go
git commit -m "feat(e2): add schemas embed.FS package

Internal packages (starting with internal/stack) read JSON Schema
sources from this embed.FS instead of the repo working tree."
```

---

## Task 9: Create `internal/stack/types.go` — structs and `EvidenceRef.String()`

The struct definitions and the one piece of pure logic that lives on a type method. No validation, no loading — just the data shape.

**Files:**
- Create: `internal/stack/types.go`
- Create: `internal/stack/stack_test.go` (first contents — will grow across later tasks)

- [ ] **Step 1: Write the failing test**

Create `internal/stack/stack_test.go`:

```go
package stack

import (
    "testing"
)

func TestEvidenceRefStringRendersFileColonPath(t *testing.T) {
    ev := EvidenceRef{File: "package.json", Path: "dependencies.express"}
    got := ev.String()
    want := "package.json:dependencies.express"
    if got != want {
        t.Errorf("EvidenceRef.String() = %q, want %q", got, want)
    }
}

func TestEvidenceRefStringIgnoresValue(t *testing.T) {
    ev := EvidenceRef{File: "package.json", Path: "dependencies.express", Value: "^4.18.0"}
    got := ev.String()
    want := "package.json:dependencies.express"
    if got != want {
        t.Errorf("EvidenceRef.String() with value = %q, want %q", got, want)
    }
}

func TestSchemaVersionConstant(t *testing.T) {
    if SchemaVersion != "1.0.0" {
        t.Errorf("SchemaVersion = %q, want %q", SchemaVersion, "1.0.0")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/stack/...
```

Expected: FAIL with `undefined: EvidenceRef`, `undefined: SchemaVersion`, etc.

- [ ] **Step 3: Create `internal/stack/types.go`**

```go
// Package stack owns the StackComponent and StackManifest data types,
// their YAML+JSON-Schema loader, the programmatic validator, and the
// accessors used by sensor generation and runtime grounding.
package stack

import "github.com/iurykrieger/lastro/internal/enums"

// SchemaVersion is the contract version this package implements for both
// stack-component.yaml and stack-manifest.yaml. Loaders reject files that
// declare a different schema_version.
const SchemaVersion = "1.0.0"

// StackComponent is one entry in the detected stack manifest — a library,
// runtime, framework, datastore, protocol, or tool the repo uses.
type StackComponent struct {
    SchemaVersion     string          `json:"schema_version" yaml:"schema_version"`
    ID                string          `json:"id" yaml:"id"`
    Kind              enums.StackKind `json:"kind" yaml:"kind"`
    Name              string          `json:"name" yaml:"name"`
    Version           string          `json:"version" yaml:"version"`
    Capabilities      []string        `json:"capabilities" yaml:"capabilities"`
    DetectionEvidence []EvidenceRef   `json:"detection_evidence" yaml:"detection_evidence"`
}

// EvidenceRef points at the source artifact that proved the component is
// present in the repo. The optional Value carries the literal value at
// that path (e.g., a version range) when available.
type EvidenceRef struct {
    File  string `json:"file" yaml:"file"`
    Path  string `json:"path" yaml:"path"`
    Value string `json:"value,omitempty" yaml:"value,omitempty"`
}

// String renders evidence as the compact "file:path" form for logs and
// reports. The optional Value is intentionally omitted.
func (e EvidenceRef) String() string {
    return e.File + ":" + e.Path
}

// StackManifest is the full detected manifest for a repository: the
// archetype plus the ordered list of detected StackComponents.
type StackManifest struct {
    SchemaVersion string           `json:"schema_version" yaml:"schema_version"`
    Archetype     enums.Archetype  `json:"archetype" yaml:"archetype"`
    Components    []StackComponent `json:"components" yaml:"components"`

    // byID is built by the loader and never marshalled. It backs ByID and
    // is the place duplicate-id detection lands.
    byID map[string]StackComponent `json:"-" yaml:"-"`
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/stack/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/stack/types.go internal/stack/stack_test.go
git commit -m "feat(e2): add internal/stack types and EvidenceRef.String()

StackComponent, EvidenceRef, StackManifest struct definitions with
both yaml and json tags (so the loader can route YAML through
sigs.k8s.io/yaml's YAML->JSON unmarshal pipeline). SchemaVersion
constant pinned to 1.0.0."
```

---

## Task 10: Add `StackComponent.Validate()` with aggregated errors

Validate the per-component invariants the JSON Schema can't fully express in a way that produces an aggregated error message naming every problem at once.

**Files:**
- Create: `internal/stack/validate.go`
- Modify: `internal/stack/stack_test.go` (append tests)

- [ ] **Step 1: Write the failing tests (append to `stack_test.go`)**

```go
import (
    "strings"
    // ... keep existing imports
)

func TestStackComponentValidateRejectsBadID(t *testing.T) {
    cases := []struct {
        name   string
        id     string
        substr string
    }{
        {"empty", "", "id"},
        {"uppercase", "Express", "id"},
        {"underscore", "express_v4", "id"},
        {"leading dash", "-express", "id"},
        {"leading digit", "4express", "id"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            c := validComponent()
            c.ID = tc.id
            err := c.Validate()
            if err == nil {
                t.Fatalf("expected error for id=%q", tc.id)
            }
            if !strings.Contains(err.Error(), tc.substr) {
                t.Errorf("error %q does not mention %q", err.Error(), tc.substr)
            }
        })
    }
}

func TestStackComponentValidateRejectsMissingFields(t *testing.T) {
    base := validComponent()

    type mutate func(*StackComponent)
    cases := []struct {
        name   string
        m      mutate
        substr string
    }{
        {"missing kind", func(c *StackComponent) { c.Kind = "" }, "kind"},
        {"missing name", func(c *StackComponent) { c.Name = "" }, "name"},
        {"missing version", func(c *StackComponent) { c.Version = "" }, "version"},
        {"empty capabilities", func(c *StackComponent) { c.Capabilities = nil }, "capabilities"},
        {"blank capability entry", func(c *StackComponent) { c.Capabilities = []string{""} }, "capabilities"},
        {"empty evidence", func(c *StackComponent) { c.DetectionEvidence = nil }, "detection_evidence"},
        {"evidence missing file", func(c *StackComponent) {
            c.DetectionEvidence = []EvidenceRef{{Path: "x"}}
        }, "file"},
        {"evidence missing path", func(c *StackComponent) {
            c.DetectionEvidence = []EvidenceRef{{File: "x"}}
        }, "path"},
        {"unknown kind", func(c *StackComponent) { c.Kind = "service" }, "kind"},
        {"wrong schema_version", func(c *StackComponent) { c.SchemaVersion = "0.1.0" }, "schema_version"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            c := base
            tc.m(&c)
            err := c.Validate()
            if err == nil {
                t.Fatalf("expected error for case %q", tc.name)
            }
            if !strings.Contains(err.Error(), tc.substr) {
                t.Errorf("error %q does not mention %q", err.Error(), tc.substr)
            }
        })
    }
}

func TestStackComponentValidateAggregates(t *testing.T) {
    c := validComponent()
    c.ID = ""
    c.Kind = ""
    c.Name = ""
    err := c.Validate()
    if err == nil {
        t.Fatal("expected error")
    }
    msg := err.Error()
    for _, want := range []string{"id", "kind", "name"} {
        if !strings.Contains(msg, want) {
            t.Errorf("aggregated error %q missing %q", msg, want)
        }
    }
}

func TestStackComponentValidateAcceptsValid(t *testing.T) {
    if err := validComponent().Validate(); err != nil {
        t.Errorf("validComponent().Validate() = %v, want nil", err)
    }
}

// validComponent returns a known-good StackComponent that individual
// tests mutate one field at a time.
func validComponent() StackComponent {
    return StackComponent{
        SchemaVersion: SchemaVersion,
        ID:            "express",
        Kind:          "framework",
        Name:          "express",
        Version:       "4.18.2",
        Capabilities:  []string{"http-routing"},
        DetectionEvidence: []EvidenceRef{
            {File: "package.json", Path: "dependencies.express", Value: "^4.18.2"},
        },
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/stack/...
```

Expected: FAIL with `undefined: c.Validate` (or similar).

- [ ] **Step 3: Create `internal/stack/validate.go`**

```go
package stack

import (
    "errors"
    "fmt"
    "regexp"
    "strings"

    "github.com/iurykrieger/lastro/internal/enums"
)

// idPattern mirrors $defs.Id in schemas/stack-component.yaml.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Validate checks programmatic invariants that JSON Schema can't fully
// express. Returns an aggregated error naming every problem; returns nil
// when the component is valid.
func (c StackComponent) Validate() error {
    var problems []string

    if c.SchemaVersion != SchemaVersion {
        problems = append(problems,
            fmt.Sprintf("schema_version: got %q, want %q", c.SchemaVersion, SchemaVersion))
    }
    if c.ID == "" {
        problems = append(problems, "id: required")
    } else if !idPattern.MatchString(c.ID) {
        problems = append(problems, fmt.Sprintf("id: %q does not match %s", c.ID, idPattern))
    } else if len(c.ID) > 128 {
        problems = append(problems, fmt.Sprintf("id: %q exceeds 128 chars", c.ID))
    }
    if c.Kind == "" {
        problems = append(problems, "kind: required")
    } else if !enums.IsValidStackKind(string(c.Kind)) {
        problems = append(problems, fmt.Sprintf("kind: %q is not a recognized StackKind", c.Kind))
    }
    if c.Name == "" {
        problems = append(problems, "name: required")
    }
    if c.Version == "" {
        problems = append(problems, "version: required")
    }
    if len(c.Capabilities) == 0 {
        problems = append(problems, "capabilities: at least one required")
    }
    for i, cap := range c.Capabilities {
        if cap == "" {
            problems = append(problems, fmt.Sprintf("capabilities[%d]: empty string", i))
        }
    }
    if len(c.DetectionEvidence) == 0 {
        problems = append(problems, "detection_evidence: at least one required")
    }
    for i, ev := range c.DetectionEvidence {
        if ev.File == "" {
            problems = append(problems, fmt.Sprintf("detection_evidence[%d].file: required", i))
        }
        if ev.Path == "" {
            problems = append(problems, fmt.Sprintf("detection_evidence[%d].path: required", i))
        }
    }

    if len(problems) == 0 {
        return nil
    }
    return errors.New("StackComponent invalid: " + strings.Join(problems, "; "))
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/stack/...
```

Expected: PASS. All component-validation tests green.

- [ ] **Step 5: Commit**

```bash
git add internal/stack/validate.go internal/stack/stack_test.go
git commit -m "feat(e2): add StackComponent.Validate with aggregated errors

Covers id regex, length, required fields, kind enum membership,
capabilities non-emptiness, evidence file/path requirement, and
schema_version match. Errors aggregate so authors fix everything
in one pass."
```

---

## Task 11: Add `StackManifest.Validate()`

Manifest-level invariants: top-level fields, every component validates, errors prefixed with component id (or list index if id is missing). Duplicate-id detection is **not** here — it lives in the loader (Task 12).

**Files:**
- Modify: `internal/stack/validate.go`
- Modify: `internal/stack/stack_test.go` (append tests)

- [ ] **Step 1: Write the failing tests (append to `stack_test.go`)**

```go
func TestStackManifestValidateAcceptsValid(t *testing.T) {
    if err := validManifest().Validate(); err != nil {
        t.Errorf("validManifest().Validate() = %v, want nil", err)
    }
}

func TestStackManifestValidateRejectsTopLevelProblems(t *testing.T) {
    cases := []struct {
        name   string
        m      func(*StackManifest)
        substr string
    }{
        {"wrong schema_version", func(m *StackManifest) { m.SchemaVersion = "9.9.9" }, "schema_version"},
        {"missing archetype", func(m *StackManifest) { m.Archetype = "" }, "archetype"},
        {"unknown archetype", func(m *StackManifest) { m.Archetype = "monolith" }, "archetype"},
        {"empty components", func(m *StackManifest) { m.Components = nil }, "components"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            m := validManifest()
            tc.m(&m)
            err := m.Validate()
            if err == nil {
                t.Fatalf("expected error for %q", tc.name)
            }
            if !strings.Contains(err.Error(), tc.substr) {
                t.Errorf("error %q does not mention %q", err.Error(), tc.substr)
            }
        })
    }
}

func TestStackManifestValidatePrefixesComponentErrorsWithID(t *testing.T) {
    m := validManifest()
    m.Components[0].Name = "" // break the first (named "express")
    err := m.Validate()
    if err == nil {
        t.Fatal("expected error")
    }
    if !strings.Contains(err.Error(), "components[express]") {
        t.Errorf("expected error to mention components[express], got %q", err.Error())
    }
}

func TestStackManifestValidatePrefixesByIndexWhenIDMissing(t *testing.T) {
    m := validManifest()
    m.Components[0].ID = ""
    m.Components[0].Name = ""
    err := m.Validate()
    if err == nil {
        t.Fatal("expected error")
    }
    if !strings.Contains(err.Error(), "components[0]") {
        t.Errorf("expected error to mention components[0], got %q", err.Error())
    }
}

// validManifest returns a known-good StackManifest with two components.
func validManifest() StackManifest {
    return StackManifest{
        SchemaVersion: SchemaVersion,
        Archetype:     "http-api",
        Components: []StackComponent{
            validComponent(),
            {
                SchemaVersion: SchemaVersion,
                ID:            "postgres",
                Kind:          "datastore",
                Name:          "postgres",
                Version:       "16",
                Capabilities:  []string{"transactions"},
                DetectionEvidence: []EvidenceRef{
                    {File: "docker-compose.yaml", Path: "services.db.image"},
                },
            },
        },
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/stack/...
```

Expected: FAIL with `undefined: (StackManifest).Validate` (or similar).

- [ ] **Step 3: Append `Validate` to `internal/stack/validate.go`**

Add the following at the end of `validate.go`:

```go
// Validate checks manifest-level invariants and every component's
// validity. Errors are aggregated; component errors are prefixed with
// the component's id when present, otherwise its list index. Duplicate
// id detection is NOT here — the loader does that after Validate.
func (m StackManifest) Validate() error {
    var problems []string

    if m.SchemaVersion != SchemaVersion {
        problems = append(problems,
            fmt.Sprintf("schema_version: got %q, want %q", m.SchemaVersion, SchemaVersion))
    }
    if m.Archetype == "" {
        problems = append(problems, "archetype: required")
    } else if !enums.IsValidArchetype(string(m.Archetype)) {
        problems = append(problems, fmt.Sprintf("archetype: %q is not a recognized Archetype", m.Archetype))
    }
    if len(m.Components) == 0 {
        problems = append(problems, "components: at least one required")
    }

    for i, c := range m.Components {
        if err := c.Validate(); err != nil {
            prefix := fmt.Sprintf("components[%d]", i)
            if c.ID != "" {
                prefix = fmt.Sprintf("components[%s]", c.ID)
            }
            problems = append(problems, prefix+": "+err.Error())
        }
    }

    if len(problems) == 0 {
        return nil
    }
    return errors.New("StackManifest invalid: " + strings.Join(problems, "; "))
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/stack/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/stack/validate.go internal/stack/stack_test.go
git commit -m "feat(e2): add StackManifest.Validate with prefixed component errors

Manifest-level invariants plus per-component validation. Component
errors are prefixed with the component id when present (so authors
can find them in big manifests), index otherwise. Duplicate-id
detection is intentionally deferred to the loader."
```

---

## Task 12: Add `Load` and `LoadComponent`

The full pipeline: read bytes → JSON Schema validate (using the embedded schemas) → YAML→JSON unmarshal into struct → programmatic Validate → build `byID` (catches duplicates).

**Files:**
- Create: `internal/stack/load.go`
- Modify: `internal/stack/stack_test.go` (append tests)

- [ ] **Step 1: Write the failing tests (append to `stack_test.go`)**

```go
import (
    "os"
    "path/filepath"
    // ... existing imports
)

func TestLoadGoldenManifestRoundTrips(t *testing.T) {
    path := repoPath(t, "schemas/examples/stack-manifest/http-api.yaml")
    first, err := Load(path)
    if err != nil {
        t.Fatalf("Load: %v", err)
    }
    // Re-marshal to YAML via the loader's pipeline by writing to a temp file
    // and reloading. We compare ignoring the unexported byID field, which is
    // rebuilt on each load.
    tmp := filepath.Join(t.TempDir(), "round-trip.yaml")
    writeYAML(t, tmp, first)
    second, err := Load(tmp)
    if err != nil {
        t.Fatalf("Load (round-trip): %v", err)
    }
    if first.SchemaVersion != second.SchemaVersion ||
        first.Archetype != second.Archetype ||
        len(first.Components) != len(second.Components) {
        t.Fatalf("round-trip mismatch: %+v vs %+v", first, second)
    }
    for i := range first.Components {
        if !componentsEqual(first.Components[i], second.Components[i]) {
            t.Errorf("component[%d] mismatch", i)
        }
    }
}

func TestLoadAllGoldenStackComponents(t *testing.T) {
    dir := repoPath(t, "schemas/examples/stack-component")
    entries, err := os.ReadDir(dir)
    if err != nil {
        t.Fatalf("readdir: %v", err)
    }
    if len(entries) == 0 {
        t.Fatal("no example files found")
    }
    for _, e := range entries {
        if filepath.Ext(e.Name()) != ".yaml" {
            continue
        }
        t.Run(e.Name(), func(t *testing.T) {
            _, err := LoadComponent(filepath.Join(dir, e.Name()))
            if err != nil {
                t.Errorf("LoadComponent(%s): %v", e.Name(), err)
            }
        })
    }
}

func TestLoadRejectsDuplicateIDs(t *testing.T) {
    yaml := `schema_version: 1.0.0
archetype: http-api
components:
  - schema_version: 1.0.0
    id: express
    kind: framework
    name: express
    version: "4.18.2"
    capabilities: [http-routing]
    detection_evidence:
      - {file: package.json, path: dependencies.express}
  - schema_version: 1.0.0
    id: express
    kind: library
    name: express-also
    version: "1.0.0"
    capabilities: [http-routing]
    detection_evidence:
      - {file: package.json, path: dependencies.express}
`
    path := writeTempYAML(t, yaml)
    _, err := Load(path)
    if err == nil {
        t.Fatal("expected duplicate-id error")
    }
    msg := err.Error()
    // Error must name both occurrences (indices 0 and 1).
    if !strings.Contains(msg, "express") {
        t.Errorf("error %q does not name the duplicate id", msg)
    }
    if !strings.Contains(msg, "0") || !strings.Contains(msg, "1") {
        t.Errorf("error %q does not name both occurrences", msg)
    }
}

func TestLoadRejectsBadJSONSchemaShape(t *testing.T) {
    // Missing required top-level field "archetype" — caught by JSON Schema
    // before the Go validator runs.
    yaml := `schema_version: 1.0.0
components:
  - schema_version: 1.0.0
    id: express
    kind: framework
    name: express
    version: "4.18.2"
    capabilities: [http-routing]
    detection_evidence:
      - {file: package.json, path: dependencies.express}
`
    path := writeTempYAML(t, yaml)
    _, err := Load(path)
    if err == nil {
        t.Fatal("expected JSON-Schema error")
    }
    if !strings.Contains(err.Error(), "archetype") {
        t.Errorf("error %q should mention archetype", err.Error())
    }
}

func TestLoadComponentRejectsBadJSONSchemaShape(t *testing.T) {
    // detection_evidence using the old string shape — should be rejected
    // by JSON Schema (items must be objects).
    yaml := `schema_version: 1.0.0
id: express
kind: framework
name: express
version: "4.18.2"
capabilities: [http-routing]
detection_evidence:
  - "package.json:dependencies.express"
`
    path := writeTempYAML(t, yaml)
    _, err := LoadComponent(path)
    if err == nil {
        t.Fatal("expected JSON-Schema error for string-form evidence")
    }
}

// --- test helpers ---

func repoPath(t *testing.T, rel string) string {
    t.Helper()
    // Tests run with CWD = the package directory; ../../ reaches the repo root.
    return filepath.Join("..", "..", rel)
}

func writeTempYAML(t *testing.T, content string) string {
    t.Helper()
    p := filepath.Join(t.TempDir(), "fixture.yaml")
    if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
        t.Fatalf("write temp: %v", err)
    }
    return p
}

func writeYAML(t *testing.T, path string, m StackManifest) {
    t.Helper()
    b, err := yamlMarshal(m)
    if err != nil {
        t.Fatalf("marshal: %v", err)
    }
    if err := os.WriteFile(path, b, 0o600); err != nil {
        t.Fatalf("write: %v", err)
    }
}

func componentsEqual(a, b StackComponent) bool {
    if a.SchemaVersion != b.SchemaVersion || a.ID != b.ID || a.Kind != b.Kind ||
        a.Name != b.Name || a.Version != b.Version {
        return false
    }
    if len(a.Capabilities) != len(b.Capabilities) {
        return false
    }
    for i := range a.Capabilities {
        if a.Capabilities[i] != b.Capabilities[i] {
            return false
        }
    }
    if len(a.DetectionEvidence) != len(b.DetectionEvidence) {
        return false
    }
    for i := range a.DetectionEvidence {
        if a.DetectionEvidence[i] != b.DetectionEvidence[i] {
            return false
        }
    }
    return true
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/stack/...
```

Expected: FAIL with `undefined: Load`, `undefined: LoadComponent`, `undefined: yamlMarshal`.

- [ ] **Step 3: Create `internal/stack/load.go`**

```go
package stack

import (
    "encoding/json"
    "fmt"
    "os"

    "github.com/iurykrieger/lastro/schemas"
    "github.com/santhosh-tekuri/jsonschema/v6"
    "sigs.k8s.io/yaml"
)

const (
    componentSchemaURL = "https://lastro.dev/harness/schemas/stack-component.yaml"
    manifestSchemaURL  = "https://lastro.dev/harness/schemas/stack-manifest.yaml"
)

// compileSchemas builds a jsonschema compiler with both stack-component.yaml
// and stack-manifest.yaml registered, so the manifest's $ref to the component
// schema resolves.
func compileSchemas() (manifest *jsonschema.Schema, component *jsonschema.Schema, err error) {
    c := jsonschema.NewCompiler()

    for url, file := range map[string]string{
        componentSchemaURL: "stack-component.yaml",
        manifestSchemaURL:  "stack-manifest.yaml",
    } {
        b, readErr := schemas.FS.ReadFile(file)
        if readErr != nil {
            return nil, nil, fmt.Errorf("read embedded %s: %w", file, readErr)
        }
        j, jErr := yaml.YAMLToJSON(b)
        if jErr != nil {
            return nil, nil, fmt.Errorf("yaml->json %s: %w", file, jErr)
        }
        var doc any
        if uErr := json.Unmarshal(j, &doc); uErr != nil {
            return nil, nil, fmt.Errorf("unmarshal %s: %w", file, uErr)
        }
        if rErr := c.AddResource(url, doc); rErr != nil {
            return nil, nil, fmt.Errorf("register %s: %w", file, rErr)
        }
    }

    manifest, err = c.Compile(manifestSchemaURL)
    if err != nil {
        return nil, nil, fmt.Errorf("compile manifest schema: %w", err)
    }
    component, err = c.Compile(componentSchemaURL)
    if err != nil {
        return nil, nil, fmt.Errorf("compile component schema: %w", err)
    }
    return manifest, component, nil
}

// Load reads, JSON-Schema-validates, unmarshals, programmatically validates,
// and indexes a stack manifest YAML file. It is the canonical entrypoint.
func Load(path string) (StackManifest, error) {
    b, err := os.ReadFile(path)
    if err != nil {
        return StackManifest{}, fmt.Errorf("read %s: %w", path, err)
    }
    manifestSch, _, err := compileSchemas()
    if err != nil {
        return StackManifest{}, err
    }
    if err := validateAgainstSchema(b, manifestSch); err != nil {
        return StackManifest{}, fmt.Errorf("%s: %w", path, err)
    }
    var m StackManifest
    if err := yaml.Unmarshal(b, &m); err != nil {
        return StackManifest{}, fmt.Errorf("unmarshal %s: %w", path, err)
    }
    if err := m.Validate(); err != nil {
        return StackManifest{}, fmt.Errorf("%s: %w", path, err)
    }
    if err := m.buildIndex(); err != nil {
        return StackManifest{}, fmt.Errorf("%s: %w", path, err)
    }
    return m, nil
}

// LoadComponent loads a single StackComponent YAML — useful for fixtures and
// tests. Follows the same pipeline as Load, against the component schema.
func LoadComponent(path string) (StackComponent, error) {
    b, err := os.ReadFile(path)
    if err != nil {
        return StackComponent{}, fmt.Errorf("read %s: %w", path, err)
    }
    _, componentSch, err := compileSchemas()
    if err != nil {
        return StackComponent{}, err
    }
    if err := validateAgainstSchema(b, componentSch); err != nil {
        return StackComponent{}, fmt.Errorf("%s: %w", path, err)
    }
    var c StackComponent
    if err := yaml.Unmarshal(b, &c); err != nil {
        return StackComponent{}, fmt.Errorf("unmarshal %s: %w", path, err)
    }
    if err := c.Validate(); err != nil {
        return StackComponent{}, fmt.Errorf("%s: %w", path, err)
    }
    return c, nil
}

func validateAgainstSchema(b []byte, sch *jsonschema.Schema) error {
    j, err := yaml.YAMLToJSON(b)
    if err != nil {
        return fmt.Errorf("yaml->json: %w", err)
    }
    var doc any
    if err := json.Unmarshal(j, &doc); err != nil {
        return fmt.Errorf("unmarshal: %w", err)
    }
    if err := sch.Validate(doc); err != nil {
        return fmt.Errorf("schema validation: %w", err)
    }
    return nil
}

// buildIndex populates m.byID and rejects duplicate component ids by naming
// both occurrences.
func (m *StackManifest) buildIndex() error {
    m.byID = make(map[string]StackComponent, len(m.Components))
    first := make(map[string]int, len(m.Components))
    for i, c := range m.Components {
        if prev, dup := first[c.ID]; dup {
            return fmt.Errorf(
                "duplicate component id %q at components[%d] and components[%d]",
                c.ID, prev, i,
            )
        }
        first[c.ID] = i
        m.byID[c.ID] = c
    }
    return nil
}

// yamlMarshal is exposed for tests that need to write a manifest back out.
func yamlMarshal(v any) ([]byte, error) {
    return yaml.Marshal(v)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/stack/...
```

Expected: PASS. If `TestLoadRejectsDuplicateIDs` fails because the duplicate-id error fires before JSON-Schema validation completes (it should not — JSON Schema permits duplicate object array entries by default), check that the test YAML actually passes JSON Schema (it should: two valid components with the same id).

- [ ] **Step 5: Commit**

```bash
git add internal/stack/load.go internal/stack/stack_test.go
git commit -m "feat(e2): add Load and LoadComponent with embedded JSON Schema validation

Pipeline: file bytes -> JSON-Schema validate (against embedded
schemas) -> YAML unmarshal -> programmatic Validate -> byID build
(which detects duplicate component ids, naming both occurrences).
Tests cover golden round-trip, all six existing stack-component
examples, duplicate-id rejection, and JSON Schema rejection."
```

---

## Task 13: Add accessors (`ByID`, `HasCapability`, `ComponentsWithCapability`)

The minimal surface sensor generation will use to read the manifest.

**Files:**
- Create: `internal/stack/accessors.go`
- Modify: `internal/stack/stack_test.go` (append tests)

- [ ] **Step 1: Write the failing tests (append to `stack_test.go`)**

```go
func TestByIDReturnsPresentComponent(t *testing.T) {
    m := loadGolden(t)
    c, ok := m.ByID("nestjs")
    if !ok {
        t.Fatal("ByID(nestjs) = _, false; want true")
    }
    if c.ID != "nestjs" {
        t.Errorf("ByID returned wrong component: %+v", c)
    }
}

func TestByIDReturnsZeroForAbsent(t *testing.T) {
    m := loadGolden(t)
    c, ok := m.ByID("never-installed")
    if ok {
        t.Errorf("ByID(never-installed) = %+v, true; want zero, false", c)
    }
    if (c != StackComponent{}) {
        t.Errorf("ByID returned non-zero for absent: %+v", c)
    }
}

func TestHasCapability(t *testing.T) {
    m := loadGolden(t)
    if !m.HasCapability("http-routing") {
        t.Error("HasCapability(http-routing) = false; want true")
    }
    if m.HasCapability("graphql-subscriptions") {
        t.Error("HasCapability(graphql-subscriptions) = true; want false")
    }
}

func TestComponentsWithCapabilityPreservesOrder(t *testing.T) {
    m := loadGolden(t)
    got := m.ComponentsWithCapability("http-routing")
    if len(got) != 1 {
        t.Fatalf("len = %d, want 1", len(got))
    }
    if got[0].ID != "nestjs" {
        t.Errorf("got[0].ID = %q, want %q", got[0].ID, "nestjs")
    }
}

func TestComponentsWithCapabilityReturnsEmptyNotNil(t *testing.T) {
    m := loadGolden(t)
    got := m.ComponentsWithCapability("never-declared")
    if got == nil {
        t.Error("got nil; want empty (non-nil) slice")
    }
    if len(got) != 0 {
        t.Errorf("len = %d, want 0", len(got))
    }
}

func loadGolden(t *testing.T) StackManifest {
    t.Helper()
    m, err := Load(repoPath(t, "schemas/examples/stack-manifest/http-api.yaml"))
    if err != nil {
        t.Fatalf("Load golden: %v", err)
    }
    return m
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/stack/...
```

Expected: FAIL with `undefined: ByID`, etc.

- [ ] **Step 3: Create `internal/stack/accessors.go`**

```go
package stack

// ByID returns the component matching the given id, plus a boolean
// indicating presence. The zero StackComponent is returned for unknown ids.
func (m StackManifest) ByID(id string) (StackComponent, bool) {
    c, ok := m.byID[id]
    return c, ok
}

// HasCapability reports whether any component in the manifest declares cap.
func (m StackManifest) HasCapability(cap string) bool {
    for _, c := range m.Components {
        for _, have := range c.Capabilities {
            if have == cap {
                return true
            }
        }
    }
    return false
}

// ComponentsWithCapability returns every component declaring cap, in
// manifest order. The returned slice is always non-nil (empty, not nil,
// when no matches) to avoid range-over-nil foot-guns at call sites.
func (m StackManifest) ComponentsWithCapability(cap string) []StackComponent {
    out := []StackComponent{}
    for _, c := range m.Components {
        for _, have := range c.Capabilities {
            if have == cap {
                out = append(out, c)
                break
            }
        }
    }
    return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/stack/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/stack/accessors.go internal/stack/stack_test.go
git commit -m "feat(e2): add ByID, HasCapability, ComponentsWithCapability

ByID is O(1) via the loader-built byID index. The capability
accessors iterate Components in manifest order.
ComponentsWithCapability returns an empty (non-nil) slice when
nothing matches to keep range-over-nil out of call sites."
```

---

## Task 14: Add `LintCapabilities` (non-blocking)

The drift helper that surfaces capability strings the framework doesn't recognize. Returns warnings only — never errors — so callers decide what to do.

**Files:**
- Modify: `internal/stack/accessors.go`
- Modify: `internal/stack/stack_test.go` (append tests)

- [ ] **Step 1: Write the failing test (append to `stack_test.go`)**

```go
func TestLintCapabilitiesWarnsOnUnknown(t *testing.T) {
    m := loadGolden(t)
    known := []string{"http-routing", "middleware", "dependency-injection", "transactions"}
    got := m.LintCapabilities(known)

    // Golden manifest declares: nestjs={dependency-injection, http-routing, middleware},
    // postgres={transactions, row-level-security}.
    // 'row-level-security' is the one unknown capability.
    if len(got) != 1 {
        t.Fatalf("len = %d, want 1; warnings = %+v", len(got), got)
    }
    w := got[0]
    if w.ComponentID != "postgres" {
        t.Errorf("ComponentID = %q, want %q", w.ComponentID, "postgres")
    }
    if w.Capability != "row-level-security" {
        t.Errorf("Capability = %q, want %q", w.Capability, "row-level-security")
    }
    if w.Message == "" {
        t.Error("Message: empty; want a human-readable string")
    }
}

func TestLintCapabilitiesReturnsEmptyWhenAllKnown(t *testing.T) {
    m := loadGolden(t)
    known := []string{
        "http-routing", "middleware", "dependency-injection",
        "transactions", "row-level-security",
    }
    got := m.LintCapabilities(known)
    if len(got) != 0 {
        t.Errorf("len = %d, want 0; warnings = %+v", len(got), got)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/stack/...
```

Expected: FAIL with `undefined: LintCapabilities`, `undefined: LintWarning`.

- [ ] **Step 3: Append to `internal/stack/accessors.go`**

```go
// LintWarning is emitted by LintCapabilities for each capability string not
// found in the supplied recognized-vocabulary list.
type LintWarning struct {
    ComponentID string
    Capability  string
    Message     string
}

// LintCapabilities returns one LintWarning per (component, capability) pair
// where the capability is not in known. Warnings only — never errors.
// Order: manifest order of components, then declared order of capabilities
// within each component.
func (m StackManifest) LintCapabilities(known []string) []LintWarning {
    set := make(map[string]struct{}, len(known))
    for _, k := range known {
        set[k] = struct{}{}
    }
    var out []LintWarning
    for _, c := range m.Components {
        for _, cap := range c.Capabilities {
            if _, ok := set[cap]; ok {
                continue
            }
            out = append(out, LintWarning{
                ComponentID: c.ID,
                Capability:  cap,
                Message:     "unrecognized capability " + cap + " on component " + c.ID,
            })
        }
    }
    return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/stack/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/stack/accessors.go internal/stack/stack_test.go
git commit -m "feat(e2): add LintCapabilities non-blocking drift helper

Per (component, capability) pair, emit a LintWarning if the
capability is not in the caller-supplied recognized list. Order
follows manifest -> declared order. Never errors — caller decides
what to do with the warnings."
```

---

## Task 15: Final sweep — full test run and schema validator

A belt-and-suspenders pass: run all package tests across the repo, run the schema validator, confirm nothing else broke (especially `internal/enums`).

**Files:** none modified.

- [ ] **Step 1: Run every Go test in the repo**

```bash
go test ./...
```

Expected: PASS across `./internal/enums/...`, `./internal/stack/...`, `./schemas/...`. No FAIL lines.

- [ ] **Step 2: Run the schema validator**

```bash
go run ./cmd/validate-schemas
```

Expected: final line `All schemas, enums, and examples validated.` with no FAIL lines anywhere in the output.

- [ ] **Step 3: Verify the deliverable acceptance criteria from the spec**

Walk through each bullet in the spec's "Deliverable acceptance" section and tick off mentally:

- [x] `schemas/stack-manifest.yaml` exists (Task 3).
- [x] `schemas/stack-component.yaml` refined (Task 1).
- [x] Examples for both exist and validate (Tasks 2, 4).
- [x] `internal/stack/` builds and tests pass (Tasks 9–14, this task).
- [x] Golden examples load, validate, round-trip (Task 12).
- [x] Every negative case produces an error; aggregating cases produce more than one (Tasks 10, 11).
- [x] `ByID`, `HasCapability`, `ComponentsWithCapability` work and tested (Task 13).
- [x] `LintCapabilities` warns (does not error) on unknown (Task 14).
- [x] E1 extended with `StackKind` (Tasks 5, 6, 7).

If any criterion can't be verified, file it as a follow-up rather than improvising.

- [ ] **Step 4: No commit needed**

This task only runs verifications; no source changes.

---

## Self-Review

**Spec coverage check:**
- Refining `stack-component.yaml` → Task 1.
- Adding `stack-manifest.yaml` → Task 3.
- Golden examples → Tasks 2, 4.
- `StackKind` enum file + Go constants + tests → Tasks 5, 6, 7.
- `internal/stack/` package (types, loader, validator, accessors, lint) → Tasks 9–14.
- Tests covering load, validate (positive + negative), round-trip, accessors → Tasks 10–14.
- The "Out: refactor of E1" note — honored; E2 only extends E1 with one new enum (Tasks 5–7), no restructure.

**Placeholder scan:** no TBDs, no "implement later", no "add validation" without code. The one place a literal "<library name>" appears (Task 2, Step 3) is intentionally a fill-in-from-existing-file instruction with the rewrite pattern shown — the engineer reads the current file's `name` field and substitutes.

**Type consistency:** `StackKind` constants (`StackKindLibrary`, `StackKindRuntime`, …) are introduced in Task 6 and never referenced elsewhere by name (the package uses string literals via `enums.IsValidStackKind`). `EvidenceRef`, `StackComponent`, `StackManifest`, `LintWarning` field names match between Task 9 (struct definition), Task 10 (validate), Task 11 (validate), Task 12 (load), Task 13 (accessors), Task 14 (lint). `SchemaVersion` constant defined Task 9, asserted Task 9, used in Task 10. `byID` private field defined Task 9, populated in Task 12 (`buildIndex`), consumed in Task 13 (`ByID`).

**Scope check:** single implementation pass covering one entity and its E1 dependency. No further decomposition needed.

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-05-22-e2-stack-component.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

**Which approach?**
