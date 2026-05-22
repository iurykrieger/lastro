# E6 — Sensor

> Source plan: [`plan.md`](plan.md) §4.4 (Sensor schema), §2 ("Sensors are the solution"), §6.1 (Resolver)

A `Sensor` is the dynamically-generated validation unit — one per `(UseCase × applicable ValidationAngle)`. It carries top-level `uses:` (StackComponents — the toolbox subset it draws from), `depends_on:` (other sensors), and ordered `steps:` (each step may bind to fixtures via `uses:`). This chunk owns the schema, Go type, the grounding validators, and the `depends_on` DAG resolver.

## Scope

In:
- The `Sensor` schema and Go type.
- Loader.
- **Grounding validator 1:** every top-level `uses:` id exists in a given `StackManifest` (E2). This is the "sensor cannot reference a non-detected component" invariant.
- **Grounding validator 2:** every step-level `uses:` id exists in the owning UseCase's fixtures (E4 + E5).
- **DAG resolver:** given a set of sensors, topologically sort by `depends_on`. Fail on cycles. Return execution order.
- Validators for: valid `angle`, `kind`, `nature`, `output_type`, immutable fields.
- Tests.

Out:
- Sensor *generation* (Phase B, `/create-sensors`).
- Sensor *execution* (Phase B, runtime — executor, signal collector).
- The angle-applicability check (does this sensor's angle apply to the use case's archetype?) — this is policy-layer concern (E9), not E6's.

## Schema (from plan §4.4)

Key fields:
- `schema_version`, `id`, `use_case_id`, `angle`, `kind`, `nature`, `output_type`
- `uses: [<StackComponent.id>]` — top-level toolbox subset (grounding invariant)
- `depends_on: [<Sensor.id>]` — optional
- `steps: [{id, run, uses?: [<Fixture.id>]}]`

## Inputs / Outputs

- **Input:** a sensor YAML file + (for grounding validation) a `StackManifest` and a `UseCase`.
- **Output:** `internal/sensor/` Go package — types, loader, grounding validators, DAG resolver.

## Dependencies

- E1 (enums) — `ValidationAngle`, `SensorKind`, `SensorNature`, `SignalOutputType`.
- E2 (StackComponent) — to validate top-level `uses:`.
- E4 (UseCase) — to know which fixtures are owned by the referenced use case.
- E5 (Fixture) — via UseCase, to validate step-level `uses:`.
- Schema-freeze gate.

**Coordination note for parallel work:** the grounding validators take `StackManifest` and `UseCase` as parameters. E6 can develop fully against stubbed inputs; integration with real E2/E4/E5 outputs happens at test time, not at code-write time.

## Open questions for `/brainstorming`

1. **`steps[].run` shape.** Plan shows `run: <command-or-skill-invocation>`. Is `run` a shell command string, a structured object (`{type: shell|skill, ...}`), or a discriminated union? Recommendation: discriminated union — `{shell: "..."}` vs `{skill: "/...", args: {...}}` — the runtime needs to dispatch differently anyway.
2. **`steps[].uses:` resolution timing.** Plan §6.1 says the *Fixture Binder* resolves step `uses:` at execution time. Does E6 do anything beyond validation? Recommendation: validate at load time, resolve at runtime. E6 is load-only.
3. **DAG cycle detection.** Tarjan, Kahn, or DFS-based? All work. Recommendation: Kahn's algorithm — easy to implement, yields the topological order directly.
4. **`depends_on` cross-use-case.** Can a sensor depend on a sensor from a different use case? Plan doesn't say explicitly. Recommendation: yes — global DAG, validated at runtime gather, not at single-sensor load.
5. **`id` derivation.** Open decision §10.5 in the plan: content-hash vs UUID. Recommendation: content hash of (use_case_id + angle + canonical step list), with a generation-metadata sidecar (when, by which skill version) — so identical regenerations dedupe.
6. **Immutable fields.** Plan §4.4 marks `use_case_id` and `angle` immutable. Where is immutability enforced — at load (reject edits via a check against a known-good version), or just convention?

## Deliverable acceptance

- `internal/sensor/` loads golden example sensors (one per angle × archetype, minimum).
- Grounding validator 1 passes for sensors that reference detected components, fails for those that don't.
- Grounding validator 2 passes for steps that reference owned fixtures, fails otherwise.
- DAG resolver produces correct topological order on a synthetic 5-node graph; rejects cycles.
- Negative tests for every invariant.
