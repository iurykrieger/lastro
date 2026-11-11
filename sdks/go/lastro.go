// Package lastro is the Go SDK of Lastro: prose -> reviewable intents -> derived
// proofs -> verification against what the code actually does.
//
//	You ──plan──▶ intents ──review (policy)──▶ spec ──prove──▶ proof + receipt
//	                 ▲                          │                │
//	                 └─ contradictions: decisions┘                └─ failures ──▶ whoever writes the code
//
// Lastro's only AI steps are extracting intents from prose and deriving the
// proofs. It never writes the implementation: any coding agent or person does,
// and Prove judges it. Every stage takes and returns plain JSON-able values in
// the same format as the TypeScript SDK and the `lastro` CLI, so the SDKs share
// a .lastro/ workspace and hashes.
package lastro

//go:generate node internal/gen-prompts.mjs

import (
	"context"
	"fmt"
	"strings"
)

// Options configures New.
type Options struct {
	// Model is the vocabulary of the proofs: inputs, outputs and helper definitions.
	Model Model
	// Ask is one LLM transport for every AI step. Each step can also be replaced below.
	Ask       AskJSON
	Extractor IntentExtractor
	Deriver   ProofDeriver
	// RequiredApprovals is how many distinct reviewers must approve each intent (your team's policy). Default 1.
	RequiredApprovals int
	// Rust is the independent Rust prover (see FindRustProver). Recommended in CI.
	Rust RustProver
	// Verify tunes the engine (sampling, limits). Its Subject and Traces are ignored.
	Verify VerifyOptions
}

// Spec is stage 2: the formal spec derived from a frozen, fully reviewed artifact.
type Spec struct {
	Artifact IntentArtifact `json:"artifact"`
	Plan     ProofPlan      `json:"plan"`
	// Contradictions are clauses that cannot all hold together. Send them back with ReturnDecisions.
	Contradictions []DecisionCard `json:"contradictions"`
	OK             bool           `json:"ok"`
}

// Proof is stage 3: the code checked against the spec.
type Proof struct {
	OK     bool               `json:"ok"`
	Report VerificationReport `json:"report"`
	Rust   *RustProofReport   `json:"rust,omitempty"`
	// EnginesAgree is true when both engines gave every clause the same verdict (nil without Rust).
	EnginesAgree *bool `json:"enginesAgree,omitempty"`
	// Failures are code problems, with counterexamples: what the coding agent (or person) must fix.
	Failures []ProofFailure `json:"failures"`
	// SpecGaps are spec problems (vacuous or weak clauses, intents with no obligation): what the team must fix.
	SpecGaps []string `json:"specGaps"`
	Receipt  string   `json:"receipt"`
}

// ProofFailure is one clause or scenario the code broke, with the input that shows it.
type ProofFailure struct {
	Kind        string   `json:"kind"` // clause | scenario
	ID          string   `json:"id"`
	IntentIDs   []string `json:"intentIds"`
	Description string   `json:"description"`
	// Formal is the obligation in LSL notation.
	Formal string         `json:"formal"`
	Input  map[string]any `json:"input"`
	Output map[string]any `json:"output"`
	// outputOrder prints outputs in the model's order in AgentFeedback.
	outputOrder []string
}

// FailuresOf lists the violated clauses (with counterexamples) and failed scenarios.
func FailuresOf(plan ProofPlan, report VerificationReport) []ProofFailure {
	out := []ProofFailure{}
	for _, r := range report.Clauses {
		if r.Verdict != "violated" || r.Counterexample == nil {
			continue
		}
		for _, c := range plan.Clauses {
			if c.ID == r.ClauseID {
				formal := "ensures " + Show(c.Ensures)
				if c.Requires != nil {
					formal = "requires " + Show(*c.Requires) + "; " + formal
				}
				out = append(out, ProofFailure{Kind: "clause", ID: c.ID, IntentIDs: nonNil(c.IntentIDs), Description: c.Description, Formal: formal,
					Input: r.Counterexample.Input, Output: r.Counterexample.Output, outputOrder: plan.Model.Outputs.Names()})
			}
		}
	}
	for _, s := range report.Scenarios {
		if s.Status != "failed" {
			continue
		}
		for _, sc := range plan.Scenarios {
			if sc.ID == s.ScenarioID {
				out = append(out, ProofFailure{Kind: "scenario", ID: sc.ID, IntentIDs: nonNil(sc.IntentIDs),
					Description: fmt.Sprintf("given %s, when %s, then %s", sc.Given, sc.When, sc.Then),
					Formal:      "expect " + Show(sc.Expect), Input: sc.Input, Output: s.Output, outputOrder: plan.Model.Outputs.Names()})
			}
		}
	}
	return out
}

// SpecGapsOf lists vacuous and weak clauses, intents with no obligation, and contradictions.
func SpecGapsOf(report VerificationReport) []string {
	out := []string{}
	for _, r := range report.Clauses {
		if r.Verdict == "vacuous" || r.Verdict == "weak" {
			out = append(out, strings.TrimSpace(fmt.Sprintf("%s is %s: %s", r.ClauseID, r.Verdict, r.Note)))
		}
	}
	for _, i := range report.Intents {
		if i.Verdict == "gap" {
			out = append(out, i.IntentID+" has no proof obligation")
		}
	}
	for _, d := range report.Decisions {
		out = append(out, "Contradiction: "+d.Question)
	}
	return out
}

