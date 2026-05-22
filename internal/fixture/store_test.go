package fixture

import (
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
