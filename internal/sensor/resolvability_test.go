package sensor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/persisterror"
)

// fakeLookPath returns a LookPath func that resolves only the given names.
func fakeLookPath(known ...string) func(string) (string, error) {
	set := map[string]bool{}
	for _, k := range known {
		set[k] = true
	}
	return func(name string) (string, error) {
		if set[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
}

func sensorWithRun(run string) Sensor {
	return Sensor{
		ID:        "s-test",
		UseCaseID: "uc-test",
		Steps:     []Step{{ID: "check", Run: run}},
	}
}

func TestValidateStepResolvability_UnresolvableBinaryFails(t *testing.T) {
	s := sensorWithRun("harness probe http --url http://localhost:8080")
	env := ResolvabilityEnv{LookPath: fakeLookPath("go", "curl")}

	err := ValidateStepResolvability(s, env)

	if err == nil {
		t.Fatal("expected error for unresolvable command \"harness\", got nil")
	}
	if !strings.Contains(err.Error(), "harness") {
		t.Errorf("error should name the unresolvable command, got: %v", err)
	}
	if !strings.Contains(err.Error(), "check") {
		t.Errorf("error should name the step id, got: %v", err)
	}
}

func TestValidateStepResolvability_ResolvableBinaryPasses(t *testing.T) {
	s := sensorWithRun("go test ./...")
	env := ResolvabilityEnv{LookPath: fakeLookPath("go")}

	if err := ValidateStepResolvability(s, env); err != nil {
		t.Fatalf("expected nil for resolvable command, got: %v", err)
	}
}

func TestValidateStepResolvability_SkipsNonBinaryHeads(t *testing.T) {
	cases := []struct {
		name string
		run  string
	}{
		{"shell builtin", "cd /tmp && go build ./..."},
		{"script-defined function", "probe() { go vet ./...; }\nprobe"},
		{"variable head", `$RUNNER test ./...`},
		{"template ref in word", `curl -sS --data @${{ fixtures.payload }} http://localhost:8080`},
		{"quoted template-only word", `cat "${{ fixtures.payload }}"`},
	}
	env := ResolvabilityEnv{LookPath: fakeLookPath("go", "curl", "cat")}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateStepResolvability(sensorWithRun(tc.run), env); err != nil {
				t.Fatalf("expected nil, got: %v", err)
			}
		})
	}
}

func TestValidateStepResolvability_ChecksStepsContainingTemplateRefs(t *testing.T) {
	s := sensorWithRun(`harness db-get --key ${{ fixtures.charge-id }}`)
	env := ResolvabilityEnv{LookPath: fakeLookPath("go")}

	err := ValidateStepResolvability(s, env)

	if err == nil {
		t.Fatal("template refs must not exempt a step from resolvability checks")
	}
	if !strings.Contains(err.Error(), "harness") {
		t.Errorf("error should name the unresolvable command, got: %v", err)
	}
}

func TestValidateStepResolvability_FindsBadHeadAmongPipedCommands(t *testing.T) {
	s := sensorWithRun("curl -sS http://localhost:9090/metrics | harness assert-metric latency_p99\ngo vet ./...")
	env := ResolvabilityEnv{LookPath: fakeLookPath("curl", "go")}

	err := ValidateStepResolvability(s, env)

	if err == nil {
		t.Fatal("expected error for unresolvable command in pipeline, got nil")
	}
	if !strings.Contains(err.Error(), "harness") {
		t.Errorf("error should name the unresolvable command, got: %v", err)
	}
	if strings.Contains(err.Error(), `"curl"`) || strings.Contains(err.Error(), `"go"`) {
		t.Errorf("resolvable commands must not be reported, got: %v", err)
	}
}

func writeMakefile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestValidateStepResolvability_MakeTargetMissingFails(t *testing.T) {
	root := writeMakefile(t, "build:\n\tgo build ./...\n\ntest:\n\tgo test ./...\n")
	s := sensorWithRun("make dev")
	env := ResolvabilityEnv{LookPath: fakeLookPath("make"), RepoRoot: root}

	err := ValidateStepResolvability(s, env)

	if err == nil {
		t.Fatal("expected error for missing make target \"dev\", got nil")
	}
	if !strings.Contains(err.Error(), "dev") {
		t.Errorf("error should name the missing target, got: %v", err)
	}
}

func TestValidateStepResolvability_MakeTargetPresentPasses(t *testing.T) {
	root := writeMakefile(t, "build:\n\tgo build ./...\n")
	s := sensorWithRun("make build")
	env := ResolvabilityEnv{LookPath: fakeLookPath("make"), RepoRoot: root}

	if err := ValidateStepResolvability(s, env); err != nil {
		t.Fatalf("expected nil for existing make target, got: %v", err)
	}
}

func TestValidateStepResolvability_MakeWithDirOrFileFlagSkipsTargetCheck(t *testing.T) {
	root := writeMakefile(t, "build:\n\tgo build ./...\n")
	env := ResolvabilityEnv{LookPath: fakeLookPath("make"), RepoRoot: root}
	for _, run := range []string{"make -C sub dev", "make -f other.mk dev", "make"} {
		if err := ValidateStepResolvability(sensorWithRun(run), env); err != nil {
			t.Fatalf("%q: expected nil (target check not statically decidable), got: %v", run, err)
		}
	}
}

func TestPersist_StepResolvability_RejectsUnresolvableCommand(t *testing.T) {
	dir := seedHappyPath(t, t.TempDir())

	s := []byte(`schema_version: 1.0.0
id: s-bad-command
use_case_id: create-order
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [express]
steps:
  - id: probe
    run: definitely-not-a-real-binary-xyz assert-http --status 200
`)
	err := Persist(s, dir)
	if err == nil {
		t.Fatal("expected error for unresolvable command, got nil")
	}
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *persisterror.Error, got %T: %v", err, err)
	}
	if pe.Kind != persisterror.StepResolvability {
		t.Fatalf("expected Kind %q, got %q", persisterror.StepResolvability, pe.Kind)
	}
	if !strings.Contains(pe.Message, "definitely-not-a-real-binary-xyz") {
		t.Errorf("message should name the unresolvable command, got: %s", pe.Message)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "sensors", "create-order", "s-bad-command.yaml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("sensor file must not be written on resolvability failure")
	}
}

func TestPersist_StepResolvability_AppliesToCoreSensors(t *testing.T) {
	dir := seedHappyPath(t, t.TempDir())

	s := []byte(`schema_version: 1.0.0
id: core-bad-command
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [express]
steps:
  - id: probe
    run: definitely-not-a-real-binary-xyz check
`)
	err := Persist(s, dir)
	var pe *persisterror.Error
	if !errors.As(err, &pe) || pe.Kind != persisterror.StepResolvability {
		t.Fatalf("expected step_resolvability error for core sensor, got: %v", err)
	}
}
