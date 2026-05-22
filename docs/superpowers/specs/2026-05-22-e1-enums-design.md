# E1 — Fixed Enums (Design)

> Source chunk: [`docs/harness-framework/E1-enums.md`](../../harness-framework/E1-enums.md)
> Sequential gate consumed: [`docs/harness-framework/00-schema-freeze.md`](../../harness-framework/00-schema-freeze.md)
> Brainstorm date: 2026-05-22

## 1. Purpose

The framework defines 8 fixed enums whose canonical source is YAML under
`schemas/enums/`. Three audiences need to agree on those values:

| Audience | What they need | Source they read |
|---|---|---|
| Claude Code skills (LLM prompts) generating entities | The list of valid values per enum | `schemas/enums/*.yaml` directly |
| JSON Schema validators (rejecting bad generations) | Inline `enum: [...]` constraints | `schemas/*.yaml` (entity schemas) |
| Go code (Phase B runtime, future entity loaders) | Typed constants and the `applicable_angles` matrix | `internal/enums/` (this chunk) |

E1 owns the third row. It also owns a drift test that protects the second
row from going stale relative to the first.

E1 does **not** generate code, modify schema-freeze's output, or add
tooling beyond `go test`.

## 2. Scope

**In:**

- A hand-written Go package at `internal/enums/`:
  - One typed string type per enum (`ValidationAngle`, `Archetype`, `SensorKind`,
    `SensorNature`, `SignalOutputType`, `FixtureRole`, `Verdict`, `TerminationReason`).
  - One named constant per enum value.
  - `All<Enum>() []<Type>` and `IsValid<Enum>(string) bool` for each.
  - `ApplicableAngles map[Archetype][]ValidationAngle` — the canonical
    archetype × angle matrix. Sourced from `archetypes.yaml`.
  - `Applies(Archetype, ValidationAngle) bool` helper.
- Unit tests asserting the Go package's internal contract.
- Drift tests asserting Go ↔ YAML and inline-schema ↔ YAML consistency.

**Out:**

- Codegen. Constants are hand-written; drift tests keep them honest.
- Edits to existing entity schemas. The inline `enum: [...]` lists stay
  as schema-freeze produced them.
- A new `cmd/` binary. Drift detection lives in `go test`.
- Loaders for entity YAMLs. Each Phase A entity chunk (E2–E9) owns its
  own loader and imports `internal/enums` for typed constants.
- Cross-entity referential integrity (runtime concern; see schema-freeze
  README §4 and §9).
- Migration strategy when an enum value is renamed or removed
  (Phase B runtime concern).
- `framework.lock` version pinning (Phase B; see [plan.md §7](../../harness-framework/plan.md)).
- Exposing informational fields (`purpose`, `lifecycle_notes`,
  `confidence_default`) in Go. Schema-freeze README §5 names
  `applicable_angles` as the sole load-bearing enum metadata.

## 3. The 8 enums

| Enum | YAML file | Values |
|---|---|---|
| `ValidationAngle` | `validation-angles.yaml` | security, build, code-structure, unit-test, e2e-test, contracts, logs, metrics, database, performance |
| `Archetype` | `archetypes.yaml` | http-api, event-consumer, event-producer, cli, sdk, library, worker, batch-job, static-site |
| `SensorKind` | `sensor-kinds.yaml` | assertion, observational |
| `SensorNature` | `sensor-natures.yaml` | computational, inferential |
| `SignalOutputType` | `signal-output-types.yaml` | single-shot, stream |
| `FixtureRole` | `fixture-roles.yaml` | input, expected-output, expected-side-effect |
| `Verdict` | `verdicts.yaml` | pass, fail, inconclusive |
| `TerminationReason` | `termination-reasons.yaml` | completed, stopped, timeout, error |

`archetypes.yaml` additionally carries each archetype's `applicable_angles`
list. That list is the canonical archetype × angle matrix; no other source
defines it.

## 4. Package layout

```
internal/enums/
├── enums.go              ← typed string types, constants, All<X>/IsValid<X>
├── archetype_angles.go   ← ApplicableAngles map + Applies helper
├── enums_test.go         ← unit tests (internal contract)
└── drift_test.go         ← consistency tests against schemas/
```

