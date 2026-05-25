package healloop

import (
	"context"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
	usecase "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
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
	llm := &stubLLM{plans: []EditPlan{{Files: []EditFile{{Path: "src/foo.go", Op: OpWrite, Content: "// fixed"}}}}}
	rev := &stubRevalidator{verdicts: []usecase.UseCaseVerdict{passing}}
	tx := &stubTransactor{}

	res, err := Run(context.Background(), failing, llm, tx, rev, Config{MaxIterations: 3})
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
	llm := &stubLLM{}
	res, err := Run(context.Background(), verdict, llm, &stubTransactor{}, &stubRevalidator{}, Config{MaxIterations: 3})
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
