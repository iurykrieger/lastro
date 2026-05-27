package validator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/stack"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// buildFakeSkill builds the testdata/fakeskill stub into a temp dir
// and returns the absolute path to the binary.
func buildFakeSkill(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "fakeskill")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "./testdata/fakeskill")
	cmd.Dir = "."
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fakeskill: %v\n%s", err, b)
	}
	return out
}

func TestValidateAllAggregatesVerdicts(t *testing.T) {
	if testing.Short() {
		t.Skip("requires go build")
	}
	fake := buildFakeSkill(t)
	sample, err := filepath.Abs("testdata/sample")
	if err != nil {
		t.Fatal(err)
	}

	script := map[string]struct {
		Exit  int      `json:"exit"`
		Lines []string `json:"lines"`
	}{
		"uc-alpha": {
			Exit: 0,
			Lines: []string{
				`{"sensor_id":"alpha-e2e","verdict":"pass"}`,
				`{"use_case_verdict":{"verdict":"pass","confidence":1.0},"use_case_run_id":"01HX","sensor_runs":[{"sensor_id":"alpha-e2e","verdict":"pass"}]}`,
			},
		},
		"uc-bravo": {
			Exit: 1,
			Lines: []string{
				`{"sensor_id":"bravo-e2e","verdict":"fail","heal_hint":{"summary":"bravo failed"}}`,
				`{"use_case_verdict":{"verdict":"fail","confidence":0.9},"use_case_run_id":"01HY","sensor_runs":[{"sensor_id":"bravo-e2e","verdict":"fail"}]}`,
			},
		},
	}
	raw, _ := json.Marshal(script)
	t.Setenv("FAKESKILL_RESPONSES", string(raw))

	skills := &SkillBinaries{ValidateUseCase: fake}
	report, err := ValidateAll(context.Background(), sample, skills)
	if err != nil {
		t.Fatalf("ValidateAll: %v", err)
	}

	if report.Summary.Total != 2 {
		t.Fatalf("Total: want 2, got %d", report.Summary.Total)
	}
	if report.Summary.Passed != 1 || report.Summary.Failed != 1 {
		t.Fatalf("Summary: want 1 pass + 1 fail, got %+v", report.Summary)
	}
	if len(report.Failed()) != 1 || report.Failed()[0].UseCaseID != "uc-bravo" {
		t.Fatalf("Failed: want [uc-bravo], got %+v", report.Failed())
	}
	if report.Failed()[0].HealHint == nil {
		t.Fatalf("Failed[0].HealHint: want non-nil")
	}

	reportPath := filepath.Join(sample, ".harness", "reports", report.RunID, "report.json")
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("report.json missing at %s: %v", reportPath, err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(sample, ".harness", "reports")) })
}

func TestValidateAllPropagatesScriptError(t *testing.T) {
	if testing.Short() {
		t.Skip("requires go build")
	}
	fake := buildFakeSkill(t)
	sample, _ := filepath.Abs("testdata/sample")

	script := map[string]struct {
		Exit  int      `json:"exit"`
		Lines []string `json:"lines"`
	}{
		"uc-alpha": {Exit: 3, Lines: []string{`{"code":"boom"}`}},
		"uc-bravo": {Exit: 0, Lines: []string{`{"use_case_verdict":{"verdict":"pass","confidence":1.0},"use_case_run_id":"x","sensor_runs":[]}`}},
	}
	raw, _ := json.Marshal(script)
	t.Setenv("FAKESKILL_RESPONSES", string(raw))

	skills := &SkillBinaries{ValidateUseCase: fake}
	if _, err := ValidateAll(context.Background(), sample, skills); err == nil {
		t.Fatalf("ValidateAll: want error on skill exit 3, got nil")
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(sample, ".harness", "reports")) })
}

// Gate tests for the http-api-sample's .harness/ artifacts. These run
// in the regular (untagged) test pass so any schema drift between the
// sample's YAML and the entity loaders is caught immediately.

func TestStackManifestLoads_HttpApi(t *testing.T) {
	if _, err := stack.Load("../http-api-sample/.harness/stack-manifest.yaml"); err != nil {
		t.Fatalf("load: %v", err)
	}
}

func TestFixturesLoad_HttpApi(t *testing.T) {
	if _, err := fixture.LoadDirectory("../http-api-sample/.harness/fixtures"); err != nil {
		t.Fatalf("load fixtures: %v", err)
	}
}

func TestUseCasesLoad_HttpApi(t *testing.T) {
	fs, err := fixture.LoadDirectory("../http-api-sample/.harness/fixtures")
	if err != nil {
		t.Fatalf("fixtures: %v", err)
	}
	entries, err := os.ReadDir("../http-api-sample/.harness/use-cases")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no use cases")
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join("../http-api-sample/.harness/use-cases", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := usecase.Load(data, fs); err != nil {
			t.Fatalf("load %s: %v", e.Name(), err)
		}
	}
}
