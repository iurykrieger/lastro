package lastro

import (
	"context"
	"encoding/json"
	"fmt"
)

// The LLM steps of proposal/v1: extract (a proposal from prose, and the recombination of a use
// case's description) and derive (proofs for the approved intents). Prompts and schemas are
// generated from the TS SDK (prompts.go), and the user turns are built byte for byte like the
// TS SDK's, so both CLIs ask the same questions and find each other's handoff answers.

func proofView(u UseCase) []obj {
	out := []obj{}
	for _, p := range u.Proofs {
		formal := ""
		if p.Check.Kind == "clause" {
			if p.Check.Requires != nil {
				formal = Show(*p.Check.Requires) + " => "
			}
			formal += Show(*p.Check.Ensures)
		} else {
			formal = Stringify(p.Check.Input) + " => " + Show(*p.Check.Expect)
		}
		out = append(out, obj{{"id", p.ID}, {"name", p.Name}, {"description", p.Description}, {"formal", formal}})
	}
	return out
}

// ProposeUserPrompt is the user turn of the propose step: the use cases and the prose.
func ProposeUserPrompt(prose string, useCases []UseCase) string {
	cases := []obj{}
	for _, u := range useCases {
		o := obj{{"slug", u.Slug}, {"name", u.Name}, {"description", u.Description}}
		if len(u.Proofs) > 0 {
			o = append(o, kv{"proofs", proofView(u)})
		}
		cases = append(cases, o)
	}
	return "<use-cases>\n" + StringifyIndent(cases) + "\n</use-cases>\n\n<prose>\n" + prose + "\n</prose>"
}

func askInto(ctx context.Context, ask AskJSON, req AskRequest, out any) error {
	raw, err := ask(ctx, req)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s: unreadable answer: %w", req.Step, err)
	}
	return nil
}

// ExtractProposal asks the LLM for a proposal and checks its answer.
func ExtractProposal(ctx context.Context, ask AskJSON, in ProposeInput) (Proposal, error) {
	var out ProposeOut
	if err := askInto(ctx, ask, AskRequest{Step: "extract", System: ProposeSystem, User: ProposeUserPrompt(in.Prose, in.UseCases), Schema: json.RawMessage(ProposeSchema)}, &out); err != nil {
		return Proposal{}, err
	}
	return ProposalFromExtraction(in, out)
}

// RecombineUserPrompt is the user turn of a recombination: the use case, its remaining intents and the reviewer's instructions.
func RecombineUserPrompt(p Proposal, useCase, instructions string) (string, error) {
	for _, c := range p.UseCases {
		if c.UseCase != useCase && c.Slug != useCase {
			continue
		}
		intents := []obj{}
		for _, i := range p.Intents {
			if contains(c.Intents, i.ID) && i.Status != "rejected" {
				intents = append(intents, obj{{"id", i.ID}, {"change", i.Change}, {"description", i.Description}})
			}
		}
		o := obj{{"useCase", c.UseCase}, {"name", c.Name}, {"before", c.Before}, {"intents", intents}}
		if instructions != "" {
			o = append(o, kv{"instructions", instructions})
		}
		return StringifyIndent(o), nil
	}
	return "", fmt.Errorf("%s is not touched by %s", useCase, p.ID)
}

// Recombined is a use case's new name and description.
type Recombined struct {
	Name  string `json:"name"`
	After string `json:"after"`
}

// RecombineAfter asks the LLM to rewrite a use case's description from its remaining intents.
func RecombineAfter(ctx context.Context, ask AskJSON, p Proposal, useCase, instructions string) (Recombined, error) {
	user, err := RecombineUserPrompt(p, useCase, instructions)
	if err != nil {
		return Recombined{}, err
	}
	var out Recombined
	err = askInto(ctx, ask, AskRequest{Step: "extract", System: RecombineSystem, User: user, Schema: json.RawMessage(RecombineSchema)}, &out)
	return out, err
}

