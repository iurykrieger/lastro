---
name: create-core-sensors
description: Generate the repo-level core sensors (parameterized primitives + environment primitives) from the stack manifest. Run /detect-stack first. No argument.
---

# /create-core-sensors

You are generating the repo-level core sensor primitives. Core sensors have
`scope: core`, carry no `use_case_id`, and live under
`.harness/sensors/core/<id>.yaml`.

## Prerequisites

`.harness/stack-manifest.yaml` must exist; if absent, stop and tell the user
to run `/detect-stack` first.

## What to read first

Use the Read tool to load:

1. `.harness/stack-manifest.yaml` — note `archetype`, `applicable_angles`, and
   `components[*].id` (the only valid ids for sensor-level `uses:`).
2. `<plugin-root>/schemas/core-inputs/*.yaml` for each parameterized primitive
   you emit — the **baseline input floor** (inputs the primitive MUST declare,
   with descriptions and suggested defaults). Floors are keyed by angle
   (`e2e-test.yaml`) or by primitive id (`provision-auth.yaml`).

## Ground environment sensors in the dependency model

If `.harness/environment-model.yaml` exists (run `/detect-environment` first),
it is the source of truth — do NOT infer boot commands. Emit **one core sensor
per node**, resolving each command from the node's `provided_by` pointer (read
the named `package.json` script / compose service):

- **`application`** → `core-run-dev` (observational/stream, `angle: environment`):
  `run` = resolved app command; a `key: ready` matcher for the readiness line;
  `depends_on` = node-name edges translated to `core-*` ids; every app-read key
  under `env:`.
- **`dependencies.<svc>`** → `core-datastore-<svc>` (observational/stream):
  `run` brings the service up via the compose file and blocks until ready; a
  `type`-appropriate `key: ready` probe (datastore → `pg_isready`);
  `uses: [compose]`. See `core-datastore-postgres.yaml`.
- **`setup[]`** → `core-<id>` (assertion/single-shot): `run` = resolved command;
  required keys under `env:`; `depends_on` the datastore. See `core-migrate.yaml`.

**Translate edges:** model `depends_on: [postgres, migrate]` →
`depends_on: [core-datastore-postgres, core-migrate]`. The resolver orders the
closure; use-case sensors depend only on `core-run-dev` and inherit the rest.

If the model is absent, fall back to the inference flow below.

## Two categories of core sensor

### Parameterized primitives (composable)

One per applicable parameterized primitive: `e2e-test`, `database` (id
`database-query`), `performance`, `logs`, `metrics`, and — for an
authenticated surface — `provision-auth`. Use-case sensors compose them via
`uses:` + `with:` to validate any journey variation (success, failure,
alternative).

