package lastro

// Use cases, proposals, intents and proofs (proposal/v1), field for field the TS
// SDK's (sdks/typescript/src/proposal.ts), so the CLIs share a .lastro/ workspace
// and compute the same hashes.
//
//	prose ─▶ PROPOSAL ─extract─▶ INTENTS (add / modify / remove one use case) ─▶ review ─▶ approve
//	      ─derive─▶ PROOFS (pending) ─▶ code (anyone) ─▶ prove (+ every use case's proofs) ─▶ apply
//
// Everything here is deterministic: the LLM answers come in through
// ProposalFromExtraction and ProofsFromDerivation, which check them before
// anything is kept. Proofs run on the existing engine: each (model, subject)
// group becomes a ProofPlan over a synthetic approved artifact.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Entities
// ---------------------------------------------------------------------------

// ChangeKinds are what an intent does to its use case.
var ChangeKinds = []string{"add", "modify", "remove"}

// ProposalPolicy is how proposals are reviewed (the `review` block of .lastro/config.json).
type ProposalPolicy struct {
	// Approvals is how many distinct reviewers must approve each intent. Default 1.
	Approvals int `json:"approvals"`
	// SubmitterCanApprove says whether the person who submitted the prose counts as a reviewer. Default true.
	SubmitterCanApprove bool `json:"submitterCanApprove"`
}

// DefaultPolicy is one approval, and the submitter counts.
var DefaultPolicy = ProposalPolicy{Approvals: 1, SubmitterCanApprove: true}

// ProofCheck is a clause ("requires ⇒ ensures" for every input) or a scenario (one input and an expectation).
type ProofCheck struct {
	Kind        string   `json:"kind"` // clause | scenario
	Requires    *Expr    `json:"requires,omitempty"`
	Ensures     *Expr    `json:"ensures,omitempty"`
	Assumptions []string `json:"assumptions,omitempty"`
	// Input keeps the key order it was written in, as the TS SDK does.
	Input  json.RawMessage `json:"input,omitempty"`
	Expect *Expr           `json:"expect,omitempty"`
}

func (c ProofCheck) MarshalJSON() ([]byte, error) {
	if c.Kind == "scenario" {
		input := c.Input
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		return json.Marshal(obj{{"kind", c.Kind}, {"input", input}, {"expect", c.Expect}})
	}
	o := obj{{"kind", c.Kind}}
	if c.Requires != nil {
		o = append(o, kv{"requires", c.Requires})
	}
	return json.Marshal(append(o, kv{"ensures", c.Ensures}, kv{"assumptions", nonNil(c.Assumptions)}))
}

// InputEnv is the scenario's input as an Env.
func (c ProofCheck) InputEnv() Env {
	var e Env
	dec := json.NewDecoder(bytes.NewReader(c.Input))
	dec.UseNumber()
	_ = dec.Decode(&e)
	return NormalizeEnv(e)
}

// ProofEntry is a deterministic check of one intent. Once it passes and its proposal is applied,
// it belongs to its use case as a regression test.
type ProofEntry struct {
	ID          string     `json:"id"` // "P-007.PF-05"
	Slug        string     `json:"slug"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Intent      string     `json:"intent"`  // "P-007.I-03"
	UseCase     string     `json:"useCase"` // "UC-012"
	Model       string     `json:"model"`
	Subject     string     `json:"subject"`
	Check       ProofCheck `json:"check"`
	// Replaces lists the superseded proofs this one takes the place of.
	Replaces []string `json:"replaces,omitempty"`
	Status   string   `json:"status"` // pending | passed | failed
	Verdict  string   `json:"verdict,omitempty"`
	Evidence string   `json:"evidence,omitempty"`
}

// UseCase is one thing the application does, as the code implements it, with the proofs that keep it working.
type UseCase struct {
	ID          string       `json:"id"` // "UC-012"
	Slug        string       `json:"slug"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Proofs      []ProofEntry `json:"proofs"`
	// History lists the applied proposals that added or changed it.
	History []string `json:"history"`
}

func (u UseCase) MarshalJSON() ([]byte, error) {
	type alias UseCase
	u.Proofs, u.History = nonNil(u.Proofs), nonNil(u.History)
	return json.Marshal(alias(u))
}

// ProofReplacement is an existing proof an intent retires on purpose, and why.
type ProofReplacement struct {
	Proof  string `json:"proof"`
	Reason string `json:"reason"`
}

// ProposalIntent adds, modifies or removes one use case, in natural language.
type ProposalIntent struct {
	ID                  string             `json:"id"` // "P-007.I-03"
	Slug                string             `json:"slug"`
	Name                string             `json:"name"`
	Description         string             `json:"description"`
	Change              string             `json:"change"` // add | modify | remove
	UseCase             string             `json:"useCase"`
	Provability         string             `json:"provability"`
	BeyondProofReason   string             `json:"beyondProofReason,omitempty"`
	Source              SourceSpan         `json:"source"`
	Replaces            []ProofReplacement `json:"replaces"`
	Status              string             `json:"status"` // proposed | approved | amended | rejected
	Approvals           []string           `json:"approvals"`
	ReviewNote          string             `json:"reviewNote,omitempty"`
	OriginalDescription string             `json:"originalDescription,omitempty"`
	// Grounds is kept as written (set by the TS SDK's bootstrap), so hashes match.
	Grounds json.RawMessage `json:"grounds,omitempty"`
}

func (i ProposalIntent) MarshalJSON() ([]byte, error) {
	type alias ProposalIntent
	i.Replaces, i.Approvals = nonNil(i.Replaces), nonNil(i.Approvals)
	return json.Marshal(alias(i))
}

// UseCaseChange is what a proposal does to one use case.
type UseCaseChange struct {
	UseCase string `json:"useCase"`
	Slug    string `json:"slug"`
	Change  string `json:"change"`
	Name    string `json:"name"`
	// Before is copied from the use case by the SDK (modify and remove), never by the LLM.
	Before string `json:"before,omitempty"`
	// After is the complete new description, every intent combined (add and modify).
	After   *string  `json:"after,omitempty"`
	Intents []string `json:"intents"`
	// Stale is set when an intent of this use case was amended or rejected: After must be rewritten.
	Stale *bool `json:"stale,omitempty"`
}

func (c UseCaseChange) MarshalJSON() ([]byte, error) {
	type alias UseCaseChange
	c.Intents = nonNil(c.Intents)
	return json.Marshal(alias(c))
}

func (c UseCaseChange) isStale() bool { return c.Stale != nil && *c.Stale }

func (c UseCaseChange) after() string {
	if c.After == nil {
		return ""
	}
	return *c.After
}

// ProposalDecision is a question the team answers: an ambiguity in the prose, or a problem derivation found.
type ProposalDecision struct {
	ID          string           `json:"id"` // "D1"
	Slug        string           `json:"slug"`
	Name        string           `json:"name"`
	Origin      string           `json:"origin"` // extraction | derivation | consistency-check
	Question    string           `json:"question"`
	Options     []DecisionOption `json:"options"`
	Recommended int              `json:"recommended"`
	Intents     []string         `json:"intents"`
	Chosen      *int             `json:"chosen,omitempty"`
	AnsweredBy  string           `json:"answeredBy,omitempty"`
}

func (d ProposalDecision) MarshalJSON() ([]byte, error) {
	type alias ProposalDecision
	d.Options, d.Intents = nonNil(d.Options), nonNil(d.Intents)
	return json.Marshal(alias(d))
}

// ThreadComment is one comment in a review thread.
type ThreadComment struct {
	By   string `json:"by"`
	Text string `json:"text"`
}

// Thread is a review conversation on an intent, a proof, a use case or the prose.
type Thread struct {
	ID         string          `json:"id"` // "T1"
	On         string          `json:"on"`
	By         string          `json:"by"`
	Comments   []ThreadComment `json:"comments"`
	Resolved   bool            `json:"resolved"`
	ResolvedBy string          `json:"resolvedBy,omitempty"`
}

func (t Thread) MarshalJSON() ([]byte, error) {
	type alias Thread
	t.Comments = nonNil(t.Comments)
	return json.Marshal(alias(t))
}

// ProofCounterexample is the input that breaks a proof and what the code returned. Both keep their key order.
type ProofCounterexample struct {
	Input  json.RawMessage `json:"input"`
	Output json.RawMessage `json:"output"`
}

// ProofOutcome is one proof's result in a run.
type ProofOutcome struct {
	Proof   string `json:"proof"`
	UseCase string `json:"useCase"`
	Intent  string `json:"intent"`
	// Role is "proposal" (derived for this proposal), "regression" (a use case's proof) or "superseded" (replaced by it).
	Role           string               `json:"role"`
	Verdict        string               `json:"verdict"`
	Passed         bool                 `json:"passed"`
	Counterexample *ProofCounterexample `json:"counterexample,omitempty"`
	Note           string               `json:"note,omitempty"`
}

// ProposalRun is the result of proving a proposal (or of `regress`).
type ProposalRun struct {
	OK       bool           `json:"ok"`
	Outcomes []ProofOutcome `json:"outcomes"`
	// Regressions are use-case proofs (not superseded) that failed.
	Regressions []string `json:"regressions"`
	// ReplacedStillPass are superseded proofs that still pass: the behavior they guarded may not have changed.
	ReplacedStillPass []string `json:"replacedStillPass"`
	// Unproved are use cases that would have no proofs after apply. Reported, never blocking.
	Unproved     []string `json:"unproved"`
	EnginesAgree *bool    `json:"enginesAgree,omitempty"`
}

