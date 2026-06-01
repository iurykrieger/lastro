# Harness Framework — Implementation Plan

> A use-case-driven validation framework that detects what an application *is* and *does*, then dynamically synthesizes angle-specific sensors to validate it.

---

## 1. Core Philosophy

- **Use cases are the problem.** Behavioral specifications in `given / when / then` form. Tech-agnostic in their *text*, but allowed to **reference** archetype-typed `entry_points` and `fixtures` via `${{ }}` interpolation. Use cases describe *what* the application does and *where* its observable surface lives. They are not tests, they do not orchestrate steps, and they make no assertions — they only describe behavioral criteria.
- **Fixtures are the concrete proof.** Input/output payloads generated alongside each use case, derived by reverse-engineering observable behavior. They are referenceable from use case text via `${{fixtures.<id>}}` and consumed by sensors that decide to bind to them.
- **The stack is the toolbox.** Detected capabilities, libraries, and components — the only materials sensors are allowed to use.
- **Sensors are the solution.** Dynamically generated per `(use case × applicable validation angle)`, grounded in the toolbox via top-level `uses:` (stack components), and binding to fixtures per-step via each step's `uses:`. They do not solve the use case; they *validate* it.
- **Signals are the contract with the LLM.** Semantically rich, structured JSON — never raw logs. Every failure carries a self-healing hint.

---

## 1.1 Implementation Conventions

- **Language:** All framework scripts and code are written in **Go**. Every package has accompanying tests; no untested code lands.
- **Schema filenames are unversioned.** Files are named `signal.yaml`, `sensor.yaml`, `use-case.yaml`, etc. The `schema_version` field lives **inside** the schema. Versioning is data, not filename metadata.
- **One generation skill for sensors.** `create-sensors` (plural) is the only sensor-generation skill. It takes a use case and generates all applicable angle sensors in one pass. No singular `create-sensor`, no filtering — keep the surface small.

---

## 2. Entity Model

| Entity | Nature | Source | Lifecycle |
|---|---|---|---|
| **ValidationAngle** | Fixed enum | Defined by framework | Static |
| **Archetype** | Fixed enum | Defined by framework | Static |
| **StackComponent** | Detected | `detect-stack` skill | Per-repo, re-detected on change |
| **UseCase** | Detected | `detect-use-cases` skill | Per-repo, behavioral spec with structured references |
| **EntryPoint** | Detected | Embedded in `UseCase` | Archetype-typed observable surface identifier (HTTP route, queue, CLI command, etc.) |
| **Fixture** | Generated | `detect-use-cases` skill (alongside use case) | Per use case, versioned with it |
| **Sensor** | Generated | `create-sensor(s)` skill | One per `(UseCase × applicable Angle)` |
| **Signal** | Emitted | Sensor runtime | Per execution |
| **AggregateSignal** | Emitted | Sensor runtime (terminal) | One per sensor run, always emitted at sensor termination |
| **ValidationPolicy** | Configured | Org / global / repo config | Declares per-archetype angle requirements |

### 2.1 Relationships

```
Repository
  ├── archetype:           Archetype             (1)
  ├── stack:               [StackComponent]      (N)
  ├── use_cases:           [UseCase]             (N)
  └── validation_policy:   ValidationPolicy      (1, inherited from org/global, overridable)

UseCase
  ├── entry_points:        [EntryPoint]          (N — archetype-typed observable surfaces)
  ├── source_refs:         [CodeRef]             (N — pointers ONLY, no embedded code)
  ├── fixtures:            [Fixture]             (N — concrete I/O examples, owned)
  └── validated_by:        [Sensor]              (1 per applicable Angle)

EntryPoint (embedded in UseCase)
  ├── archetype:           Archetype             (1, determines spec shape)
  └── spec:                <archetype-specific>  (surface + optional representative fixture)

Fixture
  └── use_case:            UseCase               (1, owning)

Sensor
  ├── angle:               ValidationAngle       (1, immutable)
  ├── use_case:            UseCase               (1, immutable — sensors validate use cases)
  ├── uses:                [StackComponent]      (N — tools picked from the box)
  ├── depends_on:          [Sensor]              (N, optional)
  └── steps:               [SensorStep]          (N — each step may `uses:` fixtures)

Archetype
  └── applicable_angles:   [ValidationAngle]     (N — gates which sensors can exist)

ValidationPolicy
  └── per_archetype:
        <archetype>:
          obligatory_angles: [ValidationAngle]
          optional_angles:   [ValidationAngle]
          disabled_angles:   [ValidationAngle]
```

