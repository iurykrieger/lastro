# E3 — EntryPoint: Design Spec (revised)

> Brainstormed from [`docs/harness-framework/E3-entry-point.md`](../../harness-framework/E3-entry-point.md). Source plan: [`docs/harness-framework/plan.md`](../../harness-framework/plan.md) §4.1.1, §2.1. Supersedes the abandoned draft at `2026-05-22-e3-entry-point-design.md` (which assumed pre-merge conventions).

## Purpose

Implement `internal/entrypoint/` — the typed Go representation of the archetype-typed observable surfaces (`http-api`, `event-consumer`, `cli`, …) that `UseCase` embeds. The package owns the YAML loader, JSON Schema validation, golden examples, and the small accessor surface (`SpecField`, `Label`) that E4's template resolver consumes.

## Scope

**In:**
- `internal/entrypoint/types.go` — the `EntryPoint` struct with `ID`, `Archetype`, `Spec`.
- `internal/entrypoint/schema.go` — embedded `schema.yaml` mirror + cached compiled JSON Schema.
- `internal/entrypoint/schema.yaml` — byte-equal copy of `schemas/entry-point.yaml`.
- `internal/entrypoint/loader.go` — `LoadEntryPoint` (single entry from YAML bytes) and `LoadFromExample(path)` (test convenience reading the golden examples).
- `internal/entrypoint/accessors.go` — `SpecField(name) (any, bool)` and `Label() string` methods on `EntryPoint`.
- `internal/entrypoint/drift_test.go` — assert the embedded schema is byte-equal to `schemas/entry-point.yaml`.
- `internal/entrypoint/*_test.go` — per-archetype happy/sad loader tests; accessor tests; schema-compile sanity test.
- `internal/entrypoint/testdata/` — minimal in-package fixtures for negative cases not covered by `schemas/examples/entry-point/`.

**Out:**
- `UseCase` ownership (E4). `EntryPoint` is embedded by `UseCase` but lives in its own package, mirroring the E5 `Fixture`/`FixtureStore` seam.
- `{{entry_points.<id>}}` interpolation (E4).
- Cross-archetype invariants — e.g., `archetype_scope` agreement with the parent UseCase, EntryPoint id uniqueness within a UseCase (E4).
- Sensor grounding against EntryPoint surfaces (E6).

## Alignment with merged main

Main has landed E1 (enums), E2 (stack-component), and E5 (fixture). This spec follows the conventions those packages established:

1. **Package layout:** `types.go` + `loader.go` + `schema.go` + embedded `schema.yaml` + `drift_test.go`. Optional `accessors.go` per E2's precedent.
2. **YAML pipeline:** `sigs.k8s.io/yaml.YAMLToJSON` → JSON Schema validation via `github.com/santhosh-tekuri/jsonschema/v6` → `json.Unmarshal` into the typed struct.
3. **Struct tags:** both `json:` and `yaml:` on every field; JSON tags drive deserialization because the pipeline runs through JSON.
4. **Enum naming:** `enums.ArchetypeHTTPAPI`, `enums.AllArchetypes()`, `enums.IsValidArchetype(s)` — match the E1 convention exactly.
5. **Embedded schema + drift test:** `//go:embed schema.yaml` keeps the package self-contained at build time; `drift_test.go` asserts byte equality with the canonical `schemas/<name>.yaml`.

## Decisions vs. the abandoned 2026-05-22 spec

The earlier spec assumed an unmerged repo and proposed several things that the on-disk reality has settled differently. The deltas:

| Decision | 2026-05-22 spec | This spec (aligned with main) |
|---|---|---|
| Spec encoding | Interface + per-archetype concrete struct + registry | `map[string]any` + accessor methods. The JSON Schema's `oneOf` enforces archetype-specific required fields; Go doesn't re-do the discriminated union. |
| Polymorphic loader | Custom `UnmarshalYAML` dispatches per archetype | Standard pipeline (YAMLToJSON → JSON Schema → json.Unmarshal). No custom unmarshaler. |
| Method validation | "Free-form string" | Closed enum per the frozen schema (`GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS`). Enforced by JSON Schema. |
| ChannelKind / TriggerKind / HTTPMethod | Promote to `schemas/enums/*.yaml` | Stay inline in `schemas/entry-point.yaml`. Main did not promote them; E3 does not either. |
| `_meta.yaml` relaxation | Proposed widening the id pattern to allow uppercase | Not needed once the enums stay inline. No schema-freeze edits. |
| Per-archetype `Validate()` methods | One method per concrete spec type | Single `EntryPoint.Validate()` that re-runs JSON Schema validation. (Often unnecessary because the loader already validated; exposed for callers that constructed an `EntryPoint` in code rather than via the loader.) |
| New API for E4 | Not anticipated | `SpecField(name) (any, bool)` and `Label() string` — required by the E4 plan's `template/resolver.go`. |

The big change is that the JSON Schema is the source of truth for archetype-spec shape, not Go interfaces. Go holds typed `ID` and `Archetype`, plus an untyped `Spec` map. When typed spec access is needed (E6 sensor generation), we'll add typed projection helpers (`HTTPAPIMethod(ep) (string, bool)`, etc.) at that point — YAGNI for now.

## API

