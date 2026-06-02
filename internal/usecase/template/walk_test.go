package template

import (
	"testing"

	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/usecase/internal/fixturestub"
)

// TestWalkersCoverEveryGrammarForm parses each grammar form, then sends the
// segments through both Resolve and RenderLabels. Neither should error;
// neither should silently drop a segment.
func TestWalkersCoverEveryGrammarForm(t *testing.T) {
	cases := []string{
		"literal only",
		"${{fixtures.fx-a}}",
		"${{fixtures.fx-a.k}}",
		"${{fixtures.fx-a.k1.k2.k3}}",
		"${{entry_points.ep-a}}",
		"${{entry_points.ep-a.spec.method}}",
		"mix ${{fixtures.fx-a}} and ${{entry_points.ep-a}}",
	}

	r := &Resolver{
		Fixtures: fixturestub.New(map[string]string{
			"fx-a": `{"k":"v","k1":{"k2":{"k3":"deep"}}}`,
		}),
		EntryPoints: map[string]entrypoint.EntryPoint{
			"ep-a": {ID: "ep-a", Archetype: enums.ArchetypeHTTPAPI, Spec: map[string]any{"method": "GET"}},
		},
	}

	for _, in := range cases {
		segs, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q) err: %v", in, err)
			continue
		}
		if _, err := r.Resolve(segs); err != nil {
			t.Errorf("Resolve(%q) err: %v", in, err)
		}
		if got := RenderLabels(segs); got == "" {
			t.Errorf("RenderLabels(%q) empty", in)
		}
	}
}
