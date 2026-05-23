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

func TestAllSensorKindsReturnsCanonicalOrder(t *testing.T) {
	got := AllSensorKinds()
	want := []SensorKind{KindAssertion, KindObservational}
	if len(got) != len(want) {
		t.Fatalf("AllSensorKinds length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllSensorKinds[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestIsValidSensorKindAcceptsCanonical(t *testing.T) {
	for _, v := range AllSensorKinds() {
		if !IsValidSensorKind(string(v)) {
			t.Errorf("IsValidSensorKind(%q) = false; want true", v)
		}
	}
}

func TestIsValidSensorKindRejectsUnknown(t *testing.T) {
	cases := []string{"", "ASSERTION", "watcher", " assertion"}
	for _, c := range cases {
		if IsValidSensorKind(c) {
			t.Errorf("IsValidSensorKind(%q) = true; want false", c)
		}
	}
}

func TestAllSensorNaturesReturnsCanonicalOrder(t *testing.T) {
	got := AllSensorNatures()
	want := []SensorNature{NatureComputational, NatureInferential}
	if len(got) != len(want) {
		t.Fatalf("AllSensorNatures length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllSensorNatures[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestIsValidSensorNatureAcceptsCanonical(t *testing.T) {
	for _, v := range AllSensorNatures() {
		if !IsValidSensorNature(string(v)) {
			t.Errorf("IsValidSensorNature(%q) = false; want true", v)
		}
	}
}

func TestIsValidSensorNatureRejectsUnknown(t *testing.T) {
	cases := []string{"", "deterministic", "INFERENTIAL", " computational"}
	for _, c := range cases {
		if IsValidSensorNature(c) {
			t.Errorf("IsValidSensorNature(%q) = true; want false", c)
		}
	}
}

func TestAllSignalOutputTypesReturnsCanonicalOrder(t *testing.T) {
	got := AllSignalOutputTypes()
	want := []SignalOutputType{OutputSingleShot, OutputStream}
	if len(got) != len(want) {
		t.Fatalf("AllSignalOutputTypes length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllSignalOutputTypes[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestIsValidSignalOutputTypeAcceptsCanonical(t *testing.T) {
	for _, v := range AllSignalOutputTypes() {
		if !IsValidSignalOutputType(string(v)) {
			t.Errorf("IsValidSignalOutputType(%q) = false; want true", v)
		}
	}
}

func TestIsValidSignalOutputTypeRejectsUnknown(t *testing.T) {
	cases := []string{"", "batched", "SINGLE-SHOT", " stream"}
	for _, c := range cases {
		if IsValidSignalOutputType(c) {
			t.Errorf("IsValidSignalOutputType(%q) = true; want false", c)
		}
	}
}

func TestAllFixtureRolesReturnsCanonicalOrder(t *testing.T) {
	got := AllFixtureRoles()
	want := []FixtureRole{RoleInput, RoleExpectedOutput, RoleExpectedSideEffect}
	if len(got) != len(want) {
		t.Fatalf("AllFixtureRoles length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllFixtureRoles[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestIsValidFixtureRoleAcceptsCanonical(t *testing.T) {
	for _, v := range AllFixtureRoles() {
		if !IsValidFixtureRole(string(v)) {
			t.Errorf("IsValidFixtureRole(%q) = false; want true", v)
		}
	}
}

func TestIsValidFixtureRoleRejectsUnknown(t *testing.T) {
	cases := []string{"", "output", "INPUT", " input"}
	for _, c := range cases {
		if IsValidFixtureRole(c) {
			t.Errorf("IsValidFixtureRole(%q) = true; want false", c)
		}
	}
}

func TestAllVerdictsReturnsCanonicalOrder(t *testing.T) {
	got := AllVerdicts()
	want := []Verdict{VerdictPass, VerdictFail, VerdictInconclusive}
	if len(got) != len(want) {
		t.Fatalf("AllVerdicts length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllVerdicts[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestIsValidVerdictAcceptsCanonical(t *testing.T) {
	for _, v := range AllVerdicts() {
		if !IsValidVerdict(string(v)) {
			t.Errorf("IsValidVerdict(%q) = false; want true", v)
		}
	}
}

func TestIsValidVerdictRejectsUnknown(t *testing.T) {
	cases := []string{"", "passed", "PASS", " pass"}
	for _, c := range cases {
		if IsValidVerdict(c) {
			t.Errorf("IsValidVerdict(%q) = true; want false", c)
		}
	}
}

func TestAllTerminationReasonsReturnsCanonicalOrder(t *testing.T) {
	got := AllTerminationReasons()
	want := []TerminationReason{
		TerminationCompleted, TerminationStopped,
		TerminationTimeout, TerminationError,
	}
	if len(got) != len(want) {
		t.Fatalf("AllTerminationReasons length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllTerminationReasons[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestIsValidTerminationReasonAcceptsCanonical(t *testing.T) {
	for _, v := range AllTerminationReasons() {
		if !IsValidTerminationReason(string(v)) {
			t.Errorf("IsValidTerminationReason(%q) = false; want true", v)
		}
	}
}

func TestIsValidTerminationReasonRejectsUnknown(t *testing.T) {
	cases := []string{"", "done", "TIMEOUT", " completed"}
	for _, c := range cases {
		if IsValidTerminationReason(c) {
			t.Errorf("IsValidTerminationReason(%q) = true; want false", c)
		}
	}
}

func TestApplicableAnglesHasAllNineArchetypes(t *testing.T) {
	if len(ApplicableAngles) != 9 {
		t.Fatalf("ApplicableAngles size: got %d, want 9", len(ApplicableAngles))
	}
	for _, a := range AllArchetypes() {
		if _, ok := ApplicableAngles[a]; !ok {
			t.Errorf("ApplicableAngles missing entry for archetype %q", a)
		}
	}
}

func TestApplicableAnglesValuesAreCanonicalAngles(t *testing.T) {
	for a, list := range ApplicableAngles {
		for _, v := range list {
			if !IsValidAngle(string(v)) {
				t.Errorf("ApplicableAngles[%q] contains non-canonical angle %q", a, v)
			}
		}
	}
}

func TestApplicableAnglesNoDuplicatesPerArchetype(t *testing.T) {
	for a, list := range ApplicableAngles {
		seen := map[ValidationAngle]bool{}
		for _, v := range list {
			if seen[v] {
				t.Errorf("ApplicableAngles[%q] contains duplicate angle %q", a, v)
			}
			seen[v] = true
		}
	}
}

func TestAppliesTrueWhenAngleInList(t *testing.T) {
	cases := []struct {
		a Archetype
		v ValidationAngle
	}{
		{ArchetypeHTTPAPI, AnglePerformance},
		{ArchetypeHTTPAPI, AngleE2ETest},
		{ArchetypeCLI, AngleContracts},
		{ArchetypeStaticSite, AngleSecurity},
		{ArchetypeBatchJob, AngleDatabase},
	}
	for _, c := range cases {
		if !Applies(c.a, c.v) {
			t.Errorf("Applies(%q, %q) = false; want true", c.a, c.v)
		}
	}
}

func TestAppliesFalseWhenAngleNotInList(t *testing.T) {
	cases := []struct {
		a Archetype
		v ValidationAngle
	}{
		{ArchetypeCLI, AnglePerformance},
		{ArchetypeCLI, AngleE2ETest},
		{ArchetypeStaticSite, AngleDatabase},
		{ArchetypeStaticSite, AngleUnitTest},
		{ArchetypeSDK, AngleE2ETest},
	}
	for _, c := range cases {
		if Applies(c.a, c.v) {
			t.Errorf("Applies(%q, %q) = true; want false", c.a, c.v)
		}
	}
}

func TestAllStackKindsReturnsCanonicalOrder(t *testing.T) {
	got := AllStackKinds()
	want := []StackKind{
		StackKindLibrary, StackKindRuntime, StackKindFramework,
		StackKindDatastore, StackKindProtocol, StackKindTool,
	}
	if len(got) != len(want) {
		t.Fatalf("AllStackKinds length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllStackKinds[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestIsValidStackKindAcceptsCanonical(t *testing.T) {
	for _, v := range AllStackKinds() {
		if !IsValidStackKind(string(v)) {
			t.Errorf("IsValidStackKind(%q) = false; want true", v)
		}
	}
}

func TestIsValidStackKindRejectsUnknown(t *testing.T) {
	cases := []string{"", "database", "LIBRARY", " tool", "service"}
	for _, c := range cases {
		if IsValidStackKind(c) {
			t.Errorf("IsValidStackKind(%q) = true; want false", c)
		}
	}
}
