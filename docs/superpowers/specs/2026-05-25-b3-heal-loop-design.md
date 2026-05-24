# B3 — Heal Loop (Design)

> Source chunk: [`docs/harness-framework/B3-heal-loop.md`](../../harness-framework/B3-heal-loop.md)
> Plan references: [`plan.md`](../../harness-framework/plan.md) §6.1 (Heal Loop), §10.4 (termination cap)
> Phase A entities consumed (read-only): E1 (enums), E4 (UseCase), E6 (Sensor), E8 (AggregateSignal), E9 (ValidationPolicy)
> Phase B chunks consumed: B1 (per-use-case aggregator), B2 (lifecycle)
> Brainstorm date: 2026-05-25

## 1. Purpose

B3 owns the iteration loop that turns a failing `UseCaseVerdict` into either a healed use case, an exhausted attempt, or an abandoned attempt — without ever leaving the working tree in a partially-edited state.

It is purely orchestration. It does not:

- Synthesize heal hints — already done by `aggregate.Rollup` (Phase A, E8).
- Compute use-case verdicts — done by `aggregator.UseCase` (B1).
- Run sensors — done by `lifecycle.RunSensor` (B2).
- Call the LLM directly — `LLMClient` is an interface satisfied by the `/heal` skill scripts at B5.

What it owns:

1. The iteration loop and its termination semantics.
2. Transactional edit application with revert on re-validate failure.
3. Carry-forward of original observational `AggregateSignal`s into the re-aggregation.
4. The `PromptInput` / `EditPlan` / `Attempt` contract between Go and the LLM-driving skill.
5. A small additive extension to `internal/policy` for the iteration cap.

## 2. Scope

**In:**

- New package `internal/runtime/healloop/` — entry point `Run`, three interfaces (`LLMClient`, `Transactor`, `Revalidator`), default implementations for `Transactor` (git stash + file backup) and `Revalidator` (composes `lifecycle.Lifecycle` + `aggregator.UseCase`).
- Additive extension to `internal/policy`: new optional field `max_heal_iterations` on `ValidationPolicy` and always-populated `MaxHealIterations` on `EffectivePolicy`. Default value `3`.
- Schema update: `schemas/policy.yaml` gains `max_heal_iterations: integer (0..20, default 3)`.
- Tests covering every spec acceptance criterion plus the additional cases this design surfaces.

**Out:**

