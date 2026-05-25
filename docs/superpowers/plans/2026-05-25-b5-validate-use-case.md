# B5 Sub-PR 2 — `/validate-use-case` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the second B5 sub-PR — the `/validate-use-case` skill that runs every sensor for one use case in DAG order (with parallelism), aggregates the verdicts via `aggregator.UseCase`, writes `verdict.json`, and emits the terminal `UseCaseVerdict` JSON.

**Architecture:** Adds one shared helper — `lib/skillruntime/scheduler.go` (DAG-aware worker pool with skipped-dependent synthesis) — and one skill: `skills/validate-use-case/{skill.md, scripts/main.go}`. Wraps existing internal packages: `internal/sensor.ResolveExecutionOrder` (validation only), `internal/runtime/aggregator/usecase.UseCase`, `internal/policy.Resolve`, and `internal/lifecycle.RunSensor`. Builds on sub-PR 1's `lib/skillio` and `lib/skillruntime.BootLifecycle`.

**Tech Stack:** Go 1.24, `golang.org/x/sync/errgroup` (already in go.mod via B2), stdlib `flag`/`os`/`encoding/json`. No new module deps.

**Source spec:** [`docs/superpowers/specs/2026-05-25-b5-skill-wrappers-design.md`](../specs/2026-05-25-b5-skill-wrappers-design.md) §8.5 (flow), §9 (scheduler), §11.2 (acceptance).

**Branching note:** Per the design's "three independent sub-PRs" rule each sub-PR branches from fresh `origin/main`. In practice sub-PR 2 needs `lib/skillio` and `lib/skillruntime/{handles,boot}.go` from sub-PR 1, so this plan continues on the existing branch (`claude/flamboyant-carson-1dfa37`) which already contains those packages. PR #17 absorbs sub-PR 2 commits; can be split later if needed.

---

## File structure

```
lib/skillruntime/
└── scheduler.go              DAG-aware parallel runner with skipped-dependent synthesis
└── scheduler_test.go         table-driven coverage of topology + failure propagation

skills/validate-use-case/
├── skill.md
├── scripts/main.go
└── scripts/main_test.go
```

**File responsibilities:**

- `lib/skillruntime/scheduler.go` — `RunAll(ctx, sensors, runner, parallelism) ([]aggregate.AggregateSignal, error)`: schedules sensors honoring `depends_on`, calls `runner(ctx, s)` concurrently up to `parallelism`, synthesizes `inconclusive`/`stopped` aggregates for sensors whose deps failed.
- `skills/validate-use-case/skill.md` — LLM-facing prompt; under 200 lines.
- `skills/validate-use-case/scripts/main.go` — `main()` and testable `run()`: loads use case, filters sensors, resolves policy, picks an in-scope archetype, schedules sensors via `RunAll`, calls `aggregator.UseCase`, writes `verdict.json`, emits terminal verdict, exits per `Verdict`.

---

## Task 1: `lib/skillruntime/scheduler.go`

**Files:**
- Create: `lib/skillruntime/scheduler.go`
- Test: `lib/skillruntime/scheduler_test.go`

The scheduler accepts a slice of `sensor.Sensor` and a `runner func(context.Context, sensor.Sensor) (aggregate.AggregateSignal, error)` and returns results in `sensor.ID`-sorted order. When a sensor's verdict is `fail`, its transitive dependents are not executed — instead they receive a synthetic `inconclusive`/`stopped` aggregate with a `heal_hint` whose `summary` matches `"skipped: depends_on <id> failed"`.

Key behaviors verified by tests:
1. Empty input → empty result.
2. Single passing sensor → one aggregate.
3. Linear chain A → B → C all pass → 3 aggregates in order.
4. Linear chain A → B → C where A fails → A.agg, B+C synthesized as skipped.
5. Diamond (A → {B,C} → D) where B fails → A passes, B fails, C passes (independent of B), D synthesized as skipped (depends_on B).
6. Cycle in `depends_on` → returns the underlying `*sensor.ErrCycle` from `ResolveExecutionOrder` (validates topology before running).
7. `ctx` cancelled mid-run → remaining sensors not yet started get `inconclusive` aggregates; in-flight sensors honor the cancel via the runner.
8. Parallelism: with `parallelism=2` and 3 independent passing sensors, the runner is called concurrently (test verifies overlap).

**Skipped-aggregate shape** (per design §9 + F4):

