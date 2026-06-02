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
  - key: http-request-ok                 # kebab-case; also the observation key
    pattern: '"path":"(?P<path>[^"]+)".*"status_code":(?P<status>2\d\d)'
    verdict: pass                         # optional, default: pass
    confidence: 1                         # optional, default: 1
    expected: true                        # optional, default: false
  - key: http-request-error
    pattern: '"status_code":(?P<status>5\d\d)'
    verdict: fail
    heal_hint: "request returned a 5xx response"   # see §2
```

Field rules:

- `key` — required, kebab-case Id (`^[a-z][a-z0-9-]*$`).
- `pattern` — required, a Go `regexp` (RE2) tested against each output line.
- `verdict` — optional, one of `pass|warn|fail|inconclusive`, default `pass`.
- `confidence` — optional number in `[0,1]`, default `1`.
- `expected` — optional bool, default `false`. Feeds completeness (§3).
- `heal_hint` — optional string summary; **required-by-effect** when `verdict` is
  `fail` or `warn` (the Signal schema mandates a heal_hint for those verdicts). If
  omitted for a fail/warn matcher, the harness synthesizes a generic heal_hint from
  the matcher key and the matched line.

The `Sensor` Go struct replaces `ExpectedObservations []ObservationMatcher` with
`SignalMatches []SignalMatch{Key, Pattern, Verdict, Confidence, Expected, HealHint}`.

### 2. Match → Signal mapping

For each output line, every matcher whose `pattern` matches synthesizes one Signal:

- `verdict` = matcher verdict (default pass); `confidence` = matcher confidence (default 1).
- `evidence` = `{ observation_key: <key>, matched_line: <line>, <named captures...> }`.
  Named capture groups `(?P<name>…)` are added as evidence fields, producing
  semantically rich signals (e.g. `status`, `path`, `latency`).
- `heal_hint`: present iff verdict is fail/warn — from the matcher's `heal_hint` or a
  synthesized fallback. This keeps every emitted signal schema-valid.
- The signal carries the sensor's identity (`sensor_id`, `use_case_id`, `angle`,
  `schema_version`) and `emitted_at`.

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
- **Completeness:** matchers with `expected: true` define the expected observation keys.
  If an expected key is never observed during the run, the aggregate is incomplete and
  contributes to a fail/inconclusive verdict with a heal_hint listing the missing keys
  (reusing the existing completeness/rollup path; `ExpectedObservations` in
  `RollupInput` is fed from the `expected` matcher keys).

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
- Identify the auth/header and other inputs a use case requires and bind them from the
  relevant core primitive, extending the core (per §4) when the input is missing.

The harness itself gains no per-library knowledge; it only compiles and runs regexes.

### 6. Migration of the current `expected_observations` work

The `expected_observations` mechanism added in PR #28 is reworked into `signal_matches`:

- `schemas/sensor.yaml`: rename/extend the field to the `signal_matches` shape (§1).
- `internal/sensor/types.go`: `ObservationMatcher` → `SignalMatch` with the new fields.
- `internal/runtime/executor/{signals,step,compose,executor}.go`: matcher compilation,
  per-line synthesis with verdict/captures/heal_hint, and completeness wiring.
- `.harness/sensors/core/run-dev.yaml` (charge-api): migrate `expected_observations`
  to `signal_matches`.
- The Signal-schema changes from PR #28 (`environment` angle, empty `use_case_id` for
  core sensors) are retained.

## Definition of Done

1. `signal_matches` is accepted by the Sensor schema for both kinds; `expected_observations`
   is removed; the loader parses `SignalMatch` (incl. `verdict`, `confidence`, `expected`,
   `heal_hint`).
2. A matched line synthesizes a schema-valid Signal with the matcher's verdict/confidence,
   named-capture evidence, and a heal_hint whenever verdict is fail/warn.
3. Matching runs on stdout and stderr for all sensors; JSON-signal decoding and the
   no-parse-error-for-plain-text behavior are preserved.
4. `expected: true` matchers drive completeness; a missing expected key yields an
   incomplete aggregate with a heal_hint naming the missing keys.
5. Core primitives expose richer inputs (`e2e-test` has `headers`); a use-case sensor can
   bind them via `with:`, and the create-sensors skills document demand-driven core
   extension and lib-aware regex generation.
6. `run-dev.yaml` uses `signal_matches`; the full test suite passes and the race detector
   is clean on the executor and lifecycle packages.

## Open Questions

- Exact evidence key names for common captures (`status`, `latency`, `path`) — left to
  generation conventions rather than hard-coded.
- Whether `confidence` should default differently for fail matchers (kept at 1 for now).
