package aggregator

import (
	"testing"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
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