```go
aggregate.AggregateSignal{
    SchemaVersion:     "1.0.0",
    SensorID:          dep.ID,
    UseCaseID:         dep.UseCaseID,
    Angle:             dep.Angle,
    Kind:              dep.Kind,
    OutputType:        dep.OutputType,
    EmittedAt:         now,
    Verdict:           enums.VerdictInconclusive,
    Confidence:        0.0,
    TerminationReason: enums.TerminationStopped,
    HealHint: &aggregate.HealHint{
        Summary:   fmt.Sprintf("skipped: depends_on %s failed", failedID),
        Rationale: fmt.Sprintf("sensor %s did not execute because %s's AggregateSignal verdict=fail", dep.ID, failedID),
    },
}
```

Look at `internal/aggregate/types.go` for the exact field names before writing code; if some optional fields are required by validators, omit only what is genuinely optional.

### Step 1: Write the failing tests

Create `lib/skillruntime/scheduler_test.go`:

```go
package skillruntime

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/sensor"
)

func mkSensor(id string, deps ...string) sensor.Sensor {
	return sensor.Sensor{
		SchemaVersion: "1.0.0",
		ID:            id,
		UseCaseID:     "uc",
		Angle:         enums.AngleBuild,
		Kind:          enums.KindAssertion,
		Nature:        enums.NatureComputational,
		OutputType:    enums.OutputSingleShot,
		Uses:          []string{"fake"},
		DependsOn:     deps,
		Steps:         []sensor.Step{{ID: "only", Run: "true"}},
	}
}

func passingAgg(s sensor.Sensor) aggregate.AggregateSignal {
	return aggregate.AggregateSignal{
		SchemaVersion: "1.0.0",
		SensorID:      s.ID,
		UseCaseID:     s.UseCaseID,
		Angle:         s.Angle,
		Kind:          s.Kind,
		OutputType:    s.OutputType,
		EmittedAt:     time.Now().UTC(),
		Verdict:       enums.VerdictPass,
		Confidence:    1.0,
	}
}

func failingAgg(s sensor.Sensor) aggregate.AggregateSignal {
	return aggregate.AggregateSignal{
		SchemaVersion:     "1.0.0",
		SensorID:          s.ID,
		UseCaseID:         s.UseCaseID,
		Angle:             s.Angle,
		Kind:              s.Kind,
		OutputType:        s.OutputType,
		EmittedAt:         time.Now().UTC(),
		Verdict:           enums.VerdictFail,
		Confidence:        1.0,
		TerminationReason: enums.TerminationCompleted,
		HealHint:          &aggregate.HealHint{Summary: "fake fail", Rationale: "for testing"},
	}
}

func TestRunAll_Empty(t *testing.T) {
	got, err := RunAll(context.Background(), nil, func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		t.Fatalf("runner called with empty input")
		return aggregate.AggregateSignal{}, nil
	}, 4)
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d results, want 0", len(got))
	}
}

func TestRunAll_SinglePass(t *testing.T) {
	s := mkSensor("a")
	got, err := RunAll(context.Background(), []sensor.Sensor{s}, func(ctx context.Context, sn sensor.Sensor) (aggregate.AggregateSignal, error) {
		return passingAgg(sn), nil
	}, 4)
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if len(got) != 1 || got[0].SensorID != "a" || got[0].Verdict != enums.VerdictPass {
		t.Errorf("unexpected results: %+v", got)
	}
}

func TestRunAll_LinearChain_AllPass(t *testing.T) {
	a := mkSensor("a")
	b := mkSensor("b", "a")
	c := mkSensor("c", "b")
	got, err := RunAll(context.Background(), []sensor.Sensor{a, b, c}, func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		return passingAgg(s), nil
	}, 4)
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
	// Sorted-by-id output (canonical).
	ids := []string{got[0].SensorID, got[1].SensorID, got[2].SensorID}
	if !sort.StringsAreSorted(ids) {
		t.Errorf("results not sorted by id: %v", ids)
	}
}

func TestRunAll_LinearChain_FirstFails_RestSkipped(t *testing.T) {
	a := mkSensor("a")
	b := mkSensor("b", "a")
	c := mkSensor("c", "b")
	called := map[string]bool{}
	got, err := RunAll(context.Background(), []sensor.Sensor{a, b, c}, func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		called[s.ID] = true
		if s.ID == "a" {
			return failingAgg(s), nil
		}
		return passingAgg(s), nil
	}, 4)
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if !called["a"] || called["b"] || called["c"] {
		t.Errorf("expected only A to run; called=%v", called)
	}
	byID := map[string]aggregate.AggregateSignal{}
	for _, g := range got {
		byID[g.SensorID] = g
	}
	if byID["a"].Verdict != enums.VerdictFail {
		t.Errorf("a verdict = %q, want fail", byID["a"].Verdict)
	}
	if byID["b"].Verdict != enums.VerdictInconclusive || byID["b"].TerminationReason != enums.TerminationStopped {
		t.Errorf("b should be skipped; got %+v", byID["b"])
	}
	if byID["b"].HealHint == nil || !strings.Contains(byID["b"].HealHint.Summary, "skipped: depends_on a failed") {
		t.Errorf("b heal_hint missing or wrong: %+v", byID["b"].HealHint)
	}
	if byID["c"].Verdict != enums.VerdictInconclusive {
		t.Errorf("c should also be skipped (transitive); got %+v", byID["c"])
	}
}

func TestRunAll_Diamond_IndependentSiblingsBothRun(t *testing.T) {
	a := mkSensor("a")
	b := mkSensor("b", "a")
	c := mkSensor("c", "a")
	d := mkSensor("d", "b", "c")
	called := map[string]bool{}
	got, err := RunAll(context.Background(), []sensor.Sensor{a, b, c, d}, func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		called[s.ID] = true
		if s.ID == "b" {
			return failingAgg(s), nil
		}
		return passingAgg(s), nil
	}, 4)
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if !called["a"] || !called["b"] || !called["c"] || called["d"] {
		t.Errorf("expected a/b/c to run, d skipped; called=%v", called)
	}
	byID := map[string]aggregate.AggregateSignal{}
	for _, g := range got {
		byID[g.SensorID] = g
	}
	if byID["c"].Verdict != enums.VerdictPass {
		t.Errorf("c should pass independently; got %q", byID["c"].Verdict)
	}
	if byID["d"].Verdict != enums.VerdictInconclusive {
		t.Errorf("d should be skipped; got %q", byID["d"].Verdict)
	}
}

func TestRunAll_CycleErrors(t *testing.T) {
	a := mkSensor("a", "b")
	b := mkSensor("b", "a")
	_, err := RunAll(context.Background(), []sensor.Sensor{a, b}, func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		return passingAgg(s), nil
	}, 4)
	var cyc *sensor.ErrCycle
	if !errors.As(err, &cyc) {
		t.Errorf("expected *sensor.ErrCycle, got %v", err)
	}
}

func TestRunAll_ContextCancel(t *testing.T) {
	a := mkSensor("a")
	b := mkSensor("b")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before any work
	got, err := RunAll(ctx, []sensor.Sensor{a, b}, func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		t.Fatalf("runner should not be called after cancel")
		return aggregate.AggregateSignal{}, ctx.Err()
	}, 4)
	if err == nil {
		t.Errorf("expected error from cancelled ctx, got nil")
	}
	_ = got // we don't care about partial results here
}

func TestRunAll_ParallelismHigherThanOne(t *testing.T) {
	a := mkSensor("a")
	b := mkSensor("b")
	c := mkSensor("c")
	var concurrent, maxConcurrent int32
	runner := func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		cur := atomic.AddInt32(&concurrent, 1)
		defer atomic.AddInt32(&concurrent, -1)
		// remember the peak
		for {
			prev := atomic.LoadInt32(&maxConcurrent)
			if cur <= prev || atomic.CompareAndSwapInt32(&maxConcurrent, prev, cur) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond) // hold the slot to let siblings spin up
		return passingAgg(s), nil
	}
	_, err := RunAll(context.Background(), []sensor.Sensor{a, b, c}, runner, 3)
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if atomic.LoadInt32(&maxConcurrent) < 2 {
		t.Errorf("expected peak concurrency ≥ 2 with parallelism=3, got %d", maxConcurrent)
	}
}
```

