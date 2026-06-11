// Package coverage scores detected use cases against the branch inventory:
// what fraction of the application's logic branches is exercised by at
// least one use case's `covers` list. The computation is pure and
// deterministic — same inventory + use cases → byte-identical report.
package coverage

import (
	"math"
	"sort"

	"github.com/iurykrieger/lastro/internal/detect/branchscan"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// UngroupedJourney is the bucket for use cases that declare no journey.
const UngroupedJourney = "ungrouped"

// JourneyReport rolls up one journey's use cases and coverage contribution.
type JourneyReport struct {
	Journey          string `json:"journey"`
	UseCases         int    `json:"use_cases"`
	SuccessCount     int    `json:"success_count"`
	FailureCount     int    `json:"failure_count"`
	AlternativeCount int    `json:"alternative_count"`
	CoveredBranches  int    `json:"covered_branches"`
}

// UncoveredBranch is a branch no use case covers — the actionable
// remainder for the detection skill.
type UncoveredBranch struct {
	ID        string           `json:"id"`
	File      string           `json:"file"`
	Line      int              `json:"line"`
	Kind      enums.BranchKind `json:"kind"`
	Condition string           `json:"condition,omitempty"`
	Enclosing string           `json:"enclosing,omitempty"`
}

// Report matches schemas/coverage-report.yaml.
type Report struct {
	SchemaVersion   string            `json:"schema_version"`
	TotalBranches   int               `json:"total_branches"`
	CoveredBranches int               `json:"covered_branches"`
	CoveragePercent float64           `json:"coverage_percent"`
	Journeys        []JourneyReport   `json:"journeys"`
	Uncovered       []UncoveredBranch `json:"uncovered"`
}

// Compute builds the coverage report. covers entries that do not exist in
// the inventory are ignored (persist already rejects them; a stale file
// must not crash the metric). Journeys are sorted by name; uncovered
// branches keep inventory order.
func Compute(inv *branchscan.Inventory, useCases []*usecase.UseCase) *Report {
	known := inv.IDs()
	covered := map[string]bool{}
	journeys := map[string]*JourneyReport{}

	for _, uc := range useCases {
		name := uc.Journey
		if name == "" {
			name = UngroupedJourney
		}
		jr := journeys[name]
		if jr == nil {
			jr = &JourneyReport{Journey: name}
			journeys[name] = jr
		}
		jr.UseCases++
		switch uc.Variation {
		case enums.VariationSuccess:
			jr.SuccessCount++
		case enums.VariationFailure:
			jr.FailureCount++
		case enums.VariationAlternative:
			jr.AlternativeCount++
		}

		journeyCovered := map[string]bool{}
		for _, id := range uc.Covers {
			if !known[id] {
				continue
			}
			covered[id] = true
			journeyCovered[id] = true
		}
		jr.CoveredBranches += len(journeyCovered)
	}

	report := &Report{
		SchemaVersion:   "1.0.0",
		TotalBranches:   len(inv.Branches),
		CoveredBranches: len(covered),
		Journeys:        []JourneyReport{},
		Uncovered:       []UncoveredBranch{},
	}
	if report.TotalBranches > 0 {
		pct := float64(report.CoveredBranches) / float64(report.TotalBranches) * 100
		report.CoveragePercent = math.Round(pct*10) / 10
	}

	names := make([]string, 0, len(journeys))
	for name := range journeys {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		report.Journeys = append(report.Journeys, *journeys[name])
	}

	for _, b := range inv.Branches {
		if covered[b.ID] {
			continue
		}
		report.Uncovered = append(report.Uncovered, UncoveredBranch{
			ID: b.ID, File: b.File, Line: b.Line,
			Kind: b.Kind, Condition: b.Condition, Enclosing: b.Enclosing,
		})
	}
	return report
}
