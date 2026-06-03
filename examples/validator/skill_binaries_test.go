package validator

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// frameworkRootForTest walks up from this file to find the lastro module root.
func frameworkRootForTest(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	// examples/validator/skill_binaries_test.go -> ../..
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func TestNewSkillBinariesBuildsBothBinaries(t *testing.T) {
	if testing.Short() {
		t.Skip("requires go build")
	}
	work := t.TempDir()
	sb, err := NewSkillBinaries(work, frameworkRootForTest(t))
	if err != nil {
		t.Fatalf("NewSkillBinaries: %v", err)
	}
	for _, p := range []string{sb.ValidateUseCase, sb.Heal} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if info.Size() == 0 {
			t.Fatalf("%s is empty", p)
		}
	}
	_ = runtime.GOOS // silence unused import on some toolchains
}

func TestNewSkillBinariesRejectsEmptyArgs(t *testing.T) {
	if _, err := NewSkillBinaries("", "/some/root"); err == nil {
		t.Fatalf("want error for empty workDir")
	}
	if _, err := NewSkillBinaries(t.TempDir(), ""); err == nil {
		t.Fatalf("want error for empty frameworkRoot")
	}
}
