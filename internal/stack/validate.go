package stack

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/iurykrieger/lastro/internal/enums"
)

// idPattern mirrors $defs.Id in schemas/stack-component.yaml.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Validate checks programmatic invariants that JSON Schema can't fully
// express. Returns an aggregated error naming every problem; returns nil
// when the component is valid.
func (c StackComponent) Validate() error {
	var problems []string

	if c.SchemaVersion != SchemaVersion {
		problems = append(problems,
			fmt.Sprintf("schema_version: got %q, want %q", c.SchemaVersion, SchemaVersion))
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
