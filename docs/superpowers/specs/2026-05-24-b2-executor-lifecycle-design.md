# B2 — Executor & Lifecycle (Design)

> Source chunk: [`docs/harness-framework/B2-executor-lifecycle.md`](../../harness-framework/B2-executor-lifecycle.md)
> Plan references: [`plan.md`](../../harness-framework/plan.md) §6.1 (Executor), §6.2 (Observational sensors)
> Phase A entities consumed (read-only): E1 (enums), E5 (Fixture), E6 (Sensor), E7 (Signal), E8 (AggregateSignal)
> Phase B chunks consumed: B1 (fixture binder)
> Brainstorm date: 2026-05-24

## 1. Purpose

B2 owns the runtime spine that takes a loaded `Sensor`, runs it to completion (or until told to stop), and returns one terminal `AggregateSignal`. It is the only entry point B5 (skill wrappers) and B6 (CLI `harness validate`) should call for sensor execution.

Two layers:

1. **Executor (`internal/runtime/executor/`)** — pure mechanism. Given a `Sensor` plus a pre-allocated run directory, it spawns each step in order, streams stdout/stderr, decodes Signals on the fly, and calls `aggregate.Rollup` to produce the terminal `AggregateSignal`. Knows nothing about sensor IDs, sidecars, or `.harness/` layout.
2. **Lifecycle (`internal/lifecycle/`)** — registry layer. Resolves sensor IDs, allocates run directories, persists handles to a central registry file, drives `StartSensor` / `StopSensor` across processes, and exposes the public surface that B5/B6 call.

B2 does **not** own:

- Multi-sensor orchestration / DAG-parallel scheduling — that is B5's `/validate-use-case`.
- Heal-loop orchestration of re-runs — B3.
- Sensor generation or observation-key derivation from the stack manifest — B4 `/create-sensors`.
- Pruning of old run directories — a future `harness clean` subcommand under B6.

## 2. Scope

**In:**

- New package `internal/runtime/executor/` — single-step + multi-step execution, signal streaming, shell wrapping, stderr drain, process-group spawn.
- New package `internal/lifecycle/` — handle registry, public Run/Start/Stop API, cross-process signal-based stop.
- New runtime directory convention under `.harness/runtime/` plus a central registry file `running_sensors.json`.
- One five-line extension to `internal/signal` exposing `DecodeLine([]byte) (Signal, error)` so the executor can decode line-by-line while interleaving raw-log writes.
- Tests covering the deliverable acceptance criteria from the source chunk plus the additional cases this design surfaces.

**Out:**

- Per-step timeout — caller's `context.WithTimeout` is sensor-wide.
- Sensor schema changes (no new fields on `Sensor` or `SensorStep`).
- Re-validation after a heal cycle — B3.
- Parallel execution of multiple sensors — B5.
- Pruning policy for `.harness/runtime/` — out of B-phase scope.

## 3. Open questions resolved

The source chunk listed six open questions. Brainstorming on 2026-05-24 resolved each, plus surfaced and resolved several follow-ups:

