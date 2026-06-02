---
name: create-core-sensors
description: Generate the repo-level core sensors (parameterized primitives + environment primitives) from the stack manifest. Run /detect-stack first. No argument.
---

# /create-core-sensors

You are generating the repo-level core sensor primitives for this repository.
Core sensors have `scope: core`, carry no `use_case_id`, and live under
`.harness/sensors/core/<id>.yaml`.

## Prerequisites

The following must exist before you proceed:

- `.harness/stack-manifest.yaml`

If it is absent, stop and tell the user to run `/detect-stack` first.

## What to read first

Use the Read tool to load:

1. `.harness/stack-manifest.yaml` — note `archetype`, `applicable_angles`, and
   `components[*].id` (the only valid ids for sensor-level `uses:`).

## Two categories of core sensor

### Parameterized primitives (angle-typed, composable)

Parameterized primitives correspond to use-case angles that need per-invocation
inputs (`e2e-test`, `database-query`, `performance`, `logs`, `metrics`).

Rules:
- Declare `inputs:` — every input **must** carry a `default:` so the primitive
  self-runs as a smoke test with no caller configuration.
- Declare `outputs:` to re-export results that consumers can bind with `with:`.
- Reference inputs in `run` via `${{ inputs.<name> }}`.
- Write step outputs to `$HARNESS_OUTPUT` as `name=value` lines.
- `uses:` at the top level must contain only `StackComponent` ids from the
  detected stack manifest (grounding invariant).
- `depends_on:` lists the ids of core sensors that must start before this one
  (e.g., `e2e-test` depends on `run-dev`).

Example shape:

```yaml
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [curl]
depends_on: [run-dev]
inputs:
  method: { required: true, default: GET }
  path:   { required: true, default: /health_check/ready }
outputs:
  body: { from: "${{ steps.request.outputs.body }}" }
steps:
  - id: request
    run: |
      resp=$(curl --fail -sS -X "${{ inputs.method }}" "http://localhost:8080${{ inputs.path }}")
      printf 'body=%s\n' "$resp" >> "$HARNESS_OUTPUT"
```

### Environment primitives (lifecycle, no inputs)

Environment primitives set up or tear down runtime infrastructure
(`run-dev`, `datastore`). They take **no** `inputs:` and produce no
composable `outputs:`. Their steps contain only `run:` commands.

## Command shape MUST match `kind` + `output_type`

This is a hard contract — the command a sensor runs is determined by its
declared semantics, not the other way around:

| kind | output_type | Command shape |
|------|-------------|---------------|
| `assertion` | `single-shot` | Runs, **exits**, verdict from exit code / parsed output. A detached or one-shot command (`docker compose up -d` + a `wait-ready` loop that `exit`s, a single `curl`, `go test`, …) is correct here. |
| `observational` | `stream` | A **long-running, foreground (non-detached) command** that blocks and streams its output for as long as the watcher lives. It must **not** detach or exit on its own. |

So a `run-dev` declared `kind: observational` + `output_type: stream` MUST run
the dev stack in the **foreground** — e.g. `docker compose --profile dev up`
(NO `-d`) as its single, blocking step. Never emit `up -d` + a `wait-ready`
loop for an observational/stream sensor: that is the assertion/single-shot
shape and contradicts the declared semantics (the watcher would exit instead
of streaming).

### Observations for observational/stream sensors (`expected_observations`)

A `stream` sensor turns its streamed output into observation signals via
`expected_observations` — a list of `{ key, pattern }` where `pattern` is a
**regex** tested against each stdout **and** stderr line. On a match the
watcher emits an observation signal carrying `key` (and `key` feeds
completeness). Derive the keys and regexes from `.harness/stack-manifest.yaml`
(component ids, health endpoints, readiness strings). Remember that
`docker compose up` prints lifecycle status (`Container X Healthy`) to stderr.

Example shape (environment primitive, observational/stream):

```yaml
id: run-dev
scope: core
angle: environment
kind: observational
nature: computational
output_type: stream
uses: [docker-compose]
expected_observations:
  - { key: api-ready,        pattern: "api ready|listening on" }
  - { key: dynamodb-healthy, pattern: "Container .*dynamodb.*Healthy" }
steps:
  - id: up
    run: |
      cd charge-api
      docker compose --profile dev up   # foreground, blocking — NO -d
```

## What to emit

For each applicable primitive (driven by `applicable_angles` and `archetype`),
emit one YAML file matching `schemas/sensor.yaml`:

- `schema_version: 1.0.0`
- `id` — lowercase, hyphenated; use the primitive's canonical name
  (`run-dev`, `e2e-test`, `database-query`, etc.).
- `scope: core`
- No `use_case_id`.
- `angle` — the angle this primitive covers.
- `kind` — `assertion` (parameterized) or `observational` (environment lifecycle).
- `nature` — `computational` or `inferential`.
- `output_type` — `single-shot` or `stream`.
- `uses: [...]` — stack component ids only; must be a subset of manifest
  `components[*].id`.
- `expected_observations: [...]` — REQUIRED for `output_type: stream` sensors;
  `{ key, pattern }` regex matchers derived from the stack manifest (see above).
  Omit for `single-shot` sensors.

## How to validate each sensor

```bash
go run ./skills/create-core-sensors/scripts/ --file /tmp/<sensor-id>.yaml --harness-dir .harness
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
- `missing_dependency` — the stack manifest is not on disk.
- `schema_violation` — a required field is missing or wrong shape.
- `unknown_enum_value` — `kind`, `nature`, `scope`, or `output_type` is invalid.
- `scope_violation` — the sensor's `scope` is not `core`, or `use_case_id` is set.

## Coverage check

After writing all sensors, list `.harness/sensors/core/` and confirm that each
expected primitive is present. Emit any missing primitive before finishing.