### Step 2: Run tests to verify they fail

Run from repo root:
```
go test ./lib/skillruntime/... -run RunAll
```
Expected: FAIL with "undefined: RunAll".

### Step 3: Implement `scheduler.go`

Create `lib/skillruntime/scheduler.go`:

```go
package skillruntime

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/sensor"
)

// SensorRunner executes one sensor and returns its terminal aggregate.
// Implementations typically wrap *lifecycle.Lifecycle.RunSensor.
type SensorRunner func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error)

// RunAll schedules sensors honoring their depends_on edges and returns
// aggregates in sensor.ID-sorted order. Sensors whose transitive dep
// failed receive a synthesized inconclusive/stopped aggregate (never
// invoke runner for them).
//
// parallelism caps concurrent runner invocations; values <1 are treated
// as 1. Cycles or dangling deps surface as *sensor.ErrCycle /
// *sensor.ErrMissingDependency via ResolveExecutionOrder's pre-check.
func RunAll(ctx context.Context, sensors []sensor.Sensor, runner SensorRunner, parallelism int) ([]aggregate.AggregateSignal, error) {
	if len(sensors) == 0 {
		return []aggregate.AggregateSignal{}, nil
	}
	if parallelism < 1 {
		parallelism = 1
	}

	// Reuse ResolveExecutionOrder solely for topology validation
	// (cycles + dangling edges). The actual order we follow is dynamic:
	// sensors run as soon as all their deps are done.
	if _, err := sensor.ResolveExecutionOrder(sensors); err != nil {
		return nil, err
	}

	byID := make(map[string]sensor.Sensor, len(sensors))
	for _, s := range sensors {
		byID[s.ID] = s
	}

	// Adjacency + in-degree.
	deps := make(map[string][]string, len(sensors))  // s.ID → sensors it waits for
	out := make(map[string][]string, len(sensors))   // s.ID → sensors waiting for it
	inDeg := make(map[string]int, len(sensors))
	for _, s := range sensors {
		deps[s.ID] = append([]string(nil), s.DependsOn...)
		inDeg[s.ID] = len(s.DependsOn)
	}
	for _, s := range sensors {
		for _, d := range s.DependsOn {
			out[d] = append(out[d], s.ID)
		}
	}

	var (
		mu        sync.Mutex
		done      = make(map[string]aggregate.AggregateSignal, len(sensors))
		failed    = make(map[string]string) // s.ID → failing ancestor's id (closest known)
		ready     []string
		results   []aggregate.AggregateSignal
		runErr    error
		semaphore = make(chan struct{}, parallelism)
		wg        sync.WaitGroup
	)

	// Seed: every sensor with zero deps is ready.
	for id, d := range inDeg {
		if d == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// runOne handles a single sensor: synthesizes if its ancestor failed,
	// otherwise calls runner. Records result, then propagates: decrements
	// dependents' in-degree and enqueues newly-ready ones.
	var runOne func(id string)
	runOne = func(id string) {
		defer wg.Done()
		defer func() { <-semaphore }()

		s := byID[id]
		var agg aggregate.AggregateSignal

		mu.Lock()
		ancestor, skipped := failed[id]
		mu.Unlock()

		if skipped {
			agg = synthesizeSkipped(s, ancestor)
		} else {
			if cctx.Err() != nil {
				agg = synthesizeCancelled(s, cctx.Err())
			} else {
				var err error
				agg, err = runner(cctx, s)
				if err != nil {
					mu.Lock()
					if runErr == nil {
						runErr = fmt.Errorf("scheduler: sensor %s: %w", s.ID, err)
					}
					mu.Unlock()
					cancel()
					return
				}
			}
		}

		mu.Lock()
		done[id] = agg
		mu.Unlock()

		// Propagate failure to transitive dependents.
		if agg.Verdict == enums.VerdictFail {
			mu.Lock()
			markSkipped(id, id, out, failed)
			mu.Unlock()
		}

		// Enqueue dependents whose remaining in-degree reaches zero.
		mu.Lock()
		var newlyReady []string
		for _, dep := range out[id] {
			inDeg[dep]--
			if inDeg[dep] == 0 {
				newlyReady = append(newlyReady, dep)
			}
		}
		mu.Unlock()
		sort.Strings(newlyReady)
		for _, n := range newlyReady {
			wg.Add(1)
			semaphore <- struct{}{}
			go runOne(n)
		}
	}

	for _, id := range ready {
		wg.Add(1)
		semaphore <- struct{}{}
		go runOne(id)
	}
	wg.Wait()

	if runErr != nil {
		return nil, runErr
	}
	if cctx.Err() != nil && ctx.Err() != nil {
		// The caller's context was cancelled.
		return nil, ctx.Err()
	}

	// Canonical sorted output by sensor id.
	for _, s := range sensors {
		if agg, ok := done[s.ID]; ok {
			results = append(results, agg)
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].SensorID < results[j].SensorID })
	return results, nil
}

// markSkipped marks every transitive dependent of rootFailed as skipped,
// recording the immediate failing ancestor each one descends from.
// Caller holds mu.
func markSkipped(failedID, ancestorForChildren string, out map[string][]string, failed map[string]string) {
	for _, dep := range out[failedID] {
		if _, already := failed[dep]; already {
			continue
		}
		failed[dep] = ancestorForChildren
		markSkipped(dep, ancestorForChildren, out, failed)
	}
}

func synthesizeSkipped(dep sensor.Sensor, failedID string) aggregate.AggregateSignal {
	return aggregate.AggregateSignal{
		SchemaVersion:     "1.0.0",
		SensorID:          dep.ID,
		UseCaseID:         dep.UseCaseID,
		Angle:             dep.Angle,
		Kind:              dep.Kind,
		OutputType:        dep.OutputType,
		EmittedAt:         time.Now().UTC(),
		Verdict:           enums.VerdictInconclusive,
		Confidence:        0.0,
		TerminationReason: enums.TerminationStopped,
		HealHint: &aggregate.HealHint{
			Summary:   fmt.Sprintf("skipped: depends_on %s failed", failedID),
			Rationale: fmt.Sprintf("sensor %s did not execute because %s's AggregateSignal verdict=fail", dep.ID, failedID),
		},
	}
}

func synthesizeCancelled(s sensor.Sensor, err error) aggregate.AggregateSignal {
	return aggregate.AggregateSignal{
		SchemaVersion:     "1.0.0",
		SensorID:          s.ID,
		UseCaseID:         s.UseCaseID,
		Angle:             s.Angle,
		Kind:              s.Kind,
		OutputType:        s.OutputType,
		EmittedAt:         time.Now().UTC(),
		Verdict:           enums.VerdictInconclusive,
		Confidence:        0.0,
		TerminationReason: enums.TerminationStopped,
		HealHint: &aggregate.HealHint{
			Summary:   "skipped: scheduler cancelled before this sensor ran",
			Rationale: err.Error(),
		},
	}
}
```

