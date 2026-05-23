// Package sensor loads, validates, and orders the dynamically-generated
// Sensor units that validate (UseCase × ValidationAngle) pairs. A Sensor
// composes a top-level toolbox subset (StackComponent ids it draws from)
// and an ordered list of Steps, each of which may bind to fixtures via
// its own Uses field.
//
// The canonical schema lives at schemas/sensor.yaml and is consumed via
// the embedded schemas.FS. This package never edits sensors — generation
// is a Phase B concern (the /create-sensors skill).
package sensor

import "github.com/iurykrieger/lastro/internal/enums"

// Sensor is the in-memory representation of a loaded sensor YAML.
type Sensor struct {
	SchemaVersion string                 `json:"schema_version"`
	ID            string                 `json:"id"`
	UseCaseID     string                 `json:"use_case_id"`
	Angle         enums.ValidationAngle  `json:"angle"`
	Kind          enums.SensorKind       `json:"kind"`
	Nature        enums.SensorNature     `json:"nature"`
	OutputType    enums.SignalOutputType `json:"output_type"`
	Uses          []string               `json:"uses"`                 // StackComponent ids (grounding invariant 1)
	DependsOn     []string               `json:"depends_on,omitempty"` // Sensor ids (optional)
	Steps         []Step                 `json:"steps"`
}

// Step is one step of a Sensor's execution plan. Run is opaque here;
// the Phase B executor interprets it (shell command vs skill invocation).
type Step struct {
	ID   string   `json:"id"`
	Run  string   `json:"run"`
	Uses []string `json:"uses,omitempty"` // Fixture ids (grounding invariant 2)
}
