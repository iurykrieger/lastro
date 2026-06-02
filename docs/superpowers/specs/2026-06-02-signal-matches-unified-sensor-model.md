# Signal Matches — Unified Sensor Signal Model + Core Composition

- **Date:** 2026-06-02
- **Status:** Draft (design approved in brainstorming)
- **Supersedes:** the `expected_observations` field added for observational sensors
  (currently in PR #28, `feat/detached-observational-watcher`).

## Problem

Two gaps in the current sensor model:

1. **Signal generation is fragmented.** Sensors only produce signals when a
   stdout line is a valid JSON `Signal` (assertion sensors via a helper) or — for
   observational sensors — when a line matches an `expected_observations` regex.
   There is no single, declarative way for *any* sensor to turn its log output
   into structured signals. An observational HTTP sensor, for example, cannot say
   "each request log line is a signal" without bespoke tooling.

2. **Core primitives are under-parameterized.** A use-case sensor composes a core
   primitive via `uses:` + `with:`, but the core primitives expose a narrow input
   set (e.g. `e2e-test` has `method`/`path`/`body` but no `headers`). A use case
   that requires authentication headers cannot express that through composition,
   so its e2e sensor cannot issue an authenticated request.

## Goals

- A single declarative mechanism — `signal_matches` — on **every** sensor that maps
  log lines to structured signals, replacing `expected_observations`.
- Verdict-bearing, semantically rich signals derived from regex matches, including
  named-capture-group evidence.
- A completeness notion (expected-but-absent instrumentation) layered on the same
  mechanism via an `expected` flag.
- Richer, composable core primitives (e.g. `e2e-test` gains `headers`), plus a
  generation rule that extends a core primitive when a use case demands an input it
  does not yet expose.
- Generation that writes `signal_matches` regexes appropriate to the logging library
  detected in `stack-manifest.yaml`, with the harness remaining a generic regex engine.

## Non-Goals

- No structured per-library log parsing in the harness (JSON field extraction, etc.).
  Lib-awareness lives in the generator prompts; the harness only runs regexes.
- No rule/expression language for inferring verdicts from captures. Different outcomes
  are expressed as different matchers.
- No runtime mutation of core sensors. "Extending a core" is a *generation-time* edit
  to the core YAML, not a runtime behavior.

## Design

### 1. `signal_matches` schema (replaces `expected_observations`)

A new optional top-level array on the Sensor schema, valid for both `assertion` and
`observational` sensors:

```yaml
signal_matches:
  # Presence-expected, neutral observation (completeness).
  - key: http-served
    pattern: '"status_code":(?P<status>\d{3})'   # match the field, order-independent
    verdict: pass                                # optional, default: pass
    confidence: 1                                # optional, default: 1
    expected: true                               # optional, default: false
  # A failure pattern (no `expected` — see expected×verdict rule below).
  - key: http-server-error
    pattern: '"status_code":5\d\d'
    verdict: fail
    heal_hint:                                   # object, not a bare string (§2)
      summary: "a request returned a 5xx response"
      rationale: "the service logged a 5xx status; the handler likely errored"
```

Field rules:

- `key` — required, kebab-case Id (`^[a-z][a-z0-9-]*$`).
- `pattern` — required, a Go `regexp` (RE2: no backreferences/lookaround) tested
  against each output line. Patterns SHOULD anchor on individual log fields and not
  rely on field ordering (structured loggers do not guarantee key order); see §5.
- `verdict` — optional, one of `pass|warn|fail|inconclusive`, default `pass`.
- `confidence` — optional number in `[0,1]`, default `1`.
- `expected` — optional bool, default `false`. Feeds completeness (§3). **`expected`
  is only meaningful on `pass` matchers** (presence expected = healthy
  instrumentation). On a non-`pass` matcher it is rejected at schema/load time to
  avoid the ambiguous "I expect to see a failure" semantics.
- `heal_hint` — optional `{summary, rationale}` object (mirrors the Signal schema's
  `HealHint`, both fields `minLength: 1`). **Required-by-effect** when `verdict` is
  `fail` or `warn`. If omitted for a fail/warn matcher, the harness synthesizes a
  deterministic heal_hint: `summary` = "matched <verdict> pattern <key>",
  `rationale` = the matched line (truncated) — guaranteeing a schema-valid signal.

The `Sensor` Go struct replaces `ExpectedObservations []ObservationMatcher` with
`SignalMatches []SignalMatch{Key, Pattern, Verdict, Confidence, Expected, HealHint}`,
where `HealHint` is the existing `{Summary, Rationale}` shape.

### 2. Match → Signal mapping

For each output line, every matcher whose `pattern` matches synthesizes one Signal:

- `verdict` = matcher verdict (default pass); `confidence` = matcher confidence (default 1).
- `evidence` = `{ observation_key: <key>, matched_line: <line>, <named captures...> }`.
  Named capture groups `(?P<name>…)` are added as evidence fields, producing
  semantically rich signals (e.g. `status`, `path`, `latency`).
- `heal_hint`: present iff verdict is fail/warn — the matcher's `{summary, rationale}`
  or the deterministic fallback from §1. Both fields are always non-empty.
- The signal carries `sensor_id`, `use_case_id`, `angle`, `emitted_at`, and a
  `schema_version` equal to the **Signal schema version** the harness emits (the
  existing `observationSignalSchemaVersion` constant), not the sensor's
  `schema_version`. (This matches current behavior; called out to avoid ambiguity.)
- **Synthesized signals are validated at emit time** via `signal.DecodeLine`/`Validate`
  before being written to `signals.jsonl`; a matcher that would produce an invalid
  signal fails the run with a descriptive error rather than emitting a line that breaks
  downstream readers. This makes "every emitted signal is schema-valid" an enforced
  invariant, not an assumption.

Multiple matchers may fire on the same line (each emits its own signal). A line that is
already a JSON `Signal` (starts with `{`) is decoded as today; non-JSON lines are tested
against `signal_matches`. Lines matching neither are plain output (no parse-error).

### 3. Where it runs + completeness + assertion interaction

- **Streams:** matched against every stdout and stderr line (raw.log aggregates both),
  matching current behavior. The `jsonlWriter` is goroutine-safe so both pumps may emit.
- **Observational** sensors: long-running watcher; matches stream into `signals.jsonl`
  as they occur (e.g. one signal per HTTP request line).
- **Assertion** sensors: `signal_matches`, JSON `Signal` lines, and the step exit code
  coexist. Exit ≠ 0 keeps the current `termination_reason: error` semantics; emitted
  signals (from matches + JSON) and completeness roll up into the verdict.
- **Verdict aggregation (unchanged severity rule):** the aggregate verdict is the most
  severe signal verdict (`severityVerdict` in `rollup.go`), so a single `fail` matcher
  firing anywhere makes the run fail. There is no per-key verdict aggregation; this
  spec does not change that rule.
- **Completeness:** matchers with `expected: true` define the expected observation keys.
  If an expected key is never observed, the aggregate is incomplete and contributes to a
  fail/inconclusive verdict with a heal_hint listing the missing keys. The existing
  completeness path (`RollupInput.ExpectedObservations` → `computeCompleteness`) is
  **currently gated to `kind: observational`** (`rollup.go`); this spec widens it to
  apply to any sensor that declares `expected` matchers, regardless of kind. This is an
  explicit code change to `computeCompleteness`/`computeVerdict` (see §6/DoD), not free.
- **Throughput & ordering:** `signals.jsonl` flushes per line under a shared mutex, so a
  high-volume stream (one signal per HTTP line) serializes on that lock with a flush per
  signal — acceptable for dev observability, noted as a known characteristic. Signal
  ordering between stdout and stderr is non-deterministic (two reader goroutines); tests
  MUST NOT assert a stable cross-stream order.

### 4. Core primitive composition (point 1)

- Core primitives expose comprehensive inputs up front. `e2e-test` gains a `headers`
  input (and any other broadly-useful knobs); use-case sensors bind them via `with:`.
  Inputs carry `default`/`required: false` so the primitive still self-runs.
- **Demand-driven extension:** when a use-case sensor needs an attribute the core does
  not expose, the generation step **adds that input to the core sensor YAML** (with a
  backward-compatible default), then binds it from the use-case sensor. This is a
  generation-time rule, documented in the create-sensors skills — not a runtime feature.

### 5. Lib-aware generation (points 1 + 3)

`/create-sensors` and `/create-core-sensors` (their SKILL.md prompts) are updated to:

- Read the logging library from `stack-manifest.yaml` and write `signal_matches`
  regexes in that library's format (e.g. zap → JSON field patterns like
  `"status_code":5\d\d`, with named captures for `status`, `path`, `latency`).
