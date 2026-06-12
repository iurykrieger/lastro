package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// TestCompose_AuthHeaderFlowsBetweenPrimitives is the issue-#45 path: a
// provisioning primitive mints a credential and re-exports it as `header`;
// a later uses-step feeds it into a request primitive via
// `${{ steps.auth.outputs.header }}` in its with-value. The request
// primitive asserts the exact header (spaces, colon, '=' inside the value)
// arrived intact.
func TestCompose_AuthHeaderFlowsBetweenPrimitives(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	const header = "Cookie: session-token=tok-abc123"

	provision := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "provision-auth", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleEnvironment, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:    []string{"fake-stack"},
		Inputs:  map[string]sensor.InputSpec{"kind": {Required: true}},
		Outputs: map[string]sensor.OutputSpec{"header": {From: "${{ steps.mint.outputs.header }}"}},
		Steps: []sensor.Step{
			{ID: "mint", Run: `printf 'header=` + header + `\n' >> "$HARNESS_OUTPUT"; ` + fakeSensorBin + ` signal pass`},
		},
	}

	request := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-request", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:   []string{"fake-stack"},
		Inputs: map[string]sensor.InputSpec{"headers": {Required: true}},
		Steps: []sensor.Step{
			// NOTE: the ref carries its own safe quoting from template.Compile
			// ("${VAR}") — wrapping it in additional quotes would unquote the
			// expansion and word-split the header value.
			{ID: "do", Run: `if [ ${{ inputs.headers }} = "` + header + `" ]; then ` + fakeSensorBin + ` signal pass; else ` + fakeSensorBin + ` signal fail; fi`},
		},
	}

	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer", UseCaseID: "fake-uc",
		Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "auth", Uses: "provision-auth", With: map[string]string{"kind": "session"}},
			{ID: "request", Uses: "core-request", With: map[string]string{"headers": "${{ steps.auth.outputs.header }}"}},
		},
	}

	ex := New(composeBaseOptions(t, uc, provision, request))
	agg, err := ex.Run(context.Background(), consumer, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass (rollup=%+v)", agg.Verdict, agg.Rollup)
	}
}

// TestCompose_ProvisioningCrashAbortsBeforeRequest pins the fail-safe of
// issue #45: when the provisioning step exits non-zero without emitting a
// signal, the run aborts before the gated request step and aggregates
// inconclusive — never a misleading behavioral fail, even when the
// consumer declares expected matchers.
func TestCompose_ProvisioningCrashAbortsBeforeRequest(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}

	provision := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "provision-auth", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleEnvironment, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:   []string{"fake-stack"},
		Inputs: map[string]sensor.InputSpec{"kind": {Required: true}},
		Steps: []sensor.Step{
			{ID: "mint", Run: `echo 'auth-not-provisioned kind=session reason=mint-failed'; exit 1`},
		},
	}

	request := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-request", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:   []string{"fake-stack"},
		Inputs: map[string]sensor.InputSpec{"headers": {Required: false}},
		Steps: []sensor.Step{
			{ID: "do", Run: `echo "${{ inputs.headers }}"; ` + fakeSensorBin + ` signal fail`},
		},
	}

	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer", UseCaseID: "fake-uc",
		Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		SignalMatches: []sensor.SignalMatch{
			{Key: "created", Pattern: "status=201", Verdict: enums.VerdictPass, Expected: true},
		},
		Steps: []sensor.Step{
			{ID: "auth", Uses: "provision-auth", With: map[string]string{"kind": "session"}},
			{ID: "request", Uses: "core-request", With: map[string]string{"headers": "${{ steps.auth.outputs.header }}"}},
		},
	}

	ex := New(composeBaseOptions(t, uc, provision, request))
	agg, err := ex.Run(context.Background(), consumer, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictInconclusive {
		t.Errorf("verdict = %q, want inconclusive (rollup=%+v, term=%q)", agg.Verdict, agg.Rollup, agg.TerminationReason)
	}
	if agg.Rollup.FailCount != 0 {
		t.Errorf("FailCount = %d, want 0 — the gated request step must never have run", agg.Rollup.FailCount)
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

// TestCompose_PrimitiveFailMatcherGradesComposition pins issue #46: a
// composed primitive's own signal_matches must fire on its inner steps.
// The primitive follows the grade-and-emit contract (print
// "expectation-unmet ..." and exit 1, covered by a fail matcher with a
// heal_hint). The composition must aggregate fail and carry the
// primitive's structured heal_hint — not crash to inconclusive.
func TestCompose_PrimitiveFailMatcherGradesComposition(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}

	prim := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-e2e", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		SignalMatches: []sensor.SignalMatch{
			{
				Key:     "expectation-unmet",
				Pattern: `expectation-unmet expected=(?P<expected>\S+) got=(?P<got>\S+)`,
				Verdict: enums.VerdictFail,
				HealHint: &sensor.MatchHealHint{
					Summary:   "e2e expectation unmet",
					Rationale: "the primitive graded its expectation inputs and they were not met",
				},
			},
		},
		Steps: []sensor.Step{
			{ID: "grade", Run: `echo 'expectation-unmet expected=201 got=500'; exit 1`},
		},
	}

	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer", UseCaseID: "fake-uc",
		Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "step1", Uses: "core-e2e"},
		},
	}

	ex := New(composeBaseOptions(t, uc, prim))
	agg, err := ex.Run(context.Background(), consumer, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictFail {
		t.Errorf("verdict = %q, want fail (rollup=%+v, term=%q)", agg.Verdict, agg.Rollup, agg.TerminationReason)
	}
	if agg.Rollup.FailCount != 1 {
		t.Errorf("FailCount = %d, want 1 — the primitive's fail matcher must fire", agg.Rollup.FailCount)
	}
	if agg.HealHint == nil || agg.HealHint.Summary != "e2e expectation unmet" {
		t.Errorf("HealHint = %+v, want the primitive matcher's structured hint", agg.HealHint)
	}
}

