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
2. `<plugin-root>/schemas/core-inputs/*.yaml` for each parameterized
   primitive you will emit — the **baseline input floor**: the inputs the
   primitive MUST declare, with descriptions and suggested defaults. Floors
   are keyed by angle (`e2e-test.yaml`) or by primitive id when the angle has
   floorless siblings (`provision-auth.yaml`).

## Two categories of core sensor

### Parameterized primitives (composable)

One per applicable parameterized primitive: `e2e-test`, `database` (primitive
id `database-query`), `performance`, `logs`, `metrics`, and — when the
manifest shows an authenticated surface — `provision-auth`. Use-case sensors
compose them via `uses:` + `with:` to validate any journey variation —
success, failure (expected rejections), alternative.

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
closely; the floor is `schemas/core-inputs/e2e-test.yaml` (`base_url`
derived from the manifest's dev-server port, `method`, `path`, `query`,
`headers`, `body`, `expect_status`, `timeout`).

**Quoting rule (all run scripts):** never wrap a `${{ ... }}` ref in your
own quotes — each ref already renders as a safely quoted `"${HARNESS_*}"`
env reference. `'${{ inputs.method }}'` stays a literal; `"${{ inputs.headers }}"`
unquotes the expansion and word-splits multi-word values (auth headers!).

### Auth provisioning (`provision-auth`)

Emit when any detected route requires credentials (session middleware,
bearer/JWT guards, API keys, Basic). Floor:
`schemas/core-inputs/provision-auth.yaml` (`kind`:
session|bearer|api-key|basic|none, `persona`); canonical shape:
`schemas/examples/sensor/core-provision-auth.yaml`. Contract:
`angle: environment`, `assertion`/`single-shot`, `depends_on` the service
the recipe needs; re-export output `header` — ONE ready-to-send header line
(`Cookie: ...`, `Authorization: Bearer ...`, `Authorization: Basic ...`;
empty for kind=none) written as `header=` to `$HARNESS_OUTPUT`. Implement
every kind a detected route accepts; recipes are stack-native and derived
from the manifest: mint the session credential with the app's own
dependency and secret (e.g. NextAuth JWT via `node` + `NEXTAUTH_SECRET`),
obtain a bearer token from the app's login/token endpoint, seed an API-key
row through the app's db tooling, or Base64 a seeded user's credentials
for Basic. When minting fails,
print `auth-not-provisioned kind=<k> reason=<r>` and exit 1 — covered by a
`verdict: inconclusive` matcher (never `fail`: the endpoint under test was
never reached). Use-case sensors compose it before their request step and
bind `headers: "${{ steps.<auth-step>.outputs.header }}"`.

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
`schemas/examples/sensor/core-run-dev.yaml` — note the reserved
`key: ready` matcher (`expected: true`), the `key: log-line` firehose, a
`fail` matcher with `heal_hint` on startup errors, and the single
foreground `docker compose --profile dev up` step (NO `-d`).

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
- `schema_violation` — a required field is missing or wrong shape, including a
  `scope` that is not `core` or a `use_case_id` that is set.
- `unknown_enum_value` — `kind`, `nature`, `scope`, or `output_type` is invalid.

## Coverage check

After writing all sensors, list `.harness/sensors/core/` and confirm that each
expected primitive is present. Emit any missing primitive before finishing.
