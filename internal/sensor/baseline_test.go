package sensor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"
)

func TestLoadBaselines_AllFiveAngles(t *testing.T) {
	baselines, err := LoadBaselines()
	if err != nil {
		t.Fatalf("LoadBaselines: %v", err)
	}
	wantAngles := []enums.ValidationAngle{
		enums.AngleE2ETest, enums.AngleDatabase, enums.AnglePerformance,
		enums.AngleLogs, enums.AngleMetrics,
	}
	if len(baselines) != len(wantAngles) {
		t.Fatalf("got %d baselines, want %d: %v", len(baselines), len(wantAngles), baselines)
	}
	for _, a := range wantAngles {
		if _, ok := baselines[a]; !ok {
			t.Errorf("missing baseline for angle %q", a)
		}
	}
}

func TestLoadBaselines_E2EFloorContents(t *testing.T) {
	baselines, err := LoadBaselines()
	if err != nil {
		t.Fatalf("LoadBaselines: %v", err)
	}
	e2e := baselines[enums.AngleE2ETest]
	for _, name := range []string{
		"base_url", "method", "path", "query", "headers", "body",
		"expect_status", "timeout",
	} {
		spec, ok := e2e.Inputs[name]
		if !ok {
			t.Errorf("e2e-test baseline missing input %q", name)
			continue
		}
		if spec.Description == "" {
			t.Errorf("e2e-test baseline input %q has empty description", name)
		}
	}
}

