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
	// Use ResolveExecutionOrder for cycle/dangling-edge validation only.
	if _, err := sensor.ResolveExecutionOrder(sensors); err != nil {
		return nil, err
	}

	byID := make(map[string]sensor.Sensor, len(sensors))
	for _, s := range sensors {
		byID[s.ID] = s
	}

	// Build adjacency (out-edges) and in-degree map.
	out := make(map[string][]string, len(sensors)) // s.ID → ids that depend on it
	inDeg := make(map[string]int, len(sensors))
	for _, s := range sensors {
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
		failed    = make(map[string]string) // s.ID → immediate failing ancestor id
		runErr    error
		semaphore = make(chan struct{}, parallelism)
		wg        sync.WaitGroup
	)

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var runOne func(id string)
	runOne = func(id string) {
		defer wg.Done()
		defer func() { <-semaphore }()

		s := byID[id]
		var agg aggregate.AggregateSignal

		mu.Lock()
		ancestor, skipped := failed[id]
		mu.Unlock()

		switch {
		case skipped:
			agg = synthesizeSkipped(s, ancestor)
		case cctx.Err() != nil:
			agg = synthesizeCancelled(s, cctx.Err())
		default:
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

		mu.Lock()
		done[id] = agg
		if agg.Verdict == enums.VerdictFail {
			markSkipped(id, id, out, failed)
		}
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

	// Seed: every sensor with zero in-degree is ready immediately.
	var ready []string
	for id, d := range inDeg {
		if d == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	for _, id := range ready {
		wg.Add(1)
		semaphore <- struct{}{}
		go runOne(id)
	}
	wg.Wait()

	if runErr != nil {
		return nil, runErr
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	results := make([]aggregate.AggregateSignal, 0, len(done))
	for _, agg := range done {
		results = append(results, agg)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].SensorID < results[j].SensorID })
	return results, nil
}

// markSkipped marks every transitive dependent of failedID as skipped,
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
	now := time.Now().UTC()
	return aggregate.AggregateSignal{
		SchemaVersion:     "1.0.0",
		Type:              aggregate.TypeAggregate,
		SensorID:          dep.ID,
		UseCaseID:         dep.UseCaseID,
		Angle:             dep.Angle,
		StartedAt:         now,
		EndedAt:           now,
		TerminationReason: enums.TerminationStopped,
		Verdict:           enums.VerdictInconclusive,
		Confidence:        0.0,
		HealHint: &aggregate.HealHint{
			Summary:   fmt.Sprintf("skipped: depends_on %s failed", failedID),
			Rationale: fmt.Sprintf("sensor %s did not execute because %s's AggregateSignal verdict=fail", dep.ID, failedID),
		},
	}
}

func synthesizeCancelled(s sensor.Sensor, err error) aggregate.AggregateSignal {
	now := time.Now().UTC()
	return aggregate.AggregateSignal{
		SchemaVersion:     "1.0.0",
		Type:              aggregate.TypeAggregate,
		SensorID:          s.ID,
		UseCaseID:         s.UseCaseID,
		Angle:             s.Angle,
		StartedAt:         now,
		EndedAt:           now,
		TerminationReason: enums.TerminationStopped,
		Verdict:           enums.VerdictInconclusive,
		Confidence:        0.0,
		HealHint: &aggregate.HealHint{
			Summary:   "skipped: scheduler cancelled before this sensor ran",
			Rationale: err.Error(),
		},
	}
}
