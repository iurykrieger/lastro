package template

import "strings"

// RenderLabels walks segments and produces the inert human-readable
// representation. Literals pass through; refs are bracketed with a
// type-prefix and the ref's full dotted form. The format is locked here:
//
//	{{fixtures.fx-a}}            → [fixture: fx-a]
//	{{fixtures.fx-a.u.n}}        → [fixture: fx-a.u.n]
//	{{entry_points.ep-c}}        → [entry: ep-c]
//	{{entry_points.ep-c.spec.k}} → [entry: ep-c.spec.k]
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
		}
	}
	return b.String()
}
