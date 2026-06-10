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
3. `.harness/sensors/core/` — **read this before writing any step**. List every core primitive
   and note its `id`, `angle`, and declared `inputs`. A use-case sensor that composes a core
   primitive needs no raw `run:` commands at all — the core sensor carries the stack-native
   commands. Step-level `uses:` accepts only these core primitive ids.

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
- `steps:` — **prefer uses-steps; fall back to run-steps only when no core primitive covers
  the angle**:
  - uses-step **(default)**: `{ id, uses: <core-primitive-id>, with: { <input>: <value> } }`.
    Bind every `required` input of the primitive via fixture refs
    (`${{ fixtures.<id> }}`) or prior step outputs (`${{ steps.<id>.outputs.<name> }}`).
    The core sensor owns the raw commands; the use-case sensor only binds parameters.
  - run-step **(fallback)**: `{ id, run }`. Use only when `.harness/sensors/core/` contains
    no primitive for this angle. Reference fixtures via `${{ fixtures.<id> }}` and prior
    step outputs via `${{ steps.<id>.outputs.<name> }}` inside `run`.
- Do NOT put fixture ids in step `uses:` — `uses:` names a core primitive to compose.

See `schemas/examples/sensor/*.yaml` for shape examples per angle and kind.

### Command grounding rule (run-step fallback only)

Only reach for a `run:` step when no core primitive covers the angle. When you do,
the command MUST be a real executable: a tool from the stack manifest (`uses:` id),
a standard POSIX utility (`curl`, `diff`, `tail`, `cat`, `docker`), or the project's
own test runner. Never write `harness <subcommand>` — those are not installed.

If a matching core primitive exists, compose it with `uses:` + `with:` instead.

Fallback commands per angle (use only when core/ has no matching primitive):

| Angle | Fallback command pattern |
|-------|--------------------------|
| `build` | Compiler: `go build ./...`, `npm run build`, `tsc --noEmit` |
| `unit-test` | Test runner: `go test ./...`, `jest --json`, `pytest -q` |
| `code-structure` | Lint tool in `uses:`: `golangci-lint run`, `npx eslint` |
| `contracts` | Spec validator: `protoc --descriptor_set_out=/dev/null`, `npx @redocly/cli lint` |
| `logs` | **Attach to the shared service — see below.** Only tail directly (`docker logs --follow`, `tail -f`) when no observational service exists. |
| `metrics` | Scrape endpoint: `curl -sS http://localhost:9090/metrics \| grep <key>` |
| `database` | Stack CLI or probe: `aws dynamodb get-item ...`, `go run ./probe/db` |
| `performance` | Load tool in `uses:`: `hey -n 100 http://...`, `k6 run script.js` |
| `security` | Scanner in `uses:`: `gosec ./...`, `npm audit --json` |

### Attaching to a shared observational service (logs, and other stream consumers)

When `.harness/sensors/core/` contains a `kind: observational` + `scope: core`
service (e.g. `run-dev`), a `logs`-angle sensor (or any sensor that needs to
watch that service's output) **attaches** to it rather than spawning its own
server or `docker logs`. The runtime starts the service once, reference-counts
it, and feeds its live signal stream to every attaching sensor. Emit:

- `kind: observational`, `output_type: stream`.
- `depends_on: [<service-id>]` — this is what pulls the shared service into the
  use case's scheduled set; without it the service is never started.
- a step `{ id: watch, uses: <service-id> }` — the attach itself. No `run:`
  server boot, no second `docker logs`.
- `signal_matches:` describing the lines to watch; each pattern is applied to
  the service's emitted `matched_line`. Mark the lines that must appear with
  `expected: true` (completeness).
- optional `observe_window: <duration>` (e.g. `45s`) bounding how long to watch
  before rolling up; omit for the runtime default.

```yaml
id: s-checkout-logs
use_case_id: uc-checkout
angle: logs
kind: observational
nature: computational
output_type: stream
uses: []
depends_on: [run-dev]
observe_window: 45s
signal_matches:
  - { key: served, pattern: "GET /checkout .* 200", verdict: pass, expected: true }
  - { key: error,  pattern: "5\\d\\d|unhandled",     verdict: fail, heal_hint: { summary: "Server error during checkout", rationale: "A 5xx surfaced while exercising the use case." } }
steps:
  - id: watch
    uses: run-dev
```

### Composing core primitives + demanding inputs

A use-case sensor composes a core primitive via `uses:` + `with:`. Bind the inputs the
use case needs (e.g. `headers` for an authenticated request). If the core primitive does
NOT expose a required input, ADD that input to the core sensor's YAML with a
backward-compatible `default`/`required: false`, then bind it — the core evolves to
satisfy the use case. Derive required auth/merchant headers and signal_matches regexes
from the use case's preconditions and the stack manifest's logging library.

## How to write each sensor

> **Plugin users:** `<plugin-root>` is the directory two levels above this skill file.
> Typical path after marketplace install: `~/.claude/plugins/lastro-harness/`.

```bash
<plugin-root>/scripts/harness-tools.sh create-sensors --file /tmp/<sensor-id>.yaml --harness-dir .harness
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
- `step_resolvability` — a run-step invokes a command not installed on this machine, or a
  `make` target missing from the repo Makefile. Switch to a stack-native tool or a
  self-bootstrapping form (`go run <module>@latest`, `npx --yes <pkg>`).
- `fixture_binding` — a step's `uses:` references a fixture not owned by this use case.
- `angle_not_applicable` — sensor's `angle` is not in `applicable_angles`.
- `missing_dependency` — the use case or stack manifest is not on disk.
- `schema_violation` — a required field is missing or wrong shape.
- `unknown_enum_value` — `kind`, `nature`, or `output_type` is invalid.

## Coverage check

After writing all sensors, list `.harness/sensors/` and confirm that exactly one sensor file
exists per angle in `applicable_angles`. If any angle is missing, emit and write its sensor.