Rules:
- Declare `inputs:` covering **at least** the angle's baseline floor (more
  when this repo needs them — never fewer). Every input carries a `default:`
  so the primitive self-runs as a smoke test (derive from the manifest, e.g.
  `base_url` from the dev-server port; else the baseline's `suggested_default`).
- Reference EVERY declared input as `${{ inputs.<name> }}` in step `run` — a
  declared-but-ignored input is rejected (`unreferenced_input`).
- Declare `outputs:` re-exporting normalized results; write step outputs to
  `$HARNESS_OUTPUT` as `name=value` lines.
- Top-level `uses:` contains only manifest `StackComponent` ids;
  `depends_on:` lists core sensors that must start first (e.g. `run-dev`).
- Declare top-level `env:` naming every ambient var a recipe reads (secrets,
  connection strings — see the floor's `env_guidance`). The runtime injects
  the manifest's `env_file` and fails fast with `missing_env` on an absent
  required var — never diagnose a missing secret in the recipe. Read injected
  vars as plain `$NAME`, never `${{ env.NAME }}` (ambient-view only).

**Grade-and-emit contract** (every parameterized primitive):

1. An unexpected response is *data*, not a transport error. Never `curl --fail`
   — a 422 must reach grading so failure-variation use cases can expect it.
2. Emit normalized `key=value` lines to stdout (`status=`, `body=`, `rows=`,
   `p95_ms=`, `matched_line=`) mirrored to `$HARNESS_OUTPUT`, stable and one
   fact per line (use-case sensors grade them).
3. Grade the expectation inputs (`expect_status`, `expect_rows`,
   `p95_budget_ms`, `pattern`/`anti_pattern`, `predicate`) in the primitive:
   met → exit 0; unmet → print `expectation-unmet expected=<e> got=<g>` and
   exit 1, covered by a fail matcher with a generic `heal_hint`.

The canonical full shape (inputs, outputs, matcher, graded curl) is
`schemas/examples/sensor/core-e2e-primitive.yaml`; the floor is
`schemas/core-inputs/e2e-test.yaml` (`base_url` from the dev-server port,
`method`, `path`, `query`, `headers`, `body`, `expect_status`, `timeout`).

**Quoting rule:** never wrap a `${{ ... }}` ref in your own quotes — each
renders as a safely quoted `"${HARNESS_*}"`. `'${{ inputs.method }}'` stays a
literal; `"${{ inputs.headers }}"` unquotes and word-splits multi-word values
(auth headers!).

### Auth provisioning (`provision-auth`)

Emit when a detected route requires credentials. Floor:
`schemas/core-inputs/provision-auth.yaml` (`kind`:
session|bearer|api-key|basic|none, `persona`); shape:
`schemas/examples/sensor/core-provision-auth.yaml`. Contract:
`angle: environment`, `assertion`/`single-shot`, `depends_on` the service it
needs; re-export output `header` — ONE ready-to-send header line (`Cookie:`,
`Authorization: Bearer`/`Basic`; empty for kind=none) written as `header=` to
`$HARNESS_OUTPUT`. Implement every kind a route accepts with stack-native
recipes from the manifest: mint the session credential with the app's own
dependency + secret (e.g. NextAuth JWT via `node` + `NEXTAUTH_SECRET`), get a
bearer from the login endpoint, seed an API-key row via the app's db tooling,
or Base64 a seeded user for Basic. On mint failure, print
`auth-not-provisioned kind=<k> reason=<r>` and exit 1 — covered by a
`verdict: inconclusive` matcher (never `fail`: the endpoint was never reached).
Use-case sensors compose it before their request step, binding
`headers: "${{ steps.<auth-step>.outputs.header }}"`.

### Environment primitives (lifecycle, no inputs)

Environment primitives set up/tear down runtime infrastructure (`run-dev`,
`datastore`). They take **no** `inputs:`, produce no composable `outputs:`,
and their steps contain only `run:` commands.

A `kind: observational` + `scope: core` environment primitive is a **shared
service**: the runtime starts one instance, reference-counts it, and keeps it
alive while any sensor is attached (attaching sensors share its live signal
stream rather than spawning a copy). Two consequences:

- **Readiness key is reserved.** The service manager blocks each attaching
  sensor until the service emits an observation whose `key` is exactly
  `ready` (not `api-ready`) or consumers never unblock.
- **Emit a firehose matcher.** Add a catch-all (`key: log-line`,
  `pattern: ".+"`, `verdict: pass`) so attaching sensors grep each log line via
  `matched_line`.

## Command shape MUST match `kind` + `output_type`

| kind | output_type | Command shape |
|------|-------------|---------------|
| `assertion` | `single-shot` | Runs, **exits**, verdict from exit code / parsed output (`up -d` + `wait-ready` loop, a single `curl`, `go test`, …). |
| `observational` | `stream` | A **long-running foreground** command that blocks and streams while the watcher lives; **must not** detach or self-exit. |

So a `run-dev` (`observational`/`stream`) MUST run the dev stack in the
**foreground** — e.g. `docker compose --profile dev up` (NO `-d`) as its
single, blocking step. Never emit `up -d` + a `wait-ready` loop for an
observational/stream sensor: that is the assertion/single-shot shape.

### signal_matches (all sensors)

Every sensor MAY declare `signal_matches: [{ key, pattern, verdict?, confidence?, expected?, heal_hint? }]`.
Each Go RE2 regex (no backreferences/lookaround) tests every output line; a
match emits a Signal with the matcher's `verdict` (default pass) and
`(?P<name>…)` captures as evidence. `expected: true` (pass matchers only)
means the key must appear at least once or the run is incomplete (fail).
Derive patterns from the manifest's logging library: anchor on individual
fields (no JSON key-order or `.*` bridging); one matcher per outcome; give
every fail/warn matcher a `heal_hint: {summary, rationale}`.

Example (observational/stream): `schemas/examples/sensor/core-run-dev.yaml`
— the reserved `key: ready` matcher, the `key: log-line` firehose, a `fail`
matcher with `heal_hint`, and the single foreground step (NO `-d`).

## What to emit

For each applicable primitive (driven by `applicable_angles` + `archetype`),
emit one YAML matching `schemas/sensor.yaml`: `schema_version: 1.0.0`,
canonical `id`, `scope: core`, no `use_case_id`, the category's
`angle`/`kind`/`nature`/`output_type`, grounded `uses:`, and `signal_matches`.

## How to validate each sensor

`<plugin-root>` is two levels above this skill file (e.g.
`~/.claude/plugins/lastro-harness/` after a marketplace install).

```bash
<plugin-root>/scripts/harness-tools.sh create-core-sensors --file /tmp/<sensor-id>.yaml --harness-dir .harness
```

Exit codes: **0** success (written, done); **2** validation failure (read JSON
error from stdout; fix YAML; retry, cap 3); **1** script error (read stderr;
surface to user; stop).

Common `kind` values on exit 2:

- `grounding` — top-level `uses:` names a component absent from the manifest.
- `step_resolvability` — a run-step invokes an uninstalled command or missing
  `make` target; use a stack-native/self-bootstrapping form
  (`go run <module>@latest`, `npx --yes <pkg>`).
- `incomplete_input_surface` — missing part of the angle's floor; declare every
  input from `schemas/core-inputs/<angle>.yaml` with a `default`.
- `unreferenced_input` — a declared input is never `${{ inputs.<name> }}`-bound.
- `missing_dependency` — the stack manifest is not on disk.
- `schema_violation` — field missing/wrong shape (incl. `scope != core` or a set
  `use_case_id`); `unknown_enum_value` — bad `kind`/`nature`/`scope`/`output_type`.

## Coverage check

List `.harness/sensors/core/` and emit any missing expected primitive before finishing.
