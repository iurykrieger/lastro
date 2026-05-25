package skillruntime

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBootLifecycle_EmptyHarness verifies the function does not blow up
// on an .harness/ tree containing empty sensors/, fixtures/, and
// use-cases/ directories. This is the minimum smoke test.
func TestBootLifecycle_EmptyHarness(t *testing.T) {
	repo := t.TempDir()
	for _, sub := range []string{"sensors", "fixtures", "use-cases", "runtime"} {
		if err := os.MkdirAll(filepath.Join(repo, ".harness", sub), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	b, err := BootLifecycle(repo)
	if err != nil {
		t.Fatalf("BootLifecycle: %v", err)
	}
	defer func() {
		if cerr := b.Cleanup(); cerr != nil {
			t.Errorf("Cleanup: %v", cerr)
		}
	}()

	if b.Lifecycle == nil {
		t.Errorf("Lifecycle is nil")
	}
	if b.Sensors == nil {
		t.Errorf("Sensors is nil")
	}
	if b.Fixtures == nil {
		t.Errorf("Fixtures is nil")
	}
	if b.UseCases == nil {
		t.Errorf("UseCases is nil")
	}
}

func TestBootLifecycle_MissingHarness(t *testing.T) {
	repo := t.TempDir()
	_, err := BootLifecycle(repo)
	if err == nil {
		t.Errorf("expected error on missing .harness/")
	}
}
