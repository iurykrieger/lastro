package template

import "testing"

func TestRenderLabelsBareFixture(t *testing.T) {
	segs, _ := Parse("a ${{fixtures.fx-order}} b")
	if got := RenderLabels(segs); got != "a [fixture: fx-order] b" {
		t.Errorf("got %q", got)
	}
}

func TestRenderLabelsFixtureJSONPath(t *testing.T) {
	segs, _ := Parse("see ${{fixtures.fx.user.name}}")
	if got := RenderLabels(segs); got != "see [fixture: fx.user.name]" {
		t.Errorf("got %q", got)
	}
}

func TestRenderLabelsBareEntryPoint(t *testing.T) {
	segs, _ := Parse("${{entry_points.ep-create}}")
	if got := RenderLabels(segs); got != "[entry: ep-create]" {
		t.Errorf("got %q", got)
	}
}

func TestRenderLabelsEntryPointSpec(t *testing.T) {
	segs, _ := Parse("${{entry_points.ep-create.spec.method}}")
	if got := RenderLabels(segs); got != "[entry: ep-create.spec.method]" {
		t.Errorf("got %q", got)
	}
}

func TestRenderLabelsInputAndStepOutput(t *testing.T) {
	segs, _ := Parse("${{ inputs.method }} ${{ steps.create.outputs.id }}")
	if got := RenderLabels(segs); got != "[input: method] [step: create.outputs.id]" {
		t.Errorf("got %q", got)
	}
}
