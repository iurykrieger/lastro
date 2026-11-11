package lastro

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// The data model, field for field the TS SDK's (sdks/typescript/src/types.ts), so
// both SDKs read and write the same JSON and compute the same hashes.
//
//	prose -> IntentArtifact (reviewed, approved) -> ProofPlan (LSL) -> VerificationReport -> receipt

// ---------------------------------------------------------------------------
// Intents
// ---------------------------------------------------------------------------

// The five limits of formal proof, after PDD Kit's "where proof stops".
const (
	ExternalCode      = "external-code"      // code the verifier cannot see (bcrypt, sqlx, jwt libs...)
	WorldFacts        = "world-facts"        // database contents, the clock, the network
	TerminationTiming = "termination-timing" // whether / how fast something finishes
	HumanJudgment     = "human-judgment"     // "the error message should be helpful"
	Unstated          = "unstated"           // requirements nobody wrote down
)

// BeyondProofReasons lists every reason, in the TS SDK's order.
var BeyondProofReasons = []string{ExternalCode, WorldFacts, TerminationTiming, HumanJudgment, Unstated}

// SourceSpan is where in the prose an intent came from (the intent map). Offsets count UTF-16 code units, as in JavaScript.
type SourceSpan struct {
	Quote string `json:"quote"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// Intent is one reviewable promise extracted from the prose.
type Intent struct {
	ID                string     `json:"id"`
	Statement         string     `json:"statement"`
	Kind              string     `json:"kind"`        // behavior | invariant | preserve | non-functional
	Provability       string     `json:"provability"` // formal | empirical | beyond-proof
	BeyondProofReason string     `json:"beyondProofReason,omitempty"`
	Source            SourceSpan `json:"source"`
	Status            string     `json:"status"` // proposed | approved | rejected | amended
	// ReviewNote is required when amended or rejected.
	ReviewNote string `json:"reviewNote,omitempty"`
	// OriginalStatement is kept when a reviewer amends the intent.
	OriginalStatement string `json:"originalStatement,omitempty"`
	// Approvals lists the reviewers who approved the current statement.
	Approvals []string `json:"approvals,omitempty"`
	// DecisionID is set when the intent records the answer to a decision (for example a contradiction in the spec).
	DecisionID string `json:"decisionId,omitempty"`
}

// DecisionOption is one answer to a decision, with its consequence.
type DecisionOption struct {
	Label       string `json:"label"`
	Consequence string `json:"consequence"`
}

// DecisionCard is one question, options with consequences, one recommended (PDD Kit decision card).
type DecisionCard struct {
	ID          string           `json:"id"`
	IntentIDs   []string         `json:"intentIds"`
	Question    string           `json:"question"`
	Options     []DecisionOption `json:"options"`
	Recommended int              `json:"recommended"`
	Chosen      *int             `json:"chosen,omitempty"`
	Origin      string           `json:"origin"` // extraction | consistency-check | verification
}

func (d DecisionCard) MarshalJSON() ([]byte, error) {
	type alias DecisionCard
	d.IntentIDs, d.Options = nonNil(d.IntentIDs), nonNil(d.Options)
	return json.Marshal(alias(d))
}

// ReviewPolicy holds the team's review rules.
type ReviewPolicy struct {
	// RequiredApprovals is how many distinct reviewers must approve each intent before it reaches the spec.
	RequiredApprovals int `json:"requiredApprovals"`
}

// IntentArtifact is the reviewable list of intents, its decisions, and (once approved) its hash.
type IntentArtifact struct {
	Lastro      string         `json:"lastro"` // "intent-artifact/v0"
	Prose       string         `json:"prose"`
	Intents     []Intent       `json:"intents"`
	Decisions   []DecisionCard `json:"decisions"`
	NotPromised []string       `json:"notPromised"`
	Policy      *ReviewPolicy  `json:"policy,omitempty"`
	Hash        string         `json:"hash,omitempty"`
}

func (a IntentArtifact) MarshalJSON() ([]byte, error) {
	type alias IntentArtifact
	a.Intents, a.Decisions, a.NotPromised = nonNil(a.Intents), nonNil(a.Decisions), nonNil(a.NotPromised)
	return json.Marshal(alias(a))
}

// ---------------------------------------------------------------------------
// Proof plan
// ---------------------------------------------------------------------------

// Def is a named helper expression of the model, referenced with Call.
type Def struct {
	Params      []string `json:"params"`
	Description string   `json:"description,omitempty"`
	Body        Expr     `json:"body"`
}

// NamedDef is a Def with its name.
type NamedDef struct {
	Name string
	Def
}

// Defs is an ordered table of defs; it reads and writes as a JSON object.
type Defs []NamedDef

// Get returns a def by name.
func (ds Defs) Get(name string) (Def, bool) {
	for _, d := range ds {
		if d.Name == name {
			return d.Def, true
		}
	}
	return Def{}, false
}

func (ds Defs) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, d := range ds {
		if i > 0 {
			b.WriteByte(',')
		}
		d.Params = nonNil(d.Params)
		v, err := json.Marshal(d.Def)
		if err != nil {
			return nil, err
		}
		b.WriteString(jsString(d.Name) + ":")
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func (ds *Defs) UnmarshalJSON(data []byte) error {
	return decodeOrderedObject(data, func(key string, raw json.RawMessage) error {
		var d Def
		if err := json.Unmarshal(raw, &d); err != nil {
			return fmt.Errorf("def %s: %w", key, err)
		}
		*ds = append(*ds, NamedDef{Name: key, Def: d})
		return nil
	}, func() { *ds = Defs{} })
}

// Model is the vocabulary the proofs talk about. Inputs include facts provided by trusted
// pieces; outputs are what the program decides.
type Model struct {
	Name    string `json:"name"`
	Inputs  Fields `json:"inputs"`
	Outputs Fields `json:"outputs"`
	Defs    Defs   `json:"defs,omitempty"`
}

// Clause is a formal obligation: for all inputs, requires(input) ⇒ ensures(input, output).
type Clause struct {
	ID          string   `json:"id"`
	IntentIDs   []string `json:"intentIds"`
	Description string   `json:"description"`
	Requires    *Expr    `json:"requires,omitempty"`
	Ensures     Expr     `json:"ensures"`
	// Assumes lists the trusted assumptions this clause relies on.
	Assumes []string `json:"assumes,omitempty"`
}

func (c Clause) MarshalJSON() ([]byte, error) {
	type alias Clause
	c.IntentIDs = nonNil(c.IntentIDs)
	return json.Marshal(alias(c))
}

// Assumption is a trusted piece: the boundary of the proof (PDD Kit trust card).
type Assumption struct {
	ID        string   `json:"id"`
	IntentIDs []string `json:"intentIds"`
	Piece     string   `json:"piece"`
	Contract  string   `json:"contract"`
	YouAccept string   `json:"youAccept"`
	IfWrong   string   `json:"ifWrong"`
	CheckedBy []string `json:"checkedBy"`
}

func (a Assumption) MarshalJSON() ([]byte, error) {
	type alias Assumption
	a.IntentIDs, a.CheckedBy = nonNil(a.IntentIDs), nonNil(a.CheckedBy)
	return json.Marshal(alias(a))
}

// Scenario is an empirical obligation: Given/When/Then, checked by running the program.
type Scenario struct {
	ID        string         `json:"id"`
	IntentIDs []string       `json:"intentIds"`
	Given     string         `json:"given"`
	When      string         `json:"when"`
	Then      string         `json:"then"`
	Input     map[string]any `json:"input"`
	Expect    Expr           `json:"expect"`
}

func (s Scenario) MarshalJSON() ([]byte, error) {
	type alias Scenario
	s.IntentIDs = nonNil(s.IntentIDs)
	if s.Input == nil {
		s.Input = map[string]any{}
	}
	return json.Marshal(alias(s))
}

// BeyondProofItem is an intent (or part of one) where proof stops.
type BeyondProofItem struct {
	IntentID string `json:"intentId"`
	Reason   string `json:"reason"`
	Note     string `json:"note"`
}

// ProofPlan holds the obligations derived from the approved intents.
type ProofPlan struct {
	Lastro       string            `json:"lastro"` // "proof-plan/v0"
	ArtifactHash string            `json:"artifactHash"`
	Model        Model             `json:"model"`
	Clauses      []Clause          `json:"clauses"`
	Assumptions  []Assumption      `json:"assumptions"`
	Scenarios    []Scenario        `json:"scenarios"`
	BeyondProof  []BeyondProofItem `json:"beyondProof"`
}

func (p ProofPlan) MarshalJSON() ([]byte, error) {
	type alias ProofPlan
	p.Clauses, p.Assumptions, p.Scenarios, p.BeyondProof = nonNil(p.Clauses), nonNil(p.Assumptions), nonNil(p.Scenarios), nonNil(p.BeyondProof)
	return json.Marshal(alias(p))
}

// ---------------------------------------------------------------------------
// Verification
// ---------------------------------------------------------------------------

// Trace is a recorded real execution.
type Trace struct {
	Input  Env    `json:"input"`
	Output Env    `json:"output"`
	Ref    string `json:"ref,omitempty"`
}

// Counterexample is an input and the output that breaks a clause.
type Counterexample struct {
	Input  Env `json:"input"`
	Output Env `json:"output"`
}

// MutantStats counts the planted bugs a clause rejected.
type MutantStats struct {
	Total  int `json:"total"`
	Killed int `json:"killed"`
}

// ClauseResult is the verdict on one clause.
type ClauseResult struct {
	ClauseID string `json:"clauseId"`
	Verdict  string `json:"verdict"`  // proved | evidenced | violated | vacuous | weak | not-run
	Evidence string `json:"evidence"` // formal | empirical | trusted | none
	Method   string `json:"method"`   // exhaustive | sampling | traces | none
	Checked  int    `json:"checked"`
	// FailureRateUpperBound95 is, for sampling, the upper bound of the failure rate at 95% (rule of three).
	FailureRateUpperBound95 *float64        `json:"failureRateUpperBound95,omitempty"`
	Counterexample          *Counterexample `json:"counterexample,omitempty"`
	Mutants                 *MutantStats    `json:"mutants,omitempty"`
	TracesChecked           *int            `json:"tracesChecked,omitempty"`
	TraceViolation          *Trace          `json:"traceViolation,omitempty"`
	Note                    string          `json:"note,omitempty"`
}

// ScenarioResult is the outcome of one scenario.
type ScenarioResult struct {
	ScenarioID string `json:"scenarioId"`
	Status     string `json:"status"` // passed | failed
	Output     Env    `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
}

