# Schema Freeze — Design Spec

> Sequential gate before Phase A of the harness framework. Locks the cross-entity YAML contract so parallel entity chunks can proceed without drift.
>
> Source plan: [`docs/harness-framework/plan.md`](../../harness-framework/plan.md) §4 (Schemas), §3 (Fixed Enums)
> Source chunk: [`docs/harness-framework/00-schema-freeze.md`](../../harness-framework/00-schema-freeze.md)

## 1. Purpose and scope

The schema-freeze gate produces only YAML schema artifacts. No Go code, no validators, no runtime, no CLI. Its sole job is to freeze the cross-entity contract — field names, ids, references, version policy — so the nine parallel Phase A entity chunks (E1–E9) read from one source of truth.

In scope:

- 8 schema documents under `schemas/` (7 entity schemas + `entry-point.yaml` which defines an embedded type referenced by `use-case.yaml`)
- 8 enum YAML documents under `schemas/enums/` (structured data + meta-schema)
- 44 worked examples under `schemas/examples/<entity>/`
- `schemas/README.md` documenting the cross-reference catalog and conventions
- A local validation script that proves every example passes its schema

Out of scope:

- Go code of any kind (typed structs, loaders, validators)
- Integrity validation across files (existence of referenced ids)
- Skill or runtime implementation
- CLI

## 2. Decisions

| Dimension | Choice |
|---|---|
| File format | YAML for both schemas and instances |
| Schema standard | JSON Schema 2020-12 embedded in YAML |
| Reference Go validator (for Phase A) | `santhosh-tekuri/jsonschema` |
| Entity `id` form | Slug only, regex `^[a-z][a-z0-9-]*$`, max 128 chars |
| `schema_version` | Independent per file, semver |
| Cross-references | Form only (string + regex); existence validated at runtime |
| Example coverage | Exhaustive over each entity's internal dimensions, not Cartesian across enums |
| Inconsistency in source doc | The freeze doc says "5 fixed-enum YAML files" but plan.md §3 defines 6 plus two more promoted during design (`verdicts`, `termination-reasons`); the correct count is **8**. |
| EntryPoint | Treated as a discriminated-union type embedded in UseCase, defined in `entry-point.yaml` and `$ref`d from `use-case.yaml` — not a standalone entity with its own `id`/`schema_version` |

## 3. File inventory

```
schemas/
├── README.md
├── stack-component.yaml          ┐
├── entry-point.yaml              │
├── use-case.yaml                 │
├── fixture.yaml                  │  8 entity schemas
├── sensor.yaml                   │
├── signal.yaml                   │
├── aggregate-signal.yaml         │
├── validation-policy.yaml        ┘
├── enums/
│   ├── _meta.yaml                ── meta-schema describing the enum file shape
│   ├── validation-angles.yaml    ┐
│   ├── archetypes.yaml           │
│   ├── sensor-kinds.yaml         │
│   ├── sensor-natures.yaml       │  8 enums
│   ├── signal-output-types.yaml  │
│   ├── fixture-roles.yaml        │
│   ├── verdicts.yaml             │
│   └── termination-reasons.yaml  ┘
└── examples/
    ├── stack-component/          (6 files — one per kind)
    ├── entry-point/              (9 files — one per archetype)
    ├── use-case/                 (9 files — one per archetype_scope)
    ├── fixture/                  (3 files — one per role)
    ├── sensor/                   (6 files — realistic kind × nature × output_type)
    ├── signal/                   (3 files — one per verdict)
    ├── aggregate-signal/         (5 files — representative termination combinations)
    └── validation-policy/        (3 files — one per scope)
```

Counts: 1 README + 8 entity schemas + 1 enum meta + 8 enums + 44 examples = **62 files**, plus the validation script.

## 4. Entity schema shape

Each file under `schemas/<entity>.yaml` is one JSON Schema 2020-12 document written as YAML. Common envelope:

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://lastro.dev/harness/schemas/<entity>.yaml"
title: <EntityName>
description: |
  <prose description>

type: object
required: [schema_version, id, ...]
additionalProperties: false

properties:
  schema_version:
    type: string
    pattern: "^\\d+\\.\\d+\\.\\d+$"
  id:
    $ref: "#/$defs/Id"
  # ... entity-specific fields

$defs:
  Id:
    type: string
    pattern: "^[a-z][a-z0-9-]*$"
    minLength: 1
    maxLength: 128
  # ... entity-local subtypes
