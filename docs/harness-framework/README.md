# Harness Framework — Parallel Implementation Decomposition

The framework plan in [`plan.md`](plan.md) is too large for one implementation pass. This directory decomposes it into parallel-implementable chunks, each scoped to a single entity or contract, so each can be brainstormed and built independently without an implementing agent having to reason about the whole framework at once.

## Decomposition strategy

Two phases:

- **Phase A — Entity foundation.** One chunk per entity. Each chunk owns its YAML schema, Go type, validator, entity-local pure logic, and tests. Independent and parallel after the sequential gate below.
- **Phase B — Verbs (skills + runtime + CLI).** Cross-entity work. Sliced separately after Phase A is complete. Not yet decomposed here.

### Sequential gate (must complete before Phase A fans out)

[`00-schema-freeze.md`](00-schema-freeze.md) — write all entity YAML schema files together in one PR. No Go code. This locks the cross-entity contract (field names, ids, references) so parallel entity work doesn't drift.

## Phase A — Entity chunks (parallel)

Each doc below is a brainstorming starter, not a finished spec. Each will go through its own `/brainstorming` session to produce a design doc, then `/writing-plans` to produce an implementation plan.

| ID | Entity | Doc |
|---|---|---|
| E1 | Fixed enums (6 of them) | [`E1-enums.md`](E1-enums.md) |
| E2 | StackComponent | [`E2-stack-component.md`](E2-stack-component.md) |
| E3 | EntryPoint | [`E3-entry-point.md`](E3-entry-point.md) |
| E4 | UseCase | [`E4-use-case.md`](E4-use-case.md) |
| E5 | Fixture | [`E5-fixture.md`](E5-fixture.md) |
| E6 | Sensor | [`E6-sensor.md`](E6-sensor.md) |
| E7 | Signal | [`E7-signal.md`](E7-signal.md) |
| E8 | AggregateSignal | [`E8-aggregate-signal.md`](E8-aggregate-signal.md) |
| E9 | ValidationPolicy | [`E9-validation-policy.md`](E9-validation-policy.md) |

## Dependency graph

```
                      00-schema-freeze
                             |
        +----------+---------+---------+----------+----------+
        |          |         |         |          |          |
       E1         E2        E3        E5         E7         E9
      enums   stack-comp  entry-pt   fixture    signal    policy
        |          |         |         |          |          |
        +----+-----+---------+----+----+----------+----+-----+
             |                    |                    |
             |                   E4                   E8
             |                use-case          aggregate-signal
             |             (uses E1, E3, E5)    (uses E1, E7)
             |                    |
             +--------------------+
                                  |
                                 E6
                              sensor
                       (uses E1, E2, E4, E5)
```

Inside Phase A, the only dependency is **everyone reads the frozen schemas from the gate**. The arrows above are conceptual reference dependencies (Sensor references UseCase ids, etc.), but because the schemas are frozen first, the Go implementations can proceed in parallel without coordination.

## Per-chunk contract

Every entity chunk must produce:

1. The YAML schema file under `schemas/` (already written in the schema-freeze gate; the chunk validates and refines it).
2. A Go package under `internal/<entity>/` with:
   - Typed structs with YAML tags
   - A loader that deserializes + validates against the schema
   - Any entity-local pure logic (e.g., template resolver for UseCase, DAG resolver for Sensor, rollup for AggregateSignal)
3. Tests covering: load, validate (positive + negative cases), entity-local logic.
4. One golden example YAML in `schemas/examples/<entity>.yaml` that round-trips through the loader.

## What is NOT in Phase A

- Detection skills (`/detect-stack`, `/detect-use-cases`)
- Sensor generation skill (`/create-sensors`)
- Runtime (executor, signal collector, lifecycle, heal loop)
- CLI

These are Phase B work and will be decomposed separately once Phase A lands.
