package fixturebinder

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
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

func makeFixture(t *testing.T, id, useCaseID, contentType string, payload []byte) fixture.Fixture {
	t.Helper()
	return fixture.Fixture{
		SchemaVersion: "1.0.0",
		ID:            id,
		UseCaseID:     useCaseID,
		Role:          fixture.RoleInput,
		ContentType:   contentType,
		Payload:       payload,
	}
}

type stubStore struct {
	byID map[string]fixture.Fixture
}

func (s *stubStore) LookupFixture(id string) (fixture.Fixture, bool) {
	f, ok := s.byID[id]
	return f, ok
}
func (s *stubStore) FixturesForUseCase(string) []fixture.Fixture { return nil }
func (s *stubStore) All() []fixture.Fixture                       { return nil }

func newStubStore(t *testing.T, fs ...fixture.Fixture) *stubStore {
	t.Helper()
	m := make(map[string]fixture.Fixture, len(fs))
	for _, f := range fs {
		m[f.ID] = f
	}
	return &stubStore{byID: m}
}

func TestBind_HappyPath_JSONAndBinary(t *testing.T) {
	scratch := t.TempDir()
	b := &Binder{ScratchDir: scratch}

	jsonFix := makeFixture(t, "login-basic", "uc-login", "application/json", []byte(`{"user":"alice"}`))
	binFix := makeFixture(t, "avatar-png", "uc-login", "image/png", []byte{0x89, 0x50, 0x4E, 0x47})

	store := newStubStore(t, jsonFix, binFix)
	uc := &usecase.UseCase{ID: "uc-login", FixtureIDs: []string{"login-basic", "avatar-png"}}
	step := sensor.Step{ID: "s1", Run: "true", Uses: []string{"avatar-png", "login-basic"}} // intentionally reversed

	binding, err := b.Bind(step, uc, store)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	// BoundIDs sorted ascending.
	wantBound := []string{"avatar-png", "login-basic"}
	if !reflect.DeepEqual(binding.BoundIDs, wantBound) {
		t.Errorf("BoundIDs = %v, want %v", binding.BoundIDs, wantBound)
	}

	// Env names canonical.
	wantEnv := map[string]string{
		"HARNESS_FIXTURE_LOGIN_BASIC": filepath.Join(scratch, "login-basic.json"),
		"HARNESS_FIXTURE_AVATAR_PNG":  filepath.Join(scratch, "avatar-png.bin"),
	}
	if !reflect.DeepEqual(binding.Env, wantEnv) {
		t.Errorf("Env = %v, want %v", binding.Env, wantEnv)
	}

	// Files map.
	wantFiles := map[string]string{
		"login-basic": filepath.Join(scratch, "login-basic.json"),
		"avatar-png":  filepath.Join(scratch, "avatar-png.bin"),
	}
	if !reflect.DeepEqual(binding.Files, wantFiles) {
		t.Errorf("Files = %v, want %v", binding.Files, wantFiles)
	}

	// File contents byte-equal to payloads.
	got, err := os.ReadFile(binding.Files["login-basic"])
	if err != nil {
		t.Fatalf("read login-basic: %v", err)
	}
	if !bytes.Equal(got, jsonFix.Payload) {
		t.Errorf("login-basic file contents = %q, want %q", string(got), string(jsonFix.Payload))
	}

	got, err = os.ReadFile(binding.Files["avatar-png"])
	if err != nil {
		t.Fatalf("read avatar-png: %v", err)
	}
	if !bytes.Equal(got, binFix.Payload) {
		t.Errorf("avatar-png file contents = %v, want %v", got, binFix.Payload)
	}
}

func TestBind_BoundIDsDeterministicAcrossCalls(t *testing.T) {
	b := &Binder{ScratchDir: t.TempDir()}
	fxA := makeFixture(t, "alpha", "uc", "application/json", []byte(`{}`))
	fxB := makeFixture(t, "bravo", "uc", "application/json", []byte(`{}`))
	fxC := makeFixture(t, "charlie", "uc", "application/json", []byte(`{}`))
	store := newStubStore(t, fxA, fxB, fxC)
	uc := &usecase.UseCase{ID: "uc", FixtureIDs: []string{"alpha", "bravo", "charlie"}}
	step := sensor.Step{Uses: []string{"charlie", "alpha", "bravo"}}

	first, err := b.Bind(step, uc, store)
	if err != nil {
		t.Fatalf("Bind first: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	if !reflect.DeepEqual(first.BoundIDs, want) {
		t.Errorf("first BoundIDs = %v, want %v", first.BoundIDs, want)
	}
	for i := 0; i < 5; i++ {
		again, err := b.Bind(step, uc, store)
		if err != nil {
			t.Fatalf("Bind iter %d: %v", i, err)
		}
		if !reflect.DeepEqual(again.BoundIDs, want) {
			t.Errorf("iter %d BoundIDs = %v, want %v", i, again.BoundIDs, want)
		}
	}
}