- The `AffectedSensors` helper listed in the source chunk. Re-validation runs all assertion sensors in the failing use case, so file-to-sensor scoping has no consumer in v1. Defer until a future caller needs it.
- LLM client implementation. B3 defines `LLMClient`; B5 (`/heal` skill) implements it.
- CLI surface (`harness heal`). B6.
- Sensor-generation regeneration after heal (the use case's sensor set is taken as-is).
- Concurrent heal of multiple use cases. Per-use-case loops run serially in v1.
- Re-running observational sensors during heal. Their original `AggregateSignal`s carry forward into the re-aggregation.
- Healing observational sensors directly. Their hints still drive code edits, but their post-edit state cannot be observed within the loop; the user re-runs them after heal completes.

## 3. Open questions resolved

The source chunk listed six open questions. Brainstorming on 2026-05-25 resolved each, plus surfaced and resolved several follow-ups:

| # | Question | Decision |
|---|---|---|
| 1 | Termination cap | **New field on `policy.EffectivePolicy`.** `MaxHealIterations int`, default `3`, range `0..20`. Resolved via the same global/local override pattern as `InferentialFloor`. `healloop.Config` reads it from the resolved policy; the loop has no direct dependency on `internal/policy`. |
| 2 | Edit transactionality | **Hybrid: git stash when in a git repo, file backup otherwise.** `DefaultTransactor(repoRoot)` auto-detects via `git rev-parse --git-dir`. Git mode scopes the stash to the target paths only, resolves to a stash SHA at snapshot time, and reverts via `git apply <sha>` + `git stash drop <sha>` (not `git stash pop`, which uses positional refs). |
| 3 | Affected-sensor scoping | **Use-case scope.** Re-validation runs every assertion sensor in the failing use case. File-level scoping deferred — no caller in v1. |
| 4 | Multi-hint loops | **Consolidated prompt per iteration.** All `HealHints` from the current verdict are sent to the LLM in one `PromptInput`. The LLM returns one `EditPlan` covering all affected files. One iteration = one Propose + one Apply + one Revalidate. |
| 5 | LLM client contract | **Sync `Propose(ctx, PromptInput) → (EditPlan, error)`.** Heal is the slow path; simplicity wins. |
| 6 | Heal-loop interleaving | **Serial per use case.** Two failing use cases trigger two `healloop.Run` calls in sequence. |
| 7 | Iteration unit semantics (surfaced) | **Original verdict is the source of truth for hints.** Every failing iteration is reverted, so post-revert state equals entry state, so hints don't change across iterations within a single `Run` call. The loop does not re-bind `currentVerdict` between iterations. |
| 8 | Attempt feedback (surfaced) | **Full history.** Iteration N+1's `PromptInput.History` carries every prior `Attempt` (iteration number, full `EditPlan`, post-revalidate `UseCaseVerdict`, `Reverted` flag). Bounded by `MaxIterations`. |
| 9 | `EditPlan` shape (surfaced) | **Structured per-file with full content.** `[]EditFile{Path, Op (write\|delete), Content string}`. No diff parsing. Simplest apply and revert. |
| 10 | Observational sensors during re-validation (surfaced) | **Skip and carry forward.** `Revalidator` re-runs assertion sensors only; the original `AggregateSignal` for each observational sensor is passed in at constructor time and substituted into the re-aggregation. Heal-only-observational use cases will exhaust naturally — the user re-runs observationals afterward to confirm. |
| 11 | `Run`'s error vs. `HealResult.Status` (surfaced) | **`Status` is for loop-level outcomes (`healed`/`exhausted`/`abandoned`).** Bare `error` is for infrastructure failures (`Snapshot`/`Apply`/`Revalidate` errors and `ctx.Done()`). LLM errors land in `HealResult.Err` with `Status=abandoned`, not a bare error. |
| 12 | Path validation (surfaced) | **Reject escaping paths before apply.** `EditFile.Path` must be repo-root-relative and free of `..` traversal. Violation → `Status=abandoned`, `Err=ErrInvalidEditPath`, no snapshot taken. |
| 13 | Revert failure semantics (surfaced) | **Log via `Err`, do not retry.** A failed `Revert` is recorded as a joined error on `HealResult.Err` and the prior `Status` is preserved. The caller decides how to surface a dirty working tree. The loop does not attempt a second revert — risks compounding damage. |
| 14 | Loop-internal logging (surfaced) | **None.** The loop never logs. Every condition either returns or sets `HealResult.Err`. The caller (CLI/skill) owns log output. Matches Phase A convention. |

## 4. Package layout

```
internal/
├── runtime/
│   └── healloop/                       (created here; does not exist yet)
│       ├── healloop.go                 Run, Config, HealResult, Status
│       ├── interfaces.go               LLMClient, Transactor, Revalidator,
│                                        PromptInput, EditPlan, EditFile, EditOp,
│                                        Attempt, TxHandle
│       ├── transactor.go               DefaultTransactor auto-detect
│       ├── transactor_git.go           gitTransactor + gitTxHandle
│       ├── transactor_file.go          fileTransactor + fileTxHandle
│       ├── revalidator.go              lifecycleRevalidator (default impl)
│       ├── prompt.go                   BuildPromptInput helper
│       ├── errors.go                   ErrLLMEmptyPlan, ErrInvalidEditPath,
│                                        ErrUseCaseNotFound
│       ├── stubs.go                    test-only stubLLM, stubRevalidator,
│                                        stubTransactor (build-tagged _test only)
│       └── *_test.go
└── policy/                             (existing; additive extension only)
    ├── types.go                        + MaxHealIterations *int / int
    ├── resolve.go                      + resolveMaxHealIterations
    ├── schema.go                       + max_heal_iterations field validation
    └── schema_test.go, resolve_test.go (new test cases listed in §10)

schemas/
└── policy.yaml                         + max_heal_iterations: integer (0..20)
```

Promotion to sub-packages (e.g., `healloop/tx/`) deferred until a file grows past ~300 lines.

## 5. Public API

```go
package healloop

type Status string

const (
    StatusHealed    Status = "healed"
    StatusExhausted Status = "exhausted"   // hit MaxIterations, last attempt reverted
    StatusAbandoned Status = "abandoned"   // LLM refused or returned an unparseable EditPlan
)

type Config struct {
    MaxIterations int               // required; typically copied from policy.EffectivePolicy.MaxHealIterations
    Now           func() time.Time  // optional; defaults to time.Now
}

type HealResult struct {
    Status         Status
    IterationsUsed int
    Attempts       []Attempt                // every attempt, oldest first; includes the successful one
    FinalVerdict   aggregator.UseCaseVerdict
    Err            error                    // populated when Status is ambiguous on its own (LLM error,
                                            // joined revert error, etc.)
}

func Run(
    ctx context.Context,
    verdict aggregator.UseCaseVerdict,
    llm LLMClient,
    tx Transactor,
    rev Revalidator,
    cfg Config,
) (HealResult, error)
```

### 5.1 Interfaces

```go
type LLMClient interface {
    Propose(ctx context.Context, in PromptInput) (EditPlan, error)
}

type Transactor interface {
    Snapshot(ctx context.Context, paths []string) (TxHandle, error)
}

type TxHandle interface {
    Revert() error
    Commit() error   // discard snapshot; for git mode, drops the stash entry
}

type Revalidator interface {
    Revalidate(ctx context.Context, useCaseID string) (aggregator.UseCaseVerdict, error)
}

type SensorLookup interface {
    SensorsForUseCase(useCaseID string) []sensor.Sensor
}

type UseCaseLookup interface {
    Lookup(useCaseID string) (*usecase.UseCase, bool)
}
```

### 5.2 Supporting types

```go
type PromptInput struct {
    UseCase *usecase.UseCase
    Verdict aggregator.UseCaseVerdict        // current failing verdict
    Hints   []aggregator.AngleHint           // == Verdict.HealHints, hoisted for convenience
    History []Attempt                        // prior attempts in this Run, oldest first; empty on iteration 1
}

type EditPlan struct {
    Files     []EditFile
    Rationale string                         // free-text "why I think this fixes it"; surfaced to the caller
                                             // (CLI/skill) for display, never interpreted by the loop
}

type EditOp string
const (
    OpWrite  EditOp = "write"
    OpDelete EditOp = "delete"
)

type EditFile struct {
    Path    string                           // repo-root-relative; rejected if it escapes via ".."
    Op      EditOp
    Content string                           // ignored when Op == OpDelete
}

type Attempt struct {
    Iteration int
    Plan      EditPlan
    Verdict   aggregator.UseCaseVerdict      // post-revalidate verdict
    Reverted  bool                           // always true for entries in History; only the successful
                                             // attempt has Reverted=false
}
```

### 5.3 Default constructors

```go
// DefaultTransactor auto-detects: returns gitTransactor when `git rev-parse --git-dir`
// succeeds in repoRoot, else fileTransactor.
func DefaultTransactor(repoRoot string) Transactor

// DefaultRevalidator wires lifecycle + aggregator with the carry-forward map.
// originalSignals MUST contain entries for every observational sensor in the use case;
// missing entries cause an inconclusive verdict for that angle.
func DefaultRevalidator(
    lc *lifecycle.Lifecycle,
    sensors SensorLookup,
    ucs UseCaseLookup,
    policy *policy.EffectivePolicy,
    originalSignals map[string]aggregate.AggregateSignal, // sensorID → original signal
) Revalidator
```

## 6. Loop body

```
attempts := []

if verdict.Verdict == pass:
    return {Status: healed, IterationsUsed: 0, FinalVerdict: verdict}, nil

for i in 1..cfg.MaxIterations:
    promptIn := PromptInput{UseCase, verdict, verdict.HealHints, attempts}

    plan, err := llm.Propose(ctx, promptIn)
    if err != nil:
        return {Status: abandoned, IterationsUsed: i-1, Attempts: attempts,
                FinalVerdict: verdict, Err: err}, nil
    if len(plan.Files) == 0:
        return {Status: abandoned, IterationsUsed: i-1, Attempts: attempts,
                FinalVerdict: verdict, Err: ErrLLMEmptyPlan}, nil
    if err := validatePaths(plan); err != nil:
        return {Status: abandoned, IterationsUsed: i-1, Attempts: attempts,
                FinalVerdict: verdict, Err: err}, nil

    paths := collectPaths(plan)
    tx, err := transactor.Snapshot(ctx, paths)
    if err != nil:
        return {}, err   // unrecoverable

    if err := applyEdit(plan); err != nil:
        if revertErr := tx.Revert(); revertErr != nil:
            return {}, errors.Join(err, revertErr)
        return {}, err   // unrecoverable

    newVerdict, err := revalidator.Revalidate(ctx, useCaseID)
    if err != nil:
        if revertErr := tx.Revert(); revertErr != nil:
            return {}, errors.Join(err, revertErr)
        return {}, err   // unrecoverable

    if newVerdict.Verdict == pass:
        if commitErr := tx.Commit(); commitErr != nil:
            attempts = append(attempts, Attempt{i, plan, newVerdict, false})
            return {Status: healed, IterationsUsed: i, Attempts: attempts,
                    FinalVerdict: newVerdict, Err: commitErr}, nil
        attempts = append(attempts, Attempt{i, plan, newVerdict, false})
        return {Status: healed, IterationsUsed: i, Attempts: attempts,
                FinalVerdict: newVerdict}, nil

    // Still failing — revert and continue.
    revertErr := tx.Revert()
    attempts = append(attempts, Attempt{i, plan, newVerdict, true})
    if revertErr != nil:
        // Working tree is dirty; record but keep looping is unsafe.
        return {Status: exhausted, IterationsUsed: i, Attempts: attempts,
                FinalVerdict: verdict, Err: revertErr}, nil

return {Status: exhausted, IterationsUsed: cfg.MaxIterations, Attempts: attempts,
        FinalVerdict: verdict}, nil
```

### 6.1 Invariants

1. **Every failing iteration is reverted before the next begins.** On `exhausted` the working tree is byte-identical to `Run`'s entry state.
2. **`verdict` is constant across iterations.** Since every failing iteration is reverted, post-revert state equals entry state, so hints don't change. The loop does not re-revalidate between iterations.
3. **`Status` is set by the exit path, never by guessing.** `healed` = re-validate passed. `exhausted` = ran cap iterations, last attempt reverted. `abandoned` = first `Propose` returned error or empty plan, or `EditPlan` failed path validation.
4. **Bare `error` is reserved for "I can't tell you a result" cases:** `Snapshot`/`Apply`/`Revalidate` failure, `ctx.Done()`. Caller treats these as infrastructure errors.
5. **`Revert` failure on a still-failing iteration is fatal to the loop** (returns `exhausted` early with `Err`), since continuing on a dirty working tree could compound damage.

## 7. Transactionality

### 7.1 `fileTransactor`

```go
type fileTxHandle struct {
    originals map[string][]byte   // path → original bytes (nil = file did not exist)
    created   map[string]bool     // paths that did not exist at snapshot time
    repoRoot  string
}
```

- `Snapshot(paths)`: for each path, record bytes if it exists, mark `created[path] = true` if not.
- `Revert()`: for each entry, if `originals[path] != nil` write it back; else if `created[path]` is true delete the file; else no-op.
- `Commit()`: no-op (the working tree already has the new bytes; the snapshot is just dropped).

### 7.2 `gitTransactor`

```go
type gitTxHandle struct {
    repoRoot string
    stashSHA string           // resolved at snapshot time; immune to other stash ops
    created  map[string]bool  // belt-and-suspenders for newly-created untracked files
}
```

- `Snapshot(paths)`:
  1. `git stash push -u -m "harness-heal-<ulid>" -- <paths>`. If nothing to stash, record an empty handle (revert and commit become no-ops).
  2. Resolve `git rev-parse stash@{0}` → `stashSHA`.
  3. Also record `created[path]` for any path that did not exist before the snapshot (mirrors `fileTransactor`).
- `Revert()`:
  1. `git checkout HEAD -- <paths>` to discard current modifications to tracked target paths.
  2. Delete any files where `created[path]` is true (these were untracked-and-now-created; not in HEAD).
  3. `git stash apply <stashSHA>` to restore the pre-edit dirty state.
  4. `git stash drop <stashSHA>` to clean up.
- `Commit()`: `git stash drop <stashSHA>` only. Keep edits in the working tree for the user to inspect and commit.

### 7.3 Why these choices

- **Scope the stash to `-- <paths>`:** the user may have unrelated uncommitted changes; heal must not touch them.
- **Resolve to SHA, not `stash@{0}`:** positional refs drift if anything else manipulates the stash mid-run. SHA is stable.
- **`git apply` + `git stash drop` instead of `git stash pop`:** `pop` targets the positional ref and is `apply && drop` under the hood. Splitting them lets us target the SHA we recorded.
- **Track `created` paths even in git mode:** `git stash push -u` records untracked files in the stash, but `git checkout HEAD --` can't undo their creation (they weren't in HEAD). Explicit tracking handles this.