**Important:** verify the exact field names of `aggregate.AggregateSignal` and `aggregate.HealHint` against `internal/aggregate/types.go`. If a required field is missing in the synthesizers (`enums.TerminationStopped` is correct), tests may fail or downstream aggregators may reject the signal. Fix as needed — the test bodies above expect a `HealHint` pointer field; align if the real type differs.

### Step 4: Run tests to verify they pass

```
go test ./lib/skillruntime/... -count=1
```
Expected: PASS, including the 8 new RunAll tests + the 10 existing (handles + boot).

### Step 5: Race test

```
go test -race ./lib/skillruntime/... -count=1
```
Expected: PASS, no race warnings (the scheduler touches `done`/`failed`/`inDeg` under `mu`).

### Step 6: Commit

```bash
git add lib/skillruntime/scheduler.go lib/skillruntime/scheduler_test.go
git commit -m "feat(skillruntime): DAG-aware RunAll scheduler with skipped-dependent synthesis"
```

---

## Task 2: `skills/validate-use-case/skill.md`

**Files:**
- Create: `skills/validate-use-case/skill.md`

### Step 1: Write the markdown

Create `skills/validate-use-case/skill.md`:

```markdown
---
name: validate-use-case
description: Run every sensor for one use case in DAG order, aggregate the verdicts, emit a UseCaseVerdict. Exit non-zero on fail/inconclusive.
---

# /validate-use-case

Run every sensor that validates the given use case (parallel within DAG
levels), aggregate their `AggregateSignal`s into a `UseCaseVerdict`, and
emit it on stdout. Writes `verdict.json` under `.harness/runtime/`.

## Usage

```
/validate-use-case <usecase-id>
```

`<usecase-id>` is the `id` field of a use-case YAML under
`.harness/use-cases/`.

## Output

- **stdout** — JSONL stream:
  - One line per sensor's terminal `AggregateSignal` (in sensor-id order)
  - Final line: the `UseCaseVerdict` (also written to `verdict.json`)
- **stderr** — empty on success; `ScriptError` envelope on failure.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | `UseCaseVerdict.verdict == pass` |
| 1 | `UseCaseVerdict.verdict == fail` |
| 2 | `UseCaseVerdict.verdict == inconclusive` |
| 3 | Script-level error (use case not found, cycle, runner error) |

## Outputs

- `.harness/runtime/use-cases/<usecase-id>/<run-id>/verdict.json` — the
  full `UseCaseVerdict` plus `sensor_runs` traceability entries listing
  every sensor's run id.

## Constraints

- The use case must have at least one entry in `archetype_scope`; the
  first entry is used to resolve the validation policy.
- Sensors with `depends_on` referencing a sensor that does not validate
  this use case are skipped silently (only sensors with matching
  `use_case_id` participate).
- Policies are loaded from `.harness/policy/global.yaml` and
  `.harness/policy/local/<usecase-id>.yaml` if present; missing files
  yield empty policies (no obligatory angles), which means the verdict
  defaults to `pass` when every sensor passes.
- Cancellation: SIGINT/SIGTERM cancel the in-flight scheduler;
  unfinished sensors get synthetic `inconclusive`/`stopped` aggregates.

## Examples

Pass:

```
$ /validate-use-case order-create
{"sensor_id":"order-create-build",…,"verdict":"pass"}
{"sensor_id":"order-create-e2e",…,"verdict":"pass"}
{"use_case_id":"order-create","verdict":"pass","confidence":1.0,…}
$ echo $?
0
```

Fail with dependency skipping:

```
$ /validate-use-case order-create
{"sensor_id":"order-create-build","verdict":"fail","heal_hint":{…}}
{"sensor_id":"order-create-e2e","verdict":"inconclusive","heal_hint":{"summary":"skipped: depends_on order-create-build failed"}}
{"use_case_id":"order-create","verdict":"fail",…}
$ echo $?
1
```
```

