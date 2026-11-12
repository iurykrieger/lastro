package lastro

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// The verification engine: contrasts a ProofPlan with the program's final
// output (a decision function and/or recorded traces). It is a port of
// sdks/typescript/src/verify/engine.ts and must agree with it verdict by verdict.
//
//   - Finite input space -> exhaustive enumeration -> proved (formal).
//   - Otherwise          -> seeded sampling        -> evidenced (empirical, rule of three).
//   - Every clause must kill at least one planted mutant, or it is weak.
//   - Clauses whose if-part never holds are vacuous.
//   - Jointly unsatisfiable clauses become decision cards (spec contradiction).

// VerifyOptions tunes the engine. Zero values mean the TS SDK's defaults.
type VerifyOptions struct {
	Subject Subject
	Traces  []Trace
	// ExhaustiveLimit is the max input-space size enumerated exhaustively (default 100000).
	ExhaustiveLimit int
	// Samples is how many inputs are sampled when the space is larger (default 2000).
	Samples int
	// Seed of the sampler (default 42).
	Seed *uint32
	// NoMutation turns the mutation check off.
	NoMutation bool
	// ConsistencyLimit is the max output-space size for the consistency check (default 4096).
	ConsistencyLimit int
}

type run struct{ input, output Env }

func (o VerifyOptions) exhaustiveLimit() float64 {
	if o.ExhaustiveLimit > 0 {
		return float64(o.ExhaustiveLimit)
	}
	return 100_000
}

func inputSet(model Model, o VerifyOptions) ([]Env, bool, error) {
	if SpaceSize(model.Inputs) <= o.exhaustiveLimit() {
		all, err := EnumerateSpace(model.Inputs)
		return all, true, err
	}
	seed := uint32(42)
	if o.Seed != nil {
		seed = *o.Seed
	}
	n := o.Samples
	if n <= 0 {
		n = 2000
	}
	next := rng(seed)
	out := make([]Env, n)
	for i := range out {
		out[i] = sampleSpace(model.Inputs, next)
	}
	return out, false, nil
}

