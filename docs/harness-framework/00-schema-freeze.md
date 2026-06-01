# 00 — Schema Freeze (Sequential Gate)

> Source plan: [`plan.md`](plan.md) §4 (Schemas), §3 (Fixed Enums)

This is the sequential gate that must complete before any Phase A entity chunk fans out. Its only deliverable is **YAML schema files** — no Go code, no validators, no logic. The point is to lock the cross-entity contract (field names, ids, references, versions) so every parallel entity track works from the same source of truth.

## Scope

In:
- All 9 entity YAML schema files under `schemas/`
- All 5 fixed-enum YAML files under `schemas/enums/`
- One worked example YAML per entity under `schemas/examples/` that the schema describes correctly
- A `schemas/README.md` documenting the cross-references and naming conventions

Out:
- Go code of any kind
- Validators (Phase A entity chunks own these)
- Tooling, CLI, runtime

## Files to produce

```
schemas/
├── README.md
├── stack-component.yaml
├── entry-point.yaml
├── use-case.yaml
├── fixture.yaml
├── sensor.yaml
├── signal.yaml
├── aggregate-signal.yaml
├── validation-policy.yaml
├── enums/
│   ├── validation-angles.yaml
│   ├── archetypes.yaml
│   ├── sensor-kinds.yaml
│   ├── sensor-natures.yaml
│   ├── signal-output-types.yaml
│   └── fixture-roles.yaml
└── examples/
    └── <one yaml per entity>
```

## Why this comes first

Every entity references other entities by id:
- `Sensor.use_case_id` → `UseCase.id`
- `Sensor.uses[]` → `StackComponent.id`
- `Sensor.steps[].uses[]` → `Fixture.id`
- `Signal.use_case_id` → `UseCase.id`
- `Signal.sensor_id` → `Sensor.id`
- `Fixture.use_case_id` → `UseCase.id`
- `UseCase.fixture_ids[]` → `Fixture.id`
- `UseCase.entry_points[].archetype` → `enums/archetypes.yaml`

If two parallel chunks each invent their own version of `UseCase.id` shape, the integration breaks. Freezing the YAML first eliminates this class of drift.

## Open questions for `/brainstorming`

1. **Schema language.** Plain YAML, JSON Schema in YAML, or CUE? The plan says YAML with `schema_version`. JSON Schema would let validators be generic; CUE would let cross-references be first-class. Recommendation: JSON Schema in YAML — readable, mature tooling, no exotic dependencies.
2. **Id shape.** All entity `id` fields say "stable-id" or "hash of canonical X." What's the canonical hashing rule, and is it the same across entities?
3. **schema_version per file vs per entity.** Plan §1.1 says filenames are unversioned and `schema_version` is inside. Confirm: does each YAML carry its own `schema_version`, or does the whole `schemas/` directory share one bumped together?
4. **Cross-reference resolution.** When `Sensor.uses[]` references a `StackComponent.id`, does the schema describe the *reference shape* (a string id), or does it embed the referent? Recommendation: reference shape only — embedding belongs to runtime.
5. **Examples.** One example per entity is the minimum. Should each example be a full happy-path or also include common edge cases (e.g., a `cli` use case alongside an `http-api` one)?

## Deliverable acceptance

- All 9 entity schemas + 5 enum schemas + 1 example each, valid YAML, lint-clean.
- README.md documenting: the cross-reference table above, naming conventions, schema_version policy.
- A short check that every example file passes a generic YAML/JSON-Schema validator (does not require Go).

## Change Records

Dated entries below document schema decisions that affect frozen fields after initial gate completion.
Each entry must land in this file **before** any Go or YAML changes that implement the decision.

### 2026-05-29 — core sensors (issue #24)

- **`Sensor.scope`** (string enum `core | use-case`, default `use-case`): repo-level vs use-case-bound.
- **`Sensor.use_case_id`** is now **conditional**: required when `scope: use-case`, forbidden when `scope: core`.
- **`ValidationAngle`** gains an 11th value **`environment`** (boot/datastore preconditions). It is added to the
  angle enum and `schemas/sensor.yaml`'s inline `angle` enum, but **NOT** to `ValidationPolicy.AngleList`
  (environment is a DAG precondition, never policy-graded).
- File layout: core sensors under `.harness/sensors/core/`, use-case sensors under `.harness/sensors/<usecase-id>/`.
- Backward compatibility: a sensor that omits `scope` defaults to `use-case` and still requires `use_case_id`.

### 2026-06-01 — Parameterized sensors via composition (#26)

- `sensor.yaml` step: discriminated union — a step has **either** `run` (string) **or**
  `uses` (a single primitive sensor id) + optional `with` (map[string]string). The previous
  step-level `uses: [fixture-id]` **array** is removed; fixtures are referenced by
  `${{ fixtures.<id> }}` interpolation in `run`/`with`.
- `sensor.yaml` adds optional top-level `inputs` (map of `{required?, default?, description?}`)
  and `outputs` (map of `{from, description?}`).
- Interpolation sentinel migrates repo-wide from `{{ }}` to `${{ }}`. New contexts:
  `${{ inputs.<name> }}` and `${{ steps.<id>.outputs.<name> }}`, alongside existing
  `${{ fixtures.* }}` / `${{ entry_points.* }}`.
- The executor's `{{fixtures.X}}`-in-run ban (`ErrTemplateFixtureInRun`) is lifted; fixture/input/
  step-output refs compile to env-var references (`HARNESS_FIXTURE_*`, `HARNESS_INPUT_*`,
  `HARNESS_STEPOUT_*`), never inline payloads.
