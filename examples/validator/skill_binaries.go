package validator

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// SkillBinaries holds absolute paths to skill binaries built by
// NewSkillBinaries. Tests share one instance per process (typically
// constructed in TestMain).
type SkillBinaries struct {
	ValidateUseCase string
	Heal            string
}

// NewSkillBinaries runs `go build` for the validate-use-case and heal
// skills, writing the binaries into workDir. frameworkRoot must be the
// lastro module root (where ./skills/... resolves). workDir must exist
// or be creatable.
func NewSkillBinaries(workDir, frameworkRoot string) (*SkillBinaries, error) {
	if workDir == "" {
		return nil, errors.New("workDir is required")
	}
	if frameworkRoot == "" {
		return nil, errors.New("frameworkRoot is required")
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir workDir: %w", err)
	}

	sb := &SkillBinaries{}
	builds := []struct {
		name string
		pkg  string
		out  *string
	}{
		{"validate-use-case", "./skills/validate-use-case/scripts", &sb.ValidateUseCase},
		{"heal", "./skills/heal/scripts", &sb.Heal},
	}
	for _, b := range builds {
		exeName := b.name
		if runtime.GOOS == "windows" {
			exeName += ".exe"
		}
		outPath := filepath.Join(workDir, exeName)
		cmd := exec.Command("go", "build", "-o", outPath, b.pkg)
		cmd.Dir = frameworkRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("go build %s: %w\n%s", b.pkg, err, out)
		}
		*b.out = outPath
	}
	return sb, nil
}
