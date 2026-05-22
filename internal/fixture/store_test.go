package fixture

import (
	"sort"
	"strings"
	"testing"
)

func TestNewStoreHappyPath(t *testing.T) {
	a := Fixture{ID: "a", UseCaseID: "uc1", Role: RoleInput, ContentType: "application/json"}
	b := Fixture{ID: "b", UseCaseID: "uc1", Role: RoleExpectedOutput, ContentType: "application/json"}
	s, err := NewStore(a, b)
	if err != nil {
		t.Fatalf("NewStore: unexpected error: %v", err)
	}
	if s == nil {
		t.Fatal("NewStore: returned nil store")
	}
}

func TestNewStoreRejectsDuplicateID(t *testing.T) {
	a := Fixture{ID: "dup", UseCaseID: "uc1", Role: RoleInput}
	b := Fixture{ID: "dup", UseCaseID: "uc2", Role: RoleExpectedOutput}
	_, err := NewStore(a, b)
	if err == nil {
		t.Fatal("NewStore: expected duplicate-id error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate id") || !strings.Contains(err.Error(), "dup") {
		t.Errorf("NewStore error %q should mention 'duplicate id' and the id %q", err.Error(), "dup")
	}
}

func TestStoreSatisfiesFixtureStoreInterface(t *testing.T) {
	var _ FixtureStore = (*Store)(nil)
}

func TestStoreLookupFixtureHit(t *testing.T) {
	want := Fixture{ID: "fx1", UseCaseID: "uc1", Role: RoleInput, ContentType: "application/json"}
	s, err := NewStore(want)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	got, ok := s.LookupFixture("fx1")
	if !ok {
		t.Fatal("LookupFixture(fx1): ok=false, want true")
	}
	if got.ID != want.ID || got.Role != want.Role {
		t.Errorf("LookupFixture(fx1): got %+v, want %+v", got, want)
	}
}

func TestStoreLookupFixtureMiss(t *testing.T) {
	s, err := NewStore(Fixture{ID: "fx1"})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	got, ok := s.LookupFixture("nope")
	if ok {
		t.Fatalf("LookupFixture(nope): ok=true; want false")
	}
	if got.ID != "" {
		t.Errorf("LookupFixture(nope): got non-zero fixture %+v; want zero", got)
	}
}

func TestStoreFixturesForUseCaseFiltersAndSorts(t *testing.T) {
	a := Fixture{ID: "b-fx", UseCaseID: "uc1"}
	b := Fixture{ID: "a-fx", UseCaseID: "uc1"}
	c := Fixture{ID: "c-fx", UseCaseID: "uc2"}
	s, err := NewStore(a, b, c)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	got := s.FixturesForUseCase("uc1")
	if len(got) != 2 {
		t.Fatalf("FixturesForUseCase(uc1): got %d fixtures, want 2", len(got))
	}
	if got[0].ID != "a-fx" || got[1].ID != "b-fx" {
		t.Errorf("FixturesForUseCase(uc1): ids out of order: %v", []string{got[0].ID, got[1].ID})
	}
	if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i].ID < got[j].ID }) {
		t.Errorf("FixturesForUseCase result not sorted ascending by id")
	}
}

func TestStoreFixturesForUseCaseUnknownReturnsEmpty(t *testing.T) {
	s, _ := NewStore(Fixture{ID: "fx", UseCaseID: "uc1"})
	if got := s.FixturesForUseCase("missing"); len(got) != 0 {
		t.Errorf("FixturesForUseCase(missing): got %d, want 0", len(got))
	}
}

func TestStoreAllReturnsSortedAcrossAllUseCases(t *testing.T) {
	a := Fixture{ID: "c-fx", UseCaseID: "uc2"}
	b := Fixture{ID: "a-fx", UseCaseID: "uc1"}
	c := Fixture{ID: "b-fx", UseCaseID: "uc1"}
	s, _ := NewStore(a, b, c)
	got := s.All()
	if len(got) != 3 {
		t.Fatalf("All: got %d, want 3", len(got))
	}
	for i, want := range []string{"a-fx", "b-fx", "c-fx"} {
		if got[i].ID != want {
			t.Errorf("All[%d].ID: got %q, want %q", i, got[i].ID, want)
		}
	}
}