## 8. Prompt construction

`healloop` produces structured `PromptInput`, not a rendered string. The `LLMClient` impl owns prompt-template formatting and any LLM-specific concerns.

`PromptInput` carries:

- The full `UseCase` (Given/When/Then text, EntryPoints, FixtureIDs).
- The current failing `UseCaseVerdict` (with `Archetype`, `EvaluatedAngles`, `FailingAngles`, `WarningAngles`, `HealHints`).
- `Hints` hoisted as a top-level slice (`== Verdict.HealHints`) for ergonomic access.
- `History` of prior `Attempt`s in this `Run`, oldest first, empty on iteration 1.

Notable omissions, by design:

- **No raw signal payloads.** The aggregator's `HealHint.Summary` and `Rationale` already condense them.
- **No fixture bytes.** If a `Locus` points at a fixture file, the LLM impl can read it from disk via the `Locus.Path`.
- **No stack manifest.** Same reasoning: the impl can load what it needs.

`History` entries carry the **full content** of every prior `EditPlan`, not just file paths. This lets the LLM reason about what specifically it tried. Token cost is bounded by `MaxIterations` (default 3) × the size of the file(s) the LLM itself produced.

`BuildPromptInput(verdict, history, useCaseLookup)` is a thin constructor in `prompt.go` that looks up the `UseCase` by `verdict.UseCaseID` and assembles the struct.

