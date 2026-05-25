package healloop

import (
	"context"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
	usecase "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
)

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