// DeriveProofsUserPrompt is the user turn of the derive step: models, subjects, the approved intents,
// the use cases they touch with their proofs, the decisions and what is not promised.
func DeriveProofsUserPrompt(p Proposal, useCases []UseCase, setup ProofSetup) string {
	forward := ForwardIntents(p)
	touched := map[string]bool{}
	approved := []obj{}
	for _, i := range forward {
		touched[i.UseCase] = true
		o := obj{{"id", i.ID}, {"change", i.Change}, {"useCase", i.UseCase}, {"description", i.Description}, {"provability", i.Provability}}
		if i.BeyondProofReason != "" {
			o = append(o, kv{"beyondProofReason", i.BeyondProofReason})
		}
		approved = append(approved, append(o, kv{"replaces", nonNil(i.Replaces)}))
	}
	cases := []obj{}
	for _, c := range p.UseCases {
		if !touched[c.UseCase] {
			continue
		}
		var u UseCase
		if x := findUseCase(useCases, func(x UseCase) bool { return x.ID == c.UseCase }); x != nil {
			u = *x
		}
		cases = append(cases, obj{{"id", c.UseCase}, {"change", c.Change}, {"name", c.Name}, {"before", c.Before}, {"after", c.after()}, {"proofs", proofView(u)}})
	}
	decisions := []obj{}
	for _, d := range p.Decisions {
		o := obj{{"question", d.Question}}
		k := d.Recommended
		if d.Chosen != nil {
			k = *d.Chosen
		}
		if k >= 0 && k < len(d.Options) {
			o = append(o, kv{"answer", d.Options[k].Label})
		}
		decisions = append(decisions, o)
	}
	models := obj{}
	for _, name := range setup.ModelOrder {
		models = append(models, kv{name, setup.Models[name]})
	}
	return StringifyIndent(obj{{"models", models}, {"subjects", nonNil(setup.Subjects)}, {"approvedIntents", approved}, {"useCases", cases}, {"decisions", decisions}, {"notPromised", nonNil(p.NotPromised)}})
}

// DeriveProofs asks the LLM for the proofs of the approved intents and checks its answer.
func DeriveProofs(ctx context.Context, ask AskJSON, p Proposal, useCases []UseCase, setup ProofSetup, opts VerifyOptions) (Proposal, error) {
	var out DeriveV1Out
	if err := askInto(ctx, ask, AskRequest{Step: "derive", System: DeriveProofsSystem, User: DeriveProofsUserPrompt(p, useCases, setup), Schema: json.RawMessage(DeriveProofsSchema)}, &out); err != nil {
		return Proposal{}, err
	}
	return ProofsFromDerivation(p, useCases, setup, out, opts)
}

// DeriveUseCaseProofsUserPrompt is the user turn of the derive step for established use cases, built like the TS
// SDK's deriveUseCaseProofsUserPrompt.
func DeriveUseCaseProofsUserPrompt(targets []UseCase, useCases []UseCase, setup ProofSetup) string {
	wanted := map[string]bool{}
	toProve := []obj{}
	for _, u := range targets {
		wanted[u.ID] = true
		toProve = append(toProve, obj{{"id", u.ID}, {"name", u.Name}, {"description", u.Description}})
	}
	proven := []obj{}
	for _, u := range useCases {
		if wanted[u.ID] || len(PassedProofs(u)) == 0 {
			continue
		}
		v := u
		v.Proofs = PassedProofs(u)
		proven = append(proven, obj{{"id", u.ID}, {"name", u.Name}, {"proofs", proofView(v)}})
	}
	models := obj{}
	for _, name := range setup.ModelOrder {
		models = append(models, kv{name, setup.Models[name]})
	}
	return StringifyIndent(obj{{"models", models}, {"subjects", nonNil(setup.Subjects)}, {"useCasesToProve", toProve}, {"provenUseCases", proven}})
}

// DeriveUseCaseProofs asks the LLM for proofs of established use cases (every one with no passed proof, or ids)
// and checks its answer.
func DeriveUseCaseProofs(ctx context.Context, ask AskJSON, useCases []UseCase, ids []string, setup ProofSetup) (UseCaseDerivation, error) {
	targets, err := UseCaseTargets(useCases, ids)
	if err != nil {
		return UseCaseDerivation{}, err
	}
	if len(targets) == 0 {
		return UseCaseDerivation{UseCases: []UseCase{}, Derived: []ProofEntry{}, Unprovable: []Unprovable{}}, nil
	}
	var out UseCaseDeriveOut
	if err := askInto(ctx, ask, AskRequest{Step: "derive", System: DeriveUseCaseProofsSystem, User: DeriveUseCaseProofsUserPrompt(targets, useCases, setup), Schema: json.RawMessage(DeriveUseCaseProofsSchema)}, &out); err != nil {
		return UseCaseDerivation{}, err
	}
	ids = []string{}
	for _, u := range targets {
		ids = append(ids, u.ID)
	}
	return ProofsFromUseCases(useCases, ids, setup, out)
}