| # | Question | Decision |
|---|---|---|
| 1 | Step `run` shape | **String shell command** (already locked in `sensor.Step.Run`). Executor wraps via `sh -c` (POSIX) / `cmd /C` (Windows). Resolved by Phase A; no further action. |
| 2 | Working directory | **Repo root for `Cmd.Dir`** (passed via `Options.RepoRoot`). Scratch (fixture payloads) lives in a per-run subdir, exposed via `HARNESS_SCRATCH_DIR`. No schema change. |
| 3 | Timeout policy | **Caller-owned via `context.Context`** — no internal default. CLI/B5/tests use `context.WithTimeout`. Per-step timeout deferred until/unless schema gains a field. |
| 4 | `RollupInput` plumbing | **Direct shape match** — `internal/aggregate.RollupInput` already exposes every field B2 needs. Executor populates and calls `Rollup` unchanged. |
| 5 | Concurrent sensors | **Out of scope for lifecycle.** `RunSensor`/`StartSensor` are single-sensor entry points. B5's `/validate-use-case` orchestrates multi-sensor parallelism via `sensor.ResolveExecutionOrder`. Same-sensor concurrent runs (e.g., two CLI invocations) are allowed because run-ids partition the runtime directory. |
| 6 | Termination reason enums | **Use existing values** — `completed`, `stopped`, `timeout`, `error` from [`schemas/enums/termination-reasons.yaml`](../../../schemas/enums/termination-reasons.yaml). No changes. |
| 7 | Observational execution model (surfaced) | **External subprocess.** The sensor's `Step.Run` spawns a long-lived process (e.g., `harness log-tail …`) that emits Signals on stdout. The executor never runs an internal matcher loop. Unifies the assertion and observational code paths — they share the same single-step machinery; only lifecycle differs. |
| 8 | Observation-key plumbing (surfaced) | **Caller parameter + evidence convention.** `StartSensor` / `RunSensor` take `expectedObs []string`. Signals carry the observed key as `evidence.observation_key: "<key>"`. Executor harvests these into `RollupInput.ObservedKeys`. No schema change. |
| 9 | Runtime directory location (surfaced) | **Per-run subdirectories under `.harness/runtime/<sensor-id>/<run-id>/`.** ULID-based `run-id` lets concurrent runs of the same sensor coexist; old runs are preserved as history (no automatic cleanup). |
| 10 | Cross-process stop mechanism (surfaced) | **OS signal to recorded PID.** A central registry file `.harness/runtime/running_sensors.json` carries `pid`, `pgid`, `started_at`, and `expected_observations` per in-flight run. `StopSensor` reads the registry, sends `SIGTERM` to the process group (POSIX) / `GenerateConsoleCtrlEvent` (Windows), then `SIGKILL` after a grace period. |
| 11 | Shell wrapping vs argv split (surfaced) | **Shell wrapper.** `/bin/sh -c <run>` on POSIX, `cmd /C <run>` on Windows. Supports pipes and shell idioms naturally. Fixture payloads reach the command via env vars (already the binder's contract), not via string interpolation. |
| 12 | Template resolution inside `Step.Run` (surfaced) | **Entry points only.** `template.Parse(step.Run)` runs at exec time; `FixtureRef` segments error out with `ErrTemplateFixtureInRun`; `EntryPointRef` segments resolve through the existing `template.Resolver`. Prevents shell-injection from fixture content while keeping `{{entry_points.X.spec.path}}` useful. |
| 13 | Exit-code interpretation (surfaced) | **Signals are truth.** If the step emitted ≥1 Signal, the exit code is advisory; `termination_reason=completed`. If exit ≠ 0 with no Signals, the step crashed: `termination_reason=error`, and a synthetic heal-hint is built from the tail of `raw.log`. Multi-step: a crashed step halts the sensor; otherwise iteration continues. |
| 14 | Registry shape (surfaced, revised) | **Single JSON document** at `.harness/runtime/running_sensors.json`, file-locked via `github.com/rogpeppe/go-internal/lockedfile`. Read-modify-write under exclusive lock; reads use shared locks. One row per in-flight `(sensor_id, run_id)`. Replaces a per-run `handle.json` sidecar — easier `list-sensors`, easier stop-all, single inspection point. |
| 15 | `raw.log` shape (surfaced) | **Interleaved stdout+stderr+parse-errors** in one annotated, line-buffered file per run. Replaces per-step stderr files and a separate parse-error log. `signals.jsonl` remains as the clean parsed projection of stdout. |

## 4. Package layout

```
internal/
├── runtime/
│   ├── executor/                       (created here; does not exist yet)
│   │   ├── executor.go                 Executor, Options, Run
│   │   ├── step.go                     single-step exec; shared by both sensor kinds
│   │   ├── signals.go                  stdout JSONL pump + observation_key extraction
│   │   ├── command.go                  shell wrapper per GOOS
│   │   ├── stderr.go                   stderr drain into raw.log
│   │   ├── rawlog.go                   line-annotated, mutex-serialized writer
│   │   ├── crash.go                    synthesizeCrashHint from raw.log tail
│   │   ├── errors.go                   ErrStepCrashed, ErrTemplateFixtureInRun, TemplateError, SpawnError
│   │   └── *_test.go
│   └── process/                        (created here; does not exist yet)
│       ├── process.go                  GroupSignaler interface, Signal enum (SignalTerm, SignalKill)
│       ├── process_posix.go            build-tagged POSIX impl (Setpgid, kill -pgid, /proc/<pid>/stat)
│       ├── process_windows.go          build-tagged Windows impl (CREATE_NEW_PROCESS_GROUP, GenerateConsoleCtrlEvent, GetProcessTimes)
│       └── *_test.go
└── lifecycle/                          (created here; does not exist yet)
    ├── lifecycle.go                    Lifecycle, Run/Start/Stop entry points
    ├── handle.go                       Handle struct + JSON marshaling
    ├── registry.go                     running_sensors.json read/write under file lock
    ├── runtime_dir.go                  .harness/runtime/<id>/<run-id>/ paths and creation
    ├── errors.go                       ErrSensorNotFound, ErrAssertionSensor, ErrSensorOrphaned, ErrSensorReplaced, ErrRegistryBusy
    └── *_test.go
```

**Dependencies:**

| Package | Imports (production) |
|---|---|
| `runtime/process` | (leaf; only `syscall`, `golang.org/x/sys/unix`, `golang.org/x/sys/windows`) |
| `runtime/executor` | `internal/aggregate`, `internal/enums`, `internal/sensor`, `internal/signal`, `internal/usecase`, `internal/usecase/template`, `internal/fixture`, `internal/runtime/fixturebinder`, `internal/runtime/process` |
| `lifecycle` | `internal/runtime/executor`, `internal/runtime/process`, `internal/sensor`, `internal/aggregate`, `internal/enums`, `github.com/rogpeppe/go-internal/lockedfile`, `github.com/oklog/ulid/v2` |

`runtime/process` is a leaf. `runtime/executor` and `lifecycle` both consume it; neither imports the other in reverse. `lifecycle` depends on `runtime/executor` only one-way. No cyclic dependencies. No Phase A package imports any of the three new packages.

## 5. Executor

### 5.1 Types

```go
package executor

type Options struct {
    RepoRoot      string                              // exec.Cmd.Dir for every step
    Resolver      *template.Resolver                  // pre-wired with Fixtures + EntryPoints
    FixtureStore  fixture.FixtureStore                // passed to fixturebinder.Bind; usually the same store wired into Resolver.Fixtures
    UseCaseLookup func(sensorID string) (*usecase.UseCase, bool)  // owner resolution for binder
    Now           func() time.Time                    // injectable for deterministic golden tests
    Shell         []string                            // optional override; default sh -c or cmd /C
    GroupSignaler GroupSignaler                       // process-group spawn/signal abstraction (see §7.1); defaults to GOOS-appropriate impl
    OnStepStart   func(stepIdx int, pid, pgid int)   // optional; called by step.go after a successful cmd.Start
}

type Executor struct {
    opts Options
}

func New(opts Options) *Executor

// Run executes one sensor end-to-end against a pre-allocated run directory.
//
// runDir must already exist (caller's responsibility) with an empty scratch/
// subdir at runDir/scratch.
//
// expectedObs is forwarded to aggregate.RollupInput; pass nil for assertion
// sensors.
//
// stop is closed by Lifecycle when StopSensor is invoked; for assertion
// sensors and synchronous Runs, the caller passes a never-closed channel
// (or nil, which is treated as never-closed).
//
// Run is blocking; concurrency is the caller's concern.
func (e *Executor) Run(
    ctx context.Context,
    s sensor.Sensor,
    runDir string,
    expectedObs []string,
    stop <-chan struct{},
) (aggregate.AggregateSignal, error)
```

### 5.2 Single-step execution (`step.go`)

For each `step` in `sensor.Steps`, in order:

1. **Template-resolve `step.Run`.**
   - `segs, err := template.Parse(step.Run)`.
   - Walk segs; any `template.FixtureRef` returns `ErrTemplateFixtureInRun` wrapped in `TemplateError{Step: i, Cause: ...}`.
   - `resolvedCmd, err := opts.Resolver.Resolve(segs)`.
2. **Bind fixtures.**
   - On the first step of a `Run`, the executor instantiates `binder := &fixturebinder.Binder{ScratchDir: filepath.Join(runDir, "scratch")}` and reuses it across the run's steps. `Binder` has only one field (`ScratchDir`), so cloning per `Run` is trivial and keeps concurrent `Run` invocations isolated. Successive writes to the same fixture id within a single Run overwrite (`os.WriteFile` truncates), which is fine — fixtures are deterministic by id.
   - `binding, err := binder.Bind(step, useCase, opts.FixtureStore)` where `useCase, _ = opts.UseCaseLookup(s.ID)`. `BindError` propagates with no wrapping (heal loop in B3 keys off it directly via `errors.As`).
3. **Build environment.**
   - Start from `os.Environ()`.
   - Append all `binding.Env` entries (`HARNESS_FIXTURE_*` → absolute path).
   - Append `HARNESS_RUN_DIR=<runDir>`, `HARNESS_SCRATCH_DIR=<runDir>/scratch`, `HARNESS_SENSOR_ID=<s.ID>`, `HARNESS_USE_CASE_ID=<s.UseCaseID>`.
4. **Build the OS command.**
   - `argv := opts.shellArgv(resolvedCmd)` — `["/bin/sh", "-c", resolvedCmd]` on POSIX, `["cmd", "/C", resolvedCmd]` on Windows.
   - `cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)`.
   - `cmd.Dir = opts.RepoRoot`; `cmd.Env = env`.
   - `opts.GroupSignaler.Spawn(cmd)` — sets `cmd.SysProcAttr` to `Setpgid: true` (POSIX) or `CreationFlags: CREATE_NEW_PROCESS_GROUP` (Windows). Interface defined in §7.1.
5. **Wire stdout/stderr.**
   - `stdoutPipe, err := cmd.StdoutPipe()`; `stderrPipe, err := cmd.StderrPipe()`.
6. **Spawn.** `cmd.Start()`. Spawn failure surfaces as `SpawnError{Step: i, Cause: err}`. On success, resolve `pgid, err := opts.GroupSignaler.GroupID(cmd)` (returns `Pid` on Windows; `Getpgid(cmd.Process.Pid)` on POSIX), then invoke `opts.OnStepStart(i, cmd.Process.Pid, pgid)` if non-nil. Lifecycle uses this to maintain the registry entry; missing hook (e.g., tests) is a no-op.
7. **Run three goroutines under a `sync.WaitGroup`:**
   - **stdout reader** — scans line-by-line; writes each line to `raw.log` with annotation `[<ts> step-NN stdout]`; calls `signal.DecodeLine(trimmed)`; on success appends to in-memory signals slice, writes raw bytes to `signals.jsonl`, extracts `evidence.observation_key` if observational and `expectedObs` is non-nil; on decode failure writes `[<ts> step-NN parse-error]` line to `raw.log`.
   - **stderr reader** — scans line-by-line; writes each line to `raw.log` with annotation `[<ts> step-NN stderr]`.
   - **stop watcher** — `select { case <-ctx.Done(): kill; case <-stop: kill; case <-exitCh: return }`. Kill = `opts.GroupSignaler.SignalGroup(cmd.Process.Pid, pgid, SignalTerm)` then `SignalKill` after a 2-second grace; both calls are no-ops if the process has already exited.
8. **`cmd.Wait()`** runs in its own goroutine; closes `exitCh` when done.
9. **Drain.** Block until all reader goroutines exit (pipes closed).
10. **Compute step outcome:**

    | (`cmd.Wait` err, signals emitted, ctx/stop fired?) | Step outcome |
    |---|---|
    | nil, *, no | continue to next step; advisory exit-code logging |
    | non-nil exit, ≥1 signal, no | continue to next step; advisory `[<ts> step-NN exit-nonzero]` line in raw.log |
    | non-nil exit, 0 signals, no | halt sensor: `ErrStepCrashed`, `termination_reason=error` |
    | `ctx.Err()=DeadlineExceeded` | halt sensor: `termination_reason=timeout` |
    | `ctx.Err()=Canceled` OR stop closed | halt sensor: `termination_reason=stopped` |
    | `cmd.Start` failed | halt sensor: `SpawnError`, `termination_reason=error` |

### 5.3 Multi-step rollup (`executor.go`)

After the per-step loop terminates (either by completing all steps or by halting early), the executor calls `aggregate.Rollup` exactly once with the accumulated signals and the chosen `TerminationReason`. The existing rollup logic in [`internal/aggregate/rollup.go`](../../../internal/aggregate/rollup.go) handles verdict, confidence, observational completeness, and heal-hint synthesis.

One executor-local helper covers the gap where no Signal carried evidence and the step crashed: `synthesizeCrashHint` reads the trailing 4 KiB of `raw.log`, filters for `[step-NN stderr]` lines from the crashed step, and constructs a `HealHint` with the stderr tail as `Rationale`. This hint is attached only when `Rollup` returned an aggregate with `verdict ∈ {inconclusive, fail}` AND `aggregate.HealHint == nil` (the latter is true when the underlying signals carried no hints).

### 5.4 `signals.jsonl` and `raw.log`

Both files live under `<runDir>/` and are appended-to throughout the run:

- `signals.jsonl` — one valid Signal per line, raw JSON, no annotations. The `stdout` goroutine writes here only on successful decode.
- `raw.log` — every line of stdout and stderr, plus parse-error and exit-status annotations, in arrival order. Format per line: `[<RFC3339Nano timestamp> step-NN <stream>] <content>` where `<stream>` ∈ `{stdout, stderr, parse-error, exit-nonzero}`. A mutex serializes writes from the stdout and stderr goroutines so lines don't tear.

`signal.DecodeLine` is the new public helper exposing the existing private `decodeAndValidateLine` in [`internal/signal/parser.go`](../../../internal/signal/parser.go). Five-line change in the `signal` package; no behavior change for existing `ParseSignals` callers.

### 5.5 Errors

```go
var (
    ErrTemplateFixtureInRun = errors.New("executor: {{fixtures.X}} not allowed in step.run; use env vars")
    ErrStepCrashed          = errors.New("executor: step exited non-zero without emitting signals")
)

type TemplateError struct {
    Step  int
    Cause error
}
func (e *TemplateError) Error() string { return fmt.Sprintf("executor: template error at step %d: %v", e.Step, e.Cause) }
func (e *TemplateError) Unwrap() error { return e.Cause }

type SpawnError struct {
    Step  int
    Cause error
}
func (e *SpawnError) Error() string { return fmt.Sprintf("executor: spawn failed at step %d: %v", e.Step, e.Cause) }
func (e *SpawnError) Unwrap() error { return e.Cause }
```

`fixturebinder.BindError` propagates unchanged (already defined in B1).

## 6. Lifecycle

### 6.1 Types

```go
package lifecycle

type Lifecycle struct {
    exec        *executor.Executor
    sensors     sensor.Store
    runtimeRoot string                                // e.g. <repo>/.harness/runtime
    newRunID    func() string                        // ULID by default; injectable
    now         func() time.Time                    // injectable
    gracePeriod time.Duration                        // SIGTERM → SIGKILL window; default 5s

    mu       sync.Mutex
    inflight map[runKey]*runEntry                    // (sensorID, runID) → in-process goroutine state
}

type runKey struct{ SensorID, RunID string }

type runEntry struct {
    handle *Handle
    stopCh chan struct{}                             // closed by StopSensor
    doneCh chan struct{}                             // closed when goroutine exits
    aggCh  chan aggregate.AggregateSignal            // delivers terminal aggregate
    errCh  chan error
}

type Handle struct {
    SensorID             string    `json:"sensor_id"`
    RunID                string    `json:"run_id"`
    RunDir               string    `json:"run_dir"`
    PID                  int       `json:"pid"`
    PGID                 int       `json:"pgid"`
    StartedAt            time.Time `json:"started_at"`
    ExpectedObservations []string  `json:"expected_observations"`
    HarnessPID           int       `json:"harness_pid"`
    HarnessVersion       string    `json:"harness_version"`
    GOOS                 string    `json:"goos"`
}

func New(opts Options) *Lifecycle

type Options struct {
    Sensors     sensor.Store
    Executor    *executor.Executor
    RuntimeRoot string                               // typically <repo>/.harness/runtime
    NewRunID    func() string                       // optional; defaults to ULID-from-time
    Now         func() time.Time                    // optional; defaults to time.Now
    GracePeriod time.Duration                       // optional; defaults to 5*time.Second
}
```

### 6.2 `RunSensor`

```go
func (l *Lifecycle) RunSensor(
    ctx context.Context,
    sensorID string,
    expectedObs []string,
) (aggregate.AggregateSignal, error)
```

Synchronous; works for both kinds.

1. Resolve `s := l.sensors.Lookup(sensorID)`; `ErrSensorNotFound` if missing.
2. Allocate `runID := l.newRunID()`; build `runDir := filepath.Join(l.runtimeRoot, sensorID, runID)`.
3. `os.MkdirAll(filepath.Join(runDir, "scratch"), 0o700)`.
4. Pre-prune the registry of any entries whose PIDs are dead (see §6.5).
5. Launch the executor in a goroutine, passing an `OnStepStart` callback (see §5.1 Options addendum) that Lifecycle uses to write or update the run's registry entry. The callback fires once per step start with the new step's `(pid, pgid)`; on the first step it appends the entry, on subsequent steps it updates the same entry's PID/PGID. Symmetry note: assertion sensors also get a registry entry — useful for `harness list-sensors` to show in-flight assertions, no extra cost.
6. Block on the executor goroutine until it returns the terminal `AggregateSignal` (or until `ctx` cancels, in which case Lifecycle still waits for the executor to drain and return).
7. Write `<runDir>/aggregate.json` (atomic: temp + rename).
8. Remove the run's registry entry under an exclusive lock.
9. Return the aggregate to the caller.

### 6.3 `StartSensor`

```go
func (l *Lifecycle) StartSensor(
    ctx context.Context,
    sensorID string,
    expectedObs []string,
) (*Handle, error)
```

Observational-only. Returns `ErrAssertionSensor` if `s.Kind != KindObservational`.

1. Same setup as `RunSensor` steps 1-3.
2. Wrap `ctx` with `context.Background()` so the spawning caller's exit doesn't cancel the watcher (the spawn survives across processes; cancellation comes only via `StopSensor` or signal).
3. Spawn the executor in a goroutine; install the `OnStepStart` hook to write/update the registry entry.
4. Wait for the first registry write (signaling that the OS process has been spawned) OR an immediate error from the goroutine.
5. Return a `*Handle` copy of the registry entry.
6. The goroutine continues; on Run return it writes `aggregate.json` and removes the registry entry.

If the registry write fails after spawn succeeds, Lifecycle kills the process group via `process.SignalGroup(pgid, SIGKILL)` and returns the registry error. The orphaned process group must not survive a failed-to-register start.

### 6.4 `StopSensor`

```go
func (l *Lifecycle) StopSensor(
    ctx context.Context,
    h *Handle,
) (aggregate.AggregateSignal, error)
```

1. **Same-process fast path:** if `runKey{h.SensorID, h.RunID}` is in `l.inflight`, close the in-process `stopCh`, wait for `doneCh`, receive from `aggCh`. Return the aggregate.
2. **Cross-process path:**
   - `entry, ok := l.findRegistryEntry(h.SensorID, h.RunID)`. If not found, check `<runDir>/aggregate.json`. If present, the run already terminated naturally — return that aggregate. If neither, `ErrSensorNotFound`.
   - Probe `entry.PID` via `process.Default().IsAlive(entry.PID, entry.StartedAt)`. If dead → `ErrSensorOrphaned`. If alive but `started_at` mismatches → `ErrSensorReplaced`. Both errors trigger an exclusive-lock prune of the stale entry.
   - `process.Default().SignalGroup(entry.PID, entry.PGID, process.SignalTerm)`.
   - Poll `<runDir>/aggregate.json` every 100 ms up to `l.gracePeriod`. If it appears, return it.
   - On grace expiry, `process.Default().SignalGroup(entry.PID, entry.PGID, process.SignalKill)`. Poll again for up to 2 seconds.
   - If `aggregate.json` still does not exist, the host goroutine never got to write it (host process died) — synthesize a partial aggregate from `<runDir>/signals.jsonl`: read all decoded signals, call `aggregate.Rollup` with `TerminationReason=stopped`, `expected_observations=h.ExpectedObservations`. Write the synthesized aggregate to `<runDir>/aggregate.json` for future reads. Remove the registry entry.

### 6.5 Registry — `running_sensors.json`

Location: `<runtimeRoot>/running_sensors.json` (e.g. `<repo>/.harness/runtime/running_sensors.json`).

Shape:

```json
{
  "schema_version": "1.0.0",
  "entries": [
    {
      "sensor_id": "logs-create-order-sensor",
      "run_id": "01JZQ9G7M0H3FX8N1QPYAS78MV",
      "run_dir": "/abs/.harness/runtime/logs-create-order-sensor/01JZQ9G7M0H3FX8N1QPYAS78MV",
      "pid": 48217,
      "pgid": 48217,
      "started_at": "2026-05-24T10:18:00.123Z",
      "expected_observations": ["order-received", "order-validated", "order-persisted"],
      "harness_pid": 47903,
      "harness_version": "0.1.0",
      "goos": "linux"
    }
  ]
}
```

**Concurrency:**

- Library: `github.com/rogpeppe/go-internal/lockedfile` — production-grade cross-platform file locking (Go toolchain uses it).
- All mutations open the file with `lockedfile.OpenFile(..., os.O_RDWR|os.O_CREATE, 0o600)` (exclusive lock). Read-modify-write completes before unlocking. Atomic write via temp file in the same directory + `os.Rename`.
- All reads use `lockedfile.Read` (shared lock).
- A small `RegistryError` wraps lock-acquisition failures, surfaced as `ErrRegistryBusy` after a 5-second timeout.

**Stale entry pruning:**

- **Lazy:** every `ListRunning` / `StopSensor.findRegistryEntry` probes the entry's PID; dead entries are removed under an exclusive lock as a side effect.
- **Eager:** every `RunSensor` / `StartSensor` prunes dead entries before appending its own.
- Probe implementation: `process.IsAlive(pid int, startedAt time.Time) bool` in `process_posix.go` / `process_windows.go`. POSIX reads `/proc/<pid>/stat` field 22 (start time); Windows uses `OpenProcess(SYNCHRONIZE)` + `GetProcessTimes`.

**Public API on Lifecycle for B6 CLI:**

```go
func (l *Lifecycle) ListRunning() ([]Handle, error)
func (l *Lifecycle) LoadHandle(sensorID, runID string) (*Handle, error)
```

### 6.6 Runtime directory layout

```
.harness/
└── runtime/
    ├── running_sensors.json                         (central registry, file-locked)
    └── <sensor-id>/
        └── <run-id>/                                (ULID; e.g. 01JZQ9G7M0H3FX8N1QPYAS78MV)
            ├── aggregate.json                       (terminal AggregateSignal; written on Run completion)
            ├── scratch/                             (fixturebinder workspace; one file per bound fixture)
            ├── signals.jsonl                        (decoded Signals, raw JSON, one per line)
            └── raw.log                              (stdout + stderr + parse-errors, annotated per line)
```

- `<run-id>` is a Crockford ULID (`github.com/oklog/ulid/v2`). Lexicographic order = temporal order. Injectable via `Options.NewRunID` for golden-test determinism.
- Per-run directories accumulate indefinitely; no automatic cleanup. A future `harness clean` subcommand (B6) prunes by age/count.
- `aggregate.json` is the only file that proves a run terminated naturally; its presence is the signal that `StopSensor` can complete cleanly.

### 6.7 Errors

```go
var (
    ErrSensorNotFound      = errors.New("lifecycle: sensor id not in store")
    ErrAssertionSensor     = errors.New("lifecycle: StartSensor called on kind:assertion sensor")
    ErrSensorOrphaned      = errors.New("lifecycle: registry entry's PID is dead")
    ErrSensorReplaced      = errors.New("lifecycle: PID is alive but started_at disagrees (PID recycled)")
    ErrRegistryBusy        = errors.New("lifecycle: could not acquire registry lock within timeout")
)
```

All wrapped via `fmt.Errorf("…: %w", err)` so `errors.Is` / `errors.As` work for heal-loop dispatch in B3.

## 7. Cross-platform considerations

### 7.1 Process group spawn

All process-group concerns live in `internal/runtime/process/`, a leaf package shared by `executor` and `lifecycle`.

- **POSIX (`process_posix.go`):** `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`. The child becomes the process-group leader. `syscall.Kill(-pgid, sig)` signals the group. `IsAlive` reads `/proc/<pid>/stat` field 22 (start time in jiffies since boot) and compares against the recorded `started_at` translated to the same unit.
- **Windows (`process_windows.go`):** `cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}`. The child becomes a console-process-group root. `windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, pid)` sends a Ctrl-Break to the whole group (Ctrl-C is preempted by the new console group, so Ctrl-Break is the right signal). `windows.TerminateProcess(handle, 1)` is the hard-kill fallback. `IsAlive` uses `OpenProcess(SYNCHRONIZE)` + `GetProcessTimes` to read process creation time.

Both implementations sit behind a small interface in `process.go`:

```go
package process

type Signal int
const (
    SignalTerm Signal = iota
    SignalKill
)

type GroupSignaler interface {
    Spawn(cmd *exec.Cmd) error                            // sets SysProcAttr appropriately
    GroupID(cmd *exec.Cmd) (int, error)                   // resolves pgid after Start; returns Pid on Windows
    SignalGroup(pid, pgid int, sig Signal) error          // dispatches SIGTERM/SIGKILL on POSIX, CTRL_BREAK/Terminate on Windows
    IsAlive(pid int, startedAt time.Time) bool            // liveness + start-time match (PID-recycling defense)
}

// Default returns the GOOS-appropriate GroupSignaler.
func Default() GroupSignaler
```

`Signal` is a package-local enum, not `os.Signal`, because Windows doesn't have Unix signals.

### 7.2 Shell wrapper

- POSIX: `["/bin/sh", "-c", resolvedCmd]`. `/bin/sh` is universal; `/bin/bash` is not (Alpine). Sensors that need bash features explicitly call `bash -c '…'` in their `run` string.
- Windows: `["cmd", "/C", resolvedCmd]`. PowerShell is *not* the default because cmd has fewer parsing quirks for the framework's intended `run` strings (simple command-with-args plus pipes).
- `Options.Shell` lets tests and power users override (e.g. `["sh", "-eu", "-c", …]` for stricter semantics).

### 7.3 ULID time source

`ulid.MustNew(ulid.Now(), entropy)` ties run-ids to wall clock. Tests inject `Options.NewRunID` to return deterministic ids; production uses `ulid.Monotonic(rand.Reader, 0)`.

## 8. Testing strategy

### 8.1 Test fake binary

`internal/runtime/executor/testdata/fake-sensor/main.go` — a tiny Go program built once in `TestMain`. CLI:

```
fake-sensor signal pass [--observation-key K]
fake-sensor signal fail --heal-summary S --heal-rationale R
fake-sensor stream N --interval D                   # emits N pass signals D apart, then exits
fake-sensor crash --exit-code C --stderr S          # writes S to stderr and exits C
fake-sensor watch                                   # emits signals until SIGTERM; loops forever
fake-sensor sleep D                                 # sleeps without emitting; used for timeout tests
```

The fake binary lets every test scenario be constructed without an external sensor. `TestMain` compiles it to `testdata/fake-sensor/fake-sensor.exe` (or no `.exe` on POSIX) and tests reference it via absolute path.

### 8.2 Unit tests

| File | Coverage |
|---|---|
| `executor/step_test.go` | template parse/resolve, fixture-binder integration, env var construction, single-step golden output |
| `executor/signals_test.go` | line-by-line decode, observation-key extraction, parse-error handling, signals.jsonl tee |
| `executor/command_test.go` | shell-argv construction per GOOS (table-driven, build-tagged) |
| `executor/stderr_test.go` | stderr drain + raw.log annotation + crash-hint synthesis |
| `executor/rawlog_test.go` | mutex serialization under contention (race detector) |
| `executor/run_test.go` | multi-step orchestration, exit-code interpretation matrix |
| `lifecycle/handle_test.go` | Handle JSON round-trip |
| `lifecycle/registry_test.go` | running_sensors.json CRUD, file locking under concurrent writers, stale-pruning |
| `lifecycle/runtime_dir_test.go` | path construction, mkdir behavior |
| `runtime/process/process_posix_test.go` | (POSIX-only) Setpgid spawn, signal group reaches all children, IsAlive via /proc/<pid>/stat |
| `runtime/process/process_windows_test.go` | (Windows-only) CREATE_NEW_PROCESS_GROUP, GenerateConsoleCtrlEvent, IsAlive via GetProcessTimes |
| `lifecycle/lifecycle_test.go` | end-to-end Run/Start/Stop, cross-process round-trip via subtest fork |

### 8.3 Integration tests

- **Assertion sensor end-to-end** — load `schemas/examples/sensor/assertion-computational-single.yaml`, swap `run` to invoke the fake binary in `signal pass` mode, execute via `Lifecycle.RunSensor`, assert `aggregate.AggregateSignal` JSON byte-matches a golden file (with deterministic `Now` and `NewRunID`).
- **Observational sensor end-to-end** — load `observational-computational-stream.yaml`, swap `run` to `fake-sensor watch --emit order-received --emit order-validated`, call `StartSensor`, wait for the two signals (poll `signals.jsonl`), call `StopSensor`, assert verdict=fail (missing `order-persisted`) and heal_hint summary matches.
- **Cross-process round-trip** — `TestStopFromOtherProcess`: parent process calls `StartSensor`, then `exec.Command(os.Args[0], "-test.run=TestStopFromOtherProcess_Child")` with `HARNESS_TEST_SENSOR_ID` / `HARNESS_TEST_RUN_ID` env vars. Child reads vars, calls `Lifecycle.LoadHandle` + `Lifecycle.StopSensor`. Parent collects child's stdout (containing the terminal aggregate JSON) and asserts.
- **Context cancellation** — parent runs sensor in goroutine, cancels mid-stream, asserts the child process is reaped (PID gone) and `aggregate.AggregateSignal.TerminationReason == stopped`.
- **Timeout** — parent passes `ctx, _ := context.WithTimeout(ctx, 200ms)`, fake binary sleeps 1 second; asserts `TerminationReason == timeout` and verdict per `aggregate.Rollup` rules.

### 8.4 Race detector

All tests run under `go test -race ./internal/runtime/executor/... ./internal/lifecycle/...` in CI. The mutex-serialized raw.log writer and the file-locked registry are the high-value race targets.

### 8.5 Determinism golden tests

With `Options.Now` returning a fixed timestamp and `Options.NewRunID` returning a fixed ULID, the executor + lifecycle output is byte-deterministic. `aggregate.json`, `signals.jsonl`, and the per-run directory structure are all compared against golden files. Same convention as [`internal/aggregate/determinism_test.go`](../../../internal/aggregate/determinism_test.go).

## 9. Acceptance criteria mapping

Mapping the B2 chunk's deliverable acceptance section to concrete tests:

| Source chunk acceptance | Test that proves it |
|---|---|
| Assertion sensor runs 3-step end-to-end against a fixture-bound step; stdout JSONL flows into `aggregate.Rollup`; terminal `AggregateSignal` matches a golden file | `executor/run_test.go::TestRunAssertion_ThreeStepGolden` |
| `lifecycle.RunSensor` returns typed `ErrSensorNotFound` on a missing id | `lifecycle/lifecycle_test.go::TestRunSensor_MissingID` |
| `lifecycle.StartSensor` / `StopSensor` round-trip an observational sensor; verdict correctly flips between pass / fail / inconclusive | `lifecycle/lifecycle_test.go::TestObservational_VerdictMatrix` (table-driven) |
| Context cancellation mid-step kills the child process; surfaces `ctx.Err()` through the returned error | `executor/run_test.go::TestContextCancellation_KillsChild` |
| Handle sidecar survives a process restart; `StartSensor` in process A, `StopSensor` in process B succeeds | `lifecycle/lifecycle_test.go::TestStopFromOtherProcess` |
| Tests run with `-race` clean | CI enforces `go test -race ./...` |

## 10. Follow-ups (not blockers; tracked separately)

1. **`signal.DecodeLine` public helper** — promote the existing private `decodeAndValidateLine` to a public function. Five-line change in `internal/signal/parser.go`. Land as a co-change in the B2 PR (same logical chunk).
2. **No `expected_observations` field on `Sensor` schema** — B2 takes it as a parameter; if B5 always derives it from fixtures, consider adding the field to the sensor schema later for explicitness. Not urgent.
3. **No per-step timeout** — caller's `context.WithTimeout` is sensor-wide. If a use case for per-step timeouts emerges (e.g., setup steps with different budgets than the assertion step), revisit by adding a `timeout` field to `SensorStep`.
4. **`harness clean` subcommand** — prune old `.harness/runtime/<sensor-id>/<run-id>/` directories by age / count. Deferred to B6 CLI.

## 11. References

- Source chunk: [`docs/harness-framework/B2-executor-lifecycle.md`](../../harness-framework/B2-executor-lifecycle.md)
- Plan: [`docs/harness-framework/plan.md`](../../harness-framework/plan.md) §6.1, §6.2, §10
- Phase A entities consumed: [E1-enums](2026-05-22-e1-enums-design.md), [E5-fixture](2026-05-22-e5-fixture-design.md), [E6-sensor](2026-05-23-e6-sensor-design.md), [E7-signal](2026-05-23-e7-signal-design.md), [E8-aggregate-signal](2026-05-23-e8-aggregate-signal-design.md)
- Phase B sibling: [B1-composed-runtime](2026-05-24-b1-composed-runtime-design.md)
- Schemas referenced: [`sensor.yaml`](../../../schemas/sensor.yaml), [`signal.yaml`](../../../schemas/signal.yaml), [`aggregate-signal.yaml`](../../../schemas/aggregate-signal.yaml), [`enums/termination-reasons.yaml`](../../../schemas/enums/termination-reasons.yaml)
- Phase A code consumed: [`internal/sensor`](../../../internal/sensor/), [`internal/signal`](../../../internal/signal/), [`internal/aggregate`](../../../internal/aggregate/), [`internal/usecase/template`](../../../internal/usecase/template/), [`internal/runtime/fixturebinder`](../../../internal/runtime/fixturebinder/)
- External libraries proposed: `github.com/rogpeppe/go-internal/lockedfile`, `github.com/oklog/ulid/v2`
