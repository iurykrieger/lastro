# B1 — Composed Runtime (Design)

> Source chunk: [`docs/harness-framework/B1-composed-runtime.md`](../../harness-framework/B1-composed-runtime.md)
> Plan references: [`plan.md`](../../harness-framework/plan.md) §6.1 (Fixture Binder), §6.3 (Verdict aggregation), §10.3 (Inferential confidence floor)
> Phase A entities consumed (read-only): E4 (UseCase), E5 (Fixture), E6 (Sensor), E7 (Signal), E8 (AggregateSignal), E9 (ValidationPolicy)
> Brainstorm date: 2026-05-24

## 1. Purpose

B1 owns the two runtime layers that remain unimplemented after Phase A:

1. **Fixture binder** — given a sensor step's `uses:` fixture-id list, write each fixture's payload to disk and expose absolute paths via environment variables, so the step's command can read concrete inputs at execution time.
2. **Per-use-case aggregator** — given the `AggregateSignal`s emitted by every sensor that validated one use case, plus the resolved `EffectivePolicy`, compute a single `UseCaseVerdict` per plan §6.3.

B1 also extends `internal/policy` with the inferential-confidence floor that plan §10.3 deferred to "design later" and E9 explicitly punted.

B1 does **not** own:
- The executor that calls the binder per step (B2).
- The heal-loop orchestration that consumes the verdict (B3).
- The `/validate-use-case` skill that drives the aggregator from a CLI surface (B5).
- Any per-sensor signal aggregation (already in `internal/aggregate`, shipped in E8).
- Any heal-hint synthesis (already in `internal/aggregate/synthesize.go`).

## 2. Scope

**In:**

- New package `internal/runtime/fixturebinder/` — `Binder`, `StepBinding`, `Bind`, `BindError`.
- New package `internal/runtime/aggregator/usecase/` — `UseCase`, `UseCaseVerdict`.
- One new field on `policy.EffectivePolicy` — `InferentialFloor float64`, plus loader + resolver + serializer + drift-test updates.
- Test fixtures covering the deliverable acceptance criteria from the source chunk plus the additional cases this design surfaces.

**Out:**

- Scratch-directory lifecycle (creation, cleanup) — B2.
- Stdout / stderr / signal-stream capture — B2.
- Template interpolation of `step.Run` command text — B2.
- Re-validation orchestration after a heal cycle — B3.
- Sensor / fixture regeneration policy — out of B-phase scope.

## 3. Open questions resolved

The source chunk listed six open questions. Brainstorming on 2026-05-24 resolved each:

| # | Question | Decision |
|---|---|---|
| 1 | Fixture exposure surface | **Files only.** Every fixture is written to disk; `HARNESS_FIXTURE_<NORMALIZED_ID>` env var holds the absolute path. No size threshold, no inline payloads, binary works for free. |
| 2 | Template tokens inside fixture payloads | **No.** Payloads are opaque bytes written verbatim. The template resolver runs only on use-case text (already wired in Phase A). |
| 3 | Verdict weighting formula | **Plan §6.3 holds.** `weight = 1.0` for computational, `weight = signal.Confidence` for inferential. |
| 3a | Weighting scope (added during brainstorm) | **All applicable signals contribute** — both obligatory and optional angles. Inconclusive signals contribute their weight. Disabled-angle signals (if leaked through) are ignored. |
| 4 | Inferential confidence floor | **Global field `EffectivePolicy.InferentialFloor` (default 0.7).** Applied only at the use-case aggregator: an inferential `AggregateSignal` with `confidence < floor` is treated as `verdict = inconclusive` for the verdict computation; the underlying `AggregateSignal` is **not mutated**. `internal/aggregate` stays policy-free. |
| 5 | Failure-first vs all-results | **Evaluate every signal.** The heal loop needs the full failure surface. |
| 6 | `AggregatedHint` shape | **List, not consolidated.** `UseCaseVerdict.HealHints []aggregate.HealHint`, in canonical angle order, one per failing angle. Consolidation would lose locus precision. |
| – | Missing obligatory signal (surfaced during brainstorm) | **Not a real case under normal operation.** The executor invariant is "every sensor process emits exactly one terminal `AggregateSignal`, carrying its exit code as a fail verdict if no JSONL was produced." If a signal is genuinely absent, the aggregator returns an error — caller bug, not a verdict outcome. |

