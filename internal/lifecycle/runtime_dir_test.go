package lifecycle

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeDir_PathsAreScopedByIDAndRun(t *testing.T) {
	root := t.TempDir()
	got := runDirPath(root, "create-order-sensor", "01JZQ9G7M0H3FX8N1QPYAS78MV")
	want := filepath.Join(root, "create-order-sensor", "01JZQ9G7M0H3FX8N1QPYAS78MV")
	if got != want {
		t.Errorf("runDirPath = %q, want %q", got, want)
	}
}

func TestRegistryPath_IsAtRoot(t *testing.T) {
	got := registryPath(t.TempDir())
	if !strings.HasSuffix(got, string(filepath.Separator)+"running_sensors.json") {
		t.Errorf("registryPath = %q, want trailing running_sensors.json", got)
	}
}
