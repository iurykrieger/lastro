package template

import (
	"fmt"
	"strings"
)

// Refs is the set of references a compiled string depends on. The executor
// uses it to build the env map and to know which fixtures/step-outputs to bind.
type Refs struct {
	Inputs      []string        // input names, first-seen order, deduped
	Fixtures    []string        // fixture ids, first-seen order, deduped
	StepOutputs []StepOutputRef // step output refs, first-seen order
	EntryPoints []EntryPointRef // entry point refs, first-seen order
	Env         []string        // env var names, first-seen order, deduped
}

// Compile rewrites every ref segment into a POSIX shell variable reference
// (always double-quoted) and returns the rewritten string plus the collected
// Refs. Values are NEVER inlined — this is the shell-injection safety boundary.
// Env var names: HARNESS_INPUT_<U>, HARNESS_FIXTURE_<U>, HARNESS_STEPOUT_<STEP>_<U>,
// HARNESS_ENTRYPOINT_<U> where <U> = upper(name) with '-' -> '_'.
// Env refs compile to a bare "${NAME}" shell lookup (the name is already
// UPPER_SNAKE_CASE per the grammar).
func Compile(segs []Segment) (string, Refs, error) {
	var b strings.Builder
	var refs Refs
	seen := map[string]bool{}
	for _, s := range segs {
		switch v := s.(type) {
		case Literal:
			b.WriteString(v.Text)
		case InputRef:
			b.WriteString(`"${` + envName("HARNESS_INPUT_", v.Name) + `}"`)
			if !seen["i:"+v.Name] {
				seen["i:"+v.Name] = true
				refs.Inputs = append(refs.Inputs, v.Name)
			}
		case FixtureRef:
			if len(v.JSONPath) > 0 {
				return "", Refs{}, &ResolveError{Pos: v.Pos, Msg: "fixture jsonpath drilling is not supported in sensor steps; bind the whole fixture"}
			}
			b.WriteString(`"${` + envName("HARNESS_FIXTURE_", v.ID) + `}"`)
			if !seen["f:"+v.ID] {
				seen["f:"+v.ID] = true
				refs.Fixtures = append(refs.Fixtures, v.ID)
			}
		case StepOutputRef:
			b.WriteString(`"${` + "HARNESS_STEPOUT_" + upperEnv(v.StepID) + "_" + upperEnv(v.Name) + `}"`)
			refs.StepOutputs = append(refs.StepOutputs, v)
		case EnvRef:
			b.WriteString(`"${` + v.Name + `}"`)
			if !seen["e:"+v.Name] {
				seen["e:"+v.Name] = true
				refs.Env = append(refs.Env, v.Name)
			}
		case EntryPointRef:
			return "", Refs{}, &ResolveError{Pos: v.Pos, Msg: "entry_points.* is not supported in sensor steps yet"}
		default:
			return "", Refs{}, fmt.Errorf("compile: unknown segment %T", s)
		}
	}
	return b.String(), refs, nil
}

func envName(prefix, id string) string { return prefix + upperEnv(id) }
func upperEnv(s string) string         { return strings.ToUpper(strings.ReplaceAll(s, "-", "_")) }
