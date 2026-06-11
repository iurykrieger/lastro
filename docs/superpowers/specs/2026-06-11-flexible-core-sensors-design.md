# Flexible Core Sensors — Baseline Input Surfaces

**Date:** 2026-06-11
**Status:** Approved
**Owner:** `/create-core-sensors`, `/create-sensors`, `internal/` validators

## Problem

`/detect-use-cases` now emits journey-grouped use cases in three variations
(`success`, `failure`, `alternative`). Use-case sensors for failure and
alternative variations must send bodies, headers, and query params, and assert
*rejections* (a 422 response, an error log line, the absence of a DB row) —
not just happy paths.

Today's core parameterized primitives can't support that:

- The generated input surface is narrow (`method`, `path`) and ad hoc per run
  of `/create-core-sensors`.
- Run scripts use failure-on-response idioms (`curl --fail`), so an expected
  4xx aborts the step instead of being graded.
- Declared inputs can be silently ignored by the run script (the golden
  `e2e-test` example declares `expect_status` but never reads it).
- `base_url` is hardcoded (`localhost:8080`).
- The only growth path is prose guidance in `/create-sensors` ("ADD that input
  to the core sensor's YAML"), with no validation that the binding is legal.

## Decisions (from brainstorming)

1. **Curated canonical inputs** per angle, not free-form passthrough.
2. The canonical set is a **floor, not a ceiling** — `/create-sensors` may
   grow a primitive's inputs when a new use case needs it.
3. **Grading is shared**: the core primitive grades canonical expectations
   *and* emits normalized output lines; the use-case sensor layers its own
   `signal_matches` on top.
4. Validation enforces the **floor at core-generation time** and **`with:`
   keys at compose time**, plus input-reference faithfulness.
5. All **five parameterized angles** get baselines in one pass.

## 1. Baseline input specs

One spec file per parameterized angle under `schemas/core-inputs/<angle>.yaml`,
embedded in the Go binary alongside the existing schemas (`schemas.go`).
Each entry carries `name`, `description`, and a `suggested_default`.

Semantics:

- A core primitive for angle X **must declare at least** the baseline inputs
  for X. It may declare more.
- Every declared input must carry a `default:` (existing invariant — the
  primitive self-runs as a smoke test with no caller configuration).
- `suggested_default` is a hint for the skill; `/create-core-sensors` may
  override it with a manifest-derived value (e.g. `base_url` from the
  detected dev-server port).

Baselines:

| angle | baseline inputs |
|---|---|
| `e2e-test` | `base_url`, `method`, `path`, `query`, `headers`, `body`, `expect_status`, `timeout` |
| `database` (primitive id `database-query`) | `query`, `params`, `expect_rows`, `timeout` |
| `performance` | `base_url`, `method`, `path`, `headers`, `body`, `duration`, `rate`, `p95_budget_ms` |
| `logs` | `pattern`, `anti_pattern`, `within`, `service` |
| `metrics` | `metrics_url`, `name`, `labels`, `predicate`, `within` |

Encoding (no `sensor.yaml` schema change — `with:` values stay strings):

- `headers`: newline-separated `Key: value` lines.
- `params`: JSON-encoded array string (e.g. `'["42", "BRL"]'`).
- `expect_status`: shell-glob class (`2xx`, `4xx`) or exact code (`422`).
- `expect_rows`: exact count (`0`, `1`) or bound (`>=1`).
- `timeout` / `within` / `duration`: Go duration strings (`10s`).

## 2. Grade-and-emit contract for primitives

Every parameterized primitive's run script follows the "both" model:

1. **Unexpected responses are data, not transport errors.** Never `curl
   --fail` or equivalent. A 422 reaches grading like any other outcome.
2. **Always emit normalized `key=value` lines** to stdout — `status=`,
   `body=`, `rows=`, `p95_ms=`, `matched_line=` — and re-export them through
   `outputs:` so downstream steps can bind them.
3. **Grade canonical expectations internally.** `expect_status` (case-glob
   match), `expect_rows`, `p95_budget_ms`, `pattern`/`anti_pattern`,
   `predicate`. Expectation met → exit 0. Unmet → non-zero exit plus a
   greppable fail line; the primitive's own `signal_matches` attaches a
   generic heal_hint.
4. **Use-case sensors refine grading** with sensor-level `signal_matches`
   over the normalized lines, carrying use-case-specific heal_hints.

Reference shape (e2e-test):

```yaml
steps:
  - id: request
    run: |
      status=$(curl -sS -o /tmp/harness-body -w '%{http_code}' \
        --max-time "${{ inputs.timeout }}" \
        -X "${{ inputs.method }}" \
        "${{ inputs.base_url }}${{ inputs.path }}${{ inputs.query }}")
      printf 'status=%s\n' "$status"
      printf 'body=%s\n' "$(cat /tmp/harness-body)"
      printf 'status=%s\nbody=%s\n' "$status" "$(cat /tmp/harness-body)" >> "$HARNESS_OUTPUT"
      case "$status" in ${{ inputs.expect_status }}) exit 0 ;; *) echo "expectation-unmet expected=${{ inputs.expect_status }} got=$status"; exit 1 ;; esac
```

A failure-variation use-case sensor then composes:

```yaml
steps:
  - id: reject
    uses: e2e-test
    with:
      method: POST
      path: /v1/charges
      body: ${{ fixtures.invalid-charge }}
      expect_status: "422"
signal_matches:
  - { key: rejected,    pattern: "status=422",      verdict: pass, expected: true }
  - { key: wrong-error, pattern: "body=.*internal", verdict: fail,
      heal_hint: { summary: "Validation returned a 5xx instead of 422",
                   rationale: "The invalid payload should be rejected by input validation, not crash the handler." } }
```

## 3. Validator changes (Go)

Three new exit-2 error kinds:

| kind | emitted by | condition |
|---|---|---|
| `incomplete_input_surface` | `create-core-sensors` | primitive for angle X lacks a baseline input name for X |
| `unknown_with_key` | `create-sensors` | a step's `with:` key is not a declared input of the composed core primitive |
| `unreferenced_input` | `create-core-sensors` | a declared input never appears as `${{ inputs.<name> }}` in any step |

`unknown_with_key` drives the explicit **evolution flow**:

1. `/create-sensors` binds `with: { idempotency_key: ... }` → exit 2
   `unknown_with_key`.
2. The skill edits `.harness/sensors/core/e2e-test.yaml`, adding
   `idempotency_key: { required: false, default: "" }` *and referencing it in
   the run script* (or `unreferenced_input` fires on re-persist).
3. Re-persist the core (floor check passes — baselines are minimums).
4. Retry the use-case sensor → exit 0.

No changes to `schemas/sensor.yaml`; `inputs`, `outputs`, and `with` already
exist. New spec files get a small meta-schema (`schemas/core-input-baseline.yaml`)
validated by the existing schema tests.

## 4. Skill and example updates

- **`skills/create-core-sensors/SKILL.md`**: add the grade-and-emit contract;
  point to `schemas/core-inputs/` as the baseline source of truth (no inlined
  per-angle tables — keeps the 200-line budget); require manifest-derived
  `base_url`; document the new exit-2 kinds.
- **`skills/create-sensors/SKILL.md` + `angles.md`**: replace the prose-only
  "ADD that input to the core sensor" rule with the `unknown_with_key`-driven
  evolution flow; per-angle sections list the baseline inputs and the
  normalized output keys (`status=`, `rows=`, …) available for matching.
- **Golden examples**: rewrite
  `schemas/examples/sensor/core-e2e-primitive.yaml` with the full baseline and
  a faithful run script (no `--fail`, real `expect_status` grading); keep
  `uc-consumer.yaml` as is — its `body` binding becomes legal once the
  baseline declares it.

## 5. Testing

Per repo rules (TDD, every package tested):

- Unit tests for the baseline-spec loader and each new validator kind
  (positive + negative cases, exact JSON error shapes).
- Golden examples re-validated by the existing `schemas_test.go` pass.
- `skills/create-core-sensors/scripts/integration_test.go` extended: generate
  all five primitives against a sample manifest; compose one
  failure-variation use-case sensor end-to-end; assert `unknown_with_key`
  fires for an undeclared binding and clears after core evolution.

## Out of scope (YAGNI)

- Typed inputs (string-only stays).
- Free-form `with:` passthrough.
- `sensor.yaml` schema version bump.
- Runtime executor changes beyond what grading already supports.
- Environment primitives (`run-dev`, `datastore`) — they remain input-less.
