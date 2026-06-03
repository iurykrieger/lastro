package sensor

import (
	"reflect"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

// gatherSensor is a minimal Sensor for gather-closure tests: only the
// fields GatherForUseCase reads (ID, UseCaseID, Scope, DependsOn).
func gatherSensor(id, useCaseID string, scope enums.SensorScope, deps ...string) Sensor {
	return Sensor{ID: id, UseCaseID: useCaseID, Scope: scope, DependsOn: deps}
}

func gatheredIDs(ss []Sensor) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.ID)
	}
	return out
}

func TestGatherForUseCase_IncludesTransitiveCoreClosure(t *testing.T) {
	// uc1-e2e -> core-e2e -> run-dev (use-case→core→core).
	st, err := NewStore(
		gatherSensor("uc1-e2e", "uc1", enums.ScopeUseCase, "core-e2e"),
		gatherSensor("core-e2e", "", enums.ScopeCore, "run-dev"),
		gatherSensor("run-dev", "", enums.ScopeCore),
		gatherSensor("uc2-build", "uc2", enums.ScopeUseCase), // unrelated use case
	)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	got := gatheredIDs(st.GatherForUseCase("uc1"))
	want := []string{"core-e2e", "run-dev", "uc1-e2e"} // sorted by id
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GatherForUseCase(uc1) = %v, want %v", got, want)
	}
}

func TestGatherForUseCase_DoesNotFollowUseCaseToUseCase(t *testing.T) {
	// uc1-a depends_on uc2-b (both use-case scope): uc2-b must NOT be pulled in.
	st, _ := NewStore(
		gatherSensor("uc1-a", "uc1", enums.ScopeUseCase, "uc2-b"),
		gatherSensor("uc2-b", "uc2", enums.ScopeUseCase),
	)
	got := gatheredIDs(st.GatherForUseCase("uc1"))
	want := []string{"uc1-a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GatherForUseCase(uc1) = %v, want %v", got, want)
	}
}

func TestGatherForUseCase_DanglingCoreDepSkipped(t *testing.T) {
	// depends_on a core id that isn't stored: gather can't add it; the
	// resolver surfaces the dangling edge later.
	st, _ := NewStore(
		gatherSensor("uc1-e2e", "uc1", enums.ScopeUseCase, "missing-core"),
	)
	got := gatheredIDs(st.GatherForUseCase("uc1"))
	want := []string{"uc1-e2e"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GatherForUseCase(uc1) = %v, want %v", got, want)
	}
}

func TestGatherForUseCase_SharedCoreDeduped(t *testing.T) {
	// Two use-case sensors of uc1 both depend on run-dev: run-dev appears once.
	st, _ := NewStore(
		gatherSensor("uc1-e2e", "uc1", enums.ScopeUseCase, "run-dev"),
		gatherSensor("uc1-logs", "uc1", enums.ScopeUseCase, "run-dev"),
		gatherSensor("run-dev", "", enums.ScopeCore),
	)
	got := gatheredIDs(st.GatherForUseCase("uc1"))
	want := []string{"run-dev", "uc1-e2e", "uc1-logs"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GatherForUseCase(uc1) = %v, want %v", got, want)
	}
}

func TestGatherForUseCase_NoSensors(t *testing.T) {
	st, _ := NewStore()
	if got := st.GatherForUseCase("nope"); len(got) != 0 {
		t.Fatalf("GatherForUseCase(nope) = %v, want empty", got)
	}
}
