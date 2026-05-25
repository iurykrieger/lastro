package healloop

import (
	"context"
	"errors"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
	usecase "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	upkg "github.com/iurykrieger/lastro/internal/usecase"
)

func TestRun_HealsOnFirstIteration_WhenLLMProposesValidEdit(t *testing.T) {
	failing := usecase.UseCaseVerdict{
		UseCaseID: "uc-1",
		Archetype: enums.Archetype("http-api"),
		Verdict:   enums.VerdictFail,
	}
	passing := usecase.UseCaseVerdict{
		UseCaseID: "uc-1",
		Archetype: enums.Archetype("http-api"),
		Verdict:   enums.VerdictPass,
	}
	uc := &upkg.UseCase{ID: "uc-1"}
	llm := &stubLLM{plans: []EditPlan{{Files: []EditFile{{Path: "src/foo.go", Op: OpWrite, Content: "// fixed"}}}}}
	rev := &stubRevalidator{verdicts: []usecase.UseCaseVerdict{passing}}
	tx := &stubTransactor{}

	res, err := Run(context.Background(), uc, failing, llm, tx, rev, Config{MaxIterations: 3})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusHealed {
		t.Errorf("Status = %q, want %q", res.Status, StatusHealed)
	}
	if res.IterationsUsed != 1 {
		t.Errorf("IterationsUsed = %d, want 1", res.IterationsUsed)
	}
	if got, want := len(tx.snapshots), 1; got != want {
		t.Fatalf("snapshots = %d, want %d", got, want)
	}
	if !tx.snapshots[0].applied {
		t.Errorf("snapshot was not applied; loop must call handle.Apply between Snapshot and Revalidate")
	}
	if !tx.snapshots[0].committed {
		t.Errorf("snapshot was not committed")
	}
	if tx.snapshots[0].reverted {
		t.Errorf("snapshot was reverted; expected commit only")
	}
	if len(res.Attempts) != 1 {
		t.Fatalf("Attempts = %d, want 1", len(res.Attempts))
	}
	if res.Attempts[0].Reverted {
		t.Errorf("successful attempt has Reverted=true; want false")
	}
}

func TestRun_ShortCircuits_WhenInputAlreadyPassing(t *testing.T) {
	verdict := usecase.UseCaseVerdict{
		UseCaseID: "uc-1",
		Archetype: enums.Archetype("http-api"),
		Verdict:   enums.VerdictPass,
	}
	uc := &upkg.UseCase{ID: "uc-1"}
	llm := &stubLLM{}
	res, err := Run(context.Background(), uc, verdict, llm, &stubTransactor{}, &stubRevalidator{}, Config{MaxIterations: 3})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusHealed {
		t.Errorf("Status = %q, want %q", res.Status, StatusHealed)
	}
	if res.IterationsUsed != 0 {
		t.Errorf("IterationsUsed = %d, want 0", res.IterationsUsed)
	}
	if llm.calls != 0 {
		t.Errorf("llm.calls = %d, want 0", llm.calls)
	}
}

func TestRun_Exhausts_WhenLLMProposesBadEditRepeatedly(t *testing.T) {
	failing := usecase.UseCaseVerdict{
		UseCaseID: "uc-1",
		Archetype: enums.Archetype("http-api"),
		Verdict:   enums.VerdictFail,
	}
	uc := &upkg.UseCase{ID: "uc-1"}
	llm := &stubLLM{plans: []EditPlan{{Files: []EditFile{{Path: "src/foo.go", Op: OpWrite, Content: "// bad"}}}}}
	rev := &stubRevalidator{verdicts: []usecase.UseCaseVerdict{failing}}
	tx := &stubTransactor{}

	res, err := Run(context.Background(), uc, failing, llm, tx, rev, Config{MaxIterations: 3})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusExhausted {
		t.Errorf("Status = %q, want %q", res.Status, StatusExhausted)
	}
	if res.IterationsUsed != 3 {
		t.Errorf("IterationsUsed = %d, want 3", res.IterationsUsed)
	}
	if got := len(tx.snapshots); got != 3 {
		t.Errorf("snapshots = %d, want 3", got)
	}
	for i, h := range tx.snapshots {
		if !h.reverted {
			t.Errorf("snapshot %d not reverted", i)
		}
		if h.committed {
			t.Errorf("snapshot %d was committed; expected revert only", i)
		}
	}
	if llm.calls != 3 {
		t.Errorf("llm.calls = %d, want 3", llm.calls)
	}
	if len(res.Attempts) != 3 {
		t.Fatalf("Attempts = %d, want 3", len(res.Attempts))
	}
	for i, a := range res.Attempts {
		if !a.Reverted {
			t.Errorf("Attempts[%d].Reverted = false, want true", i)
		}
	}
}

