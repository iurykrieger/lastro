package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRegistry_EmptyReadReturnsEmptySlice(t *testing.T) {
	root := t.TempDir()
	r := newRegistry(root)
	entries, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %v, want empty", entries)
	}
}

func TestRegistry_AppendAndFind(t *testing.T) {
	root := t.TempDir()
	r := newRegistry(root)
	h := Handle{
		SensorID: "s1", RunID: "r1", RunDir: filepath.Join(root, "s1", "r1"),
		PID: 1234, PGID: 1234, StartedAt: time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC),
	}
	if err := r.Append(h); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, ok, err := r.Find("s1", "r1")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if !ok {
		t.Fatalf("Find returned ok=false; want true")
	}
	if got.PID != 1234 {
		t.Errorf("got.PID = %d, want 1234", got.PID)
	}
}

func TestRegistry_RemoveByKey(t *testing.T) {
	root := t.TempDir()
	r := newRegistry(root)
	h := Handle{SensorID: "s1", RunID: "r1", PID: 99}
	_ = r.Append(h)
	if err := r.Remove("s1", "r1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, ok, _ := r.Find("s1", "r1")
	if ok {
		t.Errorf("Find after Remove returned ok=true")
	}
}

func TestRegistry_ConcurrentAppendDoesNotLoseEntries(t *testing.T) {
	root := t.TempDir()
	r := newRegistry(root)
	const N = 16
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h := Handle{
				SensorID: "s",
				RunID:    fmt.Sprintf("r%02d", i),
				PID:      1000 + i,
			}
			_ = r.Append(h)
		}(i)
	}
	wg.Wait()
	entries, _ := r.List()
	if len(entries) != N {
		t.Errorf("entries = %d, want %d", len(entries), N)
	}
}

func TestRegistry_AppendCreatesFile(t *testing.T) {
	root := t.TempDir()
	r := newRegistry(root)
	_ = r.Append(Handle{SensorID: "s", RunID: "r", PID: 1})
	if _, err := os.Stat(registryPath(root)); err != nil {
		t.Errorf("registry file not created: %v", err)
	}
}
