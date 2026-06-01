package executor

import (
	"context"
	"testing"

	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// composeBaseOptions builds Options with a SensorLookup over the given
// primitives. uc is the use case every sensor in this test resolves to.
func composeBaseOptions(t *testing.T, uc *usecase.UseCase, prims ...sensor.Sensor) Options {
	t.Helper()
	byID := map[string]sensor.Sensor{}
	for _, p := range prims {
		byID[p.ID] = p
	}
	return Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		SensorLookup: func(id string) (sensor.Sensor, bool) {
			p, ok := byID[id]
			return p, ok
		},
		Now: fixedExecNow,
	}
}

// TestCompose_UsesStepBindsInput verifies a consumer's uses-step expands
// the primitive's steps with the bound input env. The primitive's step
// asserts ${{ inputs.method }} == POST then emits a pass signal.
func TestCompose_UsesStepBindsInput(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}

	prim := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-request", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:   []string{"fake-stack"},
		Inputs: map[string]sensor.InputSpec{"method": {Required: true}},
		Steps: []sensor.Step{
			{ID: "do", Run: `if [ "${{ inputs.method }}" = "POST" ]; then ` + fakeSensorBin + ` signal pass; else ` + fakeSensorBin + ` signal fail; fi`},
		},
	}

	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "step1", Uses: "core-request", With: map[string]string{"method": "POST"}},
		},
	}

	ex := New(composeBaseOptions(t, uc, prim))
	agg, err := ex.Run(context.Background(), consumer, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass (rollup=%+v)", agg.Verdict, agg.Rollup)
	}
}

// TestCompose_RequiredInputUnbound verifies that a uses-step leaving a
// required input unbound fails the run.
func TestCompose_RequiredInputUnbound(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}

	prim := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-request", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:   []string{"fake-stack"},
		Inputs: map[string]sensor.InputSpec{"method": {Required: true}},
		Steps: []sensor.Step{
			{ID: "do", Run: fakeSensorBin + ` signal pass`},
		},
	}

	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "step1", Uses: "core-request"}, // method unbound
		},
	}

	ex := New(composeBaseOptions(t, uc, prim))
	agg, err := ex.Run(context.Background(), consumer, t.TempDir(), nil, nil)
	// The run must not silently pass.
	if err == nil && agg.Verdict == enums.VerdictPass {
		t.Fatalf("expected failure for unbound required input; got pass with nil err")
	}
}

// TestCompose_OutputReExport verifies a primitive's declared outputs are
// resolved from its inner steps and exposed under the uses-step id, so a
// later run-step can read ${{ steps.step1.outputs.id }}.
func TestCompose_OutputReExport(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}

	// Primitive's inner step writes id=abc123 to HARNESS_OUTPUT, then
	// emits a pass signal. The primitive re-exports it as output "id".
	prim := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-create", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:    []string{"fake-stack"},
		Outputs: map[string]sensor.OutputSpec{"id": {From: "${{ steps.request.outputs.id }}"}},
		Steps: []sensor.Step{
			{ID: "request", Run: `printf 'id=abc123\n' >> "$HARNESS_OUTPUT"; ` + fakeSensorBin + ` signal pass`},
		},
	}

	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "step1", Uses: "core-create"},
			{ID: "step2", Run: `if [ "${{ steps.step1.outputs.id }}" = "abc123" ]; then ` + fakeSensorBin + ` signal pass; else ` + fakeSensorBin + ` signal fail; fi`},
		},
	}

	ex := New(composeBaseOptions(t, uc, prim))
	agg, err := ex.Run(context.Background(), consumer, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass (rollup=%+v)", agg.Verdict, agg.Rollup)
	}
}

