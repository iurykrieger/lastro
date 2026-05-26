package healloop

import (
	"errors"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
	usecase "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	upkg "github.com/iurykrieger/lastro/internal/usecase"
)

func TestBuildPromptInput_LooksUpUseCaseAndPopulatesHints(t *testing.T) {
	verdict := usecase.UseCaseVerdict{
		UseCaseID: "uc-1",
		Archetype: enums.Archetype("http-api"),
		Verdict:   enums.VerdictFail,
		HealHints: []usecase.AngleHint{{Angle: enums.AngleBuild, Verdict: enums.VerdictFail}},
	}
	ucs := &stubUseCaseLookup{uc: &upkg.UseCase{ID: "uc-1"}}
	history := []Attempt{{Iteration: 1, Reverted: true}}

	in, err := BuildPromptInput(verdict, history, ucs)
	if err != nil {
		t.Fatalf("BuildPromptInput: %v", err)
	}
	if in.UseCase == nil || in.UseCase.ID != "uc-1" {
		t.Errorf("UseCase = %v, want id uc-1", in.UseCase)
	}
	if len(in.Hints) != 1 || in.Hints[0].Angle != enums.AngleBuild {
		t.Errorf("Hints = %v, want one AngleBuild hint", in.Hints)
	}
	if len(in.History) != 1 {
		t.Errorf("History = %v, want one entry", in.History)
	}
}

func TestBuildPromptInput_ReturnsErrUseCaseNotFound(t *testing.T) {
	verdict := usecase.UseCaseVerdict{UseCaseID: "missing"}
	ucs := &stubUseCaseLookup{uc: nil}
	_, err := BuildPromptInput(verdict, nil, ucs)
	if !errors.Is(err, ErrUseCaseNotFound) {
		t.Errorf("err = %v, want ErrUseCaseNotFound", err)
	}
}
