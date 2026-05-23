package aggregate

import (
	"reflect"
	"testing"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

func TestRollupSingleShotCarriesOverHealHint(t *testing.T) {
	s := sig(enums.VerdictFail)
	s.HealHint = &HealHint{
		Summary:        "fix broken handler",
		SuggestedLocus: []Locus{{Path: "src/handler.go", Symbol: "Handle"}},
		Rationale:      "handler panics on empty input",
	}
	in := baseInput([]signalstub.Signal{s})
	in.OutputType = enums.OutputSingleShot
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.HealHint == nil {
		t.Fatal("HealHint must not be nil for fail verdict")
	}
	if !reflect.DeepEqual(*got.HealHint, *s.HealHint) {
		t.Errorf("HealHint = %+v, want %+v", *got.HealHint, *s.HealHint)
	}
}
