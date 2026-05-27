//go:build integration

package examples_test

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/examples/validator"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/stack"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
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

// fixtureStoreOwner adapts a fixture.Store to sensor.UseCaseFixtureOwnership.
type fixtureStoreOwner struct{ store *fixture.Store }

func (o fixtureStoreOwner) OwnedFixtureIDs(useCaseID string) []string {
	var ids []string
	for _, fx := range o.store.FixturesForUseCase(useCaseID) {
		ids = append(ids, fx.ID)
	}
	return ids
}

// goModDirectDeps parses go.mod and returns direct dependency module paths.
// Lines flagged "// indirect" are skipped.
func goModDirectDeps(t *testing.T, sampleDir string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(sampleDir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	var deps []string
	inBlock := false
	for _, line := range strings.Split(string(b), "\n") {
		trim := strings.TrimSpace(line)
		if strings.Contains(trim, "// indirect") {
			continue
		}
		if strings.HasPrefix(trim, "require (") {
			inBlock = true
			continue
		}
		if inBlock && trim == ")" {
			inBlock = false
			continue
		}
		if inBlock {
			if fields := strings.Fields(trim); len(fields) >= 2 {
				deps = append(deps, fields[0])
			}
			continue
		}
		if strings.HasPrefix(trim, "require ") {
			if fields := strings.Fields(trim); len(fields) >= 3 {
				deps = append(deps, fields[1])
			}
		}
	}
	return deps
}

// componentMentions reports whether any stack component references the
// given module path either by id or by name.
func componentMentions(sm stack.StackManifest, dep string) bool {
	for _, c := range sm.Components {
		if c.ID == dep || c.Name == dep {
			return true
		}
	}
	return false
}

// TestCriterion1_StackCoverage — plan §11.1.
// archetype non-empty, ≥1 component with rationale (detection_evidence),
// and ≥95% of go.mod direct deps covered by component ids/names.
func TestCriterion1_StackCoverage(t *testing.T) {
	for _, dir := range sampleDirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			sm, err := stack.Load(filepath.Join(dir, ".harness", "stack-manifest.yaml"))
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if string(sm.Archetype) == "" {
				t.Fatalf("archetype is empty")
			}
			if len(sm.Components) == 0 {
				t.Fatalf("no components declared")
			}
			for _, c := range sm.Components {
				if len(c.DetectionEvidence) == 0 {
					t.Errorf("component %s: empty detection_evidence (acts as rationale)", c.ID)
				}
			}
			deps := goModDirectDeps(t, dir)
			if len(deps) == 0 {
				return // No third-party deps to cover.
			}
			covered := 0
			for _, dep := range deps {
				if componentMentions(sm, dep) {
					covered++
				}
			}
			ratio := float64(covered) / float64(len(deps))
			if ratio < 0.95 {
				t.Fatalf("coverage %.2f below 0.95 (covered %d of %d deps: %v)", ratio, covered, len(deps), deps)
			}
		})
	}
}

// TestCriterion2_UseCasePerEntryPoint — plan §11.2.
// Each use case has non-empty given/when/then, ≥1 entry point, ≥1 fixture,
// and the entry point's archetype matches the manifest's archetype.
func TestCriterion2_UseCasePerEntryPoint(t *testing.T) {
	for _, dir := range sampleDirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			sm, err := stack.Load(filepath.Join(dir, ".harness", "stack-manifest.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			fs, err := fixture.LoadDirectory(filepath.Join(dir, ".harness", "fixtures"))
			if err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(filepath.Join(dir, ".harness", "use-cases"))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) == 0 {
				t.Fatal("no use cases")
			}
			for _, e := range entries {
				data, err := os.ReadFile(filepath.Join(dir, ".harness", "use-cases", e.Name()))
				if err != nil {
					t.Fatal(err)
				}
				uc, err := usecase.Load(data, fs)
				if err != nil {
					t.Fatalf("%s: load: %v", e.Name(), err)
				}
				if len(uc.Given) == 0 {
					t.Errorf("%s: empty given", uc.ID)
				}
				if len(uc.When) == 0 {
					t.Errorf("%s: empty when", uc.ID)
				}
				if len(uc.Then) == 0 {
					t.Errorf("%s: empty then", uc.ID)
				}
				if len(uc.EntryPoints) == 0 {
					t.Fatalf("%s: no entry points", uc.ID)
				}
				if len(uc.FixtureIDs) == 0 {
					t.Errorf("%s: no fixture_ids", uc.ID)
				}
				for _, ep := range uc.EntryPoints {
					if string(ep.Archetype) != string(sm.Archetype) {
						t.Errorf("%s: entry point %s archetype=%s does not match manifest=%s",
							uc.ID, ep.ID, ep.Archetype, sm.Archetype)
					}
				}
			}
		})
	}
}

