# E6 — Sensor: Design Spec

> Source: [`docs/harness-framework/E6-sensor.md`](../../harness-framework/E6-sensor.md), [`plan.md`](../../harness-framework/plan.md) §4.4, §2, §6.1.
> Status: drafted 2026-05-23, awaiting written-spec review.

## 1. Purpose

Deliver the `internal/sensor/` Go package: the typed `Sensor`, a per-file loader, an in-memory `Store` mirroring E5's pattern, two opt-in grounding validators (against the detected stack and against the use case's owned fixtures), and a deterministic DAG resolver for `depends_on` ordering.

The schema is already frozen at [`schemas/sensor.yaml`](../../../schemas/sensor.yaml). E6 does **not** change the schema; it produces the Go-side machinery that loads, validates, indexes, and orders sensors conforming to it.

## 2. Scope

In:
- `internal/sensor/` package — types, loader, store, intrinsic + grounding validators, resolver, tests.
- The `UseCaseFixtureOwnership` interface — the seam between this package and the authoritative source of "which fixtures does use case X own." Today's wiring resolves it from E4's `*usecase.UseCase.FixtureIDs`; a `FixtureStore`-backed adapter remains available for callers that don't have UseCase context (heal-loop selective re-runs, sensor-only inspection).
- Two grounding validators exposed as separate functions: top-level `uses` ⊂ detected stack, step-level `uses` ⊂ owned fixtures.
- Deterministic topological sort over a sensor slice via Kahn's algorithm; typed errors for cycles and missing dependencies.
- Golden tests against `schemas/examples/sensor/` plus in-tree fixtures under `internal/sensor/testdata/` for negative cases.

Out:
- Sensor *generation* (Phase B, `/create-sensors`).
- Sensor *execution* — the executor, fixture binder, signal collector, aggregator (Phase B runtime).
- The angle-applicability check (does this sensor's angle apply to the use case's archetype?) — owned by E9 / policy layer.
- `id` derivation strategy (content-hash vs UUID) — owned by `/create-sensors` generator; this package takes `id` as given by the YAML.
- Immutability enforcement on `use_case_id` / `angle` — load-only has no notion of edit; immutability lives with version control and the generator.
- Runtime interpretation of `step.run` (shell vs skill dispatch) — owned by Phase B executor.

## 3. Decisions captured from brainstorm

| # | Decision | Rationale |
|---|---|---|
| 1 | **Grounding validators are opt-in functions**, not part of `LoadSensor`. | Matches E2/E5: `Load` does schema + intrinsic checks only; cross-entity checks are separate. Lets `LoadSensor` work in tests and inspection without standing up a `StackManifest` or `FixtureStore`. Honors the E6 doc's coordination note about developing against stubs. |
| 2 | **Fixture ownership via a one-method interface**, not direct dependency on E4's `*usecase.UseCase` or E5's `FixtureStore`. | Decouples E6 from the choice of ownership source. Production wires it over `*usecase.UseCase.FixtureIDs` (now that E4 has landed, that's the authoritative declaration); tests use a one-line `map[string][]string` fake; the heal-loop, which may not have a full UseCase in scope, can wire it over `fixture.Store.FixturesForUseCase`. Mirrors how E5 exposed `FixtureStore` as a seam. |
| 3 | **Free-function `ResolveExecutionOrder` + a `Store` type with lookups.** No stateful `Resolver`. | Topological sort is pure; state adds nothing. The `Store` is a separate concern (persistence and id indexing) that Phase B will need for selective re-runs after heal. Both compose: `ResolveExecutionOrder(store.All())`. |
| 4 | **Reuse `schemas.FS`** (E2 pattern), not a local `schema.yaml` copy + drift test (E5 pattern). | E2 is the newer pattern; reusing the central embed eliminates the duplicate file and the drift test that has to babysit it. Schema cached lazily behind `sync.Once`. |
| 5 | **Kahn's algorithm** for the DAG resolver. | Yields topological order directly, easy to implement, easy to reason about for cycle reporting (cycle members are exactly the nodes left with non-zero in-degree at termination). |
| 6 | **Typed errors** (`ErrMissingDependency`, `ErrCycle`) only for the resolver. Other validation paths return joined `fmt.Errorf` text via `errors.Join`. | The heal loop (Phase B) branches on missing-dep vs cycle to decide between widening the affected-sensors slice and reporting unrecoverable. No other caller branches on validation errors — they're all "fix the YAML." Considered (and rejected) E4's `ValidationError{Code, Message}` pattern: it's worth the API weight when downstream tools group errors by code (E4's case — many distinct USECASE_* codes), but E6's intrinsic checks are a small fixed set where text is clearer. If a future consumer (CI dashboard, dedicated heal-rule) needs codes, they can be layered on non-breakingly. |
| 7 | **Deterministic output ordering everywhere**: `Store.All` / `Store.ForUseCase` sorted by id asc; resolver tiebreaks ties by id asc. | Reproducible CI output. Same input always yields same ordering across runs and across machines. |
| 8 | **Cross-use-case `depends_on` is allowed.** | The frozen schema doesn't restrict it; the plan doesn't forbid it. The resolver operates on the global slice passed in. Missing-id errors fire if the slice was too narrow for the edges. |

