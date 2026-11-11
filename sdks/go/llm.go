package lastro

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Transport-free LLM steps: prompts, JSON schemas, and the deterministic checks
// that turn an LLM answer into Lastro objects. They are Lastro's only AI steps:
// extracting intents from prose and deriving the proofs. Nothing an LLM returns
// is trusted as is: quotes are anchored by CreateArtifact, plans are parsed and
// validated as LSL, and the code is only judged through the proofs.
//
// The prompts and schemas are generated from the TS SDK (prompts.go), so a
// request is byte-identical in both SDKs.

// AskRequest asks an LLM for one JSON value matching Schema.
type AskRequest struct {
	Step   string          `json:"step"` // extract | derive | gap-review
	System string          `json:"system"`
	User   string          `json:"user"`
	Schema json.RawMessage `json:"schema"`
}

// AskJSON is any LLM transport: the Anthropic API (package claude), the calling agent's own
// model (the CLI's handoff), or a test double.
type AskJSON func(ctx context.Context, req AskRequest) (json.RawMessage, error)

// IntentExtractor turns prose into proposed intents (non-deterministic; reviewed by a human afterwards).
type IntentExtractor interface {
	Extract(ctx context.Context, prose string) (IntentArtifact, error)
}

// ProofDeriver turns approved intents and the model vocabulary into a proof plan (checked by ValidatePlan).
type ProofDeriver interface {
	Derive(ctx context.Context, artifact IntentArtifact, model Model) (ProofPlan, error)
}

// ExtractorFunc adapts a function (for example a deterministic stand-in in tests) to IntentExtractor.
type ExtractorFunc func(ctx context.Context, prose string) (IntentArtifact, error)

func (f ExtractorFunc) Extract(ctx context.Context, prose string) (IntentArtifact, error) {
	return f(ctx, prose)
}

// DeriverFunc adapts a function to ProofDeriver.
type DeriverFunc func(ctx context.Context, artifact IntentArtifact, model Model) (ProofPlan, error)

func (f DeriverFunc) Derive(ctx context.Context, a IntentArtifact, m Model) (ProofPlan, error) {
	return f(ctx, a, m)
}

// ---------------------------------------------------------------------------
// Extraction
// ---------------------------------------------------------------------------

// ExtractOut is the extractor's answer (matches ExtractSchema).
type ExtractOut struct {
	Intents []struct {
		Statement         string `json:"statement"`
		Kind              string `json:"kind"`
		Provability       string `json:"provability"`
		BeyondProofReason string `json:"beyondProofReason"`
		Quote             string `json:"quote"`
	} `json:"intents"`
	Decisions []struct {
		IntentIndexes []int            `json:"intentIndexes"`
		Question      string           `json:"question"`
		Options       []DecisionOption `json:"options"`
		Recommended   int              `json:"recommended"`
	} `json:"decisions"`
	NotPromised []string `json:"notPromised"`
}

// ExtractUserPrompt is the user turn of the extract step.
func ExtractUserPrompt(prose, context string) string {
	ctx := ""
	if context != "" {
		ctx = "<context>\n" + context + "\n</context>\n\n"
	}
	return ctx + "<prose>\n" + prose + "\n</prose>\n\nIntents are numbered from 0 in the order you list them; decisions reference them by index."
}

// ArtifactFromExtraction anchors and checks the extractor's answer.
func ArtifactFromExtraction(prose string, out ExtractOut) (IntentArtifact, error) {
	drafts := make([]IntentDraft, len(out.Intents))
	for i, it := range out.Intents {
		reason := it.BeyondProofReason
		if reason == "none" {
			reason = ""
		}
		drafts[i] = IntentDraft{Statement: it.Statement, Kind: it.Kind, Provability: it.Provability, BeyondProofReason: reason, Quote: it.Quote}
	}
	decisions := make([]DecisionDraft, len(out.Decisions))
	for i, d := range out.Decisions {
		ids := make([]string, len(d.IntentIndexes))
		for j, n := range d.IntentIndexes {
			ids[j] = fmt.Sprintf("I%d", n+1)
		}
		decisions[i] = DecisionDraft{IntentIDs: ids, Question: d.Question, Options: d.Options, Recommended: d.Recommended}
	}
	return CreateArtifact(prose, drafts, decisions, out.NotPromised)
}

// LLMExtractor is the extract step over any AskJSON transport.
type LLMExtractor struct {
	Ask AskJSON
	// Context is optional background for the extractor (glossary, system description).
	Context string
}

