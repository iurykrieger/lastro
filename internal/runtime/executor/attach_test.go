package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/runtime/servicemgr"
	"github.com/iurykrieger/lastro/internal/sensor"
)

func TestExecAttachStep_MatchesServiceLinesAndCompletes(t *testing.T) {
	dir := t.TempDir()
	svcSignals := filepath.Join(dir, "svc-signals.jsonl")
	// run-dev emits each server log line as a signal carrying matched_line.
	if err := os.WriteFile(svcSignals, []byte(
		`{"evidence":{"observation_key":"log-line","matched_line":"GET /health 200"}}`+"\n"+
			`{"evidence":{"observation_key":"log-line","matched_line":"compiled successfully"}}`+"\n",
	), 0o600); err != nil {
		t.Fatalf("seed svc signals: %v", err)
	}

	consumer := sensor.Sensor{
		ID:        "logs-sensor",
		UseCaseID: "uc-x",
		Angle:     enums.ValidationAngle("logs"),
		Kind:      enums.KindObservational,
		SignalMatches: []sensor.SignalMatch{
			{Key: "compiled", Pattern: "compiled successfully", Verdict: enums.VerdictPass, Expected: true},
		},
	}

	att := servicemgr.Attachment{ServiceID: "run-dev", SignalsPath: svcSignals}
	got := execAttachStep(context.Background(), attachArgs{
		Consumer:      consumer,
		Attachment:    att,
		ExpectedKeys:  []string{"compiled"},
		ObserveWindow: 2 * time.Second,
		Now:           func() time.Time { return time.Unix(0, 0).UTC() },
	})

	if got.TermReason != enums.TerminationCompleted {
		t.Fatalf("term = %q, want completed", got.TermReason)
	}
	if len(got.ObservationKeys) != 1 || got.ObservationKeys[0] != "compiled" {
		t.Fatalf("observation keys = %v, want exactly [compiled]", got.ObservationKeys)
	}
}

func TestExecAttachStep_WindowElapsesWithoutAllKeys(t *testing.T) {
	dir := t.TempDir()
	svc := filepath.Join(dir, "svc.jsonl")
	if err := os.WriteFile(svc, []byte(`{"evidence":{"observation_key":"log-line","matched_line":"only this line"}}`+"\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	consumer := sensor.Sensor{
		ID: "logs", Angle: enums.ValidationAngle("logs"), Kind: enums.KindObservational,
		SignalMatches: []sensor.SignalMatch{{Key: "never", Pattern: "this never appears", Verdict: enums.VerdictPass, Expected: true}},
	}
	got := execAttachStep(context.Background(), attachArgs{
		Consumer: consumer, Attachment: servicemgr.Attachment{SignalsPath: svc},
		ExpectedKeys: []string{"never"}, ObserveWindow: 150 * time.Millisecond,
		Now: func() time.Time { return time.Unix(0, 0).UTC() },
	})
	if got.TermReason != enums.TerminationCompleted {
		t.Fatalf("term = %q, want completed (verdict comes from completeness)", got.TermReason)
	}
	if len(got.ObservationKeys) != 0 {
		t.Fatalf("keys = %v, want none", got.ObservationKeys)
	}
}

func TestExecTopStep_DispatchesToAttachForObservableTarget(t *testing.T) {
	dir := t.TempDir()
	svc := filepath.Join(dir, "svc.jsonl")
	if err := os.WriteFile(svc, []byte(`{"evidence":{"observation_key":"log-line","matched_line":"ready - started server"}}`+"\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	e := New(Options{
		Now: func() time.Time { return time.Unix(0, 0).UTC() },
		SensorLookup: func(id string) (sensor.Sensor, bool) {
			if id == "run-dev" {
				return sensor.Sensor{ID: "run-dev", Scope: enums.ScopeCore, Kind: enums.KindObservational}, true
			}
			return sensor.Sensor{}, false
		},
		ServiceAttach: func(_ context.Context, serviceID string) (servicemgr.Attachment, bool) {
			return servicemgr.Attachment{ServiceID: serviceID, SignalsPath: svc}, true
		},
	})

	consumer := sensor.Sensor{
		ID: "logs", UseCaseID: "uc", Angle: enums.ValidationAngle("logs"), Kind: enums.KindObservational,
		SignalMatches: []sensor.SignalMatch{{Key: "ready", Pattern: "ready - started", Verdict: enums.VerdictPass, Expected: true}},
	}
	idx := 0
	res := e.execTopStep(context.Background(), topStepArgs{
		Sensor:    consumer,
		Step:      sensor.Step{ID: "watch", Uses: "run-dev"},
		GlobalIdx: &idx,
		RunDir:    dir,
	})
	if res.TermReason != enums.TerminationCompleted {
		t.Fatalf("term = %q, want completed", res.TermReason)
	}
	if len(res.ObservationKeys) != 1 || res.ObservationKeys[0] != "ready" {
		t.Fatalf("keys = %v, want [ready]", res.ObservationKeys)
	}
}
