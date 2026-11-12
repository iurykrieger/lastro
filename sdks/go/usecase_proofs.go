package lastro

// Proofs derived straight from the established use cases (`lastro-go derive --usecases`), ported from the TS
// SDK's usecase-proofs.ts. A use case's description was already reviewed: these proofs need no proposal and no
// intents. They are numbered inside their use case (UC-003.PF-01), start pending, and become regression tests once
// they pass. A failing one goes to the user, never back to the code.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// UseCaseFeedbackNote is printed with every failing use-case proof.
const UseCaseFeedbackNote = "These proofs come from the use cases' own descriptions, and the code is the reference: a failing one means the description is wrong (change it with a proposal) or the code has a bug the user decides about. Do not change the code to make it pass."

var useCaseIDPattern = regexp.MustCompile(`^UC-\d+$`)

// PassedProofs is a use case's passed proofs: its regression tests. Pending and failed ones are not.
func PassedProofs(u UseCase) []ProofEntry {
	out := []ProofEntry{}
	for _, x := range u.Proofs {
		if x.Status == "passed" {
			out = append(out, x)
		}
	}
	return out
}

// IsUseCaseList reports whether s is a comma-separated list of use case ids ("UC-003,UC-016").
func IsUseCaseList(s string) bool {
	if s == "" {
		return false
	}
	for _, x := range strings.Split(s, ",") {
		if !useCaseIDPattern.MatchString(strings.TrimSpace(x)) {
			return false
		}
	}
	return true
}

// UseCaseTargets is the use cases derive --usecases works on: without ids, every use case with no passed proof;
// with ids, those, refusing any that already has passed proofs.
func UseCaseTargets(useCases []UseCase, ids []string) ([]UseCase, error) {
	if len(ids) == 0 {
		out := []UseCase{}
		for _, u := range useCases {
			if len(PassedProofs(u)) == 0 {
				out = append(out, u)
			}
		}
		return out, nil
	}
	problems := []string{}
	out := []UseCase{}
	seen := map[string]bool{}
	for _, id := range ids {
		found := -1
		for k, u := range useCases {
			if u.ID == id || u.Slug == id {
				found = k
				break
			}
		}
		switch {
		case found < 0:
			problems = append(problems, "no use case "+id)
		case len(PassedProofs(useCases[found])) > 0:
			problems = append(problems, useCases[found].ID+" already has proofs; they change only through a proposal")
		case !seen[useCases[found].ID]:
			seen[useCases[found].ID] = true
			out = append(out, useCases[found])
		}
	}
	if len(problems) > 0 {
		return nil, &ProposalError{problems}
	}
	return out, nil
}