func TestRun_Abandons_WhenLLMReturnsError(t *testing.T) {
	failing := usecase.UseCaseVerdict{
		UseCaseID: "uc-1",
		Archetype: enums.Archetype("http-api"),
		Verdict:   enums.VerdictFail,
	}
	uc := &upkg.UseCase{ID: "uc-1"}
	llm := &stubLLM{err: errStub}
	rev := &stubRevalidator{}
	tx := &stubTransactor{}

	res, err := Run(context.Background(), uc, failing, llm, tx, rev, Config{MaxIterations: 3})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusAbandoned {
		t.Errorf("Status = %q, want %q", res.Status, StatusAbandoned)
	}
	if res.IterationsUsed != 0 {
		t.Errorf("IterationsUsed = %d, want 0", res.IterationsUsed)
	}
	if res.Err == nil || !errors.Is(res.Err, errStub) {
		t.Errorf("Err = %v, want errors.Is(_, errStub)", res.Err)
	}
	if len(tx.snapshots) != 0 {
		t.Errorf("snapshots = %d, want 0", len(tx.snapshots))
	}
}

func TestRun_Abandons_WhenLLMReturnsEmptyPlan(t *testing.T) {
	failing := usecase.UseCaseVerdict{
		UseCaseID: "uc-1",
		Archetype: enums.Archetype("http-api"),
		Verdict:   enums.VerdictFail,
	}
	uc := &upkg.UseCase{ID: "uc-1"}
	llm := &stubLLM{plans: []EditPlan{{}}} // empty plan (no Files)
	rev := &stubRevalidator{}
	tx := &stubTransactor{}

	res, err := Run(context.Background(), uc, failing, llm, tx, rev, Config{MaxIterations: 3})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusAbandoned {
		t.Errorf("Status = %q, want %q", res.Status, StatusAbandoned)
	}
	if !errors.Is(res.Err, ErrLLMEmptyPlan) {
		t.Errorf("Err = %v, want errors.Is(_, ErrLLMEmptyPlan)", res.Err)
	}
	if len(tx.snapshots) != 0 {
		t.Errorf("snapshots = %d, want 0", len(tx.snapshots))
	}
}

func TestRun_Abandons_WhenEditPlanContainsEscapingPath(t *testing.T) {
	failing := usecase.UseCaseVerdict{
		UseCaseID: "uc-1",
		Archetype: enums.Archetype("http-api"),
		Verdict:   enums.VerdictFail,
	}
	cases := []struct {
		name string
		path string
	}{
		{"absolute", "/etc/passwd"},
		{"parent-dir-traversal", "../secrets/key.txt"},
		{"clean-yields-traversal", "src/../../secrets.txt"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc := &upkg.UseCase{ID: "uc-1"}
			llm := &stubLLM{plans: []EditPlan{{Files: []EditFile{{Path: tc.path, Op: OpWrite, Content: "x"}}}}}
			rev := &stubRevalidator{}
			tx := &stubTransactor{}
			res, err := Run(context.Background(), uc, failing, llm, tx, rev, Config{MaxIterations: 3})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if res.Status != StatusAbandoned {
				t.Errorf("Status = %q, want %q", res.Status, StatusAbandoned)
			}
			if !errors.Is(res.Err, ErrInvalidEditPath) {
				t.Errorf("Err = %v, want errors.Is(_, ErrInvalidEditPath)", res.Err)
			}
			if len(tx.snapshots) != 0 {
				t.Errorf("snapshots = %d, want 0", len(tx.snapshots))
			}
		})
	}
}