## 4. Package layout

```
internal/runtime/                       (created here; does not exist yet)
├── fixturebinder/
│   ├── binder.go                       Binder struct + Bind method
│   ├── types.go                        StepBinding, BindError
│   └── *_test.go
└── aggregator/
    └── usecase/
        ├── aggregator.go               UseCase function
        ├── types.go                    UseCaseVerdict
        └── *_test.go
```

**Dependencies:**

| Package | Imports |
|---|---|
| `fixturebinder` | `internal/fixture`, `internal/sensor`, `internal/usecase` |
| `aggregator/usecase` | `internal/aggregate`, `internal/policy`, `internal/enums`, `internal/usecase`, `internal/sensor` |

Both packages are leaves under `internal/runtime/`. No Phase A package imports them. No cyclic dependencies.

## 5. Fixture binder

### 5.1 API

```go
package fixturebinder

// StepBinding is the resolved per-step view a sensor step's executor consumes.
type StepBinding struct {
    // Env maps HARNESS_FIXTURE_<NORMALIZED_ID> -> absolute file path.
    Env map[string]string
    // Files maps fixture id -> absolute file path. For diagnostics/tests.
    Files map[string]string
    // BoundIDs is the canonical-ordered list of bound fixture ids.
    BoundIDs []string
}

// Binder writes fixture payloads to disk under ScratchDir.
// The caller owns ScratchDir's lifecycle (mkdir + cleanup); the binder does not.
type Binder struct {
    ScratchDir string // absolute path; must exist when Bind is called
}

// Bind resolves a step's `uses:` fixture ids against the use case's owned
// fixtures, writes each payload to ScratchDir, and returns a StepBinding.
func (b *Binder) Bind(
    step sensor.Step,
    owningUseCase *usecase.UseCase,
    store fixture.FixtureStore,
) (StepBinding, error)
```

### 5.2 Behavior

1. For each id in `step.Uses`:
   - If id is not in `owningUseCase.FixtureIDs` → `*BindError{Code: "fixture-not-owned"}`. Closes plan §2.1 invariant "fixtures bind per-step within the sensor's owning use case."
   - If `store.LookupFixture(id)` returns `false` → `*BindError{Code: "fixture-not-found"}`.
2. For each owned fixture, write the payload to `<ScratchDir>/<fixture-id><ext>`:
   - `<ext>` derives from `Fixture.ContentType`:
     - `application/json` or any `+json` suffix → `.json`
     - `application/yaml`, `text/yaml`, `application/x-yaml` → `.yaml`
     - `application/xml`, `text/xml`, or any `+xml` suffix → `.xml`
     - anything else → `.bin`
   - Payload bytes written verbatim. No template interpolation (per §3 question 2).
   - File write failures → `*BindError{Code: "write-failed", Cause: err}`.
3. Populate `StepBinding`:
   - `Env[normalizedName(id)] = <absolute path>` where `normalizedName("login-basic") = "HARNESS_FIXTURE_LOGIN_BASIC"` (uppercase, hyphens → underscores; fixture-id regex `^[a-z][a-z0-9-]*$` from E5 guarantees the result is a valid POSIX env-var name).
   - `Files[id] = <absolute path>`.
   - `BoundIDs` sorted ascending for deterministic iteration.
4. Empty `step.Uses` → returns `StepBinding{Env: {}, Files: {}, BoundIDs: []}` (all maps non-nil), no error.

Env-var name collisions are structurally impossible: fixture ids are unique strings, and the normalization is a deterministic 1:1 mapping over the id regex.

### 5.3 Errors

