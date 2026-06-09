# Shared observational services with signal attach

**Status:** Approved design — ready for implementation planning
**Date:** 2026-06-08
**Issue:** [#33 — logs sensor spawns a duplicate `next dev`, colliding on `.next/dev/lock` → inconclusive](https://github.com/iurykrieger/lastro/issues/33)

## 1. Problem & root cause

During `validate-use-case`, `run-dev` (a host-exclusive, long-lived dev server that
holds Next.js's `.next/dev/lock`) is instantiated through **two uncoordinated paths**:

1. **Scheduler path** — `GatherForUseCase` (`internal/sensor/gather.go`) pulls `run-dev`
   into the run via the `depends_on` closure; it is scheduled as a wavefront node and
   executed by `lifecycle.RunSensor` (synchronous, run-to-completion). This spawns
   `next dev` **#1**.
2. **Inline composition path** — a `logs` sensor step `uses: run-dev`. The executor's
   `execUsesStep` (`internal/runtime/executor/compose.go`) **expands `run-dev`'s inner
   steps inline and re-runs them**, spawning `next dev` **#2**.

The two processes race for the single `.next/dev/lock`; the loser exits non-zero
without emitting signals, so the consuming sensor terminates `error` → `inconclusive`.

**Underlying gap:** the framework has no concept of a *shared, long-lived environment
service* that multiple sensors attach to. Two facts confirm this is an architectural
gap, not a missing dedup check:

- `runUseCaseSensors` (`cmd/harness/usecase_runner.go`) runs **every** sensor —
  observational ones included — via `RunSensor` (run-to-completion). It never calls
  `StartSensor`. There is no "shared background service other sensors attach to."
- The lifecycle registry (`running_sensors.json`) is keyed by `(sensorID, runID)`, and
  every spawn path gets a fresh `runID` — so even as designed it cannot dedup `run-dev`
  across these two paths.
- The heal-loop revalidator already **skips observational sensors and carries their
  verdicts forward** (`internal/runtime/healloop/revalidator.go`), and the
  `validate-use-case` skill doc never mentions observational sensors — observational
  sensors inside a synchronous validate run are underspecified today.

**Goal:** a single shared `run-dev` instance per use-case run that other sensors
*attach* to **via its signal stream** (not raw logs — the watcher already turns output
into signals), with a reference-counted lifetime. Only one `next dev` ever holds the
lock.

## 2. Core model

1. **Shared observational service** — a sensor with `scope: core` **and**
   `kind: observational`. It hosts one long-lived process (e.g. `next dev`); the
   existing watcher tails its output and emits signals to `signals.jsonl`. Exactly one
   instance per service per use-case run.
2. **Attachment** — a sensor referencing a service via step-level `uses:` *or*
   sensor-level `depends_on:` *attaches* to the running instance rather than
   spawning/expanding it. The trigger is **target kind**: when `uses:`/`depends_on:`
   names a `core` + `observational` sensor, the runtime treats it as attach. Inline
   expansion via `execUsesStep` remains **only** for non-observational core primitives.
3. **Reference-counted lifetime** — the service starts on the first attach, stays alive
   while at least one sensor is attached, and is stopped when the last attacher detaches.
4. **Parameterized consumers** — an attaching angle sensor (e.g. a `logs`-angle sensor)
   carries `with:` params describing the specific signals/log patterns it watches for.
   It consumes the service's live `signals.jsonl`, applies its own matchers /
   expected-observation keys, and produces its own verdict. The service owns the process;
   the consumer owns "what to look for."
5. **Consumer termination — satisfied-on-completeness** — an attaching observational
   consumer terminates as soon as **all** its expected observation keys are observed
   (→ `pass`), or when a bounded **observation window** elapses (→ verdict derived from
   completeness: missing expected keys → `fail`/`inconclusive` per existing rollup rules).

## 3. Components

### 3.1 `servicemgr` (new — `internal/runtime/servicemgr`)
Reference-counted manager of shared observational services for one use-case run.

- `Acquire(ctx, serviceID) (Attachment, error)`:
  - If the service is not running: start it via `lifecycle.StartSensor` (detached
    watcher), set ref-count = 1.
  - If already running: ref-count++.
  - Block until the service is **ready** (see §6) or a readiness timeout elapses.
  - Return an `Attachment` carrying the service's live `signals.jsonl` path (derived
    from the `lifecycle.Handle.RunDir`).
- `Release(serviceID)`: ref-count‑‑; when it reaches zero, `lifecycle.StopSensor`.
- A **per-service-id mutex** guards the first-acquire path so concurrent wavefront
  goroutines cannot double-start the same service.
- Safe for concurrent use by parallel wavefront goroutines. One `servicemgr` instance
  per use-case run.

### 3.2 Use-case runner (`cmd/harness/usecase_runner.go`)
- **Classification:** after `GatherForUseCase`, partition sensors into:
  - *services* — `scope: core` + `kind: observational` that are referenced as attach
    targets. Removed from the wavefront node set (they are managed by `servicemgr`, not
    scheduled to completion).
  - *regular sensors* — everything else; scheduled as today via `wavefronts`.
- **Attach wiring:** a regular sensor that references a service (`uses:` step or
  `depends_on:`) calls `servicemgr.Acquire` before its run and `Release` (deferred,
  guaranteed even on error) after. The attachment's signals path is threaded into that
  sensor's executor run via an injected seam (see §3.3).
- **Ordering:** "service ready before consumer runs" is enforced by `Acquire` blocking
  on readiness — not by wavefront layering. A `depends_on:` edge to a *service* is an
  attach edge, not a scheduling edge.

### 3.3 Executor attach path (`internal/runtime/executor/compose.go`)
- `execTopStep`/`execUsesStep`: when the target primitive is `kind: observational`, do
  **not** expand. Instead run an **attach step**:
  1. Resolve the service's live `signals.jsonl` path from the injected attachment.
  2. Follow that stream (reuse the `skillruntime.ReplaySignals` follow mechanism).
  3. Feed observed signals through the consuming sensor's matchers / expected keys.
  4. Emit the consumer's own signals.
  5. Terminate on completeness (all expected keys seen) or when the observation window
     elapses.
- The `servicemgr`/attachment is **injected** through `executor.Options` (e.g. a
  `ServiceAttach func(serviceID string) (Attachment, bool)` seam supplied by the runner),
  keeping the executor per-run-stateless. Non-observational core primitives keep the
  existing inline-expand behavior unchanged.

### 3.4 Lifecycle reuse
`StartSensor` / `StopSensor` / the registry / `RunWatcher` / `ReplaySignals` already
exist. `servicemgr` orchestrates them; no new process-management primitives are needed.

## 4. Data flow (happy path)

1. `RunUseCase` → `GatherForUseCase`.
2. Runner classifies: `run-dev` (core + observational) = service; `logs` (attaches via
   `uses:`) + assertion sensors = regular.
3. Wavefront schedules the regular sensors. The `logs` goroutine calls
   `servicemgr.Acquire("run-dev")` → one `next dev` starts, runner waits for ready.
4. The `logs` executor attach step tails `run-dev`'s `signals.jsonl`, applies its watch
   params, and terminates when its expected keys are seen (pass) or the window elapses.
5. `logs` finishes → `servicemgr.Release("run-dev")`. Last attacher detaches →
   `StopSensor("run-dev")`.
6. Per-sensor `AggregateSignal`s roll up into the use-case verdict exactly as today.

## 5. Schema / artifact changes

- **`run-dev`** (`schemas/examples/sensor/core-run-dev.yaml` + the dogfood/example
  copies): `kind: assertion` → `observational`; `output_type: single-shot` → `stream`.
  Its steps background the server; the watcher tails output; `signal_matches` emit an
  explicit `ready` observation key plus log signals.
- **Sensor schema (`schemas/sensor.yaml`)**: no new *required* fields. Add an **optional**
  consumer-side observation-window field (`observe_window`, a duration string); when
  absent, a runtime default constant applies (see §8). Document that step-level `uses:`
  (and `depends_on:`) targeting an observational core sensor means *attach*.
- **Generators** (`create-sensors`, `create-core-sensors`): emit `run-dev` as
  observational and angle sensors (e.g. `logs`) in attach shape (`uses: run-dev` +
  `with:` patterns, no inline server boot). *Follow-on within this spec; may be a later
  implementation-plan phase so the runtime fix lands first.*

## 6. Error handling (defense in depth)

- **Service never ready** within the readiness timeout → `Acquire` returns an error →
  attaching sensors receive an `inconclusive` `AggregateSignal` with a `heal_hint`
  ("run-dev failed to start: <stderr tail>"), not a lock crash.
- **Second-spawn guard:** if any path still attempts to start a service the registry
  reports alive, fail fast with an actionable `heal_hint` instead of colliding on the
  lock. This kills the issue's symptom even if an attach path is missed.
- **Window elapsed without completeness** → verdict derived from completeness; the
  `heal_hint` lists the unseen expected keys.
- **Release is guaranteed** (deferred) even when a consumer errors, so services never leak.
- **Readiness definition:** a spawned `next dev` is not yet serving, so readiness is an
  **explicit `ready` observation key** emitted by `run-dev`'s `signal_matches`, not mere
  process spawn. `Acquire` blocks until that key appears in the service's stream or the
  readiness timeout elapses.

## 7. Testing (TDD)

- **`servicemgr` (unit):** start-once / stop-on-last-release; concurrent-acquire
  double-start guard; release-on-consumer-error; ref-count correctness with ≥2 attachers.
- **Executor attach step (unit):** given a fixture `signals.jsonl`, the consumer matches
  expected keys and terminates on completeness; separately, the window-elapse path
  produces a completeness-derived verdict. Uses a fake stream, no real process.
- **Use-case runner (unit):** classification (service excluded from the wavefront node
  set); acquire/release wiring; two consumers share exactly one service instance. Fake
  lifecycle + fake `servicemgr`.
- **Regression / integration (`examples/`):** reproduce #33 — a use case with `run-dev`
  plus a `logs` sensor that `uses: run-dev` → exactly one service instance, `logs`
  produces signals (no lock collision), and the verdict is not the
  crash-induced `inconclusive`.
- **Second-spawn guard (unit):** a duplicate start attempt emits the actionable
  `heal_hint`.

Every new `internal/` package ships with a sibling `_test.go` (project rule).

## 8. Resolved decisions

1. **Observation window:** optional per-sensor `observe_window` field, falling back to a
   runtime default constant (proposed: 30s; finalize in the implementation plan).
2. **Service readiness:** explicit `ready` observation key emitted by `run-dev` (a
   spawned `next dev` is not yet serving). `Acquire` blocks on it with a readiness timeout.
3. **v1 attach surface:** both step-level `uses:` and sensor-level `depends_on:` toward an
   observational core sensor are treated as attach.

## 9. Scope

**In scope:** `servicemgr`; runner classification + attach acquire/release wiring;
executor attach path; `run-dev` → observational; second-spawn guard; unit tests + the
#33 regression test.

**Follow-on (same spec, possibly later plan phase):** generator changes so freshly
generated sensors use the attach shape; updating the `validate-use-case` skill doc to
describe observational/service handling.

**Out of scope:** per-instance isolation (distinct workdir/`PORT`/lock path —
the issue's non-preferred alternative); multi-archetype service coordination.
