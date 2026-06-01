package executor

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

func TestParseStepOutputFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out")
	os.WriteFile(p, []byte("charge_id=abc123\nstatus=201\ncharge_id=override\n"), 0o600)
	got, err := parseStepOutputFile(p)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"charge_id": "override", "status": "201"} // last write wins
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestParseStepOutputFileMissing(t *testing.T) {
	got, err := parseStepOutputFile(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("missing file should yield empty map, got %v", got)
	}
}

func TestStepOutEnvName(t *testing.T) {
	if stepOutEnvName("create", "charge-id") != "HARNESS_STEPOUT_CREATE_CHARGE_ID" {
		t.Fatal(stepOutEnvName("create", "charge-id"))
	}
}

// TestRunStepOutputPropagation proves the end-to-end flow: step 1 writes to
// $HARNESS_OUTPUT; step 2 reads the value via the compiled env ref
// ${HARNESS_STEPOUT_<S1>_<NAME>} and exits 0 only when the value matches.
func TestRunStepOutputPropagation(t *testing.T) {
	uc := &usecase.UseCase{ID: "out-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0",
		ID:            "out-sensor",
		UseCaseID:     "out-uc",
		Angle:         enums.AngleBuild,
		Kind:          enums.KindAssertion,
		Nature:        enums.NatureComputational,
		OutputType:    enums.OutputSingleShot,
		Uses:          []string{"fake-stack"},
		Steps: []sensor.Step{
			// Step 1: write to $HARNESS_OUTPUT and also emit a pass signal so the
			// executor does not treat it as a crash.
			{
				ID:  "write",
				Run: `printf 'cid=42\n' >> "$HARNESS_OUTPUT" && ` + fakeSensorBin + ` signal pass`,
			},
			// Step 2: verify the value is available via the env ref that
			// template.Compile produces for ${{ steps.write.outputs.cid }}.
			// We use the compiled form directly (env var) to avoid needing
			// template compilation in the test setup.
			{
				ID:  "read",
				Run: `test "${HARNESS_STEPOUT_WRITE_CID}" = "42" && ` + fakeSensorBin + ` signal pass`,
			},
		},
	}

	ex := New(Options{
		RepoRoot:     t.TempDir(),
		Resolver:     &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore: emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) {
			if id == s.ID {
				return uc, true
			}
			return nil, false
		},
		Now: fixedExecNow,
	})

	runDir := t.TempDir()
	agg, err := ex.Run(context.Background(), s, runDir, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass; termination = %q; heal_hint = %+v",
			agg.Verdict, agg.TerminationReason, agg.HealHint)
	}
	if agg.TerminationReason != enums.TerminationCompleted {
		t.Errorf("termination_reason = %q, want completed", agg.TerminationReason)
	}
}
