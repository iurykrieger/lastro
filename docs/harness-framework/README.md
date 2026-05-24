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

### Phase A lessons applied

1. Phase A claimed "everyone parallel after the schema-freeze gate," but several chunks discovered implicit dependencies mid-flight. Phase B's decomposition makes parallel/sequential dependencies **explicit per chunk** — each doc names what it can run alongside and what it must wait for — and **every chunk doc opens with a mandatory branch-from-fresh-`main` instruction** so worktrees never start from stale or unmerged trees.
2. Phase A delivered substantially more than the original Phase B scope assumed. The first Phase B decomposition (PR #11) treated several primitives as new work, but Phase A had already shipped them under their owning entity packages (template under `usecase/`, DAG resolver under `sensor/`, per-sensor rollup + heal-hint synthesis under `aggregate/`, streaming parser under `signal/`, two-scope policy resolution under `policy/`). The pre-rebase B1 collapsed entirely; B2/B3 shrank. The current decomposition reflects this — every chunk doc lists what Phase A already delivered before describing what remains.
3. **Skill architecture is LLM-led, not Go-led.** The whole framework idea is to let the LLM **semantically** detect the stack, use cases, and sensors based on project archetype, and then invoke deterministic scripts to **persist and run** the detected entities. The three inferential skills (`/detect-stack`, `/detect-use-cases`, `/create-sensors`) do all inference in the slash-command prompt body. Their `scripts/` packages do only schema validation, invariant checks, persistence to `.harness/`, and structured error reporting — no archetype inference, no entry-point scanning, no sensor scaffolding, no source-code analysis in Go. There is **no** `internal/detect/` or `internal/sensors/` package — Go-side validation reuses Phase A entity packages (`internal/sensor.Grounding`, etc.). The new B4 doc spells this out and includes a PR-review checklist to block any Go function that produces a detection hypothesis.

### Phase B chunks (7 chunks, 4 layers)

| ID | Chunk | Doc | Layer |
|---|---|---|---|
| B1 | Composed runtime (fixture binder + per-use-case aggregator) | [`B1-composed-runtime.md`](B1-composed-runtime.md) | Runtime |
| B2 | Executor & lifecycle | [`B2-executor-lifecycle.md`](B2-executor-lifecycle.md) | Runtime |
| B3 | Heal loop (orchestration only) | [`B3-heal-loop.md`](B3-heal-loop.md) | Runtime |
| B4 | Detection & generation skills (LLM-driven) | [`B4-detection-generation.md`](B4-detection-generation.md) | Skills (producer) |
| B5 | Skill wrappers (execution + heal) | [`B5-skill-wrappers.md`](B5-skill-wrappers.md) | Skills (consumer) |
| B6 | CLI (`cmd/harness`) | [`B6-cli.md`](B6-cli.md) | CLI |
| B7 | Examples & dogfood self-validation | [`B7-examples-dogfood.md`](B7-examples-dogfood.md) | Integration |

### Phase B dependency graph

```
        Phase A complete (✓)
                 |
   +-------------+--------------+
   |                            |
   B1                          B4
   composed runtime            detection + generation
   (fixture binder,            (LLM-driven slash commands;
    per-use-case agg.)          Go = validate + persist only)
   |                            |
   B2                           |
   executor + lifecycle         |  (B4 runs in parallel with
   |                            |   B1-B3 and B5/B6;
   B3                           |   only consumes Phase A)
   heal loop                    |
   (orchestration only)         |
   |                            |
   +------+---------------------+
          |         |
         B5        B6
         skill     CLI
         wrappers  (cmd/harness)
          \        /
           \      /
            \    /
             B7
        examples + dogfood
        (integration — needs all of B1-B6)
```

### Phase B parallelism summary

| Can run in parallel | Gate |
|---|---|
| B1 ∥ B4 | both start immediately after Phase A |
| B2 ∥ B4 | B2 needs B1; B4 is independent |
| B3 ∥ B4 ∥ B5 ∥ B6 | all four can run once their respective deps land |
| B5's `/run-sensor`, `/start-sensor`, `/stop-sensor`, `/validate-use-case` | unblocked at B1+B2; the fifth sub-skill `/heal` additionally waits for B3 |
| B6 ∥ B5 | sibling surfaces over the same Go runtime; land independently |

**Sequential gates:**
- B2 must run after B1 (fixture binder).
- B3 must run after B1 + B2 (re-validation uses both).
- B5 must run after B1 + B2 (full surface also needs B3 for `/heal`).
- B6 must run after B1 + B2 + B4 (full surface also needs B3 for `harness heal`).
- B7 must run after B5 + B6 (integration).

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
