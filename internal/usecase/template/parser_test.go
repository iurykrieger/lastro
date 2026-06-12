package template

import (
	"reflect"
	"testing"
)

func TestParseLiteralOnly(t *testing.T) {
	got, err := Parse("plain text")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	want := []Segment{Literal{Text: "plain text"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestParseEmptyInput(t *testing.T) {
	got, err := Parse("")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty slice, got %#v", got)
	}
}

func TestParseFixtureRefBare(t *testing.T) {
	got, err := Parse("see ${{fixtures.fx-order}} here")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 segments, got %d: %#v", len(got), got)
	}
	if got[0] != (Literal{Text: "see "}) {
		t.Errorf("seg 0: %#v", got[0])
	}
	r, ok := got[1].(FixtureRef)
	if !ok {
		t.Fatalf("seg 1 not FixtureRef: %#v", got[1])
	}
	if r.ID != "fx-order" || len(r.JSONPath) != 0 {
		t.Errorf("FixtureRef: %#v", r)
	}
	if got[2] != (Literal{Text: " here"}) {
		t.Errorf("seg 2: %#v", got[2])
	}
}

func TestParseFixtureRefWithJSONPath(t *testing.T) {
	got, err := Parse("${{fixtures.fx-order.customer_id}}")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	r := got[0].(FixtureRef)
	if r.ID != "fx-order" {
		t.Errorf("ID: %q", r.ID)
	}
	if !reflect.DeepEqual(r.JSONPath, []string{"customer_id"}) {
		t.Errorf("JSONPath: %#v", r.JSONPath)
	}
}

func TestParseFixtureRefDeepJSONPath(t *testing.T) {
	got, err := Parse("${{fixtures.fx.user.address.city}}")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	r := got[0].(FixtureRef)
	if r.ID != "fx" {
		t.Errorf("ID: %q", r.ID)
	}
	if !reflect.DeepEqual(r.JSONPath, []string{"user", "address", "city"}) {
		t.Errorf("JSONPath: %#v", r.JSONPath)
	}
}

func TestParseAllowsWhitespaceInsideBraces(t *testing.T) {
	got, err := Parse("${{ fixtures.fx-a }}")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	r := got[0].(FixtureRef)
	if r.ID != "fx-a" {
		t.Errorf("ID: %q", r.ID)
	}
}

func TestParseEntryPointBare(t *testing.T) {
	got, err := Parse("${{entry_points.ep-create}}")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	r := got[0].(EntryPointRef)
	if r.ID != "ep-create" || r.SpecKey != "" {
		t.Errorf("EntryPointRef: %#v", r)
	}
}

func TestParseEntryPointSpecField(t *testing.T) {
	got, err := Parse("${{entry_points.ep-create.spec.method}}")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	r := got[0].(EntryPointRef)
	if r.ID != "ep-create" || r.SpecKey != "method" {
		t.Errorf("EntryPointRef: %#v", r)
	}
}

func TestParseMultipleRefsOnOneLine(t *testing.T) {
	got, err := Parse("A ${{fixtures.fx-one}} B ${{entry_points.ep-two}} C")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("want 5 segments, got %d", len(got))
	}
	if _, ok := got[1].(FixtureRef); !ok {
		t.Errorf("seg 1: %T", got[1])
	}
	if _, ok := got[3].(EntryPointRef); !ok {
		t.Errorf("seg 3: %T", got[3])
	}
}

func TestParseRejectsNested(t *testing.T) {
	_, err := Parse("${{ ${{fixtures.x}} }}")
	if err == nil {
		t.Fatal("want error for nested ${{")
	}
	if pe, ok := err.(*ParseError); !ok || pe.Msg == "" {
		t.Errorf("want *ParseError, got %T: %v", err, err)
	}
}

func TestParseRejectsUnclosed(t *testing.T) {
	_, err := Parse("text ${{fixtures.fx-a")
	if err == nil {
		t.Fatal("want error for unclosed ${{")
	}
}