func (x LLMExtractor) Extract(ctx context.Context, prose string) (IntentArtifact, error) {
	raw, err := x.Ask(ctx, AskRequest{Step: "extract", System: ExtractSystem, User: ExtractUserPrompt(prose, x.Context), Schema: json.RawMessage(ExtractSchema)})
	if err != nil {
		return IntentArtifact{}, err
	}
	var out ExtractOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return IntentArtifact{}, fmt.Errorf("extract: unreadable answer: %w", err)
	}
	return ArtifactFromExtraction(prose, out)
}

// ---------------------------------------------------------------------------
// Derivation
// ---------------------------------------------------------------------------

// DeriveOut is the deriver's answer (matches DeriveSchema). LSL expressions arrive as JSON
// strings; an object is accepted too.
type DeriveOut struct {
	Clauses []struct {
		ID          string          `json:"id"`
		IntentIDs   []string        `json:"intentIds"`
		Description string          `json:"description"`
		Requires    json.RawMessage `json:"requires"`
		Ensures     json.RawMessage `json:"ensures"`
		Assumes     []string        `json:"assumes"`
	} `json:"clauses"`
	Assumptions []Assumption `json:"assumptions"`
	Scenarios   []struct {
		ID        string          `json:"id"`
		IntentIDs []string        `json:"intentIds"`
		Given     string          `json:"given"`
		When      string          `json:"when"`
		Then      string          `json:"then"`
		InputJSON json.RawMessage `json:"inputJson"`
		Expect    json.RawMessage `json:"expect"`
	} `json:"scenarios"`
	BeyondProof []BeyondProofItem `json:"beyondProof"`
}

// DeriveUserPrompt is the user turn of the derive step: the model, the approved intents and the
// answered decisions, as JSON.stringify(..., null, 2) would print them.
func DeriveUserPrompt(artifact IntentArtifact, model Model) (string, error) {
	if artifact.Hash == "" {
		return "", fmt.Errorf("approve() the artifact before deriving proofs")
	}
	type intent struct {
		ID                string `json:"id"`
		Statement         string `json:"statement"`
		Kind              string `json:"kind"`
		Provability       string `json:"provability"`
		BeyondProofReason string `json:"beyondProofReason,omitempty"`
	}
	type decision struct {
		Question string `json:"question"`
		Answer   string `json:"answer,omitempty"`
	}
	intents := []intent{}
	for _, i := range ApprovedIntents(artifact) {
		intents = append(intents, intent{i.ID, i.Statement, i.Kind, i.Provability, i.BeyondProofReason})
	}
	decisions := []decision{}
	for _, d := range artifact.Decisions {
		pick := d.Recommended
		if d.Chosen != nil {
			pick = *d.Chosen
		}
		answer := ""
		if pick >= 0 && pick < len(d.Options) {
			answer = d.Options[pick].Label
		}
		decisions = append(decisions, decision{d.Question, answer})
	}
	return StringifyIndent(struct {
		Model           Model      `json:"model"`
		ApprovedIntents []intent   `json:"approvedIntents"`
		Decisions       []decision `json:"decisions"`
		NotPromised     []string   `json:"notPromised"`
	}{model, intents, decisions, nonNil(artifact.NotPromised)}), nil
}

// lslText reads an LSL answer that is a JSON string holding the expression, or the expression itself.
func lslText(raw json.RawMessage) (string, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, true
	}
	return string(raw), false
}

// PlanFromDerivation parses and validates the deriver's answer, reporting every problem found.
func PlanFromDerivation(artifact IntentArtifact, model Model, out DeriveOut) (ProofPlan, error) {
	parse := func(where string, raw json.RawMessage) (Expr, error) {
		text, _ := lslText(raw)
		e, err := ParseExpr(text)
		if err != nil {
			return Expr{}, fmt.Errorf("%s: not valid JSON: %s", where, text)
		}
		if errs := ValidateExpr(e, model, nil, where); len(errs) > 0 {
			return Expr{}, fmt.Errorf("%s", strings.Join(errs, "; "))
		}
		return e, nil
	}
	plan := ProofPlan{Lastro: "proof-plan/v0", ArtifactHash: artifact.Hash, Model: model,
		Clauses: []Clause{}, Assumptions: nonNil(out.Assumptions), Scenarios: []Scenario{}, BeyondProof: nonNil(out.BeyondProof)}
	for _, c := range out.Clauses {
		clause := Clause{ID: c.ID, IntentIDs: nonNil(c.IntentIDs), Description: c.Description}
		if text, _ := lslText(c.Requires); len(c.Requires) > 0 && string(c.Requires) != "null" && strings.TrimSpace(text) != "" {
			req, err := parse(c.ID+".requires", c.Requires)
			if err != nil {
				return ProofPlan{}, err
			}
			clause.Requires = &req
		}
		ens, err := parse(c.ID+".ensures", c.Ensures)
		if err != nil {
			return ProofPlan{}, err
		}
		clause.Ensures = ens
		if len(c.Assumes) > 0 {
			clause.Assumes = c.Assumes
		}
		plan.Clauses = append(plan.Clauses, clause)
	}
	for _, s := range out.Scenarios {
		text, _ := lslText(s.InputJSON)
		var input map[string]any
		if err := json.Unmarshal([]byte(text), &input); err != nil {
			return ProofPlan{}, fmt.Errorf("%s.inputJson: not valid JSON: %s", s.ID, text)
		}
		for k, v := range input {
			input[k] = Normalize(v)
		}
		expect, err := parse(s.ID+".expect", s.Expect)
		if err != nil {
			return ProofPlan{}, err
		}
		plan.Scenarios = append(plan.Scenarios, Scenario{ID: s.ID, IntentIDs: nonNil(s.IntentIDs), Given: s.Given, When: s.When, Then: s.Then, Input: input, Expect: expect})
	}
	return plan, nil
}

