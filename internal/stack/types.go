// Package stack owns the StackComponent and StackManifest data types,
// their YAML+JSON-Schema loader, the programmatic validator, and the
// accessors used by sensor generation and runtime grounding.
package stack

import "github.com/iurykrieger/lastro/internal/enums"

// SchemaVersion is the contract version this package implements for both
// stack-component.yaml and stack-manifest.yaml. Loaders reject files that
// declare a different schema_version.
const SchemaVersion = "1.0.0"

// StackComponent is one entry in the detected stack manifest — a library,
// runtime, framework, datastore, protocol, or tool the repo uses.
type StackComponent struct {
	SchemaVersion     string          `json:"schema_version" yaml:"schema_version"`
	ID                string          `json:"id" yaml:"id"`
	Kind              enums.StackKind `json:"kind" yaml:"kind"`
	Name              string          `json:"name" yaml:"name"`
	Version           string          `json:"version" yaml:"version"`
	Capabilities      []string        `json:"capabilities" yaml:"capabilities"`
	DetectionEvidence []EvidenceRef   `json:"detection_evidence" yaml:"detection_evidence"`
}

// EvidenceRef points at the source artifact that proved the component is
// present in the repo. The optional Value carries the literal value at
// that path (e.g., a version range) when available.
type EvidenceRef struct {
	File  string `json:"file" yaml:"file"`
	Path  string `json:"path" yaml:"path"`
	Value string `json:"value,omitempty" yaml:"value,omitempty"`
}

// String renders evidence as the compact "file:path" form for logs and
// reports. The optional Value is intentionally omitted.
func (e EvidenceRef) String() string {
	return e.File + ":" + e.Path
}

// StackManifest is the full detected manifest for a repository: the
// archetype, applicable validation angles (derived from archetype), plus
// the ordered list of detected StackComponents.
type StackManifest struct {
	SchemaVersion    string                  `json:"schema_version" yaml:"schema_version"`
	Archetype        enums.Archetype         `json:"archetype" yaml:"archetype"`
	// EnvFile is the project-root-relative dotenv path the application
	// loads (optional). Recorded by /detect-stack; the runtime injects its
	// values into every step's process env, host environment winning.
	EnvFile          string                  `json:"env_file,omitempty" yaml:"env_file,omitempty"`
	ApplicableAngles []enums.ValidationAngle `json:"applicable_angles" yaml:"applicable_angles"`
	Components       []StackComponent        `json:"components" yaml:"components"`

	// byID is built by the loader and never marshalled. It backs ByID and
	// is the place duplicate-id detection lands.
	byID map[string]StackComponent `json:"-" yaml:"-"`
}