// UseCaseDeriveOut is the derive step's answer for use cases.
type UseCaseDeriveOut struct {
	Proofs []struct {
		UseCase     string          `json:"useCase"`
		Slug        string          `json:"slug"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Model       string          `json:"model"`
		Subject     string          `json:"subject"`
		Kind        string          `json:"kind"`
		Requires    json.RawMessage `json:"requires"`
		Ensures     json.RawMessage `json:"ensures"`
		Assumptions []string        `json:"assumptions"`
		InputJSON   json.RawMessage `json:"inputJson"`
		Expect      json.RawMessage `json:"expect"`
	} `json:"proofs"`
	Unprovable []Unprovable `json:"unprovable"`
}

// Unprovable is a target use case the declared models cannot express, with the reason.
type Unprovable struct {
	UseCase string `json:"useCase"`
	Reason  string `json:"reason"`
}

// UseCaseDerivation is the result of deriving proofs for use cases.
type UseCaseDerivation struct {
	// UseCases are the targets with their new pending proofs (earlier pending or failed ones are replaced).
	UseCases   []UseCase
	Derived    []ProofEntry
	Unprovable []Unprovable
}

func nextProofNumber(u UseCase) int {
	re := regexp.MustCompile(`^` + regexp.QuoteMeta(u.ID) + `\.PF-(\d+)$`)
	max := 0
	for _, x := range u.Proofs {
		if m := re.FindStringSubmatch(x.ID); m != nil {
			if n, _ := strconv.Atoi(m[1]); n > max {
				max = n
			}
		}
	}
	return max + 1
}

// ProofsFromUseCases checks the derive step's answer for targets and attaches the pending proofs (see the TS SDK's
// proofsFromUseCases).
func ProofsFromUseCases(useCases []UseCase, targets []string, setup ProofSetup, out UseCaseDeriveOut) (UseCaseDerivation, error) {
	problems := []string{}
	byID := map[string]UseCase{}
	order := []string{}
	wanted := map[string]bool{}
	for _, t := range targets {
		wanted[t] = true
	}
	for _, u := range useCases {
		if wanted[u.ID] {
			byID[u.ID] = u
			order = append(order, u.ID)
		}
	}
	for _, t := range targets {
		if _, ok := byID[t]; !ok {
			problems = append(problems, "no use case "+t)
		}
	}
	next := map[string]int{}
	for id, u := range byID {
		next[id] = nextProofNumber(u)
	}
	slugs := map[string]map[string]bool{}
	derived := []ProofEntry{}
	for _, d := range out.Proofs {
		u, ok := byID[d.UseCase]
		if !ok {
			t := strings.Join(targets, ", ")
			if t == "" {
				t = "none"
			}
			problems = append(problems, fmt.Sprintf("proof %q: %s is not one of the use cases to prove (%s)", d.Slug, d.UseCase, t))
			continue
		}
		pid := u.ID + ".PF-" + pad(next[u.ID], 2)
		next[u.ID]++
		if !SlugPattern.MatchString(d.Slug) {
			problems = append(problems, fmt.Sprintf("%s: slug %q is not kebab-case", pid, d.Slug))
		}
		if slugs[u.ID] == nil {
			slugs[u.ID] = map[string]bool{}
		}
		if slugs[u.ID][d.Slug] {
			problems = append(problems, fmt.Sprintf("%s: duplicate proof slug %q", pid, d.Slug))
		}
		slugs[u.ID][d.Slug] = true
		model, hasModel := setup.Models[d.Model]
		if !hasModel {
			declared := strings.Join(setup.ModelOrder, ", ")
			if declared == "" {
				declared = "none"
			}
			problems = append(problems, fmt.Sprintf("%s: unknown model %q (declared: %s)", pid, d.Model, declared))
		}
		if !contains(setup.Subjects, d.Subject) {
			declared := strings.Join(setup.Subjects, ", ")
			if declared == "" {
				declared = "none"
			}
			problems = append(problems, fmt.Sprintf("%s: unknown subject %q (declared: %s)", pid, d.Subject, declared))
		}
		if !hasModel {
			continue
		}
		if check := checkFromAnswer(pid, d.Kind, d.Requires, d.Ensures, d.Assumptions, d.InputJSON, d.Expect, model, &problems); check != nil {
			derived = append(derived, ProofEntry{ID: pid, Slug: d.Slug, Name: d.Name, Description: d.Description, UseCase: u.ID, Model: d.Model, Subject: d.Subject, Check: *check, Status: "pending"})
		}
	}
	unprovable := []Unprovable{}
	for _, x := range out.Unprovable {
		_, ok := byID[x.UseCase]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("unprovable %s: not one of the use cases to prove", x.UseCase))
		case strings.TrimSpace(x.Reason) == "":
			problems = append(problems, fmt.Sprintf("unprovable %s: needs a reason", x.UseCase))
		default:
			unprovable = append(unprovable, x)
		}
	}
	for _, t := range order {
		has, listed := false, false
		for _, d := range out.Proofs {
			has = has || d.UseCase == t
		}
		for _, x := range unprovable {
			listed = listed || x.UseCase == t
		}
		if !has && !listed {
			problems = append(problems, t+": no proof derived and not listed as unprovable")
		}
		if has && listed {
			problems = append(problems, t+": has proofs and is also listed as unprovable")
		}
	}
	if len(problems) > 0 {
		return UseCaseDerivation{}, &ProposalError{problems}
	}
	updated := []UseCase{}
	for _, id := range order {
		v := clone(byID[id])
		mine := []ProofEntry{}
		for _, x := range derived {
			if x.UseCase == id {
				mine = append(mine, x)
			}
		}
		listed := false
		for _, x := range unprovable {
			listed = listed || x.UseCase == id
		}
		if len(mine) > 0 || listed {
			v.Proofs = append(PassedProofs(v), mine...)
		}
		updated = append(updated, v)
	}
	return UseCaseDerivation{UseCases: updated, Derived: derived, Unprovable: unprovable}, nil
}

// UseCaseRun is the result of proving the use cases' proofs (`prove --usecases`).
type UseCaseRun struct {
	OK           bool           `json:"ok"`
	Outcomes     []ProofOutcome `json:"outcomes"`
	Failed       []string       `json:"failed"`
	Regressions  []string       `json:"regressions"`
	Unproved     []string       `json:"unproved"`
	EnginesAgree *bool          `json:"enginesAgree,omitempty"`
	Note         string         `json:"note,omitempty"`
}

// ProveUseCases runs every use case's pending and failed proofs (role "usecase") together with every passed proof
// (role "regression"). A use-case proof that passes becomes passed; one that fails becomes failed. Returns the use
// cases whose proofs changed.
func ProveUseCases(ctx context.Context, useCases []UseCase, o RunOptions) ([]UseCase, UseCaseRun, error) {
	set := []RoleProof{}
	for _, u := range useCases {
		for _, x := range u.Proofs {
			role := "usecase"
			if x.Status == "passed" {
				role = "regression"
			}
			set = append(set, RoleProof{x, role})
		}
	}
	outcomes, agree, err := RunProofs(ctx, set, o)
	if err != nil {
		return nil, UseCaseRun{}, err
	}
	byProof := map[string]ProofOutcome{}
	for _, x := range outcomes {
		byProof[x.Proof] = x
	}
	changed := []UseCase{}
	after := []UseCase{}
	for _, u := range useCases {
		touched := false
		for _, x := range u.Proofs {
			touched = touched || x.Status != "passed"
		}
		if !touched {
			after = append(after, u)
			continue
		}
		v := clone(u)
		for k := range v.Proofs {
			pr := &v.Proofs[k]
			r, ok := byProof[pr.ID]
			if pr.Status == "passed" || !ok {
				continue
			}
			pr.Status = "failed"
			if r.Passed {
				pr.Status = "passed"
			}
			pr.Verdict = r.Verdict
			switch r.Verdict {
			case "proved":
				pr.Evidence = "formal"
			case "not-run":
				pr.Evidence = "none"
			default:
				pr.Evidence = "empirical"
			}
		}
		changed = append(changed, v)
		after = append(after, v)
	}
	run := UseCaseRun{Outcomes: outcomes, Failed: []string{}, Regressions: []string{}, Unproved: []string{}, EnginesAgree: agree}
	for _, x := range outcomes {
		switch {
		case x.Role == "usecase" && !x.Passed:
			run.Failed = append(run.Failed, x.Proof)
		case x.Role == "regression" && !x.Passed:
			run.Regressions = append(run.Regressions, x.Proof)
		}
	}
	for _, u := range after {
		if len(PassedProofs(u)) == 0 {
			run.Unproved = append(run.Unproved, u.ID)
		}
	}
	run.OK = len(run.Failed) == 0 && len(run.Regressions) == 0 && (agree == nil || *agree)
	if len(run.Failed) > 0 {
		run.Note = UseCaseFeedbackNote
	}
	return changed, run, nil
}

// sortedProofFiles keeps the order the TS workspace reads proof files in (file names, sorted).
func sortedProofFiles(names []string) []string {
	out := append([]string{}, names...)
	sort.Strings(out)
	return out
}
