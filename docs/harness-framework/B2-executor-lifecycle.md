# B2 — Executor & Lifecycle

> Source plan: [`plan.md`](plan.md) §6.1 (Executor), §6.2 (Observational sensors)

The runtime spine. Executor runs assertion sensors to completion. Lifecycle owns Run/Start/Stop semantics for both sensor kinds and is the only entry point the skill layer (B5) and CLI (B6) should call.

## Branching (mandatory)

```bash
git fetch origin
git checkout -b feat/b2-executor-lifecycle origin/main
```

If B1 is still in PR, branch off `main` and rebase as B1 merges. Do not stack on `feat/b1-*`.

## Parallelism

- **Can run in parallel with:** B4 (detection + generation).
- **Must run after:** B1 (fixture binder).
- **Blocks:** B3 (heal loop drives selective re-runs through lifecycle), B5 (run/start/stop/validate-use-case wrap lifecycle), B6 (`harness validate` invokes lifecycle).

## What Phase A already delivered (consume, do not rebuild)

| Need | Existing package | Key API |
|---|---|---|
| Sensor DAG ordering | `internal/sensor` | `ResolveExecutionOrder(sensors)` (and `ErrCycle`, `ErrMissingDependency`) |
| Step template interpolation | `internal/usecase/template` | `(*Resolver).Resolve(segs)` |
| JSONL signal streaming from `io.Reader` | `internal/signal` | `ParseSignals(io.Reader) iter.Seq2[Signal, error]` |
| Per-sensor terminal rollup | `internal/aggregate` | `Rollup(RollupInput)` (already populates `HealHint` for failures) |
| Sensor schema/types | `internal/sensor` | `Sensor`, `Step` (check field names — `run` shape comes from here) |

## Scope

In:
- `internal/runtime/executor/` — given a `Sensor` (kind: assertion), runs its steps in order. For each step: invoke B1's `fixturebinder.Bind` for `uses:` fixtures → optionally template-resolve the `run` payload via `usecase/template` → exec the command → stream stdout through `signal.ParseSignals` into a `RollupInput.Signals` buffer → invoke `aggregate.Rollup` to produce the terminal `AggregateSignal`. Synchronous; returns the terminal signal.
- `internal/lifecycle/` — public API surface:
  - `RunSensor(ctx, sensorID) (aggregate.AggregateSignal, error)` — assertion sensors.
  - `StartSensor(ctx, sensorID) (Handle, error)` — observational; spawns a long-lived watcher goroutine.
  - `StopSensor(ctx, handle) (aggregate.AggregateSignal, error)` — terminates and emits the terminal AggregateSignal.
- Process lifecycle: timeouts, cancellation via `context.Context`, clean stdout/stderr drain.
- Observational watcher: pattern matchers derived from the stack-manifest log library (per §6.2). For B2 the matchers are a parameter; *deriving* them from the stack is B4's `/create-sensors` concern.
- Handle persistence: write/read JSON sidecars under `.harness/handles/` so a separate `/stop-sensor` invocation can find an in-flight watcher.

Out:
- Sensor *generation* (B4).
- Heal loop orchestration (B3).
- Skill/CLI wrappers (B5/B6).
- Per-use-case orchestration of *multiple* sensors — that's `/validate-use-case` in B5.

## Inputs / Outputs

| Function | Input | Output |
|---|---|---|
| `executor.RunAssertion` | `Sensor`, `StepBinder`, `*template.Resolver`, `context.Context` | `aggregate.AggregateSignal` |
| `executor.RunObservational` | `Sensor`, `<-chan struct{}` stop signal, pattern matchers | `aggregate.AggregateSignal` (terminal) |
| `lifecycle.RunSensor` | `Sensor.id` | `aggregate.AggregateSignal` |
| `lifecycle.StartSensor` | `Sensor.id` | `Handle{ID, stop func()}` |
| `lifecycle.StopSensor` | `Handle` | `aggregate.AggregateSignal` (with completeness) |

## Open questions for `/brainstorming`

1. **Step `run` shape.** Inspect what `internal/sensor` actually exposes for `Step.run` (was an open question in E6). The executor's dispatch hangs on this — shell command vs discriminated shell-or-skill union.
2. **Working directory.** Each step runs in the repo root, the use case's subject directory, or a fresh temp dir per sensor run? Recommendation: repo root by default; sensors may declare otherwise in metadata.
3. **Timeout policy.** Per-step, per-sensor, both? Recommendation: per-sensor wall clock + optional per-step override; both via `context.WithTimeout`.
4. **`RollupInput` plumbing.** `aggregate.Rollup` takes a `RollupInput` — check exact shape (signals list, sensor metadata, expected-observations for observational). Executor must populate it correctly per sensor kind.
5. **Concurrent sensors.** Does `lifecycle` run N sensors in flight when `depends_on` allows, or strictly serial via `ResolveExecutionOrder`? Recommendation: parallel where the DAG allows; serial within a chain.
6. **Termination reason enums.** `schemas/enums/termination-reasons.yaml` exists — confirm the observational `AggregateSignal` carries one of these on stop (timeout vs explicit-stop vs end-of-action).

## Deliverable acceptance

- `executor.RunAssertion` runs a 3-step assertion sensor end-to-end against a fixture-bound step; stdout JSONL flows into `aggregate.Rollup`; terminal `AggregateSignal` matches a golden file.
- `lifecycle.RunSensor` returns typed `ErrSensorNotFound` on a missing id.
- `lifecycle.StartSensor` / `StopSensor` round-trip an observational sensor against a fake log stream; `AggregateSignal.verdict` correctly flips between `pass` / `fail` / `inconclusive` (timeout via `termination-reasons`).
- Context cancellation mid-step kills the child process; surfaces `ctx.Err()` through the returned error.
- Handle sidecar survives a process restart: `StartSensor` in process A, `StopSensor` in process B succeeds.
- Tests run with `-race` clean.
