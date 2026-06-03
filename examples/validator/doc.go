// Package validator is the shared B7 primitive driving the framework's
// /validate-use-case skill against a target directory and aggregating
// the per-use-case verdicts into a Report.
//
// Both B7 test tracks consume this package:
//   - Track 1 (-tags=integration) drives ValidateAll against samples
//     under examples/<sample>/.
//   - Track 2 (-tags=dogfood) drives ValidateAll against the framework
//     repo root.
//
// The package never invokes the LLM-driven detection skills. It only
// shells out to /validate-use-case (and /heal in the heal test).
package validator

// ReportSchemaVersion is the schema_version written into report.json.
// Bump on any breaking change to the Report shape.
const ReportSchemaVersion = 1
