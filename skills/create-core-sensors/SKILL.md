---
name: create-core-sensors
description: Generate the repo-level core sensors (parameterized primitives + environment primitives) from the stack manifest. Run /detect-stack first. No argument.
---

# /create-core-sensors

You are generating the repo-level core sensor primitives for this repository.
Core sensors have `scope: core`, carry no `use_case_id`, and live under
`.harness/sensors/core/<id>.yaml`.

## Prerequisites

`.harness/stack-manifest.yaml` must exist. If it is absent, stop and tell the
user to run `/detect-stack` first.

## What to read first

Use the Read tool to load:

1. `.harness/stack-manifest.yaml` — note `archetype`, `applicable_angles`, and
   `components[*].id` (the only valid ids for sensor-level `uses:`).
2. `<plugin-root>/schemas/core-inputs/<angle>.yaml` for each parameterized
   angle you will emit — the **baseline input floor**: the inputs the
   primitive MUST declare, with descriptions and suggested defaults.

## Two categories of core sensor

### Parameterized primitives (angle-typed, composable)

One per applicable parameterized angle: `e2e-test`, `database` (primitive id
`database-query`), `performance`, `logs`, `metrics`. Use-case sensors compose
them via `uses:` + `with:` to validate any journey variation — success,
failure (expected rejections), alternative.

Rules:
- Declare `inputs:` covering **at least** the angle's baseline floor; declare
  more when this repo's surface needs them (auth headers, tenant ids) — never
  fewer. Every input carries a `default:` so the primitive self-runs as a
  smoke test. Derive defaults from the manifest (e.g. `base_url` from the
  detected dev-server port); otherwise use the baseline's `suggested_default`.
- Reference EVERY declared input as `${{ inputs.<name> }}` in step `run` — a
  declared-but-ignored input is rejected (`unreferenced_input`).
- Declare `outputs:` re-exporting normalized results; write step outputs to
  `$HARNESS_OUTPUT` as `name=value` lines.
- Top-level `uses:` contains only `StackComponent` ids from the manifest.
- `depends_on:` lists core sensors that must start first (e.g. `run-dev`).

**Grade-and-emit contract** (every parameterized primitive):

1. An unexpected response is *data*, not a transport error. Never
   `curl --fail` — a 422 must reach grading so failure-variation use cases
   can expect it.
2. Always emit normalized `key=value` lines to stdout (`status=`, `body=`,
   `rows=`, `p95_ms=`, `matched_line=`) and mirror them to `$HARNESS_OUTPUT`.
   Use-case sensors grade these lines with their own `signal_matches` — keep
   them stable, one fact per line.
3. Grade the expectation inputs (`expect_status`, `expect_rows`,
   `p95_budget_ms`, `pattern`/`anti_pattern`, `predicate`) inside the
   primitive: met → exit 0; unmet → print one line
   `expectation-unmet expected=<e> got=<g>` and exit 1, covered by a fail
   matcher with a generic `heal_hint`.

The canonical full shape — inputs, outputs, matcher, and the graded curl
run — is `schemas/examples/sensor/core-e2e-primitive.yaml`. Follow it
closely; the e2e-test floor is:

```yaml
inputs:
  base_url:      { required: true,  default: "http://localhost:8080" }  # from manifest
  method:        { required: true,  default: GET }
  path:          { required: true,  default: /health_check/ready }
  query:         { required: false, default: "" }
  headers:       { required: false, default: "" }   # newline-separated "K: v"
  body:          { required: false, default: "" }   # fixture payload path
  expect_status: { required: true,  default: 2xx }  # class (2xx/4xx) or exact (422)
  timeout:       { required: false, default: "10" } # seconds
```

### Environment primitives (lifecycle, no inputs)

Environment primitives set up or tear down runtime infrastructure
(`run-dev`, `datastore`). They take **no** `inputs:` and produce no
composable `outputs:`. Their steps contain only `run:` commands.

A `kind: observational` + `scope: core` environment primitive is a **shared
service**: the runtime starts exactly one instance, reference-counts it, and
keeps it alive while any sensor is attached. Other sensors attach to its live
signal stream (they do not spawn their own copy). Two consequences:

- **Readiness key is reserved.** The service manager blocks each attaching
  sensor until the service emits an observation whose `key` is exactly
  `ready`. Your readiness matcher **must** use `key: ready` (not `api-ready`
  or similar) or consumers will never unblock.
- **Emit a firehose matcher.** Add a catch-all matcher (e.g. `key: log-line`,
  `pattern: ".+"`, `verdict: pass`) so attaching sensors can grep each server
  log line via `matched_line`.