```go
type BindError struct {
    Code      string // "fixture-not-found" | "fixture-not-owned" | "write-failed"
    FixtureID string
    UseCaseID string
    Cause     error  // populated for write-failed; nil otherwise
}

func (e *BindError) Error() string { /* "fixturebinder: <code>: ..." */ }
func (e *BindError) Unwrap() error { return e.Cause }
```

### 5.4 Out of scope (binder)

- `step.Run` text expansion — opaque to the binder; B2's executor decides whether to template-expand it.
- ScratchDir creation, cleanup, and runtime-tree layout — owned by B2 per §8 below.
- The original B1 sketch threaded a `*template.Resolver` argument; this is dropped because §3 question 2 ruled out template-in-payload.

## 6. Per-use-case aggregator

### 6.1 API

```go
package aggregator // internal/runtime/aggregator/usecase

// AngleHint pairs a non-pass verdict with its angle and heal hint.
// Used to surface warn and fail signals from one use case in one slice.
type AngleHint struct {
    Angle   enums.ValidationAngle
    Verdict enums.Verdict        // either warn or fail (never pass or inconclusive)
    Hint    aggregate.HealHint
}

type UseCaseVerdict struct {
    UseCaseID           string
    Archetype           enums.Archetype
    Verdict             enums.Verdict           // pass | fail | inconclusive (use-case level; warn lives only at signal level)
    Confidence          float64                  // weighted average, [0.0, 1.0]
    ObligatorySatisfied bool                     // true iff every obligatory effective verdict in {pass, warn}
    EvaluatedAngles     []enums.ValidationAngle  // every obligatory+optional angle that contributed
    FailingAngles       []enums.ValidationAngle  // angles whose post-floor verdict == fail (canonical order)
    WarningAngles       []enums.ValidationAngle  // angles whose post-floor verdict == warn (canonical order)
    HealHints           []AngleHint              // one per fail + warn, in canonical angle order
}

func UseCase(
    uc *usecase.UseCase,
    archetype enums.Archetype,
    signals []aggregate.AggregateSignal,
    sensors []sensor.Sensor,           // for Nature lookup (computational vs inferential)
    pol *policy.EffectivePolicy,
) (UseCaseVerdict, error)
```

`sensors` is passed as an argument rather than threading `Nature` into the `AggregateSignal` schema — E8 is frozen and B5 (the caller) already has these in scope. The aggregator builds a `map[sensorID]enums.SensorNature` internally.

**Why warn is signal-level only:** E8's brainstorm introduced `warn` as a "pass-grade outcome with concerns worth addressing." At the use-case level, warn collapses into pass for verdict gating (warn never fails a use case), but the affected angles are surfaced via `WarningAngles` so the heal loop can still propose fixes for non-blocking issues. The use-case verdict enum stays `{pass, fail, inconclusive}` per plan §6.3 — no plan amendment required.

### 6.2 Algorithm