**Invariants:**
- A use case's `given/when/then` text contains **no** code, regex, library names, or implementation patterns. It may interpolate `${{fixtures.<id>}}` or `${{entry_points.<id>}}` to bind to its structured references.
- A use case **describes behavior**; it does not orchestrate execution. No steps, no `needs:`, no assertions, no test setup. Anything that *runs* belongs to a sensor.
- A use case's `entry_points` are typed by the repo's `archetype` — an `http-api` use case cannot declare a `queue` entry point.
- A use case's `source_refs` contains pointers only — file paths and symbols — never embedded code.
- A sensor cannot reference a `StackComponent` not present in the repo's detected stack. This is what makes sensors *grounded*.
- A sensor binds to fixtures at the **step level** via each step's `uses:` field. No sensor-wide fixture list exists. Steps that don't need a fixture simply omit `uses:`.
- Every sensor emits signals conforming to the canonical `Signal` schema (Section 4.5). The schema is invariant; only the values (verdict, evidence, confidence, etc.) vary per sensor and execution.

---

## 3. Fixed Enums

### 3.1 `ValidationAngle` (the angles of validation)

| ID | Purpose |
|---|---|
| `security` | Secrets, vulnerable deps, SAST findings, sensitive data in logs |
| `build` | Compilation / packaging succeeds |
| `code-structure` | Conformance to architectural / lint patterns |
| `unit-test` | Existence + execution of unit tests |
| `e2e-test` | End-to-end behavior matches fixtures |
| `contracts` | API / schema / SDK contract conformance |
| `logs` | Log shape, redaction, semantic correctness |
| `metrics` | Telemetry emission and shape |
| `database` | Data writes / migrations match expectations |
| `performance` | Latency, throughput, resource ceilings |

> Angles are extensible only by framework version bump, not by user configuration. Users control *which* angles apply via `ValidationPolicy`, not *which angles exist*.

### 3.2 `Archetype`

`http-api`, `event-consumer`, `event-producer`, `cli`, `sdk`, `worker`, `batch-job`, `static-site`, `library`.

Each archetype declares its `applicable_angles` set. Example: a `cli` archetype has no HTTP `e2e-test`, but has `contracts` (flag parsing) and `logs`.

### 3.3 `SensorKind` (lifecycle shape)

- `assertion` — Runs steps, terminates, emits a verdict. (e.g., build, lint, unit test runner)
- `observational` — Spawns, watches a stream (logs, metrics, traces), emits signals continuously, terminates on completion of an observed action.

### 3.4 `SensorNature` (epistemic source)

- `computational` — Deterministic. Pass/fail derives from program output (exit codes, parsed structured data).
- `inferential` — Probabilistic. Pass/fail derives from LLM judgment over evidence. Confidence < 1.0 by definition.

### 3.5 `SignalOutputType`

- `single-shot` — One signal emitted, one verdict.
- `stream` — N signals emitted (e.g., one per test, one per matched log line).

### 3.6 `FixtureRole`

- `input` — Drives the behavior (request payload, CLI args, event message).
- `expected-output` — The asserted output (response payload, emitted event, written DB row).
- `expected-side-effect` — Non-payload effects (log line shape, metric emission).

---

## 4. Schemas

All schemas are YAML, versioned (`schema_version: <semver>`), and live under `/schemas/`.

### 4.1 `UseCase` (behavioral spec with structured references)

A use case describes behavioral criteria. Its only structured fields are `entry_points` (which observable surfaces the behavior involves) and `fixtures` (referenced from text). Everything else is natural-language description of behavior.

