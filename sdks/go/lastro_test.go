package lastro_test

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	lastro "github.com/iurykrieger/lastro/sdks/go"
	"github.com/iurykrieger/lastro/sdks/go/examples/login"
)

// must fails the test (by panicking) on an error.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func extract(t *testing.T) lastro.IntentArtifact {
	return must(login.Extract(login.Prose))
}

func reviewed(t *testing.T, d1 int, fixI5 bool) lastro.IntentArtifact {
	t.Helper()
	r := lastro.NewReview(extract(t), "").ApproveAll().Answer("D1", d1).Answer("D2", 0)
	if fixI5 {
		r = r.Amend("I5", login.FixedI5)
	}
	return must(lastro.Approve(must(r.Done())))
}

func TestSha256AndCanonicalJSON(t *testing.T) {
	h := must(lastro.HashOf(map[string]any{"b": 1, "a": []any{"x<y", 0.5, 1e21, 1e-7, true, nil}}))
	c := must(lastro.CanonicalJSON(map[string]any{"b": 1, "a": []any{"x<y", 0.5, 1e21, 1e-7, true, nil}}))
	if c != `{"a":["x<y",0.5,1e+21,1e-7,true,null],"b":1}` {
		t.Fatalf("canonical JSON: %s", c)
	}
	if !strings.HasPrefix(h, "sha256:") || len(h) != 71 {
		t.Fatal(h)
	}
}

func TestQuotesAnchorToProse(t *testing.T) {
	_, err := lastro.CreateArtifact("only the right password", []lastro.IntentDraft{{Statement: "x", Kind: "behavior", Provability: "formal", Quote: "something not in the prose"}}, nil, nil)
	var ae *lastro.ArtifactError
	if !errors.As(err, &ae) {
		t.Fatalf("want ArtifactError, got %v", err)
	}
	a := extract(t)
	i1 := a.Intents[0]
	if login.Prose[i1.Source.Start:i1.Source.End] != i1.Source.Quote {
		t.Fatal("intent map offsets")
	}
	// Offsets count UTF-16 units, like JavaScript.
	b := must(lastro.CreateArtifact("Só o login 😀 válido", []lastro.IntentDraft{{Statement: "x", Kind: "behavior", Provability: "formal", Quote: "válido"}}, nil, nil))
	if b.Intents[0].Source.Start != 14 || b.Intents[0].Source.End != 20 {
		t.Fatalf("UTF-16 offsets: %+v", b.Intents[0].Source)
	}
}

func TestApproveNeedsFullReview(t *testing.T) {
	if _, err := lastro.Approve(extract(t)); err == nil {
		t.Fatal("unreviewed artifact froze")
	}
	_, err := lastro.Approve(must(lastro.NewReview(extract(t), "").ApproveAll().Done()))
	if err == nil || !strings.Contains(err.Error(), "D1 unanswered") {
		t.Fatalf("want D1 unanswered, got %v", err)
	}
	f := reviewed(t, 0, true)
	if !lastro.IsApproved(f) {
		t.Fatal("not intact")
	}
	f.Intents[0].Statement = "something else"
	if lastro.IsApproved(f) {
		t.Fatal("tampering not detected")
	}
}

func verify(t *testing.T, a lastro.IntentArtifact, subject lastro.Subject) lastro.VerificationReport {
	t.Helper()
	return must(lastro.Verify(a, login.DerivePlan(a), lastro.VerifyOptions{Subject: subject}))
}

func TestReferenceLoginIsProved(t *testing.T) {
	a := reviewed(t, 0, true)
	plan := login.DerivePlan(a)
	if u := must(lastro.ValidatePlan(a, plan)); len(u) != 0 {
		t.Fatalf("uncovered %v", u)
	}
	r := verify(t, a, login.Decide(""))
	if !r.OK {
		t.Fatal(lastro.RenderReceipt(r, &plan, nil))
	}
	for _, c := range r.Clauses {
		if c.Verdict != "proved" || c.Method != "exhaustive" {
			t.Fatalf("%s %s", c.ClauseID, c.Verdict)
		}
	}
	if r.Summary["beyond-proof"] != 2 || r.Intents[0].Verdict != "proved-under-assumptions" {
		t.Fatalf("summary %v", r.Summary)
	}
}

func TestEveryPlantedBugIsRejected(t *testing.T) {
	a := reviewed(t, 0, true)
	for _, bug := range login.Bugs {
		r := verify(t, a, login.Decide(bug))
		violated := 0
		for _, c := range r.Clauses {
			if c.Verdict == "violated" {
				violated++
				if c.Counterexample == nil {
					t.Fatalf("%s: %s has no counterexample", bug, c.ClauseID)
				}
			}
		}
		if r.OK || violated == 0 {
			t.Fatalf("bug %s not caught", bug)
		}
	}
}

