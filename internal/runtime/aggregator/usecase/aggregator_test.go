package aggregator

import (
	"strings"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/policy"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)

func TestAngleHint_HoldsAngleVerdictHint(t *testing.T) {
	h := AngleHint{
		Angle:   enums.AngleBuild,
		Verdict: enums.VerdictFail,
		Hint:    aggregate.HealHint{},
	}
	if h.Angle != enums.AngleBuild {
		t.Errorf("Angle = %q, want build", h.Angle)
	}
	if h.Verdict != enums.VerdictFail {
		t.Errorf("Verdict = %q, want fail", h.Verdict)
	}
}

func TestUseCaseVerdict_ZeroValueShape(t *testing.T) {
	var v UseCaseVerdict
	if v.UseCaseID != "" || v.Verdict != "" || v.Confidence != 0.0 ||
		v.ObligatorySatisfied != false ||
		v.EvaluatedAngles != nil || v.FailingAngles != nil ||
		v.WarningAngles != nil || v.HealHints != nil {
		t.Errorf("zero UseCaseVerdict has non-zero fields: %+v", v)
	}
}

func makeUseCase(id string, archetypes ...enums.Archetype) *usecase.UseCase {
	return &usecase.UseCase{ID: id, ArchetypeScope: archetypes}
}

func makeSignal(useCaseID, sensorID string, angle enums.ValidationAngle, verdict enums.Verdict, confidence float64) aggregate.AggregateSignal {
	sig := aggregate.AggregateSignal{
		SchemaVersion:     "1.0.0",
		Type:              aggregate.TypeAggregate,
		SensorID:          sensorID,
		UseCaseID:         useCaseID,
		Angle:             angle,
		StartedAt:         time.Unix(1, 0),
		EndedAt:           time.Unix(2, 0),
		TerminationReason: enums.TerminationCompleted,
		Verdict:           verdict,
		Confidence:        confidence,
	}
	if verdict == enums.VerdictFail || verdict == enums.VerdictWarn {
		sig.HealHint = &aggregate.HealHint{Summary: "synthetic"}
	}
	return sig
}

func makeSensor(id string, useCaseID string, angle enums.ValidationAngle, nature enums.SensorNature) sensor.Sensor {
	return sensor.Sensor{
		SchemaVersion: "1.0.0",
		ID:            id, UseCaseID: useCaseID, Angle: angle,
		Kind: enums.KindAssertion, Nature: nature, OutputType: enums.OutputSingleShot,
	}
}

func makeEffectivePolicy(arch enums.Archetype, obligatory []enums.ValidationAngle, optional []enums.ValidationAngle, floor float64) *policy.EffectivePolicy {
	statuses := map[enums.ValidationAngle]policy.AngleStatus{}
	for _, a := range obligatory {
		statuses[a] = policy.StatusObligatory
	}
	for _, a := range optional {
		statuses[a] = policy.StatusOptional
	}
	return &policy.EffectivePolicy{
		SchemaVersion:    policy.SupportedSchemaVersion,
		ResolvedFrom:     []string{"global"},
		PerArchetype:     map[enums.Archetype]map[enums.ValidationAngle]policy.AngleStatus{arch: statuses},
		InferentialFloor: floor,
	}
}

func TestUseCase_ArchetypeNotInScopeReturnsError(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeCLI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI, []enums.ValidationAngle{enums.AngleBuild}, nil, 0.7)
	_, err := UseCase(uc, enums.ArchetypeHTTPAPI, nil, nil, pol)
	if err == nil || !strings.Contains(err.Error(), "archetype-not-in-scope") {
		t.Errorf("err = %v, want archetype-not-in-scope", err)
	}
}

func TestUseCase_ForeignSignalReturnsError(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI, []enums.ValidationAngle{enums.AngleBuild}, nil, 0.7)
	sig := makeSignal("uc-other", "s1", enums.AngleBuild, enums.VerdictPass, 1.0)
	sensors := []sensor.Sensor{makeSensor("s1", "uc-other", enums.AngleBuild, enums.NatureComputational)}
	_, err := UseCase(uc, enums.ArchetypeHTTPAPI, []aggregate.AggregateSignal{sig}, sensors, pol)
	if err == nil || !strings.Contains(err.Error(), "signal-foreign-use-case") {
		t.Errorf("err = %v, want signal-foreign-use-case", err)
	}
}

func TestUseCase_DuplicateAngleReturnsError(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI, []enums.ValidationAngle{enums.AngleBuild}, nil, 0.7)
	sig1 := makeSignal("uc-login", "s1", enums.AngleBuild, enums.VerdictPass, 1.0)
	sig2 := makeSignal("uc-login", "s2", enums.AngleBuild, enums.VerdictPass, 1.0)
	sensors := []sensor.Sensor{
		makeSensor("s1", "uc-login", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s2", "uc-login", enums.AngleBuild, enums.NatureComputational),
	}
	_, err := UseCase(uc, enums.ArchetypeHTTPAPI, []aggregate.AggregateSignal{sig1, sig2}, sensors, pol)
	if err == nil || !strings.Contains(err.Error(), "duplicate-angle-signal") {
		t.Errorf("err = %v, want duplicate-angle-signal", err)
	}
}

func TestUseCase_MissingObligatorySignalReturnsError(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI, []enums.ValidationAngle{enums.AngleBuild, enums.AngleUnitTest}, nil, 0.7)
	// Only one signal — unit-test obligatory is missing.
	sig := makeSignal("uc-login", "s1", enums.AngleBuild, enums.VerdictPass, 1.0)
	sensors := []sensor.Sensor{makeSensor("s1", "uc-login", enums.AngleBuild, enums.NatureComputational)}
	_, err := UseCase(uc, enums.ArchetypeHTTPAPI, []aggregate.AggregateSignal{sig}, sensors, pol)
	if err == nil || !strings.Contains(err.Error(), "missing-obligatory-signal") {
		t.Errorf("err = %v, want missing-obligatory-signal", err)
	}
}
