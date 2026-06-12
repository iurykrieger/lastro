package sensor

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/schemas"
)

// CoreInputBaseline is the baseline input floor for one parameterized core
// primitive, loaded from the embedded schemas/core-inputs/ directory. A floor
// is keyed by angle (file <angle>.yaml, applies to every core sensor of that
// angle) or — when Primitive is set — by primitive id (file <primitive>.yaml,
// applies only to the core sensor with that id). Primitive-keyed floors let a
// parameterized primitive live under an angle whose other primitives are
// floorless (e.g. provision-auth under environment, next to run-dev).
// The floor is a minimum, not a ceiling: a core primitive must declare at
// least these inputs (each with a default) and may declare more.
type CoreInputBaseline struct {
	SchemaVersion string                       `json:"schema_version"`
	Angle         enums.ValidationAngle        `json:"angle"`
	Primitive     string                       `json:"primitive,omitempty"`
	// EnvGuidance tells the generating skill which ambient vars this
	// primitive's recipes typically read; the generated sensor declares
	// the concrete names in its top-level env: block.
	EnvGuidance string                       `json:"env_guidance,omitempty"`
	Inputs      map[string]BaselineInputSpec `json:"inputs"`
}

// key returns the map key (and filename stem) a baseline binds to: the
// primitive id when set, the angle otherwise.
func (b CoreInputBaseline) key() string {
	if b.Primitive != "" {
		return b.Primitive
	}
	return string(b.Angle)
}

// BaselineInputSpec describes one baseline input. SuggestedDefault is the
// floor hint embedded in the schema file — distinct from InputSpec.Default,
// the actual default a generated sensor declares. The generating skill may
// override the hint with a manifest-derived value (e.g. base_url from the
// detected dev-server port).
type BaselineInputSpec struct {
	Description      string `json:"description"`
	SuggestedDefault string `json:"suggested_default,omitempty"`
}

var (
	baselineOnce sync.Once
	baselineMap  map[string]CoreInputBaseline
	baselineErr  error
)

// LoadBaselines parses every embedded schemas/core-inputs/*.yaml into a
// map keyed by primitive id (when the floor declares `primitive:`) or by
// angle. Parsed once; subsequent calls reuse the cached result. Each
// caller receives its own shallow copy of the map.
func LoadBaselines() (map[string]CoreInputBaseline, error) {
	baselineOnce.Do(func() {
		entries, err := schemas.FS.ReadDir("core-inputs")
		if err != nil {
			baselineErr = fmt.Errorf("read embedded core-inputs: %w", err)
			return
		}
		m := make(map[string]CoreInputBaseline, len(entries))
		for _, e := range entries {
			raw, err := schemas.FS.ReadFile("core-inputs/" + e.Name())
			if err != nil {
				baselineErr = fmt.Errorf("read core-inputs/%s: %w", e.Name(), err)
				return
			}
			var bl CoreInputBaseline
			if err := yaml.Unmarshal(raw, &bl); err != nil {
				baselineErr = fmt.Errorf("parse core-inputs/%s: %w", e.Name(), err)
				return
			}
			if len(bl.Inputs) == 0 {
				baselineErr = fmt.Errorf("core-inputs/%s: declares no inputs", e.Name())
				return
			}
			if want := bl.key() + ".yaml"; want != e.Name() {
				baselineErr = fmt.Errorf("core-inputs/%s: key %q does not match filename (want %s)",
					e.Name(), bl.key(), want)
				return
			}
			for name, spec := range bl.Inputs {
				if strings.TrimSpace(spec.Description) == "" {
					baselineErr = fmt.Errorf("core-inputs/%s: input %q has empty description", e.Name(), name)
					return
				}
			}
			m[bl.key()] = bl
		}
		baselineMap = m
	})
	if baselineErr != nil {
		return nil, baselineErr
	}
	// Shallow copy so no caller can corrupt the cached singleton; the
	// values are structs, so copying the entries is sufficient.
	out := make(map[string]CoreInputBaseline, len(baselineMap))
	for k, v := range baselineMap {
		out[k] = v
	}
	return out, nil
}

// FloorFor returns the baseline floor that governs core sensor s: the
// primitive-keyed floor matching s.ID when one exists, else the angle
// floor. ok=false means s has no floor and passes trivially.
func FloorFor(s Sensor, baselines map[string]CoreInputBaseline) (CoreInputBaseline, bool) {
	if bl, ok := baselines[s.ID]; ok && bl.Primitive == s.ID {
		return bl, true
	}
	bl, ok := baselines[string(s.Angle)]
	return bl, ok
}

// ValidateBaselineInputs enforces the input floor on core primitives: a
// scope=core sensor governed by a floor (primitive-keyed by id, else
// angle-keyed) must declare every baseline input, each carrying a default
// (the self-run smoke-test invariant). Non-core sensors and sensors
// without a floor pass trivially. The floor is a minimum — extra declared
// inputs are fine.
func ValidateBaselineInputs(s Sensor, baselines map[string]CoreInputBaseline) error {
	if s.Scope != enums.ScopeCore {
		return nil
	}
	bl, ok := FloorFor(s, baselines)
	if !ok {
		return nil
	}
	var missing, undefaulted []string
	for name := range bl.Inputs {
		spec, declared := s.Inputs[name]
		if !declared {
			missing = append(missing, name)
			continue
		}
		if !spec.HasDefault {
			undefaulted = append(undefaulted, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(undefaulted)
	var errs []error
	if len(missing) > 0 {
		errs = append(errs, fmt.Errorf(
			"baseline input(s) not declared: %v — see schemas/core-inputs/%s.yaml; the floor is a minimum, declare them all with defaults",
			missing, bl.key()))
	}
	if len(undefaulted) > 0 {
		errs = append(errs, fmt.Errorf(
			"baseline input(s) missing a default: %v — every core input needs a default so the primitive self-runs",
			undefaulted))
	}
	return errors.Join(errs...)
}

// ValidateInputReferences enforces faithfulness on core primitives: every
// declared input must be referenced as ${{ inputs.<name> }} in at least one
// step's run script or with value. A declared-but-ignored input is a
// contract lie — consumers would bind it and silently change nothing.
func ValidateInputReferences(s Sensor) error {
	if s.Scope != enums.ScopeCore || len(s.Inputs) == 0 {
		return nil
	}
	var blob strings.Builder
	for _, st := range s.Steps {
		blob.WriteString(st.Run)
		blob.WriteByte('\n')
		for _, v := range st.With {
			blob.WriteString(v)
			blob.WriteByte('\n')
		}
		for _, v := range st.Env {
			blob.WriteString(v)
			blob.WriteByte('\n')
		}
	}
	// outputs.from references ${{ steps.<id>.outputs.<key> }} by schema
	// convention, never ${{ inputs.<name> }}, so it is intentionally
	// excluded from this scan.
	text := blob.String()
	var unreferenced []string
	for name := range s.Inputs {
		re := regexp.MustCompile(`\$\{\{\s*inputs\.` + regexp.QuoteMeta(name) + `\s*\}\}`)
		if !re.MatchString(text) {
			unreferenced = append(unreferenced, name)
		}
	}
	sort.Strings(unreferenced)
	if len(unreferenced) > 0 {
		return fmt.Errorf(
			"declared input(s) never referenced as ${{ inputs.<name> }} in any step: %v — bind them in a run script or remove them",
			unreferenced)
	}
	return nil
}
