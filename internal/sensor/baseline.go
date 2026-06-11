package sensor

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/schemas"
)

// CoreInputBaseline is the baseline input floor for one parameterized
// core-sensor angle, loaded from the embedded schemas/core-inputs/<angle>.yaml.
// The floor is a minimum, not a ceiling: a core primitive must declare at
// least these inputs (each with a default) and may declare more.
type CoreInputBaseline struct {
	SchemaVersion string                       `json:"schema_version"`
	Angle         enums.ValidationAngle        `json:"angle"`
	Inputs        map[string]BaselineInputSpec `json:"inputs"`
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
	baselineMap  map[enums.ValidationAngle]CoreInputBaseline
	baselineErr  error
)

// LoadBaselines parses every embedded schemas/core-inputs/*.yaml into a
// map keyed by angle. Parsed once; subsequent calls reuse the cached
// result. Each caller receives its own shallow copy of the map.
func LoadBaselines() (map[enums.ValidationAngle]CoreInputBaseline, error) {
	baselineOnce.Do(func() {
		entries, err := schemas.FS.ReadDir("core-inputs")
		if err != nil {
			baselineErr = fmt.Errorf("read embedded core-inputs: %w", err)
			return
		}
		m := make(map[enums.ValidationAngle]CoreInputBaseline, len(entries))
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
			if want := string(bl.Angle) + ".yaml"; want != e.Name() {
				baselineErr = fmt.Errorf("core-inputs/%s: angle %q does not match filename (want %s)",
					e.Name(), bl.Angle, want)
				return
			}
			for name, spec := range bl.Inputs {
				if strings.TrimSpace(spec.Description) == "" {
					baselineErr = fmt.Errorf("core-inputs/%s: input %q has empty description", e.Name(), name)
					return
				}
			}
			m[bl.Angle] = bl
		}
		baselineMap = m
	})
	if baselineErr != nil {
		return nil, baselineErr
	}
	// Shallow copy so no caller can corrupt the cached singleton; the
	// values are structs, so copying the entries is sufficient.
	out := make(map[enums.ValidationAngle]CoreInputBaseline, len(baselineMap))
	for k, v := range baselineMap {
		out[k] = v
	}
	return out, nil
}

// ValidateBaselineInputs enforces the per-angle input floor on core
// primitives: a scope=core sensor whose angle has a baseline must declare
// every baseline input, each carrying a default (the self-run smoke-test
// invariant). Non-core sensors and angles without a baseline pass
// trivially. The floor is a minimum — extra declared inputs are fine.
func ValidateBaselineInputs(s Sensor, baselines map[enums.ValidationAngle]CoreInputBaseline) error {
	if s.Scope != enums.ScopeCore {
		return nil
	}
	bl, ok := baselines[s.Angle]
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
			"angle %q baseline input(s) not declared: %v — see schemas/core-inputs/%s.yaml; the floor is a minimum, declare them all with defaults",
			s.Angle, missing, s.Angle))
	}
	if len(undefaulted) > 0 {
		errs = append(errs, fmt.Errorf(
			"baseline input(s) missing a default: %v — every core input needs a default so the primitive self-runs",
			undefaulted))
	}
	return errors.Join(errs...)
}
