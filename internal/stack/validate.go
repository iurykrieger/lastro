package stack

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/iurykrieger/lastro/internal/enums"
)

// supportedMajorMinor is the "major.minor" prefix this validator accepts.
// Persist patch-bumps schema_version on every re-emit, so validators must
// tolerate any patch within the supported major.minor.
const supportedMajorMinor = "1.0"

func schemaVersionCompatible(v string) bool {
	return strings.HasPrefix(v, supportedMajorMinor+".")
}

// idPattern mirrors $defs.Id in schemas/stack-component.yaml.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Validate checks programmatic invariants that JSON Schema can't fully
// express. Returns an aggregated error naming every problem; returns nil
// when the component is valid.
func (c StackComponent) Validate() error {
	var problems []string

	if !schemaVersionCompatible(c.SchemaVersion) {
		problems = append(problems,
			fmt.Sprintf("schema_version: got %q, want major.minor.patch matching %s.x", c.SchemaVersion, supportedMajorMinor))
	}
	if c.ID == "" {
		problems = append(problems, "id: required")
	} else if !idPattern.MatchString(c.ID) {
		problems = append(problems, fmt.Sprintf("id: %q does not match %s", c.ID, idPattern))
	} else if len(c.ID) > 128 {
		problems = append(problems, fmt.Sprintf("id: %q exceeds 128 chars", c.ID))
	}
	if c.Kind == "" {
		problems = append(problems, "kind: required")
	} else if !enums.IsValidStackKind(string(c.Kind)) {
		problems = append(problems, fmt.Sprintf("kind: %q is not a recognized StackKind", c.Kind))
	}
	if c.Name == "" {
		problems = append(problems, "name: required")
	}
	if c.Version == "" {
		problems = append(problems, "version: required")
	}
	if len(c.Capabilities) == 0 {
		problems = append(problems, "capabilities: at least one required")
	}
	for i, cap := range c.Capabilities {
		if cap == "" {
			problems = append(problems, fmt.Sprintf("capabilities[%d]: empty string", i))
		}
	}
	if len(c.DetectionEvidence) == 0 {
		problems = append(problems, "detection_evidence: at least one required")
	}
	for i, ev := range c.DetectionEvidence {
		if ev.File == "" {
			problems = append(problems, fmt.Sprintf("detection_evidence[%d].file: required", i))
		}
		if ev.Path == "" {
			problems = append(problems, fmt.Sprintf("detection_evidence[%d].path: required", i))
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return errors.New("StackComponent invalid: " + strings.Join(problems, "; "))
}

// Validate checks manifest-level invariants and every component's
// validity. Errors are aggregated; component errors are prefixed with
// the component's id when present, otherwise its list index. Duplicate
// id detection is NOT here — the loader does that after Validate.
func (m StackManifest) Validate() error {
	var problems []string

	if !schemaVersionCompatible(m.SchemaVersion) {
		problems = append(problems,
			fmt.Sprintf("schema_version: got %q, want major.minor.patch matching %s.x", m.SchemaVersion, supportedMajorMinor))
	}
	if m.Archetype == "" {
		problems = append(problems, "archetype: required")
	} else if !enums.IsValidArchetype(string(m.Archetype)) {
		problems = append(problems, fmt.Sprintf("archetype: %q is not a recognized Archetype", m.Archetype))
	}
	if len(m.ApplicableAngles) == 0 {
		problems = append(problems, "applicable_angles: at least one required")
	} else if m.Archetype != "" && enums.IsValidArchetype(string(m.Archetype)) {
		// applicable_angles must match the canonical archetype × angle matrix
		// in internal/enums. Persist is the only legitimate writer of this
		// field, so any mismatch is a programmer/loader bug, not user input.
		want := enums.ApplicableAngles[m.Archetype]
		if !angleSetEqual(m.ApplicableAngles, want) {
			problems = append(problems,
				fmt.Sprintf("applicable_angles: got %v, want %v (canonical list for archetype %q)",
					m.ApplicableAngles, want, m.Archetype))
		}
	}
	if len(m.Components) == 0 {
		problems = append(problems, "components: at least one required")
	}

	if m.EnvFile != "" {
		if filepath.IsAbs(m.EnvFile) {
			problems = append(problems,
				fmt.Sprintf("env_file: must be a relative path within the project root (got %q)", m.EnvFile))
		} else {
			// Reject any path whose cleaned form escapes the project root via
			// ".." traversal. filepath.Clean normalizes "config/../../x" to
			// "../x", so a clean result starting with ".." means escape.
			clean := filepath.Clean(m.EnvFile)
			if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				problems = append(problems,
					fmt.Sprintf("env_file: must be a relative path within the project root (got %q)", m.EnvFile))
			}
		}
	}

	for i, c := range m.Components {
		if err := c.Validate(); err != nil {
			prefix := fmt.Sprintf("components[%d]", i)
			if c.ID != "" {
				prefix = fmt.Sprintf("components[%s]", c.ID)
			}
			problems = append(problems, prefix+": "+err.Error())
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return errors.New("StackManifest invalid: " + strings.Join(problems, "; "))
}

// angleSetEqual reports whether two slices of ValidationAngle contain the
// same elements (order-independent). Used to validate that applicable_angles
// matches the canonical archetype × angle matrix.
func angleSetEqual(a, b []enums.ValidationAngle) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[enums.ValidationAngle]bool, len(b))
	for _, v := range b {
		seen[v] = true
	}
	for _, v := range a {
		if !seen[v] {
			return false
		}
	}
	return true
}