func TestOneWaySpecIsVacuousOnRefuseEveryone(t *testing.T) {
	oneWay := reviewed(t, 1, true)
	r := verify(t, oneWay, login.Decide("refuse-everyone"))
	vac := false
	for _, c := range r.Clauses {
		if c.Verdict == "violated" {
			t.Fatal("the one-way proof should accept a login that refuses everyone")
		}
		vac = vac || c.Verdict == "vacuous"
	}
	if !vac || r.OK {
		t.Fatal("vacuity should expose it")
	}
}

func TestWeakVacuousAndSampling(t *testing.T) {
	a := must(lastro.Approve(must(lastro.NewReview(must(lastro.CreateArtifact("x doubles", []lastro.IntentDraft{{Statement: "y = 2x", Kind: "behavior", Provability: "formal", Quote: "x doubles"}}, nil, nil)), "").ApproveAll().Done())))
	ptr := func(e lastro.Expr) *lastro.Expr { return &e }
	plan := lastro.ProofPlan{Lastro: "proof-plan/v0", ArtifactHash: a.Hash,
		Model: lastro.Model{Name: "double", Inputs: lastro.Fields{{Name: "x", Domain: lastro.Int(0, 9)}}, Outputs: lastro.Fields{{Name: "y", Domain: lastro.Int(-1000, 10_000_000)}}},
		Clauses: []lastro.Clause{
			{ID: "C1", IntentIDs: []string{"I1"}, Description: "tautology", Ensures: lastro.Or(lastro.Ge(lastro.Out("y"), 0), lastro.Lt(lastro.Out("y"), 0))},
			{ID: "C2", IntentIDs: []string{"I1"}, Description: "never applies", Requires: ptr(lastro.Gt(lastro.In("x"), 100)), Ensures: lastro.Eq(lastro.Out("y"), 0)},
			{ID: "C3", IntentIDs: []string{"I1"}, Description: "y = 2x", Ensures: lastro.Eq(lastro.Out("y"), lastro.Mul(2, lastro.In("x")))},
		}}
	double := func(in lastro.Env) (lastro.Env, error) { return lastro.Env{"y": 2 * in["x"].(float64)}, nil }
	r := must(lastro.Verify(a, plan, lastro.VerifyOptions{Subject: double}))
	if got := []string{r.Clauses[0].Verdict, r.Clauses[1].Verdict, r.Clauses[2].Verdict}; strings.Join(got, ",") != "weak,vacuous,proved" {
		t.Fatal(got)
	}
	plan.Model.Inputs = lastro.Fields{{Name: "x", Domain: lastro.Int(0, 1_000_000)}}
	plan.Clauses = plan.Clauses[2:]
	r = must(lastro.Verify(a, plan, lastro.VerifyOptions{Subject: double, Samples: 3000}))
	if r.Clauses[0].Verdict != "evidenced" || *r.Clauses[0].FailureRateUpperBound95 != 0.001 {
		t.Fatalf("%+v", r.Clauses[0])
	}
}

func TestContradictionsBecomeDecisionsThenIntents(t *testing.T) {
	a := reviewed(t, 0, true)
	raised := lastro.RaiseDecisions(a, []lastro.DecisionCard{{ID: "X1", IntentIDs: []string{"I5", "I7"}, Question: "C5 and C7 cannot both hold for both+db-fails. Which gives way?",
		Options: []lastro.DecisionOption{{Label: "Relax C5", Consequence: "the database error wins"}, {Label: "Relax C7", Consequence: "the 400 wins"}}, Origin: "consistency-check"}})
	if raised.Hash != "" || raised.Decisions[2].ID != "D3" {
		t.Fatal("raise")
	}
	if _, err := lastro.Approve(raised); err == nil || !strings.Contains(err.Error(), "D3 unanswered") {
		t.Fatal(err)
	}
	answered := must(lastro.NewReview(raised, "ana").Answer("D3", 0).Done())
	it := answered.Intents[len(answered.Intents)-1]
	if it.DecisionID != "D3" || it.Status != "proposed" || !strings.Contains(it.Statement, "Relax C5 (the database error wins)") {
		t.Fatalf("%+v", it)
	}
	if _, err := lastro.Approve(answered); err == nil || !strings.Contains(err.Error(), "I10 not reviewed") {
		t.Fatal(err)
	}
	must(lastro.Approve(must(lastro.NewReview(answered, "bruno").Approve(it.ID).Done())))
}

