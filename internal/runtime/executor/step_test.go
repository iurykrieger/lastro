package executor

import (
	"context"
	"errors"
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

func TestRunStep_RejectsFixtureRefInRun(t *testing.T) {
	res := template.Resolver{Fixtures: stubStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}}
	uc := &usecase.UseCase{ID: "uc"}
	step := sensor.Step{ID: "s1", Run: "echo ${{fixtures.foo}}"}
	dir := t.TempDir()

	_, err := runStep(context.Background(), stepArgs{
		Step:        step,
		StepIdx:     1,
		RunDir:      dir,
		UseCase:     uc,
		Store:       stubStore{},
		Resolver:    &res,
		Signaler:    process.Default(),
		Shell:       []string{"/bin/sh", "-c"},
		ExpectedObs: nil,
		RawLog:      mustRawLog(t, dir),
		SignalsW:    mustJSONL(t, dir),
		Stop:        nil,
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var te *TemplateError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want *TemplateError", err)
	}
	if !errors.Is(te.Cause, ErrTemplateFixtureInRun) {
		t.Errorf("inner err = %v, want ErrTemplateFixtureInRun", te.Cause)
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