```yaml
schema_version: 2.0.0
id: <stable-id>                          # hash of canonical given/when/then + entry_points
title: <human-readable>
archetype_scope: [http-api]              # which archetype this use case is valid under

# --- Structured references ---

entry_points:                            # archetype-typed observable surfaces
  - id: create_order_endpoint
    archetype: http-api
    spec:                                # shape is determined by archetype (see 4.1.1)
      method: POST
      path: /orders

# --- Behavioral criteria (textual, with ${{ }} interpolation) ---

given:
  - "A request payload matching ${{fixtures.fx_create_order_request}} is constructed by the client"
when:
  - "The client invokes ${{entry_points.create_order_endpoint}}"
then:
  - "The endpoint responds with a payload matching ${{fixtures.fx_create_order_response}}"

# --- Provenance ---

source_refs:                             # pointers ONLY — no embedded code
  - path: src/handlers/orders.ts
    symbol: createOrder
    reason: "reverse-engineered to detect this use case"

fixture_ids: [fx_create_order_request, fx_create_order_response]
```

**What is *not* in a use case (intentionally):**
- No `steps`, no `needs:`, no DAG. Execution belongs to sensors.
- No selectors for where to observe a side effect. Sensors decide that based on the stack.
- No assertions or comparators. The `then:` clause states the criterion in natural language; the sensor turns it into a verifiable check.
- No code, regex, library names, or wire-protocol details beyond what an archetype's surface naturally requires (e.g., HTTP method + path).

#### 4.1.1 Archetype-specific `EntryPoint.spec` shapes

`spec` is a discriminated union keyed by `archetype`. Each archetype defines only the fields needed to **identify the observable surface** — nothing more.

| Archetype | Required spec fields |
|---|---|
| `http-api` | `method`, `path` |
| `event-consumer` | `channel_kind` (queue\|topic), `channel_name` |
| `event-producer` | `target_channel_kind`, `target_channel_name` |
| `cli` | `command` |
| `sdk` / `library` | `exported_symbol` |
| `worker` | `trigger_kind` (cron\|signal), `schedule_or_signal` |
| `batch-job` | `input_source`, `output_destination` |
| `static-site` | `route_path` |

Example — event consumer:
```yaml
entry_points:
  - id: on_order_created
    archetype: event-consumer
    spec:
      channel_kind: queue
      channel_name: orders.created

given:
  - "An event matching ${{fixtures.fx_order_created_event}} is published to ${{entry_points.on_order_created}}"
when:
  - "The handler receives the event"
then:
  - "The order is recorded in the application's storage"
```

Example — CLI:
```yaml
entry_points:
  - id: harness_detect_cli
    archetype: cli
    spec:
      command: harness

given:
  - "The user invokes ${{entry_points.harness_detect_cli}} with arguments matching ${{fixtures.fx_detect_args}}"
when:
  - "The command completes"
then:
  - "Standard output matches ${{fixtures.fx_detect_stdout}} and the process exits with code 0"
```

Fixtures are referenced from text, not embedded in the entry point spec. This keeps the entry point a pure surface identifier and lets fixtures be reused freely across multiple positions in the same use case.

#### 4.1.2 `${{ }}` interpolation grammar

Templates resolve against the use case's own structured references at validation time.

| Expression | Resolves to |
|---|---|
| `${{fixtures.<fixture_id>}}` | The fixture's `payload` |
| `${{fixtures.<fixture_id>.<jsonpath>}}` | A drilled-in value within the payload |
| `${{entry_points.<entry_point_id>}}` | The entry point object (rendered as a short label in text contexts) |
| `${{entry_points.<entry_point_id>.spec.<field>}}` | A specific spec field |

**Rules:**
- A template referencing an undefined id is a schema validation error.
- Templates are inert in textual contexts (rendered as labels for humans); they are *resolved to live values* only when consumed by a sensor.
- Templates may not be nested.

### 4.2 `Fixture` (concrete I/O proof of the behavior)

```yaml
schema_version: 1.0.0
id: <stable-id>
use_case_id: <UseCase.id>
role: input | expected-output | expected-side-effect

content_type: application/json   # or xml, text/plain, binary, etc.
payload: |
  { ... }                        # the concrete payload

binding:                         # how this fixture maps to the behavior
  channel: http                  # http, cli-args, event, stdout, log-line, db-row
  selector:                      # archetype-specific addressing
    method: POST
    path: /orders

source_refs:                     # what code shape was reverse-engineered
  - path: src/handlers/orders.ts
    symbol: createOrder
```

