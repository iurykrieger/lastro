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