```

Conventions applied uniformly:

1. `additionalProperties: false` at the root and on every nested object — undocumented fields are validation errors.
2. `$defs.Id` is duplicated locally in each schema. No `common.yaml`; each schema is self-contained.
3. Where a field is constrained to an enum value, the value list appears **inline** in the schema's `enum:` clause. The `description` of that property points to the canonical file in `enums/`.
4. `schema_version` is always required, semver-shaped, and not enumerated (each entity evolves independently).
5. No `default:` values in any field. The gate freezes shape, not policy.
6. Cross-references are typed as `$ref: "#/$defs/Id"` — purely form, no existence check.

### 4.1 Entity-by-entity surface

The fields listed below are required by the schema. Optional fields appear in the source `plan.md §4` and are carried over identically; this spec doesn't re-enumerate them.

- **`stack-component.yaml`** — `schema_version`, `id`, `kind`, `name`, `version`, `capabilities[]`, `detection_evidence[]`. `kind` enum inline.
- **`entry-point.yaml`** — defines `EntryPoint` as a discriminated union over `archetype`. Top-level: `id`, `archetype`, `spec`. `spec` is one of 9 sub-shapes keyed by `archetype` (using JSON Schema `oneOf` with an `if/then` per archetype). No top-level `schema_version` because EntryPoint is embedded.
- **`use-case.yaml`** — `schema_version`, `id`, `title`, `archetype_scope[]`, `entry_points[]` (each `$ref`ing `entry-point.yaml`), `given[]`, `when[]`, `then[]`, `source_refs[]`, `fixture_ids[]`.
- **`fixture.yaml`** — `schema_version`, `id`, `use_case_id`, `role`, `content_type`, `payload`, `binding`, `source_refs[]`. `role` enum inline.
- **`sensor.yaml`** — `schema_version`, `id`, `use_case_id`, `angle`, `kind`, `nature`, `output_type`, `uses[]`, `depends_on[]`, `steps[]`. All four enums inline. `steps[].uses[]` accepts ids only.
- **`signal.yaml`** — `schema_version`, `sensor_id`, `use_case_id`, `angle`, `emitted_at`, `verdict`, `confidence`, `evidence`, `heal_hint`. `verdict` enum inline (sourced from `enums/verdicts.yaml`). `heal_hint` required when `verdict == fail` (enforced via `if/then`).
- **`aggregate-signal.yaml`** — `schema_version`, `type` (constant `"aggregate"`), `sensor_id`, `use_case_id`, `angle`, `started_at`, `ended_at`, `termination_reason`, `verdict`, `confidence`, `rollup`, `completeness`, `heal_hint`. `termination_reason` and `verdict` enums inline. Same `if/then` for `heal_hint`.
- **`validation-policy.yaml`** — `schema_version`, `scope`, `inherits_from`, `per_archetype` (map keyed by archetype id, each value carrying `obligatory_angles[]`, `optional_angles[]`, `disabled_angles[]`).

## 5. Enum file shape

Enum files are structured data, not JSON Schemas. They serve two roles: (a) canonical source of truth for the enum's values, (b) carrier of the semantic metadata for each value.

```yaml
schema_version: <semver>
title: <EnumName>
description: |
  <prose>

values:
  - id: <slug>
    purpose: "<short phrase>"
    # extra fields specific to the enum (see table below)
```

| File | Extra fields per value |
|---|---|
| `validation-angles.yaml` | — |
| `archetypes.yaml` | `applicable_angles: [<validation-angle-id>, ...]` |
| `sensor-kinds.yaml` | `lifecycle_notes: string` |
| `sensor-natures.yaml` | `confidence_default: number (0..1)` |
| `signal-output-types.yaml` | — |
| `fixture-roles.yaml` | — |
| `verdicts.yaml` | — |
| `termination-reasons.yaml` | — |

**Meta-schema (`schemas/enums/_meta.yaml`)** is a JSON Schema 2020-12 document that constrains the shape above: required keys, regex on `values[].id`, semver pattern on `schema_version`, and forbids `additionalProperties` at each level. Every enum file validates against `_meta.yaml`; the meta itself is the only JSON Schema living under `enums/`.

**Single load-bearing fact in enum metadata:** `archetypes.yaml` carries `applicable_angles` per archetype. This is the canonical archetype × angle matrix. All other enum metadata is informational only.

**Drift contract:** the inline `enum:` lists in entity schemas duplicate `enums/<x>.yaml#/values[*].id`. The README declares enum files canonical; reconciliation between them and inline copies is a Phase A concern (likely via codegen or a CI consistency test).

## 6. Cross-reference catalog

Every cross-entity reference is a `string` matching `^[a-z][a-z0-9-]*$`. Schemas validate form only.

