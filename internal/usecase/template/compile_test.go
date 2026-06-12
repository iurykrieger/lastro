package template

import (
	"reflect"
	"testing"
)

func TestCompileRewritesRefsToEnvAndCollects(t *testing.T) {
	segs, _ := Parse(`curl -X ${{ inputs.method }} ${{ fixtures.body }} ${{ steps.create.outputs.id }}`)
	out, refs, err := Compile(segs)
	if err != nil {
		t.Fatalf("Compile err: %v", err)
	}
	want := `curl -X "${HARNESS_INPUT_METHOD}" "${HARNESS_FIXTURE_BODY}" "${HARNESS_STEPOUT_CREATE_ID}"`
	if out != want {
		t.Errorf("compiled = %q, want %q", out, want)
	}
	if !reflect.DeepEqual(refs.Inputs, []string{"method"}) ||
		!reflect.DeepEqual(refs.Fixtures, []string{"body"}) ||
		len(refs.StepOutputs) != 1 {
		t.Errorf("refs = %#v", refs)
	}
}

func TestCompileNeverInlinesValue(t *testing.T) {
	segs, _ := Parse(`echo ${{ fixtures.evil }}`)
	out, _, _ := Compile(segs)
	if out != `echo "${HARNESS_FIXTURE_EVIL}"` {
		t.Errorf("got %q", out)
	}
}

func TestCompileRejectsFixtureJSONPath(t *testing.T) {
	segs, _ := Parse(`x ${{ fixtures.body.user.name }}`)
	if _, _, err := Compile(segs); err == nil {
		t.Fatal("expected error: jsonpath drilling not allowed in sensor steps")
	}
}

func TestCompileDedupesAndOrders(t *testing.T) {
	segs, _ := Parse(`${{ inputs.a }} ${{ inputs.b }} ${{ inputs.a }}`)
	_, refs, _ := Compile(segs)
	if !reflect.DeepEqual(refs.Inputs, []string{"a", "b"}) {
		t.Errorf("inputs = %v (want first-seen, deduped)", refs.Inputs)
	}
}

func TestCompileRejectsEntryPointRef(t *testing.T) {
	segs, err := Parse(`${{ entry_points.x }}`)
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	if _, _, err := Compile(segs); err == nil {
		t.Fatal("expected error: entry_points.* is not supported in sensor steps yet")
	}
}

func TestCompile_EnvRef(t *testing.T) {
	segs, err := Parse(`echo "t=${{ env.MY_TOKEN }}" "${{ env.MY_TOKEN }}"`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, refs, err := Compile(segs)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	// Adjacent double-quoted strings concatenate in shell: "t=" + "${MY_TOKEN}" + ""
	// and "" + "${MY_TOKEN}" + "" each render as one word.
	want := `echo "t="${MY_TOKEN}"" ""${MY_TOKEN}""`
	if out != want {
		t.Errorf("compiled = %q, want %q", out, want)
	}
	if len(refs.Env) != 1 || refs.Env[0] != "MY_TOKEN" {
		t.Errorf("refs.Env = %v, want [MY_TOKEN] (deduped)", refs.Env)
	}
}
