package policy

import (
	"slices"

	"github.com/iurykrieger/lastro/internal/enums"
)

// AnglesFor returns the obligatory and optional angles for the given
// archetype, sorted alphabetically by underlying string for deterministic
// output. Disabled and unset angles are both excluded — sensor generation
// treats them identically (no sensor generated). The returned slices are
// always non-nil; an unconfigured archetype yields two empty slices.
func (p *EffectivePolicy) AnglesFor(a enums.Archetype) (obligatory, optional []enums.ValidationAngle) {
	obligatory = []enums.ValidationAngle{}
	optional = []enums.ValidationAngle{}
	if p == nil {
		return
	}
	block, ok := p.PerArchetype[a]
	if !ok {
		return
	}
	for angle, status := range block {
		switch status {
		case StatusObligatory:
			obligatory = append(obligatory, angle)
		case StatusOptional:
			optional = append(optional, angle)
		}
	}
	slices.Sort(obligatory)
	slices.Sort(optional)
	return
}

// Status returns the AngleStatus configured for the given (archetype,
// angle) pair, or "" (the unset sentinel) if no scope mentioned it.
// Lets callers distinguish "disabled by policy" from "no policy coverage".
func (p *EffectivePolicy) Status(a enums.Archetype, angle enums.ValidationAngle) AngleStatus {
	if p == nil {
		return ""
	}
	block, ok := p.PerArchetype[a]
	if !ok {
		return ""
	}
	return block[angle]
}