### Step 2: Commit

```bash
git add skills/validate-use-case/skill.md
git commit -m "docs(skills): /validate-use-case skill markdown"
```

---

## Task 3: `skills/validate-use-case/scripts/main.go`

**Files:**
- Create: `skills/validate-use-case/scripts/main.go`
- Test: `skills/validate-use-case/scripts/main_test.go`
- Reference: design doc §8.5

The script:
1. Validate argv (need use-case id; exit 3 `bad-argv`).
2. `FindRepoRoot` + `BootLifecycle`.
3. Look up use case in `b.UseCases[id]`; if not found → exit 3 `use-case-not-found`.
4. Filter `b.Sensors.All()` to those with matching `UseCaseID`.
5. Pick the archetype = `uc.ArchetypeScope[0]`; exit 3 `no-archetype` if empty.
6. Load policies (best-effort): try `<harness>/policy/global.yaml` and `<harness>/policy/local/<usecase-id>.yaml`. Missing files → nil sources; both nil → empty `*EffectivePolicy`.
7. Allocate a `ucRunID` ULID (use `github.com/oklog/ulid/v2`).
8. Schedule via `skillruntime.RunAll(ctx, sensors, runnerOverLifecycle, runtime.NumCPU())`. Runner = closure invoking `b.Lifecycle.RunSensor(ctx, s.ID, nil)`.
9. Emit each aggregate to stdout (JSONL).
10. `aggregator.UseCase(uc, archetype, aggs, sensors, pol)` → `UseCaseVerdict`.
11. Build the persisted doc with `sensor_runs` traceability (a slim envelope around `UseCaseVerdict`).
12. Write `verdict.json` under `<harness>/runtime/use-cases/<usecase-id>/<ucRunID>/`.
13. Emit the verdict on stdout as the final line.
14. Return `ExitCodeForVerdict(verdict.Verdict)`.