// TestCompose_WithFixtureRefBound verifies that a consumer's with-value
// containing ${{ fixtures.<id> }} resolves to the bound fixture file path.
// The primitive's inner step reads the file via ${{ inputs.body }} and
// asserts the payload matches; the sensor must pass.
func TestCompose_WithFixtureRefBound(t *testing.T) {
	const fixtureID = "create-charge-input"
	const payload = "PAYLOAD"

	uc := &usecase.UseCase{
		ID:         "fake-uc",
		FixtureIDs: []string{fixtureID},
	}

	store, err := fixture.NewStore(fixture.Fixture{
		SchemaVersion: "1.0.0",
		ID:            fixtureID,
		UseCaseID:     "fake-uc",
		Role:          fixture.RoleInput,
		ContentType:   "text/plain",
		Payload:       []byte(payload),
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Primitive's inner step reads the file pointed to by ${{ inputs.body }}
	// and asserts its content equals the fixture payload.
	prim := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-http", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:   []string{"fake-stack"},
		Inputs: map[string]sensor.InputSpec{"body": {Required: true}},
		Steps: []sensor.Step{
			{
				ID:  "assert-body",
				Run: `if [ "$(cat "${{ inputs.body }}")" = "` + payload + `" ]; then ` + fakeSensorBin + ` signal pass; else ` + fakeSensorBin + ` signal fail; fi`,
			},
		},
	}

	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "step1", Uses: "core-http", With: map[string]string{
				"body": "${{ fixtures." + fixtureID + " }}",
			}},
		},
	}

	opts := composeBaseOptions(t, uc, prim)
	opts.FixtureStore = store
	ex := New(opts)
	agg, err := ex.Run(context.Background(), consumer, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass (rollup=%+v)", agg.Verdict, agg.Rollup)
	}
}

// TestCompose_WithFixtureRefUnowned verifies that a with-value referencing a
// fixture not owned by the consumer's use case fails the sensor.
func TestCompose_WithFixtureRefUnowned(t *testing.T) {
	const fixtureID = "other-uc-fixture"

	// Use case does NOT list the fixture in FixtureIDs.
	uc := &usecase.UseCase{
		ID:         "fake-uc",
		FixtureIDs: []string{},
	}

	store, err := fixture.NewStore(fixture.Fixture{
		SchemaVersion: "1.0.0",
		ID:            fixtureID,
		UseCaseID:     "other-uc", // owned by a different use case
		Role:          fixture.RoleInput,
		ContentType:   "text/plain",
		Payload:       []byte("data"),
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	prim := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-http", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:   []string{"fake-stack"},
		Inputs: map[string]sensor.InputSpec{"body": {Required: true}},
		Steps: []sensor.Step{
			{ID: "do", Run: fakeSensorBin + ` signal pass`},
		},
	}

	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "step1", Uses: "core-http", With: map[string]string{
				"body": "${{ fixtures." + fixtureID + " }}",
			}},
		},
	}

	opts := composeBaseOptions(t, uc, prim)
	opts.FixtureStore = store
	ex := New(opts)
	agg, err := ex.Run(context.Background(), consumer, t.TempDir(), nil, nil)
	// Must not pass: fixture is not owned by the use case.
	if err == nil && agg.Verdict == enums.VerdictPass {
		t.Fatalf("expected failure for unowned fixture in with-value; got pass")
	}
	if agg.TerminationReason != enums.TerminationError {
		t.Errorf("termination_reason = %q, want error", agg.TerminationReason)
	}
}

// TestCompose_MissingSensorLookup verifies a uses-step with nil
// SensorLookup yields a clear error.
func TestCompose_MissingSensorLookup(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	opts := composeBaseOptions(t, uc)
	opts.SensorLookup = nil

	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "step1", Uses: "core-request", With: map[string]string{"method": "POST"}},
		},
	}

	ex := New(opts)
	agg, err := ex.Run(context.Background(), consumer, t.TempDir(), nil, nil)
	// Composition errors surface as a TerminationError aggregate (not a
	// top-level error), mirroring the crashed-step path. The run must not
	// pass.
	if err == nil && agg.Verdict == enums.VerdictPass {
		t.Fatalf("expected non-pass verdict for nil SensorLookup on uses-step; got pass")
	}
	if agg.TerminationReason != enums.TerminationError {
		t.Errorf("termination_reason = %q, want error", agg.TerminationReason)
	}
}