## 4. Package surface

```go
package sensor

type Sensor struct {
    SchemaVersion string                  // semver
    ID            string
    UseCaseID     string
    Angle         enums.ValidationAngle
    Kind          enums.SensorKind
    Nature        enums.SensorNature
    OutputType    enums.SignalOutputType
    Uses          []string                // StackComponent ids (grounding invariant 1)
    DependsOn     []string                // Sensor ids (optional)
    Steps         []Step
}

type Step struct {
    ID   string
    Run  string
    Uses []string                         // Fixture ids (grounding invariant 2)
}

// Interface seam — E6 depends on this, not on *fixture.Store or *usecase.UseCase.
type UseCaseFixtureOwnership interface {
    OwnedFixtureIDs(useCaseID string) []string
}

// Loader: per-file. Runs schema + intrinsic validation; no grounding.
func LoadSensor(path string) (Sensor, error)

// Directory walker, composes LoadSensor + NewStore.
func LoadDirectory(path string) (*Store, error)

// Store: explicit constructor, rejects duplicate ids.
type Store struct { /* unexported */ }

func NewStore(sensors ...Sensor) (*Store, error)
func (s *Store) LookupSensor(id string) (Sensor, bool)
func (s *Store) ForUseCase(useCaseID string) []Sensor // sorted by id asc
func (s *Store) All() []Sensor                        // sorted by id asc

// Grounding validators — opt-in, pure functions.
func ValidateAgainstStack(s Sensor, manifest stack.StackManifest) error
func ValidateAgainstFixtures(s Sensor, owner UseCaseFixtureOwnership) error

// DAG resolver — pure function, takes any slice, returns topological order.
func ResolveExecutionOrder(sensors []Sensor) ([]Sensor, error)

// Typed errors returned by the resolver.
type ErrMissingDependency struct {
    Sensor     string   // sensor that owns the dangling edge
    MissingIDs []string // referenced ids not present in the input slice
}
func (e *ErrMissingDependency) Error() string

type ErrCycle struct {
    InvolvedIDs []string // sensors still with in-degree > 0 after Kahn's
}
func (e *ErrCycle) Error() string
```

All other helpers (compiled-schema cache, intrinsic validation, id-set utilities) stay unexported.

## 5. Loader pipeline

`LoadSensor(path)` runs five phases. Any failure returns an error mentioning the file path and the failing phase.

1. **Read** the file bytes.
2. **JSON Schema validation.** YAML → JSON via `sigs.k8s.io/yaml`; validate the JSON against the compiled `schemas/sensor.yaml` (loaded once from `schemas.FS`, cached behind `sync.Once`). Catches missing required fields, invalid enum values, bad `id` patterns, bad `schema_version` shape, unexpected top-level keys, empty `steps[]`.
3. **Deserialize.** Unmarshal the JSON into a typed `Sensor` struct via JSON tags.
4. **Intrinsic post-schema validation** (see §6.1). Returns joined errors if multiple invariants fail.
5. **Return** the populated `Sensor`. No grounding here — that requires external dependencies the caller may not have in scope.

