package policy

import (
	"reflect"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func fixtureGlobal() *ValidationPolicy {
	return &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeGlobal,
		PerArchetype: map[enums.Archetype]ArchetypeBlock{
			enums.ArchetypeHTTPAPI: {
				Obligatory: []enums.ValidationAngle{enums.AngleBuild, enums.AngleSecurity},
				Optional:   []enums.ValidationAngle{enums.AnglePerformance},
				Disabled:   []enums.ValidationAngle{},
			},
		},
	}
}

func TestResolve_BothNil(t *testing.T) {
	got := Resolve(nil, nil)
	if got == nil {
		t.Fatal("Resolve(nil, nil) returned nil; want empty *EffectivePolicy")
	}
	if got.SchemaVersion != SupportedSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", got.SchemaVersion, SupportedSchemaVersion)
	}
	if len(got.ResolvedFrom) != 0 {
		t.Errorf("ResolvedFrom = %v, want []", got.ResolvedFrom)
	}
	if len(got.PerArchetype) != 0 {
		t.Errorf("PerArchetype size = %d, want 0", len(got.PerArchetype))
	}
}

func TestResolve_LocalNil(t *testing.T) {
	got := Resolve(fixtureGlobal(), nil)
	if !reflect.DeepEqual(got.ResolvedFrom, []string{"global"}) {
		t.Errorf("ResolvedFrom = %v, want [global]", got.ResolvedFrom)
	}
	if got.PerArchetype[enums.ArchetypeHTTPAPI][enums.AngleBuild] != StatusObligatory {
		t.Error("http-api build should be obligatory")
	}
	if got.PerArchetype[enums.ArchetypeHTTPAPI][enums.AnglePerformance] != StatusOptional {
		t.Error("http-api performance should be optional")
	}
}

func TestResolve_GlobalNil(t *testing.T) {
	local := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeLocal,
		PerArchetype: map[enums.Archetype]ArchetypeBlock{
			enums.ArchetypeCLI: {
				Obligatory: []enums.ValidationAngle{enums.AngleBuild},
				Optional:   []enums.ValidationAngle{},
				Disabled:   []enums.ValidationAngle{},
			},
		},
	}
	got := Resolve(nil, local)
	if !reflect.DeepEqual(got.ResolvedFrom, []string{"local"}) {
		t.Errorf("ResolvedFrom = %v, want [local]", got.ResolvedFrom)
	}
	if got.PerArchetype[enums.ArchetypeCLI][enums.AngleBuild] != StatusObligatory {
		t.Error("cli build should be obligatory")
	}
}

func TestResolve_PerAngleOverride(t *testing.T) {
	local := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeLocal,
		PerArchetype: map[enums.Archetype]ArchetypeBlock{
			enums.ArchetypeHTTPAPI: {
				Obligatory: []enums.ValidationAngle{enums.AnglePerformance},
				Optional:   []enums.ValidationAngle{},
				Disabled:   []enums.ValidationAngle{},
			},
		},
	}
	got := Resolve(fixtureGlobal(), local)
	if got.PerArchetype[enums.ArchetypeHTTPAPI][enums.AnglePerformance] != StatusObligatory {
		t.Error("performance should be obligatory after local override")
	}
	if got.PerArchetype[enums.ArchetypeHTTPAPI][enums.AngleBuild] != StatusObligatory {
		t.Error("build should remain obligatory from global")
	}
	if !reflect.DeepEqual(got.ResolvedFrom, []string{"global", "local"}) {
		t.Errorf("ResolvedFrom = %v, want [global local]", got.ResolvedFrom)
	}
}

func TestResolve_LocalDisablesObligatory(t *testing.T) {
	local := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeLocal,
		PerArchetype: map[enums.Archetype]ArchetypeBlock{
			enums.ArchetypeHTTPAPI: {
				Obligatory: []enums.ValidationAngle{},
				Optional:   []enums.ValidationAngle{},
				Disabled:   []enums.ValidationAngle{enums.AngleBuild},
			},
		},
	}
	got := Resolve(fixtureGlobal(), local)
	if got.PerArchetype[enums.ArchetypeHTTPAPI][enums.AngleBuild] != StatusDisabled {
		t.Error("build should be disabled after local disables it")
	}
}

func TestResolve_LocalIntroducesNewArchetype(t *testing.T) {
	local := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeLocal,
		PerArchetype: map[enums.Archetype]ArchetypeBlock{
			enums.ArchetypeCLI: {
				Obligatory: []enums.ValidationAngle{enums.AngleBuild},
				Optional:   []enums.ValidationAngle{},
				Disabled:   []enums.ValidationAngle{},
			},
		},
	}
	got := Resolve(fixtureGlobal(), local)
	if got.PerArchetype[enums.ArchetypeCLI][enums.AngleBuild] != StatusObligatory {
		t.Error("cli build should appear in effective from local-only archetype")
	}
	if got.PerArchetype[enums.ArchetypeHTTPAPI][enums.AngleBuild] != StatusObligatory {
		t.Error("http-api build should remain from global")
	}
}