```
1. Validate inputs:
   - If archetype is not in uc.ArchetypeScope:
       return error "archetype-not-in-scope"
   - If any signal.UseCaseID != uc.ID:
       return error "signal-foreign-use-case"
   - If any (use-case-id, angle) tuple appears more than once across signals:
       return error "duplicate-angle-signal"

2. Resolve angle statuses from pol.PerArchetype[archetype]:
   - statusByAngle: map angle -> {obligatory | optional | disabled}
   - For each angle whose status != disabled:
       - find the matching signal by angle
       - if not found AND status == obligatory:
           return error "missing-obligatory-signal"
       - if not found AND status == optional:
           skip silently (optional angles need not produce a signal)

3. Walk signals in enums.AllAngles() canonical order.
   For each (signal, status):
   - if status == disabled: skip (defensive)
   - look up Nature via sensors -> map[sensor-id]Nature
   - compute effective_verdict:
       if Nature == inferential AND signal.Confidence < pol.InferentialFloor:
           effective_verdict = inconclusive
           (applies uniformly: low-confidence inferential warn AND fail both become inconclusive)
       else:
           effective_verdict = signal.Verdict
   - append signal.Angle to EvaluatedAngles
   - if effective_verdict == fail:
       append signal.Angle to FailingAngles
       append AngleHint{Angle, fail, *signal.HealHint} to HealHints
   - else if effective_verdict == warn:
       append signal.Angle to WarningAngles
       append AngleHint{Angle, warn, *signal.HealHint} to HealHints
   (For both warn and fail, signal.HealHint must be non-nil per E8's invariant.
    The aggregator validates this and returns an error if violated.)

4. Verdict (plan §6.3; warn is pass-grade at the use-case level):
   - if any obligatory signal's effective_verdict == fail:
       Verdict = fail
   - else if every obligatory signal's effective_verdict in {pass, warn}:
       Verdict = pass
   - else:
       Verdict = inconclusive

5. ObligatorySatisfied = (Verdict == pass)

6. Confidence (plan §6.3, weighted average — uses RAW signal.Confidence,
   NOT the floor-demoted verdict; floor demotion affects verdict only):
   for each signal s in EvaluatedAngles:
       weight = 1.0 if Nature[s] == computational else s.Confidence
       value  = s.Confidence
   if sum(weight) == 0:
       Confidence = 0.0
   else:
       Confidence = sum(weight * value) / sum(weight)

7. Return UseCaseVerdict.
```

**Floor demotion is verdict-only.** A signal whose verdict was demoted from `fail` to `inconclusive` by the floor still contributes its original `signal.Confidence` to the weighted average. The aggregator never mutates the underlying `AggregateSignal`.

### 6.3 Worked example

Use case `login`, archetype `http-api`. Policy:
- obligatory: `build`, `unit-test`, `e2e-test`
- optional: `security`

Signals:
| Angle | Nature | Verdict | Confidence |
|---|---|---|---|
| `build` | computational | pass | 1.0 |
| `unit-test` | computational | pass | 1.0 |
| `e2e-test` | inferential | fail | 0.5 |
| `security` | inferential | pass | 0.9 |

With `InferentialFloor = 0.7`:
- `e2e-test`: confidence 0.5 < floor 0.7 → effective verdict = **inconclusive** (not fail).
- No obligatory signal is `fail`. Not every obligatory is `pass` (e2e-test is inconclusive). → `Verdict = inconclusive`, `ObligatorySatisfied = false`.
- `FailingAngles = []`, `HealHints = []`.
- Confidence weights: `build`=1.0, `unit-test`=1.0, `e2e-test`=0.5, `security`=0.9. Sum = 3.4. Sum of `weight*value` = 1.0·1.0 + 1.0·1.0 + 0.5·0.5 + 0.9·0.9 = 1.0 + 1.0 + 0.25 + 0.81 = 3.06. **Confidence ≈ 0.9.**

Now flip `e2e-test` confidence to 0.95:
- 0.95 ≥ 0.7 → effective verdict stays `fail`.
- `Verdict = fail` (any obligatory fail), `FailingAngles = [e2e-test]`, `HealHints = [signal.HealHint]`, `ObligatorySatisfied = false`.
- Confidence weights: `build`=1.0, `unit-test`=1.0, `e2e-test`=0.95, `security`=0.9. Sum = 3.85. Sum of weighted = 1.0 + 1.0 + 0.9025 + 0.81 = 3.7125. **Confidence ≈ 0.964.**

## 7. `EffectivePolicy` extension

### 7.1 Schema change (`schemas/validation-policy.yaml`)

Add an optional top-level property to the source `ValidationPolicy`:

```yaml
inferential_floor:
  type: number
  minimum: 0.0
  maximum: 1.0
  description: |
    Minimum confidence below which an inferential sensor's verdict is
    treated as inconclusive when computing the use-case verdict.
    Defaults to 0.7 when omitted at every scope. Computational sensors
    are unaffected.
```

### 7.2 Source policy field (nullable to distinguish "unset" from "explicitly 0.0")