`LoadDirectory(path)` walks `path` non-recursively for `*.yaml` and `*.yml` files (one sensor per file, matching E5's convention), calls `LoadSensor` on each, and hands the result to `NewStore(...)`. Walk errors and per-file load errors abort the whole load with the offending file path. Duplicate-id errors name **both** source files (matching E5's diagnostic style).

## 6. Validator scope

### 6.1 Intrinsic validators (run by `LoadSensor`)

Catch invariants JSON Schema can't express, accumulated and joined with `errors.Join` so a single load surfaces every violation:

- **Step ids unique within a sensor.** Schema validates each id's pattern but not array-level uniqueness.
- **Top-level `uses` ids unique.** A sensor listing `[node, node]` is a generator bug.
- **Step-level `uses` ids unique within the same step.** Same reasoning.
- **`depends_on` does not contain the sensor's own id.** A trivial self-cycle, caught here so the resolver never sees it.

### 6.2 Grounding validators (opt-in)

**`ValidateAgainstStack(s, manifest)`** — for each id in `s.Uses`, asserts `manifest.ById(id)` returns true. All offending ids are collected and reported via a joined error.

**`ValidateAgainstFixtures(s, owner)`** — computes `owned := set(owner.OwnedFixtureIDs(s.UseCaseID))`, then for each step iterates `step.Uses` and asserts every id is in `owned`. Errors are reported per step, naming the step id and the unknown fixture ids.

Both validators are pure (no state, no caching, no side effects). Tests construct minimal `Sensor` values in-code and pass real `StackManifest` / fake `UseCaseFixtureOwnership` implementations.

### 6.3 What E6 explicitly does **not** validate

- **Whether `Angle` is `applicable` for the use case's archetype.** That's an E9 / policy concern.
- **Whether `Angle` is currently `disabled` by the active policy.** Same — Phase B aggregator's job.
- **Whether the use case referenced by `UseCaseID` exists at all.** E6 has no use-case world view; that check belongs to the integration layer once E4 lands.
- **Whether `step.Run` is well-formed.** It's an opaque string here; the executor (Phase B) interprets it.
- **Whether `depends_on` ids are real sensors.** That's the resolver's concern at graph-build time, not the loader's — sensors load fine in isolation.

## 7. DAG resolver

`ResolveExecutionOrder(sensors []Sensor) ([]Sensor, error)` implements Kahn's algorithm:

1. Build an `in_degree` map from `DependsOn` edges and an `adj` map from each sensor to its dependents.
2. Detect dangling edges first: any `DependsOn` id not present in the input slice → `ErrMissingDependency` naming the owning sensor and the missing ids. (Do this up front so callers get a clean error class even when the dangling reference happens to be part of a would-be cycle.)
3. Initialize a queue with all sensors whose `in_degree == 0`. **Sort the initial queue by id** ascending.
4. Pop the smallest, append to result. For each dependent, decrement its in-degree; when it reaches zero, insert into the queue **maintaining sorted-by-id order**.
5. When the queue empties: if `len(result) == len(sensors)`, return the result. Otherwise, every sensor still with `in_degree > 0` is part of a cycle → `ErrCycle{InvolvedIDs: <sorted ids>}`.

The resolver is `O(V + E log V)` for the sorted-insert path; trivially deterministic on identical input.

## 8. Tests

All under `internal/sensor/`, following the repo's `_test.go`-sibling convention.

**`loader_test.go` — load happy path + schema/intrinsic negatives:**
- Load each file in `schemas/examples/sensor/` (6 today — assertion × {computational, inferential} × {single, stream}, plus 2 observational variants); assert every field deserializes.
- Negative cases from `testdata/invalid/`:
  - Missing required `uses` → schema-validation error.
  - Empty `steps[]` → schema-validation error.
  - Invalid `angle` value → schema-validation error.
  - Malformed YAML → load error from phase 1.

