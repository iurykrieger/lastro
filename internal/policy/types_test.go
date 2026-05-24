package policy

import (
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestSupportedSchemaVersionMatchesExample(t *testing.T) {
	if SupportedSchemaVersion != "1.0.0" {
		t.Errorf("SupportedSchemaVersion = %q, want %q", SupportedSchemaVersion, "1.0.0")
	}
}

func TestScopeConstants(t *testing.T) {
	if ScopeGlobal != "global" {
		t.Errorf("ScopeGlobal = %q, want global", ScopeGlobal)
	}
	if ScopeLocal != "local" {
		t.Errorf("ScopeLocal = %q, want local", ScopeLocal)
	}
}

func TestAngleStatusConstants(t *testing.T) {
	cases := map[AngleStatus]string{
		StatusObligatory: "obligatory",
		StatusOptional:   "optional",
		StatusDisabled:   "disabled",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("AngleStatus %q has underlying string %q, want %q", got, string(got), want)
		}
	}
}

func TestZeroValueAngleStatusIsUnset(t *testing.T) {
	var zero AngleStatus
	if zero != "" {
		t.Errorf("zero AngleStatus = %q, want empty string (unset sentinel)", zero)
	}
}

func TestValidationPolicyShape(t *testing.T) {
	var p ValidationPolicy
	if p.PerArchetype != nil {
		t.Errorf("zero ValidationPolicy.PerArchetype = %v, want nil", p.PerArchetype)
	}
	p.SchemaVersion = "1.0.0"
	p.Scope = ScopeGlobal
	p.PerArchetype = map[enums.Archetype]ArchetypeBlock{
		enums.ArchetypeHTTPAPI: {
			Obligatory: []enums.ValidationAngle{enums.AngleBuild},
			Optional:   []enums.ValidationAngle{enums.AngleLogs},
			Disabled:   []enums.ValidationAngle{},
		},
	}
	if got := p.PerArchetype[enums.ArchetypeHTTPAPI].Obligatory[0]; got != enums.AngleBuild {
		t.Errorf("round-trip Obligatory[0] = %q, want build", got)
	}
}

func TestEffectivePolicyShape(t *testing.T) {
	var p EffectivePolicy
	if p.PerArchetype != nil {
		t.Errorf("zero EffectivePolicy.PerArchetype = %v, want nil", p.PerArchetype)
	}
	if p.ResolvedFrom != nil {
		t.Errorf("zero EffectivePolicy.ResolvedFrom = %v, want nil", p.ResolvedFrom)
	}
	p.PerArchetype = map[enums.Archetype]map[enums.ValidationAngle]AngleStatus{
		enums.ArchetypeCLI: {enums.AngleBuild: StatusObligatory},
	}
	if got := p.PerArchetype[enums.ArchetypeCLI][enums.AngleBuild]; got != StatusObligatory {
		t.Errorf("round-trip status = %q, want obligatory", got)
	}
}