func TestBaselineFiles_MatchMetaSchema(t *testing.T) {
	raw, err := schemas.FS.ReadFile("core-input-baseline.yaml")
	if err != nil {
		t.Fatalf("read meta-schema: %v", err)
	}
	asJSON, err := yaml.YAMLToJSON(raw)
	if err != nil {
		t.Fatalf("yaml->json meta-schema: %v", err)
	}
	var doc any
	if err := json.Unmarshal(asJSON, &doc); err != nil {
		t.Fatalf("unmarshal meta-schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	const url = "https://lastro.dev/harness/schemas/core-input-baseline.yaml"
	if err := c.AddResource(url, doc); err != nil {
		t.Fatalf("add resource: %v", err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		t.Fatalf("compile meta-schema: %v", err)
	}
	entries, err := schemas.FS.ReadDir("core-inputs")
	if err != nil {
		t.Fatalf("read core-inputs dir: %v", err)
	}
	for _, e := range entries {
		b, err := schemas.FS.ReadFile("core-inputs/" + e.Name())
		if err != nil {
			t.Errorf("read %s: %v", e.Name(), err)
			continue
		}
		j, err := yaml.YAMLToJSON(b)
		if err != nil {
			t.Errorf("yaml->json %s: %v", e.Name(), err)
			continue
		}
		var inst any
		if err := json.Unmarshal(j, &inst); err != nil {
			t.Errorf("unmarshal %s: %v", e.Name(), err)
			continue
		}
		if err := sch.Validate(inst); err != nil {
			t.Errorf("FAIL %s against meta-schema: %v", e.Name(), err)
		}
	}
}

// fullE2EInputs returns an Inputs map satisfying the e2e-test floor.
func fullE2EInputs() map[string]InputSpec {
	names := []string{
		"base_url", "method", "path", "query", "headers", "body",
		"expect_status", "timeout",
	}
	m := make(map[string]InputSpec, len(names))
	for _, n := range names {
		m[n] = InputSpec{HasDefault: true}
	}
	return m
}

func mustBaselines(t *testing.T) map[enums.ValidationAngle]CoreInputBaseline {
	t.Helper()
	baselines, err := LoadBaselines()
	if err != nil {
		t.Fatalf("LoadBaselines: %v", err)
	}
	return baselines
}

func TestValidateBaselineInputs_FullFloorPasses(t *testing.T) {
	s := Sensor{ID: "e2e-test", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest, Inputs: fullE2EInputs()}
	if err := ValidateBaselineInputs(s, mustBaselines(t)); err != nil {
		t.Fatalf("full floor should pass: %v", err)
	}
}

func TestValidateBaselineInputs_ExtraInputsStillPass(t *testing.T) {
	in := fullE2EInputs()
	in["idempotency_key"] = InputSpec{HasDefault: true}
	s := Sensor{ID: "e2e-test", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest, Inputs: in}
	if err := ValidateBaselineInputs(s, mustBaselines(t)); err != nil {
		t.Fatalf("floor is a minimum, extras must pass: %v", err)
	}
}

func TestValidateBaselineInputs_MissingInputFails(t *testing.T) {
	in := fullE2EInputs()
	delete(in, "headers")
	s := Sensor{ID: "e2e-test", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest, Inputs: in}
	err := ValidateBaselineInputs(s, mustBaselines(t))
	if err == nil {
		t.Fatal("missing baseline input must fail")
	}
	if !strings.Contains(err.Error(), "headers") {
		t.Fatalf("error should name the missing input, got: %v", err)
	}
}

func TestValidateBaselineInputs_MissingDefaultFails(t *testing.T) {
	in := fullE2EInputs()
	in["body"] = InputSpec{HasDefault: false}
	s := Sensor{ID: "e2e-test", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest, Inputs: in}
	err := ValidateBaselineInputs(s, mustBaselines(t))
	if err == nil {
		t.Fatal("baseline input without default must fail")
	}
	if !strings.Contains(err.Error(), "body") {
		t.Fatalf("error should name the undefaulted input, got: %v", err)
	}
}

func TestValidateBaselineInputs_SkipsNonCoreAndUnknownAngles(t *testing.T) {
	useCase := Sensor{ID: "s-x", Scope: enums.ScopeUseCase, UseCaseID: "uc", Angle: enums.AngleE2ETest}
	if err := ValidateBaselineInputs(useCase, mustBaselines(t)); err != nil {
		t.Fatalf("use-case scope must be skipped: %v", err)
	}
	env := Sensor{ID: "run-dev", Scope: enums.ScopeCore, Angle: enums.AngleEnvironment}
	if err := ValidateBaselineInputs(env, mustBaselines(t)); err != nil {
		t.Fatalf("angle without a baseline must be skipped: %v", err)
	}
}

func TestValidateInputReferences_AllReferencedPasses(t *testing.T) {
	s := Sensor{
		ID: "e2e-test", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest,
		Inputs: map[string]InputSpec{
			"method": {HasDefault: true},
			"path":   {HasDefault: true},
		},
		Steps: []Step{{
			ID:  "request",
			Run: `curl -X "${{ inputs.method }}" "http://x${{inputs.path}}"`,
		}},
	}
	if err := ValidateInputReferences(s); err != nil {
		t.Fatalf("all inputs referenced (with and without inner spaces): %v", err)
	}
}

func TestValidateInputReferences_UnreferencedFails(t *testing.T) {
	s := Sensor{
		ID: "e2e-test", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest,
		Inputs: map[string]InputSpec{
			"method":        {HasDefault: true},
			"expect_status": {HasDefault: true},
		},
		Steps: []Step{{ID: "request", Run: `curl -X "${{ inputs.method }}" http://x/`}},
	}
	err := ValidateInputReferences(s)
	if err == nil {
		t.Fatal("declared-but-ignored input must fail")
	}
	if !strings.Contains(err.Error(), "expect_status") {
		t.Fatalf("error should name the unreferenced input, got: %v", err)
	}
}

func TestValidateInputReferences_WithValueCounts(t *testing.T) {
	s := Sensor{
		ID: "wrapper", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest,
		Inputs: map[string]InputSpec{"path": {HasDefault: true}},
		Steps: []Step{{
			ID:   "inner",
			Uses: "e2e-test",
			With: map[string]string{"path": "${{ inputs.path }}"},
		}},
	}
	if err := ValidateInputReferences(s); err != nil {
		t.Fatalf("a reference inside a with value counts: %v", err)
	}
}

func TestValidateInputReferences_SkipsNonCore(t *testing.T) {
	s := Sensor{
		ID: "s-x", Scope: enums.ScopeUseCase, UseCaseID: "uc", Angle: enums.AngleE2ETest,
		Inputs: map[string]InputSpec{"ghost": {}},
		Steps:  []Step{{ID: "a", Run: "echo hi"}},
	}
	if err := ValidateInputReferences(s); err != nil {
		t.Fatalf("non-core sensors are skipped: %v", err)
	}
}

func TestValidateInputReferences_ZeroStepsFails(t *testing.T) {
	s := Sensor{
		ID: "e2e-test", Scope: enums.ScopeCore, Angle: enums.AngleE2ETest,
		Inputs: map[string]InputSpec{"base_url": {HasDefault: true}},
		Steps:  nil,
	}
	if err := ValidateInputReferences(s); err == nil {
		t.Fatal("inputs with no steps should fail: no step can reference them")
	}
}
