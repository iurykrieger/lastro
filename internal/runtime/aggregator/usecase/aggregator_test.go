package aggregator

import (
	"reflect"
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

func TestUseCase_AllObligatoryPass(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild, enums.AngleUnitTest, enums.AngleE2ETest},
		nil, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-build", enums.AngleBuild, enums.VerdictPass, 1.0),
		makeSignal("uc-login", "s-unit", enums.AngleUnitTest, enums.VerdictPass, 1.0),
		makeSignal("uc-login", "s-e2e", enums.AngleE2ETest, enums.VerdictPass, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-build", "uc-login", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s-unit", "uc-login", enums.AngleUnitTest, enums.NatureComputational),
		makeSensor("s-e2e", "uc-login", enums.AngleE2ETest, enums.NatureComputational),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Verdict != enums.VerdictPass {
		t.Errorf("Verdict = %q, want pass", v.Verdict)
	}
	if !v.ObligatorySatisfied {
		t.Error("ObligatorySatisfied = false, want true")
	}
	if len(v.FailingAngles) != 0 || len(v.WarningAngles) != 0 || len(v.HealHints) != 0 {
		t.Errorf("unexpected non-pass surface: failing=%v warning=%v hints=%d", v.FailingAngles, v.WarningAngles, len(v.HealHints))
	}
}

func TestUseCase_OneObligatoryFail(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild, enums.AngleUnitTest}, nil, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-build", enums.AngleBuild, enums.VerdictPass, 1.0),
		makeSignal("uc-login", "s-unit", enums.AngleUnitTest, enums.VerdictFail, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-build", "uc-login", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s-unit", "uc-login", enums.AngleUnitTest, enums.NatureComputational),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Verdict != enums.VerdictFail {
		t.Errorf("Verdict = %q, want fail", v.Verdict)
	}
	if v.ObligatorySatisfied {
		t.Error("ObligatorySatisfied = true, want false")
	}
	if len(v.FailingAngles) != 1 || v.FailingAngles[0] != enums.AngleUnitTest {
		t.Errorf("FailingAngles = %v, want [unit-test]", v.FailingAngles)
	}
	if len(v.HealHints) != 1 || v.HealHints[0].Angle != enums.AngleUnitTest || v.HealHints[0].Verdict != enums.VerdictFail {
		t.Errorf("HealHints = %+v, want one entry for unit-test:fail", v.HealHints)
	}
}

func TestUseCase_OnlyOptionalFails_VerdictStaysPass(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild},
		[]enums.ValidationAngle{enums.AngleSecurity}, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-build", enums.AngleBuild, enums.VerdictPass, 1.0),
		makeSignal("uc-login", "s-sec", enums.AngleSecurity, enums.VerdictFail, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-build", "uc-login", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s-sec", "uc-login", enums.AngleSecurity, enums.NatureComputational),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Verdict != enums.VerdictPass {
		t.Errorf("Verdict = %q, want pass", v.Verdict)
	}
	if !v.ObligatorySatisfied {
		t.Error("ObligatorySatisfied = false, want true")
	}
	if len(v.FailingAngles) != 1 || v.FailingAngles[0] != enums.AngleSecurity {
		t.Errorf("FailingAngles = %v, want [security]", v.FailingAngles)
	}
}

func TestUseCase_ObligatoryWarn_StaysPass(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild}, nil, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-build", enums.AngleBuild, enums.VerdictWarn, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-build", "uc-login", enums.AngleBuild, enums.NatureComputational),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Verdict != enums.VerdictPass {
		t.Errorf("Verdict = %q, want pass", v.Verdict)
	}
	if !v.ObligatorySatisfied {
		t.Error("ObligatorySatisfied = false, want true (warn is pass-grade)")
	}
	if len(v.WarningAngles) != 1 || v.WarningAngles[0] != enums.AngleBuild {
		t.Errorf("WarningAngles = %v, want [build]", v.WarningAngles)
	}
	if len(v.HealHints) != 1 || v.HealHints[0].Verdict != enums.VerdictWarn {
		t.Errorf("HealHints = %+v, want one entry for warn", v.HealHints)
	}
}