## 9. Re-validation wiring

`lifecycleRevalidator` is the default `Revalidator`:

```go
type lifecycleRevalidator struct {
    lc              *lifecycle.Lifecycle
    sensors         SensorLookup
    ucs             UseCaseLookup
    policy          *policy.EffectivePolicy
    originalSignals map[string]aggregate.AggregateSignal  // sensorID → carry-forward
}

func (r *lifecycleRevalidator) Revalidate(ctx, useCaseID) (aggregator.UseCaseVerdict, error) {
    uc, ok := r.ucs.Lookup(useCaseID)
    if !ok { return {}, ErrUseCaseNotFound }

    sensors := r.sensors.SensorsForUseCase(useCaseID)
    ordered, err := sensor.ResolveExecutionOrder(sensors)
    if err != nil { return {}, err }

    aggs := make([]aggregate.AggregateSignal, 0, len(ordered))
    for _, s := range ordered {
        if s.Kind == enums.KindObservational {
            if orig, ok := r.originalSignals[s.ID]; ok {
                aggs = append(aggs, orig)
            }
            continue
        }
        agg, err := r.lc.RunSensor(ctx, s.ID, nil)
        if err != nil { return {}, err }
        aggs = append(aggs, agg)
    }

    return aggregator.UseCase(uc, uc.Archetype(), aggs, ordered, r.policy)
}
```

