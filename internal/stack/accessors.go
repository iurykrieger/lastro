package stack

// ByID returns the component matching the given id, plus a boolean
// indicating presence. The zero StackComponent is returned for unknown ids.
func (m StackManifest) ByID(id string) (StackComponent, bool) {
	c, ok := m.byID[id]
	return c, ok
}

// HasCapability reports whether any component in the manifest declares cap.
func (m StackManifest) HasCapability(cap string) bool {
	for _, c := range m.Components {
		for _, have := range c.Capabilities {
			if have == cap {
				return true
			}
		}
	}
	return false
}

// ComponentsWithCapability returns every component declaring cap, in
// manifest order. The returned slice is always non-nil (empty, not nil,
// when no matches) to avoid range-over-nil foot-guns at call sites.
func (m StackManifest) ComponentsWithCapability(cap string) []StackComponent {
	out := []StackComponent{}
	for _, c := range m.Components {
		for _, have := range c.Capabilities {
			if have == cap {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// LintWarning is emitted by LintCapabilities for each capability string not
// found in the supplied recognized-vocabulary list.
type LintWarning struct {
	ComponentID string
	Capability  string
	Message     string
}

// LintCapabilities returns one LintWarning per (component, capability) pair
// where the capability is not in known. Warnings only — never errors.
// Order: manifest order of components, then declared order of capabilities
// within each component.
func (m StackManifest) LintCapabilities(known []string) []LintWarning {
	set := make(map[string]struct{}, len(known))
	for _, k := range known {
		set[k] = struct{}{}
	}
	var out []LintWarning
	for _, c := range m.Components {
		for _, cap := range c.Capabilities {
			if _, ok := set[cap]; ok {
				continue
			}
			out = append(out, LintWarning{
				ComponentID: c.ID,
				Capability:  cap,
				Message:     "unrecognized capability " + cap + " on component " + c.ID,
			})
		}
	}
	return out
}
