# E5 — Fixture: Design Spec

> Source: [`docs/harness-framework/E5-fixture.md`](../../harness-framework/E5-fixture.md), [`plan.md`](../../harness-framework/plan.md) §4.2.
> Status: drafted 2026-05-22, awaiting written-spec review.

## 1. Purpose

Deliver the `internal/fixture/` Go package: the typed `Fixture`, a per-file loader, an in-memory `Store` with explicit constructor and duplicate-id rejection, and the `FixtureStore` interface that E4 (template resolver) and E6 (sensor grounding validator) bind against.

The schema is already frozen at [`schemas/fixture.yaml`](../../../schemas/fixture.yaml). E5 does **not** change the schema; it produces the Go-side machinery that loads, parses, and serves fixtures conforming to it.

## 2. Scope

In:
- `internal/fixture/` package — types, loader, store, validators, tests.
- Eager payload parsing for JSON, YAML, and XML content types.
- Cross-fixture invariants inside a store (no duplicate ids).
- Golden tests against `schemas/examples/fixture/` plus in-tree fixtures under `internal/fixture/testdata/` for cases the examples don't cover.

Out:
- Fixture *generation* (Phase B, `/detect-use-cases`).
- Template interpolation that *uses* fixtures (E4 owns it).
- Role × archetype × channel coherence checks (sensor-time, owned by E6 / future angle validators).
- The `/detect-use-cases` skill itself.

## 3. Decisions captured from brainstorm

| # | Decision | Rationale |
|---|---|---|
| 1 | **Eager parse** payloads at load time, dispatched by `content_type`. | Catches malformed structured payloads at the loader boundary; E4's jsonpath resolver reads a parsed tree without re-parsing. |
| 2 | **Schema-only validator scope.** No role × binding × archetype coherence checks in E5. | Keeps E5 standalone and decoupled. Sensor-time validators have full context to do the coherence check. |
| 3 | **Three-method `FixtureStore` interface:** `LookupFixture`, `FixturesForUseCase`, `All`. | Directly serves both consumers (E4 + E6) plus admin/test iteration. |
| 4 | **Per-file loader + explicit `NewStore` constructor + `LoadDirectory` helper.** Duplicate ids in a store are a build error. | Tests build stores from in-memory fixtures; runtime walks a directory. One concept, two entry points. |
| 5 | **Parse dispatch covers JSON, YAML, XML.** Other content types load with `Parsed == nil` and raw payload preserved. | Matches the breadth the framework will plausibly see; binary / text remain raw. |
| 6 | **XML library: `github.com/clbanning/mxj/v2`.** | Produces a JSON-shaped `map[string]any`, consistent with the JSON/YAML branches. Most likely future revisit if XML usage grows. |

## 4. Package surface

```go
package fixture

type Role string    // input | expected-output | expected-side-effect
type Channel string // http | cli-args | event | stdout | log-line | db-row

type SourceRef struct {
    Path   string
    Symbol string
    Reason string
}

type Binding struct {
    Channel  Channel
    Selector map[string]any
}

type Fixture struct {
    SchemaVersion string
    ID            string
    UseCaseID     string
    Role          Role
    ContentType   string
    Payload       []byte          // raw bytes from YAML payload field
    Parsed        any             // eager parse result; nil for non-structured content types
    Binding       *Binding        // optional, may be nil (schema-optional)
    SourceRefs    []SourceRef
}

// Loader: per-file.
func LoadFixture(path string) (Fixture, error)

// Store: explicit constructor, rejects duplicate ids.
func NewStore(fixtures ...Fixture) (*Store, error)

// Directory walker, composes LoadFixture + NewStore.
func LoadDirectory(path string) (*Store, error)

type Store struct { /* unexported */ }

func (s *Store) LookupFixture(id string) (Fixture, bool)
func (s *Store) FixturesForUseCase(useCaseID string) []Fixture // sorted by id asc, deterministic
func (s *Store) All() []Fixture                                // sorted by id asc, deterministic

// Interface seam — both E4 and E6 depend on this, not on *Store.
type FixtureStore interface {
    LookupFixture(id string) (Fixture, bool)
    FixturesForUseCase(useCaseID string) []Fixture
    All() []Fixture
}
```