func TestUseCase_ObligatoryWarnAndFail_VerdictIsFail(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild, enums.AngleUnitTest}, nil, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-build", enums.AngleBuild, enums.VerdictWarn, 1.0),
		makeSignal("uc-login", "s-unit", enums.AngleUnitTest, enums.VerdictFail, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-build", "uc-login", enums.AngleBuild, enums.NatureComputational),
		makeSensor("s-unit", "uc-login", enums.AngleUnitTest, enums.NatureComputational),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Verdict != enums.VerdictFail {
		t.Errorf("Verdict = %q, want fail", v.Verdict)
	}
	if v.ObligatorySatisfied {
		t.Error("ObligatorySatisfied = true, want false")
	}
	if len(v.FailingAngles) != 1 || v.FailingAngles[0] != enums.AngleUnitTest {
		t.Errorf("FailingAngles = %v, want [unit-test]", v.FailingAngles)
	}
	if len(v.WarningAngles) != 1 || v.WarningAngles[0] != enums.AngleBuild {
		t.Errorf("WarningAngles = %v, want [build]", v.WarningAngles)
	}
	if len(v.HealHints) != 2 {
		t.Fatalf("HealHints = %d, want 2", len(v.HealHints))
	}
	// Canonical angle order: build (index 1 in AllAngles) before unit-test (index 3).
	if v.HealHints[0].Angle != enums.AngleBuild || v.HealHints[0].Verdict != enums.VerdictWarn {
		t.Errorf("HealHints[0] = %+v, want build:warn", v.HealHints[0])
	}
	if v.HealHints[1].Angle != enums.AngleUnitTest || v.HealHints[1].Verdict != enums.VerdictFail {
		t.Errorf("HealHints[1] = %+v, want unit-test:fail", v.HealHints[1])
	}
}

func TestUseCase_InferentialFailBelowFloor_Demotes(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleE2ETest}, nil, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-e2e", enums.AngleE2ETest, enums.VerdictFail, 0.5),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-e2e", "uc-login", enums.AngleE2ETest, enums.NatureInferential),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Verdict != enums.VerdictInconclusive {
		t.Errorf("Verdict = %q, want inconclusive (demoted)", v.Verdict)
	}
	if len(v.FailingAngles) != 0 {
		t.Errorf("FailingAngles = %v, want [] (demoted)", v.FailingAngles)
	}
	if len(v.HealHints) != 0 {
		t.Errorf("HealHints = %d, want 0 (demoted)", len(v.HealHints))
	}
}

func TestUseCase_InferentialWarnBelowFloor_Demotes(t *testing.T) {
	uc := makeUseCase("uc-login", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleE2ETest}, nil, 0.7)
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-login", "s-e2e", enums.AngleE2ETest, enums.VerdictWarn, 0.6),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-e2e", "uc-login", enums.AngleE2ETest, enums.NatureInferential),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	if v.Verdict != enums.VerdictInconclusive {
		t.Errorf("Verdict = %q, want inconclusive (demoted)", v.Verdict)
	}
	if len(v.WarningAngles) != 0 {
		t.Errorf("WarningAngles = %v, want [] (demoted)", v.WarningAngles)
	}
}

func TestUseCase_CanonicalAngleOrder(t *testing.T) {
	// Submit signals in non-canonical order; verify output sorted per enums.AllAngles().
	uc := makeUseCase("uc-multi", enums.ArchetypeHTTPAPI)
	pol := makeEffectivePolicy(enums.ArchetypeHTTPAPI,
		[]enums.ValidationAngle{enums.AngleBuild, enums.AngleSecurity, enums.AngleUnitTest},
		nil, 0.7)
	// Reverse order intentionally.
	signals := []aggregate.AggregateSignal{
		makeSignal("uc-multi", "s-unit", enums.AngleUnitTest, enums.VerdictFail, 1.0),
		makeSignal("uc-multi", "s-sec", enums.AngleSecurity, enums.VerdictFail, 1.0),
		makeSignal("uc-multi", "s-build", enums.AngleBuild, enums.VerdictFail, 1.0),
	}
	sensors := []sensor.Sensor{
		makeSensor("s-unit", "uc-multi", enums.AngleUnitTest, enums.NatureComputational),
		makeSensor("s-sec", "uc-multi", enums.AngleSecurity, enums.NatureComputational),
		makeSensor("s-build", "uc-multi", enums.AngleBuild, enums.NatureComputational),
	}
	v, err := UseCase(uc, enums.ArchetypeHTTPAPI, signals, sensors, pol)
	if err != nil {
		t.Fatalf("UseCase: %v", err)
	}
	// enums.AllAngles() order is: security, build, code-structure, unit-test, ...
	want := []enums.ValidationAngle{enums.AngleSecurity, enums.AngleBuild, enums.AngleUnitTest}
	if !reflect.DeepEqual(v.FailingAngles, want) {
		t.Errorf("FailingAngles = %v, want %v (canonical order)", v.FailingAngles, want)
	}
	for i, h := range v.HealHints {
		if h.Angle != want[i] {
			t.Errorf("HealHints[%d].Angle = %q, want %q", i, h.Angle, want[i])
		}
	}
}
