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
