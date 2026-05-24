package policy

import "github.com/iurykrieger/lastro/internal/enums"

// policySource pairs a *ValidationPolicy with its scope name for
// ResolvedFrom tracking. Unexported; an implementation detail of Resolve.
type policySource struct {
	name string
	pol  *ValidationPolicy
}

// Resolve merges a global default and a local override into a single
// EffectivePolicy. Override granularity is per (archetype, angle): local
// wins for any angle it mentions; angles local omits inherit from global.
// Either argument may be nil. Both nil yields an empty *EffectivePolicy
// with ResolvedFrom = [].
//
// The resolver iterates only enums.ApplicableAngles[archetype]; any angle
// outside that matrix is ignored even if a source carries it. The loader
// already rejects inapplicable pairs at load time.
func Resolve(global, local *ValidationPolicy) *EffectivePolicy {
	var sources []policySource
	if global != nil {
		sources = append(sources, policySource{"global", global})
	}
	if local != nil {
		sources = append(sources, policySource{"local", local})
	}

	resolvedFrom := make([]string, 0, len(sources))
	for _, s := range sources {
		resolvedFrom = append(resolvedFrom, s.name)
	}

	eff := &EffectivePolicy{
		SchemaVersion: SupportedSchemaVersion,
		ResolvedFrom:  resolvedFrom,
		PerArchetype:  map[enums.Archetype]map[enums.ValidationAngle]AngleStatus{},
	}

	archetypes := unionArchetypes(sources)
	for _, arch := range archetypes {
		for _, angle := range enums.ApplicableAngles[arch] {
			status := AngleStatus("")
			for _, s := range sources {
				block, ok := s.pol.PerArchetype[arch]
				if !ok {
					continue
				}
				switch {
				case containsAngle(block.Obligatory, angle):
					status = StatusObligatory
				case containsAngle(block.Optional, angle):
					status = StatusOptional
				case containsAngle(block.Disabled, angle):
					status = StatusDisabled
				}
			}
			if status == "" {
				continue
			}
			if eff.PerArchetype[arch] == nil {
				eff.PerArchetype[arch] = map[enums.ValidationAngle]AngleStatus{}
			}
			eff.PerArchetype[arch][angle] = status
		}
	}
	return eff
}

// unionArchetypes returns the union of archetype keys across all sources,
// emitted in enums.AllArchetypes() order. The fixed-order traversal gives
// deterministic output regardless of map iteration order.
func unionArchetypes(sources []policySource) []enums.Archetype {
	seen := map[enums.Archetype]struct{}{}
	for _, s := range sources {
		for arch := range s.pol.PerArchetype {
			seen[arch] = struct{}{}
		}
	}
	out := make([]enums.Archetype, 0, len(seen))
	for _, arch := range enums.AllArchetypes() {
		if _, ok := seen[arch]; ok {
			out = append(out, arch)
		}
	}
	return out
}

func containsAngle(list []enums.ValidationAngle, target enums.ValidationAngle) bool {
	for _, a := range list {
		if a == target {
			return true
		}
	}
	return false
}