func pipeline(approvals int) *lastro.Lastro {
	return lastro.New(lastro.Options{
		Model:     login.Model,
		Extractor: lastro.ExtractorFunc(func(_ context.Context, prose string) (lastro.IntentArtifact, error) { return login.Extract(prose) }),
		Deriver: lastro.DeriverFunc(func(_ context.Context, a lastro.IntentArtifact, _ lastro.Model) (lastro.ProofPlan, error) {
			return login.DerivePlan(a), nil
		}),
		RequiredApprovals: approvals,
		Rust:              rustProver(),
	})
}

func TestPipelineWithTwoApprovals(t *testing.T) {
	ctx := context.Background()
	l := pipeline(2)
	a := must(l.Plan(ctx, login.Prose, nil))
	a = must(l.Review(a, "ana").ApproveAll().Amend("I5", login.FixedI5).Answer("D1", 0).Answer("D2", 0).Done())
	if !matches(l.Pending(a), `I1 needs 1 more approval`) {
		t.Fatal(l.Pending(a))
	}
	if _, err := l.Spec(ctx, a); err == nil {
		t.Fatal("spec before the policy is met")
	}
	a = must(l.Review(a, "ana").ApproveAll().Done())
	if !matches(l.Pending(a), `I5 needs 1 more`) {
		t.Fatal("amendment should reset I5")
	}
	a = must(l.Review(a, "bruno").ApproveAll().Done())
	if len(l.Pending(a)) != 0 {
		t.Fatal(l.Pending(a))
	}
	spec := must(l.Spec(ctx, a))
	proof := must(l.Prove(ctx, spec, login.Decide(""), nil))
	if !proof.OK || len(proof.Failures) != 0 || !strings.Contains(proof.Receipt, "login_flow") {
		t.Fatal(proof.Receipt)
	}
	bad := must(l.Prove(ctx, spec, login.Decide("skip-password"), nil))
	if bad.OK || bad.Failures[0].Input["password"] != "wrong" {
		t.Fatalf("%+v", bad.Failures)
	}
	if !regexp.MustCompile(`counterexample: .*"password":"wrong"`).MatchString(lastro.AgentFeedback(bad.Failures, bad.SpecGaps)) {
		t.Fatal(lastro.AgentFeedback(bad.Failures, bad.SpecGaps))
	}
	if lastro.AgentFeedback(proof.Failures, proof.SpecGaps) != "Every proof passes." {
		t.Fatal("feedback")
	}
}

func TestLLMStepsAreChecked(t *testing.T) {
	ctx := context.Background()
	approved := reviewed(t, 0, true)
	ref := login.DerivePlan(approved)
	type clause struct {
		ID          string   `json:"id"`
		IntentIDs   []string `json:"intentIds"`
		Description string   `json:"description"`
		Requires    string   `json:"requires"`
		Ensures     string   `json:"ensures"`
		Assumes     []string `json:"assumes"`
	}
	var clauses []clause
	for _, c := range ref.Clauses {
		req := ""
		if c.Requires != nil {
			req = string(must(json.Marshal(c.Requires)))
		}
		clauses = append(clauses, clause{c.ID, c.IntentIDs, c.Description, req, string(must(json.Marshal(c.Ensures))), c.Assumes})
	}
	var scenarios []map[string]any
	for _, s := range ref.Scenarios {
		scenarios = append(scenarios, map[string]any{"id": s.ID, "intentIds": s.IntentIDs, "given": s.Given, "when": s.When, "then": s.Then,
			"inputJson": string(must(json.Marshal(s.Input))), "expect": string(must(json.Marshal(s.Expect)))})
	}
	answers := map[string]any{
		"extract": map[string]any{"intents": []any{map[string]any{"statement": "Only the right password gets a token.", "kind": "invariant", "provability": "formal", "beyondProofReason": "none", "quote": "only the right password gets a token"}}, "decisions": []any{}, "notPromised": []any{}},
		"derive":  map[string]any{"clauses": clauses, "assumptions": ref.Assumptions, "scenarios": scenarios, "beyondProof": ref.BeyondProof},
	}
	ask := func(_ context.Context, req lastro.AskRequest) (json.RawMessage, error) {
		return json.Marshal(answers[req.Step])
	}
	ex := must(lastro.LLMExtractor{Ask: ask}.Extract(ctx, approved.Prose))
	if ex.Intents[0].Source.Quote != "only the right password gets a token" {
		t.Fatal("extract")
	}
	plan := must(lastro.LLMDeriver{Ask: ask}.Derive(ctx, approved, login.Model))
	r := must(lastro.Verify(approved, plan, lastro.VerifyOptions{Subject: login.Decide("")}))
	if !r.OK {
		t.Fatal("derived plan should prove the reference login")
	}
	clauses[0].Ensures = `{"var":"out.cookie"}`
	answers["derive"].(map[string]any)["clauses"] = clauses
	if _, err := (lastro.LLMDeriver{Ask: ask}).Derive(ctx, approved, login.Model); err == nil || !strings.Contains(err.Error(), "cookie") {
		t.Fatalf("want a cookie error, got %v", err)
	}
}

