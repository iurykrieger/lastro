package enums

// ApplicableAngles is the canonical archetype × angle matrix. Sourced from
// schemas/enums/archetypes.yaml. A sensor's angle must be in the list for
// its use case's archetype.
var ApplicableAngles = map[Archetype][]ValidationAngle{
	ArchetypeHTTPAPI: {
		AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
		AngleE2ETest, AngleContracts, AngleLogs, AngleMetrics,
		AngleDatabase, AnglePerformance,
	},
	ArchetypeEventConsumer: {
		AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
		AngleE2ETest, AngleContracts, AngleLogs, AngleMetrics, AngleDatabase,
	},
	ArchetypeEventProducer: {
		AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
		AngleContracts, AngleLogs, AngleMetrics,
	},
	ArchetypeCLI: {
		AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
		AngleContracts, AngleLogs,
	},
	ArchetypeSDK: {
		AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
		AngleContracts,
	},
	ArchetypeLibrary: {
		AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
		AngleContracts,
	},
	ArchetypeWorker: {
		AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
		AngleLogs, AngleMetrics, AngleDatabase,
	},
	ArchetypeBatchJob: {
		AngleSecurity, AngleBuild, AngleCodeStructure, AngleUnitTest,
		AngleLogs, AngleMetrics, AngleDatabase, AnglePerformance,
	},
	ArchetypeStaticSite: {
		AngleSecurity, AngleBuild, AngleCodeStructure,
		AngleContracts, AnglePerformance,
	},
}

// Applies reports whether the given angle is applicable to the given archetype.
func Applies(a Archetype, v ValidationAngle) bool {
	for _, applicable := range ApplicableAngles[a] {
		if applicable == v {
			return true
		}
	}
	return false
}
