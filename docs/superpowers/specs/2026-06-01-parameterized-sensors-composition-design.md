# Parameterized Sensors via Composition (GitHub-Actions-style): Design Spec

> Source: GitHub issue [#26](https://github.com/iurykrieger/lastro/issues/26).
> Builds on: [`2026-05-29-core-sensors-design.md`](2026-05-29-core-sensors-design.md) (the `scope: core | use-case` discriminator and the intra-core `depends_on` DAG).
> Source plan: [`plan.md`](../../harness-framework/plan.md) §4.4 (Sensor schema), §6.1 (Resolver / Fixture Binder); [`E6-sensor.md`](../../harness-framework/E6-sensor.md) Q1 (`steps[].run` shape).
> Status: drafted 2026-06-01, awaiting written-spec review.

## 1. Purpose

Make core (`scope: core`) sensors **reusable across use cases** by letting them declare **input parameters**
and letting per-use-case sensors **compose** them with use-case-specific values. The composition model is
**GitHub Actions composite actions**: a primitive declares `inputs:` (and `outputs:`), and a consumer step
references it with `uses: <primitive>` + `with: {...}`.

Today a sensor's `steps[].run` is a static command string with no way to inject values at run time. The
clearest casualty is the `e2e-test` core sensor — intended as a generic curl/wget wrapper. To validate
different use cases (`POST /v1/charges`, `GET /v1/charges/:id`, `…/capture`, `…/cancel`) it must drive that
same primitive with different method/path/headers/body/expected-status. The only way to express that today is
to **duplicate the whole sensor per use case**, which defeats the purpose of repo-level base sensors.

## 2. Root cause (why this is structural)

Reusable primitives are impossible to express today across three layers:

1. **Schema** (`schemas/sensor.yaml`): a step is `{id, run, uses?:[fixtures]}`. `run` is a static string. There
   is no input declaration, no binding, no interpolation, no step output.
2. **Runtime** (`internal/runtime/executor`, `fixturebinder`): the executor runs `run` verbatim; the binder only
   resolves a step's fixture-id list into env vars + files. Neither composes one sensor's steps into another.
3. **Generation** (`/create-core-sensors`, `/create-sensors`): emits one self-contained sensor per
   `(scope × angle)`; there is no notion of a primitive that other sensors instantiate with parameters.

The reuse the issue wants — single source of truth for the primitive, per-use-case inputs — needs a
**composition relationship**, not duplication.

## 3. Decisions captured from brainstorm

| # | Decision | Rationale |
|---|---|---|
| 1 | **Core primitives stay concrete AND parameterizable.** A `scope: core` sensor runs standalone with its input defaults (a smoke), and is also composable with overrides. | Preserves the #24 model where core sensors are runnable DAG nodes; avoids a separate "abstract template" concept. Requires that a self-running primitive provide a default for every input it needs to run. |
| 2 | **Composition model = GitHub Actions, not inheritance.** `uses:` + `with:` (composition), never `extends:` (extension). | User decision. Composition is explicit, additive, and avoids inheritance's override ambiguity. |
| 3 | **Composition is step-level** (composite-action pattern), not sensor-level. A step is `uses: <primitive> + with:`; one sensor may compose several primitives across its steps. | More faithful to GHA's most common reuse and more composable (e.g. one step POSTs via `e2e-test`, the next verifies the row via `database-query`). |
| 4 | **Binding resolves at runtime (resolve-time), not generation-time.** The executor reads `uses`+`with`, inlines the primitive's steps, binds inputs. The on-disk consumer stays compositional (references the primitive → single source of truth). | GHA-faithful (actions resolve at run time). Honors *"determinism beats prediction"*: inlining + substitution is deterministic Go; only generation (which primitive, which values) is inferential. |
| 5 | **Interpolation syntax = `${{ inputs.x }}` (GHA).** Contexts: `inputs.*`, `fixtures.*`, `steps.<id>.outputs.*`. | Consistency with the GHA metaphor the user chose. |
| 6 | **Step outputs ship in v1.** Primitives declare `outputs:`; steps produce them via a `$HARNESS_OUTPUT` file; `${{ steps.<id>.outputs.<name> }}` reads them. | The real charge-api flow (capture/cancel a charge created in a prior step) needs the id from `create` — blocking from day one. |
| 7 | **Fixtures bind by interpolation**, not by an explicit per-step list. The binder collects `${{ fixtures.<id> }}` references in `run`/`with`, validates use-case ownership, and injects payloads (env var + file, as today). | Unifies params and fixtures (a `with:` value may be a fixture ref — issue Q4), and frees the step-level `uses:` keyword for composition. |
| 8 | **`uses:` keyword collision resolved by level.** Top-level `uses:` stays = StackComponent ids (grounding). Step-level `uses:` is repurposed for composition. The old step-level fixture list is removed (see #7). | Minimal blast radius on the grounding invariant; step `uses:` now means exactly "compose this primitive," matching GHA. |
| 9 | **Interpolation compiles to env-var references, never raw textual substitution.** `${{ inputs.method }}` → `"${__IN_method}"`, with the value injected as an env var. | Shell safety (issue Q3): no injection, no quoting hell for headers/body. Inherits the fixture binder's env-var+file mechanism. Inputs are **data, never code**. |
| 10 | **Composing a primitive propagates its `depends_on`** to the consumer's effective dependency set. | `e2e-test depends_on run-dev` ⇒ a sensor composing `e2e-test` also waits for `run-dev`, without restating it. |

## 4. Schema changes (`schemas/sensor.yaml`)

### 4.1 Step becomes a discriminated union (`run`-step XOR `uses`-step)

```yaml
$defs:
  SensorStep:
    type: object
    required: [id]
    additionalProperties: false
    oneOf:
      - required: [run]                  # run-step
        not: { required: [uses, with] }
      - required: [uses]                 # uses-step
        not: { required: [run] }
    properties:
      id:   { $ref: "#/$defs/Id" }
      run:  { type: string, minLength: 1 }
      uses: { $ref: "#/$defs/Id" }       # a primitive sensor id to compose
      with:                              # inputs passed to the composed primitive
        type: object
        additionalProperties: { type: string }
```

> Note: the step-level `uses:` is now a **single id** (the primitive to compose), not an array of fixture ids.
> The old fixture-list meaning is removed (decision #7/#8).

### 4.2 Sensor declares `inputs:` and `outputs:`

```yaml
properties:
  inputs:
    type: object
    additionalProperties:
      type: object
      additionalProperties: false
      properties:
        required: { type: boolean, default: false }
        default:  { type: string }
        description: { type: string }
  outputs:
    type: object
    additionalProperties:
      type: object
      required: [from]
      additionalProperties: false
      properties:
        from: { type: string }          # an interpolation expr, e.g. "${{ steps.request.outputs.id }}"
        description: { type: string }
```

Both are optional. Top-level `uses:` (StackComponents), `depends_on:`, `scope`, `use_case_id`, `angle`,
`kind`, `nature`, `output_type` are **unchanged**.

### 4.3 Worked example

```yaml
# PRIMITIVE — .harness/sensors/core/e2e-test.yaml
schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [curl]                              # grounding: StackComponent ids (unchanged)
depends_on: [run-dev]
inputs:
  method:        { required: true, default: GET }
  path:          { required: true, default: /health_check/ready }
  headers:       { required: false, default: "" }
  body:          { required: false }
  expect_status: { required: false, default: "2xx" }
outputs:
  body: { from: "${{ steps.request.outputs.body }}" }
steps:
  - id: request
    run: |
      resp=$(curl --fail -sS -X "${{ inputs.method }}" ${{ inputs.headers }} \
        ${{ inputs.body }} "http://localhost:8080${{ inputs.path }}")
      printf 'body=%s\n' "$resp" >> "$HARNESS_OUTPUT"
```

```yaml
# CONSUMER — .harness/sensors/uc-create-charge/create-charge-e2e.yaml
schema_version: 1.0.0
id: create-charge-e2e
scope: use-case
use_case_id: uc-create-charge
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: create
    uses: e2e-test
    with:
      method: POST
      path: /v1/charges
      headers: "-H 'Authorization: Bearer x' -H 'Provider-Id: y'"
      body: ${{ fixtures.create-charge-input }}
      expect_status: "201"
  - id: capture
    uses: e2e-test
    with:
      method: POST
      path: /v1/charges/${{ steps.create.outputs.charge_id }}/capture
      expect_status: "200"
```

**Backward compatibility:** existing sensors with plain `run`-steps and no `${{ }}` are valid unchanged
(`inputs`/`outputs`/`with` are all optional). The only break is sensors that used the **array** form of step
`uses:` for fixtures — see §8 (migration).

## 5. `internal/sensor` changes

- `Step` gains `Uses string` (primitive id) and `With map[string]string`; `Run` becomes optional. Add
  `Inputs map[string]InputSpec` and `Outputs map[string]OutputSpec` to `Sensor`.
- **Discriminated-union validator:** exactly one of `Run` / `Uses` set per step; `With` only on `uses`-steps.
- **Input/output validators:** input names and output names are valid `Id`s; each `outputs[*].from` is a
  syntactically valid interpolation expression.
- **Grounding (unchanged in spirit):** top-level `uses:` ⊆ StackManifest. New: a `uses`-step's target id must
  resolve to a loaded sensor with `scope: core` (a primitive). Validated at the store/resolver level (needs
  the global sensor set), not at single-file load.
- **Loader:** parse the new fields; default `inputs[*].required=false`.

## 6. `internal/runtime` changes

### 6.1 Interpolation compiler (new, deterministic)

A package that, given a `run` string (or a `with` value) and a resolution context
(`inputs`, `fixtures`, `steps.<id>.outputs`), rewrites every `${{ ctx.name }}` into a **shell variable
reference** (`"${__IN_name}"`, `"${__FX_name}"`, `"${__OUT_id_name}"`) and returns the env map to set. It
**never** inlines the raw value into the command string. Unknown contexts/names → error. This is the safety
boundary (decision #9).

### 6.2 Composition resolver (new, deterministic)

When the executor encounters a `uses`-step:
1. Look up the primitive by id; assert `scope: core`.
2. Validate every `required` input is satisfied by `with:` or a `default` → else `input-unbound` error.
3. Build the input env map from `with:` (values themselves first interpolated against the consumer's context —
   so `body: ${{ fixtures.create-charge-input }}` resolves the fixture before entering the primitive).
4. **Inline** the primitive's steps, compiling each `run` against `{inputs (from with/defaults), fixtures,
   steps.*}`.
5. Merge the primitive's `depends_on` into the consumer's effective dependency set (decision #10).

### 6.3 Fixture binder (changed)

Instead of reading `step.Uses` (the removed array), the binder **collects `${{ fixtures.<id> }}` references**
from a step's `run` and `with` values, validates each id is owned by the use case (existing `fixture-not-owned`
/ `fixture-not-found` errors), writes payloads to scratch, and exposes them as `__FX_<id>` env + file path.

### 6.4 Step-output capture (new)

Before each step the executor creates a temp file and sets `$HARNESS_OUTPUT` to its path. After the step it
parses `name=value` lines (one per line; last write wins) into that step's outputs, available to later steps
as `${{ steps.<id>.outputs.<name> }}`. A primitive's declared `outputs:` re-export selected step outputs under
the **consumer's** `uses`-step id (so the consumer reads `${{ steps.create.outputs.body }}`). Format:
`name=value` lines (matches `$GITHUB_OUTPUT`'s simple form); multiline values are out of scope for v1.

## 7. Generation (skills)

- **`/create-core-sensors`:** emit primitives with `inputs:` (sensible defaults so the smoke self-runs) and,
  where a downstream step needs it, `outputs:`. `e2e-test`, `database-query`, `performance`, `logs`, `metrics`
  become parameterized primitives; `run-dev`/`datastore` stay parameter-free environment roots.
- **`/create-sensors <uc>`:** for each applicable angle, emit a consumer sensor whose steps `uses:` the matching
  core primitive with `with:` values grounded in the detected stack (endpoints, methods, fixtures). Every
  `required` input must be bound. Fixtures referenced as `${{ fixtures.<id> }}`.

## 8. Migration

Existing on-disk sensors using the **array** step `uses:` for fixtures:
`.harness/sensors/uc-harness-validate-use-case-build.yaml` and `…-unit-test.yaml` (the dogfood sensors). Rewrite
their `uses: [sample-harness-tree]` to a `${{ fixtures.sample-harness-tree }}` reference inside the step's
`run`. The loader does **not** accept the old array form for step `uses:` (it is now a scalar primitive id);
the migration is a hard cutover covered by a test asserting the rewritten sensors load and validate.

## 9. Validation & error surface

- `input-unbound` — a `required` primitive input has no `with:` value and no `default` (resolve-time;
  `/validate-use-case`, `/run-sensor` exit non-zero).
- `uses-target-not-core` — a step `uses:` an id that is not a loaded `scope: core` sensor.
- `interp-unknown-ref` — `${{ ctx.name }}` references an unknown context, input, fixture, or step output.
- `step-shape` — a step sets both `run` and `uses`, or neither.
- `fixture-not-owned` / `fixture-not-found` — unchanged, now triggered by interpolation refs.

## 10. Grounding invariant (issue Q5) — preserved

The grounding check (`uses:` ⊆ StackManifest) is enforced on the **primitive**. Composition cannot weaken it:
`with:` carries **data only**, never a StackComponent id or a command fragment; the executing toolset is the
primitive's. A `uses`-step adds no new top-level `uses:` of its own. The only new grounding rule is that a
`uses`-step's target must be a real `scope: core` primitive (§5).

## 11. Testing

- **Schema/loader:** `run`-xor-`uses` union (both/neither → error); `inputs`/`outputs` parse; backward-compat
  load of a plain `run`-only sensor.
- **Interpolation compiler:** `${{ inputs.x }}`/`${{ fixtures.x }}`/`${{ steps.id.outputs.y }}` → env refs;
  quoting safety (a header/body containing quotes survives intact); unknown ref → error.
- **Composition resolver:** required input unbound → `input-unbound`; `with` fixture ref resolved before entering
  the primitive; primitive steps inlined; `depends_on` propagation.
- **Step-output capture:** a step writing `charge_id=…` to `$HARNESS_OUTPUT` is readable by a later step;
  primitive `outputs:` re-export under the consumer's `uses`-step id.
- **Fixture binder:** refs collected from `run` + `with`; ownership errors preserved.
- **Generation:** `/create-core-sensors` emits primitives with defaulted inputs; `/create-sensors` binds every
  required input.
- **Dogfood:** regenerate this repo's sensors in the new shape; `/detect-stack → /create-core-sensors →
  /create-sensors → /validate-use-case` runs end-to-end, with a use case reusing one primitive across ≥2 angles.

## 12. Open questions for the plan / freeze gate

1. **`$HARNESS_OUTPUT` output format.** `name=value` lines (chosen) vs JSON. v1 picks `name=value`; multiline
   values deferred. Confirm at freeze.
2. **Schema-freeze sign-off.** This touches the frozen `sensor.yaml` (step union, `inputs`/`outputs`, step
   `uses:` array→scalar) → record in `00-schema-freeze.md` before any Go code lands.
3. **Repeatable inputs (multi-`-H` headers).** v1 models `headers` as a single pre-quoted string the primitive
   passes through. A typed list/array input is a possible follow-up if pass-through proves fragile.
4. **Primitive self-run vs composition-only.** A primitive with a `required` input lacking a `default` cannot
   self-run (effectively composition-only). v1 expects core primitives to default every input (decision #1); a
   validator warning when a core sensor has a required-no-default input is optional.

## 13. Acceptance criteria

- A `scope: core` primitive with `inputs:` (all defaulted) loads, validates, and runs standalone as a smoke.
- A use-case sensor with `uses: <primitive>` + `with:` resolves: the primitive's steps run with the bound
  inputs; a `required` input left unbound fails with `input-unbound` and a non-zero exit.
- `${{ fixtures.<id> }}` in a `with:` value binds the fixture's payload; `${{ steps.<id>.outputs.<name> }}` from
  a prior step is readable.
- A header/body value containing shell metacharacters is passed intact (no injection, no broken quoting).
- The grounding check still passes/fails on the primitive's top-level `uses:`; composition introduces no new
  ungrounded component.
- The dogfood chain runs end-to-end with one primitive reused across ≥2 use cases.