**Sharing rule:** The same fixture can be referenced by multiple sensors across angles. The e2e-test sensor uses it as a live request payload; the unit-test sensor uses the same payload to drive an isolated function call; the contracts sensor uses it as a schema example. One fixture, many angles.

### 4.3 `StackComponent`

```yaml
schema_version: 1.0.0
id: <library-or-capability-id>
kind: library | runtime | framework | datastore | protocol | tool
name: express
version: 4.18.x
capabilities: [http-routing, middleware, json-body-parsing]
detection_evidence:
  - package.json:dependencies.express
```

### 4.4 `Sensor`

GitHub-Actions-inspired schema. Steps execute in order; sensors compose via `depends_on`. Fixtures are referenced per-step via each step's `uses:` field — never at sensor level. Every sensor emits signals conforming to the canonical `Signal` schema (4.5); no `emits:` declaration is needed.

```yaml
schema_version: 1.0.0
id: <generated-id>
use_case_id: <UseCase.id>          # sensors validate use cases
angle: e2e-test                    # one ValidationAngle, immutable
kind: assertion                    # assertion | observational
nature: computational              # computational | inferential
output_type: single-shot           # single-shot | stream

uses:                              # StackComponents the sensor draws from
  - <StackComponent.id>            # must exist in the detected stack manifest

depends_on:                        # other sensors that must pass first
  - <Sensor.id>

steps:
  - id: spin-up
    run: <command-or-skill-invocation>

  - id: probe
    run: <command>
    uses:                          # fixtures referenced by this step
      - <Fixture.id>               # e.g., the input payload to send

  - id: assert
    run: <comparator>
    uses:
      - <Fixture.id>               # e.g., the expected-output fixture to compare against
```

**Schema rules:**
- Top-level `uses:` lists **StackComponent ids only** — the toolbox subset this sensor depends on. All ids must exist in the detected stack manifest (grounding invariant).
- Step-level `uses:` lists **Fixture ids only** — the fixtures that step needs to do its work. All ids must exist on the sensor's `use_case`.
- Steps without `uses:` are valid (e.g., a `build` step that needs no fixture).
- No `fixtures:` block at sensor level. No `emits:` block. Both were redundant.

### 4.5 `Signal` (JSON Lines, one per line)

```json
{
  "schema_version": "1.0.0",
  "sensor_id": "<Sensor.id>",
  "use_case_id": "<UseCase.id>",
  "angle": "e2e-test",
  "emitted_at": "<iso8601>",
  "verdict": "pass | fail | inconclusive",
  "confidence": 0.0,
  "evidence": {
    "expected": "...",
    "actual": "...",
    "fixture_id": "<Fixture.id>"             // optional, only if sensor bound to a fixture
  },
  "heal_hint": {
    "summary": "<one-line actionable instruction>",
    "suggested_locus": [
      { "path": "src/...", "symbol": "..." }
    ],
    "rationale": "<short, structured — not raw logs>"
  }
}
```

**Rule:** `heal_hint` is **required** when `verdict = fail`. Its purpose is to compress feedback into LLM-ingestible action, not to log diagnostics.

### 4.6 `AggregateSignal` (terminal, always emitted)

Every sensor execution ends with exactly one `AggregateSignal`, regardless of `output_type`. For single-shot sensors, it is identical in spirit to the single signal but framed as the terminal record. For stream sensors, it rolls up all emitted signals into counts and a verdict. For observational sensors, it is emitted at stop time and reports whether the sensor's expected observations were complete.

```json
{
  "schema_version": "1.0.0",
  "type": "aggregate",
  "sensor_id": "<Sensor.id>",
  "use_case_id": "<UseCase.id>",
  "angle": "unit-test",
  "started_at": "<iso8601>",
  "ended_at": "<iso8601>",
  "termination_reason": "completed | stopped | timeout | error",

  "verdict": "pass | fail | inconclusive",
  "confidence": 0.0,

  "rollup": {
    "total_signals": 700,
    "pass_count": 630,
    "fail_count": 70,
    "inconclusive_count": 0
  },

  "completeness": {                            // for observational sensors
    "expected_observations": ["app-started", "log-emitted", "metric-scraped"],
    "missing_observations": []
  },

  "heal_hint": {                               // required when verdict = fail
    "summary": "70 of 700 unit tests failed",
    "suggested_locus": [
      { "path": "src/...", "symbol": "..." }
    ],
    "rationale": "<structured summary; individual failure signals carry the detail>"
  }
}
```