`*Store` implements `FixtureStore` by definition; downstream packages take the interface so they can be stubbed easily.

## 5. Loader pipeline

`LoadFixture(path)` runs five phases. Any failure returns an error mentioning the file path and the failing phase.

1. **Read & YAML→JSON.** Read file bytes. Convert YAML to JSON via `sigs.k8s.io/yaml`. Normalizes the document for the next two phases.
2. **JSON Schema validation.** Validate the JSON against the compiled `schemas/fixture.yaml` using `github.com/santhosh-tekuri/jsonschema/v6`. Catches missing required fields, invalid `role`, invalid `binding.channel`, bad `id` pattern, bad `schema_version` shape, unexpected top-level keys. The compiled schema is cached behind `sync.Once`.
3. **Deserialize.** Unmarshal the JSON into a typed `Fixture` struct via Go struct tags. `Payload` is captured as `[]byte` from the YAML `payload` string field (UTF-8 bytes of the literal payload text).
4. **Eager payload parse.** Dispatch on `ContentType`:
   - `application/json` or `application/*+json` → `encoding/json.Unmarshal` into `any`.
   - `application/yaml`, `text/yaml`, `application/x-yaml` → `sigs.k8s.io/yaml.Unmarshal` into `any`.
   - `application/xml`, `text/xml`, `application/*+xml` → parse via `mxj/v2` into `map[string]any`.
   - Anything else → `Parsed = nil`; `Payload` is the source of truth.

   A malformed payload for a structured content type is a load error. An unknown content type is **not** an error.
5. **Return** the populated `Fixture`.

Suffix matching (`+json`, `+xml`) follows the spirit of RFC 6839. YAML has no analogous suffix convention; the three listed media types are the canonical set.