- Anchor patterns on **individual fields** and avoid relying on JSON key order or
  bridging fields with `.*` (RE2 has no lookaround/backreferences; structured loggers
  do not guarantee key order). Prefer one matcher per outcome over one complex pattern.
- Identify the auth/header and other inputs a use case requires and bind them from the
  relevant core primitive, extending the core (per §4) when the input is missing.

The harness itself gains no per-library knowledge; it only compiles and runs regexes.

### 6. Migration of the current `expected_observations` work

The `expected_observations` mechanism added in PR #28 is reworked into `signal_matches`:

- `schemas/sensor.yaml`: rename/extend the field to the `signal_matches` shape (§1).
- `internal/sensor/types.go`: `ObservationMatcher` → `SignalMatch` with the new fields.
- `internal/runtime/executor/{signals,step,compose,executor}.go`: matcher compilation,
  per-line synthesis with verdict/captures/heal_hint, emit-time validation, and
  completeness wiring.
- `internal/aggregate/rollup.go`: `computeCompleteness` (today `kind == observational`
  only) is widened to run whenever `expected` matcher keys are present, so assertion
  sensors get completeness too; `computeVerdict` factors the resulting incompleteness.
- `.harness/sensors/core/run-dev.yaml` (charge-api): migrate `expected_observations`
  to `signal_matches`.
