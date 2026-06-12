package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// resolveStepEnv resolves a step's declared env: map into concrete
// NAME=value pairs at spawn time. Values support the full ${{ }} surface:
// literals pass through verbatim; env.* resolves from the ambient view
// (unset-or-empty names are collected into missing — never injected as
// empty strings); inputs.* / steps.* resolve from the compiled env maps;
// fixtures.* resolves to the bound payload path. refDerived marks names
// whose value contains at least one resolved ref — secrets-by-construction
// that must be registered for redaction (inline literals are repo-visible
// and exempt). Structural problems (unbound input/fixture, entry_points)
// are errors, not missing-env.
func resolveStepEnv(
	stepEnv map[string]string,
	view envView,
	inputEnv, stepOutEnv map[string]string,
	fixturePaths map[string]string,
) (resolved map[string]string, refDerived map[string]bool, missing []string, err error) {
	resolved = map[string]string{}
	refDerived = map[string]bool{}
	for name, raw := range stepEnv {
		segs, perr := template.Parse(raw)
		if perr != nil {
			return nil, nil, nil, fmt.Errorf("env %q: %w", name, perr)
		}
		var b strings.Builder
		sawRef := false
		miss := false
		for _, seg := range segs {
			switch v := seg.(type) {
			case template.Literal:
				b.WriteString(v.Text)
			case template.EnvRef:
				val, ok := view.lookup(v.Name)
				if !ok || val == "" {
					missing = append(missing, v.Name)
					miss = true
					continue
				}
				b.WriteString(val)
				sawRef = true
			case template.InputRef:
				val, ok := inputEnv[inputEnvName(v.Name)]
				if !ok {
					return nil, nil, nil, fmt.Errorf("env %q: input %q is not bound in this context", name, v.Name)
				}
				b.WriteString(val)
				sawRef = true
			case template.StepOutputRef:
				b.WriteString(stepOutEnv[stepOutEnvName(v.StepID, v.Name)])
				sawRef = true
			case template.FixtureRef:
				if len(v.JSONPath) > 0 {
					return nil, nil, nil, fmt.Errorf("env %q: fixture jsonpath drilling is not supported", name)
				}
				p, ok := fixturePaths[v.ID]
				if !ok {
					return nil, nil, nil, fmt.Errorf("env %q: fixture %q was not bound", name, v.ID)
				}
				b.WriteString(p)
				sawRef = true
			case template.EntryPointRef:
				return nil, nil, nil, fmt.Errorf("env %q: entry_points.* is not valid in env values", name)
			default:
				return nil, nil, nil, fmt.Errorf("env %q: unsupported segment %T", name, seg)
			}
		}
		if miss {
			continue // never inject a partial value
		}
		resolved[name] = b.String()
		refDerived[name] = sawRef
	}
	sort.Strings(missing)
	return resolved, refDerived, missing, nil
}