// TestCompose_PrimitiveInconclusiveMatcherAbortsBeforeGatedStep pins the
// #45 scenario with live primitive matchers (#46): provision-auth's own
// inconclusive "auth-not-provisioned" matcher fires when composed, and the
// graded non-zero exit ends the run before the gated request step — so the
// aggregate is inconclusive, never outranked by a downstream fail.
func TestCompose_PrimitiveInconclusiveMatcherAbortsBeforeGatedStep(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}

	provision := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "provision-auth", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleEnvironment, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:   []string{"fake-stack"},
		Inputs: map[string]sensor.InputSpec{"kind": {Required: true}},
		SignalMatches: []sensor.SignalMatch{
			{
				Key:     "auth-not-provisioned",
				Pattern: `auth-not-provisioned kind=(?P<kind>\S+) reason=(?P<reason>\S+)`,
				Verdict: enums.VerdictInconclusive,
			},
		},
		Steps: []sensor.Step{
			{ID: "mint", Run: `echo 'auth-not-provisioned kind=session reason=mint-failed'; exit 1`},
		},
	}

	request := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-request", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:   []string{"fake-stack"},
		Inputs: map[string]sensor.InputSpec{"headers": {Required: false}},
		Steps: []sensor.Step{
			{ID: "do", Run: `echo "${{ inputs.headers }}"; ` + fakeSensorBin + ` signal fail`},
		},
	}

	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer", UseCaseID: "fake-uc",
		Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		SignalMatches: []sensor.SignalMatch{
			{Key: "created", Pattern: "status=201", Verdict: enums.VerdictPass, Expected: true},
		},
		Steps: []sensor.Step{
			{ID: "auth", Uses: "provision-auth", With: map[string]string{"kind": "session"}},
			{ID: "request", Uses: "core-request", With: map[string]string{"headers": "${{ steps.auth.outputs.header }}"}},
		},
	}

	ex := New(composeBaseOptions(t, uc, provision, request))
	agg, err := ex.Run(context.Background(), consumer, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictInconclusive {
		t.Errorf("verdict = %q, want inconclusive (rollup=%+v, term=%q)", agg.Verdict, agg.Rollup, agg.TerminationReason)
	}
	if agg.Rollup.InconclusiveCount != 1 {
		t.Errorf("InconclusiveCount = %d, want 1 — the primitive's diagnostic matcher must fire", agg.Rollup.InconclusiveCount)
	}
	if agg.Rollup.FailCount != 0 {
		t.Errorf("FailCount = %d, want 0 — the gated request step must never have run", agg.Rollup.FailCount)
	}
}