func TestCarryReview(t *testing.T) {
	before := must(lastro.NewReview(extract(t), "").ApproveAll().Amend("I5", login.FixedI5).Answer("D1", 0).Answer("D2", 0).Done())
	fresh := extract(t)
	next := lastro.CarryReview(before, fresh)
	for _, i := range next.Intents {
		if i.Status == "proposed" {
			t.Fatalf("%s should keep its review", i.ID)
		}
	}
	if next.Intents[4].Statement != login.FixedI5 || next.Intents[4].Status != "amended" {
		t.Fatal("amendment not reapplied")
	}
	fresh.Intents[6].Statement = "A database failure gets 503 and no token."
	changed := lastro.CarryReview(before, fresh)
	if changed.Intents[6].Status != "proposed" || changed.Intents[5].Status == "proposed" {
		t.Fatal("only I7 should need a new look")
	}
}

func TestLSLValidationAndSmt(t *testing.T) {
	if len(lastro.ValidateExpr(lastro.Eq(lastro.In("nope"), 1), login.Model, nil, "expr")) == 0 {
		t.Fatal("unknown variable accepted")
	}
	if e := lastro.ValidateExpr(lastro.Eq(lastro.In("sends"), "email"), login.Model, nil, "expr"); len(e) != 0 {
		t.Fatal(e)
	}
	bad := must(lastro.ParseExpr(`{"op":"eq","args":[{"var":"in.sends"}]}`))
	if e := lastro.ValidateExpr(bad, login.Model, nil, "C1"); len(e) != 1 || e[0] != "C1: op 'eq' arity 1" {
		t.Fatal(e)
	}
	plan := login.DerivePlan(reviewed(t, 0, true))
	smt := lastro.ToSmtLib(plan.Model, plan.Clauses[0].Requires, plan.Clauses[0].Ensures, nil)
	if !strings.Contains(smt, "(assert (not ") || strings.Count(smt, "(") != strings.Count(smt, ")") {
		t.Fatal(smt)
	}
	if got := lastro.Show(plan.Clauses[0].Ensures); got != `(out.status = 400) ∧ ¬out.token ∧ (out.role = "none")` {
		t.Fatal(got)
	}
}

func TestRustProverAgreesOnEveryBug(t *testing.T) {
	prover := rustProver()
	if prover == nil {
		t.Skip("lastro-prove not found: cargo build --release -p lastro-core")
	}
	a := reviewed(t, 0, true)
	plan := login.DerivePlan(a)
	for _, bug := range append([]string{""}, login.Bugs...) {
		subject := login.Decide(bug)
		ts := must(lastro.Verify(a, plan, lastro.VerifyOptions{Subject: subject}))
		rs := must(prover.Prove(context.Background(), plan.Model, plan.Clauses, must(lastro.Tabulate(plan.Model, subject, 0))))
		if !rs.Coverage.Complete || *rs.Coverage.Space != 72 {
			t.Fatalf("coverage %+v", rs.Coverage)
		}
		allProved := true
		for _, c := range ts.Clauses {
			allProved = allProved && c.Verdict == "proved"
			for _, r := range rs.Clauses {
				if r.ClauseID == c.ClauseID && r.Verdict != c.Verdict {
					t.Fatalf("%q %s: go %s, rust %s", bug, c.ClauseID, c.Verdict, r.Verdict)
				}
			}
		}
		if rs.OK != allProved {
			t.Fatalf("%q: rust ok %v", bug, rs.OK)
		}
	}
	table := must(lastro.Tabulate(plan.Model, login.Decide(""), 0))[:50]
	rs := must(prover.Prove(context.Background(), plan.Model, plan.Clauses, table))
	if rs.Coverage.Complete || rs.Coverage.Missing != 22 {
		t.Fatalf("partial table: %+v", rs.Coverage)
	}
}

func rustProver() lastro.RustProver {
	if p := lastro.FindRustProver(); p != nil {
		return p
	}
	return nil
}

func matches(xs []string, pattern string) bool {
	re := regexp.MustCompile(pattern)
	for _, x := range xs {
		if re.MatchString(x) {
			return true
		}
	}
	return false
}