func copyEnv(e Env) Env {
	out := make(Env, len(e))
	for k, v := range e {
		out[k] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// Single-run explanation
// ---------------------------------------------------------------------------

// ClauseExplanation says what one clause made of one run.
type ClauseExplanation struct {
	ClauseID string `json:"clauseId"`
	Status   string `json:"status"` // if-part-false | holds | fails | error
	Error    string `json:"error,omitempty"`
}

// ExplainRun classifies each clause for a single input/output pair.
func ExplainRun(plan ProofPlan, input, output Env) []ClauseExplanation {
	out := make([]ClauseExplanation, len(plan.Clauses))
	ctx := EvalContext{Input: input, Output: output, Defs: plan.Model.Defs}
	for i, c := range plan.Clauses {
		out[i] = ClauseExplanation{ClauseID: c.ID}
		if c.Requires != nil {
			ok, err := Holds(*c.Requires, ctx)
			if err != nil {
				out[i].Status, out[i].Error = "error", err.Error()
				continue
			}
			if !ok {
				out[i].Status = "if-part-false"
				continue
			}
		}
		ok, err := Holds(c.Ensures, ctx)
		switch {
		case err != nil:
			out[i].Status, out[i].Error = "error", err.Error()
		case ok:
			out[i].Status = "holds"
		default:
			out[i].Status = "fails"
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Spec consistency
// ---------------------------------------------------------------------------

// CheckConsistency asks, for every input, whether at least one output satisfies all applicable
// clauses. If not, the spec contradicts itself and the team must decide: one card per set of clashing clauses.
func CheckConsistency(plan ProofPlan, o VerifyOptions) ([]DecisionCard, error) {
	limit := 4096.0
	if o.ConsistencyLimit > 0 {
		limit = float64(o.ConsistencyLimit)
	}
	if len(plan.Clauses) == 0 || SpaceSize(plan.Model.Outputs) > limit {
		return []DecisionCard{}, nil
	}
	outputs, err := EnumerateSpace(plan.Model.Outputs)
	if err != nil {
		return nil, err
	}
	inputs, _, err := inputSet(plan.Model, o)
	if err != nil {
		return nil, err
	}
	defs := plan.Model.Defs
	cards := []DecisionCard{}
	seen := map[string]bool{}
	for _, input := range inputs {
		var applicable []Clause
		for _, c := range plan.Clauses {
			if c.Requires == nil {
				applicable = append(applicable, c)
				continue
			}
			for _, out := range outputs {
				ok, err := Holds(*c.Requires, EvalContext{Input: input, Output: out, Defs: defs})
				if err != nil || ok {
					applicable = append(applicable, c)
					break
				}
			}
		}
		satisfiable := false
		for _, out := range outputs {
			all := true
			for _, c := range plan.Clauses {
				ctx := EvalContext{Input: input, Output: out, Defs: defs}
				// A clause that cannot be evaluated here does not rule the output out (as in the Rust prover).
				if c.Requires != nil {
					req, err := Holds(*c.Requires, ctx)
					if err != nil || !req {
						continue
					}
				}
				ens, err := Holds(c.Ensures, ctx)
				if err != nil {
					continue
				}
				if !ens {
					all = false
					break
				}
			}
			if all {
				satisfiable = true
				break
			}
		}
		if satisfiable {
			continue
		}
		ids := make([]string, len(applicable))
		for i, c := range applicable {
			ids[i] = c.ID
		}
		key := strings.Join(ids, ",")
		if seen[key] {
			continue
		}
		seen[key] = true
		intentIDs := []string{}
		options := []DecisionOption{}
		for _, c := range applicable {
			for _, id := range c.IntentIDs {
				if !contains(intentIDs, id) {
					intentIDs = append(intentIDs, id)
				}
			}
			options = append(options, DecisionOption{Label: "Relax " + c.ID, Consequence: c.Description})
		}
		cards = append(cards, DecisionCard{
			ID:          fmt.Sprintf("X%d", len(cards)+1),
			IntentIDs:   intentIDs,
			Question:    fmt.Sprintf("Clauses %s cannot all hold for input %s. Which one should give way?", key, Stringify(input)),
			Options:     options,
			Recommended: 0,
			Origin:      "consistency-check",
		})
	}
	return cards, nil
}

// ---------------------------------------------------------------------------
// Main entry point
// ---------------------------------------------------------------------------

// Verify checks the plan against the subject (every input of a finite space, or seeded samples)
// and against the traces, and rolls the results up per intent.
func Verify(artifact IntentArtifact, plan ProofPlan, o VerifyOptions) (VerificationReport, error) {
	check := CheckPlan(artifact, plan)
	if len(check.Errors) > 0 {
		return VerificationReport{}, &PlanError{Errors: check.Errors}
	}
	model := plan.Model
	defs := model.Defs

	// 1. Run the program over the input set.
	var runs []run
	exhaustive := false
	var runErrors []string
	if o.Subject != nil {
		inputs, ex, err := inputSet(model, o)
		if err != nil {
			return VerificationReport{}, err
		}
		exhaustive = ex
		for _, input := range inputs {
			output, err := o.Subject(copyEnv(input))
			if err != nil {
				runErrors = append(runErrors, fmt.Sprintf("subject threw for %s: %s", Stringify(input), err.Error()))
				continue
			}
			output = NormalizeEnv(output)
			for _, f := range model.Outputs {
				if !f.Domain.Contains(output[f.Name]) {
					runErrors = append(runErrors, fmt.Sprintf("output '%s'=%s outside its domain for %s", f.Name, jsonOrUndefined(output, f.Name), Stringify(input)))
				}
			}
			runs = append(runs, run{input, output})
		}
		if len(runErrors) > 0 {
			valid := runs[:0]
			for _, r := range runs {
				if outputsInDomain(model, r.output) {
					valid = append(valid, r)
				}
			}
			runs = valid
		}
	}

	// 2. Clauses.
	var mutants []mutant
	if !o.NoMutation {
		mutants = outputMutants(model)
	}
	clauses := make([]ClauseResult, len(plan.Clauses))
	for i, c := range plan.Clauses {
		clauses[i] = checkClause(c, runs, exhaustive, mutants, o.Traces, model)
		if len(runErrors) > 0 {
			note := fmt.Sprintf("%d invalid runs (e.g. %s)", len(runErrors), runErrors[0])
			if clauses[i].Note != "" {
				note = clauses[i].Note + "; " + note
			}
			clauses[i].Note = note
		}
	}

	// 3. Scenarios (Given/When/Then).
	scenarios := []ScenarioResult{}
	for _, s := range plan.Scenarios {
		if o.Subject == nil {
			scenarios = append(scenarios, ScenarioResult{ScenarioID: s.ID, Status: "failed", Error: "no subject to run"})
			continue
		}
		input := NormalizeEnv(Env(s.Input))
		output, err := o.Subject(copyEnv(input))
		if err != nil {
			scenarios = append(scenarios, ScenarioResult{ScenarioID: s.ID, Status: "failed", Error: err.Error()})
			continue
		}
		output = NormalizeEnv(output)
		ok, err := Holds(s.Expect, EvalContext{Input: input, Output: output, Defs: defs})
		if err != nil {
			scenarios = append(scenarios, ScenarioResult{ScenarioID: s.ID, Status: "failed", Error: err.Error()})
			continue
		}
		status := "failed"
		if ok {
			status = "passed"
		}
		scenarios = append(scenarios, ScenarioResult{ScenarioID: s.ID, Status: status, Output: output})
	}

	// 4. Spec consistency.
	decisions, err := CheckConsistency(plan, o)
	if err != nil {
		return VerificationReport{}, err
	}

	// 5. Roll up per intent.
	intents := rollup(artifact, plan, clauses, scenarios, check.Uncovered)
	summary := map[string]int{}
	for _, v := range IntentVerdicts {
		summary[v] = 0
	}
	for _, i := range intents {
		summary[i.Verdict]++
	}
	planHash, err := HashOf(plan)
	if err != nil {
		return VerificationReport{}, err
	}
	return VerificationReport{
		Lastro:       "verification-report/v0",
		ArtifactHash: artifact.Hash,
		PlanHash:     planHash,
		Model:        model.Name,
		Clauses:      clauses,
		Scenarios:    scenarios,
		Intents:      intents,
		Assumptions:  nonNil(plan.Assumptions),
		BeyondProof:  nonNil(plan.BeyondProof),
		NotPromised:  nonNil(artifact.NotPromised),
		Decisions:    decisions,
		Summary:      summary,
		OK:           summary["violated"] == 0 && summary["gap"] == 0 && len(decisions) == 0,
	}, nil
}

func jsonOrUndefined(e Env, k string) string {
	v, ok := e[k]
	if !ok {
		return "undefined"
	}
	return Stringify(v)
}

func outputsInDomain(model Model, out Env) bool {
	for _, f := range model.Outputs {
		if !f.Domain.Contains(out[f.Name]) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Clause checking
// ---------------------------------------------------------------------------

type mutant struct {
	label string
	apply func(Env) Env
}

// outputMutants plants one bug on the output side: negate a boolean, force a field to each
// value of a small domain, or shift an integer by one.
func outputMutants(model Model) []mutant {
	var ms []mutant
	set := func(field string, f func(any) any) func(Env) Env {
		return func(o Env) Env {
			c := copyEnv(o)
			c[field] = f(o[field])
			return c
		}
	}
	for _, fd := range model.Outputs {
		field, d := fd.Name, fd.Domain
		if d.Type == "bool" {
			ms = append(ms, mutant{"¬" + field, set(field, func(v any) any { b, _ := v.(bool); return !b })})
		}
		if d.Size() <= 16 {
			vs, _ := d.Enumerate()
			for _, v := range vs {
				v := v
				ms = append(ms, mutant{field + ":=" + Stringify(v), set(field, func(any) any { return v })})
			}
		}
		if d.Type == "int" {
			for _, delta := range []float64{1, -1} {
				delta := delta
				ms = append(ms, mutant{fmt.Sprintf("%s%+d", field, int(delta)), set(field, func(v any) any {
					n, ok := v.(float64)
					if !ok {
						return math.NaN()
					}
					return n + delta
				})})
			}
		}
	}
	return ms
}

func checkClause(c Clause, runs []run, exhaustive bool, mutants []mutant, traces []Trace, model Model) ClauseResult {
	defs := model.Defs
	applies := func(in, out Env) (bool, error) {
		if c.Requires == nil {
			return true, nil
		}
		return Holds(*c.Requires, EvalContext{Input: in, Output: out, Defs: defs})
	}
	ok := func(in, out Env) (bool, error) {
		return Holds(c.Ensures, EvalContext{Input: in, Output: out, Defs: defs})
	}
	result := ClauseResult{ClauseID: c.ID, Verdict: "not-run", Evidence: "none", Method: "none"}

	err := func() error {
		if len(runs) > 0 {
			var applicable []run
			for _, r := range runs {
				a, err := applies(r.input, r.output)
				if err != nil {
					return err
				}
				if a {
					applicable = append(applicable, r)
				}
			}
			result.Checked = len(applicable)
			result.Method = "sampling"
			if exhaustive {
				result.Method = "exhaustive"
			}
			var bad *run
			for i := range applicable {
				good, err := ok(applicable[i].input, applicable[i].output)
				if err != nil {
					return err
				}
				if !good {
					bad = &applicable[i]
					break
				}
			}
			switch {
			case bad != nil:
				result.Verdict = "violated"
				result.Evidence = "empirical"
				if exhaustive {
					result.Evidence = "formal"
				}
				result.Counterexample = &Counterexample{Input: bad.input, Output: bad.output}
			case len(applicable) == 0:
				result.Verdict = "vacuous"
				result.Note = "no run makes the if-part true: the clause holds vacuously and proves nothing"
			case exhaustive:
				result.Verdict, result.Evidence = "proved", "formal"
			default:
				result.Verdict, result.Evidence = "evidenced", "empirical"
				bound := math.Min(1, 3/float64(len(applicable)))
				result.FailureRateUpperBound95 = &bound
			}

			// Mutation check: the clause must reject at least one planted bug.
			if len(mutants) > 0 && (result.Verdict == "proved" || result.Verdict == "evidenced") {
				killed := 0
				for _, m := range mutants {
					for _, r := range runs {
						mo := m.apply(r.output)
						a, err := applies(r.input, mo)
						if err != nil {
							return err
						}
						if !a {
							continue
						}
						good, err := ok(r.input, mo)
						if err != nil {
							return err
						}
						if !good {
							killed++
							break
						}
					}
				}
				result.Mutants = &MutantStats{Total: len(mutants), Killed: killed}
				if killed == 0 {
					result.Verdict = "weak"
					result.Note = "no planted mutant was rejected: the clause verifies but rules out no bug (vibe verification)"
				}
			}
		}

		// Production traces: the model proof can hold while reality disagrees.
		if len(traces) > 0 {
			applicable := 0
			var bad *Trace
			for i := range traces {
				t := traces[i]
				in, out := NormalizeEnv(t.Input), NormalizeEnv(t.Output)
				a, err := applies(in, out)
				if err != nil {
					return err
				}
				if !a {
					continue
				}
				applicable++
				if bad == nil {
					good, err := ok(in, out)
					if err != nil {
						return err
					}
					if !good {
						bad = &Trace{Input: in, Output: out, Ref: t.Ref}
					}
				}
			}
			result.TracesChecked = &applicable
			if bad != nil {
				provedBefore := result.Verdict == "proved"
				result.Verdict, result.Evidence = "violated", "empirical"
				result.TraceViolation = bad
				if provedBefore {
					result.Note = "proved on the model, but a real trace violates it: a trusted piece (assumption) does not hold"
				}
			} else if len(runs) == 0 && applicable > 0 {
				result.Method = "traces"
				result.Checked = applicable
				result.Verdict, result.Evidence = "evidenced", "empirical"
				bound := math.Min(1, 3/float64(applicable))
				result.FailureRateUpperBound95 = &bound
			}
		}
		return nil
	}()
	if err != nil {
		var le *LslError
		if !errors.As(err, &le) {
			panic(err)
		}
		result.Verdict = "violated"
		result.Note = "evaluation error: " + err.Error()
	}
	return result
}

// ---------------------------------------------------------------------------
// Intent roll-up
// ---------------------------------------------------------------------------

func rollup(artifact IntentArtifact, plan ProofPlan, clauses []ClauseResult, scenarios []ScenarioResult, uncovered []string) []IntentResult {
	out := []IntentResult{}
	for _, intent := range ApprovedIntents(artifact) {
		id := intent.ID
		var cIDs, aIDs, assumed, sIDs []string
		for _, c := range plan.Clauses {
			if contains(c.IntentIDs, id) {
				cIDs = append(cIDs, c.ID)
				assumed = append(assumed, c.Assumes...)
			}
		}
		var cRes []ClauseResult
		for _, r := range clauses {
			if contains(cIDs, r.ClauseID) {
				cRes = append(cRes, r)
			}
		}
		for _, a := range plan.Assumptions {
			if contains(a.IntentIDs, id) {
				aIDs = append(aIDs, a.ID)
			}
		}
		all := uniq(append(append([]string{}, aIDs...), assumed...))
		for _, s := range plan.Scenarios {
			if contains(s.IntentIDs, id) {
				sIDs = append(sIDs, s.ID)
			}
		}
		var sRes []ScenarioResult
		for _, r := range scenarios {
			if contains(sIDs, r.ScenarioID) {
				sRes = append(sRes, r)
			}
		}
		var beyond []BeyondProofItem
		for _, b := range plan.BeyondProof {
			if b.IntentID == id {
				beyond = append(beyond, b)
			}
		}
		notes := []string{}
		base := IntentResult{IntentID: id, Statement: intent.Statement, Clauses: nonNil(cIDs), Assumptions: nonNil(all), Scenarios: nonNil(sIDs)}
		var evidence []string
		for _, r := range cRes {
			if r.Evidence != "none" {
				evidence = append(evidence, r.Evidence)
			}
		}
		evidence = uniq(evidence)
		if len(all) > 0 {
			evidence = append(evidence, "trusted")
		}
		if len(sRes) > 0 && !contains(evidence, "empirical") {
			evidence = append(evidence, "empirical")
		}
		evidence = nonNil(evidence)
		res := func(verdict string, ev []string, confidence float64, n []string) IntentResult {
			r := base
			r.Verdict, r.Evidence, r.Confidence, r.Notes = verdict, ev, confidence, n
			return r
		}

		if contains(uncovered, id) {
			out = append(out, res("gap", []string{"none"}, 0, []string{"approved intent with no derived obligation"}))
			continue
		}
		var violated []ClauseResult
		var failedScenarios []ScenarioResult
		for _, r := range cRes {
			if r.Verdict == "violated" {
				violated = append(violated, r)
			}
		}
		for _, r := range sRes {
			if r.Status == "failed" {
				failedScenarios = append(failedScenarios, r)
			}
		}
		if len(violated) > 0 || len(failedScenarios) > 0 {
			for _, r := range violated {
				notes = append(notes, r.ClauseID+" violated")
			}
			for _, r := range failedScenarios {
				n := r.ScenarioID + " failed"
				if r.Error != "" {
					n += ": " + r.Error
				}
				notes = append(notes, n)
			}
			out = append(out, res("violated", evidence, 0, notes))
			continue
		}
		for _, r := range cRes {
			if r.Verdict == "weak" || r.Verdict == "vacuous" {
				notes = append(notes, fmt.Sprintf("%s %s: %s", r.ClauseID, r.Verdict, r.Note))
			}
		}
		for _, r := range cRes {
			if r.Verdict == "not-run" {
				notes = append(notes, r.ClauseID+" not run (no subject/traces)")
			}
		}
		var useful []ClauseResult
		for _, r := range cRes {
			if r.Verdict == "proved" || r.Verdict == "evidenced" {
				useful = append(useful, r)
			}
		}
		orNone := func(ev []string) []string {
			if len(ev) == 0 {
				return []string{"none"}
			}
			return ev
		}
		if len(beyond) > 0 && len(useful) == 0 {
			for _, b := range beyond {
				notes = append(notes, fmt.Sprintf("beyond proof (%s): %s", b.Reason, b.Note))
			}
			if len(sRes) > 0 {
				notes = append(notes, fmt.Sprintf("%d scenario(s) passed as partial evidence", len(sRes)))
			}
			out = append(out, res("beyond-proof", orNone(evidence), 0, notes))
			continue
		}
		if len(useful) == 0 {
			switch {
			case len(sRes) > 0:
				notes = append(notes, "examples only (scenarios), no sampling: confidence not quantified")
				out = append(out, res("evidenced", evidence, 0, notes))
			case len(all) > 0:
				notes = append(notes, "backed by trusted pieces only")
				out = append(out, res("evidenced", evidence, 0, notes))
			default:
				out = append(out, res("gap", []string{"none"}, 0, notes))
			}
			continue
		}
		allProved := true
		for _, r := range useful {
			if r.Verdict != "proved" {
				allProved = false
			}
		}
		if allProved && len(useful) == len(cRes) {
			verdict := "proved"
			if len(all) > 0 {
				verdict = "proved-under-assumptions"
			}
			out = append(out, res(verdict, evidence, 1, notes))
			continue
		}
		bound := math.Inf(-1)
		for _, r := range useful {
			b := 0.0
			if r.FailureRateUpperBound95 != nil {
				b = *r.FailureRateUpperBound95
			}
			bound = math.Max(bound, b)
		}
		if len(useful) != len(cRes) {
			notes = append(notes, "not every clause carries a useful guarantee")
		}
		out = append(out, res("evidenced", evidence, 1-bound, notes))
	}
	return out
}

func uniq(xs []string) []string {
	out := []string{}
	for _, x := range xs {
		if !contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}