// IntentResult rolls the clause, scenario and assumption results up to one intent.
type IntentResult struct {
	IntentID    string   `json:"intentId"`
	Statement   string   `json:"statement"`
	Verdict     string   `json:"verdict"` // proved | proved-under-assumptions | evidenced | violated | beyond-proof | gap
	Evidence    []string `json:"evidence"`
	Clauses     []string `json:"clauses"`
	Assumptions []string `json:"assumptions"`
	Scenarios   []string `json:"scenarios"`
	// Confidence is 1 for proved; 1 - upper bound for evidenced; 0 for violated/gap.
	Confidence float64  `json:"confidence"`
	Notes      []string `json:"notes"`
}

func (r IntentResult) MarshalJSON() ([]byte, error) {
	type alias IntentResult
	r.Evidence, r.Clauses, r.Assumptions, r.Scenarios, r.Notes = nonNil(r.Evidence), nonNil(r.Clauses), nonNil(r.Assumptions), nonNil(r.Scenarios), nonNil(r.Notes)
	return json.Marshal(alias(r))
}

// The intent verdicts, in the order the receipt counts them.
var IntentVerdicts = []string{"proved", "proved-under-assumptions", "evidenced", "violated", "beyond-proof", "gap"}

// VerificationReport is the full result of checking a plan against the program.
type VerificationReport struct {
	Lastro       string            `json:"lastro"` // "verification-report/v0"
	ArtifactHash string            `json:"artifactHash"`
	PlanHash     string            `json:"planHash"`
	Model        string            `json:"model"`
	Clauses      []ClauseResult    `json:"clauses"`
	Scenarios    []ScenarioResult  `json:"scenarios"`
	Intents      []IntentResult    `json:"intents"`
	Assumptions  []Assumption      `json:"assumptions"`
	BeyondProof  []BeyondProofItem `json:"beyondProof"`
	NotPromised  []string          `json:"notPromised"`
	// Decisions are the contradictions in the spec, returned to the team.
	Decisions []DecisionCard `json:"decisions"`
	Summary   map[string]int `json:"summary"`
	OK        bool           `json:"ok"`
}

func (r VerificationReport) MarshalJSON() ([]byte, error) {
	type alias VerificationReport
	r.Clauses, r.Scenarios, r.Intents, r.Assumptions = nonNil(r.Clauses), nonNil(r.Scenarios), nonNil(r.Intents), nonNil(r.Assumptions)
	r.BeyondProof, r.NotPromised, r.Decisions = nonNil(r.BeyondProof), nonNil(r.NotPromised), nonNil(r.Decisions)
	return json.Marshal(alias(r))
}

// Subject is the program's final output: a decision function over the model inputs.
type Subject func(input Env) (Env, error)
