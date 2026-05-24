package policy

import (
	"reflect"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func effectiveFixture() *EffectivePolicy {
	return &EffectivePolicy{
		SchemaVersion: SupportedSchemaVersion,
		ResolvedFrom:  []string{"global", "local"},
		PerArchetype: map[enums.Archetype]map[enums.ValidationAngle]AngleStatus{
			enums.ArchetypeHTTPAPI: {
				enums.AngleBuild:       StatusObligatory,
				enums.AngleSecurity:    StatusObligatory,
				enums.AnglePerformance: StatusOptional,
				enums.AngleLogs:        StatusOptional,
				enums.AngleE2ETest:     StatusDisabled,
			},
		},
	}
}

func TestAnglesFor_ReturnsSortedObligatoryAndOptional(t *testing.T) {
	p := effectiveFixture()
	got, optional := p.AnglesFor(enums.ArchetypeHTTPAPI)
	wantObligatory := []enums.ValidationAngle{enums.AngleBuild, enums.AngleSecurity}
	wantOptional := []enums.ValidationAngle{enums.AngleLogs, enums.AnglePerformance}
	if !reflect.DeepEqual(got, wantObligatory) {
		t.Errorf("obligatory = %v, want %v", got, wantObligatory)
	}
	if !reflect.DeepEqual(optional, wantOptional) {
		t.Errorf("optional = %v, want %v", optional, wantOptional)
	}
}

func TestAnglesFor_ExcludesDisabled(t *testing.T) {
	p := effectiveFixture()
	obligatory, optional := p.AnglesFor(enums.ArchetypeHTTPAPI)
	for _, a := range obligatory {
		if a == enums.AngleE2ETest {
			t.Error("disabled angle e2e-test must not appear in obligatory")
		}
	}
	for _, a := range optional {
		if a == enums.AngleE2ETest {
			t.Error("disabled angle e2e-test must not appear in optional")
		}
	}
}

func TestAnglesFor_ExcludesUnsetWithinConfiguredArchetype(t *testing.T) {
	p := effectiveFixture()
	// AngleContracts is applicable to http-api per E1, but the fixture
	// does not configure it — it should appear in neither output slice.
	obligatory, optional := p.AnglesFor(enums.ArchetypeHTTPAPI)
	for _, a := range obligatory {
		if a == enums.AngleContracts {
			t.Error("unset angle contracts must not appear in obligatory")
		}
	}
	for _, a := range optional {
		if a == enums.AngleContracts {
			t.Error("unset angle contracts must not appear in optional")
		}
	}
}

func TestAnglesFor_NilReceiver(t *testing.T) {
	var p *EffectivePolicy
	obl, opt := p.AnglesFor(enums.ArchetypeHTTPAPI)
	if obl == nil || opt == nil {
		t.Fatal("nil receiver must return empty slices, not nil")
	}
	if len(obl) != 0 || len(opt) != 0 {
		t.Errorf("nil receiver must return empty slices, got obl=%v opt=%v", obl, opt)
	}
}

func TestStatus_NilReceiver(t *testing.T) {
	var p *EffectivePolicy
	if got := p.Status(enums.ArchetypeHTTPAPI, enums.AngleBuild); got != "" {
		t.Errorf("nil receiver Status = %q, want %q", got, "")
	}
}

func TestAnglesFor_ReturnsEmptyNotNilForUnknownArchetype(t *testing.T) {
	p := effectiveFixture()
	obligatory, optional := p.AnglesFor(enums.ArchetypeCLI)
	if obligatory == nil || optional == nil {
		t.Fatal("AnglesFor must return empty slices, not nil")
	}
	if len(obligatory) != 0 || len(optional) != 0 {
		t.Errorf("expected empty slices, got obligatory=%v optional=%v", obligatory, optional)
	}
}

func TestStatus_ReturnsConfiguredValues(t *testing.T) {
	p := effectiveFixture()
	cases := []struct {
		arch  enums.Archetype
		angle enums.ValidationAngle
		want  AngleStatus
	}{
		{enums.ArchetypeHTTPAPI, enums.AngleBuild, StatusObligatory},
		{enums.ArchetypeHTTPAPI, enums.AnglePerformance, StatusOptional},
		{enums.ArchetypeHTTPAPI, enums.AngleE2ETest, StatusDisabled},
		{enums.ArchetypeHTTPAPI, enums.AngleContracts, ""}, // unset
		{enums.ArchetypeCLI, enums.AngleBuild, ""},         // unconfigured archetype
	}
	for _, tc := range cases {
		got := p.Status(tc.arch, tc.angle)
		if got != tc.want {
			t.Errorf("Status(%s, %s) = %q, want %q", tc.arch, tc.angle, got, tc.want)
		}
	}
}
