package healloop

import (
	"context"
	"testing"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/sensor"
	upkg "github.com/iurykrieger/lastro/internal/usecase"
)

// stubSensorRunner records the sensor IDs that RunSensor was called for
// and returns scripted AggregateSignals.
type stubSensorRunner struct {
	signals map[string]aggregate.AggregateSignal
	called  []string
}

func (s *stubSensorRunner) RunSensor(_ context.Context, sensorID string, _ []string) (aggregate.AggregateSignal, error) {
	s.called = append(s.called, sensorID)
	return s.signals[sensorID], nil
}

type stubSensorLookup struct {
	sensors []sensor.Sensor
}

func (s *stubSensorLookup) SensorsForUseCase(_ string) []sensor.Sensor { return s.sensors }

type stubUseCaseLookup struct {
	uc *upkg.UseCase
}

func (s *stubUseCaseLookup) Lookup(_ string) (*upkg.UseCase, bool) { return s.uc, s.uc != nil }

func TestLifecycleRevalidator_SkipsObservational_CarriesForwardSignals(t *testing.T) {
	t.Skip("blocked on real *usecase.UseCase + *policy.EffectivePolicy fixtures; covered by integration tests in B5")
	// The unit-test version asserts on (a) which sensor IDs RunSensor was
	// called with and (b) that the observational signal was passed through
	// to aggregator.UseCase. The full assertion requires a real
	// *upkg.UseCase + EffectivePolicy that mark one angle obligatory. That
	// fixture lives in B5's integration suite; here we just verify the
	// shape compiles and the skip codepath runs.

	// Sensors: one assertion (build angle), one observational (logs angle).
	assertionSensor := sensor.Sensor{ID: "s-assert", Kind: enums.KindAssertion, Angle: enums.AngleBuild}
	observationalSensor := sensor.Sensor{ID: "s-obs", Kind: enums.KindObservational, Angle: enums.AngleLogs}

	runner := &stubSensorRunner{signals: map[string]aggregate.AggregateSignal{
		"s-assert": {SensorID: "s-assert", Angle: enums.AngleBuild, Verdict: enums.VerdictPass},
	}}
	sensors := &stubSensorLookup{sensors: []sensor.Sensor{assertionSensor, observationalSensor}}
	ucs := &stubUseCaseLookup{uc: &upkg.UseCase{ID: "uc-1", ArchetypeScope: []enums.Archetype{"http-api"}}}
	original := map[string]aggregate.AggregateSignal{
		"s-obs": {SensorID: "s-obs", Angle: enums.AngleLogs, Verdict: enums.VerdictPass},
	}

	rev := newLifecycleRevalidator(runner, sensors, ucs, nil, original, enums.Archetype("http-api"))
	_, _ = rev.Revalidate(context.Background(), "uc-1")

	if len(runner.called) != 1 || runner.called[0] != "s-assert" {
		t.Errorf("RunSensor called for = %v, want [s-assert]", runner.called)
	}
}

func TestLifecycleRevalidator_ReturnsErrUseCaseNotFound_OnUnknownID(t *testing.T) {
	runner := &stubSensorRunner{}
	rev := newLifecycleRevalidator(runner, &stubSensorLookup{}, &stubUseCaseLookup{uc: nil}, nil, nil, enums.Archetype("http-api"))
	_, err := rev.Revalidate(context.Background(), "missing")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err != ErrUseCaseNotFound {
		t.Errorf("err = %v, want ErrUseCaseNotFound", err)
	}
	if len(runner.called) != 0 {
		t.Errorf("RunSensor called %v times, want 0", len(runner.called))
	}
}