**Sensor execution is serial** in v1. `sensor.ResolveExecutionOrder` returns a topo-sorted list; we iterate it sequentially. Parallel-where-DAG-allows is deferred (cost is low: one use case, typically <10 sensors).

**`SensorLookup` and `UseCaseLookup` are interfaces** so the loop's tests don't pull in `*sensor.Store` or `*usecase.Store`.

**`expectedObs` passed to `RunSensor` is always nil** because we skip observational sensors.

## 10. Policy extension

Additive change to `internal/policy`:

```go
// internal/policy/types.go
type ValidationPolicy struct {
    SchemaVersion     string
    Scope             Scope
    PerArchetype      map[enums.Archetype]ArchetypeBlock
    InferentialFloor  *float64
    MaxHealIterations *int    // NEW; nil = use default
}

type EffectivePolicy struct {
    SchemaVersion     string
    ResolvedFrom      []string
    PerArchetype      map[enums.Archetype]map[enums.ValidationAngle]AngleStatus
    InferentialFloor  float64
    MaxHealIterations int     // NEW; always populated post-Resolve
}

const DefaultMaxHealIterations = 3
```

```go
// internal/policy/resolve.go
func resolveMaxHealIterations(global, local *ValidationPolicy) int {
    if local != nil && local.MaxHealIterations != nil {
        return *local.MaxHealIterations
    }
    if global != nil && global.MaxHealIterations != nil {
        return *global.MaxHealIterations
    }
    return DefaultMaxHealIterations
}
```