// TestCompose_ConsumerMatcherWinsOnKeyCollision verifies the merge rule:
// when consumer and primitive declare the same matcher key, the consumer's
// matcher wins and the line is matched exactly once (no double-emission).
func TestCompose_ConsumerMatcherWinsOnKeyCollision(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}

	prim := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-graded", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		SignalMatches: []sensor.SignalMatch{
			{
				Key: "graded", Pattern: "outcome=bad", Verdict: enums.VerdictFail,
				HealHint: &sensor.MatchHealHint{Summary: "primitive hint", Rationale: "primitive"},
			},
		},
		Steps: []sensor.Step{
			{ID: "do", Run: `echo 'outcome=bad'`},
		},
	}

	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		SignalMatches: []sensor.SignalMatch{
			{
				Key: "graded", Pattern: "outcome=bad", Verdict: enums.VerdictWarn,
				HealHint: &sensor.MatchHealHint{Summary: "consumer hint", Rationale: "consumer"},
			},
		},
		Steps: []sensor.Step{
			{ID: "step1", Uses: "core-graded"},
		},
	}

	ex := New(composeBaseOptions(t, uc, prim))
	agg, err := ex.Run(context.Background(), consumer, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictWarn {
		t.Errorf("verdict = %q, want warn (consumer matcher wins; rollup=%+v)", agg.Verdict, agg.Rollup)
	}
	if agg.Rollup.TotalSignals != 1 {
		t.Errorf("TotalSignals = %d, want 1 — colliding keys must not double-match", agg.Rollup.TotalSignals)
	}
}

// TestCompose_PrimitivePassMatcherDoesNotHaltExpansion verifies that a
// primitive's pass matcher firing (with a clean exit) lets the expansion
// continue to later inner steps.
func TestCompose_PrimitivePassMatcherDoesNotHaltExpansion(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}

	prim := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-multi", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		SignalMatches: []sensor.SignalMatch{
			{Key: "ready", Pattern: "service ready", Verdict: enums.VerdictPass},
		},
		Steps: []sensor.Step{
			{ID: "boot", Run: `echo 'service ready'`},
			{ID: "verify", Run: fakeSensorBin + ` signal pass`},
		},
	}

	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "step1", Uses: "core-multi"},
		},
	}

	ex := New(composeBaseOptions(t, uc, prim))
	agg, err := ex.Run(context.Background(), consumer, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass (rollup=%+v, term=%q)", agg.Verdict, agg.Rollup, agg.TerminationReason)
	}
	if agg.Rollup.PassCount != 2 {
		t.Errorf("PassCount = %d, want 2 — both inner steps must have run", agg.Rollup.PassCount)
	}
}

// TestCompose_PrimitiveBadMatcherPattern verifies a composed primitive with
// an uncompilable signal_match pattern terminates the run with a clear
// error instead of silently ignoring the matcher.
func TestCompose_PrimitiveBadMatcherPattern(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}

	prim := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-bad", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		SignalMatches: []sensor.SignalMatch{
			{Key: "broken", Pattern: "([unclosed", Verdict: enums.VerdictFail},
		},
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
			{ID: "step1", Uses: "core-bad"},
		},
	}

	ex := New(composeBaseOptions(t, uc, prim))
	agg, err := ex.Run(context.Background(), consumer, t.TempDir(), nil, nil)
	if err == nil && agg.Verdict == enums.VerdictPass {
		t.Fatalf("expected non-pass verdict for uncompilable primitive matcher; got pass")
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

// TestExecUsesStep_PrimitiveDeclaredEnvMissingIsInconclusive pins issue #49:
// when a composed primitive declares a required env var and neither the
// consumer's uses-step env: injection nor the ambient view provides it, the
// run terminates with verdict inconclusive (missing-env signal), and the
// primitive's inner step is never spawned (the marker file is never created).
func TestExecUsesStep_PrimitiveDeclaredEnvMissingIsInconclusive(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}

	// Use a marker file to detect whether the inner step ran.
	marker := filepath.Join(t.TempDir(), "never-runs")

	// Primitive declares HARNESS_T10_SECRET as required (default).
	prim := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-env-prim", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleEnvironment, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Env:  map[string]sensor.EnvSpec{"HARNESS_T10_SECRET": {Description: "required"}},
		Steps: []sensor.Step{
			// This line must never appear in raw.log.
			{ID: "inner", Run: "touch " + marker + "; echo never-runs"},
		},
	}

	// Consumer provides no env: for the primitive's requirement.
	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer-t10-missing", UseCaseID: "fake-uc",
		Angle: enums.AngleEnvironment, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "step1", Uses: "core-env-prim"}, // no env: injection
		},
	}

	opts := composeBaseOptions(t, uc, prim)
	ex := New(opts)
	runDir := t.TempDir()
	agg, err := ex.Run(context.Background(), consumer, runDir, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictInconclusive {
		t.Errorf("verdict = %q, want inconclusive (rollup=%+v)", agg.Verdict, agg.Rollup)
	}
	// signals.jsonl must contain a missing-env record naming the secret.
	sigBytes, _ := os.ReadFile(filepath.Join(runDir, "signals.jsonl"))
	if !strings.Contains(string(sigBytes), "missing-env") {
		t.Errorf("signals.jsonl missing 'missing-env' key: %s", sigBytes)
	}
	if !strings.Contains(string(sigBytes), "HARNESS_T10_SECRET") {
		t.Errorf("signals.jsonl does not name HARNESS_T10_SECRET: %s", sigBytes)
	}
	// The inner step must never have run (marker file absent).
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("inner step ran despite missing required env (must be pre-spawn blocked)")
	}
}

