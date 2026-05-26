package healloop

import (
	"context"
	"errors"
	"testing"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/policy"
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
	// Sensors: one assertion (build angle), one observational (logs angle).
	assertionSensor := sensor.Sensor{
		ID:     "s-assert",
		Kind:   enums.KindAssertion,
		Angle:  enums.AngleBuild,
		Nature: enums.NatureComputational,
	}
	observationalSensor := sensor.Sensor{
		ID:     "s-obs",
		Kind:   enums.KindObservational,
		Angle:  enums.AngleLogs,
		Nature: enums.NatureComputational,
	}

	runner := &stubSensorRunner{signals: map[string]aggregate.AggregateSignal{
		"s-assert": {SensorID: "s-assert", UseCaseID: "uc-1", Angle: enums.AngleBuild, Verdict: enums.VerdictPass, Confidence: 1.0},
	}}
	sensors := &stubSensorLookup{sensors: []sensor.Sensor{assertionSensor, observationalSensor}}
	uc := &upkg.UseCase{
		ID:             "uc-1",
		SchemaVersion:  "1.0.0",
		Title:          "test",
		ArchetypeScope: []enums.Archetype{enums.Archetype("http-api")},
	}
	ucs := &stubUseCaseLookup{uc: uc}
	original := map[string]aggregate.AggregateSignal{
		"s-obs": {SensorID: "s-obs", UseCaseID: "uc-1", Angle: enums.AngleLogs, Verdict: enums.VerdictPass, Confidence: 1.0},
	}
	pol := &policy.EffectivePolicy{
		SchemaVersion: "1.0.0",
		PerArchetype: map[enums.Archetype]map[enums.ValidationAngle]policy.AngleStatus{
			enums.Archetype("http-api"): {
				enums.AngleBuild: policy.StatusObligatory,
				enums.AngleLogs:  policy.StatusObligatory,
			},
		},
		InferentialFloor:  0.7,
		MaxHealIterations: 3,
	}

	rev := newLifecycleRevalidator(runner, sensors, ucs, pol, original, enums.Archetype("http-api"))
	verdict, err := rev.Revalidate(context.Background(), "uc-1")
	if err != nil {
		t.Fatalf("Revalidate: %v", err)
	}

	// Only the assertion sensor should have been run; the observational one is skipped.
	if len(runner.called) != 1 || runner.called[0] != "s-assert" {
		t.Errorf("RunSensor called for = %v, want [s-assert]", runner.called)
	}
	// The aggregated verdict should pass (both obligatory angles covered with pass signals).
	if verdict.Verdict != enums.VerdictPass {
		t.Errorf("Verdict = %q, want pass", verdict.Verdict)
	}
}

func TestLifecycleRevalidator_ReturnsErrUseCaseNotFound_OnUnknownID(t *testing.T) {
	runner := &stubSensorRunner{}
	rev := newLifecycleRevalidator(runner, &stubSensorLookup{}, &stubUseCaseLookup{uc: nil}, nil, nil, enums.Archetype("http-api"))
	_, err := rev.Revalidate(context.Background(), "missing")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrUseCaseNotFound) {
		t.Errorf("err = %v, want errors.Is(_, ErrUseCaseNotFound)", err)
	}
	if len(runner.called) != 0 {
		t.Errorf("RunSensor called %v times, want 0", len(runner.called))
	}
}
