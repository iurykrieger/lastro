# Harness Framework — Parallel Implementation Decomposition

The framework plan in [`plan.md`](plan.md) is too large for one implementation pass. This directory decomposes it into parallel-implementable chunks, each scoped to a single entity or contract, so each can be brainstormed and built independently without an implementing agent having to reason about the whole framework at once.

## Decomposition strategy

Two phases:

- **Phase A — Entity foundation** (✓ complete). One chunk per entity. Each chunk owns its YAML schema, Go type, validator, entity-local pure logic, and tests.
- **Phase B — Verbs (runtime + skills + CLI)**. Cross-entity work. Decomposed below into 8 chunks across 4 layers.

### Sequential gate (Phase A)

[`00-schema-freeze.md`](00-schema-freeze.md) — wrote all entity YAML schema files together in one PR before fanning Phase A out. Locks the cross-entity contract (field names, ids, references) so parallel entity work doesn't drift.

## Phase A — Entity chunks (✓ complete)

Each doc went through `/brainstorming` → design doc → `/writing-plans` → implementation plan → merge.

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

### Phase A dependency graph

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

## Phase B — Verbs (runtime + skills + CLI)

Phase B implements the verbs that consume the Phase A entities: the runtime that executes sensors, the skills that produce them, the CLI that orchestrates user interaction, and the dogfood integration that proves the whole thing works.

### Phase A lesson applied

Phase A claimed "everyone parallel after the schema-freeze gate," but in practice several chunks discovered implicit dependencies on each other's work mid-flight. Phase B's decomposition makes parallel/sequential dependencies **explicit per chunk** (each doc names what it can run alongside and what it must wait for), and **every chunk doc opens with a mandatory branch-from-fresh-`main` instruction** so worktrees never start from stale or unmerged trees.

### Phase B chunks (8 chunks, 4 layers)

| ID | Chunk | Doc | Layer |
|---|---|---|---|
| B1 | Runtime primitives | [`B1-runtime-primitives.md`](B1-runtime-primitives.md) | Runtime |
| B2 | Composed runtime | [`B2-composed-runtime.md`](B2-composed-runtime.md) | Runtime |
| B3 | Executor & lifecycle | [`B3-executor-lifecycle.md`](B3-executor-lifecycle.md) | Runtime |
| B4 | Heal loop | [`B4-heal-loop.md`](B4-heal-loop.md) | Runtime |
| B5 | Detection & generation skills | [`B5-detection-generation.md`](B5-detection-generation.md) | Skills (producer) |
| B6 | Skill wrappers (execution + heal) | [`B6-skill-wrappers.md`](B6-skill-wrappers.md) | Skills (consumer) |
| B7 | CLI (`cmd/harness`) | [`B7-cli.md`](B7-cli.md) | CLI |
| B8 | Examples & dogfood self-validation | [`B8-examples-dogfood.md`](B8-examples-dogfood.md) | Integration |

### Phase B dependency graph

```
        Phase A complete (✓)
                 |
   +-------------+--------------+
   |                            |
   B1                          B5
   runtime primitives          detection + generation
   (template, policy,          (/detect-stack,
    resolver, signalColl.)      /detect-use-cases,
   |                            /create-sensors)
   B2                          |
   composed runtime            |  (B5 runs in parallel
   (fixtureBinder,             |   with B1-B4 and B6/B7;
    sensor + use-case agg.)    |   only consumes Phase A)
   |                           |
   B3                          |
   executor + lifecycle        |
   |                           |
   +-----+----+----------------+
         |    |
        B4   B6   B7
        heal skill CLI
        loop wrap- (cmd/harness)
             pers
         \   |   /
          \  |  /
           \ | /
            B8
       examples + dogfood
       (integration — needs all of B1-B7)
```

### Phase B parallelism summary

The key parallel opportunities and their gates:

| Can run in parallel | Gate |
|---|---|
| B1 ∥ B5 | both start immediately after Phase A |
| B2 ∥ B5 | B2 needs B1; B5 is independent |
| B3 ∥ B5 | B3 needs B1+B2; B5 is independent |
| B4 ∥ B5 ∥ B6 ∥ B7 | all four can run once their respective deps land |
| B6's `/run-sensor`, `/start-sensor`, `/stop-sensor`, `/validate-use-case` | unblocked at B2+B3+B5; the fifth sub-skill `/heal` waits for B4 |
| B7 ∥ B6 | sibling surfaces over the same Go runtime; can land independently |

**Sequential gates:**
- B2 must run after B1.
- B3 must run after B1 + B2.
- B4 must run after B2 + B3.
- B6 must run after B2 + B3 (full surface needs B4 too).
- B7 must run after B2 + B3 + B5 (full surface needs B4 too).
- B8 must run after B6 + B7 (integration).

### Per-chunk contract for Phase B

Every Phase B chunk doc must:

1. Open with the **branch-from-fresh-main instruction** verbatim:
   ```bash
   git fetch origin
   git checkout -b feat/bN-<short-name> origin/main
   ```
2. State **explicit parallelism**: "Can run in parallel with", "Must run after", "Blocks".
3. Scope statement (in/out) — like Phase A docs.
4. Inputs/Outputs table.
5. Dependencies (Phase A entities + other B chunks).
6. Open questions for `/brainstorming`.
7. Deliverable acceptance.

The chunk doc is a brainstorming starter — it goes through the same `/brainstorming` → design doc → `/writing-plans` → implementation plan → merge flow as Phase A.

## Per-chunk contract (Phase A — historical reference)

Every entity chunk in Phase A produced:

1. The YAML schema file under `schemas/` (already written in the schema-freeze gate; the chunk validated and refined it).
2. A Go package under `internal/<entity>/` with:
   - Typed structs with YAML tags
   - A loader that deserializes + validates against the schema
   - Any entity-local pure logic (e.g., template resolver for UseCase, DAG resolver for Sensor, rollup for AggregateSignal)
3. Tests covering: load, validate (positive + negative cases), entity-local logic.
4. One golden example YAML in `schemas/examples/<entity>.yaml` that round-trips through the loader.