```go
package entrypoint

import "github.com/iurykrieger/lastro/internal/enums"

// EntryPoint is an archetype-typed observable surface — an HTTP route,
// queue/topic subscription, CLI command, exported SDK symbol, etc. It is
// always embedded inside a UseCase; this package owns only the shape and
// validation. UseCase ownership lives in package usecase (E4).
type EntryPoint struct {
    ID        string          `json:"id"        yaml:"id"`
    Archetype enums.Archetype `json:"archetype" yaml:"archetype"`
    Spec      map[string]any  `json:"spec"      yaml:"spec"`
}

// LoadEntryPoint parses a single EntryPoint from YAML bytes and validates
// it against the canonical JSON Schema. Returns a wrapped error naming
// the failing phase and (when available) the entry-point id.
func LoadEntryPoint(raw []byte) (EntryPoint, error)

// LoadFromExample loads one of the per-archetype golden examples under
// schemas/examples/entry-point/. Test-only convenience; not for runtime use.
func LoadFromExample(path string) (EntryPoint, error)

// Validate runs JSON Schema validation against the receiver. Useful for
// EntryPoints constructed in code (e.g., by tests) that did not go
// through LoadEntryPoint. The loader already calls this internally.
func (e EntryPoint) Validate() error

// SpecField looks up a single key in the spec map. Returns the raw value
// and true if present; nil and false otherwise.
//
// E4's template resolver uses this to resolve {{entry_points.<id>.spec.<key>}}
// references. The returned `any` is whatever JSON yielded from yaml-to-json
// (string, float64, bool, []any, map[string]any).
func (e EntryPoint) SpecField(name string) (any, bool)

// Label renders the EntryPoint as the compact "<archetype>:<id>" form used
// in human-facing log lines and template fallback rendering. E4's template
// resolver renders bare {{entry_points.<id>}} (without a spec field) using
// Label.
//
// Example: EntryPoint{ID: "create_order", Archetype: ArchetypeHTTPAPI}
// renders as "http-api:create_order".
func (e EntryPoint) Label() string
```

The pipeline mirrors `internal/fixture/loader.go`:

1. `yaml.YAMLToJSON(raw)` — normalize.
2. `compiledSchema().Validate(instance)` — JSON Schema validates the discriminated union (`oneOf` per archetype enforces required spec fields and the inline `method`/`channel_kind`/`trigger_kind` enums).
3. `json.Unmarshal(asJSON, &ep)` — deserialize.

All three phases produce errors wrapped with the failing phase name; phase 2 wraps with the entry-point id when extractable from the raw input.

## Package layout

```
internal/entrypoint/
├── types.go              # EntryPoint struct
├── schema.go             # //go:embed schema.yaml + compiledSchema()
├── schema.yaml           # byte-equal mirror of /schemas/entry-point.yaml
├── loader.go             # LoadEntryPoint + LoadFromExample + Validate impl
├── accessors.go          # SpecField + Label
├── drift_test.go         # asserts schema.yaml ≡ schemas/entry-point.yaml
├── load_test.go          # per-archetype happy/sad loader cases
├── accessors_test.go     # SpecField + Label correctness
├── schema_test.go        # compiledSchema() returns non-nil, no error
└── testdata/             # invalid fixtures for negative cases
    ├── missing-id.yaml
    ├── unknown-archetype.yaml
    ├── http-api-missing-method.yaml
    ├── event-consumer-bad-channel-kind.yaml
    └── worker-bad-trigger-kind.yaml
```

Per-archetype happy-path tests load from `../../schemas/examples/entry-point/<archetype>.yaml` rather than maintaining a parallel copy under `testdata/`. Negative tests live under `testdata/` because the golden examples are valid by definition.

## Test plan

- **`drift_test.go`** — byte-equal check (same shape as `internal/fixture/drift_test.go`).
- **`schema_test.go`** — `compiledSchema()` returns a non-nil schema, no error, idempotent across calls (the `sync.Once` works).
- **`load_test.go`**:
  - Happy: load each of the 9 golden examples; assert `ID`, `Archetype`, and the spec keys most relevant to that archetype (e.g., `Spec["method"] == "POST"` for `http-api`).
  - Sad: each `testdata/*.yaml` produces a load error whose message names the failing condition (missing field, unknown archetype, invalid inline-enum value).
- **`accessors_test.go`**:
  - `SpecField("method")` returns the string `"POST"` + true for a loaded `http-api` fixture.
  - `SpecField("not_a_key")` returns nil + false.
  - `Label()` returns `"http-api:create-order-endpoint"` for the golden `http-api` example.
  - `Label()` on a zero-value EntryPoint returns `":"` (documented behavior, not a panic).

## Deliverable acceptance

- `go build ./internal/entrypoint/...` succeeds.
- `go vet ./internal/entrypoint/...` clean.
- `go test ./internal/entrypoint/... -count=1` green.
- All 9 archetype golden examples load via `LoadFromExample` and produce the expected `EntryPoint`.
- Every negative `testdata/*.yaml` produces a load error.
- `drift_test.go` confirms the embedded schema matches the canonical one byte-for-byte.
- `EntryPoint` satisfies the API surface that `feat/e4-use-case`'s plan expects (fields `ID`/`Archetype`; methods `SpecField`, `Label`) so E4 can drop its "Task 2: minimal E3 scaffold" and import this package instead.

## Open follow-ups (not blocking E3)

- **Typed spec helpers** (`HTTPAPIMethod`, `EventConsumerChannel`, …) — defer until E6 (sensor generation) needs them. The current `SpecField` + `map[string]any` shape is sufficient for E4.
- **E4 reconciliation** — once E3 lands on main, the `feat/e4-use-case` plan should drop its Task 2 (E3 scaffold) and the loader should import `internal/entrypoint`. Coordinated with whoever owns E4.
