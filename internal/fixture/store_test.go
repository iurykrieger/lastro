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