**Rules:**
- `AggregateSignal` is always the **last** record emitted by a sensor.
- For single-shot sensors, `rollup.total_signals == 1` and the aggregate verdict equals the sole signal's verdict.
- For stream sensors, `verdict = pass` only if `fail_count == 0` and `inconclusive_count == 0`.
- For observational sensors, `verdict = fail` if `missing_observations` is non-empty (e.g., the sensor never received the logs it was supposed to observe) — this distinguishes "the sensor worked and observed no problems" from "the sensor failed to do its job."
- `heal_hint` is required when `verdict = fail`, same rule as `Signal`.

### 4.7 `ValidationPolicy` (org / global / repo override)

```yaml
schema_version: 1.0.0
scope: org | global | repo
inherits_from: <policy-id>       # optional

per_archetype:
  http-api:
    obligatory_angles: [build, security, unit-test, e2e-test, contracts]
    optional_angles:   [performance, metrics, logs]
    disabled_angles:   []
  cli:
    obligatory_angles: [build, security, contracts]
    optional_angles:   [unit-test, logs]
    disabled_angles:   [e2e-test, database]
```

**Resolution order:** repo overrides org overrides global. Disabled angles must be explicit.

---

## 5. Skills (AI Primitives)

Each skill is invokable as a Claude Code slash command and has an explicit input/output contract.

| Skill | Input | Output | Determinism |
|---|---|---|---|
| `/detect-stack` | repo root | `stack-manifest.yaml` (includes `archetype` field) | inferential, cacheable |
| `/detect-use-cases` | repo root, `stack-manifest.yaml` | `use-cases/*.yaml` (with entry points) **+ `fixtures/*.yaml`** | inferential, paired output |
| `/create-sensors` | `UseCase`, `ValidationPolicy`, `stack-manifest.yaml` | N `Sensor` yamls — one per applicable angle, in one pass | inferential generation, deterministic schema |
| `/run-sensor` | `Sensor.id` (kind: assertion) | Signal stream + terminal `AggregateSignal`; blocks until done | synchronous execution |
| `/start-sensor` | `Sensor.id` (kind: observational) | sensor handle; spawns the watcher | asynchronous spawn |
| `/stop-sensor` | sensor handle | terminal `AggregateSignal` reporting observation completeness | synchronous stop |
| `/validate-use-case` | `UseCase.id` | aggregated signals across all sensors + use-case verdict | orchestrates run/start/stop |
| `/heal` | failing `Signal` or `AggregateSignal` | code edit proposal | consumes `heal_hint` |

**Notes:**
- There is no singular `/create-sensor`. `create-sensors` fans out across all applicable angles in one call.
- `/run-sensor` is for `kind: assertion` sensors only. Stream-output assertion sensors (e.g., unit tests) still complete synchronously and emit individual signals followed by one `AggregateSignal`.
- `/start-sensor` and `/stop-sensor` are for `kind: observational` sensors. The `AggregateSignal` is emitted by `stop-sensor`, not at sensor launch.

**Critical:** `/detect-use-cases` emits both use cases and their fixtures atomically. A use case without fixtures is incomplete and rejected by the schema validator.

**Skill ordering for a fresh repo:**
```
detect-stack → detect-use-cases (+ fixtures)
  → (for each use case) create-sensors
  → validate-use-case
  → (on failure) heal → re-validate
```

---

## 6. Sensor Execution Model

### 6.1 Runtime components