func TestParseRejectsUnknownNamespace(t *testing.T) {
	_, err := Parse("${{stack.something}}")
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Msg != "unknown namespace: stack" {
		t.Errorf("msg: %q", pe.Msg)
	}
}

func TestParseRejectsBadSpecAccess(t *testing.T) {
	_, err := Parse("${{entry_points.ep-x.archetype}}")
	if err == nil {
		t.Fatal("want error for non-spec entry-point access")
	}
}

func TestParseRejectsSpecMultiKey(t *testing.T) {
	_, err := Parse("${{entry_points.ep-x.spec.a.b}}")
	if err == nil {
		t.Fatal("want error for multi-key spec access")
	}
}

func TestParseRejectsBadID(t *testing.T) {
	_, err := Parse("${{fixtures.Bad_ID}}")
	if err == nil {
		t.Fatal("want error for invalid id charset")
	}
}

func TestParseErrorPositionIsAccurate(t *testing.T) {
	_, err := Parse("line one\nline two ${{stack.x}}")
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Pos.Line != 2 {
		t.Errorf("line: %d, want 2", pe.Pos.Line)
	}
}

func TestParseInputRef(t *testing.T) {
	got, err := Parse("x ${{ inputs.method }} y")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	want := []Segment{Literal{Text: "x "}, InputRef{Name: "method", Pos: Position{Line: 1, Col: 7, Offset: 6}}, Literal{Text: " y"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}

func TestParseInputRefSnakeCase(t *testing.T) {
	// Input names are snake_case across the framework (golden example
	// declares `expect_status`; core sensors use `base_url`). The parser
	// must read the underscore, not stop at it. Regression for #32.
	got, err := Parse("${{ inputs.base_url }}")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	ref, ok := got[0].(InputRef)
	if !ok || ref.Name != "base_url" {
		t.Errorf("got %#v, want InputRef{Name: base_url}", got[0])
	}
}

func TestParseAdjacentInputRefs(t *testing.T) {
	// Two back-to-back `}}${{` expressions on one line — the shape from the
	// core e2e-test/performance run scripts. Regression for #32.
	got, err := Parse(`url="${{ inputs.base_url }}${{ inputs.path }}"`)
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	want := []Segment{
		Literal{Text: `url="`},
		InputRef{Name: "base_url", Pos: Position{Line: 1, Col: 10, Offset: 9}},
		InputRef{Name: "path", Pos: Position{Line: 1, Col: 32, Offset: 31}},
		Literal{Text: `"`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}

func TestParseStepOutputRef(t *testing.T) {
	got, err := Parse("${{ steps.create.outputs.charge-id }}")
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	ref, ok := got[0].(StepOutputRef)
	if !ok || ref.StepID != "create" || ref.Name != "charge-id" {
		t.Errorf("got %#v", got[0])
	}
}

func TestParseStepOutputRefRequiresOutputs(t *testing.T) {
	if _, err := Parse("${{ steps.create.charge-id }}"); err == nil {
		t.Fatal("expected error for missing 'outputs' segment")
	}
}

func TestParse_EnvRef(t *testing.T) {
	segs, err := Parse("${{ env.NEXTAUTH_SECRET }}")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(segs) != 1 {
		t.Fatalf("segments = %d, want 1", len(segs))
	}
	ref, ok := segs[0].(EnvRef)
	if !ok {
		t.Fatalf("segment type = %T, want EnvRef", segs[0])
	}
	if ref.Name != "NEXTAUTH_SECRET" {
		t.Errorf("name = %q, want NEXTAUTH_SECRET", ref.Name)
	}
}

func TestParse_EnvRefRejectsBadNames(t *testing.T) {
	for _, in := range []string{
		"${{ env.lower }}",     // lowercase not an env var name
		"${{ env.NAME.more }}", // no further keys
		"${{ env. }}",          // empty name
	} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q): expected error, got nil", in)
		}
	}
}