No subpackages. No `init()` functions. No file loading at runtime. The
constants are baked in at compile time; only the drift tests read YAML.

## 5. Go API

### 5.1 Naming conventions

| Type | Constant prefix | `All` / `IsValid` form |
|---|---|---|
| `ValidationAngle` | `Angle` | `AllAngles()`, `IsValidAngle(string) bool` |
| `Archetype` | `Archetype` | `AllArchetypes()`, `IsValidArchetype(string) bool` |
| `SensorKind` | `Kind` | `AllSensorKinds()`, `IsValidSensorKind(string) bool` |
| `SensorNature` | `Nature` | `AllSensorNatures()`, `IsValidSensorNature(string) bool` |
| `SignalOutputType` | `Output` | `AllSignalOutputTypes()`, `IsValidSignalOutputType(string) bool` |
| `FixtureRole` | `Role` | `AllFixtureRoles()`, `IsValidFixtureRole(string) bool` |
| `Verdict` | `Verdict` | `AllVerdicts()`, `IsValidVerdict(string) bool` |
| `TerminationReason` | `Termination` | `AllTerminationReasons()`, `IsValidTerminationReason(string) bool` |

Constants combine the prefix with the value name in PascalCase, hyphens
dropped, with acronyms kept uppercase per Go convention:
`AngleE2ETest`, `ArchetypeHTTPAPI`, `ArchetypeEventConsumer`,
`RoleExpectedSideEffect`.

### 5.2 Sketch (`enums.go`)

```go
package enums

// ValidationAngle is one of the ten facets a sensor can validate.
type ValidationAngle string

const (
    AngleSecurity      ValidationAngle = "security"
    AngleBuild         ValidationAngle = "build"
    AngleCodeStructure ValidationAngle = "code-structure"
    AngleUnitTest      ValidationAngle = "unit-test"
    AngleE2ETest       ValidationAngle = "e2e-test"
    AngleContracts     ValidationAngle = "contracts"
    AngleLogs          ValidationAngle = "logs"
    AngleMetrics       ValidationAngle = "metrics"
    AngleDatabase      ValidationAngle = "database"
    AnglePerformance   ValidationAngle = "performance"
)

func AllAngles() []ValidationAngle {
    return []ValidationAngle{
        AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
        AngleE2ETest, AngleContracts, AngleLogs, AngleMetrics,
        AngleDatabase, AnglePerformance,
    }
}

func IsValidAngle(s string) bool {
    for _, v := range AllAngles() {
        if string(v) == s {
            return true
        }
    }
    return false
}
```

The other 7 enums follow the same pattern.

### 5.3 Sketch (`archetype_angles.go`)

```go
package enums

// ApplicableAngles is the canonical archetype × angle matrix. Sourced from
// schemas/enums/archetypes.yaml. A sensor's angle must be in the list for
// its use case's archetype.
var ApplicableAngles = map[Archetype][]ValidationAngle{
    ArchetypeHTTPAPI: {
        AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
        AngleE2ETest, AngleContracts, AngleLogs, AngleMetrics,
        AngleDatabase, AnglePerformance,
    },
    ArchetypeEventConsumer: {
        AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
        AngleE2ETest, AngleContracts, AngleLogs, AngleMetrics, AngleDatabase,
    },
    // … one entry per archetype, exactly mirroring archetypes.yaml
}

// Applies reports whether the given angle is applicable to the given archetype.
func Applies(a Archetype, v ValidationAngle) bool {
    for _, applicable := range ApplicableAngles[a] {
        if applicable == v {
            return true
        }
    }
    return false
}
```

Slice form is intentional: it preserves canonical ordering so any caller
iterating produces stable, YAML-aligned output. The matrix is 9 × ≤10;
linear scan is fine.

## 6. Drift tests (`drift_test.go`)

Two tests, both pure `go test`. They reach the schemas via `../../schemas/`.

### 6.1 `TestGoConstantsMatchYAML` (and siblings)

For each enum, parse `schemas/enums/<name>.yaml`, extract `values[*].id`
**in order**, compare to `All<Enum>()` stringified. Order matters — a
reorder is drift, because callers iterate the slice.