### Step 1: Write the test

Create `skills/validate-use-case/scripts/main_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var fakeSensorBin string

func TestMain(m *testing.M) {
	bin, err := buildFakeSensor()
	if err != nil {
		panic("build fakesensor: " + err.Error())
	}
	fakeSensorBin = bin
	defer os.Remove(bin)
	os.Exit(m.Run())
}

func buildFakeSensor() (string, error) {
	dir, err := os.MkdirTemp("", "fakesensor-validate-")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "fakesensor")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "../../../internal/testutil/fakesensor/main.go")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
}

const useCaseYAML = `schema_version: 2.0.0
id: test-uc
title: Test use case
archetype_scope: [http-api]
entry_points:
  - id: test-ep
    archetype: http-api
    spec:
      method: GET
      path: /test
given:
  - "a request"
when:
  - "the test runs"
then:
  - "it passes"
`

func setupHarness(t *testing.T, sensors map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for _, sub := range []string{"sensors", "fixtures", "use-cases", "runtime"} {
		if err := os.MkdirAll(filepath.Join(root, ".harness", sub), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".harness", "use-cases", "test-uc.yaml"), []byte(useCaseYAML), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for name, body := range sensors {
		if err := os.WriteFile(filepath.Join(root, ".harness", "sensors", name+".yaml"), []byte(body), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return root
}

func TestRun_BadArgv(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate-use-case"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
}

func TestRun_UseCaseNotFound(t *testing.T) {
	root := setupHarness(t, nil)
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate-use-case", "no-such-uc"}, nil, &stdout, &stderr, root)
	if code != 3 {
		t.Errorf("exit = %d, want 3; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "use-case-not-found") {
		t.Errorf("stderr missing use-case-not-found: %q", stderr.String())
	}
}

func TestRun_TwoPassingSensors(t *testing.T) {
	sBuild := `schema_version: 1.0.0
id: test-build
use_case_id: test-uc
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses: [fake]
steps:
  - id: only
    run: "` + fakeSensorBin + ` signal pass"