func (r ProposalRun) MarshalJSON() ([]byte, error) {
	type alias ProposalRun
	r.Outcomes, r.Regressions, r.ReplacedStillPass, r.Unproved = nonNil(r.Outcomes), nonNil(r.Regressions), nonNil(r.ReplacedStillPass), nonNil(r.Unproved)
	return json.Marshal(alias(r))
}

// Proposal is one change: its prose, the intents extracted from it, its review and its proofs.
type Proposal struct {
	Lastro      string             `json:"lastro"` // "proposal/v1"
	ID          string             `json:"id"`     // "P-007"
	Slug        string             `json:"slug"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Status      string             `json:"status"` // open | changes-requested | approved | proving | proved | applied | closed
	SubmittedBy string             `json:"submittedBy"`
	Prose       string             `json:"prose"`
	Baseline    string             `json:"baseline"`
	Intents     []ProposalIntent   `json:"intents"`
	UseCases    []UseCaseChange    `json:"useCases"`
	Decisions   []ProposalDecision `json:"decisions"`
	Threads     []Thread           `json:"threads"`
	NotPromised []string           `json:"notPromised"`
	Policy      ProposalPolicy     `json:"policy"`
	Proofs      []ProofEntry       `json:"proofs"`
	Supersedes  []string           `json:"supersedes"`
	LastRun     *ProposalRun       `json:"lastRun,omitempty"`
	Hash        string             `json:"hash,omitempty"`
	// Origin and Setup are kept as written (set by the TS SDK's bootstrap); approval binds them when present.
	Origin json.RawMessage `json:"origin,omitempty"`
	Setup  json.RawMessage `json:"setup,omitempty"`
}

func (p Proposal) MarshalJSON() ([]byte, error) {
	type alias Proposal
	p.Intents, p.UseCases, p.Decisions, p.Threads = nonNil(p.Intents), nonNil(p.UseCases), nonNil(p.Decisions), nonNil(p.Threads)
	p.NotPromised, p.Proofs, p.Supersedes = nonNil(p.NotPromised), nonNil(p.Proofs), nonNil(p.Supersedes)
	return json.Marshal(alias(p))
}

// ProposalError lists every problem found in an LLM answer or a refused step.
type ProposalError struct{ Problems []string }

func (e *ProposalError) Error() string {
	return "Invalid proposal:\n - " + strings.Join(e.Problems, "\n - ")
}

// ---------------------------------------------------------------------------
// Ids, slugs, baseline
// ---------------------------------------------------------------------------

// SlugPattern is kebab-case.
var SlugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func pad(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// NextID is the next free number after every id ("UC-001" style). Numbers are never reused.
func NextID(prefix string, ids []string) string {
	re := regexp.MustCompile(`^` + prefix + `-(\d+)$`)
	max := 0
	for _, id := range ids {
		if m := re.FindStringSubmatch(id); m != nil {
			if n, _ := strconv.Atoi(m[1]); n > max {
				max = n
			}
		}
	}
	return prefix + "-" + pad(max+1, 3)
}

// FileNameOf is "<id>-<slug>.json".
func FileNameOf(id, slug string) string { return id + "-" + slug + ".json" }

// BaselineOf is the sha256 over the canonical use cases without their proofs: what the prose was read against.
func BaselineOf(useCases []UseCase) string {
	type core struct {
		ID          string `json:"id"`
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	cs := []core{}
	for _, u := range useCases {
		cs = append(cs, core{u.ID, u.Slug, u.Name, u.Description})
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].ID < cs[j].ID })
	return mustHash(cs)
}

// ReadPolicy reads the review policy of a config, including the v0 requiredApprovals key.
func ReadPolicy(review *struct {
	Approvals           *int  `json:"approvals"`
	SubmitterCanApprove *bool `json:"submitterCanApprove"`
}, requiredApprovals *int) ProposalPolicy {
	p := DefaultPolicy
	if review != nil && review.Approvals != nil {
		p.Approvals = *review.Approvals
	} else if requiredApprovals != nil {
		p.Approvals = *requiredApprovals
	}
	if p.Approvals < 1 {
		p.Approvals = 1
	}
	if review != nil && review.SubmitterCanApprove != nil {
		p.SubmitterCanApprove = *review.SubmitterCanApprove
	}
	return p
}

// Anchor finds quote in prose, ignoring case and whitespace differences, and returns the
// prose's own words with UTF-16 offsets. Nil when absent.
func Anchor(prose, quote string) *SourceSpan {
	if quote != "" {
		if i := indexUnits(utf16Units(prose), utf16Units(quote)); i >= 0 {
			return &SourceSpan{Quote: quote, Start: i, End: i + len(utf16Units(quote))}
		}
	}
	words := strings.Fields(quote)
	if len(words) == 0 {
		return nil
	}
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	re, err := regexp.Compile(`(?i)` + strings.Join(words, `\s+`))
	if err != nil {
		return nil
	}
	loc := re.FindStringIndex(prose)
	if loc == nil {
		return nil
	}
	start := len(utf16Units(prose[:loc[0]]))
	m := prose[loc[0]:loc[1]]
	return &SourceSpan{Quote: m, Start: start, End: start + len(utf16Units(m))}
}

// ---------------------------------------------------------------------------
// Extraction: the LLM's answer, checked
// ---------------------------------------------------------------------------

// ProposeOut is what the extract step returns. Use cases are named by slug; the SDK assigns ids.
type ProposeOut struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Intents     []struct {
		Slug              string             `json:"slug"`
		Name              string             `json:"name"`
		Description       string             `json:"description"`
		Change            string             `json:"change"`
		UseCase           string             `json:"useCase"`
		Provability       string             `json:"provability"`
		BeyondProofReason string             `json:"beyondProofReason"`
		Quote             string             `json:"quote"`
		Replaces          []ProofReplacement `json:"replaces"`
	} `json:"intents"`
	UseCases []struct {
		Slug   string `json:"slug"`
		Change string `json:"change"`
		Name   string `json:"name"`
		After  string `json:"after"`
	} `json:"useCases"`
	Decisions []struct {
		Slug        string           `json:"slug"`
		Name        string           `json:"name"`
		Question    string           `json:"question"`
		Options     []DecisionOption `json:"options"`
		Recommended int              `json:"recommended"`
		Intents     []string         `json:"intents"`
	} `json:"decisions"`
	NotPromised []string `json:"notPromised"`
}

// ProposeInput is what a proposal is extracted from.
type ProposeInput struct {
	ID          string
	Prose       string
	SubmittedBy string
	UseCases    []UseCase
	Policy      ProposalPolicy
	// Reserved are use-case ids other open proposals already reserved for their adds; new adds number after them too.
	Reserved []string
}

// ReservedUseCaseIDs are the use-case ids reserved by the adds of proposals that are not applied or closed (excluding except).
func ReservedUseCaseIDs(proposals []Proposal, except string) []string {
	out := []string{}
	for _, p := range proposals {
		if p.ID == except || p.Status == "applied" || p.Status == "closed" {
			continue
		}
		for _, c := range p.UseCases {
			if c.Change == "add" {
				out = append(out, c.UseCase)
			}
		}
	}
	return out
}

func findUseCase(useCases []UseCase, pred func(UseCase) bool) *UseCase {
	for i := range useCases {
		if pred(useCases[i]) {
			return &useCases[i]
		}
	}
	return nil
}

func changeWord(k string) string {
	switch k {
	case "add":
		return "added"
	case "modify":
		return "modified"
	}
	return "removed"
}

// ProposalFromExtraction builds a proposal from the extract step's answer. It returns a *ProposalError with every problem found.
func ProposalFromExtraction(in ProposeInput, out ProposeOut) (Proposal, error) {
	id, prose, useCases := in.ID, in.Prose, in.UseCases
	problems := []string{}
	bySlug := func(s string) *UseCase { return findUseCase(useCases, func(u UseCase) bool { return u.Slug == s }) }
	ids := []string{}
	for _, u := range useCases {
		ids = append(ids, u.ID)
	}
	next, _ := strconv.Atoi(NextID("UC", append(ids, in.Reserved...))[3:])
	reserved, reservedOrder := map[string]string{}, []string{}
	ucID := func(slug, change string) string {
		if e := bySlug(slug); e != nil {
			return e.ID
		}
		if change != "add" {
			return ""
		}
		if _, ok := reserved[slug]; !ok {
			reserved[slug] = "UC-" + pad(next, 3)
			reservedOrder = append(reservedOrder, slug)
			next++
		}
		return reserved[slug]
	}

	if !SlugPattern.MatchString(out.Slug) {
		problems = append(problems, fmt.Sprintf("proposal slug %q is not kebab-case", out.Slug))
	}
	seen := map[string]bool{}
	intents := []ProposalIntent{}
	for n, d := range out.Intents {
		iid := id + ".I-" + pad(n+1, 2)
		if !SlugPattern.MatchString(d.Slug) {
			problems = append(problems, fmt.Sprintf("%s: slug %q is not kebab-case", iid, d.Slug))
		}
		if seen[d.Slug] {
			problems = append(problems, fmt.Sprintf("%s: duplicate intent slug %q", iid, d.Slug))
		}
		seen[d.Slug] = true
		if !contains(ChangeKinds, d.Change) {
			problems = append(problems, fmt.Sprintf("%s: unknown change %q", iid, d.Change))
		}
		existing := bySlug(d.UseCase)
		if d.Change == "add" && existing != nil {
			problems = append(problems, fmt.Sprintf("%s: add names existing use case %s (%s); an add reserves a new slug", iid, existing.ID, d.UseCase))
		}
		if d.Change != "add" && existing == nil {
			problems = append(problems, fmt.Sprintf("%s: %s names unknown use case %q", iid, d.Change, d.UseCase))
		}
		if d.Change == "add" && !SlugPattern.MatchString(d.UseCase) {
			problems = append(problems, fmt.Sprintf("%s: new use-case slug %q is not kebab-case", iid, d.UseCase))
		}
		source := Anchor(prose, d.Quote)
		if source == nil {
			problems = append(problems, fmt.Sprintf("%s: quote not found in prose: %q", iid, d.Quote))
		}
		reason := ""
		if d.BeyondProofReason != "" && d.BeyondProofReason != "none" {
			reason = d.BeyondProofReason
		}
		if d.Provability == "beyond-proof" && reason == "" {
			problems = append(problems, iid+": beyond-proof intent needs a beyondProofReason")
		}
		if reason != "" && !isReason(reason) {
			problems = append(problems, fmt.Sprintf("%s: unknown beyondProofReason %q", iid, reason))
		}
		useCase := ucID(d.UseCase, d.Change)
		if useCase == "" {
			useCase = d.UseCase
		}
		replaces := nonNil(d.Replaces)
		if d.Change == "remove" && existing != nil {
			replaces = []ProofReplacement{}
			for _, p := range existing.Proofs {
				replaces = append(replaces, ProofReplacement{Proof: p.ID, Reason: "the use case is removed"})
			}
		} else if d.Change == "add" && len(replaces) > 0 {
			problems = append(problems, iid+": an add cannot replace proofs")
		} else {
			for _, r := range replaces {
				owner := findUseCase(useCases, func(u UseCase) bool {
					for _, p := range u.Proofs {
						if p.ID == r.Proof {
							return true
						}
					}
					return false
				})
				if owner == nil {
					problems = append(problems, fmt.Sprintf("%s: replaces unknown proof %s", iid, r.Proof))
				} else if owner.ID != useCase {
					problems = append(problems, fmt.Sprintf("%s: replaces %s, which belongs to %s, not %s", iid, r.Proof, owner.ID, useCase))
				}
			}
		}
		src := SourceSpan{Quote: d.Quote}
		if source != nil {
			src = *source
		}
		intents = append(intents, ProposalIntent{ID: iid, Slug: d.Slug, Name: d.Name, Description: d.Description, Change: d.Change, UseCase: useCase,
			Provability: d.Provability, BeyondProofReason: reason, Source: src, Replaces: replaces, Status: "proposed", Approvals: []string{}})
	}

	// Exactly one UseCaseChange per touched use case; kinds must fit together.
	decisions := []ProposalDecision{}
	changes := []UseCaseChange{}
	touched := []string{}
	for _, i := range intents {
		if !contains(touched, i.UseCase) {
			touched = append(touched, i.UseCase)
		}
	}
	for _, uc := range touched {
		its := []ProposalIntent{}
		for _, i := range intents {
			if i.UseCase == uc {
				its = append(its, i)
			}
		}
		slug := ""
		for _, s := range reservedOrder {
			if reserved[s] == uc {
				slug = s
				break
			}
		}
		if slug == "" {
			if u := findUseCase(useCases, func(u UseCase) bool { return u.ID == uc }); u != nil {
				slug = u.Slug
			} else {
				slug = uc
			}
		}
		var entries []int
		for k, c := range out.UseCases {
			if c.Slug == slug {
				entries = append(entries, k)
			}
		}
		if len(entries) != 1 {
			problems = append(problems, fmt.Sprintf("%s (%s): expected exactly one use-case change, got %d", uc, slug, len(entries)))
			continue
		}
		entry := out.UseCases[entries[0]]
		kinds := []string{}
		iids := []string{}
		for _, i := range its {
			if !contains(kinds, i.Change) {
				kinds = append(kinds, i.Change)
			}
			iids = append(iids, i.ID)
		}
		if len(kinds) > 1 {
			options := []DecisionOption{}
			for _, k := range kinds {
				options = append(options, DecisionOption{Label: k, Consequence: fmt.Sprintf("%s is %s; reject the intents that say otherwise.", uc, changeWord(k))})
			}
			decisions = append(decisions, ProposalDecision{Slug: "change-kind-" + slug, Name: "How " + slug + " changes", Origin: "extraction",
				Question: fmt.Sprintf("The intents on %s (%s) disagree on whether it is added, modified or removed. Which one is meant?", uc, slug),
				Options:  options, Recommended: 0, Intents: iids})
		} else if entry.Change != kinds[0] {
			problems = append(problems, fmt.Sprintf("%s (%s): the use-case change says %s but its intents say %s", uc, slug, entry.Change, kinds[0]))
		}
		existing := findUseCase(useCases, func(u UseCase) bool { return u.ID == uc })
		if entry.Change != "remove" && strings.TrimSpace(entry.After) == "" {
			problems = append(problems, fmt.Sprintf("%s (%s): %s needs the complete after description", uc, slug, entry.Change))
		}
		name := entry.Name
		if name == "" && existing != nil {
			name = existing.Name
		}
		if name == "" {
			name = slug
		}
		c := UseCaseChange{UseCase: uc, Slug: slug, Change: entry.Change, Name: name, Intents: iids}
		if existing != nil && entry.Change != "add" {
			c.Before = existing.Description
		}
		if entry.Change != "remove" {
			after := entry.After
			c.After = &after
		}
		changes = append(changes, c)
	}
	for _, c := range out.UseCases {
		has := false
		for _, x := range changes {
			if x.Slug == c.Slug {
				has = true
			}
		}
		mentioned := false
		for _, p := range problems {
			if strings.Contains(p, "("+c.Slug+")") {
				mentioned = true
			}
		}
		if !has && !mentioned {
			problems = append(problems, fmt.Sprintf("use-case change %q has no intent", c.Slug))
		}
	}

	intentBySlug := map[string]string{}
	for _, i := range intents {
		intentBySlug[i.Slug] = i.ID
	}
	for _, d := range out.Decisions {
		dslug := d.Slug
		if dslug == "" {
			dslug = fmt.Sprintf("decision-%d", len(decisions)+1)
		}
		if len(d.Options) < 2 {
			problems = append(problems, fmt.Sprintf("decision %q needs at least two options", dslug))
		}
		if d.Recommended < 0 || d.Recommended >= len(d.Options) {
			problems = append(problems, fmt.Sprintf("decision %q: recommended out of range", dslug))
		}
		refs := []string{}
		for _, s := range d.Intents {
			if id, ok := intentBySlug[s]; ok {
				refs = append(refs, id)
				continue
			}
			found := false
			for _, i := range intents {
				if i.ID == s {
					found = true
				}
			}
			if found {
				refs = append(refs, s)
			} else {
				problems = append(problems, fmt.Sprintf("decision %q: unknown intent %q", dslug, s))
			}
		}
		name := d.Name
		if name == "" {
			name = d.Question
		}
		decisions = append([]ProposalDecision{{Slug: dslug, Name: name, Origin: "extraction", Question: d.Question, Options: nonNil(d.Options), Recommended: d.Recommended, Intents: refs}}, decisions...)
	}
	if len(intents) == 0 {
		problems = append(problems, "the extraction found no intent")
	}
	if len(problems) > 0 {
		return Proposal{}, &ProposalError{problems}
	}
	for k := range decisions {
		decisions[k].ID = fmt.Sprintf("D%d", k+1)
	}
	return Proposal{
		Lastro: "proposal/v1", ID: id, Slug: out.Slug, Name: out.Name, Description: out.Description, Status: "open",
		SubmittedBy: in.SubmittedBy, Prose: prose, Baseline: BaselineOf(useCases), Intents: intents, UseCases: changes,
		Decisions: decisions, Threads: []Thread{}, NotPromised: nonNil(out.NotPromised), Policy: in.Policy, Proofs: []ProofEntry{}, Supersedes: []string{},
	}, nil
}

// CarryProposalReview carries a previous review over to a fresh extraction of edited prose, like a
// pull request keeps the approvals of untouched files. Same rules as the TS SDK's carryProposalReview.
func CarryProposalReview(previous, next Proposal) Proposal {
	out := clone(next)
	key := func(i ProposalIntent, d string) string { return i.Change + "|" + i.UseCase + "|" + normWS(d) }
	reviewed := []ProposalIntent{}
	for _, i := range previous.Intents {
		if i.Status != "proposed" || len(i.Approvals) > 0 {
			reviewed = append(reviewed, i)
		}
	}
	for k := range out.Intents {
		it := &out.Intents[k]
		var same, amended *ProposalIntent
		for j := range reviewed {
			p := &reviewed[j]
			if same == nil && key(*p, p.Description) == key(*it, it.Description) {
				same = p
			}
			if amended == nil && p.Status == "amended" && p.OriginalDescription != "" && key(*p, p.OriginalDescription) == key(*it, it.Description) && normWS(p.Source.Quote) == normWS(it.Source.Quote) {
				amended = p
			}
		}
		from := same
		if from == nil {
			from = amended
		}
		if from == nil {
			continue
		}
		if amended != nil && same == nil {
			it.OriginalDescription = it.Description
			it.Description = amended.Description
			it.Replaces = clone(amended.Replaces)
		} else if same.OriginalDescription != "" {
			it.OriginalDescription = same.OriginalDescription
		}
		it.Status = from.Status
		it.Approvals = append([]string{}, from.Approvals...)
		if from.ReviewNote != "" {
			it.ReviewNote = from.ReviewNote
		}
	}
	for k := range out.Decisions {
		d := &out.Decisions[k]
		for _, p := range previous.Decisions {
			if normWS(p.Question) == normWS(d.Question) {
				if p.Chosen != nil && *p.Chosen < len(d.Options) {
					c := *p.Chosen
					d.Chosen = &c
					if p.AnsweredBy != "" {
						d.AnsweredBy = p.AnsweredBy
					}
				}
				break
			}
		}
	}
	out.Threads = clone(nonNil(previous.Threads))
	out.Policy = previous.Policy
	out.SubmittedBy = previous.SubmittedBy
	return WithStatus(out)
}

// ---------------------------------------------------------------------------
// Review
// ---------------------------------------------------------------------------

func counted(it ProposalIntent) bool { return it.Status == "approved" || it.Status == "amended" }

// CountedApprovals are the approvals that count under the proposal's policy.
func CountedApprovals(p Proposal, it ProposalIntent) []string {
	out := []string{}
	if !counted(it) {
		return out
	}
	for _, a := range it.Approvals {
		if p.Policy.SubmitterCanApprove || a != p.SubmittedBy {
			out = append(out, a)
		}
	}
	return out
}

// ForwardIntents are the intents that go forward to derivation and implementation.
func ForwardIntents(p Proposal) []ProposalIntent {
	out := []ProposalIntent{}
	for _, i := range p.Intents {
		if counted(i) {
			out = append(out, i)
		}
	}
	return out
}

// LiveChanges are the use-case changes that still apply (at least one intent not rejected).
func LiveChanges(p Proposal) []UseCaseChange {
	live := map[string]bool{}
	for _, i := range ForwardIntents(p) {
		live[i.ID] = true
	}
	out := []UseCaseChange{}
	for _, c := range p.UseCases {
		for _, i := range c.Intents {
			if live[i] {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

func (p Proposal) intentByID(id string) *ProposalIntent {
	for k := range p.Intents {
		if p.Intents[k].ID == id {
			return &p.Intents[k]
		}
	}
	return nil
}

// PendingProposal lists what blocks approval: unreviewed intents, missing approvals, stale use cases, open decisions and threads.
func PendingProposal(p Proposal) []string {
	out := []string{}
	need := p.Policy.Approvals
	if need < 1 {
		need = 1
	}
	for _, it := range p.Intents {
		if it.Status == "rejected" {
			continue
		}
		n := len(CountedApprovals(p, it))
		if it.Status == "proposed" && len(it.Approvals) == 0 {
			out = append(out, it.ID+" not reviewed")
		} else if n < need {
			extra := ""
			if !p.Policy.SubmitterCanApprove {
				extra = " from someone other than the submitter"
			}
			out = append(out, fmt.Sprintf("%s needs %d more approval(s)%s", it.ID, need-n, extra))
		}
	}
	for _, c := range p.UseCases {
		if !c.isStale() {
			continue
		}
		for _, i := range c.Intents {
			if it := p.intentByID(i); it == nil || it.Status != "rejected" {
				out = append(out, c.UseCase+" after is stale: rewrite it or recombine it")
				break
			}
		}
	}
	for _, d := range p.Decisions {
		if d.Chosen == nil {
			out = append(out, d.ID+" unanswered: "+d.Question)
		}
	}
	for _, t := range p.Threads {
		if !t.Resolved {
			out = append(out, t.ID+" open thread on "+t.On)
		}
	}
	return out
}

// reviewCore is the part of a proposal that approval binds.
func reviewCore(p Proposal) any {
	o := obj{{"lastro", p.Lastro}, {"id", p.ID}, {"slug", p.Slug}, {"name", p.Name}, {"description", p.Description}, {"submittedBy", p.SubmittedBy},
		{"prose", p.Prose}, {"baseline", p.Baseline}, {"intents", nonNil(p.Intents)}, {"useCases", nonNil(p.UseCases)}, {"decisions", nonNil(p.Decisions)},
		{"threads", nonNil(p.Threads)}, {"notPromised", nonNil(p.NotPromised)}, {"policy", p.Policy}, {"supersedes", nonNil(p.Supersedes)}}
	if len(p.Origin) > 0 {
		o = append(o, kv{"origin", p.Origin})
	}
	if len(p.Setup) > 0 {
		o = append(o, kv{"setup", p.Setup})
	}
	return o
}

// SupersedesOf lists the proofs the forward intents replace, sorted.
func SupersedesOf(p Proposal) []string {
	out := []string{}
	for _, i := range ForwardIntents(p) {
		for _, r := range i.Replaces {
			if !contains(out, r.Proof) {
				out = append(out, r.Proof)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return utf16Less(out[i], out[j]) })
	return out
}

// IsProposalApproved is true when the reviewed part has not changed since approval.
func IsProposalApproved(p Proposal) bool {
	return p.Hash != "" && mustHash(reviewCore(p)) == p.Hash
}

// WithStatus sets the status that follows from the proposal's content (applied and closed are kept).
func WithStatus(p Proposal) Proposal {
	if p.Status == "applied" || p.Status == "closed" {
		return p
	}
	if IsProposalApproved(p) {
		switch {
		case len(p.Proofs) == 0:
			p.Status = "approved"
		case p.LastRun != nil && p.LastRun.OK && allPassed(p.Proofs):
			p.Status = "proved"
		default:
			p.Status = "proving"
		}
		return p
	}
	p.Hash = ""
	stale, raised := false, false
	for _, c := range p.UseCases {
		stale = stale || c.isStale()
	}
	for _, d := range p.Decisions {
		raised = raised || (d.Origin != "extraction" && d.Chosen == nil)
	}
	if stale || raised {
		p.Status = "changes-requested"
	} else {
		p.Status = "open"
	}
	return p
}

func allPassed(ps []ProofEntry) bool {
	for _, x := range ps {
		if x.Status != "passed" {
			return false
		}
	}
	return true
}

// ProposalReview is one person's review of a proposal. Chain the actions, then call Done.
// The first error is kept and returned by Done.
type ProposalReview struct {
	p        Proposal
	reviewer string
	err      error
}

// ReviewProposal starts one person's review.
func ReviewProposal(p Proposal, reviewer string) *ProposalReview {
	r := &ProposalReview{p: clone(p), reviewer: reviewer}
	if p.Status == "applied" || p.Status == "closed" {
		r.err = fmt.Errorf("%s is %s", p.ID, p.Status)
	} else if reviewer == "" {
		r.err = fmt.Errorf("review needs the reviewer's name")
	}
	return r
}

func (r *ProposalReview) fail(format string, a ...any) *ProposalReview {
	if r.err == nil {
		r.err = fmt.Errorf(format, a...)
	}
	return r
}

func (r *ProposalReview) intent(id string) *ProposalIntent {
	for k := range r.p.Intents {
		it := &r.p.Intents[k]
		if it.ID == id || it.ID == r.p.ID+"."+id || it.Slug == id {
			return it
		}
	}
	r.fail("Unknown intent %s", id)
	return nil
}

func (r *ProposalReview) change(useCase string) *UseCaseChange {
	for k := range r.p.UseCases {
		c := &r.p.UseCases[k]
		if c.UseCase == useCase || c.Slug == useCase {
			return c
		}
	}
	r.fail("%s is not touched by %s", useCase, r.p.ID)
	return nil
}

// Approve signs intents. Approving an intent accepts the replacement of the proofs it lists.
func (r *ProposalReview) Approve(ids ...string) *ProposalReview {
	if r.err != nil {
		return r
	}
	if r.reviewer == r.p.SubmittedBy && !r.p.Policy.SubmitterCanApprove {
		return r.fail("%s submitted %s; under this policy the submitter can amend but not approve", r.reviewer, r.p.ID)
	}
	for _, id := range ids {
		it := r.intent(id)
		if it == nil {
			return r
		}
		if it.Status == "rejected" {
			return r.fail("%s is rejected; amend it to bring it back", it.ID)
		}
		if it.Status == "proposed" {
			it.Status = "approved"
		}
		if !contains(it.Approvals, r.reviewer) {
			it.Approvals = append(it.Approvals, r.reviewer)
		}
	}
	return r
}

// ApproveAll approves every intent that is not rejected.
func (r *ProposalReview) ApproveAll() *ProposalReview {
	ids := []string{}
	for _, i := range r.p.Intents {
		if i.Status != "rejected" {
			ids = append(ids, i.ID)
		}
	}
	return r.Approve(ids...)
}

// Amend rewrites an intent. It needs approval again; its use case's after becomes stale.
func (r *ProposalReview) Amend(id, description string) *ProposalReview {
	it := r.intent(id)
	if r.err != nil {
		return r
	}
	if it.OriginalDescription == "" {
		it.OriginalDescription = it.Description
	}
	it.Description = description
	it.Status = "amended"
	it.ReviewNote = "amended by reviewer (" + r.reviewer + ")"
	it.Approvals = []string{r.reviewer}
	if c := r.change(it.UseCase); c != nil {
		t := true
		c.Stale = &t
	}
	return r
}

// Reject says the intent should not be built. Its use case's after becomes stale.
func (r *ProposalReview) Reject(id, note string) *ProposalReview {
	it := r.intent(id)
	if r.err != nil {
		return r
	}
	it.Status = "rejected"
	it.ReviewNote = note + " (" + r.reviewer + ")"
	it.Approvals = []string{}
	if c := r.change(it.UseCase); c != nil {
		s := false
		for _, i := range c.Intents {
			if x := r.p.intentByID(i); x == nil || x.Status != "rejected" {
				s = true
			}
		}
		c.Stale = &s
	}
	return r
}

// RewriteAfter writes a use case's after by hand (or the LLM's recombination). Its intents need review again.
func (r *ProposalReview) RewriteAfter(useCase, after, name string) *ProposalReview {
	c := r.change(useCase)
	if r.err != nil {
		return r
	}
	if c.Change == "remove" {
		return r.fail("%s is removed; it has no after", c.UseCase)
	}
	c.After = &after
	if name != "" {
		c.Name = name
	}
	c.Stale = nil
	for _, i := range c.Intents {
		it := r.intent(i)
		if it == nil || it.Status == "rejected" {
			continue
		}
		it.Approvals = []string{}
		if it.Status == "approved" {
			it.Status = "proposed"
		}
	}
	return r
}

// Answer answers a decision.
func (r *ProposalReview) Answer(decisionID string, option int) *ProposalReview {
	if r.err != nil {
		return r
	}
	for k := range r.p.Decisions {
		d := &r.p.Decisions[k]
		if d.ID == decisionID || d.Slug == decisionID {
			if option < 0 || option >= len(d.Options) {
				return r.fail("Option %d out of range for %s", option, d.ID)
			}
			c := option
			d.Chosen, d.AnsweredBy = &c, r.reviewer
			return r
		}
	}
	return r.fail("Unknown decision %s", decisionID)
}

// Comment starts a thread on an intent, a proof, a use case or the prose.
func (r *ProposalReview) Comment(on, text string) *ProposalReview {
	if r.err != nil {
		return r
	}
	full := r.p.ID + "." + on
	target := ""
	if on == "prose" {
		target = on
	}
	for _, i := range r.p.Intents {
		if target == "" && (i.ID == on || i.ID == full || i.Slug == on) {
			target = i.ID
		}
	}
	for _, x := range r.p.Proofs {
		if target == "" && (x.ID == on || x.ID == full || x.Slug == on) {
			target = x.ID
		}
	}
	for _, c := range r.p.UseCases {
		if target == "" && (c.UseCase == on || c.Slug == on) {
			target = c.UseCase
		}
	}
	if target == "" {
		return r.fail("Nothing called %s in %s", on, r.p.ID)
	}
	r.p.Threads = append(r.p.Threads, Thread{ID: fmt.Sprintf("T%d", len(r.p.Threads)+1), On: target, By: r.reviewer, Comments: []ThreadComment{{r.reviewer, text}}})
	return r
}

func (r *ProposalReview) thread(id string) *Thread {
	for k := range r.p.Threads {
		if r.p.Threads[k].ID == id {
			return &r.p.Threads[k]
		}
	}
	r.fail("Unknown thread %s", id)
	return nil
}

// Reply adds a comment to a thread.
func (r *ProposalReview) Reply(threadID, text string) *ProposalReview {
	if r.err != nil {
		return r
	}
	if t := r.thread(threadID); t != nil {
		t.Comments = append(t.Comments, ThreadComment{r.reviewer, text})
	}
	return r
}

// Resolve closes a thread. Threads are resolved by whoever started them or by the submitter.
func (r *ProposalReview) Resolve(threadID string) *ProposalReview {
	if r.err != nil {
		return r
	}
	t := r.thread(threadID)
	if t == nil {
		return r
	}
	if r.reviewer != t.By && r.reviewer != r.p.SubmittedBy {
		return r.fail("%s can be resolved by %s or %s", t.ID, t.By, r.p.SubmittedBy)
	}
	t.Resolved, t.ResolvedBy = true, r.reviewer
	return r
}

// Current is the proposal as reviewed so far (for a recombination prompt).
func (r *ProposalReview) Current() Proposal { return clone(r.p) }

// Done ends the review. Any change after approval withdraws it: derived proofs and runs go with it.
func (r *ProposalReview) Done() (Proposal, error) {
	if r.err != nil {
		return Proposal{}, r.err
	}
	out := clone(r.p)
	out.Supersedes = SupersedesOf(out)
	if out.Hash != "" && !IsProposalApproved(out) {
		out.Hash = ""
		out.Proofs = []ProofEntry{}
		out.LastRun = nil
	}
	return WithStatus(out), nil
}

// ApproveProposal approves a fully reviewed proposal. The hash binds the baseline: if the use
// cases changed since extraction, the approval is refused and the prose must be extracted again.
func ApproveProposal(p Proposal, useCases []UseCase) (Proposal, error) {
	if IsProposalApproved(p) {
		return p, nil
	}
	pending := PendingProposal(p)
	if BaselineOf(useCases) != p.Baseline {
		pending = append(pending, fmt.Sprintf("the use cases changed since %s was extracted: run propose again", p.ID))
	}
	if len(pending) > 0 {
		return Proposal{}, &ProposalError{pending}
	}
	out := clone(p)
	out.Hash = ""
	out.Supersedes = SupersedesOf(out)
	out.Proofs = []ProofEntry{}
	out.LastRun = nil
	out.Hash = mustHash(reviewCore(out))
	return WithStatus(out), nil
}

// ---------------------------------------------------------------------------
// Derivation: the LLM's proofs, checked
// ---------------------------------------------------------------------------

// DeriveV1Out is what the derive step returns. Expressions are JSON strings.
type DeriveV1Out struct {
	Proofs []struct {
		Intent      string          `json:"intent"`
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
		Replaces    []string        `json:"replaces"`
	} `json:"proofs"`
	Decisions []struct {
		Slug        string           `json:"slug"`
		Name        string           `json:"name"`
		Question    string           `json:"question"`
		Options     []DecisionOption `json:"options"`
		Recommended int              `json:"recommended"`
		Intents     []string         `json:"intents"`
		Breaks      string           `json:"breaks"`
	} `json:"decisions"`
}

// ProofSetup is what proofs may name: the declared models and subjects.
type ProofSetup struct {
	Models map[string]Model
	// ModelOrder keeps the config's order (for prompts and messages).
	ModelOrder []string
	Subjects   []string
}

// jsonText returns the JSON a string field holds (the LLM writes expressions as JSON strings), or the raw value itself.
func jsonText(raw json.RawMessage) (text string, isString bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, true
	}
	return string(raw), false
}

// ProofsFromDerivation checks the derive step's answer and attaches the pending proofs. When derivation
// returns decisions, the proposal goes back to review (see the TS SDK's proofsFromDerivation).
func ProofsFromDerivation(p Proposal, useCases []UseCase, setup ProofSetup, out DeriveV1Out, opts VerifyOptions) (Proposal, error) {
	if !IsProposalApproved(p) {
		return Proposal{}, &ProposalError{[]string{p.ID + " is not approved"}}
	}
	problems := []string{}
	forward := ForwardIntents(p)
	byID := map[string]ProposalIntent{}
	for _, i := range forward {
		byID[i.ID] = i
	}
	slugs := map[string]bool{}
	proofs := []ProofEntry{}
	for n, d := range out.Proofs {
		pid := p.ID + ".PF-" + pad(n+1, 2)
		it, hasIt := byID[d.Intent]
		if !hasIt {
			problems = append(problems, fmt.Sprintf("%s: intent %s is not an approved or amended intent of %s", pid, d.Intent, p.ID))
		}
		if !SlugPattern.MatchString(d.Slug) {
			problems = append(problems, fmt.Sprintf("%s: slug %q is not kebab-case", pid, d.Slug))
		}
		if slugs[d.Slug] {
			problems = append(problems, fmt.Sprintf("%s: duplicate proof slug %q", pid, d.Slug))
		}
		slugs[d.Slug] = true
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
		for _, r := range d.Replaces {
			listed := false
			if hasIt {
				for _, x := range it.Replaces {
					listed = listed || x.Proof == r
				}
			}
			if !listed {
				problems = append(problems, fmt.Sprintf("%s: replaces %s, which its intent does not list", pid, r))
			}
		}
		if !hasModel {
			continue
		}
		parse := func(where string, raw json.RawMessage) *Expr {
			text, _ := jsonText(raw)
			var e Expr
			if err := json.Unmarshal([]byte(text), &e); err != nil {
				problems = append(problems, fmt.Sprintf("%s.%s: not valid JSON: %s", pid, where, text))
				return nil
			}
			if errs := ValidateExpr(e, model, nil, pid+"."+where); len(errs) > 0 {
				problems = append(problems, errs...)
				return nil
			}
			return &e
		}
		var check *ProofCheck
		switch d.Kind {
		case "clause":
			var requires *Expr
			if text, _ := jsonText(d.Requires); len(d.Requires) > 0 && strings.TrimSpace(text) != "" && text != "null" {
				requires = parse("requires", d.Requires)
			}
			if ensures := parse("ensures", d.Ensures); ensures != nil {
				check = &ProofCheck{Kind: "clause", Requires: requires, Ensures: ensures, Assumptions: nonNil(d.Assumptions)}
			}
		case "scenario":
			text, _ := jsonText(d.InputJSON)
			var input json.RawMessage
			var probe map[string]any
			if err := json.Unmarshal([]byte(text), &probe); err != nil || probe == nil {
				problems = append(problems, pid+".input: not valid JSON")
			} else {
				var buf bytes.Buffer
				_ = json.Compact(&buf, []byte(text))
				input = buf.Bytes()
				for _, k := range model.Inputs.Names() {
					if _, ok := probe[k]; !ok {
						problems = append(problems, fmt.Sprintf("%s.input: missing model input %s", pid, k))
					}
				}
			}
			if expect := parse("expect", d.Expect); input != nil && expect != nil {
				check = &ProofCheck{Kind: "scenario", Input: input, Expect: expect}
			}
		default:
			problems = append(problems, fmt.Sprintf("%s: unknown kind %q", pid, d.Kind))
		}
		if check == nil || !hasIt {
			continue
		}
		pr := ProofEntry{ID: pid, Slug: d.Slug, Name: d.Name, Description: d.Description, Intent: it.ID, UseCase: it.UseCase, Model: d.Model, Subject: d.Subject, Check: *check, Status: "pending"}
		if len(d.Replaces) > 0 {
			pr.Replaces = append([]string{}, d.Replaces...)
		}
		proofs = append(proofs, pr)
	}

	type raisedDecision struct {
		ProposalDecision
		breaks string
	}
	decided := []raisedDecision{}
	for _, d := range out.Decisions {
		kept := []string{}
		for _, x := range d.Intents {
			if _, ok := byID[x]; ok {
				kept = append(kept, x)
			} else {
				problems = append(problems, fmt.Sprintf("decision %q: unknown or rejected intent %s", d.Slug, x))
			}
		}
		if len(d.Options) < 2 {
			problems = append(problems, fmt.Sprintf("decision %q needs at least two options", d.Slug))
		}
		if d.Breaks != "" {
			owner := findUseCase(useCases, func(u UseCase) bool {
				for _, x := range u.Proofs {
					if x.ID == d.Breaks {
						return true
					}
				}
				return false
			})
			if owner == nil {
				problems = append(problems, fmt.Sprintf("decision %q: breaks unknown proof %s", d.Slug, d.Breaks))
			}
		}
		decided = append(decided, raisedDecision{ProposalDecision{Slug: d.Slug, Name: d.Name, Origin: "derivation", Question: d.Question, Options: nonNil(d.Options), Recommended: d.Recommended, Intents: kept}, d.Breaks})
	}
	// Coverage: every forward intent has a proof, unless it is beyond proof or a decision covers it.
	for _, it := range forward {
		covered := false
		for _, x := range proofs {
			covered = covered || x.Intent == it.ID
		}
		for _, d := range decided {
			covered = covered || contains(d.Intents, it.ID)
		}
		if covered {
			continue
		}
		if it.Change == "remove" {
			problems = append(problems, it.ID+": a remove needs a proof that the old behavior is gone, or a decision")
		} else if it.Provability != "beyond-proof" {
			problems = append(problems, it.ID+": no proof derived")
		}
	}
	if len(problems) > 0 {
		return Proposal{}, &ProposalError{problems}
	}

	// Contradictions between the new clauses of one model come back as decisions.
	contradictions := []raisedDecision{}
	for _, g := range plansByGroup(proofs, setup.Models, func(x ProofEntry) string { return x.Model }) {
		cards, err := CheckConsistency(g.plan, opts)
		if err != nil {
			return Proposal{}, err
		}
		for _, c := range cards {
			contradictions = append(contradictions, raisedDecision{ProposalDecision{Slug: fmt.Sprintf("contradiction-%s-%d", g.key, len(contradictions)+1), Name: "Contradiction in " + g.key,
				Origin: "consistency-check", Question: c.Question, Options: c.Options, Recommended: c.Recommended, Intents: nonNil(c.IntentIDs)}, ""})
		}
	}

	next := clone(p)
	raised := append(decided, contradictions...)
	if len(raised) == 0 {
		next.Proofs = proofs
		next.LastRun = nil
		return WithStatus(next), nil
	}
	// Back to review: the approval is withdrawn and no proof is kept.
	for _, d := range raised {
		dup := false
		for _, x := range next.Decisions {
			dup = dup || x.Question == d.Question
		}
		if dup {
			continue
		}
		card := d.ProposalDecision
		card.ID = fmt.Sprintf("D%d", len(next.Decisions)+1)
		next.Decisions = append(next.Decisions, card)
		if d.breaks == "" {
			continue
		}
		for _, iid := range d.Intents {
			it := next.intentByID(iid)
			listed := false
			for _, r := range it.Replaces {
				listed = listed || r.Proof == d.breaks
			}
			if !listed {
				it.Replaces = append(it.Replaces, ProofReplacement{Proof: d.breaks, Reason: d.Question})
			}
			it.Status = "proposed"
			it.Approvals = []string{}
		}
	}
	next.Hash = ""
	next.Proofs = []ProofEntry{}
	next.LastRun = nil
	next.Supersedes = SupersedesOf(next)
	return WithStatus(next), nil
}

// ---------------------------------------------------------------------------
// Proving: the proposal's proofs plus every use case's (regression)
// ---------------------------------------------------------------------------

type planGroup struct {
	key  string
	plan ProofPlan
}

func plansByGroup(proofs []ProofEntry, models map[string]Model, key func(ProofEntry) string) []planGroup {
	out := []planGroup{}
	index := map[string]int{}
	for _, pr := range proofs {
		model, ok := models[pr.Model]
		if !ok {
			continue
		}
		k := key(pr)
		i, seen := index[k]
		if !seen {
			i = len(out)
			index[k] = i
			out = append(out, planGroup{k, ProofPlan{Lastro: "proof-plan/v0", Model: model, Clauses: []Clause{}, Assumptions: []Assumption{}, Scenarios: []Scenario{}, BeyondProof: []BeyondProofItem{}}})
		}
		plan := &out[i].plan
		if pr.Check.Kind == "clause" {
			plan.Clauses = append(plan.Clauses, Clause{ID: pr.ID, IntentIDs: []string{pr.ID}, Description: pr.Description, Requires: pr.Check.Requires, Ensures: *pr.Check.Ensures})
		} else {
			plan.Scenarios = append(plan.Scenarios, Scenario{ID: pr.ID, IntentIDs: []string{pr.ID}, Given: pr.Description, When: "the subject runs", Then: Show(*pr.Check.Expect), Input: pr.Check.InputEnv(), Expect: *pr.Check.Expect})
		}
	}
	return out
}

// artifactFor is a synthetic approved artifact, so the engine can run proofs that are not tied to a v0 artifact.
func artifactFor(plan ProofPlan) IntentArtifact {
	a := IntentArtifact{Lastro: "intent-artifact/v0", Prose: "", Decisions: []DecisionCard{}, NotPromised: []string{}}
	for _, c := range plan.Clauses {
		a.Intents = append(a.Intents, Intent{ID: c.ID, Statement: c.ID, Kind: "behavior", Provability: "formal", Status: "approved"})
	}
	for _, s := range plan.Scenarios {
		a.Intents = append(a.Intents, Intent{ID: s.ID, Statement: s.ID, Kind: "behavior", Provability: "formal", Status: "approved"})
	}
	a.Hash = mustHash(a)
	return a
}

// RunOptions is what proving needs: the code under proof by subject name, the models, and optionally the Rust prover.
type RunOptions struct {
	Subjects map[string]Subject
	Models   map[string]Model
	Rust     RustProver
	Verify   VerifyOptions
}

// RoleProof is a proof and its role in a run.
type RoleProof struct {
	Proof ProofEntry
	Role  string
}

func envJSON(e Env, order []string) json.RawMessage {
	if e == nil {
		return json.RawMessage("null")
	}
	o := obj{}
	used := map[string]bool{}
	for _, k := range order {
		if v, ok := e[k]; ok {
			o = append(o, kv{k, v})
			used[k] = true
		}
	}
	rest := []string{}
	for k := range e {
		if !used[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		o = append(o, kv{k, e[k]})
	}
	b, _ := json.Marshal(o)
	return b
}

// RunProofs runs proofs grouped by (model, subject) on the engine (and the Rust prover, if given).
func RunProofs(ctx context.Context, proofs []RoleProof, o RunOptions) ([]ProofOutcome, *bool, error) {
	outcomes := []ProofOutcome{}
	var agree *bool
	role := map[string]string{}
	all := []ProofEntry{}
	for _, x := range proofs {
		role[x.Proof.ID] = x.Role
		all = append(all, x.Proof)
	}
	runnable := []ProofEntry{}
	for _, pr := range all {
		_, hasModel := o.Models[pr.Model]
		_, hasSubject := o.Subjects[pr.Subject]
		switch {
		case !hasModel:
			outcomes = append(outcomes, ProofOutcome{Proof: pr.ID, UseCase: pr.UseCase, Intent: pr.Intent, Role: role[pr.ID], Verdict: "not-run", Note: "model " + pr.Model + " is not declared"})
		case !hasSubject:
			outcomes = append(outcomes, ProofOutcome{Proof: pr.ID, UseCase: pr.UseCase, Intent: pr.Intent, Role: role[pr.ID], Verdict: "not-run", Note: "subject " + pr.Subject + " is not available"})
		default:
			runnable = append(runnable, pr)
		}
	}
	find := func(id string) ProofEntry {
		for _, x := range runnable {
			if x.ID == id {
				return x
			}
		}
		return ProofEntry{}
	}
	for _, g := range plansByGroup(runnable, o.Models, func(x ProofEntry) string { return x.Model + "\x00" + x.Subject }) {
		subject := o.Subjects[strings.SplitN(g.key, "\x00", 2)[1]]
		plan := g.plan
		artifact := artifactFor(plan)
		plan.ArtifactHash = artifact.Hash
		vo := o.Verify
		vo.Subject, vo.Traces = subject, nil
		report, err := Verify(artifact, plan, vo)
		if err != nil {
			return nil, nil, err
		}
		if o.Rust != nil && len(plan.Clauses) > 0 {
			table, err := Tabulate(plan.Model, subject, 0)
			if err != nil {
				return nil, nil, err
			}
			rust, err := o.Rust.Prove(ctx, plan.Model, plan.Clauses, table)
			if err != nil {
				return nil, nil, err
			}
			same := rustAgrees(report.Clauses, rust)
			a := (agree == nil || *agree) && same
			agree = &a
		}
		outputs := plan.Model.Outputs.Names()
		for _, r := range report.Clauses {
			pr := find(r.ClauseID)
			oc := ProofOutcome{Proof: pr.ID, UseCase: pr.UseCase, Intent: pr.Intent, Role: role[pr.ID], Verdict: r.Verdict, Passed: r.Verdict == "proved" || r.Verdict == "evidenced", Note: r.Note}
			if r.Counterexample != nil {
				oc.Counterexample = &ProofCounterexample{Input: envJSON(r.Counterexample.Input, nil), Output: envJSON(r.Counterexample.Output, outputs)}
			}
			outcomes = append(outcomes, oc)
		}
		for _, r := range report.Scenarios {
			pr := find(r.ScenarioID)
			oc := ProofOutcome{Proof: pr.ID, UseCase: pr.UseCase, Intent: pr.Intent, Role: role[pr.ID], Verdict: "violated", Passed: r.Status == "passed", Note: r.Error}
			if oc.Passed {
				oc.Verdict = "evidenced"
			} else {
				oc.Counterexample = &ProofCounterexample{Input: pr.Check.Input, Output: envJSON(r.Output, outputs)}
			}
			outcomes = append(outcomes, oc)
		}
	}
	order := map[string]int{}
	for i, x := range all {
		order[x.ID] = i
	}
	sort.SliceStable(outcomes, func(i, j int) bool { return order[outcomes[i].Proof] < order[outcomes[j].Proof] })
	return outcomes, agree, nil
}

type resultingUseCase struct {
	id     string
	proofs int
}

func resultingUseCases(p Proposal, useCases []UseCase) []resultingUseCase {
	sup := map[string]bool{}
	for _, s := range p.Supersedes {
		sup[s] = true
	}
	changes := LiveChanges(p)
	own := func(id string) int {
		n := 0
		for _, x := range p.Proofs {
			if x.UseCase == id {
				n++
			}
		}
		return n
	}
	out := []resultingUseCase{}
	for _, u := range useCases {
		removed := false
		for _, c := range changes {
			removed = removed || (c.UseCase == u.ID && c.Change == "remove")
		}
		if removed {
			continue
		}
		n := own(u.ID)
		for _, x := range u.Proofs {
			if !sup[x.ID] {
				n++
			}
		}
		out = append(out, resultingUseCase{u.ID, n})
	}
	for _, c := range changes {
		if c.Change == "add" {
			out = append(out, resultingUseCase{c.UseCase, own(c.UseCase)})
		}
	}
	return out
}

// ProveProposal runs the proposal's proofs and every use case's proofs except the superseded ones (a
// failing one is a regression). Superseded proofs run too, only to note the ones that still pass.
func ProveProposal(ctx context.Context, p Proposal, useCases []UseCase, o RunOptions) (Proposal, ProposalRun, error) {
	if !IsProposalApproved(p) {
		return Proposal{}, ProposalRun{}, &ProposalError{[]string{p.ID + " is not approved"}}
	}
	if len(p.Proofs) == 0 {
		for _, i := range ForwardIntents(p) {
			if i.Provability != "beyond-proof" {
				return Proposal{}, ProposalRun{}, &ProposalError{[]string{p.ID + " has no proofs: run derive first"}}
			}
		}
	}
	sup := map[string]bool{}
	for _, s := range p.Supersedes {
		sup[s] = true
	}
	removed := map[string]bool{}
	for _, c := range LiveChanges(p) {
		if c.Change == "remove" {
			removed[c.UseCase] = true
		}
	}
	set := []RoleProof{}
	for _, x := range p.Proofs {
		set = append(set, RoleProof{x, "proposal"})
	}
	for _, u := range useCases {
		for _, x := range u.Proofs {
			if !sup[x.ID] && !removed[x.UseCase] {
				set = append(set, RoleProof{x, "regression"})
			}
		}
	}
	for _, u := range useCases {
		for _, x := range u.Proofs {
			if sup[x.ID] {
				set = append(set, RoleProof{x, "superseded"})
			}
		}
	}
	outcomes, agree, err := RunProofs(ctx, set, o)
	if err != nil {
		return Proposal{}, ProposalRun{}, err
	}
	run := ProposalRun{Outcomes: outcomes, Regressions: []string{}, ReplacedStillPass: []string{}, Unproved: []string{}, EnginesAgree: agree}
	ok := true
	for _, x := range outcomes {
		switch {
		case x.Role == "regression" && !x.Passed:
			run.Regressions = append(run.Regressions, x.Proof)
		case x.Role == "superseded" && x.Passed:
			run.ReplacedStillPass = append(run.ReplacedStillPass, x.Proof)
		case x.Role == "proposal" && !x.Passed:
			ok = false
		}
	}
	for _, u := range resultingUseCases(p, useCases) {
		if u.proofs == 0 {
			run.Unproved = append(run.Unproved, u.id)
		}
	}
	run.OK = ok && len(run.Regressions) == 0 && (agree == nil || *agree)
	next := clone(p)
	for k := range next.Proofs {
		pr := &next.Proofs[k]
		for _, r := range outcomes {
			if r.Role == "proposal" && r.Proof == pr.ID {
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
				break
			}
		}
	}
	next.LastRun = &run
	return WithStatus(next), run, nil
}

// Regress runs every use case's proofs alone (`regress`).
func Regress(ctx context.Context, useCases []UseCase, o RunOptions) (ProposalRun, error) {
	set := []RoleProof{}
	for _, u := range useCases {
		for _, x := range u.Proofs {
			set = append(set, RoleProof{x, "regression"})
		}
	}
	outcomes, agree, err := RunProofs(ctx, set, o)
	if err != nil {
		return ProposalRun{}, err
	}
	run := ProposalRun{Outcomes: outcomes, Regressions: []string{}, ReplacedStillPass: []string{}, Unproved: []string{}, EnginesAgree: agree}
	for _, x := range outcomes {
		if !x.Passed {
			run.Regressions = append(run.Regressions, x.Proof)
		}
	}
	for _, u := range useCases {
		if len(u.Proofs) == 0 {
			run.Unproved = append(run.Unproved, u.ID)
		}
	}
	run.OK = len(run.Regressions) == 0 && (agree == nil || *agree)
	return run, nil
}

// ---------------------------------------------------------------------------
// Apply
// ---------------------------------------------------------------------------

// Applied is the result of ApplyProposal.
type Applied struct {
	Proposal Proposal
	// UseCases is the whole catalog after the change.
	UseCases []UseCase
	// Written are use cases whose files must be written; Removed, ids whose files must be deleted.
	Written  []string
	Removed  []string
	Baseline string
}

// ApplyProposal attaches the proofs and syncs the use cases. It refuses unless the proposal is approved,
// its baseline still matches and its last run passed every proof, new and regression.
func ApplyProposal(p Proposal, useCases []UseCase) (Applied, error) {
	problems := []string{}
	if !IsProposalApproved(p) {
		problems = append(problems, p.ID+" is not approved")
	}
	if BaselineOf(useCases) != p.Baseline {
		problems = append(problems, fmt.Sprintf("the use cases changed since %s was extracted: run propose again", p.ID))
	}
	if p.LastRun == nil {
		problems = append(problems, p.ID+" has not been proved: run prove first")
	} else if !p.LastRun.OK {
		failed := []string{}
		for _, x := range p.LastRun.Outcomes {
			if x.Role == "proposal" && !x.Passed {
				failed = append(failed, x.Proof)
			}
		}
		failed = append(failed, p.LastRun.Regressions...)
		problems = append(problems, fmt.Sprintf("%s's last run failed: %s", p.ID, strings.Join(failed, ", ")))
	}
	for _, pr := range p.Proofs {
		if pr.Status != "passed" {
			problems = append(problems, pr.ID+" is "+pr.Status)
		}
	}
	if len(problems) > 0 {
		return Applied{}, &ProposalError{problems}
	}
	sup := map[string]bool{}
	for _, s := range p.Supersedes {
		sup[s] = true
	}
	out := clone(nonNil(useCases))
	written, removed := []string{}, []string{}
	attach := func(u *UseCase) {
		kept := []ProofEntry{}
		for _, x := range u.Proofs {
			if !sup[x.ID] {
				kept = append(kept, x)
			}
		}
		for _, x := range p.Proofs {
			if x.UseCase == u.ID {
				kept = append(kept, clone(x))
			}
		}
		u.Proofs = kept
		if !contains(u.History, p.ID) {
			u.History = append(nonNil(u.History), p.ID)
		}
		written = append(written, u.ID)
	}
	for _, c := range LiveChanges(p) {
		i := -1
		for k, u := range out {
			if u.ID == c.UseCase {
				i = k
			}
		}
		switch c.Change {
		case "remove":
			if i >= 0 {
				out = append(out[:i], out[i+1:]...)
			}
			removed = append(removed, c.UseCase)
		case "add":
			u := UseCase{ID: c.UseCase, Slug: c.Slug, Name: c.Name, Description: c.after(), Proofs: []ProofEntry{}, History: []string{}}
			attach(&u)
			out = append(out, u)
		default:
			out[i].Name = c.Name
			out[i].Description = c.after()
			attach(&out[i])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	next := clone(p)
	next.Status = "applied"
	return Applied{Proposal: next, UseCases: out, Written: written, Removed: removed, Baseline: BaselineOf(out)}, nil
}

// ---------------------------------------------------------------------------
// Feedback and receipt
// ---------------------------------------------------------------------------

func formalOf(pr ProofEntry) string {
	if pr.Check.Kind == "clause" {
		s := ""
		if pr.Check.Requires != nil {
			s = "requires " + Show(*pr.Check.Requires) + "; "
		}
		return s + "ensures " + Show(*pr.Check.Ensures)
	}
	return "input " + Stringify(pr.Check.Input) + "; expect " + Show(*pr.Check.Expect)
}

func findProof(all []ProofEntry, id string) *ProofEntry {
	for k := range all {
		if all[k].ID == id {
			return &all[k]
		}
	}
	return nil
}

func allProofs(p Proposal, useCases []UseCase) []ProofEntry {
	all := append([]ProofEntry{}, p.Proofs...)
	for _, u := range useCases {
		all = append(all, u.Proofs...)
	}
	return all
}

// ProposalFeedback renders the last run's failures as a brief for whoever writes the code.
func ProposalFeedback(p Proposal, useCases []UseCase) string {
	run := p.LastRun
	if run == nil {
		return p.ID + " has not been proved yet."
	}
	all := allProofs(p, useCases)
	failing := []ProofOutcome{}
	for _, x := range run.Outcomes {
		if x.Role != "superseded" && !x.Passed {
			failing = append(failing, x)
		}
	}
	if len(failing) == 0 {
		s := "Every proof passes."
		if len(run.Unproved) > 0 {
			s += " Use cases with no proofs yet: " + strings.Join(run.Unproved, ", ") + "."
		}
		return s
	}
	lines := []string{"The implementation breaks these proofs. Fix the code so each holds, without breaking the others:"}
	for _, f := range failing {
		pr := findProof(all, f.Proof)
		what := f.Intent
		if f.Role == "regression" {
			what = "regression in " + f.UseCase
		}
		desc, formal := "", ""
		if pr != nil {
			desc, formal = pr.Description, formalOf(*pr)
		}
		line := fmt.Sprintf("- %s (%s): %s\n  %s", f.Proof, what, desc, formal)
		if f.Counterexample != nil {
			line += fmt.Sprintf("\n  counterexample: %s returned %s", Stringify(f.Counterexample.Input), Stringify(f.Counterexample.Output))
		} else if f.Note != "" {
			line += "\n  " + f.Note
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

var verdictMark = map[string]string{"proved": "✅ proved", "evidenced": "🟡 evidenced", "violated": "❌ violated", "vacuous": "⚠️ vacuous", "weak": "⚠️ weak", "not-run": "⏸ not run"}

// RenderProposalReceipt is the Markdown receipt: the use-case diff, proofs gained and replaced, regressions.
// Same text as the TS SDK's renderProposalReceipt.
func RenderProposalReceipt(p Proposal, useCases []UseCase) string {
	approval := ""
	if p.Hash != "" {
		approval = " · approval `" + p.Hash + "`"
	}
	L := []string{"# Receipt: " + p.ID + " " + p.Name, "", fmt.Sprintf("Status: **%s** · submitted by %s · baseline `%s`%s", p.Status, p.SubmittedBy, p.Baseline, approval), "", p.Description, ""}
	run := p.LastRun
	all := allProofs(p, useCases)
	L = append(L, "## Use cases", "")
	for _, c := range p.UseCases {
		its := []ProposalIntent{}
		live := 0
		for _, i := range p.Intents {
			if contains(c.Intents, i.ID) {
				its = append(its, i)
				if i.Status != "rejected" {
					live++
				}
			}
		}
		mark := "✏️"
		if c.Change == "add" {
			mark = "➕"
		} else if c.Change == "remove" {
			mark = "➖"
		}
		rejected := ""
		if live == 0 {
			rejected = ", every intent rejected"
		}
		L = append(L, fmt.Sprintf("### %s %s %s (%s%s)", mark, c.UseCase, c.Name, c.Change, rejected), "")
		if c.Before != "" {
			L = append(L, "- Before: "+c.Before)
		}
		if c.After != nil && *c.After != "" && c.Change != "remove" {
			L = append(L, "- After: "+*c.After)
		}
		for _, i := range its {
			L = append(L, fmt.Sprintf("- **%s** `%s` %s", i.ID, i.Status, i.Description))
			for _, r := range i.Replaces {
				old := ""
				if o := findProof(all, r.Proof); o != nil {
					old = " (" + o.Description + ")"
				}
				by := []string{}
				for _, x := range p.Proofs {
					if contains(x.Replaces, r.Proof) {
						by = append(by, x.ID)
					}
				}
				guarded := ""
				if len(by) > 0 {
					guarded = "; now guarded by " + strings.Join(by, ", ")
				}
				L = append(L, fmt.Sprintf("  - replaces %s%s: %s%s", r.Proof, old, r.Reason, guarded))
			}
		}
		gained := []string{}
		for _, x := range p.Proofs {
			if x.UseCase == c.UseCase {
				gained = append(gained, x.ID)
			}
		}
		if len(gained) > 0 {
			L = append(L, "- Gains: "+strings.Join(gained, ", "))
		}
		L = append(L, "")
	}
	outcome := func(id string) *ProofOutcome {
		if run == nil {
			return nil
		}
		for k := range run.Outcomes {
			if run.Outcomes[k].Proof == id {
				return &run.Outcomes[k]
			}
		}
		return nil
	}
	if len(p.Proofs) > 0 {
		L = append(L, "## Proofs", "")
		for _, pr := range p.Proofs {
			r := outcome(pr.ID)
			v := pr.Status
			if r != nil {
				v = verdictMark[r.Verdict]
			}
			L = append(L, fmt.Sprintf("- **%s** %s — %s (%s, %s/%s)", pr.ID, v, pr.Description, pr.Intent, pr.Model, pr.Subject), "  - `"+formalOf(pr)+"`")
			if r != nil && r.Counterexample != nil {
				L = append(L, fmt.Sprintf("  - Counterexample: `%s` → `%s`", Stringify(r.Counterexample.Input), Stringify(r.Counterexample.Output)))
			}
			if r != nil && r.Note != "" {
				L = append(L, "  - Note: "+r.Note)
			}
		}
		L = append(L, "")
	}
	if run != nil {
		reg := 0
		for _, x := range run.Outcomes {
			if x.Role == "regression" {
				reg++
			}
		}
		engines := ""
		if run.EnginesAgree != nil {
			engines = " Engines agree: NO."
			if *run.EnginesAgree {
				engines = " Engines agree: yes."
			}
		}
		L = append(L, "## Regression", "", fmt.Sprintf("%d use-case proof(s) ran; %d failed.%s", reg, len(run.Regressions), engines), "")
		for _, id := range run.Regressions {
			r := outcome(id)
			detail := ""
			if r.Counterexample != nil {
				detail = fmt.Sprintf(": `%s` → `%s`", Stringify(r.Counterexample.Input), Stringify(r.Counterexample.Output))
			} else if r.Note != "" {
				detail = ": " + r.Note
			}
			L = append(L, fmt.Sprintf("- ❌ **%s** (%s)%s", id, r.UseCase, detail))
		}
		for _, id := range run.ReplacedStillPass {
			L = append(L, fmt.Sprintf("- ⚠️ **%s** is superseded but still passes: the behavior it guarded may not have changed.", id))
		}
		if len(run.Unproved) > 0 {
			L = append(L, "- Unproved use cases (no proofs yet): "+strings.Join(run.Unproved, ", "))
		}
		L = append(L, "")
	}
	if len(p.Decisions) > 0 {
		L = append(L, "## Decisions", "")
		for _, d := range p.Decisions {
			answer := "unanswered"
			if d.Chosen != nil {
				answer = d.Options[*d.Chosen].Label
				if d.AnsweredBy != "" {
					answer += " (" + d.AnsweredBy + ")"
				}
			}
			L = append(L, fmt.Sprintf("- **%s** %s → %s", d.ID, d.Question, answer))
		}
		L = append(L, "")
	}
	if len(p.NotPromised) > 0 {
		L = append(L, "## Not promised", "")
		for _, n := range p.NotPromised {
			L = append(L, "- "+n)
		}
		L = append(L, "")
	}
	return strings.Join(L, "\n")
}
