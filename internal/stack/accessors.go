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