Schema (`schemas/policy.yaml`): `max_heal_iterations: integer, minimum: 0, maximum: 20`. The loader rejects out-of-range values with a typed `ErrMaxHealIterationsOutOfRange`, mirroring the inferential-floor pattern.

Existing `EffectivePolicy` golden test data needs a mechanical update to include the new field; this is internal-only (no consumer schema impact).

Wiring: the CLI/skill caller does `cfg := healloop.Config{MaxIterations: effectivePolicy.MaxHealIterations, ...}`. `healloop` does not import `internal/policy`.

## 11. Error handling — full exit matrix

| Trigger | Returns | `Status` | `Err` | Working tree |
|---|---|---|---|---|
| Input verdict already passing | `HealResult`, nil error | `healed` | nil | unchanged |
| `MaxIterations == 0` | `HealResult`, nil error | `exhausted` | nil | unchanged |
| Iteration N re-validate → `pass` | `HealResult`, nil error | `healed` | nil | edits committed (kept) |
| All iterations failed, last reverted | `HealResult`, nil error | `exhausted` | nil | restored to entry state |
| `Commit()` failed on a healed iteration (git mode: `git stash drop` failed) | `HealResult`, nil error | `healed` | commit error | edits in place; stash entry leaks (visible via `git stash list`) |
| `Propose` returned error | `HealResult`, nil error | `abandoned` | wrapped Propose error | unchanged (no snapshot taken) |
| `Propose` returned empty `EditPlan` | `HealResult`, nil error | `abandoned` | `ErrLLMEmptyPlan` | unchanged |
| `EditPlan` path escapes repo root | `HealResult`, nil error | `abandoned` | `ErrInvalidEditPath` | unchanged |
| `Transactor.Snapshot` failed | zero `HealResult`, bare error | — | — | unchanged |
| `applyEdit` failed mid-iteration | zero `HealResult`, bare error | — | — | reverted (best-effort, joined) |
| `Revalidator.Revalidate` failed | zero `HealResult`, bare error | — | — | reverted (best-effort, joined) |
| `ctx.Done()` mid-iteration | zero `HealResult`, `ctx.Err()` | — | — | reverted (best-effort, joined) |
| `Revert()` failed on a failing iteration | `HealResult`, nil error | `exhausted` (early) | revert error | dirty (logged) |

### 11.1 Error sentinels (in `errors.go`)

```go
var (
    ErrLLMEmptyPlan    = errors.New("healloop: LLM returned empty EditPlan")
    ErrInvalidEditPath = errors.New("healloop: EditFile.Path escapes repo root")
    ErrUseCaseNotFound = errors.New("healloop: use case not found in revalidator")
)
```

Additionally in `internal/policy`:

```go
var ErrMaxHealIterationsOutOfRange = errors.New("policy: max_heal_iterations out of range [0, 20]")
```

### 11.2 Path validation

Every `EditFile.Path`:

- Must be non-empty.
- Must be relative (no leading `/`, no Windows drive letter).
- After `filepath.Clean`, must not start with `..` or contain a `..` segment.

The `repoRoot` passed to `DefaultTransactor` is the implicit base for all paths. A future tightening could add an explicit `Config.RepoRoot` field for absolute-vs-relative containment checks; deferred unless a test surfaces a regression.

Violation → loop exits with `Status=abandoned`, `Err=ErrInvalidEditPath`, no snapshot taken.

### 11.3 Loop-internal logging

