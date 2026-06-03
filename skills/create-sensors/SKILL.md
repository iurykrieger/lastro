---
name: create-sensors
description: Generate one sensor per applicable validation angle for a given use case. Run /detect-stack and /detect-use-cases first.
---

# /create-sensors `<use-case-id>`

You are generating one sensor YAML per applicable validation angle for the specified use case.

## Prerequisites

Both of the following must exist before you proceed:

- `.harness/stack-manifest.yaml`
- `.harness/use-cases/<use-case-id>.yaml`

If either is absent, stop and tell the user which command to run first (`/detect-stack` or
`/detect-use-cases`).

## What to read first

Use the Read tool to load:

1. `.harness/stack-manifest.yaml` — note `applicable_angles` (the list of angles to cover) and
   `components[*].id` (the only valid ids for sensor-level `uses:`).
2. `.harness/use-cases/<use-case-id>.yaml` — note the use case `id` and its `fixture_ids`
   (valid ids for `${{ fixtures.<id> }}` interpolation in step `run` and `with` values).
3. `.harness/sensors/core/` (optional) — list available core primitives; their `id` values are
   the only valid targets for step-level `uses:` in a uses-step.

## What to emit

One sensor per angle in `applicable_angles`. For each sensor, write a YAML file matching
`schemas/sensor.yaml`:

- `schema_version: 1.0.0`
- `id` — lowercase, hyphenated (`^[a-z][a-z0-9-]*$`); convention: `s-<use-case-id>-<angle>`.
- `use_case_id: <use-case-id>`
- `angle: <angle>` — must be one of the values from `applicable_angles`.
- `kind` — `assertion` or `observational` (see `schemas/enums/sensor-kinds.yaml`).
- `nature` — `computational` or `inferential` (see `schemas/enums/sensor-natures.yaml`).
- `output_type` — `single-shot` or `stream` (see `schemas/enums/signal-output-types.yaml`).
- `uses: [...]` — **top-level only**: ids from `stack-manifest.components[*].id`. Pick the subset
  this sensor needs. Do NOT include fixture ids here.
- `steps:` — each step is EITHER a run-step or a uses-step:
  - run-step: `{ id, run }`. Reference fixtures via `${{ fixtures.<id> }}` and prior step outputs
    via `${{ steps.<id>.outputs.<name> }}` inside `run`.
  - uses-step: `{ id, uses: <core-primitive-id>, with: { <input>: <value> } }`. Bind every
    `required` input of the primitive. A `with` value may be a fixture ref
    (`${{ fixtures.<id> }}`) or a prior step output (`${{ steps.<id>.outputs.<name> }}`).
- Do NOT put fixture ids in step `uses:` — `uses:` now names a core primitive to compose.

See `schemas/examples/sensor/*.yaml` for shape examples per angle and kind.

### Composing core primitives + demanding inputs

A use-case sensor composes a core primitive via `uses:` + `with:`. Bind the inputs the
use case needs (e.g. `headers` for an authenticated request). If the core primitive does
NOT expose a required input, ADD that input to the core sensor's YAML with a
backward-compatible `default`/`required: false`, then bind it — the core evolves to
satisfy the use case. Derive required auth/merchant headers and signal_matches regexes
from the use case's preconditions and the stack manifest's logging library.

## How to write each sensor

```bash
go run ./skills/create-sensors/scripts/ --file /tmp/<sensor-id>.yaml --harness-dir .harness
```

## Exit code contract

| Exit | Meaning | Action |
|------|---------|--------|
| 0 | Success | Done with this write |
| 2 | Validation failure | Read JSON error from stdout; fix YAML; retry (cap 3) |
| 1 | Script-level error | Read stderr; surface to user; stop |

**Exit 2 JSON shape:**
```json
{"kind":"grounding","entity_type":"sensor","path":"...","value":"...","expected":"...","message":"..."}
```

Common `kind` values on exit 2:

- `grounding` — top-level `uses:` contains a component id not in the stack manifest.
- `fixture_binding` — a step's `uses:` references a fixture not owned by this use case.
- `angle_not_applicable` — sensor's `angle` is not in `applicable_angles`.
- `missing_dependency` — the use case or stack manifest is not on disk.
- `schema_violation` — a required field is missing or wrong shape.
- `unknown_enum_value` — `kind`, `nature`, or `output_type` is invalid.

## Coverage check

After writing all sensors, list `.harness/sensors/` and confirm that exactly one sensor file
exists per angle in `applicable_angles`. If any angle is missing, emit and write its sensor.
