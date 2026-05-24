# B3 — Executor & Lifecycle

> Source plan: [`plan.md`](plan.md) §6.1 (Executor), §6.2 (Observational sensors)

The runtime spine. Executor runs assertion sensors to completion. Lifecycle owns Run/Start/Stop semantics for both sensor kinds and is the only entry point the skill layer (B6) and CLI (B7) should call.

## Branching (mandatory)

Before starting any work on this chunk:

```bash
git fetch origin
git checkout -b feat/b3-executor-lifecycle origin/main
```

If B1/B2 are still in PR, branch off `main` and rebase as they merge. Do not stack on `feat/b2-*`.

## Parallelism

- **Can run in parallel with:** B5 (detection + generation).
- **Must run after:** B1 (resolver, template, signalCollector), B2 (fixtureBinder, per-sensor aggregator).
- **Blocks:** B4 (heal loop calls lifecycle for selective re-runs), B6 (run/start/stop/validate-use-case skills wrap lifecycle), B7 (CLI's `harness validate` invokes lifecycle).

## Scope

In:
- `internal/runtime/executor/` — given a `Sensor` (kind: assertion), runs its steps in order, binds fixtures (B2), pipes step stdout to signal collector (B1), invokes per-sensor aggregator (B2) on completion. Synchronous; returns the terminal `AggregateSignal`.
- `internal/lifecycle/` — the public API surface:
  - `RunSensor(ctx, sensorID) (AggregateSignal, error)` — assertion sensors.
  - `StartSensor(ctx, sensorID) (Handle, error)` — observational; spawns a long-lived watcher goroutine.
  - `StopSensor(ctx, handle) (AggregateSignal, error)` — terminates and emits the terminal AggregateSignal.
- Process/goroutine lifecycle: timeouts, cancellation via `context.Context`, clean stdout/stderr drain.
- Observational watcher: pattern matchers derived from stack-manifest log library (per §6.2). For B3 the pattern source is a parameter; deriving them from the stack is B5's `/create-sensors` concern.

Out:
- Sensor *generation* (B5).
- Heal loop (B4).
- Skill/CLI wrappers (B6/B7).
- DAG-level orchestration of *multiple* sensors for one use case — lives in B6's `/validate-use-case`.

## Inputs / Outputs

| Function | Input | Output |
|---|---|---|
| `executor.Run` | `Sensor`, `StepBinder`, `SignalSink`, `context.Context` | `AggregateSignal` |
| `lifecycle.RunSensor` | `Sensor.id` | `AggregateSignal` |
| `lifecycle.StartSensor` | `Sensor.id` | `Handle{ID, stop func()}` |
| `lifecycle.StopSensor` | `Handle` | `AggregateSignal` (with completeness) |

## Dependencies

- B1, B2.
- Phase A: `internal/sensor`, `internal/aggregate`.
- External: `os/exec` for step shell commands; standard library only for goroutine + channel patterns.

## Open questions for `/brainstorming`

1. **Step `run` shape.** E6's open question 1 is unresolved (shell string vs discriminated union). Executor needs this to land or to make a temporary call. Recommendation: implement against discriminated union — `{shell: "..."}` vs `{skill: "/...", args: {...}}` — even if E6's schema currently only allows the shell form. Skill dispatch can stub-fail until B6.
2. **Working directory.** Each step runs in the repo root, the use case's "subject" directory, or a fresh temp dir per sensor run? Recommendation: repo root by default; sensors may declare otherwise in metadata (Phase B refinement).
3. **Timeout policy.** Per-step, per-sensor, both? Recommendation: per-sensor wall clock + optional per-step override; both surfaced via context cancellation.
4. **Observational handle persistence.** Does a `Handle` survive a CLI invocation (write to disk, reload) so a separate `/stop-sensor` invocation can find it? Recommendation: yes — JSON sidecar in a `.harness/handles/` directory, with PID + start time. Single-machine for Phase B.
5. **Concurrent sensor runs.** Does `lifecycle` allow N sensors in flight for one use case (parallelism), or strictly serial via the resolver's topological order? Recommendation: parallel where `depends_on` allows; serial within a `depends_on` chain.

## Deliverable acceptance

- `executor.Run` runs a 3-step assertion sensor end-to-end against a fixture-bound step; signal collector receives the streamed signals; terminal `AggregateSignal` matches a golden file.
- `lifecycle.RunSensor` succeeds with a valid sensor id, returns a typed `ErrSensorNotFound` on a missing id.
- `lifecycle.StartSensor` / `StopSensor` round-trip an observational sensor against a fake log stream; the terminal `AggregateSignal.verdict` correctly flips between `pass` (all expected observations seen), `fail` (missing), `inconclusive` (timeout).
- Context cancellation mid-step kills the child process and surfaces `ctx.Err()` through the returned error.
- Tests run with `-race` clean.
