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
