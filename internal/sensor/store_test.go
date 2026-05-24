package sensor

import (
	"path/filepath"
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

func TestStore_LookupSensor(t *testing.T) {
	a := Sensor{ID: "a-sensor", UseCaseID: "uc-1"}
	store, _ := NewStore(a)
	got, ok := store.LookupSensor("a-sensor")
	if !ok {
		t.Fatal("LookupSensor(a-sensor): ok=false, want true")
	}
	if got.ID != "a-sensor" {
		t.Errorf("LookupSensor(a-sensor).ID = %q, want %q", got.ID, "a-sensor")
	}
	_, ok = store.LookupSensor("unknown")
	if ok {
		t.Error("LookupSensor(unknown): ok=true, want false")
	}
}

func TestStore_ForUseCase_SortedByID(t *testing.T) {
	// Insert in non-sorted order to verify the store sorts.
	c := Sensor{ID: "c-sensor", UseCaseID: "uc-1"}
	a := Sensor{ID: "a-sensor", UseCaseID: "uc-1"}
	b := Sensor{ID: "b-sensor", UseCaseID: "uc-2"}
	store, _ := NewStore(c, a, b)
	got := store.ForUseCase("uc-1")
	if len(got) != 2 {
		t.Fatalf("ForUseCase(uc-1) returned %d, want 2", len(got))
	}
	if got[0].ID != "a-sensor" || got[1].ID != "c-sensor" {
		t.Errorf("ForUseCase(uc-1) not sorted; got %v", []string{got[0].ID, got[1].ID})
	}
	if empty := store.ForUseCase("unknown"); len(empty) != 0 {
		t.Errorf("ForUseCase(unknown): got %d, want 0", len(empty))
	}
}

func TestStore_All_SortedByID(t *testing.T) {
	c := Sensor{ID: "c-sensor", UseCaseID: "uc-1"}
	a := Sensor{ID: "a-sensor", UseCaseID: "uc-2"}
	b := Sensor{ID: "b-sensor", UseCaseID: "uc-1"}
	store, _ := NewStore(c, a, b)
	got := store.All()
	want := []string{"a-sensor", "b-sensor", "c-sensor"}
	if len(got) != len(want) {
		t.Fatalf("All() length: got %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].ID != w {
			t.Errorf("All()[%d].ID = %q, want %q", i, got[i].ID, w)
		}
	}
}

func TestLoadDirectory_HappyPath(t *testing.T) {
	dir := filepath.Join("..", "..", "schemas", "examples", "sensor")
	store, err := LoadDirectory(dir)
	if err != nil {
		t.Fatalf("LoadDirectory: %v", err)
	}
	if len(store.All()) < 6 {
		t.Errorf("LoadDirectory(%s): got %d sensors, want at least 6", dir, len(store.All()))
	}
}

func TestLoadDirectory_DuplicateIDAcrossFiles_NamesBothPaths(t *testing.T) {
	dir := filepath.Join("testdata", "duplicate-id")
	_, err := LoadDirectory(dir)
	if err == nil {
		t.Fatal("expected duplicate-id error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "duplicate") {
		t.Errorf("error did not mention 'duplicate'; got: %v", err)
	}
	if !strings.Contains(msg, "a.yaml") || !strings.Contains(msg, "b.yaml") {
		t.Errorf("error did not name both files a.yaml and b.yaml; got: %v", err)
	}
}
