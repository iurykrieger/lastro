# E2 — StackComponent Design

> Source chunk: [`docs/harness-framework/E2-stack-component.md`](../../harness-framework/E2-stack-component.md)
> Parent plan: [`docs/harness-framework/plan.md`](../../harness-framework/plan.md) §4.3, §2

## Purpose

A `StackComponent` is one entry in the detected stack manifest — a library, runtime, framework, datastore, protocol, or tool the repo uses. Sensors are *grounded* in the toolbox: a sensor cannot declare `uses: [<id>]` for a component not present in the manifest. This chunk owns the YAML schemas, the Go types, the loader/validator, and the accessors used by sensor generation and runtime.

## Scope

In:
- Refining `schemas/stack-component.yaml` (authored at the schema-freeze gate as JSON Schema 2020-12 in YAML).
- Adding `schemas/stack-manifest.yaml` (new — the gate covers only the 8 entity schemas; the manifest container is not among them).
- One worked golden example each under `schemas/examples/stack-component/` and `schemas/examples/stack-manifest/`.
- Extending E1 with a `StackKind` enum: `schemas/enums/stack-kinds.yaml` plus Go constants. The existing drift contract (inline `enum:` in `stack-component.yaml` ↔ `enums/stack-kinds.yaml`) applies.
- Go package `internal/stack/` — types, loader, validator, accessors.
- A non-blocking capability lint helper.
- Tests covering load, validate (positive + negative), round-trip, and accessors.

Out:
- The `/detect-stack` skill (Phase B). This chunk owns the data shape, not how the manifest is produced.
- The recognized-capabilities vocabulary itself — E2 ships the lint function, not the list.
- Refactor of E1 itself; this chunk hands E1 a new enum file and Go constant to fold in.

## Open questions — resolved