- **Resolver** — Topologically sorts sensors by `depends_on`, fails fast on cycles.
- **Template Resolver** — Resolves `${{ }}` interpolations in use case text against fixtures and entry points at sensor execution time.
- **Executor** — Runs `assertion` sensors to completion via `/run-sensor`; spawns `observational` sensors as long-lived watchers via `/start-sensor` and terminates them via `/stop-sensor`.
- **Fixture Binder** — At step execution time, resolves each step's `uses:` fixture ids to concrete payloads and injects them into the step's command environment.
- **Signal Collector** — Reads JSON Lines from sensor stdout, validates against schema, stores per-execution.
- **Aggregator** — Two layers: (1) **per-sensor aggregation** emits the terminal `AggregateSignal` at the end of every sensor run (rolls up streamed signals, reports observational completeness); (2) **per-use-case aggregation** consumes the AggregateSignals from all sensors validating one use case and computes the use case verdict, weighted confidence, and obligatory-angle satisfaction.
- **Heal Loop** — Feeds failing signals' `heal_hint` blocks back to the LLM for code edits, then re-runs only affected sensors.

### 6.2 Observational sensors

Spawned via `/start-sensor`, terminated via `/stop-sensor`:

```
start-sensor → register watcher (log tail, metrics scrape, etc.)
             → emit signals as patterns match (regex / structured matchers
               derived from detected log library)
             → continue until:
                 (a) /stop-sensor is invoked
                 (b) end of observed action
                 (c) timeout
stop-sensor  → terminate watcher
             → emit one terminal AggregateSignal:
                 verdict = pass         if all expected_observations were seen
                 verdict = fail         if any expected_observations are missing
                                        (e.g., logs could not be fetched,
                                         application failed to start)
                 verdict = inconclusive if termination was due to timeout
                                        without clear observation coverage
```

Pattern derivation is grounded: the stack manifest declares the log library, so the framework knows the log line shape and can generate matchers without guessing.

### 6.3 Verdict aggregation

```
# Inputs: one AggregateSignal per sensor validating this use case.

use_case.verdict =
  pass         if all obligatory-angle sensors' AggregateSignal.verdict = pass
  fail         if any obligatory-angle sensor's AggregateSignal.verdict = fail
  inconclusive otherwise

confidence = weighted average of AggregateSignal.confidence values,
             weighted by (nature: computational=1.0, inferential=aggregate.confidence)
```

---

## 7. Versioning & Evolvability

- Every schema carries `schema_version` (semver).
- Angles, archetypes, sensor kinds, and natures are versioned as part of the framework release.
- Generated sensors and fixtures are pinned to the schema version at creation time; runtime tolerates older instances via migrations.
- A `framework.lock` file in each repo records the schema versions in use.

---

## 8. Repository Layout (proposed)

```
harness-framework/
├── go.mod
├── go.sum
├── cmd/
│   └── harness/                      # CLI entry point (main package)
├── schemas/                          # unversioned filenames; schema_version is inside
│   ├── use-case.yaml
│   ├── fixture.yaml
│   ├── entry-point.yaml
│   ├── stack-component.yaml
│   ├── sensor.yaml
│   ├── signal.yaml
│   ├── aggregate-signal.yaml
│   ├── validation-policy.yaml
│   └── enums/
│       ├── validation-angles.yaml
│       ├── archetypes.yaml
│       ├── sensor-kinds.yaml
│       ├── sensor-natures.yaml
│       └── fixture-roles.yaml
├── internal/                         # Go packages
│   ├── detect/                       # detect-stack (stack + archetype), detect-use-cases
│   ├── sensors/                      # create-sensors generation logic
│   ├── runtime/
│   │   ├── resolver/
│   │   ├── template/                 # ${{ }} interpolation
│   │   ├── executor/
│   │   ├── fixtureBinder/
│   │   ├── signalCollector/
│   │   ├── aggregator/               # per-sensor + per-use-case
│   │   └── healLoop/
│   ├── lifecycle/                    # run-sensor, start-sensor, stop-sensor
│   ├── schema/                       # schema loading + validation
│   └── policy/                       # ValidationPolicy resolution
├── skills/                           # AI primitive definitions (slash commands)
│   ├── detect-stack/
│   ├── detect-use-cases/
│   ├── create-sensors/
│   ├── run-sensor/
│   ├── start-sensor/
│   ├── stop-sensor/
│   ├── validate-use-case/
│   └── heal/
└── examples/
    ├── http-api-sample/
    └── cli-sample/
```