`
	sUnit := `schema_version: 1.0.0
id: test-unit
use_case_id: test-uc
angle: unit-test
kind: assertion
nature: computational
output_type: single-shot
uses: [fake]
depends_on: [test-build]
steps:
  - id: only
    run: "` + fakeSensorBin + ` signal pass"
`
	root := setupHarness(t, map[string]string{"test-build": sBuild, "test-unit": sUnit})

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate-use-case", "test-uc"}, nil, &stdout, &stderr, root)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) < 3 { // 2 aggregates + 1 verdict
		t.Fatalf("got %d lines, want ≥3: %q", len(lines), stdout.String())
	}
	var verdict map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &verdict); err != nil {
		t.Fatalf("verdict line not JSON: %v (%q)", err, lines[len(lines)-1])
	}
	if verdict["verdict"] != "pass" {
		t.Errorf("verdict = %v, want pass", verdict["verdict"])
	}
	// verdict.json should exist
	matches, _ := filepath.Glob(filepath.Join(root, ".harness", "runtime", "use-cases", "test-uc", "*", "verdict.json"))
	if len(matches) != 1 {
		t.Errorf("verdict.json count = %d, want 1", len(matches))
	}
}

func TestRun_DependencyFailedSkipsDependent(t *testing.T) {
	sBuild := `schema_version: 1.0.0
id: test-build
use_case_id: test-uc
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses: [fake]
steps:
  - id: only
    run: "` + fakeSensorBin + ` signal fail --summary 'build failure'"
`
	sUnit := `schema_version: 1.0.0
id: test-unit
use_case_id: test-uc
angle: unit-test
kind: assertion
nature: computational
output_type: single-shot
uses: [fake]
depends_on: [test-build]
steps:
  - id: only
    run: "` + fakeSensorBin + ` signal pass"
`
	root := setupHarness(t, map[string]string{"test-build": sBuild, "test-unit": sUnit})

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate-use-case", "test-uc"}, nil, &stdout, &stderr, root)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (fail); stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"skipped: depends_on test-build failed"`) {
		t.Errorf("expected dependency-skip heal_hint in stdout: %q", stdout.String())
	}
}
```

### Step 2: Run tests to verify they fail

```
go test ./skills/validate-use-case/scripts/...
```
Expected: FAIL with "undefined: run".

### Step 3: Implement main.go

Create `skills/validate-use-case/scripts/main.go`:

```go
// Command validate-use-case backs the /validate-use-case skill.
// See skills/validate-use-case/skill.md.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/policy"
	"github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/lib/skillio"
	"github.com/iurykrieger/lastro/lib/skillruntime"
	"github.com/oklog/ulid/v2"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		skillio.EmitError(os.Stderr, "cwd-failed", err.Error(), nil)
		os.Exit(skillio.ExitScriptError)
	}
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr, cwd))
}

// persistedVerdict adds run-id traceability around the aggregator's
// UseCaseVerdict. The skill writes this struct (not the raw verdict)
// to .harness/runtime/use-cases/<id>/<run-id>/verdict.json.
type persistedVerdict struct {
	UseCaseVerdict aggregator.UseCaseVerdict `json:"use_case_verdict"`
	UseCaseRunID   string                    `json:"use_case_run_id"`
	SensorRuns     []sensorRun               `json:"sensor_runs"`
}