```go
package policy

// ValidationPolicy (source form, loaded from YAML)
type ValidationPolicy struct {
    SchemaVersion    string
    Scope            Scope
    PerArchetype     map[enums.Archetype]ArchetypeBlock
    InferentialFloor *float64 // nil = field omitted in YAML; non-nil = explicit value
}
```

Pointer form is required because `float64` has no sentinel value distinguishable from a legitimate `0.0`. The loader unmarshal path:
- YAML field absent → `InferentialFloor` stays `nil`.
- YAML field present (even as `0.0` or `1.0`) → `InferentialFloor` points to the parsed value.

### 7.3 Resolved policy field (always populated)

```go
package policy

const DefaultInferentialFloor = 0.7

type EffectivePolicy struct {
    SchemaVersion    string
    ResolvedFrom     []string
    PerArchetype     map[enums.Archetype]map[enums.ValidationAngle]AngleStatus
    InferentialFloor float64 // always populated post-Resolve; never the zero value by accident
}
```

### 7.4 Resolution rules (`internal/policy/resolve.go`)

- Local scope's `InferentialFloor` (when non-nil) overrides global's. Matches existing per-(archetype, angle) override granularity.
- If both source `InferentialFloor` pointers are nil, `EffectivePolicy.InferentialFloor = DefaultInferentialFloor` (0.7).
- The loader rejects values outside `[0.0, 1.0]` (JSON Schema `minimum`/`maximum` enforces this at parse time).

`EffectivePolicy.MarshalYAML` (in `internal/policy/serialize.go`) emits the field. Drift tests update accordingly.

## 8. Runtime directory tree convention (documented for B2)

B1 does not implement this layout, but the spec captures it so B2 has a target. Convention:

```
.harness/                              (gitignored)
└── runtime/
    └── <sensor-id>/
        └── <run-id>/                  run-id = monotonic timestamp + short hash
            ├── fixtures/              ScratchDir passed to fixturebinder.Bind
            │   └── <fixture-id>.<ext>
            ├── stdout.log             raw process stdout
            ├── stderr.log             raw process stderr
            ├── signals.jsonl          validated Signal stream as parsed
            └── aggregate.json         terminal AggregateSignal as written
```

B1's tests use `t.TempDir()` for `ScratchDir`. The on-disk layout above is a B2 concern; the binder itself stays path-agnostic.

## 9. Determinism guarantees

- `fixturebinder.Bind`: `BoundIDs` sorted ascending; file contents are byte-equal to `Fixture.Payload`. `Env` and `Files` map iteration order is not part of the contract — callers don't depend on it.
- `aggregator.UseCase`:
  - Signals walked in `enums.AllAngles()` order.
  - `EvaluatedAngles`, `FailingAngles`, `HealHints` produced in canonical angle order; `HealHints[i]` always corresponds to `FailingAngles[i]`.
  - Confidence computed with stable accumulation: build `sum(weight*value)` and `sum(weight)` first, then divide once. No successive divisions.
  - JSON-marshaled `UseCaseVerdict` is byte-identical across runs given identical inputs.

## 10. Test matrix