- The Signal-schema changes from PR #28 (`environment` angle, empty `use_case_id` for
  core sensors) are retained. **The Signal schema is not otherwise modified by this
  spec** — its `HealHint` already requires `{summary, rationale}`, which the matcher
  reuses. If any future change does touch `schemas/signal.yaml`, the embedded copy
  `internal/signal/schema.yaml` MUST be re-synced (guarded by `drift_test.go`).

## Definition of Done

1. `signal_matches` is accepted by the Sensor schema for both kinds; `expected_observations`
   is removed; the loader parses `SignalMatch` (incl. `verdict`, `confidence`, `expected`,
   `heal_hint{summary,rationale}`). Schema/load rejects `expected: true` on a non-`pass` matcher.
2. A matched line synthesizes a Signal with the matcher's verdict/confidence and
   named-capture evidence; fail/warn matchers carry a `{summary, rationale}` heal_hint
   (matcher-supplied or synthesized). The synthesized signal passes `signal.DecodeLine`
   validation at emit time; an invalid one fails the run with a descriptive error.
3. Matching runs on stdout and stderr for all sensors; JSON-signal decoding and the
   no-parse-error-for-plain-text behavior are preserved.
4. `expected: true` matchers drive completeness for **any** sensor kind (not just
   observational); a run where an expected key is never observed yields an incomplete
   aggregate whose verdict is fail/inconclusive with a heal_hint naming the missing keys.
5a. `e2e-test` exposes a `headers` input that a use-case sensor binds via `with:`,
   verifiable through the executor's input-env build (a bound header reaches the request).
5b. (Doc acceptance, not test-suite) `/create-sensors` and `/create-core-sensors`
   SKILL.md document demand-driven core extension and lib-aware `signal_matches` regex
   generation.
6. `run-dev.yaml` uses `signal_matches`; the full test suite passes and the race detector
   is clean on the executor and lifecycle packages.

## Resolved during review

- **`expected` × `verdict`:** `expected` is valid only on `pass` matchers; rejected on
  others at load time (avoids the "I expect a failure" ambiguity).
- **heal_hint shape:** a `{summary, rationale}` object (not a bare string), with a
  deterministic synthesized fallback, so fail/warn signals are always schema-valid.
- **Completeness scope:** widened from observational-only to any sensor with `expected`
  matchers (explicit `rollup.go` change).
- **schema_version of synthesized signals:** the Signal schema version constant, not the
  sensor's `schema_version` (matches current behavior).

## Open Questions

- Exact evidence key names for common captures (`status`, `latency`, `path`) — left to
  generation conventions rather than hard-coded.
- Whether `confidence` should default differently for fail matchers (kept at 1 for now).
