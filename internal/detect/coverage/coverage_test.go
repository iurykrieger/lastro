package coverage

import (
	"testing"

	"github.com/iurykrieger/lastro/internal/detect/branchscan"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/usecase"
)

func inventory() *branchscan.Inventory {
	return &branchscan.Inventory{
		SchemaVersion: "1.0.0",
		SourceRoot:    ".",
		Branches: []branchscan.Branch{
			{ID: "br-aaaaaaaaaaaa", File: "h.go", Line: 5, Kind: enums.BranchIf, Condition: "err != nil"},
			{ID: "br-bbbbbbbbbbbb", File: "h.go", Line: 9, Kind: enums.BranchElse},
			{ID: "br-cccccccccccc", File: "h.go", Line: 14, Kind: enums.BranchCase, Condition: `"GET"`},
			{ID: "br-dddddddddddd", File: "h.go", Line: 16, Kind: enums.BranchDefault},
		},
	}
}

func uc(id, journey string, variation enums.UseCaseVariation, covers ...string) *usecase.UseCase {
	return &usecase.UseCase{ID: id, Journey: journey, Variation: variation, Covers: covers}
}

func TestCompute_PercentJourneysAndUncovered(t *testing.T) {
	report := Compute(inventory(), []*usecase.UseCase{
		uc("uc-ok", "orders", enums.VariationSuccess, "br-aaaaaaaaaaaa", "br-cccccccccccc"),
		uc("uc-bad", "orders", enums.VariationFailure, "br-aaaaaaaaaaaa"),
		uc("uc-alt", "orders", enums.VariationAlternative, "br-bbbbbbbbbbbb"),
	})

	if report.TotalBranches != 4 || report.CoveredBranches != 3 {
		t.Fatalf("total/covered = %d/%d, want 4/3", report.TotalBranches, report.CoveredBranches)
	}
	if report.CoveragePercent != 75.0 {
		t.Errorf("coverage_percent = %v, want 75.0", report.CoveragePercent)
	}

	if len(report.Journeys) != 1 {
		t.Fatalf("journeys = %+v, want exactly one", report.Journeys)
	}
	j := report.Journeys[0]
	if j.Journey != "orders" || j.UseCases != 3 || j.SuccessCount != 1 || j.FailureCount != 1 || j.AlternativeCount != 1 {
		t.Errorf("journey rollup = %+v", j)
	}

	if len(report.Uncovered) != 1 || report.Uncovered[0].ID != "br-dddddddddddd" {
		t.Errorf("uncovered = %+v, want only br-dddddddddddd", report.Uncovered)
	}
}

func TestCompute_UngroupedAndStaleRefs(t *testing.T) {
	report := Compute(inventory(), []*usecase.UseCase{
		uc("uc-legacy", "", "", "br-aaaaaaaaaaaa", "br-999999999999"), // stale ref ignored
	})

	if report.CoveredBranches != 1 {
		t.Errorf("covered = %d, want 1 (stale ref must not count)", report.CoveredBranches)
	}
	if report.CoveragePercent != 25.0 {
		t.Errorf("coverage_percent = %v, want 25.0", report.CoveragePercent)
	}
	if len(report.Journeys) != 1 || report.Journeys[0].Journey != UngroupedJourney {
		t.Errorf("journeys = %+v, want single %q bucket", report.Journeys, UngroupedJourney)
	}
}

func TestCompute_EmptyInventory(t *testing.T) {
	report := Compute(&branchscan.Inventory{SchemaVersion: "1.0.0"}, nil)
	if report.CoveragePercent != 0 || report.TotalBranches != 0 {
		t.Errorf("empty inventory report = %+v", report)
	}
	if report.Journeys == nil || report.Uncovered == nil {
		t.Error("journeys/uncovered must be empty slices, not nil (YAML arrays)")
	}
}
