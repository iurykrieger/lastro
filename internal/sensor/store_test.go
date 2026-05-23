package sensor

import (
	"strings"
	"testing"
)

func TestNewStore_HappyPath(t *testing.T) {
	a := Sensor{ID: "a-sensor", UseCaseID: "uc-1"}
	b := Sensor{ID: "b-sensor", UseCaseID: "uc-2"}
	store, err := NewStore(a, b)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if store == nil {
		t.Fatal("NewStore returned nil store with nil error")
	}
}

func TestNewStore_RejectsDuplicateIDs(t *testing.T) {
	a := Sensor{ID: "dup", UseCaseID: "uc-1"}
	b := Sensor{ID: "dup", UseCaseID: "uc-2"}
	_, err := NewStore(a, b)
	if err == nil {
		t.Fatal("expected duplicate-id error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "dup") {
		t.Errorf("error did not mention duplicate id %q; got: %v", "dup", err)
	}
}

func TestNewStore_EmptyInput(t *testing.T) {
	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() with no sensors: %v", err)
	}
	if store == nil {
		t.Fatal("NewStore() returned nil store with nil error")
	}
}