// TestCriterion3_TemplateResolution — plan §11.3.
// Every {{fixtures.X}} / {{entry_points.X}} reference resolves to an id
// defined in the same use case.
func TestCriterion3_TemplateResolution(t *testing.T) {
	for _, dir := range sampleDirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			fs, err := fixture.LoadDirectory(filepath.Join(dir, ".harness", "fixtures"))
			if err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(filepath.Join(dir, ".harness", "use-cases"))
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				data, _ := os.ReadFile(filepath.Join(dir, ".harness", "use-cases", e.Name()))
				uc, err := usecase.Load(data, fs)
				if err != nil {
					t.Fatalf("%s: load: %v", e.Name(), err)
				}
				fixSet := map[string]struct{}{}
				for _, id := range uc.FixtureIDs {
					fixSet[id] = struct{}{}
				}
				epSet := map[string]struct{}{}
				for _, ep := range uc.EntryPoints {
					epSet[ep.ID] = struct{}{}
				}
				for _, block := range [][]string{uc.Given, uc.When, uc.Then} {
					for _, line := range block {
						segs, err := template.Parse(line)
						if err != nil {
							t.Errorf("%s: parse %q: %v", uc.ID, line, err)
							continue
						}
						for _, s := range segs {
							switch v := s.(type) {
							case template.FixtureRef:
								if _, ok := fixSet[v.ID]; !ok {
									t.Errorf("%s: fixture %q not in fixture_ids %v", uc.ID, v.ID, uc.FixtureIDs)
								}
							case template.EntryPointRef:
								if _, ok := epSet[v.ID]; !ok {
									t.Errorf("%s: entry point %q not in entry_points", uc.ID, v.ID)
								}
							}
						}
					}
				}
			}
		})
	}
}

// TestCriterion5_ValidateExecution_HappyPath — plan §11.5.
// ValidateAll on passing samples returns AllPassed().
func TestCriterion5_ValidateExecution_HappyPath(t *testing.T) {
	for _, dir := range []string{"./http-api-sample", "./cli-sample"} {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			abs, _ := filepath.Abs(dir)
			_ = os.RemoveAll(filepath.Join(abs, ".harness", "reports"))

			report, err := validator.ValidateAll(validateCtx(t), abs, skills)
			if err != nil {
				t.Fatalf("ValidateAll: %v", err)
			}
			if !report.AllPassed() {
				t.Fatalf("not all passed: summary=%+v failed=%+v", report.Summary, report.Failed())
			}
			t.Logf("report: %s/.harness/reports/%s/report.json", abs, report.RunID)
		})
	}
}

// TestCriterion5_ValidateExecution_FailingPath — plan §11.5 (failure shape).
// ValidateAll on the broken sample returns exactly one failing use case —
// uc-create-order-bad-input — with a non-nil HealHint.
func TestCriterion5_ValidateExecution_FailingPath(t *testing.T) {
	abs, _ := filepath.Abs("./http-api-sample-broken")
	_ = os.RemoveAll(filepath.Join(abs, ".harness", "reports"))

	report, err := validator.ValidateAll(validateCtx(t), abs, skills)
	if err != nil {
		t.Fatalf("ValidateAll: %v", err)
	}
	failed := report.Failed()
	if len(failed) != 1 {
		t.Fatalf("want exactly 1 failure, got %d: %+v", len(failed), failed)
	}
	if failed[0].UseCaseID != "uc-create-order-bad-input" {
		t.Fatalf("want failing use case = uc-create-order-bad-input, got %s", failed[0].UseCaseID)
	}
	if failed[0].HealHint == nil {
		t.Fatalf("want non-nil HealHint on failure")
	}
	t.Logf("heal hint: %+v", failed[0].HealHint)
}

// TestCriterion4_SensorGrounding — plan §11.4.
// Every sensor's top-level uses references valid stack components, and
// every step-level uses references fixtures owned by the sensor's use case.
func TestCriterion4_SensorGrounding(t *testing.T) {
	for _, dir := range sampleDirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			sm, err := stack.Load(filepath.Join(dir, ".harness", "stack-manifest.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			fs, err := fixture.LoadDirectory(filepath.Join(dir, ".harness", "fixtures"))
			if err != nil {
				t.Fatal(err)
			}
			store, err := sensor.LoadDirectory(filepath.Join(dir, ".harness", "sensors"))
			if err != nil {
				t.Fatalf("load sensors: %v", err)
			}
			owner := fixtureStoreOwner{store: fs}
			for _, s := range store.All() {
				if err := sensor.ValidateAgainstStack(s, sm); err != nil {
					t.Errorf("sensor %s grounding (stack): %v", s.ID, err)
				}
				if err := sensor.ValidateAgainstFixtures(s, owner); err != nil {
					t.Errorf("sensor %s grounding (fixtures): %v", s.ID, err)
				}
			}
		})
	}
}