**Go conventions:**
- Every package under `internal/` has a sibling `_test.go` file covering its exported surface.
- No package lands without tests.
- Schema loaders use struct tags to deserialize YAML into typed Go structs; `schema_version` validation happens at load time.

---

## 9. Phased Roadmap

### Phase 0 — Foundations
- Lock the entity model (this document).
- Author all schemas under `/schemas/` with one passing example each.
- Define the fixed enums as YAML files (single source of truth for code generation).

### Phase 1 — Detection skills
- `/detect-stack` (must be reliable before anything else; emits the full stack manifest including the detected `archetype` field — everything downstream is grounded in its output).
- `/detect-use-cases` — emits paired use cases + fixtures.

### Phase 2 — Sensor generation
- `/create-sensors` — single fan-out skill, one pass per use case.
- Validation that generated sensors only reference detected stack components at top-level `uses:`.
- Validation that step-level `uses:` references only fixtures owned by the use case.

### Phase 3 — Runtime (assertion path)
- Resolver + Go executor.
- Fixture binder.
- Signal collector + schema validation.
- Per-sensor aggregator (emits terminal `AggregateSignal`).
- `/run-sensor` skill — synchronous execution of assertion sensors.
- Per-use-case aggregator with obligatory/optional angle gating.

### Phase 4 — Observational sensors
- `/start-sensor` + `/stop-sensor` skills.
- Long-lived sensor lifecycle with handle management.
- Log-pattern derivation from detected log library.
- Stream signal handling.
- `AggregateSignal.completeness` reporting (missing observations → fail).

### Phase 5 — Heal loop
- `/heal` skill consuming `heal_hint`.
- Selective re-validation of affected sensors only.

### Phase 6 — DX & ergonomics
- CLI (`harness detect`, `harness validate`, `harness heal`).
- Reports.
- Caching of detection outputs.

---

## 10. Open Design Decisions (to resolve before Phase 0 closes)

1. **Sensor regeneration policy.** When the stack changes, do we regenerate all sensors, or diff and regenerate only affected ones? Recommendation: diff-based, with a `--force` escape hatch.
2. **Fixture regeneration policy.** When source code changes, do fixtures auto-regenerate? Risk: drift between fixture and use case if regeneration is automatic; risk: stale fixtures if it's manual. Recommendation: regenerate on `source_refs` change, prompt on use case text change.
3. **Inferential confidence floor.** Below what confidence does an inferential sensor's verdict get treated as `inconclusive` rather than `pass`/`fail`? Suggest `0.7` as default, configurable per `ValidationPolicy`.
4. **Heal loop termination.** Max iterations before a failing use case is reported as unhealable. Suggest `3`.
5. **Sensor identity.** Is a sensor's `id` content-hash-based (so identical generations dedupe) or generation-UUID-based (so each generation is auditable)? Recommendation: content hash + generation metadata sidecar.
6. **Existing repo migration.** Does the current `harness-framework` repo get refactored in-place, or is a clean repo created and old code archived? Pending repo inspection.

---

## 11. Acceptance Criteria for "Plan Implemented"

The framework is considered functionally complete when, on a fresh sample repo:

1. `/detect-stack` produces a manifest covering ≥95% of declared dependencies and a single `archetype` value with rationale.
2. `/detect-use-cases` produces ≥1 valid use case per public entry point, each with: a `given/when/then` block, at least one archetype-typed `entry_point`, and ≥1 fixture referenced from the text.
3. `${{ }}` interpolation in use case text resolves cleanly: every referenced fixture and entry point id exists in the same use case.
4. `/create-sensors` produces one sensor per `(use case × applicable obligatory angle)`, all schema-valid, all top-level `uses:` referencing only detected stack components, and any step-level `uses:` referencing only fixtures owned by the sensor's use case.
5. `/validate-use-case` executes the sensor graph and produces aggregated signals.
6. A deliberately broken sample emits a failing signal with a `heal_hint` actionable enough that `/heal` produces a fix on the first attempt.
7. The same fixture is demonstrably reused across at least two angles (e.g., e2e-test and unit-test) for the same use case.
