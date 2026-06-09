package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// stubRunnerFactory returns a SensorRunner that emits one
// pre-configured AggregateSignal per sensor ID.
func stubRunnerFactory(verdicts map[string]enums.Verdict) runnerFactory {
	return func(arts *HarnessArtifacts, repoRoot string) (SensorRunner, ServiceManager, func(), error) {
		return &fakeRunnerVerdict{verdicts: verdicts}, &fakeServiceMgr{}, func() {}, nil
	}
}

type fakeRunnerVerdict struct {
	verdicts map[string]enums.Verdict
}

func (f *fakeRunnerVerdict) RunSensor(ctx context.Context, sensorID string, expectedObs []string) (aggregate.AggregateSignal, error) {
	v := enums.VerdictPass
	if vv, ok := f.verdicts[sensorID]; ok {
		v = vv
	}
	return aggregate.AggregateSignal{
		SchemaVersion: "1.0.0",
		Type:          aggregate.TypeAggregate,
		SensorID:      sensorID,
		Verdict:       v,
		Confidence:    1.0,
		Rollup:        aggregate.RollupCounts{TotalSignals: 1, PassCount: 1},
	}, nil
}

func TestValidate_AllPass(t *testing.T) {
	harnessDir, err := filepath.Abs(passingTreeRel)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if _, err := os.Stat(harnessDir); err != nil {
		t.Skipf("fixture tree missing: %v (hand-author cmd/harness/testdata/harness/passing/.harness/ to enable)", err)
	}

	cfg := &Config{Output: "json", RepoRoot: filepath.Dir(filepath.Dir(harnessDir))}
	out := &bytes.Buffer{}
	if err := runValidateWith(context.Background(), cfg, nil, true, out, stubRunnerFactory(nil)); err != nil {
		t.Fatalf("runValidateWith: %v", err)
	}

	var got map[string]any
	if jerr := json.Unmarshal(out.Bytes(), &got); jerr != nil {
		t.Fatalf("decode: %v\nraw: %s", jerr, out.String())
	}
	result, _ := got["result"].(map[string]any)
	summary, _ := result["summary"].(map[string]any)
	if fail, _ := summary["fail_count"].(float64); fail != 0 {
		t.Errorf("fail_count = %v, want 0", fail)
	}
}

func TestValidate_OneFails(t *testing.T) {
	harnessDir, err := filepath.Abs(passingTreeRel)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if _, err := os.Stat(harnessDir); err != nil {
		t.Skipf("fixture tree missing: %v", err)
	}

	// Pick the first sensor in the fixture tree and mark it failing.
	arts, err := LoadHarnessArtifacts(harnessDir, filepath.Join(harnessDir, "validation-policy.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// sensor.Store exposes All(), not List() — confirmed during Task 17.
	all := arts.Sensors.All()
	if len(all) == 0 {
		t.Skip("fixture tree has no sensors")
	}
	first := all[0]
	verdicts := map[string]enums.Verdict{first.ID: enums.VerdictFail}

	cfg := &Config{Output: "json", RepoRoot: filepath.Dir(filepath.Dir(harnessDir))}
	out := &bytes.Buffer{}
	err = runValidateWith(context.Background(), cfg, nil, true, out, stubRunnerFactory(verdicts))

	if err != VerdictFailError {
		t.Fatalf("err = %v, want VerdictFailError", err)
	}
}

func TestDefaultRunnerFactory_BuildsServiceManager(t *testing.T) {
	st, err := sensor.NewStore()
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	arts := &HarnessArtifacts{
		Sensors:     st,
		UseCases:    map[string]*usecase.UseCase{},
		RuntimeRoot: t.TempDir(),
	}
	runner, mgr, cleanup, err := defaultRunnerFactory(arts, t.TempDir())
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	defer cleanup()
	if runner == nil || mgr == nil {
		t.Fatalf("factory returned nil runner/mgr")
	}
}
