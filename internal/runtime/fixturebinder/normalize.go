package fixturebinder

import "strings"

// normalizeEnvName converts a fixture id (regex ^[a-z][a-z0-9-]*$, enforced
// by the E5 schema) to the POSIX env-var name HARNESS_FIXTURE_<UPPER_UNDER>.
// Uppercase + hyphens-to-underscores; the input regex guarantees no other
// transformation is needed.
func normalizeEnvName(fixtureID string) string {
	return "HARNESS_FIXTURE_" + strings.ToUpper(strings.ReplaceAll(fixtureID, "-", "_"))
}
