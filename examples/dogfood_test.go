//go:build dogfood

package examples_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/examples/validator"
)

// dogfoodFrameworkRoot walks up from this file to the lastro module root.
func dogfoodFrameworkRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), ".."))
}

func TestMain(m *testing.M) {
	if !flag.Parsed() {
		flag.Parse()
	}
	if testing.Short() {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestFrameworkSelfValidation — Track 2 dogfood gate.
//
// Validates the framework's own committed .harness/. The framework is
// the CONSUMER here; ValidateAll invokes /validate-use-case once per
// detected use case and asserts AllPassed(). No plan §11 criterion is
// asserted by id — this is a regression gate against the framework's
// own contracts about itself.
func TestFrameworkSelfValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood is slow")
	}
	repoRoot := dogfoodFrameworkRoot()

	work, err := os.MkdirTemp("", "dogfood-skills-*")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(work) })

	skills, err := validator.NewSkillBinaries(work, repoRoot)
	if err != nil {
		t.Fatalf("build skills: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	_ = os.RemoveAll(filepath.Join(repoRoot, ".harness", "reports"))

	report, err := validator.ValidateAll(ctx, repoRoot, skills)
	if err != nil {
		t.Fatalf("ValidateAll: %v", err)
	}
	if !report.AllPassed() {
		var ids []string
		for _, uc := range report.Failed() {
			ids = append(ids, uc.UseCaseID)
		}
		t.Fatalf("dogfood failed:\n  summary=%+v\n  failed=%v\n  report=%s",
			report.Summary, ids,
			fmt.Sprintf("%s/.harness/reports/%s/report.json", repoRoot, report.RunID))
	}
	t.Logf("dogfood passed: %d use cases all green; report at %s/.harness/reports/%s/report.json",
		report.Summary.Total, repoRoot, report.RunID)
}
