package skillruntime

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
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
	now := time.Now().UTC()
	return aggregate.AggregateSignal{
		SchemaVersion:     "1.0.0",
		Type:              aggregate.TypeAggregate,
		SensorID:          s.ID,
		UseCaseID:         s.UseCaseID,
		Angle:             s.Angle,
		StartedAt:         now,
		EndedAt:           now,
		TerminationReason: enums.TerminationCompleted,
		Verdict:           enums.VerdictPass,
		Confidence:        1.0,
	}
}

func failingAgg(s sensor.Sensor) aggregate.AggregateSignal {
	now := time.Now().UTC()
	return aggregate.AggregateSignal{
		SchemaVersion:     "1.0.0",
		Type:              aggregate.TypeAggregate,
		SensorID:          s.ID,
		UseCaseID:         s.UseCaseID,
		Angle:             s.Angle,
		StartedAt:         now,
		EndedAt:           now,
		TerminationReason: enums.TerminationCompleted,
		Verdict:           enums.VerdictFail,
		Confidence:        1.0,
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
	ids := []string{got[0].SensorID, got[1].SensorID, got[2].SensorID}
	if !sort.StringsAreSorted(ids) {
		t.Errorf("results not sorted by id: %v", ids)
	}
}

func TestRunAll_LinearChain_FirstFails_RestSkipped(t *testing.T) {
	a := mkSensor("a")
	b := mkSensor("b", "a")
	c := mkSensor("c", "b")
	var calledMu sync.Mutex
	called := map[string]bool{}
	got, err := RunAll(context.Background(), []sensor.Sensor{a, b, c}, func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		calledMu.Lock()
		called[s.ID] = true
		calledMu.Unlock()
		if s.ID == "a" {
			return failingAgg(s), nil
		}
		return passingAgg(s), nil
	}, 4)
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	calledMu.Lock()
	if !called["a"] || called["b"] || called["c"] {
		t.Errorf("expected only A to run; called=%v", called)
	}
	calledMu.Unlock()
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
	var calledMu sync.Mutex
	called := map[string]bool{}
	got, err := RunAll(context.Background(), []sensor.Sensor{a, b, c, d}, func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		calledMu.Lock()
		called[s.ID] = true
		calledMu.Unlock()
		if s.ID == "b" {
			return failingAgg(s), nil
		}
		return passingAgg(s), nil
	}, 4)
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	calledMu.Lock()
	if !called["a"] || !called["b"] || !called["c"] || called["d"] {
		t.Errorf("expected a/b/c to run, d skipped; called=%v", called)
	}
	calledMu.Unlock()
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

func TestRunAll_ParallelismHigherThanOne(t *testing.T) {
	a := mkSensor("a")
	b := mkSensor("b")
	c := mkSensor("c")
	var concurrent, maxConcurrent int32
	runner := func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		cur := atomic.AddInt32(&concurrent, 1)
		defer atomic.AddInt32(&concurrent, -1)
		for {
			prev := atomic.LoadInt32(&maxConcurrent)
			if cur <= prev || atomic.CompareAndSwapInt32(&maxConcurrent, prev, cur) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		return passingAgg(s), nil
	}
	_, err := RunAll(context.Background(), []sensor.Sensor{a, b, c}, runner, 3)
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if atomic.LoadInt32(&maxConcurrent) < 2 {
		t.Errorf("expected peak concurrency >= 2 with parallelism=3, got %d", maxConcurrent)
	}
}