| Source (entity.field) | Target | Cardinality |
|---|---|---|
| `Sensor.use_case_id` | UseCase | 1 |
| `Sensor.uses[]` | StackComponent | 0..N |
| `Sensor.depends_on[]` | Sensor | 0..N |
| `Sensor.steps[].uses[]` | Fixture | 0..N (must belong to `Sensor.use_case_id` — runtime invariant) |
| `Signal.sensor_id` | Sensor | 1 |
| `Signal.use_case_id` | UseCase | 1 |
| `Signal.evidence.fixture_id` | Fixture | 0..1 |
| `AggregateSignal.sensor_id` | Sensor | 1 |
| `AggregateSignal.use_case_id` | UseCase | 1 |
| `Fixture.use_case_id` | UseCase | 1 |
| `UseCase.fixture_ids[]` | Fixture | 1..N |
| `UseCase.entry_points[].id` | (local — unique within the UseCase) | — |
| `UseCase.entry_points[].archetype` | enum: archetypes | 1 |
| `UseCase.archetype_scope[]` | enum: archetypes | 1..N |
| `ValidationPolicy.inherits_from` | ValidationPolicy | 0..1 |

Enum-valued fields validate against the inline `enum:` clause; the enum file is the canonical source if a mismatch arises.

## 7. Examples

44 example files prove that each schema accepts a realistic payload. Exhaustive over each entity's internal dimensions; not Cartesian across enums.

Layout: `schemas/examples/<entity>/<scenario>.yaml`. Scenarios listed in §3.

Conventions:

1. Every example is a complete, schema-valid payload.
2. Example ids are descriptive slugs (`create-order-endpoint`, `order-input-fixture`).
3. Cross-references inside an example use ids that appear in sibling examples — schema doesn't check this; it's for human readability of the graph.
4. Each file opens with 2–3 comment lines describing the scenario.
5. The pair `observational + single-shot` is excluded from `sensor/` examples (conceptual contradiction). The schema does **not** forbid it (avoids complex `if/then/else`); the contradiction is documented in the README only.

## 8. README contents (`schemas/README.md`)

Ten sections:

1. Purpose — what these files are and aren't
2. Layout — annotated tree
3. Conventions — JSON Schema 2020-12, id form, schema_version policy, additionalProperties, enum duplication strategy
4. Cross-reference catalog — table from §6, marked "form-only validation"
5. Enums — list of the 8 files, how to add/remove a value, drift contract
6. Examples — coverage policy, validation command, comment convention
7. Validation — exact commands for `ajv` and the planned Go validator
8. Versioning — how to bump a schema, how to bump an enum, `framework.lock` implications
9. What this gate does NOT do — explicit out-of-scope list
10. Open items deferred to Phase A — items below

## 9. Items deferred to Phase A

These were raised during brainstorming but are not blockers for the gate. Phase A chunks own them:

- Canonical hashing rule (if entity chunks decide to derive ids from content hashes; the gate only enforces form).
- Reconciliation mechanism between inline `enum:` clauses in entity schemas and `enums/<x>.yaml#/values[*].id` (codegen vs CI consistency test).
- Whether Go loaders treat unknown fields strictly (schema already says `additionalProperties: false`, but loaders may choose to error or warn).
- Migration strategy when an enum value is removed or renamed.

## 10. Acceptance criteria

The gate PR is complete when:

1. All 8 entity schemas exist in `schemas/*.yaml`, are syntactically valid JSON Schema 2020-12.
2. All 8 enums exist in `schemas/enums/*.yaml` matching the §5 shape.
3. `schemas/enums/_meta.yaml` exists and validates each enum file.
4. 44 examples exist in `schemas/examples/<entity>/<scenario>.yaml`, each passing its schema.
5. `schemas/README.md` covers the 10 sections in §8.
6. A `scripts/validate-schemas.sh` (or equivalent) script:
   - Confirms each schema is itself a valid JSON Schema 2020-12
   - Validates each example against its entity's schema
   - Validates each enum file against `_meta.yaml`
   - Exits zero on success
7. No `.go` files are introduced. No files outside `schemas/` and `scripts/`.

## 11. Risks and trade-offs

- **Enum duplication risk.** Inline enum lists in entity schemas can diverge from `enums/*.yaml`. Mitigated by README contract + Phase A drift test, but not by the gate itself.
- **Permissive sensor schema.** `observational + single-shot` is allowed by the schema even though it's semantically nonsensical. Reduces JSON Schema complexity at the cost of looser validation; documented, not enforced.
- **Example volume (44 files).** Larger gate than the original doc implied (one example per entity). Trade-off accepted: exhaustive coverage gives Phase A chunks a richer fixture set to test against.
- **No referential integrity.** The gate freezes shape only. A Sensor referencing a non-existent UseCase passes the gate's validation. Runtime / Phase A loaders must check existence.