type sensorRun struct {
	SensorID string `json:"sensor_id"`
	Verdict  string `json:"verdict"`
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected use-case-id as first argument", nil)
		return skillio.ExitScriptError
	}
	useCaseID := args[1]

	repoRoot, err := skillio.FindRepoRoot(cwd)
	if err != nil {
		skillio.EmitError(stderr, "repo-root-not-found", err.Error(), nil)
		return skillio.ExitScriptError
	}

	b, err := skillruntime.BootLifecycle(repoRoot)
	if err != nil {
		skillio.EmitError(stderr, "boot-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	defer func() { _ = b.Cleanup() }()

	uc, ok := b.UseCases[useCaseID]
	if !ok {
		skillio.EmitError(stderr, "use-case-not-found", fmt.Sprintf("no use case %q in .harness/use-cases/", useCaseID), map[string]any{"use_case_id": useCaseID})
		return skillio.ExitScriptError
	}
	if len(uc.ArchetypeScope) == 0 {
		skillio.EmitError(stderr, "no-archetype", "use case has empty archetype_scope", map[string]any{"use_case_id": useCaseID})
		return skillio.ExitScriptError
	}
	archetype := uc.ArchetypeScope[0]

	// Collect sensors that validate this use case.
	var sensors []sensor.Sensor
	for _, s := range b.Sensors.All() {
		if s.UseCaseID == useCaseID {
			sensors = append(sensors, s)
		}
	}

	// Load policies (best effort).
	pol := loadPolicies(filepath.Join(skillio.HarnessDir(repoRoot), "policy"), useCaseID)

	// Run all sensors via the DAG-aware scheduler.
	ctx := context.Background()
	runner := func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		return b.Lifecycle.RunSensor(ctx, s.ID, nil)
	}
	aggs, err := skillruntime.RunAll(ctx, sensors, runner, runtime.NumCPU())
	if err != nil {
		skillio.EmitError(stderr, "scheduler-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	// Emit each aggregate as JSONL.
	for _, a := range aggs {
		if err := skillio.EmitJSON(stdout, a); err != nil {
			skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
			return skillio.ExitScriptError
		}
	}

	verdict, err := aggregator.UseCase(uc, archetype, aggs, sensors, pol)
	if err != nil {
		skillio.EmitError(stderr, "aggregate-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	// Persist verdict.json with traceability envelope.
	ucRunID := newULID()
	persisted := persistedVerdict{
		UseCaseVerdict: verdict,
		UseCaseRunID:   ucRunID,
		SensorRuns:     make([]sensorRun, 0, len(aggs)),
	}
	for _, a := range aggs {
		persisted.SensorRuns = append(persisted.SensorRuns, sensorRun{SensorID: a.SensorID, Verdict: string(a.Verdict)})
	}
	if err := writeVerdict(filepath.Join(b.RuntimeRoot, "use-cases", useCaseID, ucRunID, "verdict.json"), persisted); err != nil {
		skillio.EmitError(stderr, "persist-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	if err := skillio.EmitJSON(stdout, persisted); err != nil {
		skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	return skillio.ExitCodeForVerdict(verdict.Verdict)
}

// loadPolicies reads optional global + local policy YAMLs. Returns an
// empty *EffectivePolicy when both are absent; the aggregator treats
// it as "no obligatory angles".
func loadPolicies(policyDir, useCaseID string) *policy.EffectivePolicy {
	global := loadOne(filepath.Join(policyDir, "global.yaml"))
	local := loadOne(filepath.Join(policyDir, "local", useCaseID+".yaml"))
	return policy.Resolve(global, local)
}

func loadOne(path string) *policy.ValidationPolicy {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	p, err := policy.Load(f)
	if err != nil {
		return nil // malformed policy is silently ignored to keep the skill best-effort
	}
	return p
}

func writeVerdict(path string, v persistedVerdict) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func newULID() string {
	ms := ulid.Timestamp(time.Now())
	id, _ := ulid.New(ms, rand.Reader)
	return id.String()
}
```

### Step 4: Run tests to verify they pass

```
go test ./skills/validate-use-case/scripts/... -count=1
```
Expected: PASS, four tests (bad-argv, use-case-not-found, two-passing, dependency-failed).

### Step 5: Race test

```
go test -race ./skills/validate-use-case/scripts/... -count=1
```
Expected: PASS.

### Step 6: Commit

```bash
git add skills/validate-use-case/scripts/main.go skills/validate-use-case/scripts/main_test.go
git commit -m "feat(skills/validate-use-case): orchestrate sensor DAG + aggregate verdict"
```

---

## Task 4: Verify, push, update PR

### Step 1: Run the full test suite

```
go test -race ./...
```
Expected: PASS across all packages, no race warnings.

### Step 2: Push the branch

```
git push origin claude/flamboyant-carson-1dfa37
```

### Step 3: Update PR #17 description to encompass both sub-PRs

Use the gh CLI to amend the body with a new section noting sub-PR 2 is now included. Keep sub-PR 1 sections intact; add a `## Sub-PR 2 — /validate-use-case` block describing the scheduler + skill and the acceptance criteria covered.

---

## Spec-coverage check

| Spec requirement | Where covered |
|---|---|
| §8.5 `/validate-use-case` flow (load, schedule, aggregate, persist, emit) | Task 3 |
| §9 DAG-aware scheduler (parallelism, skipped-dependent synthesis, cancellation) | Task 1 |
| §11.2 mixed-dependency exec + dependency-skip + determinism | Tasks 1 + 3 tests |
| §11.2 verdict.json with sensor_runs traceability | Task 3 (`persistedVerdict`) |
| §11.2 `go test -race` clean | Task 4 |
| §12 unit + race | Task 1 race test, Task 3 race test, Task 4 full suite |
