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

func TestResolveExecutionOrder_LinearChain_TopoBeatsAlpha(t *testing.T) {
	// Chain: z → m → a (z runs first; m depends on z; a depends on m)
	// Topological: [z, m, a]. Alphabetical: [a, m, z] — these differ,
	// which pins down that the resolver honors edges, not id-sort.
	sensors := []Sensor{
		{ID: "a", DependsOn: []string{"m"}},
		{ID: "z"},
		{ID: "m", DependsOn: []string{"z"}},
	}
	out, err := ResolveExecutionOrder(sensors)
	if err != nil {
		t.Fatalf("linear chain: %v", err)
	}
	got := ids(out)
	want := []string{"z", "m", "a"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestResolveExecutionOrder_DiamondWithIDSortTiebreak(t *testing.T) {
	// Diamond with deliberately divergent id naming:
	//   z has no deps               <- root
	//   y depends on z              <- y and x both ready after z;
	//   x depends on z                 id-sort tiebreak picks x first
	//   a depends on y and x        <- leaf
	// Topological + id-sort tiebreak: [z, x, y, a].
	// Alphabetical-only: [a, x, y, z] — divergent.
	sensors := []Sensor{
		{ID: "a", DependsOn: []string{"y", "x"}},
		{ID: "y", DependsOn: []string{"z"}},
		{ID: "z"},
		{ID: "x", DependsOn: []string{"z"}},
	}
	out, err := ResolveExecutionOrder(sensors)
	if err != nil {
		t.Fatalf("diamond: %v", err)
	}
	got := ids(out)
	want := []string{"z", "x", "y", "a"}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestResolveExecutionOrder_CrossUseCaseEdgesAllowed(t *testing.T) {
	// Sensor z (uc-1) is depended on by sensor a (uc-2). The resolver
	// does not care about UseCaseID — cross-use-case edges are legal
	// at the structural layer (policy may reject them later, but
	// that's E9 + Phase B's concern). Id naming keeps topo divergent
	// from alphabetical.
	sensors := []Sensor{
		{ID: "a", UseCaseID: "uc-2", DependsOn: []string{"z"}},
		{ID: "z", UseCaseID: "uc-1"},
	}
	out, err := ResolveExecutionOrder(sensors)
	if err != nil {
		t.Fatalf("cross-use-case edges: %v", err)
	}
	got := ids(out)
	want := []string{"z", "a"}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestResolveExecutionOrder_FiveNodeGraph(t *testing.T) {
	// Graph (id naming deliberately makes topological order diverge
	// from alphabetical):
	//   z -> x -> m -> a
	//   z -> y -> m
	//
	// z is the only zero-in-degree node — runs first.
	// After z, x and y are both ready; id-sort picks x then y.
	// m depends on both x and y; ready only after both — runs next.
	// a depends on m — runs last.
	//
	// Topological with id-tiebreak: [z, x, y, m, a].
	// Alphabetical-only: [a, m, x, y, z] — divergent, so a placeholder
	// sort-by-id resolver would fail this test.
	sensors := []Sensor{
		{ID: "a", DependsOn: []string{"m"}},
		{ID: "m", DependsOn: []string{"x", "y"}},
		{ID: "y", DependsOn: []string{"z"}},
		{ID: "x", DependsOn: []string{"z"}},
		{ID: "z"},
	}
	out, err := ResolveExecutionOrder(sensors)
	if err != nil {
		t.Fatalf("five-node: %v", err)
	}
	got := ids(out)
	want := []string{"z", "x", "y", "m", "a"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
