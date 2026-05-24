package fixturebinder

import (
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)

func TestBindError_ImplementsError(t *testing.T) {
	e := &BindError{Code: "fixture-not-found", FixtureID: "login", UseCaseID: "uc-login"}
	msg := e.Error()
	for _, want := range []string{"fixturebinder", "fixture-not-found", "login", "uc-login"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, missing %q", msg, want)
		}
	}
}

func TestBindError_UnwrapReturnsCause(t *testing.T) {
	cause := &errStub{msg: "disk full"}
	e := &BindError{Code: "write-failed", Cause: cause}
	if e.Unwrap() != cause {
		t.Errorf("Unwrap() = %v, want %v", e.Unwrap(), cause)
	}
}

func TestBind_EmptyUsesReturnsEmptyBinding(t *testing.T) {
	b := &Binder{ScratchDir: t.TempDir()}
	step := sensor.Step{ID: "step-1", Run: "true", Uses: nil}
	uc := &usecase.UseCase{ID: "uc-login", FixtureIDs: []string{"login-basic"}}
	// Empty step.Uses never queries the store; nil is safe.
	binding, err := b.Bind(step, uc, nil)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if binding.Env == nil || len(binding.Env) != 0 {
		t.Errorf("Env = %v, want empty non-nil map", binding.Env)
	}
	if binding.Files == nil || len(binding.Files) != 0 {
		t.Errorf("Files = %v, want empty non-nil map", binding.Files)
	}
	if binding.BoundIDs == nil || len(binding.BoundIDs) != 0 {
		t.Errorf("BoundIDs = %v, want empty non-nil slice", binding.BoundIDs)
	}
}

type errStub struct{ msg string }

func (e *errStub) Error() string { return e.msg }

// Ensure fixture.FixtureStore is referenced so the import is used.
var _ fixture.FixtureStore = (*fixture.Store)(nil)
