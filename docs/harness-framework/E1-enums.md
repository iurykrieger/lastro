# E1 — Fixed Enums

> Source plan: [`plan.md`](plan.md) §3 (Fixed Enums), §2.1 (Archetype → applicable_angles)

Six fixed enums underpin the whole framework. They are not user-extensible — they evolve only with a framework version bump. This chunk owns their canonical definitions, Go constants, and the `archetype → applicable_angles` mapping.

## Scope

In:
- Six enum YAML files (locked in [`00-schema-freeze.md`](00-schema-freeze.md); this chunk consumes them)
- A Go package `internal/enums/` with typed constants for each enum
- The `archetype → applicable_angles` lookup table (data-driven, sourced from the YAML)
- Validators: `IsValidAngle(string) bool`, `IsValidArchetype(string) bool`, etc.
- Tests covering: every enum value is recognized, unknown values rejected, applicable-angles lookup is correct.

Out:
- Anything that consumes the enums (entity schemas, sensors, policies). Those live in their own chunks.

## Enums in scope

| Enum | Values |
|---|---|
| `ValidationAngle` | security, build, code-structure, unit-test, e2e-test, contracts, logs, metrics, database, performance |
| `Archetype` | http-api, event-consumer, event-producer, cli, sdk, worker, batch-job, static-site, library |
| `SensorKind` | assertion, observational |
| `SensorNature` | computational, inferential |
| `SignalOutputType` | single-shot, stream |
| `FixtureRole` | input, expected-output, expected-side-effect |

## Inputs / Outputs

- **Input:** the frozen YAML enum files from schema-freeze gate.
- **Output:** `internal/enums/` package — typed Go constants, the `applicable_angles` table, validators.

## Dependencies

- Phase A: schema-freeze (the YAML files exist).
- No other entity chunk depends on E1 directly — they import the Go package and reference enum values by their string keys.

## Open questions for `/brainstorming`

1. **Code generation vs hand-written.** Generate Go constants from the YAML at build time (one source of truth, slight tooling overhead), or hand-maintain both with a test that asserts they match? Recommendation: generate, with `go generate` directive.
2. **`applicable_angles` source.** Plan §2 says each archetype declares its applicable angles. Where does this live — inside `archetypes.yaml`, or in a separate `archetype-angle-matrix.yaml`? Recommendation: inside `archetypes.yaml` — keep the mapping next to the archetype definition.
3. **Open vs closed type.** Should an unknown enum value at load time be a hard error or a warning-with-fallback? Recommendation: hard error. Closed type is the whole point of "fixed enums."
4. **Version bumping.** When an enum adds a new value (e.g., a new ValidationAngle), what version field bumps and where? The framework-level `framework.lock`, the per-schema `schema_version`, both?

## Deliverable acceptance

- `internal/enums/` Go package compiles, tests pass, 100% of enum values covered by tests.
- A `go generate ./...` (if chosen) regenerates from YAML deterministically.
- A negative test: `IsValidAngle("not-a-real-angle")` returns false.
- The `applicable_angles` matrix covers all 9 archetypes.
