# Schemas — Harness Framework Cross-Entity Contract

This directory is the **sequential gate** before Phase A of the harness
framework. It locks the YAML contract that every parallel entity chunk
(E1–E9) reads from.

## 1. Purpose

These files define the *shape* of every artifact the framework deals with:
StackComponents, EntryPoints, UseCases, Fixtures, Sensors, Signals,
AggregateSignals, and ValidationPolicies. They do **not** validate that
referenced ids actually exist — that is a runtime concern.

## 2. Layout

```
schemas/
├── README.md
├── stack-component.yaml          ┐
├── entry-point.yaml              │
├── use-case.yaml                 │
├── fixture.yaml                  │  8 schemas (7 entities + entry-point embedded type)
├── sensor.yaml                   │
├── signal.yaml                   │
├── aggregate-signal.yaml         │
├── validation-policy.yaml        ┘
├── enums/
│   ├── _meta.yaml                ── meta-schema for enum files
│   ├── validation-angles.yaml    ┐
│   ├── archetypes.yaml           │
│   ├── sensor-kinds.yaml         │  8 enums
│   ├── sensor-natures.yaml       │
│   ├── signal-output-types.yaml  │
│   ├── fixture-roles.yaml        │
│   ├── verdicts.yaml             │
│   └── termination-reasons.yaml  ┘
└── examples/
    └── <entity>/<scenario>.yaml  ── 44 worked examples
```

## 3. Conventions

- **Standard:** JSON Schema 2020-12, embedded in YAML.
- **Id form:** lowercase slug, regex `^[a-z][a-z0-9-]*$`, max 128 chars.
- **`schema_version`:** required on every entity (and every enum file).
  Semver. Versions evolve **independently** per file.
- **`additionalProperties: false`** at every object level. Undocumented
  fields are validation errors.
- **Cross-references** are typed as strings matching the id regex. No
  cross-file `$ref` for ids (except where one schema embeds another's
  type — UseCase `$ref`s EntryPoint).
- **Enums** appear inline (`enum: [...]`) in entity schemas; the
  canonical source is the matching file in `enums/`. See §5.
- **No `default:` values** in any schema. The gate freezes shape, not policy.

## 4. Cross-reference catalog

Every cross-entity reference is a string matching `^[a-z][a-z0-9-]*$`.
Schemas validate form only; existence is a **runtime** concern.

| Source | Target | Cardinality |
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

## 5. Enums

The eight files under `enums/` carry the canonical lists for the framework's
fixed enums. Each is structured data, not a JSON Schema; they are
validated by `enums/_meta.yaml`.

| File | Purpose |
|---|---|
| `validation-angles.yaml` | The 10 angles a sensor can validate |
| `archetypes.yaml` | The 9 application archetypes + their applicable_angles matrix |
| `sensor-kinds.yaml` | `assertion`, `observational` |
| `sensor-natures.yaml` | `computational`, `inferential` |
| `signal-output-types.yaml` | `single-shot`, `stream` |
| `fixture-roles.yaml` | `input`, `expected-output`, `expected-side-effect` |
| `verdicts.yaml` | `pass`, `fail`, `inconclusive` |
| `termination-reasons.yaml` | `completed`, `stopped`, `timeout`, `error` |

### Adding or removing a value

1. Edit the enum file.
2. Bump its `schema_version` (additive minor; removal major).
3. Mirror the change in any entity schema's inline `enum:` clause.
4. Run `go run ./cmd/validate-schemas`. All examples must still pass.

**Drift contract:** the inline `enum:` lists in entity schemas duplicate
`enums/<x>.yaml#/values[*].id`. The enum file is canonical. Reconciling
inline copies with the canonical source is a Phase A concern (likely a
codegen step or a CI consistency test).

### Sole load-bearing enum metadata

`archetypes.yaml` carries `applicable_angles` per archetype — the
canonical archetype × angle matrix. This is the **only** enum metadata
the framework reads programmatically. All other `purpose` /
`lifecycle_notes` / `confidence_default` fields are informational.

## 6. Examples

`examples/<entity>/<scenario>.yaml` carries 44 worked examples (one or more
per entity, covering each entity's internal dimensions). Each file:

- Opens with 2–3 comment lines describing the scenario
- Uses descriptive slug ids (`create-order-endpoint`, `order-input-fixture`)
- Cross-references ids found in sibling examples (purely for human
  readability — the schema does not check existence)

The pair `observational + single-shot` is excluded from `sensor/`
examples because it is a conceptual contradiction (observational implies
a stream). The schema **does not** enforce this — keeping it permissive
avoids brittle `if/then/else` constructs.

**Dangling cross-references in use-case examples.** Eight of nine use-case
examples list `fixture_ids` referencing fixtures that are not present in
`examples/fixture/` (the gate ships only one fixture per role: input,
expected-output, expected-side-effect). This is intentional — the gate
demonstrates the *shape* of fixture references, not a closed fixture
inventory. Full fixture sets are produced by `/detect-use-cases` at runtime,
not shipped in the gate.

## 7. Validation

The gate ships a Go validator under `cmd/validate-schemas`.

```
go run ./cmd/validate-schemas
```

Behavior:

1. Loads every entity schema and confirms it is itself a valid JSON
   Schema 2020-12.
2. Loads `enums/_meta.yaml` and validates each enum file against it.
3. Walks `examples/<entity>/*.yaml` and validates each against the
   corresponding entity schema.
4. Exits zero on success; non-zero with a list of errors otherwise.

Dependencies: `github.com/santhosh-tekuri/jsonschema/v6` (the standard
2020-12 implementation Phase A loaders will also use) and
`sigs.k8s.io/yaml` for YAML→JSON conversion.

## 8. Versioning policy

Each schema bumps **independently**:

- **Patch bump** (`1.0.0 → 1.0.1`): clarifying description text only.
- **Minor bump** (`1.0.0 → 1.1.0`): adding optional fields.
- **Major bump** (`1.0.0 → 2.0.0`): removing fields, tightening constraints,
  changing required field set, or changing field types.

Enum files follow the same rules: additive value → minor; removal or
rename → major.

Phase B's `framework.lock` will record the version of each schema and
each enum in use. The gate itself does not produce a `framework.lock`.

## 9. What this gate does NOT do

- **Does not generate Go types.** Phase A entity chunks own typed
  structs and per-entity loaders.
- **Does not enforce referential integrity.** A Sensor referencing a
  non-existent UseCase passes the gate's validation. Runtime loaders
  must check existence.
- **Does not provide a CLI.** Only `cmd/validate-schemas` exists; the
  `harness` CLI is Phase B.
- **Does not implement skills or runtime logic.**

## 10. Items deferred to Phase A

- Canonical content-hashing rule for ids (if entity chunks decide to
  derive ids from content hashes — the gate only enforces form).
- Reconciliation mechanism between inline `enum:` clauses and
  `enums/*.yaml` (codegen vs CI consistency test).
- Whether Go loaders treat unknown fields strictly (schema already
  says `additionalProperties: false`, but loaders may add their own
  policy).
- Migration strategy when an enum value is renamed or removed.
- Schema-file rename strategy. The versioning policy in §8 covers field-level
  changes within a schema file, but not what happens when an entity schema
  itself is renamed (e.g., `stack-component.yaml` → `platform-component.yaml`).
  Every cross-reference description, every example file path, and every
  inline doc would need updating in lockstep.
