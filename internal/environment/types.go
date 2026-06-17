// internal/environment/types.go

// Package environment models the operational dependency graph of a project —
// every piece of software the application needs to execute — and the
// deterministic parsers + validators that produce it. It mirrors the
// internal/stack package: deterministic parse/validate/persist; the LLM
// (the /detect-environment skill) only classifies raw facts into the model.
package environment

// DependencyType labels a backing service. It drives the SHAPE of the core
// sensor generation emits (readiness probe + observational vs single-shot),
// never a duplicated command.
type DependencyType string

const (
	DependencyDatastore DependencyType = "datastore"
	DependencyCache     DependencyType = "cache"
	DependencyBroker    DependencyType = "broker"
)

// ProvidedBy is a grounded pointer to where a node's launch command lives. It
// is NOT the resolved command: generation reads the command from this {file,
// path} at generation time, so the command string exists in exactly one
// authored place (package.json / docker-compose.yml).
type ProvidedBy struct {
	File string `json:"file" yaml:"file"`
	Path string `json:"path" yaml:"path"`
}

// Application is the system under test. It has no `type` (it is implicitly the
// run-dev service). depends_on names backing dependencies and setup nodes.
type Application struct {
	ProvidedBy ProvidedBy `json:"provided_by" yaml:"provided_by"`
	DependsOn  []string   `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
}

// Dependency is a long-running backing service (datastore/cache/broker).
type Dependency struct {
	Type       DependencyType `json:"type" yaml:"type"`
	ProvidedBy ProvidedBy     `json:"provided_by" yaml:"provided_by"`
	DependsOn  []string       `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
}

// SetupNode is a run-to-completion task (migrate/seed). Success = exit 0.
type SetupNode struct {
	ID         string     `json:"id" yaml:"id"`
	Type       string     `json:"type" yaml:"type"` // always "setup"
	ProvidedBy ProvidedBy `json:"provided_by" yaml:"provided_by"`
	DependsOn  []string   `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Optional   bool       `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// EnvironmentModel is the full dependency graph persisted to
// .harness/environment-model.yaml.
type EnvironmentModel struct {
	SchemaVersion string                `json:"schema_version" yaml:"schema_version"`
	Application   Application           `json:"application" yaml:"application"`
	Dependencies  map[string]Dependency `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Setup         []SetupNode           `json:"setup,omitempty" yaml:"setup,omitempty"`
}

// NodeNames returns every depend-able node name (dependency keys + setup ids).
// The application is excluded — nothing depends on the app.
func (m EnvironmentModel) NodeNames() []string {
	names := make([]string, 0, len(m.Dependencies)+len(m.Setup))
	for k := range m.Dependencies {
		names = append(names, k)
	}
	for _, s := range m.Setup {
		names = append(names, s.ID)
	}
	return names
}
