package sensor

import (
	"sort"

	"github.com/iurykrieger/lastro/internal/enums"
)

// GatherForUseCase returns the sensors that participate in validating
// useCaseID: every sensor owned by the use case, plus the transitive
// depends_on closure restricted to scope: core — following use-case→core
// and core→core edges, never use-case→use-case edges (spec §8).
//
// This is the load-bearing gather that lets a use-case sensor's
// depends_on into a shared core root (e.g. run-dev) resolve at scheduling
// time: without pulling the core sensor in, ResolveExecutionOrder's
// dangling-edge check would fail. depends_on ids that resolve to no
// stored sensor are skipped here and surface as dangling edges in the
// resolver instead.
//
// Result is sorted by id ascending with no duplicates.
func (s *Store) GatherForUseCase(useCaseID string) []Sensor {
	roots := s.ForUseCase(useCaseID)
	included := make(map[string]Sensor, len(roots))
	queue := make([]Sensor, 0, len(roots))
	for _, r := range roots {
		included[r.ID] = r
		queue = append(queue, r)
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, dep := range cur.DependsOn {
			if _, seen := included[dep]; seen {
				continue
			}
			d, ok := s.LookupSensor(dep)
			// Expand only into core sensors: a dangling id (!ok) or a
			// use-case→use-case edge is not part of the closure.
			if !ok || d.Scope != enums.ScopeCore {
				continue
			}
			included[d.ID] = d
			queue = append(queue, d)
		}
	}

	out := make([]Sensor, 0, len(included))
	for _, sn := range included {
		out = append(out, sn)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
