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

// ErrCycle is returned when DependsOn edges among the input sensors
// form a cycle. InvolvedIDs lists every sensor still with non-zero
// in-degree after Kahn's terminates — exactly the members of the
// cycle's strongly-connected component(s).
type ErrCycle struct {
	InvolvedIDs []string
}

func (e *ErrCycle) Error() string {
	return fmt.Sprintf("resolver: cycle involving sensor(s): %s",
		strings.Join(e.InvolvedIDs, ", "))
}

// ResolveExecutionOrder returns sensors in a deterministic topological
// order honoring DependsOn edges. Empty input returns an empty slice.
// Unknown DependsOn ids yield *ErrMissingDependency; the next task
// adds Kahn's algorithm and cycle detection.
func ResolveExecutionOrder(sensors []Sensor) ([]Sensor, error) {
	if len(sensors) == 0 {
		return []Sensor{}, nil
	}

	// Dangling-edge pre-check (unchanged).
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

	// Kahn's algorithm with deterministic id-sort tiebreak.
	byID := make(map[string]Sensor, len(sensors))
	inDegree := make(map[string]int, len(sensors))
	adj := make(map[string][]string, len(sensors))
	for _, s := range sensors {
		byID[s.ID] = s
		if _, ok := inDegree[s.ID]; !ok {
			inDegree[s.ID] = 0
		}
	}
	for _, s := range sensors {
		for _, dep := range s.DependsOn {
			adj[dep] = append(adj[dep], s.ID)
			inDegree[s.ID]++
		}
	}

	// Initial queue: all zero-in-degree ids, sorted ascending.
	queue := make([]string, 0)
	for id, d := range inDegree {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)

	result := make([]Sensor, 0, len(sensors))
	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]
		result = append(result, byID[head])
		// Decrement dependents; collect newly-zero ids for sorted re-insertion.
		var ready []string
		for _, dep := range adj[head] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				ready = append(ready, dep)
			}
		}
		if len(ready) > 0 {
			queue = mergeSorted(queue, ready)
		}
	}

	if len(result) != len(sensors) {
		var stuck []string
		for id, d := range inDegree {
			if d > 0 {
				stuck = append(stuck, id)
			}
		}
		sort.Strings(stuck)
		return nil, &ErrCycle{InvolvedIDs: stuck}
	}
	return result, nil
}

// mergeSorted inserts each newcomer into the already-sorted queue,
// keeping the queue sorted ascending. Allocates a fresh slice.
func mergeSorted(queue, newcomers []string) []string {
	sort.Strings(newcomers)
	merged := make([]string, 0, len(queue)+len(newcomers))
	i, j := 0, 0
	for i < len(queue) && j < len(newcomers) {
		if queue[i] <= newcomers[j] {
			merged = append(merged, queue[i])
			i++
		} else {
			merged = append(merged, newcomers[j])
			j++
		}
	}
	merged = append(merged, queue[i:]...)
	merged = append(merged, newcomers[j:]...)
	return merged
}