None. The loop never logs directly. Every error path either returns or stores into `HealResult.Err`. The caller decides log format and verbosity.

### 11.4 Idempotency

Calling `Run` twice on the same already-healed input is safe: the first call returns `healed` immediately with no LLM call; the second call sees `Verdict == pass` and short-circuits identically.

## 12. Testing strategy

All tests in `internal/runtime/healloop/*_test.go`. Run `-race` clean. No `cmd/` integration test — that's B5/B6.

### 12.1 Test scaffolding (`stubs.go`, test-only)

```go
type stubLLM struct {
    plans []EditPlan
    err   error
    calls int
    onCall func(in PromptInput)  // assert on PromptInput contents
}

type stubRevalidator struct {
    verdicts []aggregator.UseCaseVerdict
    err      error
    calls    int
}

type stubTransactor struct {
    snapshots []*recordingTxHandle  // captures Snapshot/Revert/Commit
    snapErr   error
}
```

### 12.2 Loop behavior tests (`healloop_test.go`)

| # | Test | What it asserts |
|---|---|---|
| 1 | `Run_HealsOnFirstIteration_WhenLLMProposesValidEdit` | `Status=healed`, `IterationsUsed=1`, 1 snapshot, 1 apply, 1 commit, 0 reverts |
| 2 | `Run_Exhausts_WhenLLMProposesBadEditRepeatedly` | cap=3, `Status=exhausted`, `IterationsUsed=3`, 3 reverts, working tree byte-identical to entry |
| 3 | `Run_Abandons_WhenLLMReturnsError` | `Status=abandoned`, `IterationsUsed=0`, `Err` wraps LLM error, no snapshot taken |
| 4 | `Run_Abandons_WhenLLMReturnsEmptyPlan` | `Status=abandoned`, `Err=ErrLLMEmptyPlan` |
| 5 | `Run_Abandons_WhenEditPlanContainsEscapingPath` | `Status=abandoned`, `Err=ErrInvalidEditPath`, no apply called |
| 6 | `Run_ShortCircuits_WhenInputAlreadyPassing` | `Status=healed`, `IterationsUsed=0`, LLM never called |
| 7 | `Run_HealsOnIteration2_WithHistoryInPrompt` | iteration 2's `PromptInput.History` contains iteration 1's `Attempt` (stub LLM asserts via `onCall`) |
| 8 | `Run_PropagatesCtxCancellation` | cancel `ctx` between `Propose` and Apply → bare `ctx.Err()`, revert called |
| 9 | `Run_ReturnsBareError_WhenSnapshotFails` | zero `HealResult`, bare error, LLM not called |
| 10 | `Run_ReturnsExhaustedEarly_WhenRevertFails` | `Status=exhausted`, `Err` set to revert error |
| 11 | `Run_AttemptsCarryRevertedFlag` | every entry in `Attempts` for an exhausted run has `Reverted=true`; the successful attempt has `Reverted=false` |

### 12.3 Transactor tests (`transactor_test.go`)

