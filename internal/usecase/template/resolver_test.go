package template

import (
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/usecase/internal/fixturestub"
)

func mkResolver(t *testing.T, fx map[string]string, eps []entrypoint.EntryPoint) *Resolver {
	t.Helper()
	epMap := make(map[string]entrypoint.EntryPoint, len(eps))
	for _, e := range eps {
		epMap[e.ID] = e
	}
	return &Resolver{Fixtures: fixturestub.New(fx), EntryPoints: epMap}
}

func TestResolveLiteralPassesThrough(t *testing.T) {
	r := mkResolver(t, nil, nil)
	segs := []Segment{Literal{Text: "hello world"}}
	got, err := r.Resolve(segs)
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q", got)
	}
}

func TestResolveBareFixtureReturnsWholePayload(t *testing.T) {
	r := mkResolver(t, map[string]string{
		"fx-a": `{"k":1}`,
	}, nil)
	segs, _ := Parse("payload=${{fixtures.fx-a}}")
	got, err := r.Resolve(segs)
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if !strings.Contains(got, `"k"`) || !strings.Contains(got, "1") {
		t.Errorf("got %q; want JSON containing k:1", got)
	}
}

func TestResolveJSONPathLeaf(t *testing.T) {
	r := mkResolver(t, map[string]string{
		"fx-order": `{"customer_id":"c-001","items":[{"sku":"A","qty":2}]}`,
	}, nil)
	segs, _ := Parse("${{fixtures.fx-order.customer_id}}")
	got, err := r.Resolve(segs)
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if got != "c-001" {
		t.Errorf("got %q, want c-001", got)
	}
}

func TestResolveJSONPathMissKey(t *testing.T) {
	r := mkResolver(t, map[string]string{"fx": `{"a":1}`}, nil)
	segs, _ := Parse("${{fixtures.fx.missing}}")
	_, err := r.Resolve(segs)
	if err == nil {
		t.Fatal("want error for missing key")
	}
	if _, ok := err.(*ResolveError); !ok {
		t.Errorf("want *ResolveError, got %T: %v", err, err)
	}
}

func TestResolveJSONPathCrossesScalar(t *testing.T) {
	r := mkResolver(t, map[string]string{"fx": `{"a":1}`}, nil)
	segs, _ := Parse("${{fixtures.fx.a.b}}")
	_, err := r.Resolve(segs)
	if err == nil {
		t.Fatal("want error when path crosses scalar")
	}
}

func TestResolveEntryPointBare(t *testing.T) {
	r := mkResolver(t, nil, []entrypoint.EntryPoint{
		{ID: "ep-create", Archetype: enums.ArchetypeHTTPAPI, Spec: map[string]any{}},
	})
	segs, _ := Parse("call ${{entry_points.ep-create}}")
	got, err := r.Resolve(segs)
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if got != "call http-api:ep-create" {
		t.Errorf("got %q", got)
	}
}

func TestResolveEntryPointSpecField(t *testing.T) {
	r := mkResolver(t, nil, []entrypoint.EntryPoint{
		{ID: "ep-create", Archetype: enums.ArchetypeHTTPAPI, Spec: map[string]any{"method": "POST"}},
	})
	segs, _ := Parse("${{entry_points.ep-create.spec.method}}")
	got, err := r.Resolve(segs)
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if got != "POST" {
		t.Errorf("got %q", got)
	}
}

func TestResolveEntryPointUnknownSpecKey(t *testing.T) {
	r := mkResolver(t, nil, []entrypoint.EntryPoint{
		{ID: "ep", Archetype: enums.ArchetypeHTTPAPI, Spec: map[string]any{"method": "GET"}},
	})
	segs, _ := Parse("${{entry_points.ep.spec.nope}}")
	_, err := r.Resolve(segs)
	if err == nil {
		t.Fatal("want error for unknown spec key")
	}
}

func TestResolveUnknownFixtureID(t *testing.T) {
	r := mkResolver(t, nil, nil)
	segs, _ := Parse("${{fixtures.fx-missing}}")
	_, err := r.Resolve(segs)
	if err == nil {
		t.Fatal("want error for unknown fixture")
	}
}

func TestResolveUnknownEntryPointID(t *testing.T) {
	r := mkResolver(t, nil, nil)
	segs, _ := Parse("${{entry_points.ep-missing}}")
	_, err := r.Resolve(segs)
	if err == nil {
		t.Fatal("want error for unknown entry_point")
	}
}
