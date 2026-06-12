package executor

import (
	"strings"
	"testing"
)

func TestResolveStepEnv_LiteralAndRefs(t *testing.T) {
	view := envView{ambient: map[string]string{"AMBIENT_SECRET": "shhh-value"}}
	stepOut := map[string]string{stepOutEnvName("auth", "header"): "Cookie: tok=1"}
	resolved, refDerived, missing, err := resolveStepEnv(
		map[string]string{
			"NODE_ENV":  "test",
			"SECRET":    "${{ env.AMBIENT_SECRET }}",
			"AUTH_HDR":  "${{ steps.auth.outputs.header }}",
			"COMPOSITE": "pre-${{ env.AMBIENT_SECRET }}-post",
		},
		view, nil, stepOut, nil)
	if err != nil || len(missing) != 0 {
		t.Fatalf("err=%v missing=%v", err, missing)
	}
	if resolved["NODE_ENV"] != "test" || refDerived["NODE_ENV"] {
		t.Errorf("literal: %q refDerived=%v (want test, false)", resolved["NODE_ENV"], refDerived["NODE_ENV"])
	}
	if resolved["SECRET"] != "shhh-value" || !refDerived["SECRET"] {
		t.Errorf("env ref: %q refDerived=%v (want shhh-value, true)", resolved["SECRET"], refDerived["SECRET"])
	}
	if resolved["AUTH_HDR"] != "Cookie: tok=1" {
		t.Errorf("step output ref: %q", resolved["AUTH_HDR"])
	}
	if resolved["COMPOSITE"] != "pre-shhh-value-post" {
		t.Errorf("composite: %q", resolved["COMPOSITE"])
	}
}

func TestResolveStepEnv_MissingAndEmptyCollect(t *testing.T) {
	view := envView{ambient: map[string]string{"EMPTY_ONE": ""}}
	resolved, _, missing, err := resolveStepEnv(
		map[string]string{
			"A": "${{ env.TOTALLY_ABSENT_XYZ }}",
			"B": "${{ env.EMPTY_ONE }}",
		},
		view, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 2 || missing[0] != "EMPTY_ONE" || missing[1] != "TOTALLY_ABSENT_XYZ" {
		t.Errorf("missing = %v, want [EMPTY_ONE TOTALLY_ABSENT_XYZ]", missing)
	}
	if _, ok := resolved["A"]; ok {
		t.Error("partially-resolved entry must not be injected")
	}
}

func TestResolveStepEnv_EntryPointRejected(t *testing.T) {
	_, _, _, err := resolveStepEnv(
		map[string]string{"X": "${{ entry_points.create-thing }}"},
		envView{}, nil, nil, nil)
	if err == nil {
		t.Error("entry_points.* must be rejected in env values")
	}
}

func TestResolveStepEnv_InputRefUnboundErrors(t *testing.T) {
	// inputs.* refs are only valid inside primitive inner steps where
	// inputEnv is populated by the caller. At consumer (sensor) level
	// inputEnv is nil, so any inputs.* ref must be an error — not silently
	// resolved to empty — so compose.go can rely on that invariant.
	_, _, _, err := resolveStepEnv(
		map[string]string{"X": "${{ inputs.foo }}"},
		envView{}, nil /* inputEnv = nil */, nil, nil)
	if err == nil {
		t.Error("inputs.* with nil inputEnv must return an error")
	}
	if err != nil && !strings.Contains(err.Error(), "foo") {
		t.Errorf("error should mention the input name %q, got: %v", "foo", err)
	}
}
