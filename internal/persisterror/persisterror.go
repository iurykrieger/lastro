// Package persisterror defines the structured error returned by every
// entity Persist function in this framework. Skill scripts type-assert
// these and JSON-encode them to stdout so the slash-command body's
// retry loop can read them.
package persisterror

import "fmt"

// Kind discriminates the failure mode so the retry prompt can switch on
// it. Every Persist returns one of these.
type Kind string

const (
	SchemaViolation      Kind = "schema_violation"
	FixtureBinding       Kind = "fixture_binding"
	Grounding            Kind = "grounding"
	TemplateResolution   Kind = "template_resolution"
	MissingRequiredField Kind = "missing_required_field"
	UnknownEnumValue     Kind = "unknown_enum_value"
	AngleNotApplicable   Kind = "angle_not_applicable"
	MissingDependency    Kind = "missing_dependency"
	StepResolvability    Kind = "step_resolvability"
)

// Error is the structured error type. Skill scripts marshal it to JSON.
// Path is YAML JSONPath (best effort; may be empty when the upstream
// validator doesn't surface a path).
type Error struct {
	Kind       Kind           `json:"kind"`
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id,omitempty"`
	File       string         `json:"file,omitempty"`
	Path       string         `json:"path,omitempty"`
	Value      any            `json:"value,omitempty"`
	Expected   string         `json:"expected,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	Message    string         `json:"message"`
}

func (e *Error) Error() string {
	if e.EntityID != "" {
		return fmt.Sprintf("%s on %s %q: %s", e.Kind, e.EntityType, e.EntityID, e.Message)
	}
	return fmt.Sprintf("%s on %s: %s", e.Kind, e.EntityType, e.Message)
}