**`validate_test.go` — intrinsic post-schema checks:**
- Duplicate step id → joined error naming the duplicate id.
- Duplicate top-level `uses` id → joined error naming the duplicate.
- Duplicate step-level `uses` id within the same step → joined error.
- Sensor whose `depends_on` contains its own id → joined error.
- A file violating two invariants → joined error contains both violations (assert via `errors.Is` walking the join).

**`store_test.go` — store API:**
- `NewStore` happy path: three distinct sensors across two use cases.
- `LookupSensor(known)` → `(s, true)`. `LookupSensor(unknown)` → `(zero, false)`.
- `ForUseCase(uc)` returns matching sensors, sorted by id ascending.
- `All()` returns every sensor, sorted by id ascending.
- `NewStore` with two sensors sharing an id → returns the duplicate-id error.
- `LoadDirectory` over `testdata/duplicate-id/` (containing `a.yaml` + `b.yaml` with the same sensor id) → returns the duplicate-id error with both file paths.

**`grounding_test.go` — grounding validators:**
- **Stack:**
  - `ValidateAgainstStack` against `schemas/examples/stack-manifest/http-api.yaml` for a sensor whose `uses` is a subset of the manifest's components → `nil` error.
  - Sensor referencing two unknown stack ids → joined error naming both.
- **Fixtures:**
  - Uses a one-line `fakeOwner map[string][]string` implementing `UseCaseFixtureOwnership`.
  - Sensor with steps referencing owned fixtures → `nil` error.
  - Sensor with two steps referencing unknown fixtures → joined error, one entry per offending step naming the step id and unknown fixture ids.
  - Sensor with no step-level `uses` (e.g., a `build` sensor) → `nil` error trivially.

**`resolver_test.go` — DAG resolver:**
- Empty slice → `[]`, `nil`.
- Single sensor, no deps → `[that sensor]`, `nil`.
- Linear chain `A ← B ← C` → `[A, B, C]`.
- Diamond `A ← {B, C} ← D` → `[A, B, C, D]` (deterministic id tiebreak).
- 5-node synthetic graph (per E6 acceptance) with mixed fan-out/fan-in → known good order, asserted exactly.
- Cross-use-case edges (sensor A in use case u1, sensor B in u2, `B.depends_on=[A]`) → `[A, B]`, `nil`.
- Cycle `A ← B ← A` → `*ErrCycle{InvolvedIDs: [A, B]}` (assert via `errors.As`).
- Missing dependency: input `[A, B]`, `B.depends_on=[Z]` → `*ErrMissingDependency{Sensor: "B", MissingIDs: ["Z"]}`.
- Determinism: same diamond, 100 runs, all yield the same slice.

**`schema_test.go` — embed sanity:**
- `compiledSchema()` returns non-nil on first call, same instance on subsequent calls.
- `schemas.FS` contains `sensor.yaml` (defensive against accidental embed deletion).

**Coverage floor.** No percentage gate, but every exported symbol must be exercised and every error code path must have a negative test.

## 9. Dependencies

**No new third-party additions.** All required libs already in `go.mod` via E2/E5:

- `sigs.k8s.io/yaml` — YAML→JSON normalization.
- `github.com/santhosh-tekuri/jsonschema/v6` — JSON Schema validation.

**Inter-package:**
- `github.com/iurykrieger/lastro/schemas` — for `schemas.FS` (embedded `sensor.yaml`).
- `github.com/iurykrieger/lastro/internal/enums` — typed constants for `ValidationAngle`, `SensorKind`, `SensorNature`, `SignalOutputType`.
- `github.com/iurykrieger/lastro/internal/stack` — `StackManifest` as parameter to `ValidateAgainstStack`.
- Schema-freeze gate — satisfied (sensor schema + 6 example files exist).

**Not depended on:**
- `internal/fixture` — `UseCaseFixtureOwnership` interface decouples us. Phase B wiring code in `cmd/harness/` imports both and bridges them.
- `internal/usecase` — exists today (as of commit `dc25e74`) but not imported here. The `UseCaseFixtureOwnership` interface keeps the dependency arrow E4 → wiring → E6 instead of E4 → E6. Tests need no `*usecase.UseCase` construction (which requires entry_points + given/when/then + a fixture store) just to validate a sensor.
- `internal/entrypoint` — the E4-driven stub exists but isn't referenced by sensors at all (sensors reference use-case ids, not entry-point ids).

