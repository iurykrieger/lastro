package sensor

import (
	"fmt"
	"sort"
	"strings"
)

// ErrMissingDependency is returned when a sensor's DependsOn list
// references an id not present in the slice handed to
// ResolveExecutionOrder. Phase B's heal loop uses errors.As against
// this type to decide whether to widen the affected-sensors slice
// and retry.
type ErrMissingDependency struct {
	Sensor     string   // id of the sensor that owns the dangling edge
	MissingIDs []string // referenced ids not present in the input slice
}

func (e *ErrMissingDependency) Error() string {
	return fmt.Sprintf("sensor %q depends_on references unknown id(s): %s",
		e.Sensor, strings.Join(e.MissingIDs, ", "))
}

// ResolveExecutionOrder returns sensors in a deterministic topological
// order honoring DependsOn edges. Empty input returns an empty slice.
// Unknown DependsOn ids yield *ErrMissingDependency; the next task
// adds Kahn's algorithm and cycle detection.
func ResolveExecutionOrder(sensors []Sensor) ([]Sensor, error) {
	if len(sensors) == 0 {
		return []Sensor{}, nil
	}

	// Detect dangling edges up front so callers get a clean error
	// class even when the dangling reference would otherwise be part
	// of a would-be cycle.
	known := make(map[string]bool, len(sensors))
	for _, s := range sensors {
		known[s.ID] = true
	}
	for _, s := range sensors {
		var missing []string
		for _, dep := range s.DependsOn {
			if !known[dep] {
				missing = append(missing, dep)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return nil, &ErrMissingDependency{Sensor: s.ID, MissingIDs: missing}
		}
	}

	// Trivial case: single sensor, no edges. Kahn's lands in Task 17.
	if len(sensors) == 1 {
		return []Sensor{sensors[0]}, nil
	}

	// Placeholder until Task 17: pass through, sorted by id for
	// deterministic output. Tests in Task 17 will fail if this is left.
	out := make([]Sensor, len(sensors))
	copy(out, sensors)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