func TestResolve_InapplicableAngleSilentlyDropped(t *testing.T) {
	global := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeGlobal,
		PerArchetype: map[enums.Archetype]ArchetypeBlock{
			enums.ArchetypeCLI: {
				Obligatory: []enums.ValidationAngle{},
				Optional:   []enums.ValidationAngle{},
				Disabled:   []enums.ValidationAngle{enums.AngleE2ETest},
			},
		},
	}
	local := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeLocal,
		PerArchetype:  map[enums.Archetype]ArchetypeBlock{},
	}
	got := Resolve(global, local)
	if _, present := got.PerArchetype[enums.ArchetypeCLI][enums.AngleE2ETest]; present {
		t.Error("cli e2e-test should be absent (not applicable per E1 matrix)")
	}
}

func TestResolve_GlobalDisablesLocalSilent(t *testing.T) {
	global := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeGlobal,
		PerArchetype: map[enums.Archetype]ArchetypeBlock{
			enums.ArchetypeCLI: {
				Obligatory: []enums.ValidationAngle{},
				Optional:   []enums.ValidationAngle{},
				Disabled:   []enums.ValidationAngle{enums.AngleBuild},
			},
		},
	}
	local := &ValidationPolicy{
		SchemaVersion: "1.0.0",
		Scope:         ScopeLocal,
		PerArchetype:  map[enums.Archetype]ArchetypeBlock{}, // silent on cli
	}
	got := Resolve(global, local)
	if got.PerArchetype[enums.ArchetypeCLI][enums.AngleBuild] != StatusDisabled {
		t.Errorf("cli build = %q, want disabled (local silent on it)", got.PerArchetype[enums.ArchetypeCLI][enums.AngleBuild])
	}
}

func floatPtr(v float64) *float64 { return &v }

func TestResolve_FloorBothNilUsesDefault(t *testing.T) {
	g := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeGlobal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}}
	l := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeLocal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}}
	eff := Resolve(g, l)
	if eff.InferentialFloor != DefaultInferentialFloor {
		t.Errorf("InferentialFloor = %v, want %v", eff.InferentialFloor, DefaultInferentialFloor)
	}
}

func TestResolve_FloorGlobalSet_LocalNil(t *testing.T) {
	g := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeGlobal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}, InferentialFloor: floatPtr(0.5)}
	l := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeLocal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}}
	eff := Resolve(g, l)
	if eff.InferentialFloor != 0.5 {
		t.Errorf("InferentialFloor = %v, want 0.5", eff.InferentialFloor)
	}
}

func TestResolve_FloorLocalSet_GlobalNil(t *testing.T) {
	g := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeGlobal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}}
	l := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeLocal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}, InferentialFloor: floatPtr(0.9)}
	eff := Resolve(g, l)
	if eff.InferentialFloor != 0.9 {
		t.Errorf("InferentialFloor = %v, want 0.9", eff.InferentialFloor)
	}
}

func TestResolve_FloorBothSet_LocalWins(t *testing.T) {
	g := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeGlobal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}, InferentialFloor: floatPtr(0.5)}
	l := &ValidationPolicy{SchemaVersion: "1.0.0", Scope: ScopeLocal, PerArchetype: map[enums.Archetype]ArchetypeBlock{}, InferentialFloor: floatPtr(0.9)}
	eff := Resolve(g, l)
	if eff.InferentialFloor != 0.9 {
		t.Errorf("InferentialFloor = %v, want 0.9 (local wins)", eff.InferentialFloor)
	}
}

func TestResolve_NilNil_FloorIsDefault(t *testing.T) {
	eff := Resolve(nil, nil)
	if eff.InferentialFloor != DefaultInferentialFloor {
		t.Errorf("Resolve(nil,nil).InferentialFloor = %v, want %v", eff.InferentialFloor, DefaultInferentialFloor)
	}
}

func TestResolve_MaxHealIterations_DefaultsTo3_WhenUnset(t *testing.T) {
	eff := Resolve(nil, nil)
	if eff.MaxHealIterations != DefaultMaxHealIterations {
		t.Errorf("MaxHealIterations = %d, want %d", eff.MaxHealIterations, DefaultMaxHealIterations)
	}
}

func TestResolve_MaxHealIterations_GlobalSet_LocalUnset(t *testing.T) {
	five := 5
	eff := Resolve(&ValidationPolicy{SchemaVersion: SupportedSchemaVersion, Scope: ScopeGlobal, MaxHealIterations: &five}, nil)
	if eff.MaxHealIterations != 5 {
		t.Errorf("MaxHealIterations = %d, want 5", eff.MaxHealIterations)
	}
}

func TestResolve_MaxHealIterations_LocalOverridesGlobal(t *testing.T) {
	five := 5
	eight := 8
	eff := Resolve(
		&ValidationPolicy{SchemaVersion: SupportedSchemaVersion, Scope: ScopeGlobal, MaxHealIterations: &five},
		&ValidationPolicy{SchemaVersion: SupportedSchemaVersion, Scope: ScopeLocal, MaxHealIterations: &eight},
	)
	if eff.MaxHealIterations != 8 {
		t.Errorf("MaxHealIterations = %d, want 8", eff.MaxHealIterations)
	}
}