| Package | Test | Verifies |
|---|---|---|
| `fixturebinder` | bind two fixtures (json + binary) | env vars set with canonical names, files on disk byte-equal to payloads, extensions correct |
| `fixturebinder` | bind unknown fixture id | `BindError{Code:"fixture-not-found"}` |
| `fixturebinder` | bind fixture not owned by use case | `BindError{Code:"fixture-not-owned"}` |
| `fixturebinder` | empty `step.Uses` | empty `StepBinding`, no error, maps non-nil |
| `fixturebinder` | write failure (read-only ScratchDir) | `BindError{Code:"write-failed", Cause: ...}` |
| `fixturebinder` | `BoundIDs` sorted across multiple Bind calls with same step | deterministic ordering |
| `aggregator/usecase` | 3 obligatory all pass | `Verdict=pass`, `ObligatorySatisfied=true`, `FailingAngles=[]` |
| `aggregator/usecase` | 1 obligatory fail | `Verdict=fail`, `FailingAngles` populated, `HealHints` non-empty |
| `aggregator/usecase` | only optional fails | `Verdict=pass`, `FailingAngles` populated with the optional angle |
| `aggregator/usecase` | obligatory warn (computational) | `Verdict=pass`, `ObligatorySatisfied=true`, `WarningAngles` populated, `HealHints` has one entry with `Verdict=warn` |
| `aggregator/usecase` | obligatory warn + obligatory fail | `Verdict=fail`, `FailingAngles` and `WarningAngles` both populated |
| `aggregator/usecase` | inferential signal below floor (verdict=fail) | effective verdict = inconclusive; not in FailingAngles |
| `aggregator/usecase` | inferential signal below floor (verdict=warn) | effective verdict = inconclusive; not in WarningAngles |
| `aggregator/usecase` | mixed computational + inferential | weighted confidence matches hand-calculated value (§6.3 worked example) |
| `aggregator/usecase` | duplicate angle signal | error `"duplicate-angle-signal"` |
| `aggregator/usecase` | signal foreign to use case | error `"signal-foreign-use-case"` |
| `aggregator/usecase` | archetype not in use case scope | error `"archetype-not-in-scope"` |
| `aggregator/usecase` | missing obligatory signal | error `"missing-obligatory-signal"` |
| `aggregator/usecase` | fail signal with nil HealHint | error (E8 invariant violation) |
| `aggregator/usecase` | golden — fixed inputs → byte-identical JSON | determinism |
| `policy` | global sets floor, local omits | effective = global's |
| `policy` | local sets floor, global omits | effective = local's |
| `policy` | both set | effective = local's (local wins) |
| `policy` | neither sets | effective = `DefaultInferentialFloor` |
| `policy` | loader rejects out-of-range | floor `-0.1` or `1.5` → load error |
| both | `go test -race ./internal/runtime/...` | concurrency clean |

## 11. Out of scope (explicit)

| Concern | Owner |
|---|---|
| `step.Run` template interpolation | B2 (executor) |
| `ScratchDir` creation & cleanup | B2 |
| `stdout.log` / `stderr.log` / `signals.jsonl` capture | B2 |
| Per-sensor `Rollup` (signals → AggregateSignal) | `internal/aggregate` (Phase A) |
| Heal-hint synthesis (per-signal) | `internal/aggregate/synthesize.go` (Phase A) |
| Heal-loop orchestration | B3 |
| `/validate-use-case` skill | B5 |
| Stream signal parsing | `internal/signal` (Phase A) |
| Two-scope policy resolution | `internal/policy` (Phase A) — B1 extends with one field |
| Sensor DAG topo sort | `internal/sensor.ResolveExecutionOrder` (Phase A) |
| Detection / generation | B4 (parallel chunk) |

## 12. Acceptance criteria

- `fixturebinder.Bind` resolves a step with 2 fixtures (one JSON, one binary), surfaces them via env + file, errors cleanly on `fixture-not-found` and `fixture-not-owned`.
- `aggregator.UseCase` correctly applies the policy: 3 obligatory all pass → `pass`; one obligatory fails → `fail`; only optional fails → `pass` with `FailingAngles` populated.
- The inferential floor demotes a fail-with-low-confidence to inconclusive at the use-case level without mutating the underlying `AggregateSignal`.
- Golden test: fixed use case + policy + 3 mocked `AggregateSignal`s → byte-identical `UseCaseVerdict` JSON across runs.
- `policy.Resolve` honors `InferentialFloor` overrides and defaults.
- `go test -race ./internal/runtime/... ./internal/policy/...` passes.

## 13. Parallelism & sequencing

- **Can run in parallel with:** B4 (detection + generation — separate code path).
- **Must run after:** Phase A (✓ — all entities frozen).
- **Blocks:** B2 (executor calls fixture binder per step), B3 (heal loop calls aggregator for re-validation), B5 (`/validate-use-case` invokes aggregator).

## 14. Branching

```bash
git fetch origin
git checkout -b feat/b1-composed-runtime origin/main
```