// AgentFeedback renders failures as a prompt-ready brief for whatever writes the code
// (a coding agent or a person). Same text as the TS SDK's agentFeedback.
func AgentFeedback(failures []ProofFailure, specGaps []string) string {
	if len(failures) == 0 {
		s := "Every proof passes."
		if len(specGaps) > 0 {
			s += " Spec gaps for the team: " + strings.Join(specGaps, "; ")
		}
		return s
	}
	lines := []string{"The implementation breaks these proof obligations. Fix the code so each holds for every input, without breaking the others:"}
	for _, f := range failures {
		out := stringifyOrdered(f.Output, f.outputOrder)
		lines = append(lines, fmt.Sprintf("- %s (%s): %s\n  %s\n  counterexample: %s returned %s", f.ID, strings.Join(f.IntentIDs, ", "), f.Description, f.Formal, Stringify(f.Input), out))
	}
	return strings.Join(lines, "\n")
}

// Lastro is the pipeline behind one small interface. Create it with New.
type Lastro struct {
	o         Options
	extractor IntentExtractor
	deriver   ProofDeriver
	gaps      *LLMGapReviewer
	policy    ReviewPolicy
}

// New creates the pipeline. The approval policy is fixed here (default 1).
func New(o Options) *Lastro {
	l := &Lastro{o: o, extractor: o.Extractor, deriver: o.Deriver, policy: ReviewPolicy{RequiredApprovals: o.RequiredApprovals}}
	if l.policy.RequiredApprovals <= 0 {
		l.policy.RequiredApprovals = 1
	}
	if o.Ask != nil {
		if l.extractor == nil {
			l.extractor = LLMExtractor{Ask: o.Ask}
		}
		if l.deriver == nil {
			l.deriver = LLMDeriver{Ask: o.Ask}
		}
		l.gaps = &LLMGapReviewer{Ask: o.Ask}
	}
	return l
}

func (l *Lastro) withPolicy(a IntentArtifact) IntentArtifact {
	if a.Policy == nil {
		p := l.policy
		a.Policy = &p
	}
	return a
}

// Plan is stage 1: extract reviewable intents from prose. Pass the previous artifact to keep its review.
func (l *Lastro) Plan(ctx context.Context, prose string, previous *IntentArtifact) (IntentArtifact, error) {
	if l.extractor == nil {
		return IntentArtifact{}, fmt.Errorf("Extractor needs an LLM: pass Ask or a custom Extractor to lastro.New()")
	}
	fresh, err := l.extractor.Extract(ctx, prose)
	if err != nil {
		return IntentArtifact{}, err
	}
	p := l.policy
	fresh.Policy = &p
	if previous != nil {
		return CarryReview(*previous, fresh), nil
	}
	return fresh, nil
}

// Review starts one person's review. Chain Approve/Amend/Reject/Answer, then Done.
func (l *Lastro) Review(a IntentArtifact, reviewer string) *Review {
	if a.Hash != "" {
		a = Thaw(a)
	}
	return NewReview(l.withPolicy(a), reviewer)
}

// Pending lists what still blocks the spec: missing approvals and unanswered decisions.
func (l *Lastro) Pending(a IntentArtifact) []string { return PendingReview(l.withPolicy(a)) }

// Spec is stage 2: freeze the reviewed intents, derive clauses, validate, look for contradictions.
func (l *Lastro) Spec(ctx context.Context, a IntentArtifact) (Spec, error) {
	frozen, err := Freeze(l.withPolicy(a))
	if err != nil {
		return Spec{}, err
	}
	if l.deriver == nil {
		return Spec{}, fmt.Errorf("Deriver needs an LLM: pass Ask or a custom Deriver to lastro.New()")
	}
	plan, err := l.deriver.Derive(ctx, frozen, l.o.Model)
	if err != nil {
		return Spec{}, err
	}
	if _, err := ValidatePlan(frozen, plan); err != nil {
		return Spec{}, err
	}
	contradictions, err := CheckConsistency(plan, l.o.Verify)
	if err != nil {
		return Spec{}, err
	}
	return Spec{Artifact: frozen, Plan: plan, Contradictions: contradictions, OK: len(contradictions) == 0}, nil
}

// ReturnDecisions sends contradictions back to the team as decision cards; the artifact must be reviewed again.
func (l *Lastro) ReturnDecisions(s Spec) IntentArtifact {
	return RaiseDecisions(s.Artifact, s.Contradictions)
}

// Prove is stage 3: check the code against every clause (and the Rust prover, if configured).
func (l *Lastro) Prove(ctx context.Context, s Spec, subject Subject, traces []Trace) (Proof, error) {
	vo := l.o.Verify
	vo.Subject, vo.Traces = subject, traces
	report, err := Verify(s.Artifact, s.Plan, vo)
	if err != nil {
		return Proof{}, err
	}
	p := Proof{Report: report, Failures: FailuresOf(s.Plan, report), SpecGaps: SpecGapsOf(report)}
	ok := report.OK
	if l.o.Rust != nil && subject != nil {
		table, err := Tabulate(s.Plan.Model, subject, 0)
		if err != nil {
			return Proof{}, err
		}
		rust, err := l.o.Rust.Prove(ctx, s.Plan.Model, s.Plan.Clauses, table)
		if err != nil {
			return Proof{}, err
		}
		agree := rustAgrees(report.Clauses, rust)
		p.Rust, p.EnginesAgree = &rust, &agree
		ok = ok && rust.OK && agree
	}
	p.OK = ok
	plan := s.Plan
	p.Receipt = RenderReceipt(report, &plan, p.Rust)
	return p, nil
}

// GapReview asks the LLM what the proofs cannot see. Evidence for humans, never proof.
func (l *Lastro) GapReview(ctx context.Context, s Spec, source string) ([]Gap, error) {
	if l.gaps == nil {
		return nil, fmt.Errorf("Gap reviewer needs an LLM: pass Ask to lastro.New()")
	}
	return l.gaps.Review(ctx, s.Artifact, s.Plan, source)
}
