package policy

import (
	"bytes"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestMarshalYAML_ContainsResolvedFromAndNoScope(t *testing.T) {
	p := effectiveFixture()
	out, err := p.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	if !bytes.Contains(out, []byte("resolved_from:")) {
		t.Errorf("output missing resolved_from:\n%s", out)
	}
	if bytes.Contains(out, []byte("scope:")) {
		t.Errorf("output must not contain scope: but did:\n%s", out)
	}
}

func TestMarshalYAML_IsDeterministic(t *testing.T) {
	p := effectiveFixture()
	a, err := p.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML a: %v", err)
	}
	b, err := p.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("MarshalYAML is non-deterministic.\nA:\n%s\nB:\n%s", a, b)
	}
}

func TestMarshalYAML_SortsAnglesAndArchetypes(t *testing.T) {
	p := &EffectivePolicy{
		SchemaVersion: SupportedSchemaVersion,
		ResolvedFrom:  []string{"global", "local"},
		PerArchetype: map[enums.Archetype]map[enums.ValidationAngle]AngleStatus{
			enums.ArchetypeHTTPAPI: {
				enums.AngleSecurity: StatusObligatory,
				enums.AngleBuild:    StatusObligatory,
			},
			enums.ArchetypeCLI: {
				enums.AngleBuild: StatusObligatory,
			},
		},
	}
	out, err := p.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	s := string(out)
	cliIdx := strings.Index(s, "cli:")
	httpIdx := strings.Index(s, "http-api:")
	if cliIdx < 0 || httpIdx < 0 || cliIdx >= httpIdx {
		t.Errorf("archetypes not alphabetical:\n%s", s)
	}
	httpBlock := s[httpIdx:]
	buildIdx := strings.Index(httpBlock, "build")
	secIdx := strings.Index(httpBlock, "security")
	if buildIdx < 0 || secIdx < 0 || buildIdx >= secIdx {
		t.Errorf("angles in http-api obligatory not alphabetical:\n%s", httpBlock)
	}
}

func TestMarshalYAML_EmptyListsRenderedAsBrackets(t *testing.T) {
	p := &EffectivePolicy{
		SchemaVersion: SupportedSchemaVersion,
		ResolvedFrom:  []string{"global"},
		PerArchetype: map[enums.Archetype]map[enums.ValidationAngle]AngleStatus{
			enums.ArchetypeCLI: {
				enums.AngleBuild: StatusObligatory,
			},
		},
	}
	out, err := p.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "optional_angles: []") {
		t.Errorf("optional_angles: [] missing:\n%s", s)
	}
	if !strings.Contains(s, "disabled_angles: []") {
		t.Errorf("disabled_angles: [] missing:\n%s", s)
	}
}

func TestMarshalYAML_RoundTripIntoLoadFails(t *testing.T) {
	p := effectiveFixture()
	out, err := p.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	_, err = Load(bytes.NewReader(out))
	if err == nil {
		t.Fatal("Load(MarshalYAML(effective)) succeeded; should have failed (effective dumps are not re-ingestable)")
	}
}