A separate sub-test, `TestApplicableAnglesMatchYAML`, parses
`archetypes.yaml` and asserts each archetype's `applicable_angles` list
matches `ApplicableAngles[archetype]` exactly (order included).

### 6.2 `TestInlineSchemaEnumsMatchYAML`

Walks every `enum: [...]` block in `schemas/*.yaml` (excluding
`schemas/enums/` itself and `schemas/examples/`). For each block:

1. Compute the block's value set.
2. For each canonical enum, check:
   - **Equal set:** treat as a "full enum duplicate" site. Already passes
     by construction; would fail only after a future divergent edit.
     Record that this canonical enum was referenced.
   - **Strict subset:** treat as a local domain constraint (e.g.,
     `entry-point.yaml` line 60: `enum: [sdk, library]`). Skip silently.
   - **Unrelated:** treat as a local enum unrelated to the framework's 8
     (e.g., HTTP methods, channel kinds). Skip silently.
3. After walking all sites, assert every canonical enum was referenced
   by at least one inline duplicate site. Catches "a canonical enum was
   added but no schema uses it."

Set-equality detection means **no marker comments** are needed in the
YAMLs and **no path table** is hard-coded in the test. The approach is
robust to schema refactoring.

### 6.3 Known edge case

If a future local enum is added whose value set is identical to a
canonical enum's full value set, the test would misidentify it as a
duplicate and enforce coupling that doesn't exist semantically. None of
the current local enums (HTTP methods, `queue|topic`, `cron|signal`,
`http|cli-args|event|stdout|log-line|db-row`, `library|runtime|framework|datastore|protocol|tool`,
`org|global|repo`) collide with any canonical enum. If that ever
changes, the escape hatch is a marker comment on the local site;
deferred until needed.

## 7. Unit tests (`enums_test.go`)

Separate from drift tests; they assert the Go package's internal contract
without touching YAML.

- `TestEnumValuesAreValid` — for every constant `C` of every enum, assert
  `IsValid<Enum>(string(C)) == true`. Table-driven; one row per (enum, value).
- `TestUnknownValuesRejected` — for each `IsValid<Enum>`, assert it
  returns `false` for: `""`, `"not-a-real-value"`, uppercase variant
  (e.g., `"E2E-TEST"`), and leading whitespace (e.g., `" pass"`).
- `TestApplicableAnglesMatrixCoverage` — `ApplicableAngles` has exactly
  9 keys (one per archetype), every value list is a subset of `AllAngles()`,
  no duplicate angles within a single archetype's list.
- `TestAppliesHelper` — table-driven; representative passes and fails
  (e.g., `(ArchetypeHTTPAPI, AnglePerformance)` → true,
  `(ArchetypeCLI, AnglePerformance)` → false,
  `(ArchetypeStaticSite, AngleDatabase)` → false).

## 8. Acceptance criteria

1. `internal/enums/` compiles. `go test ./internal/enums/...` passes.
2. All 8 enums have typed constants and helpers matching their canonical
   YAML in **value and order**.
3. `ApplicableAngles` covers all 9 archetypes; each list matches
   `archetypes.yaml` exactly in **value and order**.
4. `IsValid<Enum>("")` and `IsValid<Enum>("nonsense")` return `false`
   for every enum.
5. The drift test fails if Go and YAML disagree (either direction).
6. The drift test fails if a canonical enum is not referenced by any
   inline `enum: [...]` site in the entity schemas.
7. The existing `go run ./cmd/validate-schemas` continues to pass
   unchanged.

## 9. Dependencies

- Phase A sequential gate: complete (schema-freeze landed at
  `dde2779`).
- Go module: already initialized at the repo root
  (`github.com/iurykrieger/lastro`, Go 1.24.2).
- YAML parsing for drift tests: reuse `sigs.k8s.io/yaml` already in
  `go.mod`.

No new third-party dependencies.

## 10. Consumers (informational)

`internal/enums` will be imported by:

- Each Phase A entity loader (E2–E9) — for typed field values when
  deserializing entity YAMLs.
- Phase B runtime — when making programmatic decisions on `Verdict`,
  `TerminationReason`, `SensorKind`, etc., and when validating that a
  generated sensor's angle is in `ApplicableAngles[archetype]`.

Phase B is out of scope for this design but motivates the existence of
the Go package.