## 10. Integration seam

**Phase B `/run-sensor` and runtime executor:**
- Load all sensors via `LoadDirectory(".harness/sensors/")`.
- Construct the ownership adapter (either `useCaseOwnership` over loaded `*usecase.UseCase` map, or a `FixtureStore`-backed adapter for sensor-only paths).
- Call `ValidateAgainstStack` per sensor against the loaded `stack.StackManifest`.
- Call `ValidateAgainstFixtures` per sensor with the chosen adapter.
- Call `ResolveExecutionOrder` over the full store to obtain dispatch sequence.

**Phase B `/heal` selective re-validation:**
- After applying a fix, fetch `store.ForUseCase(affectedUseCase)`.
- Re-run `ResolveExecutionOrder` over the slice. If `*ErrMissingDependency` fires, the heal loop widens the slice (adds the missing ids from the full store) and retries.
- `*ErrCycle` is unrecoverable — reported to the user as a generator-side bug.

**E4 (UseCase, current):**
- `*usecase.UseCase` already exposes `FixtureIDs []string`. The production wiring adapter is a single tiny type living wherever Phase B wires components together (e.g., `cmd/harness/`):

  ```go
  type useCaseOwnership map[string]*usecase.UseCase  // keyed by UseCase.ID

  func (m useCaseOwnership) OwnedFixtureIDs(useCaseID string) []string {
      if uc, ok := m[useCaseID]; ok {
          return uc.FixtureIDs
      }
      return nil
  }
  ```

  A returned `nil` (use case unknown) causes `ValidateAgainstFixtures` to fail every step that has a fixture ref, since `nil` is treated as an empty owned-set. That's the right behavior — a sensor pointing at a missing use case shouldn't load + ground silently.

**Parallel-work guarantee (historical):** E6's interface seam was originally a hedge against E4 not yet existing. With E4 merged, the interface still earns its keep for test stubbing and for the heal-loop's `FixtureStore`-backed alternate adapter.

## 11. Acceptance criteria

Mirror of the E6 doc's deliverable acceptance, made concrete:

- `internal/sensor/` loads every file in `schemas/examples/sensor/` cleanly.
- All tests listed in §8 pass; `go vet ./internal/sensor/...` and `go test ./internal/sensor/...` both clean.
- `ValidateAgainstStack` returns `nil` for a sensor whose `uses` is a subset of the loaded `http-api` stack manifest, and a joined error naming all offenders when not.
- `ValidateAgainstFixtures` returns `nil` for a sensor whose step `uses` are owned by its use case, and a per-step joined error when not.
- `ResolveExecutionOrder` produces the correct topological order on the 5-node synthetic graph and rejects cycles with `*ErrCycle`.
- A compile-time assertion `var _ UseCaseFixtureOwnership = (*fakeOwner)(nil)` (in the test) keeps the interface stable.
- `*Store` exposes `LookupSensor`, `ForUseCase`, `All` and returns sorted output, asserted in `store_test.go`.

## 12. Out of scope (deferred decisions)

- **Sensor `id` derivation strategy.** Plan §10.5 (content hash vs UUID). E6 takes id as-given; resolution is `/create-sensors` generator's concern in Phase B.
- **Immutability enforcement of `use_case_id` / `angle`.** Schemas are stateless; immutability lives with version control and the generator that refuses to rewrite. E6 load-only has no notion of "edit."
- **Cross-use-case dependency policy.** Allowed structurally today. Whether the executor or `/validate-use-case` should disallow it under some configurations is a Phase B / policy decision.
- **`step.Run` discriminated union (shell vs skill dispatch).** Schema says string; runtime interpretation deferred to Phase B executor.
- **Caching across `LoadSensor` calls.** Each call re-reads + re-validates. If Phase B needs caching, it adds a wrapper without changing E6.
- **Pluggable cycle-detection algorithm.** Kahn's is hard-coded. If a use case for Tarjan (e.g., reporting all SCCs separately for huge graphs) emerges, swap behind the existing function signature.
