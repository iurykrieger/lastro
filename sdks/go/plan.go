package lastro

import (
	"fmt"
	"strings"
)

// Deterministic checks on a derived ProofPlan. The deriver may be an LLM; this
// is what keeps its output honest before anything is verified.

// PlanCheck is the result of CheckPlan.
type PlanCheck struct {
	Errors []string
	// Uncovered lists approved intents with no obligation at all. They become `gap` in the report.
	Uncovered []string
}

// PlanError lists the structural problems of a plan.
type PlanError struct{ Errors []string }

func (e *PlanError) Error() string {
	return "Invalid proof plan:\n - " + strings.Join(e.Errors, "\n - ")
}

// CheckPlan checks that only approved intents are cited, every expression uses only model
// variables, ids are unique and scenario inputs are in their domains.
func CheckPlan(artifact IntentArtifact, plan ProofPlan) PlanCheck {
	errs := []string{}
	if artifact.Hash == "" || !IsApproved(artifact) {
		errs = append(errs, "artifact is not approved or was modified after approve()")
	}
	if plan.ArtifactHash != artifact.Hash {
		errs = append(errs, fmt.Sprintf("plan was derived from %s, artifact is %s", plan.ArtifactHash, artifact.Hash))
	}
	approvedList := ApprovedIntents(artifact)
	approved := map[string]bool{}
	for _, i := range approvedList {
		approved[i.ID] = true
	}
	known := map[string]bool{}
	for _, i := range artifact.Intents {
		known[i.ID] = true
	}
	refIntents := func(where string, ids []string) {
		if len(ids) == 0 {
			errs = append(errs, where+": must reference at least one intent")
		}
		for _, id := range ids {
			if !known[id] {
				errs = append(errs, fmt.Sprintf("%s: unknown intent %s", where, id))
			} else if !approved[id] {
				errs = append(errs, fmt.Sprintf("%s: intent %s was not approved (rejected intents must not be built)", where, id))
			}
		}
	}
	seen := map[string]bool{}
	unique := func(id string) {
		if seen[id] {
			errs = append(errs, "duplicate id "+id)
		}
		seen[id] = true
	}
	model := plan.Model
	if len(model.Outputs) == 0 {
		errs = append(errs, "model has no outputs")
	}
	for _, d := range model.Defs {
		errs = append(errs, ValidateExpr(d.Body, model, d.Params, "def "+d.Name)...)
	}
	assumptionIDs, scenarioIDs := map[string]bool{}, map[string]bool{}
	for _, a := range plan.Assumptions {
		assumptionIDs[a.ID] = true
	}
	for _, s := range plan.Scenarios {
		scenarioIDs[s.ID] = true
	}
	for _, c := range plan.Clauses {
		unique(c.ID)
		refIntents(c.ID, c.IntentIDs)
		if c.Requires != nil {
			errs = append(errs, ValidateExpr(*c.Requires, model, nil, c.ID+".requires")...)
		}
		errs = append(errs, ValidateExpr(c.Ensures, model, nil, c.ID+".ensures")...)
		for _, t := range c.Assumes {
			if !assumptionIDs[t] {
				errs = append(errs, fmt.Sprintf("%s: unknown assumption %s", c.ID, t))
			}
		}
	}
	for _, a := range plan.Assumptions {
		unique(a.ID)
		refIntents(a.ID, a.IntentIDs)
		for _, s := range a.CheckedBy {
			if !scenarioIDs[s] {
				errs = append(errs, fmt.Sprintf("%s: checkedBy unknown scenario %s", a.ID, s))
			}
		}
	}
	for _, s := range plan.Scenarios {
		unique(s.ID)
		refIntents(s.ID, s.IntentIDs)
		errs = append(errs, ValidateExpr(s.Expect, model, nil, s.ID+".expect")...)
		for _, f := range model.Inputs {
			v, ok := s.Input[f.Name]
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: input '%s' missing", s.ID, f.Name))
			} else if !f.Domain.Contains(v) {
				errs = append(errs, fmt.Sprintf("%s: input '%s'=%s outside its domain", s.ID, f.Name, Stringify(v)))
			}
		}
	}
	for _, b := range plan.BeyondProof {
		refIntents(fmt.Sprintf("beyondProof(%s)", b.IntentID), []string{b.IntentID})
		if !isReason(b.Reason) {
			errs = append(errs, fmt.Sprintf("beyondProof(%s): unknown reason %s", b.IntentID, b.Reason))
		}
	}
	covered := map[string]bool{}
	for _, c := range plan.Clauses {
		markAll(covered, c.IntentIDs)
	}
	for _, a := range plan.Assumptions {
		markAll(covered, a.IntentIDs)
	}
	for _, s := range plan.Scenarios {
		markAll(covered, s.IntentIDs)
	}
	for _, b := range plan.BeyondProof {
		covered[b.IntentID] = true
	}
	uncovered := []string{}
	for _, i := range approvedList {
		if !covered[i.ID] {
			uncovered = append(uncovered, i.ID)
		}
	}
	return PlanCheck{Errors: errs, Uncovered: uncovered}
}

func markAll(set map[string]bool, ids []string) {
	for _, id := range ids {
		set[id] = true
	}
}

// ValidatePlan fails on structural errors and returns the uncovered intents (reported as gaps).
func ValidatePlan(artifact IntentArtifact, plan ProofPlan) ([]string, error) {
	r := CheckPlan(artifact, plan)
	if len(r.Errors) > 0 {
		return nil, &PlanError{Errors: r.Errors}
	}
	return r.Uncovered, nil
}