// LLMDeriver is the derive step over any AskJSON transport.
type LLMDeriver struct{ Ask AskJSON }

func (x LLMDeriver) Derive(ctx context.Context, artifact IntentArtifact, model Model) (ProofPlan, error) {
	user, err := DeriveUserPrompt(artifact, model)
	if err != nil {
		return ProofPlan{}, err
	}
	raw, err := x.Ask(ctx, AskRequest{Step: "derive", System: DeriveSystem, User: user, Schema: json.RawMessage(DeriveSchema)})
	if err != nil {
		return ProofPlan{}, err
	}
	var out DeriveOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return ProofPlan{}, fmt.Errorf("derive: unreadable answer: %w", err)
	}
	return PlanFromDerivation(artifact, model, out)
}

// ---------------------------------------------------------------------------
// LLM gap review (evidence for humans, never proof)
// ---------------------------------------------------------------------------

// Gap is something the proofs cannot see, tied to an intent.
type Gap struct {
	IntentID   string `json:"intentId"`
	Gap        string `json:"gap"`
	Suggestion string `json:"suggestion"`
	Severity   string `json:"severity"` // low | medium | high
}

// GapReviewUserPrompt is the user turn of the gap-review step.
func GapReviewUserPrompt(artifact IntentArtifact, plan ProofPlan, source string) string {
	type intent struct {
		ID        string `json:"id"`
		Statement string `json:"statement"`
	}
	type clause struct {
		ID          string   `json:"id"`
		IntentIDs   []string `json:"intentIds"`
		Description string   `json:"description"`
		Formal      string   `json:"formal"`
	}
	type piece struct {
		ID       string `json:"id"`
		Piece    string `json:"piece"`
		Contract string `json:"contract"`
	}
	intents := []intent{}
	for _, i := range ApprovedIntents(artifact) {
		intents = append(intents, intent{i.ID, i.Statement})
	}
	clauses := []clause{}
	for _, c := range plan.Clauses {
		clauses = append(clauses, clause{c.ID, nonNil(c.IntentIDs), c.Description, showClause(c, " => ")})
	}
	pieces := []piece{}
	for _, a := range plan.Assumptions {
		pieces = append(pieces, piece{a.ID, a.Piece, a.Contract})
	}
	return StringifyIndent(struct {
		ApprovedIntents []intent          `json:"approvedIntents"`
		Clauses         []clause          `json:"clauses"`
		TrustedPieces   []piece           `json:"trustedPieces"`
		BeyondProof     []BeyondProofItem `json:"beyondProof"`
		Code            string            `json:"code"`
	}{intents, clauses, pieces, nonNil(plan.BeyondProof), source})
}

// LLMGapReviewer asks an LLM what the proofs cannot see.
type LLMGapReviewer struct{ Ask AskJSON }

func (x LLMGapReviewer) Review(ctx context.Context, artifact IntentArtifact, plan ProofPlan, source string) ([]Gap, error) {
	raw, err := x.Ask(ctx, AskRequest{Step: "gap-review", System: GapReviewSystem, User: GapReviewUserPrompt(artifact, plan, source), Schema: json.RawMessage(GapReviewSchema)})
	if err != nil {
		return nil, err
	}
	var out struct {
		Gaps []Gap `json:"gaps"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return []Gap{}, nil
	}
	return nonNil(out.Gaps), nil
}

// HandoffID names a request the way the TS CLI does (step + first 12 hex digits of the SHA-256 of
// JSON.stringify(request)), so both CLIs find each other's handoff answers in .lastro/llm/.
func HandoffID(req AskRequest) string {
	return req.Step + "-" + sha256Hex(Stringify(req))[:12]
}