`LoadDirectory(path)` walks `path` non-recursively for `*.yaml` and `*.yml` files (one fixture per file, per the doc's convention), calls `LoadFixture` on each, and hands the result to `NewStore(...)`. Walk errors and any per-file load error abort the whole load with the offending file path in the error.

`NewStore(fixtures ...Fixture)` builds the internal id index. If two fixtures share an id, returns an error of the shape `fixture: duplicate id "X"` — with enough detail for `LoadDirectory` to add the file paths.

## 6. Validator scope

E5 validates:

- Everything expressible in the JSON Schema (phase 2).
- Payload syntax for `application/json`, `application/yaml`, `application/xml` and their suffix-matched variants (phase 4).
- Cross-fixture: no duplicate `id`s in a single store (`NewStore`).

E5 explicitly does **not** validate:

- Whether `Binding` is present. The frozen schema lists `binding` as optional; E5 honors that.
- Whether `Role` is consistent with `Binding.Channel` (e.g., `input` + `log-line` is suspicious but legal here).
- Whether `Binding.Selector` has the "right" fields for its `Channel`. `Selector` stays a `map[string]any`.
- Whether `UseCaseID` refers to a real use case. E4 owns the use-case world.
- Whether `Parsed` is semantically valid for its use case.

All of those are sensor-time concerns, owned downstream.

## 7. Tests

All under `internal/fixture/`, following the repo's `_test.go`-sibling convention.

**`fixture_load_test.go` — happy path:**
- Load each of `schemas/examples/fixture/{input,expected-output,expected-side-effect}.yaml`.
- For JSON examples, assert `Parsed` is a `map[string]any` with the expected keys.
- For the `text/plain` log-line example, assert `Parsed == nil` and `Payload` is the literal payload.
- Load in-tree fixtures from `internal/fixture/testdata/`:
  - One YAML-content-type fixture → asserts `Parsed` is a map.
  - One XML-content-type fixture → asserts `Parsed` is a map (mxj shape).
  - One fixture with no `binding` block → asserts `Binding == nil`, no error.

**`fixture_store_test.go` — store API:**
- `NewStore` happy path: three distinct fixtures across two use cases.
- `LookupFixture(known)` → `(fx, true)`. `LookupFixture(unknown)` → `(zero, false)`.
- `FixturesForUseCase(uc)` returns only matching fixtures, sorted by id ascending.
- `All()` returns every fixture, sorted by id ascending.
- **Reuse demonstration** (matches E5 acceptance criterion): one fixture loaded once, two distinct call sites in the same test get it via `LookupFixture` and the test asserts both observe the same parsed tree (proving eager parse happens once and is shared).

**`fixture_validation_test.go` — negative cases:**
- Missing required `payload` → load error mentioning `payload`.
- Invalid `role` value (e.g., `"output"`) → schema-validation error.
- Invalid `binding.channel` value → schema-validation error.
- Malformed JSON in `application/json` payload → load error from phase 4.
- Malformed YAML in `application/yaml` payload → load error from phase 4.
- Malformed XML in `application/xml` payload → load error from phase 4.
- Unknown content type (e.g., `application/octet-stream`) with non-empty payload → loads cleanly; `Parsed == nil`.
- `NewStore` with two fixtures sharing an id → returns the duplicate-id error.
- `LoadDirectory` over a directory containing the above duplicate-id pair → returns the duplicate-id error with both file paths.

**Coverage floor.** No percentage gate, but every exported symbol must be exercised and every load-error code path must have a negative test.

## 8. Dependencies

**New:**
- `github.com/clbanning/mxj/v2` — XML → `map[string]any`.

**Existing (already in `go.mod`):**
- `sigs.k8s.io/yaml` — YAML→JSON normalization and YAML payload parsing.
- `github.com/santhosh-tekuri/jsonschema/v6` — JSON Schema validation.

**Inter-package:**
- E1 (`internal/enums/`) — if available, E5 reuses `Role` and `Channel` string constants. If E1 isn't landed yet, E5 ships its own constants with identical string values; unifying later is a no-op import change.
- Schema-freeze gate — satisfied (schemas exist, examples pass).

## 9. Integration seam

**E4 (template resolver):**
- Imports `fixture.FixtureStore`.
- `{{fixtures.<id>}}` → `store.LookupFixture(id)`.
- `{{fixtures.<id>.<jsonpath>}}` → walk `Fixture.Parsed` with E4's jsonpath. If `Parsed == nil`, E4 returns its own error (E5 does not).
- Test isolation: E4 builds in-memory stores via `NewStore(...)` — no filesystem dependency.

**E6 (sensor grounding validator 2):**
- For each `step.uses[]` fixture id, checks `LookupFixture(id)` returns true **and** the resulting fixture's `UseCaseID == sensor.UseCaseID`.
- May use `FixturesForUseCase(sensor.UseCaseID)` to produce "did you mean X?" hints on validation failure.
- Test isolation: same as E4.

**Parallel-work guarantee:** E4 and E6 can develop entirely against `FixtureStore` interface stubs while E5 is in flight. The interface is small, frozen, and lives in this spec.

## 10. Acceptance criteria

Mirror of the E5 doc's deliverable acceptance, made concrete:

- `internal/fixture/` loads cleanly on `schemas/examples/fixture/{input,expected-output,expected-side-effect}.yaml`.
- All tests listed in §7 pass.
- `FixtureStore` interface is exported and `*Store` satisfies it (asserted by a compile-time `var _ FixtureStore = (*Store)(nil)`).
- A test demonstrates payload reuse: one fixture, two consumer call sites, shared parsed tree.
- `LoadDirectory` over `schemas/examples/fixture/` produces a store with three fixtures, all sharing `UseCaseID == "create-order-use-case"` and covering all three `Role` values.
- `go vet ./internal/fixture/...` and `go test ./internal/fixture/...` both pass.

## 11. Out of scope (deferred decisions)

- **Binary payloads / payload_uri.** The E5 doc's Q5 raised external-reference for large binary payloads. The frozen schema doesn't include `payload_uri` — punted to a future schema bump. Today, binary payloads load as raw bytes with `Parsed == nil`; a sensor that needs to ship them to an HTTP endpoint reads `Payload` directly.
- **Content-hash-based id dedup.** The doc's Q4 (reuse contract). Today: id-only — the user / `/detect-use-cases` decides reuse intentionally. No content-hash dedup in E5.
- **`Selector` as discriminated union.** Doc's Q3. The frozen schema keeps it `map[string]any`; promotion to a typed union would require both a schema change and a coordinated E5 / sensor-time validator change. Defer until a real consumer demands it.
