//go:build integration

package examples_test

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/examples/validator"
)

var (
	skills        *validator.SkillBinaries
	frameworkRoot string
	sampleDirs    = []string{"./http-api-sample", "./http-api-sample-broken", "./cli-sample"}
)

// resolveFrameworkRoot walks up from this file to the lastro module root.
func resolveFrameworkRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	// examples/integration_test.go → ..
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), ".."))
}

func TestMain(m *testing.M) {
	// flag.Parse so testing.Short() works inside individual tests.
	if !flag.Parsed() {
		flag.Parse()
	}
	if testing.Short() {
		os.Exit(0)
	}
	frameworkRoot = resolveFrameworkRoot()
	tmp, err := os.MkdirTemp("", "b7-skills-*")
	if err != nil {
		log.Fatalf("mkdtemp: %v", err)
	}
	defer os.RemoveAll(tmp)

	sb, err := validator.NewSkillBinaries(tmp, frameworkRoot)
	if err != nil {
		log.Fatalf("build skills: %v", err)
	}
	skills = sb
	os.Exit(m.Run())
}

// validateCtx returns a context with a per-test timeout so a stuck
// sensor cannot wedge the run.
func validateCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}