| # | Test | What it asserts |
|---|---|---|
| 12 | `FileTransactor_RestoresOriginalBytes` | write → snapshot → modify → revert → byte-identical |
| 13 | `FileTransactor_DeletesCreatedFiles_OnRevert` | snapshot a path that doesn't exist → create file → revert → file is gone |
| 14 | `FileTransactor_Commit_IsNoOp` | snapshot → modify → commit → new bytes remain on disk |
| 15 | `GitTransactor_RestoresViaStashApply` | `git init` temp dir → snapshot → modify → revert → `git diff` clean. Skip with `t.Skip` if `git` not in `$PATH`. |
| 16 | `GitTransactor_PreservesUnrelatedDirtyState` | dirty an unrelated file, snapshot only target, modify target, revert → target restored, unrelated still dirty |
| 17 | `GitTransactor_Commit_DropsStashKeepsEdits` | snapshot → modify → commit → file has modifications, `git stash list` does not contain `harness-heal-*` entry |
| 18 | `GitTransactor_DeletesCreatedFiles_OnRevert` | snapshot a nonexistent path → create → revert → file gone (even though `git checkout HEAD --` can't undo a never-tracked file) |

### 12.4 Revalidator integration test (`revalidator_test.go`)

| # | Test | What it asserts |
|---|---|---|
| 19 | `LifecycleRevalidator_SkipsObservational_CarriesForwardSignals` | 2-sensor use case (1 assertion + 1 observational), construct with original observational signal → `RunSensor` called only for assertion sensor; final `UseCaseVerdict` aggregates both signals |
| 20 | `LifecycleRevalidator_ReturnsErrUseCaseNotFound_OnUnknownID` | `ErrUseCaseNotFound` returned, no `RunSensor` calls |

### 12.5 Policy extension tests (`internal/policy/*_test.go`, additive)

| # | Test | What it asserts |
|---|---|---|
| 21 | `Resolve_MaxHealIterations_DefaultsTo3_WhenUnset` | both global and local nil → 3 |
| 22 | `Resolve_MaxHealIterations_GlobalSet_LocalUnset` | global=5, local=nil → 5 |
| 23 | `Resolve_MaxHealIterations_LocalOverridesGlobal` | global=5, local=8 → 8 |
| 24 | `Loader_RejectsNegativeMaxHealIterations` | yaml with `-1` → `ErrMaxHealIterationsOutOfRange` |
| 25 | `Loader_RejectsAboveCeiling` | yaml with `21` → `ErrMaxHealIterationsOutOfRange` |
| 26 | `Loader_LoadsValidMaxHealIterations` | yaml with `5` → resolves to 5, round-trips through `Load → Resolve → Serialize` |

### 12.6 Coverage targets

- `healloop.go`: every branch in the loop body covered by a test in §12.2.
- `transactor_git.go` / `transactor_file.go`: every revert/commit path covered.
- `errors.go`: each typed error asserted by `errors.Is` in at least one test.

### 12.7 No golden tests

`HealResult` is dominated by stub-injected verdicts and plans. Golden files would test the stubs, not the loop. The Phase A golden-test pattern fits deterministic transformers; orchestration loops are exercised by stubbed-collaborator assertions instead.

## 13. Dependencies and consumers

**Imports from B3:**

- `context`, `errors`, `os`, `os/exec`, `path/filepath`, `time` (stdlib)
- `github.com/iurykrieger/lastro/internal/aggregate` (for `HealHint`, `Locus`, `AggregateSignal`)
- `github.com/iurykrieger/lastro/internal/enums` (for `KindObservational`)
- `github.com/iurykrieger/lastro/internal/lifecycle` (default revalidator only)
- `github.com/iurykrieger/lastro/internal/policy` (consumers convert to `Config`; `healloop` itself does not import this)
- `github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase` (for `UseCaseVerdict`, `AngleHint`)
- `github.com/iurykrieger/lastro/internal/sensor` (for `Sensor`, `ResolveExecutionOrder`)
- `github.com/iurykrieger/lastro/internal/usecase` (for `*UseCase`)
- `github.com/oklog/ulid/v2` (for stash message uniqueness)

**Consumers of B3 (later chunks):**

- B5 (skill wrappers) — `/heal` script implements `LLMClient` and constructs `DefaultTransactor` + `DefaultRevalidator`, then calls `healloop.Run`.
- B6 (CLI) — `harness heal <use-case-id>` reads the last failing run's signals, builds the carry-forward map, and invokes `healloop.Run` via the same path the skill uses.

## 14. Out of scope / deferred

- **`AffectedSensors` helper.** Re-validation is use-case-scoped; no caller needs file-to-sensor mapping in v1. Add when a CLI command like `harness explain --file <path>` needs it.
- **Parallel sensor execution in re-validation.** Serial works; latency cost is low for typical use cases.
- **Concurrent heal of multiple use cases.** Caller loops over failing verdicts serially.
- **Re-running observational sensors during heal.** Carry-forward is sufficient; explicit observational re-runs deferred until evidence shows the carry-forward strategy misleads users.
- **Sensor regeneration after heal.** If the LLM's edit changes the stack manifest, sensors should arguably be regenerated; that's a B4 concern and a Phase-C policy question.
- **`Config.RepoRoot` for path scoping.** Path validation in §11.2 uses `filepath.Clean` heuristics. Tight repo-root scoping deferred unless a test surfaces a regression.
- **Promotion of sub-packages.** All files live under `internal/runtime/healloop/` until a single file grows past ~300 lines.
