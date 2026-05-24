package aggregate

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

func TestRollupIsByteDeterministic(t *testing.T) {
	signals := []signalstub.Signal{
		sig(enums.VerdictPass), sig(enums.VerdictPass), sig(enums.VerdictPass),
		sig(enums.VerdictWarn),
		sig(enums.VerdictFail), sig(enums.VerdictFail),
	}
	signals[3].HealHint = &HealHint{
		Summary: "w", Rationale: "r",
		SuggestedLocus: []Locus{{Path: "w.go", Symbol: "W"}},
	}
	signals[4].HealHint = &HealHint{
		Summary: "f1", Rationale: "r",
		SuggestedLocus: []Locus{{Path: "a.go", Symbol: "A"}, {Path: "b.go", Symbol: "B"}},
	}
	signals[5].HealHint = &HealHint{
		Summary: "f2", Rationale: "r",
		SuggestedLocus: []Locus{{Path: "a.go", Symbol: "A"}}, // duplicate
	}
	in := baseInput(signals)

	var first []byte
	for i := 0; i < 100; i++ {
		a, err := Rollup(in)
		if err != nil {
			t.Fatalf("Rollup iter %d: %v", i, err)
		}
		buf, err := json.Marshal(a)
		if err != nil {
			t.Fatalf("Marshal iter %d: %v", i, err)
		}
		if i == 0 {
			first = buf
			continue
		}
		if !bytes.Equal(first, buf) {
			t.Fatalf("iter %d diverged from first run:\n  first: %s\n  this:  %s", i, first, buf)
		}
	}
}