// TestExecUsesStep_ConsumerEnvSatisfiesPrimitiveRequirement pins the positive
// path of issue #49: a consumer uses-step's env: map injects the required
// secret into every inner step of the expansion, satisfying the primitive's
// declared requirement. The value is derived from an ambient env_file var
// (via ${{ env.HARNESS_T10_AMBIENT }}) and is redacted from raw.log.
func TestExecUsesStep_ConsumerEnvSatisfiesPrimitiveRequirement(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}

	// Primitive declares HARNESS_T10_SECRET as required. Its inner step
	// uses a derived-fact pattern: compare the secret value and echo
	// "got=present" or "got=absent" — never the literal secret. This avoids
	// the redaction-vs-match trap.
	prim := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "core-env-prim", UseCaseID: "fake-uc",
		Scope: enums.ScopeCore,
		Angle: enums.AngleEnvironment, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Env:  map[string]sensor.EnvSpec{"HARNESS_T10_SECRET": {Description: "required"}},
		SignalMatches: []sensor.SignalMatch{
			{Key: "secret-present", Pattern: `got=present`, Verdict: enums.VerdictPass},
			{Key: "secret-absent", Pattern: `got=absent`, Verdict: enums.VerdictFail,
				HealHint: &sensor.MatchHealHint{
					Summary:   "secret was absent in expansion",
					Rationale: "HARNESS_T10_SECRET was not injected into the inner step",
				}},
		},
		Steps: []sensor.Step{
			{
				ID:  "inner",
				Run: `[ "$HARNESS_T10_SECRET" = "long-secret-value" ] && echo "got=present" || echo "got=absent"`,
			},
		},
	}

	// Consumer uses-step injects the secret from an ambient env_file var
	// via ${{ env.HARNESS_T10_AMBIENT }}.
	consumer := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "consumer-t10-satisfies", UseCaseID: "fake-uc",
		Angle: enums.AngleEnvironment, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{
				ID:   "step1",
				Uses: "core-env-prim",
				Env:  map[string]string{"HARNESS_T10_SECRET": "${{ env.HARNESS_T10_AMBIENT }}"},
			},
		},
	}

	// Write the env_file that provides HARNESS_T10_AMBIENT.
	repo := t.TempDir()
	envFile := filepath.Join(repo, ".env")
	if err := os.WriteFile(envFile, []byte("HARNESS_T10_AMBIENT=long-secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts := composeBaseOptions(t, uc, prim)
	opts.RepoRoot = repo
	opts.EnvFile = envFile
	ex := New(opts)
	runDir := t.TempDir()
	agg, err := ex.Run(context.Background(), consumer, runDir, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass (rollup=%+v)", agg.Verdict, agg.Rollup)
	}
	if agg.Rollup.TotalSignals == 0 {
		t.Error("zero signals: the pass matcher never fired, test would be vacuous")
	}
	// raw.log must not contain the literal secret value — it is redacted.
	rawBytes, _ := os.ReadFile(filepath.Join(runDir, "raw.log"))
	if strings.Contains(string(rawBytes), "long-secret-value") {
		t.Errorf("raw.log contains unredacted secret: %s", rawBytes)
	}
}
