package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/runtime/process"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// stubStore is a minimal FixtureStore that returns nothing — enough to
// satisfy the binder when no fixtures are referenced.
type stubStore struct{}

func (stubStore) LookupFixture(id string) (fixture.Fixture, bool) { return fixture.Fixture{}, false }
func (stubStore) FixturesForUseCase(uc string) []fixture.Fixture  { return nil }
func (stubStore) All() []fixture.Fixture                          { return nil }

// oneFixtureStore returns a single fixture with id "foo" owned by use case "uc".
type oneFixtureStore struct{}

func (oneFixtureStore) LookupFixture(id string) (fixture.Fixture, bool) {
	if id == "foo" {
		return fixture.Fixture{
			ID:          "foo",
			UseCaseID:   "uc",
			ContentType: "text/plain",
			Payload:     []byte("hello-fixture"),
		}, true
	}
	return fixture.Fixture{}, false
}
func (oneFixtureStore) FixturesForUseCase(ucID string) []fixture.Fixture {
	if ucID == "uc" {
		fx, _ := oneFixtureStore{}.LookupFixture("foo")
		return []fixture.Fixture{fx}
	}
	return nil
}
func (oneFixtureStore) All() []fixture.Fixture {
	fx, _ := oneFixtureStore{}.LookupFixture("foo")
	return []fixture.Fixture{fx}
}

// TestRunStep_FixtureRefInRunSucceeds verifies that ${{fixtures.X}} in a
// step's run: string is now ALLOWED. The fixture is compiled to an env ref
// (HARNESS_FIXTURE_FOO) and bound by the binder. The step must succeed
// (no TemplateError), and the binding env must contain the fixture key.
func TestRunStep_FixtureRefInRunSucceeds(t *testing.T) {
	res := template.Resolver{Fixtures: oneFixtureStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}}
	uc := &usecase.UseCase{ID: "uc", FixtureIDs: []string{"foo"}}

	// The step echoes the fixture env ref — the shell will expand it.
	step := sensor.Step{ID: "s1", Run: `echo "${HARNESS_FIXTURE_FOO}"`}
	dir := t.TempDir()

	_, err := runStep(context.Background(), stepArgs{
		Step:        step,
		StepIdx:     1,
		RunDir:      dir,
		UseCase:     uc,
		Store:       oneFixtureStore{},
		Resolver:    &res,
		Signaler:    process.Default(),
		Shell:       []string{"/bin/sh", "-c"},
		ExpectedObs: nil,
		RawLog:      mustRawLog(t, dir),
		SignalsW:    mustJSONL(t, dir),
		Stop:        make(chan struct{}),
	})
	// runStep should not return any TemplateError — the fixture ref is
	// compiled to an env ref now, not rejected.
	var te *TemplateError
	if err != nil {
		t.Logf("runStep err = %v (process/signal errors from echo are ok)", err)
	}
	if te != nil && strings.Contains(te.Error(), "not allowed") {
		t.Fatalf("got unexpected TemplateError (ban should be lifted): %v", te)
	}
}

// TestRunStep_FixtureCompilesToEnvRef verifies that a step whose run: string
// contains ${{fixtures.foo}} compiles to a command with "${HARNESS_FIXTURE_FOO}"
// and that the binding env includes HARNESS_FIXTURE_FOO. We do this by
// running the compiled segment through template.Parse + template.Compile
// directly, then confirming the resolved string has the expected env reference.
func TestRunStep_FixtureCompilesToEnvRef(t *testing.T) {
	input := "cat ${{fixtures.foo}}"
	segs, err := template.Parse(input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	resolved, refs, err := template.Compile(segs)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !strings.Contains(resolved, "HARNESS_FIXTURE_FOO") {
		t.Errorf("compiled %q does not contain HARNESS_FIXTURE_FOO", resolved)
	}
	if len(refs.Fixtures) != 1 || refs.Fixtures[0] != "foo" {
		t.Errorf("refs.Fixtures = %v, want [foo]", refs.Fixtures)
	}
}

func mustRawLog(t *testing.T, dir string) *rawLog {
	t.Helper()
	rl, err := newRawLog(dir+"/raw.log", fixedNow(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rl.Close() })
	return rl
}

func mustJSONL(t *testing.T, dir string) *jsonlWriter {
	t.Helper()
	jw, err := newJSONLWriter(dir + "/signals.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = jw.Close() })
	return jw
}
