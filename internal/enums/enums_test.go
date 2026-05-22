package enums

import "testing"

func TestAllAnglesReturnsCanonicalOrder(t *testing.T) {
	got := AllAngles()
	want := []ValidationAngle{
		AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
		AngleE2ETest, AngleContracts, AngleLogs, AngleMetrics,
		AngleDatabase, AnglePerformance,
	}
	if len(got) != len(want) {
		t.Fatalf("AllAngles length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllAngles[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestIsValidAngleAcceptsCanonical(t *testing.T) {
	for _, v := range AllAngles() {
		if !IsValidAngle(string(v)) {
			t.Errorf("IsValidAngle(%q) = false; want true", v)
		}
	}
}

func TestIsValidAngleRejectsUnknown(t *testing.T) {
	cases := []string{"", "not-a-real-angle", "E2E-TEST", " pass"}
	for _, c := range cases {
		if IsValidAngle(c) {
			t.Errorf("IsValidAngle(%q) = true; want false", c)
		}
	}
}

func TestAllArchetypesReturnsCanonicalOrder(t *testing.T) {
	got := AllArchetypes()
	want := []Archetype{
		ArchetypeHTTPAPI, ArchetypeEventConsumer, ArchetypeEventProducer,
		ArchetypeCLI, ArchetypeSDK, ArchetypeLibrary,
		ArchetypeWorker, ArchetypeBatchJob, ArchetypeStaticSite,
	}
	if len(got) != len(want) {
		t.Fatalf("AllArchetypes length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllArchetypes[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestIsValidArchetypeAcceptsCanonical(t *testing.T) {
	for _, v := range AllArchetypes() {
		if !IsValidArchetype(string(v)) {
			t.Errorf("IsValidArchetype(%q) = false; want true", v)
		}
	}
}

func TestIsValidArchetypeRejectsUnknown(t *testing.T) {
	cases := []string{"", "monolith", "HTTP-API", " cli"}
	for _, c := range cases {
		if IsValidArchetype(c) {
			t.Errorf("IsValidArchetype(%q) = true; want false", c)
		}
	}
}
