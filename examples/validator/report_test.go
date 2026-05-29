package validator

import "testing"

func TestReportAllPassed(t *testing.T) {
	r := &Report{Summary: Summary{Total: 3, Passed: 3}}
	if !r.AllPassed() {
		t.Fatalf("AllPassed: want true when Passed==Total>0, got false")
	}

	r2 := &Report{Summary: Summary{Total: 3, Passed: 2, Failed: 1}}
	if r2.AllPassed() {
		t.Fatalf("AllPassed: want false when any failed")
	}

	r3 := &Report{Summary: Summary{Total: 0}}
	if r3.AllPassed() {
		t.Fatalf("AllPassed: want false when Total==0 (empty report is not a passing report)")
	}
}

func TestReportFailed(t *testing.T) {
	r := &Report{UseCases: []UseCaseResult{
		{UseCaseID: "a", Verdict: "pass"},
		{UseCaseID: "b", Verdict: "fail"},
		{UseCaseID: "c", Verdict: "inconclusive"},
		{UseCaseID: "d", Verdict: "fail"},
	}}
	failed := r.Failed()
	if len(failed) != 2 {
		t.Fatalf("Failed: want 2, got %d", len(failed))
	}
	if failed[0].UseCaseID != "b" || failed[1].UseCaseID != "d" {
		t.Fatalf("Failed: want [b,d], got %+v", failed)
	}
}

func TestReportFailedEmpty(t *testing.T) {
	r := &Report{UseCases: []UseCaseResult{{UseCaseID: "a", Verdict: "pass"}}}
	failed := r.Failed()
	if failed == nil {
		t.Fatalf("Failed: must return non-nil empty slice, got nil")
	}
	if len(failed) != 0 {
		t.Fatalf("Failed: want empty, got %+v", failed)
	}
}
