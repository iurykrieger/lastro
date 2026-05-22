// Package enums provides typed constants and validators for the framework's
// eight fixed enums, plus the canonical archetype × angle matrix.
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