## Command shape MUST match `kind` + `output_type`

| kind | output_type | Command shape |
|------|-------------|---------------|
| `assertion` | `single-shot` | Runs, **exits**, verdict from exit code / parsed output. A detached or one-shot command (`docker compose up -d` + a `wait-ready` loop that `exit`s, a single `curl`, `go test`, …) is correct here. |
| `observational` | `stream` | A **long-running, foreground (non-detached) command** that blocks and streams its output for as long as the watcher lives. It must **not** detach or exit on its own. |

So a `run-dev` declared `kind: observational` + `output_type: stream` MUST run
the dev stack in the **foreground** — e.g. `docker compose --profile dev up`
(NO `-d`) as its single, blocking step. Never emit `up -d` + a `wait-ready`
loop for an observational/stream sensor: that is the assertion/single-shot
shape and contradicts the declared semantics.

### signal_matches (all sensors)

Every sensor MAY declare `signal_matches: [{ key, pattern, verdict?, confidence?, expected?, heal_hint? }]`.
Each regex (Go RE2 — no backreferences/lookahead/lookbehind) is tested against
every stdout/stderr line; a match emits a Signal with the matcher's `verdict`
(default pass) and named capture groups `(?P<name>…)` as evidence.
`expected: true` (pass matchers only) means the key must be observed at least
once or the run is incomplete (fail).

Derive patterns from the logging library in `stack-manifest.yaml`:
- Anchor on individual fields; do NOT rely on JSON key order or bridge fields with `.*`.
- Prefer one matcher per outcome (a pass matcher for 2xx, a fail matcher for 5xx).
- For fail/warn matchers, provide a `heal_hint: {summary, rationale}`.

Example shape (environment primitive, observational/stream):

```yaml
id: run-dev
scope: core
angle: environment
kind: observational
nature: computational
output_type: stream
uses: [docker-compose]
signal_matches:
  - { key: ready,         pattern: "api ready|listening on", verdict: pass, expected: true }   # reserved readiness key
  - { key: log-line,      pattern: ".+",                     verdict: pass }                    # firehose
  - { key: startup-error, pattern: "Error|error|fatal",      verdict: fail, heal_hint: { summary: "Service failed to start", rationale: "Check container logs for the failing service." } }
steps:
  - id: up
    run: |
      docker compose --profile dev up   # foreground, blocking — NO -d
```

## What to emit

For each applicable primitive (driven by `applicable_angles` and `archetype`),
emit one YAML file matching `schemas/sensor.yaml`: `schema_version: 1.0.0`,
canonical `id` (`run-dev`, `e2e-test`, `database-query`, …), `scope: core`,
no `use_case_id`, the `angle`/`kind`/`nature`/`output_type` for the category,
grounded `uses:`, and `signal_matches`.

## How to validate each sensor

> **Plugin users:** `<plugin-root>` is the directory two levels above this skill file.
> Typical path after marketplace install: `~/.claude/plugins/lastro-harness/`.

```bash
<plugin-root>/scripts/harness-tools.sh create-core-sensors --file /tmp/<sensor-id>.yaml --harness-dir .harness
```

## Exit code contract

| Exit | Meaning | Action |
|------|---------|--------|
| 0 | Success | Done with this write |
| 2 | Validation failure | Read JSON error from stdout; fix YAML; retry (cap 3) |
| 1 | Script-level error | Read stderr; surface to user; stop |

Common `kind` values on exit 2:

- `grounding` — top-level `uses:` contains a component id not in the stack manifest.
- `step_resolvability` — a run-step invokes a command not installed on this machine, or a
  `make` target missing from the repo Makefile. Switch to a stack-native tool or a
  self-bootstrapping form (`go run <module>@latest`, `npx --yes <pkg>`).
- `incomplete_input_surface` — the primitive misses part of its angle's baseline floor;
  declare every input from `schemas/core-inputs/<angle>.yaml`, each with a `default`.
- `unreferenced_input` — a declared input is never referenced as `${{ inputs.<name> }}`
  in any step; bind it in the run script or remove it.
- `missing_dependency` — the stack manifest is not on disk.
- `schema_violation` — a required field is missing or wrong shape.
- `unknown_enum_value` — `kind`, `nature`, `scope`, or `output_type` is invalid.
- `scope_violation` — the sensor's `scope` is not `core`, or `use_case_id` is set.

## Coverage check

After writing all sensors, list `.harness/sensors/core/` and confirm that each
expected primitive is present. Emit any missing primitive before finishing.
