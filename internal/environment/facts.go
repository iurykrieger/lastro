// internal/environment/facts.go
package environment

import (
	"strings"
)

// ComposeHealthcheck mirrors a compose service healthcheck (captured verbatim
// when declared; generation reads it to derive readiness).
type ComposeHealthcheck struct {
	Test     []string `json:"test,omitempty" yaml:"test,omitempty"`
	Interval string   `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout  string   `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Retries  int      `json:"retries,omitempty" yaml:"retries,omitempty"`
}

// ComposeService is a parsed docker-compose service, verbatim.
type ComposeService struct {
	Image       string              `json:"image,omitempty" yaml:"image,omitempty"`
	Ports       []string            `json:"ports,omitempty" yaml:"ports,omitempty"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	DependsOn   []string            `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Healthcheck *ComposeHealthcheck `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
}

// RawFacts is the deterministic parser output: operational facts extracted
// verbatim from infra files, with no interpretation. The classifier (skill)
// turns these into an EnvironmentModel; ValidateGrounding cross-checks every
// provided_by against them.
type RawFacts struct {
	Scripts          map[string]string         `json:"scripts,omitempty" yaml:"scripts,omitempty"`
	MakeTargets      map[string]string         `json:"make_targets,omitempty" yaml:"make_targets,omitempty"`
	ProcfileEntries  map[string]string         `json:"procfile_entries,omitempty" yaml:"procfile_entries,omitempty"`
	ComposeServices  map[string]ComposeService `json:"compose_services,omitempty" yaml:"compose_services,omitempty"`
	ComposeFile      string                    `json:"compose_file,omitempty" yaml:"compose_file,omitempty"`
	EnvKeys          []string                  `json:"env_keys,omitempty" yaml:"env_keys,omitempty"`
	// RequiredEnvHints is reserved for config-throw detection (e.g. drizzle's "if (!process.env.DATABASE_URL) throw"). Not yet populated by Parse — deferred; env keys currently come from parseDotenvKeys. TODO(#52 follow-up).
	RequiredEnvHints []string `json:"required_env_hints,omitempty" yaml:"required_env_hints,omitempty"`
}

// Resolve returns the launch command a provided_by pointer grounds to, and
// whether it resolved. Compose services resolve to a deterministic
// `docker compose up -d <svc>` (the actual image/ports live in the compose
// file, which that command invokes — never duplicated here).
func (f RawFacts) Resolve(p ProvidedBy) (string, bool) {
	switch {
	case isPackageJSON(p.File) && strings.HasPrefix(p.Path, "scripts."):
		name := strings.TrimPrefix(p.Path, "scripts.")
		cmd, ok := f.Scripts[name]
		if !ok {
			return "", false
		}
		return cmd, true
	case isComposeFile(p.File) && strings.HasPrefix(p.Path, "services."):
		name := strings.TrimPrefix(p.Path, "services.")
		if _, ok := f.ComposeServices[name]; !ok {
			return "", false
		}
		return "docker compose up -d " + name, true
	case p.File == "Makefile":
		cmd, ok := f.MakeTargets[p.Path]
		if !ok {
			return "", false
		}
		return cmd, true
	case p.File == "Procfile":
		cmd, ok := f.ProcfileEntries[p.Path]
		if !ok {
			return "", false
		}
		return cmd, true
	}
	return "", false
}

func isPackageJSON(file string) bool { return file == "package.json" }

func isComposeFile(file string) bool {
	switch file {
	case "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
		return true
	}
	return false
}
