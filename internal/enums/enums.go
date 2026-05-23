// Package enums provides typed constants and validators for the framework's
// nine fixed enums, plus the canonical archetype × angle matrix.
//
// The canonical source for every enum is YAML under schemas/enums/. Drift
// between this package and that source is caught by drift_test.go.
package enums

// ValidationAngle is one of the ten facets a sensor can validate.
type ValidationAngle string

const (
	AngleSecurity      ValidationAngle = "security"
	AngleBuild         ValidationAngle = "build"
	AngleCodeStructure ValidationAngle = "code-structure"
	AngleUnitTest      ValidationAngle = "unit-test"
	AngleE2ETest       ValidationAngle = "e2e-test"
	AngleContracts     ValidationAngle = "contracts"
	AngleLogs          ValidationAngle = "logs"
	AngleMetrics       ValidationAngle = "metrics"
	AngleDatabase      ValidationAngle = "database"
	AnglePerformance   ValidationAngle = "performance"
)

// AllAngles returns every ValidationAngle in canonical (YAML) order.
func AllAngles() []ValidationAngle {
	return []ValidationAngle{
		AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
		AngleE2ETest, AngleContracts, AngleLogs, AngleMetrics,
		AngleDatabase, AnglePerformance,
	}
}

// IsValidAngle reports whether s is one of the canonical ValidationAngle values.
func IsValidAngle(s string) bool {
	for _, v := range AllAngles() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// Archetype is the shape of an application's observable surface.
type Archetype string

const (
	ArchetypeHTTPAPI       Archetype = "http-api"
	ArchetypeEventConsumer Archetype = "event-consumer"
	ArchetypeEventProducer Archetype = "event-producer"
	ArchetypeCLI           Archetype = "cli"
	ArchetypeSDK           Archetype = "sdk"
	ArchetypeLibrary       Archetype = "library"
	ArchetypeWorker        Archetype = "worker"
	ArchetypeBatchJob      Archetype = "batch-job"
	ArchetypeStaticSite    Archetype = "static-site"
)

// AllArchetypes returns every Archetype in canonical (YAML) order.
func AllArchetypes() []Archetype {
	return []Archetype{
		ArchetypeHTTPAPI, ArchetypeEventConsumer, ArchetypeEventProducer,
		ArchetypeCLI, ArchetypeSDK, ArchetypeLibrary,
		ArchetypeWorker, ArchetypeBatchJob, ArchetypeStaticSite,
	}
}

// IsValidArchetype reports whether s is one of the canonical Archetype values.
func IsValidArchetype(s string) bool {
	for _, v := range AllArchetypes() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// SensorKind is the lifecycle shape of a sensor.
type SensorKind string

const (
	KindAssertion     SensorKind = "assertion"
	KindObservational SensorKind = "observational"
)

// AllSensorKinds returns every SensorKind in canonical (YAML) order.
func AllSensorKinds() []SensorKind {
	return []SensorKind{KindAssertion, KindObservational}
}

// IsValidSensorKind reports whether s is one of the canonical SensorKind values.
func IsValidSensorKind(s string) bool {
	for _, v := range AllSensorKinds() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// SensorNature is the epistemic source of a sensor's verdicts.
type SensorNature string

const (
	NatureComputational SensorNature = "computational"
	NatureInferential   SensorNature = "inferential"
)

// AllSensorNatures returns every SensorNature in canonical (YAML) order.
func AllSensorNatures() []SensorNature {
	return []SensorNature{NatureComputational, NatureInferential}
}

// IsValidSensorNature reports whether s is one of the canonical SensorNature values.
func IsValidSensorNature(s string) bool {
	for _, v := range AllSensorNatures() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// SignalOutputType describes whether a sensor emits one signal or a stream.
type SignalOutputType string

const (
	OutputSingleShot SignalOutputType = "single-shot"
	OutputStream     SignalOutputType = "stream"
)

// AllSignalOutputTypes returns every SignalOutputType in canonical (YAML) order.
func AllSignalOutputTypes() []SignalOutputType {
	return []SignalOutputType{OutputSingleShot, OutputStream}
}

// IsValidSignalOutputType reports whether s is one of the canonical SignalOutputType values.
func IsValidSignalOutputType(s string) bool {
	for _, v := range AllSignalOutputTypes() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// FixtureRole describes the role a fixture plays in driving or verifying behavior.
type FixtureRole string

const (
	RoleInput              FixtureRole = "input"
	RoleExpectedOutput     FixtureRole = "expected-output"
	RoleExpectedSideEffect FixtureRole = "expected-side-effect"
)

// AllFixtureRoles returns every FixtureRole in canonical (YAML) order.
func AllFixtureRoles() []FixtureRole {
	return []FixtureRole{RoleInput, RoleExpectedOutput, RoleExpectedSideEffect}
}

// IsValidFixtureRole reports whether s is one of the canonical FixtureRole values.
func IsValidFixtureRole(s string) bool {
	for _, v := range AllFixtureRoles() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// Verdict is the outcome of a sensor execution or aggregation.
type Verdict string

const (
	VerdictPass         Verdict = "pass"
	VerdictWarn         Verdict = "warn"
	VerdictFail         Verdict = "fail"
	VerdictInconclusive Verdict = "inconclusive"
)

// AllVerdicts returns every Verdict in canonical (YAML) order.
func AllVerdicts() []Verdict {
	return []Verdict{VerdictPass, VerdictWarn, VerdictFail, VerdictInconclusive}
}

// IsValidVerdict reports whether s is one of the canonical Verdict values.
func IsValidVerdict(s string) bool {
	for _, v := range AllVerdicts() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// TerminationReason explains why a sensor execution ended.
type TerminationReason string

const (
	TerminationCompleted TerminationReason = "completed"
	TerminationStopped   TerminationReason = "stopped"
	TerminationTimeout   TerminationReason = "timeout"
	TerminationError     TerminationReason = "error"
)

// AllTerminationReasons returns every TerminationReason in canonical (YAML) order.
func AllTerminationReasons() []TerminationReason {
	return []TerminationReason{
		TerminationCompleted, TerminationStopped,
		TerminationTimeout, TerminationError,
	}
}

// IsValidTerminationReason reports whether s is one of the canonical TerminationReason values.
func IsValidTerminationReason(s string) bool {
	for _, v := range AllTerminationReasons() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// StackKind is the classification of a detected StackComponent.
type StackKind string

const (
	StackKindLibrary   StackKind = "library"
	StackKindRuntime   StackKind = "runtime"
	StackKindFramework StackKind = "framework"
	StackKindDatastore StackKind = "datastore"
	StackKindProtocol  StackKind = "protocol"
	StackKindTool      StackKind = "tool"
)

// AllStackKinds returns every StackKind in canonical (YAML) order.
func AllStackKinds() []StackKind {
	return []StackKind{
		StackKindLibrary, StackKindRuntime, StackKindFramework,
		StackKindDatastore, StackKindProtocol, StackKindTool,
	}
}

// IsValidStackKind reports whether s is one of the canonical StackKind values.
func IsValidStackKind(s string) bool {
	for _, v := range AllStackKinds() {
		if string(v) == s {
			return true
		}
	}
	return false
}
