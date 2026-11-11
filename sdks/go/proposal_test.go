package lastro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAddsNumberAfterReservedUseCaseIDs(t *testing.T) {
	var out ProposeOut
	b, err := os.ReadFile(filepath.Join("..", "..", "apps", "docs", "examples", "shipping", "answers", "propose.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	prose, _ := os.ReadFile(filepath.Join("..", "..", "apps", "docs", "examples", "shipping", "prose.md"))
	in := ProposeInput{ID: "P-001", Prose: string(prose), SubmittedBy: "ana", Policy: DefaultPolicy}
	first, err := ProposalFromExtraction(in, out)
	if err != nil {
		t.Fatal(err)
	}
	if got := ReservedUseCaseIDs([]Proposal{first}, ""); !reflect.DeepEqual(got, []string{"UC-001"}) {
		t.Fatal(got)
	}
	if got := ReservedUseCaseIDs([]Proposal{first}, "P-001"); len(got) != 0 {
		t.Fatal("a proposal does not reserve against itself", got)
	}
	in.ID, in.Reserved = "P-002", ReservedUseCaseIDs([]Proposal{first}, "P-002")
	second, err := ProposalFromExtraction(in, out)
	if err != nil {
		t.Fatal(err)
	}
	if second.UseCases[0].UseCase != "UC-002" {
		t.Fatal(second.UseCases)
	}
}
