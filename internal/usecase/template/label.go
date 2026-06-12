package template

import "strings"

// RenderLabels walks segments and produces the inert human-readable
// representation. Literals pass through; refs are bracketed with a
// type-prefix and the ref's full dotted form. The format is locked here:
//
//	${{fixtures.fx-a}}                  → [fixture: fx-a]
//	${{fixtures.fx-a.u.n}}              → [fixture: fx-a.u.n]
//	${{entry_points.ep-c}}              → [entry: ep-c]
//	${{entry_points.ep-c.spec.k}}       → [entry: ep-c.spec.k]
//	${{inputs.base_url}}                → [input: base_url]
//	${{steps.create.outputs.id}}        → [step: create.outputs.id]
//	${{env.MY_TOKEN}}                   → [env: MY_TOKEN]
func RenderLabels(segs []Segment) string {
	var b strings.Builder
	for _, s := range segs {
		switch v := s.(type) {
		case Literal:
			b.WriteString(v.Text)
		case FixtureRef:
			b.WriteString("[fixture: ")
			b.WriteString(v.ID)
			for _, k := range v.JSONPath {
				b.WriteByte('.')
				b.WriteString(k)
			}
			b.WriteByte(']')
		case EntryPointRef:
			b.WriteString("[entry: ")
			b.WriteString(v.ID)
			if v.SpecKey != "" {
				b.WriteString(".spec.")
				b.WriteString(v.SpecKey)
			}
			b.WriteByte(']')
		case InputRef:
			b.WriteString("[input: ")
			b.WriteString(v.Name)
			b.WriteByte(']')
		case StepOutputRef:
			b.WriteString("[step: ")
			b.WriteString(v.StepID)
			b.WriteString(".outputs.")
			b.WriteString(v.Name)
			b.WriteByte(']')
		case EnvRef:
			b.WriteString("[env: ")
			b.WriteString(v.Name)
			b.WriteByte(']')
		}
	}
	return b.String()
}
