package sensor

import (
	"errors"
	"sort"
	"testing"
)

func TestResolveExecutionOrder_EmptyInput(t *testing.T) {
	out, err := ResolveExecutionOrder(nil)
	if err != nil {
		t.Fatalf("empty input: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("empty input: got %d sensors, want 0", len(out))
	}
}

func TestResolveExecutionOrder_SingleSensor(t *testing.T) {
	s := Sensor{ID: "solo"}
	out, err := ResolveExecutionOrder([]Sensor{s})
	if err != nil {
		t.Fatalf("single sensor: %v", err)
	}
	if len(out) != 1 || out[0].ID != "solo" {
		t.Errorf("single sensor: got %v, want [solo]", ids(out))
	}
}

func TestResolveExecutionOrder_MissingDependency_TypedError(t *testing.T) {
	// Sensor "b" references "ghost-sensor" which isn't in the input.
	sensors := []Sensor{
		{ID: "a"},
		{ID: "b", DependsOn: []string{"ghost-sensor"}},
	}
	_, err := ResolveExecutionOrder(sensors)
	if err == nil {
		t.Fatal("expected ErrMissingDependency, got nil")
	}
	var missing *ErrMissingDependency
	if !errors.As(err, &missing) {
		t.Fatalf("expected *ErrMissingDependency; got %T: %v", err, err)
	}
	if missing.Sensor != "b" {
		t.Errorf("Sensor: got %q, want %q", missing.Sensor, "b")
	}
	if len(missing.MissingIDs) != 1 || missing.MissingIDs[0] != "ghost-sensor" {
		t.Errorf("MissingIDs: got %v, want [ghost-sensor]", missing.MissingIDs)
	}
}

// ids extracts ids in order for readable test diffs.
func ids(ss []Sensor) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.ID
	}
	return out
}

// sortedIDs returns a sorted copy — used by tests that don't care
// about the exact deterministic ordering, only the set membership.
func sortedIDs(ss []Sensor) []string {
	out := ids(ss)
	sort.Strings(out)
	return out
}