1. **`kind` ownership.** Closed enum, added to E1 as a new `StackKind` enum (joining the 8 enums already in E1's scope, making 9 total). E2 imports it. Consistent with the framework's fixed-enum philosophy; lets sensors safely match by kind.
2. **`version` semantics.** Free-form string, no validation. Forgiving across ecosystems (Go pseudo-versions, Python wheels, calendar versions, git SHAs). Leaves room to layer a parser later without a schema change.
3. **`capabilities` vocabulary.** Free-form strings plus a non-blocking lint helper. Lets `/detect-stack` evolve without framework releases; warnings surface drift early. The recognized-capabilities list is curated outside E2.
4. **`detection_evidence` shape.** Structured object `{file, path, value?}` with a `String()` renderer producing the compact `file:path` form for human display. Lossless, machine-reasonable, and still readable.
5. **Manifest container.** YAML list for authoring (ordered, diff-friendly). Go loader builds an unexported `map[string]StackComponent` for O(1) lookup and enforces id uniqueness at load time.

## Schemas

### `schemas/stack-component.yaml`

Single-component schema:

```yaml
schema_version: 1.0.0
id: express                           # required; unique within manifest; ^[a-z][a-z0-9-]*$
kind: framework                       # required; StackKind enum (from E1)
name: express                         # required; human-readable
version: "4.18.2"                     # required; free-form string
capabilities:                         # required; non-empty list of free-form strings
  - http-routing
  - middleware
  - json-body-parsing
detection_evidence:                   # required; non-empty list
  - file: package.json                # required
    path: dependencies.express        # required
    value: "^4.18.2"                  # optional
```

### `schemas/stack-manifest.yaml`

Container schema bundling archetype + components:

```yaml
schema_version: 1.0.0
archetype: http-api                   # required; Archetype enum (from E1)
components:                           # required; non-empty list of StackComponent
  - { id: express, kind: framework, name: express, version: "4.18.2",
      capabilities: [http-routing], detection_evidence: [{file: package.json, path: dependencies.express}] }
  - { id: postgres, kind: datastore, name: postgres, version: "15",
      capabilities: [relational-storage], detection_evidence: [{file: docker-compose.yml, path: services.db.image}] }
```

### Examples

Following the schema-freeze convention `schemas/examples/<entity>/<scenario>.yaml`:
- `schemas/examples/stack-component/express.yaml` — happy path, framework kind.
- `schemas/examples/stack-component/postgres.yaml` — datastore kind, multi-evidence.
- `schemas/examples/stack-manifest/http-api.yaml` — multi-component manifest with `archetype: http-api`.

Each must round-trip through the loader (after canonical marshal) and validate against its JSON Schema.

## Go types

Package `internal/stack/`:

```go
package stack

import "github.com/iurykrieger/lastro/internal/enums"

const SchemaVersion = "1.0.0"

// StackComponent is one entry in the detected stack manifest.
type StackComponent struct {
    SchemaVersion     string          `yaml:"schema_version"`
    ID                string          `yaml:"id"`
    Kind              enums.StackKind `yaml:"kind"`
    Name              string          `yaml:"name"`
    Version           string          `yaml:"version"`
    Capabilities      []string        `yaml:"capabilities"`
    DetectionEvidence []EvidenceRef   `yaml:"detection_evidence"`
}

// EvidenceRef points at the source artifact that proved the component is present.
type EvidenceRef struct {
    File  string `yaml:"file"`
    Path  string `yaml:"path"`
    Value string `yaml:"value,omitempty"`
}

func (e EvidenceRef) String() string  // renders "file:path"

// StackManifest is the full detected manifest for a repository.
type StackManifest struct {
    SchemaVersion string           `yaml:"schema_version"`
    Archetype     enums.Archetype  `yaml:"archetype"`
    Components    []StackComponent `yaml:"components"`

    byID map[string]StackComponent `yaml:"-"` // built at load time, unexported
}
```

`byID` is unexported and only populated by the loader, so it cannot drift from `Components`.

## Loader

```go
func Load(path string) (StackManifest, error)
func LoadComponent(path string) (StackComponent, error)
```

`Load` steps (consistent with the schema-freeze reference validator and the E1 loader pattern):
1. Read file bytes.
2. JSON-Schema-validate against `schemas/stack-manifest.yaml` using `github.com/santhosh-tekuri/jsonschema/v6`. This catches structural problems (missing required fields, wrong types, bad `kind` value against the inline enum) early and uniformly with the rest of the framework.
3. Unmarshal into `StackManifest` using `sigs.k8s.io/yaml`.
4. Run programmatic `Validate()` for the things JSON Schema can't express (id regex, id uniqueness *across* components, cross-field consistency). Aggregates all problems, not just the first.
5. Build `byID`, rejecting duplicates with an error naming both occurrences.
6. Return.

`LoadComponent` exists for testing individual fixtures and follows the same pipeline against `schemas/stack-component.yaml`.

## Validator

```go
func (c StackComponent) Validate() error
func (m StackManifest) Validate() error
```

Both return aggregated errors so authors fix everything in one pass.

**Component-level checks:**
- `schema_version` equals `SchemaVersion` constant.
- `id` non-empty and matches `^[a-z][a-z0-9-]*$` (aligned with the existing `$defs.Id` pattern in the schema-freeze gate — first char must be a letter).
- `kind` recognized by `enums.IsValidStackKind`.
- `name`, `version` non-empty.
- `capabilities` non-empty; each entry non-empty.
- `detection_evidence` non-empty; each entry has non-empty `file` and `path`.

**Manifest-level checks:**
- `schema_version` equals `SchemaVersion`.
- `archetype` recognized by `enums.IsValidArchetype`.
- `components` non-empty.
- Every component validates (errors prefixed with component id or list index).
- No duplicate ids (enforced in the loader after struct validation).

## Capability lint (non-blocking)

```go
type LintWarning struct {
    ComponentID string
    Capability  string
    Message     string
}

func (m StackManifest) LintCapabilities(known []string) []LintWarning
```

Returns warnings (never errors) for capability strings not in the supplied `known` list. E2 ships the function; the curated `known` list lives elsewhere (consumed by sensor generation).

## Accessors

```go
func (m StackManifest) ByID(id string) (StackComponent, bool)
func (m StackManifest) HasCapability(cap string) bool
func (m StackManifest) ComponentsWithCapability(cap string) []StackComponent
```

`ComponentsWithCapability` preserves manifest order and returns an empty slice (not `nil`) when no matches — small ergonomic detail that prevents `range` foot-guns.

## Dependencies

- **E1 (enums):** consumes `Archetype`. Hands E1 a new `StackKind` enum (9th enum file: `schemas/enums/stack-kinds.yaml`) for E1 to fold into its existing pattern (codegen / drift test). E2 imports the generated `enums.StackKind` constants.
- **Schema-freeze gate:** `schemas/stack-component.yaml` is first authored at the gate; E2 validates and refines it. `schemas/stack-manifest.yaml` is **not** at the gate (the gate covers entity schemas only) — E2 introduces it.
- **Existing dependencies in `go.mod`:** `sigs.k8s.io/yaml` (unmarshal) and `github.com/santhosh-tekuri/jsonschema/v6` (validation) — both already pulled in by earlier chunks; no new module deps.

## Coordination notes

- The inline `enum:` for `kind` in `schemas/stack-component.yaml` (per the schema-freeze) must match `schemas/enums/stack-kinds.yaml` values exactly. The framework's existing inline-enum drift contract handles this; no new mechanism needed.
- E2 should not land before E1 picks up `StackKind`. If parallel work is desired, E2 can stub `enums.StackKind` locally and replace the import once E1 lands; preferable to coordinate so the stub is unnecessary.

## Tests

All tests follow AAA. Located in `internal/stack/stack_test.go` with fixtures under `internal/stack/testdata/{valid,invalid}/`.

**Golden round-trip:**
- Load each `schemas/examples/stack-manifest/*.yaml` → marshal → load again → deep-equal (ignoring `byID`).
- Same for each `schemas/examples/stack-component/*.yaml`.

**Positive load tests:**
- Minimal valid manifest (one component, one capability, one evidence entry).
- Multi-component manifest mixing kinds (framework + datastore + tool).
- Components with and without optional evidence `value`.

**Negative load tests (table-driven):**
- Missing required fields: `id`, `kind`, `name`, `version`.
- Invalid `kind` (`"databse"`).
- Invalid `archetype` (`"htpp-api"`).
- Empty `capabilities`.
- Empty `detection_evidence`.
- Evidence entry missing `file` or `path`.
- Duplicate component `id` (error names both occurrences).
- Bad `id` format: `Express`, `express_v4`, leading dash.
- Wrong `schema_version`.
- Each case asserts `Validate()` aggregates *all* problems, not just the first.

**Accessor tests:**
- `ByID` returns `(component, true)` for present; `(zero, false)` for absent.
- `HasCapability` true/false cases.
- `ComponentsWithCapability` returns matches in manifest order; returns empty slice (not `nil`) when absent.

**Lint test:**
- `LintCapabilities(["http-routing", "middleware"])` against a manifest with `["http-routing", "graphql-subscriptions"]` returns one warning naming `graphql-subscriptions`.

**`EvidenceRef.String()` test:**
- `{file: "package.json", path: "dependencies.express"}` renders as `"package.json:dependencies.express"`.

## Deliverable acceptance

- `schemas/stack-manifest.yaml` exists (new) and `schemas/stack-component.yaml` (from gate) is refined to match this design.
- `schemas/examples/stack-component/*.yaml` and `schemas/examples/stack-manifest/*.yaml` exist and validate.
- `internal/stack/` Go package builds and tests pass.
- The golden manifest examples load, validate, and round-trip.
- Every negative case above produces an error; aggregating cases produce more than one error.
- `ByID`, `HasCapability`, `ComponentsWithCapability` work and are tested.
- `LintCapabilities` warns (does not error) on unknown capabilities.
- E1 has been extended with a `StackKind` enum (`schemas/enums/stack-kinds.yaml` + Go constants) that E2 imports.
